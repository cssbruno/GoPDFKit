// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// ErrNilWriter reports that a PDF output method received a nil writer.
var ErrNilWriter = errors.New("pdf output writer is nil")

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
}

// OutputFileOptions is kept for source compatibility. Prefer OutputOptions for
// new code.
type OutputFileOptions = OutputOptions

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
	return f.outputFileAndClose(fileStr, !f.outputPolicy.DisableSync)
}

// OutputFileAndCloseNoSync creates or truncates fileStr without fsyncing the
// temporary file before rename. It is intended for high-throughput batch
// generation where the caller accepts weaker crash-durability guarantees.
func (f *Document) OutputFileAndCloseNoSync(fileStr string) error {
	return f.outputFileAndClose(fileStr, false)
}

// OutputFileAndCloseWithOptions creates or truncates fileStr using explicit
// file output options. A zero-value OutputFileOptions keeps the durable default.
func (f *Document) OutputFileAndCloseWithOptions(fileStr string, options OutputFileOptions) error {
	return f.OutputFileWithOptions(fileStr, options)
}

func (f *Document) outputFileAndClose(fileStr string, syncOutput bool) error {
	if f.err != nil {
		return f.err
	}
	return writeFileAtomically(fileStr, syncOutput, f.Output)
}

// OutputFileWithOptions creates or truncates fileStr using output-wide options.
func (f *Document) OutputFileWithOptions(fileStr string, options OutputOptions) error {
	if f.err != nil {
		return f.err
	}
	return writeFileAtomically(fileStr, f.syncOutputForOptions(options), func(w io.Writer) error {
		return f.OutputWithOptions(w, options)
	})
}

// OutputFileContext creates or truncates fileStr and stops before writing if
// ctx is canceled. The atomic output path removes the temporary file on error.
func (f *Document) OutputFileContext(ctx context.Context, fileStr string) error {
	if f.err != nil {
		return f.err
	}
	return writeFileAtomically(fileStr, !f.outputPolicy.DisableSync, func(w io.Writer) error {
		return f.OutputContext(ctx, w)
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
	if err := f.applyOutputOptions(options); err != nil {
		return err
	}
	return f.OutputContext(context.Background(), w)
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

func (f *Document) syncOutputForOptions(options OutputOptions) bool {
	return !(f.outputPolicy.DisableSync || options.DisableSync)
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
		_, _ = f.buffer.WriteString(s)
		_, _ = f.buffer.WriteString("\n")
		if f.fileIDHash != nil {
			_, _ = f.fileIDHash.Write([]byte(s))
			_, _ = f.fileIDHash.Write([]byte{'\n'})
		}
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
		if f.fileIDHash != nil {
			r = io.TeeReader(r, f.fileIDHash)
		}
		if _, err := f.buffer.ReadFrom(r); err != nil {
			f.SetError(err)
			return err
		}
		_, _ = f.buffer.WriteString("\n")
		if f.fileIDHash != nil {
			_, _ = f.fileIDHash.Write([]byte{'\n'})
		}
	}
	return nil
}

func (f *Document) outbytes(b []byte) {
	if f.state == 2 {
		f.markAliasPageBytes(b)
		_, _ = f.pages[f.page].Write(b)
		_ = f.pages[f.page].WriteByte('\n')
	} else {
		_, _ = f.buffer.Write(b)
		_ = f.buffer.WriteByte('\n')
		if f.fileIDHash != nil {
			_, _ = f.fileIDHash.Write(b)
			_, _ = f.fileIDHash.Write([]byte{'\n'})
		}
	}
}

func (f *Document) writeRawBytes(b []byte) {
	if f.state == 2 {
		f.markAliasPageBytes(b)
		_, _ = f.pages[f.page].Write(b)
	} else {
		_, _ = f.buffer.Write(b)
		if f.fileIDHash != nil {
			_, _ = f.fileIDHash.Write(b)
		}
	}
}

func (f *Document) writeRawByte(b byte) {
	if f.state == 2 {
		_ = f.pages[f.page].WriteByte(b)
	} else {
		_ = f.buffer.WriteByte(b)
		if f.fileIDHash != nil {
			_, _ = f.fileIDHash.Write([]byte{b})
		}
	}
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
