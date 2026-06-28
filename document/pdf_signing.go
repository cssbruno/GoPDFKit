// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"io"

	"github.com/cssbruno/gopdfkit/sign"
)

// OutputSigned writes the current document as a signed PDF.
func (f *Document) OutputSigned(w io.Writer, options sign.Options) error {
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	signed, err := f.outputSignedBytes(options)
	if err != nil {
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
	return writeFileAtomically(fileStr, true, func(w io.Writer) error {
		return f.OutputSigned(w, options)
	})
}

// OutputSignedFileWithOptions writes the current document as a signed PDF using
// explicit file output options. A zero-value OutputFileOptions keeps the durable
// default.
func (f *Document) OutputSignedFileWithOptions(fileStr string, signOptions sign.Options, fileOptions OutputFileOptions) error {
	if fileStr == "" {
		f.SetError(sign.ErrMissingOutput)
		return sign.ErrMissingOutput
	}
	return writeFileAtomically(fileStr, !fileOptions.DisableSync, func(w io.Writer) error {
		return f.OutputSigned(w, signOptions)
	})
}

func (f *Document) outputSignedBytes(options sign.Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.Output(&buf); err != nil {
		return nil, err
	}
	signed, err := sign.AppendBytes(buf.Bytes(), options)
	if err != nil {
		f.SetError(err)
		return nil, err
	}
	return signed, nil
}
