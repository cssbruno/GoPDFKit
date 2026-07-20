// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

const (
	defaultPageMasterMaxBlocks uint32 = 100_000
	defaultPageMasterMaxPages  uint32 = 10_000
	hardPageMasterMaxBlocks    uint32 = 1_000_000
	hardPageMasterMaxPages     uint32 = 100_000
)

var (
	ErrPageMasterInvalid       = errors.New("layoutengine: page master is invalid")
	ErrPageMasterRegionEmpty   = errors.New("layoutengine: page master region is empty")
	ErrPageMasterRegionOutside = errors.New("layoutengine: page master region lies outside its page")
	ErrPageMasterRegionOverlap = errors.New("layoutengine: page master regions overlap or are out of order")
	ErrPageMasterWorkLimit     = errors.New("layoutengine: page master planner work limit exceeded")
)

// PageMasterID is a readable, canonical identity for one page geometry.
type PageMasterID string

// NewPageMasterID validates and constructs a lowercase page-master slug.
func NewPageMasterID(value string) (PageMasterID, error) {
	id, err := NewRegionID(value)
	return PageMasterID(id), err
}

func (id PageMasterID) valid() bool {
	_, err := NewPageMasterID(string(id))
	return err == nil
}

// PageMaster defines fixed geometry for the three canonical vertical-flow
// regions. Body must be non-empty. Header and Footer may be the zero rectangle,
// which explicitly disables that region. Any enabled region must have positive
// extents, lie wholly on the page, and occupy its vertical band without
// overlapping another enabled region. Horizontal disjointness does not relax
// the band rule: header must end before body starts, and body before footer.
type PageMaster struct {
	ID       PageMasterID `json:"id"`
	PageSize Size         `json:"page_size"`
	Header   Rect         `json:"header"`
	Body     Rect         `json:"body"`
	Footer   Rect         `json:"footer"`
}

// Region returns the fixed rectangle for a canonical page-master region.
func (m PageMaster) Region(region RegionID) (Rect, bool) {
	var rect Rect
	switch region {
	case RegionHeader:
		rect = m.Header
	case RegionBody:
		rect = m.Body
	case RegionFooter:
		rect = m.Footer
	default:
		return Rect{}, false
	}
	return rect, rect != (Rect{})
}

// PageMasterSet selects page geometry with stable precedence: First wins for
// page one; otherwise Odd or Even wins for its parity; an absent override uses
// Default. All supplied masters are validated, including currently unselected
// overrides, so a later page cannot reveal a latent geometry error.
type PageMasterSet struct {
	Default PageMaster
	First   *PageMaster
	Odd     *PageMaster
	Even    *PageMaster
}

// Select returns the value-copy selected for a one-based page number.
func (s PageMasterSet) Select(page uint32) (PageMaster, error) {
	if page == 0 {
		return PageMaster{}, fmt.Errorf("%w: page number is zero", ErrPageMasterInvalid)
	}
	if err := s.validate(); err != nil {
		return PageMaster{}, err
	}
	return s.selectValidated(page), nil
}

func (s PageMasterSet) selectValidated(page uint32) PageMaster {
	if page == 1 && s.First != nil {
		return *s.First
	}
	if page%2 == 1 && s.Odd != nil {
		return *s.Odd
	}
	if page%2 == 0 && s.Even != nil {
		return *s.Even
	}
	return s.Default
}

func (s PageMasterSet) validate() error {
	masters := []struct {
		slot   string
		master *PageMaster
	}{{"default", &s.Default}, {"first", s.First}, {"odd", s.Odd}, {"even", s.Even}}
	for _, candidate := range masters {
		if candidate.master == nil {
			continue
		}
		if err := validatePageMaster(candidate.slot, *candidate.master); err != nil {
			return err
		}
	}
	return nil
}

// PageMasterLimits bound planner work. Zero fields use conservative defaults.
// Values over the hard bounds are rejected rather than silently clamped.
type PageMasterLimits struct {
	MaxBlocks uint32
	MaxPages  uint32
}

func (l PageMasterLimits) resolve() (PageMasterLimits, error) {
	if l.MaxBlocks == 0 {
		l.MaxBlocks = defaultPageMasterMaxBlocks
	}
	if l.MaxPages == 0 {
		l.MaxPages = defaultPageMasterMaxPages
	}
	if l.MaxBlocks > hardPageMasterMaxBlocks {
		return PageMasterLimits{}, pageMasterLimitError("configured block limit exceeds hard bound", uint64(l.MaxBlocks), uint64(hardPageMasterMaxBlocks))
	}
	if l.MaxPages > hardPageMasterMaxPages {
		return PageMasterLimits{}, pageMasterLimitError("configured page limit exceeds hard bound", uint64(l.MaxPages), uint64(hardPageMasterMaxPages))
	}
	return l, nil
}

// PageMasterFlowInput contains three independent, already-measured vertical
// streams. Each stream starts on page one and advances through the selected
// region on later pages. The final plan has the maximum page count needed by
// any stream and emits fragments canonically in page, header/body/footer order.
type PageMasterFlowInput struct {
	Masters PageMasterSet
	Header  []VerticalFlowBlock
	Body    []VerticalFlowBlock
	Footer  []VerticalFlowBlock
	Limits  PageMasterLimits
}

type masterPlacementKey struct {
	region RegionID
	index  int
}

type masterPlacement struct {
	key      masterPlacementKey
	block    VerticalFlowBlock
	page     uint32
	box      Rect
	capacity Fixed
}

type masterBreak struct {
	from, to              uint32
	region                RegionID
	preceding, triggering masterPlacementKey
	reason                BreakReason
	required, available   Fixed
}

// PlanPageMasterFlow produces a geometry-only immutable LayoutPlan. It does no
// text shaping, painting, Document mutation, or HTML/CSS interpretation.
func PlanPageMasterFlow(input PageMasterFlowInput) (LayoutPlan, error) {
	return PlanPageMasterFlowContext(context.Background(), input)
}

// PlanPageMasterFlowContext charges selection, validation, placement, and page
// assembly to the request-owned cumulative budget carried by ctx.
func PlanPageMasterFlowContext(ctx context.Context, input PageMasterFlowInput) (LayoutPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := input.Masters.validate(); err != nil {
		return LayoutPlan{}, err
	}
	limits, err := input.Limits.resolve()
	if err != nil {
		return LayoutPlan{}, err
	}
	total := uint64(len(input.Header)) + uint64(len(input.Body)) + uint64(len(input.Footer))
	if err := ChargePlanningWork(ctx, "page master planning", total+1); err != nil {
		return LayoutPlan{}, err
	}
	if total > uint64(limits.MaxBlocks) || total > uint64(^uint32(0)) {
		return LayoutPlan{}, pageMasterLimitError("block count exceeds planner limit", total, uint64(limits.MaxBlocks))
	}
	streams := []struct {
		region RegionID
		blocks []VerticalFlowBlock
	}{{RegionHeader, input.Header}, {RegionBody, input.Body}, {RegionFooter, input.Footer}}

	placements := make([]masterPlacement, 0, int(total))
	breakSpecs := make([]masterBreak, 0)
	pageCount := uint32(1)
	for _, stream := range streams {
		for i, block := range stream.blocks {
			if err := ChargePlanningWork(ctx, "page master stream planning", 1); err != nil {
				return LayoutPlan{}, err
			}
			if err := block.validate(); err != nil {
				return LayoutPlan{}, fmt.Errorf("layoutengine: page master %s block %d: %w", stream.region, i, err)
			}
		}
		placed, breaks, pages, err := planMasterStream(input.Masters, stream.region, stream.blocks, limits.MaxPages)
		if err != nil {
			return LayoutPlan{}, err
		}
		placements = append(placements, placed...)
		breakSpecs = append(breakSpecs, breaks...)
		if pages > pageCount {
			pageCount = pages
		}
	}
	if pageCount > limits.MaxPages {
		return LayoutPlan{}, pageMasterLimitError("page count exceeds planner limit", uint64(pageCount), uint64(limits.MaxPages))
	}
	if err := ChargePlanningWork(ctx, "page master plan assembly", uint64(pageCount)+uint64(len(placements))+uint64(len(breakSpecs))); err != nil {
		return LayoutPlan{}, err
	}

	return buildPageMasterPlan(input.Masters, pageCount, placements, breakSpecs)
}

func planMasterStream(masters PageMasterSet, region RegionID, blocks []VerticalFlowBlock, maxPages uint32) ([]masterPlacement, []masterBreak, uint32, error) {
	if len(blocks) == 0 {
		return nil, nil, 1, nil
	}
	page := uint32(1)
	rect, err := selectedRegion(masters, page, region)
	if err != nil {
		return nil, nil, 0, err
	}
	cursor := rect.Y
	consumed := false
	lastIndex := -1
	lastOverflow := false
	placements := make([]masterPlacement, 0, len(blocks))
	breaks := make([]masterBreak, 0)

	for index, block := range blocks {
		bottom, _ := rect.Bottom()
		available, err := bottom.Sub(cursor)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("layoutengine: page master %s remaining height: %w", region, err)
		}
		if block.Height > 0 && block.Height > available && consumed {
			if page == maxPages {
				return nil, nil, 0, pageMasterLimitError("page transition exceeds planner limit", uint64(page+1), uint64(maxPages))
			}
			reason := BreakInsufficientRemainingBodySpace
			if lastOverflow {
				reason = BreakPreviousFragmentOverflow
				available = 0
			}
			breaks = append(breaks, masterBreak{
				from: page, to: page + 1, region: region,
				preceding:  masterPlacementKey{region, lastIndex},
				triggering: masterPlacementKey{region, index},
				reason:     reason, required: block.Height, available: available,
			})
			page++
			rect, err = selectedRegion(masters, page, region)
			if err != nil {
				return nil, nil, 0, err
			}
			cursor = rect.Y
			consumed = false
			lastIndex = -1
			lastOverflow = false
		}
		box, err := NewRect(rect.X, cursor, rect.Width, block.Height)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("layoutengine: page master %s block %d box: %w", region, index, err)
		}
		placements = append(placements, masterPlacement{
			key: masterPlacementKey{region, index}, block: block, page: page,
			box: box, capacity: rect.Height,
		})
		cursor, err = box.Bottom()
		if err != nil {
			return nil, nil, 0, fmt.Errorf("layoutengine: page master %s block %d cursor: %w", region, index, err)
		}
		if block.Height > 0 {
			consumed = true
			lastIndex = index
			regionBottom, _ := rect.Bottom()
			lastOverflow = cursor > regionBottom
		}
	}
	return placements, breaks, page, nil
}

func selectedRegion(masters PageMasterSet, page uint32, region RegionID) (Rect, error) {
	master := masters.selectValidated(page)
	rect, enabled := master.Region(region)
	if !enabled {
		return Rect{}, newPlanningError(ErrPageMasterRegionEmpty, Diagnostic{
			Code: DiagnosticPageMasterRegionEmpty, Severity: SeverityError, Stage: StageLayout,
			Message:  "page master flow targets a disabled region",
			Location: DiagnosticLocation{Page: page, Region: region},
			Evidence: []DiagnosticEvidence{{Key: "master", Value: string(master.ID)}},
		})
	}
	return rect, nil
}

func buildPageMasterPlan(masters PageMasterSet, pageCount uint32, placements []masterPlacement, breakSpecs []masterBreak) (LayoutPlan, error) {
	byPage := make([][]masterPlacement, pageCount+1)
	for _, placement := range placements {
		byPage[placement.page] = append(byPage[placement.page], placement)
	}
	for page := uint32(1); page <= pageCount; page++ {
		sort.SliceStable(byPage[page], func(i, j int) bool {
			left, right := regionOrder(byPage[page][i].key.region), regionOrder(byPage[page][j].key.region)
			if left != right {
				return left < right
			}
			return byPage[page][i].key.index < byPage[page][j].key.index
		})
	}
	planInput := LayoutPlanInput{Fragments: make([]Fragment, 0, len(placements))}
	ids := make(map[masterPlacementKey]FragmentID, len(placements))
	for page := uint32(1); page <= pageCount; page++ {
		start := len(planInput.Fragments)
		master := masters.selectValidated(page)
		for _, region := range []struct {
			id     RegionID
			bounds Rect
		}{{RegionHeader, master.Header}, {RegionBody, master.Body}, {RegionFooter, master.Footer}} {
			if region.bounds != (Rect{}) {
				planInput.PageRegions = append(planInput.PageRegions, PlannedPageRegion{Page: page, Region: region.id, Bounds: region.bounds, Master: master.ID})
			}
		}
		for _, placement := range byPage[page] {
			id := FragmentID(len(planInput.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			ids[placement.key] = id
			block := placement.block
			planInput.Fragments = append(planInput.Fragments, Fragment{
				ID: id, Node: block.Node, Key: block.Key, Instance: block.Instance,
				Page: page, Region: placement.key.region, BorderBox: placement.box,
				ContentBox: placement.box, Source: block.Source, Continuation: ContinuationWhole,
			})
			if block.Height > placement.capacity {
				overflow, _ := block.Height.Sub(placement.capacity)
				planInput.Diagnostics = append(planInput.Diagnostics, Diagnostic{
					Code: DiagnosticUnbreakableTooTall, Severity: SeverityWarning, Stage: StageLayout,
					Message: "indivisible block exceeds its page-master region and was emitted once",
					Location: DiagnosticLocation{Node: block.Node, Key: block.Key, Source: block.Source,
						Instance: block.Instance, Fragment: id, Page: page, Region: placement.key.region,
						Bounds: placement.box, HasBounds: true},
					Evidence: []DiagnosticEvidence{
						{Key: "block_height_fixed", Value: strconv.FormatInt(int64(block.Height), 10)},
						{Key: "overflow_fixed", Value: strconv.FormatInt(int64(overflow), 10)},
						{Key: "region_height_fixed", Value: strconv.FormatInt(int64(placement.capacity), 10)},
					},
				})
			}
		}
		planInput.Pages = append(planInput.Pages, PlannedPage{
			Number: page, Size: master.PageSize,
			Fragments: IndexRange{Start: uint32(start), Count: uint32(len(planInput.Fragments) - start)}, // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		})
	}
	sort.SliceStable(breakSpecs, func(i, j int) bool {
		if breakSpecs[i].from != breakSpecs[j].from {
			return breakSpecs[i].from < breakSpecs[j].from
		}
		return regionOrder(breakSpecs[i].region) < regionOrder(breakSpecs[j].region)
	})
	for _, spec := range breakSpecs {
		planInput.Breaks = append(planInput.Breaks, BreakDecision{
			Reason: spec.reason, FromPage: spec.from, ToPage: spec.to, Region: spec.region,
			Preceding: ids[spec.preceding], Triggering: ids[spec.triggering],
			Required: spec.required, Available: spec.available,
		})
	}
	return NewLayoutPlan(planInput)
}

func validatePageMaster(slot string, master PageMaster) error {
	if !master.ID.valid() {
		return pageMasterGeometryError(ErrPageMasterInvalid, slot, master, "", "master ID is not a canonical lowercase slug", Rect{}, false)
	}
	if err := master.PageSize.Validate(); err != nil || master.PageSize.IsEmpty() {
		return pageMasterGeometryError(ErrPageMasterInvalid, slot, master, "", "page size must have positive valid extents", Rect{}, false)
	}
	regions := []struct {
		id       RegionID
		rect     Rect
		required bool
	}{{RegionHeader, master.Header, false}, {RegionBody, master.Body, true}, {RegionFooter, master.Footer, false}}
	for _, region := range regions {
		if region.rect == (Rect{}) && !region.required {
			continue
		}
		if err := region.rect.Validate(); err != nil {
			return pageMasterGeometryError(ErrPageMasterInvalid, slot, master, region.id, "region geometry is invalid", Rect{}, false)
		}
		if region.rect.IsEmpty() {
			return pageMasterGeometryError(ErrPageMasterRegionEmpty, slot, master, region.id, "enabled region must have positive extents", region.rect, true)
		}
		right, _ := region.rect.Right()
		bottom, _ := region.rect.Bottom()
		if region.rect.X < 0 || region.rect.Y < 0 || right > master.PageSize.Width || bottom > master.PageSize.Height {
			return pageMasterGeometryError(ErrPageMasterRegionOutside, slot, master, region.id, "region lies outside its page", region.rect, true)
		}
	}
	headerBottom, _ := master.Header.Bottom()
	bodyBottom, _ := master.Body.Bottom()
	if master.Header != (Rect{}) && headerBottom > master.Body.Y {
		return pageMasterGeometryError(ErrPageMasterRegionOverlap, slot, master, RegionHeader, "header band overlaps or follows body", master.Header, true)
	}
	if master.Footer != (Rect{}) && bodyBottom > master.Footer.Y {
		return pageMasterGeometryError(ErrPageMasterRegionOverlap, slot, master, RegionFooter, "footer band overlaps or precedes body", master.Footer, true)
	}
	return nil
}

func pageMasterGeometryError(cause error, slot string, master PageMaster, region RegionID, message string, bounds Rect, hasBounds bool) error {
	code := DiagnosticPageMasterRegionInvalid
	if errors.Is(cause, ErrPageMasterRegionEmpty) {
		code = DiagnosticPageMasterRegionEmpty
	} else if errors.Is(cause, ErrPageMasterRegionOverlap) {
		code = DiagnosticPageMasterRegionOverlap
	}
	location := DiagnosticLocation{Region: region, Bounds: bounds, HasBounds: hasBounds}
	return newPlanningError(cause, Diagnostic{
		Code: code, Severity: SeverityError, Stage: StageLayout, Message: message,
		Location: location,
		Evidence: []DiagnosticEvidence{{Key: "master", Value: string(master.ID)}, {Key: "selection_slot", Value: slot}},
	})
}

func pageMasterLimitError(message string, actual, limit uint64) error {
	return newPlanningError(ErrPageMasterWorkLimit, Diagnostic{
		Code: DiagnosticWorkLimit, Severity: SeverityError, Stage: StageLayout, Message: message,
		Evidence: []DiagnosticEvidence{{Key: "actual", Value: strconv.FormatUint(actual, 10)}, {Key: "limit", Value: strconv.FormatUint(limit, 10)}},
	})
}

func regionOrder(region RegionID) int {
	switch region {
	case RegionHeader:
		return 0
	case RegionBody:
		return 1
	case RegionFooter:
		return 2
	default:
		return 3
	}
}
