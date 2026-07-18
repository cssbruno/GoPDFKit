// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestBreakShapedTextPrefersSpacesAndPreservesStableRanges(t *testing.T) {
	shaped := shapedLineFixture("AA BB", DirectionLTR, []ShapedGlyph{
		shapedLineGlyph('A', 5, 0, 1), shapedLineGlyph('A', 5, 1, 2), shapedLineGlyph(' ', 5, 2, 3),
		shapedLineGlyph('B', 5, 3, 4), shapedLineGlyph('B', 5, 4, 5),
	})
	first, err := BreakShapedText(context.Background(), shaped, 15, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("BreakShapedText() = %v", err)
	}
	second, err := BreakShapedText(context.Background(), shaped, 15, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("second BreakShapedText() = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("line layouts differ:\n%+v\n%+v", first, second)
	}
	if len(first.Lines) != 2 || first.Lines[0].TextStart != 0 || first.Lines[0].TextEnd != 3 || first.Lines[0].Width != 15 || first.Lines[0].Break != ShapedLineWrap ||
		first.Lines[1].TextStart != 3 || first.Lines[1].TextEnd != 5 || first.Lines[1].Width != 10 || first.Lines[1].Break != ShapedLineEnd {
		t.Fatalf("lines = %+v", first.Lines)
	}
	encoded, err := first.CanonicalJSON()
	if err != nil || len(encoded) == 0 {
		t.Fatalf("CanonicalJSON() = %s, %v", encoded, err)
	}
	shaped.Glyphs[0].ID = 999
	if first.Lines[0].Glyphs[0].ID != 'A' {
		t.Fatal("line layout aliases shaped input glyphs")
	}
}

func TestBreakShapedTextHandlesExplicitAndEmptyNewlineLines(t *testing.T) {
	shaped := shapedLineFixture("A\n\nB", DirectionLTR, []ShapedGlyph{
		shapedLineGlyph('A', 4, 0, 1), shapedLineGlyph('B', 4, 3, 4),
	})
	layout, err := BreakShapedText(context.Background(), shaped, 20, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("BreakShapedText() = %v", err)
	}
	if len(layout.Lines) != 3 ||
		layout.Lines[0].TextStart != 0 || layout.Lines[0].TextEnd != 1 || layout.Lines[0].Break != ShapedLineNewline ||
		layout.Lines[1].TextStart != 2 || layout.Lines[1].TextEnd != 2 || layout.Lines[1].Break != ShapedLineNewline || len(layout.Lines[1].Glyphs) != 0 ||
		layout.Lines[2].TextStart != 3 || layout.Lines[2].TextEnd != 4 || layout.Lines[2].Break != ShapedLineEnd {
		t.Fatalf("newline lines = %+v", layout.Lines)
	}
}

func TestBreakShapedTextNeverSplitsCombiningCluster(t *testing.T) {
	shaped := shapedLineFixture("e\u0301 x", DirectionLTR, []ShapedGlyph{
		shapedLineGlyph(101, 8, 0, 3), shapedLineGlyph(' ', 4, 3, 4), shapedLineGlyph('x', 8, 4, 5),
	})
	layout, err := BreakShapedText(context.Background(), shaped, 12, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("BreakShapedText() = %v", err)
	}
	if len(layout.Lines) != 2 || layout.Lines[0].TextEnd != 4 || layout.Lines[0].Glyphs[0].Cluster != (UTF8Cluster{Start: 0, End: 3}) ||
		layout.Lines[1].TextStart != 4 {
		t.Fatalf("combining-cluster lines = %+v", layout.Lines)
	}
}

func TestBreakShapedTextPreservesRTLVisualGlyphOrderPerLine(t *testing.T) {
	shaped := shapedLineFixture("אב גד", DirectionRTL, []ShapedGlyph{
		shapedLineGlyph(4, 5, 7, 9), shapedLineGlyph(3, 5, 5, 7), shapedLineGlyph(' ', 5, 4, 5),
		shapedLineGlyph(2, 5, 2, 4), shapedLineGlyph(1, 5, 0, 2),
	})
	shaped.Language = "he"
	layout, err := BreakShapedText(context.Background(), shaped, 15, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("BreakShapedText() = %v", err)
	}
	if len(layout.Lines) != 2 || layout.Lines[0].TextEnd != 5 || layout.Lines[1].TextStart != 5 {
		t.Fatalf("RTL ranges = %+v", layout.Lines)
	}
	if got := []uint32{layout.Lines[0].Glyphs[0].ID, layout.Lines[0].Glyphs[1].ID, layout.Lines[0].Glyphs[2].ID}; !reflect.DeepEqual(got, []uint32{' ', 2, 1}) {
		t.Fatalf("first RTL visual glyph order = %v", got)
	}
	if got := []uint32{layout.Lines[1].Glyphs[0].ID, layout.Lines[1].Glyphs[1].ID}; !reflect.DeepEqual(got, []uint32{4, 3}) {
		t.Fatalf("second RTL visual glyph order = %v", got)
	}
}

func TestBreakShapedTextInternationalAdversarialMatrix(t *testing.T) {
	t.Run("CJK clusters", func(t *testing.T) {
		shaped := shapedLineFixture("日本語", DirectionLTR, []ShapedGlyph{
			shapedLineGlyph(1, 6, 0, 3),
			shapedLineGlyph(2, 6, 3, 6),
			shapedLineGlyph(3, 6, 6, 9),
		})
		shaped.Language = "ja"
		layout, err := BreakShapedText(context.Background(), shaped, 12, ShapedLineLimits{})
		if err != nil {
			t.Fatalf("BreakShapedText() = %v", err)
		}
		if len(layout.Lines) != 2 || layout.Lines[0].TextEnd != 6 || layout.Lines[1].TextStart != 6 ||
			layout.Lines[0].Width != 12 || layout.Lines[1].Width != 6 {
			t.Fatalf("CJK lines = %+v", layout.Lines)
		}
	})

	t.Run("mixed bidi visual order", func(t *testing.T) {
		shaped := shapedLineFixture("A אב", DirectionLTR, []ShapedGlyph{
			shapedLineGlyph('A', 4, 0, 1),
			shapedLineGlyph(' ', 2, 1, 2),
			shapedLineGlyph(2, 4, 4, 6),
			shapedLineGlyph(1, 4, 2, 4),
		})
		shaped.Language = "he"
		layout, err := BreakShapedText(context.Background(), shaped, 8, ShapedLineLimits{})
		if err != nil {
			t.Fatalf("BreakShapedText() = %v", err)
		}
		if len(layout.Lines) != 2 || layout.Lines[0].TextEnd != 2 || layout.Lines[1].TextStart != 2 {
			t.Fatalf("mixed-bidi ranges = %+v", layout.Lines)
		}
		if got := []uint32{layout.Lines[1].Glyphs[0].ID, layout.Lines[1].Glyphs[1].ID}; !reflect.DeepEqual(got, []uint32{2, 1}) {
			t.Fatalf("mixed-bidi visual glyph order = %v", got)
		}
	})

	t.Run("emoji ZWJ cluster", func(t *testing.T) {
		const emoji = "👩‍⚕️"
		text := emoji + " X"
		emojiEnd := uint32(len(emoji))
		shaped := shapedLineFixture(text, DirectionLTR, []ShapedGlyph{
			shapedLineGlyph(10, 8, 0, emojiEnd),
			shapedLineGlyph(' ', 2, emojiEnd, emojiEnd+1),
			shapedLineGlyph('X', 4, emojiEnd+1, emojiEnd+2),
		})
		layout, err := BreakShapedText(context.Background(), shaped, 10, ShapedLineLimits{})
		if err != nil {
			t.Fatalf("BreakShapedText() = %v", err)
		}
		if len(layout.Lines) != 2 || layout.Lines[0].TextEnd != emojiEnd+1 ||
			layout.Lines[0].Glyphs[0].Cluster != (UTF8Cluster{Start: 0, End: emojiEnd}) || layout.Lines[1].TextStart != emojiEnd+1 {
			t.Fatalf("emoji lines = %+v", layout.Lines)
		}
	})
}

type shapedLineRecordingSink struct {
	lines  []ShapedLine
	glyphs [][]ShapedGlyph
}

func (sink *shapedLineRecordingSink) BeginShapedLine(line ShapedLine) error {
	sink.lines = append(sink.lines, line)
	return nil
}

func (sink *shapedLineRecordingSink) PaintShapedFontRun(_ ShapedFontRun, glyphs []ShapedGlyph) error {
	sink.glyphs = append(sink.glyphs, append([]ShapedGlyph(nil), glyphs...))
	if len(glyphs) != 0 {
		glyphs[0].ID = 999
	}
	return nil
}

func (*shapedLineRecordingSink) EndShapedLine(ShapedLine) error { return nil }

func TestReplayShapedLineLayoutUsesStoredGlyphsWithoutReshaping(t *testing.T) {
	shaped := shapedLineFixture("AB", DirectionLTR, []ShapedGlyph{shapedLineGlyph('A', 5, 0, 1), shapedLineGlyph('B', 5, 1, 2)})
	layout, err := BreakShapedText(context.Background(), shaped, 5, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("BreakShapedText() = %v", err)
	}
	sink := &shapedLineRecordingSink{}
	if err := ReplayShapedLineLayout(layout, sink); err != nil {
		t.Fatalf("ReplayShapedLineLayout() = %v", err)
	}
	if len(sink.lines) != 2 || len(sink.glyphs) != 2 || sink.glyphs[0][0].ID != 'A' || sink.glyphs[1][0].ID != 'B' ||
		layout.Lines[0].Glyphs[0].ID != 'A' {
		t.Fatalf("replay = lines %+v glyphs %+v layout %+v", sink.lines, sink.glyphs, layout.Lines)
	}
}

func TestBreakShapedTextRejectsOverwideClusterAndEnforcesLimits(t *testing.T) {
	shaped := shapedLineFixture("e\u0301", DirectionLTR, []ShapedGlyph{shapedLineGlyph(101, 20, 0, 3)})
	if _, err := BreakShapedText(context.Background(), shaped, 19, ShapedLineLimits{}); !errors.Is(err, ErrShapedLineUnbreakable) {
		t.Fatalf("overwide cluster = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BreakShapedText(canceled, shaped, 20, ShapedLineLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled break = %v", err)
	}
	limits := DefaultShapedLineLimits()
	limits.MaxTextBytes = 2
	if _, err := BreakShapedText(context.Background(), shaped, 20, limits); !errors.Is(err, ErrShapedLineLimit) {
		t.Fatalf("byte limit = %v", err)
	}
	limits = DefaultShapedLineLimits()
	limits.MaxLines = hardMaxShapedLines + 1
	if _, err := BreakShapedText(context.Background(), shaped, 20, limits); err == nil {
		t.Fatal("hard line cap unexpectedly accepted")
	}
	limits = DefaultShapedLineLimits()
	limits.MaxWork = 1
	if _, err := BreakShapedText(context.Background(), shaped, 20, limits); !errors.Is(err, ErrShapedLineLimit) {
		t.Fatalf("work limit = %v", err)
	}
}

func TestShapedLineLayoutValidationRejectsMissingClusterCoverage(t *testing.T) {
	shaped := shapedLineFixture("AB", DirectionLTR, []ShapedGlyph{shapedLineGlyph('A', 5, 0, 1), shapedLineGlyph('B', 5, 1, 2)})
	layout, err := BreakShapedText(context.Background(), shaped, 10, ShapedLineLimits{})
	if err != nil {
		t.Fatalf("BreakShapedText() = %v", err)
	}
	layout.Lines[0].Glyphs = layout.Lines[0].Glyphs[:1]
	layout.Lines[0].Width = 5
	if err := layout.Validate(); !errors.Is(err, ErrShapedLineInvalid) {
		t.Fatalf("Validate() = %v", err)
	}
}

func shapedLineFixture(text string, direction TextDirection, glyphs []ShapedGlyph) ShapedText {
	font := ShapeFont{Name: "test-font", Digest: digestOf("3")}
	return ShapedText{
		Text: text, Language: "en", Direction: direction, Glyphs: glyphs,
		FontRuns: []ShapedFontRun{{Font: font, GlyphCount: uint32(len(glyphs)), TextEnd: uint32(len(text))}},
	}
}

func shapedLineGlyph(id uint32, advance Fixed, start, end uint32) ShapedGlyph {
	return ShapedGlyph{ID: id, Advance: advance, Cluster: UTF8Cluster{Start: start, End: end}}
}
