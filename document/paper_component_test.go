// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlanAndWritePaperExpandsComponentsWithInstanceProvenance(t *testing.T) {
	const source = "document:\n" +
		"  component @card:\n" +
		"    slot @content:\n" +
		"      type: \"text\"\n" +
		"      paragraph @fallback:\n" +
		"        text: \"Fallback\"\n" +
		"  page:\n" +
		"    margin: 20pt\n" +
		"    body:\n" +
		"      use @first:\n" +
		"        component: \"@card\"\n" +
		"        fill @content:\n" +
		"          paragraph @provided:\n" +
		"            text: \"Provided\"\n" +
		"      use @second:\n" +
		"        component: \"@card\"\n"
	plan, result, err := PlanPaper("components.paper", source)
	if err != nil || result.Pages != 1 {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	provided, err := plan.Query(PaperPlanSelector{Key: "@provided", MaxResults: 4})
	if err != nil || !strings.Contains(string(provided.JSON()), `"instance":"@first/@content/@provided"`) {
		t.Fatalf("provided query = %s, %v", provided.JSON(), err)
	}
	fallback, err := plan.Query(PaperPlanSelector{Page: 1, MaxResults: 16})
	if err != nil || !strings.Contains(string(fallback.JSON()), `"instance":"@second/@content/`) {
		t.Fatalf("fallback query = %s, %v", fallback.JSON(), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	painted, err := target.WritePaperPlan(plan)
	if err != nil || painted.Pages != 1 {
		t.Fatalf("WritePaperPlan() = %#v, %v", painted, err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	providedGlyph := bytes.Index(content, []byte("(P) Tj"))
	fallbackGlyph := bytes.Index(content, []byte("(F) Tj"))
	if providedGlyph < 0 || fallbackGlyph <= providedGlyph || !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatalf("expanded component glyph order is absent from PDF page:\n%s", content)
	}
}

func TestPlanPaperScenarioRendersTypedPropsAndQualifiedLayoutSlot(t *testing.T) {
	const source = "document:\n  scenario @compact:\n  scenario @expanded:\n  component @card:\n    prop @title:\n      type: \"string\"\n      required: true\n    heading:\n      level: 2\n      text: \"${title}\"\n    slot @body:\n      type: \"text\"\n      cardinality: \"one\"\n      required: true\n      layout-affecting: true\n      scenarios: \"@compact, @expanded\"\n  page:\n    body:\n      use @one:\n        component: \"@card\"\n        arg @title: \"Typed card\"\n        fill @body:\n          scenario: \"@compact\"\n          paragraph @short:\n            text: \"Compact body\"\n        fill @body:\n          scenario: \"@expanded\"\n          paragraph @long:\n            text: \"Expanded body\"\n"
	plan, result, err := PlanPaperScenario("typed-card.paper", source, "compact")
	if err != nil || result.Pages != 1 {
		t.Fatalf("PlanPaperScenario() = %#v, %v", result, err)
	}
	query, err := plan.Query(PaperPlanSelector{Page: 1, MaxResults: 16})
	if err != nil || !strings.Contains(string(query.JSON()), `"key":"@short"`) || !strings.Contains(string(query.JSON()), `"instance":"@one/@body/@short"`) || strings.Contains(string(query.JSON()), `"key":"@long"`) {
		t.Fatalf("typed scenario query = %s, %v", query.JSON(), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WritePaperPlan(plan); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	if !bytes.Contains(content, []byte("(T) Tj")) || !bytes.Contains(content, []byte("(C) Tj")) {
		t.Fatalf("typed scenario glyphs missing:\n%s", content)
	}
}
