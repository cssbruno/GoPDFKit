// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
)

func TestMeasureParagraphBlockUsesTextWrapping(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	ctx := newMeasureContext(pdf, 25)

	short := layout.MeasureBlock(ctx, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "short"}}})
	long := layout.MeasureBlock(ctx, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "this paragraph should wrap onto several lines in a narrow column"}}})

	if long.Height <= short.Height {
		t.Fatalf("long paragraph height = %.2f, short = %.2f; want long > short", long.Height, short.Height)
	}
	if !long.Splittable {
		t.Fatal("paragraph should be splittable by default")
	}
	if !long.CanStart(long.MinHeight) {
		t.Fatal("paragraph should be able to start with its minimum height")
	}
}

func TestMeasureParagraphScalesLineHeightWithFontSize(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	ctx := newMeasureContext(pdf, 120)

	regular := layout.MeasureBlock(ctx, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "same text"}}})
	large := layout.MeasureBlock(ctx, layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "same text"}},
		Style:    layout.TextStyle{FontSize: 24},
	})

	if large.MinHeight <= regular.MinHeight*1.5 {
		t.Fatalf("large min height = %.2f, regular = %.2f; want scaled line height", large.MinHeight, regular.MinHeight)
	}
}

func TestMeasureHeadingUsesDocumentHeadingSize(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	ctx := newMeasureContext(pdf, 120)

	h1 := layout.MeasureBlock(ctx, layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Title"}}})
	h4 := layout.MeasureBlock(ctx, layout.HeadingBlock{Level: 4, Segments: []layout.TextSegment{{Text: "Title"}}})

	if h1.MinHeight <= h4.MinHeight {
		t.Fatalf("h1 min height = %.2f, h4 = %.2f; want h1 > h4", h1.MinHeight, h4.MinHeight)
	}
}

func TestDocumentRendererMeasuresTableRowsWithRenderedWidths(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Courier", "", 12)
	pdf.AddPage()
	renderer := documentRenderer{pdf: pdf}
	row := layout.TableRow{Cells: []layout.TableCell{
		{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "MMMMMMMMMMMMMMMMMMMM"}}}}},
	}}

	narrow := renderer.measureRenderedTableRow(row, []float64{18})
	wide := renderer.measureRenderedTableRow(row, []float64{120})

	if narrow <= wide {
		t.Fatalf("narrow row height = %.2f, wide = %.2f; want narrow > wide", narrow, wide)
	}
}

func TestMeasureTextRestoresPDFFontState(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Courier", "I", 10)
	ctx := newMeasureContext(pdf, 25)

	_ = layout.MeasureBlock(ctx, layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "styled"}},
		Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 14, Bold: true},
	})

	if pdf.fontFamily != "courier" || pdf.fontStyle != "I" || pdf.fontSizePt != 10 {
		t.Fatalf("font state = %s/%s/%.1f, want courier/I/10", pdf.fontFamily, pdf.fontStyle, pdf.fontSizePt)
	}
}

func TestMeasureTextDoesNotWritePageContent(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Courier", "I", 10)
	pdf.AddPage()
	before := pdf.pages[pdf.page].String()
	ctx := newMeasureContext(pdf, 25)

	_ = layout.MeasureBlock(ctx, layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "styled"}},
		Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 14, Bold: true},
	})

	if after := pdf.pages[pdf.page].String(); after != before {
		t.Fatalf("measurement wrote page content:\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestMeasureHeadingKeepsWithNext(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	ctx := newMeasureContext(pdf, 80)

	measure := layout.MeasureBlock(ctx, layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: "Heading"}}})
	if measure.Splittable {
		t.Fatal("heading should not be splittable")
	}
	if !measure.KeepTogether || !measure.KeepWithNext {
		t.Fatalf("heading keep flags = together:%v next:%v, want both true", measure.KeepTogether, measure.KeepWithNext)
	}
}

func TestMeasureAppliesParagraphAndHeadingSpacing(t *testing.T) {
	ctx := newMeasureContext(nil, 80)

	paragraph := layout.MeasureBlock(ctx, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Paragraph"}}})
	heading := layout.MeasureBlock(ctx, layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: "Heading"}}})

	if paragraph.Height <= paragraph.MinHeight {
		t.Fatalf("paragraph height = %.2f min = %.2f, want spacing included", paragraph.Height, paragraph.MinHeight)
	}
	if heading.Height <= heading.MinHeight {
		t.Fatalf("heading height = %.2f min = %.2f, want spacing included", heading.Height, heading.MinHeight)
	}
}

func TestMeasurePageBreakBlock(t *testing.T) {
	measure := layout.MeasureBlock(newMeasureContext(nil, 80), layout.PageBreakBlock{Before: true})
	if !measure.BreakBefore {
		t.Fatal("page break should report BreakBefore")
	}
	if !measure.ShouldMoveToNextPage(100) {
		t.Fatal("page break should move to next page")
	}
}

func TestMeasureTableRows(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	ctx := newMeasureContext(pdf, 80)

	table := layout.TableBlock{
		Header: []layout.TableRow{{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Header"}}}}}}}},
		Body: []layout.TableRow{
			{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "A longer cell value that wraps"}}}}}}},
			{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Second row"}}}}}}},
		},
	}
	measure := layout.MeasureBlock(ctx, table)
	if len(measure.ChildMeasures) != 3 {
		t.Fatalf("row measures = %d, want 3", len(measure.ChildMeasures))
	}
	if !measure.Splittable {
		t.Fatal("table should be splittable when rows are not forced together")
	}
	if measure.MinHeight <= 0 || measure.Height <= measure.MinHeight {
		t.Fatalf("table height = %.2f min = %.2f, want height > min > 0", measure.Height, measure.MinHeight)
	}
}

func TestMeasureContainerIncludesChildren(t *testing.T) {
	ctx := newMeasureContext(nil, 80)
	section := layout.SectionBlock{
		Title: "Section",
		Blocks: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "First"}}},
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Second"}}},
		},
		Box: layout.BoxStyle{Padding: layout.Spacing{Top: 2, Bottom: 2}},
	}

	measure := layout.MeasureBlock(ctx, section)
	if len(measure.ChildMeasures) != 2 {
		t.Fatalf("child measures = %d, want 2", len(measure.ChildMeasures))
	}
	if measure.Height <= measure.MinHeight {
		t.Fatalf("section height = %.2f min = %.2f, want height > min", measure.Height, measure.MinHeight)
	}
}

func TestMeasureSignatureRowIsKeptTogether(t *testing.T) {
	ctx := newMeasureContext(nil, 100)
	measure := layout.MeasureBlock(ctx, layout.SignatureRowBlock{
		Columns: []layout.SignatureColumn{
			{Label: "Primary", Name: "A"},
			{Label: "Secondary", Name: "B"},
		},
	})

	if measure.Splittable {
		t.Fatal("signature row should not be splittable")
	}
	if !measure.KeepTogether {
		t.Fatal("signature row should be kept together")
	}
	if !measure.ShouldMoveToNextPage(measure.Height - 1) {
		t.Fatal("signature row should move when it does not fit")
	}
}

func TestMeasureQRVerificationBlockUsesQRSize(t *testing.T) {
	ctx := newMeasureContext(nil, 100)
	measure := layout.MeasureBlock(ctx, layout.QRVerificationBlock{
		QR:   layout.QRBlock{Value: "https://example.test/verify", Size: 30},
		Text: []layout.TextSegment{{Text: "Verification"}},
	})

	if measure.Height < 30 {
		t.Fatalf("QR verification height = %.2f, want at least QR size", measure.Height)
	}
	if measure.Splittable {
		t.Fatal("QR verification block should not be splittable")
	}
	if !measure.KeepTogether {
		t.Fatal("QR verification block should be kept together")
	}
}
