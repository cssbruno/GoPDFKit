// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperscenario"
)

const (
	defaultJSONDataBytes = 8 << 20
	maxJSONDataDepth     = 64
	maxJSONDataNodes     = 100_000
	maxJSONDataString    = 1 << 20
	maxJSONDataNumber    = 4096
)

// ErrJSONData identifies a strict external-data validation failure.
var ErrJSONData = errors.New("papercompile: JSON data")

// JSONDataOptions selects the declared schema and deterministic presentation
// metadata for one external JSON render. Schema may include or omit its @.
type JSONDataOptions struct {
	Name   string
	Schema string
	Locale string
}

// JSONDataError preserves an RFC 6901 pointer to invalid external data.
type JSONDataError struct {
	Pointer string
	Problem string
}

func (e *JSONDataError) Error() string {
	pointer := "#"
	if e.Pointer != "" {
		pointer += e.Pointer
	}
	return fmt.Sprintf("%v: %s: %s", ErrJSONData, pointer, e.Problem)
}

func (e *JSONDataError) Unwrap() error { return ErrJSONData }

type jsonDataValue struct {
	kind   byte
	object map[string]*jsonDataValue
	array  []*jsonDataValue
	text   string
	bool   bool
}

type jsonDataDecoder struct {
	nodes uint32
}

// FixtureFromJSONData strictly validates one JSON object against exactly one
// declared schema and returns the normalized fixture used by rendering.
func FixtureFromJSONData(source []byte, schemas []SchemaDescriptor, options JSONDataOptions) (paperscenario.Fixture, error) {
	if len(source) == 0 {
		return paperscenario.Fixture{}, jsonDataError("", "data document is empty")
	}
	if len(source) > defaultJSONDataBytes {
		return paperscenario.Fixture{}, jsonDataError("", fmt.Sprintf("data document exceeds %d bytes", defaultJSONDataBytes))
	}
	if !utf8.Valid(source) {
		return paperscenario.Fixture{}, jsonDataError("", "data document is not valid UTF-8")
	}
	schema, err := selectJSONDataSchema(schemas, options.Schema)
	if err != nil {
		return paperscenario.Fixture{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	parser := jsonDataDecoder{}
	root, err := parser.decode(decoder, "", 0)
	if err != nil {
		return paperscenario.Fixture{}, err
	}
	if token, trailingErr := decoder.Token(); trailingErr != io.EOF {
		if trailingErr != nil {
			return paperscenario.Fixture{}, jsonDataError("", fmt.Sprintf("invalid trailing JSON: %v", trailingErr))
		}
		return paperscenario.Fixture{}, jsonDataError("", fmt.Sprintf("trailing JSON value begins with %v", token))
	}
	if root.kind != 'o' {
		return paperscenario.Fixture{}, jsonDataError("", "data root must be a JSON object")
	}
	value, err := convertJSONDataValue(root, FieldDescriptor{Kind: SchemaObject, Required: true, Fields: schema.Fields}, "")
	if err != nil {
		return paperscenario.Fixture{}, err
	}
	name := strings.TrimPrefix(strings.TrimSpace(options.Name), "@")
	if name == "" {
		name = "external-data"
	}
	resolved, err := paperscenario.Resolve([]paperscenario.Scenario{{Name: name, Locale: strings.TrimSpace(options.Locale), Values: value.Object}}, paperscenario.Limits{})
	if err != nil {
		return paperscenario.Fixture{}, jsonDataError("", err.Error())
	}
	return resolved[0], nil
}

func selectJSONDataSchema(schemas []SchemaDescriptor, requested string) (SchemaDescriptor, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && !strings.HasPrefix(requested, "@") {
		requested = "@" + requested
	}
	if requested != "" {
		for _, schema := range schemas {
			if schema.Name == requested {
				return schema, nil
			}
		}
		return SchemaDescriptor{}, jsonDataError("", fmt.Sprintf("schema %s is not declared", requested))
	}
	if len(schemas) == 0 {
		return SchemaDescriptor{}, jsonDataError("", "document declares no schema")
	}
	if len(schemas) != 1 {
		return SchemaDescriptor{}, jsonDataError("", "document declares multiple schemas; select one with --schema")
	}
	return schemas[0], nil
}

func (p *jsonDataDecoder) decode(decoder *json.Decoder, pointer string, depth uint32) (*jsonDataValue, error) {
	if depth > maxJSONDataDepth {
		return nil, jsonDataError(pointer, "JSON nesting exceeds the bounded depth")
	}
	p.nodes++
	if p.nodes > maxJSONDataNodes {
		return nil, jsonDataError(pointer, "JSON value count exceeds the bounded limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, jsonDataError(pointer, fmt.Sprintf("invalid JSON: %v", err))
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			node := &jsonDataValue{kind: 'o', object: make(map[string]*jsonDataValue)}
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				if keyErr != nil {
					return nil, jsonDataError(pointer, fmt.Sprintf("invalid object key: %v", keyErr))
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, jsonDataError(pointer, "object key is not a string")
				}
				childPointer := joinJSONPointer(pointer, key)
				if _, duplicate := node.object[key]; duplicate {
					return nil, jsonDataError(childPointer, "duplicate object key")
				}
				child, childErr := p.decode(decoder, childPointer, depth+1)
				if childErr != nil {
					return nil, childErr
				}
				node.object[key] = child
			}
			if _, endErr := decoder.Token(); endErr != nil {
				return nil, jsonDataError(pointer, fmt.Sprintf("invalid object ending: %v", endErr))
			}
			return node, nil
		case '[':
			node := &jsonDataValue{kind: 'a'}
			for index := 0; decoder.More(); index++ {
				child, childErr := p.decode(decoder, joinJSONPointer(pointer, strconv.Itoa(index)), depth+1)
				if childErr != nil {
					return nil, childErr
				}
				node.array = append(node.array, child)
			}
			if _, endErr := decoder.Token(); endErr != nil {
				return nil, jsonDataError(pointer, fmt.Sprintf("invalid array ending: %v", endErr))
			}
			return node, nil
		}
	case string:
		if len(value) > maxJSONDataString {
			return nil, jsonDataError(pointer, "string exceeds the bounded byte limit")
		}
		return &jsonDataValue{kind: 's', text: value}, nil
	case json.Number:
		canonical, numberErr := canonicalJSONDataNumber(value.String())
		if numberErr != nil {
			return nil, jsonDataError(pointer, numberErr.Error())
		}
		return &jsonDataValue{kind: 'n', text: canonical}, nil
	case bool:
		return &jsonDataValue{kind: 'b', bool: value}, nil
	case nil:
		return &jsonDataValue{kind: '0'}, nil
	}
	return nil, jsonDataError(pointer, "unsupported JSON token")
}

func convertJSONDataValue(node *jsonDataValue, descriptor FieldDescriptor, pointer string) (paperscenario.Value, error) {
	if node.kind == '0' {
		if descriptor.Required {
			return paperscenario.Value{}, jsonDataError(pointer, "required value cannot be null")
		}
		return paperscenario.Value{Kind: paperscenario.Null}, nil
	}
	switch descriptor.Kind {
	case SchemaString:
		if node.kind != 's' {
			return paperscenario.Value{}, jsonDataTypeError(pointer, "string", node.kind)
		}
		return paperscenario.Value{Kind: paperscenario.String, String: node.text}, nil
	case SchemaNumber:
		if node.kind != 'n' {
			return paperscenario.Value{}, jsonDataTypeError(pointer, "number", node.kind)
		}
		return paperscenario.Value{Kind: paperscenario.Number, Number: node.text}, nil
	case SchemaBool:
		if node.kind != 'b' {
			return paperscenario.Value{}, jsonDataTypeError(pointer, "boolean", node.kind)
		}
		return paperscenario.Value{Kind: paperscenario.Bool, Bool: node.bool}, nil
	case SchemaObject:
		if node.kind != 'o' {
			return paperscenario.Value{}, jsonDataTypeError(pointer, "object", node.kind)
		}
		known := make(map[string]bool, len(descriptor.Fields))
		fields := make([]paperscenario.Field, 0, len(descriptor.Fields))
		for _, field := range descriptor.Fields {
			known[field.Name] = true
			child, exists := node.object[field.Name]
			childPointer := joinJSONPointer(pointer, field.Name)
			if !exists {
				if field.Required {
					return paperscenario.Value{}, jsonDataError(childPointer, "required field is missing")
				}
				continue
			}
			value, err := convertJSONDataValue(child, field, childPointer)
			if err != nil {
				return paperscenario.Value{}, err
			}
			fields = append(fields, paperscenario.Field{Name: field.Name, Value: value})
		}
		unknown := make([]string, 0)
		for name := range node.object {
			if !known[name] {
				unknown = append(unknown, name)
			}
		}
		if len(unknown) != 0 {
			sort.Strings(unknown)
			return paperscenario.Value{}, jsonDataError(joinJSONPointer(pointer, unknown[0]), "field is not declared by the selected schema")
		}
		return paperscenario.Value{Kind: paperscenario.Object, Object: fields}, nil
	case SchemaList:
		if node.kind != 'a' {
			return paperscenario.Value{}, jsonDataTypeError(pointer, "array", node.kind)
		}
		if uint32(len(node.array)) > descriptor.MaxItems { // #nosec G115 -- JSON nodes are already bounded.
			return paperscenario.Value{}, jsonDataError(pointer, fmt.Sprintf("array has %d items; schema permits at most %d", len(node.array), descriptor.MaxItems))
		}
		itemDescriptor := FieldDescriptor{Kind: descriptor.ItemKind, Required: descriptor.ItemRequired, Fields: descriptor.Fields}
		items := make([]paperscenario.Item, 0, len(node.array))
		seen := make(map[string]int)
		for index, child := range node.array {
			value, err := convertJSONDataValue(child, itemDescriptor, joinJSONPointer(pointer, strconv.Itoa(index)))
			if err != nil {
				return paperscenario.Value{}, err
			}
			encoded, err := json.Marshal(value)
			if err != nil {
				return paperscenario.Value{}, jsonDataError(pointer, fmt.Sprintf("encode list item %d: %v", index, err))
			}
			digest := sha256.Sum256(encoded)
			base := "item-" + hex.EncodeToString(digest[:8])
			seen[base]++
			key := base
			if seen[base] > 1 {
				key += "-" + strconv.Itoa(seen[base])
			}
			items = append(items, paperscenario.Item{Key: key, Value: value})
		}
		return paperscenario.Value{Kind: paperscenario.List, List: items}, nil
	default:
		return paperscenario.Value{}, jsonDataError(pointer, fmt.Sprintf("unsupported schema kind %q", descriptor.Kind))
	}
}

func jsonDataTypeError(pointer, expected string, kind byte) error {
	actual := map[byte]string{'o': "object", 'a': "array", 's': "string", 'n': "number", 'b': "boolean", '0': "null"}[kind]
	if actual == "" {
		actual = "unknown"
	}
	return jsonDataError(pointer, fmt.Sprintf("expected %s, got %s", expected, actual))
}

func jsonDataError(pointer, problem string) error {
	return &JSONDataError{Pointer: pointer, Problem: problem}
}

func canonicalJSONDataNumber(raw string) (string, error) {
	if len(raw) == 0 || len(raw) > maxJSONDataNumber {
		return "", errors.New("number exceeds the bounded representation limit")
	}
	negative := false
	if raw[0] == '-' {
		negative = true
		raw = raw[1:]
	}
	exponent := 0
	if index := strings.IndexAny(raw, "eE"); index >= 0 {
		parsed, err := strconv.Atoi(raw[index+1:])
		if err != nil || parsed < -maxJSONDataNumber || parsed > maxJSONDataNumber {
			return "", errors.New("number exponent exceeds the bounded representation limit")
		}
		exponent = parsed
		raw = raw[:index]
	}
	fraction := 0
	if index := strings.IndexByte(raw, '.'); index >= 0 {
		fraction = len(raw) - index - 1
		raw = raw[:index] + raw[index+1:]
	}
	raw = strings.TrimLeft(raw, "0")
	if raw == "" {
		return "0", nil
	}
	decimal := len(raw) - fraction + exponent
	var value string
	switch {
	case decimal <= 0:
		value = "0." + strings.Repeat("0", -decimal) + raw
	case decimal >= len(raw):
		value = raw + strings.Repeat("0", decimal-len(raw))
	default:
		value = raw[:decimal] + "." + raw[decimal:]
	}
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		value = strings.TrimRight(value, "0")
		value = strings.TrimSuffix(value, ".")
	}
	if len(value) > maxJSONDataNumber {
		return "", errors.New("number expands beyond the bounded representation limit")
	}
	if negative && value != "0" {
		value = "-" + value
	}
	return value, nil
}
