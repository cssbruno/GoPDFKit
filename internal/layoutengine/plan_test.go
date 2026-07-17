// SPDX-License-Identifier: MIT
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
	if got, want := firstHash.String(), "d7a7125efd5ef97fc51e2b7b916e6426450ce55ab793bf8687ab3068205f73db"; got != want {
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
