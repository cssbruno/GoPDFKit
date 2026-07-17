// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperexpr"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperrepeat"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

// ScenarioCompileLimits bounds fixture resolution, expressions, keyed repeat
// and declarative loop output, component expansion, and schema traversal. Zero
// selects defaults; Context provides explicit cancellation.
type ScenarioCompileLimits struct {
	Components  ExpansionLimits
	Schemas     SchemaLimits
	Scenarios   paperscenario.Limits
	Repeats     paperrepeat.Limits
	Expressions paperexpr.LanguageLimits
	// Context cancels scenario expression and control-flow lowering. Nil uses
	// context.Background; it is never stored or exposed to expressions.
	Context context.Context
}

type scenarioCompileRequest struct {
	name    string
	limits  ScenarioCompileLimits
	fixture *paperscenario.Fixture
}

// CompileScenario compiles one explicitly selected source fixture. It is the
// only compile entry point that expands repeat and loop nodes.
func CompileScenario(ast paperlang.AST, scenario string) Result {
	return compilePipeline(ast, ExpansionLimits{}, SchemaLimits{}, &scenarioCompileRequest{name: scenario}, AssetCatalog{}, nil)
}

// CompileScenarioWithAssets is CompileScenario with the same explicit,
// immutable asset boundary used by CompileWithAssets.
func CompileScenarioWithAssets(ast paperlang.AST, scenario string, assets AssetCatalog) Result {
	return compilePipeline(ast, ExpansionLimits{}, SchemaLimits{}, &scenarioCompileRequest{name: scenario}, assets, nil)
}

// CompileScenarioWithResolver is CompileScenario with an explicit source
// boundary for reusable design imports.
func CompileScenarioWithResolver(ast paperlang.AST, scenario string, resolver ImportResolver) Result {
	return compilePipeline(ast, ExpansionLimits{}, SchemaLimits{}, &scenarioCompileRequest{name: scenario}, AssetCatalog{}, resolver)
}

// CompileScenarioWithAssetsAndResolver combines the explicit asset and source
// boundaries used by the scenario compiler.
func CompileScenarioWithAssetsAndResolver(ast paperlang.AST, scenario string, assets AssetCatalog, resolver ImportResolver) Result {
	return compilePipeline(ast, ExpansionLimits{}, SchemaLimits{}, &scenarioCompileRequest{name: scenario}, assets, resolver)
}

// CompileScenarioWithLimits is CompileScenario with explicit bounded policies.
func CompileScenarioWithLimits(ast paperlang.AST, scenario string, limits ScenarioCompileLimits) Result {
	return compilePipeline(ast, limits.Components, limits.Schemas, &scenarioCompileRequest{name: scenario, limits: limits}, AssetCatalog{}, nil)
}

type repeatExpansionContext struct {
	ctx          context.Context
	fixture      paperscenario.Fixture
	schemas      schemaAnalysis
	repeatLimits paperrepeat.Limits
	exprLimits   paperexpr.LanguageLimits
	nodeLimit    uint32
	provenance   map[*paperlang.Node]expansionProvenance
	diagnostics  []paperlang.Diagnostic
	nodes        uint32
	serial       uint32
	inputs       uint64
	instances    uint64
	work         uint64
	state        uint64
	limitHit     bool
}

type loopFrame struct {
	root        paperscenario.Value
	environment []paperexpr.PathKind
	index       int64
	first       bool
	last        bool
	identity    string
	path        string
	loop        *paperlang.Node
}

type repeatFrame struct {
	fields      []FieldDescriptor
	value       paperscenario.Value
	bindingBase string
	source      string
	key         string
	identity    string
	path        string
	repeat      *paperlang.Node
}

func expandSelectedScenario(ast paperlang.AST, schemas schemaAnalysis, components componentExpansionResult, request *scenarioCompileRequest) componentExpansionResult {
	result := components
	source := ExtractScenariosWithLimits(ast, request.limits.Scenarios)
	result.diagnostics = append(result.diagnostics, source.Diagnostics...)
	if !source.OK() {
		return result
	}
	fixtures, err := paperscenario.Resolve(source.Scenarios, request.limits.Scenarios)
	if err != nil {
		result.diagnostics = append(result.diagnostics, repeatDiagnostic("PAPER_REPEAT_SCENARIO", err.Error(), "fix the selected scenario fixture", rootSpan(ast)))
		return result
	}
	selected := strings.TrimPrefix(strings.TrimSpace(request.name), "@")
	if selected == "" {
		result.diagnostics = append(result.diagnostics, repeatDiagnostic("PAPER_REPEAT_SCENARIO_REQUIRED", "CompileScenario requires an explicit scenario name", "select a declared @scenario", rootSpan(ast)))
		return result
	}
	var fixture *paperscenario.Fixture
	for index := range fixtures {
		if fixtures[index].Name == selected {
			fixture = &fixtures[index]
			break
		}
	}
	if fixture == nil {
		result.diagnostics = append(result.diagnostics, repeatDiagnostic("PAPER_REPEAT_SCENARIO_UNKNOWN", fmt.Sprintf("scenario @%s is not declared", selected), "select one declared scenario", rootSpan(ast)))
		return result
	}
	request.fixture = fixture
	repeatLimits := request.limits.Repeats
	if repeatLimits == (paperrepeat.Limits{}) {
		repeatLimits = paperrepeat.DefaultLimits()
	}
	exprLimits := request.limits.Expressions
	if exprLimits == (paperexpr.LanguageLimits{}) {
		exprLimits = paperexpr.DefaultLanguageLimits()
		exprLimits.Program = repeatLimits.Expression
	}
	nodeLimit := request.limits.Components.MaxNodes
	if nodeLimit == 0 {
		nodeLimit = DefaultExpansionLimits().MaxNodes
	}
	expansionContext := repeatExpansionContext{
		ctx:     request.limits.Context,
		fixture: *fixture, schemas: schemas, repeatLimits: repeatLimits, exprLimits: exprLimits,
		nodeLimit: nodeLimit, provenance: make(map[*paperlang.Node]expansionProvenance),
	}
	if expansionContext.ctx == nil {
		expansionContext.ctx = context.Background()
	}
	cloned := expansionContext.cloneOrdinaryRoot(components.ast.Root, components.provenance)
	result.ast = paperlang.AST{File: components.ast.File, Root: cloned}
	result.provenance = expansionContext.provenance
	result.diagnostics = append(result.diagnostics, expansionContext.diagnostics...)
	conditionDiagnostics := evaluateScenarioConditions(expansionContext.ctx, result.ast, result.provenance, schemas, *fixture, exprLimits)
	result.diagnostics = append(result.diagnostics, conditionDiagnostics...)
	return result
}

func (e *repeatExpansionContext) cloneOrdinaryRoot(source *paperlang.Node, prior map[*paperlang.Node]expansionProvenance) *paperlang.Node {
	if source == nil {
		return nil
	}
	nodes := e.expandNode(source, prior)
	if len(nodes) == 0 {
		return nil
	}
	return nodes[0]
}

func (e *repeatExpansionContext) expandNode(source *paperlang.Node, prior map[*paperlang.Node]expansionProvenance) []*paperlang.Node {
	if source == nil {
		return nil
	}
	if source.Kind == paperlang.NodeRepeat {
		return e.expandRepeat(source, prior, nil, 1)
	}
	if source.Kind == paperlang.NodeLoop {
		return e.expandLoop(source, prior, nil, nil, 1)
	}
	clone := e.cloneHeader(source)
	if clone == nil {
		return nil
	}
	if provenance, exists := prior[source]; exists {
		e.provenance[clone] = provenance
	}
	for _, member := range source.Members {
		if member.Property != nil {
			clone.Members = append(clone.Members, cloneExpansionProperty(member.Property))
			continue
		}
		for _, child := range e.expandNode(member.Node, prior) {
			clone.Members = append(clone.Members, paperlang.Member{Node: child})
		}
	}
	return []*paperlang.Node{clone}
}

func (e *repeatExpansionContext) expandRepeat(node *paperlang.Node, prior map[*paperlang.Node]expansionProvenance, parent *repeatFrame, depth uint32) []*paperlang.Node {
	if e.limitHit {
		return nil
	}
	if depth > e.repeatLimits.MaxDepth {
		e.limit("nested repeat depth exceeds the configured limit", "reduce repeat nesting or raise the bounded depth limit", node.HeaderSpan)
		return nil
	}
	properties := make(map[string]*paperlang.Property)
	children := make([]*paperlang.Node, 0, 1)
	for _, member := range node.Members {
		if member.Property != nil {
			if properties[member.Property.Name] != nil {
				e.add("PAPER_REPEAT_PROPERTY_DUPLICATE", fmt.Sprintf("repeat property %q is repeated", member.Property.Name), "remove the duplicate", member.Property.Span)
				continue
			}
			properties[member.Property.Name] = member.Property
		} else if member.Node != nil {
			children = append(children, member.Node)
		}
	}
	for name, property := range properties {
		if name != "source" && name != "instance-prefix" && name != "max-items" && name != "when" {
			e.add("PAPER_REPEAT_PROPERTY", fmt.Sprintf("repeat property %q is unsupported", name), "use source, instance-prefix, max-items, and optional when", property.Span)
		}
	}
	if len(children) != 1 {
		e.add("PAPER_REPEAT_TEMPLATE", "repeat requires exactly one block, component, or nested repeat template", "add one paragraph, heading, list, row, column, text, page-break, use, or repeat child", node.HeaderSpan)
		return nil
	}
	source, sourceSpan, ok := repeatStringProperty(properties["source"])
	if !ok {
		e.add("PAPER_REPEAT_SOURCE", "repeat source must be a quoted absolute schema list path", "add source: \"@schema.items\"", propertySpan(properties["source"], node.HeaderSpan))
		return nil
	}
	prefix, _, ok := repeatStringProperty(properties["instance-prefix"])
	if !ok {
		e.add("PAPER_REPEAT_PREFIX", "repeat instance-prefix must be quoted", "add instance-prefix: \"invoice-lines\"", propertySpan(properties["instance-prefix"], node.HeaderSpan))
		return nil
	}
	maxItems, ok := repeatMaxItems(properties["max-items"])
	if !ok {
		e.add("PAPER_REPEAT_MAX_ITEMS", "repeat max-items must be a positive integer", "add max-items within repeat limits", propertySpan(properties["max-items"], node.HeaderSpan))
		return nil
	}
	canonicalSource := source
	var itemFields []FieldDescriptor
	var items []paperscenario.Item
	var err error
	if parent == nil || strings.HasPrefix(source, "@") {
		itemFields, err = repeatSchemaItem(source, e.schemas)
		if err == nil {
			items, err = repeatFixtureItems(e.fixture, source)
		}
	} else {
		canonicalSource = combineBindingPath(parent.bindingBase, source)
		itemFields, err = nestedRepeatSchemaItem(parent.fields, source)
		if err == nil {
			items, err = nestedRepeatFixtureItems(parent.value, source)
		}
	}
	if err != nil {
		code, hint := "PAPER_REPEAT_SCHEMA", "use an object-item list declared by a schema"
		if strings.Contains(err.Error(), "fixture") || strings.Contains(err.Error(), "scenario") || strings.Contains(err.Error(), "value") {
			code, hint = "PAPER_REPEAT_FIXTURE", "make the selected fixture path a keyed-list of objects"
		}
		e.add(code, err.Error(), hint, sourceSpan)
		return nil
	}
	if !e.chargeInput(uint64(len(items)), node.HeaderSpan) {
		return nil
	}

	var predicate *paperexpr.Program
	if when := properties["when"]; when != nil {
		expression, _, valid := repeatStringProperty(when)
		if !valid {
			e.add("PAPER_REPEAT_WHEN", "repeat when must be a quoted boolean expression", "quote the bounded expression", when.Value.Span)
			return nil
		}
		environment := repeatExpressionEnvironment(itemFields, "")
		program, kind, err := paperexpr.Compile(expression, environment, e.exprLimits)
		if err != nil {
			e.add("PAPER_REPEAT_WHEN", err.Error(), "use item-relative primitive paths and boolean operators", when.Value.Span)
			return nil
		}
		if kind != paperexpr.Bool {
			e.add("PAPER_REPEAT_WHEN_TYPE", "repeat when expression must return bool", "compare values or use a boolean item field", when.Value.Span)
			return nil
		}
		predicate = &program
	}
	identityPrefix := prefix
	if parent != nil {
		identityPrefix = strings.TrimSuffix(parent.identity, "/") + "/" + strings.TrimPrefix(prefix, "/")
	}
	expansion, err := paperrepeat.Expand(e.ctx, paperrepeat.Input{
		Items: items, MaxOutput: maxItems, Predicate: predicate, InstancePrefix: identityPrefix,
	}, e.repeatLimits)
	if err != nil {
		code := "PAPER_REPEAT_EXPAND"
		if errors.Is(err, paperrepeat.ErrLimit) {
			code = "PAPER_REPEAT_LIMIT"
		}
		e.add(code, err.Error(), "fix keys, predicate bindings, or repeat bounds", node.HeaderSpan)
		return nil
	}
	instances := expansion.Instances()
	work := uint64(len(items) + len(instances) + 1)
	if predicate != nil {
		work = saturatingAdd(work, saturatingMultiply(uint64(len(items)), uint64(len(predicate.Code)+len(predicate.Paths)+1)))
	}
	if !e.chargeExpansion(uint64(len(instances)), work, node.HeaderSpan) {
		return nil
	}
	output := make([]*paperlang.Node, 0, expansion.Len())
	for _, instance := range instances {
		instancePath := instance.Path
		if parent != nil {
			instancePath = strings.TrimSuffix(parent.path, "/") + "/" + strings.Trim(prefix, "/") + "[" + instance.Key + "]"
		}
		frame := &repeatFrame{
			fields: itemFields, value: instance.Value, bindingBase: canonicalSource + "[]", source: canonicalSource,
			key: instance.Key, identity: instance.Identity, path: instancePath, repeat: node,
		}
		if children[0].Kind == paperlang.NodeRepeat {
			output = append(output, e.expandRepeat(children[0], prior, frame, depth+1)...)
			continue
		}
		if children[0].Kind == paperlang.NodeLoop {
			output = append(output, e.expandLoop(children[0], prior, frame, nil, depth+1)...)
			continue
		}
		if clone := e.cloneInstance(children[0], prior, frame, depth); clone != nil {
			output = append(output, clone)
		}
	}
	return output
}

func (e *repeatExpansionContext) cloneInstance(source *paperlang.Node, prior map[*paperlang.Node]expansionProvenance, frame *repeatFrame, depth uint32) *paperlang.Node {
	if source == nil || e.limitHit {
		return nil
	}
	if source.Kind == paperlang.NodeRepeat {
		return nil
	}
	clone := e.cloneHeader(source)
	if clone == nil {
		return nil
	}
	e.serial++
	prefix := strings.ReplaceAll(frame.identity, "/", "--")
	name := strings.TrimPrefix(source.ID, "@")
	if name == "" {
		name = fmt.Sprintf("node-%d", source.HeaderSpan.Start.Offset)
	}
	clone.ID = "@" + prefix + "--" + name
	original := prior[source]
	provenance := expansionProvenance{
		definition: source.Span, invocation: frame.repeat.Span, instancePath: frame.path,
		bindingBase: frame.bindingBase, bindingSpan: propertySpan(findNodeProperty(frame.repeat, "source"), frame.repeat.HeaderSpan), bindingRequired: true, repeatItem: true,
		repeatSource: frame.source, repeatKey: frame.key, repeatItemBase: frame.bindingBase, repeatValue: frame.value, repeatFields: frame.fields,
	}
	if original.definition.File != "" {
		provenance.definition = original.definition
	}
	if original.invocation.File != "" {
		provenance.invocation = original.invocation
	}
	if original.instancePath != "" {
		provenance.instancePath += "/" + original.instancePath
	}
	if original.bindingBase != "" {
		provenance.bindingBase = combineBindingPath(frame.bindingBase, original.bindingBase)
		provenance.bindingSpan = original.bindingSpan
		provenance.bindingRequired = original.bindingRequired
		provenance.repeatItem = true
	}
	e.provenance[clone] = provenance
	for _, member := range source.Members {
		if member.Property != nil {
			clone.Members = append(clone.Members, cloneExpansionProperty(member.Property))
			continue
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeRepeat {
			for _, child := range e.expandRepeat(member.Node, prior, frame, depth+1) {
				clone.Members = append(clone.Members, paperlang.Member{Node: child})
			}
			continue
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeLoop {
			for _, child := range e.expandLoop(member.Node, prior, frame, nil, depth+1) {
				clone.Members = append(clone.Members, paperlang.Member{Node: child})
			}
			continue
		}
		if child := e.cloneInstance(member.Node, prior, frame, depth); child != nil {
			clone.Members = append(clone.Members, paperlang.Member{Node: child})
		}
	}
	return clone
}

func (e *repeatExpansionContext) chargeInput(count uint64, span paperlang.Span) bool {
	e.inputs = saturatingAdd(e.inputs, count)
	if e.inputs > uint64(e.repeatLimits.MaxInputItems) {
		e.limit("combined nested repeat input exceeds the configured limit", "reduce nested fixture items or raise the bounded input limit", span)
		return false
	}
	return true
}

func (e *repeatExpansionContext) chargeExpansion(count, work uint64, span paperlang.Span) bool {
	e.instances = saturatingAdd(e.instances, count)
	e.work = saturatingAdd(e.work, work)
	if e.instances > uint64(e.repeatLimits.MaxOutput) {
		e.limit("combined nested repeat output exceeds the configured limit", "reduce max-items or raise the bounded output limit", span)
		return false
	}
	if e.work > e.repeatLimits.MaxWork {
		e.limit("combined nested repeat work exceeds the configured limit", "reduce nested input/predicates or raise the bounded work limit", span)
		return false
	}
	return true
}

func (e *repeatExpansionContext) limit(message, hint string, span paperlang.Span) {
	if !e.limitHit {
		e.add("PAPER_REPEAT_LIMIT", message, hint, span)
	}
	e.limitHit = true
}

func saturatingAdd(left, right uint64) uint64 {
	if left > ^uint64(0)-right {
		return ^uint64(0)
	}
	return left + right
}

func saturatingMultiply(left, right uint64) uint64 {
	if left != 0 && right > ^uint64(0)/left {
		return ^uint64(0)
	}
	return left * right
}

func (e *repeatExpansionContext) cloneHeader(source *paperlang.Node) *paperlang.Node {
	if e.nodes >= e.nodeLimit {
		e.add("PAPER_REPEAT_NODE_LIMIT", "scenario expansion exceeds the component node limit", "reduce repeat output or raise the bounded limit", source.HeaderSpan)
		return nil
	}
	e.nodes++
	clone := &paperlang.Node{Kind: source.Kind, ID: source.ID, HeaderSpan: source.HeaderSpan, Span: source.Span}
	if source.Value != nil {
		value := cloneExpansionScalar(*source.Value)
		clone.Value = &value
	}
	return clone
}

func repeatSchemaItem(path string, schemas schemaAnalysis) ([]FieldDescriptor, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "@") {
		return nil, fmt.Errorf("source %q is not an absolute schema path", path)
	}
	schema := schemas.byName[parts[0]]
	if schema == nil {
		return nil, fmt.Errorf("schema %s is not declared", parts[0])
	}
	fields := schema.Fields
	var field *FieldDescriptor
	for index, name := range parts[1:] {
		if !validBindingName(name) {
			return nil, fmt.Errorf("source segment %q is invalid", name)
		}
		field = findSchemaField(fields, name)
		if field == nil {
			return nil, fmt.Errorf("source field %q is not declared", name)
		}
		if index < len(parts)-2 {
			if field.Kind != SchemaObject {
				return nil, fmt.Errorf("source traverses non-object field %q", name)
			}
			fields = field.Fields
		}
	}
	if field == nil || field.Kind != SchemaList || field.ItemKind != SchemaObject {
		return nil, fmt.Errorf("source must terminate at a list with object items")
	}
	return field.Fields, nil
}

func repeatFixtureItems(fixture paperscenario.Fixture, schemaPath string) ([]paperscenario.Item, error) {
	parts := strings.Split(schemaPath, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("source has no fixture field path")
	}
	fields := fixture.Values
	var value paperscenario.Value
	for index, name := range parts[1:] {
		found := false
		for _, field := range fields {
			if field.Name == name {
				value, found = field.Value, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("selected scenario @%s has no field %q", fixture.Name, name)
		}
		if index < len(parts)-2 {
			if value.Kind != paperscenario.Object {
				return nil, fmt.Errorf("fixture path %q is not an object", name)
			}
			fields = value.Object
		}
	}
	if value.Kind != paperscenario.List {
		return nil, fmt.Errorf("selected fixture source is not a keyed-list")
	}
	return value.List, nil
}

func nestedRepeatSchemaItem(fields []FieldDescriptor, path string) ([]FieldDescriptor, error) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || strings.HasPrefix(strings.TrimSpace(path), "@") {
		return nil, fmt.Errorf("nested repeat source %q must be item-relative", path)
	}
	var field *FieldDescriptor
	for index, name := range parts {
		if !validBindingName(name) {
			return nil, fmt.Errorf("nested repeat source segment %q is invalid", name)
		}
		field = findSchemaField(fields, name)
		if field == nil {
			return nil, fmt.Errorf("nested repeat source field %q is not declared", name)
		}
		if index < len(parts)-1 {
			if field.Kind != SchemaObject {
				return nil, fmt.Errorf("nested repeat source traverses non-object field %q", name)
			}
			fields = field.Fields
		}
	}
	if field == nil || field.Kind != SchemaList || field.ItemKind != SchemaObject {
		return nil, fmt.Errorf("nested repeat source must terminate at a list with object items")
	}
	return field.Fields, nil
}

func nestedRepeatFixtureItems(item paperscenario.Value, path string) ([]paperscenario.Item, error) {
	if item.Kind != paperscenario.Object {
		return nil, fmt.Errorf("nested repeat parent fixture value is not an object")
	}
	parts := strings.Split(strings.TrimSpace(path), ".")
	fields := item.Object
	var value paperscenario.Value
	for index, name := range parts {
		found := false
		for _, field := range fields {
			if field.Name == name {
				value, found = field.Value, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("nested fixture item has no field %q", name)
		}
		if index < len(parts)-1 {
			if value.Kind != paperscenario.Object {
				return nil, fmt.Errorf("nested fixture field %q is not an object", name)
			}
			fields = value.Object
		}
	}
	if value.Kind != paperscenario.List {
		return nil, fmt.Errorf("nested fixture source is not a keyed-list")
	}
	return value.List, nil
}

func repeatExpressionEnvironment(fields []FieldDescriptor, prefix string) []paperexpr.PathKind {
	result := make([]paperexpr.PathKind, 0)
	for _, field := range fields {
		path := field.Name
		if prefix != "" {
			path = prefix + "." + field.Name
		}
		switch field.Kind {
		case SchemaString:
			result = append(result, paperexpr.PathKind{Path: path, Kind: paperexpr.String})
		case SchemaNumber:
			result = append(result, paperexpr.PathKind{Path: path, Kind: paperexpr.Integer})
		case SchemaBool:
			result = append(result, paperexpr.PathKind{Path: path, Kind: paperexpr.Bool})
		case SchemaObject:
			result = append(result, repeatExpressionEnvironment(field.Fields, path)...)
		}
	}
	return result
}

func repeatStringProperty(property *paperlang.Property) (string, paperlang.Span, bool) {
	if property == nil || property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		return "", paperlang.Span{}, false
	}
	return strings.TrimSpace(*property.Value.StringValue), property.Value.Span, true
}

func repeatMaxItems(property *paperlang.Property) (uint32, bool) {
	if property == nil || property.Value.Kind != paperlang.ScalarNumber || property.Value.NumberValue == nil {
		return 0, false
	}
	value := *property.Value.NumberValue
	if value <= 0 || value > math.MaxUint32 || math.Trunc(value) != value {
		return 0, false
	}
	return uint32(value), true
}

func findNodeProperty(node *paperlang.Node, name string) *paperlang.Property {
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == name {
			return member.Property
		}
	}
	return nil
}

func propertySpan(property *paperlang.Property, fallback paperlang.Span) paperlang.Span {
	if property != nil {
		return property.Value.Span
	}
	return fallback
}

func (e *repeatExpansionContext) add(code, message, hint string, span paperlang.Span) {
	e.diagnostics = append(e.diagnostics, repeatDiagnostic(code, message, hint, span))
}

func repeatDiagnostic(code, message, hint string, span paperlang.Span) paperlang.Diagnostic {
	return paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span}
}
