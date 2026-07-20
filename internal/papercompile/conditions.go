// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cssbruno/paperrune/internal/paperexpr"
	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

// scenarioConditionEvaluator removes visual nodes whose explicitly authored
// `when` expression is false. It runs only from CompileScenario: ordinary
// Compile deliberately retains conditional nodes and never reads fixture data.
type scenarioConditionEvaluator struct {
	ctx         context.Context
	schemas     schemaAnalysis
	fixture     paperscenario.Fixture
	limits      paperexpr.LanguageLimits
	provenance  map[*paperlang.Node]expansionProvenance
	diagnostics []paperlang.Diagnostic
}

func evaluateScenarioConditions(ctx context.Context, ast paperlang.AST, provenance map[*paperlang.Node]expansionProvenance, schemas schemaAnalysis, fixture paperscenario.Fixture, limits paperexpr.LanguageLimits) []paperlang.Diagnostic {
	if ctx == nil {
		ctx = context.Background()
	}
	evaluator := scenarioConditionEvaluator{ctx: ctx, schemas: schemas, fixture: fixture, limits: limits, provenance: provenance}
	if ast.Root != nil {
		evaluator.filterChildren(ast.Root)
	}
	return evaluator.diagnostics
}

func (e *scenarioConditionEvaluator) filterChildren(parent *paperlang.Node) {
	filtered := make([]paperlang.Member, 0, len(parent.Members))
	for _, member := range parent.Members {
		if member.Node == nil {
			filtered = append(filtered, member)
			continue
		}
		include := e.include(member.Node)
		if include {
			e.filterChildren(member.Node)
			filtered = append(filtered, member)
		}
	}
	parent.Members = filtered
}

func (e *scenarioConditionEvaluator) include(node *paperlang.Node) bool {
	condition, duplicate := takeConditionProperty(node)
	if condition == nil {
		return true
	}
	if duplicate != nil {
		e.add("PAPER_WHEN_DUPLICATE", "property \"when\" is repeated on "+string(node.Kind), "remove the duplicate; the first expression is retained", duplicate.Span)
	}
	if !conditionNodeKind(node.Kind) {
		e.add("PAPER_WHEN_NODE", fmt.Sprintf("when is unsupported on %s", node.Kind), "put when on a paragraph, heading, list, item, row, or column", condition.Span)
		return true
	}
	if condition.Value.Kind != paperlang.ScalarString || condition.Value.StringValue == nil {
		e.add("PAPER_WHEN_VALUE", "when must be a quoted boolean expression", "quote a bounded expression such as active && quantity == 1", condition.Value.Span)
		return true
	}

	environment, root, problem := e.conditionEnvironment(node)
	if problem != "" {
		e.add("PAPER_WHEN_CONTEXT", problem, "use a declared primitive schema path available in this node's binding context", condition.Value.Span)
		return true
	}
	expression := strings.TrimSpace(*condition.Value.StringValue)
	program, kind, err := paperexpr.Compile(expression, environment, e.limits)
	if err != nil {
		code := "PAPER_WHEN_EXPRESSION"
		if errors.Is(err, paperexpr.ErrLimit) {
			code = "PAPER_WHEN_LIMIT"
		} else if errors.Is(err, paperexpr.ErrBinding) {
			code = "PAPER_WHEN_PATH"
		} else if errors.Is(err, paperexpr.ErrType) {
			code = "PAPER_WHEN_TYPE"
		}
		e.add(code, err.Error(), "use declared primitive paths and a boolean result", condition.Value.Span)
		return true
	}
	if kind != paperexpr.Bool {
		e.add("PAPER_WHEN_TYPE", "when expression must return bool", "compare values or use a boolean field", condition.Value.Span)
		return true
	}
	bindings, err := e.expressionBindings(node, program.Paths, root)
	if err != nil {
		e.add("PAPER_WHEN_BINDING", err.Error(), "make the selected fixture provide values matching the declared schema", condition.Value.Span)
		return true
	}
	value, err := paperexpr.Evaluate(e.ctx, program, bindings, e.limits.Program)
	if err != nil {
		code := "PAPER_WHEN_EVALUATE"
		if errors.Is(err, paperexpr.ErrLimit) {
			code = "PAPER_WHEN_LIMIT"
		} else if errors.Is(err, paperexpr.ErrBinding) {
			code = "PAPER_WHEN_BINDING"
		} else if errors.Is(err, paperexpr.ErrType) {
			code = "PAPER_WHEN_TYPE"
		}
		e.add(code, err.Error(), "make the selected fixture and expression types agree", condition.Value.Span)
		return true
	}
	if value.Kind != paperexpr.Bool {
		e.add("PAPER_WHEN_TYPE", "when expression evaluated to a non-bool value", "make the expression return true or false", condition.Value.Span)
		return true
	}
	return value.Bool
}

// takeConditionProperty removes condition syntax before normal lowering. The
// first authored property wins, matching compiler duplicate-property behavior.
func takeConditionProperty(node *paperlang.Node) (first, duplicate *paperlang.Property) {
	members := make([]paperlang.Member, 0, len(node.Members))
	for _, member := range node.Members {
		if member.Property == nil || member.Property.Name != "when" {
			members = append(members, member)
			continue
		}
		if first == nil {
			first = member.Property
		} else if duplicate == nil {
			duplicate = member.Property
		}
	}
	node.Members = members
	return first, duplicate
}

func conditionNodeKind(kind paperlang.NodeKind) bool {
	switch kind {
	case paperlang.NodeParagraph, paperlang.NodeHeading, paperlang.NodeList, paperlang.NodeItem, paperlang.NodeRow, paperlang.NodeColumn, paperlang.NodeImage, paperlang.NodeTable:
		return true
	default:
		return false
	}
}

func (e *scenarioConditionEvaluator) conditionEnvironment(node *paperlang.Node) ([]paperexpr.PathKind, paperscenario.Value, string) {
	origin := e.provenance[node]
	if origin.loopItem {
		return append([]paperexpr.PathKind(nil), origin.loopEnvironment...), origin.loopRoot, ""
	}
	if origin.repeatItem {
		if len(origin.repeatFields) != 0 && origin.repeatValue.Kind == paperscenario.Object {
			if origin.bindingBase != "" && origin.repeatItemBase != "" &&
				origin.bindingBase != origin.repeatItemBase && !strings.HasPrefix(origin.bindingBase, strings.TrimSuffix(origin.repeatItemBase, ".")+".") {
				fields, value, problem := e.bindingContext(origin.bindingBase)
				if problem != "" {
					return nil, paperscenario.Value{}, problem
				}
				return repeatExpressionEnvironment(fields, ""), value, ""
			}
			fields, value, problem := repeatConditionContext(origin)
			if problem != "" {
				return nil, paperscenario.Value{}, problem
			}
			return repeatExpressionEnvironment(fields, ""), value, ""
		}
		fields, err := repeatSchemaItem(origin.repeatSource, e.schemas)
		if err != nil {
			return nil, paperscenario.Value{}, err.Error()
		}
		items, err := repeatFixtureItems(e.fixture, origin.repeatSource)
		if err != nil {
			return nil, paperscenario.Value{}, err.Error()
		}
		for _, item := range items {
			if item.Key == origin.repeatKey {
				if item.Value.Kind != paperscenario.Object {
					return nil, paperscenario.Value{}, fmt.Sprintf("repeat item %s[%s] is not an object", origin.repeatSource, origin.repeatKey)
				}
				return repeatExpressionEnvironment(fields, ""), item.Value, ""
			}
		}
		return nil, paperscenario.Value{}, fmt.Sprintf("repeat source %s has no stable item key %q", origin.repeatSource, origin.repeatKey)
	}

	if origin.bindingBase != "" {
		fields, value, problem := e.bindingContext(origin.bindingBase)
		if problem != "" {
			return nil, paperscenario.Value{}, problem
		}
		return repeatExpressionEnvironment(fields, ""), value, ""
	}

	// Scenario fixture fields share the top-level namespace. Merge all declared
	// schemas deterministically; conflicting field contracts are diagnosed.
	kinds := make(map[string]paperexpr.Kind)
	for _, schema := range e.schemas.descriptors {
		for _, field := range schema.Fields {
			for _, path := range repeatExpressionEnvironment([]FieldDescriptor{field}, "") {
				if prior, exists := kinds[path.Path]; exists && prior != path.Kind {
					return nil, paperscenario.Value{}, fmt.Sprintf("schema path %q has conflicting primitive types", path.Path)
				}
				kinds[path.Path] = path.Kind
			}
		}
	}
	environment := make([]paperexpr.PathKind, 0, len(kinds))
	for path, kind := range kinds {
		environment = append(environment, paperexpr.PathKind{Path: path, Kind: kind})
	}
	return environment, paperscenario.Value{Kind: paperscenario.Object, Object: e.fixture.Values}, ""
}

func (e *scenarioConditionEvaluator) expressionBindings(node *paperlang.Node, paths []string, root paperscenario.Value) ([]paperexpr.Binding, error) {
	origin := e.provenance[node]
	if origin.loopItem {
		return loopExpressionBindings(paths, root, origin.loopIndex, origin.loopFirst, origin.loopLast)
	}
	return conditionBindings(paths, root)
}

func repeatConditionContext(origin expansionProvenance) ([]FieldDescriptor, paperscenario.Value, string) {
	fields := origin.repeatFields
	value := origin.repeatValue
	base := strings.TrimSuffix(origin.repeatItemBase, ".")
	binding := strings.TrimSuffix(origin.bindingBase, ".")
	if binding == "" || binding == base {
		return fields, value, ""
	}
	prefix := base + "."
	if !strings.HasPrefix(binding, prefix) {
		return nil, paperscenario.Value{}, fmt.Sprintf("condition binding %q is outside repeat item context %q", binding, base)
	}
	for _, name := range strings.Split(strings.TrimPrefix(binding, prefix), ".") {
		if strings.HasSuffix(name, "[]") {
			return nil, paperscenario.Value{}, fmt.Sprintf("condition context %q crosses a nested collection", binding)
		}
		field := findSchemaField(fields, name)
		if field == nil || field.Kind != SchemaObject {
			return nil, paperscenario.Value{}, fmt.Sprintf("condition context %q does not resolve to an object", binding)
		}
		resolved, found, problem := lookupFixtureFields(value.Object, []string{name})
		if problem != "" || !found || resolved.Kind != paperscenario.Object {
			return nil, paperscenario.Value{}, fmt.Sprintf("selected repeat item has no object for condition context %q", binding)
		}
		fields = field.Fields
		value = resolved
	}
	return fields, value, ""
}

func (e *scenarioConditionEvaluator) bindingContext(path string) ([]FieldDescriptor, paperscenario.Value, string) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || !strings.HasPrefix(parts[0], "@") {
		return nil, paperscenario.Value{}, fmt.Sprintf("binding context %q is not absolute", path)
	}
	schema := e.schemas.byName[parts[0]]
	if schema == nil {
		return nil, paperscenario.Value{}, fmt.Sprintf("binding context schema %s is not declared", parts[0])
	}
	fields := schema.Fields
	fixtureFields := e.fixture.Values
	if len(parts) == 1 {
		return fields, paperscenario.Value{Kind: paperscenario.Object, Object: fixtureFields}, ""
	}
	for _, name := range parts[1:] {
		field := findSchemaField(fields, strings.TrimSuffix(name, "[]"))
		if field == nil || field.Kind != SchemaObject {
			return nil, paperscenario.Value{}, fmt.Sprintf("binding context %q does not resolve to an object", path)
		}
		value, found, problem := lookupFixtureFields(fixtureFields, []string{strings.TrimSuffix(name, "[]")})
		if problem != "" || !found || value.Kind != paperscenario.Object {
			return nil, paperscenario.Value{}, fmt.Sprintf("selected scenario @%s has no object for binding context %q", e.fixture.Name, path)
		}
		fields = field.Fields
		fixtureFields = value.Object
	}
	return fields, paperscenario.Value{Kind: paperscenario.Object, Object: fixtureFields}, ""
}

func conditionBindings(paths []string, root paperscenario.Value) ([]paperexpr.Binding, error) {
	bindings := make([]paperexpr.Binding, 0, len(paths))
	for _, path := range paths {
		value, found, collection := resolveConditionPath(root, path)
		if !found {
			return nil, fmt.Errorf("when binding %q is missing", path)
		}
		if collection {
			return nil, fmt.Errorf("when binding %q resolves to a collection", path)
		}
		converted, err := conditionPrimitive(value)
		if err != nil {
			return nil, fmt.Errorf("when binding %q: %w", path, err)
		}
		bindings = append(bindings, paperexpr.Binding{Path: path, Value: converted})
	}
	return bindings, nil
}

func resolveConditionPath(root paperscenario.Value, path string) (paperscenario.Value, bool, bool) {
	current := root
	for _, name := range strings.Split(path, ".") {
		if current.Kind != paperscenario.Object {
			return paperscenario.Value{}, true, true
		}
		found := false
		for _, field := range current.Object {
			if field.Name == name {
				current, found = field.Value, true
				break
			}
		}
		if !found {
			return paperscenario.Value{}, false, false
		}
	}
	return current, true, current.Kind == paperscenario.Object || current.Kind == paperscenario.List
}

func conditionPrimitive(value paperscenario.Value) (paperexpr.Value, error) {
	switch value.Kind {
	case paperscenario.Null:
		return paperexpr.Value{Kind: paperexpr.Null}, nil
	case paperscenario.Bool:
		return paperexpr.Value{Kind: paperexpr.Bool, Bool: value.Bool}, nil
	case paperscenario.String:
		return paperexpr.Value{Kind: paperexpr.String, String: value.String}, nil
	case paperscenario.Number:
		integer, err := strconv.ParseInt(value.Number, 10, 64)
		if err != nil {
			return paperexpr.Value{}, errors.New("number is not a canonical int64")
		}
		return paperexpr.Value{Kind: paperexpr.Integer, Integer: integer}, nil
	default:
		return paperexpr.Value{}, fmt.Errorf("value is non-primitive %s", value.Kind)
	}
}

func (e *scenarioConditionEvaluator) add(code, message, hint string, span paperlang.Span) {
	e.diagnostics = append(e.diagnostics, paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span})
}
