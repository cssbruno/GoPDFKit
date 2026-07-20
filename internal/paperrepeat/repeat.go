// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperrepeat expands stable-keyed scenario items into deterministic
// instances. Authored list position controls presentation order only; instance
// identity and paths are derived exclusively from the stable key.
package paperrepeat

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperexpr"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

const (
	HardMaxInputItems uint32 = 100_000
	HardMaxOutput     uint32 = 100_000
	HardMaxDepth      uint32 = 128
	HardMaxPathBytes  uint32 = 4 << 10
	HardMaxStateBytes uint64 = 64 << 20
	HardMaxWork       uint64 = 10_000_000
)

var (
	ErrInvalid   = errors.New("paperrepeat: invalid repeat input")
	ErrLimit     = errors.New("paperrepeat: repeat limit exceeded")
	ErrBinding   = errors.New("paperrepeat: predicate binding error")
	ErrPredicate = errors.New("paperrepeat: predicate result error")
)

type Limits struct {
	MaxInputItems uint32
	MaxOutput     uint32
	MaxDepth      uint32
	MaxPathBytes  uint32
	MaxStateBytes uint64
	MaxWork       uint64
	Expression    paperexpr.Limits
}

func DefaultLimits() Limits {
	return Limits{MaxInputItems: 10_000, MaxOutput: 10_000, MaxDepth: 64, MaxPathBytes: 1 << 10,
		MaxStateBytes: 16 << 20, MaxWork: 1_000_000, Expression: paperexpr.DefaultLimits()}
}

func (limits Limits) validate() error {
	if limits.MaxInputItems == 0 || limits.MaxOutput == 0 || limits.MaxDepth == 0 || limits.MaxPathBytes == 0 ||
		limits.MaxStateBytes == 0 || limits.MaxWork == 0 || limits.Expression == (paperexpr.Limits{}) {
		return ErrLimit
	}
	defaults := DefaultLimits()
	if limits.MaxInputItems > HardMaxInputItems || limits.MaxOutput > HardMaxOutput || limits.MaxDepth > HardMaxDepth ||
		limits.MaxPathBytes > HardMaxPathBytes || limits.MaxStateBytes > HardMaxStateBytes || limits.MaxWork > HardMaxWork ||
		limits.Expression.MaxInstructions == 0 || limits.Expression.MaxInstructions > defaults.Expression.MaxInstructions ||
		limits.Expression.MaxConstants == 0 || limits.Expression.MaxConstants > defaults.Expression.MaxConstants ||
		limits.Expression.MaxPaths == 0 || limits.Expression.MaxPaths > defaults.Expression.MaxPaths ||
		limits.Expression.MaxStack == 0 || limits.Expression.MaxStack > defaults.Expression.MaxStack ||
		limits.Expression.MaxStringBytes == 0 || limits.Expression.MaxStringBytes > defaults.Expression.MaxStringBytes ||
		limits.Expression.MaxPatternBytes == 0 || limits.Expression.MaxPatternBytes > defaults.Expression.MaxPatternBytes ||
		limits.Expression.MaxWork == 0 || limits.Expression.MaxWork > defaults.Expression.MaxWork {
		return ErrLimit
	}
	return nil
}

type Input struct {
	Items          []paperscenario.Item
	MaxOutput      uint32
	Predicate      *paperexpr.Program
	InstancePrefix string
}

type Instance struct {
	Key      string
	Identity string
	Path     string
	Value    paperscenario.Value
}

type Expansion struct {
	instances []Instance
}

func (expansion Expansion) Instances() []Instance {
	return cloneInstances(expansion.instances)
}

func (expansion Expansion) Len() int { return len(expansion.instances) }

type ExpansionError struct {
	Path    string
	Problem string
	Cause   error
}

func (err *ExpansionError) Error() string {
	return fmt.Sprintf("%v at %s: %s", err.Cause, err.Path, err.Problem)
}

func (err *ExpansionError) Unwrap() error { return err.Cause }

func Expand(ctx context.Context, input Input, limits Limits) (Expansion, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if err := limits.validate(); err != nil {
		return Expansion{}, repeatError("limits", "limits are incomplete or exceed hard caps", ErrLimit)
	}
	if err := ctx.Err(); err != nil {
		return Expansion{}, err
	}
	if input.MaxOutput == 0 || input.MaxOutput > limits.MaxOutput {
		return Expansion{}, repeatError("max_output", "explicit maximum is zero or exceeds limits", ErrLimit)
	}
	if uint64(len(input.Items)) > uint64(limits.MaxInputItems) {
		return Expansion{}, repeatError("items", "input count exceeds limits", ErrLimit)
	}
	if !validPrefix(input.InstancePrefix, limits.MaxPathBytes) {
		return Expansion{}, repeatError("instance_prefix", "prefix is not a normalized identifier path", ErrInvalid)
	}
	var predicate *paperexpr.Program
	if input.Predicate != nil {
		copy := cloneProgram(*input.Predicate)
		predicate = &copy
	}
	state := expansionState{limits: limits, predicate: predicate, maxOutput: input.MaxOutput,
		prefix: input.InstancePrefix, seen: make(map[string]bool, len(input.Items)), output: make([]Instance, 0)}
	if err := state.charge(uint64(len(input.InstancePrefix)), "instance_prefix"); err != nil {
		return Expansion{}, err
	}
	for index, item := range input.Items {
		if err := ctx.Err(); err != nil {
			return Expansion{}, err
		}
		itemPath := fmt.Sprintf("items[%d]", index)
		if !validIdentifier(item.Key) {
			return Expansion{}, repeatError(itemPath+".key", "stable key is missing or invalid", ErrInvalid)
		}
		if state.seen[item.Key] {
			return Expansion{}, repeatError(itemPath+".key", fmt.Sprintf("duplicate stable key %q", item.Key), ErrInvalid)
		}
		state.seen[item.Key] = true
		if item.Value.Kind != paperscenario.Object {
			return Expansion{}, repeatError(itemPath+".value", "repeat item must be an object", ErrInvalid)
		}
		if err := state.validateValue(item.Value, itemPath+".value", 1); err != nil {
			return Expansion{}, err
		}
		include := true
		if predicate != nil {
			bindings, err := state.bindings(item.Value, itemPath)
			if err != nil {
				return Expansion{}, err
			}
			state.work += uint64(len(predicate.Code) + len(predicate.Paths) + 1)
			if state.work > limits.MaxWork {
				return Expansion{}, repeatError(itemPath+".predicate", "predicate work exceeds limits", ErrLimit)
			}
			result, err := paperexpr.Evaluate(ctx, *predicate, bindings, limits.Expression)
			if err != nil {
				cause := ErrPredicate
				if errors.Is(err, paperexpr.ErrBinding) {
					cause = ErrBinding
				}
				return Expansion{}, repeatError(itemPath+".predicate", err.Error(), cause)
			}
			if result.Kind != paperexpr.Bool {
				return Expansion{}, repeatError(itemPath+".predicate", "predicate result is not bool", ErrPredicate)
			}
			include = result.Bool
		}
		if !include {
			continue
		}
		if uint64(len(state.output)) >= uint64(input.MaxOutput) {
			return Expansion{}, repeatError(itemPath, "expanded output exceeds explicit maximum", ErrLimit)
		}
		identity := input.InstancePrefix + "/" + item.Key
		stablePath := input.InstancePrefix + "[" + item.Key + "]"
		if uint64(len(identity)) > uint64(limits.MaxPathBytes) || uint64(len(stablePath)) > uint64(limits.MaxPathBytes) {
			return Expansion{}, repeatError(itemPath+".key", "derived instance identity or path exceeds limits", ErrLimit)
		}
		state.work += state.valueNodes(item.Value)
		if state.work > limits.MaxWork {
			return Expansion{}, repeatError(itemPath, "detached output work exceeds limits", ErrLimit)
		}
		value := cloneScenarioValue(item.Value)
		if err := state.charge(uint64(len(item.Key)+len(identity)+len(stablePath))+state.valueBytes(item.Value), itemPath); err != nil {
			return Expansion{}, err
		}
		state.output = append(state.output, Instance{Key: item.Key, Identity: identity, Path: stablePath, Value: value})
	}
	return Expansion{instances: cloneInstances(state.output)}, nil
}

type expansionState struct {
	limits    Limits
	predicate *paperexpr.Program
	maxOutput uint32
	prefix    string
	seen      map[string]bool
	output    []Instance
	work      uint64
	bytes     uint64
}

func (state *expansionState) validateValue(value paperscenario.Value, valuePath string, depth uint32) error {
	if depth > state.limits.MaxDepth {
		return repeatError(valuePath, "value depth exceeds limits", ErrLimit)
	}
	state.work++
	if state.work > state.limits.MaxWork {
		return repeatError(valuePath, "value work exceeds limits", ErrLimit)
	}
	if err := state.charge(state.shallowValueBytes(value), valuePath); err != nil {
		return err
	}
	switch value.Kind {
	case paperscenario.Null:
		if value.String != "" || value.Number != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return repeatError(valuePath, "null contains incompatible data", ErrInvalid)
		}
	case paperscenario.String:
		if !utf8.ValidString(value.String) || value.Number != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return repeatError(valuePath, "string contains incompatible or invalid UTF-8 data", ErrInvalid)
		}
	case paperscenario.Number:
		if !validScenarioNumber(value.Number) || value.String != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return repeatError(valuePath, "number is not canonical decimal", ErrInvalid)
		}
	case paperscenario.Bool:
		if value.String != "" || value.Number != "" || len(value.Object) != 0 || len(value.List) != 0 {
			return repeatError(valuePath, "bool contains incompatible data", ErrInvalid)
		}
	case paperscenario.Object:
		if value.String != "" || value.Number != "" || value.Bool || len(value.List) != 0 {
			return repeatError(valuePath, "object contains incompatible data", ErrInvalid)
		}
		seen := make(map[string]bool, len(value.Object))
		for index, field := range value.Object {
			fieldPath := valuePath + "." + field.Name
			if err := state.charge(uint64(len(field.Name)), fieldPath); err != nil {
				return err
			}
			if !validIdentifier(field.Name) {
				return repeatError(fmt.Sprintf("%s.object[%d].name", valuePath, index), "field name is invalid", ErrInvalid)
			}
			if seen[field.Name] {
				return repeatError(fieldPath, "duplicate object field", ErrInvalid)
			}
			seen[field.Name] = true
			if err := state.validateValue(field.Value, fieldPath, depth+1); err != nil {
				return err
			}
		}
	case paperscenario.List:
		if value.String != "" || value.Number != "" || value.Bool || len(value.Object) != 0 {
			return repeatError(valuePath, "list contains incompatible data", ErrInvalid)
		}
		seen := make(map[string]bool, len(value.List))
		for index, item := range value.List {
			itemPath := fmt.Sprintf("%s.list[%d]", valuePath, index)
			if err := state.charge(uint64(len(item.Key)), itemPath+".key"); err != nil {
				return err
			}
			if !validIdentifier(item.Key) || seen[item.Key] {
				return repeatError(itemPath+".key", "nested list key is missing, invalid, or duplicate", ErrInvalid)
			}
			seen[item.Key] = true
			if err := state.validateValue(item.Value, itemPath+".value", depth+1); err != nil {
				return err
			}
		}
	default:
		return repeatError(valuePath, "unsupported scenario value kind", ErrInvalid)
	}
	return nil
}

func (state *expansionState) bindings(item paperscenario.Value, itemPath string) ([]paperexpr.Binding, error) {
	bindings := make([]paperexpr.Binding, 0, len(state.predicate.Paths))
	for _, bindingPath := range state.predicate.Paths {
		value, found, collection, work := resolveScenarioPath(item, bindingPath)
		diagnostic := itemPath + ".bindings." + bindingPath
		state.work += work
		if state.work > state.limits.MaxWork {
			return nil, repeatError(diagnostic, "binding resolution work exceeds limits", ErrLimit)
		}
		if !found {
			return nil, repeatError(diagnostic, "predicate binding is missing", ErrBinding)
		}
		if collection {
			return nil, repeatError(diagnostic, "predicate binding resolves to a collection", ErrBinding)
		}
		converted, err := scenarioPrimitive(value)
		if err != nil {
			return nil, repeatError(diagnostic, err.Error(), ErrBinding)
		}
		bindings = append(bindings, paperexpr.Binding{Path: bindingPath, Value: converted})
		if err := state.charge(uint64(len(bindingPath))+state.shallowValueBytes(value), diagnostic); err != nil {
			return nil, err
		}
	}
	return bindings, nil
}

func resolveScenarioPath(root paperscenario.Value, bindingPath string) (paperscenario.Value, bool, bool, uint64) {
	current := root
	var work uint64
	for _, component := range strings.Split(bindingPath, ".") {
		work++
		if current.Kind != paperscenario.Object {
			return paperscenario.Value{}, true, true, work
		}
		found := false
		for _, field := range current.Object {
			work++
			if field.Name == component {
				current, found = field.Value, true
				break
			}
		}
		if !found {
			return paperscenario.Value{}, false, false, work
		}
	}
	return current, true, current.Kind == paperscenario.Object || current.Kind == paperscenario.List, work
}

func scenarioPrimitive(value paperscenario.Value) (paperexpr.Value, error) {
	switch value.Kind {
	case paperscenario.Null:
		return paperexpr.Value{Kind: paperexpr.Null}, nil
	case paperscenario.Bool:
		return paperexpr.Value{Kind: paperexpr.Bool, Bool: value.Bool}, nil
	case paperscenario.String:
		return paperexpr.Value{Kind: paperexpr.String, String: string([]byte(value.String))}, nil
	case paperscenario.Number:
		integer, err := strconv.ParseInt(value.Number, 10, 64)
		if err != nil {
			return paperexpr.Value{}, errors.New("numeric binding is not a canonical int64")
		}
		return paperexpr.Value{Kind: paperexpr.Integer, Integer: integer}, nil
	default:
		return paperexpr.Value{}, errors.New("binding is not primitive")
	}
}

func (state *expansionState) charge(amount uint64, diagnosticPath string) error {
	if state.bytes > ^uint64(0)-amount {
		state.bytes = ^uint64(0)
	} else {
		state.bytes += amount
	}
	if state.bytes > state.limits.MaxStateBytes {
		return repeatError(diagnosticPath, "repeat state exceeds its byte limit", ErrLimit)
	}
	return nil
}

func (state *expansionState) shallowValueBytes(value paperscenario.Value) uint64 {
	return uint64(len(value.String) + len(value.Number) + 32)
}

func (state *expansionState) valueBytes(value paperscenario.Value) uint64 {
	total := state.shallowValueBytes(value)
	for _, field := range value.Object {
		total += uint64(len(field.Name)) + state.valueBytes(field.Value)
	}
	for _, item := range value.List {
		total += uint64(len(item.Key)) + state.valueBytes(item.Value)
	}
	return total
}

func (state *expansionState) valueNodes(value paperscenario.Value) uint64 {
	total := uint64(1)
	for _, field := range value.Object {
		total += state.valueNodes(field.Value)
	}
	for _, item := range value.List {
		total += state.valueNodes(item.Value)
	}
	return total
}

func repeatError(path, problem string, cause error) error {
	return &ExpansionError{Path: path, Problem: problem, Cause: cause}
}

func validIdentifier(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) {
		return false
	}
	for index, character := range value {
		if character != '_' && character != '-' && (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') && (index == 0 || character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validPrefix(value string, maxBytes uint32) bool {
	if value == "" || uint64(len(value)) > uint64(maxBytes) || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if !validIdentifier(component) {
			return false
		}
	}
	return true
}

func validScenarioNumber(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '+' || strings.HasSuffix(value, ".") || value == "-0" {
		return false
	}
	if value[0] == '-' {
		value = value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 1 && parts[0][0] == '0' {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return len(parts) != 2 || parts[1][len(parts[1])-1] != '0'
}

func cloneProgram(program paperexpr.Program) paperexpr.Program {
	program.Constants = append([]paperexpr.Value(nil), program.Constants...)
	program.Paths = append([]string(nil), program.Paths...)
	program.Code = append([]paperexpr.Instruction(nil), program.Code...)
	return program
}

func cloneScenarioValue(value paperscenario.Value) paperscenario.Value {
	value.String = string([]byte(value.String))
	value.Number = string([]byte(value.Number))
	value.Object = append([]paperscenario.Field(nil), value.Object...)
	for index := range value.Object {
		value.Object[index].Name = string([]byte(value.Object[index].Name))
		value.Object[index].Value = cloneScenarioValue(value.Object[index].Value)
	}
	value.List = append([]paperscenario.Item(nil), value.List...)
	for index := range value.List {
		value.List[index].Key = string([]byte(value.List[index].Key))
		value.List[index].Value = cloneScenarioValue(value.List[index].Value)
	}
	return value
}

func cloneInstances(instances []Instance) []Instance {
	output := make([]Instance, len(instances))
	for index, instance := range instances {
		output[index] = Instance{Key: string([]byte(instance.Key)), Identity: string([]byte(instance.Identity)),
			Path: string([]byte(instance.Path)), Value: cloneScenarioValue(instance.Value)}
	}
	return output
}
