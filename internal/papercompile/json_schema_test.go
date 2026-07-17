// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaDescriptorFromJSONSchema(t *testing.T) {
	t.Parallel()

	source := []byte(`{
  "required": ["customerID", "items"],
  "properties": {
    "score": {"type": "integer"},
    "items": {
      "maxItems": 50,
      "items": {
        "required": ["price"],
        "additionalProperties": false,
        "properties": {
          "note": {"type": "string"},
          "price": {"type": "number"}
        },
        "type": "object"
      },
      "type": "array"
    },
    "customerID": {"type": "string"},
    "active": {"type": "boolean"}
  },
  "additionalProperties": false,
  "type": "object"
}`)
	descriptor, err := SchemaDescriptorFromJSONSchema("@invoice", source, JSONSchemaPolicy{})
	if err != nil {
		t.Fatalf("convert JSON Schema: %v", err)
	}
	if descriptor.Name != "@invoice" || descriptor.Kind != SchemaObject || len(descriptor.Fields) != 4 {
		t.Fatalf("unexpected root descriptor: %#v", descriptor)
	}
	wantNames := []string{"active", "customerID", "items", "score"}
	for index, want := range wantNames {
		if descriptor.Fields[index].Name != want {
			t.Fatalf("property order = %#v, want %#v", descriptor.Fields, wantNames)
		}
	}
	if descriptor.Fields[0].Required || !descriptor.Fields[1].Required || descriptor.Fields[3].Kind != SchemaNumber {
		t.Fatalf("unexpected primitive contracts: %#v", descriptor.Fields)
	}
	items := descriptor.Fields[2]
	if items.Kind != SchemaList || !items.Required || items.ItemKind != SchemaObject || !items.ItemRequired || items.MaxItems != 50 || len(items.Fields) != 2 {
		t.Fatalf("unexpected array contract: %#v", items)
	}
	if items.Fields[0].Name != "note" || items.Fields[0].Required || items.Fields[1].Name != "price" || !items.Fields[1].Required {
		t.Fatalf("unexpected item fields: %#v", items.Fields)
	}

	reordered := []byte(`{"type":"object","properties":{"customerID":{"type":"string"},"active":{"type":"boolean"},"score":{"type":"integer"},"items":{"type":"array","items":{"type":"object","properties":{"price":{"type":"number"},"note":{"type":"string"}},"additionalProperties":false,"required":["price"]},"maxItems":50}},"required":["items","customerID"],"additionalProperties":false}`)
	again, err := SchemaDescriptorFromJSONSchema("@invoice", reordered, JSONSchemaPolicy{})
	if err != nil {
		t.Fatalf("convert reordered JSON Schema: %v", err)
	}
	if !reflect.DeepEqual(descriptor, again) {
		t.Fatalf("conversion is not deterministic:\n%#v\n%#v", descriptor, again)
	}
}

func TestSchemaDescriptorFromJSONSchemaRootKinds(t *testing.T) {
	t.Parallel()

	primitive, err := SchemaDescriptorFromJSONSchema("@count", []byte(`{"type":"integer"}`), JSONSchemaPolicy{})
	if err != nil || primitive.Kind != SchemaNumber {
		t.Fatalf("root integer: %#v, %v", primitive, err)
	}
	list, err := SchemaDescriptorFromJSONSchema("@names", []byte(`{"maxItems":12,"items":{"type":"string"},"type":"array"}`), JSONSchemaPolicy{})
	if err != nil || list.Kind != SchemaList || list.ItemKind != SchemaString || !list.ItemRequired || list.MaxItems != 12 {
		t.Fatalf("root array: %#v, %v", list, err)
	}
}

func TestSchemaDescriptorFromJSONSchemaRejectsInvalidSubset(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  string
		problem string
		pointer string
	}{
		{name: "duplicate keyword", source: `{"type":"string","type":"number"}`, problem: "duplicate JSON", pointer: "/type"},
		{name: "duplicate property", source: `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"},"name":{"type":"string"}}}`, problem: "duplicate JSON", pointer: "/properties/name"},
		{name: "trailing data", source: `{"type":"string"} {"type":"string"}`, problem: "trailing JSON", pointer: ""},
		{name: "ref", source: `{"type":"string","$ref":"https://example.test/schema"}`, problem: "$ref", pointer: "/$ref"},
		{name: "union", source: `{"type":"string","anyOf":[]}`, problem: "anyOf", pointer: "/anyOf"},
		{name: "type union", source: `{"type":["string","null"]}`, problem: "supported string", pointer: "/type"},
		{name: "unknown type", source: `{"type":"null"}`, problem: "unsupported type", pointer: "/type"},
		{name: "open object", source: `{"type":"object","properties":{"name":{"type":"string"}}}`, problem: "additionalProperties", pointer: "/additionalProperties"},
		{name: "true additional", source: `{"type":"object","additionalProperties":true,"properties":{"name":{"type":"string"}}}`, problem: "additionalProperties", pointer: "/additionalProperties"},
		{name: "empty object", source: `{"type":"object","additionalProperties":false,"properties":{}}`, problem: "non-empty", pointer: "/properties"},
		{name: "required type", source: `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}},"required":"name"}`, problem: "required must", pointer: "/required"},
		{name: "required duplicate", source: `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}},"required":["name","name"]}`, problem: "duplicated", pointer: "/required/1"},
		{name: "required missing", source: `{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}},"required":["other"]}`, problem: "not declared", pointer: "/required/0"},
		{name: "invalid property", source: `{"type":"object","additionalProperties":false,"properties":{"bad name":{"type":"string"}}}`, problem: "paper path", pointer: "/properties/bad name"},
		{name: "escaped pointer", source: `{"type":"object","additionalProperties":false,"properties":{"bad/name":{"type":"string"}}}`, problem: "paper path", pointer: "/properties/bad~1name"},
		{name: "unbounded array", source: `{"type":"array","items":{"type":"string"}}`, problem: "maxItems", pointer: "/maxItems"},
		{name: "missing items", source: `{"type":"array","maxItems":2}`, problem: "items", pointer: "/items"},
		{name: "zero bound", source: `{"type":"array","maxItems":0,"items":{"type":"string"}}`, problem: "positive", pointer: "/maxItems"},
		{name: "decimal bound", source: `{"type":"array","maxItems":2.0,"items":{"type":"string"}}`, problem: "canonical", pointer: "/maxItems"},
		{name: "exponent bound", source: `{"type":"array","maxItems":2e1,"items":{"type":"string"}}`, problem: "canonical", pointer: "/maxItems"},
		{name: "nested array", source: `{"type":"array","maxItems":2,"items":{"type":"array","maxItems":2,"items":{"type":"string"}}}`, problem: "nested arrays", pointer: "/items"},
		{name: "schema boolean", source: `true`, problem: "JSON object", pointer: ""},
		{name: "malformed", source: `{"type":`, problem: "invalid JSON", pointer: "/type"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := SchemaDescriptorFromJSONSchema("@test", []byte(test.source), JSONSchemaPolicy{})
			assertJSONSchemaError(t, err, test.pointer, test.problem)
		})
	}
}

func TestSchemaDescriptorFromJSONSchemaEnforcesLimits(t *testing.T) {
	t.Parallel()

	object := []byte(`{"type":"object","additionalProperties":false,"properties":{"first":{"type":"string"},"second":{"type":"string"}}}`)
	limits := DefaultSchemaLimits()
	limits.MaxFields = 1
	_, err := SchemaDescriptorFromJSONSchema("@test", object, JSONSchemaPolicy{Limits: limits})
	assertJSONSchemaError(t, err, "/properties/second", "field count")

	nested := []byte(`{"type":"object","additionalProperties":false,"properties":{"child":{"type":"object","additionalProperties":false,"properties":{"name":{"type":"string"}}}}}`)
	limits = DefaultSchemaLimits()
	limits.MaxDepth = 1
	_, err = SchemaDescriptorFromJSONSchema("@test", nested, JSONSchemaPolicy{Limits: limits})
	assertJSONSchemaError(t, err, "/properties/child/properties/name", "nesting")

	limits = DefaultSchemaLimits()
	limits.MaxPathSegments = 1
	_, err = SchemaDescriptorFromJSONSchema("@test", nested, JSONSchemaPolicy{Limits: limits})
	assertJSONSchemaError(t, err, "/properties/child/properties/name", "segment limit")

	limits = DefaultSchemaLimits()
	limits.MaxPathBytes = 4
	_, err = SchemaDescriptorFromJSONSchema("@test", object, JSONSchemaPolicy{Limits: limits})
	assertJSONSchemaError(t, err, "/properties/first", "byte limit")

	array := []byte(`{"type":"array","maxItems":3,"items":{"type":"string"}}`)
	limits = DefaultSchemaLimits()
	limits.MaxListItems = 2
	_, err = SchemaDescriptorFromJSONSchema("@test", array, JSONSchemaPolicy{Limits: limits})
	assertJSONSchemaError(t, err, "/maxItems", "bound")

	_, err = SchemaDescriptorFromJSONSchema("@test", []byte(`{"type":"string"}`), JSONSchemaPolicy{Limits: SchemaLimits{MaxFields: 1}})
	assertJSONSchemaError(t, err, "", "limits")

	_, err = SchemaDescriptorFromJSONSchema("@test", []byte(`{"type":"string"}`), JSONSchemaPolicy{MaxDocumentBytes: 2})
	assertJSONSchemaError(t, err, "", "MaxDocumentBytes")

	_, err = SchemaDescriptorFromJSONSchema("@test", []byte(`{"type":"string"}`), JSONSchemaPolicy{MaxDocumentBytes: maxJSONSchemaBytes + 1})
	assertJSONSchemaError(t, err, "", "hard cap")
}

func TestSchemaDescriptorFromJSONSchemaValidatesBoundary(t *testing.T) {
	t.Parallel()

	_, err := SchemaDescriptorFromJSONSchema("test", []byte(`{"type":"string"}`), JSONSchemaPolicy{})
	assertJSONSchemaError(t, err, "", "@name")
	_, err = SchemaDescriptorFromJSONSchema("@test", nil, JSONSchemaPolicy{})
	assertJSONSchemaError(t, err, "", "empty")
	_, err = SchemaDescriptorFromJSONSchema("@test", []byte{0xff}, JSONSchemaPolicy{})
	assertJSONSchemaError(t, err, "", "UTF-8")
}

func assertJSONSchemaError(t *testing.T, err error, pointer, problem string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", problem)
	}
	if !errors.Is(err, ErrJSONSchemaAdapter) {
		t.Fatalf("error does not wrap ErrJSONSchemaAdapter: %v", err)
	}
	var typed *JSONSchemaError
	if !errors.As(err, &typed) {
		t.Fatalf("error is not a JSONSchemaError: %T", err)
	}
	if typed.Pointer != pointer {
		t.Fatalf("error pointer = %q, want %q (%v)", typed.Pointer, pointer, err)
	}
	if !strings.Contains(err.Error(), problem) {
		t.Fatalf("error %q does not contain %q", err, problem)
	}
}
