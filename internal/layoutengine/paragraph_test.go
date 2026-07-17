// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"reflect"
	"testing"
)

func TestParagraphFragmentExactFitAndOneUnitBoundary(t *testing.T) {
	plan := mustParagraphLinePlan(t, 5, 2, 2, ParagraphBreakPrefer)
	token := plan.Start()

	whole, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(50), RegionEmpty: true, NextRegionCapacity: fixedPoints(50),
	}, token)
	if err != nil {
		t.Fatalf("Fragment(exact) = %v", err)
	}
	if whole.Action != ParagraphPlace || whole.Lines != (IndexRange{Count: 5}) ||
		whole.Height != fixedPoints(50) || whole.Continuation != ContinuationWhole || !whole.Done {
		t.Fatalf("exact fragment = %#v", whole)
	}

	split, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(50) - 1, RegionEmpty: true, NextRegionCapacity: fixedPoints(50),
	}, token)
	if err != nil {
		t.Fatalf("Fragment(one unit short) = %v", err)
	}
	if split.Lines != (IndexRange{Count: 3}) || split.Height != fixedPoints(30) ||
		split.Continuation != ContinuationStart || split.Done || !split.PolicyBreak ||
		split.BreakRequired != fixedPoints(20) {
		t.Fatalf("one-unit-short fragment = %#v, want 3+2 policy split", split)
	}
}

func TestParagraphFragmentDefersOnPartialRegionAndRelaxesOnEmptyRegion(t *testing.T) {
	plan := mustParagraphLinePlan(t, 3, 2, 2, ParagraphBreakPrefer)
	token := plan.Start()
	partial := ParagraphFragmentSpace{Available: fixedPoints(10), NextRegionCapacity: fixedPoints(20)}
	deferred, err := plan.Fragment(partial, token)
	if err != nil {
		t.Fatalf("Fragment(partial) = %v", err)
	}
	if deferred.Action != ParagraphDefer || deferred.Next != token || deferred.Lines.Count != 0 {
		t.Fatalf("partial result = %#v, want unchanged deferral", deferred)
	}

	placed, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20),
	}, token)
	if err != nil {
		t.Fatalf("Fragment(empty) = %v", err)
	}
	if placed.Action != ParagraphPlace || placed.Lines != (IndexRange{Count: 1}) ||
		!placed.RelaxedOrphans || placed.RelaxedWidows || placed.Next.NextLine() != 1 {
		t.Fatalf("empty result = %#v, want 1+2 preserving widows", placed)
	}
	last, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20),
	}, placed.Next)
	if err != nil || !last.Done || last.Lines != (IndexRange{Start: 1, Count: 2}) || last.Continuation != ContinuationEnd {
		t.Fatalf("resumed fragment = %#v, %v", last, err)
	}
}

func TestParagraphFragmentStrictImpossibleConstraintFailsWithoutProgress(t *testing.T) {
	plan := mustParagraphLinePlan(t, 3, 2, 2, ParagraphBreakStrict)
	_, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20),
	}, plan.Start())
	if !errors.Is(err, ErrParagraphConstraintUnsatisfiable) {
		t.Fatalf("strict Fragment() = %v, want %v", err, ErrParagraphConstraintUnsatisfiable)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticParagraphConstraintUnsatisfiable {
		t.Fatalf("strict error = %#v, want structured paragraph diagnostic", err)
	}
}

func TestParagraphShorterThanMinimaFitsWithoutRelaxation(t *testing.T) {
	plan := mustParagraphLinePlan(t, 2, 4, 4, ParagraphBreakStrict)
	result, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20),
	}, plan.Start())
	if err != nil || !result.Done || result.Continuation != ContinuationWhole ||
		result.RelaxedOrphans || result.RelaxedWidows {
		t.Fatalf("short whole paragraph = %#v, %v; constraints should not apply without a split", result, err)
	}
}

func TestPlanParagraphFlowStrictDiagnosticHasKnownPageAndBody(t *testing.T) {
	input := testParagraphFlowInput(3, 2, 2, 2, ParagraphBreakStrict)
	_, err := PlanParagraphFlow(input)
	if !errors.Is(err, ErrParagraphConstraintUnsatisfiable) {
		t.Fatalf("PlanParagraphFlow() = %v, want strict constraint error", err)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) {
		t.Fatalf("PlanParagraphFlow() error = %T, want *PlanningError", err)
	}
	location := planning.Diagnostic.Location
	if location.Page != 1 || location.Region != RegionBody || !location.HasBounds || location.Bounds != input.Body {
		t.Fatalf("strict diagnostic location = %#v, want page 1 body %#v", location, input.Body)
	}
}

func TestParagraphFragmentVariableHeightWidowLookahead(t *testing.T) {
	input := testParagraphLinePlanInput(4, 1, 2, ParagraphBreakPrefer)
	input.Lines[2].Height = fixedPoints(25)
	input.Lines[2].Baseline = fixedPoints(20)
	input.Lines[3].Height = fixedPoints(5)
	input.Lines[3].Baseline = fixedPoints(4)
	plan, err := NewParagraphLinePlan(input)
	if err != nil {
		t.Fatalf("NewParagraphLinePlan() = %v", err)
	}

	deferred, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), NextRegionCapacity: fixedPoints(20),
	}, plan.Start())
	if err != nil || deferred.Action != ParagraphDefer {
		t.Fatalf("partial variable-height result = %#v, %v; want defer", deferred, err)
	}
	placed, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20),
	}, plan.Start())
	if err != nil || placed.Lines.Count != 2 || !placed.RelaxedWidows {
		t.Fatalf("empty variable-height result = %#v, %v; want deterministic widow relaxation", placed, err)
	}
}

func TestRelaxedCarriedWidowMinimumDoesNotWeakenNextToken(t *testing.T) {
	plan := mustParagraphLinePlan(t, 7, 1, 3, ParagraphBreakPrefer)
	first, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(30),
	}, plan.Start())
	if err != nil || first.Lines.Count != 2 || first.Next.leadingMinimum != 3 {
		t.Fatalf("first fragment = %#v, %v; want token carrying three widows", first, err)
	}
	middle, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(30),
	}, first.Next)
	if err != nil {
		t.Fatalf("middle Fragment() = %v", err)
	}
	if middle.Lines != (IndexRange{Start: 2, Count: 2}) || !middle.RelaxedWidows ||
		middle.WidowsApplied != 2 || middle.Next.leadingMinimum != 3 {
		t.Fatalf("middle fragment = %#v, want current minimum relaxed but next minimum preserved", middle)
	}
	last, err := plan.Fragment(ParagraphFragmentSpace{
		Available: fixedPoints(30), RegionEmpty: true, NextRegionCapacity: fixedPoints(30),
	}, middle.Next)
	if err != nil || !last.Done || last.Lines != (IndexRange{Start: 4, Count: 3}) {
		t.Fatalf("last fragment = %#v, %v", last, err)
	}
}

func TestPlanParagraphFlowWidowRebalanceProducesCanonicalLines(t *testing.T) {
	input := testParagraphFlowInput(5, 4, 2, 2, ParagraphBreakPrefer)
	input.Lines[0].OffsetX = fixedPoints(5)
	plan, err := PlanParagraphFlow(input)
	if err != nil {
		t.Fatalf("PlanParagraphFlow() = %v", err)
	}
	projection := plan.Projection()
	if got, want := paragraphFragmentLineCounts(projection), []uint32{3, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fragment line counts = %#v, want %#v", got, want)
	}
	if got, want := []FragmentContinuation{projection.Fragments[0].Continuation, projection.Fragments[1].Continuation},
		[]FragmentContinuation{ContinuationStart, ContinuationEnd}; !reflect.DeepEqual(got, want) {
		t.Fatalf("continuations = %#v, want %#v", got, want)
	}
	for i, line := range projection.Lines {
		if line.Index != uint32(i) || line.Fragment != FragmentID(line.Index/3+1) && i < 3 {
			t.Fatalf("line %d = %#v, want contiguous canonical indexes", i, line)
		}
		if line.Bounds.Y != input.Body.Y+Fixed(i%3)*fixedPoints(10) && i < 3 {
			t.Fatalf("line %d y = %d", i, line.Bounds.Y)
		}
	}
	if len(projection.Breaks) != 1 || projection.Breaks[0].Reason != BreakPaginationConstraint ||
		projection.Breaks[0].Required != fixedPoints(20) || projection.Breaks[0].Available != fixedPoints(10) {
		t.Fatalf("breaks = %#v, want truthful widow-policy evidence", projection.Breaks)
	}
	if len(projection.Diagnostics) != 0 {
		t.Fatalf("normal widow rebalance diagnostics = %#v, want none", projection.Diagnostics)
	}
	if got, want := projection.Lines[0].Bounds.X, input.Body.X+fixedPoints(5); got != want {
		t.Fatalf("first line x = %d, want offset x %d", got, want)
	}
}

func TestPlanParagraphFlowThreePageContinuation(t *testing.T) {
	plan, err := PlanParagraphFlow(testParagraphFlowInput(8, 3, 2, 2, ParagraphBreakPrefer))
	if err != nil {
		t.Fatalf("PlanParagraphFlow() = %v", err)
	}
	projection := plan.Projection()
	if got, want := paragraphFragmentLineCounts(projection), []uint32{3, 3, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("line counts = %#v, want %#v", got, want)
	}
	if got, want := []FragmentContinuation{
		projection.Fragments[0].Continuation, projection.Fragments[1].Continuation, projection.Fragments[2].Continuation,
	}, []FragmentContinuation{ContinuationStart, ContinuationMiddle, ContinuationEnd}; !reflect.DeepEqual(got, want) {
		t.Fatalf("continuations = %#v, want %#v", got, want)
	}
	for i, line := range projection.Lines {
		if line.Index != uint32(i) {
			t.Fatalf("line %d index = %d", i, line.Index)
		}
		fragment := projection.Fragments[line.Fragment-1]
		if fragment.Node != 7 || fragment.Key != "@paragraph" || fragment.Instance != "@paragraph" {
			t.Fatalf("line %d fragment provenance = %#v", i, fragment)
		}
	}
}

func TestParagraphMiddleFragmentAlsoEnforcesOrphans(t *testing.T) {
	plan := mustParagraphLinePlan(t, 5, 3, 1, ParagraphBreakPrefer)
	space := ParagraphFragmentSpace{
		Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20),
	}
	first, err := plan.Fragment(space, plan.Start())
	if err != nil || first.Lines.Count != 2 || !first.RelaxedOrphans {
		t.Fatalf("first fragment = %#v, %v; want relaxed two-line placement", first, err)
	}
	middle, err := plan.Fragment(space, first.Next)
	if err != nil || middle.Lines != (IndexRange{Start: 2, Count: 2}) ||
		middle.Continuation != ContinuationMiddle || !middle.RelaxedOrphans {
		t.Fatalf("middle fragment = %#v, %v; want orphan enforcement and relaxation", middle, err)
	}
}

func TestPlanParagraphFlowOversizedLineAdvancesExactlyOnce(t *testing.T) {
	input := testParagraphFlowInput(3, 4, 1, 1, ParagraphBreakPrefer)
	input.Lines[1].Height = fixedPoints(40) + 1
	input.Lines[1].Baseline = fixedPoints(30)
	plan, err := PlanParagraphFlow(input)
	if err != nil {
		t.Fatalf("PlanParagraphFlow() = %v", err)
	}
	projection := plan.Projection()
	if got, want := paragraphFragmentLineCounts(projection), []uint32{1, 1, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("line counts = %#v, want %#v", got, want)
	}
	if len(projection.Pages) != 3 || projection.Fragments[1].BorderBox.Height != fixedPoints(40)+1 {
		t.Fatalf("oversized projection = %#v", projection)
	}
	if len(projection.Breaks) != 2 || projection.Breaks[1].Reason != BreakPreviousFragmentOverflow {
		t.Fatalf("breaks = %#v, want post-overflow transition", projection.Breaks)
	}
	foundOversize := false
	for _, diagnostic := range projection.Diagnostics {
		if diagnostic.Code == DiagnosticUnbreakableTooTall && diagnostic.Location.Fragment == 2 {
			foundOversize = true
			if diagnostic.Location.Page != 2 || diagnostic.Location.Bounds != projection.Fragments[1].BorderBox {
				t.Fatalf("oversized diagnostic location = %#v", diagnostic.Location)
			}
			evidence := diagnosticEvidenceMap(diagnostic.Evidence)
			for key, want := range map[string]string{
				"line_index": "1", "line_height_fixed": "40961",
				"body_height_fixed": "40960", "overflow_fixed": "1",
			} {
				if evidence[key] != want {
					t.Fatalf("oversized evidence[%q] = %q, want %q; all %#v", key, evidence[key], want, evidence)
				}
			}
		}
	}
	if !foundOversize {
		t.Fatalf("diagnostics = %#v, want oversized line 1", projection.Diagnostics)
	}
}

func TestParagraphPlanTokensAreBoundAndInputIsCopied(t *testing.T) {
	input := testParagraphLinePlanInput(5, 2, 2, ParagraphBreakPrefer)
	first, err := NewParagraphLinePlan(input)
	if err != nil {
		t.Fatalf("NewParagraphLinePlan(first) = %v", err)
	}
	foreign := mustParagraphLinePlan(t, 6, 2, 2, ParagraphBreakPrefer)
	if _, err := first.Fragment(ParagraphFragmentSpace{Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20)}, foreign.Start()); !errors.Is(err, ErrParagraphToken) {
		t.Fatalf("foreign token error = %v, want %v", err, ErrParagraphToken)
	}
	input.Lines[0].Height = fixedPoints(99)
	result, err := first.Fragment(ParagraphFragmentSpace{Available: fixedPoints(20), RegionEmpty: true, NextRegionCapacity: fixedPoints(20)}, first.Start())
	if err != nil || result.Height != fixedPoints(20) {
		t.Fatalf("fragment after input mutation = %#v, %v; plan retained alias", result, err)
	}
}

func TestParagraphTokensAreStableAdvanceStrictlyAndRejectCompletedState(t *testing.T) {
	plan := mustParagraphLinePlan(t, 8, 2, 2, ParagraphBreakPrefer)
	space := ParagraphFragmentSpace{
		Available: fixedPoints(30), RegionEmpty: true, NextRegionCapacity: fixedPoints(30),
	}
	first, err := plan.Fragment(space, plan.Start())
	if err != nil {
		t.Fatalf("Fragment(first) = %v", err)
	}
	repeated, err := plan.Fragment(space, plan.Start())
	if err != nil || !reflect.DeepEqual(first, repeated) {
		t.Fatalf("repeated result = %#v, %v; want %#v", repeated, err, first)
	}

	token := plan.Start()
	var ranges []IndexRange
	for {
		result, err := plan.Fragment(space, token)
		if err != nil {
			t.Fatalf("manual Fragment() = %v", err)
		}
		if result.Lines.Count == 0 || result.Lines.Start != token.NextLine() {
			t.Fatalf("manual result = %#v for token %d", result, token.NextLine())
		}
		ranges = append(ranges, result.Lines)
		if result.Done {
			break
		}
		if result.Next.NextLine() <= token.NextLine() {
			t.Fatalf("token did not advance: %d -> %d", token.NextLine(), result.Next.NextLine())
		}
		token = result.Next
	}
	if got, want := ranges, []IndexRange{{Count: 3}, {Start: 3, Count: 3}, {Start: 6, Count: 2}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("manual ranges = %#v, want %#v", got, want)
	}

	completed := plan.Start()
	completed.nextLine = uint32(len(plan.lines))
	if _, err := plan.Fragment(space, completed); !errors.Is(err, ErrParagraphToken) {
		t.Fatalf("completed token error = %v, want %v", err, ErrParagraphToken)
	}
	outOfRange := plan.Start()
	outOfRange.nextLine = uint32(len(plan.lines) + 1)
	if _, err := plan.Fragment(space, outOfRange); !errors.Is(err, ErrParagraphToken) {
		t.Fatalf("out-of-range token error = %v, want %v", err, ErrParagraphToken)
	}
	if _, err := plan.Fragment(space, ParagraphBreakToken{}); !errors.Is(err, ErrParagraphToken) {
		t.Fatalf("zero token error = %v, want %v", err, ErrParagraphToken)
	}
}

func TestPlanParagraphFlowPreservesSourceProvenanceAndRelaxationEvidence(t *testing.T) {
	input := testParagraphFlowInput(3, 2, 2, 2, ParagraphBreakPrefer)
	input.Source = testParagraphSourceSpan("paragraph.paper", 0, 1, 1, 30, 1, 31)
	for index := range input.Lines {
		input.Lines[index].Source = testParagraphSourceSpan(
			"paragraph.paper", uint64(index*10), uint32(index+1), 1,
			uint64(index*10+9), uint32(index+1), 10,
		)
	}
	plan, err := PlanParagraphFlow(input)
	if err != nil {
		t.Fatalf("PlanParagraphFlow() = %v", err)
	}
	projection := plan.Projection()
	for _, fragment := range projection.Fragments {
		if fragment.Source != input.Source {
			t.Fatalf("fragment source = %#v, want %#v", fragment.Source, input.Source)
		}
	}
	for index, line := range projection.Lines {
		if line.Source != input.Lines[index].Source {
			t.Fatalf("line %d source = %#v, want %#v", index, line.Source, input.Lines[index].Source)
		}
	}
	if len(projection.Diagnostics) != 1 || projection.Diagnostics[0].Code != DiagnosticParagraphConstraintRelaxed {
		t.Fatalf("diagnostics = %#v, want one relaxation warning", projection.Diagnostics)
	}
	diagnostic := projection.Diagnostics[0]
	if diagnostic.Location.Fragment != 1 || diagnostic.Location.Page != 1 ||
		diagnostic.Location.Source != input.Source || diagnostic.Location.Bounds != projection.Fragments[0].BorderBox {
		t.Fatalf("relaxation location = %#v", diagnostic.Location)
	}
	evidence := diagnosticEvidenceMap(diagnostic.Evidence)
	for key, want := range map[string]string{
		"line_count": "3", "orphans_requested": "2", "orphans_applied": "1",
		"orphans_relaxed": "true", "widows_requested": "2", "widows_applied": "2",
		"widows_relaxed": "false",
	} {
		if evidence[key] != want {
			t.Fatalf("relaxation evidence[%q] = %q, want %q; all %#v", key, evidence[key], want, evidence)
		}
	}
}

func TestPlanParagraphFlowDeterminismAndSmallStateSpace(t *testing.T) {
	for lineCount := 1; lineCount <= 12; lineCount++ {
		for capacity := 1; capacity <= 5; capacity++ {
			for minimum := 1; minimum <= 3; minimum++ {
				appliedMinimum := minimum
				if appliedMinimum > lineCount {
					appliedMinimum = lineCount
				}
				input := testParagraphFlowInput(lineCount, capacity, uint32(appliedMinimum), uint32(appliedMinimum), ParagraphBreakPrefer)
				first, err := PlanParagraphFlow(input)
				if err != nil {
					t.Fatalf("lines=%d capacity=%d minimum=%d: %v", lineCount, capacity, minimum, err)
				}
				second, err := PlanParagraphFlow(input)
				if err != nil {
					t.Fatalf("second plan: %v", err)
				}
				firstHash, _ := first.Hash()
				secondHash, _ := second.Hash()
				if firstHash != secondHash {
					t.Fatalf("nondeterministic hashes for lines=%d capacity=%d minimum=%d", lineCount, capacity, minimum)
				}
				projection := first.Projection()
				if len(projection.Lines) != lineCount || len(projection.Pages) > lineCount {
					t.Fatalf("projection sizes = %d lines/%d pages, want %d lines and <= pages", len(projection.Lines), len(projection.Pages), lineCount)
				}
				for i, line := range projection.Lines {
					if line.Index != uint32(i) {
						t.Fatalf("line indexes are not a gap-free range at %d: %#v", i, projection.Lines)
					}
				}
			}
		}
	}
}

func TestLayoutPlanRejectsMalformedPlannedLinesAndContinuation(t *testing.T) {
	plan, err := PlanParagraphFlow(testParagraphFlowInput(5, 3, 2, 2, ParagraphBreakPrefer))
	if err != nil {
		t.Fatalf("PlanParagraphFlow() = %v", err)
	}
	projection := plan.Projection()

	input := layoutPlanInputFromProjection(projection)
	input.Lines[1].Index = 9
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("non-contiguous paragraph line indexes unexpectedly validated")
	}

	input = layoutPlanInputFromProjection(projection)
	input.Lines[input.Pages[1].Lines.Start].Fragment = 1
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("cross-page planned line unexpectedly validated")
	}

	input = layoutPlanInputFromProjection(projection)
	input.Fragments[0].Continuation = ContinuationMiddle
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("paragraph continuation beginning with middle unexpectedly validated")
	}

	input = layoutPlanInputFromProjection(projection)
	input.Fragments[len(input.Fragments)-1].Continuation = ContinuationMiddle
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("paragraph continuation without end unexpectedly validated")
	}

	input = layoutPlanInputFromProjection(projection)
	input.Fragments[1].Source = testParagraphSourceSpan("other.paper", 0, 1, 1, 1, 1, 2)
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("paragraph continuation with changed source unexpectedly validated")
	}

	input = layoutPlanInputFromProjection(projection)
	extra := input.Fragments[len(input.Fragments)-1]
	extra.ID = FragmentID(len(input.Fragments) + 1)
	extra.Continuation = ContinuationWhole
	input.Fragments = append(input.Fragments, extra)
	input.Pages[len(input.Pages)-1].Fragments.Count++
	if _, err := NewLayoutPlan(input); err == nil {
		t.Fatal("zero-line fragment sharing paragraph identity unexpectedly validated")
	}
}

func TestNewParagraphLinePlanRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ParagraphLinePlanInput)
		want   error
	}{
		{"no lines", func(in *ParagraphLinePlanInput) { in.Lines = nil }, ErrParagraphHasNoLines},
		{"zero height", func(in *ParagraphLinePlanInput) { in.Lines[0].Height = 0 }, ErrParagraphLineExtent},
		{"negative width", func(in *ParagraphLinePlanInput) { in.Lines[0].Width = -1 }, ErrParagraphLineExtent},
		{"bad baseline", func(in *ParagraphLinePlanInput) { in.Lines[0].Baseline = in.Lines[0].Height + 1 }, ErrParagraphLineExtent},
		{"zero orphans", func(in *ParagraphLinePlanInput) { in.Orphans = 0 }, ErrParagraphPolicy},
		{"zero widows", func(in *ParagraphLinePlanInput) { in.Widows = 0 }, ErrParagraphPolicy},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := testParagraphLinePlanInput(2, 1, 1, ParagraphBreakPrefer)
			test.mutate(&input)
			_, err := NewParagraphLinePlan(input)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func mustParagraphLinePlan(t *testing.T, lines int, orphans, widows uint32, mode ParagraphBreakMode) ParagraphLinePlan {
	t.Helper()
	plan, err := NewParagraphLinePlan(testParagraphLinePlanInput(lines, orphans, widows, mode))
	if err != nil {
		t.Fatalf("NewParagraphLinePlan() = %v", err)
	}
	return plan
}

func testParagraphLinePlanInput(lines int, orphans, widows uint32, mode ParagraphBreakMode) ParagraphLinePlanInput {
	metrics := make([]ParagraphLineInput, lines)
	for i := range metrics {
		metrics[i] = ParagraphLineInput{Width: fixedPoints(50), Height: fixedPoints(10), Baseline: fixedPoints(8)}
	}
	return ParagraphLinePlanInput{
		Node: 7, Key: "@paragraph", Instance: "@paragraph", Lines: metrics,
		Orphans: orphans, Widows: widows, Mode: mode,
	}
}

func testParagraphFlowInput(lines, capacity int, orphans, widows uint32, mode ParagraphBreakMode) ParagraphFlowInput {
	bodyHeight := fixedPoints(int64(capacity * 10))
	return ParagraphFlowInput{
		PageSize:               Size{Width: fixedPoints(100), Height: bodyHeight + fixedPoints(40)},
		Body:                   Rect{X: fixedPoints(10), Y: fixedPoints(20), Width: fixedPoints(80), Height: bodyHeight},
		ParagraphLinePlanInput: testParagraphLinePlanInput(lines, orphans, widows, mode),
	}
}

func paragraphFragmentLineCounts(projection LayoutPlanProjection) []uint32 {
	counts := make([]uint32, len(projection.Fragments))
	for _, line := range projection.Lines {
		counts[line.Fragment-1]++
	}
	return counts
}

func layoutPlanInputFromProjection(projection LayoutPlanProjection) LayoutPlanInput {
	return LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	}
}

func testParagraphSourceSpan(file string, startOffset uint64, startLine, startColumn uint32, endOffset uint64, endLine, endColumn uint32) SourceSpan {
	return SourceSpan{
		File:  file,
		Start: SourcePosition{Offset: startOffset, Line: startLine, Column: startColumn},
		End:   SourcePosition{Offset: endOffset, Line: endLine, Column: endColumn},
	}
}

func diagnosticEvidenceMap(evidence []DiagnosticEvidence) map[string]string {
	result := make(map[string]string, len(evidence))
	for _, item := range evidence {
		result[item.Key] = item.Value
	}
	return result
}
