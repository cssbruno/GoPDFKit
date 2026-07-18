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

var (
	ErrStackSurfaceInvalid = errors.New("layoutengine: stack surface is invalid")
	ErrStackChildInvalid   = errors.New("layoutengine: stack child is invalid")
	ErrStackLimitsInvalid  = errors.New("layoutengine: stack planning limits are invalid")
	ErrStackWorkLimit      = errors.New("layoutengine: stack planning work limit exceeded")
	ErrStackStateLimit     = errors.New("layoutengine: stack planning state limit exceeded")
)

const (
	hardMaxStackChildren   uint32 = 100_000
	hardMaxStackWork       uint64 = 4_000_000
	hardMaxStackStateBytes uint64 = 64 << 20
	stackChildStateBase    uint64 = 256
)

// StackAlignment places one measured child on an axis. Stretch replaces the
// measured extent on that axis with the container extent before applying the
// explicit offset.
type StackAlignment string

const (
	StackAlignStart   StackAlignment = "start"
	StackAlignCenter  StackAlignment = "center"
	StackAlignEnd     StackAlignment = "end"
	StackAlignStretch StackAlignment = "stretch"
)

func (alignment StackAlignment) valid() bool {
	return alignment == StackAlignStart || alignment == StackAlignCenter ||
		alignment == StackAlignEnd || alignment == StackAlignStretch
}

// StackChild is an already-measured layer. ZIndex is the primary back-to-front
// paint key and PaintOrder is the explicit tie breaker. Exact ties retain input
// order. Overlap is intentional and never causes a diagnostic.
type StackChild struct {
	Node       NodeID
	Key        NodeKey
	Instance   InstanceID
	Source     SourceSpan
	Size       Size
	Horizontal StackAlignment
	Vertical   StackAlignment
	Offset     Point
	ZIndex     int32
	PaintOrder int32
}

// StackPlanInput describes one fixed page and one explicit stack container.
// The container must be non-empty and wholly inside the page.
type StackPlanInput struct {
	PageSize  Size
	Container Rect
	Children  []StackChild
}

// StackPlanLimits bounds retained planner state and deterministic work. The
// zero value selects DefaultStackPlanLimits; partial zero values are rejected.
type StackPlanLimits struct {
	MaxChildren   uint32
	MaxWork       uint64
	MaxStateBytes uint64
}

func DefaultStackPlanLimits() StackPlanLimits {
	return StackPlanLimits{MaxChildren: hardMaxStackChildren, MaxWork: hardMaxStackWork, MaxStateBytes: hardMaxStackStateBytes}
}

type stackBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *stackBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "stack planning", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
			Stage: StageLayout, Message: "stack planning was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrStackWorkLimit, Diagnostic{Code: DiagnosticWorkLimit, Severity: SeverityError,
			Stage: StageLayout, Message: "stack planning exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(budget.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)}}})
	}
	budget.used += amount
	return nil
}

type indexedStackChild struct {
	child StackChild
	index uint32
}

// PlanStack creates a deterministic geometry-only, single-page LayoutPlan.
// Fragment order is canonical paint order: ascending (ZIndex, PaintOrder,
// original input index), so later fragments paint above earlier fragments.
// Placement uses fixed-point arithmetic only and emits one warning for each
// child extending outside the container.
func PlanStack(ctx context.Context, input StackPlanInput, limits StackPlanLimits) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeStackLimits(limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	budget := stackBudget{ctx: ctx, limit: limits.MaxWork}
	if err := validateStackSurface(input.PageSize, input.Container); err != nil {
		return LayoutPlan{}, err
	}
	if uint64(len(input.Children)) > uint64(limits.MaxChildren) {
		return LayoutPlan{}, newPlanningError(ErrStackStateLimit, Diagnostic{Code: DiagnosticResourceLimit,
			Severity: SeverityError, Stage: StageLayout, Message: "stack child count exceeds its state limit"})
	}
	if _, err := validateStackChildren(input.Children, limits.MaxStateBytes, &budget); err != nil {
		return LayoutPlan{}, err
	}

	children := make([]indexedStackChild, len(input.Children))
	for index, child := range input.Children {
		children[index] = indexedStackChild{child: child, index: uint32(index)}
	}
	sortCost := uint64(len(children)) * uint64(bits.Len(uint(len(children))+1)+1)
	if err := budget.charge(sortCost); err != nil {
		return LayoutPlan{}, err
	}
	sort.SliceStable(children, func(left, right int) bool {
		a, b := children[left], children[right]
		if a.child.ZIndex != b.child.ZIndex {
			return a.child.ZIndex < b.child.ZIndex
		}
		if a.child.PaintOrder != b.child.PaintOrder {
			return a.child.PaintOrder < b.child.PaintOrder
		}
		return a.index < b.index
	})
	if err := budget.charge(1); err != nil {
		return LayoutPlan{}, err
	}

	output := LayoutPlanInput{Pages: []PlannedPage{{Number: 1, Size: input.PageSize,
		Fragments: IndexRange{Count: uint32(len(children))}}}, Fragments: make([]Fragment, 0, len(children))}
	for index, indexed := range children {
		if err := budget.charge(1); err != nil {
			return LayoutPlan{}, err
		}
		bounds, err := placeStackChild(input.Container, indexed.child)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: stack child %d placement: %w", indexed.index, err)
		}
		fragment := Fragment{ID: FragmentID(index + 1), Node: indexed.child.Node, Key: indexed.child.Key,
			Instance: indexed.child.Instance, Page: 1, Region: RegionBody, BorderBox: bounds,
			ContentBox: bounds, Source: indexed.child.Source, Continuation: ContinuationWhole}
		output.Fragments = append(output.Fragments, fragment)
		if !rectContains(input.Container, bounds) {
			output.Diagnostics = append(output.Diagnostics, Diagnostic{Code: DiagnosticStackChildOverflow,
				Severity: SeverityWarning, Stage: StageLayout, Message: "stack child extends outside its container",
				Location: DiagnosticLocation{Node: fragment.Node, Key: fragment.Key, Source: fragment.Source,
					Instance: fragment.Instance, Fragment: fragment.ID, Page: 1, Region: RegionBody,
					Bounds: bounds, HasBounds: true},
				Evidence: []DiagnosticEvidence{{Key: "z_index", Value: strconv.FormatInt(int64(indexed.child.ZIndex), 10)},
					{Key: "paint_order", Value: strconv.FormatInt(int64(indexed.child.PaintOrder), 10)}}})
		}
	}
	return NewLayoutPlan(output)
}

func normalizeStackLimits(limits StackPlanLimits) (StackPlanLimits, error) {
	if limits == (StackPlanLimits{}) {
		return DefaultStackPlanLimits(), nil
	}
	if limits.MaxChildren == 0 || limits.MaxWork == 0 || limits.MaxStateBytes == 0 {
		return StackPlanLimits{}, fmt.Errorf("%w: all bounds must be positive", ErrStackLimitsInvalid)
	}
	if limits.MaxChildren > hardMaxStackChildren || limits.MaxWork > hardMaxStackWork || limits.MaxStateBytes > hardMaxStackStateBytes {
		return StackPlanLimits{}, fmt.Errorf("%w: caller bounds exceed implementation hard caps", ErrStackLimitsInvalid)
	}
	return limits, nil
}

func validateStackSurface(page Size, container Rect) error {
	if err := page.Validate(); err != nil || page.IsEmpty() {
		return fmt.Errorf("%w: page size", ErrStackSurfaceInvalid)
	}
	if err := container.Validate(); err != nil || container.IsEmpty() || container.X < 0 || container.Y < 0 {
		return fmt.Errorf("%w: container", ErrStackSurfaceInvalid)
	}
	right, rightErr := container.Right()
	bottom, bottomErr := container.Bottom()
	if rightErr != nil || bottomErr != nil || right > page.Width || bottom > page.Height {
		return fmt.Errorf("%w: container lies outside page", ErrStackSurfaceInvalid)
	}
	return nil
}

func validateStackChildren(children []StackChild, maxState uint64, budget *stackBudget) (uint64, error) {
	var state uint64
	for index, child := range children {
		if err := budget.charge(1); err != nil {
			return 0, err
		}
		if !child.Node.Valid() || !child.Instance.Valid() || !child.Horizontal.valid() || !child.Vertical.valid() {
			return 0, fmt.Errorf("%w: child %d identity or alignment", ErrStackChildInvalid, index)
		}
		if err := validateTextIdentity("stack child node key", string(child.Key)); err != nil {
			return 0, fmt.Errorf("%w: child %d: %w", ErrStackChildInvalid, index, err)
		}
		if err := validateTextIdentity("stack child instance ID", string(child.Instance)); err != nil {
			return 0, fmt.Errorf("%w: child %d: %w", ErrStackChildInvalid, index, err)
		}
		if err := child.Source.Validate(); err != nil {
			return 0, fmt.Errorf("%w: child %d source: %w", ErrStackChildInvalid, index, err)
		}
		if err := child.Size.Validate(); err != nil {
			return 0, fmt.Errorf("%w: child %d size: %w", ErrStackChildInvalid, index, err)
		}
		cost := stackChildStateBase + uint64(len(child.Key)) + uint64(len(child.Instance)) + uint64(len(child.Source.File))
		if cost > maxState-state {
			return 0, newPlanningError(ErrStackStateLimit, Diagnostic{Code: DiagnosticResourceLimit,
				Severity: SeverityError, Stage: StageLayout, Message: "stack planning exceeded its retained-state limit",
				Evidence: []DiagnosticEvidence{{Key: "state_limit_bytes", Value: strconv.FormatUint(maxState, 10)},
					{Key: "state_used_bytes", Value: strconv.FormatUint(state, 10)},
					{Key: "state_requested_bytes", Value: strconv.FormatUint(cost, 10)}}})
		}
		state += cost
	}
	return state, nil
}

func placeStackChild(container Rect, child StackChild) (Rect, error) {
	x, width, err := stackAxis(container.X, container.Width, child.Size.Width, child.Horizontal, child.Offset.X)
	if err != nil {
		return Rect{}, err
	}
	y, height, err := stackAxis(container.Y, container.Height, child.Size.Height, child.Vertical, child.Offset.Y)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(x, y, width, height)
}

func stackAxis(start, containerExtent, childExtent Fixed, alignment StackAlignment, offset Fixed) (Fixed, Fixed, error) {
	extent := childExtent
	delta := Fixed(0)
	remaining, err := containerExtent.Sub(childExtent)
	if err != nil {
		return 0, 0, err
	}
	switch alignment {
	case StackAlignCenter:
		delta, err = remaining.DivInt(2)
	case StackAlignEnd:
		delta = remaining
	case StackAlignStretch:
		extent = containerExtent
	case StackAlignStart:
	default:
		return 0, 0, ErrStackChildInvalid
	}
	if err != nil {
		return 0, 0, err
	}
	position, err := start.Add(delta)
	if err != nil {
		return 0, 0, err
	}
	position, err = position.Add(offset)
	return position, extent, err
}

func rectContains(container, child Rect) bool {
	containerRight, err1 := container.Right()
	containerBottom, err2 := container.Bottom()
	childRight, err3 := child.Right()
	childBottom, err4 := child.Bottom()
	return err1 == nil && err2 == nil && err3 == nil && err4 == nil &&
		child.X >= container.X && child.Y >= container.Y && childRight <= containerRight && childBottom <= containerBottom
}
