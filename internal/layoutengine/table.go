// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

var (
	ErrTableDimensionsInvalid = errors.New("layoutengine: table dimensions are invalid")
	ErrTableSpanInvalid       = errors.New("layoutengine: table span is invalid")
	ErrTableOccupancyInvalid  = errors.New("layoutengine: table occupancy is not rectangular")
	ErrTableTrackOverflow     = errors.New("layoutengine: table track minimums exceed the available width")
	ErrTableTrackUnresolved   = errors.New("layoutengine: fixed table tracks do not resolve to the table width")
	ErrTableWorkLimit         = errors.New("layoutengine: table planning work limit exceeded")
	ErrTablePageLimit         = errors.New("layoutengine: table planning page limit exceeded")
	ErrTableLimitsInvalid     = errors.New("layoutengine: table planning limits are invalid")
)

const (
	hardMaxTableRows           uint32 = 100_000
	hardMaxTableColumns        uint32 = 1_024
	hardMaxTableCells          uint32 = 1_000_000
	hardMaxTableOccupancySlots uint64 = 2_000_000
	hardMaxTablePages          uint32 = 100_000
	hardMaxTableWork           uint64 = 20_000_000
)

// TableTrackKind distinguishes exact author-supplied tracks from intrinsic
// tracks. Fixed tracks never grow; intrinsic tracks receive cell minimums,
// preferred contributions, and any remaining table width.
type TableTrackKind string

const (
	TableTrackFixed     TableTrackKind = "fixed"
	TableTrackIntrinsic TableTrackKind = "intrinsic"
)

// TableColumn is one column sizing constraint. Width must be positive for a
// fixed track and zero for an intrinsic track.
type TableColumn struct {
	Kind     TableTrackKind
	Width    Fixed
	MinWidth Fixed
	MaxWidth Fixed
}

// TableCell is an already-measured table cell. Row and Column are zero-based.
// MinWidth and PreferredWidth are intrinsic border-box contributions;
// PreferredWidth must be at least MinWidth. MinHeight is the cell's measured
// border-box height before rowspan deficit distribution.
type TableCell struct {
	Node           NodeID
	Key            NodeKey
	Instance       InstanceID
	Source         SourceSpan
	Row            uint32
	Column         uint32
	RowSpan        uint32
	ColumnSpan     uint32
	MinWidth       Fixed
	PreferredWidth Fixed
	MinHeight      Fixed
}

// TableCaption is an already-measured caption emitted once before the first
// page's header rows. It deliberately does not participate in the table grid.
type TableCaption struct {
	Node      NodeID
	Key       NodeKey
	Instance  InstanceID
	Source    SourceSpan
	MinHeight Fixed
}

// TablePlanInput contains only fixed geometry and measured cell constraints.
// The planner does not shape text, consult a painter, or call a legacy layout
// engine. Every grid slot must be occupied by exactly one cell after spans are
// expanded. Header rows repeat as one indivisible unit on continuation pages.
type TablePlanInput struct {
	PageSize   Size
	Body       Rect
	Width      Fixed
	Rows       uint32
	Columns    []TableColumn
	HeaderRows uint32
	Cells      []TableCell
	Caption    *TableCaption
	// KeepTogether makes the complete non-header row sequence one preferred
	// pagination unit. RowKeepWithNext joins authored row boundaries before
	// pagination. Orphans and Widows are minimum body-row counts retained at
	// the first and last table-page boundaries when representable.
	KeepTogether    bool
	KeepWithNext    bool
	Orphans         uint32
	Widows          uint32
	RowKeepWithNext []bool
	// BodyForPage optionally selects the body region for a one-based page.
	// Returned regions must preserve Body.X and Body.Width, remain within the
	// page, and have positive height. Selection is invoked only during planning.
	BodyForPage func(page uint32) (Rect, error)
}

// TablePlanLimits bounds both retained state and planner work. A zero-value
// limits object selects DefaultTablePlanLimits; partially zero limits are
// rejected so callers cannot accidentally disable a bound.
type TablePlanLimits struct {
	MaxRows           uint32
	MaxColumns        uint32
	MaxCells          uint32
	MaxOccupancySlots uint64
	MaxPages          uint32
	MaxWork           uint64
}

// DefaultTablePlanLimits returns a bounded policy by value.
func DefaultTablePlanLimits() TablePlanLimits {
	return TablePlanLimits{
		MaxRows:           hardMaxTableRows,
		MaxColumns:        hardMaxTableColumns,
		MaxCells:          hardMaxTableCells,
		MaxOccupancySlots: hardMaxTableOccupancySlots,
		MaxPages:          hardMaxTablePages,
		MaxWork:           hardMaxTableWork,
	}
}

type tableBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (b *tableBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(b.ctx, "table planning", amount); err != nil {
		return err
	}
	if err := b.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{
			Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout,
			Message: "table planning was canceled",
		})
	}
	if amount > b.limit-b.used {
		return newPlanningError(ErrTableWorkLimit, Diagnostic{
			Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout,
			Message: "table planning exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{
				{Key: "work_limit", Value: strconv.FormatUint(b.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(b.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)},
			},
		})
	}
	b.used += amount
	return nil
}

type tableResolved struct {
	cells        []TableCell
	columnWidths []Fixed
	rowHeights   []Fixed
	bodyGroups   []tableRowGroup
}

type tableRowGroup struct {
	start  uint32
	end    uint32 // exclusive
	height Fixed
	policy string
}

type tableUnitGeometry struct {
	rowOffsets    []Fixed
	columnOffsets []Fixed
	height        Fixed
}

// PlanTable builds a canonical geometry-only LayoutPlan. Cells are emitted in
// row/column order, independent of input slice order. Rowspans derive the
// smallest indivisible body row groups. A group that cannot fit below repeated
// headers is emitted exactly once with a warning rather than retried forever.
func PlanTable(ctx context.Context, input TablePlanInput, limits TablePlanLimits) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeTableLimits(limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	budget := tableBudget{ctx: ctx, limit: limits.MaxWork}
	resolved, err := resolveTable(input, limits, &budget)
	if err != nil {
		return LayoutPlan{}, err
	}
	planner := tablePaginator{
		input: input, limits: limits, budget: &budget,
		cells: resolved.cells, columns: resolved.columnWidths, rows: resolved.rowHeights,
		groups: resolved.bodyGroups,
	}
	return planner.plan()
}

func normalizeTableLimits(limits TablePlanLimits) (TablePlanLimits, error) {
	if limits == (TablePlanLimits{}) {
		return DefaultTablePlanLimits(), nil
	}
	if limits.MaxRows == 0 || limits.MaxColumns == 0 || limits.MaxCells == 0 ||
		limits.MaxOccupancySlots == 0 || limits.MaxPages == 0 || limits.MaxWork == 0 {
		return TablePlanLimits{}, fmt.Errorf("%w: all bounds must be positive", ErrTableLimitsInvalid)
	}
	if limits.MaxRows > hardMaxTableRows || limits.MaxColumns > hardMaxTableColumns ||
		limits.MaxCells > hardMaxTableCells || limits.MaxOccupancySlots > hardMaxTableOccupancySlots ||
		limits.MaxPages > hardMaxTablePages || limits.MaxWork > hardMaxTableWork {
		return TablePlanLimits{}, fmt.Errorf("%w: caller bounds exceed the implementation hard caps", ErrTableLimitsInvalid)
	}
	return limits, nil
}

func resolveTable(input TablePlanInput, limits TablePlanLimits, budget *tableBudget) (tableResolved, error) {
	if err := validateTableSurface(input); err != nil {
		return tableResolved{}, err
	}
	columns := uint32(len(input.Columns)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if input.Rows > limits.MaxRows || columns > limits.MaxColumns || uint64(len(input.Cells)) > uint64(limits.MaxCells) {
		return tableResolved{}, tableResourceLimit("table dimensions exceed retained-state limits", limits, input)
	}
	slots := uint64(input.Rows) * uint64(columns)
	// The hard retained-state cap is deliberately far below MaxInt on every
	// supported Go target. Keep the platform check adjacent to the conversion
	// so a future cap increase cannot make uint64-to-int truncation possible.
	maxInt := uint64(^uint(0) >> 1)
	if slots > limits.MaxOccupancySlots || slots > maxInt {
		return tableResolved{}, tableResourceLimit("table occupancy grid exceeds its retained-state limit", limits, input)
	}
	if err := budget.charge(uint64(len(input.Columns)) + uint64(len(input.Cells))); err != nil {
		return tableResolved{}, err
	}

	cells := append([]TableCell(nil), input.Cells...)
	sort.Slice(cells, func(i, j int) bool {
		if cells[i].Row != cells[j].Row {
			return cells[i].Row < cells[j].Row
		}
		return cells[i].Column < cells[j].Column
	})
	occupancy := make([]int32, int(slots))
	for i := range occupancy {
		occupancy[i] = -1
	}
	for index, cell := range cells {
		if err := validateTableCell(cell, input.Rows, columns, input.HeaderRows); err != nil {
			return tableResolved{}, err
		}
		area := uint64(cell.RowSpan) * uint64(cell.ColumnSpan)
		if err := budget.charge(area); err != nil {
			return tableResolved{}, err
		}
		for row := cell.Row; row < cell.Row+cell.RowSpan; row++ {
			base := uint64(row) * uint64(columns)
			for column := cell.Column; column < cell.Column+cell.ColumnSpan; column++ {
				slot := int(base + uint64(column)) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
				if occupancy[slot] >= 0 {
					return tableResolved{}, tableSpanError(cell, "table cells overlap after expanding row and column spans")
				}
				occupancy[slot] = int32(index)
			}
		}
	}
	for slot, owner := range occupancy {
		if owner < 0 {
			row := uint32(slot) / columns
			column := uint32(slot) % columns
			return tableResolved{}, newPlanningError(ErrTableOccupancyInvalid, Diagnostic{
				Code: DiagnosticTableSpanInvalid, Severity: SeverityError, Stage: StageLayout,
				Message: "table grid contains an unoccupied slot",
				Evidence: []DiagnosticEvidence{
					{Key: "row", Value: strconv.FormatUint(uint64(row), 10)},
					{Key: "column", Value: strconv.FormatUint(uint64(column), 10)},
				},
			})
		}
	}

	widths, err := resolveColumnWidths(input, cells, budget)
	if err != nil {
		return tableResolved{}, err
	}
	heights, err := resolveRowHeights(input.Rows, cells, budget)
	if err != nil {
		return tableResolved{}, err
	}
	groups, err := deriveBodyGroups(input.HeaderRows, input.Rows, cells, heights, input.RowKeepWithNext, input.KeepTogether, input.Orphans, input.Widows, budget)
	if err != nil {
		return tableResolved{}, err
	}
	return tableResolved{cells: cells, columnWidths: widths, rowHeights: heights, bodyGroups: groups}, nil
}

func validateTableSurface(input TablePlanInput) error {
	if err := input.PageSize.Validate(); err != nil || input.PageSize.IsEmpty() {
		return fmt.Errorf("%w: page size must have positive valid extents", ErrTableDimensionsInvalid)
	}
	if err := input.Body.Validate(); err != nil || input.Body.IsEmpty() {
		return fmt.Errorf("%w: body must have positive valid extents", ErrTableDimensionsInvalid)
	}
	if input.Body.X < 0 || input.Body.Y < 0 {
		return fmt.Errorf("%w: body lies outside the page", ErrTableDimensionsInvalid)
	}
	right, err := input.Body.Right()
	if err != nil || right > input.PageSize.Width {
		return fmt.Errorf("%w: body lies outside the page", ErrTableDimensionsInvalid)
	}
	bottom, err := input.Body.Bottom()
	if err != nil || bottom > input.PageSize.Height {
		return fmt.Errorf("%w: body lies outside the page", ErrTableDimensionsInvalid)
	}
	if input.Rows == 0 || len(input.Columns) == 0 || input.HeaderRows > input.Rows || input.Width <= 0 || input.Width > input.Body.Width {
		return ErrTableDimensionsInvalid
	}
	if len(input.Cells) == 0 {
		return ErrTableOccupancyInvalid
	}
	if input.Caption != nil && (!input.Caption.Node.Valid() || input.Caption.Key == "" || input.Caption.Instance == "" || input.Caption.MinHeight <= 0) {
		return fmt.Errorf("%w: caption identity and height must be valid", ErrTableDimensionsInvalid)
	}
	if len(input.RowKeepWithNext) != 0 && len(input.RowKeepWithNext) != int(input.Rows) {
		return fmt.Errorf("%w: row keep policy count must equal the declared row count", ErrTableDimensionsInvalid)
	}
	if input.Orphans > input.Rows-input.HeaderRows || input.Widows > input.Rows-input.HeaderRows {
		return fmt.Errorf("%w: table widow/orphan counts exceed body rows", ErrTableDimensionsInvalid)
	}
	for index, column := range input.Columns {
		if column.MinWidth < 0 || column.MaxWidth < 0 || (column.MaxWidth > 0 && column.MaxWidth < column.MinWidth) {
			return fmt.Errorf("%w: column %d bounds are invalid", ErrTableDimensionsInvalid, index)
		}
		switch column.Kind {
		case TableTrackFixed:
			if column.Width <= 0 || column.Width < column.MinWidth || (column.MaxWidth > 0 && column.Width > column.MaxWidth) {
				return fmt.Errorf("%w: column %d fixed width must be positive", ErrTableDimensionsInvalid, index)
			}
		case TableTrackIntrinsic:
			if column.Width != 0 {
				return fmt.Errorf("%w: column %d intrinsic width must be zero", ErrTableDimensionsInvalid, index)
			}
		default:
			return fmt.Errorf("%w: column %d has an invalid track kind", ErrTableDimensionsInvalid, index)
		}
	}
	return nil
}

func validateSelectedTableBody(input TablePlanInput, body Rect) error {
	if err := body.Validate(); err != nil || body.IsEmpty() || body.X != input.Body.X || body.Width != input.Body.Width || body.Y < 0 {
		return fmt.Errorf("%w: selected body must preserve x/width and have positive valid extents", ErrTableDimensionsInvalid)
	}
	right, err := body.Right()
	if err != nil || right > input.PageSize.Width {
		return fmt.Errorf("%w: selected body lies outside the page", ErrTableDimensionsInvalid)
	}
	bottom, err := body.Bottom()
	if err != nil || bottom > input.PageSize.Height {
		return fmt.Errorf("%w: selected body lies outside the page", ErrTableDimensionsInvalid)
	}
	if input.Width > body.Width {
		return fmt.Errorf("%w: table width exceeds selected body", ErrTableDimensionsInvalid)
	}
	return nil
}

func validateTableCell(cell TableCell, rows, columns, headerRows uint32) error {
	if cell.RowSpan == 0 || cell.ColumnSpan == 0 {
		return tableSpanError(cell, "table cell spans must be positive")
	}
	if uint64(cell.Row)+uint64(cell.RowSpan) > uint64(rows) || uint64(cell.Column)+uint64(cell.ColumnSpan) > uint64(columns) {
		return tableSpanError(cell, "table cell span extends outside the declared grid")
	}
	if cell.Row < headerRows && cell.Row+cell.RowSpan > headerRows {
		return tableSpanError(cell, "table cell rowspan crosses the repeatable header boundary")
	}
	if cell.MinWidth < 0 || cell.PreferredWidth < cell.MinWidth || cell.MinHeight < 0 {
		return tableSpanError(cell, "table cell intrinsic measurements are invalid")
	}
	if !cell.Node.Valid() || cell.Key == "" || !cell.Instance.Valid() {
		return tableSpanError(cell, "table cell has an absent semantic identity")
	}
	if err := cell.Source.Validate(); err != nil {
		return tableSpanError(cell, "table cell source provenance is invalid")
	}
	return nil
}

func tableSpanError(cell TableCell, message string) error {
	return newPlanningError(ErrTableSpanInvalid, Diagnostic{
		Code: DiagnosticTableSpanInvalid, Severity: SeverityError, Stage: StageLayout,
		Message:  message,
		Location: DiagnosticLocation{Node: cell.Node, Key: cell.Key, Source: cell.Source, Instance: cell.Instance},
		Evidence: []DiagnosticEvidence{
			{Key: "row", Value: strconv.FormatUint(uint64(cell.Row), 10)},
			{Key: "column", Value: strconv.FormatUint(uint64(cell.Column), 10)},
			{Key: "row_span", Value: strconv.FormatUint(uint64(cell.RowSpan), 10)},
			{Key: "column_span", Value: strconv.FormatUint(uint64(cell.ColumnSpan), 10)},
		},
	})
}

func tableResourceLimit(message string, limits TablePlanLimits, input TablePlanInput) error {
	return newPlanningError(ErrTableWorkLimit, Diagnostic{
		Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout,
		Message: message,
		Evidence: []DiagnosticEvidence{
			{Key: "rows", Value: strconv.FormatUint(uint64(input.Rows), 10)},
			{Key: "columns", Value: strconv.Itoa(len(input.Columns))},
			{Key: "cells", Value: strconv.Itoa(len(input.Cells))},
			{Key: "max_rows", Value: strconv.FormatUint(uint64(limits.MaxRows), 10)},
			{Key: "max_columns", Value: strconv.FormatUint(uint64(limits.MaxColumns), 10)},
			{Key: "max_cells", Value: strconv.FormatUint(uint64(limits.MaxCells), 10)},
			{Key: "max_occupancy_slots", Value: strconv.FormatUint(limits.MaxOccupancySlots, 10)},
		},
	})
}

func resolveColumnWidths(input TablePlanInput, cells []TableCell, budget *tableBudget) ([]Fixed, error) {
	widths := make([]Fixed, len(input.Columns))
	intrinsic := make([]int, 0, len(input.Columns))
	for index, column := range input.Columns {
		if column.Kind == TableTrackFixed {
			widths[index] = column.Width
		} else {
			widths[index] = column.MinWidth
			intrinsic = append(intrinsic, index)
		}
	}
	for _, cell := range cells {
		if err := budget.charge(uint64(cell.ColumnSpan)); err != nil {
			return nil, err
		}
		span := widths[cell.Column : cell.Column+cell.ColumnSpan]
		current, err := sumFixed(span)
		if err != nil {
			return nil, fmt.Errorf("layoutengine: table column minimum sum: %w", err)
		}
		if current >= cell.MinWidth {
			continue
		}
		deficit, _ := cell.MinWidth.Sub(current)
		indexes := intrinsicInSpan(input.Columns, cell.Column, cell.ColumnSpan)
		if len(indexes) == 0 {
			return nil, trackOverflowError(cell, input.Width, "cell minimum cannot fit its fixed column span")
		}
		distributed, err := distributeTableWidth(widths, input.Columns, indexes, deficit)
		if err != nil {
			return nil, err
		}
		if distributed != deficit {
			return nil, trackOverflowError(cell, input.Width, "cell minimum exceeds its column maximums")
		}
	}
	total, err := sumFixed(widths)
	if err != nil {
		return nil, err
	}
	if total > input.Width {
		return nil, trackOverflowError(TableCell{}, input.Width, "table column minimums exceed the explicit table width")
	}
	remaining, _ := input.Width.Sub(total)
	for _, cell := range cells {
		if remaining == 0 {
			break
		}
		span := widths[cell.Column : cell.Column+cell.ColumnSpan]
		current, err := sumFixed(span)
		if err != nil {
			return nil, err
		}
		if current >= cell.PreferredWidth {
			continue
		}
		wanted, _ := cell.PreferredWidth.Sub(current)
		if wanted > remaining {
			wanted = remaining
		}
		indexes := intrinsicInSpan(input.Columns, cell.Column, cell.ColumnSpan)
		if len(indexes) == 0 {
			continue // preferred contributions never enlarge fixed tracks
		}
		distributed, err := distributeTableWidth(widths, input.Columns, indexes, wanted)
		if err != nil {
			return nil, err
		}
		remaining, _ = remaining.Sub(distributed)
	}
	if remaining > 0 {
		if len(intrinsic) == 0 {
			return nil, newPlanningError(ErrTableTrackUnresolved, Diagnostic{
				Code: DiagnosticConstraintOverdetermined, Severity: SeverityError, Stage: StageLayout,
				Message: "fixed table columns do not sum to the explicit table width",
				Evidence: []DiagnosticEvidence{
					{Key: "table_width_fixed", Value: strconv.FormatInt(int64(input.Width), 10)},
					{Key: "fixed_tracks_width_fixed", Value: strconv.FormatInt(int64(total), 10)},
				},
			})
		}
		distributed, err := distributeTableWidth(widths, input.Columns, intrinsic, remaining)
		if err != nil {
			return nil, err
		}
		if distributed != remaining {
			return nil, newPlanningError(ErrTableTrackUnresolved, Diagnostic{
				Code: DiagnosticConstraintOverdetermined, Severity: SeverityError, Stage: StageLayout,
				Message: "table column maximums do not fill the explicit table width",
			})
		}
	}
	return widths, nil
}

// ResolveTableColumnWidths resolves the same bounded fixed-point sizing used
// by PlanTable without constructing page geometry. It exists for adapters that
// must know final track widths before measuring wrapped cell heights.
func ResolveTableColumnWidths(ctx context.Context, width Fixed, columns []TableColumn, cells []TableCell, limits TablePlanLimits) ([]Fixed, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeTableLimits(limits)
	if err != nil {
		return nil, err
	}
	if width <= 0 || len(columns) == 0 || uint32(len(columns)) > limits.MaxColumns || uint64(len(cells)) > uint64(limits.MaxCells) { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return nil, ErrTableDimensionsInvalid
	}
	input := TablePlanInput{Width: width, Columns: cloneSlice(columns)}
	for index, column := range columns {
		if column.MinWidth < 0 || column.MaxWidth < 0 || (column.MaxWidth > 0 && column.MaxWidth < column.MinWidth) {
			return nil, fmt.Errorf("%w: column %d bounds are invalid", ErrTableDimensionsInvalid, index)
		}
		if column.Kind == TableTrackFixed {
			if column.Width <= 0 || column.Width < column.MinWidth || (column.MaxWidth > 0 && column.Width > column.MaxWidth) {
				return nil, fmt.Errorf("%w: column %d fixed width is outside its bounds", ErrTableDimensionsInvalid, index)
			}
		} else if column.Kind != TableTrackIntrinsic || column.Width != 0 {
			return nil, fmt.Errorf("%w: column %d track is invalid", ErrTableDimensionsInvalid, index)
		}
	}
	canonical := cloneSlice(cells)
	sort.SliceStable(canonical, func(i, j int) bool {
		if canonical[i].Row != canonical[j].Row {
			return canonical[i].Row < canonical[j].Row
		}
		return canonical[i].Column < canonical[j].Column
	})
	for _, cell := range canonical {
		if cell.ColumnSpan == 0 || uint64(cell.Column)+uint64(cell.ColumnSpan) > uint64(len(columns)) ||
			cell.MinWidth < 0 || cell.PreferredWidth < cell.MinWidth {
			return nil, ErrTableDimensionsInvalid
		}
	}
	budget := &tableBudget{ctx: ctx, limit: limits.MaxWork}
	if err := budget.charge(uint64(len(columns)) + uint64(len(canonical))); err != nil {
		return nil, err
	}
	return resolveColumnWidths(input, canonical, budget)
}

func distributeTableWidth(values []Fixed, columns []TableColumn, indexes []int, amount Fixed) (Fixed, error) {
	if amount < 0 || len(indexes) == 0 {
		return 0, ErrTableDimensionsInvalid
	}
	remaining := amount
	active := append([]int(nil), indexes...)
	var distributed Fixed
	for remaining > 0 && len(active) > 0 {
		share, err := remaining.DivInt(int64(len(active)))
		if err != nil {
			return 0, err
		}
		if share == 0 {
			share = 1
		}
		next := active[:0]
		for _, index := range active {
			addition := share
			if addition > remaining {
				addition = remaining
			}
			maximum := columns[index].MaxWidth
			if maximum > 0 {
				capacity, err := maximum.Sub(values[index])
				if err != nil || capacity <= 0 {
					continue
				}
				if addition > capacity {
					addition = capacity
				}
			}
			values[index], err = values[index].Add(addition)
			if err != nil {
				return 0, err
			}
			distributed, err = distributed.Add(addition)
			if err != nil {
				return 0, err
			}
			remaining, err = remaining.Sub(addition)
			if err != nil {
				return 0, err
			}
			if maximum == 0 || values[index] < maximum {
				next = append(next, index)
			}
			if remaining == 0 {
				break
			}
		}
		active = next
	}
	return distributed, nil
}

func intrinsicInSpan(columns []TableColumn, start, count uint32) []int {
	result := make([]int, 0, count)
	for index := start; index < start+count; index++ {
		if columns[index].Kind == TableTrackIntrinsic {
			result = append(result, int(index))
		}
	}
	return result
}

func distributeFixed(values []Fixed, indexes []int, amount Fixed) error {
	if amount < 0 || len(indexes) == 0 {
		return ErrTableDimensionsInvalid
	}
	share, err := amount.DivInt(int64(len(indexes)))
	if err != nil {
		return err
	}
	used, err := share.MulInt(int64(len(indexes)))
	if err != nil {
		return err
	}
	remainder, err := amount.Sub(used)
	if err != nil {
		return err
	}
	for order, index := range indexes {
		addition := share
		if Fixed(order) < remainder {
			addition, err = addition.Add(1)
			if err != nil {
				return err
			}
		}
		values[index], err = values[index].Add(addition)
		if err != nil {
			return err
		}
	}
	return nil
}

func trackOverflowError(cell TableCell, width Fixed, message string) error {
	location := DiagnosticLocation{}
	if cell.Node.Valid() {
		location = DiagnosticLocation{Node: cell.Node, Key: cell.Key, Source: cell.Source, Instance: cell.Instance}
	}
	return newPlanningError(ErrTableTrackOverflow, Diagnostic{
		Code: DiagnosticTrackMinOverflow, Severity: SeverityError, Stage: StageLayout,
		Message: message, Location: location,
		Evidence: []DiagnosticEvidence{{Key: "table_width_fixed", Value: strconv.FormatInt(int64(width), 10)}},
	})
}

func resolveRowHeights(rows uint32, cells []TableCell, budget *tableBudget) ([]Fixed, error) {
	heights := make([]Fixed, rows)
	for _, cell := range cells {
		if cell.RowSpan == 1 && cell.MinHeight > heights[cell.Row] {
			heights[cell.Row] = cell.MinHeight
		}
	}
	for _, cell := range cells {
		if cell.RowSpan == 1 {
			continue
		}
		if err := budget.charge(uint64(cell.RowSpan)); err != nil {
			return nil, err
		}
		span := heights[cell.Row : cell.Row+cell.RowSpan]
		current, err := sumFixed(span)
		if err != nil {
			return nil, err
		}
		if current >= cell.MinHeight {
			continue
		}
		deficit, _ := cell.MinHeight.Sub(current)
		indexes := make([]int, cell.RowSpan)
		for offset := range indexes {
			indexes[offset] = int(cell.Row) + offset
		}
		if err := distributeFixed(heights, indexes, deficit); err != nil {
			return nil, err
		}
	}
	return heights, nil
}

func deriveBodyGroups(headerRows, rows uint32, cells []TableCell, heights []Fixed, rowKeepWithNext []bool, keepTogether bool, orphans, widows uint32, budget *tableBudget) ([]tableRowGroup, error) {
	// Each row records the farthest exclusive row reached by a cell that starts
	// there. Walking those reaches computes the transitive rowspan components
	// without rescanning the complete cell set for every body row.
	reach := make([]uint32, rows)
	if err := budget.charge(uint64(len(cells)) + uint64(rows)); err != nil {
		return nil, err
	}
	for _, cell := range cells {
		if cell.Row < headerRows {
			continue
		}
		cellEnd := cell.Row + cell.RowSpan
		if cellEnd > reach[cell.Row] {
			reach[cell.Row] = cellEnd
		}
	}
	groups := make([]tableRowGroup, 0)
	for start := headerRows; start < rows; {
		end := start + 1
		for row := start; row < end; row++ {
			if reach[row] > end {
				end = reach[row]
			}
		}
		height, err := sumFixed(heights[start:end])
		if err != nil {
			return nil, err
		}
		groups = append(groups, tableRowGroup{start: start, end: end, height: height})
		start = end
	}
	if len(groups) == 0 {
		return groups, nil
	}
	merge := func(first, last int, policy string) error {
		if first < 0 || last >= len(groups) || first >= last {
			return nil
		}
		var total Fixed
		var err error
		for _, group := range groups[first : last+1] {
			total, err = total.Add(group.height)
			if err != nil {
				return err
			}
		}
		combined := tableRowGroup{start: groups[first].start, end: groups[last].end, height: total, policy: policy}
		groups = append(groups[:first], append([]tableRowGroup{combined}, groups[last+1:]...)...)
		return nil
	}
	// Join row-level keep-with-next chains transitively over rowspan groups.
	for index := 0; index+1 < len(groups); {
		lastRow := groups[index].end - 1
		join := int(lastRow) < len(rowKeepWithNext) && rowKeepWithNext[lastRow]
		if !join {
			index++
			continue
		}
		if err := merge(index, index+1, "row_keep_with_next"); err != nil {
			return nil, err
		}
	}
	if keepTogether {
		if err := merge(0, len(groups)-1, "table_keep_together"); err != nil {
			return nil, err
		}
		return groups, nil
	}
	if orphans > 1 {
		last := 0
		for last+1 < len(groups) && groups[last].end-headerRows < orphans {
			last++
		}
		if err := merge(0, last, "table_orphans"); err != nil {
			return nil, err
		}
	}
	if widows > 1 && len(groups) > 1 {
		first := len(groups) - 1
		rowsHeld := groups[first].end - groups[first].start
		for first > 0 && rowsHeld < widows {
			first--
			rowsHeld = groups[len(groups)-1].end - groups[first].start
		}
		if err := merge(first, len(groups)-1, "table_widows"); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

func sumFixed(values []Fixed) (Fixed, error) {
	var total Fixed
	for _, value := range values {
		var err error
		total, err = total.Add(value)
		if err != nil {
			return 0, err
		}
	}
	return total, nil
}

type tablePaginator struct {
	input   TablePlanInput
	limits  TablePlanLimits
	budget  *tableBudget
	cells   []TableCell
	columns []Fixed
	rows    []Fixed
	groups  []tableRowGroup

	output              LayoutPlanInput
	page                uint32
	pageFragmentStart   int
	cursor              Fixed
	lastBodyFragment    FragmentID
	lastBodyOverflow    bool
	headerHeight        Fixed
	headerGeometry      tableUnitGeometry
	headerGeometryReady bool
	headerGeometryPlans uint32
	headerTooTallAdded  bool
	body                Rect
	gridGroup           uint32
}

func (p *tablePaginator) plan() (LayoutPlan, error) {
	var err error
	p.headerHeight, err = sumFixed(p.rows[:p.input.HeaderRows])
	if err != nil {
		return LayoutPlan{}, err
	}
	if err := p.startPage(); err != nil {
		return LayoutPlan{}, err
	}
	if len(p.groups) == 0 {
		p.finishPage()
		return NewLayoutPlan(p.output)
	}
	for _, group := range p.groups {
		if err := p.budget.charge(uint64(group.end - group.start)); err != nil {
			return LayoutPlan{}, err
		}
		remaining, err := p.remaining()
		if err != nil {
			return LayoutPlan{}, err
		}
		if group.height > remaining && p.lastBodyFragment.Valid() {
			from := p.page
			preceding := p.lastBodyFragment
			previousOverflow := p.lastBodyOverflow
			available := remaining
			p.finishPage()
			if err := p.startPage(); err != nil {
				return LayoutPlan{}, err
			}
			triggering, _, err := p.placeRows(group.start, group.end, false)
			if err != nil {
				return LayoutPlan{}, err
			}
			reason := BreakPaginationConstraint
			if previousOverflow {
				reason = BreakPreviousFragmentOverflow
				available = 0
			}
			p.output.Breaks = append(p.output.Breaks, BreakDecision{
				Reason: reason, FromPage: from, ToPage: p.page, Region: RegionBody,
				Preceding: preceding, Triggering: triggering,
				Required: group.height, Available: available,
			})
			if err := p.noteOversize(group, triggering); err != nil {
				return LayoutPlan{}, err
			}
			continue
		}
		first, _, err := p.placeRows(group.start, group.end, false)
		if err != nil {
			return LayoutPlan{}, err
		}
		if err := p.noteOversize(group, first); err != nil {
			return LayoutPlan{}, err
		}
	}
	p.finishPage()
	return NewLayoutPlan(p.output)
}

func (p *tablePaginator) startPage() error {
	if p.page >= p.limits.MaxPages {
		return newPlanningError(ErrTablePageLimit, Diagnostic{
			Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout,
			Message:  "table pagination exceeded its page limit",
			Evidence: []DiagnosticEvidence{{Key: "max_pages", Value: strconv.FormatUint(uint64(p.limits.MaxPages), 10)}},
		})
	}
	if err := p.budget.charge(1); err != nil {
		return err
	}
	p.page++
	p.body = p.input.Body
	if p.input.BodyForPage != nil {
		selected, err := p.input.BodyForPage(p.page)
		if err != nil {
			return fmt.Errorf("layoutengine: select table body for page %d: %w", p.page, err)
		}
		if err := validateSelectedTableBody(p.input, selected); err != nil {
			return fmt.Errorf("layoutengine: table body for page %d: %w", p.page, err)
		}
		p.body = selected
	}
	p.output.PageRegions = append(p.output.PageRegions, PlannedPageRegion{Page: p.page, Region: RegionBody, Bounds: p.body})
	p.pageFragmentStart = len(p.output.Fragments)
	p.cursor = p.body.Y
	p.lastBodyFragment = 0
	p.lastBodyOverflow = false
	if p.page == 1 && p.input.Caption != nil {
		caption := p.input.Caption
		box, err := NewRect(p.body.X, p.cursor, p.input.Width, caption.MinHeight)
		if err != nil {
			return err
		}
		id := FragmentID(len(p.output.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		p.output.Fragments = append(p.output.Fragments, Fragment{ID: id, Node: caption.Node, Key: caption.Key, Instance: caption.Instance, Page: p.page, Region: RegionBody, BorderBox: box, ContentBox: box, Source: caption.Source, Continuation: ContinuationWhole})
		p.cursor, err = p.cursor.Add(caption.MinHeight)
		if err != nil {
			return err
		}
	}
	if p.input.HeaderRows > 0 {
		first, _, err := p.placeRows(0, p.input.HeaderRows, true)
		if err != nil {
			return err
		}
		if p.headerHeight > p.body.Height && !p.headerTooTallAdded {
			fragment := p.fragment(first)
			p.output.Diagnostics = append(p.output.Diagnostics, Diagnostic{
				Code: DiagnosticRepeatedHeaderTooTall, Severity: SeverityWarning, Stage: StageLayout,
				Message:  "repeatable table header exceeds the page body height and was emitted once per table page",
				Location: diagnosticLocationForFragment(fragment),
				Evidence: []DiagnosticEvidence{
					{Key: "header_height_fixed", Value: strconv.FormatInt(int64(p.headerHeight), 10)},
					{Key: "body_height_fixed", Value: strconv.FormatInt(int64(p.body.Height), 10)},
				},
			})
			p.headerTooTallAdded = true
		}
	}
	return nil
}

func (p *tablePaginator) finishPage() {
	p.output.Pages = append(p.output.Pages, PlannedPage{
		Number: p.page, Size: p.input.PageSize,
		Fragments: IndexRange{Start: uint32(p.pageFragmentStart), Count: uint32(len(p.output.Fragments) - p.pageFragmentStart)}, // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	})
}

func (p *tablePaginator) remaining() (Fixed, error) {
	bottom, err := p.body.Bottom()
	if err != nil {
		return 0, err
	}
	remaining, err := bottom.Sub(p.cursor)
	if err != nil {
		return 0, err
	}
	if remaining < 0 {
		return 0, nil
	}
	return remaining, nil
}

// placeRows emits cells anchored inside [start,end). No cell may cross this
// range because header-boundary validation and rowspan-derived groups ensure
// every repeated or paginated unit is closed under rowspans.
func (p *tablePaginator) placeRows(start, end uint32, header bool) (FragmentID, FragmentID, error) {
	if header {
		geometry, err := p.repeatedHeaderGeometry(start, end)
		if err != nil {
			return 0, 0, err
		}
		return p.placeRowsWithGeometry(start, end, true, geometry.rowOffsets, geometry.columnOffsets, geometry.height)
	}
	rowOffsets := make([]Fixed, end-start+1)
	for row := start; row < end; row++ {
		next, err := rowOffsets[row-start].Add(p.rows[row])
		if err != nil {
			return 0, 0, err
		}
		rowOffsets[row-start+1] = next
	}
	columnOffsets := make([]Fixed, len(p.columns)+1)
	for column, width := range p.columns {
		next, err := columnOffsets[column].Add(width)
		if err != nil {
			return 0, 0, err
		}
		columnOffsets[column+1] = next
	}
	return p.placeRowsWithGeometry(start, end, false, rowOffsets, columnOffsets, rowOffsets[len(rowOffsets)-1])
}

func (p *tablePaginator) placeRowsWithGeometry(start, end uint32, header bool, rowOffsets, columnOffsets []Fixed, unitHeight Fixed) (FragmentID, FragmentID, error) {
	p.gridGroup++
	for column, width := range p.columns {
		x, err := p.body.X.Add(columnOffsets[column])
		if err != nil {
			return 0, 0, err
		}
		bounds, err := NewRect(x, p.cursor, width, unitHeight)
		if err != nil {
			return 0, 0, err
		}
		p.output.GridTracks = append(p.output.GridTracks, PlannedGridTrack{Group: p.gridGroup, Page: p.page, Region: RegionBody,
			Axis: GridTrackColumn, Index: uint32(column), Bounds: bounds})
	}
	for row := start; row < end; row++ {
		y, err := p.cursor.Add(rowOffsets[row-start])
		if err != nil {
			return 0, 0, err
		}
		bounds, err := NewRect(p.body.X, y, p.input.Width, p.rows[row])
		if err != nil {
			return 0, 0, err
		}
		p.output.GridTracks = append(p.output.GridTracks, PlannedGridTrack{Group: p.gridGroup, Page: p.page, Region: RegionBody,
			Axis: GridTrackRow, Index: row - start, Bounds: bounds})
	}
	var first, last FragmentID
	for _, cell := range p.cells {
		if cell.Row < start || cell.Row >= end {
			continue
		}
		if err := p.budget.charge(1); err != nil {
			return 0, 0, err
		}
		x, err := p.body.X.Add(columnOffsets[cell.Column])
		if err != nil {
			return 0, 0, err
		}
		y, err := p.cursor.Add(rowOffsets[cell.Row-start])
		if err != nil {
			return 0, 0, err
		}
		width, err := columnOffsets[cell.Column+cell.ColumnSpan].Sub(columnOffsets[cell.Column])
		if err != nil {
			return 0, 0, err
		}
		height, err := rowOffsets[cell.Row-start+cell.RowSpan].Sub(rowOffsets[cell.Row-start])
		if err != nil {
			return 0, 0, err
		}
		box, err := NewRect(x, y, width, height)
		if err != nil {
			return 0, 0, err
		}
		id := FragmentID(len(p.output.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		p.output.Fragments = append(p.output.Fragments, Fragment{
			ID: id, Node: cell.Node, Key: cell.Key, Instance: cell.Instance,
			Page: p.page, Region: RegionBody, BorderBox: box, ContentBox: box,
			Source: cell.Source, Continuation: ContinuationWhole, Repeated: header && p.page > 1,
		})
		if !first.Valid() {
			first = id
		}
		last = id
	}
	nextCursor, err := p.cursor.Add(unitHeight)
	if err != nil {
		return 0, 0, err
	}
	p.cursor = nextCursor
	if !header {
		p.lastBodyFragment = last
		bodyBottom, err := p.body.Bottom()
		if err != nil {
			return 0, 0, err
		}
		p.lastBodyOverflow = p.cursor > bodyBottom
	}
	return first, last, nil
}

func (p *tablePaginator) repeatedHeaderGeometry(start, end uint32) (tableUnitGeometry, error) {
	if p.headerGeometryReady {
		return p.headerGeometry, nil
	}
	geometry := tableUnitGeometry{rowOffsets: make([]Fixed, end-start+1), columnOffsets: make([]Fixed, len(p.columns)+1)}
	for row := start; row < end; row++ {
		next, err := geometry.rowOffsets[row-start].Add(p.rows[row])
		if err != nil {
			return tableUnitGeometry{}, err
		}
		geometry.rowOffsets[row-start+1] = next
	}
	for column, width := range p.columns {
		next, err := geometry.columnOffsets[column].Add(width)
		if err != nil {
			return tableUnitGeometry{}, err
		}
		geometry.columnOffsets[column+1] = next
	}
	geometry.height = geometry.rowOffsets[len(geometry.rowOffsets)-1]
	p.headerGeometry = geometry
	p.headerGeometryReady = true
	p.headerGeometryPlans++
	return geometry, nil
}

func (p *tablePaginator) noteOversize(group tableRowGroup, first FragmentID) error {
	available, err := p.body.Height.Sub(p.headerHeight)
	if err != nil {
		return err
	}
	if available < 0 {
		available = 0
	}
	if group.height <= available {
		return nil
	}
	fragment := p.fragment(first)
	code := DiagnosticUnbreakableTooTall
	message := "indivisible table row exceeds the space available below repeated headers and was emitted once"
	if group.end-group.start > 1 && group.policy == "" {
		code = DiagnosticTableRowspanCrossesPage
		message = "rowspan-connected table row group cannot fit below repeated headers and was emitted once"
	}
	p.output.Diagnostics = append(p.output.Diagnostics, Diagnostic{
		Code: code, Severity: SeverityWarning, Stage: StageLayout, Message: message,
		Location: diagnosticLocationForFragment(fragment),
		Evidence: []DiagnosticEvidence{
			{Key: "first_row", Value: strconv.FormatUint(uint64(group.start), 10)},
			{Key: "row_count", Value: strconv.FormatUint(uint64(group.end-group.start), 10)},
			{Key: "group_height_fixed", Value: strconv.FormatInt(int64(group.height), 10)},
			{Key: "available_below_header_fixed", Value: strconv.FormatInt(int64(available), 10)},
		},
	})
	if group.policy != "" {
		p.output.Diagnostics = append(p.output.Diagnostics, Diagnostic{
			Code: DiagnosticKeepTooLarge, Severity: SeverityWarning, Stage: StageLayout,
			Message:  "table pagination preference exceeded the available page body and was relaxed after one deterministic placement",
			Location: diagnosticLocationForFragment(fragment),
			Evidence: []DiagnosticEvidence{
				{Key: "policy", Value: group.policy},
				{Key: "requested_rows", Value: strconv.FormatUint(uint64(group.end-group.start), 10)},
				{Key: "available_below_header_fixed", Value: strconv.FormatInt(int64(available), 10)},
			},
		})
	}
	return nil
}

func (p *tablePaginator) fragment(id FragmentID) Fragment {
	if !id.Valid() || int(id) > len(p.output.Fragments) {
		return Fragment{}
	}
	return p.output.Fragments[id-1]
}

func diagnosticLocationForFragment(fragment Fragment) DiagnosticLocation {
	return DiagnosticLocation{
		Node: fragment.Node, Key: fragment.Key, Source: fragment.Source, Instance: fragment.Instance,
		Fragment: fragment.ID, Page: fragment.Page, Region: fragment.Region,
		Bounds: fragment.BorderBox, HasBounds: true,
	}
}
