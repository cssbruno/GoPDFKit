// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

func TestPaintAttachedBoxDecorationsAsExactPDFFills(t *testing.T) {
	unit := layoutengine.Fixed(layoutengine.FixedScale)
	box := layoutengine.Rect{X: 10 * unit, Y: 20 * unit, Width: 40 * unit, Height: 30 * unit}
	base, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: []layoutengine.PlannedPage{{Number: 1, Size: layoutengine.Size{Width: 100 * unit, Height: 100 * unit}, Fragments: layoutengine.IndexRange{Count: 1}}},
		Fragments: []layoutengine.Fragment{{ID: 1, Node: 1, Key: "@box", Instance: "@box", Page: 1, Region: layoutengine.RegionBody,
			BorderBox: box, ContentBox: box, Continuation: layoutengine.ContinuationWhole}},
	})
	if err != nil {
		t.Fatal(err)
	}
	red := layoutengine.CoreRGBColor{R: 255, Set: true}
	plan, err := layoutengine.AttachBoxDecorations(base, []layoutengine.BoxDecoration{{
		Fragment: 1, Background: layoutengine.CoreRGBColor{R: 10, G: 20, B: 30, Set: true},
		Top: layoutengine.BoxBorderSide{Width: unit, Color: red}, Right: layoutengine.BoxBorderSide{Width: 2 * unit, Color: red},
		Bottom: layoutengine.BoxBorderSide{Width: 3 * unit, Color: red}, Left: layoutengine.BoxBorderSide{Width: 4 * unit, Color: red},
	}})
	if err != nil {
		t.Fatal(err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	if got := bytes.Count(content, []byte(" rg ")); got != 5 {
		t.Fatalf("non-stroking color operators = %d, want 5:\n%s", got, content)
	}
	if bytes.Contains(content, []byte(" RG ")) || bytes.Contains(content, []byte(" S\n")) {
		t.Fatalf("box border escaped fill-only contract:\n%s", content)
	}
	if !bytes.Contains(content, []byte("10.0000000000 80.0000000000 m 50.0000000000 80.0000000000 l")) {
		t.Fatalf("background path was not converted exactly to PDF coordinates:\n%s", content)
	}
	var output bytes.Buffer
	if err := target.Output(&output); err != nil || !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatalf("PDF output = %d bytes, %v", output.Len(), err)
	}
}
