// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	defaultJSONSchemaBytes uint32 = 1 << 20
	maxJSONSchemaBytes     uint32 = 16 << 20
)

// ErrJSONSchemaAdapter identifies failures at the bounded JSON Schema boundary.
var ErrJSONSchemaAdapter = errors.New("papercompile: JSON Schema adapter")

// JSONSchemaPolicy bounds parsing before JSON Schema is converted to the shared
// compile-time descriptor IR. A zero policy uses safe defaults.
type JSONSchemaPolicy struct {
	Limits           SchemaLimits
	MaxDocumentBytes uint32
}

func DefaultJSONSchemaPolicy() JSONSchemaPolicy {
	return JSONSchemaPolicy{Limits: DefaultSchemaLimits(), MaxDocumentBytes: defaultJSONSchemaBytes}
}

// JSONSchemaError preserves the RFC 6901 JSON pointer for a rejected value.
// The empty pointer identifies the schema document root.
type JSONSchemaError struct {
	Pointer string
	Problem string
}

func (e *JSONSchemaError) Error() string {
	pointer := "#"
	if e.Pointer != "" {
		pointer += e.Pointer
	}
	return fmt.Sprintf("%v: %s: %s", ErrJSONSchemaAdapter, pointer, e.Problem)
}

func (e *JSONSchemaError) Unwrap() error { return ErrJSONSchemaAdapter }

// SchemaDescriptorFromJSONSchema accepts a strict, local, bounded JSON Schema
// subset and converts it into deterministic compile-time schema IR.
func SchemaDescriptorFromJSONSchema(name string, source []byte, policy JSONSchemaPolicy) (SchemaDescriptor, error) {
	if !strings.HasPrefix(name, "@") || !validBindingName(strings.TrimPrefix(name, "@")) {
		return SchemaDescriptor{}, jsonSchemaError("", "schema name must be a readable @name")
	}
	limits := policy.Limits
	if limits == (SchemaLimits{}) {
		limits = DefaultSchemaLimits()
	} else if !validSchemaLimits(limits) {
		return SchemaDescriptor{}, jsonSchemaError("", "schema limits are incomplete or exceed hard caps")
	}
	maxBytes := policy.MaxDocumentBytes
	if maxBytes == 0 {
		maxBytes = defaultJSONSchemaBytes
	}
	if maxBytes > maxJSONSchemaBytes {
		return SchemaDescriptor{}, jsonSchemaError("", fmt.Sprintf("MaxDocumentBytes exceeds the hard cap of %d", maxJSONSchemaBytes))
	}
	if len(source) == 0 {
		return SchemaDescriptor{}, jsonSchemaError("", "schema document is empty")
	}
	if uint64(len(source)) > uint64(maxBytes) {
		return SchemaDescriptor{}, jsonSchemaError("", "schema document exceeds MaxDocumentBytes")
	}
	if !utf8.Valid(source) {
		return SchemaDescriptor{}, jsonSchemaError("", "schema document is not valid UTF-8")
	}

	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	root, err := decodeJSONSchemaValue(decoder, "", 0, limits.MaxDepth*2+4)
	if err != nil {
		return SchemaDescriptor{}, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return SchemaDescriptor{}, jsonSchemaError("", fmt.Sprintf("invalid trailing JSON: %v", err))
		}
		return SchemaDescriptor{}, jsonSchemaError("", fmt.Sprintf("trailing JSON value begins with %v", token))
	}

	builder := jsonSchemaBuilder{limits: limits}
	described, err := builder.describe(root, "", "$", true, 0)
	if err != nil {
		return SchemaDescriptor{}, err
	}
	return SchemaDescriptor{
		Name:         name,
		Kind:         described.Kind,
		ItemKind:     described.ItemKind,
		ItemRequired: described.ItemRequired,
		MaxItems:     described.MaxItems,
		Fields:       described.Fields,
	}, nil
}

type jsonSchemaValue struct {
	object map[string]*jsonSchemaValue
	array  []*jsonSchemaValue
	scalar any
	kind   byte
}

func decodeJSONSchemaValue(decoder *json.Decoder, pointer string, depth, maxDepth uint32) (*jsonSchemaValue, error) {
	if depth > maxDepth {
		return nil, jsonSchemaError(pointer, "JSON nesting exceeds the bounded parser depth")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, jsonSchemaError(pointer, fmt.Sprintf("invalid JSON: %v", err))
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node := &jsonSchemaValue{kind: 'o', object: make(map[string]*jsonSchemaValue)}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return nil, jsonSchemaError(pointer, fmt.Sprintf("invalid object key: %v", err))
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, jsonSchemaError(pointer, "object key is not a string")
				}
				childPointer := joinJSONPointer(pointer, key)
				if _, duplicate := node.object[key]; duplicate {
					return nil, jsonSchemaError(childPointer, "duplicate JSON object key")
				}
				child, err := decodeJSONSchemaValue(decoder, childPointer, depth+1, maxDepth)
				if err != nil {
					return nil, err
				}
				node.object[key] = child
			}
			if _, err := decoder.Token(); err != nil {
				return nil, jsonSchemaError(pointer, fmt.Sprintf("invalid object ending: %v", err))
			}
			return node, nil
		case '[':
			node := &jsonSchemaValue{kind: 'a'}
			for index := 0; decoder.More(); index++ {
				child, err := decodeJSONSchemaValue(decoder, joinJSONPointer(pointer, strconv.Itoa(index)), depth+1, maxDepth)
				if err != nil {
					return nil, err
				}
				node.array = append(node.array, child)
			}
			if _, err := decoder.Token(); err != nil {
				return nil, jsonSchemaError(pointer, fmt.Sprintf("invalid array ending: %v", err))
			}
			return node, nil
		default:
			return nil, jsonSchemaError(pointer, "unexpected JSON delimiter")
		}
	case string:
		return &jsonSchemaValue{kind: 's', scalar: value}, nil
	case json.Number:
		return &jsonSchemaValue{kind: 'n', scalar: value}, nil
	case bool:
		return &jsonSchemaValue{kind: 'b', scalar: value}, nil
	case nil:
		return &jsonSchemaValue{kind: '0'}, nil
	default:
		return nil, jsonSchemaError(pointer, "unsupported JSON token")
	}
}

type jsonSchemaBuilder struct {
	limits SchemaLimits
	fields uint32
}

func (b *jsonSchemaBuilder) describe(node *jsonSchemaValue, pointer, paperPath string, required bool, depth uint32) (FieldDescriptor, error) {
	if node == nil || node.kind != 'o' {
		return FieldDescriptor{}, jsonSchemaError(pointer, "schema must be a JSON object")
	}
	if depth > b.limits.MaxDepth {
		return FieldDescriptor{}, jsonSchemaError(pointer, "schema nesting exceeds the configured depth")
	}
	if err := b.checkPaperPath(pointer, paperPath); err != nil {
		return FieldDescriptor{}, err
	}
	typeNode := node.object["type"]
	if typeNode == nil || typeNode.kind != 's' {
		return FieldDescriptor{}, jsonSchemaError(joinJSONPointer(pointer, "type"), "type must be a supported string")
	}
	typeName := typeNode.scalar.(string)

	switch typeName {
	case "string", "number", "integer", "boolean":
		if err := rejectJSONSchemaKeywords(node, pointer, "type"); err != nil {
			return FieldDescriptor{}, err
		}
		kind := SchemaString
		switch typeName {
		case "number", "integer":
			kind = SchemaNumber
		case "boolean":
			kind = SchemaBool
		}
		return FieldDescriptor{Kind: kind, Required: required}, nil

	case "object":
		if err := rejectJSONSchemaKeywords(node, pointer, "type", "properties", "required", "additionalProperties"); err != nil {
			return FieldDescriptor{}, err
		}
		additional := node.object["additionalProperties"]
		if additional == nil || additional.kind != 'b' || additional.scalar.(bool) {
			return FieldDescriptor{}, jsonSchemaError(joinJSONPointer(pointer, "additionalProperties"), "object requires additionalProperties: false")
		}
		properties := node.object["properties"]
		if properties == nil || properties.kind != 'o' || len(properties.object) == 0 {
			return FieldDescriptor{}, jsonSchemaError(joinJSONPointer(pointer, "properties"), "object requires a non-empty properties object")
		}
		requiredNames, err := parseJSONSchemaRequired(node.object["required"], pointer, properties.object)
		if err != nil {
			return FieldDescriptor{}, err
		}
		names := make([]string, 0, len(properties.object))
		for name := range properties.object {
			names = append(names, name)
		}
		sort.Strings(names)
		descriptor := FieldDescriptor{Kind: SchemaObject, Required: required}
		for _, name := range names {
			propertyPointer := joinJSONPointer(joinJSONPointer(pointer, "properties"), name)
			if !validBindingName(name) {
				return FieldDescriptor{}, jsonSchemaError(propertyPointer, "property name is not valid in a paper path")
			}
			if b.fields >= b.limits.MaxFields {
				return FieldDescriptor{}, jsonSchemaError(propertyPointer, "field count exceeds the configured limit")
			}
			b.fields++
			field, err := b.describe(properties.object[name], propertyPointer, joinJSONSchemaPaperPath(paperPath, name), requiredNames[name], depth+1)
			if err != nil {
				return FieldDescriptor{}, err
			}
			field.Name = name
			descriptor.Fields = append(descriptor.Fields, field)
		}
		return descriptor, nil

	case "array":
		if err := rejectJSONSchemaKeywords(node, pointer, "type", "items", "maxItems"); err != nil {
			return FieldDescriptor{}, err
		}
		maxItems, err := parseJSONSchemaMaxItems(node.object["maxItems"], joinJSONPointer(pointer, "maxItems"), b.limits.MaxListItems)
		if err != nil {
			return FieldDescriptor{}, err
		}
		items := node.object["items"]
		if items == nil {
			return FieldDescriptor{}, jsonSchemaError(joinJSONPointer(pointer, "items"), "array requires one local items schema")
		}
		item, err := b.describe(items, joinJSONPointer(pointer, "items"), paperPath+"[]", true, depth+1)
		if err != nil {
			return FieldDescriptor{}, err
		}
		if item.Kind == SchemaList {
			return FieldDescriptor{}, jsonSchemaError(joinJSONPointer(pointer, "items"), "nested arrays are not representable by the current schema IR")
		}
		return FieldDescriptor{
			Kind:         SchemaList,
			Required:     required,
			ItemKind:     item.Kind,
			ItemRequired: true,
			MaxItems:     maxItems,
			Fields:       item.Fields,
		}, nil

	default:
		return FieldDescriptor{}, jsonSchemaError(joinJSONPointer(pointer, "type"), fmt.Sprintf("unsupported type %q", typeName))
	}
}

func (b *jsonSchemaBuilder) checkPaperPath(pointer, path string) error {
	if uint64(len(path)) > uint64(b.limits.MaxPathBytes) {
		return jsonSchemaError(pointer, "paper path exceeds the configured byte limit")
	}
	if path == "$" {
		return nil
	}
	if uint64(strings.Count(path, ".")+1) > uint64(b.limits.MaxPathSegments) {
		return jsonSchemaError(pointer, "paper path exceeds the configured segment limit")
	}
	return nil
}

func rejectJSONSchemaKeywords(node *jsonSchemaValue, pointer string, allowed ...string) error {
	allow := make(map[string]bool, len(allowed))
	for _, keyword := range allowed {
		allow[keyword] = true
	}
	unknown := make([]string, 0)
	for keyword := range node.object {
		if !allow[keyword] {
			unknown = append(unknown, keyword)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return jsonSchemaError(joinJSONPointer(pointer, unknown[0]), fmt.Sprintf("unsupported JSON Schema keyword %q", unknown[0]))
}

func parseJSONSchemaRequired(node *jsonSchemaValue, pointer string, properties map[string]*jsonSchemaValue) (map[string]bool, error) {
	required := make(map[string]bool)
	if node == nil {
		return required, nil
	}
	requiredPointer := joinJSONPointer(pointer, "required")
	if node.kind != 'a' {
		return nil, jsonSchemaError(requiredPointer, "required must be an array of unique property names")
	}
	for index, value := range node.array {
		itemPointer := joinJSONPointer(requiredPointer, strconv.Itoa(index))
		if value.kind != 's' {
			return nil, jsonSchemaError(itemPointer, "required entry must be a property name string")
		}
		name := value.scalar.(string)
		if required[name] {
			return nil, jsonSchemaError(itemPointer, fmt.Sprintf("required property %q is duplicated", name))
		}
		if properties[name] == nil {
			return nil, jsonSchemaError(itemPointer, fmt.Sprintf("required property %q is not declared", name))
		}
		required[name] = true
	}
	return required, nil
}

func parseJSONSchemaMaxItems(node *jsonSchemaValue, pointer string, limit uint32) (uint32, error) {
	if node == nil || node.kind != 'n' {
		return 0, jsonSchemaError(pointer, "array requires a positive canonical integer maxItems")
	}
	raw := node.scalar.(json.Number).String()
	if raw == "" || raw[0] == '0' {
		return 0, jsonSchemaError(pointer, "maxItems must be a positive canonical integer")
	}
	for _, character := range raw {
		if character < '0' || character > '9' {
			return 0, jsonSchemaError(pointer, "maxItems must use canonical base-10 integer notation")
		}
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || parsed == 0 || parsed > uint64(limit) {
		return 0, jsonSchemaError(pointer, "maxItems exceeds the configured positive bound")
	}
	return uint32(parsed), nil
}

func joinJSONPointer(parent, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return parent + "/" + token
}

func joinJSONSchemaPaperPath(parent, name string) string {
	if parent == "$" {
		return name
	}
	return parent + "." + name
}

func jsonSchemaError(pointer, problem string) error {
	return &JSONSchemaError{Pointer: pointer, Problem: problem}
}
