// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperscenario resolves immutable, bounded fixture scenarios without
// consulting ambient time, I/O, reflection, or process state.
package paperscenario

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

type Kind string

const (
	Null   Kind = "null"
	String Kind = "string"
	Number Kind = "number"
	Bool   Kind = "bool"
	Object Kind = "object"
	List   Kind = "list"
)

// Value is a closed fixture value. Number is a canonical decimal spelling;
// callers perform locale-aware presentation later.
type Value struct {
	Kind   Kind    `json:"kind"`
	String string  `json:"string,omitempty"`
	Number string  `json:"number,omitempty"`
	Bool   bool    `json:"bool,omitempty"`
	Object []Field `json:"object,omitempty"`
	List   []Item  `json:"list,omitempty"`
}

type Field struct {
	Name  string `json:"name"`
	Value Value  `json:"value"`
}

// Item requires an authored stable key. List position is presentation order,
// never identity.
type Item struct {
	Key   string `json:"key"`
	Value Value  `json:"value"`
}

type Mutation struct {
	Path   string `json:"path"`
	Delete bool   `json:"delete,omitempty"`
	Value  Value  `json:"value"`
}

type Scenario struct {
	Name      string     `json:"name"`
	Parent    string     `json:"parent,omitempty"`
	Locale    string     `json:"locale,omitempty"`
	Values    []Field    `json:"values,omitempty"`
	Mutations []Mutation `json:"mutations,omitempty"`
}

type Fixture struct {
	Name   string  `json:"name"`
	Locale string  `json:"locale,omitempty"`
	Values []Field `json:"values,omitempty"`
	Digest string  `json:"digest"`
}

type Limits struct {
	MaxScenarios uint32
	MaxNodes     uint32
	MaxDepth     uint32
	MaxListItems uint32
	MaxPathBytes uint32
	MaxWork      uint64
}

func DefaultLimits() Limits {
	return Limits{MaxScenarios: 1024, MaxNodes: 100_000, MaxDepth: 64, MaxListItems: 100_000, MaxPathBytes: 4096, MaxWork: 1_000_000}
}

var (
	ErrInvalid = errors.New("paperscenario: invalid scenario")
	ErrLimit   = errors.New("paperscenario: limit exceeded")
)

type resolver struct {
	input  []Scenario
	byName map[string]int
	state  []uint8
	output []Fixture
	limits Limits
	nodes  uint32
	work   uint64
}

// Resolve returns fixtures in authored scenario order. Parent declarations may
// appear later. Returned values are detached from the inputs.
func Resolve(input []Scenario, limits Limits) ([]Fixture, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if !validLimits(limits) {
		return nil, fmt.Errorf("%w: invalid limits", ErrLimit)
	}
	if uint32(len(input)) > limits.MaxScenarios {
		return nil, fmt.Errorf("%w: scenario count", ErrLimit)
	}
	r := resolver{input: input, byName: make(map[string]int, len(input)), state: make([]uint8, len(input)), output: make([]Fixture, len(input)), limits: limits}
	for i, scenario := range input {
		if !validName(scenario.Name) {
			return nil, fmt.Errorf("%w: scenario[%d] has invalid name", ErrInvalid, i)
		}
		if _, exists := r.byName[scenario.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate scenario %q", ErrInvalid, scenario.Name)
		}
		r.byName[scenario.Name] = i
	}
	for i := range input {
		if err := r.resolve(i); err != nil {
			return nil, err
		}
	}
	return cloneFixtures(r.output), nil
}

func validLimits(l Limits) bool {
	h := DefaultLimits()
	return l.MaxScenarios > 0 && l.MaxScenarios <= h.MaxScenarios && l.MaxNodes > 0 && l.MaxNodes <= h.MaxNodes &&
		l.MaxDepth > 0 && l.MaxDepth <= h.MaxDepth && l.MaxListItems > 0 && l.MaxListItems <= h.MaxListItems &&
		l.MaxPathBytes > 0 && l.MaxPathBytes <= h.MaxPathBytes && l.MaxWork > 0 && l.MaxWork <= h.MaxWork
}

func (r *resolver) resolve(index int) error {
	if r.state[index] == 2 {
		return nil
	}
	if r.state[index] == 1 {
		return fmt.Errorf("%w: scenario inheritance cycle at %q", ErrInvalid, r.input[index].Name)
	}
	r.state[index] = 1
	scenario := r.input[index]
	fixture := Fixture{Name: scenario.Name, Locale: scenario.Locale}
	if scenario.Parent != "" {
		parent, exists := r.byName[scenario.Parent]
		if !exists {
			return fmt.Errorf("%w: scenario %q has missing parent %q", ErrInvalid, scenario.Name, scenario.Parent)
		}
		if err := r.resolve(parent); err != nil {
			return err
		}
		fixture.Locale = r.output[parent].Locale
		fixture.Values = cloneFields(r.output[parent].Values)
		if scenario.Locale != "" {
			fixture.Locale = scenario.Locale
		}
	}
	values, err := r.normalizeFields(scenario.Values, 1)
	if err != nil {
		return fmt.Errorf("scenario %q: %w", scenario.Name, err)
	}
	fixture.Values = overlay(fixture.Values, values)
	for mutationIndex, mutation := range scenario.Mutations {
		if err := r.mutate(&fixture.Values, mutation); err != nil {
			return fmt.Errorf("scenario %q mutation[%d]: %w", scenario.Name, mutationIndex, err)
		}
	}
	encoded, err := json.Marshal(struct {
		Name   string  `json:"name"`
		Locale string  `json:"locale,omitempty"`
		Values []Field `json:"values,omitempty"`
	}{fixture.Name, fixture.Locale, fixture.Values})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(encoded)
	fixture.Digest = hex.EncodeToString(sum[:])
	r.output[index] = fixture
	r.state[index] = 2
	return nil
}

func (r *resolver) normalizeFields(fields []Field, depth uint32) ([]Field, error) {
	result := cloneFields(fields)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	for i := range result {
		if !validName(result[i].Name) {
			return nil, fmt.Errorf("%w: invalid field name %q", ErrInvalid, result[i].Name)
		}
		if i > 0 && result[i-1].Name == result[i].Name {
			return nil, fmt.Errorf("%w: duplicate field %q", ErrInvalid, result[i].Name)
		}
		value, err := r.normalizeValue(result[i].Value, depth)
		if err != nil {
			return nil, err
		}
		result[i].Value = value
	}
	return result, nil
}

func (r *resolver) normalizeValue(value Value, depth uint32) (Value, error) {
	if depth > r.limits.MaxDepth {
		return Value{}, fmt.Errorf("%w: value depth", ErrLimit)
	}
	r.nodes++
	r.work++
	if r.nodes > r.limits.MaxNodes || r.work > r.limits.MaxWork {
		return Value{}, fmt.Errorf("%w: fixture work", ErrLimit)
	}
	switch value.Kind {
	case Null:
		if value.String != "" || value.Number != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return Value{}, fmt.Errorf("%w: null contains data", ErrInvalid)
		}
	case String:
		if value.Number != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return Value{}, fmt.Errorf("%w: string contains incompatible data", ErrInvalid)
		}
	case Number:
		if !canonicalNumber(value.Number) || value.String != "" || value.Bool || len(value.Object) != 0 || len(value.List) != 0 {
			return Value{}, fmt.Errorf("%w: number is not canonical decimal", ErrInvalid)
		}
	case Bool:
		if value.String != "" || value.Number != "" || len(value.Object) != 0 || len(value.List) != 0 {
			return Value{}, fmt.Errorf("%w: bool contains incompatible data", ErrInvalid)
		}
	case Object:
		if value.String != "" || value.Number != "" || value.Bool || len(value.List) != 0 {
			return Value{}, fmt.Errorf("%w: object contains incompatible data", ErrInvalid)
		}
		fields, err := r.normalizeFields(value.Object, depth+1)
		if err != nil {
			return Value{}, err
		}
		value.Object = fields
	case List:
		if value.String != "" || value.Number != "" || value.Bool || len(value.Object) != 0 {
			return Value{}, fmt.Errorf("%w: list contains incompatible data", ErrInvalid)
		}
		if uint32(len(value.List)) > r.limits.MaxListItems {
			return Value{}, fmt.Errorf("%w: list items", ErrLimit)
		}
		seen := make(map[string]bool, len(value.List))
		for i := range value.List {
			if !validName(value.List[i].Key) {
				return Value{}, fmt.Errorf("%w: list item %d has missing or invalid stable key", ErrInvalid, i)
			}
			if seen[value.List[i].Key] {
				return Value{}, fmt.Errorf("%w: duplicate list key %q", ErrInvalid, value.List[i].Key)
			}
			seen[value.List[i].Key] = true
			normalized, err := r.normalizeValue(value.List[i].Value, depth+1)
			if err != nil {
				return Value{}, err
			}
			value.List[i].Value = normalized
		}
	default:
		return Value{}, fmt.Errorf("%w: unsupported value kind %q", ErrInvalid, value.Kind)
	}
	return value, nil
}

func (r *resolver) mutate(fields *[]Field, mutation Mutation) error {
	if mutation.Path == "" || uint32(len(mutation.Path)) > r.limits.MaxPathBytes {
		return fmt.Errorf("%w: invalid mutation path", ErrInvalid)
	}
	parts := strings.Split(mutation.Path, ".")
	for _, part := range parts {
		if !validName(part) {
			return fmt.Errorf("%w: invalid mutation path %q", ErrInvalid, mutation.Path)
		}
	}
	r.work += uint64(len(parts))
	if r.work > r.limits.MaxWork {
		return fmt.Errorf("%w: mutation work", ErrLimit)
	}
	if !mutation.Delete {
		normalized, err := r.normalizeValue(mutation.Value, 1)
		if err != nil {
			return err
		}
		mutation.Value = normalized
	}
	current := fields
	for _, part := range parts[:len(parts)-1] {
		index := fieldIndex(*current, part)
		if index < 0 || (*current)[index].Value.Kind != Object {
			return fmt.Errorf("%w: mutation parent %q is missing or not an object", ErrInvalid, part)
		}
		current = &(*current)[index].Value.Object
	}
	name := parts[len(parts)-1]
	index := fieldIndex(*current, name)
	if mutation.Delete {
		if index < 0 {
			return fmt.Errorf("%w: cannot delete missing path %q", ErrInvalid, mutation.Path)
		}
		*current = append((*current)[:index], (*current)[index+1:]...)
		return nil
	}
	if index >= 0 {
		(*current)[index].Value = mutation.Value
	} else {
		*current = append(*current, Field{Name: name, Value: mutation.Value})
		sort.Slice(*current, func(i, j int) bool { return (*current)[i].Name < (*current)[j].Name })
	}
	return nil
}

func overlay(base, override []Field) []Field {
	result := cloneFields(base)
	for _, field := range override {
		if index := fieldIndex(result, field.Name); index >= 0 {
			result[index] = field
		} else {
			result = append(result, field)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func fieldIndex(fields []Field, name string) int {
	index := sort.Search(len(fields), func(i int) bool { return fields[i].Name >= name })
	if index < len(fields) && fields[index].Name == name {
		return index
	}
	return -1
}

func validName(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for i, r := range value {
		if !(r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func canonicalNumber(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '+' || strings.HasSuffix(value, ".") {
		return false
	}
	digits := value
	negative := false
	if digits[0] == '-' {
		negative = true
		digits = digits[1:]
	}
	parts := strings.Split(digits, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 1 && parts[0][0] == '0' {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	if len(parts) == 2 && parts[1][len(parts[1])-1] == '0' {
		return false
	}
	return !(negative && parts[0] == "0" && len(parts) == 1)
}

func cloneFields(input []Field) []Field {
	output := make([]Field, len(input))
	for i := range input {
		output[i] = Field{Name: input[i].Name, Value: cloneValue(input[i].Value)}
	}
	return output
}

func cloneValue(value Value) Value {
	value.Object = cloneFields(value.Object)
	value.List = append([]Item(nil), value.List...)
	for i := range value.List {
		value.List[i].Value = cloneValue(value.List[i].Value)
	}
	return value
}

func cloneFixtures(input []Fixture) []Fixture {
	output := append([]Fixture(nil), input...)
	for i := range output {
		output[i].Values = cloneFields(output[i].Values)
	}
	return output
}
