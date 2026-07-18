// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
	"math/bits"
	"sort"
	"strconv"
)

type GridTrackKind string

const (
	GridTrackFixed    GridTrackKind = "fixed"
	GridTrackAuto     GridTrackKind = "auto"
	GridTrackFraction GridTrackKind = "fraction"
)

var (
	ErrGridLimits            = errors.New("layoutengine: grid work limit exceeded")
	ErrGridTrack             = errors.New("layoutengine: invalid grid track")
	ErrGridPlacement         = errors.New("layoutengine: invalid grid placement")
	ErrGridOverlap           = errors.New("layoutengine: grid items overlap")
	ErrGridIntrinsicOverflow = errors.New("layoutengine: grid intrinsic minimum exceeds its region")
	ErrGridWorkLimit         = errors.New("layoutengine: grid deterministic work limit exceeded")
)

// GridTrack is one explicit fixed, intrinsic auto, or fractional track. Min
// is a fixed-point lower bound. Fixed tracks use Size; fraction tracks require
// a positive Weight. Auto tracks stretch equally after intrinsic resolution.
type GridTrack struct {
	Kind   GridTrackKind
	Size   Fixed
	Min    Fixed
	Weight uint32
}

type GridItem struct {
	Node       NodeID
	Key        NodeKey
	Instance   InstanceID
	Source     SourceSpan
	Row        uint32
	Column     uint32
	RowSpan    uint32
	ColumnSpan uint32
	MinWidth   Fixed
	MinHeight  Fixed
}

type GridPlanLimits struct {
	MaxColumns uint32
	MaxRows    uint32
	MaxItems   uint32
	MaxCells   uint64
	MaxWork    uint64
}

func DefaultGridPlanLimits() GridPlanLimits {
	return GridPlanLimits{MaxColumns: 1024, MaxRows: 1 << 16, MaxItems: 1 << 20, MaxCells: 1 << 24, MaxWork: 1 << 27}
}

type gridBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *gridBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "grid planning", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
			Stage: StageLayout, Message: "grid planning was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrGridWorkLimit, Diagnostic{Code: DiagnosticWorkLimit, Severity: SeverityError,
			Stage: StageLayout, Message: "grid planning exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(budget.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)}}})
	}
	budget.used += amount
	return nil
}

type GridPlanInput struct {
	PageSize  Size
	Region    Rect
	Columns   []GridTrack
	Rows      []GridTrack
	ColumnGap Fixed
	RowGap    Fixed
	Items     []GridItem
	Limits    GridPlanLimits
}

// GridPlanResult owns the immutable geometry plan and detached resolved track
// sizes. Track sums plus gaps never exceed the supplied region.
type GridPlanResult struct {
	Plan        LayoutPlan
	columnSizes []Fixed
	rowSizes    []Fixed
	UsedWidth   Fixed
	UsedHeight  Fixed
}

func (result GridPlanResult) ColumnSizes() []Fixed { return cloneSlice(result.columnSizes) }
func (result GridPlanResult) RowSizes() []Fixed    { return cloneSlice(result.rowSizes) }

// PlanGrid resolves intrinsic span contributions, distributes remaining space
// deterministically, rejects overlapping occupancy, and emits one whole
// fragment per item. It performs no text measurement or painting.
func PlanGrid(input GridPlanInput) (GridPlanResult, error) {
	return PlanGridContext(context.Background(), input)
}

// PlanGridContext performs cancellation-aware, work-bounded grid resolution.
// Limits remain part of GridPlanInput so plan identity is independent of the
// caller's context implementation.
func PlanGridContext(ctx context.Context, input GridPlanInput) (GridPlanResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits := input.Limits
	if limits == (GridPlanLimits{}) {
		limits = DefaultGridPlanLimits()
	}
	if err := validateGridLimits(limits); err != nil {
		return GridPlanResult{}, err
	}
	budget := &gridBudget{ctx: ctx, limit: limits.MaxWork}
	if err := budget.charge(1); err != nil {
		return GridPlanResult{}, err
	}
	if err := validateVerticalFlowInput(VerticalFlowInput{PageSize: input.PageSize, Body: input.Region}); err != nil {
		return GridPlanResult{}, err
	}
	if input.ColumnGap < 0 || input.RowGap < 0 {
		return GridPlanResult{}, fmt.Errorf("%w: gaps must be non-negative", ErrGridTrack)
	}
	if len(input.Columns) == 0 || len(input.Rows) == 0 {
		return GridPlanResult{}, fmt.Errorf("%w: grid requires rows and columns", ErrGridTrack)
	}
	if uint64(len(input.Columns)) > uint64(limits.MaxColumns) || uint64(len(input.Rows)) > uint64(limits.MaxRows) ||
		uint64(len(input.Items)) > uint64(limits.MaxItems) {
		return GridPlanResult{}, ErrGridLimits
	}
	cells := uint64(len(input.Columns)) * uint64(len(input.Rows))
	if cells > limits.MaxCells {
		return GridPlanResult{}, ErrGridLimits
	}

	items := append([]GridItem(nil), input.Items...)
	if err := validateAndSortGridItems(items, uint32(len(input.Rows)), uint32(len(input.Columns)), cells, budget); err != nil { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return GridPlanResult{}, err
	}
	columnContributions := make([]gridSpanContribution, 0, len(items))
	rowContributions := make([]gridSpanContribution, 0, len(items))
	for _, item := range items {
		if err := budget.charge(1); err != nil {
			return GridPlanResult{}, err
		}
		columnContributions = append(columnContributions, gridSpanContribution{Start: item.Column, Span: item.ColumnSpan, Minimum: item.MinWidth})
		rowContributions = append(rowContributions, gridSpanContribution{Start: item.Row, Span: item.RowSpan, Minimum: item.MinHeight})
	}
	columns, usedWidth, err := resolveGridAxis(input.Region.Width, input.ColumnGap, input.Columns, columnContributions, budget)
	if err != nil {
		return GridPlanResult{}, fmt.Errorf("layoutengine: grid columns: %w", err)
	}
	rows, usedHeight, err := resolveGridAxis(input.Region.Height, input.RowGap, input.Rows, rowContributions, budget)
	if err != nil {
		return GridPlanResult{}, fmt.Errorf("layoutengine: grid rows: %w", err)
	}
	columnOffsets, err := gridOffsets(input.Region.X, columns, input.ColumnGap, budget)
	if err != nil {
		return GridPlanResult{}, err
	}
	rowOffsets, err := gridOffsets(input.Region.Y, rows, input.RowGap, budget)
	if err != nil {
		return GridPlanResult{}, err
	}

	planInput := LayoutPlanInput{Pages: []PlannedPage{{
		Number: 1, Size: input.PageSize, Fragments: IndexRange{Count: uint32(len(items))}, // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}}, PageRegions: []PlannedPageRegion{{Page: 1, Region: RegionBody, Bounds: input.Region}}}
	for index, width := range columns {
		bounds, boundsErr := NewRect(columnOffsets[index], input.Region.Y, width, usedHeight)
		if boundsErr != nil {
			return GridPlanResult{}, boundsErr
		}
		gapAfter := Fixed(0)
		if index+1 < len(columns) {
			gapAfter = input.ColumnGap
		}
		planInput.GridTracks = append(planInput.GridTracks, PlannedGridTrack{Group: 1, Page: 1, Region: RegionBody,
			Axis: GridTrackColumn, Index: uint32(index), Bounds: bounds, GapAfter: gapAfter})
	}
	for index, height := range rows {
		bounds, boundsErr := NewRect(input.Region.X, rowOffsets[index], usedWidth, height)
		if boundsErr != nil {
			return GridPlanResult{}, boundsErr
		}
		gapAfter := Fixed(0)
		if index+1 < len(rows) {
			gapAfter = input.RowGap
		}
		planInput.GridTracks = append(planInput.GridTracks, PlannedGridTrack{Group: 1, Page: 1, Region: RegionBody,
			Axis: GridTrackRow, Index: uint32(index), Bounds: bounds, GapAfter: gapAfter})
	}
	for index, item := range items {
		if err := budget.charge(1); err != nil {
			return GridPlanResult{}, err
		}
		width, err := gridSpanExtent(columns, input.ColumnGap, item.Column, item.ColumnSpan, budget)
		if err != nil {
			return GridPlanResult{}, err
		}
		height, err := gridSpanExtent(rows, input.RowGap, item.Row, item.RowSpan, budget)
		if err != nil {
			return GridPlanResult{}, err
		}
		bounds, err := NewRect(columnOffsets[item.Column], rowOffsets[item.Row], width, height)
		if err != nil {
			return GridPlanResult{}, err
		}
		planInput.Fragments = append(planInput.Fragments, Fragment{
			ID: FragmentID(index + 1), Node: item.Node, Key: item.Key, Instance: item.Instance,
			Page: 1, Region: RegionBody, BorderBox: bounds, ContentBox: bounds,
			Source: item.Source, Continuation: ContinuationWhole,
		})
	}
	plan, err := NewLayoutPlan(planInput)
	if err != nil {
		return GridPlanResult{}, err
	}
	return GridPlanResult{Plan: plan, columnSizes: columns, rowSizes: rows, UsedWidth: usedWidth, UsedHeight: usedHeight}, nil
}

type gridSpanContribution struct {
	Start   uint32
	Span    uint32
	Minimum Fixed
}

func validateGridLimits(limits GridPlanLimits) error {
	if limits.MaxColumns == 0 || limits.MaxRows == 0 || limits.MaxItems == 0 || limits.MaxCells == 0 ||
		limits.MaxWork == 0 || limits.MaxColumns > 1<<16 || limits.MaxRows > 1<<18 || limits.MaxItems > 1<<22 ||
		limits.MaxCells > 1<<26 || limits.MaxWork > 1<<29 {
		return errors.New("layoutengine: grid limits must be positive and within hard caps")
	}
	return nil
}

func validateAndSortGridItems(items []GridItem, rows, columns uint32, cells uint64, budget *gridBudget) error {
	occupied := make([]bool, cells)
	for index, item := range items {
		if err := budget.charge(1); err != nil {
			return err
		}
		if !item.Node.Valid() || !item.Instance.Valid() || item.Key == "" || item.RowSpan == 0 || item.ColumnSpan == 0 ||
			item.MinWidth < 0 || item.MinHeight < 0 {
			return fmt.Errorf("%w: item %d", ErrGridPlacement, index)
		}
		if err := validateTextIdentity("grid item key", string(item.Key)); err != nil {
			return fmt.Errorf("%w: item %d: %w", ErrGridPlacement, index, err)
		}
		if err := validateTextIdentity("grid item instance", string(item.Instance)); err != nil {
			return fmt.Errorf("%w: item %d: %w", ErrGridPlacement, index, err)
		}
		if err := item.Source.Validate(); err != nil {
			return fmt.Errorf("%w: item %d source: %w", ErrGridPlacement, index, err)
		}
		rowEnd := uint64(item.Row) + uint64(item.RowSpan)
		columnEnd := uint64(item.Column) + uint64(item.ColumnSpan)
		if rowEnd > uint64(rows) || columnEnd > uint64(columns) {
			return fmt.Errorf("%w: item %d exceeds grid", ErrGridPlacement, index)
		}
		for row := item.Row; row < uint32(rowEnd); row++ { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			for column := item.Column; column < uint32(columnEnd); column++ { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
				if err := budget.charge(1); err != nil {
					return err
				}
				cell := uint64(row)*uint64(columns) + uint64(column)
				if occupied[cell] {
					return fmt.Errorf("%w: row %d column %d", ErrGridOverlap, row, column)
				}
				occupied[cell] = true
			}
		}
	}
	sortWork := uint64(len(items)) * uint64(bits.Len(uint(len(items))+1)+1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if err := budget.charge(sortWork); err != nil {
		return err
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Row != items[j].Row {
			return items[i].Row < items[j].Row
		}
		if items[i].Column != items[j].Column {
			return items[i].Column < items[j].Column
		}
		if items[i].Key != items[j].Key {
			return items[i].Key < items[j].Key
		}
		return items[i].Node < items[j].Node
	})
	return nil
}

func resolveGridAxis(available, gap Fixed, tracks []GridTrack, contributions []gridSpanContribution, budget *gridBudget) ([]Fixed, Fixed, error) {
	sizes := make([]Fixed, len(tracks))
	flexible := make([]bool, len(tracks))
	var totalWeight uint64
	for index, track := range tracks {
		if err := budget.charge(1); err != nil {
			return nil, 0, err
		}
		if track.Min < 0 || track.Size < 0 {
			return nil, 0, ErrGridTrack
		}
		switch track.Kind {
		case GridTrackFixed:
			if track.Weight != 0 || track.Size < track.Min {
				return nil, 0, ErrGridTrack
			}
			sizes[index] = track.Size
		case GridTrackAuto:
			if track.Weight != 0 || track.Size != 0 {
				return nil, 0, ErrGridTrack
			}
			sizes[index], flexible[index] = track.Min, true
		case GridTrackFraction:
			if track.Weight == 0 || track.Size != 0 {
				return nil, 0, ErrGridTrack
			}
			sizes[index], flexible[index] = track.Min, true
			totalWeight += uint64(track.Weight)
		default:
			return nil, 0, ErrGridTrack
		}
	}
	for _, contribution := range contributions {
		if err := budget.charge(1); err != nil {
			return nil, 0, err
		}
		current, err := gridSpanExtent(sizes, gap, contribution.Start, contribution.Span, budget)
		if err != nil {
			return nil, 0, err
		}
		if current >= contribution.Minimum {
			continue
		}
		deficit, err := contribution.Minimum.Sub(current)
		if err != nil {
			return nil, 0, err
		}
		candidates := make([]int, 0, contribution.Span)
		for index := contribution.Start; index < contribution.Start+contribution.Span; index++ {
			if err := budget.charge(1); err != nil {
				return nil, 0, err
			}
			if flexible[index] {
				candidates = append(candidates, int(index))
			}
		}
		if len(candidates) == 0 {
			return nil, 0, ErrGridIntrinsicOverflow
		}
		if err := distributeGridUnits(sizes, candidates, deficit, nil, budget); err != nil {
			return nil, 0, err
		}
	}
	used, err := gridAxisUsed(sizes, gap, budget)
	if err != nil {
		return nil, 0, err
	}
	if used > available {
		return nil, 0, ErrGridIntrinsicOverflow
	}
	remaining, err := available.Sub(used)
	if err != nil {
		return nil, 0, err
	}
	if remaining > 0 {
		fractionIndexes := make([]int, 0)
		weights := make([]uint32, 0)
		for index, track := range tracks {
			if err := budget.charge(1); err != nil {
				return nil, 0, err
			}
			if track.Kind == GridTrackFraction {
				fractionIndexes = append(fractionIndexes, index)
				weights = append(weights, track.Weight)
			}
		}
		if len(fractionIndexes) != 0 {
			if totalWeight == 0 || totalWeight > uint64(^uint32(0)) {
				return nil, 0, ErrGridTrack
			}
			if err := distributeGridUnits(sizes, fractionIndexes, remaining, weights, budget); err != nil {
				return nil, 0, err
			}
		} else {
			autoIndexes := make([]int, 0)
			for index, track := range tracks {
				if err := budget.charge(1); err != nil {
					return nil, 0, err
				}
				if track.Kind == GridTrackAuto {
					autoIndexes = append(autoIndexes, index)
				}
			}
			if len(autoIndexes) != 0 {
				if err := distributeGridUnits(sizes, autoIndexes, remaining, nil, budget); err != nil {
					return nil, 0, err
				}
			}
		}
	}
	used, err = gridAxisUsed(sizes, gap, budget)
	return sizes, used, err
}

func distributeGridUnits(sizes []Fixed, indexes []int, amount Fixed, weights []uint32, budget *gridBudget) error {
	if amount < 0 || len(indexes) == 0 {
		return ErrGridTrack
	}
	if len(weights) != 0 && len(weights) != len(indexes) {
		return ErrGridTrack
	}
	if len(weights) == 0 {
		weights = make([]uint32, len(indexes))
		for index := range weights {
			weights[index] = 1
		}
	}
	var total uint64
	for _, weight := range weights {
		if err := budget.charge(1); err != nil {
			return err
		}
		total += uint64(weight)
	}
	if total == 0 || total > uint64(^uint32(0)) {
		return ErrGridTrack
	}
	amountInt := int64(amount)
	allocated := int64(0)
	for index, trackIndex := range indexes {
		if err := budget.charge(1); err != nil {
			return err
		}
		amountUnsigned := uint64(amountInt) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		weight := uint64(weights[index])
		shareUnsigned := (amountUnsigned/total)*weight + (amountUnsigned%total)*weight/total
		if shareUnsigned > uint64(MaxFixed) {
			return ErrGeometryOverflow
		}
		share := int64(shareUnsigned)
		next, err := sizes[trackIndex].Add(Fixed(share))
		if err != nil {
			return err
		}
		sizes[trackIndex] = next
		allocated += share
	}
	remainder := amountInt - allocated
	for index := int64(0); index < remainder; index++ {
		if err := budget.charge(1); err != nil {
			return err
		}
		trackIndex := indexes[index%int64(len(indexes))]
		next, err := sizes[trackIndex].Add(1)
		if err != nil {
			return err
		}
		sizes[trackIndex] = next
	}
	return nil
}

func gridAxisUsed(sizes []Fixed, gap Fixed, budget *gridBudget) (Fixed, error) {
	if err := budget.charge(uint64(len(sizes))); err != nil {
		return 0, err
	}
	used, err := addFixed(sizes...)
	if err != nil || len(sizes) < 2 {
		return used, err
	}
	gaps, err := gap.MulInt(int64(len(sizes) - 1))
	if err != nil {
		return 0, err
	}
	return used.Add(gaps)
}

func gridOffsets(origin Fixed, sizes []Fixed, gap Fixed, budget *gridBudget) ([]Fixed, error) {
	offsets := make([]Fixed, len(sizes))
	cursor := origin
	for index, size := range sizes {
		if err := budget.charge(1); err != nil {
			return nil, err
		}
		offsets[index] = cursor
		var err error
		cursor, err = cursor.Add(size)
		if err != nil {
			return nil, err
		}
		if index+1 < len(sizes) {
			cursor, err = cursor.Add(gap)
			if err != nil {
				return nil, err
			}
		}
	}
	return offsets, nil
}

func gridSpanExtent(sizes []Fixed, gap Fixed, start, span uint32, budget *gridBudget) (Fixed, error) {
	end := uint64(start) + uint64(span)
	if span == 0 || end > uint64(len(sizes)) {
		return 0, ErrGridPlacement
	}
	if err := budget.charge(uint64(span)); err != nil {
		return 0, err
	}
	extent, err := addFixed(sizes[start:uint32(end)]...) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	if err != nil || span == 1 {
		return extent, err
	}
	gaps, err := gap.MulInt(int64(span - 1))
	if err != nil {
		return 0, err
	}
	return extent.Add(gaps)
}
