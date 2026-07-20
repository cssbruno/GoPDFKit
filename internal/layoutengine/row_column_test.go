// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestPlanRowColumnResolvesTracksAlignmentAndProvenance(t *testing.T) {
	input := testRowColumnInput(RowDirection)
	input.Gap = 2
	input.Align = CrossStart
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@fixed", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 10}, 0, 10, CrossStart),
		testRowColumnChild(2, "@auto", RowColumnTrack{Kind: RowColumnTrackAuto}, 11, 11, CrossCenter),
		testRowColumnChild(3, "@fraction-one", RowColumnTrack{Kind: RowColumnTrackFraction, Weight: 1}, 0, 12, CrossEnd),
		testRowColumnChild(4, "@fraction-two", RowColumnTrack{Kind: RowColumnTrackFraction, Weight: 2}, 0, 99, CrossStretch),
	}
	input.Children[1].Source = SourceSpan{
		File:  "layout.paper",
		Start: SourcePosition{Offset: 10, Line: 2, Column: 3},
		End:   SourcePosition{Offset: 14, Line: 2, Column: 7},
	}

	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatalf("PlanRowColumn() = %v", err)
	}
	if got, want := result.MainSizes(), []Fixed{10, 11, 25, 48}; !reflect.DeepEqual(got, want) {
		t.Fatalf("main sizes = %v, want %v", got, want)
	}
	if result.UsedMain != 100 {
		t.Fatalf("used main = %d, want 100", result.UsedMain)
	}
	projection := result.Plan.Projection()
	wantBounds := []Rect{
		{X: 10, Y: 20, Width: 10, Height: 10},
		{X: 22, Y: 34, Width: 11, Height: 11},
		{X: 35, Y: 48, Width: 25, Height: 12},
		{X: 62, Y: 20, Width: 48, Height: 40},
	}
	if len(projection.Pages) != 1 || len(projection.Fragments) != len(wantBounds) {
		t.Fatalf("projection cardinality = %d pages, %d fragments", len(projection.Pages), len(projection.Fragments))
	}
	for index, want := range wantBounds {
		fragment := projection.Fragments[index]
		if fragment.BorderBox != want || fragment.ContentBox != want {
			t.Errorf("fragment %d bounds = %+v, want %+v", index, fragment.BorderBox, want)
		}
		if fragment.Key != input.Children[index].Key || fragment.Source != input.Children[index].Source {
			t.Errorf("fragment %d provenance = key %q source %+v", index, fragment.Key, fragment.Source)
		}
	}

	mainSizes := result.MainSizes()
	mainSizes[0] = 999
	if result.MainSizes()[0] != 10 {
		t.Fatal("MainSizes exposed mutable planner state")
	}
}

func TestPlanRowColumnColumnUsesSharedAxesAndDeterministicRemainder(t *testing.T) {
	input := testRowColumnInput(ColumnDirection)
	input.Region = Rect{X: 10, Y: 20, Width: 40, Height: 10}
	input.Align = CrossCenter
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFraction, Weight: 1}, 0, 10, ""),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFraction, Weight: 1}, 0, 20, CrossEnd),
		testRowColumnChild(3, "@three", RowColumnTrack{Kind: RowColumnTrackFraction, Weight: 1}, 0, 0, CrossStretch),
	}

	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatalf("PlanRowColumn() = %v", err)
	}
	if got, want := result.MainSizes(), []Fixed{4, 3, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("remainder = %v, want %v", got, want)
	}
	fragments := result.Plan.Projection().Fragments
	want := []Rect{
		{X: 25, Y: 20, Width: 10, Height: 4},
		{X: 30, Y: 24, Width: 20, Height: 3},
		{X: 10, Y: 27, Width: 40, Height: 3},
	}
	for index := range want {
		if fragments[index].BorderBox != want[index] {
			t.Errorf("fragment %d = %+v, want %+v", index, fragments[index].BorderBox, want[index])
		}
	}
}

func TestPlanRowColumnAutoTracksShareRemainingAndFixedTracksMayUnderfill(t *testing.T) {
	input := testRowColumnInput(RowDirection)
	input.Region.Width = 10
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, ""),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, ""),
		testRowColumnChild(3, "@three", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, ""),
	}
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatalf("auto PlanRowColumn() = %v", err)
	}
	if got, want := result.MainSizes(), []Fixed{4, 3, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("auto remainder = %v, want %v", got, want)
	}

	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 2}, 0, 1, ""),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 3}, 0, 1, ""),
	}
	result, err = PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatalf("fixed PlanRowColumn() = %v", err)
	}
	if result.UsedMain != 5 {
		t.Fatalf("fixed used main = %d, want 5", result.UsedMain)
	}
}

func TestPlanRowColumnMainAxisJustificationUsesDeterministicFixedPointSlots(t *testing.T) {
	tests := []struct {
		name    string
		justify MainAlignment
		wantX   []Fixed
	}{
		{"start", MainStart, []Fixed{10, 15}},
		{"center", MainCenter, []Fixed{16, 21}},
		{"end", MainEnd, []Fixed{22, 27}},
		{"space-between", MainSpaceBetween, []Fixed{10, 27}},
		{"space-around", MainSpaceAround, []Fixed{13, 24}},
		{"space-evenly", MainSpaceEvenly, []Fixed{14, 23}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := testRowColumnInput(RowDirection)
			input.Region.Width = 20
			input.Gap = 2
			input.Justify = test.justify
			input.Children = []RowColumnChild{
				testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 3}, 0, 1, ""),
				testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 3}, 0, 1, ""),
			}
			result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
			if err != nil {
				t.Fatal(err)
			}
			fragments := result.Plan.Projection().Fragments
			if fragments[0].BorderBox.X != test.wantX[0] || fragments[1].BorderBox.X != test.wantX[1] || result.UsedMain != 8 {
				t.Fatalf("x=%d,%d used=%d want x=%v used=8", fragments[0].BorderBox.X, fragments[1].BorderBox.X, result.UsedMain, test.wantX)
			}
		})
	}

	input := testRowColumnInput(RowDirection)
	input.Region.Width = 19 // 11 free units: center assigns the first slot 6.
	input.Justify = MainCenter
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 1, ""),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 1, ""),
	}
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil || result.Plan.Projection().Fragments[0].BorderBox.X != 16 {
		t.Fatalf("odd center result=%+v err=%v", result.Plan.Projection().Fragments, err)
	}
}

func TestPlanRowColumnOverflowCarriesAuthoredDiagnostic(t *testing.T) {
	input := testRowColumnInput(RowDirection)
	input.Region.Width = 10
	input.Children = []RowColumnChild{
		testRowColumnChild(7, "@too-wide", RowColumnTrack{Kind: RowColumnTrackAuto}, 11, 4, ""),
	}
	_, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if !errors.Is(err, ErrRowColumnOverflow) {
		t.Fatalf("main overflow = %v, want ErrRowColumnOverflow", err)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticTrackMinOverflow {
		t.Fatalf("main diagnostic = %#v", planning)
	}
	if planning.Diagnostic.Location.Key != "@too-wide" || planning.Diagnostic.Location.Node != 7 {
		t.Fatalf("main location = %+v", planning.Diagnostic.Location)
	}

	input.Region.Width = 100
	input.Region.Height = 10
	input.Children[0].MinMain = 1
	input.Children[0].CrossSize = 11
	_, err = PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if !errors.Is(err, ErrRowColumnOverflow) || !errors.As(err, &planning) {
		t.Fatalf("cross overflow = %v", err)
	}
	if planning.Diagnostic.Location.Key != "@too-wide" || planning.Diagnostic.Evidence[0].Value != "cross" {
		t.Fatalf("cross diagnostic = %+v", planning.Diagnostic)
	}
}

func TestPlanRowColumnCancellationWorkAndStateBounds(t *testing.T) {
	input := testRowColumnInput(RowDirection)
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, ""),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, ""),
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PlanRowColumn(canceled, input, RowColumnPlanLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled = %v, want context.Canceled", err)
	}

	limits := RowColumnPlanLimits{MaxChildren: 1, MaxWork: 10, MaxStateBytes: 1 << 20}
	if _, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnResourceLimit) {
		t.Fatalf("state limit = %v, want ErrRowColumnResourceLimit", err)
	}
	limits = RowColumnPlanLimits{MaxChildren: 2, MaxWork: 2, MaxStateBytes: 1 << 20}
	if _, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnWorkLimit) {
		t.Fatalf("work limit = %v, want ErrRowColumnWorkLimit", err)
	}
	limits = RowColumnPlanLimits{MaxChildren: hardMaxRowColumnChildren + 1, MaxWork: 1, MaxStateBytes: 1}
	if _, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnLimitsInvalid) {
		t.Fatalf("hard cap = %v, want ErrRowColumnLimitsInvalid", err)
	}
	limits = RowColumnPlanLimits{MaxChildren: 2, MaxWork: 10, MaxStateBytes: 1024}
	if _, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnResourceLimit) {
		t.Fatalf("state bytes = %v, want ErrRowColumnResourceLimit", err)
	}
	largeProvenance := testRowColumnChild(1, "@source", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, "")
	largeProvenance.Source = SourceSpan{
		File:  strings.Repeat("a", 700),
		Start: SourcePosition{Offset: 1, Line: 1, Column: 1},
		End:   SourcePosition{Offset: 2, Line: 1, Column: 2},
	}
	input.Children = []RowColumnChild{largeProvenance}
	limits = RowColumnPlanLimits{MaxChildren: 2, MaxWork: 10, MaxStateBytes: 2000}
	if _, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnResourceLimit) {
		t.Fatalf("provenance state bytes = %v, want ErrRowColumnResourceLimit", err)
	}
	limits = RowColumnPlanLimits{MaxChildren: 2, MaxWork: 10}
	if _, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnLimitsInvalid) {
		t.Fatalf("partial limits = %v, want ErrRowColumnLimitsInvalid", err)
	}
}

func TestPlanRowColumnRejectsInvalidConstraints(t *testing.T) {
	input := testRowColumnInput(RowDirection)
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@bad", RowColumnTrack{Kind: RowColumnTrackFraction}, 0, 1, ""),
	}
	if _, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnTrack) {
		t.Fatalf("zero fraction weight = %v, want ErrRowColumnTrack", err)
	}
	input.Children[0] = testRowColumnChild(1, "@bad", RowColumnTrack{Kind: RowColumnTrackAuto}, 0, 1, CrossAlignment("sideways"))
	if _, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnAlignment) {
		t.Fatalf("alignment = %v, want ErrRowColumnAlignment", err)
	}
	input.Children[0].Align = ""
	input.Justify = MainAlignment("sideways")
	if _, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnJustification) {
		t.Fatalf("justification = %v, want ErrRowColumnJustification", err)
	}
}

func testRowColumnInput(direction RowColumnDirection) RowColumnPlanInput {
	return RowColumnPlanInput{
		PageSize:  Size{Width: 200, Height: 100},
		Region:    Rect{X: 10, Y: 20, Width: 100, Height: 40},
		Direction: direction,
	}
}

func testRowColumnChild(node NodeID, key NodeKey, track RowColumnTrack, minMain, cross Fixed, align CrossAlignment) RowColumnChild {
	return RowColumnChild{
		Node: node, Key: key, Instance: InstanceID(key), Track: track,
		MinMain: minMain, CrossSize: cross, Align: align,
	}
}
