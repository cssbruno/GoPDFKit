// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf_test

import (
	"bytes"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/importpdf"
)

func TestOpenBytesPageAndSizes(t *testing.T) {
	source := importSourcePDF(t)

	pdf, err := importpdf.OpenBytes(source)
	if err != nil {
		t.Fatalf("OpenBytes() error = %v", err)
	}
	if got := pdf.PageCount(); got != 1 {
		t.Fatalf("PageCount() = %d, want 1", got)
	}

	page, err := pdf.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	if math.Abs(page.WidthPoints()-595.28) > 0.01 || math.Abs(page.HeightPoints()-841.89) > 0.01 {
		t.Fatalf("unexpected page size %.2fx%.2f", page.WidthPoints(), page.HeightPoints())
	}
	if len(page.Content()) == 0 {
		t.Fatal("expected imported page content")
	}
	if len(page.Resources()) == 0 {
		t.Fatal("expected imported page resources")
	}

	sizes, err := importpdf.GetPageSizes(bytes.NewReader(source))
	if err != nil {
		t.Fatalf("GetPageSizes() error = %v", err)
	}
	if got := sizes[1]["MediaBox"]; math.Abs(got.Wd-595.28) > 0.01 || math.Abs(got.Ht-841.89) > 0.01 {
		t.Fatalf("unexpected MediaBox size: %#v", got)
	}
}

func TestOpenBytesImmutablePageAndSizes(t *testing.T) {
	source := importSourcePDF(t)

	pdf, err := importpdf.OpenBytesImmutable(source)
	if err != nil {
		t.Fatalf("OpenBytesImmutable() error = %v", err)
	}
	if got := pdf.PageCount(); got != 1 {
		t.Fatalf("PageCount() = %d, want 1", got)
	}
	if _, err := pdf.Page(1, "MediaBox"); err != nil {
		t.Fatalf("Page() error = %v", err)
	}
}

func TestOpenReaderAtPageAndSizes(t *testing.T) {
	source := importSourcePDF(t)

	pdf, err := importpdf.OpenReaderAt(byteReaderAt(source), int64(len(source)))
	if err != nil {
		t.Fatalf("OpenReaderAt() error = %v", err)
	}
	if got := pdf.PageCount(); got != 1 {
		t.Fatalf("PageCount() = %d, want 1", got)
	}
	page, err := pdf.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("Page() error = %v", err)
	}
	if math.Abs(page.WidthPoints()-595.28) > 0.01 || math.Abs(page.HeightPoints()-841.89) > 0.01 {
		t.Fatalf("unexpected page size %.2fx%.2f", page.WidthPoints(), page.HeightPoints())
	}
}

func TestSourceCacheOpenFileReusesParsedSource(t *testing.T) {
	source := importSourcePDF(t)
	path := filepath.Join(t.TempDir(), "source.pdf")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("write source PDF: %v", err)
	}
	cache := importpdf.NewSourceCache()
	first, err := cache.OpenFile(path)
	if err != nil {
		t.Fatalf("first OpenFile(cache) error = %v", err)
	}
	second, err := cache.OpenFile(path)
	if err != nil {
		t.Fatalf("second OpenFile(cache) error = %v", err)
	}
	if first != second {
		t.Fatal("SourceCache returned different source pointers for unchanged file")
	}
	if _, err := importpdf.Open(first); err != nil {
		t.Fatalf("Open(*Source) error = %v", err)
	}
}

func TestSourceCacheCanonicalizesFilePath(t *testing.T) {
	source := importSourcePDF(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "source.pdf")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("write source PDF: %v", err)
	}
	t.Chdir(dir)

	cache := importpdf.NewSourceCache()
	first, err := cache.OpenFile("source.pdf")
	if err != nil {
		t.Fatalf("relative OpenFile(cache) error = %v", err)
	}
	second, err := cache.OpenFile(path)
	if err != nil {
		t.Fatalf("absolute OpenFile(cache) error = %v", err)
	}
	if first != second {
		t.Fatal("SourceCache did not reuse equivalent relative and absolute paths")
	}
}

func TestSourceCacheEvictsByByteBudget(t *testing.T) {
	source := importSourcePDF(t)
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.pdf")
	secondPath := filepath.Join(dir, "second.pdf")
	if err := os.WriteFile(firstPath, source, 0o600); err != nil {
		t.Fatalf("write first PDF: %v", err)
	}
	if err := os.WriteFile(secondPath, source, 0o600); err != nil {
		t.Fatalf("write second PDF: %v", err)
	}

	cache := importpdf.NewSourceCacheWithMaxBytes(int64(len(source)))
	first, err := cache.OpenFile(firstPath)
	if err != nil {
		t.Fatalf("first OpenFile(cache) error = %v", err)
	}
	if _, err := cache.OpenFile(secondPath); err != nil {
		t.Fatalf("second OpenFile(cache) error = %v", err)
	}
	reopened, err := cache.OpenFile(firstPath)
	if err != nil {
		t.Fatalf("reopen first OpenFile(cache) error = %v", err)
	}
	if reopened == first {
		t.Fatal("SourceCache kept first source after byte-budget eviction")
	}
}

type byteReaderAt []byte

func (r byteReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r)) {
		return 0, os.ErrInvalid
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func importSourcePDF(t *testing.T) []byte {
	t.Helper()
	pdf := document.New("P", "pt", "A4", "")
	pdf.SetCompression(true)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 16)
	pdf.Text(72, 96, "Imported PDF source page")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("source Output() error = %v", err)
	}
	return out.Bytes()
}
