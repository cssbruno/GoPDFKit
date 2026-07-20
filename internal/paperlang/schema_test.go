// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestSchemaFieldAndBindingSyntaxFormatsStably(t *testing.T) {
	const source = `document:
  schema invoice:
    optional object customer:
      string name
    list object items:
      max-items: 100
      number price
  page:
    body:
      paragraph @price:
        bind: "items[].price"
        text: "Price"
`
	parsed := Parse("schema.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	schema := parsed.AST.Root.Members[0].Node
	if schema.Kind != NodeSchema || schema.ID != "@invoice" || schema.Members[0].Node.Kind != NodeField {
		t.Fatalf("schema AST = %#v", schema)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	reparsed := Parse("schema.paper", string(formatted))
	second, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(formatted, second) {
		t.Fatalf("schema format stability failed: %+v / %v\n%s\n%s", reparsed.Diagnostics, secondErr, formatted, second)
	}
}

func TestLegacySchemaDeclarationsAreRejected(t *testing.T) {
	for _, source := range []string{
		"document:\n  schema @invoice:\n    string total\n",
		"document:\n  schema:\n    field @total:\n      type: \"number\"\n",
	} {
		if parsed := Parse("legacy-schema.paper", source); parsed.OK() {
			t.Fatalf("legacy schema unexpectedly parsed:\n%s", source)
		}
	}
}

func TestCustomObjectTypesFormatStably(t *testing.T) {
	const source = `document:
  object Address:
    string street
    string city
  schema:
    Address billing
    optional Address shipping
    list Address previous:
      max-items: 5
  page:
    body:
      text: "Addresses"
`
	parsed := Parse("custom-objects.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	objectType := parsed.AST.Root.Members[0].Node
	schema := parsed.AST.Root.Members[1].Node
	if objectType.Kind != NodeObjectType || objectType.ID != "@Address" {
		t.Fatalf("custom object AST = %#v", objectType)
	}
	if field := schema.Members[0].Node; field.TypeRef != "Address" || field.FieldType != "" {
		t.Fatalf("custom field AST = %#v", field)
	}
	if field := schema.Members[2].Node; field.FieldType != FieldList || field.ItemTypeRef != "Address" || field.ItemType != "" {
		t.Fatalf("custom list AST = %#v", field)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	reparsed := Parse("custom-objects.paper", string(formatted))
	second, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(formatted, second) {
		t.Fatalf("custom object format stability failed: %+v / %v\n%s\n%s", reparsed.Diagnostics, secondErr, formatted, second)
	}
}
