// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestExplainLayoutSelectsDiagnosticCodeAndIncludesSemantics(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	explanation, err := plan.ExplainLayoutContext(context.Background(), []StructuralQuery{{
		DiagnosticCode: DiagnosticWorkLimit, MaxResults: 10,
	}}, DefaultExplainLayoutLimits(), 100_000)
	if err != nil {
		t.Fatal(err)
	}
	target := explanation.Targets[0]
	if target.Selector.DiagnosticCode != DiagnosticWorkLimit || target.Selection.Diagnostics.Matches != 3 ||
		len(target.Diagnostics) != 3 || len(target.Fragments) != 3 {
		t.Fatalf("issue explanation = %#v", target)
	}
	for _, diagnostic := range target.Diagnostics {
		if diagnostic.Diagnostic.Code != DiagnosticWorkLimit {
			t.Fatalf("wrong diagnostic selected = %#v", diagnostic)
		}
	}
	invalid := StructuralQuery{DiagnosticCode: "not canonical", MaxResults: 1}
	if _, err := plan.ExplainLayout([]StructuralQuery{invalid}, DefaultExplainLayoutLimits()); !errors.Is(err, ErrStructuralQueryInvalidSelector) {
		t.Fatalf("invalid code error = %v", err)
	}
}

func TestExplainLayoutContextEnforcesCancellationAndWorkAtomically(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	selector := []StructuralQuery{{Node: 1, MaxResults: 10}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := plan.ExplainLayoutContext(ctx, selector, DefaultExplainLayoutLimits(), 100_000); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, LayoutExplanation{}) {
		t.Fatalf("canceled explanation = %#v, %v", got, err)
	}
	if got, err := plan.ExplainLayoutContext(context.Background(), selector, DefaultExplainLayoutLimits(), 1); !errors.Is(err, ErrExplainLayoutInvalidLimits) || !reflect.DeepEqual(got, LayoutExplanation{}) {
		t.Fatalf("bounded explanation = %#v, %v", got, err)
	}
}

func TestExplainLayoutReturnsExactContinuationAndCausalEvidence(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	explanation, err := plan.ExplainLayout(
		[]StructuralQuery{{Fragment: 2, MaxResults: 10}},
		DefaultExplainLayoutLimits(),
	)
	if err != nil {
		t.Fatalf("ExplainLayout() = %v", err)
	}
	if explanation.SchemaVersion != ExplainLayoutSchemaVersion || explanation.PlanHash == "" || len(explanation.Targets) != 1 {
		t.Fatalf("explanation identity = %#v", explanation)
	}
	target := explanation.Targets[0]
	if target.Selection.Fragments != (ExplainLayoutCount{Matches: 1, Returned: 1}) ||
		target.Selection.Diagnostics.Matches != 1 {
		t.Fatalf("direct selection = %#v", target.Selection)
	}
	if got, want := target.Evidence.ContinuationFragments, (ExplainLayoutCount{Matches: 2, Returned: 2}); got != want {
		t.Fatalf("continuation count = %#v, want %#v", got, want)
	}
	if got := []FragmentID{target.ContinuationFragments[0].ID, target.ContinuationFragments[1].ID}; !reflect.DeepEqual(got, []FragmentID{1, 2}) {
		t.Fatalf("continuation IDs = %#v", got)
	}
	if target.ContinuationFragments[0].Continuation != ContinuationStart ||
		target.ContinuationFragments[1].Continuation != ContinuationEnd ||
		target.ContinuationFragments[0].PageSize != (Size{Width: 100, Height: 100}) {
		t.Fatalf("continuation geometry = %#v", target.ContinuationFragments)
	}
	if target.Evidence.Breaks != (ExplainLayoutCount{Matches: 1, Returned: 1}) ||
		target.Breaks[0].Decision.Preceding != 1 || target.Breaks[0].Decision.Triggering != 2 {
		t.Fatalf("causal breaks = %#v / %#v", target.Evidence.Breaks, target.Breaks)
	}
	if target.Evidence.Diagnostics != (ExplainLayoutCount{Matches: 2, Returned: 2}) {
		t.Fatalf("chain diagnostics = %#v", target.Evidence.Diagnostics)
	}
	if target.Fragments[0].Source.Node != 1 || target.Fragments[0].Source.Key != "@alpha" ||
		target.Fragments[0].Page != 2 || target.Fragments[0].Region != RegionBody {
		t.Fatalf("source and positioned evidence = %#v", target.Fragments[0])
	}
}

func TestExplainLayoutProjectsGlyphAndImageResources(t *testing.T) {
	glyphPlan, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("glyph NewLayoutPlan() = %v", err)
	}
	glyphExplanation, err := glyphPlan.ExplainLayout(
		[]StructuralQuery{{Node: 1, MaxResults: 10}}, DefaultExplainLayoutLimits(),
	)
	if err != nil {
		t.Fatalf("glyph ExplainLayout() = %v", err)
	}
	glyphTarget := glyphExplanation.Targets[0]
	if glyphTarget.Evidence.Glyphs != (ExplainLayoutCount{Matches: 1, Returned: 1}) || len(glyphTarget.Images) != 0 {
		t.Fatalf("glyph evidence summary = %#v", glyphTarget.Evidence)
	}
	if glyphTarget.Glyphs[0].CommandIndex != 0 || glyphTarget.Glyphs[0].RunIndex != 0 ||
		glyphTarget.Glyphs[0].Run.Codes != "AB" || glyphTarget.Glyphs[0].Font.Face != CoreFontCourier {
		t.Fatalf("glyph evidence = %#v", glyphTarget.Glyphs[0])
	}

	imagePlan, err := NewLayoutPlan(imagePlanInput())
	if err != nil {
		t.Fatalf("image NewLayoutPlan() = %v", err)
	}
	imageExplanation, err := imagePlan.ExplainLayout(
		[]StructuralQuery{{Fragment: 1, MaxResults: 10}}, DefaultExplainLayoutLimits(),
	)
	if err != nil {
		t.Fatalf("image ExplainLayout() = %v", err)
	}
	imageTarget := imageExplanation.Targets[0]
	if imageTarget.Evidence.Images != (ExplainLayoutCount{Matches: 1, Returned: 1}) || len(imageTarget.Glyphs) != 0 {
		t.Fatalf("image evidence summary = %#v", imageTarget.Evidence)
	}
	if imageTarget.Images[0].ImageIndex != 0 || imageTarget.Images[0].Image.Resource != 1 ||
		imageTarget.Images[0].Resource.Format != ImagePNG || imageTarget.Images[0].Resource.PixelWidth != 300 {
		t.Fatalf("image evidence = %#v", imageTarget.Images[0])
	}
}

func TestExplainLayoutBoundsExpandedEvidenceAndCanonicalJSON(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	selectors := []StructuralQuery{{Fragment: 2, MaxResults: 1}, {Page: 2, MaxResults: 1}}
	limits := DefaultExplainLayoutLimits()
	first, err := plan.ExplainLayout(selectors, limits)
	if err != nil {
		t.Fatalf("ExplainLayout() = %v", err)
	}
	if len(first.Targets) != 2 || first.Targets[0].Evidence.ContinuationFragments !=
		(ExplainLayoutCount{Matches: 2, Returned: 1, Truncated: true}) ||
		first.Targets[0].Evidence.Diagnostics != (ExplainLayoutCount{Matches: 2, Returned: 1, Truncated: true}) {
		t.Fatalf("bounded evidence = %#v", first.Targets)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	second, err := plan.ExplainLayout(selectors, limits)
	if err != nil {
		t.Fatalf("second ExplainLayout() = %v", err)
	}
	secondJSON, err := second.CanonicalJSON()
	if err != nil || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical JSON mismatch: %v\n%s\n%s", err, firstJSON, secondJSON)
	}
	for _, field := range []string{`"schema_version":1`, `"source_identity"`, `"continuation_fragments"`, `"plan_hash"`} {
		if !bytes.Contains(firstJSON, []byte(field)) {
			t.Fatalf("canonical JSON missing %s: %s", field, firstJSON)
		}
	}
}

func TestExplainLayoutReturnsDetachedNestedEvidence(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Diagnostics = []Diagnostic{structuralQueryDiagnostic(1, 1, "@glyph", "@glyph", 1)}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	first, err := plan.ExplainLayout([]StructuralQuery{{Node: 1, MaxResults: 10}}, DefaultExplainLayoutLimits())
	if err != nil {
		t.Fatalf("ExplainLayout() = %v", err)
	}
	first.Targets[0].Glyphs[0].Run.Advances[0] = 999
	first.Targets[0].Diagnostics[0].Diagnostic.Evidence[0].Value = "mutated"
	first.Targets[0].Diagnostics[0].Diagnostic.Related[0].Location.Key = "@mutated"
	second, err := plan.ExplainLayout([]StructuralQuery{{Node: 1, MaxResults: 10}}, DefaultExplainLayoutLimits())
	if err != nil {
		t.Fatalf("second ExplainLayout() = %v", err)
	}
	if second.Targets[0].Glyphs[0].Run.Advances[0] == 999 ||
		second.Targets[0].Diagnostics[0].Diagnostic.Evidence[0].Value != "1" ||
		second.Targets[0].Diagnostics[0].Diagnostic.Related[0].Location.Key != "@glyph" {
		t.Fatalf("explanation exposed plan storage: %#v", second.Targets[0])
	}
}

func TestExplainLayoutRejectsInvalidBoundsAndSelectors(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	defaults := DefaultExplainLayoutLimits()
	tests := []struct {
		name      string
		selectors []StructuralQuery
		limits    ExplainLayoutLimits
		want      error
	}{
		{"no selectors", nil, defaults, ErrExplainLayoutNoSelectors},
		{"zero selector limit", []StructuralQuery{{Node: 1, MaxResults: 1}}, ExplainLayoutLimits{MaxCanonicalBytes: 1}, ErrExplainLayoutInvalidLimits},
		{"too many selectors", []StructuralQuery{{Node: 1, MaxResults: 1}, {Node: 1, MaxResults: 1}}, ExplainLayoutLimits{MaxSelectors: 1, MaxCanonicalBytes: 1000}, ErrExplainLayoutInvalidLimits},
		{"invalid structural selector", []StructuralQuery{{Node: 1}}, defaults, ErrStructuralQueryInvalidLimit},
		{"canonical bytes", []StructuralQuery{{Node: 1, MaxResults: 1}}, ExplainLayoutLimits{MaxSelectors: 1, MaxCanonicalBytes: 1}, ErrExplainLayoutTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := plan.ExplainLayout(test.selectors, test.limits)
			if !errors.Is(err, test.want) || !reflect.DeepEqual(got, LayoutExplanation{}) {
				t.Fatalf("ExplainLayout() = (%#v, %v), want %v", got, err, test.want)
			}
		})
	}
}

func TestExplainLayoutIsSafeForConcurrentReaders(t *testing.T) {
	plan := structuralQueryTestPlan(t)
	const readers = 16
	results := make([][]byte, readers)
	errorsFound := make([]error, readers)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			explanation, err := plan.ExplainLayout(
				[]StructuralQuery{{Node: 1, MaxResults: 10}}, DefaultExplainLayoutLimits(),
			)
			if err != nil {
				errorsFound[index] = err
				return
			}
			results[index], errorsFound[index] = explanation.CanonicalJSON()
		}(index)
	}
	wait.Wait()
	for index := range results {
		if errorsFound[index] != nil {
			t.Fatalf("reader %d = %v", index, errorsFound[index])
		}
		if index > 0 && !bytes.Equal(results[0], results[index]) {
			t.Fatalf("reader %d returned non-deterministic output", index)
		}
		if strings.Contains(string(results[index]), "heuristic") {
			t.Fatalf("reader %d returned generated prose", index)
		}
	}
}
