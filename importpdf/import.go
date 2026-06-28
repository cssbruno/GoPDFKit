// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// MaxSourceBytes is the largest PDF source accepted by the built-in importer.
const MaxSourceBytes = 128 * 1024 * 1024

// PageImporter is implemented by document types that can import a page from a file.
type PageImporter interface {
	ImportPage(sourceFile string, pageNo int, box string) int
}

// PageImporterError is implemented by document types that can import a page
// from a file and report errors directly.
type PageImporterError interface {
	ImportPageError(sourceFile string, pageNo int, box string) (int, error)
}

// StreamPageImporter is implemented by document types that can import a page from a stream.
type StreamPageImporter interface {
	ImportPageStream(source io.Reader, pageNo int, box string) int
}

// StreamPageImporterError is implemented by document types that can import a
// page from a stream and report errors directly.
type StreamPageImporterError interface {
	ImportPageStreamError(source io.Reader, pageNo int, box string) (int, error)
}

// PageUser is implemented by document types that can draw an imported page.
type PageUser interface {
	UseImportedPage(pageID int, x, y, w, h float64)
}

// PageUserError is implemented by document types that can draw an imported
// page and report errors directly.
type PageUserError interface {
	UseImportedPageError(pageID int, x, y, w, h float64) error
}

// Open parses a PDF source. source may be a file path string, []byte, or io.Reader.
func Open(source any) (*Source, error) {
	switch src := source.(type) {
	case *Source:
		if src == nil {
			return nil, errors.New("PDF import source is nil")
		}
		return src, nil
	case string:
		return OpenFile(src)
	case []byte:
		return OpenBytes(src)
	case io.Reader:
		return OpenReader(src)
	default:
		return nil, fmt.Errorf("unsupported PDF import source type %T", source)
	}
}

// OpenFile parses a PDF file.
func OpenFile(path string) (*Source, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if info, err := file.Stat(); err == nil && info.Mode().IsRegular() && info.Size() > MaxSourceBytes {
		return nil, errors.New("PDF import source exceeds maximum size")
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	source, err := OpenReaderAt(file, info.Size())
	if err != nil {
		return nil, err
	}
	source.path = path
	source.readerAt = nil
	return source, nil
}

// OpenBytes parses PDF bytes.
func OpenBytes(data []byte) (*Source, error) {
	if len(data) > MaxSourceBytes {
		return nil, errors.New("PDF import source exceeds maximum size")
	}
	return OpenBytesImmutable(append([]byte(nil), data...))
}

// OpenBytesImmutable parses PDF bytes without copying them. The caller must not
// mutate data while the returned Source is in use.
func OpenBytesImmutable(data []byte) (*Source, error) {
	if len(data) > MaxSourceBytes {
		return nil, errors.New("PDF import source exceeds maximum size")
	}
	return parseSource(data)
}

// OpenReaderAt parses a seekable PDF source without copying the whole file.
// The caller must keep r readable while the returned Source is used.
func OpenReaderAt(r io.ReaderAt, size int64) (*Source, error) {
	if r == nil {
		return nil, errors.New("PDF import source is nil")
	}
	if size < 0 {
		return nil, errors.New("PDF import source size is invalid")
	}
	if size > MaxSourceBytes {
		return nil, errors.New("PDF import source exceeds maximum size")
	}
	return parseSourceReaderAt(r, size)
}

// OpenReader reads and parses a PDF stream.
func OpenReader(r io.Reader) (*Source, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxSourceBytes+1))
	if err == nil && len(data) > MaxSourceBytes {
		err = errors.New("PDF import source exceeds maximum size")
	}
	if err != nil {
		return nil, err
	}
	return OpenBytesImmutable(data)
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

// Page imports a page from sourceFile into pdf and returns its page ID.
func Page(pdf PageImporter, sourceFile string, pageNo int, box string) int {
	return pdf.ImportPage(sourceFile, pageNo, box)
}

// PageError imports a page from sourceFile into pdf and returns its page ID or
// an error.
func PageError(pdf PageImporterError, sourceFile string, pageNo int, box string) (int, error) {
	return pdf.ImportPageError(sourceFile, pageNo, box)
}

// PageStream imports a page from source into pdf and returns its page ID.
func PageStream(pdf StreamPageImporter, source io.Reader, pageNo int, box string) int {
	return pdf.ImportPageStream(source, pageNo, box)
}

// PageStreamError imports a page from source into pdf and returns its page ID
// or an error.
func PageStreamError(pdf StreamPageImporterError, source io.Reader, pageNo int, box string) (int, error) {
	return pdf.ImportPageStreamError(source, pageNo, box)
}

// UsePage draws an imported page on pdf.
func UsePage(pdf PageUser, pageID int, x, y, w, h float64) {
	pdf.UseImportedPage(pageID, x, y, w, h)
}

// UsePageError draws an imported page on pdf and reports errors directly.
func UsePageError(pdf PageUserError, pageID int, x, y, w, h float64) error {
	return pdf.UseImportedPageError(pageID, x, y, w, h)
}
