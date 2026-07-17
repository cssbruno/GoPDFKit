// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestComponentExpansionUsesFilledAndDefaultSlotsDeterministically(t *testing.T) {
	const source = "document:\n" +
		"  component @card:\n" +
		"    slot @content:\n" +
		"      type: \"text\"\n" +
		"      paragraph @fallback:\n" +
		"        text: \"Fallback\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      use @first:\n" +
		"        component: \"@card\"\n" +
		"        fill @content:\n" +
		"          paragraph @provided:\n" +
		"            text: \"Provided\"\n" +
		"      use @second:\n" +
		"        component: \"@card\"\n"
	parsed := paperlang.Parse("components.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 2 {
		t.Fatalf("expanded body length = %d", len(compiled.Document.Body))
	}
	texts := []string{
		layout.TextSegmentsPlainText(compiled.Document.Body[0].(layout.ParagraphBlock).Segments),
		layout.TextSegmentsPlainText(compiled.Document.Body[1].(layout.ParagraphBlock).Segments),
	}
	if !reflect.DeepEqual(texts, []string{"Provided", "Fallback"}) {
		t.Fatalf("expanded text = %v", texts)
	}
	mappings := make(map[string]NodeMapping)
	for _, mapping := range compiled.Mapping.Nodes {
		mappings[mapping.ID] = mapping
	}
	provided := mappings["@provided"]
	if provided.BodyIndex != 0 || provided.DefinitionSpan.File != "components.paper" ||
		provided.InvocationSpan.File != "components.paper" || !strings.Contains(provided.InstancePath, "@first/@content") ||
		provided.DefinitionSpan.Start.Offset == provided.InvocationSpan.Start.Offset {
		t.Fatalf("provided provenance = %+v", provided)
	}
	var fallback NodeMapping
	for id, mapping := range mappings {
		if strings.Contains(id, "@second--content--fallback") {
			fallback = mapping
		}
	}
	if fallback.BodyIndex != 1 || fallback.InstancePath == "" || fallback.InvocationSpan.File == "" {
		t.Fatalf("default provenance = %+v; mappings=%+v", fallback, compiled.Mapping.Nodes)
	}
	second := Compile(parsed.AST)
	if !reflect.DeepEqual(compiled.Mapping, second.Mapping) {
		t.Fatalf("expansion mapping is nondeterministic:\n%+v\n%+v", compiled.Mapping, second.Mapping)
	}
}

func TestComponentExpansionSupportsListAndRowColumnSlotContent(t *testing.T) {
	const source = "document:\n  component @layout:\n    slot @main:\n      type: \"blocks\"\n      required: true\n  page:\n    body:\n      use @instance:\n        component: \"@layout\"\n        fill @main:\n          list:\n            item:\n              text: \"One\"\n          row:\n            paragraph:\n              track: \"fraction\"\n              track-weight: 1\n              text: \"Two\"\n"
	parsed := paperlang.Parse("component-layout.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 2 || compiled.Document.Body[0].DocumentBlockKind() != layout.BlockKindList ||
		compiled.Document.Body[1].DocumentBlockKind() != layout.BlockKindRowColumn {
		t.Fatalf("expanded body = %#v", compiled.Document.Body)
	}
}

func TestComponentExpansionReportsContractFailures(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{"unknown", "document:\n  page:\n    body:\n      use @x:\n        component: \"@missing\"\n", "PAPER_COMPONENT_UNKNOWN"},
		{"missing", "document:\n  component @c:\n    slot @s:\n      type: \"blocks\"\n      required: true\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n", "PAPER_SLOT_MISSING"},
		{"duplicate fill", "document:\n  component @c:\n    slot @s:\n      type: \"blocks\"\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n        fill @s:\n          text: \"a\"\n        fill @s:\n          text: \"b\"\n", "PAPER_FILL_DUPLICATE"},
		{"unknown fill", "document:\n  component @c:\n    slot @s:\n      type: \"blocks\"\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n        fill @nope:\n          text: \"a\"\n", "PAPER_FILL_UNKNOWN"},
		{"typed slot", "document:\n  component @c:\n    slot @s:\n      type: \"list\"\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n        fill @s:\n          paragraph:\n            text: \"no\"\n", "PAPER_SLOT_TYPE"},
		{"binding", "document:\n  component @c:\n    data: \"unsupported\"\n    paragraph:\n      text: \"x\"\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n", "PAPER_COMPONENT_BINDING_UNSUPPORTED"},
		{"cycle", "document:\n  component @a:\n    use @b-use:\n      component: \"@b\"\n  component @b:\n    use @a-use:\n      component: \"@a\"\n  page:\n    body:\n      use @root-use:\n        component: \"@a\"\n", "PAPER_COMPONENT_CYCLE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := paperlang.Parse(test.name+".paper", test.source)
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %+v", parsed.Diagnostics)
			}
			compiled := Compile(parsed.AST)
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("compile diagnostics = %+v, want %s", compiled.Diagnostics, test.code)
			}
		})
	}
}

func TestComponentExpansionLimitsAreEnforced(t *testing.T) {
	parsed := paperlang.Parse("limits.paper", "document:\n  component @c:\n    paragraph:\n      text: \"x\"\n  page:\n    body:\n      use @one:\n        component: \"@c\"\n      use @two:\n        component: \"@c\"\n")
	compiled := CompileWithExpansionLimits(parsed.AST, ExpansionLimits{MaxDepth: 4, MaxNodes: 3, MaxComponents: 2})
	if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, "PAPER_COMPONENT_NODE_LIMIT") {
		t.Fatalf("limit diagnostics = %+v", compiled.Diagnostics)
	}
	invalid := CompileWithExpansionLimits(parsed.AST, ExpansionLimits{MaxDepth: 1})
	if invalid.OK() || !hasCompileDiagnostic(invalid.Diagnostics, "PAPER_COMPONENT_LIMITS") {
		t.Fatalf("partial limit diagnostics = %+v", invalid.Diagnostics)
	}
}

func TestTypedComponentPropsSubstituteScalarArgumentsAndDefaults(t *testing.T) {
	const source = "document:\n  component @title-card:\n    prop @title:\n      type: \"string\"\n      required: true\n    prop @level:\n      type: \"number\"\n      default: 2\n    heading:\n      level: \"${level}\"\n      text: \"${title}\"\n  page:\n    body:\n      use @card-one:\n        component: \"@title-card\"\n        arg @title: \"Typed title\"\n"
	parsed := paperlang.Parse("typed-props.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	heading := compiled.Document.Body[0].(layout.HeadingBlock)
	if heading.Level != 2 || layout.TextSegmentsPlainText(heading.Segments) != "Typed title" {
		t.Fatalf("expanded heading = %#v", heading)
	}
	if len(compiled.Mapping.Nodes) == 0 || compiled.Mapping.Nodes[0].DefinitionSpan.File != "typed-props.paper" || compiled.Mapping.Nodes[0].InvocationSpan.File != "typed-props.paper" {
		t.Fatalf("typed prop provenance = %+v", compiled.Mapping.Nodes)
	}
}

func TestTypedComponentPropsRejectMissingUnknownDuplicateAndMismatchedArgs(t *testing.T) {
	tests := []struct {
		name string
		args string
		code string
	}{
		{"missing", "", "PAPER_COMPONENT_ARG_MISSING"},
		{"unknown", "        arg @other: \"x\"\n", "PAPER_COMPONENT_ARG_UNKNOWN"},
		{"duplicate", "        arg @title: \"x\"\n        arg @title: \"y\"\n", "PAPER_COMPONENT_ARG_DUPLICATE"},
		{"type", "        arg @title: 4\n", "PAPER_COMPONENT_ARG_TYPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "document:\n  component @card:\n    prop @title:\n      type: \"string\"\n      required: true\n    text: \"${title}\"\n  page:\n    body:\n      use @one:\n        component: \"@card\"\n" + test.args
			parsed := paperlang.Parse(test.name+".paper", source)
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %+v", parsed.Diagnostics)
			}
			compiled := Compile(parsed.AST)
			if !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("compile diagnostics = %+v, want %s", compiled.Diagnostics, test.code)
			}
		})
	}
}

func TestScenarioQualifiedLayoutSlotsSelectOneTypedFill(t *testing.T) {
	const source = "document:\n  scenario @compact:\n  scenario @expanded:\n  component @card:\n    slot @content:\n      type: \"text\"\n      cardinality: \"one\"\n      required: true\n      layout-affecting: true\n      scenarios: \"@compact, @expanded\"\n  page:\n    body:\n      use @one:\n        component: \"@card\"\n        fill @content:\n          scenario: \"@compact\"\n          paragraph @short:\n            text: \"Short\"\n        fill @content:\n          scenario: \"@expanded\"\n          paragraph @long:\n            text: \"Long\"\n"
	parsed := paperlang.Parse("scenario-slot.paper", source)
	compact := CompileScenario(parsed.AST, "compact")
	expanded := CompileScenario(parsed.AST, "expanded")
	if !parsed.OK() || !compact.OK() || !expanded.OK() {
		t.Fatalf("diagnostics = %+v / %+v / %+v", parsed.Diagnostics, compact.Diagnostics, expanded.Diagnostics)
	}
	if got := layout.TextSegmentsPlainText(compact.Document.Body[0].(layout.ParagraphBlock).Segments); got != "Short" {
		t.Fatalf("compact text = %q", got)
	}
	if got := layout.TextSegmentsPlainText(expanded.Document.Body[0].(layout.ParagraphBlock).Segments); got != "Long" {
		t.Fatalf("expanded text = %q", got)
	}
	ordinary := Compile(parsed.AST)
	if !hasCompileDiagnostic(ordinary.Diagnostics, "PAPER_SLOT_SCENARIO_REQUIRED") {
		t.Fatalf("ordinary diagnostics = %+v", ordinary.Diagnostics)
	}
}

func TestSlotCardinalityAndScenarioContractsRejectAmbiguity(t *testing.T) {
	cardinality := paperlang.Parse("cardinality.paper", "document:\n  component @c:\n    slot @s:\n      cardinality: \"one\"\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n        fill @s:\n          text: \"one\"\n          text: \"two\"\n")
	if result := Compile(cardinality.AST); !hasCompileDiagnostic(result.Diagnostics, "PAPER_SLOT_CARDINALITY") {
		t.Fatalf("cardinality diagnostics = %+v", result.Diagnostics)
	}
	missingNames := paperlang.Parse("scenario-names.paper", "document:\n  component @c:\n    slot @s:\n      layout-affecting: true\n  page:\n    body:\n      use @x:\n        component: \"@c\"\n")
	if result := Compile(missingNames.AST); !hasCompileDiagnostic(result.Diagnostics, "PAPER_SLOT_SCENARIOS_REQUIRED") {
		t.Fatalf("scenario diagnostics = %+v", result.Diagnostics)
	}
}

func hasCompileDiagnostic(diagnostics []paperlang.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
