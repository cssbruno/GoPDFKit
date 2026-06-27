// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"io"
	"os"

	"github.com/cssbruno/gopdfkit/sign"
)

// OutputSigned writes the current document as a signed PDF.
func (f *Document) OutputSigned(w io.Writer, options sign.Options) error {
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return err
	}
	signed, err := sign.AppendBytes(buf.Bytes(), options)
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
func (f *Document) OutputSignedFile(fileStr string, options sign.Options) error {
	if fileStr == "" {
		f.SetError(sign.ErrMissingOutput)
		return sign.ErrMissingOutput
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
