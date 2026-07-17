// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const schemaFixture = "document:\n" +
	"  schema @invoice:\n" +
	"    field @customer:\n" +
	"      type: \"object\"\n" +
	"      required: false\n" +
	"      field @name:\n" +
	"        type: \"string\"\n" +
	"    field @items:\n" +
	"      type: \"list\"\n" +
	"      item-type: \"object\"\n" +
	"      max-items: 100\n" +
	"      field @price:\n" +
	"        type: \"number\"\n" +
	"    field @total:\n" +
	"      type: \"number\"\n"

func TestCompileBuildsDeterministicSchemaIRAndValidatesListPath(t *testing.T) {
	source := schemaFixture + "  page:\n    body:\n      paragraph @price-output:\n        bind: \"@invoice.total\"\n        text: \"Price\"\n"
	parsed := paperlang.Parse("schema.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if len(compiled.Schemas) != 1 || compiled.Schemas[0].Name != "@invoice" || len(compiled.Schemas[0].Fields) != 3 {
		t.Fatalf("schemas = %+v", compiled.Schemas)
	}
	items := compiled.Schemas[0].Fields[1]
	if items.Kind != SchemaList || items.ItemKind != SchemaObject || items.MaxItems != 100 || len(items.Fields) != 1 || items.Fields[0].Kind != SchemaNumber {
		t.Fatalf("items descriptor = %+v", items)
	}
	mapping := mappingByID(compiled.Mapping, "@price-output")
	if mapping.BindingPath != "@invoice.total" || mapping.BindingNullable || mapping.BindingCollection || mapping.BindingSpan.File != "schema.paper" {
		t.Fatalf("binding mapping = %+v", mapping)
	}
	second := Compile(parsed.AST)
	if len(second.Schemas) != 1 || second.Schemas[0].Fields[1].MaxItems != compiled.Schemas[0].Fields[1].MaxItems {
		t.Fatalf("schema compilation is nondeterministic: %+v / %+v", compiled.Schemas, second.Schemas)
	}
}

func TestCompileComponentRelativeBindingPreservesInstanceProvenance(t *testing.T) {
	source := schemaFixture + "  component @customer-card:\n    paragraph @name:\n      bind: \"name\"\n      text: \"Name\"\n  page:\n    body:\n      use @card-one:\n        component: \"@customer-card\"\n        bind: \"@invoice.customer\"\n        bind-required: false\n"
	parsed := paperlang.Parse("component-binding.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	var mapping NodeMapping
	for _, candidate := range compiled.Mapping.Nodes {
		if strings.Contains(candidate.ID, "card-one--name") {
			mapping = candidate
		}
	}
	if mapping.BindingPath != "@invoice.customer.name" || !mapping.BindingNullable ||
		mapping.InstancePath != "@card-one" || mapping.DefinitionSpan.File == "" || mapping.InvocationSpan.File == "" {
		t.Fatalf("component binding provenance = %+v", mapping)
	}
}

func TestCompileRejectsInvalidBindingsAndSchemaContracts(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{"unknown field", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.missing\"\n        text: \"x\"\n", "PAPER_BIND_PATH"},
		{"list traversal", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.items.price\"\n        text: \"x\"\n", "PAPER_BIND_PATH"},
		{"collection text", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.items[].price\"\n        bind-required: false\n        text: \"x\"\n", "PAPER_BIND_COLLECTION"},
		{"object terminal", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.customer\"\n        bind-required: false\n        text: \"x\"\n", "PAPER_BIND_TARGET_TYPE"},
		{"list terminal", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.items\"\n        text: \"x\"\n", "PAPER_BIND_TARGET_TYPE"},
		{"nullable required", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.customer.name\"\n        text: \"x\"\n", "PAPER_BIND_NULLABLE"},
		{"expression", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"@invoice.items + total\"\n        text: \"x\"\n", "PAPER_BIND_PATH"},
		{"duplicate field", "document:\n  schema @s:\n    field @x:\n      type: \"string\"\n    field @x:\n      type: \"number\"\n  page:\n    body:\n      text: \"x\"\n", "PAPER_FIELD_DUPLICATE"},
		{"unbounded list", "document:\n  schema @s:\n    field @xs:\n      type: \"list\"\n      item-type: \"string\"\n  page:\n    body:\n      text: \"x\"\n", "PAPER_FIELD_LIST_BOUND"},
		{"computed field", "document:\n  schema @s:\n    field @x:\n      type: \"number\"\n      expression: \"1 + 2\"\n  page:\n    body:\n      text: \"x\"\n", "PAPER_SCHEMA_EXPRESSION_UNSUPPORTED"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := paperlang.Parse(test.name+".paper", test.source)
			if !parsed.OK() {
				t.Fatalf("parse diagnostics = %+v", parsed.Diagnostics)
			}
			compiled := Compile(parsed.AST)
			if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("diagnostics = %+v, want %s", compiled.Diagnostics, test.code)
			}
		})
	}
}

func TestSchemaAndPathLimitsAreEnforced(t *testing.T) {
	parsed := paperlang.Parse("schema-limits.paper", schemaFixture+"  page:\n    body:\n      text: \"x\"\n")
	partial := CompileWithSchemaLimits(parsed.AST, SchemaLimits{MaxSchemas: 1})
	if partial.OK() || !hasCompileDiagnostic(partial.Diagnostics, "PAPER_SCHEMA_LIMITS") {
		t.Fatalf("partial limits diagnostics = %+v", partial.Diagnostics)
	}
	limits := DefaultSchemaLimits()
	limits.MaxFields = 1
	limited := CompileWithSchemaLimits(parsed.AST, limits)
	if limited.OK() || !hasCompileDiagnostic(limited.Diagnostics, "PAPER_SCHEMA_FIELD_LIMIT") {
		t.Fatalf("field limits diagnostics = %+v", limited.Diagnostics)
	}
}

func mappingByID(mapping CompileMapping, id string) NodeMapping {
	for _, node := range mapping.Nodes {
		if node.ID == id {
			return node
		}
	}
	return NodeMapping{}
}
