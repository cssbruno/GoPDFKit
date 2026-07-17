// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/bits"
	"strconv"
)

var (
	ErrRowColumnDirection     = errors.New("layoutengine: invalid row or column direction")
	ErrRowColumnTrack         = errors.New("layoutengine: invalid row or column track")
	ErrRowColumnAlignment     = errors.New("layoutengine: invalid row or column cross-axis alignment")
	ErrRowColumnJustification = errors.New("layoutengine: invalid row or column main-axis justification")
	ErrRowColumnWrap          = errors.New("layoutengine: invalid row or column wrapping mode")
	ErrRowColumnContentAlign  = errors.New("layoutengine: invalid row or column line alignment")
	ErrRowColumnOverflow      = errors.New("layoutengine: row or column minimum exceeds its region")
	ErrRowColumnWorkLimit     = errors.New("layoutengine: row or column planning work limit exceeded")
	ErrRowColumnResourceLimit = errors.New("layoutengine: row or column retained-state limit exceeded")
	ErrRowColumnLimitsInvalid = errors.New("layoutengine: row or column planning limits are invalid")
)

const (
	hardMaxRowColumnChildren uint32 = 1 << 20
	hardMaxRowColumnWork     uint64 = 8 << 20
	hardMaxRowColumnState    uint64 = 256 << 20
	defaultRowColumnState    uint64 = 64 << 20
	rowColumnBaseState       uint64 = 1024
	rowColumnChildState      uint64 = 512
)

// RowColumnDirection selects the main axis. A row advances left to right; a
// column advances top to bottom. Both use the same fixed-point track solver.
type RowColumnDirection string

const (
	RowDirection    RowColumnDirection = "row"
	ColumnDirection RowColumnDirection = "column"
)

// RowColumnTrackKind distinguishes exact, intrinsic, and weighted tracks.
type RowColumnTrackKind string

const (
	RowColumnTrackFixed    RowColumnTrackKind = "fixed"
	RowColumnTrackAuto     RowColumnTrackKind = "auto"
	RowColumnTrackFraction RowColumnTrackKind = "fraction"
	// RowColumnTrackFlex applies CSS-compatible grow and scaled-shrink factors
	// to an authored fixed or percentage basis. It is deliberately separate
	// from Fraction so existing typed row/column tracks keep their contract.
	RowColumnTrackFlex RowColumnTrackKind = "flex"
)

type RowColumnFlexBasisKind string

const (
	RowColumnFlexBasisFixed   RowColumnFlexBasisKind = "fixed"
	RowColumnFlexBasisPercent RowColumnFlexBasisKind = "percent"
	RowColumnFlexBasisContent RowColumnFlexBasisKind = "content"
)

// RowColumnTrack is one main-axis constraint. Fixed tracks use Size. Auto
// tracks receive an equal share when no fractional tracks exist. Fractional
// tracks require Weight and receive remaining space proportionally. Flex
// tracks use an authored fixed/percentage Basis plus CSS grow and scaled-
// shrink factors, freezing at Min/Max. The child's measured MinMain is an
// additional lower bound.
type RowColumnTrack struct {
	Kind         RowColumnTrackKind
	Size         Fixed
	Min          Fixed
	Max          Fixed
	Weight       uint32
	BasisKind    RowColumnFlexBasisKind
	Basis        Fixed
	BasisPercent uint32 // millionths of one percent; 100% is 100_000_000
	Grow         uint32
	Shrink       uint32
	GrowFactor   uint64 // millionths; zero falls back to Grow*1e6
	ShrinkFactor uint64 // millionths; zero falls back to Shrink*1e6
	MinPercent   uint32 // millionths of one percent
	MaxPercent   uint32 // millionths of one percent
}

// CrossAlignment positions a measured child on the axis perpendicular to the
// flow direction. The empty value inherits the container alignment.
type CrossAlignment string

const (
	CrossStart   CrossAlignment = "start"
	CrossCenter  CrossAlignment = "center"
	CrossEnd     CrossAlignment = "end"
	CrossStretch CrossAlignment = "stretch"
)

// MainAlignment distributes space left after resolving tracks and the authored
// gap. It is resolved in fixed point by the planner, never by a frontend.
type MainAlignment string

const (
	MainStart        MainAlignment = "start"
	MainCenter       MainAlignment = "center"
	MainEnd          MainAlignment = "end"
	MainSpaceBetween MainAlignment = "space-between"
	MainSpaceAround  MainAlignment = "space-around"
	MainSpaceEvenly  MainAlignment = "space-evenly"
)

// RowColumnWrap controls whether ordered children form one line or as many
// bounded lines as the main axis requires. WrapReverse only reverses physical
// cross-axis placement; fragment and identity order remain authored order.
type RowColumnWrap string

const (
	RowColumnNoWrap      RowColumnWrap = "nowrap"
	RowColumnWrapForward RowColumnWrap = "wrap"
	RowColumnWrapReverse RowColumnWrap = "wrap-reverse"
)

// ContentAlignment distributes free cross-axis space between formed lines.
// Stretch grows line cross extents; it never changes authored child sizes
// unless the child's CrossAlignment is itself stretch.
type ContentAlignment string

const (
	ContentStart        ContentAlignment = "start"
	ContentCenter       ContentAlignment = "center"
	ContentEnd          ContentAlignment = "end"
	ContentSpaceBetween ContentAlignment = "space-between"
	ContentSpaceAround  ContentAlignment = "space-around"
	ContentSpaceEvenly  ContentAlignment = "space-evenly"
	ContentStretch      ContentAlignment = "stretch"
)

// RowColumnChild is already measured. MinMain contributes to main-axis track
// sizing. CrossSize is used by start, center, and end; stretch fills the
// region's cross axis. Layout never calls a painter or text measurer.
type RowColumnChild struct {
	Node            NodeID
	Key             NodeKey
	Instance        InstanceID
	Source          SourceSpan
	Track           RowColumnTrack
	MinMain         Fixed
	ContentMain     Fixed
	CrossSize       Fixed
	CrossMin        Fixed
	CrossMax        Fixed
	CrossMinPercent uint32
	CrossMaxPercent uint32
	Align           CrossAlignment
}

type RowColumnPlanInput struct {
	PageSize     Size
	Region       Rect
	Direction    RowColumnDirection
	Gap          Fixed
	Align        CrossAlignment
	Justify      MainAlignment
	Wrap         RowColumnWrap
	CrossGap     Fixed
	AlignContent ContentAlignment
	ReverseMain  bool
	Children     []RowColumnChild
}

// RowColumnPlanLimits bounds retained child state and deterministic work. A
// zero value selects the defaults; partial zero values are rejected.
type RowColumnPlanLimits struct {
	MaxChildren   uint32
	MaxWork       uint64
	MaxStateBytes uint64
}

func DefaultRowColumnPlanLimits() RowColumnPlanLimits {
	return RowColumnPlanLimits{
		MaxChildren:   hardMaxRowColumnChildren,
		MaxWork:       hardMaxRowColumnWork,
		MaxStateBytes: defaultRowColumnState,
	}
}

// RowColumnPlanResult owns the immutable plan and a detached resolved-track
// projection. UsedMain includes gaps; it may be smaller than the region when
// every track is fixed.
type RowColumnPlanResult struct {
	Plan      LayoutPlan
	mainSizes []Fixed
	lines     []RowColumnLine
	UsedMain  Fixed
}

func (result RowColumnPlanResult) MainSizes() []Fixed { return cloneSlice(result.mainSizes) }

// RowColumnLine is a detached projection of one logical line. CrossStart is
// the physical coordinate after wrap-reverse and align-content are applied.
type RowColumnLine struct {
	Children   IndexRange
	CrossStart Fixed
	CrossSize  Fixed
	UsedMain   Fixed
}

func (result RowColumnPlanResult) Lines() []RowColumnLine { return cloneSlice(result.lines) }

type rowColumnBudget struct {
	ctx   context.Context
	limit uint64
	used  uint64
}

func (budget *rowColumnBudget) charge(amount uint64) error {
	if err := ChargePlanningWork(budget.ctx, "row or column planning", amount); err != nil {
		return err
	}
	if err := budget.ctx.Err(); err != nil {
		return newPlanningError(err, Diagnostic{
			Code: DiagnosticCanceled, Severity: SeverityError, Stage: StageLayout,
			Message: "row or column planning was canceled",
		})
	}
	if amount > budget.limit-budget.used {
		return newPlanningError(ErrRowColumnWorkLimit, Diagnostic{
			Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout,
			Message: "row or column planning exceeded its deterministic work limit",
			Evidence: []DiagnosticEvidence{
				{Key: "work_limit", Value: strconv.FormatUint(budget.limit, 10)},
				{Key: "work_used", Value: strconv.FormatUint(budget.used, 10)},
				{Key: "work_requested", Value: strconv.FormatUint(amount, 10)},
			},
		})
	}
	budget.used += amount
	return nil
}

// PlanRowColumn creates canonical one-page geometry for a bounded row or
// column. Input order is semantic main-axis order and therefore fragment order,
// even when ReverseMain mirrors physical placement. Remainders are assigned
// from the first logical child forward, one fixed unit at a time, making the
// result independent of maps, floating point, and painters.
func PlanRowColumn(ctx context.Context, input RowColumnPlanInput, limits RowColumnPlanLimits) (RowColumnPlanResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeRowColumnLimits(limits)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	budget := rowColumnBudget{ctx: ctx, limit: limits.MaxWork}
	if err := budget.charge(1); err != nil {
		return RowColumnPlanResult{}, err
	}
	if err := validateVerticalFlowInput(VerticalFlowInput{PageSize: input.PageSize, Body: input.Region}); err != nil {
		return RowColumnPlanResult{}, err
	}
	if input.Direction != RowDirection && input.Direction != ColumnDirection {
		return RowColumnPlanResult{}, ErrRowColumnDirection
	}
	if input.Gap < 0 {
		return RowColumnPlanResult{}, fmt.Errorf("%w: gap must be non-negative", ErrRowColumnTrack)
	}
	containerAlign := input.Align
	if containerAlign == "" {
		containerAlign = CrossStart
	}
	if !containerAlign.valid() {
		return RowColumnPlanResult{}, ErrRowColumnAlignment
	}
	justify := input.Justify
	if justify == "" {
		justify = MainStart
	}
	if !justify.valid() {
		return RowColumnPlanResult{}, ErrRowColumnJustification
	}
	wrap := input.Wrap
	if wrap == "" {
		wrap = RowColumnNoWrap
	}
	if !wrap.valid() {
		return RowColumnPlanResult{}, ErrRowColumnWrap
	}
	alignContent := input.AlignContent
	if alignContent == "" {
		alignContent = ContentStart
	}
	if !alignContent.valid() {
		return RowColumnPlanResult{}, ErrRowColumnContentAlign
	}
	if input.CrossGap < 0 {
		return RowColumnPlanResult{}, fmt.Errorf("%w: cross gap must be non-negative", ErrRowColumnTrack)
	}
	if uint64(len(input.Children)) > uint64(limits.MaxChildren) {
		return RowColumnPlanResult{}, rowColumnResourceLimit(limits, len(input.Children), 0)
	}
	stateBytes, ok := rowColumnStateBytes(input.Children)
	if !ok || stateBytes > limits.MaxStateBytes {
		return RowColumnPlanResult{}, rowColumnResourceLimit(limits, len(input.Children), stateBytes)
	}
	if err := budget.charge(uint64(len(input.Children))); err != nil {
		return RowColumnPlanResult{}, err
	}
	if wrap != RowColumnNoWrap {
		return planWrappedRowColumn(input, wrap, alignContent, containerAlign, justify, &budget)
	}

	mainAvailable, crossAvailable := rowColumnAxes(input.Direction, input.Region.Width, input.Region.Height)
	mainSizes, usedMain, err := resolveRowColumnTracks(input, mainAvailable, &budget)
	if err != nil {
		return RowColumnPlanResult{}, err
	}

	planInput := LayoutPlanInput{Pages: []PlannedPage{{
		Number: 1, Size: input.PageSize, Fragments: IndexRange{Count: uint32(len(input.Children))},
	}}}
	planInput.Fragments = make([]Fragment, 0, len(input.Children))
	if err := budget.charge(uint64(len(input.Children)) + 1); err != nil {
		return RowColumnPlanResult{}, err
	}
	mainOffset, distributedGaps, err := resolveMainAxisPlacement(justify, len(input.Children), mainAvailable, usedMain)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	mainCursor, err := rowColumnMainOrigin(input.Direction, input.Region).Add(mainOffset)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	crossOrigin := rowColumnCrossOrigin(input.Direction, input.Region)
	for index, child := range input.Children {
		if err := budget.charge(1); err != nil {
			return RowColumnPlanResult{}, err
		}
		align := child.Align
		if align == "" {
			align = containerAlign
		}
		if !align.valid() {
			return RowColumnPlanResult{}, fmt.Errorf("%w: child %d", ErrRowColumnAlignment, index)
		}
		crossPosition, crossExtent, err := resolveCrossAxis(child, align, crossOrigin, crossAvailable, input.Region, index)
		if err != nil {
			return RowColumnPlanResult{}, err
		}
		mainPosition, err := rowColumnPhysicalMainPosition(input.Direction, input.Region, mainCursor, mainSizes[index], input.ReverseMain)
		if err != nil {
			return RowColumnPlanResult{}, err
		}
		bounds, err := rowColumnRect(input.Direction, mainPosition, crossPosition, mainSizes[index], crossExtent)
		if err != nil {
			return RowColumnPlanResult{}, fmt.Errorf("layoutengine: row or column child %d bounds: %w", index, err)
		}
		planInput.Fragments = append(planInput.Fragments, Fragment{
			ID: FragmentID(index + 1), Node: child.Node, Key: child.Key, Instance: child.Instance,
			Page: 1, Region: RegionBody, BorderBox: bounds, ContentBox: bounds,
			Source: child.Source, Continuation: ContinuationWhole,
		})
		mainCursor, err = mainCursor.Add(mainSizes[index])
		if err != nil {
			return RowColumnPlanResult{}, err
		}
		if index+1 < len(input.Children) {
			mainCursor, err = mainCursor.Add(input.Gap)
			if err != nil {
				return RowColumnPlanResult{}, err
			}
			mainCursor, err = mainCursor.Add(distributedGaps[index])
			if err != nil {
				return RowColumnPlanResult{}, err
			}
		}
	}
	plan, err := NewLayoutPlan(planInput)
	if err != nil {
		return RowColumnPlanResult{}, err
	}
	lines := []RowColumnLine(nil)
	if len(input.Children) != 0 {
		lines = []RowColumnLine{{Children: IndexRange{Count: uint32(len(input.Children))}, CrossStart: crossOrigin, CrossSize: crossAvailable, UsedMain: usedMain}}
	}
	return RowColumnPlanResult{Plan: plan, mainSizes: mainSizes, lines: lines, UsedMain: usedMain}, nil
}

// resolveMainAxisPlacement assigns indivisible fixed-point remainder units from
// the first logical spacing slot forward. This makes centering and distributed
// spacing deterministic without floating point or painter participation.
func resolveMainAxisPlacement(justify MainAlignment, children int, available, used Fixed) (Fixed, []Fixed, error) {
	if used > available {
		return 0, nil, ErrRowColumnOverflow
	}
	gapCount := children - 1
	if gapCount < 0 {
		gapCount = 0
	}
	gaps := make([]Fixed, gapCount)
	free := available - used
	if free == 0 || children == 0 || justify == MainStart || (justify == MainSpaceBetween && children == 1) {
		return 0, gaps, nil
	}
	switch justify {
	case MainCenter:
		slots := distributeMainSpace(free, 2)
		return slots[0], gaps, nil
	case MainEnd:
		return free, gaps, nil
	case MainSpaceBetween:
		slots := distributeMainSpace(free, children-1)
		copy(gaps, slots)
		return 0, gaps, nil
	case MainSpaceAround:
		slots := distributeMainSpace(free, children*2)
		for index := range gaps {
			gaps[index] = slots[index*2+1] + slots[index*2+2]
		}
		return slots[0], gaps, nil
	case MainSpaceEvenly:
		slots := distributeMainSpace(free, children+1)
		copy(gaps, slots[1:children])
		return slots[0], gaps, nil
	default:
		return 0, nil, ErrRowColumnJustification
	}
}

func distributeMainSpace(total Fixed, slots int) []Fixed {
	result := make([]Fixed, slots)
	if slots <= 0 {
		return result
	}
	base := total / Fixed(slots)
	remainder := total % Fixed(slots)
	for index := range result {
		result[index] = base
		if Fixed(index) < remainder {
			result[index]++
		}
	}
	return result
}

func normalizeRowColumnLimits(limits RowColumnPlanLimits) (RowColumnPlanLimits, error) {
	if limits == (RowColumnPlanLimits{}) {
		return DefaultRowColumnPlanLimits(), nil
	}
	if limits.MaxChildren == 0 || limits.MaxWork == 0 || limits.MaxStateBytes == 0 {
		return RowColumnPlanLimits{}, fmt.Errorf("%w: all bounds must be positive", ErrRowColumnLimitsInvalid)
	}
	if limits.MaxChildren > hardMaxRowColumnChildren || limits.MaxWork > hardMaxRowColumnWork ||
		limits.MaxStateBytes > hardMaxRowColumnState {
		return RowColumnPlanLimits{}, fmt.Errorf("%w: caller bounds exceed implementation hard caps", ErrRowColumnLimitsInvalid)
	}
	return limits, nil
}

func rowColumnResourceLimit(limits RowColumnPlanLimits, children int, stateBytes uint64) error {
	return newPlanningError(ErrRowColumnResourceLimit, Diagnostic{
		Code: DiagnosticResourceLimit, Severity: SeverityError, Stage: StageLayout,
		Message: "row or column planning exceeds a retained-state limit",
		Evidence: []DiagnosticEvidence{
			{Key: "children", Value: strconv.Itoa(children)},
			{Key: "max_children", Value: strconv.FormatUint(uint64(limits.MaxChildren), 10)},
			{Key: "state_bytes", Value: strconv.FormatUint(stateBytes, 10)},
			{Key: "max_state_bytes", Value: strconv.FormatUint(limits.MaxStateBytes, 10)},
		},
	})
}

// rowColumnStateBytes conservatively accounts the resolved-size slice, plan
// fragments, plan-owned copies, and identity/provenance bytes retained by the
// result. The fixed estimate intentionally exceeds the current struct sizes so
// adding fields cannot silently make the bound optimistic.
func rowColumnStateBytes(children []RowColumnChild) (uint64, bool) {
	count := uint64(len(children))
	if count > (^(uint64(0))-rowColumnBaseState)/rowColumnChildState {
		return ^uint64(0), false
	}
	total := rowColumnBaseState + count*rowColumnChildState
	for _, child := range children {
		for _, value := range []string{string(child.Key), string(child.Instance), child.Source.File} {
			length := uint64(len(value))
			if length > ^uint64(0)-total {
				return ^uint64(0), false
			}
			total += length
		}
	}
	return total, true
}

func resolveRowColumnTracks(input RowColumnPlanInput, available Fixed, budget *rowColumnBudget) ([]Fixed, Fixed, error) {
	count := len(input.Children)
	if count == 0 {
		return nil, 0, nil
	}
	gapTotal, err := input.Gap.MulInt(int64(count - 1))
	if err != nil {
		return nil, 0, err
	}
	trackAvailable, err := available.Sub(gapTotal)
	if err != nil {
		return nil, 0, err
	}
	if trackAvailable < 0 {
		return nil, 0, rowColumnOverflow(input, -1, "main", gapTotal, available)
	}

	sizes := make([]Fixed, count)
	fractions := make([]int, 0, count)
	fractionWeights := make([]uint32, 0, count)
	autos := make([]int, 0, count)
	flexes := make([]int, 0, count)
	flexBases := make([]Fixed, count)
	for index, child := range input.Children {
		if err := budget.charge(1); err != nil {
			return nil, 0, err
		}
		if err := validateRowColumnChild(child, index); err != nil {
			return nil, 0, err
		}
		minimum, constraintErr := resolveRowColumnMinimum(child.Track, available)
		if constraintErr != nil {
			return nil, 0, fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, constraintErr)
		}
		if child.MinMain > minimum {
			minimum = child.MinMain
		}
		switch child.Track.Kind {
		case RowColumnTrackFixed:
			if child.Track.Weight != 0 || child.Track.Max != 0 || child.Track.MinPercent != 0 || child.Track.MaxPercent != 0 || child.Track.BasisKind != "" || child.Track.Basis != 0 || child.Track.BasisPercent != 0 || child.Track.Grow != 0 || child.Track.Shrink != 0 || child.Track.GrowFactor != 0 || child.Track.ShrinkFactor != 0 {
				return nil, 0, fmt.Errorf("%w: child %d fixed track has a weight", ErrRowColumnTrack, index)
			}
			if child.Track.Size < minimum {
				return nil, 0, rowColumnOverflow(input, index, "main", minimum, child.Track.Size)
			}
			sizes[index] = child.Track.Size
		case RowColumnTrackAuto:
			if child.Track.Weight != 0 || child.Track.Size != 0 || child.Track.Max != 0 || child.Track.MinPercent != 0 || child.Track.MaxPercent != 0 || child.Track.BasisKind != "" || child.Track.Basis != 0 || child.Track.BasisPercent != 0 || child.Track.Grow != 0 || child.Track.Shrink != 0 || child.Track.GrowFactor != 0 || child.Track.ShrinkFactor != 0 {
				return nil, 0, fmt.Errorf("%w: child %d auto track has fixed or weighted data", ErrRowColumnTrack, index)
			}
			sizes[index] = minimum
			autos = append(autos, index)
		case RowColumnTrackFraction:
			if child.Track.Weight == 0 || child.Track.Size != 0 || child.Track.Max != 0 || child.Track.MinPercent != 0 || child.Track.MaxPercent != 0 || child.Track.BasisKind != "" || child.Track.Basis != 0 || child.Track.BasisPercent != 0 || child.Track.Grow != 0 || child.Track.Shrink != 0 || child.Track.GrowFactor != 0 || child.Track.ShrinkFactor != 0 {
				return nil, 0, fmt.Errorf("%w: child %d fractional track requires only a positive weight", ErrRowColumnTrack, index)
			}
			sizes[index] = minimum
			fractions = append(fractions, index)
			fractionWeights = append(fractionWeights, child.Track.Weight)
		case RowColumnTrackFlex:
			if child.Track.Size != 0 || child.Track.Weight != 0 {
				return nil, 0, fmt.Errorf("%w: child %d flex track has legacy fixed or fraction data", ErrRowColumnTrack, index)
			}
			basis, basisErr := resolveRowColumnFlexBasis(child.Track, available, child.ContentMain)
			if basisErr != nil {
				return nil, 0, fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, basisErr)
			}
			flexBases[index] = basis
			maximum, maxErr := resolveRowColumnMaximum(child.Track, available)
			if maxErr != nil {
				return nil, 0, fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, maxErr)
			}
			sizes[index] = clampRowColumnFlexSize(basis, minimum, maximum)
			flexes = append(flexes, index)
		default:
			return nil, 0, fmt.Errorf("%w: child %d", ErrRowColumnTrack, index)
		}
	}
	minimumUsed, err := addFixed(sizes...)
	if err != nil {
		return nil, 0, err
	}
	if minimumUsed > trackAvailable && len(flexes) == 0 {
		overflowIndex := -1
		var cumulative Fixed
		for index, size := range sizes {
			cumulative, err = cumulative.Add(size)
			if err != nil {
				return nil, 0, err
			}
			if cumulative > trackAvailable {
				overflowIndex = index
				break
			}
		}
		return nil, 0, rowColumnOverflow(input, overflowIndex, "main", minimumUsed, trackAvailable)
	}
	if len(flexes) != 0 {
		if err := resolveFlexibleRowColumnSizes(input, sizes, flexBases, flexes, fractions, trackAvailable, available, budget); err != nil {
			return nil, 0, err
		}
	} else {
		remaining, subtractErr := trackAvailable.Sub(minimumUsed)
		if subtractErr != nil {
			return nil, 0, subtractErr
		}
		if remaining > 0 && len(fractions) != 0 {
			if err := distributeRowColumnRemainder(sizes, fractions, fractionWeights, remaining); err != nil {
				return nil, 0, err
			}
		} else if remaining > 0 && len(autos) != 0 {
			if err := distributeRowColumnRemainder(sizes, autos, nil, remaining); err != nil {
				return nil, 0, err
			}
		}
	}
	used, err := addFixed(sizes...)
	if err != nil {
		return nil, 0, err
	}
	used, err = used.Add(gapTotal)
	return sizes, used, err
}

func rowColumnPhysicalMainPosition(direction RowColumnDirection, region Rect, logicalPosition, extent Fixed, reverse bool) (Fixed, error) {
	if !reverse {
		return logicalPosition, nil
	}
	origin := rowColumnMainOrigin(direction, region)
	available, _ := rowColumnAxes(direction, region.Width, region.Height)
	offset, err := logicalPosition.Sub(origin)
	if err != nil {
		return 0, err
	}
	physicalOffset, err := available.Sub(offset)
	if err == nil {
		physicalOffset, err = physicalOffset.Sub(extent)
	}
	if err != nil {
		return 0, err
	}
	return origin.Add(physicalOffset)
}

func resolveRowColumnFlexBasis(track RowColumnTrack, available, content Fixed) (Fixed, error) {
	switch track.BasisKind {
	case RowColumnFlexBasisFixed:
		if track.BasisPercent != 0 {
			return 0, errors.New("fixed flex basis has percentage data")
		}
		return track.Basis, nil
	case RowColumnFlexBasisPercent:
		if track.Basis != 0 || track.BasisPercent > 100_000_000 {
			return 0, errors.New("percentage flex basis is outside 0%..100%")
		}
		product := new(big.Int).Mul(big.NewInt(int64(available)), new(big.Int).SetUint64(uint64(track.BasisPercent)))
		product.Quo(product, big.NewInt(100_000_000))
		if !product.IsInt64() {
			return 0, ErrGeometryOverflow
		}
		return Fixed(product.Int64()), nil
	case RowColumnFlexBasisContent:
		if track.Basis != 0 || track.BasisPercent != 0 || content <= 0 {
			return 0, errors.New("content flex basis requires a positive measured content size")
		}
		return content, nil
	default:
		return 0, errors.New("flex basis kind is invalid")
	}
}

func resolveRowColumnPercentage(value uint32, available Fixed) (Fixed, error) {
	if value > 100_000_000 {
		return 0, errors.New("percentage constraint is outside 0%..100%")
	}
	product := new(big.Int).Mul(big.NewInt(int64(available)), new(big.Int).SetUint64(uint64(value)))
	product.Quo(product, big.NewInt(100_000_000))
	if !product.IsInt64() {
		return 0, ErrGeometryOverflow
	}
	return Fixed(product.Int64()), nil
}

func resolveRowColumnMinimum(track RowColumnTrack, available Fixed) (Fixed, error) {
	minimum := track.Min
	if track.MinPercent != 0 {
		percent, err := resolveRowColumnPercentage(track.MinPercent, available)
		if err != nil {
			return 0, err
		}
		if percent > minimum {
			minimum = percent
		}
	}
	return minimum, nil
}

func resolveRowColumnMaximum(track RowColumnTrack, available Fixed) (Fixed, error) {
	maximum := track.Max
	if track.MaxPercent != 0 {
		percent, err := resolveRowColumnPercentage(track.MaxPercent, available)
		if err != nil {
			return 0, err
		}
		if maximum == 0 || percent < maximum {
			maximum = percent
		}
	}
	return maximum, nil
}

func clampRowColumnFlexSize(size, minimum, maximum Fixed) Fixed {
	if size < minimum {
		size = minimum
	}
	if maximum > 0 && size > maximum {
		size = maximum
	}
	return size
}

// resolveFlexibleRowColumnSizes performs bounded freeze-and-redistribute
// sizing. Positive free space uses grow factors. Negative free space uses the
// CSS scaled shrink factor (shrink multiplied by the unclamped flex basis).
// Min/max constraints freeze an item and the remainder is recomputed. Integer
// division remainders always go to the first logical unfrozen item.
func resolveFlexibleRowColumnSizes(input RowColumnPlanInput, sizes, bases []Fixed, flexes, fractions []int, available, percentageReference Fixed, budget *rowColumnBudget) error {
	active := append([]int(nil), flexes...)
	active = append(active, fractions...)
	for iteration := 0; iteration <= len(active); iteration++ {
		if err := budget.charge(uint64(len(active)) + 1); err != nil {
			return err
		}
		used, err := addFixed(sizes...)
		if err != nil {
			return err
		}
		free, err := available.Sub(used)
		if err != nil || free == 0 {
			return err
		}
		grow := free > 0
		indexes := make([]int, 0, len(active))
		weights := make([]*big.Int, 0, len(active))
		for _, index := range active {
			track := input.Children[index].Track
			var weight *big.Int
			if track.Kind == RowColumnTrackFraction {
				if !grow {
					continue
				}
				weight = new(big.Int).SetUint64(uint64(track.Weight))
			} else if grow {
				weight = new(big.Int).SetUint64(rowColumnGrowFactor(track))
			} else {
				weight = new(big.Int).Mul(new(big.Int).SetUint64(rowColumnShrinkFactor(track)), big.NewInt(int64(bases[index])))
			}
			if weight.Sign() > 0 {
				indexes = append(indexes, index)
				weights = append(weights, weight)
			}
		}
		if len(indexes) == 0 {
			if free < 0 {
				return rowColumnOverflow(input, -1, "main", used, available)
			}
			return nil
		}
		amount := free
		if amount < 0 {
			amount = -amount
		}
		shares, err := distributeBigWeightedAmount(amount, weights)
		if err != nil {
			return err
		}
		froze := false
		nextActive := make([]int, 0, len(active))
		participating := make(map[int]int, len(indexes))
		for position, index := range indexes {
			participating[index] = position
		}
		for _, index := range active {
			position, ok := participating[index]
			if !ok {
				continue
			}
			candidate := sizes[index]
			if grow {
				candidate, err = candidate.Add(shares[position])
			} else {
				candidate, err = candidate.Sub(shares[position])
			}
			if err != nil {
				return err
			}
			track := input.Children[index].Track
			minimum, constraintErr := resolveRowColumnMinimum(track, percentageReference)
			if constraintErr != nil {
				return constraintErr
			}
			if input.Children[index].MinMain > minimum {
				minimum = input.Children[index].MinMain
			}
			maximum, constraintErr := resolveRowColumnMaximum(track, percentageReference)
			if constraintErr != nil {
				return constraintErr
			}
			clamped := clampRowColumnFlexSize(candidate, minimum, maximum)
			sizes[index] = clamped
			if clamped != candidate {
				froze = true
			} else {
				nextActive = append(nextActive, index)
			}
		}
		if !froze {
			return nil
		}
		active = nextActive
	}
	return ErrRowColumnTrack
}

func rowColumnGrowFactor(track RowColumnTrack) uint64 {
	if track.GrowFactor != 0 {
		return track.GrowFactor
	}
	return uint64(track.Grow) * 1_000_000
}

func rowColumnShrinkFactor(track RowColumnTrack) uint64 {
	if track.ShrinkFactor != 0 {
		return track.ShrinkFactor
	}
	return uint64(track.Shrink) * 1_000_000
}

func distributeBigWeightedAmount(amount Fixed, weights []*big.Int) ([]Fixed, error) {
	if amount < 0 || len(weights) == 0 {
		return nil, ErrRowColumnTrack
	}
	total := new(big.Int)
	for _, weight := range weights {
		if weight == nil || weight.Sign() < 0 {
			return nil, ErrRowColumnTrack
		}
		total.Add(total, weight)
	}
	if total.Sign() == 0 {
		return nil, ErrRowColumnTrack
	}
	shares := make([]Fixed, len(weights))
	allocated := Fixed(0)
	for index, weight := range weights {
		share := new(big.Int).Mul(big.NewInt(int64(amount)), weight)
		share.Quo(share, total)
		if !share.IsInt64() || share.Int64() > int64(MaxFixed) {
			return nil, ErrGeometryOverflow
		}
		shares[index] = Fixed(share.Int64())
		allocated += shares[index]
	}
	for remainder := amount - allocated; remainder > 0; remainder-- {
		shares[(amount-allocated-remainder)%Fixed(len(shares))]++
	}
	return shares, nil
}

func validateRowColumnChild(child RowColumnChild, index int) error {
	if !child.Node.Valid() || !child.Instance.Valid() || child.Key == "" {
		return fmt.Errorf("%w: child %d has an absent identity", ErrRowColumnTrack, index)
	}
	if err := validateTextIdentity("row or column child key", string(child.Key)); err != nil {
		return fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, err)
	}
	if err := validateTextIdentity("row or column child instance", string(child.Instance)); err != nil {
		return fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, err)
	}
	if err := child.Source.Validate(); err != nil {
		return fmt.Errorf("%w: child %d source: %v", ErrRowColumnTrack, index, err)
	}
	if child.Track.Min < 0 || child.Track.Max < 0 || child.Track.Size < 0 || child.Track.Basis < 0 || child.MinMain < 0 || child.ContentMain < 0 || child.CrossSize < 0 || child.CrossMin < 0 || child.CrossMax < 0 ||
		(child.Track.Max > 0 && child.Track.Max < child.Track.Min) {
		return fmt.Errorf("%w: child %d has a negative constraint", ErrRowColumnTrack, index)
	}
	if child.Track.MinPercent > 100_000_000 || child.Track.MaxPercent > 100_000_000 || child.CrossMinPercent > 100_000_000 || child.CrossMaxPercent > 100_000_000 {
		return fmt.Errorf("%w: child %d has an invalid percentage constraint", ErrRowColumnTrack, index)
	}
	return nil
}

func distributeRowColumnRemainder(sizes []Fixed, indexes []int, weights []uint32, amount Fixed) error {
	if amount < 0 || len(indexes) == 0 || (len(weights) != 0 && len(weights) != len(indexes)) {
		return ErrRowColumnTrack
	}
	if len(weights) == 0 {
		weights = make([]uint32, len(indexes))
		for index := range weights {
			weights[index] = 1
		}
	}
	var totalWeight uint64
	for _, weight := range weights {
		totalWeight += uint64(weight)
	}
	if totalWeight == 0 {
		return ErrRowColumnTrack
	}
	amountUnsigned := uint64(amount)
	var allocated uint64
	for index, target := range indexes {
		weight := uint64(weights[index])
		high, low := bits.Mul64(amountUnsigned, weight)
		share, _ := bits.Div64(high, low, totalWeight)
		if share > uint64(MaxFixed) {
			return ErrGeometryOverflow
		}
		next, err := sizes[target].Add(Fixed(share))
		if err != nil {
			return err
		}
		sizes[target] = next
		allocated += share
	}
	for remainder := amountUnsigned - allocated; remainder > 0; remainder-- {
		target := indexes[(amountUnsigned-allocated-remainder)%uint64(len(indexes))]
		next, err := sizes[target].Add(1)
		if err != nil {
			return err
		}
		sizes[target] = next
	}
	return nil
}

func resolveCrossAxis(child RowColumnChild, align CrossAlignment, origin, available Fixed, region Rect, index int) (Fixed, Fixed, error) {
	extent, err := resolveRowColumnCrossExtent(child, available, align == CrossStretch)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: child %d: %v", ErrRowColumnTrack, index, err)
	}
	if align == CrossStretch {
		return origin, extent, nil
	}
	if extent > available {
		return 0, 0, rowColumnChildOverflow(child, region, index, "cross", extent, available)
	}
	free, err := available.Sub(extent)
	if err != nil {
		return 0, 0, err
	}
	offset := Fixed(0)
	switch align {
	case CrossStart:
	case CrossCenter:
		offset = free / 2
	case CrossEnd:
		offset = free
	default:
		return 0, 0, ErrRowColumnAlignment
	}
	position, err := origin.Add(offset)
	return position, extent, err
}

func resolveRowColumnCrossExtent(child RowColumnChild, available Fixed, stretch bool) (Fixed, error) {
	extent := child.CrossSize
	if stretch {
		extent = available
	}
	minimum := child.CrossMin
	if child.CrossMinPercent != 0 {
		value, err := resolveRowColumnPercentage(child.CrossMinPercent, available)
		if err != nil {
			return 0, err
		}
		if value > minimum {
			minimum = value
		}
	}
	maximum := child.CrossMax
	if child.CrossMaxPercent != 0 {
		value, err := resolveRowColumnPercentage(child.CrossMaxPercent, available)
		if err != nil {
			return 0, err
		}
		if maximum == 0 || value < maximum {
			maximum = value
		}
	}
	if maximum > 0 && minimum > maximum {
		return 0, errors.New("cross-axis minimum exceeds maximum")
	}
	if extent < minimum {
		extent = minimum
	}
	if maximum > 0 && extent > maximum {
		extent = maximum
	}
	return extent, nil
}

func rowColumnOverflow(input RowColumnPlanInput, childIndex int, axis string, required, available Fixed) error {
	location := DiagnosticLocation{Region: RegionBody, Bounds: input.Region, HasBounds: true}
	evidence := []DiagnosticEvidence{
		{Key: "axis", Value: axis},
		{Key: "required", Value: strconv.FormatInt(int64(required), 10)},
		{Key: "available", Value: strconv.FormatInt(int64(available), 10)},
	}
	if childIndex >= 0 {
		child := input.Children[childIndex]
		location.Node, location.Key, location.Instance, location.Source = child.Node, child.Key, child.Instance, child.Source
		evidence = append(evidence, DiagnosticEvidence{Key: "child_index", Value: strconv.Itoa(childIndex)})
	}
	return newPlanningError(ErrRowColumnOverflow, Diagnostic{
		Code: DiagnosticTrackMinOverflow, Severity: SeverityError, Stage: StageLayout,
		Message:  "row or column minimums exceed the available region",
		Location: location, Evidence: evidence,
	})
}

func rowColumnChildOverflow(child RowColumnChild, region Rect, index int, axis string, required, available Fixed) error {
	return newPlanningError(ErrRowColumnOverflow, Diagnostic{
		Code: DiagnosticTrackMinOverflow, Severity: SeverityError, Stage: StageLayout,
		Message: "row or column child exceeds the available cross axis",
		Location: DiagnosticLocation{
			Node: child.Node, Key: child.Key, Instance: child.Instance, Source: child.Source,
			Region: RegionBody, Bounds: region, HasBounds: true,
		},
		Evidence: []DiagnosticEvidence{
			{Key: "axis", Value: axis},
			{Key: "required", Value: strconv.FormatInt(int64(required), 10)},
			{Key: "available", Value: strconv.FormatInt(int64(available), 10)},
			{Key: "child_index", Value: strconv.Itoa(index)},
		},
	})
}

func (alignment CrossAlignment) valid() bool {
	return alignment == CrossStart || alignment == CrossCenter || alignment == CrossEnd || alignment == CrossStretch
}

func (alignment MainAlignment) valid() bool {
	return alignment == MainStart || alignment == MainCenter || alignment == MainEnd || alignment == MainSpaceBetween ||
		alignment == MainSpaceAround || alignment == MainSpaceEvenly
}

func rowColumnAxes(direction RowColumnDirection, width, height Fixed) (Fixed, Fixed) {
	if direction == RowDirection {
		return width, height
	}
	return height, width
}

func rowColumnMainOrigin(direction RowColumnDirection, region Rect) Fixed {
	if direction == RowDirection {
		return region.X
	}
	return region.Y
}

func rowColumnCrossOrigin(direction RowColumnDirection, region Rect) Fixed {
	if direction == RowDirection {
		return region.Y
	}
	return region.X
}

func rowColumnRect(direction RowColumnDirection, main, cross, mainSize, crossSize Fixed) (Rect, error) {
	if direction == RowDirection {
		return NewRect(main, cross, mainSize, crossSize)
	}
	return NewRect(cross, main, crossSize, mainSize)
}
