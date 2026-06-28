// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"compress/zlib"
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

// MaxAttachmentBytes is the default largest attachment content accepted for
// embedding.
const MaxAttachmentBytes = 64 * 1024 * 1024

// AttachmentOptions controls file-backed attachment validation.
type AttachmentOptions struct {
	MaxBytes int64 // Maximum file size; 0 uses MaxAttachmentBytes.
	Eager    bool  // Validate file existence and size before returning.
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

	objectNumber   int    // Filled when the filespec is embedded.
	mimeType       string // Normalized lazily or when cloned.
	afRelationship string // Normalized lazily or when cloned.
	checksum       string // PDF MD5 checksum, computed once per attachment content.
	maxBytes       int64  // Per-attachment maximum content size; 0 uses document/default limit.
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

// checksum returns the hex-encoded PDF attachment checksum of data.
func attachmentChecksum(data []byte) string {
	tmp := md5.Sum(data)
	return hex.EncodeToString(tmp[:])
}

// writeCompressedFileObject writes a deflate-compressed /EmbeddedFile object
// with length, compressed length, and MD5 checksum metadata.
func (f *Document) writeCompressedFileObject(content []byte, mimeType, sum string) {
	lenUncompressed := len(content)
	streamKey := attachmentStreamKey{size: lenUncompressed, checksum: sum, mimeType: mimeType}
	compressed := f.attachmentCompressed[streamKey]
	if compressed == nil {
		compressed = f.compressBytes(content)
	}
	if f.err != nil {
		return
	}
	lenCompressed := len(compressed)
	f.newobj()
	f.outf("<< /Type /EmbeddedFile /Subtype /%s /Length %d /Filter /FlateDecode /Params << /CheckSum <%s> /Size %d >> >>\n",
		escapePDFName(mimeType), lenCompressed, sum, lenUncompressed)
	f.putstream(compressed)
	f.out("endobj")
}

type attachmentCompressionTask struct {
	attachment *Attachment
	mimeType   string
	content    []byte
}

func (f *Document) prepareAttachmentCompression() {
	if f.err != nil {
		return
	}
	tasks := f.attachmentCompressionTasks()
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
		data       []byte
		err        error
	}
	results := make(chan result, len(tasks))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range jobs {
				data, checksum, err := compressAttachmentWithChecksum(task.content, f.compressLevel)
				results <- result{attachment: task.attachment, mimeType: task.mimeType, checksum: checksum, data: data, err: err}
			}
		}()
	}
	for _, task := range tasks {
		jobs <- task
	}
	close(jobs)
	wg.Wait()
	close(results)
	for result := range results {
		if result.err != nil {
			f.SetError(result.err)
			return
		}
		if result.attachment.checksum == "" {
			result.attachment.checksum = result.checksum
		}
		key := attachmentStreamKey{size: len(result.attachment.Content), checksum: result.attachment.checksum, mimeType: result.mimeType}
		if f.attachmentCompressed[key] == nil {
			f.attachmentCompressed[key] = result.data
		}
	}
}

func (f *Document) attachmentCompressionTasks() []attachmentCompressionTask {
	seen := make(map[*Attachment]bool)
	tasks := make([]attachmentCompressionTask, 0, len(f.attachments))
	add := func(a *Attachment) {
		if a == nil || seen[a] {
			return
		}
		if !f.loadAttachmentContent(a) {
			return
		}
		a.mimeType = attachmentMIMEType(*a)
		a.afRelationship = attachmentAFRelationship(*a)
		if a.checksum != "" {
			key := attachmentStreamKey{size: len(a.Content), checksum: a.checksum, mimeType: a.mimeType}
			if f.attachmentStreams[key] != 0 || f.attachmentCompressed[key] != nil {
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

func compressAttachmentWithChecksum(content []byte, level int) ([]byte, string, error) {
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
		return nil, "", err
	}
	if _, err = io.MultiWriter(cmp, sum).Write(content); err != nil {
		_ = cmp.Close()
		releaseZlibWriter(list, cmp)
		releaseCompressBuffer(buf)
		return nil, "", err
	}
	if err = cmp.Close(); err != nil {
		releaseZlibWriter(list, cmp)
		releaseCompressBuffer(buf)
		return nil, "", err
	}
	releaseZlibWriter(list, cmp)
	checksum := hex.EncodeToString(sum.Sum(nil))
	if buf.Len() >= largeCompressedStreamNoCopyThreshold {
		return buf.Bytes(), checksum, nil
	}
	defer releaseCompressBuffer(buf)
	return append([]byte(nil), buf.Bytes()...), checksum, nil
}

// embed includes the attachment content and updates its internal reference.
func (f *Document) embed(a *Attachment) {
	if a.objectNumber != 0 { // Already embedded; object numbers start at 2.
		return
	}
	if !f.loadAttachmentContent(a) {
		return
	}
	normalizeAttachment(a)
	streamKey := attachmentStreamKey{
		size:     len(a.Content),
		checksum: a.checksum,
		mimeType: a.mimeType,
	}
	fileKey := attachmentFileKey{
		stream:       streamKey,
		filename:     a.Filename,
		description:  a.Description,
		relationship: a.afRelationship,
	}
	if objectNumber := f.attachmentFiles[fileKey]; objectNumber != 0 {
		a.objectNumber = objectNumber
		return
	}
	oldState := f.state
	f.state = 1 // Write file content to the main buffer.
	streamID := f.attachmentStreams[streamKey]
	if streamID == 0 {
		f.writeCompressedFileObject(a.Content, a.mimeType, a.checksum)
		if f.err != nil {
			f.state = oldState
			return
		}
		streamID = f.n
		f.attachmentStreams[streamKey] = streamID
	}
	f.newobj()
	fileSpec := make([]byte, 0, len(a.Filename)*2+len(a.Description)*2+128)
	fileSpec = append(fileSpec, "<< /Type /Filespec /F () /UF "...)
	fileSpec = f.appendUTF16TextString(fileSpec, a.Filename)
	fileSpec = append(fileSpec, " /AFRelationship /"...)
	fileSpec = append(fileSpec, a.afRelationship...)
	fileSpec = append(fileSpec, " /EF << /F "...)
	fileSpec = appendPDFInt(fileSpec, streamID)
	fileSpec = append(fileSpec, " 0 R >> /Desc "...)
	fileSpec = f.appendUTF16TextString(fileSpec, a.Description)
	fileSpec = append(fileSpec, "\n>>"...)
	f.outbytes(fileSpec)
	f.out("endobj")
	a.objectNumber = f.n
	f.attachmentFiles[fileKey] = a.objectNumber
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
	if a.checksum == "" && (len(a.Content) > 0 || strings.TrimSpace(a.FilePath) == "") {
		a.checksum = attachmentChecksum(a.Content)
	}
}

func (f *Document) loadAttachmentContent(a *Attachment) bool {
	if a == nil {
		return true
	}
	if strings.TrimSpace(a.FilePath) != "" {
		if err := f.requireSecurityFeature("file-backed attachments", f.securityPolicy.AllowFileAttachments); err != nil {
			return false
		}
	}
	limit := f.attachmentMaxBytes(*a)
	if limit >= 0 && int64(len(a.Content)) > limit {
		f.SetError(fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge))
		return false
	}
	if len(a.Content) > 0 || strings.TrimSpace(a.FilePath) == "" {
		return true
	}
	data, err := readAttachmentFileLimit(a.FilePath, limit)
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
	return true
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

func readAttachmentFileLimit(filename string, limit int64) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if limit >= 0 {
		if info, statErr := file.Stat(); statErr == nil && info.Mode().IsRegular() && info.Size() > limit {
			return nil, fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
		}
		data, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err == nil && int64(len(data)) > limit {
			err = fmt.Errorf("%w: attachment data exceeds maximum size", ErrAttachmentTooLarge)
		}
		return data, err
	}
	return io.ReadAll(file)
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

// putAttachments embeds the current attachments and stores their object numbers
// for later use by getEmbeddedFiles().
func (f *Document) putAttachments() {
	for i, a := range f.attachments {
		f.embed(&a)
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
	a.Content = append([]byte(nil), a.Content...)
	normalizeAttachment(&a)
	a.objectNumber = 0
	return a
}

func cloneAttachmentImmutable(a Attachment) Attachment {
	normalizeAttachment(&a)
	a.objectNumber = 0
	return a
}

// putAnnotationsAttachments embeds attachments used by annotations and stores
// their object numbers for appendAttachmentAnnotationLinks(), which is called for
// each page.
func (f *Document) putAnnotationsAttachments() {
	for _, l := range f.pageAttachments {
		for _, an := range l {
			f.embed(an.Attachment)
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
