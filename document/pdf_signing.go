// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"io"

	"github.com/cssbruno/gopdfkit/sign"
)

// OutputSigned writes the current document as a signed PDF.
func (f *Document) OutputSigned(w io.Writer, options sign.Options) error {
	return f.OutputSignedContext(context.Background(), w, options)
}

// OutputSignedContext writes the current document as a signed PDF and checks
// ctx before generation/signing and before the final writer call.
func (f *Document) OutputSignedContext(ctx context.Context, w io.Writer, options sign.Options) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if isNilWriter(w) {
		f.SetError(ErrNilWriter)
		return f.err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return err
	}
	signed, err := f.outputSignedBytesContext(ctx, options)
	if err != nil {
		return err
	}
	if err := outputCanceledError(ctx); err != nil {
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
	return writeFileAtomically(fileStr, !f.outputPolicy.DisableSync, func(w io.Writer) error {
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
	return writeFileAtomically(fileStr, f.syncOutputForOptions(fileOptions), func(w io.Writer) error {
		return f.OutputSignedWithOptions(w, signOptions, fileOptions)
	})
}

// OutputSignedWithOptions writes the current document as a signed PDF using
// output-wide options before signing.
func (f *Document) OutputSignedWithOptions(w io.Writer, signOptions sign.Options, outputOptions OutputOptions) error {
	if err := f.applyOutputOptions(outputOptions); err != nil {
		return err
	}
	return f.OutputSignedContext(context.Background(), w, signOptions)
}

func (f *Document) outputSignedBytes(options sign.Options) ([]byte, error) {
	return f.outputSignedBytesContext(context.Background(), options)
}

func (f *Document) outputSignedBytesContext(ctx context.Context, options sign.Options) ([]byte, error) {
	var buf bytes.Buffer
	if err := f.OutputContext(ctx, &buf); err != nil {
		return nil, err
	}
	if err := outputCanceledError(ctx); err != nil {
		f.SetError(err)
		return nil, err
	}
	signed, err := sign.AppendBytes(buf.Bytes(), options)
	if err != nil {
		f.SetError(err)
		return nil, err
	}
	return signed, nil
}
