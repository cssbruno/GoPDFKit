// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLayoutPlanCopiesInputAndReturnedProjection(t *testing.T) {
	input := testPlanInput()
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}

	input.Pages[0].Number = 99
	input.Breaks[0].Reason = BreakPreviousFragmentOverflow
	input.Diagnostics[0].Evidence[0].Value = "mutated input"
	projection := plan.Projection()
	if projection.Pages[0].Number != 1 ||
		projection.Breaks[0].Reason != BreakInsufficientRemainingBodySpace ||
		projection.Diagnostics[0].Evidence[0].Value != "12pt" {
		t.Fatal("plan retained mutable input aliases")
	}

	projection.Fragments[0].Key = "@mutated"
	projection.Breaks[0].Region = "mutated"
	projection.Diagnostics[0].Evidence[0].Value = "mutated projection"
	projection = plan.Projection()
	if projection.Fragments[0].Key != "@lines" || projection.Breaks[0].Region != RegionBody ||
		projection.Diagnostics[0].Evidence[0].Value != "12pt" {
		t.Fatal("Projection() exposed mutable plan storage")
	}
}

func TestLayoutPlanProjectionAndHashAreDeterministic(t *testing.T) {
	first, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan(first) = %v", err)
	}
	second, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan(second) = %v", err)
	}

	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	secondJSON, _ := second.CanonicalJSON()
	if string(firstJSON) != string(secondJSON) {
		t.Fatalf("canonical projections differ:\n%s\n%s", firstJSON, secondJSON)
	}
	if !strings.Contains(string(firstJSON), `"width"`) || strings.Contains(string(firstJSON), `"Width"`) {
		t.Fatalf("canonical projection did not use the pinned lower-case geometry schema: %s", firstJSON)
	}
	if decodedVersion := first.Projection(); decodedVersion.PlannerVersion != PlannerVersion || decodedVersion.PainterContractVersion != PainterContractVersion {
		t.Fatalf("canonical projection versions = %+v", decodedVersion)
	}
	var decoded LayoutPlanProjection
	if err := json.Unmarshal(firstJSON, &decoded); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	roundTrip, err := json.Marshal(decoded)
	if err != nil || string(roundTrip) != string(firstJSON) {
		t.Fatalf("projection round trip = %s, %v; want %s", roundTrip, err, firstJSON)
	}
	firstHash, err := first.Hash()
	if err != nil {
		t.Fatalf("Hash() = %v", err)
	}
	secondHash, _ := second.Hash()
	if firstHash != secondHash {
		t.Fatalf("hashes differ: %s != %s", firstHash, secondHash)
	}
	if got, want := firstHash.String(), "b76f4922fd0609b7d8e3d285f4f3abf3b76a98f050d26e8302154caaa793055a"; got != want {
		t.Fatalf("Hash() = %s, want %s", got, want)
	}
}

func TestLayoutPlanValidationRejectsIdentityCollisionAndBadPageRange(t *testing.T) {
	input := testPlanInput()
	input.Fragments = append(input.Fragments, input.Fragments[0])
	input.Pages[0].Fragments.Count++
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("duplicate FragmentID unexpectedly validated")
	}

	input = testPlanInput()
	input.Pages[0].Commands.Start = 1
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("non-contiguous command range unexpectedly validated")
	}

	input = testPlanInput()
	input.Fragments[0].Region = ""
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("absent fragment region unexpectedly validated")
	}

	input = testPlanInput()
	input.Fragments[0].Instance = InstanceID(string([]byte{'@', 0xff}))
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("invalid UTF-8 fragment instance unexpectedly validated")
	}

	input = testPlanInput()
	input.Fragments[0].Repeated = true
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("repeated fragment without an earlier original unexpectedly validated")
	}
}

func TestLayoutPlanDefaultsCoincidentBoxLayersAndRejectsInvalidNesting(t *testing.T) {
	input := testPlanInput()
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	fragment := plan.Projection().Fragments[0]
	if fragment.MarginBox != fragment.BorderBox || fragment.PaddingBox != fragment.ContentBox {
		t.Fatalf("default box layers = %+v", fragment)
	}

	input = testPlanInput()
	input.Fragments[0].MarginBox = Rect{X: 20, Y: 20, Width: 10, Height: 10}
	input.Fragments[0].PaddingBox = input.Fragments[0].ContentBox
	if _, err := NewLayoutPlan(input); err == nil || !strings.Contains(err.Error(), "box-model rectangles are not nested") {
		t.Fatalf("invalid nesting = %v", err)
	}
}

func TestLayoutPlanValidatesAndCopiesRetainedGridTracks(t *testing.T) {
	input := testPlanInput()
	input.GridTracks = []PlannedGridTrack{
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackColumn, Bounds: Rect{X: 0, Y: 0, Width: 10, Height: 20}, GapAfter: 2},
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackRow, Bounds: Rect{X: 0, Y: 0, Width: 10, Height: 20}},
	}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	input.GridTracks[0].Bounds.Width = 99
	projection := plan.Projection()
	projection.GridTracks[0].Bounds.Width = 88
	if got := plan.Projection().GridTracks[0].Bounds.Width; got != 10 {
		t.Fatalf("retained grid track width = %d, want 10", got)
	}

	invalid := testPlanInput()
	invalid.GridTracks = []PlannedGridTrack{{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackRow, Bounds: Rect{Width: 10, Height: 10}}}
	if _, err := NewLayoutPlan(invalid); err == nil || !strings.Contains(err.Error(), "begin with column zero") {
		t.Fatalf("row-first grid tracks = %v", err)
	}
	invalid.GridTracks = []PlannedGridTrack{
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackColumn, Bounds: Rect{Width: 10, Height: 10}},
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackColumn, Index: 2, Bounds: Rect{Width: 10, Height: 10}},
	}
	if _, err := NewLayoutPlan(invalid); err == nil || !strings.Contains(err.Error(), "indexes are not consecutive") {
		t.Fatalf("nonconsecutive grid tracks = %v", err)
	}
}

func TestLayoutPlanValidatesAndCopiesRetainedPageRegions(t *testing.T) {
	input := testPlanInput()
	input.PageRegions = []PlannedPageRegion{{Page: 1, Region: RegionBody, Bounds: Rect{X: 5, Y: 5, Width: 90, Height: 90}, Master: "default"}}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatal(err)
	}
	input.PageRegions[0].Bounds.Width = 1
	projection := plan.Projection()
	projection.PageRegions[0].Bounds.Width = 2
	if got := plan.Projection().PageRegions[0].Bounds.Width; got != 90 {
		t.Fatalf("retained page-region width = %d, want 90", got)
	}
	invalid := testPlanInput()
	invalid.PageRegions = []PlannedPageRegion{{Page: 1, Region: RegionBody, Bounds: Rect{X: invalid.Pages[0].Size.Width - 5, Width: 10, Height: 10}}}
	if _, err := NewLayoutPlan(invalid); err == nil || !strings.Contains(err.Error(), "outside its page") {
		t.Fatalf("outside page region = %v", err)
	}
	invalid.PageRegions = []PlannedPageRegion{
		{Page: 1, Region: RegionBody, Bounds: Rect{Width: 10, Height: 10}},
		{Page: 1, Region: RegionHeader, Bounds: Rect{Width: 10, Height: 10}},
	}
	if _, err := NewLayoutPlan(invalid); err == nil || !strings.Contains(err.Error(), "unique and ordered") {
		t.Fatalf("unordered page regions = %v", err)
	}
}

func TestLayoutPlanValidationRejectsContradictoryDiagnosticFragmentProvenance(t *testing.T) {
	input := testPlanInput()
	input.Diagnostics[0].Location.Key = "@wrong"
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("contradictory diagnostic node provenance unexpectedly validated")
	}

	input = testPlanInput()
	input.Diagnostics[0].Location.Region = "header"
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("contradictory diagnostic fragment provenance unexpectedly validated")
	}
}

func TestLayoutPlanRequiresSourceAndPageEvidenceForRetainedLayoutDiagnostics(t *testing.T) {
	input := testPlanInput()
	input.Diagnostics[0].Location.Page = 0
	if _, err := NewLayoutPlan(input); err == nil || !strings.Contains(err.Error(), "no page evidence") {
		t.Fatalf("missing page evidence error = %v", err)
	}

	input = testPlanInput()
	input.Diagnostics[0].Location.Fragment = 0
	input.Diagnostics[0].Location.Node = 0
	input.Diagnostics[0].Location.Key = ""
	input.Diagnostics[0].Location.Instance = ""
	input.Diagnostics[0].Location.Source = SourceSpan{}
	if _, err := NewLayoutPlan(input); err == nil || !strings.Contains(err.Error(), "no source evidence") {
		t.Fatalf("missing source evidence error = %v", err)
	}
}

func TestLayoutPlanValidatesBreakDecisionReasonsAndProvenance(t *testing.T) {
	input := testPlanInput()
	input.Breaks[0].Reason = BreakPreviousFragmentOverflow
	input.Breaks[0].Required = 1
	input.Breaks[0].Available = 0
	if _, err := NewLayoutPlan(input); err != nil {
		t.Fatalf("valid previous-overflow break = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*BreakDecision)
	}{
		{"reason", func(b *BreakDecision) { b.Reason = "unknown" }},
		{"pages", func(b *BreakDecision) { b.ToPage = b.FromPage }},
		{"region", func(b *BreakDecision) { b.Region = "header" }},
		{"preceding", func(b *BreakDecision) { b.Preceding = b.Triggering }},
		{"triggering", func(b *BreakDecision) { b.Triggering = 99 }},
		{"negative required", func(b *BreakDecision) { b.Required = -1 }},
		{"space evidence", func(b *BreakDecision) { b.Required = b.Available }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := testPlanInput()
			test.mutate(&input.Breaks[0])
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid break decision unexpectedly validated")
			}
		})
	}

	input = testPlanInput()
	input.Breaks[0].Reason = BreakPreviousFragmentOverflow
	input.Breaks[0].Available = 1
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("previous-overflow break with available height unexpectedly validated")
	}
}

func TestLayoutPlanHashNormalizesUnorderedDiagnosticEvidence(t *testing.T) {
	firstInput := testPlanInput()
	firstInput.Diagnostics[0].Evidence = append(firstInput.Diagnostics[0].Evidence, DiagnosticEvidence{Key: "available", Value: "100pt"})
	secondInput := testPlanInput()
	secondInput.Diagnostics[0].Evidence = []DiagnosticEvidence{
		{Key: "available", Value: "100pt"},
		{Key: "overflow", Value: "12pt"},
	}
	first, err := NewLayoutPlan(firstInput)
	if err != nil {
		t.Fatalf("NewLayoutPlan(first) = %v", err)
	}
	second, err := NewLayoutPlan(secondInput)
	if err != nil {
		t.Fatalf("NewLayoutPlan(second) = %v", err)
	}
	firstHash, err := first.Hash()
	if err != nil {
		t.Fatalf("first.Hash() = %v", err)
	}
	secondHash, err := second.Hash()
	if err != nil {
		t.Fatalf("second.Hash() = %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("normalized hashes differ: %s != %s", firstHash, secondHash)
	}
}

func TestIndexRangeRejectsAnOutOfRangeEndBeforeIntConversion(t *testing.T) {
	if _, ok := (IndexRange{Start: ^uint32(0), Count: 1}).end(1); ok {
		t.Fatal("out-of-range index range unexpectedly validated")
	}
}

func testPlanInput() LayoutPlanInput {
	return LayoutPlanInput{
		Pages: []PlannedPage{
			{
				Number:    1,
				Size:      Size{Width: Fixed(612 * FixedScale), Height: Fixed(792 * FixedScale)},
				Fragments: IndexRange{Count: 1},
				Commands:  IndexRange{Count: 2},
			},
			{
				Number:    2,
				Size:      Size{Width: Fixed(612 * FixedScale), Height: Fixed(792 * FixedScale)},
				Fragments: IndexRange{Start: 1, Count: 1},
				Commands:  IndexRange{Start: 2},
			},
		},
		Fragments: []Fragment{
			{
				ID:           1,
				Node:         7,
				Key:          "@lines",
				Instance:     "@lines",
				Page:         1,
				Region:       RegionBody,
				BorderBox:    Rect{X: 10, Y: 20, Width: 300, Height: 80},
				ContentBox:   Rect{X: 14, Y: 24, Width: 292, Height: 72},
				Continuation: ContinuationStart,
			},
			{
				ID:           2,
				Node:         7,
				Key:          "@lines",
				Instance:     "@lines",
				Page:         2,
				Region:       RegionBody,
				BorderBox:    Rect{X: 10, Y: 20, Width: 300, Height: 60},
				ContentBox:   Rect{X: 14, Y: 24, Width: 292, Height: 52},
				Continuation: ContinuationEnd,
			},
		},
		Commands: []DisplayCommand{
			{Kind: CommandSaveState},
			{Kind: CommandFillPath, Fragment: 1, Bounds: Rect{X: 14, Y: 24, Width: 100, Height: 12}, Payload: 3},
		},
		Breaks: []BreakDecision{{
			Reason:     BreakInsufficientRemainingBodySpace,
			FromPage:   1,
			ToPage:     2,
			Region:     RegionBody,
			Preceding:  1,
			Triggering: 2,
			Required:   60,
			Available:  40,
		}},
		Diagnostics: []Diagnostic{testDiagnostic()},
	}
}
