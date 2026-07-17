// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"strings"
	"testing"
)

func TestAttachLinksOwnsExactDestinationsAndCanonicalCommandOrder(t *testing.T) {
	geometry, destinations, links := linkTestInputs(t)
	plan, err := AttachLinks(geometry, destinations, links)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if projection.SchemaVersion != LayoutPlanSchemaVersion || len(projection.Destinations) != 2 || len(projection.Links) != 2 || len(projection.Commands) != 2 {
		t.Fatalf("link projection = %+v", projection)
	}
	for index, command := range projection.Commands {
		if command.Kind != CommandLink || command.Payload != uint32(index) || command.Fragment != 1 {
			t.Fatalf("command %d = %+v", index, command)
		}
	}
	if projection.Pages[0].Commands.Count != 2 || projection.Pages[1].Commands.Count != 0 {
		t.Fatalf("page command ranges = %+v", projection.Pages)
	}
	// Destination 2 is a standalone anchor reserved for future bookmarks and
	// remains valid even though no link currently targets it.
	if projection.Destinations[1].ID != 2 {
		t.Fatalf("standalone destination = %+v", projection.Destinations[1])
	}
	encoded, err := plan.CanonicalJSON()
	if err != nil || !strings.Contains(string(encoded), `"destinations"`) || !strings.Contains(string(encoded), `"kind":"link"`) {
		t.Fatalf("canonical link JSON = %s, %v", encoded, err)
	}
	hash, _ := plan.Hash()
	again, _ := AttachLinks(geometry, destinations, links)
	againHash, _ := again.Hash()
	if hash != againHash {
		t.Fatalf("deterministic hashes differ: %s != %s", hash, againHash)
	}
	projection.Links[0].URI = "https://mutated.test"
	if plan.Projection().Links[0].URI != "" {
		t.Fatal("projection mutation reached immutable plan")
	}
}

func TestPlannedLinkValidationRejectsInvalidTargetsAndProvenance(t *testing.T) {
	geometry, destinations, links := linkTestInputs(t)
	valid, err := AttachLinks(geometry, destinations, links)
	if err != nil {
		t.Fatal(err)
	}
	base := valid.Projection()
	tests := []struct {
		name   string
		mutate func(*LayoutPlanInput)
	}{
		{"missing destination", func(input *LayoutPlanInput) { input.Links[0].Destination = 9 }},
		{"both targets", func(input *LayoutPlanInput) { input.Links[0].URI = "https://example.test" }},
		{"uppercase scheme", func(input *LayoutPlanInput) { input.Links[1].URI = "HTTPS://example.test" }},
		{"unsupported scheme", func(input *LayoutPlanInput) { input.Links[1].URI = "file:///tmp/x" }},
		{"link source", func(input *LayoutPlanInput) { input.Links[0].Source = input.Fragments[1].Source }},
		{"destination source", func(input *LayoutPlanInput) { input.Destinations[0].Source = input.Fragments[0].Source }},
		{"half-open destination edge", func(input *LayoutPlanInput) {
			fragment := input.Fragments[1]
			right, _ := fragment.BorderBox.Right()
			input.Destinations[0].Point.X = right
		}},
		{"link command bounds", func(input *LayoutPlanInput) { input.Commands[0].Bounds.Width++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := layoutPlanInputFromStoredProjection(base)
			test.mutate(&input)
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid link plan unexpectedly validated")
			}
		})
	}
}

func TestDisplayLinkRecordingAndLimitsPreflightBeforeCallbacks(t *testing.T) {
	geometry, destinations, links := linkTestInputs(t)
	plan, _ := AttachLinks(geometry, destinations, links)
	recording, err := RecordDisplayPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	events := recording.Events()
	linkEvents := 0
	for _, event := range events {
		if event.Kind == DisplayPaintLink {
			linkEvents++
			if event.Command.Kind != CommandLink || event.Link.Fragment != 1 {
				t.Fatalf("link event = %+v", event)
			}
		}
	}
	if linkEvents != 2 {
		t.Fatalf("recorded link events = %d, events=%+v", linkEvents, events)
	}
	sink := &recordingDisplaySink{}
	limits := DefaultDisplayPaintLimits()
	limits.MaxLinks = 1
	if err := PaintDisplayPlanWithLimits(plan, sink, limits); !errors.Is(err, ErrDisplayPaintLimit) {
		t.Fatalf("link limit error = %v", err)
	}
	if len(sink.calls) != 0 {
		t.Fatalf("limit failure invoked sink: %+v", sink.calls)
	}
	limits = DefaultDisplayPaintLimits()
	limits.MaxDestinations = 1
	if err := PaintDisplayPlanWithLimits(plan, sink, limits); !errors.Is(err, ErrDisplayPaintLimit) || len(sink.calls) != 0 {
		t.Fatalf("destination preflight = %v, calls=%+v", err, sink.calls)
	}
}

func linkTestInputs(t *testing.T) (LayoutPlan, []PlannedDestination, []PlannedLink) {
	t.Helper()
	source1 := SourceSpan{File: "links.paper", Start: SourcePosition{Line: 1, Column: 1}, End: SourcePosition{Offset: 1, Line: 1, Column: 2}}
	source2 := SourceSpan{File: "links.paper", Start: SourcePosition{Offset: 2, Line: 2, Column: 1}, End: SourcePosition{Offset: 3, Line: 2, Column: 2}}
	firstBounds := Rect{X: 10, Y: 10, Width: 80, Height: 30}
	secondBounds := Rect{X: 20, Y: 40, Width: 100, Height: 50}
	geometry, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{
			{Number: 1, Size: Size{Width: 200, Height: 150}, Fragments: IndexRange{Count: 1}},
			{Number: 2, Size: Size{Width: 200, Height: 150}, Fragments: IndexRange{Start: 1, Count: 1}},
		},
		Fragments: []Fragment{
			{ID: 1, Node: 1, Key: "@origin", Instance: "@origin", Page: 1, Region: RegionBody,
				BorderBox: firstBounds, ContentBox: firstBounds, Source: source1, Continuation: ContinuationWhole},
			{ID: 2, Node: 2, Key: "@target", Instance: "@target", Page: 2, Region: RegionBody,
				BorderBox: secondBounds, ContentBox: secondBounds, Source: source2, Continuation: ContinuationWhole},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	destinations := []PlannedDestination{
		{ID: 1, Page: 2, Fragment: 2, Point: Point{X: 25, Y: 45}, Source: source2},
		{ID: 2, Page: 2, Point: Point{X: 200, Y: 150}},
	}
	links := []PlannedLink{
		{Fragment: 1, Bounds: Rect{X: 12, Y: 12, Width: 20, Height: 10}, Destination: 1, Source: source1},
		{Fragment: 1, Bounds: Rect{X: 35, Y: 12, Width: 30, Height: 10}, URI: "https://example.test/path", Source: source1},
	}
	return geometry, destinations, links
}
