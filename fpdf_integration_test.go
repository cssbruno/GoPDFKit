// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"encoding/base64"
	"math"
	"strings"
	"testing"
)

func TestFpdfDocumentFlowSmoke(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetCatalogSort(true)
	pdf.SetTitle("Document flow", false)
	pdf.SetAuthor("cssBruno", false)
	pdf.SetSubject("document flow coverage", false)
	pdf.SetCreator("integration test", false)
	pdf.SetDisplayMode("fullwidth", "continuous")
	pdf.SetMargins(12, 14, 16)
	pdf.SetAutoPageBreak(true, 20)
	pdf.SetPageBox("trim", 1, 2, 50, 60)

	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetDrawColor(30, 60, 90)
	pdf.SetFillColor(230, 240, 250)
	pdf.SetTextColor(10, 20, 30)
	pdf.Rect(15, 18, 30, 12, "DF")
	pdf.Line(15, 35, 45, 35)
	pdf.LinearGradient(50, 18, 25, 12, 255, 255, 255, 160, 190, 220, 50, 18, 75, 18)
	pdf.Text(15, 45, "document flow smoke")

	link := pdf.AddLink()
	pdf.SetLink(link, 10, 1)
	pdf.Link(15, 47, 20, 5, link)
	pdf.LinkString(40, 47, 25, 5, "https://example.test/document")

	pixel := decodeIntegrationPNG(t)
	info := pdf.RegisterImageOptionsReader("integration-pixel", ImageOptions{ImageType: "png"}, bytes.NewReader(pixel))
	if info == nil {
		t.Fatal("RegisterImageOptionsReader returned nil image info")
	}
	pdf.ImageOptions("integration-pixel", 15, 55, 6, 6, false, ImageOptions{ImageType: "png"}, 0, "")
	pdf.RawWriteStr("%integration-raw")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := out.String()
	for _, want := range []string{
		"%PDF-",
		"/Title (Document flow)",
		"/Author (cssBruno)",
		"/TrimBox",
		"/Annots",
		"/Subtype /Image",
		"/Shading",
		"document flow smoke",
		"%integration-raw",
	} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain %q", want)
		}
	}
}

func TestFpdfPageStateRestoredAcrossPages(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 14)
	pdf.SetDrawColor(11, 22, 33)
	pdf.SetFillColor(44, 55, 66)
	pdf.SetTextColor(77, 88, 99)
	pdf.SetLineWidth(0.7)
	pdf.Cell(0, 8, "first page")

	pdf.AddPage()
	if pdf.PageNo() != 2 || pdf.PageCount() != 2 {
		t.Fatalf("page state = page %d count %d, want page 2 count 2", pdf.PageNo(), pdf.PageCount())
	}
	ptSize, _ := pdf.GetFontSize()
	if math.Abs(ptSize-14) > 1e-9 {
		t.Fatalf("font size = %.4f, want 14", ptSize)
	}
	if math.Abs(pdf.GetLineWidth()-0.7) > 1e-9 {
		t.Fatalf("line width = %.4f, want 0.7", pdf.GetLineWidth())
	}
	if r, g, b := pdf.GetDrawColor(); r != 11 || g != 22 || b != 33 {
		t.Fatalf("draw color = %d,%d,%d, want 11,22,33", r, g, b)
	}
	if r, g, b := pdf.GetFillColor(); r != 44 || g != 55 || b != 66 {
		t.Fatalf("fill color = %d,%d,%d, want 44,55,66", r, g, b)
	}
	if r, g, b := pdf.GetTextColor(); r != 77 || g != 88 || b != 99 {
		t.Fatalf("text color = %d,%d,%d, want 77,88,99", r, g, b)
	}

	pdf.Cell(0, 8, "second page")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(out.String(), "second page") {
		t.Fatal("generated PDF does not contain second page text")
	}
}

func decodeIntegrationPNG(t *testing.T) []byte {
	t.Helper()
	const pixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	data, err := base64.StdEncoding.DecodeString(pixelPNG)
	if err != nil {
		t.Fatalf("decode PNG fixture: %v", err)
	}
	return data
}
