// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// ErrNilWriter reports that a PDF output method received a nil writer.
var ErrNilWriter = errors.New("pdf output writer is nil")

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
	if f.err != nil {
		return f.err
	}
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

	if err := f.Output(pdfFile); err != nil {
		_ = pdfFile.Close()
		return err
	}
	if err := pdfFile.Sync(); err != nil {
		_ = pdfFile.Close()
		return err
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
	if f.err != nil {
		return f.err
	}
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	if f.state < 3 {
		f.Close()
	}
	if f.err != nil {
		return f.err
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
func (f *Document) outbuf(r io.Reader) {
	if f.state == 2 {
		f.markAliasPageConservative()
		_, _ = f.pages[f.page].ReadFrom(r)
		_, _ = f.pages[f.page].WriteString("\n")
	} else {
		if f.fileIDHash != nil {
			r = io.TeeReader(r, f.fileIDHash)
		}
		_, _ = f.buffer.ReadFrom(r)
		_, _ = f.buffer.WriteString("\n")
		if f.fileIDHash != nil {
			_, _ = f.fileIDHash.Write([]byte{'\n'})
		}
	}
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
	if f.tagged.enabled {
		f.SetErrorf("tagged PDF raw writes must use RawWriteArtifactStr or semantic drawing APIs")
		return
	}
	f.out(str)
}

// RawWriteArtifactStr writes raw PDF content marked as an artifact when tagged
// PDF output is enabled.
func (f *Document) RawWriteArtifactStr(str string) {
	f.outTaggedContent([]byte(str), taggedContentOptions{Artifact: true})
}

// RawWriteBuf writes the contents of the specified buffer directly to the PDF
// generation buffer. This is a low-level function that is not required for
// normal PDF construction. An understanding of the PDF specification is needed
// to use this method correctly.
func (f *Document) RawWriteBuf(r io.Reader) {
	if f.tagged.enabled {
		f.SetErrorf("tagged PDF raw writes must use RawWriteArtifactBuf or semantic drawing APIs")
		return
	}
	f.outbuf(r)
}

// RawWriteArtifactBuf writes raw PDF content marked as an artifact when tagged
// PDF output is enabled.
func (f *Document) RawWriteArtifactBuf(r io.Reader) {
	var buf strings.Builder
	_, _ = io.Copy(&buf, r)
	f.RawWriteArtifactStr(buf.String())
}

// outf adds a formatted line to the document.
func (f *Document) outf(fmtStr string, args ...any) {
	f.out(sprintf(fmtStr, args...))
}
