// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"hash"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

// ErrNilWriter reports that a PDF output method received a nil writer.
var ErrNilWriter = errors.New("pdf output writer is nil")

// ErrStreamingOutputConsumed reports that final PDF bytes were previously
// streamed without retaining the in-memory output buffer.
var ErrStreamingOutputConsumed = errors.New("streaming PDF output has already been consumed")

type pdfOutputSink struct {
	w    io.Writer
	n    int
	hash hash.Hash
	err  error
}

func newPDFOutputSink(w io.Writer, offset int, hash hash.Hash) *pdfOutputSink {
	return &pdfOutputSink{w: w, n: offset, hash: hash}
}

func (s *pdfOutputSink) Len() int {
	if s == nil {
		return 0
	}
	return s.n
}

func (s *pdfOutputSink) Write(p []byte) (int, error) {
	if s.err != nil {
		return 0, s.err
	}
	n, err := s.w.Write(p)
	if n > 0 {
		if s.hash != nil {
			_, _ = s.hash.Write(p[:n])
		}
		s.n += n
	}
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		s.err = err
	}
	return n, err
}

func (s *pdfOutputSink) WriteString(str string) error {
	if s.err != nil {
		return s.err
	}
	if stringWriter, ok := s.w.(io.StringWriter); ok {
		n, err := stringWriter.WriteString(str)
		if n > 0 {
			if s.hash != nil {
				written := n
				if written > len(str) {
					written = len(str)
				}
				_, _ = io.WriteString(s.hash, str[:written])
			}
			s.n += n
		}
		if err == nil && n != len(str) {
			err = io.ErrShortWrite
		}
		if err != nil {
			s.err = err
		}
		return err
	}
	_, err := s.Write([]byte(str))
	return err
}

func (s *pdfOutputSink) WriteByte(b byte) error {
	if s.err != nil {
		return s.err
	}
	if byteWriter, ok := s.w.(interface{ WriteByte(byte) error }); ok {
		if err := byteWriter.WriteByte(b); err != nil {
			s.err = err
			return err
		}
		if s.hash != nil {
			_, _ = s.hash.Write([]byte{b})
		}
		s.n++
		return nil
	}
	_, err := s.Write([]byte{b})
	return err
}

func (s *pdfOutputSink) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(s, r)
}

// OutputOptions controls behavior applied during PDF output. Zero fields leave
// the document's current settings unchanged, except DisableSync which defaults
// to the durable file-output behavior.
type OutputOptions struct {
	// DisableSync skips fsyncing the temporary output file before rename. The
	// zero value preserves the durable default.
	DisableSync bool
	// Compression optionally overrides document compression for this output.
	Compression CompressionPolicy
	// Limits optionally tightens output-time resource limits.
	Limits Limits
	// Deterministic applies deterministic output defaults before output.
	Deterministic bool
	// StreamFinal uses the one-shot streaming final-writer path for this
	// output. It lowers peak memory for very large unsigned PDFs but does not
	// retain the final PDF buffer for repeated output from the same Document.
	StreamFinal bool
}

// OutputFileOptions is kept for source compatibility. Prefer OutputOptions for
// new code.
type OutputFileOptions = OutputOptions

type outputRequest struct {
	options OutputOptions
	write   func(context.Context, io.Writer) error
}

// OutputAndClose sends the PDF document to the writer specified by w. This
// method will close both f and w, even if an error is detected and no document
// is produced.
func (f *Document) OutputAndClose(w io.WriteCloser) error {
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	err := f.Output(w)
	if closeErr := w.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	return err
}

// OutputFileAndClose creates or truncates the file specified by fileStr and
// writes the PDF document to it. This method will close f and the newly
// written file, even if an error is detected and no document is produced.
//
// Most examples demonstrate the use of this method.
func (f *Document) OutputFileAndClose(fileStr string) error {
	return f.coordinateFileOutput(context.Background(), fileStr, f.documentOutputRequest(OutputOptions{}, false), !f.outputPolicy.DisableSync)
}

// OutputFileAndCloseNoSync creates or truncates fileStr without fsyncing the
// temporary file before rename. It is intended for high-throughput batch
// generation where the caller accepts weaker crash-durability guarantees.
func (f *Document) OutputFileAndCloseNoSync(fileStr string) error {
	return f.coordinateFileOutput(context.Background(), fileStr, f.documentOutputRequest(OutputOptions{}, false), false)
}

// OutputFile creates or truncates fileStr and writes the PDF document to it.
func (f *Document) OutputFile(fileStr string) error {
	return f.OutputFileContext(context.Background(), fileStr)
}

// OutputFileAndCloseWithOptions creates or truncates fileStr using explicit
// file output options. A zero-value OutputFileOptions keeps the durable default.
func (f *Document) OutputFileAndCloseWithOptions(fileStr string, options OutputFileOptions) error {
	return f.OutputFileWithOptions(fileStr, options)
}

// OutputFileWithOptions creates or truncates fileStr using output-wide options.
func (f *Document) OutputFileWithOptions(fileStr string, options OutputOptions) error {
	return f.OutputFileWithOptionsContext(context.Background(), fileStr, options)
}

// OutputFileWithOptionsContext creates or truncates fileStr using output-wide
// options and context cancellation.
func (f *Document) OutputFileWithOptionsContext(ctx context.Context, fileStr string, options OutputOptions) error {
	return f.coordinateFileOutput(ctx, fileStr, f.documentOutputRequest(options, false), f.syncOutputForOptions(options))
}

// OutputFileStream creates or truncates fileStr and streams final PDF
// serialization directly to the temporary file used for atomic output.
func (f *Document) OutputFileStream(fileStr string) error {
	return f.OutputFileStreamContext(context.Background(), fileStr)
}

// OutputFileStreamContext creates or truncates fileStr and streams final PDF
// serialization directly to the temporary file used for atomic output.
func (f *Document) OutputFileStreamContext(ctx context.Context, fileStr string) error {
	return f.coordinateFileOutput(ctx, fileStr, f.documentOutputRequest(OutputOptions{}, true), !f.outputPolicy.DisableSync)
}

// OutputFileStreamWithOptions creates or truncates fileStr and streams final
// PDF serialization using output-wide options.
func (f *Document) OutputFileStreamWithOptions(fileStr string, options OutputOptions) error {
	return f.OutputFileStreamWithOptionsContext(context.Background(), fileStr, options)
}

// OutputFileStreamWithOptionsContext creates or truncates fileStr and streams
// final PDF serialization using output-wide options and context cancellation.
func (f *Document) OutputFileStreamWithOptionsContext(ctx context.Context, fileStr string, options OutputOptions) error {
	return f.coordinateFileOutput(ctx, fileStr, f.documentOutputRequest(options, true), f.syncOutputForOptions(options))
}

// OutputFileContext creates or truncates fileStr and stops before writing if
// ctx is canceled. The atomic output path removes the temporary file on error.
func (f *Document) OutputFileContext(ctx context.Context, fileStr string) error {
	return f.coordinateFileOutput(ctx, fileStr, f.documentOutputRequest(OutputOptions{}, false), !f.outputPolicy.DisableSync)
}

func (f *Document) documentOutputRequest(options OutputOptions, forceStream bool) outputRequest {
	return outputRequest{
		options: options,
		write: func(ctx context.Context, w io.Writer) error {
			if forceStream || f.streamFinalForOptions(options) {
				return f.OutputStreamContext(ctx, w)
			}
			return f.OutputContext(ctx, w)
		},
	}
}

func (f *Document) coordinateFileOutput(ctx context.Context, fileStr string, request outputRequest, syncOutput bool) error {
	if f.err != nil {
		return f.err
	}
	return writeFileAtomically(fileStr, syncOutput, func(w io.Writer) error {
		return f.coordinateOutput(ctx, w, request)
	})
}

func writeFileAtomically(fileStr string, syncOutput bool, write func(io.Writer) error) error {
	dir := filepath.Dir(fileStr)
	base := filepath.Base(fileStr)
	mode := outputFileMode(fileStr)
	pdfFile, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tempName := pdfFile.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tempName)
		}
	}()

	if err := write(pdfFile); err != nil {
		_ = pdfFile.Close()
		return err
	}
	if syncOutput {
		if err := pdfFile.Sync(); err != nil {
			_ = pdfFile.Close()
			return err
		}
	}
	if err := pdfFile.Chmod(mode); err != nil {
		_ = pdfFile.Close()
		return err
	}
	if err := pdfFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, fileStr); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func outputFileMode(fileStr string) os.FileMode {
	if info, err := os.Stat(fileStr); err == nil {
		return info.Mode().Perm()
	}
	return 0o644
}

// Output sends the PDF document to the writer specified by w. No output will
// be written if an error has occurred in the document generation process. w
// remains open after this function returns. After returning, f is in a closed
// state and its methods should not be called.
func (f *Document) Output(w io.Writer) error {
	return f.OutputContext(context.Background(), w)
}

// OutputWithOptions sends the PDF document to w using output-wide options.
func (f *Document) OutputWithOptions(w io.Writer, options OutputOptions) error {
	return f.OutputWithOptionsContext(context.Background(), w, options)
}

// OutputWithOptionsContext sends the PDF document to w using output-wide
// options and context cancellation. Output options are applied to the document;
// if output fails before the document closes, the previous output settings are
// restored while the output error remains latched.
func (f *Document) OutputWithOptionsContext(ctx context.Context, w io.Writer, options OutputOptions) error {
	return f.coordinateOutput(ctx, w, f.documentOutputRequest(options, false))
}

// OutputStream sends the PDF document to w while streaming final PDF
// serialization directly to w.
//
// Unlike Output, this method does not retain the final PDF buffer for repeated
// output. Use it for very large unsigned PDFs where lower peak memory is more
// important than writing the same Document instance more than once.
func (f *Document) OutputStream(w io.Writer) error {
	return f.OutputStreamContext(context.Background(), w)
}

// OutputStreamWithOptions streams final PDF serialization to w using
// output-wide options.
func (f *Document) OutputStreamWithOptions(w io.Writer, options OutputOptions) error {
	return f.OutputStreamWithOptionsContext(context.Background(), w, options)
}

// OutputStreamWithOptionsContext streams final PDF serialization to w using
// output-wide options and context cancellation.
func (f *Document) OutputStreamWithOptionsContext(ctx context.Context, w io.Writer, options OutputOptions) error {
	return f.coordinateOutput(ctx, w, f.documentOutputRequest(options, true))
}

// coordinateOutput is the one path for public option-bearing output methods.
// Writer and file variants supply only their destination; unsigned and signed
// variants supply only their final serialization callback.
func (f *Document) coordinateOutput(ctx context.Context, w io.Writer, request outputRequest) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return f.withOutputOptions(request.options, func() error {
		return request.write(ctx, w)
	})
}

func (f *Document) withOutputOptions(options OutputOptions, output func() error) error {
	snapshot := f.outputSettingsSnapshot()
	if err := f.applyOutputOptions(options); err != nil {
		return err
	}
	err := output()
	if err != nil && f.state < 3 {
		f.restoreOutputSettings(snapshot)
	}
	return err
}

// OutputContext sends the PDF document to w and checks ctx before document
// closing and before the final writer call. Cancellation during arbitrary writer
// implementations still depends on the writer honoring the context.
func (f *Document) OutputContext(ctx context.Context, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if f.err != nil {
		return f.err
	}
	if f.streamedOutput {
		f.SetError(ErrStreamingOutputConsumed)
		return f.err
	}
	if f.outputPolicy.StreamFinal && f.state < 3 {
		return f.OutputStreamContext(ctx, w)
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return err
	}
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	if f.state < 3 {
		f.closeContext(ctx)
	}
	if f.err != nil {
		return f.err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return err
	}
	n, err := w.Write(f.buffer.Bytes())
	if err != nil {
		return err
	}
	if n != f.buffer.Len() {
		return io.ErrShortWrite
	}
	return nil
}

// OutputStreamContext sends the PDF document to w and writes final PDF objects
// directly to w instead of first assembling the final PDF in Document.buffer.
//
// This is a one-shot output path for very large unsigned documents. It
// preserves the existing Output behavior by being opt-in: Output remains
// repeatable because it retains the final PDF buffer, while OutputStreamContext
// trades repeatability for lower peak memory. Signed output still buffers
// because PDF signing needs the complete byte range.
func (f *Document) OutputStreamContext(ctx context.Context, w io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if f.err != nil {
		return f.err
	}
	if f.streamedOutput {
		f.SetError(ErrStreamingOutputConsumed)
		return f.err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return err
	}
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	if f.state == 3 {
		return f.OutputContext(ctx, w)
	}

	sink := newPDFOutputSink(w, 0, nil)
	if f.buffer.Len() > 0 {
		if _, err := sink.Write(f.buffer.Bytes()); err != nil {
			f.SetError(err)
			return err
		}
		f.buffer.Reset()
	}

	previousSink := f.outputSink
	f.outputSink = sink
	defer func() { f.outputSink = previousSink }()

	f.closeContext(ctx)
	if sink.err != nil {
		f.SetError(sink.err)
		return sink.err
	}
	if f.err != nil {
		return f.err
	}
	f.buffer.Reset()
	f.streamedOutput = true
	return nil
}

func (f *Document) applyOutputOptions(options OutputOptions) error {
	if options.Compression != (CompressionPolicy{}) {
		if err := f.SetCompressionPolicy(options.Compression); err != nil {
			return err
		}
	}
	if options.Limits != (Limits{}) {
		if err := f.applyLimits(options.Limits); err != nil {
			return err
		}
	}
	if options.Deterministic {
		f.applyDeterministicOutput()
	}
	return f.err
}

type outputSettingsSnapshot struct {
	compress                       bool
	compressLevel                  int
	pageCompressionWorkers         int
	attachmentCompressionWorkers   int
	compressionTinyStreamThreshold int
	limits                         Limits
	limitsSet                      bool
	maxAttachmentBytes             int64
	catalogSort                    bool
	creationDate                   time.Time
	modDate                        time.Time
}

func (f *Document) outputSettingsSnapshot() outputSettingsSnapshot {
	return outputSettingsSnapshot{
		compress:                       f.compress,
		compressLevel:                  f.compressLevel,
		pageCompressionWorkers:         f.pageCompressionWorkers,
		attachmentCompressionWorkers:   f.attachmentCompressionWorkers,
		compressionTinyStreamThreshold: f.compressionTinyStreamThreshold,
		limits:                         f.limits,
		limitsSet:                      f.limitsSet,
		maxAttachmentBytes:             f.maxAttachmentBytes,
		catalogSort:                    f.catalogSort,
		creationDate:                   f.creationDate,
		modDate:                        f.modDate,
	}
}

func (f *Document) restoreOutputSettings(snapshot outputSettingsSnapshot) {
	f.compress = snapshot.compress
	f.compressLevel = snapshot.compressLevel
	f.pageCompressionWorkers = snapshot.pageCompressionWorkers
	f.attachmentCompressionWorkers = snapshot.attachmentCompressionWorkers
	f.compressionTinyStreamThreshold = snapshot.compressionTinyStreamThreshold
	f.limits = snapshot.limits
	f.limitsSet = snapshot.limitsSet
	f.maxAttachmentBytes = snapshot.maxAttachmentBytes
	f.catalogSort = snapshot.catalogSort
	f.creationDate = snapshot.creationDate
	f.modDate = snapshot.modDate
}

func (f *Document) syncOutputForOptions(options OutputOptions) bool {
	return !f.outputPolicy.DisableSync && !options.DisableSync
}

func (f *Document) streamFinalForOptions(options OutputOptions) bool {
	return f.outputPolicy.StreamFinal || options.StreamFinal
}

func isNilWriter(w io.Writer) bool {
	if w == nil {
		return true
	}
	v := reflect.ValueOf(w)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// out adds a line to the document.
func (f *Document) out(s string) {
	if f.state == 2 {
		f.markAliasPageString(s)
		_, _ = f.pages[f.page].WriteString(s)
		_, _ = f.pages[f.page].WriteString("\n")
	} else {
		f.writeFinalString(s)
		f.writeFinalString("\n")
	}
}

// outbuf adds a buffered line to the document.
func (f *Document) outbuf(r io.Reader) error {
	if r == nil {
		err := errors.New("pdf raw reader is nil")
		f.SetError(err)
		return err
	}
	if f.state == 2 {
		f.markAliasPageConservative()
		if _, err := f.pages[f.page].ReadFrom(r); err != nil {
			f.SetError(err)
			return err
		}
		_, _ = f.pages[f.page].WriteString("\n")
	} else {
		if _, err := f.readFinalFrom(r); err != nil {
			f.SetError(err)
			return err
		}
		f.writeFinalString("\n")
	}
	return nil
}

func (f *Document) outbytes(b []byte) {
	if f.state == 2 {
		f.markAliasPageBytes(b)
		_, _ = f.pages[f.page].Write(b)
		_ = f.pages[f.page].WriteByte('\n')
	} else {
		f.writeFinalBytes(b)
		f.writeFinalByte('\n')
	}
}

func (f *Document) writeRawBytes(b []byte) {
	if f.state == 2 {
		f.markAliasPageBytes(b)
		_, _ = f.pages[f.page].Write(b)
	} else {
		f.writeFinalBytes(b)
	}
}

func (f *Document) writeRawByte(b byte) {
	if f.state == 2 {
		_ = f.pages[f.page].WriteByte(b)
	} else {
		f.writeFinalByte(b)
	}
}

func (f *Document) finalOutputOffset() int {
	if f.outputSink != nil {
		return f.outputSink.Len()
	}
	return f.buffer.Len()
}

func (f *Document) writeFinalString(s string) {
	if f.outputSink != nil {
		if err := f.outputSink.WriteString(s); err != nil {
			f.SetError(err)
		}
		return
	}
	_, _ = f.buffer.WriteString(s)
	if f.fileIDHash != nil {
		_, _ = f.fileIDHash.Write([]byte(s))
	}
}

func (f *Document) writeFinalBytes(b []byte) {
	if f.outputSink != nil {
		if _, err := f.outputSink.Write(b); err != nil {
			f.SetError(err)
		}
		return
	}
	_, _ = f.buffer.Write(b)
	if f.fileIDHash != nil {
		_, _ = f.fileIDHash.Write(b)
	}
}

func (f *Document) writeFinalByte(b byte) {
	if f.outputSink != nil {
		if err := f.outputSink.WriteByte(b); err != nil {
			f.SetError(err)
		}
		return
	}
	_ = f.buffer.WriteByte(b)
	if f.fileIDHash != nil {
		_, _ = f.fileIDHash.Write([]byte{b})
	}
}

func (f *Document) readFinalFrom(r io.Reader) (int64, error) {
	if f.outputSink != nil {
		return f.outputSink.ReadFrom(r)
	}
	if f.fileIDHash != nil {
		r = io.TeeReader(r, f.fileIDHash)
	}
	return f.buffer.ReadFrom(r)
}

// RawWriteStr writes a string directly to the PDF generation buffer. This is a
// low-level function that is not required for normal PDF construction. An
// understanding of the PDF specification is needed to use this method
// correctly.
func (f *Document) RawWriteStr(str string) {
	_ = f.RawWriteStrError(str)
}

// RawWriteStrError writes a string directly to the PDF generation buffer and
// returns any tagged-PDF restriction error.
func (f *Document) RawWriteStrError(str string) error {
	if err := f.requireSecurityFeature("raw PDF writes", f.securityPolicy.AllowRawWrites); err != nil {
		return err
	}
	if f.tagged.enabled {
		f.SetErrorf("tagged PDF raw writes must use RawWriteArtifactStr or semantic drawing APIs")
		return f.err
	}
	f.out(str)
	return f.err
}

// RawWriteArtifactStr writes raw PDF content marked as an artifact when tagged
// PDF output is enabled.
func (f *Document) RawWriteArtifactStr(str string) {
	_ = f.RawWriteArtifactStrError(str)
}

// RawWriteArtifactStrError writes raw PDF content marked as an artifact when
// tagged PDF output is enabled and returns any latched document error.
func (f *Document) RawWriteArtifactStrError(str string) error {
	if err := f.requireSecurityFeature("raw PDF writes", f.securityPolicy.AllowRawWrites); err != nil {
		return err
	}
	f.outTaggedContent([]byte(str), taggedContentOptions{Artifact: true})
	return f.err
}

// RawWriteBuf writes the contents of the specified buffer directly to the PDF
// generation buffer. This is a low-level function that is not required for
// normal PDF construction. An understanding of the PDF specification is needed
// to use this method correctly.
func (f *Document) RawWriteBuf(r io.Reader) {
	_ = f.RawWriteBufError(r)
}

// RawWriteBufError writes the contents of the specified reader directly to the
// PDF generation buffer and returns any reader error.
func (f *Document) RawWriteBufError(r io.Reader) error {
	if err := f.requireSecurityFeature("raw PDF writes", f.securityPolicy.AllowRawWrites); err != nil {
		return err
	}
	if f.tagged.enabled {
		f.SetErrorf("tagged PDF raw writes must use RawWriteArtifactBuf or semantic drawing APIs")
		return f.err
	}
	return f.outbuf(r)
}

// RawWriteArtifactBuf writes raw PDF content marked as an artifact when tagged
// PDF output is enabled.
func (f *Document) RawWriteArtifactBuf(r io.Reader) {
	_ = f.RawWriteArtifactBufError(r)
}

// RawWriteArtifactBufError writes raw PDF content marked as an artifact when
// tagged PDF output is enabled and returns any reader error.
func (f *Document) RawWriteArtifactBufError(r io.Reader) error {
	if err := f.requireSecurityFeature("raw PDF writes", f.securityPolicy.AllowRawWrites); err != nil {
		return err
	}
	var buf strings.Builder
	if r == nil {
		err := errors.New("pdf raw reader is nil")
		f.SetError(err)
		return err
	}
	if _, err := io.Copy(&buf, r); err != nil {
		f.SetError(err)
		return err
	}
	return f.RawWriteArtifactStrError(buf.String())
}

// outf adds a formatted line to the document.
func (f *Document) outf(fmtStr string, args ...any) {
	f.out(sprintf(fmtStr, args...))
}
