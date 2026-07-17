// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"testing"
)

func TestRowColumnFlexGrowShrinkPercentMinMaxAndRemainders(t *testing.T) {
	input := RowColumnPlanInput{
		PageSize: Size{Width: 101, Height: 20}, Region: Rect{Width: 101, Height: 20}, Direction: RowDirection,
		Children: []RowColumnChild{
			testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFlex, BasisKind: RowColumnFlexBasisFixed, Basis: 20, Grow: 1, Shrink: 1, Max: 30}, 10, 2, ""),
			testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFlex, BasisKind: RowColumnFlexBasisPercent, BasisPercent: 25_000_000, Grow: 1, Shrink: 2}, 5, 2, ""),
			testRowColumnChild(3, "@three", RowColumnTrack{Kind: RowColumnTrackFlex, BasisKind: RowColumnFlexBasisFixed, Basis: 20, Grow: 1, Shrink: 1}, 5, 2, ""),
		},
	}
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	// 25% truncates to 25. The first item freezes at max 30; remaining free
	// space is redistributed exactly, with the indivisible unit going first.
	if got, want := result.MainSizes(), []Fixed{30, 38, 33}; !fixedSlicesEqual(got, want) {
		t.Fatalf("grow sizes = %v, want %v", got, want)
	}

	input.Region.Width = 40
	input.PageSize.Width = 40
	input.Children[0].Track.Max = 0
	input.Children[1].Track.BasisKind = RowColumnFlexBasisFixed
	input.Children[1].Track.BasisPercent = 0
	input.Children[1].Track.Basis = 20
	result, err = PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	// Deficit 20 is apportioned by scaled shrink factors 20:40:20.
	if got, want := result.MainSizes(), []Fixed{15, 10, 15}; !fixedSlicesEqual(got, want) {
		t.Fatalf("shrink sizes = %v, want %v", got, want)
	}
}

func TestRowColumnFlexReverseMainPreservesSemanticOrder(t *testing.T) {
	input := RowColumnPlanInput{
		PageSize: Size{Width: 35, Height: 10}, Region: Rect{X: 5, Y: 2, Width: 30, Height: 8},
		Direction: RowDirection, ReverseMain: true, Gap: 2,
		Children: []RowColumnChild{
			testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 5}, 0, 2, ""),
			testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 7}, 0, 2, ""),
		},
	}
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	fragments := result.Plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].BorderBox.X != 30 || fragments[1].BorderBox.X != 21 {
		t.Fatalf("reverse geometry = %+v", fragments)
	}
	if fragments[0].Node != 1 || fragments[1].Node != 2 {
		t.Fatalf("semantic order changed: %+v", fragments)
	}
	input.PageSize = Size{Width: 10, Height: 35}
	input.Region = Rect{X: 2, Y: 5, Width: 8, Height: 30}
	input.Direction = ColumnDirection
	result, err = PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	fragments = result.Plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].BorderBox.Y != 30 || fragments[1].BorderBox.Y != 21 || fragments[0].Node != 1 || fragments[1].Node != 2 {
		t.Fatalf("reverse column geometry/order = %+v", fragments)
	}
}

func TestRowColumnFlexConstraintsAndWorkRemainBounded(t *testing.T) {
	child := testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFlex, BasisKind: RowColumnFlexBasisFixed, Basis: 10, Grow: 1}, 0, 2, "")
	input := RowColumnPlanInput{PageSize: Size{Width: 20, Height: 10}, Region: Rect{Width: 20, Height: 10}, Direction: RowDirection, Children: []RowColumnChild{child}}
	if _, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{MaxChildren: 1, MaxWork: 2, MaxStateBytes: 2048}); !errors.Is(err, ErrRowColumnWorkLimit) {
		t.Fatalf("work error = %v", err)
	}
	input.Children[0].Track.Max = 5
	input.Children[0].Track.Min = 6
	if _, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{}); !errors.Is(err, ErrRowColumnTrack) {
		t.Fatalf("constraint error = %v", err)
	}
}

func TestRowColumnFlexFractionalFactorsContentBasisAndPercentageConstraints(t *testing.T) {
	input := RowColumnPlanInput{
		PageSize: Size{Width: 100, Height: 40}, Region: Rect{Width: 100, Height: 40}, Direction: RowDirection,
		Children: []RowColumnChild{
			testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFlex, BasisKind: RowColumnFlexBasisContent, GrowFactor: 500_000, ShrinkFactor: 500_000, MaxPercent: 30_000_000}, 0, 8, ""),
			testRowColumnChild(2, "@two", RowColumnTrack{Kind: RowColumnTrackFlex, BasisKind: RowColumnFlexBasisContent, GrowFactor: 1_500_000, ShrinkFactor: 1_500_000, MinPercent: 40_000_000}, 0, 8, ""),
		},
	}
	input.Children[0].ContentMain, input.Children[0].MinMain = 10, 5
	input.Children[1].ContentMain, input.Children[1].MinMain = 10, 5
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.MainSizes(), []Fixed{23, 77}; !fixedSlicesEqual(got, want) {
		t.Fatalf("fractional grow/content/percent sizes = %v, want %v", got, want)
	}

	input.Region.Width, input.PageSize.Width = 30*1024, 30*1024
	input.Children[0].Track.MaxPercent, input.Children[1].Track.MinPercent = 0, 0
	input.Children[0].ContentMain, input.Children[1].ContentMain = 20*1024, 20*1024
	result, err = PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.MainSizes(), []Fixed{17_920, 12_800}; !fixedSlicesEqual(got, want) {
		t.Fatalf("fractional scaled-shrink sizes = %v, want %v", got, want)
	}
}

func TestRowColumnFlexCrossAxisFixedAndPercentageBounds(t *testing.T) {
	input := RowColumnPlanInput{PageSize: Size{Width: 60, Height: 100}, Region: Rect{Width: 60, Height: 100}, Direction: ColumnDirection, Align: CrossStretch,
		Children: []RowColumnChild{testRowColumnChild(1, "@one", RowColumnTrack{Kind: RowColumnTrackFixed, Size: 20}, 0, 10, "")}}
	input.Children[0].CrossMinPercent = 50_000_000
	input.Children[0].CrossMax = 40
	result, err := PlanRowColumn(context.Background(), input, RowColumnPlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Plan.Projection().Fragments[0].BorderBox.Width; got != 40 {
		t.Fatalf("bounded stretch width = %d, want 40", got)
	}
}

func fixedSlicesEqual(left, right []Fixed) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
