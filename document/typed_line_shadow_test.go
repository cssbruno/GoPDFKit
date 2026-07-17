// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestTypedParagraphLineShadowSoftWrapMatchesLegacyPageAndLineAllocation(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 60, Ht: 50}))
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	pdf.SetFont("Courier", "", 10)
	text := "AA BB CC DD EE FF GG HH II JJ"
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: text}},
		Style:    layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 10},
	}}

	shadow, err := pdf.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("planTypedParagraphLineShadow() = %v", err)
	}
	projection := shadow.Plan.Projection()
	if len(shadow.Lines) < 4 || len(projection.Pages) < 2 || shadow.Lines[0].Break != wrappedBreakSoftSpace {
		t.Fatalf("soft-wrap shadow = %d lines, %d pages, first break %q", len(shadow.Lines), len(projection.Pages), shadow.Lines[0].Break)
	}
	for _, line := range shadow.Lines {
		if line.Break == wrappedBreakSoftSpace && line.NextByte != line.EndByte+1 {
			t.Fatalf("soft line does not consume exactly one space: %#v", line)
		}
	}

	pdf.writeDocumentLegacy(doc)
	if err := pdf.Error(); err != nil || pdf.PageCount() != len(projection.Pages) {
		t.Fatalf("legacy output = %d pages, %v; shadow pages %d", pdf.PageCount(), err, len(projection.Pages))
	}
	for pageIndex, page := range projection.Pages {
		content := pdf.pages[pageIndex+1].Bytes()
		if got, want := bytes.Count(content, []byte(")Tj ET")), int(page.Lines.Count); got != want {
			t.Fatalf("legacy page %d painted %d text lines, want %d", pageIndex+1, got, want)
		}
		for lineIndex := page.Lines.Start; lineIndex < page.Lines.Start+page.Lines.Count; lineIndex++ {
			wrapped := shadow.Lines[lineIndex]
			visible := []byte(shadow.Text[wrapped.StartByte:wrapped.EndByte])
			if len(visible) > 0 && !bytes.Contains(content, visible) {
				t.Fatalf("legacy page %d does not contain planned line %q", pageIndex+1, visible)
			}
		}
	}
}

func TestTypedParagraphLineShadowMatchesLegacyPageAllocationWithoutMutation(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 60}))
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	pdf.SetFont("Courier", "", 10)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "A\nB\nC\nD\nE"}},
		Style:    layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 10},
	}}

	before := typedShadowSnapshotOf(pdf)
	shadow, err := pdf.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("planTypedParagraphLineShadow() = %v", err)
	}
	if after := typedShadowSnapshotOf(pdf); after != before {
		t.Fatalf("line shadow mutated live document:\nbefore %#v\nafter  %#v", before, after)
	}
	projection := shadow.Plan.Projection()
	if got, want := len(projection.Pages), 2; got != want {
		t.Fatalf("shadow pages = %d, want %d", got, want)
	}
	if got, want := []uint32{projection.Pages[0].Lines.Count, projection.Pages[1].Lines.Count}, []uint32{4, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("page line counts = %#v, want %#v", got, want)
	}
	if len(projection.Lines) != 5 || len(shadow.Lines) != 5 {
		t.Fatalf("planned/wrapped lines = %d/%d, want 5", len(projection.Lines), len(shadow.Lines))
	}
	for index, line := range shadow.Lines {
		if got, want := shadow.Text[line.StartByte:line.EndByte], string(rune('A'+index)); got != want {
			t.Fatalf("wrapped line %d = %q, want %q", index, got, want)
		}
		if index < 4 && line.NextByte != line.EndByte+1 {
			t.Fatalf("wrapped line %d does not own its LF: %#v", index, line)
		}
	}
	fixedMargin, _ := layoutengine.FixedFromPoints(10)
	fixedCellMargin, _ := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(pdf.cMargin))
	fixedBaseline, _ := layoutengine.FixedFromPoints(8)
	if projection.Lines[0].Bounds.X != fixedMargin+fixedCellMargin ||
		projection.Lines[0].Baseline != fixedMargin+fixedBaseline {
		t.Fatalf("first line geometry = %#v, want cell-margin x and compatibility baseline", projection.Lines[0])
	}

	pdf.writeDocumentLegacy(doc)
	if err := pdf.Error(); err != nil {
		t.Fatalf("WriteDocument() = %v", err)
	}
	if got, want := pdf.PageCount(), len(projection.Pages); got != want {
		t.Fatalf("legacy pages = %d, shadow pages = %d", got, want)
	}
}

func TestTypedParagraphLineShadowUsesExactCoreTrailingLFProfile(t *testing.T) {
	tests := []struct {
		text      string
		wantLines int
	}{
		{"", 1},
		{"\n", 1},
		{"A\n", 1},
		{"A\n\n", 2},
		{"\n\n", 2},
		{"A\n\nB", 3},
	}
	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}))
			pdf.SetMargins(10, 10, 10)
			pdf.SetAutoPageBreak(true, 10)
			doc := layout.NewLayoutDocument()
			doc.Body = []layout.Block{layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: test.text}},
				Style:    layout.TextStyle{LineHeight: 10},
			}}
			shadow, err := pdf.planTypedParagraphLineShadow(doc)
			if err != nil {
				t.Fatalf("planTypedParagraphLineShadow() = %v", err)
			}
			if got := len(shadow.Plan.Projection().Lines); got != test.wantLines {
				t.Fatalf("planned lines = %d, want %d", got, test.wantLines)
			}
			pdf.writeDocumentLegacy(doc)
			if err := pdf.Error(); err != nil || pdf.PageCount() != 1 {
				t.Fatalf("legacy output = pages %d, error %v", pdf.PageCount(), err)
			}
		})
	}
}

func TestTypedParagraphLineShadowConvertsLineGeometryAcrossUnits(t *testing.T) {
	for _, unit := range []Unit{UnitPoint, UnitMillimeter, UnitCentimeter, UnitInch} {
		t.Run(unit.String(), func(t *testing.T) {
			pdf := MustNew(WithUnit(unit))
			margin := pdf.PointConvert(36)
			pdf.SetMargins(margin, margin, margin)
			pdf.SetAutoPageBreak(true, margin)
			doc := layout.NewLayoutDocument()
			doc.Body = []layout.Block{layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: "AA"}},
				Style: layout.TextStyle{
					FontFamily: "Courier", FontSize: 10, LineHeight: pdf.PointConvert(12), Align: "R",
				},
			}}
			shadow, err := pdf.planTypedParagraphLineShadow(doc)
			if err != nil {
				t.Fatalf("planTypedParagraphLineShadow() = %v", err)
			}
			projection := shadow.Plan.Projection()
			line := projection.Lines[0]
			wantHeight, _ := layoutengine.FixedFromPoints(12)
			wantWidth, _ := layoutengine.FixedFromPoints(12)   // two Courier 600-unit glyphs at 10pt
			wantBaseline, _ := layoutengine.FixedFromPoints(9) // .5*12pt + .3*10pt
			bodyRight, _ := projection.Fragments[0].ContentBox.Right()
			cellMargin, _ := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(pdf.cMargin))
			if line.Bounds.Height != wantHeight || line.Bounds.Width != wantWidth ||
				line.Baseline-line.Bounds.Y != wantBaseline || line.Bounds.X+line.Bounds.Width != bodyRight-cellMargin {
				t.Fatalf("line geometry = %#v, body right %d margin %d", line, bodyRight, cellMargin)
			}
		})
	}
}

func TestTypedParagraphLineShadowRejectsUnsupportedContentWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Document, *layout.LayoutDocument)
		reason typedShadowUnsupportedReason
	}{
		{"two blocks", func(_ *Document, doc *layout.LayoutDocument) {
			doc.Body = append(doc.Body, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "two"}}})
		}, typedShadowDocumentEnvelope},
		{"keep together", func(_ *Document, doc *layout.LayoutDocument) {
			paragraph := doc.Body[0].(layout.ParagraphBlock)
			paragraph.Box.KeepTogether = true
			doc.Body[0] = paragraph
		}, typedShadowParagraphContract},
		{"non ASCII", func(_ *Document, doc *layout.LayoutDocument) {
			paragraph := doc.Body[0].(layout.ParagraphBlock)
			paragraph.Segments[0].Text = "café"
			doc.Body[0] = paragraph
		}, typedShadowParagraphContract},
		{"tab", func(_ *Document, doc *layout.LayoutDocument) {
			paragraph := doc.Body[0].(layout.ParagraphBlock)
			paragraph.Segments[0].Text = "A\tB"
			doc.Body[0] = paragraph
		}, typedShadowParagraphContract},
		{"invalid UTF-8", func(_ *Document, doc *layout.LayoutDocument) {
			paragraph := doc.Body[0].(layout.ParagraphBlock)
			paragraph.Segments[0].Text = string([]byte{'A', 0xff, 'B'})
			doc.Body[0] = paragraph
		}, typedShadowParagraphContract},
		{"custom lifecycle", func(pdf *Document, _ *layout.LayoutDocument) {
			pdf.SetAcceptPageBreakFunc(func() bool { return true })
		}, typedShadowDocumentPolicy},
		{"oversized line height", func(_ *Document, doc *layout.LayoutDocument) {
			paragraph := doc.Body[0].(layout.ParagraphBlock)
			paragraph.Style.LineHeight = 10000
			doc.Body[0] = paragraph
		}, typedShadowGeometry},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pdf := MustNew(WithUnit(UnitPoint))
			pdf.SetMargins(20, 20, 20)
			pdf.SetAutoPageBreak(true, 20)
			doc := layout.NewLayoutDocument()
			doc.Body = []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "plain"}}}}
			test.mutate(pdf, doc)
			before := typedShadowSnapshotOf(pdf)
			_, err := pdf.planTypedParagraphLineShadow(doc)
			if !errors.Is(err, errTypedShadowUnsupported) {
				t.Fatalf("line shadow error = %v, want errTypedShadowUnsupported", err)
			}
			var unsupported *typedShadowUnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Reason != test.reason {
				t.Fatalf("line shadow error = %#v, want reason %q", err, test.reason)
			}
			if after := typedShadowSnapshotOf(pdf); after != before {
				t.Fatalf("failed line shadow mutated document:\nbefore %#v\nafter  %#v", before, after)
			}
		})
	}
}

func TestTypedParagraphLineShadowRejectsFixedRoundingPageDrift(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 60}))
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "A\nB\nC\nD\nE"}},
		Style:    layout.TextStyle{LineHeight: 10.0001},
	}}
	before := typedShadowSnapshotOf(pdf)
	_, err := pdf.planTypedParagraphLineShadow(doc)
	if !errors.Is(err, errTypedShadowUnsupported) {
		t.Fatalf("rounding drift error = %v, want errTypedShadowUnsupported", err)
	}
	var unsupported *typedShadowUnsupportedError
	if !errors.As(err, &unsupported) || unsupported.Reason != typedShadowGeometry {
		t.Fatalf("rounding drift error = %#v, want geometry reason", err)
	}
	if after := typedShadowSnapshotOf(pdf); after != before {
		t.Fatalf("rounding rejection mutated document:\nbefore %#v\nafter  %#v", before, after)
	}
}
