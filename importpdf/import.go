// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// MaxSourceBytes is the largest PDF source accepted by the built-in importer.
const MaxSourceBytes = 128 * 1024 * 1024

// ErrSourceTooLarge reports that a PDF source exceeded the configured byte
// limit.
var ErrSourceTooLarge = errors.New("PDF import source exceeds maximum size")

// ImportOptions controls parser limits for the built-in PDF importer. Zero
// fields use package defaults.
type ImportOptions struct {
	MaxSourceBytes       int64
	MaxReferencedObjects int
}

func normalizeImportOptions(options ImportOptions) (ImportOptions, error) {
	if options.MaxSourceBytes == 0 {
		options.MaxSourceBytes = MaxSourceBytes
	}
	if options.MaxReferencedObjects == 0 {
		options.MaxReferencedObjects = MaxReferencedObjects
	}
	if options.MaxSourceBytes < 0 {
		return ImportOptions{}, errors.New("PDF import source size limit is invalid")
	}
	if options.MaxReferencedObjects < 0 {
		return ImportOptions{}, errors.New("PDF referenced object limit is invalid")
	}
	if options.MaxSourceBytes > int64(math.MaxInt) {
		return ImportOptions{}, errors.New("PDF import source size limit is too large")
	}
	return options, nil
}

// Open parses a PDF source. source may be a file path string, []byte, or io.Reader.
func Open(source any) (*Source, error) {
	return OpenWithOptions(source, ImportOptions{})
}

// OpenWithOptions parses a PDF source using explicit parser limits. source may
// be a file path string, []byte, io.Reader, or *Source.
func OpenWithOptions(source any, options ImportOptions) (*Source, error) {
	return OpenWithOptionsContext(context.Background(), source, options)
}

// OpenWithOptionsContext parses a PDF source using explicit parser limits and
// checks ctx before and after bounded reads/parsing.
func OpenWithOptionsContext(ctx context.Context, source any, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	switch src := source.(type) {
	case *Source:
		if src == nil {
			return nil, errors.New("PDF import source is nil")
		}
		options, err := normalizeImportOptions(options)
		if err != nil {
			return nil, err
		}
		return src.withOptions(options)
	case string:
		return OpenFileWithOptionsContext(ctx, src, options)
	case []byte:
		return OpenBytesWithOptionsContext(ctx, src, options)
	case io.Reader:
		return OpenReaderWithOptionsContext(ctx, src, options)
	default:
		return nil, fmt.Errorf("unsupported PDF import source type %T", source)
	}
}

// OpenFile parses a PDF file.
func OpenFile(path string) (*Source, error) {
	return OpenFileWithOptions(path, ImportOptions{})
}

// OpenFileWithOptions parses a PDF file using explicit parser limits.
func OpenFileWithOptions(path string, options ImportOptions) (*Source, error) {
	return OpenFileWithOptionsContext(context.Background(), path, options)
}

// OpenFileWithOptionsContext parses a PDF file using explicit parser limits and
// checks ctx before opening and after parsing.
func OpenFileWithOptionsContext(ctx context.Context, path string, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path) // #nosec G304 -- OpenFile is an explicit caller-path API and enforces source limits.
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if info, err := file.Stat(); err == nil && info.Mode().IsRegular() && info.Size() > options.MaxSourceBytes {
		return nil, ErrSourceTooLarge
	}
	// Keep one immutable snapshot for the Source lifetime. Reopening path during
	// lazy reads could otherwise combine the xref from this file with objects
	// from a later replacement at the same path.
	return OpenReaderWithOptionsContext(ctx, file, options)
}

// OpenBytes parses PDF bytes.
func OpenBytes(data []byte) (*Source, error) {
	return OpenBytesWithOptions(data, ImportOptions{})
}

// OpenBytesWithOptions parses PDF bytes using explicit parser limits.
func OpenBytesWithOptions(data []byte, options ImportOptions) (*Source, error) {
	return OpenBytesWithOptionsContext(context.Background(), data, options)
}

// OpenBytesWithOptionsContext parses PDF bytes using explicit parser limits and
// checks ctx before copying and after parsing.
func OpenBytesWithOptionsContext(ctx context.Context, data []byte, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > options.MaxSourceBytes {
		return nil, ErrSourceTooLarge
	}
	return OpenBytesImmutableWithOptionsContext(ctx, append([]byte(nil), data...), options)
}

// OpenBytesImmutable parses PDF bytes without copying them. The caller must not
// mutate data while the returned Source is in use.
func OpenBytesImmutable(data []byte) (*Source, error) {
	return OpenBytesImmutableWithOptions(data, ImportOptions{})
}

// OpenBytesImmutableWithOptions parses PDF bytes without copying them using
// explicit parser limits. The caller must not mutate data while the returned
// Source is in use.
func OpenBytesImmutableWithOptions(data []byte, options ImportOptions) (*Source, error) {
	return OpenBytesImmutableWithOptionsContext(context.Background(), data, options)
}

// OpenBytesImmutableWithOptionsContext parses PDF bytes without copying them
// using explicit parser limits and checks ctx before and after parsing. The
// caller must not mutate data while the returned Source is in use.
func OpenBytesImmutableWithOptionsContext(ctx context.Context, data []byte, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > options.MaxSourceBytes {
		return nil, ErrSourceTooLarge
	}
	source, err := parseSourceWithOptionsContext(ctx, data, options)
	if err != nil {
		return nil, err
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return source, nil
}

// OpenReaderAt parses a seekable PDF source without copying the whole file.
// The caller must keep r readable while the returned Source is used.
func OpenReaderAt(r io.ReaderAt, size int64) (*Source, error) {
	return OpenReaderAtWithOptions(r, size, ImportOptions{})
}

// OpenReaderAtWithOptions parses a seekable PDF source using explicit parser
// limits. The caller must keep r readable while the returned Source is used.
func OpenReaderAtWithOptions(r io.ReaderAt, size int64, options ImportOptions) (*Source, error) {
	return OpenReaderAtWithOptionsContext(context.Background(), r, size, options)
}

// OpenReaderAtWithOptionsContext parses a seekable PDF source using explicit
// parser limits and checks ctx before and after parsing. The caller must keep r
// readable while the returned Source is used.
func OpenReaderAtWithOptionsContext(ctx context.Context, r io.ReaderAt, size int64, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errors.New("PDF import source is nil")
	}
	if size < 0 {
		return nil, errors.New("PDF import source size is invalid")
	}
	if size > options.MaxSourceBytes {
		return nil, ErrSourceTooLarge
	}
	source, err := parseSourceReaderAtWithOptionsContext(ctx, r, size, options)
	if err != nil {
		return nil, err
	}
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	return source, nil
}

// OpenReader reads and parses a PDF stream.
func OpenReader(r io.Reader) (*Source, error) {
	return OpenReaderWithOptions(r, ImportOptions{})
}

// OpenReaderWithOptions reads and parses a PDF stream using explicit parser
// limits.
func OpenReaderWithOptions(r io.Reader, options ImportOptions) (*Source, error) {
	return OpenReaderWithOptionsContext(context.Background(), r, options)
}

// OpenReaderWithOptionsContext reads and parses a PDF stream using explicit
// parser limits and checks ctx before and during bounded reads.
func OpenReaderWithOptionsContext(ctx context.Context, r io.Reader, options ImportOptions) (*Source, error) {
	if err := importContextErr(ctx); err != nil {
		return nil, err
	}
	options, err := normalizeImportOptions(options)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, errors.New("PDF import source is nil")
	}
	data, err := io.ReadAll(io.LimitReader(importContextReader{ctx: ctx, r: r}, options.MaxSourceBytes+1))
	if err == nil && int64(len(data)) > options.MaxSourceBytes {
		err = ErrSourceTooLarge
	}
	if err != nil {
		return nil, err
	}
	return OpenBytesImmutableWithOptionsContext(ctx, data, options)
}

// GetPageSizes returns available page box sizes for a PDF source. Sizes are
// reported in PDF points.
func GetPageSizes(source any) (map[int]map[string]Size, error) {
	src, err := Open(source)
	if err != nil {
		return nil, err
	}
	return src.PageSizes(), nil
}

func importContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

type importContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r importContextReader) Read(p []byte) (int, error) {
	if err := importContextErr(r.ctx); err != nil {
		return 0, err
	}
	n, err := r.r.Read(p)
	if err != nil {
		return n, err
	}
	if n == 0 {
		return n, importContextErr(r.ctx)
	}
	return n, nil
}
