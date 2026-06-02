/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import "testing"

func TestMeasureParagraphBlockUsesTextWrapping(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	ctx := NewMeasureContext(pdf, 25)

	short := MeasureBlock(ctx, ParagraphBlock{Segments: []TextSegment{{Text: "short"}}})
	long := MeasureBlock(ctx, ParagraphBlock{Segments: []TextSegment{{Text: "this paragraph should wrap onto several lines in a narrow column"}}})

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

func TestMeasureTextRestoresPDFFontState(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Courier", "I", 10)
	ctx := NewMeasureContext(pdf, 25)

	_ = MeasureBlock(ctx, ParagraphBlock{
		Segments: []TextSegment{{Text: "styled"}},
		Style:    TextStyle{FontFamily: "Helvetica", FontSize: 14, Bold: true},
	})

	if pdf.fontFamily != "courier" || pdf.fontStyle != "I" || pdf.fontSizePt != 10 {
		t.Fatalf("font state = %s/%s/%.1f, want courier/I/10", pdf.fontFamily, pdf.fontStyle, pdf.fontSizePt)
	}
}

func TestMeasureHeadingKeepsWithNext(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	ctx := NewMeasureContext(pdf, 80)

	measure := MeasureBlock(ctx, HeadingBlock{Level: 2, Segments: []TextSegment{{Text: "Heading"}}})
	if measure.Splittable {
		t.Fatal("heading should not be splittable")
	}
	if !measure.KeepTogether || !measure.KeepWithNext {
		t.Fatalf("heading keep flags = together:%v next:%v, want both true", measure.KeepTogether, measure.KeepWithNext)
	}
}

func TestMeasureAppliesParagraphAndHeadingSpacing(t *testing.T) {
	ctx := NewMeasureContext(nil, 80)

	paragraph := MeasureBlock(ctx, ParagraphBlock{Segments: []TextSegment{{Text: "Paragraph"}}})
	heading := MeasureBlock(ctx, HeadingBlock{Level: 2, Segments: []TextSegment{{Text: "Heading"}}})

	if paragraph.Height <= paragraph.MinHeight {
		t.Fatalf("paragraph height = %.2f min = %.2f, want spacing included", paragraph.Height, paragraph.MinHeight)
	}
	if heading.Height <= heading.MinHeight {
		t.Fatalf("heading height = %.2f min = %.2f, want spacing included", heading.Height, heading.MinHeight)
	}
}

func TestMeasurePageBreakBlock(t *testing.T) {
	measure := MeasureBlock(NewMeasureContext(nil, 80), PageBreakBlock{Before: true})
	if !measure.BreakBefore {
		t.Fatal("page break should report BreakBefore")
	}
	if !measure.ShouldMoveToNextPage(100) {
		t.Fatal("page break should move to next page")
	}
}

func TestMeasureTableRows(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	ctx := NewMeasureContext(pdf, 80)

	table := TableBlock{
		Header: []TableRow{{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Header"}}}}}}}},
		Body: []TableRow{
			{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "A longer cell value that wraps"}}}}}}},
			{Cells: []TableCell{{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Second row"}}}}}}},
		},
	}
	measure := MeasureBlock(ctx, table)
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
	ctx := NewMeasureContext(nil, 80)
	section := SectionBlock{
		Title: "Section",
		Blocks: []Block{
			ParagraphBlock{Segments: []TextSegment{{Text: "First"}}},
			ParagraphBlock{Segments: []TextSegment{{Text: "Second"}}},
		},
		Box: BoxStyle{Padding: Spacing{Top: 2, Bottom: 2}},
	}

	measure := MeasureBlock(ctx, section)
	if len(measure.ChildMeasures) != 2 {
		t.Fatalf("child measures = %d, want 2", len(measure.ChildMeasures))
	}
	if measure.Height <= measure.MinHeight {
		t.Fatalf("section height = %.2f min = %.2f, want height > min", measure.Height, measure.MinHeight)
	}
}

func TestMeasureSignatureRowIsKeptTogether(t *testing.T) {
	ctx := NewMeasureContext(nil, 100)
	measure := MeasureBlock(ctx, SignatureRowBlock{
		Columns: []SignatureColumn{
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
	ctx := NewMeasureContext(nil, 100)
	measure := MeasureBlock(ctx, QRVerificationBlock{
		QR:   QRBlock{Value: "https://example.test/verify", Size: 30},
		Text: []TextSegment{{Text: "Verification"}},
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
