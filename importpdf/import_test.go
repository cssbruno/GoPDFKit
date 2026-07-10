// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf_test

import (
	"bytes"
	"fmt"
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

func TestOpenFileKeepsSnapshotAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.pdf")
	original := importPDFWithContent("BT (original) Tj ET")
	replacement := importPDFWithContent("BT (replacement) Tj ET")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("write original source PDF: %v", err)
	}

	source, err := importpdf.OpenFile(path)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := os.WriteFile(path, replacement, 0o600); err != nil {
		t.Fatalf("replace source PDF: %v", err)
	}

	page, err := source.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("Page() after replacement error = %v", err)
	}
	content, err := page.ContentWithError()
	if err != nil {
		t.Fatalf("ContentWithError() after replacement error = %v", err)
	}
	if !bytes.Contains(content, []byte("original")) || bytes.Contains(content, []byte("replacement")) {
		t.Fatalf("imported content = %q, want the original file snapshot", content)
	}
}

func TestSourceCacheKeepsSnapshotAfterReplacement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.pdf")
	if err := os.WriteFile(path, importPDFWithContent("BT (cached original) Tj ET"), 0o600); err != nil {
		t.Fatalf("write original source PDF: %v", err)
	}

	source, err := importpdf.NewSourceCache().OpenFile(path)
	if err != nil {
		t.Fatalf("SourceCache.OpenFile() error = %v", err)
	}
	if err := os.WriteFile(path, importPDFWithContent("BT (cached replacement) Tj ET"), 0o600); err != nil {
		t.Fatalf("replace source PDF: %v", err)
	}

	page, err := source.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("Page() after replacement error = %v", err)
	}
	content, err := page.ContentWithError()
	if err != nil {
		t.Fatalf("ContentWithError() after replacement error = %v", err)
	}
	if !bytes.Contains(content, []byte("cached original")) || bytes.Contains(content, []byte("cached replacement")) {
		t.Fatalf("cached imported content = %q, want the original file snapshot", content)
	}
}

func TestOpenersAcceptContentStreamContainingEndobj(t *testing.T) {
	source := importPDFWithContent("BT (endobj) Tj ET")
	openers := map[string]func() (*importpdf.Source, error){
		"bytes": func() (*importpdf.Source, error) {
			return importpdf.OpenBytes(source)
		},
		"reader-at": func() (*importpdf.Source, error) {
			return importpdf.OpenReaderAt(byteReaderAt(source), int64(len(source)))
		},
	}
	for name, open := range openers {
		t.Run(name, func(t *testing.T) {
			pdf, err := open()
			if err != nil {
				t.Fatalf("open PDF: %v", err)
			}
			page, err := pdf.Page(1, "MediaBox")
			if err != nil {
				t.Fatalf("Page() error = %v", err)
			}
			content, err := page.ContentWithError()
			if err != nil {
				t.Fatalf("ContentWithError() error = %v", err)
			}
			if !bytes.Contains(content, []byte("(endobj)")) {
				t.Fatalf("content = %q, want literal endobj", content)
			}
		})
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
	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
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

func importPDFWithContent(content string) []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 10 10] /Resources <<>> /Contents 4 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
	}
	var output bytes.Buffer
	output.WriteString("%PDF-1.4\n")
	offsets := make([]int, 1, len(objects)+1)
	for i, object := range objects {
		offsets = append(offsets, output.Len())
		fmt.Fprintf(&output, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := output.Len()
	fmt.Fprintf(&output, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&output, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&output, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%EOF\n", len(offsets), xref)
	return output.Bytes()
}
