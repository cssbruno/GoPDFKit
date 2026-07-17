// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperrepeat"
	"github.com/cssbruno/gopdfkit/layout"
)

const loopScenarioSource = `document:
  schema @settings:
    field @enabled:
      type: "bool"
  scenario @preview:
    value @enabled: true
  page:
    body:
      loop @copies:
        from: 1
        through: 3
        step: 1
        max-iterations: 3
        instance-prefix: "copies"
        when: "enabled && (loop.first || loop.last)"
        paragraph @copy:
          when: "loop.index == 1 || loop.last"
          text: "Copy"
`

func TestCompileScenarioLowersBoundedLoopWithStableIdentityAndConditions(t *testing.T) {
	parsed := paperlang.Parse("loop.paper", loopScenarioSource)
	before, err := parsed.AST.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	compiled := CompileScenario(parsed.AST, "preview")
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 2 {
		t.Fatalf("body = %#v", compiled.Document.Body)
	}
	paths := make([]string, 0, len(compiled.Mapping.Nodes))
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind != paperlang.NodeParagraph {
			continue
		}
		paths = append(paths, mapping.InstancePath)
		if mapping.DefinitionSpan.File != "loop.paper" || mapping.InvocationSpan.File != "loop.paper" {
			t.Fatalf("provenance = %+v", mapping)
		}
	}
	if !reflect.DeepEqual(paths, []string{"copies[1]", "copies[3]"}) {
		t.Fatalf("instance paths = %v", paths)
	}
	again := CompileScenario(parsed.AST, "preview")
	if !reflect.DeepEqual(compiled.Mapping, again.Mapping) {
		t.Fatalf("nondeterministic mapping:\n%+v\n%+v", compiled.Mapping, again.Mapping)
	}
	after, err := parsed.AST.CanonicalJSON()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("loop lowering mutated source AST: %v\n%s\n%s", err, before, after)
	}
}

func TestCompileScenarioSupportsNestedLoopConditions(t *testing.T) {
	const source = `document:
  scenario @preview:
  page:
    body:
      loop @outer:
        from: 1
        through: 2
        step: 1
        max-iterations: 2
        instance-prefix: "outer"
        loop @inner:
          from: 5
          through: 6
          step: 1
          max-iterations: 2
          instance-prefix: "inner"
          paragraph @line:
            when: "loop.last && loop.index == 6"
            text: "Nested"
`
	parsed := paperlang.Parse("nested-loop.paper", source)
	compiled := CompileScenario(parsed.AST, "preview")
	if !parsed.OK() || !compiled.OK() || len(compiled.Document.Body) != 2 {
		t.Fatalf("diagnostics/body = %+v / %+v / %#v", parsed.Diagnostics, compiled.Diagnostics, compiled.Document.Body)
	}
	paths := make([]string, 0, 2)
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph {
			paths = append(paths, mapping.InstancePath)
		}
	}
	want := []string{"outer[1]/inner[6]", "outer[2]/inner[6]"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("nested paths = %v, want %v", paths, want)
	}
}

func TestLoopLastTracksInclusiveBoundaryForNonDivisibleStride(t *testing.T) {
	source := strings.Replace(loopScenarioSource, "through: 3", "through: 4", 1)
	source = strings.Replace(source, "step: 1", "step: 2", 1)
	source = strings.Replace(source, "enabled && (loop.first || loop.last)", "enabled && loop.last", 1)
	parsed := paperlang.Parse("stride-loop.paper", source)
	compiled := CompileScenario(parsed.AST, "preview")
	if !parsed.OK() || !compiled.OK() || len(compiled.Document.Body) != 1 {
		t.Fatalf("diagnostics/body = %+v / %+v / %#v", parsed.Diagnostics, compiled.Diagnostics, compiled.Document.Body)
	}
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph && mapping.InstancePath != "copies[3]" {
			t.Fatalf("stride last identity = %+v", mapping)
		}
	}
}

func TestLoopRejectsDirectionBoundsWorkDepthAndCancellationDeterministically(t *testing.T) {
	tests := []struct {
		name   string
		source string
		limits ScenarioCompileLimits
		code   string
	}{
		{name: "direction", source: strings.Replace(loopScenarioSource, "step: 1", "step: -1", 1), code: "PAPER_LOOP_DIRECTION"},
		{name: "explicit max", source: strings.Replace(loopScenarioSource, "max-iterations: 3", "max-iterations: 2", 1), code: "PAPER_LOOP_LIMIT"},
		{name: "output", source: loopScenarioSource, limits: loopLimits(func(value *paperrepeat.Limits) { value.MaxOutput = 1 }), code: "PAPER_LOOP_LIMIT"},
		{name: "depth", source: strings.Replace(loopScenarioSource, "paragraph @copy:\n          when: \"loop.index == 1 || loop.last\"\n          text: \"Copy\"", "loop @inner:\n          from: 1\n          through: 1\n          step: 1\n          max-iterations: 1\n          instance-prefix: \"inner\"\n          text: \"Copy\"", 1), limits: loopLimits(func(value *paperrepeat.Limits) { value.MaxDepth = 1 }), code: "PAPER_LOOP_LIMIT"},
		{name: "work", source: loopScenarioSource, limits: loopLimits(func(value *paperrepeat.Limits) { value.MaxWork = 5 }), code: "PAPER_LOOP_LIMIT"},
		{name: "state", source: loopScenarioSource, limits: loopLimits(func(value *paperrepeat.Limits) { value.MaxStateBytes = 32 }), code: "PAPER_LOOP_LIMIT"},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tests = append(tests, struct {
		name   string
		source string
		limits ScenarioCompileLimits
		code   string
	}{name: "cancel", source: loopScenarioSource, limits: ScenarioCompileLimits{Context: ctx}, code: "PAPER_LOOP_CANCELLED"})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := paperlang.Parse(test.name+".paper", test.source)
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %+v", parsed.Diagnostics)
			}
			compiled := CompileScenarioWithLimits(parsed.AST, "preview", test.limits)
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("diagnostics = %+v, want %s", compiled.Diagnostics, test.code)
			}
		})
	}
}

func TestLoopCanLowerInsideTypedRepeatItemContext(t *testing.T) {
	const source = `document:
  schema @invoice:
    field @items:
      type: "list"
      item-type: "object"
      max-items: 2
      field @active:
        type: "bool"
  scenario @preview:
    keyed-list @items:
      object @alpha:
        value @active: true
  page:
    body:
      repeat @items:
        source: "@invoice.items"
        instance-prefix: "items"
        max-items: 1
        loop @copies:
          from: 1
          through: 2
          step: 1
          max-iterations: 2
          instance-prefix: "copies"
          when: "active"
          paragraph @copy:
            when: "active && loop.index == 2"
            text: "Copy"
`
	parsed := paperlang.Parse("repeat-loop.paper", source)
	compiled := CompileScenario(parsed.AST, "preview")
	if !parsed.OK() || !compiled.OK() || len(compiled.Document.Body) != 1 {
		t.Fatalf("diagnostics/body = %+v / %+v / %#v", parsed.Diagnostics, compiled.Diagnostics, compiled.Document.Body)
	}
	if got := layout.TextSegmentsPlainText(compiled.Document.Body[0].(layout.ParagraphBlock).Segments); got != "Copy" {
		t.Fatalf("text = %q", got)
	}
	path := ""
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph {
			path = mapping.InstancePath
		}
	}
	if path != "items[alpha]/copies[2]" {
		t.Fatalf("instance path = %q", path)
	}
}

func loopLimits(change func(*paperrepeat.Limits)) ScenarioCompileLimits {
	limits := paperrepeat.DefaultLimits()
	change(&limits)
	return ScenarioCompileLimits{Repeats: limits}
}
