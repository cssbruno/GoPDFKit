// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPlanRowColumnWrapFormsLinesAndPreservesAuthoredIdentity(t *testing.T) {
	input := testRowColumnInput(RowDirection)
	input.Region = Rect{X: 10, Y: 20, Width: 10, Height: 30}
	input.Wrap = RowColumnWrapForward
	input.Gap = 1
	input.CrossGap = 2
	input.Justify = MainCenter
	input.AlignContent = ContentSpaceBetween
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 3, CrossStart),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 2, CrossEnd),
		testRowColumnChild(3, "@three", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 5, CrossStretch),
		testRowColumnChild(4, "@four", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 1, CrossCenter),
		testRowColumnChild(5, "@five", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 4}, 0, 2, CrossStart),
	}

	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	wantLines := []RowColumnLine{
		{Children: IndexRange{Start: 0, Count: 2}, CrossStart: 20, CrossSize: 3, UsedMain: 9},
		{Children: IndexRange{Start: 2, Count: 2}, CrossStart: 33, CrossSize: 5, UsedMain: 9},
		{Children: IndexRange{Start: 4, Count: 1}, CrossStart: 48, CrossSize: 2, UsedMain: 4},
	}
	if got := result.Lines(); !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("lines = %+v, want %+v", got, wantLines)
	}
	if got, want := result.MainSizes(), []Fixed{4, 4, 4, 4, 4}; !reflect.DeepEqual(got, want) || result.UsedMain != 9 {
		t.Fatalf("sizes=%v used=%d, want %v/9", got, result.UsedMain, want)
	}
	wantBounds := []Rect{
		{X: 11, Y: 20, Width: 4, Height: 3},
		{X: 16, Y: 21, Width: 4, Height: 2},
		{X: 11, Y: 33, Width: 4, Height: 5},
		{X: 16, Y: 35, Width: 4, Height: 1},
		{X: 13, Y: 48, Width: 4, Height: 2},
	}
	fragments := result.Plan.Projection().Fragments
	for index, want := range wantBounds {
		if fragments[index].BorderBox != want {
			t.Errorf("fragment %d bounds = %+v, want %+v", index, fragments[index].BorderBox, want)
		}
		if fragments[index].Node != input.Children[index].Node || fragments[index].Key != input.Children[index].Key || fragments[index].Instance != input.Children[index].Instance {
			t.Errorf("fragment %d identity changed: %+v", index, fragments[index])
		}
	}
	lines := result.Lines()
	lines[0].CrossStart = 999
	if result.Lines()[0].CrossStart != 20 {
		t.Fatal("Lines exposed mutable planner state")
	}
}

func TestPlanRowColumnWrapReverseOnlyReversesPhysicalLinePlacement(t *testing.T) {
	input := testWrappedAlignmentInput(ContentSpaceBetween)
	input.Region = Rect{X: 10, Y: 20, Width: 10, Height: 17}
	input.Wrap = RowColumnWrapReverse
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	lines := result.Lines()
	if got, want := []Fixed{lines[0].CrossStart, lines[1].CrossStart, lines[2].CrossStart}, []Fixed{35, 27, 20}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reverse starts = %v, want %v", got, want)
	}
	fragments := result.Plan.Projection().Fragments
	for index := range fragments {
		if fragments[index].Key != input.Children[index].Key || fragments[index].ID != FragmentID(index+1) {
			t.Fatalf("fragment order/identity changed at %d: %+v", index, fragments[index])
		}
	}
}

func TestPlanRowColumnAlignContentUsesDeterministicLogicalSlots(t *testing.T) {
	tests := []struct {
		name      string
		alignment ContentAlignment
		wantStart []Fixed
		wantSize  []Fixed
	}{
		{"start", ContentStart, []Fixed{20, 23, 26}, []Fixed{2, 2, 2}},
		{"center", ContentCenter, []Fixed{25, 28, 31}, []Fixed{2, 2, 2}},
		{"end", ContentEnd, []Fixed{29, 32, 35}, []Fixed{2, 2, 2}},
		{"space-between", ContentSpaceBetween, []Fixed{20, 28, 35}, []Fixed{2, 2, 2}},
		{"space-around", ContentSpaceAround, []Fixed{22, 29, 34}, []Fixed{2, 2, 2}},
		{"space-evenly", ContentSpaceEvenly, []Fixed{23, 28, 33}, []Fixed{2, 2, 2}},
		{"stretch", ContentStretch, []Fixed{20, 26, 32}, []Fixed{5, 5, 5}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := PlanRowColumn(context.Background(), testWrappedAlignmentInput(test.alignment), RowColumnPlanLimits{})
			if err != nil {
				t.Fatal(err)
			}
			lines := result.Lines()
			for index := range lines {
				if lines[index].CrossStart != test.wantStart[index] || lines[index].CrossSize != test.wantSize[index] {
					t.Fatalf("line %d = start %d size %d, want %d/%d", index, lines[index].CrossStart, lines[index].CrossSize, test.wantStart[index], test.wantSize[index])
				}
			}
		})
	}
}

func TestPlanRowColumnColumnWrapUsesSharedAxesAndPerLineTrackSolver(t *testing.T) {
	input := testRowColumnInput(ColumnDirection)
	input.Region = Rect{X: 10, Y: 20, Width: 20, Height: 10}
	input.Wrap = RowColumnWrapForward
	input.Gap = 1
	input.CrossGap = 2
	input.AlignContent = ContentCenter
	input.Align = CrossStretch
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackAuto}, 6, 3, ""),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackAuto}, 6, 5, ""),
	}
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.MainSizes(), []Fixed{10, 10}; !reflect.DeepEqual(got, want) {
		t.Fatalf("per-line sizes = %v, want %v", got, want)
	}
	fragments := result.Plan.Projection().Fragments
	want := []Rect{{X: 15, Y: 20, Width: 3, Height: 10}, {X: 20, Y: 20, Width: 5, Height: 10}}
	for index := range want {
		if fragments[index].BorderBox != want[index] {
			t.Errorf("fragment %d = %+v, want %+v", index, fragments[index].BorderBox, want[index])
		}
	}
}

func TestPlanRowColumnWrapRejectsInvalidOverflowAndBoundedWorkAtomically(t *testing.T) {
	input := testWrappedAlignmentInput(ContentStart)
	input.Wrap = RowColumnWrap("sideways")
	if result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnWrap) || len(result.Plan.Projection().Pages) != 0 {
		t.Fatalf("invalid wrap result=%+v err=%v", result, err)
	}
	input.Wrap = RowColumnWrapForward
	input.AlignContent = ContentAlignment("baseline")
	if result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnContentAlign) || len(result.Plan.Projection().Pages) != 0 {
		t.Fatalf("invalid alignment result=%+v err=%v", result, err)
	}
	input.AlignContent = ContentStart
	input.CrossGap = 6
	if result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnOverflow) || len(result.Plan.Projection().Pages) != 0 {
		t.Fatalf("cross overflow result=%+v err=%v", result, err)
	}
	input.CrossGap = 1
	limits := RowColumnPlanLimits{MaxChildren: 3, MaxWork: 4, MaxStateBytes: 1 << 20}
	if result, err := PlanRowColumn(context.Background(), input, limits); !errors.Is(err, ErrRowColumnWorkLimit) || len(result.Plan.Projection().Pages) != 0 {
		t.Fatalf("work bound result=%+v err=%v", result, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := PlanRowColumn(canceled, input, RowColumnPlanLimits{}); !errors.Is(err, context.Canceled) || len(result.Plan.Projection().Pages) != 0 {
		t.Fatalf("cancellation result=%+v err=%v", result, err)
	}
}

func testWrappedAlignmentInput(alignment ContentAlignment) RowColumnPlanInput {
	input := testRowColumnInput(RowDirection)
	input.Region = Rect{X: 10, Y: 20, Width: 10, Height: 17}
	input.Wrap = RowColumnWrapForward
	input.CrossGap = 1
	input.AlignContent = alignment
	input.Children = []RowColumnChild{
		testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 6}, 0, 2, CrossStretch),
		testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 6}, 0, 2, CrossStretch),
		testRowColumnChild(3, "@three", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 6}, 0, 2, CrossStretch),
	}
	return input
}
