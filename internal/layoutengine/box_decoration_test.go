// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"
)

func TestAttachBoxDecorationsExactPlanOrderAndRaster(t *testing.T) {
	unit := Fixed(FixedScale)
	box := Rect{X: 2 * unit, Y: 2 * unit, Width: 16 * unit, Height: 16 * unit}
	geometry, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{Number: 1, Size: Size{Width: 20 * unit, Height: 20 * unit}, Fragments: IndexRange{Count: 1}}},
		Fragments: []Fragment{{ID: 1, Node: 1, Key: "@box", Instance: "@box", Page: 1, Region: RegionBody,
			BorderBox: box, ContentBox: box, Continuation: ContinuationWhole}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contentBox := Rect{X: 6 * unit, Y: 6 * unit, Width: 2 * unit, Height: 2 * unit}
	contentPath, err := boxDecorationRectPath(contentBox)
	if err != nil {
		t.Fatal(err)
	}
	base, err := AttachDisplayList(geometry, DisplayListInput{
		Paths: []PlannedPath{contentPath},
		Fills: []PlannedFill{{Path: 0, Rule: FillNonZero, Color: CoreRGBColor{Set: true}, Fragment: 1}},
		Items: []DisplayItem{{Kind: CommandFillPath, Payload: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	red := CoreRGBColor{R: 255, Set: true}
	green := CoreRGBColor{G: 255, Set: true}
	blue := CoreRGBColor{B: 255, Set: true}
	magenta := CoreRGBColor{R: 255, B: 255, Set: true}
	yellow := CoreRGBColor{R: 255, G: 255, Set: true}
	decorated, err := AttachBoxDecorations(base, []BoxDecoration{{
		Fragment: 1, Background: yellow,
		Top: BoxBorderSide{Width: 2 * unit, Color: red}, Right: BoxBorderSide{Width: 3 * unit, Color: green},
		Bottom: BoxBorderSide{Width: 4 * unit, Color: blue}, Left: BoxBorderSide{Width: unit, Color: magenta},
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := decorated.Projection()
	if len(projection.Paths) != 6 || len(projection.Fills) != 6 || len(projection.Commands) != 6 || projection.Pages[0].Commands.Count != 6 {
		t.Fatalf("decorated resources paths=%d fills=%d commands=%d page=%+v", len(projection.Paths), len(projection.Fills), len(projection.Commands), projection.Pages[0].Commands)
	}
	wantBounds := []Rect{
		box,
		{X: 2 * unit, Y: 2 * unit, Width: 16 * unit, Height: 2 * unit},
		{X: 15 * unit, Y: 2 * unit, Width: 3 * unit, Height: 16 * unit},
		{X: 2 * unit, Y: 14 * unit, Width: 16 * unit, Height: 4 * unit},
		{X: 2 * unit, Y: 2 * unit, Width: unit, Height: 16 * unit},
		contentBox,
	}
	for index, command := range projection.Commands {
		if command.Kind != CommandFillPath || command.Fragment != 1 || command.Bounds != wantBounds[index] {
			t.Fatalf("command[%d] = %+v, want bounds %+v", index, command, wantBounds[index])
		}
	}

	request := rasterRequest()
	request.Profile.DPI = 72
	artifact, err := CaptureDisplayPlanPNG(decorated, DisplayRasterSources{}, request)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.Decode(bytes.NewReader(artifact.PNG()))
	if err != nil {
		t.Fatal(err)
	}
	assertPixel := func(x, y int, want color.RGBA) {
		t.Helper()
		if got := color.RGBAModel.Convert(image.At(x, y)).(color.RGBA); got != want {
			t.Fatalf("pixel(%d,%d) = %+v, want %+v", x, y, got, want)
		}
	}
	assertPixel(10, 2, color.RGBA{255, 0, 0, 255})
	assertPixel(17, 3, color.RGBA{0, 255, 0, 255})
	assertPixel(10, 16, color.RGBA{0, 0, 255, 255})
	assertPixel(2, 10, color.RGBA{255, 0, 255, 255})
	assertPixel(10, 10, color.RGBA{255, 255, 0, 255})
	assertPixel(6, 6, color.RGBA{0, 0, 0, 255})
}

func TestAttachBoxDecorationsRejectsInvalidInputAtomically(t *testing.T) {
	unit := Fixed(FixedScale)
	box := Rect{Width: 10 * unit, Height: 10 * unit}
	base, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{Number: 1, Size: Size{Width: 20 * unit, Height: 20 * unit}, Fragments: IndexRange{Count: 1}}},
		Fragments: []Fragment{{ID: 1, Node: 1, Key: "@box", Instance: "@box", Page: 1, Region: RegionBody,
			BorderBox: box, ContentBox: box, Continuation: ContinuationWhole}},
	})
	if err != nil {
		t.Fatal(err)
	}
	visible := CoreRGBColor{R: 1, Set: true}
	tests := []struct {
		name        string
		decorations []BoxDecoration
	}{
		{"missing fragment", []BoxDecoration{{Fragment: 2, Background: visible}}},
		{"duplicate fragment", []BoxDecoration{{Fragment: 1, Background: visible}, {Fragment: 1, Background: visible}}},
		{"negative width", []BoxDecoration{{Fragment: 1, Top: BoxBorderSide{Width: -1, Color: visible}}}},
		{"oversized width", []BoxDecoration{{Fragment: 1, Right: BoxBorderSide{Width: 11 * unit, Color: visible}}}},
		{"color on zero width", []BoxDecoration{{Fragment: 1, Bottom: BoxBorderSide{Color: visible}}}},
		{"ignored background color", []BoxDecoration{{Fragment: 1, Background: CoreRGBColor{R: 1}}}},
		{"ignored border color", []BoxDecoration{{Fragment: 1, Bottom: BoxBorderSide{Color: CoreRGBColor{B: 1}}}}},
		{"missing visible color", []BoxDecoration{{Fragment: 1, Left: BoxBorderSide{Width: unit}}}},
		{"empty override", []BoxDecoration{{Fragment: 1, BorderBox: &Rect{}, Background: visible}}},
		{"override outside page", []BoxDecoration{{Fragment: 1, BorderBox: &Rect{X: 19 * unit, Width: 2 * unit, Height: unit}, Background: visible}}},
		{"negative radius", []BoxDecoration{{Fragment: 1, Radius: -1, Background: visible}}},
		{"shadow geometry without color", []BoxDecoration{{Fragment: 1, Shadow: BoxShadow{OffsetX: unit}}}},
		{"unequal rounded border", []BoxDecoration{{Fragment: 1, Radius: unit, Background: visible, Top: BoxBorderSide{Width: unit, Color: visible}}}},
		{"transparent rounded border interior", []BoxDecoration{{Fragment: 1, Radius: unit, Top: BoxBorderSide{Width: unit, Color: visible}, Right: BoxBorderSide{Width: unit, Color: visible}, Bottom: BoxBorderSide{Width: unit, Color: visible}, Left: BoxBorderSide{Width: unit, Color: visible}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AttachBoxDecorations(base, test.decorations)
			if err == nil || len(got.Projection().Pages) != 0 {
				t.Fatalf("result pages=%d err=%v", len(got.Projection().Pages), err)
			}
		})
	}
}

func TestAttachBoxDecorationsRoundedShadowUsesSharedCurvesAndDirectRaster(t *testing.T) {
	unit := Fixed(FixedScale)
	box := Rect{X: 4 * unit, Y: 4 * unit, Width: 12 * unit, Height: 10 * unit}
	base, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{Number: 1, Size: Size{Width: 24 * unit, Height: 24 * unit}, Fragments: IndexRange{Count: 1}}},
		Fragments: []Fragment{{ID: 1, Node: 1, Key: "@rounded", Instance: "@rounded", Page: 1, Region: RegionBody,
			BorderBox: box, ContentBox: box, Continuation: ContinuationWhole}},
	})
	if err != nil {
		t.Fatal(err)
	}
	border := CoreRGBColor{R: 20, G: 40, B: 60, Set: true}
	background := CoreRGBColor{R: 240, G: 245, B: 250, Set: true}
	shadow := CoreRGBColor{R: 80, G: 90, B: 100, Set: true}
	side := BoxBorderSide{Width: unit, Color: border}
	decorated, err := AttachBoxDecorations(base, []BoxDecoration{{
		Fragment: 1, Radius: 3 * unit, Background: background,
		Shadow: BoxShadow{OffsetX: 2 * unit, OffsetY: 3 * unit, Spread: unit, Color: shadow},
		Top:    side, Right: side, Bottom: side, Left: side,
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := decorated.Projection()
	if len(projection.Paths) != 3 || len(projection.Fills) != 3 || len(projection.Commands) != 3 {
		t.Fatalf("rounded resources paths=%d fills=%d commands=%d", len(projection.Paths), len(projection.Fills), len(projection.Commands))
	}
	wantShadow := Rect{X: 5 * unit, Y: 6 * unit, Width: 14 * unit, Height: 12 * unit}
	if projection.Commands[0].Bounds != wantShadow || projection.Commands[1].Bounds != box ||
		projection.Commands[2].Bounds != (Rect{X: 5 * unit, Y: 5 * unit, Width: 10 * unit, Height: 8 * unit}) {
		t.Fatalf("rounded command bounds=%+v", projection.Commands)
	}
	for index, path := range projection.Paths {
		hasCurve := false
		for _, segment := range path.Segments {
			hasCurve = hasCurve || segment.Kind == PathCubicTo
		}
		if !hasCurve || projection.Fills[index].Rule != FillNonZero {
			t.Fatalf("path[%d] curves=%v fill=%+v", index, hasCurve, projection.Fills[index])
		}
	}
	request := rasterRequest()
	request.Profile.DPI = 72
	artifact, err := CaptureDisplayPlanPNG(decorated, DisplayRasterSources{}, request)
	if err != nil || len(artifact.PNG()) == 0 {
		t.Fatalf("direct rounded raster bytes=%d err=%v", len(artifact.PNG()), err)
	}
	svg, err := CaptureDisplayPlanSVG(decorated, 1, nil)
	if err != nil || !bytes.Contains(svg.SVG, []byte(" C")) {
		t.Fatalf("rounded SVG bytes=%d err=%v: %s", len(svg.SVG), err, svg.SVG)
	}
}

func TestAttachBoxDecorationsUsesOwnedExplicitGroupBounds(t *testing.T) {
	unit := Fixed(FixedScale)
	fragmentBox := Rect{X: 6 * unit, Y: 6 * unit, Width: 4 * unit, Height: 3 * unit}
	base, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{Number: 1, Size: Size{Width: 20 * unit, Height: 20 * unit}, Fragments: IndexRange{Count: 1}}},
		Fragments: []Fragment{{ID: 1, Node: 1, Key: "@child", Instance: "@child", Page: 1, Region: RegionHeader,
			BorderBox: fragmentBox, ContentBox: fragmentBox, Continuation: ContinuationWhole}},
	})
	if err != nil {
		t.Fatal(err)
	}
	groupBox := Rect{X: 2 * unit, Y: 2 * unit, Width: 16 * unit, Height: 12 * unit}
	blue := CoreRGBColor{B: 255, Set: true}
	decorated, err := AttachBoxDecorations(base, []BoxDecoration{{
		Fragment: 1, BorderBox: &groupBox, Background: blue,
		Top: BoxBorderSide{Width: unit, Color: blue},
	}})
	if err != nil {
		t.Fatal(err)
	}
	groupBox.X = 9 * unit // Caller mutation cannot change the immutable plan.
	projection := decorated.Projection()
	if projection.Fragments[0].BorderBox != fragmentBox {
		t.Fatalf("child fragment geometry changed: %+v", projection.Fragments[0])
	}
	if len(projection.Commands) != 2 || projection.Commands[0].Bounds != (Rect{X: 2 * unit, Y: 2 * unit, Width: 16 * unit, Height: 12 * unit}) ||
		projection.Commands[1].Bounds != (Rect{X: 2 * unit, Y: 2 * unit, Width: 16 * unit, Height: unit}) {
		t.Fatalf("explicit group commands = %+v", projection.Commands)
	}
}
