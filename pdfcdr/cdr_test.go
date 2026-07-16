// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package pdfcdr

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/importpdf"
	"github.com/cssbruno/gopdfkit/inspect"
)

func TestSanitizeRemovesActiveDocumentStructures(t *testing.T) {
	source := activeSourcePDF(t)
	clean, err := Sanitize(source)
	if err != nil {
		t.Fatalf("Sanitize() error = %v", err)
	}
	if !bytes.HasPrefix(clean, []byte("%PDF-1.7")) {
		t.Fatalf("output does not start with a PDF header: %q", clean[:min(len(clean), 16)])
	}
	for _, forbidden := range []string{"/OpenAction", "/JavaScript", "/JS", "/AA", "/Annots", "/Metadata", "/Names"} {
		if bytes.Contains(clean, []byte(forbidden)) {
			t.Fatalf("output contains removed structure %s", forbidden)
		}
	}
	if !bytes.Contains(clean, []byte("Hello CDR")) {
		t.Fatal("output does not preserve page text")
	}
	if pages, err := inspect.PageCount(clean); err != nil || pages != 1 {
		t.Fatalf("PageCount() = %d, error = %v; want one page", pages, err)
	}
	text, err := inspect.Text(clean)
	if err != nil {
		t.Fatalf("inspect.Text() error = %v", err)
	}
	if !strings.Contains(text, "Hello CDR") {
		t.Fatalf("extracted text = %q, want Hello CDR", text)
	}
	pageSource, err := importpdf.OpenBytes(clean)
	if err != nil {
		t.Fatalf("reconstructed PDF cannot be imported: %v", err)
	}
	page, err := pageSource.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("reconstructed page cannot be opened: %v", err)
	}
	if page.ObjectCount() != 2 {
		t.Fatalf("reconstructed page object count = %d, want the resources and font objects", page.ObjectCount())
	}
}

func TestSanitizeRewritesRenderingResourceReferences(t *testing.T) {
	clean, err := Sanitize(nonSequentialResourcePDF(t))
	if err != nil {
		t.Fatalf("Sanitize() error = %v", err)
	}
	pageSource, err := importpdf.OpenBytes(clean)
	if err != nil {
		t.Fatalf("reconstructed PDF cannot be imported: %v", err)
	}
	page, err := pageSource.Page(1, "MediaBox")
	if err != nil {
		t.Fatalf("reconstructed page resources cannot be resolved: %v", err)
	}
	if page.ObjectCount() != 2 {
		t.Fatalf("reconstructed page object count = %d, want the resources and font objects", page.ObjectCount())
	}
}

func TestSanitizeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SanitizeContext(ctx, activeSourcePDF(t), Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SanitizeContext() error = %v, want context.Canceled", err)
	}
}

func TestSanitizeFileAtomicOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.pdf")
	output := filepath.Join(dir, "output.pdf")
	if err := os.WriteFile(input, activeSourcePDF(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SanitizeFile(input, output); err != nil {
		t.Fatalf("SanitizeFile() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importpdf.OpenBytes(data); err != nil {
		t.Fatalf("output file is not a PDF: %v", err)
	}
	if mode := fileMode(t, output); mode.Perm() != 0o600 {
		t.Fatalf("output mode = %o, want 600", mode.Perm())
	}
}

func TestSanitizeRejectsUnsupportedInput(t *testing.T) {
	_, err := Sanitize(42)
	if !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("Sanitize() error = %v, want ErrInvalidSource", err)
	}
}

func activeSourcePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 9)
	for i := range ids {
		ids[i] = builder.reserve()
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R /OpenAction 7 0 R /Names << /JavaScript << /Names [(run) 7 0 R] >> >> >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources 5 0 R /Contents 4 0 R /Annots [9 0 R] >>"))
	builder.set(ids[3], pdfStreamBody([]byte("BT /F1 12 Tf 72 72 Td (Hello CDR) Tj ET")))
	builder.set(ids[4], []byte("<< /Font << /F1 6 0 R >> /AA 7 0 R /Metadata 8 0 R >>"))
	builder.set(ids[5], []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	builder.set(ids[6], []byte("<< /Type /Action /S /JavaScript /JS (app.alert) >>"))
	builder.set(ids[7], []byte("<< /Type /Metadata /Subtype /XML /Length 11 >>\nstream\n<xml></xml>\nendstream"))
	builder.set(ids[8], []byte("<< /Type /Annot /Subtype /Link /A 7 0 R /Rect [0 0 1 1] >>"))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func nonSequentialResourcePDF(t testing.TB) []byte {
	t.Helper()
	builder := &pdfBuilder{}
	ids := make([]int, 9)
	for i := range ids {
		ids[i] = builder.reserve()
		builder.set(ids[i], []byte("null"))
	}
	builder.set(ids[0], []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	builder.set(ids[1], []byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"))
	builder.set(ids[2], []byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << /Font << /F1 9 0 R >> >> /Contents 4 0 R >>"))
	builder.set(ids[3], pdfStreamBody([]byte("BT /F1 12 Tf 10 10 Td (resource reference) Tj ET")))
	builder.set(ids[8], []byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	data, err := builder.bytes(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
