// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"strings"
	"testing"
)

func TestCoreGlyphPlanOwnsNestedAdvances(t *testing.T) {
	input := coreGlyphPlanInput()
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	wantHash, err := plan.Hash()
	if err != nil {
		t.Fatalf("Hash() = %v", err)
	}
	input.GlyphRuns[0].Advances[0] = 99
	if got, _ := plan.Hash(); got != wantHash {
		t.Fatalf("input advance mutation changed hash: %s != %s", got, wantHash)
	}
	projection := plan.Projection()
	projection.GlyphRuns[0].Advances[0] = 98
	if got := plan.Projection().GlyphRuns[0].Advances[0]; got != 4 {
		t.Fatalf("projection advance mutation reached plan: got %d", got)
	}
	encoded, err := plan.CanonicalJSON()
	if err != nil || !strings.Contains(string(encoded), `"schema_version":16`) ||
		!strings.Contains(string(encoded), `"planner_version":"layoutengine/0.1"`) ||
		!strings.Contains(string(encoded), `"glyph_runs"`) || !strings.Contains(string(encoded), `"payload":0`) {
		t.Fatalf("CanonicalJSON() = %s, %v", encoded, err)
	}
}

func TestCoreGlyphPlanValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*LayoutPlanInput)
	}{
		{"font ID", func(input *LayoutPlanInput) { input.Fonts[0].ID = 2 }},
		{"font face", func(input *LayoutPlanInput) { input.Fonts[0].Face = "arial" }},
		{"font digest uppercase", func(input *LayoutPlanInput) {
			input.Fonts[0].MetricsDigest = CoreFontMetricsDigest(strings.Repeat("A", 64))
		}},
		{"font digest zero", func(input *LayoutPlanInput) {
			input.Fonts[0].MetricsDigest = CoreFontMetricsDigest(strings.Repeat("0", 64))
		}},
		{"unused font", func(input *LayoutPlanInput) {
			input.Fonts = append(input.Fonts, CoreFontResource{ID: 2, Face: CoreFontHelvetica, MetricsDigest: CoreFontMetricsDigest(strings.Repeat("2", 64))})
		}},
		{"duplicate font face", func(input *LayoutPlanInput) {
			input.Fonts = append(input.Fonts, CoreFontResource{ID: 2, Face: CoreFontCourier, MetricsDigest: CoreFontMetricsDigest(strings.Repeat("2", 64))})
		}},
		{"run line", func(input *LayoutPlanInput) { input.GlyphRuns[0].Line = 1 }},
		{"run font", func(input *LayoutPlanInput) { input.GlyphRuns[0].Font = 2 }},
		{"run size", func(input *LayoutPlanInput) { input.GlyphRuns[0].FontSize = 0 }},
		{"run unset color", func(input *LayoutPlanInput) { input.GlyphRuns[0].Color.R = 1 }},
		{"run origin", func(input *LayoutPlanInput) { input.GlyphRuns[0].Origin.X++ }},
		{"empty codes", func(input *LayoutPlanInput) { input.GlyphRuns[0].Codes = ""; input.GlyphRuns[0].Advances = nil }},
		{"control code", func(input *LayoutPlanInput) { input.GlyphRuns[0].Codes = "A\n" }},
		{"advance length", func(input *LayoutPlanInput) { input.GlyphRuns[0].Advances = []Fixed{10} }},
		{"advance negative", func(input *LayoutPlanInput) { input.GlyphRuns[0].Advances[0] = -1 }},
		{"advance sum", func(input *LayoutPlanInput) { input.GlyphRuns[0].Advances[0]++ }},
		{"command payload", func(input *LayoutPlanInput) { input.Commands[0].Payload = 1 }},
		{"command bounds", func(input *LayoutPlanInput) { input.Commands[0].Bounds.Width++ }},
		{"unreferenced run", func(input *LayoutPlanInput) { input.Commands[0].Kind = CommandFillPath }},
		{"duplicate line run", func(input *LayoutPlanInput) {
			input.GlyphRuns = append(input.GlyphRuns, input.GlyphRuns[0])
			input.Commands = append(input.Commands, DisplayCommand{Kind: CommandGlyphRun, Fragment: 1, Bounds: input.Lines[0].Bounds, Payload: 1})
			input.Pages[0].Commands.Count = 2
		}},
		{"out of order runs", func(input *LayoutPlanInput) {
			*input = twoLineCoreGlyphPlanInput()
			input.GlyphRuns[0], input.GlyphRuns[1] = input.GlyphRuns[1], input.GlyphRuns[0]
			input.Commands[0] = DisplayCommand{Kind: CommandGlyphRun, Fragment: 1, Bounds: input.Lines[1].Bounds, Payload: 0}
			input.Commands[1] = DisplayCommand{Kind: CommandGlyphRun, Fragment: 1, Bounds: input.Lines[0].Bounds, Payload: 1}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := coreGlyphPlanInput()
			test.mutate(&input)
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid glyph plan unexpectedly validated")
			}
		})
	}
}

func TestEmbeddedUTF8FontPlanIsCanonicalDetachedAndValidated(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Fonts[0] = CoreFontResource{ID: 1, MetricsDigest: CoreFontMetricsDigest(strings.Repeat("3", 64)), EmbeddedUTF8: &EmbeddedUTF8Font{
		Name: "PlanSans", Digest: CoreFontMetricsDigest(strings.Repeat("4", 64)), ByteLength: 1234,
	}}
	input.GlyphRuns[0].Codes = "Ação"
	input.GlyphRuns[0].Advances = []Fixed{2, 3, 2, 3}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan(embedded UTF-8) = %v", err)
	}
	firstHash, _ := plan.Hash()
	input.Fonts[0].EmbeddedUTF8.Name = "mutated"
	projection := plan.Projection()
	projection.Fonts[0].EmbeddedUTF8.Name = "also-mutated"
	secondHash, _ := plan.Hash()
	if firstHash != secondHash || plan.Projection().Fonts[0].EmbeddedUTF8.Name != "PlanSans" {
		t.Fatal("embedded font descriptor mutation reached immutable plan")
	}
	encoded, err := plan.CanonicalJSON()
	if err != nil || !strings.Contains(string(encoded), `"embedded_utf8":{"name":"PlanSans"`) || strings.Contains(string(encoded), "font_program") {
		t.Fatalf("canonical embedded font JSON = %s, %v", encoded, err)
	}
	catalog, err := ResourceCatalogFromPlan(plan)
	if err != nil || len(catalog.Resources) != 1 || catalog.Resources[0].Kind != "embedded-utf8-font" ||
		catalog.Resources[0].Digest != strings.Repeat("4", 64) {
		t.Fatalf("embedded font resource catalog = %#v, %v", catalog, err)
	}

	invalid := coreGlyphPlanInput()
	invalid.Fonts[0] = CoreFontResource{ID: 1, Face: CoreFontCourier, MetricsDigest: CoreFontMetricsDigest(strings.Repeat("3", 64)), EmbeddedUTF8: &EmbeddedUTF8Font{
		Name: "PlanSans", Digest: CoreFontMetricsDigest(strings.Repeat("4", 64)), ByteLength: 1234,
	}}
	if _, err := NewLayoutPlan(invalid); err == nil {
		t.Fatal("embedded font with a core face unexpectedly validated")
	}
}

func TestAttachCoreGlyphRunsBuildsPageCommandRanges(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Pages[0].Commands = IndexRange{}
	geometry, err := NewLayoutPlan(LayoutPlanInput{
		Pages: input.Pages, Fragments: input.Fragments, Lines: input.Lines,
	})
	if err != nil {
		t.Fatalf("geometry NewLayoutPlan() = %v", err)
	}
	plan, err := AttachCoreGlyphRuns(geometry, input.Fonts, input.GlyphRuns)
	if err != nil {
		t.Fatalf("AttachCoreGlyphRuns() = %v", err)
	}
	projection := plan.Projection()
	if got := projection.Pages[0].Commands; got != (IndexRange{Count: 1}) {
		t.Fatalf("page command range = %+v", got)
	}
	if got := projection.Commands[0]; got.Kind != CommandGlyphRun || got.Payload != 0 || got.Fragment != 1 {
		t.Fatalf("glyph command = %+v", got)
	}
}

func TestAttachCoreGlyphRunsAcceptsExactAdjacentMixedRunsOnOneLine(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Pages[0].Commands = IndexRange{}
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: input.Pages, Fragments: input.Fragments, Lines: input.Lines})
	if err != nil {
		t.Fatal(err)
	}
	line := input.Lines[0]
	runs := []CoreGlyphRun{
		{Line: 0, Font: 1, FontSize: input.GlyphRuns[0].FontSize, Origin: Point{X: line.Bounds.X, Y: line.Baseline}, Codes: input.GlyphRuns[0].Codes[:1], Advances: append([]Fixed(nil), input.GlyphRuns[0].Advances[:1]...)},
		{Line: 0, Font: 1, FontSize: input.GlyphRuns[0].FontSize, Origin: Point{X: line.Bounds.X + input.GlyphRuns[0].Advances[0], Y: line.Baseline}, Codes: input.GlyphRuns[0].Codes[1:], Advances: append([]Fixed(nil), input.GlyphRuns[0].Advances[1:]...), Color: CoreRGBColor{B: 200, Set: true}},
	}
	plan, err := AttachCoreGlyphRuns(geometry, input.Fonts, runs)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.GlyphRuns) != 2 || projection.Pages[0].Commands.Count != 2 || projection.GlyphRuns[1].Origin.X != line.Bounds.X+input.GlyphRuns[0].Advances[0] {
		t.Fatalf("mixed line projection = %+v / %+v", projection.GlyphRuns, projection.Pages[0])
	}
}

func coreGlyphPlanInput() LayoutPlanInput {
	line := PlannedLine{
		Fragment: 1, Bounds: Rect{X: 10, Y: 20, Width: 10, Height: 12}, Baseline: 29,
	}
	return LayoutPlanInput{
		Pages: []PlannedPage{{
			Number: 1, Size: Size{Width: 100, Height: 100}, Fragments: IndexRange{Count: 1},
			Lines: IndexRange{Count: 1}, Commands: IndexRange{Count: 1},
		}},
		Fragments: []Fragment{{
			ID: 1, Node: 1, Key: "@glyph", Instance: "@glyph", Page: 1, Region: RegionBody,
			BorderBox:  Rect{X: 10, Y: 20, Width: 10, Height: 12},
			ContentBox: Rect{X: 10, Y: 20, Width: 10, Height: 12}, Continuation: ContinuationWhole,
		}},
		Lines: []PlannedLine{line},
		Fonts: []CoreFontResource{{
			ID: 1, Face: CoreFontCourier, MetricsDigest: CoreFontMetricsDigest(strings.Repeat("1", 64)),
		}},
		GlyphRuns: []CoreGlyphRun{{
			Line: 0, Font: 1, FontSize: Fixed(10 * FixedScale), Origin: Point{X: line.Bounds.X, Y: line.Baseline},
			Codes: "AB", Advances: []Fixed{4, 6},
		}},
		Commands: []DisplayCommand{{Kind: CommandGlyphRun, Fragment: 1, Bounds: line.Bounds}},
	}
}

func twoLineCoreGlyphPlanInput() LayoutPlanInput {
	input := coreGlyphPlanInput()
	secondLine := input.Lines[0]
	secondLine.Index = 1
	secondLine.Bounds.Y += secondLine.Bounds.Height
	secondLine.Baseline += secondLine.Bounds.Height
	input.Lines = append(input.Lines, secondLine)
	input.Pages[0].Lines.Count = 2
	secondRun := input.GlyphRuns[0]
	secondRun.Line = 1
	secondRun.Origin = Point{X: secondLine.Bounds.X, Y: secondLine.Baseline}
	input.GlyphRuns = append(input.GlyphRuns, secondRun)
	input.Commands = append(input.Commands, DisplayCommand{
		Kind: CommandGlyphRun, Fragment: secondLine.Fragment, Bounds: secondLine.Bounds, Payload: 1,
	})
	input.Pages[0].Commands.Count = 2
	return input
}
