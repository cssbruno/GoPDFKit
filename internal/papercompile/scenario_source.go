// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

// ScenarioSourceResult is the deterministic source-to-scenario projection.
// Scenarios remain available for editor inspection when diagnostics are
// present; callers must check OK before publishing or resolving them.
type ScenarioSourceResult struct {
	Scenarios   []paperscenario.Scenario
	Diagnostics []paperlang.Diagnostic
}

func (r ScenarioSourceResult) OK() bool {
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.Severity == paperlang.SeverityError {
			return false
		}
	}
	return true
}

// CompileScenarioSource parses one .paper file and extracts its scenario
// declarations using the scenario package's conservative default limits.
func CompileScenarioSource(file, source string) ScenarioSourceResult {
	return CompileScenarioSourceWithLimits(file, source, paperscenario.Limits{})
}

// CompileScenarioSourceWithLimits is CompileScenarioSource with explicit
// bounded fixture limits. A zero value selects paperscenario.DefaultLimits.
func CompileScenarioSourceWithLimits(file, source string, limits paperscenario.Limits) ScenarioSourceResult {
	parsed := paperlang.Parse(file, source)
	extracted := ExtractScenariosWithLimits(parsed.AST, limits)
	extracted.Diagnostics = append(append([]paperlang.Diagnostic(nil), parsed.Diagnostics...), extracted.Diagnostics...)
	return extracted
}

// ExtractScenarios extracts scenario declarations from an already parsed AST.
func ExtractScenarios(ast paperlang.AST) ScenarioSourceResult {
	return ExtractScenariosWithLimits(ast, paperscenario.Limits{})
}

// ExtractScenariosWithLimits extracts scenario declarations while enforcing
// the same hard caps used by paperscenario.Resolve.
func ExtractScenariosWithLimits(ast paperlang.AST, limits paperscenario.Limits) ScenarioSourceResult {
	a := scenarioSourceAnalysis{limits: limits, byName: make(map[string]int)}
	if limits == (paperscenario.Limits{}) {
		a.limits = paperscenario.DefaultLimits()
	} else if !validScenarioSourceLimits(limits) {
		a.add("PAPER_SCENARIO_LIMITS", "scenario limits are incomplete or exceed hard caps", "use positive limits no greater than paperscenario.DefaultLimits", rootSpan(ast))
		a.limits = paperscenario.DefaultLimits()
	}
	if ast.Root == nil || ast.Root.Kind != paperlang.NodeDocument {
		a.add("PAPER_SCENARIO_DOCUMENT", "scenario extraction requires one document root", "parse a valid document before extracting scenarios", rootSpan(ast))
		return a.result
	}

	for _, member := range ast.Root.Members {
		if member.Node == nil || member.Node.Kind != paperlang.NodeScenario {
			continue
		}
		a.scenario(member.Node)
	}
	a.validateParents()
	return a.result
}

type scenarioSourceAnalysis struct {
	result    ScenarioSourceResult
	limits    paperscenario.Limits
	byName    map[string]int
	metadata  []scenarioSourceMetadata
	nodes     uint32
	listItems uint32
	work      uint64
}

type scenarioSourceMetadata struct {
	node       *paperlang.Node
	parentSpan paperlang.Span
}

func (a *scenarioSourceAnalysis) scenario(node *paperlang.Node) {
	if uint32(len(a.result.Scenarios)) >= a.limits.MaxScenarios { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		a.add("PAPER_SCENARIO_LIMIT", "scenario count exceeds the configured limit", "split declarations or raise the bounded limit", node.HeaderSpan)
		return
	}
	name, valid := sourceName(node.ID)
	if !valid {
		a.add("PAPER_SCENARIO_NAME", "scenario requires a valid readable @name", "use at most 256 bytes of letters, digits, '_' or '-'", node.HeaderSpan)
		return
	}
	if _, duplicate := a.byName[name]; duplicate {
		a.add("PAPER_SCENARIO_DUPLICATE", fmt.Sprintf("scenario @%s is declared more than once", name), "keep one declaration per scenario", node.HeaderSpan)
		return
	}

	scenario := paperscenario.Scenario{Name: name}
	seenProperties := make(map[string]bool)
	fields := make([]*paperlang.Node, 0)
	metadata := scenarioSourceMetadata{node: node}
	for _, member := range node.Members {
		if member.Node != nil {
			fields = append(fields, member.Node)
			continue
		}
		if member.Property == nil {
			continue
		}
		property := member.Property
		if seenProperties[property.Name] {
			a.add("PAPER_SCENARIO_PROPERTY_DUPLICATE", fmt.Sprintf("scenario property %q is repeated", property.Name), "remove the duplicate property", property.Span)
			continue
		}
		seenProperties[property.Name] = true
		switch property.Name {
		case "locale":
			if value, ok := scenarioString(property, a, "locale"); ok {
				scenario.Locale = value
			}
		case "parent":
			metadata.parentSpan = property.Value.Span
			if value, ok := scenarioString(property, a, "parent"); ok {
				value = strings.TrimPrefix(value, "@")
				if !validSourceName(value) {
					a.add("PAPER_SCENARIO_PARENT", "scenario parent is not a valid name", "use a quoted scenario name such as \"base\" or \"@base\"", property.Value.Span)
				} else {
					scenario.Parent = value
				}
			}
		default:
			a.add("PAPER_SCENARIO_PROPERTY", fmt.Sprintf("property %q is not supported on scenario", property.Name), "use only parent and locale properties", property.Span)
		}
	}
	scenario.Values = a.objectFields(fields, 1, 0)
	a.byName[name] = len(a.result.Scenarios)
	a.result.Scenarios = append(a.result.Scenarios, scenario)
	a.metadata = append(a.metadata, metadata)
}

func scenarioString(property *paperlang.Property, a *scenarioSourceAnalysis, name string) (string, bool) {
	if property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		a.add("PAPER_SCENARIO_PROPERTY_TYPE", fmt.Sprintf("scenario %s must be a quoted string", name), "use a quoted string value", property.Value.Span)
		return "", false
	}
	return *property.Value.StringValue, true
}

func (a *scenarioSourceAnalysis) objectFields(nodes []*paperlang.Node, depth uint32, pathBytes uint32) []paperscenario.Field {
	seen := make(map[string]bool)
	fields := make([]paperscenario.Field, 0, len(nodes))
	for _, node := range nodes {
		name, valid := sourceName(node.ID)
		if !isScenarioValueNode(node.Kind) || !valid {
			a.add("PAPER_SCENARIO_FIELD", "scenario fields require value, object, or keyed-list with a readable @name", "write value @name, object @name, or keyed-list @name", node.HeaderSpan)
			continue
		}
		if seen[name] {
			a.add("PAPER_SCENARIO_FIELD_DUPLICATE", fmt.Sprintf("fixture field @%s is repeated in one object", name), "keep one field per object scope", node.HeaderSpan)
			continue
		}
		seen[name] = true
		value, ok := a.value(node, depth, pathSize(pathBytes, name))
		if ok {
			fields = append(fields, paperscenario.Field{Name: name, Value: value})
		}
	}
	return fields
}

func (a *scenarioSourceAnalysis) value(node *paperlang.Node, depth uint32, pathBytes uint32) (paperscenario.Value, bool) {
	if depth > a.limits.MaxDepth {
		a.add("PAPER_SCENARIO_DEPTH", "fixture nesting exceeds the configured depth", "flatten the fixture or raise the bounded depth", node.HeaderSpan)
		return paperscenario.Value{}, false
	}
	if pathBytes > a.limits.MaxPathBytes {
		a.add("PAPER_SCENARIO_PATH_LIMIT", "fixture path exceeds the configured byte limit", "shorten nested field and item names", node.HeaderSpan)
		return paperscenario.Value{}, false
	}
	a.nodes++
	a.work++
	if a.nodes > a.limits.MaxNodes || a.work > a.limits.MaxWork {
		a.add("PAPER_SCENARIO_NODE_LIMIT", "fixture data exceeds the configured node/work limit", "reduce fixture data or raise the bounded limit", node.HeaderSpan)
		return paperscenario.Value{}, false
	}

	children := make([]*paperlang.Node, 0, len(node.Members))
	for _, member := range node.Members {
		if member.Property != nil {
			a.add("PAPER_SCENARIO_VALUE_PROPERTY", fmt.Sprintf("property %q is not supported inside %s", member.Property.Name, node.Kind), "use nested value, object, or keyed-list declarations", member.Property.Span)
			continue
		}
		if member.Node != nil {
			children = append(children, member.Node)
		}
	}

	switch node.Kind {
	case paperlang.NodeValue:
		if node.Value == nil {
			return paperscenario.Value{}, false
		}
		switch scalar := node.Value; scalar.Kind {
		case paperlang.ScalarNull:
			return paperscenario.Value{Kind: paperscenario.Null}, true
		case paperlang.ScalarString:
			if scalar.StringValue != nil {
				return paperscenario.Value{Kind: paperscenario.String, String: *scalar.StringValue}, true
			}
		case paperlang.ScalarBool:
			if scalar.BoolValue != nil {
				return paperscenario.Value{Kind: paperscenario.Bool, Bool: *scalar.BoolValue}, true
			}
		case paperlang.ScalarNumber:
			if scalar.NumberValue != nil && !math.IsNaN(*scalar.NumberValue) && !math.IsInf(*scalar.NumberValue, 0) {
				number := strconv.FormatFloat(*scalar.NumberValue, 'f', -1, 64)
				if *scalar.NumberValue == 0 {
					number = "0"
				}
				return paperscenario.Value{Kind: paperscenario.Number, Number: number}, true
			}
		case paperlang.ScalarUnit:
			a.add("PAPER_SCENARIO_UNIT", "fixture values cannot contain layout units", "use a unitless number and format it at presentation time", scalar.Span)
			return paperscenario.Value{}, false
		}
		a.add("PAPER_SCENARIO_SCALAR", "fixture value has an invalid scalar", "use null, a string, boolean, or finite decimal number", node.Value.Span)
		return paperscenario.Value{}, false
	case paperlang.NodeObject:
		return paperscenario.Value{Kind: paperscenario.Object, Object: a.objectFields(children, depth+1, pathBytes)}, true
	case paperlang.NodeKeyedList:
		items := make([]paperscenario.Item, 0, len(children))
		seen := make(map[string]bool)
		for _, child := range children {
			key, valid := sourceName(child.ID)
			if !isScenarioValueNode(child.Kind) || !valid {
				a.add("PAPER_SCENARIO_LIST_ITEM", "keyed-list items require a fixture declaration with a stable @key", "write value @key, object @key, or keyed-list @key", child.HeaderSpan)
				continue
			}
			if seen[key] {
				a.add("PAPER_SCENARIO_LIST_KEY_DUPLICATE", fmt.Sprintf("stable list key @%s is repeated", key), "use a unique key within the list", child.HeaderSpan)
				continue
			}
			seen[key] = true
			a.listItems++
			if a.listItems > a.limits.MaxListItems {
				a.add("PAPER_SCENARIO_LIST_LIMIT", "keyed-list items exceed the configured limit", "reduce list data or raise the bounded limit", child.HeaderSpan)
				continue
			}
			itemValue, ok := a.value(child, depth+1, pathSize(pathBytes, key))
			if ok {
				items = append(items, paperscenario.Item{Key: key, Value: itemValue})
			}
		}
		return paperscenario.Value{Kind: paperscenario.List, List: items}, true
	default:
		a.add("PAPER_SCENARIO_VALUE_KIND", fmt.Sprintf("%s is not fixture data", node.Kind), "use value, object, or keyed-list", node.HeaderSpan)
		return paperscenario.Value{}, false
	}
}

func (a *scenarioSourceAnalysis) validateParents() {
	for index, scenario := range a.result.Scenarios {
		if scenario.Parent == "" {
			continue
		}
		if _, exists := a.byName[scenario.Parent]; !exists {
			a.add("PAPER_SCENARIO_PARENT_MISSING", fmt.Sprintf("scenario @%s has missing parent @%s", scenario.Name, scenario.Parent), "declare the parent scenario in this document", a.metadata[index].parentSpan)
		}
	}
	state := make([]uint8, len(a.result.Scenarios))
	var visit func(int)
	visit = func(index int) {
		if state[index] == 2 {
			return
		}
		state[index] = 1
		parentName := a.result.Scenarios[index].Parent
		if parent, exists := a.byName[parentName]; exists && parentName != "" {
			switch state[parent] {
			case 1:
				a.add("PAPER_SCENARIO_PARENT_CYCLE", fmt.Sprintf("scenario inheritance cycle reaches @%s", parentName), "remove one parent edge from the cycle", a.metadata[index].parentSpan)
			case 0:
				visit(parent)
			}
		}
		state[index] = 2
	}
	for index := range a.result.Scenarios {
		if state[index] == 0 {
			visit(index)
		}
	}
}

func (a *scenarioSourceAnalysis) add(code, message, hint string, span paperlang.Span) {
	a.result.Diagnostics = append(a.result.Diagnostics, paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span})
}

func sourceName(id string) (string, bool) {
	if !strings.HasPrefix(id, "@") {
		return "", false
	}
	name := strings.TrimPrefix(id, "@")
	return name, validSourceName(name)
}

func validSourceName(name string) bool {
	if name == "" || len(name) > 256 || !utf8.ValidString(name) {
		return false
	}
	for index, character := range name {
		if character != '_' && character != '-' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (index == 0 || character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func isScenarioValueNode(kind paperlang.NodeKind) bool {
	return kind == paperlang.NodeValue || kind == paperlang.NodeObject || kind == paperlang.NodeKeyedList
}

func pathSize(parent uint32, name string) uint32 {
	if parent == 0 {
		return uint32(len(name)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	return parent + 1 + uint32(len(name)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
}

func validScenarioSourceLimits(limits paperscenario.Limits) bool {
	hard := paperscenario.DefaultLimits()
	return limits.MaxScenarios > 0 && limits.MaxScenarios <= hard.MaxScenarios &&
		limits.MaxNodes > 0 && limits.MaxNodes <= hard.MaxNodes &&
		limits.MaxDepth > 0 && limits.MaxDepth <= hard.MaxDepth &&
		limits.MaxListItems > 0 && limits.MaxListItems <= hard.MaxListItems &&
		limits.MaxPathBytes > 0 && limits.MaxPathBytes <= hard.MaxPathBytes &&
		limits.MaxWork > 0 && limits.MaxWork <= hard.MaxWork
}
