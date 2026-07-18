// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"testing"
)

func TestPlanVerticalFlowContextEnforcesCancellationWorkAndStateLimits(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(60)),
		testFlowBlock(t, 2, "@second", fixedPoints(60)),
	)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := PlanVerticalFlowContext(canceled, input, VerticalFlowLimits{})
	var planning *PlanningError
	if !errors.Is(err, context.Canceled) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticCanceled {
		t.Fatalf("canceled flow = %v", err)
	}

	limits := DefaultVerticalFlowLimits()
	limits.MaxWork = 1
	_, err = PlanVerticalFlowContext(context.Background(), input, limits)
	if !errors.Is(err, ErrFlowWorkLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticWorkLimit {
		t.Fatalf("work-limited flow = %v", err)
	}

	limits = DefaultVerticalFlowLimits()
	limits.MaxBlocks = 1
	_, err = PlanVerticalFlowContext(context.Background(), input, limits)
	if !errors.Is(err, ErrFlowStateLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticResourceLimit {
		t.Fatalf("block-limited flow = %v", err)
	}

	limits = DefaultVerticalFlowLimits()
	limits.MaxPages = 1
	_, err = PlanVerticalFlowContext(context.Background(), input, limits)
	if !errors.Is(err, ErrFlowStateLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticResourceLimit {
		t.Fatalf("page-limited flow = %v", err)
	}

	if _, err := PlanVerticalFlowContext(context.Background(), input, VerticalFlowLimits{MaxBlocks: 1}); !errors.Is(err, ErrFlowLimitsInvalid) {
		t.Fatalf("partial limits = %v", err)
	}
}

func TestPlanVerticalFlowConvenienceAndBoundedEntryPointsAreEquivalent(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(60)),
		testFlowBlock(t, 2, "@second", fixedPoints(60)),
	)
	convenience, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := PlanVerticalFlowContext(context.Background(), input, VerticalFlowLimits{})
	if err != nil {
		t.Fatal(err)
	}
	convenienceHash, _ := convenience.Hash()
	boundedHash, _ := bounded.Hash()
	if convenienceHash != boundedHash {
		t.Fatalf("entry point hashes differ: %s != %s", convenienceHash, boundedHash)
	}
}

func TestPlanVerticalFlowExactFitStaysOnOnePage(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(40)),
		testFlowBlock(t, 2, "@second", fixedPoints(60)),
	)
	plan, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow() = %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("plan.Validate() = %v", err)
	}

	projection := plan.Projection()
	if len(projection.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(projection.Pages))
	}
	if got, want := projection.Pages[0].Fragments, (IndexRange{Start: 0, Count: 2}); got != want {
		t.Fatalf("fragment range = %#v, want %#v", got, want)
	}
	if got, want := projection.Pages[0].Commands, (IndexRange{Start: 0, Count: 0}); got != want {
		t.Fatalf("command range = %#v, want %#v", got, want)
	}
	if got, want := projection.Fragments[0].BorderBox, (Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(80), Height: fixedPoints(40)}); got != want {
		t.Fatalf("first border box = %#v, want %#v", got, want)
	}
	if got, want := projection.Fragments[1].BorderBox, (Rect{X: fixedPoints(10), Y: fixedPoints(60), Width: fixedPoints(80), Height: fixedPoints(60)}); got != want {
		t.Fatalf("second border box = %#v, want %#v", got, want)
	}
	if len(projection.Commands) != 0 {
		t.Fatalf("commands = %#v, want none until a painter can consume real payloads", projection.Commands)
	}
	if len(projection.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", projection.Diagnostics)
	}
	if projection.Fragments[0].Source != input.Blocks[0].Source {
		t.Fatalf("fragment source = %#v, want %#v", projection.Fragments[0].Source, input.Blocks[0].Source)
	}
	if projection.Fragments[0].Region != RegionBody {
		t.Fatalf("fragment region = %q, want %q", projection.Fragments[0].Region, RegionBody)
	}
}

func TestPlanVerticalFlowOneFixedUnitOverflowBreaks(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@nearly-full", fixedPoints(100)-1),
		testFlowBlock(t, 2, "@over-by-one", 2),
	)
	plan, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(projection.Pages))
	}
	if got, want := projection.Fragments[0].Page, uint32(1); got != want {
		t.Fatalf("first fragment page = %d, want %d", got, want)
	}
	if got, want := projection.Fragments[1].Page, uint32(2); got != want {
		t.Fatalf("overflowing fragment page = %d, want %d", got, want)
	}
	if got, want := projection.Fragments[1].BorderBox.Y, fixedPoints(20); got != want {
		t.Fatalf("second-page fragment y = %d, want body start %d", got, want)
	}
	for index, page := range projection.Pages {
		if page.Fragments.Count != 1 || page.Commands.Count != 0 {
			t.Fatalf("page %d ranges = %#v/%#v, want one fragment and no fake commands", index+1, page.Fragments, page.Commands)
		}
	}
	requireBreaks(t, projection.Breaks, []BreakDecision{{
		Reason:     BreakInsufficientRemainingBodySpace,
		FromPage:   1,
		ToPage:     2,
		Region:     RegionBody,
		Preceding:  1,
		Triggering: 2,
		Required:   2,
		Available:  1,
	}})
}

func TestPlanVerticalFlowOversizedBlockAdvancesWithoutBlankPages(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@before", fixedPoints(30)),
		testFlowBlock(t, 2, "@too-tall", fixedPoints(100)+1),
		testFlowBlock(t, 3, "@after", fixedPoints(20)),
	)
	plan, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow() = %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("plan.Validate() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 3 {
		t.Fatalf("pages = %d, want 3", len(projection.Pages))
	}
	for index, page := range projection.Pages {
		if page.Fragments.Count != 1 || page.Commands.Count != 0 {
			t.Fatalf("page %d was blank or overfilled: %#v/%#v", index+1, page.Fragments, page.Commands)
		}
	}
	for index, fragment := range projection.Fragments {
		if got, want := fragment.Page, uint32(index+1); got != want {
			t.Fatalf("fragment %d page = %d, want %d", index, got, want)
		}
		if got, want := fragment.BorderBox.Y, fixedPoints(20); got != want {
			t.Fatalf("fragment %d y = %d, want body start %d", index, got, want)
		}
	}
	if got, want := projection.Fragments[1].BorderBox.Height, fixedPoints(100)+1; got != want {
		t.Fatalf("oversized fragment height = %d, want %d", got, want)
	}
	if len(projection.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one oversized-block diagnostic", projection.Diagnostics)
	}
	diagnostic := projection.Diagnostics[0]
	if diagnostic.Code != DiagnosticUnbreakableTooTall || diagnostic.Severity != SeverityWarning || diagnostic.Stage != StageLayout {
		t.Fatalf("diagnostic = %#v, want layout warning %s", diagnostic, DiagnosticUnbreakableTooTall)
	}
	if diagnostic.Location.Page != 2 || diagnostic.Location.Fragment != 2 || diagnostic.Location.Region != RegionBody || diagnostic.Location.Bounds != projection.Fragments[1].BorderBox {
		t.Fatalf("diagnostic location = %#v, want oversized fragment on page 2", diagnostic.Location)
	}
	if got, want := diagnostic.Evidence, []DiagnosticEvidence{
		{Key: "block_height_fixed", Value: "102401"},
		{Key: "body_height_fixed", Value: "102400"},
		{Key: "overflow_fixed", Value: "1"},
	}; !equalDiagnosticEvidence(got, want) {
		t.Fatalf("diagnostic evidence = %#v, want %#v", got, want)
	}
	requireBreaks(t, projection.Breaks, []BreakDecision{
		{
			Reason:     BreakInsufficientRemainingBodySpace,
			FromPage:   1,
			ToPage:     2,
			Region:     RegionBody,
			Preceding:  1,
			Triggering: 2,
			Required:   fixedPoints(100) + 1,
			Available:  fixedPoints(70),
		},
		{
			Reason:     BreakPreviousFragmentOverflow,
			FromPage:   2,
			ToPage:     3,
			Region:     RegionBody,
			Preceding:  2,
			Triggering: 3,
			Required:   fixedPoints(20),
			Available:  0,
		},
	})
}

func TestPlanVerticalFlowZeroHeightDoesNotCreateTransitionBeforeOversize(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@zero", 0),
		testFlowBlock(t, 2, "@too-tall", fixedPoints(100)+1),
	)
	plan, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(projection.Pages))
	}
	if got, want := projection.Pages[0].Fragments, (IndexRange{Start: 0, Count: 2}); got != want {
		t.Fatalf("fragment range = %#v, want %#v", got, want)
	}
	if projection.Fragments[0].Page != 1 || projection.Fragments[1].Page != 1 {
		t.Fatalf("fragment pages = %d/%d, want both on page 1", projection.Fragments[0].Page, projection.Fragments[1].Page)
	}
	if got, want := projection.Fragments[0].BorderBox.Y, fixedPoints(20); got != want {
		t.Fatalf("zero-height fragment y = %d, want %d", got, want)
	}
	if got, want := projection.Fragments[1].BorderBox.Y, fixedPoints(20); got != want {
		t.Fatalf("oversized fragment y = %d, want %d", got, want)
	}
	requireBreaks(t, projection.Breaks, nil)
	if len(projection.Diagnostics) != 1 || projection.Diagnostics[0].Location.Fragment != 2 {
		t.Fatalf("diagnostics = %#v, want only oversized fragment 2", projection.Diagnostics)
	}
}

func TestPlanVerticalFlowOversizeZeroOversizeUsesPositivePredecessorForBreak(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first-too-tall", fixedPoints(100)+1),
		testFlowBlock(t, 2, "@zero", 0),
		testFlowBlock(t, 3, "@second-too-tall", fixedPoints(100)+1),
	)
	plan, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow() = %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("plan.Validate() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 2 {
		t.Fatalf("pages = %d, want 2", len(projection.Pages))
	}
	if got, want := []uint32{projection.Fragments[0].Page, projection.Fragments[1].Page, projection.Fragments[2].Page}, []uint32{1, 1, 2}; !equalPages(got, want) {
		t.Fatalf("fragment pages = %#v, want %#v", got, want)
	}
	if got, want := projection.Fragments[1].BorderBox.Y, fixedPoints(120)+1; got != want {
		t.Fatalf("zero-height fragment y = %d, want overflowing predecessor bottom %d", got, want)
	}
	requireBreaks(t, projection.Breaks, []BreakDecision{{
		Reason:     BreakPreviousFragmentOverflow,
		FromPage:   1,
		ToPage:     2,
		Region:     RegionBody,
		Preceding:  1,
		Triggering: 3,
		Required:   fixedPoints(100) + 1,
		Available:  0,
	}})
	if len(projection.Diagnostics) != 2 || projection.Diagnostics[0].Location.Fragment != 1 || projection.Diagnostics[1].Location.Fragment != 3 {
		t.Fatalf("diagnostics = %#v, want one per oversized fragment", projection.Diagnostics)
	}
}

func TestPlanVerticalFlowRejectsInvalidGeometryAndBlockInput(t *testing.T) {
	input := testVerticalFlowInput(testFlowBlock(t, 1, "@block", fixedPoints(1)))
	input.Body.Width = 0
	if _, err := PlanVerticalFlow(input); !errors.Is(err, ErrFlowBodyEmpty) {
		t.Fatalf("empty body error = %v, want %v", err, ErrFlowBodyEmpty)
	}

	input = testVerticalFlowInput(testFlowBlock(t, 1, "@block", fixedPoints(1)))
	input.Body.X = -1
	if _, err := PlanVerticalFlow(input); !errors.Is(err, ErrFlowBodyOutsidePage) {
		t.Fatalf("outside body error = %v, want %v", err, ErrFlowBodyOutsidePage)
	}

	input = testVerticalFlowInput(testFlowBlock(t, 1, "@block", -1))
	if _, err := PlanVerticalFlow(input); !errors.Is(err, ErrFlowBlockNegativeHeight) {
		t.Fatalf("negative block error = %v, want %v", err, ErrFlowBlockNegativeHeight)
	}

	input = testVerticalFlowInput(testFlowBlock(t, 1, "@block", fixedPoints(1)))
	input.Blocks[0].Instance = InstanceID(string([]byte{'@', 0xff}))
	if _, err := PlanVerticalFlow(input); err == nil {
		t.Fatal("invalid UTF-8 flow instance unexpectedly validated")
	}
}

func TestPlanVerticalFlowNoBodySpaceReturnsStructuredPlanningError(t *testing.T) {
	input := testVerticalFlowInput(testFlowBlock(t, 1, "@block", fixedPoints(1)))
	input.Body.Width = 0
	_, err := PlanVerticalFlow(input)
	if !errors.Is(err, ErrFlowBodyEmpty) {
		t.Fatalf("errors.Is(err, ErrFlowBodyEmpty) = false for %v", err)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) {
		t.Fatalf("errors.As(%T, *PlanningError) = false", err)
	}
	if !errors.Is(planning.Unwrap(), ErrFlowBodyEmpty) {
		t.Fatalf("PlanningError.Unwrap() = %v, want %v", planning.Unwrap(), ErrFlowBodyEmpty)
	}
	diagnostic := planning.Diagnostic
	if err := diagnostic.Validate(); err != nil {
		t.Fatalf("planning diagnostic Validate() = %v", err)
	}
	if diagnostic.Code != DiagnosticPageRegionNoBodySpace || diagnostic.Severity != SeverityError || diagnostic.Stage != StageLayout {
		t.Fatalf("planning diagnostic = %#v, want PAGE_REGION_NO_BODY_SPACE layout error", diagnostic)
	}
	if diagnostic.Location.Region != RegionBody || !diagnostic.Location.HasBounds || diagnostic.Location.Bounds != input.Body {
		t.Fatalf("planning diagnostic location = %#v, want body region and bounds %#v", diagnostic.Location, input.Body)
	}
}

func TestPlanVerticalFlowEmptyInputCreatesOneValidEmptyPage(t *testing.T) {
	plan, err := PlanVerticalFlow(testVerticalFlowInput())
	if err != nil {
		t.Fatalf("PlanVerticalFlow() = %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("plan.Validate() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 || len(projection.Fragments) != 0 || len(projection.Commands) != 0 {
		t.Fatalf("empty flow projection = %#v, want one empty page and no plan entries", projection)
	}
	if got, want := projection.Pages[0].Fragments, (IndexRange{}); got != want {
		t.Fatalf("empty page fragment range = %#v, want %#v", got, want)
	}
	if got, want := projection.Pages[0].Commands, (IndexRange{}); got != want {
		t.Fatalf("empty page command range = %#v, want %#v", got, want)
	}
}

func TestPlanVerticalFlowIsDeterministicAndDoesNotAliasInputs(t *testing.T) {
	input := testVerticalFlowInput(
		testFlowBlock(t, 1, "@first", fixedPoints(40)),
		testFlowBlock(t, 2, "@second", fixedPoints(60)),
	)
	first, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow(first) = %v", err)
	}
	second, err := PlanVerticalFlow(input)
	if err != nil {
		t.Fatalf("PlanVerticalFlow(second) = %v", err)
	}
	firstHash, err := first.Hash()
	if err != nil {
		t.Fatalf("first.Hash() = %v", err)
	}
	secondHash, err := second.Hash()
	if err != nil {
		t.Fatalf("second.Hash() = %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("hashes differ: %s != %s", firstHash, secondHash)
	}

	input.Blocks[0].Height = 1
	projection := first.Projection()
	if got, want := projection.Fragments[0].BorderBox.Height, fixedPoints(40); got != want {
		t.Fatalf("first plan retained mutated input height %d, want %d", got, want)
	}
}

func testVerticalFlowInput(blocks ...VerticalFlowBlock) VerticalFlowInput {
	return VerticalFlowInput{
		PageSize: Size{Width: fixedPoints(100), Height: fixedPoints(160)},
		Body: Rect{
			X:      fixedPoints(10),
			Y:      fixedPoints(20),
			Width:  fixedPoints(80),
			Height: fixedPoints(100),
		},
		Blocks: blocks,
	}
}

func fixedPoints(points int64) Fixed {
	return Fixed(points * FixedScale)
}

func equalDiagnosticEvidence(left, right []DiagnosticEvidence) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func requireBreaks(t *testing.T, got, want []BreakDecision) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("breaks = %#v, want %#v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("break %d = %#v, want %#v", index, got[index], want[index])
		}
	}
}

func equalPages(left, right []uint32) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func testFlowBlock(t *testing.T, node NodeID, key string, height Fixed) VerticalFlowBlock {
	t.Helper()
	nodeKey, err := NewNodeKey(key)
	if err != nil {
		t.Fatalf("NewNodeKey(%q) = %v", key, err)
	}
	instance, err := NewInstanceID(key + "-instance")
	if err != nil {
		t.Fatalf("NewInstanceID(%q) = %v", key+"-instance", err)
	}
	return VerticalFlowBlock{
		Node:     node,
		Key:      nodeKey,
		Instance: instance,
		Source: SourceSpan{
			File:  "example.paper",
			Start: SourcePosition{Offset: uint64(node - 1), Line: 1, Column: uint32(node)},
			End:   SourcePosition{Offset: uint64(node), Line: 1, Column: uint32(node + 1)},
		},
		Height: height,
	}
}
