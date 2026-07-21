// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"reflect"
	"testing"
)

func TestQueryStructureReturnsOrderedMultiPageContext(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	result, err := plan.QueryStructure(StructuralQuery{Node: 1, MaxResults: 10})
	if err != nil {
		t.Fatalf("QueryStructure() = %v", err)
	}
	if got, want := result.Summary, (StructuralQuerySummary{
		Pages:       2,
		Fragments:   StructuralQueryCount{Matches: 2, Returned: 2},
		Lines:       StructuralQueryCount{Matches: 2, Returned: 2},
		Commands:    StructuralQueryCount{Matches: 2, Returned: 2},
		Breaks:      StructuralQueryCount{Matches: 1, Returned: 1},
		Diagnostics: StructuralQueryCount{Matches: 2, Returned: 2},
	}); got != want {
		t.Fatalf("summary = %#v, want %#v", got, want)
	}
	if got := []FragmentID{result.Fragments[0].Fragment.ID, result.Fragments[1].Fragment.ID}; !reflect.DeepEqual(got, []FragmentID{1, 2}) {
		t.Fatalf("fragment order = %#v", got)
	}
	if result.Fragments[0].Index != 0 || result.Fragments[1].Index != 1 ||
		result.Fragments[0].PageIndex != 0 || result.Fragments[1].PageIndex != 0 {
		t.Fatalf("fragment indexes = %#v", result.Fragments)
	}
	if got := []uint32{result.Lines[0].Line.Index, result.Lines[1].Line.Index}; !reflect.DeepEqual(got, []uint32{0, 1}) {
		t.Fatalf("line order = %#v", got)
	}
	if result.Lines[0].Page != 1 || result.Lines[1].Page != 2 ||
		result.Lines[0].Key != "@alpha" || result.Lines[1].Instance != "@alpha" {
		t.Fatalf("line provenance = %#v", result.Lines)
	}
	if got := []uint64{result.Commands[0].Index, result.Commands[1].Index}; !reflect.DeepEqual(got, []uint64{0, 2}) {
		t.Fatalf("command order = %#v", got)
	}
	if !result.Commands[0].HasFragmentProvenance || result.Commands[0].Node != 1 || result.Commands[1].Page != 2 {
		t.Fatalf("command provenance = %#v", result.Commands)
	}
	if result.Breaks[0].Decision.Preceding != 1 || result.Breaks[0].Decision.Triggering != 2 {
		t.Fatalf("break = %#v", result.Breaks[0])
	}
	if got := []uint64{result.Diagnostics[0].Index, result.Diagnostics[1].Index}; !reflect.DeepEqual(got, []uint64{0, 1}) {
		t.Fatalf("diagnostic order = %#v", got)
	}
}

func TestQueryStructurePageIncludesPageLevelCommands(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	result, err := plan.QueryStructure(StructuralQuery{Page: 2, MaxResults: 10})
	if err != nil {
		t.Fatalf("QueryStructure() = %v", err)
	}
	if result.Summary.Pages != 1 || result.Summary.Fragments.Matches != 2 ||
		result.Summary.Lines.Matches != 2 || result.Summary.Commands.Matches != 3 ||
		result.Summary.Breaks.Matches != 1 || result.Summary.Diagnostics.Matches != 1 {
		t.Fatalf("page summary = %#v", result.Summary)
	}
	if got := []uint64{result.Commands[0].Index, result.Commands[1].Index, result.Commands[2].Index}; !reflect.DeepEqual(got, []uint64{2, 3, 4}) {
		t.Fatalf("page commands = %#v", got)
	}
	pageLevel := result.Commands[2]
	if pageLevel.HasFragmentProvenance || pageLevel.Command.Fragment.Valid() || pageLevel.PageIndex != 2 {
		t.Fatalf("page-level command = %#v", pageLevel)
	}
	if result.Diagnostics[0].Diagnostic.Location.Fragment != 3 {
		t.Fatalf("page diagnostic = %#v", result.Diagnostics[0])
	}
}

func TestQueryStructureCombinesSelectorsAndFragmentContext(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	for _, selector := range []StructuralQuery{
		{Key: "@alpha", MaxResults: 10},
		{Instance: "@alpha", MaxResults: 10},
		{Node: 1, Key: "@alpha", Instance: "@alpha", MaxResults: 10},
	} {
		result, err := plan.QueryStructure(selector)
		if err != nil || result.Summary.Fragments.Matches != 2 {
			t.Fatalf("QueryStructure(%#v) = summary %#v, %v", selector, result.Summary, err)
		}
	}

	fragment, err := plan.QueryStructure(StructuralQuery{Fragment: 2, MaxResults: 10})
	if err != nil {
		t.Fatalf("fragment QueryStructure() = %v", err)
	}
	if fragment.Summary.Fragments.Matches != 1 || fragment.Fragments[0].Fragment.ID != 2 ||
		fragment.Summary.Lines.Matches != 1 || fragment.Lines[0].Line.Index != 1 ||
		fragment.Summary.Commands.Matches != 1 || fragment.Summary.Breaks.Matches != 1 ||
		fragment.Summary.Diagnostics.Matches != 1 || fragment.Diagnostics[0].Index != 1 {
		t.Fatalf("fragment query = %#v", fragment)
	}

	empty, err := plan.QueryStructure(StructuralQuery{Node: 99, MaxResults: 10})
	if err != nil || empty.Summary != (StructuralQuerySummary{}) || len(empty.Fragments) != 0 {
		t.Fatalf("empty query = %#v, %v", empty, err)
	}
}

func TestQueryStructureBoundsEveryCategoryAndReportsExactCounts(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	result, err := plan.QueryStructure(StructuralQuery{Page: 2, MaxResults: 1})
	if err != nil {
		t.Fatalf("QueryStructure() = %v", err)
	}
	if len(result.Fragments) != 1 || len(result.Lines) != 1 || len(result.Commands) != 1 ||
		len(result.Breaks) != 1 || len(result.Diagnostics) != 1 {
		t.Fatalf("bounded result lengths = %d/%d/%d/%d/%d", len(result.Fragments), len(result.Lines),
			len(result.Commands), len(result.Breaks), len(result.Diagnostics))
	}
	if result.Summary.Fragments != (StructuralQueryCount{Matches: 2, Returned: 1, Truncated: true}) ||
		result.Summary.Lines != (StructuralQueryCount{Matches: 2, Returned: 1, Truncated: true}) ||
		result.Summary.Commands != (StructuralQueryCount{Matches: 3, Returned: 1, Truncated: true}) ||
		result.Summary.Breaks.Truncated || result.Summary.Diagnostics.Truncated {
		t.Fatalf("bounded summary = %#v", result.Summary)
	}
	if result.Fragments[0].Index != 1 || result.Lines[0].Index != 1 || result.Commands[0].Index != 2 {
		t.Fatalf("truncation did not preserve canonical order: %#v", result)
	}
}

func TestQueryStructureReturnsCopyIsolatedDiagnostics(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	first, err := plan.QueryStructure(StructuralQuery{Node: 1, MaxResults: 10})
	if err != nil {
		t.Fatalf("QueryStructure() = %v", err)
	}
	first.Fragments[0].Fragment.Key = "@mutated"
	first.Lines[0].Line.Bounds.Width = 999
	first.Commands[0].Command.Bounds.Width = 999
	first.Diagnostics[0].Diagnostic.Evidence[0].Value = "mutated"
	first.Diagnostics[0].Diagnostic.Related[0].Location.Key = "@mutated"
	first.Diagnostics[0].Diagnostic.Fixes[0].Value = "mutated"

	second, err := plan.QueryStructure(StructuralQuery{Node: 1, MaxResults: 10})
	if err != nil {
		t.Fatalf("second QueryStructure() = %v", err)
	}
	if second.Fragments[0].Fragment.Key != "@alpha" || second.Lines[0].Line.Bounds.Width == 999 ||
		second.Commands[0].Command.Bounds.Width == 999 || second.Diagnostics[0].Diagnostic.Evidence[0].Value != "1" ||
		second.Diagnostics[0].Diagnostic.Related[0].Location.Key != "@alpha" ||
		second.Diagnostics[0].Diagnostic.Fixes[0].Value != "false" {
		t.Fatalf("query exposed plan storage: %#v", second)
	}
}

func TestQueryStructureRejectsInvalidSelectorsAndLimitsDeterministically(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	tests := []struct {
		name  string
		query StructuralQuery
		want  error
	}{
		{"no selector", StructuralQuery{MaxResults: 1}, ErrStructuralQueryNoSelector},
		{"zero limit", StructuralQuery{Node: 1}, ErrStructuralQueryInvalidLimit},
		{"large limit", StructuralQuery{Node: 1, MaxResults: StructuralQueryMaxResults + 1}, ErrStructuralQueryInvalidLimit},
		{"invalid key", StructuralQuery{Key: " bad", MaxResults: 1}, ErrStructuralQueryInvalidSelector},
		{"invalid instance", StructuralQuery{Instance: "bad\n", MaxResults: 1}, ErrStructuralQueryInvalidSelector},
		{"page missing", StructuralQuery{Page: 3, MaxResults: 1}, ErrStructuralQueryPageNotFound},
		{"fragment missing", StructuralQuery{Fragment: 99, MaxResults: 1}, ErrStructuralQueryFragmentNotFound},
		{"fragment conflict", StructuralQuery{Fragment: 1, Page: 2, MaxResults: 1}, ErrStructuralQuerySelectorConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, firstErr := plan.QueryStructure(test.query)
			second, secondErr := plan.QueryStructure(test.query)
			if !errors.Is(firstErr, test.want) || !errors.Is(secondErr, test.want) ||
				firstErr.Error() != secondErr.Error() || !reflect.DeepEqual(first, StructuralQueryResult{}) ||
				!reflect.DeepEqual(second, StructuralQueryResult{}) {
				t.Fatalf("QueryStructure() = (%#v, %v) / (%#v, %v), want %v", first, firstErr, second, secondErr, test.want)
			}
		})
	}
}

func structuralQueryTestPlan(t *testing.T) LayoutPlan {
	t.Helper()
	input := LayoutPlanInput{
		Pages: []PlannedPage{
			{Number: 1, Size: Size{Width: 100, Height: 100}, Fragments: IndexRange{Count: 1}, Lines: IndexRange{Count: 1}, Commands: IndexRange{Count: 2}},
			{Number: 2, Size: Size{Width: 100, Height: 100}, Fragments: IndexRange{Start: 1, Count: 2}, Lines: IndexRange{Start: 1, Count: 2}, Commands: IndexRange{Start: 2, Count: 3}},
		},
		Fragments: []Fragment{
			{ID: 1, Node: 1, Key: "@alpha", Instance: "@alpha", Page: 1, Region: RegionBody, BorderBox: Rect{X: 10, Y: 10, Width: 20, Height: 10}, ContentBox: Rect{X: 10, Y: 10, Width: 20, Height: 10}, Continuation: ContinuationStart},
			{ID: 2, Node: 1, Key: "@alpha", Instance: "@alpha", Page: 2, Region: RegionBody, BorderBox: Rect{X: 10, Y: 10, Width: 20, Height: 10}, ContentBox: Rect{X: 10, Y: 10, Width: 20, Height: 10}, Continuation: ContinuationEnd},
			{ID: 3, Node: 2, Key: "@beta", Instance: "@beta", Page: 2, Region: RegionBody, BorderBox: Rect{X: 10, Y: 30, Width: 20, Height: 10}, ContentBox: Rect{X: 10, Y: 30, Width: 20, Height: 10}, Continuation: ContinuationWhole},
		},
		Lines: []PlannedLine{
			{Fragment: 1, Index: 0, Bounds: Rect{X: 10, Y: 10, Width: 20, Height: 10}, Baseline: 18},
			{Fragment: 2, Index: 1, Bounds: Rect{X: 10, Y: 10, Width: 20, Height: 10}, Baseline: 18},
			{Fragment: 3, Index: 0, Bounds: Rect{X: 10, Y: 30, Width: 20, Height: 10}, Baseline: 38},
		},
		Commands: []DisplayCommand{
			{Kind: CommandFillPath, Fragment: 1, Bounds: Rect{X: 10, Y: 10, Width: 20, Height: 10}},
			{Kind: CommandSaveState},
			{Kind: CommandFillPath, Fragment: 2, Bounds: Rect{X: 10, Y: 10, Width: 20, Height: 10}},
			{Kind: CommandFillPath, Fragment: 3, Bounds: Rect{X: 10, Y: 30, Width: 20, Height: 10}},
			{Kind: CommandRestoreState},
		},
		Breaks: []BreakDecision{{
			Reason: BreakInsufficientRemainingBodySpace, FromPage: 1, ToPage: 2, Region: RegionBody,
			Preceding: 1, Triggering: 2, Required: 10, Available: 5,
		}},
		Diagnostics: []Diagnostic{
			structuralQueryDiagnostic(1, 1, "@alpha", "@alpha", 1),
			structuralQueryDiagnostic(0, 1, "@alpha", "@alpha", 0),
			structuralQueryDiagnostic(3, 2, "@beta", "@beta", 2),
		},
	}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	return plan
}

func structuralQueryDiagnostic(fragment FragmentID, node NodeID, key NodeKey, instance InstanceID, page uint32) Diagnostic {
	return Diagnostic{
		Code: DiagnosticWorkLimit, Severity: SeverityWarning, Stage: StageLayout, Message: "query fixture",
		Location: DiagnosticLocation{Fragment: fragment, Node: node, Key: key, Instance: instance, Page: page},
		Evidence: []DiagnosticEvidence{{Key: "count", Value: "1"}},
		Related:  []DiagnosticReference{{Code: DiagnosticKeepTooLarge, Location: DiagnosticLocation{Key: key}}},
		Fixes:    []DiagnosticFix{{Kind: FixDisableFeature, Target: key, Property: "enabled", Value: "false"}},
	}
}
