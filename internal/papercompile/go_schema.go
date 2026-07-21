// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

// ErrGoSchemaAdapter identifies failures at the explicit Go-to-schema boundary.
var ErrGoSchemaAdapter = errors.New("papercompile: Go schema adapter")

// GoSchemaPolicy contains the limits and explicit maximum lengths used while
// converting Go types. Slice bounds are keyed by their final paper path: for
// example "items", "orders[].lines", or "$" for a root slice. Arrays use
// their declared length and must not have a policy entry.
type GoSchemaPolicy struct {
	ListBounds map[string]uint32
	Limits     SchemaLimits
}

// GoSchemaError reports a deterministic adapter failure without exposing
// reflection to layout or rendering code.
type GoSchemaError struct {
	Path    string
	Problem string
}

func (e *GoSchemaError) Error() string {
	if e.Path == "" {
		return fmt.Sprintf("%v: %s", ErrGoSchemaAdapter, e.Problem)
	}
	return fmt.Sprintf("%v: %s: %s", ErrGoSchemaAdapter, e.Path, e.Problem)
}

func (e *GoSchemaError) Unwrap() error { return ErrGoSchemaAdapter }

// SchemaDescriptorFromGoValue converts only the value's static type. It never
// reads fields or collection contents.
func SchemaDescriptorFromGoValue(name string, value any, policy GoSchemaPolicy) (SchemaDescriptor, error) {
	if value == nil {
		return SchemaDescriptor{}, goSchemaError("$", "nil has no static Go type")
	}
	return SchemaDescriptorFromGoType(name, reflect.TypeOf(value), policy)
}

// SchemaDescriptorFromGoType converts supported Go types into deterministic
// compile-time schema IR. The returned descriptor contains no reflect values or
// types and is safe to pass to later compilation stages.
func SchemaDescriptorFromGoType(name string, typ reflect.Type, policy GoSchemaPolicy) (SchemaDescriptor, error) {
	if typ == nil {
		return SchemaDescriptor{}, goSchemaError("$", "nil reflect.Type")
	}
	if !strings.HasPrefix(name, "@") || !validGoSchemaName(strings.TrimPrefix(name, "@")) {
		return SchemaDescriptor{}, goSchemaError("$", "schema name must be a readable @name")
	}
	limits := policy.Limits
	if limits == (SchemaLimits{}) {
		limits = DefaultSchemaLimits()
	} else if !validSchemaLimits(limits) {
		return SchemaDescriptor{}, goSchemaError("$", "schema limits are incomplete or exceed hard caps")
	}

	bounds := make(map[string]uint32, len(policy.ListBounds))
	for path, bound := range policy.ListBounds {
		bounds[path] = bound
	}
	builder := goSchemaBuilder{
		limits:     limits,
		bounds:     bounds,
		usedBounds: make(map[string]bool, len(bounds)),
		visiting:   make(map[reflect.Type]bool),
	}
	root, err := builder.describe(typ, "$", true, 0)
	if err != nil {
		return SchemaDescriptor{}, err
	}
	if unused := builder.firstUnusedBound(); unused != "" {
		return SchemaDescriptor{}, goSchemaError(unused, "list bound does not match a slice path")
	}
	return SchemaDescriptor{
		Name:         name,
		Kind:         root.Kind,
		ItemKind:     root.ItemKind,
		ItemRequired: root.ItemRequired,
		MaxItems:     root.MaxItems,
		Fields:       root.Fields,
	}, nil
}

type goSchemaBuilder struct {
	limits     SchemaLimits
	bounds     map[string]uint32
	usedBounds map[string]bool
	visiting   map[reflect.Type]bool
	fields     uint32
}

func (b *goSchemaBuilder) describe(typ reflect.Type, path string, required bool, depth uint32) (FieldDescriptor, error) {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if depth > b.limits.MaxDepth {
		return FieldDescriptor{}, goSchemaError(path, "type nesting exceeds the configured depth")
	}
	if err := b.checkPath(path); err != nil {
		return FieldDescriptor{}, err
	}
	if kind, ok := goPrimitiveKind(typ.Kind()); ok {
		return FieldDescriptor{Kind: kind, Required: required}, nil
	}

	switch typ.Kind() {
	case reflect.Struct:
		if b.visiting[typ] {
			return FieldDescriptor{}, goSchemaError(path, "recursive Go type cycle is unsupported")
		}
		b.visiting[typ] = true
		defer delete(b.visiting, typ)

		descriptor := FieldDescriptor{Kind: SchemaObject, Required: required}
		seen := make(map[string]bool)
		for index := 0; index < typ.NumField(); index++ {
			goField := typ.Field(index)
			if goField.PkgPath != "" { // Unexported fields are outside the schema boundary.
				continue
			}
			name, fieldRequired, skip, err := goFieldContract(goField)
			if err != nil {
				return FieldDescriptor{}, goSchemaError(joinGoPath(path, lowerGoFieldName(goField.Name)), err.Error())
			}
			if skip {
				continue
			}
			fieldPath := joinGoPath(path, name)
			if !validGoSchemaName(name) {
				return FieldDescriptor{}, goSchemaError(fieldPath, "field name is not valid in a paper path")
			}
			if seen[name] {
				return FieldDescriptor{}, goSchemaError(fieldPath, "field name is duplicated after paper tag mapping")
			}
			seen[name] = true
			if b.fields >= b.limits.MaxFields {
				return FieldDescriptor{}, goSchemaError(fieldPath, "field count exceeds the configured limit")
			}
			b.fields++
			field, err := b.describe(goField.Type, fieldPath, fieldRequired, depth+1)
			if err != nil {
				return FieldDescriptor{}, err
			}
			field.Name = name
			descriptor.Fields = append(descriptor.Fields, field)
		}
		if len(descriptor.Fields) == 0 {
			return FieldDescriptor{}, goSchemaError(path, "struct has no exported schema fields")
		}
		return descriptor, nil

	case reflect.Slice, reflect.Array:
		if b.visiting[typ] {
			return FieldDescriptor{}, goSchemaError(path, "recursive Go collection type is unsupported")
		}
		b.visiting[typ] = true
		defer delete(b.visiting, typ)

		var maxItems uint32
		if typ.Kind() == reflect.Slice {
			bound, exists := b.bounds[path]
			if !exists {
				return FieldDescriptor{}, goSchemaError(path, "slice requires an explicit ListBounds entry")
			}
			if bound == 0 || bound > b.limits.MaxListItems {
				return FieldDescriptor{}, goSchemaError(path, "slice bound must be positive and within MaxListItems")
			}
			maxItems = bound
			b.usedBounds[path] = true
		} else {
			if typ.Len() == 0 {
				return FieldDescriptor{}, goSchemaError(path, "zero-length arrays are unsupported")
			}
			if uint64(typ.Len()) > uint64(b.limits.MaxListItems) {
				return FieldDescriptor{}, goSchemaError(path, "array length exceeds MaxListItems")
			}
			maxItems = uint32(typ.Len())
		}

		itemRequired := typ.Elem().Kind() != reflect.Pointer
		item, err := b.describe(typ.Elem(), path+"[]", itemRequired, depth+1)
		if err != nil {
			return FieldDescriptor{}, err
		}
		if item.Kind == SchemaList {
			return FieldDescriptor{}, goSchemaError(path, "nested lists are not representable by the current schema IR")
		}
		return FieldDescriptor{
			Kind:         SchemaList,
			Required:     required,
			ItemKind:     item.Kind,
			ItemRequired: item.Required,
			MaxItems:     maxItems,
			Fields:       item.Fields,
		}, nil

	default:
		return FieldDescriptor{}, goSchemaError(path, fmt.Sprintf("Go kind %s is unsupported", typ.Kind()))
	}
}

func (b *goSchemaBuilder) checkPath(path string) error {
	if uint64(len(path)) > uint64(b.limits.MaxPathBytes) {
		return goSchemaError(path, "path exceeds the configured byte limit")
	}
	if path == "$" {
		return nil
	}
	segments := strings.Count(path, ".") + 1
	if uint64(segments) > uint64(b.limits.MaxPathSegments) {
		return goSchemaError(path, "path exceeds the configured segment limit")
	}
	return nil
}

func (b *goSchemaBuilder) firstUnusedBound() string {
	unused := make([]string, 0)
	for path := range b.bounds {
		if !b.usedBounds[path] {
			unused = append(unused, path)
		}
	}
	sort.Strings(unused)
	if len(unused) == 0 {
		return ""
	}
	return unused[0]
}

func goFieldContract(field reflect.StructField) (name string, required bool, skip bool, err error) {
	name = lowerGoFieldName(field.Name)
	required = field.Type.Kind() != reflect.Pointer && field.Type.Kind() != reflect.Slice
	raw, exists := field.Tag.Lookup("paper")
	if !exists {
		return name, required, false, nil
	}
	parts := strings.Split(raw, ",")
	if parts[0] == "-" {
		if len(parts) != 1 {
			return "", false, false, fmt.Errorf("paper skip tag cannot have options")
		}
		return "", false, true, nil
	}
	if parts[0] != "" {
		name = parts[0]
	}
	seenContract := false
	for _, option := range parts[1:] {
		switch option {
		case "required":
			if seenContract {
				return "", false, false, fmt.Errorf("paper tag repeats or conflicts on required/optional")
			}
			required = true
			seenContract = true
		case "optional":
			if seenContract {
				return "", false, false, fmt.Errorf("paper tag repeats or conflicts on required/optional")
			}
			required = false
			seenContract = true
		default:
			return "", false, false, fmt.Errorf("unsupported paper tag option %q", option)
		}
	}
	return name, required, false, nil
}

func goPrimitiveKind(kind reflect.Kind) (SchemaKind, bool) {
	switch kind {
	case reflect.String:
		return SchemaString, true
	case reflect.Bool:
		return SchemaBool, true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return SchemaNumber, true
	default:
		return "", false
	}
}

func lowerGoFieldName(name string) string {
	runes := []rune(name)
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func joinGoPath(parent, name string) string {
	if parent == "$" {
		return name
	}
	return parent + "." + name
}

func validGoSchemaName(name string) bool {
	return validBindingName(name)
}

func goSchemaError(path, problem string) error {
	return &GoSchemaError{Path: path, Problem: problem}
}
