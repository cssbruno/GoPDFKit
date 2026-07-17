// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"reflect"
	"testing"
)

func TestAttachGlyphRunLinksUsesExactAdvancesAndCommandOrder(t *testing.T) {
	plan := glyphLinkTestPlan(t)
	linked, err := AttachGlyphRunLinks(plan, []GlyphRunLinkSpan{
		{Run: 1, Start: 0, Count: 2, URI: "mailto:paper@example.test"},
		{Run: 0, Start: 1, Count: 2, URI: "https://example.test/exact"},
	})
	if err != nil {
		t.Fatal(err)
	}
	projection := linked.Projection()
	if got, want := projection.Links, []PlannedLink{
		{Fragment: 1, Bounds: Rect{X: 14, Y: 20, Width: 11, Height: 12}, URI: "https://example.test/exact"},
		{Fragment: 2, Bounds: Rect{X: 30, Y: 40, Width: 10, Height: 12}, URI: "mailto:paper@example.test"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("links = %#v, want %#v", got, want)
	}
	wantKinds := []DisplayCommandKind{CommandGlyphRun, CommandLink, CommandGlyphRun, CommandLink}
	if len(projection.Commands) != len(wantKinds) {
		t.Fatalf("commands = %#v", projection.Commands)
	}
	for index, kind := range wantKinds {
		if projection.Commands[index].Kind != kind {
			t.Fatalf("commands[%d] = %s, want %s", index, projection.Commands[index].Kind, kind)
		}
	}
	if projection.Pages[0].Commands.Count != 2 || projection.Pages[1].Commands.Count != 2 {
		t.Fatalf("page commands = %#v", projection.Pages)
	}
	if original := plan.Projection(); len(original.Links) != 0 || len(original.Commands) != 2 {
		t.Fatalf("source plan was mutated: %#v", original)
	}
}

func TestAttachGlyphRunLinksRejectsInvalidTargetsRangesAndReuse(t *testing.T) {
	plan := glyphLinkTestPlan(t)
	tests := []struct {
		name  string
		spans []GlyphRunLinkSpan
	}{
		{"missing run", []GlyphRunLinkSpan{{Run: 2, Count: 1, URI: "https://example.test"}}},
		{"empty", []GlyphRunLinkSpan{{Run: 0, URI: "https://example.test"}}},
		{"outside", []GlyphRunLinkSpan{{Run: 0, Start: 2, Count: 2, URI: "https://example.test"}}},
		{"overlap", []GlyphRunLinkSpan{{Run: 0, Count: 2, URI: "https://one.test"}, {Run: 0, Start: 1, Count: 2, URI: "https://two.test"}}},
		{"unsafe URI", []GlyphRunLinkSpan{{Run: 0, Count: 1, URI: "javascript:alert(1)"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := AttachGlyphRunLinks(plan, test.spans); err == nil {
				t.Fatal("invalid glyph link was accepted")
			}
		})
	}
	linked, err := AttachGlyphRunLinks(plan, []GlyphRunLinkSpan{{Run: 0, Count: 1, URI: "https://example.test"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AttachGlyphRunLinks(linked, []GlyphRunLinkSpan{{Run: 0, Count: 1, URI: "https://example.test"}}); !errors.Is(err, ErrGlyphLinkContract) {
		t.Fatalf("reuse = %v, want ErrGlyphLinkContract", err)
	}
}

func TestAttachGlyphRunLinksWithDestinationsPreservesExactInternalTarget(t *testing.T) {
	plan := glyphLinkTestPlan(t)
	destinations := []PlannedDestination{{ID: 1, Page: 2, Fragment: 2, Point: Point{X: 30, Y: 40}}}
	linked, err := AttachGlyphRunLinksWithDestinations(plan, destinations, []GlyphRunLinkSpan{{Run: 0, Start: 1, Count: 2, Destination: 1}})
	if err != nil {
		t.Fatal(err)
	}
	projection := linked.Projection()
	if !reflect.DeepEqual(projection.Destinations, destinations) || len(projection.Links) != 1 ||
		projection.Links[0].Destination != 1 || projection.Links[0].URI != "" || projection.Links[0].Bounds != (Rect{X: 14, Y: 20, Width: 11, Height: 12}) {
		t.Fatalf("internal glyph link = destinations %#v links %#v", projection.Destinations, projection.Links)
	}
	anchorOnly, err := AttachGlyphRunLinksWithDestinations(plan, destinations, nil)
	if err != nil || !reflect.DeepEqual(anchorOnly.Projection().Destinations, destinations) || len(anchorOnly.Projection().Links) != 0 {
		t.Fatalf("anchor-only plan = %#v, %v", anchorOnly.Projection(), err)
	}
	invalid := []GlyphRunLinkSpan{
		{Run: 0, Count: 1},
		{Run: 0, Count: 1, URI: "https://example.test", Destination: 1},
		{Run: 0, Count: 1, Destination: 2},
	}
	for index, span := range invalid {
		if _, err := AttachGlyphRunLinksWithDestinations(plan, destinations, []GlyphRunLinkSpan{span}); !errors.Is(err, ErrGlyphLinkContract) {
			t.Fatalf("invalid internal span %d = %v", index, err)
		}
	}
}

func glyphLinkTestPlan(t *testing.T) LayoutPlan {
	t.Helper()
	first := Rect{X: 10, Y: 20, Width: 15, Height: 12}
	second := Rect{X: 30, Y: 40, Width: 10, Height: 12}
	plan, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{
			{Number: 1, Size: Size{Width: 100, Height: 100}, Fragments: IndexRange{Count: 1}, Lines: IndexRange{Count: 1}, Commands: IndexRange{Count: 1}},
			{Number: 2, Size: Size{Width: 100, Height: 100}, Fragments: IndexRange{Start: 1, Count: 1}, Lines: IndexRange{Start: 1, Count: 1}, Commands: IndexRange{Start: 1, Count: 1}},
		},
		Fragments: []Fragment{
			{ID: 1, Node: 1, Key: "@one", Instance: "@one", Page: 1, Region: RegionBody, BorderBox: first, ContentBox: first, Continuation: ContinuationWhole},
			{ID: 2, Node: 2, Key: "@two", Instance: "@two", Page: 2, Region: RegionBody, BorderBox: second, ContentBox: second, Continuation: ContinuationWhole},
		},
		Lines: []PlannedLine{{Fragment: 1, Bounds: first, Baseline: 29}, {Fragment: 2, Bounds: second, Baseline: 49}},
		Fonts: []CoreFontResource{{ID: 1, Face: CoreFontCourier, MetricsDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		GlyphRuns: []CoreGlyphRun{
			{Line: 0, Font: 1, FontSize: Fixed(FixedScale * 10), Origin: Point{X: 10, Y: 29}, Codes: "ABC", Advances: []Fixed{4, 5, 6}},
			{Line: 1, Font: 1, FontSize: Fixed(FixedScale * 10), Origin: Point{X: 30, Y: 49}, Codes: "DE", Advances: []Fixed{3, 7}},
		},
		Commands: []DisplayCommand{{Kind: CommandGlyphRun, Fragment: 1, Bounds: first}, {Kind: CommandGlyphRun, Fragment: 2, Bounds: second, Payload: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
