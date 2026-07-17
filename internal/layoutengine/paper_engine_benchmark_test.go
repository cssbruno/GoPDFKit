// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"strconv"
	"testing"
)

var paperEngineTablePlanSink LayoutPlan

func TestPaperEngineTenThousandRowFixturePlansWithinBoundedWork(t *testing.T) {
	plan, err := PlanTable(context.Background(), paperEngineTableRowsFixture(10_000), TablePlanLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 100 || len(projection.Fragments) != 10_000 || len(projection.Breaks) != 99 {
		t.Fatalf("10k-row projection = %d pages, %d fragments, %d breaks",
			len(projection.Pages), len(projection.Fragments), len(projection.Breaks))
	}
}

// BenchmarkPaperEngineTableRows10000 isolates the unified table kernel's
// vertical occupancy and pagination cost. Text measurement is deliberately
// excluded; the typed end-to-end table cohorts measure that adapter cost.
func BenchmarkPaperEngineTableRows10000(b *testing.B) {
	input := paperEngineTableRowsFixture(10_000)
	if plan, err := PlanTable(context.Background(), input, TablePlanLimits{}); err != nil || len(plan.Projection().Pages) == 0 {
		b.Fatalf("validate 10k-row table = %d pages, %v", len(plan.Projection().Pages), err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
		if err != nil {
			b.Fatal(err)
		}
		paperEngineTablePlanSink = plan
	}
}

// BenchmarkPaperEngineTableContentSizedPremeasurement makes the cost of
// consuming premeasured intrinsic cell contributions visible independently
// from text shaping. The planner receives stable MinWidth/PreferredWidth
// values exactly as a frontend measurement pass would publish them.
func BenchmarkPaperEngineTableContentSizedPremeasurement(b *testing.B) {
	input := paperEngineTableRowsFixture(1_000)
	input.Columns = []TableColumn{{Kind: TableTrackIntrinsic}}
	for index := range input.Cells {
		input.Cells[index].MinWidth = Fixed(20 + index%17)
		input.Cells[index].PreferredWidth = Fixed(40 + index%29)
	}
	if plan, err := PlanTable(context.Background(), input, TablePlanLimits{}); err != nil || len(plan.Projection().Pages) == 0 {
		b.Fatalf("validate content-sized table = %d pages, %v", len(plan.Projection().Pages), err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plan, err := PlanTable(context.Background(), input, TablePlanLimits{})
		if err != nil {
			b.Fatal(err)
		}
		paperEngineTablePlanSink = plan
	}
}

func paperEngineTableRowsFixture(rows uint32) TablePlanInput {
	cells := make([]TableCell, rows)
	for row := uint32(0); row < rows; row++ {
		identity := "row-" + strconv.FormatUint(uint64(row), 10)
		cells[row] = TableCell{Node: NodeID(row + 1), Key: NodeKey(identity), Instance: InstanceID("table/" + identity),
			Row: row, RowSpan: 1, ColumnSpan: 1, MinWidth: 50, PreferredWidth: 50, MinHeight: 1}
	}
	return TablePlanInput{
		PageSize: Size{Width: 200, Height: 200}, Body: Rect{X: 10, Y: 10, Width: 100, Height: 100}, Width: 100,
		Rows: rows, Columns: []TableColumn{{Kind: TableTrackFixed, Width: 100}}, Cells: cells,
	}
}
