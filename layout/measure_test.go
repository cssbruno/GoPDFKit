// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

import "testing"

func TestMeasureBlockUsesTextMeasurer(t *testing.T) {
	ctx := NewMeasureContext(20, TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 4})
	ctx.TextMeasurer = fixedLineMeasurer{lines: 3}

	measure := MeasureBlock(ctx, ParagraphBlock{Segments: []TextSegment{{Text: "wrapped text"}}})
	if measure.Height != 14 {
		t.Fatalf("Height = %v, want 14", measure.Height)
	}
	if measure.MinHeight != 4 {
		t.Fatalf("MinHeight = %v, want 4", measure.MinHeight)
	}
}

func TestMeasureBlockFallbackWrapsByFontAndCharacters(t *testing.T) {
	ctx := NewMeasureContext(8, TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 4})

	narrow := MeasureBlock(ctx, ParagraphBlock{Segments: []TextSegment{{Text: "iiiiii"}}})
	wide := MeasureBlock(ctx, ParagraphBlock{Segments: []TextSegment{{Text: "MMMMMM"}}})

	if wide.Height <= narrow.Height {
		t.Fatalf("wide text height = %.2f, narrow = %.2f; want wide > narrow", wide.Height, narrow.Height)
	}
}

func TestMergedTextStyleScalesLineHeightWithFontSize(t *testing.T) {
	style := MergedTextStyle(
		TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 4},
		TextStyle{FontSize: 20},
	)

	if style.LineHeight != 8 {
		t.Fatalf("LineHeight = %.2f, want 8", style.LineHeight)
	}
}

func TestMeasureHeadingUsesLevelFontSize(t *testing.T) {
	ctx := NewMeasureContext(80, TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 4})

	h1 := MeasureBlock(ctx, HeadingBlock{Level: 1, Segments: []TextSegment{{Text: "Title"}}})
	h4 := MeasureBlock(ctx, HeadingBlock{Level: 4, Segments: []TextSegment{{Text: "Title"}}})

	if h1.Height <= h4.Height {
		t.Fatalf("h1 height = %.2f, h4 = %.2f; want h1 > h4", h1.Height, h4.Height)
	}
}

func TestMeasureTableRowUsesColumnWidths(t *testing.T) {
	ctx := NewMeasureContext(100, TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 4})
	table := TableBlock{
		Columns: []TableColumn{{Width: 10}, {Width: 90}},
		Body: []TableRow{{Cells: []TableCell{
			{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "MMMMMMMMMMMM"}}}}},
			{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "ok"}}}}},
		}}},
	}

	measure := MeasureBlock(ctx, table)
	if len(measure.ChildMeasures) != 1 {
		t.Fatalf("row measures = %d, want 1", len(measure.ChildMeasures))
	}
	if measure.ChildMeasures[0].Height <= ResolvedLineHeight(ctx.DefaultStyle)+paragraphSpacing {
		t.Fatalf("row height = %.2f, want wrapped narrow-column height", measure.ChildMeasures[0].Height)
	}
}

func TestMeasurePageBreakBlock(t *testing.T) {
	measure := MeasureBlock(NewMeasureContext(80, TextStyle{}), PageBreakBlock{Before: true, After: true})
	if !measure.BreakBefore {
		t.Fatal("expected BreakBefore")
	}
	if !measure.BreakAfter {
		t.Fatal("expected BreakAfter")
	}
}

func TestRequiredStartHeightIncludesKeptNextBlock(t *testing.T) {
	current := BlockMeasurement{Height: 12, MinHeight: 8, KeepWithNext: true}
	next := BlockMeasurement{Height: 20, MinHeight: 5}
	if got := current.RequiredStartHeight(&next); got != 17 {
		t.Fatalf("RequiredStartHeight(next) = %.2f, want 17", got)
	}
	next.KeepTogether = true
	if got := current.RequiredStartHeight(&next); got != 32 {
		t.Fatalf("RequiredStartHeight(kept next) = %.2f, want 32", got)
	}
}

func TestMeasureClauseIncludesRenderedTitle(t *testing.T) {
	ctx := NewMeasureContext(80, TextStyle{})
	body := []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Clause body"}}}}
	untitled := MeasureBlock(ctx, ClauseBlock{Blocks: body})
	titled := MeasureBlock(ctx, ClauseBlock{Number: "1.", Title: "Scope", Blocks: body})
	if titled.Height <= untitled.Height {
		t.Fatalf("titled clause height = %g, want more than untitled height %g", titled.Height, untitled.Height)
	}
	if titled.MinHeight <= untitled.MinHeight {
		t.Fatalf("titled clause minimum = %g, want title included above %g", titled.MinHeight, untitled.MinHeight)
	}
	kept := MeasureBlock(ctx, ClauseBlock{Number: "1.", Title: "Scope", Blocks: body, KeepTogether: true})
	if kept.MinHeight != kept.Height {
		t.Fatalf("kept clause minimum = %g, want full height %g", kept.MinHeight, kept.Height)
	}
}

func TestMeasureNoteBoxUsesEffectiveBoxAndIncludesTitle(t *testing.T) {
	ctx := NewMeasureContext(80, TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 4})
	box := BoxStyle{Padding: Spacing{Top: 3, Right: 4, Bottom: 5, Left: 4}}
	body := []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Body"}}}}
	withReference := MeasureBlock(ctx, NoteBoxBlock{
		Title:  "Notice",
		Body:   body,
		Box:    BoxStyle{Padding: Spacing{Top: 99}},
		BoxRef: &box,
	})
	withValue := MeasureBlock(ctx, NoteBoxBlock{Title: "Notice", Body: body, Box: box})
	withoutTitle := MeasureBlock(ctx, NoteBoxBlock{Body: body, Box: box})

	if withReference.Height != withValue.Height || withReference.MinHeight != withValue.MinHeight {
		t.Fatalf("BoxRef measurement = %#v, value measurement = %#v", withReference, withValue)
	}
	if withReference.Height <= withoutTitle.Height {
		t.Fatalf("title height = %.2f, untitled = %.2f; title was not measured", withReference.Height, withoutTitle.Height)
	}
	if len(withReference.ChildMeasures) != 2 {
		t.Fatalf("note child measurements = %d, want title and body", len(withReference.ChildMeasures))
	}
}

func TestMeasureSectionKeepTitleWithBodyChangesStartRequirement(t *testing.T) {
	ctx := NewMeasureContext(80, TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 4})
	box := BoxStyle{Padding: Spacing{Top: 3, Bottom: 5}}
	base := SectionBlock{
		Title:  "Section",
		Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: "First body"}}}},
		BoxRef: &box,
	}
	split := MeasureBlock(ctx, base)
	base.KeepTitleWithBody = true
	kept := MeasureBlock(ctx, base)

	if kept.Height != split.Height {
		t.Fatalf("kept section height = %.2f, split = %.2f; hint must not change geometry", kept.Height, split.Height)
	}
	if kept.MinHeight <= split.MinHeight {
		t.Fatalf("kept section min height = %.2f, split = %.2f; want title/body start requirement", kept.MinHeight, split.MinHeight)
	}
}

type fixedLineMeasurer struct {
	lines int
}

func (m fixedLineMeasurer) TextLineCount(string, TextStyle, float64) int {
	return m.lines
}
