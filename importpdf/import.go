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

// StreamPageImporter is implemented by document types that can import a page from a stream.
type StreamPageImporter interface {
	ImportPageStream(source io.Reader, pageNo int, box string) int
}

// PageUser is implemented by document types that can draw an imported page.
type PageUser interface {
	UseImportedPage(pageID int, x, y, w, h float64)
}

// Open parses a PDF source. source may be a file path string, []byte, or io.Reader.
func Open(source any) (*Source, error) {
	switch src := source.(type) {
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
	return OpenReader(file)
}

// OpenBytes parses PDF bytes.
func OpenBytes(data []byte) (*Source, error) {
	if len(data) > MaxSourceBytes {
		return nil, errors.New("PDF import source exceeds maximum size")
	}
	return parseSource(append([]byte(nil), data...))
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
	return parseSource(data)
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

// PageStream imports a page from source into pdf and returns its page ID.
func PageStream(pdf StreamPageImporter, source io.Reader, pageNo int, box string) int {
	return pdf.ImportPageStream(source, pageNo, box)
}

// UsePage draws an imported page on pdf.
func UsePage(pdf PageUser, pageID int, x, y, w, h float64) {
	pdf.UseImportedPage(pageID, x, y, w, h)
}
