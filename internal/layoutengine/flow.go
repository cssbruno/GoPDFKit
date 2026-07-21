// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

var (
	// ErrFlowPageSizeEmpty reports a page with no usable extent.
	ErrFlowPageSizeEmpty = errors.New("layoutengine: flow page size is empty")
	// ErrFlowBodyEmpty reports a body region with no usable extent.
	ErrFlowBodyEmpty = errors.New("layoutengine: flow body rectangle is empty")
	// ErrFlowBodyOutsidePage reports a body region that is not wholly on its
	// fixed-size page.
	ErrFlowBodyOutsidePage = errors.New("layoutengine: flow body rectangle lies outside the page")
	// ErrFlowBlockNegativeHeight reports a block whose requested extent is
	// invalid before pagination begins.
	ErrFlowBlockNegativeHeight = errors.New("layoutengine: flow block height is negative")
	ErrFlowLimitsInvalid       = errors.New("layoutengine: flow limits are invalid")
	ErrFlowWorkLimit           = errors.New("layoutengine: flow work limit exceeded")
	ErrFlowStateLimit          = errors.New("layoutengine: flow state limit exceeded")
)

const (
	hardMaxFlowBlocks uint32 = 1_000_000
	hardMaxFlowPages  uint32 = 1_000_000
	hardMaxFlowWork   uint64 = 8_000_000
)

// PlanningError couples a planner failure with the structured diagnostic that
// explains it. It unwraps to its sentinel cause so callers can continue to
// classify known failures with errors.Is.
type PlanningError struct {
	Diagnostic Diagnostic
	cause      error
}

func (e *PlanningError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("layoutengine: planning error %s: %v", e.Diagnostic.Code, e.cause)
}

// Unwrap exposes the stable sentinel cause for errors.Is and errors.As.
func (e *PlanningError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// VerticalFlowBlock is one already-measured, indivisible block in a vertical
// flow. It deliberately has no style, wrapping, or painter behavior: this
// first planner slice exists to pin down pagination and plan ownership.
//
// Node, Key, and Instance are the identity triple required by LayoutPlan.
// Source is optional for generated blocks. Height is a fixed-point border-box
// height; width is supplied by the body's fixed width.
type VerticalFlowBlock struct {
	Node     NodeID
	Key      NodeKey
	Instance InstanceID
	Source   SourceSpan
	Height   Fixed
	// Repeated preserves explicit repeated/nested-region evidence on the
	// resulting fragment; it does not alter pagination.
	Repeated bool
}

// VerticalFlowInput is the complete fixed geometry and measured block input
// for PlanVerticalFlow. The body must be non-empty and wholly contained in the
// page surface whose origin is (0, 0).
type VerticalFlowInput struct {
	PageSize Size
	Body     Rect
	Blocks   []VerticalFlowBlock
}

// VerticalFlowLimits bounds retained fragments/pages and deterministic work.
// The zero value selects conservative hard-capped defaults; partial values are
// rejected so callers cannot accidentally leave one dimension unbounded.
type VerticalFlowLimits struct {
	MaxBlocks uint32
	MaxPages  uint32
	MaxWork   uint64
}

func DefaultVerticalFlowLimits() VerticalFlowLimits {
	return VerticalFlowLimits{MaxBlocks: hardMaxFlowBlocks, MaxPages: hardMaxFlowPages, MaxWork: hardMaxFlowWork}
}

type verticalFlowBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *verticalFlowBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "vertical flow planning", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{Code: DiagnosticCanceled, Severity: SeverityError,
			Stage: StageLayout, Message: "vertical flow planning was canceled"})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrFlowWorkLimit, Diagnostic{Code: DiagnosticWorkLimit, Severity: SeverityError,
			Stage: StageLayout, Message: "vertical flow planning exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(budget.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)}}})
	}
	budget.used += amount
	return nil
}

// PlanVerticalFlow creates a deterministic, indivisible vertical flow plan.
// Each input block becomes exactly one whole fragment. This position-only
// prototype intentionally emits no display commands: a later painter slice
// must add real command payloads rather than a fake stand-in. A block that is
// one fixed unit too large for the remaining body starts on the next page. A
// block taller than an empty body is emitted once on a fresh page with
// UNBREAKABLE_TOO_TALL rather than causing blank-page retry loops.
//
// An empty block list produces one valid empty page.
func PlanVerticalFlow(input VerticalFlowInput) (LayoutPlan, error) {
	return PlanVerticalFlowContext(context.Background(), input, VerticalFlowLimits{})
}

// PlanVerticalFlowContext is the bounded, cancellation-aware vertical-flow
// entry point. Cancellation and budget failures carry structured diagnostics.
func PlanVerticalFlowContext(ctx context.Context, input VerticalFlowInput, limits VerticalFlowLimits) (LayoutPlan, error) {
	token, err := StartVerticalFlowContext(ctx, input, limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	chunk := uint32(len(input.Blocks))
	if chunk == 0 {
		chunk = 1
	}
	result, err := ResumeVerticalFlowContext(ctx, input, token, chunk, limits)
	if err != nil {
		return LayoutPlan{}, err
	}
	return result.Plan, nil
}

func normalizeVerticalFlowLimits(limits VerticalFlowLimits) (VerticalFlowLimits, error) {
	if limits == (VerticalFlowLimits{}) {
		return DefaultVerticalFlowLimits(), nil
	}
	if limits.MaxBlocks == 0 || limits.MaxPages == 0 || limits.MaxWork == 0 ||
		limits.MaxBlocks > hardMaxFlowBlocks || limits.MaxPages > hardMaxFlowPages || limits.MaxWork > hardMaxFlowWork {
		return VerticalFlowLimits{}, ErrFlowLimitsInvalid
	}
	return limits, nil
}

func validateVerticalFlowInput(input VerticalFlowInput) error {
	limits := DefaultVerticalFlowLimits()
	budget := verticalFlowBudget{ctx: context.Background(), limit: limits.MaxWork}
	return validateVerticalFlowInputBounded(input, limits, &budget)
}

func validateVerticalFlowInputBounded(input VerticalFlowInput, limits VerticalFlowLimits, budget *verticalFlowBudget) error {
	if err := input.PageSize.Validate(); err != nil {
		return fmt.Errorf("layoutengine: flow page size: %w", err)
	}
	if input.PageSize.IsEmpty() {
		return ErrFlowPageSizeEmpty
	}
	if err := input.Body.Validate(); err != nil {
		return fmt.Errorf("layoutengine: flow body rectangle: %w", err)
	}
	if input.Body.IsEmpty() {
		return newPlanningError(ErrFlowBodyEmpty, Diagnostic{
			Code:     DiagnosticPageRegionNoBodySpace,
			Severity: SeverityError,
			Stage:    StageLayout,
			Message:  "page body region has no usable space",
			Location: DiagnosticLocation{
				Region:    RegionBody,
				Bounds:    input.Body,
				HasBounds: true,
			},
		})
	}
	if input.Body.X < 0 || input.Body.Y < 0 {
		return ErrFlowBodyOutsidePage
	}
	right, err := input.Body.Right()
	if err != nil {
		return fmt.Errorf("layoutengine: flow body right edge: %w", err)
	}
	bottom, err := input.Body.Bottom()
	if err != nil {
		return fmt.Errorf("layoutengine: flow body bottom edge: %w", err)
	}
	if right > input.PageSize.Width || bottom > input.PageSize.Height {
		return ErrFlowBodyOutsidePage
	}
	if uint64(len(input.Blocks)) > uint64(limits.MaxBlocks) {
		return newPlanningError(ErrFlowStateLimit, Diagnostic{Code: DiagnosticResourceLimit, Severity: SeverityError,
			Stage: StageLayout, Message: "vertical flow block count exceeds its state limit"})
	}
	for index, block := range input.Blocks {
		if err := budget.charge(1); err != nil {
			return err
		}
		if err := block.validate(); err != nil {
			return fmt.Errorf("layoutengine: flow block %d: %w", index, err)
		}
	}
	return nil
}

func newPlanningError(cause error, diagnostic Diagnostic) error {
	if err := diagnostic.Validate(); err != nil {
		return fmt.Errorf("layoutengine: invalid internal planning diagnostic: %w", err)
	}
	return &PlanningError{Diagnostic: diagnostic, cause: cause}
}

func (block VerticalFlowBlock) validate() error {
	if !block.Node.Valid() {
		return errors.New("has an absent node ID")
	}
	if err := validateTextIdentity("flow block node key", string(block.Key)); err != nil {
		return err
	}
	if !block.Instance.Valid() {
		return errors.New("has an absent instance ID")
	}
	if err := validateTextIdentity("flow block instance ID", string(block.Instance)); err != nil {
		return err
	}
	if err := block.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if block.Height < 0 {
		return ErrFlowBlockNegativeHeight
	}
	return nil
}

type verticalFlowPaginator struct {
	pageSize Size
	body     Rect
	input    LayoutPlanInput
	maxPages uint32
	budget   *verticalFlowBudget

	pageNumber        uint32
	pageFragmentStart int
	pageCommandStart  int
	cursor            Fixed

	// Only positive-height fragments consume body capacity. Keeping this state
	// separate from the fragment count prevents zero-height anchors from
	// creating an otherwise empty page transition.
	hasConsumedPositiveHeight bool
	lastPositiveFragment      FragmentID
	lastPositiveOverflow      bool
}

func (p *verticalFlowPaginator) startPage() error {
	if p.pageNumber >= p.maxPages {
		return newPlanningError(ErrFlowStateLimit, Diagnostic{Code: DiagnosticResourceLimit, Severity: SeverityError,
			Stage: StageLayout, Message: "vertical flow page count exceeds its state limit"})
	}
	if err := p.budget.charge(1); err != nil {
		return err
	}
	p.pageNumber++
	p.pageFragmentStart = len(p.input.Fragments)
	p.pageCommandStart = len(p.input.Commands)
	p.cursor = p.body.Y
	p.hasConsumedPositiveHeight = false
	p.lastPositiveFragment = 0
	p.lastPositiveOverflow = false
	return nil
}

func (p *verticalFlowPaginator) finishPage() {
	p.input.Pages = append(p.input.Pages, PlannedPage{
		Number: p.pageNumber,
		Size:   p.pageSize,
		Fragments: IndexRange{
			Start: uint32(p.pageFragmentStart),
			Count: uint32(len(p.input.Fragments) - p.pageFragmentStart),
		},
		Commands: IndexRange{
			Start: uint32(p.pageCommandStart),
			Count: uint32(len(p.input.Commands) - p.pageCommandStart),
		},
	})
}

func (p *verticalFlowPaginator) remainingHeight() (Fixed, error) {
	bodyBottom, err := p.body.Bottom()
	if err != nil {
		return 0, err
	}
	return bodyBottom.Sub(p.cursor)
}

// advanceForBlock closes the current page and returns the causal decision for
// placing triggering on its successor. It is called only for positive-height
// input, after the current page has consumed positive body space.
func (p *verticalFlowPaginator) advanceForBlock(triggering FragmentID, required, available Fixed) (*BreakDecision, error) {
	if !p.lastPositiveFragment.Valid() {
		return nil, errors.New("positive flow consumption has no preceding fragment")
	}
	decision := &BreakDecision{
		FromPage:   p.pageNumber,
		Region:     RegionBody,
		Preceding:  p.lastPositiveFragment,
		Triggering: triggering,
		Required:   required,
		Available:  available,
	}
	if p.lastPositiveOverflow {
		decision.Reason = BreakPreviousFragmentOverflow
		// A prior overflow makes the rest of the region unavailable. The plan
		// contract represents that as zero, not a negative remaining height.
		decision.Available = 0
	} else {
		if available < 0 {
			return nil, errors.New("negative remaining body height without an overflowing predecessor")
		}
		decision.Reason = BreakInsufficientRemainingBodySpace
	}

	p.finishPage()
	if err := p.startPage(); err != nil {
		return nil, err
	}
	decision.ToPage = p.pageNumber
	return decision, nil
}

func (p *verticalFlowPaginator) place(block VerticalFlowBlock, id FragmentID) error {
	box, err := NewRect(p.body.X, p.cursor, p.body.Width, block.Height)
	if err != nil {
		return fmt.Errorf("border box: %w", err)
	}

	fragment := Fragment{
		ID:           id,
		Node:         block.Node,
		Key:          block.Key,
		Instance:     block.Instance,
		Page:         p.pageNumber,
		Region:       RegionBody,
		BorderBox:    box,
		ContentBox:   box,
		Source:       block.Source,
		Continuation: ContinuationWhole,
		Repeated:     block.Repeated,
	}
	p.input.Fragments = append(p.input.Fragments, fragment)

	if block.Height > p.body.Height {
		overflow, err := block.Height.Sub(p.body.Height)
		if err != nil {
			return fmt.Errorf("overflow amount: %w", err)
		}
		p.input.Diagnostics = append(p.input.Diagnostics, Diagnostic{
			Code:     DiagnosticUnbreakableTooTall,
			Severity: SeverityWarning,
			Stage:    StageLayout,
			Message:  "indivisible block exceeds the page body height and was emitted once",
			Location: DiagnosticLocation{
				Node:      block.Node,
				Key:       block.Key,
				Source:    block.Source,
				Instance:  block.Instance,
				Fragment:  id,
				Page:      p.pageNumber,
				Region:    RegionBody,
				Bounds:    box,
				HasBounds: true,
			},
			Evidence: []DiagnosticEvidence{
				{Key: "block_height_fixed", Value: strconv.FormatInt(int64(block.Height), 10)},
				{Key: "body_height_fixed", Value: strconv.FormatInt(int64(p.body.Height), 10)},
				{Key: "overflow_fixed", Value: strconv.FormatInt(int64(overflow), 10)},
			},
		})
	}

	cursor, err := box.Bottom()
	if err != nil {
		return fmt.Errorf("next cursor: %w", err)
	}
	p.cursor = cursor
	if block.Height > 0 {
		bodyBottom, err := p.body.Bottom()
		if err != nil {
			return fmt.Errorf("body bottom: %w", err)
		}
		p.hasConsumedPositiveHeight = true
		p.lastPositiveFragment = id
		p.lastPositiveOverflow = cursor > bodyBottom
	}
	return nil
}
