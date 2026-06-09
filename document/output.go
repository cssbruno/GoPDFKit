// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"io"
	"os"
	"strings"
)

// OutputAndClose sends the PDF document to the writer specified by w. This
// method will close both f and w, even if an error is detected and no document
// is produced.
func (f *Document) OutputAndClose(w io.WriteCloser) error {
	_ = f.Output(w)
	if err := w.Close(); f.err == nil && err != nil {
		f.err = err
	}
	return f.err
}

// OutputFileAndClose creates or truncates the file specified by fileStr and
// writes the PDF document to it. This method will close f and the newly
// written file, even if an error is detected and no document is produced.
//
// Most examples demonstrate the use of this method.
func (f *Document) OutputFileAndClose(fileStr string) error {
	if f.err == nil {
		pdfFile, err := os.Create(fileStr)
		if err == nil {
			_ = f.Output(pdfFile)
			if err = pdfFile.Close(); f.err == nil && err != nil {
				f.err = err
			}
		} else {
			f.err = err
		}
	}
	return f.err
}

// Output sends the PDF document to the writer specified by w. No output will
// be written if an error has occurred in the document generation process. w
// remains open after this function returns. After returning, f is in a closed
// state and its methods should not be called.
func (f *Document) Output(w io.Writer) error {
	if f.err != nil {
		return f.err
	}
	if f.state < 3 {
		f.Close()
	}
	_, err := f.buffer.WriteTo(w)
	if err != nil {
		f.err = err
	}
	return f.err
}

// out adds a line to the document.
func (f *Document) out(s string) {
	if f.state == 2 {
		_, _ = f.pages[f.page].WriteString(s)
		_, _ = f.pages[f.page].WriteString("\n")
	} else {
		_, _ = f.buffer.WriteString(s)
		_, _ = f.buffer.WriteString("\n")
	}
}

// outbuf adds a buffered line to the document.
func (f *Document) outbuf(r io.Reader) {
	if f.state == 2 {
		_, _ = f.pages[f.page].ReadFrom(r)
		_, _ = f.pages[f.page].WriteString("\n")
	} else {
		_, _ = f.buffer.ReadFrom(r)
		_, _ = f.buffer.WriteString("\n")
	}
}

func (f *Document) outbytes(b []byte) {
	if f.state == 2 {
		_, _ = f.pages[f.page].Write(b)
		_ = f.pages[f.page].WriteByte('\n')
	} else {
		_, _ = f.buffer.Write(b)
		_ = f.buffer.WriteByte('\n')
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
	f.outbytes(f.wrapTaggedContent([]byte(str), taggedContentOptions{Artifact: true}))
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
