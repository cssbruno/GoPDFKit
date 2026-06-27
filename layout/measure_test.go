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

func TestMeasurePageBreakBlock(t *testing.T) {
	measure := MeasureBlock(NewMeasureContext(80, TextStyle{}), PageBreakBlock{Before: true, After: true})
	if !measure.BreakBefore {
		t.Fatal("expected BreakBefore")
	}
	if !measure.BreakAfter {
		t.Fatal("expected BreakAfter")
	}
}

type fixedLineMeasurer struct {
	lines int
}

func (m fixedLineMeasurer) TextLineCount(string, TextStyle, float64) int {
	return m.lines
}
