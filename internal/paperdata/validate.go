// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperdata validates resolved scenario values against compile-time
// schema descriptors and performs side-effect-free primitive binding lookup.
package paperdata

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

var (
	ErrInvalid = errors.New("paperdata: fixture does not satisfy schema")
	ErrPath    = errors.New("paperdata: binding path is invalid")
	ErrLimit   = errors.New("paperdata: validation limit exceeded")
)

type Error struct {
	Path    string
	Problem string
	Cause   error
}

func (e *Error) Error() string { return fmt.Sprintf("%v at %s: %s", e.Cause, e.Path, e.Problem) }
func (e *Error) Unwrap() error { return e.Cause }

type Limits struct {
	MaxNodes     uint32
	MaxDepth     uint32
	MaxWork      uint64
	MaxPathBytes uint32
}

func DefaultLimits() Limits {
	return Limits{MaxNodes: 1_000_000, MaxDepth: 64, MaxWork: 4_000_000, MaxPathBytes: 4096}
}

type budget struct {
	limits Limits
	nodes  uint32
	work   uint64
}

// ValidateFixture validates one scenario fixture as the object root of schema.
// It rejects unknown fields, missing required fields, nullability violations,
// kind mismatches, and list bounds.
func ValidateFixture(schema papercompile.SchemaDescriptor, fixture paperscenario.Fixture, limits Limits) error {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return err
	}
	if schema.Kind != papercompile.SchemaObject {
		return dataError("$", "fixture roots require an object schema", ErrInvalid)
	}
	b := budget{limits: limits}
	root := paperscenario.Value{Kind: paperscenario.Object, Object: cloneFields(fixture.Values)}
	return validateValue(root, papercompile.FieldDescriptor{Kind: schema.Kind, Required: true, Fields: schema.Fields}, "$", 1, &b)
}

// LookupPrimitive validates the fixture first, then resolves a dotted path to
// a detached string/number/bool/null value. Collection traversal is rejected;
// bounded repeated evaluation has a separate future contract.
func LookupPrimitive(schema papercompile.SchemaDescriptor, fixture paperscenario.Fixture, path string, limits Limits) (paperscenario.Value, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return paperscenario.Value{}, err
	}
	if err := ValidateFixture(schema, fixture, limits); err != nil {
		return paperscenario.Value{}, err
	}
	canonical := strings.TrimPrefix(path, schema.Name)
	canonical = strings.TrimPrefix(canonical, "@"+strings.TrimPrefix(schema.Name, "@"))
	canonical = strings.TrimPrefix(canonical, ".")
	if canonical == "" || uint32(len(canonical)) > limits.MaxPathBytes || strings.ContainsAny(canonical, "[]") { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return paperscenario.Value{}, dataError(path, "primitive lookup requires a bounded non-collection dotted path", ErrPath)
	}
	parts := strings.Split(canonical, ".")
	fields := fixture.Values
	currentPath := "$"
	for index, part := range parts {
		if !validSegment(part) {
			return paperscenario.Value{}, dataError(path, "path contains an invalid segment", ErrPath)
		}
		fieldIndex := sort.Search(len(fields), func(i int) bool { return fields[i].Name >= part })
		if fieldIndex == len(fields) || fields[fieldIndex].Name != part {
			return paperscenario.Value{}, dataError(currentPath+"."+part, "field is missing", ErrPath)
		}
		value := fields[fieldIndex].Value
		currentPath += "." + part
		if index == len(parts)-1 {
			if value.Kind == paperscenario.Object || value.Kind == paperscenario.List {
				return paperscenario.Value{}, dataError(currentPath, "binding terminal is not primitive", ErrPath)
			}
			return cloneValue(value), nil
		}
		if value.Kind != paperscenario.Object {
			return paperscenario.Value{}, dataError(currentPath, "binding traverses a non-object", ErrPath)
		}
		fields = value.Object
	}
	return paperscenario.Value{}, dataError(path, "binding path is empty", ErrPath)
}

// LookupKeyedPrimitive resolves one stable-keyed list item and then a primitive
// dotted path within that item. It never treats presentation position as item
// identity.
func LookupKeyedPrimitive(schema papercompile.SchemaDescriptor, fixture paperscenario.Fixture, collectionPath, key, itemPath string, limits Limits) (paperscenario.Value, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return paperscenario.Value{}, err
	}
	if err := ValidateFixture(schema, fixture, limits); err != nil {
		return paperscenario.Value{}, err
	}
	collectionPath = trimSchemaPrefix(schema.Name, collectionPath)
	if collectionPath == "" || itemPath == "" || !validSegment(key) || uint64(len(collectionPath))+uint64(len(itemPath))+uint64(len(key)) > uint64(limits.MaxPathBytes) {
		return paperscenario.Value{}, dataError(collectionPath, "keyed lookup paths and stable key must be valid and bounded", ErrPath)
	}
	value, err := lookupValue(fixture.Values, strings.Split(collectionPath, "."), "$", false)
	if err != nil {
		return paperscenario.Value{}, err
	}
	if value.Kind != paperscenario.List {
		return paperscenario.Value{}, dataError("$."+collectionPath, "keyed lookup target is not a list", ErrPath)
	}
	var item *paperscenario.Value
	for index := range value.List {
		if value.List[index].Key == key {
			item = &value.List[index].Value
			break
		}
	}
	if item == nil {
		return paperscenario.Value{}, dataError("$."+collectionPath+"["+key+"]", "stable list key is missing", ErrPath)
	}
	if item.Kind != paperscenario.Object {
		return paperscenario.Value{}, dataError("$."+collectionPath+"["+key+"]", "list item is not an object", ErrPath)
	}
	return lookupValue(item.Object, strings.Split(itemPath, "."), "$."+collectionPath+"["+key+"]", true)
}

func trimSchemaPrefix(schemaName, path string) string {
	name := strings.TrimPrefix(schemaName, "@")
	path = strings.TrimPrefix(path, "@"+name)
	return strings.TrimPrefix(path, ".")
}

func lookupValue(fields []paperscenario.Field, parts []string, base string, primitive bool) (paperscenario.Value, error) {
	currentPath := base
	for index, part := range parts {
		if !validSegment(part) {
			return paperscenario.Value{}, dataError(currentPath, "path contains an invalid segment", ErrPath)
		}
		fieldIndex := sort.Search(len(fields), func(i int) bool { return fields[i].Name >= part })
		if fieldIndex == len(fields) || fields[fieldIndex].Name != part {
			return paperscenario.Value{}, dataError(currentPath+"."+part, "field is missing", ErrPath)
		}
		value := fields[fieldIndex].Value
		currentPath += "." + part
		if index == len(parts)-1 {
			if primitive && (value.Kind == paperscenario.Object || value.Kind == paperscenario.List) {
				return paperscenario.Value{}, dataError(currentPath, "binding terminal is not primitive", ErrPath)
			}
			return cloneValue(value), nil
		}
		if value.Kind != paperscenario.Object {
			return paperscenario.Value{}, dataError(currentPath, "binding traverses a non-object", ErrPath)
		}
		fields = value.Object
	}
	return paperscenario.Value{}, dataError(currentPath, "binding path is empty", ErrPath)
}

func validateValue(value paperscenario.Value, descriptor papercompile.FieldDescriptor, path string, depth uint32, b *budget) error {
	if depth > b.limits.MaxDepth {
		return dataError(path, "value nesting exceeds the configured depth", ErrLimit)
	}
	b.nodes++
	b.work++
	if b.nodes > b.limits.MaxNodes || b.work > b.limits.MaxWork {
		return dataError(path, "validation exceeds configured work", ErrLimit)
	}
	if value.Kind == paperscenario.Null {
		if descriptor.Required {
			return dataError(path, "required value is null", ErrInvalid)
		}
		return nil
	}
	want := scenarioKind(descriptor.Kind)
	if value.Kind != want {
		return dataError(path, fmt.Sprintf("expected %s, got %s", descriptor.Kind, value.Kind), ErrInvalid)
	}
	switch descriptor.Kind {
	case papercompile.SchemaObject:
		return validateObject(value.Object, descriptor.Fields, path, depth, b)
	case papercompile.SchemaList:
		if uint32(len(value.List)) > descriptor.MaxItems { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			return dataError(path, fmt.Sprintf("list has %d items, maximum is %d", len(value.List), descriptor.MaxItems), ErrInvalid)
		}
		itemDescriptor := papercompile.FieldDescriptor{Kind: descriptor.ItemKind, Required: descriptor.ItemRequired, Fields: descriptor.Fields}
		seenKeys := make(map[string]bool, len(value.List))
		for _, item := range value.List {
			if !validSegment(item.Key) || seenKeys[item.Key] {
				return dataError(path, fmt.Sprintf("list key %q is missing, invalid, or duplicated", item.Key), ErrInvalid)
			}
			seenKeys[item.Key] = true
			if err := validateValue(item.Value, itemDescriptor, path+"["+item.Key+"]", depth+1, b); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateObject(values []paperscenario.Field, descriptors []papercompile.FieldDescriptor, path string, depth uint32, b *budget) error {
	for i := 1; i < len(values); i++ {
		if values[i-1].Name >= values[i].Name {
			return dataError(path, "fixture object fields are not canonical and unique", ErrInvalid)
		}
	}
	byName := make(map[string]papercompile.FieldDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byName[descriptor.Name] = descriptor
	}
	seen := make(map[string]bool, len(values))
	for _, field := range values {
		descriptor, exists := byName[field.Name]
		if !exists {
			return dataError(path+"."+field.Name, "field is not declared by the schema", ErrInvalid)
		}
		seen[field.Name] = true
		if err := validateValue(field.Value, descriptor, path+"."+field.Name, depth+1, b); err != nil {
			return err
		}
	}
	for _, descriptor := range descriptors {
		if descriptor.Required && !seen[descriptor.Name] {
			return dataError(path+"."+descriptor.Name, "required field is missing", ErrInvalid)
		}
	}
	return nil
}

func scenarioKind(kind papercompile.SchemaKind) paperscenario.Kind {
	switch kind {
	case papercompile.SchemaString:
		return paperscenario.String
	case papercompile.SchemaNumber:
		return paperscenario.Number
	case papercompile.SchemaBool:
		return paperscenario.Bool
	case papercompile.SchemaObject:
		return paperscenario.Object
	case papercompile.SchemaList:
		return paperscenario.List
	default:
		return ""
	}
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits == (Limits{}) {
		return DefaultLimits(), nil
	}
	hard := DefaultLimits()
	if limits.MaxNodes == 0 || limits.MaxNodes > hard.MaxNodes || limits.MaxDepth == 0 || limits.MaxDepth > hard.MaxDepth ||
		limits.MaxWork == 0 || limits.MaxWork > hard.MaxWork || limits.MaxPathBytes == 0 || limits.MaxPathBytes > hard.MaxPathBytes {
		return Limits{}, dataError("$", "limits are incomplete or exceed hard caps", ErrLimit)
	}
	return limits, nil
}

func validSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for i, r := range segment {
		if r != '_' && r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func dataError(path, problem string, cause error) error {
	return &Error{Path: path, Problem: problem, Cause: cause}
}

func cloneFields(input []paperscenario.Field) []paperscenario.Field {
	result := make([]paperscenario.Field, len(input))
	for i := range input {
		result[i] = paperscenario.Field{Name: input[i].Name, Value: cloneValue(input[i].Value)}
	}
	return result
}

func cloneValue(value paperscenario.Value) paperscenario.Value {
	value.Object = cloneFields(value.Object)
	value.List = append([]paperscenario.Item(nil), value.List...)
	for i := range value.List {
		value.List[i].Value = cloneValue(value.List[i].Value)
	}
	return value
}
