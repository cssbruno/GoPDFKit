// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

const repeatSourceFixture = `document @doc:
  schema @invoice:
    field @items:
      type: "list"
      item-type: "object"
      max-items: 10
      field @name:
        type: "string"
      field @active:
        type: "bool"
      field @quantity:
        type: "number"
  scenario @sample:
    keyed-list @items:
      object @line-a:
        value @name: "Alpha"
        value @active: true
        value @quantity: 1
      object @line-b:
        value @name: "Beta"
        value @active: false
        value @quantity: 1
      object @line-c:
        value @name: "Gamma"
        value @active: true
        value @quantity: 1
  page:
    body:
      repeat @visible-lines:
        source: "@invoice.items"
        instance-prefix: "invoice-lines"
        max-items: 2
        when: "active && quantity == 1"
        paragraph @line:
          bind: "name"
          text: "Line"
`

func TestCompileScenarioExpandsStableKeyedRepeat(t *testing.T) {
	t.Parallel()

	parsed := paperlang.Parse("repeat.paper", repeatSourceFixture)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileScenario(parsed.AST, "@sample")
	if !compiled.OK() {
		t.Fatalf("compile diagnostics = %#v", compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 2 {
		t.Fatalf("body = %#v", compiled.Document.Body)
	}
	if got := []string{
		layout.TextSegmentsPlainText(compiled.Document.Body[0].(layout.ParagraphBlock).Segments),
		layout.TextSegmentsPlainText(compiled.Document.Body[1].(layout.ParagraphBlock).Segments),
	}; !equalStrings(got, []string{"Alpha", "Gamma"}) {
		t.Fatalf("evaluated repeat text = %#v", got)
	}
	instances := make([]NodeMapping, 0)
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph && strings.Contains(mapping.ID, "invoice-lines") {
			instances = append(instances, mapping)
		}
	}
	if len(instances) != 2 {
		t.Fatalf("repeat mappings = %#v", compiled.Mapping.Nodes)
	}
	if instances[0].InstancePath != "invoice-lines[line-a]" || instances[1].InstancePath != "invoice-lines[line-c]" {
		t.Fatalf("instance paths = %#v", instances)
	}
	for _, instance := range instances {
		if instance.BindingPath != "@invoice.items[].name" || instance.BindingCollection || instance.DefinitionSpan.File == "" || instance.InvocationSpan.File == "" {
			t.Fatalf("mapping provenance = %#v", instance)
		}
	}

	again := CompileScenario(parsed.AST, "sample")
	if !again.OK() || len(again.Mapping.Nodes) != len(compiled.Mapping.Nodes) {
		t.Fatalf("second compile = %#v", again.Diagnostics)
	}
	for index := range compiled.Mapping.Nodes {
		if compiled.Mapping.Nodes[index].ID != again.Mapping.Nodes[index].ID || compiled.Mapping.Nodes[index].InstancePath != again.Mapping.Nodes[index].InstancePath {
			t.Fatalf("nondeterministic mapping = %#v / %#v", compiled.Mapping.Nodes, again.Mapping.Nodes)
		}
	}
}

func TestCompileDefersDynamicFlowWithoutSelectedScenario(t *testing.T) {
	parsed := paperlang.Parse("repeat.paper", repeatSourceFixture)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := Compile(parsed.AST)
	if !compiled.OK() || len(compiled.Document.Body) != 0 {
		t.Fatalf("neutral compile = body %#v diagnostics %#v", compiled.Document.Body, compiled.Diagnostics)
	}
	found := false
	for _, diagnostic := range compiled.Diagnostics {
		found = found || diagnostic.Code == "PAPER_COMPILE_DYNAMIC_DEFERRED" && diagnostic.Severity == paperlang.SeverityWarning
	}
	if !found {
		t.Fatalf("neutral compile omitted deferred warning: %#v", compiled.Diagnostics)
	}
	selected := CompileScenario(parsed.AST, "@sample")
	if !selected.OK() || len(selected.Document.Body) != 2 {
		t.Fatalf("selected compile after neutral projection = body %#v diagnostics %#v", selected.Document.Body, selected.Diagnostics)
	}
}

func TestCompileScenarioInjectsPrimitiveAndOptionalBindingValues(t *testing.T) {
	t.Parallel()

	const source = `document @doc:
  schema @invoice:
    field @customer:
      type: "string"
    field @quantity:
      type: "number"
    field @paid:
      type: "bool"
    field @note:
      type: "string"
      required: false
  scenario @sample:
    value @customer: "Ada"
    value @quantity: 12.50
    value @paid: true
    value @note: null
  page:
    body:
      heading @customer:
        bind: "@invoice.customer"
        text: "Customer placeholder"
      paragraph @quantity:
        bind: "@invoice.quantity"
        format: "decimal"
        format-locale: "en-US"
        format-min-fraction: 2
        format-max-fraction: 2
        text: "Quantity placeholder"
      paragraph @paid:
        bind: "@invoice.paid"
        text: "Paid placeholder"
      list:
        item:
          paragraph @note:
            bind: "@invoice.note"
            bind-required: false
            text: "Note placeholder"
`
	parsed := paperlang.Parse("primitive-bindings.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileScenario(parsed.AST, "sample")
	if !compiled.OK() {
		t.Fatalf("compile diagnostics = %#v", compiled.Diagnostics)
	}
	got := []string{
		layout.TextSegmentsPlainText(compiled.Document.Body[0].(layout.HeadingBlock).Segments),
		layout.TextSegmentsPlainText(compiled.Document.Body[1].(layout.ParagraphBlock).Segments),
		layout.TextSegmentsPlainText(compiled.Document.Body[2].(layout.ParagraphBlock).Segments),
		layout.TextSegmentsPlainText(compiled.Document.Body[3].(layout.ListBlock).Items[0].Blocks[0].(layout.ParagraphBlock).Segments),
	}
	if !equalStrings(got, []string{"Ada", "12.50", "true", ""}) {
		t.Fatalf("evaluated primitive text = %#v", got)
	}

	ordinary := Compile(parsed.AST)
	if !ordinary.OK() {
		t.Fatalf("ordinary compile diagnostics = %#v", ordinary.Diagnostics)
	}
	if got := layout.TextSegmentsPlainText(ordinary.Document.Body[0].(layout.HeadingBlock).Segments); got != "Customer placeholder" {
		t.Fatalf("ordinary compile unexpectedly selected data: %q", got)
	}
}

func TestCompileScenarioLocaleControlsDeterministicFormatting(t *testing.T) {
	t.Parallel()

	const source = `document @doc:
  schema @invoice:
    field @total:
      type: "number"
  scenario @english:
    locale: "en-US"
    value @total: 1234.5
  scenario @brazil:
    locale: "pt-BR"
    value @total: 1234.5
  page:
    body:
      paragraph @total:
        bind: "@invoice.total"
        format: "decimal"
        format-min-fraction: 2
        format-max-fraction: 2
        text: "placeholder"
`
	parsed := paperlang.Parse("locale-scenarios.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	english := CompileScenario(parsed.AST, "english")
	brazil := CompileScenario(parsed.AST, "brazil")
	if !english.OK() || !brazil.OK() {
		t.Fatalf("compile diagnostics = english %#v, brazil %#v", english.Diagnostics, brazil.Diagnostics)
	}
	englishText := layout.TextSegmentsPlainText(english.Document.Body[0].(layout.ParagraphBlock).Segments)
	brazilText := layout.TextSegmentsPlainText(brazil.Document.Body[0].(layout.ParagraphBlock).Segments)
	if englishText != "1,234.50" || brazilText != "1.234,50" || englishText == brazilText {
		t.Fatalf("locale expansion = english %q, brazil %q", englishText, brazilText)
	}
	again := CompileScenario(parsed.AST, "brazil")
	if !again.OK() || layout.TextSegmentsPlainText(again.Document.Body[0].(layout.ParagraphBlock).Segments) != brazilText {
		t.Fatalf("repeated brazil expansion = %#v", again)
	}
}

func TestCompileScenarioDiagnosesIncompleteBindingFormat(t *testing.T) {
	t.Parallel()
	const source = `document:
  schema @invoice:
    field @total:
      type: "number"
  scenario @sample:
    value @total: 12.5
  page:
    body:
      paragraph:
        bind: "@invoice.total"
        format: "currency"
        format-locale: "en-US"
        text: "placeholder"
`
	parsed := paperlang.Parse("binding-format.paper", source)
	compiled := CompileScenario(parsed.AST, "sample")
	if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, "PAPER_BIND_FORMAT") {
		t.Fatalf("format diagnostics = %#v", compiled.Diagnostics)
	}
}

func TestCompileScenarioDiagnosesRequiredMissingNullAndTypeMismatch(t *testing.T) {
	t.Parallel()

	base := `document:
  schema @invoice:
    field @name:
      type: "string"
  scenario @sample:
%s
  page:
    body:
      paragraph:
        bind: "@invoice.name"
        text: "placeholder"
`
	tests := []struct {
		name  string
		value string
		code  string
	}{
		{name: "missing", value: "", code: "PAPER_BIND_VALUE_MISSING"},
		{name: "null", value: "    value @name: null", code: "PAPER_BIND_VALUE_NULL"},
		{name: "mismatch", value: "    value @name: 42", code: "PAPER_BIND_VALUE_TYPE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed := paperlang.Parse(test.name+"-binding.paper", fmt.Sprintf(base, test.value))
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
			}
			compiled := CompileScenario(parsed.AST, "sample")
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("diagnostics = %#v, want %s", compiled.Diagnostics, test.code)
			}
		})
	}
}

func equalStrings(left, right []string) bool {
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

func TestCompileScenarioSupportsComponentTemplate(t *testing.T) {
	t.Parallel()

	source := strings.Replace(repeatSourceFixture,
		"  page:\n",
		"  component @line-card:\n    paragraph @name:\n      bind: \"name\"\n      text: \"Card\"\n  page:\n", 1)
	source = strings.Replace(source,
		"        paragraph @line:\n          bind: \"name\"\n          text: \"Line\"\n",
		"        use @line:\n          component: \"@line-card\"\n", 1)
	parsed := paperlang.Parse("repeat-component.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileScenario(parsed.AST, "sample")
	if !compiled.OK() || len(compiled.Document.Body) != 2 {
		t.Fatalf("component repeat diagnostics = %#v, body = %#v", compiled.Diagnostics, compiled.Document.Body)
	}
	found := 0
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph && mapping.BindingPath == "@invoice.items[].name" {
			found++
			if !strings.Contains(mapping.InstancePath, "invoice-lines[line-") || mapping.DefinitionSpan.File == "" || mapping.InvocationSpan.File == "" {
				t.Fatalf("component provenance = %#v", mapping)
			}
		}
	}
	if found != 2 {
		t.Fatalf("component mappings = %#v", compiled.Mapping.Nodes)
	}
}

func TestCompileScenarioRejectsInvalidSelectionSchemaPredicateAndBounds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		source   string
		scenario string
		code     string
	}{
		{name: "missing selection", source: repeatSourceFixture, scenario: "", code: "PAPER_REPEAT_SCENARIO_REQUIRED"},
		{name: "unknown selection", source: repeatSourceFixture, scenario: "missing", code: "PAPER_REPEAT_SCENARIO_UNKNOWN"},
		{name: "unknown schema path", source: strings.Replace(repeatSourceFixture, "@invoice.items\"", "@invoice.missing\"", 1), scenario: "sample", code: "PAPER_REPEAT_SCHEMA"},
		{name: "unknown predicate path", source: strings.Replace(repeatSourceFixture, "active && quantity == 1", "missing == 1", 1), scenario: "sample", code: "PAPER_REPEAT_WHEN"},
		{name: "predicate type", source: strings.Replace(repeatSourceFixture, "active && quantity == 1", "quantity + 1", 1), scenario: "sample", code: "PAPER_REPEAT_WHEN_TYPE"},
		{name: "output bound", source: strings.Replace(repeatSourceFixture, "max-items: 2", "max-items: 1", 1), scenario: "sample", code: "PAPER_REPEAT_LIMIT"},
		{name: "multiple templates", source: strings.Replace(repeatSourceFixture, "          text: \"Line\"\n", "          text: \"Line\"\n        text: \"extra\"\n", 1), scenario: "sample", code: "PAPER_REPEAT_TEMPLATE"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed := paperlang.Parse(test.name+".paper", test.source)
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
			}
			compiled := CompileScenario(parsed.AST, test.scenario)
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("diagnostics = %#v, want %s", compiled.Diagnostics, test.code)
			}
		})
	}
}
