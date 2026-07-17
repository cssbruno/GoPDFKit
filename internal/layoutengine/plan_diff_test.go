// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"testing"
)

func TestDiffLayoutPlansReportsExactStructuralAndDisplayChanges(t *testing.T) {
	before, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan(before) = %v", err)
	}
	afterInput := coreGlyphPlanInput()
	afterInput.Fragments[0].BorderBox.X++
	afterInput.Fragments[0].ContentBox.X++
	afterInput.Lines[0].Bounds.X++
	afterInput.GlyphRuns[0].Origin.X++
	afterInput.GlyphRuns[0].Codes = "CD"
	afterInput.Commands[0].Bounds.X++
	after, err := NewLayoutPlan(afterInput)
	if err != nil {
		t.Fatalf("NewLayoutPlan(after) = %v", err)
	}
	diff, err := DiffLayoutPlans(before, after)
	if err != nil {
		t.Fatalf("DiffLayoutPlans() = %v", err)
	}
	if diff.Equal || !diff.DisplayListChanged || diff.FragmentChangeTotal != 1 || len(diff.FragmentChanges) != 1 {
		t.Fatalf("diff = %+v", diff)
	}
	change := diff.FragmentChanges[0]
	if change.Kind != PlanDiffModified || change.Identity.Key != "@glyph" || change.Before == nil || change.After == nil {
		t.Fatalf("fragment change = %+v", change)
	}
	first, err := diff.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	second, _ := diff.CanonicalJSON()
	if !bytes.Equal(first, second) {
		t.Fatal("diff JSON is not deterministic")
	}
}

func TestDiffLayoutPlansReturnsExactTotalsWhenBounded(t *testing.T) {
	before, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan(before) = %v", err)
	}
	afterInput := testPlanInput()
	for index := range afterInput.Fragments {
		afterInput.Fragments[index].BorderBox.X++
		afterInput.Fragments[index].ContentBox.X++
	}
	after, err := NewLayoutPlan(afterInput)
	if err != nil {
		t.Fatalf("NewLayoutPlan(after) = %v", err)
	}
	diff, err := DiffLayoutPlansWithLimits(before, after, PlanDiffLimits{MaxPageChanges: 1, MaxFragmentChanges: 1})
	if err != nil {
		t.Fatalf("DiffLayoutPlansWithLimits() = %v", err)
	}
	if diff.FragmentChangeTotal != 2 || len(diff.FragmentChanges) != 1 || !diff.FragmentChangesTruncated {
		t.Fatalf("bounded fragment changes = total %d returned %d cut %v",
			diff.FragmentChangeTotal, len(diff.FragmentChanges), diff.FragmentChangesTruncated)
	}
}

func TestDiffLayoutPlansIdenticalPlansAreEqual(t *testing.T) {
	plan, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	diff, err := DiffLayoutPlans(plan, plan)
	if err != nil || !diff.Equal || diff.PageChangeTotal != 0 || diff.FragmentChangeTotal != 0 || diff.DisplayListChanged {
		t.Fatalf("identical diff = %+v, %v", diff, err)
	}
}
