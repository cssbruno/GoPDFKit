// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

func TestWriteDocumentRendersSharedBlocks(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	doc := NewLayoutDocument(DocumentKindReport)
	doc.Title = "Shared renderer"
	doc.Metadata.Subject = "Renderer test"
	doc.PageTemplate.Header = &HeaderBlock{
		Height: 8,
		Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Header text"}}, Style: TextStyle{FontSize: 9}}},
	}
	doc.PageTemplate.Footer = &FooterBlock{
		Height:          8,
		ReservePageArea: true,
	}
	doc.PageTemplate.PageNumbers = PageNumberOptions{Enabled: true, TotalPageAlias: "{total}"}
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

func TestWriteDocumentEmitsTaggedRoles(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Tagged document"})
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.Body = []Block{
		HeadingBlock{Level: 2, Segments: []TextSegment{{Text: "Tagged heading"}}},
		ParagraphBlock{Segments: []TextSegment{{Text: "Tagged paragraph"}}},
		ListBlock{Items: []ListItem{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Tagged item"}}}}}}},
		TableBlock{
			Header: []TableRow{{Cells: []TableCell{{ColSpan: 2, Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Head"}}}}}}}},
			Body:   []TableRow{{Cells: []TableCell{{RowSpan: 2, Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Body"}}}}}, {Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "More"}}}}}}}},
		},
	}

	pdf.WriteDocument(doc)
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	text := output.String()
	for _, want := range []string{
		"/StructTreeRoot ",
		"/H2 <</MCID 0>> BDC",
		"/P <</MCID 1>> BDC",
		"/Lbl <</MCID 2>> BDC",
		"/P <</MCID 3>> BDC",
		"/S /H2",
		"/S /P",
		"/S /L",
		"/S /LI",
		"/S /Lbl",
		"/S /LBody",
		"/S /Table",
		"/S /TR",
		"/S /TH",
		"/S /TD",
		"/A << /O /Table /Scope /Column /ColSpan 2 >>",
		"/A << /O /Table /RowSpan 2 >>",
		"/Artifact BMC",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tagged document output missing %q", want)
		}
	}
}

func TestWriteDocumentPageBreakBlockAddsPage(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	doc := NewLayoutDocument(DocumentKindGeneric)
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
	doc := NewLayoutDocument(DocumentKindGeneric)
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
	doc := NewLayoutDocument(DocumentKindGeneric)
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
	doc := NewLayoutDocument(DocumentKindGeneric)
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
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.Body = []Block{QRVerificationBlock{QR: QRBlock{Label: "Verify"}}}

	pdf.WriteDocument(doc)
	if err := pdf.Error(); err == nil || !strings.Contains(err.Error(), "QR verification block requires a value or URL") {
		t.Fatalf("Error() = %v, want QR value error", err)
	}
}

func TestCellFormatUTF8JustifiedSingleWordDoesNotWriteInvalidNumber(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	fontBytes, err := os.ReadFile("../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	pdf.AddUTF8FontFromBytes("dejavu", "", fontBytes)
	pdf.SetFont("dejavu", "", 12)
	pdf.AddPage()

	pdf.CellFormat(80, 8, "Alone", "", 1, "J", false, 0, "")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := out.String()
	if strings.Contains(content, "+Inf") || strings.Contains(content, "-Inf") || strings.Contains(content, "NaN") {
		t.Fatalf("PDF output contains invalid numeric token")
	}
}

func TestWriteDocumentAppliesPageTemplateMargins(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.PageTemplate.Margins = Spacing{Left: 18, Top: 16, Right: 14, Bottom: 22}
	doc.Body = []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Body"}}}}

	pdf.WriteDocument(doc)

	left, top, right, bottom := pdf.GetMargins()
	if left != 18 || top != 16 || right != 14 || bottom != 22 {
		t.Fatalf("margins = %.1f %.1f %.1f %.1f, want 18 16 14 22", left, top, right, bottom)
	}
}

func TestWriteDocumentRendersTemplateFooterOnEveryRendererPage(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.PageTemplate.Footer = &FooterBlock{
		Height:          8,
		ReservePageArea: true,
		Blocks:          []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Repeated footer"}}}},
	}
	doc.Body = []Block{
		ParagraphBlock{Segments: []TextSegment{{Text: "Page one"}}},
		PageBreakBlock{After: true},
		ParagraphBlock{Segments: []TextSegment{{Text: "Page two"}}},
	}

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if got := strings.Count(out.String(), "Repeated footer"); got != 2 {
		t.Fatalf("footer count = %d, want 2", got)
	}
}

func TestWriteDocumentSelectsTemplateHeadersAndFootersPerPage(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.PageTemplate.Header = &HeaderBlock{
		Height: 6,
		Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Default header"}}}},
	}
	doc.PageTemplate.FirstPageHeader = &HeaderBlock{
		Height: 6,
		Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "First header"}}}},
	}
	doc.PageTemplate.Footer = &FooterBlock{
		Height:          6,
		ReservePageArea: true,
		Blocks:          []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Default footer"}}}},
	}
	doc.PageTemplate.FirstPageFooter = &FooterBlock{
		Height:          6,
		ReservePageArea: true,
		Blocks:          []Block{ParagraphBlock{Segments: []TextSegment{{Text: "First footer"}}}},
	}
	doc.PageTemplate.EvenPageFooter = &FooterBlock{
		Height:          6,
		ReservePageArea: true,
		Blocks:          []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Even footer"}}}},
	}
	doc.Body = []Block{
		ParagraphBlock{Segments: []TextSegment{{Text: "Page one body"}}},
		PageBreakBlock{After: true},
		ParagraphBlock{Segments: []TextSegment{{Text: "Page two body"}}},
		PageBreakBlock{After: true},
		ParagraphBlock{Segments: []TextSegment{{Text: "Page three body"}}},
	}

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := out.String()
	for _, want := range []string{"First header", "Default header", "First footer", "Even footer", "Default footer"} {
		if !strings.Contains(content, want) {
			t.Fatalf("PDF output missing %q", want)
		}
	}
	if got := strings.Count(content, "Default header"); got != 2 {
		t.Fatalf("default header count = %d, want 2", got)
	}
}

func TestWriteDocumentMapsLayoutAttachments(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.Attachments = []AttachmentBlock{{
		Name:        "evidence.txt",
		Description: "Evidence",
		Data:        []byte("attached"),
	}}

	pdf.WriteDocument(doc)

	if len(pdf.attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(pdf.attachments))
	}
	if pdf.attachments[0].Filename != "evidence.txt" || !bytes.Equal(pdf.attachments[0].Content, []byte("attached")) {
		t.Fatalf("attachment = %#v, want mapped layout attachment", pdf.attachments[0])
	}
}

func TestWriteDocumentInlineImagesUseContentHashAndFit(t *testing.T) {
	pixel := decodeDocumentRenderTestPNG(t)
	pdf := New("P", "mm", "A4", "")
	doc := NewLayoutDocument(DocumentKindGeneric)
	doc.Body = []Block{
		ImageBlock{Data: pixel, Format: "png", Width: 16, Height: 8, Fit: ImageFitContain},
		ImageBlock{Data: pixel, Format: "png", Width: 16, Height: 8, Fit: ImageFitCover},
	}

	pdf.WriteDocument(doc)

	if err := pdf.Error(); err != nil {
		t.Fatalf("WriteDocument() error = %v", err)
	}
	if got := len(pdf.images); got != 1 {
		t.Fatalf("registered images = %d, want deterministic reuse of identical inline data", got)
	}
	for name := range pdf.images {
		if !strings.HasPrefix(name, "document-image-") {
			t.Fatalf("registered image name = %q, want hash-based document image name", name)
		}
	}
}

func decodeDocumentRenderTestPNG(t *testing.T) []byte {
	t.Helper()
	const pixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	data, err := base64.StdEncoding.DecodeString(pixelPNG)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	return data
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
