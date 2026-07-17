// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrParagraphHasNoLines              = errors.New("layoutengine: paragraph has no lines")
	ErrParagraphLineExtent              = errors.New("layoutengine: paragraph line has an invalid extent")
	ErrParagraphPolicy                  = errors.New("layoutengine: paragraph pagination policy is invalid")
	ErrParagraphToken                   = errors.New("layoutengine: paragraph break token is invalid")
	ErrParagraphSpace                   = errors.New("layoutengine: paragraph fragment space is invalid")
	ErrParagraphConstraintUnsatisfiable = errors.New("layoutengine: paragraph pagination constraint is unsatisfiable")
)

// ParagraphBreakMode controls whether impossible widow/orphan constraints are
// relaxed with a warning or stop planning. Both modes still emit an individual
// oversized line once on an empty region so pagination cannot loop forever.
type ParagraphBreakMode uint8

const (
	ParagraphBreakPrefer ParagraphBreakMode = iota + 1
	ParagraphBreakStrict
)

func (m ParagraphBreakMode) valid() bool {
	return m == ParagraphBreakPrefer || m == ParagraphBreakStrict
}

// ParagraphLineInput is an already-shaped and already-broken line metric.
// Baseline is measured down from the line's top. Text and glyph data remain in
// future immutable resource tables; the pagination kernel only owns geometry
// and provenance.
type ParagraphLineInput struct {
	OffsetX  Fixed      `json:"offset_x"`
	Width    Fixed      `json:"width"`
	Height   Fixed      `json:"height"`
	Baseline Fixed      `json:"baseline"`
	Source   SourceSpan `json:"source"`
}

// ParagraphLinePlanInput is copied by NewParagraphLinePlan. Orphans and
// Widows are required positive line counts so policy is never implicit.
type ParagraphLinePlanInput struct {
	Node     NodeID
	Key      NodeKey
	Instance InstanceID
	Source   SourceSpan
	Lines    []ParagraphLineInput
	Orphans  uint32
	Widows   uint32
	Mode     ParagraphBreakMode
}

// ParagraphLinePlan is immutable after construction and safe for concurrent
// fragmentation. Its fingerprint binds continuation tokens to exact line
// metrics, provenance, identity, and policy.
type ParagraphLinePlan struct {
	node        NodeID
	key         NodeKey
	instance    InstanceID
	source      SourceSpan
	lines       []ParagraphLineInput
	orphans     uint32
	widows      uint32
	mode        ParagraphBreakMode
	fingerprint [sha256.Size]byte
}

// ParagraphBreakToken is an opaque, immutable resume point. A token from a
// different paragraph plan, or from a completed plan, is rejected.
type ParagraphBreakToken struct {
	fingerprint    [sha256.Size]byte
	nextLine       uint32
	leadingMinimum uint32
}

// NextLine reports the zero-based index of the next unconsumed line.
func (t ParagraphBreakToken) NextLine() uint32 { return t.nextLine }

// ParagraphFragmentSpace describes one region attempt. Available is the
// current remaining height. RegionEmpty tells the kernel whether deferral is
// legal. NextRegionCapacity is required lookahead for variable-height widow
// lines and may differ under future page masters.
type ParagraphFragmentSpace struct {
	Available          Fixed
	RegionEmpty        bool
	NextRegionCapacity Fixed
}

// ParagraphFragmentAction distinguishes content progress from a request that
// the enclosing flow advance to its next region without consuming the token.
type ParagraphFragmentAction uint8

const (
	ParagraphPlace ParagraphFragmentAction = iota + 1
	ParagraphDefer
)

// ParagraphFragmentResult is a deterministic answer for one fragmentation
// attempt. A placement always has a non-empty Lines range and advances Next,
// unless Done is true. A deferral preserves the supplied token.
type ParagraphFragmentResult struct {
	Action         ParagraphFragmentAction
	Lines          IndexRange
	Height         Fixed
	Continuation   FragmentContinuation
	Next           ParagraphBreakToken
	Done           bool
	OversizedLine  bool
	RelaxedOrphans bool
	RelaxedWidows  bool
	OrphansApplied uint32
	WidowsApplied  uint32
	PolicyBreak    bool
	BreakRequired  Fixed
}

// ParagraphFlowInput plans one paragraph through a uniform page body. It is a
// private-engine integration fixture, not a production document entry point.
type ParagraphFlowInput struct {
	PageSize Size
	Body     Rect
	ParagraphLinePlanInput
}

// NewParagraphLinePlan validates and takes an immutable copy of prebroken line
// metrics. Wrapping and shaping deliberately happen before this boundary.
func NewParagraphLinePlan(input ParagraphLinePlanInput) (ParagraphLinePlan, error) {
	return NewParagraphLinePlanContext(context.Background(), input)
}

// NewParagraphLinePlanContext validates and fingerprints the paragraph while
// charging the cumulative request meter carried by ctx.
func NewParagraphLinePlanContext(ctx context.Context, input ParagraphLinePlanInput) (ParagraphLinePlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ChargePlanningWork(ctx, "paragraph planning", uint64(len(input.Lines))+1); err != nil {
		return ParagraphLinePlan{}, err
	}
	if !input.Node.Valid() {
		return ParagraphLinePlan{}, errors.New("layoutengine: paragraph has an absent node ID")
	}
	if err := validateTextIdentity("paragraph node key", string(input.Key)); err != nil {
		return ParagraphLinePlan{}, err
	}
	if err := validateTextIdentity("paragraph instance ID", string(input.Instance)); err != nil {
		return ParagraphLinePlan{}, err
	}
	if err := input.Source.Validate(); err != nil {
		return ParagraphLinePlan{}, fmt.Errorf("layoutengine: paragraph source: %w", err)
	}
	if len(input.Lines) == 0 {
		return ParagraphLinePlan{}, ErrParagraphHasNoLines
	}
	if uint64(len(input.Lines)) > uint64(^uint32(0)) {
		return ParagraphLinePlan{}, errors.New("layoutengine: paragraph has too many lines")
	}
	maxInt := uint64(^uint(0) >> 1)
	if input.Orphans == 0 || input.Widows == 0 ||
		uint64(input.Orphans) > maxInt || uint64(input.Widows) > maxInt || !input.Mode.valid() {
		return ParagraphLinePlan{}, ErrParagraphPolicy
	}
	lines := cloneSlice(input.Lines)
	var total Fixed
	for i, line := range lines {
		if line.Width < 0 || line.Height <= 0 || line.Baseline < 0 || line.Baseline > line.Height {
			return ParagraphLinePlan{}, fmt.Errorf("layoutengine: paragraph line %d: %w", i, ErrParagraphLineExtent)
		}
		if err := line.Source.Validate(); err != nil {
			return ParagraphLinePlan{}, fmt.Errorf("layoutengine: paragraph line %d source: %w", i, err)
		}
		var err error
		total, err = total.Add(line.Height)
		if err != nil {
			return ParagraphLinePlan{}, fmt.Errorf("layoutengine: paragraph line heights: %w", err)
		}
	}
	plan := ParagraphLinePlan{
		node: input.Node, key: input.Key, instance: input.Instance, source: input.Source,
		lines: lines, orphans: input.Orphans, widows: input.Widows, mode: input.Mode,
	}
	encoded, err := json.Marshal(struct {
		Node     NodeID               `json:"node"`
		Key      NodeKey              `json:"key"`
		Instance InstanceID           `json:"instance"`
		Source   SourceSpan           `json:"source"`
		Lines    []ParagraphLineInput `json:"lines"`
		Orphans  uint32               `json:"orphans"`
		Widows   uint32               `json:"widows"`
		Mode     ParagraphBreakMode   `json:"mode"`
	}{input.Node, input.Key, input.Instance, input.Source, lines, input.Orphans, input.Widows, input.Mode})
	if err != nil {
		return ParagraphLinePlan{}, fmt.Errorf("layoutengine: paragraph fingerprint: %w", err)
	}
	plan.fingerprint = sha256.Sum256(encoded)
	return plan, nil
}

// Start returns the only valid initial token for this immutable plan.
func (p ParagraphLinePlan) Start() ParagraphBreakToken {
	return ParagraphBreakToken{fingerprint: p.fingerprint}
}

// Fragment selects the largest legal prefix for the supplied space. Preferred
// constraints first preserve the widow obligation at the next region, then
// relax bottom orphans/leading minima when no legal split exists. Strict mode
// returns a structured error instead of relaxing.
func (p ParagraphLinePlan) Fragment(space ParagraphFragmentSpace, token ParagraphBreakToken) (ParagraphFragmentResult, error) {
	return p.FragmentContext(context.Background(), space, token)
}

// FragmentContext performs one resumable attempt against the same request
// budget as its enclosing document planner.
func (p ParagraphLinePlan) FragmentContext(ctx context.Context, space ParagraphFragmentSpace, token ParagraphBreakToken) (ParagraphFragmentResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workAmount := uint64(1)
	if token.nextLine < uint32(len(p.lines)) {
		workAmount += uint64(len(p.lines)) - uint64(token.nextLine)
	}
	if err := ChargePlanningWork(ctx, "paragraph fragmentation", workAmount); err != nil {
		return ParagraphFragmentResult{}, err
	}
	if token.fingerprint != p.fingerprint || token.nextLine >= uint32(len(p.lines)) {
		return ParagraphFragmentResult{}, ErrParagraphToken
	}
	if space.Available < 0 || space.NextRegionCapacity <= 0 {
		return ParagraphFragmentResult{}, ErrParagraphSpace
	}

	start := int(token.nextLine)
	remaining := len(p.lines) - start
	fitCount, fitHeight, err := p.fittingPrefix(start, space.Available)
	if err != nil {
		return ParagraphFragmentResult{}, err
	}
	if fitCount == 0 {
		if !space.RegionEmpty {
			return ParagraphFragmentResult{Action: ParagraphDefer, Next: token}, nil
		}
		line := p.lines[start]
		relaxedOrphans := remaining > 1 && p.orphans > 1
		relaxedWidows := remaining > 1 && token.leadingMinimum > 1
		preserveNextWidows := true
		if remaining > 1 {
			preserveNextWidows = remaining-1 >= int(p.widows)
			if preserveNextWidows {
				widowHeight, err := p.lineHeight(start+1, int(p.widows))
				if err != nil {
					return ParagraphFragmentResult{}, err
				}
				preserveNextWidows = widowHeight <= space.NextRegionCapacity
			}
			relaxedWidows = relaxedWidows || !preserveNextWidows
		}
		widowsApplied := p.widows
		if relaxedWidows {
			widowsApplied = 1
		}
		return p.placedResult(token, 1, line.Height, true, relaxedOrphans, relaxedWidows,
			preserveNextWidows, 1, widowsApplied, false, 0), nil
	}
	if fitCount == remaining {
		return p.placedResult(token, fitCount, fitHeight, false, false, false,
			true, p.orphans, p.widows, false, 0), nil
	}

	selected, required, ok, err := p.legalSplit(start, fitCount, token.leadingMinimum, space.NextRegionCapacity)
	if err != nil {
		return ParagraphFragmentResult{}, err
	}
	if ok {
		height, err := p.lineHeight(start, selected)
		if err != nil {
			return ParagraphFragmentResult{}, err
		}
		return p.placedResult(token, selected, height, false, false, false,
			true, p.orphans, p.widows, selected < fitCount, required), nil
	}
	if !space.RegionEmpty {
		return ParagraphFragmentResult{Action: ParagraphDefer, Next: token}, nil
	}
	if p.mode == ParagraphBreakStrict {
		return ParagraphFragmentResult{}, p.unsatisfiableError(token)
	}

	// Preferred mode preserves the next-region widow group before relaxing the
	// first-fragment orphan or carried leading-minimum requirement.
	selected, required, widowOK, err := p.splitPreservingWidows(start, fitCount, space.NextRegionCapacity)
	if err != nil {
		return ParagraphFragmentResult{}, err
	}
	relaxedOrphans := selected < int(p.orphans)
	relaxedLeading := token.leadingMinimum > 0 && selected < int(token.leadingMinimum)
	relaxedWidows := !widowOK
	if !widowOK {
		selected = fitCount
		required, err = p.lineHeight(start+selected, remaining-selected)
		if err != nil {
			return ParagraphFragmentResult{}, err
		}
		relaxedOrphans = selected < int(p.orphans)
		relaxedLeading = token.leadingMinimum > 0 && selected < int(token.leadingMinimum)
	}
	height, err := p.lineHeight(start, selected)
	if err != nil {
		return ParagraphFragmentResult{}, err
	}
	widowsApplied := p.widows
	if relaxedLeading && uint32(selected) < widowsApplied {
		widowsApplied = uint32(selected)
	}
	if !widowOK {
		widowsApplied = 1
	}
	orphansApplied := p.orphans
	if relaxedOrphans {
		orphansApplied = uint32(selected)
	}
	return p.placedResult(token, selected, height, false, relaxedOrphans, relaxedWidows || relaxedLeading,
		widowOK, orphansApplied, widowsApplied, selected < fitCount, required), nil
}

func (p ParagraphLinePlan) fittingPrefix(start int, available Fixed) (int, Fixed, error) {
	var height Fixed
	count := 0
	for i := start; i < len(p.lines); i++ {
		next, err := height.Add(p.lines[i].Height)
		if err != nil {
			return 0, 0, err
		}
		if next > available {
			break
		}
		height = next
		count++
	}
	return count, height, nil
}

func (p ParagraphLinePlan) legalSplit(start, fitCount int, leading uint32, nextCapacity Fixed) (int, Fixed, bool, error) {
	minimumBottom := int(leading)
	if int(p.orphans) > minimumBottom {
		minimumBottom = int(p.orphans)
	}
	for count := fitCount; count >= 1; count-- {
		if count < minimumBottom || len(p.lines)-(start+count) < int(p.widows) {
			continue
		}
		widowHeight, err := p.lineHeight(start+count, int(p.widows))
		if err != nil {
			return 0, 0, false, err
		}
		if widowHeight > nextCapacity {
			continue
		}
		required, err := p.lineHeight(start+count, len(p.lines)-(start+count))
		return count, required, true, err
	}
	return 0, 0, false, nil
}

func (p ParagraphLinePlan) splitPreservingWidows(start, fitCount int, nextCapacity Fixed) (int, Fixed, bool, error) {
	for count := fitCount; count >= 1; count-- {
		if len(p.lines)-(start+count) < int(p.widows) {
			continue
		}
		widowHeight, err := p.lineHeight(start+count, int(p.widows))
		if err != nil {
			return 0, 0, false, err
		}
		if widowHeight > nextCapacity {
			continue
		}
		required, err := p.lineHeight(start+count, len(p.lines)-(start+count))
		return count, required, true, err
	}
	return fitCount, 0, false, nil
}

func (p ParagraphLinePlan) lineHeight(start, count int) (Fixed, error) {
	var height Fixed
	for i := start; i < start+count; i++ {
		var err error
		height, err = height.Add(p.lines[i].Height)
		if err != nil {
			return 0, err
		}
	}
	return height, nil
}

func (p ParagraphLinePlan) placedResult(token ParagraphBreakToken, count int, height Fixed, oversized, relaxedOrphans, relaxedWidows, preserveNextWidows bool, orphansApplied, widowsApplied uint32, policyBreak bool, required Fixed) ParagraphFragmentResult {
	start := token.nextLine
	end := start + uint32(count)
	done := end == uint32(len(p.lines))
	continuation := ContinuationMiddle
	switch {
	case start == 0 && done:
		continuation = ContinuationWhole
	case start == 0:
		continuation = ContinuationStart
	case done:
		continuation = ContinuationEnd
	}
	result := ParagraphFragmentResult{
		Action: ParagraphPlace, Lines: IndexRange{Start: start, Count: uint32(count)},
		Height: height, Continuation: continuation, Done: done, OversizedLine: oversized,
		RelaxedOrphans: relaxedOrphans, RelaxedWidows: relaxedWidows,
		OrphansApplied: orphansApplied, WidowsApplied: widowsApplied,
		PolicyBreak: policyBreak, BreakRequired: required,
	}
	if !done {
		leading := uint32(1)
		if preserveNextWidows {
			leading = p.widows
		}
		result.Next = ParagraphBreakToken{fingerprint: p.fingerprint, nextLine: end, leadingMinimum: leading}
	}
	return result
}

func (p ParagraphLinePlan) unsatisfiableError(token ParagraphBreakToken) error {
	diagnostic := Diagnostic{
		Code: DiagnosticParagraphConstraintUnsatisfiable, Severity: SeverityError, Stage: StageLayout,
		Message:  "strict paragraph widow/orphan constraints cannot be satisfied on an empty region",
		Location: DiagnosticLocation{Node: p.node, Key: p.key, Source: p.source, Instance: p.instance, Region: RegionBody},
		Evidence: []DiagnosticEvidence{
			{Key: "next_line", Value: strconv.FormatUint(uint64(token.nextLine), 10)},
			{Key: "orphans_requested", Value: strconv.FormatUint(uint64(p.orphans), 10)},
			{Key: "widows_requested", Value: strconv.FormatUint(uint64(p.widows), 10)},
		},
	}
	return newPlanningError(ErrParagraphConstraintUnsatisfiable, diagnostic)
}

// PlanParagraphFlow turns the pure resumable kernel into one canonical
// LayoutPlan with exact line geometry. It emits no glyph commands and never
// touches document.Document.
func PlanParagraphFlow(input ParagraphFlowInput) (LayoutPlan, error) {
	return PlanParagraphFlowContext(context.Background(), input)
}

// PlanParagraphFlowContext propagates cancellation and one cumulative request
// work meter through validation and every fragmentation attempt.
func PlanParagraphFlowContext(ctx context.Context, input ParagraphFlowInput) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateParagraphFlowGeometry(input.PageSize, input.Body); err != nil {
		return LayoutPlan{}, err
	}
	paragraph, err := NewParagraphLinePlanContext(ctx, input.ParagraphLinePlanInput)
	if err != nil {
		return LayoutPlan{}, err
	}

	planInput := LayoutPlanInput{}
	token := paragraph.Start()
	var previous *ParagraphFragmentResult
	var firstRelaxed FragmentID
	var relaxedOrphans, relaxedWidows bool
	var firstRelaxedOrphansApplied, firstRelaxedWidowsApplied uint32
	for {
		result, err := paragraph.FragmentContext(ctx, ParagraphFragmentSpace{
			Available: input.Body.Height, RegionEmpty: true, NextRegionCapacity: input.Body.Height,
		}, token)
		if err != nil {
			var planning *PlanningError
			if errors.As(err, &planning) && errors.Is(err, ErrParagraphConstraintUnsatisfiable) {
				diagnostic := cloneDiagnostic(planning.Diagnostic)
				diagnostic.Location.Page = uint32(len(planInput.Pages) + 1)
				diagnostic.Location.Bounds = input.Body
				diagnostic.Location.HasBounds = true
				return LayoutPlan{}, newPlanningError(ErrParagraphConstraintUnsatisfiable, diagnostic)
			}
			return LayoutPlan{}, err
		}
		if result.Action != ParagraphPlace || result.Lines.Count == 0 {
			return LayoutPlan{}, errors.New("layoutengine: paragraph made no progress on an empty body")
		}

		pageNumber := uint32(len(planInput.Pages) + 1)
		fragmentID := FragmentID(len(planInput.Fragments) + 1)
		box, err := NewRect(input.Body.X, input.Body.Y, input.Body.Width, result.Height)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("layoutengine: paragraph fragment box: %w", err)
		}
		lineStart := len(planInput.Lines)
		y := input.Body.Y
		for index := result.Lines.Start; index < result.Lines.Start+result.Lines.Count; index++ {
			lineInput := paragraph.lines[index]
			x, err := input.Body.X.Add(lineInput.OffsetX)
			if err != nil {
				return LayoutPlan{}, fmt.Errorf("layoutengine: paragraph line %d x offset: %w", index, err)
			}
			bounds, err := NewRect(x, y, lineInput.Width, lineInput.Height)
			if err != nil {
				return LayoutPlan{}, fmt.Errorf("layoutengine: paragraph line %d bounds: %w", index, err)
			}
			baseline, err := y.Add(lineInput.Baseline)
			if err != nil {
				return LayoutPlan{}, fmt.Errorf("layoutengine: paragraph line %d baseline: %w", index, err)
			}
			planInput.Lines = append(planInput.Lines, PlannedLine{
				Fragment: fragmentID, Index: index, Bounds: bounds, Baseline: baseline, Source: lineInput.Source,
			})
			y, err = y.Add(lineInput.Height)
			if err != nil {
				return LayoutPlan{}, fmt.Errorf("layoutengine: paragraph line %d cursor: %w", index, err)
			}
		}
		planInput.Fragments = append(planInput.Fragments, Fragment{
			ID: fragmentID, Node: paragraph.node, Key: paragraph.key, Instance: paragraph.instance,
			Page: pageNumber, Region: RegionBody, BorderBox: box, ContentBox: box,
			Source: paragraph.source, Continuation: result.Continuation,
		})
		planInput.Pages = append(planInput.Pages, PlannedPage{
			Number: pageNumber, Size: input.PageSize,
			Fragments: IndexRange{Start: uint32(len(planInput.Fragments) - 1), Count: 1},
			Lines:     IndexRange{Start: uint32(lineStart), Count: result.Lines.Count},
		})

		if previous != nil {
			available := input.Body.Height - previous.Height
			reason := BreakInsufficientRemainingBodySpace
			required := paragraph.lines[result.Lines.Start].Height
			if previous.OversizedLine {
				reason = BreakPreviousFragmentOverflow
				available = 0
			} else if previous.PolicyBreak {
				reason = BreakPaginationConstraint
				required = previous.BreakRequired
			}
			planInput.Breaks = append(planInput.Breaks, BreakDecision{
				Reason: reason, FromPage: pageNumber - 1, ToPage: pageNumber, Region: RegionBody,
				Preceding: fragmentID - 1, Triggering: fragmentID, Required: required, Available: available,
			})
		}
		if result.OversizedLine {
			overflow, err := result.Height.Sub(input.Body.Height)
			if err != nil {
				return LayoutPlan{}, err
			}
			planInput.Diagnostics = append(planInput.Diagnostics, Diagnostic{
				Code: DiagnosticUnbreakableTooTall, Severity: SeverityWarning, Stage: StageLayout,
				Message: "indivisible paragraph line exceeds the page body height and was emitted once",
				Location: DiagnosticLocation{Node: paragraph.node, Key: paragraph.key, Source: paragraph.source,
					Instance: paragraph.instance, Fragment: fragmentID, Page: pageNumber, Region: RegionBody,
					Bounds: box, HasBounds: true},
				Evidence: []DiagnosticEvidence{
					{Key: "body_height_fixed", Value: strconv.FormatInt(int64(input.Body.Height), 10)},
					{Key: "line_height_fixed", Value: strconv.FormatInt(int64(result.Height), 10)},
					{Key: "line_index", Value: strconv.FormatUint(uint64(result.Lines.Start), 10)},
					{Key: "overflow_fixed", Value: strconv.FormatInt(int64(overflow), 10)},
				},
			})
		}
		if result.RelaxedOrphans || result.RelaxedWidows {
			if !firstRelaxed.Valid() {
				firstRelaxed = fragmentID
				firstRelaxedOrphansApplied = paragraph.orphans
				firstRelaxedWidowsApplied = paragraph.widows
			}
			if result.RelaxedOrphans && result.OrphansApplied < firstRelaxedOrphansApplied {
				firstRelaxedOrphansApplied = result.OrphansApplied
			}
			if result.RelaxedWidows && result.WidowsApplied < firstRelaxedWidowsApplied {
				firstRelaxedWidowsApplied = result.WidowsApplied
			}
			relaxedOrphans = relaxedOrphans || result.RelaxedOrphans
			relaxedWidows = relaxedWidows || result.RelaxedWidows
		}
		if result.Done {
			break
		}
		copyResult := result
		previous = &copyResult
		token = result.Next
	}

	if firstRelaxed.Valid() {
		fragment := planInput.Fragments[firstRelaxed-1]
		planInput.Diagnostics = append(planInput.Diagnostics, Diagnostic{
			Code: DiagnosticParagraphConstraintRelaxed, Severity: SeverityWarning, Stage: StageLayout,
			Message: "preferred paragraph pagination constraints were relaxed to guarantee progress",
			Location: DiagnosticLocation{Node: paragraph.node, Key: paragraph.key, Source: paragraph.source,
				Instance: paragraph.instance, Fragment: firstRelaxed, Page: fragment.Page, Region: RegionBody,
				Bounds: fragment.BorderBox, HasBounds: true},
			Evidence: []DiagnosticEvidence{
				{Key: "line_count", Value: strconv.Itoa(len(paragraph.lines))},
				{Key: "orphans_applied", Value: strconv.FormatUint(uint64(firstRelaxedOrphansApplied), 10)},
				{Key: "orphans_relaxed", Value: strconv.FormatBool(relaxedOrphans)},
				{Key: "orphans_requested", Value: strconv.FormatUint(uint64(paragraph.orphans), 10)},
				{Key: "widows_applied", Value: strconv.FormatUint(uint64(firstRelaxedWidowsApplied), 10)},
				{Key: "widows_relaxed", Value: strconv.FormatBool(relaxedWidows)},
				{Key: "widows_requested", Value: strconv.FormatUint(uint64(paragraph.widows), 10)},
			},
		})
	}
	return NewLayoutPlan(planInput)
}

func validateParagraphFlowGeometry(pageSize Size, body Rect) error {
	if err := pageSize.Validate(); err != nil {
		return fmt.Errorf("layoutengine: paragraph page size: %w", err)
	}
	if pageSize.IsEmpty() {
		return ErrFlowPageSizeEmpty
	}
	if err := body.Validate(); err != nil {
		return fmt.Errorf("layoutengine: paragraph body rectangle: %w", err)
	}
	if body.IsEmpty() {
		return newPlanningError(ErrFlowBodyEmpty, Diagnostic{
			Code: DiagnosticPageRegionNoBodySpace, Severity: SeverityError, Stage: StageLayout,
			Message:  "page body region has no usable space",
			Location: DiagnosticLocation{Region: RegionBody, Bounds: body, HasBounds: true},
		})
	}
	if body.X < 0 || body.Y < 0 {
		return ErrFlowBodyOutsidePage
	}
	right, err := body.Right()
	if err != nil {
		return err
	}
	bottom, err := body.Bottom()
	if err != nil {
		return err
	}
	if right > pageSize.Width || bottom > pageSize.Height {
		return ErrFlowBodyOutsidePage
	}
	return nil
}
