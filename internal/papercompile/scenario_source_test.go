// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"reflect"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

func TestCompileScenarioSourceExtractsResolvableDeterministicFixtures(t *testing.T) {
	source := "document @invoice:\n" +
		"  scenario @child:\n" +
		"    parent: \"@base\"\n" +
		"    locale: \"pt-BR\"\n" +
		"    value @paid: true\n" +
		"    value @nothing: null\n" +
		"    object @customer:\n" +
		"      value @name: \"Ada\"\n" +
		"      value @score: +12.50\n" +
		"    keyed-list @lines:\n" +
		"      object @line-a:\n" +
		"        value @sku: \"A-1\"\n" +
		"        value @quantity: 2\n" +
		"      value @summary: \"one line\"\n" +
		"  scenario @base:\n" +
		"    locale: \"en-US\"\n" +
		"    value @currency: \"USD\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      text: \"preview\"\n"
	first := CompileScenarioSource("invoice.paper", source)
	second := CompileScenarioSource("invoice.paper", source)
	if !first.OK() || !reflect.DeepEqual(first, second) {
		t.Fatalf("scenario compile = %#v / %#v", first, second)
	}
	want := []paperscenario.Scenario{
		{
			Name: "child", Parent: "base", Locale: "pt-BR",
			Values: []paperscenario.Field{
				{Name: "paid", Value: paperscenario.Value{Kind: paperscenario.Bool, Bool: true}},
				{Name: "nothing", Value: paperscenario.Value{Kind: paperscenario.Null}},
				{Name: "customer", Value: paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{
					{Name: "name", Value: paperscenario.Value{Kind: paperscenario.String, String: "Ada"}},
					{Name: "score", Value: paperscenario.Value{Kind: paperscenario.Number, Number: "12.5"}},
				}}},
				{Name: "lines", Value: paperscenario.Value{Kind: paperscenario.List, List: []paperscenario.Item{
					{Key: "line-a", Value: paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{
						{Name: "sku", Value: paperscenario.Value{Kind: paperscenario.String, String: "A-1"}},
						{Name: "quantity", Value: paperscenario.Value{Kind: paperscenario.Number, Number: "2"}},
					}}},
					{Key: "summary", Value: paperscenario.Value{Kind: paperscenario.String, String: "one line"}},
				}}},
			},
		},
		{Name: "base", Locale: "en-US", Values: []paperscenario.Field{{Name: "currency", Value: paperscenario.Value{Kind: paperscenario.String, String: "USD"}}}},
	}
	if !reflect.DeepEqual(first.Scenarios, want) {
		t.Fatalf("scenarios = %#v\nwant %#v", first.Scenarios, want)
	}
	fixtures, err := paperscenario.Resolve(first.Scenarios, paperscenario.Limits{})
	if err != nil || len(fixtures) != 2 || fixtures[0].Locale != "pt-BR" {
		t.Fatalf("Resolve() = %#v, %v", fixtures, err)
	}

	// Scenario declarations are compile-time data and must not appear as visual
	// nodes in the ordinary layout compiler.
	parsed := paperlang.Parse("invoice.paper", source)
	layoutResult := Compile(parsed.AST)
	if !layoutResult.OK() {
		t.Fatalf("layout compiler did not ignore scenario declarations: %#v", layoutResult.Diagnostics)
	}
}

func TestCompileScenarioSourceReportsLocatedSemanticDiagnostics(t *testing.T) {
	source := "document:\n" +
		"  scenario @broken:\n" +
		"    parent: \"missing\"\n" +
		"    mystery: true\n" +
		"    value @distance: 12pt\n" +
		"    value @same: 1\n" +
		"    value @same: 2\n" +
		"    keyed-list @lines:\n" +
		"      value @one: \"a\"\n" +
		"      value @one: \"b\"\n"
	first := CompileScenarioSource("broken.paper", source)
	second := CompileScenarioSource("broken.paper", source)
	if first.OK() || !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("diagnostics = %#v / %#v", first.Diagnostics, second.Diagnostics)
	}
	codes := make(map[string]bool)
	for _, diagnostic := range first.Diagnostics {
		codes[diagnostic.Code] = true
		if diagnostic.Span.File != "broken.paper" || diagnostic.Span.Start.Line == 0 {
			t.Fatalf("diagnostic is not source located: %#v", diagnostic)
		}
	}
	for _, code := range []string{"PAPER_SCENARIO_PROPERTY", "PAPER_SCENARIO_UNIT", "PAPER_SCENARIO_FIELD_DUPLICATE", "PAPER_SCENARIO_LIST_KEY_DUPLICATE", "PAPER_SCENARIO_PARENT_MISSING"} {
		if !codes[code] {
			t.Fatalf("codes = %#v, want %s; diagnostics=%#v", codes, code, first.Diagnostics)
		}
	}
}

func TestExtractScenariosEnforcesExistingHardLimits(t *testing.T) {
	source := "document:\n  scenario @bounded:\n    object @outer:\n      value @inner: 1\n"
	limits := paperscenario.DefaultLimits()
	limits.MaxNodes = 1
	result := CompileScenarioSourceWithLimits("bounded.paper", source, limits)
	if result.OK() || len(result.Diagnostics) == 0 || result.Diagnostics[len(result.Diagnostics)-1].Code != "PAPER_SCENARIO_NODE_LIMIT" {
		t.Fatalf("limited result = %#v", result)
	}
}

func TestCompileScenarioSourceDetectsParentCycles(t *testing.T) {
	source := "document:\n  scenario @a:\n    parent: \"b\"\n  scenario @b:\n    parent: \"a\"\n"
	result := CompileScenarioSource("cycle.paper", source)
	if result.OK() {
		t.Fatal("cycle unexpectedly accepted")
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "PAPER_SCENARIO_PARENT_CYCLE"
	}
	if !found {
		t.Fatalf("cycle diagnostics = %#v", result.Diagnostics)
	}
}
