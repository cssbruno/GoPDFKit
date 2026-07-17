// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPlanGridContextEnforcesCancellationAndWorkLimits(t *testing.T) {
	input := testGridInput()
	input.Columns = []GridTrack{{Kind: GridTrackAuto}, {Kind: GridTrackAuto}}
	input.Rows = []GridTrack{{Kind: GridTrackAuto}}
	input.Items = []GridItem{testGridItem(1, "@one", 0, 0, 1, 1, fixedPoints(20), fixedPoints(10))}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PlanGridContext(canceled, input)
	var planning *PlanningError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticCanceled {
		t.Fatalf("canceled grid = %v", err)
	}

	input.Limits = DefaultGridPlanLimits()
	input.Limits.MaxWork = 1
	_, err = PlanGridContext(context.Background(), input)
	if !errors.Is(err, ErrGridWorkLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticWorkLimit {
		t.Fatalf("work-limited grid = %v", err)
	}

	input.Limits = GridPlanLimits{MaxColumns: 2}
	if _, err := PlanGridContext(context.Background(), input); err == nil {
		t.Fatal("grid accepted partially specified limits")
	}
}

func TestPlanGridConvenienceAndContextEntryPointsAreEquivalent(t *testing.T) {
	input := testGridInput()
	input.Columns = []GridTrack{{Kind: GridTrackAuto}, {Kind: GridTrackFraction, Weight: 2}}
	input.Rows = []GridTrack{{Kind: GridTrackAuto}}
	input.Items = []GridItem{testGridItem(1, "@one", 0, 0, 1, 2, fixedPoints(50), fixedPoints(10))}

	convenience, err := PlanGrid(input)
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := PlanGridContext(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	convenienceHash, err := convenience.Plan.Hash()
	if err != nil {
		t.Fatal(err)
	}
	boundedHash, err := bounded.Plan.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if convenienceHash != boundedHash || !reflect.DeepEqual(convenience.ColumnSizes(), bounded.ColumnSizes()) ||
		!reflect.DeepEqual(convenience.RowSizes(), bounded.RowSizes()) {
		t.Fatalf("entry points differ: %s != %s", convenienceHash, boundedHash)
	}
}

func TestPlanGridResolvesTracksSpansAndCanonicalPlacement(t *testing.T) {
	input := testGridInput()
	input.Columns = []GridTrack{{Kind: GridTrackAuto}, {Kind: GridTrackAuto}, {Kind: GridTrackAuto}}
	input.Rows = []GridTrack{{Kind: GridTrackAuto}}
	input.Items = []GridItem{
		testGridItem(2, "@last", 0, 2, 1, 1, fixedPoints(10), fixedPoints(10)),
		testGridItem(1, "@span", 0, 0, 1, 2, fixedPoints(50), fixedPoints(10)),
	}
	result, err := PlanGrid(input)
	if err != nil {
		t.Fatalf("PlanGrid() = %v", err)
	}
	if got, want := result.ColumnSizes(), []Fixed{fixedPoints(35), fixedPoints(35), fixedPoints(20)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %+v, want %+v", got, want)
	}
	if got, want := result.RowSizes(), []Fixed{fixedPoints(50)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	projection := result.Plan.Projection()
	if got, want := projection.GridTracks, []PlannedGridTrack{
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackColumn, Index: 0, Bounds: Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(35), Height: fixedPoints(50)}},
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackColumn, Index: 1, Bounds: Rect{X: fixedPoints(45), Y: fixedPoints(20), Width: fixedPoints(35), Height: fixedPoints(50)}},
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackColumn, Index: 2, Bounds: Rect{X: fixedPoints(80), Y: fixedPoints(20), Width: fixedPoints(20), Height: fixedPoints(50)}},
		{Group: 1, Page: 1, Region: RegionBody, Axis: GridTrackRow, Index: 0, Bounds: Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(90), Height: fixedPoints(50)}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained grid tracks = %+v, want %+v", got, want)
	}
	if len(projection.Fragments) != 2 || projection.Fragments[0].Key != "@span" || projection.Fragments[1].Key != "@last" {
		t.Fatalf("canonical fragments = %+v", projection.Fragments)
	}
	if got, want := projection.Fragments[0].BorderBox, (Rect{
		X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(70), Height: fixedPoints(50),
	}); got != want {
		t.Fatalf("spanning bounds = %+v, want %+v", got, want)
	}
	if got, want := projection.Fragments[1].BorderBox, (Rect{
		X: fixedPoints(80), Y: fixedPoints(20), Width: fixedPoints(20), Height: fixedPoints(50),
	}); got != want {
		t.Fatalf("last bounds = %+v, want %+v", got, want)
	}
}

func TestPlanGridFractionTracksFillWithDeterministicRemainder(t *testing.T) {
	input := testGridInput()
	input.Region.Width = 10
	input.Region.Height = 10
	input.Columns = []GridTrack{
		{Kind: GridTrackFraction, Weight: 1},
		{Kind: GridTrackFraction, Weight: 1},
		{Kind: GridTrackFraction, Weight: 1},
	}
	input.Rows = []GridTrack{{Kind: GridTrackFixed, Size: 10}}
	result, err := PlanGrid(input)
	if err != nil {
		t.Fatalf("PlanGrid() = %v", err)
	}
	if got, want := result.ColumnSizes(), []Fixed{4, 3, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fraction remainder = %+v, want %+v", got, want)
	}
	if result.UsedWidth != 10 || result.UsedHeight != 10 {
		t.Fatalf("used extent = %d x %d, want 10 x 10", result.UsedWidth, result.UsedHeight)
	}
}

func TestPlanGridFixedFractionAndAutoPolicy(t *testing.T) {
	input := testGridInput()
	input.Columns = []GridTrack{
		{Kind: GridTrackFixed, Size: fixedPoints(20)},
		{Kind: GridTrackAuto, Min: fixedPoints(10)},
		{Kind: GridTrackFraction, Min: fixedPoints(5), Weight: 2},
	}
	input.Rows = []GridTrack{{Kind: GridTrackAuto, Min: fixedPoints(10)}, {Kind: GridTrackAuto, Min: fixedPoints(10)}}
	input.ColumnGap = fixedPoints(2)
	input.RowGap = fixedPoints(2)
	result, err := PlanGrid(input)
	if err != nil {
		t.Fatalf("PlanGrid() = %v", err)
	}
	if got, want := result.ColumnSizes(), []Fixed{fixedPoints(20), fixedPoints(10), fixedPoints(56)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("columns = %+v, want %+v", got, want)
	}
	if got, want := result.RowSizes(), []Fixed{fixedPoints(24), fixedPoints(24)}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
}

func TestPlanGridRejectsOverlapOverflowAndLimits(t *testing.T) {
	input := testGridInput()
	input.Columns = []GridTrack{{Kind: GridTrackFixed, Size: fixedPoints(40)}, {Kind: GridTrackFixed, Size: fixedPoints(40)}}
	input.Rows = []GridTrack{{Kind: GridTrackFixed, Size: fixedPoints(20)}}
	input.Items = []GridItem{
		testGridItem(1, "@one", 0, 0, 1, 2, 0, 0),
		testGridItem(2, "@two", 0, 1, 1, 1, 0, 0),
	}
	if _, err := PlanGrid(input); !errors.Is(err, ErrGridOverlap) {
		t.Fatalf("overlap = %v, want ErrGridOverlap", err)
	}
	input.Items = []GridItem{testGridItem(1, "@wide", 0, 0, 1, 2, fixedPoints(90), 0)}
	if _, err := PlanGrid(input); !errors.Is(err, ErrGridIntrinsicOverflow) {
		t.Fatalf("fixed intrinsic overflow = %v, want ErrGridIntrinsicOverflow", err)
	}
	input.Items = nil
	input.Limits = GridPlanLimits{MaxColumns: 1, MaxRows: 1, MaxItems: 1, MaxCells: 1, MaxWork: 100}
	if _, err := PlanGrid(input); !errors.Is(err, ErrGridLimits) {
		t.Fatalf("limit = %v, want ErrGridLimits", err)
	}
	input.Limits = GridPlanLimits{MaxColumns: 1 << 17, MaxRows: 1, MaxItems: 1, MaxCells: 1, MaxWork: 100}
	if _, err := PlanGrid(input); err == nil {
		t.Fatal("grid accepted limits above the hard caps")
	}
}

func testGridInput() GridPlanInput {
	return GridPlanInput{
		PageSize: Size{Width: fixedPoints(120), Height: fixedPoints(100)},
		Region:   Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(90), Height: fixedPoints(50)},
	}
}

func testGridItem(node NodeID, key NodeKey, row, column, rowSpan, columnSpan uint32, minWidth, minHeight Fixed) GridItem {
	return GridItem{
		Node: node, Key: key, Instance: InstanceID(key), Row: row, Column: column,
		RowSpan: rowSpan, ColumnSpan: columnSpan, MinWidth: minWidth, MinHeight: minHeight,
	}
}
