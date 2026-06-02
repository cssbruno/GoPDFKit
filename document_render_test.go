// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteDocumentRendersSharedBlocks(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	doc := NewDocument(DocumentKindReport)
	doc.Title = "Shared renderer"
	doc.Metadata.Subject = "Renderer test"
	doc.Header = &HeaderBlock{
		Height: 8,
		Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Header text"}}, Style: TextStyle{FontSize: 9}}},
	}
	doc.Footer = &FooterBlock{
		Height:          8,
		ShowPageNumber:  true,
		TotalPageAlias:  "{total}",
		ReservePageArea: true,
	}
	doc.Body = []Block{
		HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Shared Document"}}},
		MetadataGridBlock{Fields: []MetadataField{{Label: "ID", Value: "ABC-123"}, {Label: "Status", Value: "Ready"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "The shared renderer writes model blocks into PDF output."}}},
		ListBlock{Items: []ListItem{
			{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "First item"}}}}},
			{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Second item"}}}}},
		}},
		TableBlock{
			Caption: "Sample table",
			Header:  []TableRow{{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Name"}}}}}, {Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Value"}}}}}}}},
			Body:    []TableRow{{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Alpha"}}}}}, {Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "42"}}}}}}}},
		},
		QRVerificationBlock{QR: QRBlock{Label: "Verify", URL: "https://example.test/verify", Size: 18}},
	}
	doc.Signature = &SignatureBlock{Rows: []SignatureRowBlock{{Columns: []SignatureColumn{{Label: "Primary"}, {Label: "Secondary"}}}}}

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := out.String()
	for _, want := range []string{
		"Header text",
		"Shared Document",
		"ID: ABC-123",
		"Sample table",
		"Alpha",
		"Verify",
		"Primary",
		"Page 1 / 1",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("PDF output missing %q", want)
		}
	}
}

func TestWriteDocumentPageBreakBlockAddsPage(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	doc := NewDocument(DocumentKindGeneric)
	doc.Body = []Block{
		ParagraphBlock{Segments: []TextSegment{{Text: "before break"}}},
		PageBreakBlock{After: true},
		ParagraphBlock{Segments: []TextSegment{{Text: "after break"}}},
	}

	pdf.WriteDocument(doc)
	if got := pdf.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}
}

func TestWriteDocumentErrorsForUnknownFont(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	doc := NewDocument(DocumentKindGeneric)
	doc.Body = []Block{
		ParagraphBlock{
			Segments: []TextSegment{{Text: "font error text"}},
			Style:    TextStyle{FontFamily: "MissingFont", Bold: true, Italic: true},
		},
	}

	pdf.WriteDocument(doc)
	if err := pdf.Error(); err == nil || !strings.Contains(err.Error(), "undefined font: missingfont BI") {
		t.Fatalf("Error() = %v, want undefined font error", err)
	}
}

func TestWriteDocumentErrorsForUnavailableBoldItalicFace(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.fonts["custom"] = fontDefinition{}
	doc := NewDocument(DocumentKindGeneric)
	doc.Body = []Block{
		ParagraphBlock{
			Segments: []TextSegment{{Text: "font face error text"}},
			Style:    TextStyle{FontFamily: "custom", Bold: true, Italic: true},
		},
	}

	pdf.WriteDocument(doc)
	if err := pdf.Error(); err == nil || !strings.Contains(err.Error(), "undefined font: custom BI") {
		t.Fatalf("Error() = %v, want undefined custom bold/italic font error", err)
	}
}

func TestWriteDocumentRendersSignatureMetadata(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	doc := NewDocument(DocumentKindGeneric)
	doc.Signature = &SignatureBlock{Rows: []SignatureRowBlock{{
		Columns: []SignatureColumn{{
			Label: "Signed by",
			Name:  "Alex Example",
			Role:  "Reviewer",
			Metadata: []MetadataField{
				{Label: "ID", Value: "123"},
			},
		}},
	}}}

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := out.String()
	for _, want := range []string{"Signed by", "Alex Example", "Reviewer", "ID: 123"} {
		if !strings.Contains(content, want) {
			t.Fatalf("PDF output missing signature metadata %q", want)
		}
	}
}

func TestWriteDocumentErrorsForEmptyQRVerificationBlock(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	doc := NewDocument(DocumentKindGeneric)
	doc.Body = []Block{QRVerificationBlock{QR: QRBlock{Label: "Verify"}}}

	pdf.WriteDocument(doc)
	if err := pdf.Error(); err == nil || !strings.Contains(err.Error(), "QR verification block requires a value or URL") {
		t.Fatalf("Error() = %v, want QR value error", err)
	}
}

func TestDocumentListMarkerWidthUsesWidestMarker(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	renderer := documentRenderer{pdf: pdf}

	oneDigitWidth := renderer.listMarkerWidth(ListBlock{Ordered: true, Items: make([]ListItem, 9)})
	twoDigitWidth := renderer.listMarkerWidth(ListBlock{Ordered: true, Items: make([]ListItem, 10)})
	if twoDigitWidth <= oneDigitWidth {
		t.Fatalf("two-digit marker width = %.2f, one-digit = %.2f, want two-digit wider", twoDigitWidth, oneDigitWidth)
	}
}
