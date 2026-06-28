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
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return err
	}
	signed, err := sign.AppendBytes(buf.Bytes(), options)
	if err != nil {
		f.SetError(err)
		return err
	}
	n, err := w.Write(signed)
	if err != nil {
		f.SetError(err)
		return err
	}
	if n != len(signed) {
		f.SetError(io.ErrShortWrite)
		return io.ErrShortWrite
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
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return err
	}
	signed, err := sign.AppendBytes(buf.Bytes(), options)
	if err != nil {
		f.SetError(err)
		return err
	}
	file, err := os.Create(fileStr)
	if err != nil {
		f.SetError(err)
		return err
	}
	n, err := file.Write(signed)
	if err != nil {
		_ = file.Close()
		f.SetError(err)
		return err
	}
	if n != len(signed) {
		_ = file.Close()
		f.SetError(io.ErrShortWrite)
		return io.ErrShortWrite
	}
	if err := file.Close(); err != nil {
		f.SetError(err)
		return err
	}
	return nil
}
