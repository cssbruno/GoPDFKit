// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestPlanTableResolvesTracksDeterministically(t *testing.T) {
	input := tableTestInput(2, 3, 60, 100)
	input.Columns = []TableColumn{
		{Kind: TableTrackIntrinsic},
		{Kind: TableTrackFixed, Width: 20},
		{Kind: TableTrackIntrinsic},
	}
	input.Cells = []TableCell{
		tableTestCell(4, 1, 0, 1, 3, 3, 60, 60, 21),
		tableTestCell(2, 0, 1, 1, 1, 1, 10, 10, 8),
		tableTestCell(1, 0, 0, 1, 1, 1, 10, 20, 8),
		tableTestCell(3, 0, 2, 1, 1, 1, 10, 10, 8),
	}

	plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatalf("PlanTable: %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 || len(projection.Fragments) != len(input.Cells) {
		t.Fatalf("unexpected projection sizes: pages=%d fragments=%d", len(projection.Pages), len(projection.Fragments))
	}
	if len(projection.GridTracks) != 8 {
		t.Fatalf("retained table tracks = %d, want two 3-column/1-row groups", len(projection.GridTracks))
	}
	for group := uint32(1); group <= 2; group++ {
		base := int((group - 1) * 4)
		wantHeight := Fixed(8)
		if group == 2 {
			wantHeight = 21
		}
		for index, track := range projection.GridTracks[base : base+3] {
			if track.Group != group || track.Axis != GridTrackColumn || track.Index != uint32(index) || track.Bounds.Width != 20 || track.Bounds.Height != wantHeight {
				t.Fatalf("group %d column track[%d] = %+v", group, index, track)
			}
		}
		row := projection.GridTracks[base+3]
		if row.Group != group || row.Axis != GridTrackRow || row.Index != 0 || row.Bounds.Height != wantHeight {
			t.Fatalf("group %d row track = %+v", group, row)
		}
	}

	// Canonical row/column order is independent of the shuffled input. The
	// spanning row establishes exact 20/20/20 tracks.
	wantWidths := []Fixed{20, 20, 20, 60}
	gotWidths := make([]Fixed, len(projection.Fragments))
	for i, fragment := range projection.Fragments {
		gotWidths[i] = fragment.BorderBox.Width
	}
	if !slices.Equal(gotWidths, wantWidths) {
		t.Fatalf("fragment widths = %v, want %v", gotWidths, wantWidths)
	}
	if projection.Fragments[3].BorderBox.Height != 21 {
		t.Fatalf("spanning cell height = %d, want 21", projection.Fragments[3].BorderBox.Height)
	}
	// Reordering cells cannot change the canonical plan.
	slices.Reverse(input.Cells)
	again, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatalf("PlanTable reversed: %v", err)
	}
	firstHash, _ := plan.Hash()
	secondHash, _ := again.Hash()
	if firstHash != secondHash {
		t.Fatalf("input order changed plan hash: %s != %s", firstHash, secondHash)
	}
}

func TestPlanTablePaginatesGroupsAndRepeatsHeaders(t *testing.T) {
	input := tableTestInput(4, 2, 40, 25)
	input.HeaderRows = 1
	input.Cells = []TableCell{
		tableTestCell(1, 0, 0, 1, 1, 1, 5, 5, 5),
		tableTestCell(2, 0, 1, 1, 1, 1, 5, 5, 5),
		tableTestCell(3, 1, 0, 1, 1, 1, 5, 5, 10),
		tableTestCell(4, 1, 1, 1, 1, 1, 5, 5, 10),
		tableTestCell(5, 2, 0, 1, 1, 1, 5, 5, 10),
		tableTestCell(6, 2, 1, 1, 1, 1, 5, 5, 10),
		tableTestCell(7, 3, 0, 1, 1, 1, 5, 5, 10),
		tableTestCell(8, 3, 1, 1, 1, 1, 5, 5, 10),
	}

	plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatalf("PlanTable: %v", err)
	}
	p := plan.Projection()
	if len(p.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(p.Pages))
	}
	if p.Pages[0].Fragments.Count != 6 || p.Pages[1].Fragments.Count != 4 {
		t.Fatalf("page fragment counts = %d,%d, want 6,4", p.Pages[0].Fragments.Count, p.Pages[1].Fragments.Count)
	}
	if len(p.Breaks) != 1 {
		t.Fatalf("breaks = %d, want 1", len(p.Breaks))
	}
	decision := p.Breaks[0]
	if decision.Reason != BreakPaginationConstraint || decision.Required != 10 || decision.Available != 0 || decision.FromPage != 1 || decision.ToPage != 2 {
		t.Fatalf("unexpected break decision: %+v", decision)
	}
	// The second-page header is a new fragment of the same semantic nodes.
	if p.Fragments[0].Repeated || !p.Fragments[6].Repeated || p.Fragments[6].Node != p.Fragments[0].Node || p.Fragments[6].Page != 2 || p.Fragments[6].BorderBox.Y != input.Body.Y {
		t.Fatalf("header was not repeated canonically: first=%+v repeat=%+v", p.Fragments[0], p.Fragments[6])
	}
	if len(p.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", p.Diagnostics)
	}
	var pageTwoTracks int
	for _, track := range p.GridTracks {
		if track.Page == 2 {
			pageTwoTracks++
		}
	}
	if pageTwoTracks != 6 {
		t.Fatalf("page-two repeated-header/body tracks = %d, want 6", pageTwoTracks)
	}
}

func TestPlanTablePaginationPoliciesJoinRowsAndRetainOrphansWidows(t *testing.T) {
	input := tableTestInput(5, 1, 40, 25)
	for row := uint32(0); row < input.Rows; row++ {
		input.Cells = append(input.Cells, tableTestCell(NodeID(row+1), row, 0, 1, 1, int(row+1), 5, 5, 10))
	}
	input.Orphans, input.Widows = 2, 2
	plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 3 || projection.Pages[0].Fragments.Count != 2 || projection.Pages[1].Fragments.Count != 1 || projection.Pages[2].Fragments.Count != 2 {
		t.Fatalf("orphan/widow pages = %+v", projection.Pages)
	}
	if len(projection.Breaks) != 2 || projection.Breaks[0].Required != 10 || projection.Breaks[0].Available != 5 || projection.Breaks[1].Required != 20 || projection.Breaks[1].Available != 15 {
		t.Fatalf("orphan/widow causal breaks = %+v", projection.Breaks)
	}

	input.Orphans, input.Widows = 0, 0
	input.RowKeepWithNext = []bool{false, true, false, false, false}
	plan, err = PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection = plan.Projection()
	if len(projection.Pages) != 3 || projection.Pages[1].Fragments.Count != 2 || projection.Breaks[0].Required != 20 || projection.Breaks[0].Available != 15 {
		t.Fatalf("row keep-with-next plan = pages %+v breaks %+v", projection.Pages, projection.Breaks)
	}
}

func TestPlanTableKeepTogetherRelaxesOnceWhenLargerThanPage(t *testing.T) {
	input := tableTestInput(3, 1, 40, 20)
	input.KeepTogether = true
	for row := uint32(0); row < input.Rows; row++ {
		input.Cells = append(input.Cells, tableTestCell(NodeID(row+1), row, 0, 1, 1, int(row+1), 5, 5, 10))
	}
	plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 || projection.Pages[0].Fragments.Count != 3 || len(projection.Diagnostics) != 2 {
		t.Fatalf("relaxed keep projection = pages %+v diagnostics %+v", projection.Pages, projection.Diagnostics)
	}
	policy := ""
	for _, evidence := range projection.Diagnostics[1].Evidence {
		if evidence.Key == "policy" {
			policy = evidence.Value
		}
	}
	if projection.Diagnostics[0].Code != DiagnosticUnbreakableTooTall || projection.Diagnostics[1].Code != DiagnosticKeepTooLarge || policy != "table_keep_together" {
		t.Fatalf("relaxed keep diagnostics = %+v", projection.Diagnostics)
	}
}

func TestPlanTableSelectsAndValidatesPerPageBodyRegions(t *testing.T) {
	input := tableTestInput(5, 2, 40, 25)
	input.HeaderRows = 1
	input.Cells = make([]TableCell, 0, 10)
	for row := uint32(0); row < 5; row++ {
		for column := uint32(0); column < 2; column++ {
			input.Cells = append(input.Cells, tableTestCell(NodeID(len(input.Cells)+1), row, column, 1, 1, 1, 5, 5, 10))
		}
	}
	input.BodyForPage = func(page uint32) (Rect, error) {
		if page == 1 {
			return Rect{X: input.Body.X, Y: 10, Width: input.Body.Width, Height: 25}, nil
		}
		return Rect{X: input.Body.X, Y: 3, Width: input.Body.Width, Height: 35}, nil
	}
	plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 3 || len(projection.Breaks) != 2 || projection.Breaks[0].Reason != BreakPaginationConstraint {
		t.Fatalf("pages/breaks = %d/%+v", len(projection.Pages), projection.Breaks)
	}
	if projection.Fragments[0].BorderBox.Y != 10 {
		t.Fatalf("first body y = %d, want 10", projection.Fragments[0].BorderBox.Y)
	}
	secondStart := projection.Pages[1].Fragments.Start
	if projection.Fragments[secondStart].BorderBox.Y != 3 || !projection.Fragments[secondStart].Repeated {
		t.Fatalf("second-page repeated header = %+v", projection.Fragments[secondStart])
	}

	input.BodyForPage = func(uint32) (Rect, error) {
		return Rect{X: input.Body.X + 1, Y: 0, Width: input.Body.Width, Height: 20}, nil
	}
	if _, err := PlanTable(context.Background(), input, TablePlanLimits{}); !errors.Is(err, ErrTableDimensionsInvalid) {
		t.Fatalf("invalid selected body error = %v", err)
	}
}

func TestRepeatedTableHeaderGeometryIsPlannedOncePerResolvedWidth(t *testing.T) {
	input := tableTestInput(5, 2, 40, 25)
	input.HeaderRows = 1
	input.Cells = make([]TableCell, 0, 10)
	for row := uint32(0); row < 5; row++ {
		for column := uint32(0); column < 2; column++ {
			input.Cells = append(input.Cells, tableTestCell(NodeID(len(input.Cells)+1), row, column, 1, 1, 1, 5, 5, 10))
		}
	}
	input.BodyForPage = func(page uint32) (Rect, error) {
		return Rect{X: input.Body.X, Y: Fixed(page), Width: input.Body.Width, Height: input.Body.Height}, nil
	}
	limits := DefaultTablePlanLimits()
	budget := tableBudget{ctx: context.Background(), limit: limits.MaxWork}
	resolved, err := resolveTable(input, limits, &budget)
	if err != nil {
		t.Fatal(err)
	}
	planner := tablePaginator{input: input, limits: limits, budget: &budget, cells: resolved.cells,
		columns: resolved.columnWidths, rows: resolved.rowHeights, groups: resolved.bodyGroups}
	plan, err := planner.plan()
	if err != nil {
		t.Fatal(err)
	}
	pages := len(plan.Projection().Pages)
	if pages < 2 || planner.headerGeometryPlans != 1 || !planner.headerGeometryReady {
		t.Fatalf("pages/header geometry plans = %d/%d (%+v)", pages, planner.headerGeometryPlans, planner.headerGeometry)
	}
}

func TestPlanTableEmitsOversizeRowspanGroupOnceAndAdvancesAfterOverflow(t *testing.T) {
	input := tableTestInput(4, 2, 40, 20)
	input.HeaderRows = 1
	input.Cells = []TableCell{
		tableTestCell(1, 0, 0, 1, 1, 1, 5, 5, 5),
		tableTestCell(2, 0, 1, 1, 1, 1, 5, 5, 5),
		tableTestCell(3, 1, 0, 2, 1, 1, 5, 5, 21),
		tableTestCell(4, 1, 1, 1, 1, 1, 5, 5, 8),
		tableTestCell(5, 2, 1, 1, 1, 1, 5, 5, 8),
		tableTestCell(6, 3, 0, 1, 1, 1, 5, 5, 8),
		tableTestCell(7, 3, 1, 1, 1, 1, 5, 5, 8),
	}

	plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
	if err != nil {
		t.Fatalf("PlanTable: %v", err)
	}
	p := plan.Projection()
	if len(p.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(p.Pages))
	}
	if len(p.Breaks) != 1 || p.Breaks[0].Reason != BreakPreviousFragmentOverflow || p.Breaks[0].Available != 0 || p.Breaks[0].Required != 8 {
		t.Fatalf("unexpected post-overflow break: %+v", p.Breaks)
	}
	if len(p.Diagnostics) != 1 || p.Diagnostics[0].Code != DiagnosticTableRowspanCrossesPage {
		t.Fatalf("unexpected diagnostics: %+v", p.Diagnostics)
	}
	if p.Fragments[3].BorderBox.Height != 11 || p.Fragments[4].BorderBox.Height != 10 {
		t.Fatalf("rowspan deficit distribution = %d,%d, want 11,10", p.Fragments[3].BorderBox.Height, p.Fragments[4].BorderBox.Height)
	}
	if p.Diagnostics[0].Evidence[2] != (DiagnosticEvidence{Key: "group_height_fixed", Value: "21"}) {
		t.Fatalf("oversize evidence is not exact: %+v", p.Diagnostics[0].Evidence)
	}
	// Two header cells + three cells in the rowspan group. It was emitted once.
	if p.Pages[0].Fragments.Count != 5 || p.Pages[1].Fragments.Count != 4 {
		t.Fatalf("fragment counts = %d,%d, want 5,4", p.Pages[0].Fragments.Count, p.Pages[1].Fragments.Count)
	}
}

func TestPlanTableRejectsInvalidOccupancyAndTracks(t *testing.T) {
	base := tableTestInput(1, 2, 40, 100)
	base.Cells = []TableCell{
		tableTestCell(1, 0, 0, 1, 1, 1, 10, 10, 10),
		tableTestCell(2, 0, 1, 1, 1, 1, 10, 10, 10),
	}
	tests := []struct {
		name string
		edit func(*TablePlanInput)
		want error
	}{
		{"zero span", func(in *TablePlanInput) { in.Cells[0].RowSpan = 0 }, ErrTableSpanInvalid},
		{"out of range", func(in *TablePlanInput) { in.Cells[1].ColumnSpan = 2 }, ErrTableSpanInvalid},
		{"overlap", func(in *TablePlanInput) { in.Cells[1].Column = 0 }, ErrTableSpanInvalid},
		{"hole", func(in *TablePlanInput) { in.Cells = in.Cells[:1] }, ErrTableOccupancyInvalid},
		{"fixed span too narrow", func(in *TablePlanInput) {
			in.Columns = []TableColumn{{Kind: TableTrackFixed, Width: 20}, {Kind: TableTrackIntrinsic}}
			in.Cells[0].MinWidth = 21
			in.Cells[0].PreferredWidth = 21
		}, ErrTableTrackOverflow},
		{"all fixed unresolved", func(in *TablePlanInput) {
			in.Columns = []TableColumn{{Kind: TableTrackFixed, Width: 10}, {Kind: TableTrackFixed, Width: 10}}
		}, ErrTableTrackUnresolved},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.Columns = append([]TableColumn(nil), base.Columns...)
			input.Cells = append([]TableCell(nil), base.Cells...)
			test.edit(&input)
			_, err := PlanTable(context.Background(), input, TablePlanLimits{})
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want errors.Is(_, %v)", err, test.want)
			}
			var planning *PlanningError
			if !errors.As(err, &planning) || planning.Diagnostic.Code == "" {
				t.Fatalf("error has no structured diagnostic: %v", err)
			}
		})
	}
}

func TestPlanTableHonorsCancellationAndWorkLimits(t *testing.T) {
	input := tableTestInput(1, 1, 20, 100)
	input.Cells = []TableCell{tableTestCell(1, 0, 0, 1, 1, 1, 1, 1, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PlanTable(ctx, input, TablePlanLimits{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticCanceled {
		t.Fatalf("canceled diagnostic = %+v", planning)
	}

	limits := DefaultTablePlanLimits()
	limits.MaxWork = 1
	_, err = PlanTable(context.Background(), input, limits)
	if !errors.Is(err, ErrTableWorkLimit) {
		t.Fatalf("work-limit error = %v", err)
	}
}

func TestResolveTableColumnWidthsHonorsBoundsSpansAndCanonicalRemainders(t *testing.T) {
	columns := []TableColumn{
		{Kind: TableTrackFixed, Width: 20, MinWidth: 10, MaxWidth: 20},
		{Kind: TableTrackIntrinsic, MinWidth: 10, MaxWidth: 30},
		{Kind: TableTrackIntrinsic, MinWidth: 5},
	}
	cells := []TableCell{
		{Row: 1, Column: 1, ColumnSpan: 2, MinWidth: 50, PreferredWidth: 70},
		{Row: 0, Column: 0, ColumnSpan: 1, MinWidth: 20, PreferredWidth: 20},
	}
	widths, err := ResolveTableColumnWidths(context.Background(), 100, columns, cells, TablePlanLimits{})
	if err != nil || !slices.Equal(widths, []Fixed{20, 30, 50}) {
		t.Fatalf("ResolveTableColumnWidths() = %v, %v", widths, err)
	}
	slices.Reverse(cells)
	again, err := ResolveTableColumnWidths(context.Background(), 100, columns, cells, TablePlanLimits{})
	if err != nil || !slices.Equal(widths, again) {
		t.Fatalf("reordered cells changed widths: %v / %v, %v", widths, again, err)
	}

	remainder, err := ResolveTableColumnWidths(context.Background(), 10, []TableColumn{
		{Kind: TableTrackIntrinsic}, {Kind: TableTrackIntrinsic}, {Kind: TableTrackIntrinsic},
	}, nil, TablePlanLimits{})
	if err != nil || !slices.Equal(remainder, []Fixed{4, 3, 3}) {
		t.Fatalf("canonical remainder = %v, %v", remainder, err)
	}
}

func TestResolveTableColumnWidthsRejectsImpossibleBoundsCancellationAndWork(t *testing.T) {
	columns := []TableColumn{{Kind: TableTrackIntrinsic, MinWidth: 10, MaxWidth: 20}, {Kind: TableTrackIntrinsic, MinWidth: 10, MaxWidth: 20}}
	if _, err := ResolveTableColumnWidths(context.Background(), 50, columns, nil, TablePlanLimits{}); !errors.Is(err, ErrTableTrackUnresolved) {
		t.Fatalf("maximum underfill error = %v", err)
	}
	cells := []TableCell{{Column: 0, ColumnSpan: 2, MinWidth: 45, PreferredWidth: 45}}
	if _, err := ResolveTableColumnWidths(context.Background(), 40, columns, cells, TablePlanLimits{}); !errors.Is(err, ErrTableTrackOverflow) {
		t.Fatalf("spanning minimum overflow error = %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ResolveTableColumnWidths(canceled, 40, columns, nil, TablePlanLimits{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled resolution error = %v", err)
	}
	limits := DefaultTablePlanLimits()
	limits.MaxWork = 1
	if _, err := ResolveTableColumnWidths(context.Background(), 40, columns, nil, limits); !errors.Is(err, ErrTableWorkLimit) {
		t.Fatalf("work limit error = %v", err)
	}
}

func TestPlanTableRejectsCallerLimitsAboveHardCaps(t *testing.T) {
	input := tableTestInput(1, 1, 20, 100)
	input.Cells = []TableCell{tableTestCell(1, 0, 0, 1, 1, 1, 1, 1, 1)}
	limits := DefaultTablePlanLimits()
	limits.MaxOccupancySlots++
	_, err := PlanTable(context.Background(), input, limits)
	if !errors.Is(err, ErrTableLimitsInvalid) {
		t.Fatalf("oversized caller limit error = %v, want %v", err, ErrTableLimitsInvalid)
	}

	limits = DefaultTablePlanLimits()
	limits.MaxRows = 0
	_, err = PlanTable(context.Background(), input, limits)
	if !errors.Is(err, ErrTableLimitsInvalid) {
		t.Fatalf("zero caller limit error = %v, want %v", err, ErrTableLimitsInvalid)
	}
}

func tableTestInput(rows, columns uint32, width, bodyHeight Fixed) TablePlanInput {
	tracks := make([]TableColumn, columns)
	for i := range tracks {
		tracks[i] = TableColumn{Kind: TableTrackIntrinsic}
	}
	return TablePlanInput{
		PageSize: Size{Width: 200, Height: 200},
		Body:     Rect{X: 10, Y: 8, Width: 100, Height: bodyHeight},
		Width:    width,
		Rows:     rows,
		Columns:  tracks,
	}
}

func tableTestCell(node NodeID, row, column, rowSpan, columnSpan uint32, instance int, minWidth, preferredWidth, minHeight Fixed) TableCell {
	return TableCell{
		Node: node, Key: NodeKey("cell-" + string(rune('a'+node-1))), Instance: InstanceID("table/cell-" + string(rune('a'+instance-1))),
		Row: row, Column: column, RowSpan: rowSpan, ColumnSpan: columnSpan,
		MinWidth: minWidth, PreferredWidth: preferredWidth, MinHeight: minHeight,
	}
}
