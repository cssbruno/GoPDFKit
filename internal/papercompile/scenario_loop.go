// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperexpr"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

// expandLoop lowers a closed integer range. It deliberately supports no loop
// body mutation, callbacks, dynamic modules, or ambient values: the only
// changing inputs are the three immutable loop.* bindings below.
func (e *repeatExpansionContext) expandLoop(node *paperlang.Node, prior map[*paperlang.Node]expansionProvenance, repeatParent *repeatFrame, loopParent *loopFrame, depth uint32) []*paperlang.Node {
	if e.limitHit {
		return nil
	}
	if depth > e.repeatLimits.MaxDepth {
		e.loopLimit("nested loop depth exceeds the configured limit", "reduce loop nesting or raise the bounded depth limit", node.HeaderSpan)
		return nil
	}
	properties := make(map[string]*paperlang.Property)
	children := make([]*paperlang.Node, 0, 1)
	for _, member := range node.Members {
		if member.Property != nil {
			if properties[member.Property.Name] != nil {
				e.add("PAPER_LOOP_PROPERTY_DUPLICATE", fmt.Sprintf("loop property %q is repeated", member.Property.Name), "remove the duplicate", member.Property.Span)
				continue
			}
			properties[member.Property.Name] = member.Property
		} else if member.Node != nil {
			children = append(children, member.Node)
		}
	}
	for name, property := range properties {
		if name != "from" && name != "through" && name != "step" && name != "max-iterations" && name != "instance-prefix" && name != "when" {
			e.add("PAPER_LOOP_PROPERTY", fmt.Sprintf("loop property %q is unsupported", name), "use from, through, step, max-iterations, instance-prefix, and optional when", property.Span)
		}
	}
	if len(children) != 1 {
		e.add("PAPER_LOOP_TEMPLATE", "loop requires exactly one block, component, or nested loop template", "add one visual block, component use, or loop child", node.HeaderSpan)
		return nil
	}
	from, ok := loopIntegerProperty(properties["from"])
	if !ok {
		e.add("PAPER_LOOP_FROM", "loop from must be an integer", "add from: 0", propertySpan(properties["from"], node.HeaderSpan))
		return nil
	}
	through, ok := loopIntegerProperty(properties["through"])
	if !ok {
		e.add("PAPER_LOOP_THROUGH", "loop through must be an integer", "add through: 10", propertySpan(properties["through"], node.HeaderSpan))
		return nil
	}
	step, ok := loopIntegerProperty(properties["step"])
	if !ok || step == 0 {
		e.add("PAPER_LOOP_STEP", "loop step must be a non-zero integer", "add step: 1 or step: -1", propertySpan(properties["step"], node.HeaderSpan))
		return nil
	}
	if from < through && step < 0 || from > through && step > 0 {
		e.add("PAPER_LOOP_DIRECTION", "loop step moves away from through", "make step positive for ascending ranges and negative for descending ranges", propertySpan(properties["step"], node.HeaderSpan))
		return nil
	}
	maxIterations, ok := repeatMaxItems(properties["max-iterations"])
	if !ok || maxIterations > e.repeatLimits.MaxInputItems {
		e.add("PAPER_LOOP_MAX_ITERATIONS", "loop max-iterations must be a positive integer within configured input limits", "add an explicit bounded max-iterations", propertySpan(properties["max-iterations"], node.HeaderSpan))
		return nil
	}
	prefix, _, ok := repeatStringProperty(properties["instance-prefix"])
	if !ok || !validLoopPrefix(prefix, e.repeatLimits.MaxPathBytes) {
		e.add("PAPER_LOOP_PREFIX", "loop instance-prefix must be a bounded readable path", "add instance-prefix: \"copies\"", propertySpan(properties["instance-prefix"], node.HeaderSpan))
		return nil
	}

	environment, root, problem := e.loopEnvironment(repeatParent, loopParent)
	if problem != "" {
		e.add("PAPER_LOOP_CONTEXT", problem, "use a typed selected-scenario context", node.HeaderSpan)
		return nil
	}
	var predicate *paperexpr.Program
	predicateSpan := node.HeaderSpan
	if when := properties["when"]; when != nil {
		predicateSpan = when.Value.Span
		expression, _, valid := repeatStringProperty(when)
		if !valid {
			e.add("PAPER_LOOP_WHEN", "loop when must be a quoted boolean expression", "use fixture paths and loop.index, loop.first, or loop.last", when.Value.Span)
			return nil
		}
		program, kind, err := paperexpr.Compile(expression, environment, e.exprLimits)
		if err != nil {
			code := "PAPER_LOOP_WHEN"
			if errors.Is(err, paperexpr.ErrLimit) {
				code = "PAPER_LOOP_LIMIT"
			}
			e.add(code, err.Error(), "use typed fixture paths and loop bindings", when.Value.Span)
			return nil
		}
		if kind != paperexpr.Bool {
			e.add("PAPER_LOOP_WHEN_TYPE", "loop when expression must return bool", "compare values or use a boolean binding", when.Value.Span)
			return nil
		}
		predicate = &program
	}

	identityPrefix, pathPrefix := prefix, prefix
	if repeatParent != nil {
		identityPrefix = strings.TrimSuffix(repeatParent.identity, "/") + "/" + prefix
		pathPrefix = strings.TrimSuffix(repeatParent.path, "/") + "/" + prefix
	} else if loopParent != nil {
		identityPrefix = strings.TrimSuffix(loopParent.identity, "/") + "/" + prefix
		pathPrefix = strings.TrimSuffix(loopParent.path, "/") + "/" + prefix
	}
	output := make([]*paperlang.Node, 0)
	iteration := uint32(0)
	for value := from; ; {
		if err := e.ctx.Err(); err != nil {
			e.add("PAPER_LOOP_CANCELLED", err.Error(), "resume compilation with a live context", node.HeaderSpan)
			return nil
		}
		iteration++
		if iteration > maxIterations {
			e.loopLimit("loop range exceeds explicit max-iterations", "raise the bounded max-iterations or shorten the range", node.HeaderSpan)
			return nil
		}
		if !e.chargeLoopInput(1, node.HeaderSpan) {
			return nil
		}
		next, overflow := loopNext(value, step)
		last := value == through || overflow || step > 0 && next > through || step < 0 && next < through
		include := true
		if predicate != nil {
			bindings, err := loopExpressionBindings(predicate.Paths, root, value, iteration == 1, last)
			if err != nil {
				e.add("PAPER_LOOP_BINDING", err.Error(), "make the selected fixture match its schema", predicateSpan)
				return nil
			}
			result, err := paperexpr.Evaluate(e.ctx, *predicate, bindings, e.exprLimits.Program)
			if err != nil {
				code := "PAPER_LOOP_EVALUATE"
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					code = "PAPER_LOOP_CANCELLED"
				} else if errors.Is(err, paperexpr.ErrLimit) {
					code = "PAPER_LOOP_LIMIT"
				}
				e.add(code, err.Error(), "fix the expression or its explicit bounds", predicateSpan)
				return nil
			}
			include = result.Kind == paperexpr.Bool && result.Bool
		}
		work := uint64(1)
		if predicate != nil {
			work = saturatingAdd(work, uint64(len(predicate.Code)+len(predicate.Paths)+1))
		}
		if include {
			if !e.chargeLoopExpansion(1, work, node.HeaderSpan) {
				return nil
			}
			key := strconv.FormatInt(value, 10)
			identity := identityPrefix + "[" + key + "]"
			path := pathPrefix + "[" + key + "]"
			if uint32(len(identity)) > e.repeatLimits.MaxPathBytes || uint32(len(path)) > e.repeatLimits.MaxPathBytes { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				e.loopLimit("nested loop instance path exceeds the configured byte limit", "shorten instance prefixes or nesting", node.HeaderSpan)
				return nil
			}
			if !e.chargeLoopState(uint64(len(identity)+len(path)+64), node.HeaderSpan) {
				return nil
			}
			frame := &loopFrame{root: root, environment: environment, index: value, first: iteration == 1, last: last,
				identity: identity, path: path, loop: node}
			if children[0].Kind == paperlang.NodeLoop {
				output = append(output, e.expandLoop(children[0], prior, repeatParent, frame, depth+1)...)
			} else if clone := e.cloneLoopInstance(children[0], prior, repeatParent, frame, depth); clone != nil {
				output = append(output, clone)
			}
		} else if !e.chargeLoopExpansion(0, work, node.HeaderSpan) {
			return nil
		}
		if last {
			break
		}
		value = next
	}
	return output
}

func (e *repeatExpansionContext) cloneLoopInstance(source *paperlang.Node, prior map[*paperlang.Node]expansionProvenance, repeatParent *repeatFrame, frame *loopFrame, depth uint32) *paperlang.Node {
	if source == nil || e.limitHit || source.Kind == paperlang.NodeLoop || source.Kind == paperlang.NodeRepeat {
		return nil
	}
	clone := e.cloneHeader(source)
	if clone == nil {
		return nil
	}
	name := strings.TrimPrefix(source.ID, "@")
	if name == "" {
		name = fmt.Sprintf("node-%d", source.HeaderSpan.Start.Offset)
	}
	clone.ID = "@" + strings.ReplaceAll(frame.identity, "/", "--") + "--" + name
	original := prior[source]
	provenance := expansionProvenance{definition: source.Span, invocation: frame.loop.Span, instancePath: frame.path,
		loopItem: true, loopIndex: frame.index, loopFirst: frame.first, loopLast: frame.last,
		loopRoot: frame.root, loopEnvironment: append([]paperexpr.PathKind(nil), frame.environment...)}
	if repeatParent != nil {
		provenance.bindingBase = repeatParent.bindingBase
		provenance.bindingSpan = propertySpan(findNodeProperty(repeatParent.repeat, "source"), repeatParent.repeat.HeaderSpan)
		provenance.bindingRequired = true
		provenance.repeatItem = true
		provenance.repeatSource = repeatParent.source
		provenance.repeatKey = repeatParent.key
		provenance.repeatItemBase = repeatParent.bindingBase
		provenance.repeatValue = repeatParent.value
		provenance.repeatFields = repeatParent.fields
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
		provenance.bindingBase = original.bindingBase
		if repeatParent != nil {
			provenance.bindingBase = combineBindingPath(repeatParent.bindingBase, original.bindingBase)
		}
		provenance.bindingSpan = original.bindingSpan
		provenance.bindingRequired = original.bindingRequired
	}
	e.provenance[clone] = provenance
	for _, member := range source.Members {
		if member.Property != nil {
			clone.Members = append(clone.Members, cloneExpansionProperty(member.Property))
			continue
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeLoop {
			for _, child := range e.expandLoop(member.Node, prior, repeatParent, frame, depth+1) {
				clone.Members = append(clone.Members, paperlang.Member{Node: child})
			}
			continue
		}
		if child := e.cloneLoopInstance(member.Node, prior, repeatParent, frame, depth); child != nil {
			clone.Members = append(clone.Members, paperlang.Member{Node: child})
		}
	}
	return clone
}

func (e *repeatExpansionContext) loopEnvironment(repeatParent *repeatFrame, loopParent *loopFrame) ([]paperexpr.PathKind, paperscenario.Value, string) {
	var environment []paperexpr.PathKind
	var root paperscenario.Value
	if repeatParent != nil {
		environment = repeatExpressionEnvironment(repeatParent.fields, "")
		root = repeatParent.value
	} else if loopParent != nil {
		root = loopParent.root
		for _, entry := range loopParent.environment {
			if !strings.HasPrefix(entry.Path, "loop.") {
				environment = append(environment, entry)
			}
		}
	} else {
		root = paperscenario.Value{Kind: paperscenario.Object, Object: e.fixture.Values}
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
		for path, kind := range kinds {
			environment = append(environment, paperexpr.PathKind{Path: path, Kind: kind})
		}
	}
	environment = append(environment,
		paperexpr.PathKind{Path: "loop.first", Kind: paperexpr.Bool},
		paperexpr.PathKind{Path: "loop.index", Kind: paperexpr.Integer},
		paperexpr.PathKind{Path: "loop.last", Kind: paperexpr.Bool})
	sort.Slice(environment, func(i, j int) bool { return environment[i].Path < environment[j].Path })
	return environment, root, ""
}

func loopExpressionBindings(paths []string, root paperscenario.Value, index int64, first, last bool) ([]paperexpr.Binding, error) {
	fixturePaths := make([]string, 0, len(paths))
	bindings := make([]paperexpr.Binding, 0, len(paths))
	for _, path := range paths {
		switch path {
		case "loop.index":
			bindings = append(bindings, paperexpr.Binding{Path: path, Value: paperexpr.Value{Kind: paperexpr.Integer, Integer: index}})
		case "loop.first":
			bindings = append(bindings, paperexpr.Binding{Path: path, Value: paperexpr.Value{Kind: paperexpr.Bool, Bool: first}})
		case "loop.last":
			bindings = append(bindings, paperexpr.Binding{Path: path, Value: paperexpr.Value{Kind: paperexpr.Bool, Bool: last}})
		default:
			fixturePaths = append(fixturePaths, path)
		}
	}
	fixture, err := conditionBindings(fixturePaths, root)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, fixture...)
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Path < bindings[j].Path })
	return bindings, nil
}

func loopIntegerProperty(property *paperlang.Property) (int64, bool) {
	if property == nil || property.Value.Kind != paperlang.ScalarNumber || property.Value.NumberValue == nil {
		return 0, false
	}
	value := *property.Value.NumberValue
	if math.Trunc(value) != value || value < math.MinInt64 || value > math.MaxInt64 {
		return 0, false
	}
	return int64(value), true
}

func loopNext(value, step int64) (int64, bool) {
	if step > 0 && value > math.MaxInt64-step || step < 0 && value < math.MinInt64-step {
		return 0, true
	}
	return value + step, false
}

func validLoopPrefix(prefix string, maxBytes uint32) bool {
	if prefix == "" || uint32(len(prefix)) > maxBytes { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return false
	}
	for index, character := range prefix {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character == '_' || character == '-' || index > 0 && character >= '0' && character <= '9' {
			continue
		}
		return false
	}
	return true
}

func (e *repeatExpansionContext) loopLimit(message, hint string, span paperlang.Span) {
	if !e.limitHit {
		e.add("PAPER_LOOP_LIMIT", message, hint, span)
	}
	e.limitHit = true
}

func (e *repeatExpansionContext) chargeLoopInput(count uint64, span paperlang.Span) bool {
	e.inputs = saturatingAdd(e.inputs, count)
	if e.inputs > uint64(e.repeatLimits.MaxInputItems) {
		e.loopLimit("combined loop input exceeds the configured limit", "reduce loop ranges or raise the bounded input limit", span)
		return false
	}
	return true
}

func (e *repeatExpansionContext) chargeLoopExpansion(count, work uint64, span paperlang.Span) bool {
	e.instances = saturatingAdd(e.instances, count)
	e.work = saturatingAdd(e.work, work)
	if e.instances > uint64(e.repeatLimits.MaxOutput) {
		e.loopLimit("combined control-flow output exceeds the configured limit", "reduce loop output or raise the bounded output limit", span)
		return false
	}
	if e.work > e.repeatLimits.MaxWork {
		e.loopLimit("combined loop work exceeds the configured limit", "reduce ranges/conditions or raise the bounded work limit", span)
		return false
	}
	return true
}

func (e *repeatExpansionContext) chargeLoopState(bytes uint64, span paperlang.Span) bool {
	e.state = saturatingAdd(e.state, bytes)
	if e.state > e.repeatLimits.MaxStateBytes {
		e.loopLimit("combined loop identity state exceeds the configured byte limit", "reduce output/path sizes or raise the bounded state limit", span)
		return false
	}
	return true
}
