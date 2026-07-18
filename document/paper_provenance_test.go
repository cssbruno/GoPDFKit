// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPaperPlanExplainCarriesBindingAndTokenProvenance(t *testing.T) {
	source := "document @report:\n" +
		"  theme: \"@print\"\n" +
		"  theme @base:\n" +
		"    token @font:\n      type: \"string\"\n      value: \"Courier\"\n" +
		"    token @size:\n      type: \"length\"\n      value: 11pt\n" +
		"    token @leading:\n      type: \"length\"\n      value: 14pt\n" +
		"    token @ink:\n      type: \"color\"\n      value: \"#336699\"\n" +
		"  theme @print:\n" +
		"    parent: \"base\"\n" +
		"    token @print-ink:\n      type: \"color\"\n      reference: \"ink\"\n" +
		"  schema @invoice:\n" +
		"    field @total:\n      type: \"number\"\n" +
		"  page:\n" +
		"    width: 160pt\n    height: 80pt\n    margin: 8pt\n" +
		"    body:\n" +
		"      paragraph @message:\n" +
		"        bind: \"@invoice.total\"\n" +
		"        font-token: \"font\"\n" +
		"        size-token: \"size\"\n" +
		"        line-height-token: \"leading\"\n" +
		"        color-token: \"print-ink\"\n" +
		"        text: \"Visible\"\n"

	plan, planned, err := PlanPaper("provenance.paper", source)
	if err != nil || !planned.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", planned, err)
	}
	provenance, err := plan.Provenance()
	if err != nil || len(provenance.Bindings) != 1 || len(provenance.StyleTokens) != 4 || len(provenance.ComputedStyles) != 1 {
		t.Fatalf("provenance = %#v, %v", provenance, err)
	}
	if style := provenance.ComputedStyles[0]; style.Node != "@message" || style.TextStyle == nil || style.TextStyle.FontFamily != "Courier" || style.TextStyle.FontSize != 11 || style.TextStyle.LineHeight != 14 {
		t.Fatalf("computed style provenance = %#v", style)
	}
	if binding := provenance.Bindings[0]; binding.Node != "@message" || binding.Path != "@invoice.total" || binding.Kind != "paragraph" || binding.Source.StartLine == 0 {
		t.Fatalf("binding provenance = %#v", binding)
	}
	var color PaperPlanStyleTokenProvenance
	for _, property := range provenance.StyleTokens {
		if property.Property == "color-token" {
			color = property
		}
	}
	if color.Node != "@message" || color.Theme != "print" || color.Token != "print-ink" || color.Value != "#336699" || len(color.TokenChain) != 2 || color.TokenChain[1].Theme != "base" {
		t.Fatalf("color provenance = %#v", color)
	}

	provenance.Bindings[0].Path = "@mutated"
	again, err := plan.Provenance()
	if err != nil || again.Bindings[0].Path != "@invoice.total" || again.StyleTokens[0].TokenChain[0].Scope != nil {
		t.Fatalf("provenance was not detached = %#v, %v", again, err)
	}

	explanation, err := plan.ExplainContext(context.Background(), []PaperPlanSelector{{Key: "@message", MaxResults: 16}}, 1, 1<<20, 1<<20)
	if err != nil || !bytes.Contains(explanation.JSON(), []byte(`"provenance":{"bindings"`)) || !bytes.Contains(explanation.JSON(), []byte(`"path":"@invoice.total"`)) || !bytes.Contains(explanation.JSON(), []byte(`"style_tokens"`)) || !bytes.Contains(explanation.JSON(), []byte(`"computed_styles"`)) {
		t.Fatalf("explanation provenance = %s / %v", explanation.JSON(), err)
	}
}

func TestPaperPlanTraceFragmentJoinsExactLayoutAndCompilerProvenance(t *testing.T) {
	const source = `document @report:
  theme: "@print"
  theme @print:
    token @font:
      type: "string"
      value: "Courier"
    token @size:
      type: "length"
      value: 11pt
  schema @invoice:
    field @total:
      type: "number"
  page:
    body:
      paragraph @message:
        bind: "@invoice.total"
        font-token: "font"
        size-token: "size"
        text: "Visible"
`
	plan, planned, err := PlanPaper("trace.paper", source)
	if err != nil || !planned.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", planned, err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 {
		t.Fatalf("fragments = %#v", projection.Fragments)
	}
	trace, err := plan.TraceFragment(uint32(projection.Fragments[0].ID))
	if err != nil || trace.PlanHash != plan.Hash() {
		t.Fatalf("TraceFragment() = %s, %v", trace.JSON(), err)
	}
	var decoded struct {
		SourceRevision string `json:"source_revision"`
		Fragment       struct {
			Fragments []struct {
				ID     uint32 `json:"id"`
				Page   uint32 `json:"page"`
				Region string `json:"region"`
				Source struct {
					Key      string `json:"key"`
					Instance string `json:"instance"`
					Source   struct {
						File string `json:"file"`
					} `json:"source"`
				} `json:"source_identity"`
				Semantic *struct {
					Roles []string `json:"roles"`
				} `json:"semantic_ownership"`
			} `json:"fragments"`
		} `json:"fragment"`
		Provenance PaperPlanProvenance `json:"provenance"`
	}
	if err := json.Unmarshal(trace.JSON(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.SourceRevision == "" || len(decoded.Fragment.Fragments) != 1 {
		t.Fatalf("trace identity = %#v", decoded)
	}
	fragment := decoded.Fragment.Fragments[0]
	if fragment.ID != uint32(projection.Fragments[0].ID) || fragment.Page != 1 || fragment.Region != "body" ||
		fragment.Source.Key != "@message" || fragment.Source.Instance != "@message" || fragment.Source.Source.File != "trace.paper" ||
		fragment.Semantic == nil || len(fragment.Semantic.Roles) == 0 || fragment.Semantic.Roles[0] != "paragraph" {
		t.Fatalf("fragment trace = %#v", fragment)
	}
	if len(decoded.Provenance.Bindings) != 1 || decoded.Provenance.Bindings[0].Path != "@invoice.total" ||
		len(decoded.Provenance.StyleTokens) != 2 || len(decoded.Provenance.ComputedStyles) != 1 {
		t.Fatalf("fragment provenance = %#v", decoded.Provenance)
	}
	if _, err := plan.TraceFragment(0); err == nil {
		t.Fatal("TraceFragment(0) unexpectedly succeeded")
	}
}

func TestAnonymousPaperSourceIdentityIsDeterministicAndRevisionScoped(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      paragraph:\n        text: \"Anonymous\"\n"
	first, firstResult, err := PlanPaper("anonymous.paper", source)
	if err != nil || !firstResult.OK() {
		t.Fatalf("first PlanPaper() = %#v, %v", firstResult, err)
	}
	again, againResult, err := PlanPaper("anonymous.paper", source)
	if err != nil || !againResult.OK() {
		t.Fatalf("again PlanPaper() = %#v, %v", againResult, err)
	}
	changed, changedResult, err := PlanPaper("anonymous.paper", strings.Replace(source, "Anonymous", "Changed", 1))
	if err != nil || !changedResult.OK() {
		t.Fatalf("changed PlanPaper() = %#v, %v", changedResult, err)
	}
	firstFragments := first.plan.Projection().Fragments
	againFragments := again.plan.Projection().Fragments
	changedFragments := changed.plan.Projection().Fragments
	if len(firstFragments) != 1 || len(againFragments) != 1 || len(changedFragments) != 1 {
		t.Fatalf("fragment counts = %d/%d/%d", len(firstFragments), len(againFragments), len(changedFragments))
	}
	key := string(firstFragments[0].Key)
	if !strings.HasPrefix(key, "anon/") || key != string(againFragments[0].Key) || key == string(changedFragments[0].Key) ||
		firstFragments[0].Source.File != "anonymous.paper" || firstFragments[0].Source.Start.Offset == firstFragments[0].Source.End.Offset {
		t.Fatalf("anonymous keys = %q / %q / %q, source=%#v", key, againFragments[0].Key, changedFragments[0].Key, firstFragments[0].Source)
	}
	trace, err := first.TraceFragment(uint32(firstFragments[0].ID))
	if err != nil || !bytes.Contains(trace.JSON(), []byte(`"key":"anon/`)) || !bytes.Contains(trace.JSON(), []byte(`"file":"anonymous.paper"`)) {
		t.Fatalf("anonymous trace = %s, %v", trace.JSON(), err)
	}
}

func TestExpandedPaperInstancesRemainDistinctFromSharedDefinition(t *testing.T) {
	const source = `document:
  component @card:
    paragraph @line:
      text: "Card"
  page:
    body:
      use @first:
        component: "@card"
      use @second:
        component: "@card"
`
	plan, result, err := PlanPaper("expanded.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].ID == fragments[1].ID || fragments[0].Instance == fragments[1].Instance ||
		fragments[0].Key == fragments[1].Key || fragments[0].Source.File != "expanded.paper" || fragments[1].Source.File != "expanded.paper" {
		t.Fatalf("expanded fragments = %#v", fragments)
	}
	for _, fragment := range fragments {
		trace, traceErr := plan.TraceFragment(uint32(fragment.ID))
		if traceErr != nil || !bytes.Contains(trace.JSON(), []byte(`"expansions"`)) ||
			!bytes.Contains(trace.JSON(), []byte(`"definition"`)) || !bytes.Contains(trace.JSON(), []byte(`"invocation"`)) {
			t.Fatalf("expanded trace = %s, %v", trace.JSON(), traceErr)
		}
	}
}
