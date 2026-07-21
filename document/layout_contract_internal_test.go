// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
	"github.com/cssbruno/gopdfkit/sign"
)

func TestWriteDocumentAppliesLanguageToCatalog(t *testing.T) {
	pdf := MustNew()
	pdf.SetCompression(false)
	doc := layout.NewLayoutDocument()
	doc.Language = "pt-BR"
	doc.Body = []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Olá"}}}}
	pdf.WriteDocument(doc)
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(output.String(), "/Lang (pt-BR)") {
		t.Fatal("PDF catalog is missing the layout document language")
	}
}

func TestWriteDocumentRendersSegmentStyleAndLink(t *testing.T) {
	pdf := MustNew()
	pdf.SetCompression(false)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{
		{Text: "plain "},
		{Text: "linked", Link: "https://example.test/segment", Style: layout.TextStyle{Bold: true, Color: layout.DocumentColor{R: 180, Set: true}}},
	}}}
	pdf.WriteDocument(doc)
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := output.String()
	if !strings.Contains(content, "/URI (https://example.test/segment)") {
		t.Fatal("styled text segment did not emit its link annotation")
	}
	if !strings.Contains(content, "/BaseFont /Helvetica-Bold") {
		t.Fatal("styled text segment did not register its bold font")
	}
}

func TestWriteDocumentRendersQRCodeImage(t *testing.T) {
	pdf := MustNew()
	pdf.SetCompression(false)
	doc := layout.NewLayoutDocument()
	doc.QR = &layout.QRBlock{Value: "verified-value", Label: "Verify", Size: 18}
	pdf.WriteDocument(doc)
	if image, ok := pdf.ensureResourceStore().image(QRCodeImageName("verified-value")); !ok || image == nil {
		t.Fatal("QR block did not register its deterministic image")
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := output.String()
	if !strings.Contains(content, "/Subtype /Image") {
		t.Fatal("QR block did not render its registered image")
	}
}

func TestTypedTableRepeatsHeaderAcrossPages(t *testing.T) {
	pdf := MustNew(WithCustomPageSize(Size{Wd: 90, Ht: 90}))
	pdf.SetCompression(false)
	doc := layout.NewLayoutDocument()
	doc.PageTemplate.Margins = layout.Spacing{Top: 8, Right: 8, Bottom: 8, Left: 8}
	body := make([]layout.TableRow, 30)
	for i := range body {
		body[i] = layout.TableRow{Cells: []layout.TableCell{{Blocks: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: fmt.Sprintf("row-%02d", i)}}},
		}}}}
	}
	doc.Body = []layout.Block{layout.TableBlock{
		Header: []layout.TableRow{{Cells: []layout.TableCell{{Blocks: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "repeated-header"}}},
		}}}}},
		Body:  body,
		Style: layout.TableStyle{RepeatHeader: true, BorderCollapse: true},
	}}
	pdf.WriteDocument(doc)
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if pdf.PageCount() < 2 {
		t.Fatalf("PageCount() = %d, want a multipage table", pdf.PageCount())
	}
	if count := strings.Count(output.String(), "repeated-header"); count != pdf.PageCount() {
		t.Fatalf("header occurrences = %d, want one on each of %d pages", count, pdf.PageCount())
	}
}

func TestTypedTableLayoutPlacesRowSpanAndHonorsColumnBounds(t *testing.T) {
	pdf := MustNew()
	pdf.AddPage()
	renderer := documentRenderer{pdf: pdf}
	block := layout.TableBlock{
		Columns: []layout.TableColumn{{MinWidth: 40}, {MaxWidth: 20}},
		Body: []layout.TableRow{
			{Cells: []layout.TableCell{{RowSpan: 2, Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "span"}}}}}, {Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "one"}}}}}}},
			{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "two"}}}}}}},
		},
	}
	widths := tableRenderWidths(block, 100, 2)
	if widths[0] != 80 || widths[1] != 20 {
		t.Fatalf("table widths = %v, want [80 20]", widths)
	}
	measured := renderer.measureTableLayout(block, widths)
	if got := measured.rows[0].cells[0].rowSpan; got != 2 {
		t.Fatalf("row span = %d, want 2", got)
	}
	if got := measured.rows[1].cells[0].column; got != 1 {
		t.Fatalf("second-row cell column = %d, want 1 after rowspan occupancy", got)
	}
}

func TestTypedAndHTMLTablesSharePaginationBoundary(t *testing.T) {
	const rowCount = 24
	typedPDF := MustNew(WithCustomPageSize(Size{Wd: 90, Ht: 90}))
	typedDoc := layout.NewLayoutDocument()
	typedDoc.PageTemplate.Margins = layout.Spacing{Top: 8, Right: 8, Bottom: 8, Left: 8}
	typedRows := make([]layout.TableRow, rowCount)
	for i := range typedRows {
		typedRows[i] = layout.TableRow{Cells: []layout.TableCell{{Blocks: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: fmt.Sprintf("row-%02d", i)}}, Style: layout.TextStyle{FontSize: 8, LineHeight: 5}},
		}}}}
	}
	typedDoc.Body = []layout.Block{layout.TableBlock{Body: typedRows, Style: layout.TableStyle{BorderCollapse: true}}}
	typedPDF.WriteDocument(typedDoc)

	htmlPDF := MustNew(WithCustomPageSize(Size{Wd: 90, Ht: 90}))
	htmlPDF.SetMargins(8, 8, 8)
	htmlPDF.SetAutoPageBreak(true, 8)
	htmlPDF.AddPage()
	htmlPDF.SetFont("Helvetica", "", 8)
	var tableHTML strings.Builder
	tableHTML.WriteString(`<table style="border-collapse:collapse;font-size:8pt">`)
	for i := range rowCount {
		fmt.Fprintf(&tableHTML, "<tr><td>row-%02d</td></tr>", i)
	}
	tableHTML.WriteString(`</table>`)
	html := htmlPDF.HTMLNew()
	if err := html.WriteContext(t.Context(), 5, tableHTML.String()); err != nil {
		t.Fatalf("HTML.WriteContext() error = %v", err)
	}
	if typedPDF.PageCount() != htmlPDF.PageCount() {
		t.Fatalf("typed pages = %d, HTML pages = %d", typedPDF.PageCount(), htmlPDF.PageCount())
	}
}

func TestTypedSignatureSuppliesDefaultSigningFieldName(t *testing.T) {
	pdf := MustNew()
	doc := layout.NewLayoutDocument()
	doc.Signature = &layout.SignatureBlock{PlaceholderReference: "ApprovalSignature"}
	pdf.WriteDocument(doc)
	if got := pdf.signingOptions(sign.Options{}).FieldName; got != "ApprovalSignature" {
		t.Fatalf("signing field name = %q, want ApprovalSignature", got)
	}
	explicit := pdf.signingOptions(sign.Options{FieldName: "ExplicitSignature"})
	if explicit.FieldName != "ExplicitSignature" {
		t.Fatalf("explicit signing field name = %q", explicit.FieldName)
	}
}

func TestTypedBoxBackgroundUsesMeasuredContentHeight(t *testing.T) {
	pdf := MustNew()
	pdf.SetCompression(false)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "background"}},
		Style:    layout.TextStyle{LineHeight: 8},
		Box: layout.BoxStyle{
			Padding:         layout.Spacing{Top: 2, Right: 2, Bottom: 2, Left: 2},
			BackgroundColor: layout.DocumentColor{R: 240, G: 240, B: 240, Set: true},
		},
	}}
	pdf.WriteDocument(doc)
	content := pdf.pages[pdf.page].String()
	if !strings.Contains(content, "-34.02 re f") {
		t.Fatalf("background rectangle did not use measured block height:\n%s", content)
	}
}
