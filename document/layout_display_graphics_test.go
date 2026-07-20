// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

func displayGraphFixed(value int64) layoutengine.Fixed {
	return layoutengine.Fixed(value * layoutengine.FixedScale)
}

func TestPaintDisplayGraphicsProducesBalancedOrderedPDFOperators(t *testing.T) {
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{
		Number: 1, Size: layoutengine.Size{Width: displayGraphFixed(100), Height: displayGraphFixed(100)},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	path := layoutengine.PlannedPath{
		Bounds: layoutengine.Rect{X: displayGraphFixed(10), Y: displayGraphFixed(10), Width: displayGraphFixed(30), Height: displayGraphFixed(20)},
		Segments: []layoutengine.PathSegment{
			{Kind: layoutengine.PathMoveTo, Point: layoutengine.Point{X: displayGraphFixed(10), Y: displayGraphFixed(10)}},
			{Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: displayGraphFixed(40), Y: displayGraphFixed(10)}},
			{Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: displayGraphFixed(40), Y: displayGraphFixed(30)}},
			{Kind: layoutengine.PathLineTo, Point: layoutengine.Point{X: displayGraphFixed(10), Y: displayGraphFixed(30)}},
			{Kind: layoutengine.PathClose},
		},
	}
	plan, err := layoutengine.AttachDisplayList(geometry, layoutengine.DisplayListInput{
		Paths:      []layoutengine.PlannedPath{path},
		Transforms: []layoutengine.Transform{layoutengine.TranslationTransform(displayGraphFixed(5), displayGraphFixed(7))},
		Clips:      []layoutengine.PlannedClip{{Path: 0, Rule: layoutengine.FillEvenOdd}},
		Fills:      []layoutengine.PlannedFill{{Path: 0, Rule: layoutengine.FillNonZero, Color: layoutengine.CoreRGBColor{R: 17, G: 34, B: 51, Set: true}}},
		Strokes:    []layoutengine.PlannedStroke{{Path: 0, Color: layoutengine.CoreRGBColor{R: 200, G: 100, B: 50, Set: true}, Width: displayGraphFixed(2)}},
		Items: []layoutengine.DisplayItem{
			{Kind: layoutengine.CommandSaveState, Page: 1}, {Kind: layoutengine.CommandTransform, Page: 1},
			{Kind: layoutengine.CommandClip, Page: 1}, {Kind: layoutengine.CommandFillPath, Page: 1},
			{Kind: layoutengine.CommandStrokePath, Page: 1}, {Kind: layoutengine.CommandRestoreState, Page: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	operators := [][]byte{[]byte("q\n"), []byte("cm\n"), []byte("W* n\n"), []byte(" rg "), []byte(" f\n"), []byte(" RG "), []byte(" w "), []byte(" S\n"), []byte("Q\n")}
	position := -1
	for _, operator := range operators {
		next := bytes.Index(content[position+1:], operator)
		if next < 0 {
			t.Fatalf("missing ordered operator %q:\n%s", operator, content)
		}
		position += next + 1
	}
	if !bytes.Contains(content, []byte("1.0000000000 0.0000000000 0.0000000000 1.0000000000 5.0000000000 -7.0000000000 cm")) {
		t.Fatalf("top-left transform was not converted exactly:\n%s", content)
	}
	if target.transformNest != 0 || target.clipNest != 0 {
		t.Fatalf("graphics nesting leaked: transform=%d clip=%d", target.transformNest, target.clipNest)
	}
	var pdf bytes.Buffer
	if err := target.Output(&pdf); err != nil || !bytes.HasPrefix(pdf.Bytes(), []byte("%PDF-")) {
		t.Fatalf("PDF output = %d bytes, %v", pdf.Len(), err)
	}
}
