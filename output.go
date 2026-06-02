/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"io"
	"os"
)

// OutputAndClose sends the PDF document to the writer specified by w. This
// method will close both f and w, even if an error is detected and no document
// is produced.

func (f *Fpdf) OutputAndClose(w io.WriteCloser) error {
	f.Output(w)
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

func (f *Fpdf) OutputFileAndClose(fileStr string) error {
	if f.err == nil {
		pdfFile, err := os.Create(fileStr)
		if err == nil {
			f.Output(pdfFile)
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
// take place if an error has occurred in the document generation process. w
// remains open after this function returns. After returning, f is in a closed
// state and its methods should not be called.

func (f *Fpdf) Output(w io.Writer) error {
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

// out; Add a line to the document

func (f *Fpdf) out(s string) {
	if f.state == 2 {
		f.pages[f.page].WriteString(s)
		f.pages[f.page].WriteString("\n")
	} else {
		f.buffer.WriteString(s)
		f.buffer.WriteString("\n")
	}
}

// outbuf adds a buffered line to the document

func (f *Fpdf) outbuf(r io.Reader) {
	if f.state == 2 {
		f.pages[f.page].ReadFrom(r)
		f.pages[f.page].WriteString("\n")
	} else {
		f.buffer.ReadFrom(r)
		f.buffer.WriteString("\n")
	}
}

func (f *Fpdf) outbytes(b []byte) {
	if f.state == 2 {
		f.pages[f.page].Write(b)
		f.pages[f.page].WriteByte('\n')
	} else {
		f.buffer.Write(b)
		f.buffer.WriteByte('\n')
	}
}

// RawWriteStr writes a string directly to the PDF generation buffer. This is a
// low-level function that is not required for normal PDF construction. An
// understanding of the PDF specification is needed to use this method
// correctly.

func (f *Fpdf) RawWriteStr(str string) {
	f.out(str)
}

// RawWriteBuf writes the contents of the specified buffer directly to the PDF
// generation buffer. This is a low-level function that is not required for
// normal PDF construction. An understanding of the PDF specification is needed
// to use this method correctly.

func (f *Fpdf) RawWriteBuf(r io.Reader) {
	f.outbuf(r)
}

// outf adds a formatted line to the document

func (f *Fpdf) outf(fmtStr string, args ...any) {
	f.out(sprintf(fmtStr, args...))
}
