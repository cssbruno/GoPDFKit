// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"io"
	"os"
)

// OutputSigned writes the current document as a signed PDF.
func (f *Fpdf) OutputSigned(w io.Writer, options SignOptions) error {
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return err
	}
	signed, err := SignPDFBytes(buf.Bytes(), options)
	if err != nil {
		f.SetError(err)
		return err
	}
	if _, err := w.Write(signed); err != nil {
		f.SetError(err)
		return err
	}
	return nil
}

// OutputSignedFile creates or truncates fileStr and writes the current document
// as a signed PDF.
func (f *Fpdf) OutputSignedFile(fileStr string, options SignOptions) error {
	if fileStr == "" {
		f.SetError(ErrMissingOutput)
		return ErrMissingOutput
	}
	var signed bytes.Buffer
	if err := f.OutputSigned(&signed, options); err != nil {
		return err
	}
	file, err := os.Create(fileStr)
	if err != nil {
		f.SetError(err)
		return err
	}
	if _, err := file.Write(signed.Bytes()); err != nil {
		_ = file.Close()
		f.SetError(err)
		return err
	}
	if err := file.Close(); err != nil {
		f.SetError(err)
		return err
	}
	return nil
}
