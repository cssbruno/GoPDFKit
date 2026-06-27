// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package importpdf_test

import (
	"bytes"
	"math"
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
