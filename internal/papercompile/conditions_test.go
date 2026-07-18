// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperexpr"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompileScenarioEvaluatesVisualWhenWithoutChangingOrdinaryCompile(t *testing.T) {
	t.Parallel()

	const source = `document @doc:
  schema @invoice:
    field @show-title:
      type: "bool"
    field @show-note:
      type: "bool"
  scenario @sample:
    value @show-title: true
    value @show-note: false
  page:
    body:
      heading @title:
        when: "show-title"
        text: "Visible title"
      paragraph @note:
        when: "show-note"
        text: "Hidden note"
      paragraph @always:
        text: "Always"
`
	parsed := paperlang.Parse("conditions.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	scenario := CompileScenario(parsed.AST, "sample")
	if !scenario.OK() {
		t.Fatalf("scenario diagnostics = %#v", scenario.Diagnostics)
	}
	if len(scenario.Document.Body) != 2 {
		t.Fatalf("scenario body = %#v", scenario.Document.Body)
	}
	if got := layout.TextSegmentsPlainText(scenario.Document.Body[0].(layout.HeadingBlock).Segments); got != "Visible title" {
		t.Fatalf("conditional heading = %q", got)
	}
	if got := layout.TextSegmentsPlainText(scenario.Document.Body[1].(layout.ParagraphBlock).Segments); got != "Always" {
		t.Fatalf("unconditional paragraph = %q", got)
	}
	if mappingByID(scenario.Mapping, "@note").ID != "" {
		t.Fatalf("false node retained mapping: %#v", scenario.Mapping.Nodes)
	}
	title := mappingByID(scenario.Mapping, "@title")
	if title.ID == "" || title.Span.File != "conditions.paper" || title.Span.Start.Line != 12 {
		t.Fatalf("true node lost source mapping: %#v", title)
	}

	ordinary := Compile(parsed.AST)
	if !ordinary.OK() || len(ordinary.Document.Body) != 3 {
		t.Fatalf("ordinary compile consulted scenario: body=%#v diagnostics=%#v", ordinary.Document.Body, ordinary.Diagnostics)
	}
}

func TestCompileScenarioEvaluatesRepeatItemRelativeWhen(t *testing.T) {
	t.Parallel()

	const source = `document @doc:
  schema @invoice:
    field @items:
      type: "list"
      item-type: "object"
      max-items: 4
      field @name:
        type: "string"
      field @active:
        type: "bool"
  scenario @sample:
    keyed-list @items:
      object @line-a:
        value @name: "Alpha"
        value @active: true
      object @line-b:
        value @name: "Beta"
        value @active: false
      object @line-c:
        value @name: "Gamma"
        value @active: true
  page:
    body:
      repeat @lines:
        source: "@invoice.items"
        instance-prefix: "lines"
        max-items: 3
        paragraph @line:
          when: "active && name matches \"*a\""
          bind: "name"
          text: "placeholder"
`
	parsed := paperlang.Parse("repeat-conditions.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileScenario(parsed.AST, "sample")
	if !compiled.OK() || len(compiled.Document.Body) != 2 {
		t.Fatalf("compile result = body %#v diagnostics %#v", compiled.Document.Body, compiled.Diagnostics)
	}
	got := make([]string, len(compiled.Document.Body))
	for index := range compiled.Document.Body {
		got[index] = layout.TextSegmentsPlainText(compiled.Document.Body[index].(layout.ParagraphBlock).Segments)
	}
	if !equalStrings(got, []string{"Alpha", "Gamma"}) {
		t.Fatalf("conditional repeat output = %#v", got)
	}
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph && strings.Contains(mapping.InstancePath, "line-b") {
			t.Fatalf("false repeat item retained provenance: %#v", mapping)
		}
	}
	for _, key := range []string{"line-a", "line-c"} {
		found := false
		for _, mapping := range compiled.Mapping.Nodes {
			if mapping.Kind == paperlang.NodeParagraph && strings.Contains(mapping.InstancePath, key) {
				found = mapping.DefinitionSpan.File == "repeat-conditions.paper" && mapping.InvocationSpan.File == "repeat-conditions.paper"
			}
		}
		if !found {
			t.Fatalf("repeat item %s lost instance provenance: %#v", key, compiled.Mapping.Nodes)
		}
	}
}

func TestCompileScenarioDiagnosesWhenPathTypeRuntimeBindingAndLimits(t *testing.T) {
	t.Parallel()

	const base = `document:
  schema @invoice:
    field @active:
      type: "bool"
      required: false
    field @quantity:
      type: "number"
    field @name:
      type: "string"
  scenario @sample:
    value @quantity: 2
    value @name: "Ada"
%s
  page:
    body:
      paragraph:
        when: %s
        text: "conditional"
`
	tests := []struct {
		name       string
		fixture    string
		expression string
		code       string
		limits     *ScenarioCompileLimits
	}{
		{name: "unknown-path", fixture: "    value @active: true", expression: `"missing"`, code: "PAPER_WHEN_PATH"},
		{name: "non-bool-result", fixture: "    value @active: true", expression: `"quantity + 1"`, code: "PAPER_WHEN_TYPE"},
		{name: "matches-type", fixture: "    value @active: true", expression: `"quantity matches \"*\""`, code: "PAPER_WHEN_TYPE"},
		{name: "invalid-match-pattern", fixture: "    value @active: true", expression: `"name matches \"bad\\\\\""`, code: "PAPER_WHEN_EXPRESSION"},
		{name: "missing-runtime-binding", expression: `"active"`, code: "PAPER_WHEN_BINDING"},
		{name: "wrong-property-type", fixture: "    value @active: true", expression: "true", code: "PAPER_WHEN_VALUE"},
	}
	bounded := paperexpr.DefaultLanguageLimits()
	bounded.MaxSourceBytes = 4
	tests = append(tests, struct {
		name       string
		fixture    string
		expression string
		code       string
		limits     *ScenarioCompileLimits
	}{name: "expression-limit", fixture: "    value @active: true", expression: `"active"`, code: "PAPER_WHEN_LIMIT", limits: &ScenarioCompileLimits{Expressions: bounded}})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			parsed := paperlang.Parse(test.name+".paper", fmt.Sprintf(base, test.fixture, test.expression))
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
			}
			var compiled Result
			if test.limits == nil {
				compiled = CompileScenario(parsed.AST, "sample")
			} else {
				compiled = CompileScenarioWithLimits(parsed.AST, "sample", *test.limits)
			}
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("diagnostics = %#v, want %s", compiled.Diagnostics, test.code)
			}
			for _, diagnostic := range compiled.Diagnostics {
				if diagnostic.Code == test.code && (diagnostic.Span.File == "" || diagnostic.Span.Start.Line == 0) {
					t.Fatalf("diagnostic is not source-located: %#v", diagnostic)
				}
			}
		})
	}
}
