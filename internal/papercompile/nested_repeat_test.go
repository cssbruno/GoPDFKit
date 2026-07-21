// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperrepeat"
	"github.com/cssbruno/paperrune/layout"
)

const nestedRepeatSource = `document @doc:
  schema invoice:
    list object groups:
      max-items: 4
      bool enabled
      list object lines:
        max-items: 8
        string name
        bool visible
  scenario @sample:
    keyed-list @groups:
      object @group-a:
        value @enabled: true
        keyed-list @lines:
          object @line-a:
            value @name: "Alpha"
            value @visible: true
          object @line-b:
            value @name: "Beta"
            value @visible: false
      object @group-b:
        value @enabled: false
        keyed-list @lines:
          object @line-c:
            value @name: "Arc"
            value @visible: true
      object @group-c:
        value @enabled: true
        keyed-list @lines:
          object @line-d:
            value @name: "Delta"
            value @visible: true
          object @line-e:
            value @name: "Atom"
            value @visible: true
  page:
    body:
      repeat @groups:
        source: "groups"
        instance-prefix: "groups"
        max-items: 3
        when: "enabled"
        repeat @lines:
          source: "lines"
          instance-prefix: "lines"
          max-items: 2
          when: "visible && name matches \"A*\""
          paragraph @line:
            bind: "name"
            text: "placeholder"
`

func TestCompileScenarioExpandsNestedStableKeyedRepeatsAndPredicates(t *testing.T) {
	t.Parallel()

	parsed := paperlang.Parse("nested-repeat.paper", nestedRepeatSource)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileScenario(parsed.AST, "sample")
	if !compiled.OK() {
		t.Fatalf("compile diagnostics = %#v", compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 2 {
		t.Fatalf("nested body = %#v", compiled.Document.Body)
	}
	got := []string{
		layout.TextSegmentsPlainText(compiled.Document.Body[0].(layout.ParagraphBlock).Segments),
		layout.TextSegmentsPlainText(compiled.Document.Body[1].(layout.ParagraphBlock).Segments),
	}
	if !equalStrings(got, []string{"Alpha", "Atom"}) {
		t.Fatalf("nested text = %#v", got)
	}
	wantPaths := []string{"groups[group-a]/lines[line-a]", "groups[group-c]/lines[line-e]"}
	found := make([]NodeMapping, 0, 2)
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph && mapping.BindingPath != "" {
			found = append(found, mapping)
		}
	}
	if len(found) != 2 {
		t.Fatalf("nested mappings = %#v", compiled.Mapping.Nodes)
	}
	for index, mapping := range found {
		if mapping.InstancePath != wantPaths[index] || mapping.BindingPath != "@invoice.groups[].lines[].name" ||
			mapping.DefinitionSpan.File == "" || mapping.InvocationSpan.File == "" {
			t.Fatalf("nested mapping[%d] = %#v", index, mapping)
		}
	}
}

func TestNestedRepeatIdentityIsStableAcrossKeyReordering(t *testing.T) {
	t.Parallel()

	reordered := strings.Replace(nestedRepeatSource, `          object @line-a:
            value @name: "Alpha"
            value @visible: true
          object @line-b:
            value @name: "Beta"
            value @visible: false`, `          object @line-b:
            value @name: "Beta"
            value @visible: false
          object @line-a:
            value @name: "Alpha"
            value @visible: true`, 1)
	groupA := strings.Index(reordered, "      object @group-a:\n")
	groupB := strings.Index(reordered, "      object @group-b:\n")
	groupC := strings.Index(reordered, "      object @group-c:\n")
	page := strings.Index(reordered, "  page:\n")
	if groupA < 0 || groupB <= groupA || groupC <= groupB || page <= groupC {
		t.Fatal("failed to locate authored keyed groups")
	}
	reordered = reordered[:groupA] + reordered[groupC:page] + reordered[groupA:groupB] + reordered[groupB:groupC] + reordered[page:]

	first := CompileScenario(paperlang.Parse("first.paper", nestedRepeatSource).AST, "sample")
	second := CompileScenario(paperlang.Parse("second.paper", reordered).AST, "sample")
	if !first.OK() || !second.OK() {
		t.Fatalf("reordered diagnostics = %#v / %#v", first.Diagnostics, second.Diagnostics)
	}
	identity := func(result Result) map[string]string {
		values := make(map[string]string)
		for _, mapping := range result.Mapping.Nodes {
			if mapping.Kind == paperlang.NodeParagraph && mapping.BindingPath != "" {
				values[mapping.InstancePath] = mapping.ID
			}
		}
		return values
	}
	firstIDs, secondIDs := identity(first), identity(second)
	if !reflect.DeepEqual(firstIDs, secondIDs) {
		t.Fatalf("identities changed after nested key reorder: %#v / %#v", firstIDs, secondIDs)
	}
}

func TestNestedRepeatSupportsExpandedComponentTemplate(t *testing.T) {
	t.Parallel()

	source := strings.Replace(nestedRepeatSource, "  scenario @sample:\n", `  component @line-card:
    paragraph @value:
      bind: "name"
      text: "card"
  scenario @sample:
`, 1)
	source = strings.Replace(source, `          paragraph @line:
            bind: "name"
            text: "placeholder"`, `          use @line:
            component: "@line-card"`, 1)
	parsed := paperlang.Parse("nested-component.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileScenario(parsed.AST, "sample")
	if !compiled.OK() || len(compiled.Document.Body) != 2 {
		t.Fatalf("component diagnostics = %#v, body = %#v", compiled.Diagnostics, compiled.Document.Body)
	}
	for _, block := range compiled.Document.Body {
		text := layout.TextSegmentsPlainText(block.(layout.ParagraphBlock).Segments)
		if text != "Alpha" && text != "Atom" {
			t.Fatalf("component nested text = %q", text)
		}
	}
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.Kind == paperlang.NodeParagraph && mapping.BindingPath != "" &&
			(mapping.DefinitionSpan.File == "" || mapping.InvocationSpan.File == "" || !strings.Contains(mapping.InstancePath, "/lines[")) {
			t.Fatalf("component nested provenance = %#v", mapping)
		}
	}
}

func TestNestedRepeatEnforcesCombinedOutputDepthAndWorkLimits(t *testing.T) {
	t.Parallel()

	parsed := paperlang.Parse("nested-limits.paper", nestedRepeatSource)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	tests := []struct {
		name   string
		limits paperrepeat.Limits
	}{
		{name: "combined-output", limits: func() paperrepeat.Limits { value := paperrepeat.DefaultLimits(); value.MaxOutput = 3; return value }()},
		{name: "depth", limits: func() paperrepeat.Limits { value := paperrepeat.DefaultLimits(); value.MaxDepth = 1; return value }()},
		{name: "work", limits: func() paperrepeat.Limits { value := paperrepeat.DefaultLimits(); value.MaxWork = 30; return value }()},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			compiled := CompileScenarioWithLimits(parsed.AST, "sample", ScenarioCompileLimits{Repeats: test.limits})
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, "PAPER_REPEAT_LIMIT") {
				t.Fatalf("%s diagnostics = %#v", test.name, compiled.Diagnostics)
			}
		})
	}
}
