// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const defaultAttachmentCompressionWorkers = 4

const attachmentCompressionSpoolThreshold = 4 * 1024 * 1024

// MaxAttachmentBytes is the default largest attachment content accepted for
// embedding.
const MaxAttachmentBytes = 64 * 1024 * 1024

// AttachmentOptions controls file-backed attachment validation.
type AttachmentOptions struct {
	MaxBytes int64 // Maximum file size; 0 uses MaxAttachmentBytes.
	Eager    bool  // Validate file existence and size before returning.
}

// AttachmentLoader opens attachment content for output. The returned size may
// be -1 when unknown. GoPDFKit still reads the stream into memory before
// embedding in v0.9.x, so loader implementations must be bounded by policy.
type AttachmentLoader interface {
	OpenAttachment(ctx context.Context) (io.ReadCloser, int64, error)
}

// AttachmentLoaderFunc adapts a function into an AttachmentLoader.
type AttachmentLoaderFunc func(context.Context) (io.ReadCloser, int64, error)

// OpenAttachment calls f(ctx).
func (f AttachmentLoaderFunc) OpenAttachment(ctx context.Context) (io.ReadCloser, int64, error) {
	if f == nil {
		return nil, 0, fmt.Errorf("attachment loader is nil")
	}
	return f(ctx)
}

// Attachment defines content to include in the PDF in one of the following ways:
//   - associated with the document as a whole; see SetAttachments()
//   - accessible through a link anchored on a page; see AddAttachmentAnnotation()
type Attachment struct {
	// Content contains the bytes embedded in the PDF.
	Content []byte

	// FilePath optionally names a file to load as Content during output. When
	// Filename is empty, the base name of FilePath is used.
	FilePath string

	// Filename is the displayed name of the attachment.
	Filename string

	// Description is only displayed when using AddAttachmentAnnotation(),
	// and might be modified by the PDF reader.
	Description string

	// MIMEType is written to the embedded file stream /Subtype. When empty,
	// it is inferred from Filename and falls back to application/octet-stream.
	MIMEType string

	// AFRelationship describes why the file is associated with the document.
	// Common PDF/A-4f values are Source, Data, Alternative, Supplement, and
	// Unspecified. When empty, Data is used.
	AFRelationship string

	// Loader optionally opens attachment content during output. Content takes
	// precedence over Loader, and FilePath takes precedence over Loader.
	Loader AttachmentLoader

	objectNumber   int    // Filled when the filespec is embedded.
	mimeType       string // Normalized lazily or when cloned.
	afRelationship string // Normalized lazily or when cloned.
	checksum       string // PDF MD5 checksum, computed once per attachment content.
	maxBytes       int64  // Per-attachment maximum content size; 0 uses document/default limit.
	contentSize    int    // Size of the attachment content once known.
	contentReady   bool   // Whether checksum/contentSize describe loaded content.
}

type attachmentStreamKey struct {
	size     int
	checksum string
	mimeType string
}

type attachmentFileKey struct {
	stream       attachmentStreamKey
	filename     string
	description  string
	relationship string
}

type attachmentStream struct {
	data     []byte
	tempFile string
	size     int
}

// checksum returns the hex-encoded PDF attachment checksum of data.
func attachmentChecksum(data []byte) string {
	tmp := md5.Sum(data)
	return hex.EncodeToString(tmp[:])
}

// writeCompressedFileObject writes a deflate-compressed /EmbeddedFile object
// with length, compressed length, and MD5 checksum metadata.
func (f *Document) writeCompressedFileObject(streamKey attachmentStreamKey, content []byte) {
	compressed, ok := f.ensureResourceStore().compressedAttachment(streamKey)
	if !ok {
		if content == nil && streamKey.size > 0 {
			f.SetErrorf("attachment content is unavailable")
			return
		}
		data := f.compressBytes(content)
		if f.err != nil {
			return
		}
		compressed = attachmentStream{data: data, size: len(data)}
	}
	if f.err != nil {
		return
	}
	f.newPDFDictObject()
	f.outf("/Type /EmbeddedFile /Subtype /%s", escapePDFName(streamKey.mimeType))
	f.outf("/Length %d /Filter /FlateDecode", compressed.size)
	f.out("/Params")
	f.beginPDFDict()
	f.outf("/CheckSum <%s> /Size %d", streamKey.checksum, streamKey.size)
	f.endPDFDict()
	f.endPDFDict()
	f.putAttachmentStream(compressed)
	f.endPDFObject()
}

func (f *Document) putAttachmentStream(stream attachmentStream) {
	if f.err != nil {
		return
	}
	if stream.tempFile == "" {
		f.putstream(stream.data)
		return
	}
	file, err := os.Open(stream.tempFile)
	if err != nil {
		f.SetError(err)
		return
	}
	defer func() { _ = file.Close() }()
	if f.protect.encrypted {
		data, err := io.ReadAll(file)
		if err != nil {
			f.SetError(err)
			return
		}
		f.putstream(data)
		return
	}
	f.out("stream")
	if err := f.outbuf(file); err != nil {
		return
	}
	f.out("endstream")
}

type attachmentCompressionTask struct {
	attachment *Attachment
	mimeType   string
	content    []byte
}

func (f *Document) prepareAttachmentCompressionContext(ctx context.Context) {
	if f.err != nil {
		return
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return
	}
	tasks := f.attachmentCompressionTasksContext(ctx)
	if len(tasks) == 0 {
		return
	}
	workers := f.attachmentCompressionWorkers
	if workers < 0 {
		workers = defaultAttachmentCompressionWorkers
	}
	if workers == 0 {
		return
	}
	if workers > len(tasks) {
		workers = len(tasks)
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan attachmentCompressionTask)
	type result struct {
		attachment *Attachment
		mimeType   string
		checksum   string
		stream     attachmentStream
		size       int
		err        error
	}
	results := make(chan result, len(tasks))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				if ctx.Err() != nil {
					return
				}
				stream, checksum, err := compressAttachmentWithChecksum(task.content, f.compressLevel, attachmentShouldSpool(task.attachment, task.content))
				select {
				case results <- result{attachment: task.attachment, mimeType: task.mimeType, checksum: checksum, stream: stream, size: len(task.content), err: err}:
				case <-ctx.Done():
					stream.cleanup()
					return
				}
			}
		}()
	}
schedule:
	for _, task := range tasks {
		select {
		case jobs <- task:
		case <-ctx.Done():
			break schedule
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	if err := outputCanceledError(ctx); err != nil {
		for result := range results {
			result.stream.cleanup()
		}
		f.SetError(err)
		return
	}
	for result := range results {
		if result.err != nil {
			result.stream.cleanup()
			for remaining := range results {
				remaining.stream.cleanup()
			}
			f.SetError(result.err)
			return
		}
		if result.attachment.checksum == "" {
			result.attachment.checksum = result.checksum
		}
		result.attachment.contentSize = result.size
		result.attachment.contentReady = true
		key := attachmentStreamKey{size: result.size, checksum: result.attachment.checksum, mimeType: result.mimeType}
		if !f.ensureResourceStore().addCompressedAttachment(key, result.stream) {
			result.stream.cleanup()
		}
		if attachmentShouldReleaseLoadedContent(result.attachment, result.size) {
			result.attachment.Content = nil
		}
	}
}

func (f *Document) attachmentCompressionTasksContext(ctx context.Context) []attachmentCompressionTask {
	seen := make(map[*Attachment]bool)
	tasks := make([]attachmentCompressionTask, 0, len(f.attachments))
	add := func(a *Attachment) {
		if a == nil || seen[a] {
			return
		}
		if outputCanceledError(ctx) != nil {
			return
		}
		if !f.loadAttachmentContentContext(ctx, a) {
			return
		}
		a.mimeType = attachmentMIMEType(*a)
		a.afRelationship = attachmentAFRelationship(*a)
		if a.checksum != "" {
			resources := f.ensureResourceStore()
			key := attachmentStreamKey{size: attachmentContentSize(*a), checksum: a.checksum, mimeType: a.mimeType}
			if resources.attachmentStreamObject(key) != 0 {
				return
			}
			if _, ok := resources.compressedAttachment(key); ok {
				return
			}
		}
		seen[a] = true
		tasks = append(tasks, attachmentCompressionTask{attachment: a, mimeType: a.mimeType, content: a.Content})
	}
	for i := range f.attachments {
		add(&f.attachments[i])
	}
	for _, annotations := range f.pageAttachments {
		for _, an := range annotations {
			add(an.Attachment)
		}
	}
	return tasks
}

func compressAttachmentWithChecksum(content []byte, level int, spool bool) (attachmentStream, string, error) {
	if spool {
		return compressAttachmentWithChecksumFile(content, level)
	}
	if !validCompressionLevel(level) {
		level = zlib.BestSpeed
	}
	buf := compressBufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	sum := md5.New()
	list := zlibFreeList(level)
	cmp, err := pooledZlibWriter(list, buf, level)
	if err != nil {
		releaseCompressBuffer(buf)
		return attachmentStream{}, "", err
	}
	if _, err = io.MultiWriter(cmp, sum).Write(content); err != nil {
		_ = cmp.Close()
		releaseZlibWriter(list, cmp)
		releaseCompressBuffer(buf)
		return attachmentStream{}, "", err
	}
	if err = cmp.Close(); err != nil {
		releaseZlibWriter(list, cmp)
		releaseCompressBuffer(buf)
		return attachmentStream{}, "", err
	}
	releaseZlibWriter(list, cmp)
	checksum := hex.EncodeToString(sum.Sum(nil))
	if buf.Len() >= largeCompressedStreamNoCopyThreshold {
		return attachmentStream{data: buf.Bytes(), size: buf.Len()}, checksum, nil
	}
	defer releaseCompressBuffer(buf)
	data := append([]byte(nil), buf.Bytes()...)
	return attachmentStream{data: data, size: len(data)}, checksum, nil
}

func compressAttachmentWithChecksumFile(content []byte, level int) (attachmentStream, string, error) {
	if !validCompressionLevel(level) {
		level = zlib.BestSpeed
	}
	file, err := os.CreateTemp("", "gopdfkit-attachment-*.z")
	if err != nil {
		return attachmentStream{}, "", err
	}
	path := file.Name()
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	sum := md5.New()
	cmp, err := zlib.NewWriterLevel(file, level)
	if err != nil {
		return attachmentStream{}, "", err
	}
	if _, err = io.MultiWriter(cmp, sum).Write(content); err != nil {
		_ = cmp.Close()
		return attachmentStream{}, "", err
	}
	if err = cmp.Close(); err != nil {
		return attachmentStream{}, "", err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return attachmentStream{}, "", err
	}
	info, err := file.Stat()
	if err != nil {
		return attachmentStream{}, "", err
	}
	cleanup = false
	return attachmentStream{tempFile: path, size: int(info.Size())}, hex.EncodeToString(sum.Sum(nil)), nil
}

func (s attachmentStream) cleanup() {
	if s.tempFile != "" {
		_ = os.Remove(s.tempFile)
	}
}

func attachmentShouldSpool(a *Attachment, content []byte) bool {
	return a != nil && (a.FilePath != "" || a.Loader != nil) && len(content) >= attachmentCompressionSpoolThreshold
}

func attachmentShouldReleaseLoadedContent(a *Attachment, size int) bool {
	return a != nil && (a.FilePath != "" || a.Loader != nil) && size >= attachmentCompressionSpoolThreshold
}

func (f *Document) embedContext(ctx context.Context, a *Attachment) {
	if a == nil {
		f.SetErrorf("attachment is nil")
		return
	}
	if a.objectNumber != 0 { // Already embedded; object numbers start at 2.
		return
	}
	if !a.contentReady && !f.loadAttachmentContentContext(ctx, a) {
		return
	}
	normalizeAttachment(a)
	streamKey := attachmentStreamKey{
		size:     attachmentContentSize(*a),
		checksum: a.checksum,
		mimeType: a.mimeType,
	}
	fileKey := attachmentFileKey{
		stream:       streamKey,
		filename:     a.Filename,
		description:  a.Description,
		relationship: a.afRelationship,
	}
	resources := f.ensureResourceStore()
	if objectNumber := resources.attachmentFileObject(fileKey); objectNumber != 0 {
		a.objectNumber = objectNumber
		return
	}
	oldState := f.state
	f.state = 1 // Write file content to the main buffer.
	streamID := resources.attachmentStreamObject(streamKey)
	if streamID == 0 {
		f.writeCompressedFileObject(streamKey, a.Content)
		if f.err != nil {
			f.state = oldState
			return
		}
		streamID = f.n
		resources.setAttachmentStreamObject(streamKey, streamID)
	}
	f.newPDFDictObject()
	f.out("/Type /Filespec /F ()")
	fileSpec := make([]byte, 0, len(a.Filename)*2+len(a.Description)*2+16)
	fileSpec = append(fileSpec, "/UF "...)
	fileSpec = f.appendUTF16TextString(fileSpec, a.Filename)
	f.outbytes(fileSpec)
	f.outf("/AFRelationship /%s", a.afRelationship)
	f.out("/EF")
	f.beginPDFDict()
	f.outf("/F %d 0 R", streamID)
	f.endPDFDict()
	fileSpec = fileSpec[:0]
	fileSpec = append(fileSpec, "/Desc "...)
	fileSpec = f.appendUTF16TextString(fileSpec, a.Description)
	f.outbytes(fileSpec)
	f.endPDFDict()
	f.endPDFObject()
	a.objectNumber = f.n
	resources.setAttachmentFileObject(fileKey, a.objectNumber)
	f.state = oldState
}

func attachmentMIMEType(a Attachment) string {
	if a.mimeType != "" {
		return a.mimeType
	}
	if strings.TrimSpace(a.MIMEType) != "" {
		return strings.TrimSpace(a.MIMEType)
	}
	if ext := filepath.Ext(a.Filename); ext != "" {
		if typ := mime.TypeByExtension(ext); typ != "" {
			if base, _, ok := strings.Cut(typ, ";"); ok {
				return strings.TrimSpace(base)
			}
			return strings.TrimSpace(typ)
		}
	}
	if a.Filename == "" && a.FilePath != "" {
		if ext := filepath.Ext(a.FilePath); ext != "" {
			if typ := mime.TypeByExtension(ext); typ != "" {
				if base, _, ok := strings.Cut(typ, ";"); ok {
					return strings.TrimSpace(base)
				}
				return strings.TrimSpace(typ)
			}
		}
	}
	return "application/octet-stream"
}

func attachmentAFRelationship(a Attachment) string {
	if a.afRelationship != "" {
		return a.afRelationship
	}
	switch strings.TrimSpace(a.AFRelationship) {
	case "Source", "Data", "Alternative", "Supplement", "Unspecified":
		return strings.TrimSpace(a.AFRelationship)
	default:
		return "Data"
	}
}

func normalizeAttachment(a *Attachment) {
	if a == nil {
		return
	}
	if strings.TrimSpace(a.Filename) == "" && strings.TrimSpace(a.FilePath) != "" {
		a.Filename = filepath.Base(a.FilePath)
	}
	a.mimeType = attachmentMIMEType(*a)
	a.afRelationship = attachmentAFRelationship(*a)
	if a.checksum == "" && attachmentHasInlineContent(*a) {
		a.checksum = attachmentChecksum(a.Content)
	}
	if !a.contentReady && attachmentHasInlineContent(*a) {
		a.contentSize = len(a.Content)
		a.contentReady = true
	}
}

func attachmentHasInlineContent(a Attachment) bool {
	return a.Content != nil || (strings.TrimSpace(a.FilePath) == "" && a.Loader == nil)
}

func (f *Document) loadAttachmentContentContext(ctx context.Context, a *Attachment) bool {
	if a == nil {
		return true
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return false
	}
	filePath := strings.TrimSpace(a.FilePath)
	hasInlineContent := attachmentHasInlineContent(*a)
	if filePath != "" {
		if err := f.requireSecurityFeature("file-backed attachments", f.securityPolicy.AllowFileAttachments); err != nil {
			return false
		}
	}
	if a.Loader != nil && filePath == "" && !hasInlineContent {
		if err := f.requireSecurityFeature("attachment loaders", f.securityPolicy.AllowFileAttachments); err != nil {
			return false
		}
	}
	limit := f.attachmentMaxBytes(*a)
	if hasInlineContent && limit >= 0 && int64(len(a.Content)) > limit {
		f.SetError(fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge))
		return false
	}
	if hasInlineContent || filePath == "" {
		if !hasInlineContent && a.Loader != nil {
			data, err := readAttachmentLoaderLimitContext(ctx, a.Loader, limit)
			if err != nil {
				f.SetError(err)
				return false
			}
			a.Content = data
			if f.hooks.OnAttachmentLoaded != nil {
				f.hooks.OnAttachmentLoaded(a.Filename, int64(len(data)))
			}
			if a.checksum == "" {
				a.checksum = attachmentChecksum(data)
			}
			a.contentSize = len(data)
			a.contentReady = true
		}
		if !a.contentReady {
			a.contentSize = len(a.Content)
			a.contentReady = true
		}
		return true
	}
	data, err := f.readAttachmentFileLimitContext(ctx, a.FilePath, limit)
	if err != nil {
		f.SetError(err)
		return false
	}
	a.Content = data
	if strings.TrimSpace(a.Filename) == "" {
		a.Filename = filepath.Base(a.FilePath)
	}
	if f.hooks.OnAttachmentLoaded != nil {
		f.hooks.OnAttachmentLoaded(a.Filename, int64(len(data)))
	}
	if a.checksum == "" {
		a.checksum = attachmentChecksum(data)
	}
	a.contentSize = len(data)
	a.contentReady = true
	return true
}

func attachmentContentSize(a Attachment) int {
	if a.contentReady {
		return a.contentSize
	}
	return len(a.Content)
}

func (f *Document) attachmentMaxBytes(a Attachment) int64 {
	if a.maxBytes > 0 {
		return a.maxBytes
	}
	if f.maxAttachmentBytes > 0 {
		return f.maxAttachmentBytes
	}
	return MaxAttachmentBytes
}

// SetMaxAttachmentBytes sets the maximum content size accepted for attachments
// embedded by this document. Passing zero restores the package default.
func (f *Document) SetMaxAttachmentBytes(maxBytes int64) {
	if maxBytes < 0 {
		f.SetErrorf("invalid max attachment bytes: %d", maxBytes)
		return
	}
	if maxBytes == 0 {
		maxBytes = MaxAttachmentBytes
	}
	f.maxAttachmentBytes = maxBytes
}

func readAttachmentFileLimitContext(ctx context.Context, filename string, limit int64) ([]byte, error) {
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	if limit >= 0 {
		if info, statErr := file.Stat(); statErr == nil && info.Mode().IsRegular() && info.Size() > limit {
			return nil, fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
		}
		return readAttachmentReaderLimitContext(ctx, file, limit)
	}
	return readAttachmentReaderLimitContext(ctx, file, limit)
}

func (f *Document) readAttachmentFileLimitContext(ctx context.Context, filename string, limit int64) ([]byte, error) {
	if f.resourceLoader == nil {
		return readAttachmentFileLimitContext(ctx, filename, limit)
	}
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	reader, info, err := f.resourceLoader.OpenResource(ctx, ResourceAttachment, filename)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, fmt.Errorf("resource loader returned nil reader")
	}
	defer func() { _ = reader.Close() }()
	if limit >= 0 && info.Size >= 0 && info.Size > limit {
		return nil, fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
	}
	return readAttachmentReaderLimitContext(ctx, reader, limit)
}

func readAttachmentLoaderLimitContext(ctx context.Context, loader AttachmentLoader, limit int64) ([]byte, error) {
	if loader == nil {
		return nil, fmt.Errorf("attachment loader is nil")
	}
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	reader, size, err := loader.OpenAttachment(ctx)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return nil, fmt.Errorf("attachment loader returned nil reader")
	}
	defer func() { _ = reader.Close() }()
	if limit >= 0 && size >= 0 && size > limit {
		return nil, fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
	}
	return readAttachmentReaderLimitContext(ctx, reader, limit)
}

func readAttachmentReaderLimitContext(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	reader := io.Reader(contextReader{ctx: ctx, r: r})
	var data []byte
	var err error
	if limit >= 0 {
		data, err = io.ReadAll(io.LimitReader(reader, limit+1))
		if err == nil && int64(len(data)) > limit {
			err = fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
		}
	} else {
		data, err = io.ReadAll(reader)
	}
	if err == nil {
		err = outputCanceledError(ctx)
	}
	return data, err
}

var pdfNameReserved [256]bool

func init() {
	for _, c := range []byte("()<>[]{}/%#") {
		pdfNameReserved[c] = true
	}
}

func escapePDFName(name string) string {
	const hexDigits = "0123456789ABCDEF"
	var out strings.Builder
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c <= ' ' || c >= 0x7f || pdfNameReserved[c] {
			out.WriteByte('#')
			out.WriteByte(hexDigits[c>>4])
			out.WriteByte(hexDigits[c&0x0f])
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

// SetAttachments writes attachments as embedded files attached to the document.
// These attachments are global; see AddAttachmentAnnotation() for links
// anchored on a page. Only the last call to SetAttachments is used; previous
// calls are discarded. Be aware that not all PDF readers
// support document attachments. See the SetAttachment example for a
// demonstration of this method.
func (f *Document) SetAttachments(as []Attachment) {
	f.attachments = make([]Attachment, len(as))
	for i := range as {
		f.attachments[i] = cloneAttachment(as[i])
	}
}

// SetAttachmentsImmutable writes attachments as embedded files without copying
// each attachment's content slice. The caller must not mutate attachment content
// until Output, OutputAndClose, or OutputFileAndClose returns.
func (f *Document) SetAttachmentsImmutable(as []Attachment) {
	f.attachments = make([]Attachment, len(as))
	for i := range as {
		f.attachments[i] = cloneAttachmentImmutable(as[i])
	}
}

// AttachmentFromFile returns a file-backed attachment descriptor. The file is
// read when the document is output. Callers may override Filename, MIMEType,
// Description, or AFRelationship on the returned value before passing it to
// SetAttachments or AddAttachmentAnnotation.
func AttachmentFromFile(fileStr string) Attachment {
	return Attachment{FilePath: fileStr, Filename: filepath.Base(fileStr)}
}

// AttachmentFromFileWithOptions returns a file-backed attachment descriptor and
// optionally validates the file immediately.
func AttachmentFromFileWithOptions(fileStr string, options AttachmentOptions) (Attachment, error) {
	attachment := AttachmentFromFile(fileStr)
	if options.MaxBytes < 0 {
		return attachment, fmt.Errorf("invalid max attachment bytes: %d", options.MaxBytes)
	}
	if options.MaxBytes > 0 {
		attachment.maxBytes = options.MaxBytes
	}
	if options.Eager {
		limit := options.MaxBytes
		if limit == 0 {
			limit = MaxAttachmentBytes
		}
		file, err := os.Open(fileStr)
		if err != nil {
			return attachment, err
		}
		defer func() { _ = file.Close() }()
		if info, err := file.Stat(); err != nil {
			return attachment, err
		} else if info.Mode().IsRegular() && limit >= 0 && info.Size() > limit {
			return attachment, fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
		}
	}
	return attachment, nil
}

// AttachmentFromLoader returns a loader-backed attachment descriptor. The
// loader is opened when the document is output. Callers may override MIMEType,
// Description, or AFRelationship on the returned value before passing it to
// SetAttachments or AddAttachmentAnnotation.
func AttachmentFromLoader(filename string, loader AttachmentLoader) Attachment {
	return Attachment{Filename: filename, Loader: loader}
}

// AttachmentFromLoaderWithOptions returns a loader-backed attachment descriptor
// and optionally opens the loader immediately to validate availability and known
// size. Content is still read during output.
func AttachmentFromLoaderWithOptions(filename string, loader AttachmentLoader, options AttachmentOptions) (Attachment, error) {
	attachment := AttachmentFromLoader(filename, loader)
	if loader == nil {
		return attachment, fmt.Errorf("attachment loader is nil")
	}
	if options.MaxBytes < 0 {
		return attachment, fmt.Errorf("invalid max attachment bytes: %d", options.MaxBytes)
	}
	if options.MaxBytes > 0 {
		attachment.maxBytes = options.MaxBytes
	}
	if options.Eager {
		limit := options.MaxBytes
		if limit == 0 {
			limit = MaxAttachmentBytes
		}
		reader, size, err := loader.OpenAttachment(context.Background())
		if err != nil {
			return attachment, err
		}
		if reader == nil {
			return attachment, fmt.Errorf("attachment loader returned nil reader")
		}
		_ = reader.Close()
		if size >= 0 && limit >= 0 && size > limit {
			return attachment, fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
		}
	}
	return attachment, nil
}

func (f *Document) putAttachmentsContext(ctx context.Context) {
	for i, a := range f.attachments {
		if err := outputCanceledError(ctx); err != nil {
			f.SetError(err)
			return
		}
		f.embedContext(ctx, &a)
		f.attachments[i] = a
	}
}

// getEmbeddedFiles returns the /EmbeddedFiles name-tree catalog entry.
func (f Document) getEmbeddedFiles() string {
	var names strings.Builder
	for i, as := range f.attachments {
		if i > 0 {
			names.WriteByte('\n')
		}
		names.WriteString("(Attachment")
		names.WriteString(strconv.Itoa(i + 1))
		names.WriteString(") ")
		names.WriteString(strconv.Itoa(as.objectNumber))
		names.WriteString(" 0 R ")
	}
	return "<< /Names [\n " + names.String() + " \n] >>"
}

// ---------------------------------- Annotations ----------------------------------
type annotationAttach struct {
	*Attachment

	x, y, w, h float64 // PDF coordinates; y has been adjusted and scaled.
}

// AddAttachmentAnnotation puts a link on the current page over the rectangle
// defined by x, y, w, and h. This link points to the content defined in a,
// which is embedded in the document. This method does not draw anything; call a
// method such as Cell() or Rect() to indicate that a link is present. The
// attachment descriptor is copied when it is added, so later caller mutations do
// not affect the document. Equivalent attachments are deduplicated during
// output. Be aware that not all PDF readers support annotated attachments. See
// the AddAttachmentAnnotation example for a demonstration of this method.
func (f *Document) AddAttachmentAnnotation(a *Attachment, x, y, w, h float64) {
	if f.err != nil {
		return
	}
	if a == nil {
		f.SetErrorf("attachment annotation requires an attachment")
		return
	}
	if f.page <= 0 {
		f.SetErrorf("attachment annotation requires an active page")
		return
	}
	if !finiteNumbers(x, y, w, h) {
		f.SetErrorf("invalid attachment annotation rectangle")
		return
	}
	attachment := cloneAttachment(*a)
	f.pageAttachments[f.page] = append(f.pageAttachments[f.page], annotationAttach{
		Attachment: &attachment,
		x:          x * f.k, y: f.hPt - y*f.k, w: w * f.k, h: h * f.k,
	})
}

func cloneAttachment(a Attachment) Attachment {
	if a.Content != nil {
		a.Content = append([]byte{}, a.Content...)
	}
	normalizeAttachment(&a)
	a.objectNumber = 0
	return a
}

func cloneAttachmentImmutable(a Attachment) Attachment {
	normalizeAttachment(&a)
	a.objectNumber = 0
	return a
}

func (f *Document) cleanupAttachmentCompressedFiles() {
	f.ensureResourceStore().cleanupAttachmentCompressedFiles()
}

func (f *Document) putAnnotationsAttachmentsContext(ctx context.Context) {
	for _, l := range f.pageAttachments {
		for _, an := range l {
			if err := outputCanceledError(ctx); err != nil {
				f.SetError(err)
				return
			}
			f.embedContext(ctx, an.Attachment)
		}
	}
}

func (f *Document) appendAttachmentAnnotationLinks(out []byte, page int) []byte {
	for _, an := range f.pageAttachments[page] {
		x1, y1, x2, y2 := an.x, an.y, an.x+an.w, an.y-an.h
		out = append(out, "<< /Type /Annot /Subtype /FileAttachment /Rect ["...)
		out = appendPDFNumberSpace(out, x1, 2)
		out = appendPDFNumberSpace(out, y1, 2)
		out = appendPDFNumberSpace(out, x2, 2)
		out = appendPDFNumber(out, y2, 2)
		out = append(out, "] /Border [0 0 0]\n/Contents "...)
		out = f.appendUTF16TextString(out, an.Description)
		out = append(out, " /T "...)
		out = f.appendUTF16TextString(out, an.Filename)
		out = append(out, " /AP << /N << /Type /XObject /Subtype /Form /BBox ["...)
		out = appendPDFNumberSpace(out, x1, 2)
		out = appendPDFNumberSpace(out, y1, 2)
		out = appendPDFNumberSpace(out, x2, 2)
		out = appendPDFNumber(out, y2, 2)
		out = append(out, "] /Length 0 >>\nstream\nendstream>>/FS "...)
		out = appendPDFObjectRef(out, an.objectNumber)
		out = append(out, " >>\n"...)
	}
	return out
}
