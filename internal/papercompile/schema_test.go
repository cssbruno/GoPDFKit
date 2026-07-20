// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
)

const schemaFixture = `document:
  schema invoice:
    optional object customer:
      string name
    list object items:
      max-items: 100
      number price
    number total
`

func TestCompileBuildsDeterministicSchemaIRAndValidatesListPath(t *testing.T) {
	source := schemaFixture + "  page:\n    body:\n      paragraph @price-output:\n        bind: \"total\"\n        text: \"Price\"\n"
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

func TestCompileSingleSchemaAllowsRootRelativeBindings(t *testing.T) {
	source := schemaFixture + "  page:\n    body:\n" +
		"      paragraph @total-output:\n        bind: \"total\"\n        text: \"Total\"\n" +
		"      paragraph @customer-output:\n        bind: \"customer.name\"\n        bind-required: false\n        text: \"Customer\"\n"
	parsed := paperlang.Parse("relative-binding.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if got := mappingByID(compiled.Mapping, "@total-output").BindingPath; got != "@invoice.total" {
		t.Fatalf("total binding = %q, want @invoice.total", got)
	}
	if got := mappingByID(compiled.Mapping, "@customer-output").BindingPath; got != "@invoice.customer.name" {
		t.Fatalf("customer binding = %q, want @invoice.customer.name", got)
	}
}

func TestCompileAnonymousSchemaUsesOnlyRootRelativeAuthorSyntax(t *testing.T) {
	const source = `document:
  schema:
    string name
  page:
    body:
      paragraph @name:
        bind: "name"
        text: "Name"
`
	parsed := paperlang.Parse("anonymous-schema.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if len(compiled.Schemas) != 1 || compiled.Schemas[0].Name != "@root" || mappingByID(compiled.Mapping, "@name").BindingPath != "@root.name" {
		t.Fatalf("anonymous schema = %+v / %+v", compiled.Schemas, compiled.Mapping)
	}
}

func TestCompileExpandsReusableCustomObjects(t *testing.T) {
	const source = `document:
  object Address:
    string street
    string city
  object Patient:
    string name
    Address address
  schema:
    Patient patient
    optional Address delivery
    list Address history:
      max-items: 5
  page:
    body:
      paragraph @city:
        bind: "patient.address.city"
        text: "City"
`
	parsed := paperlang.Parse("custom-objects.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	if len(compiled.Schemas) != 1 || len(compiled.Schemas[0].Fields) != 3 {
		t.Fatalf("schemas = %+v", compiled.Schemas)
	}
	patient := compiled.Schemas[0].Fields[0]
	if patient.Kind != SchemaObject || len(patient.Fields) != 2 || patient.Fields[1].Name != "address" || len(patient.Fields[1].Fields) != 2 {
		t.Fatalf("patient descriptor = %+v", patient)
	}
	history := compiled.Schemas[0].Fields[2]
	if history.Kind != SchemaList || history.ItemKind != SchemaObject || history.MaxItems != 5 || len(history.Fields) != 2 {
		t.Fatalf("history descriptor = %+v", history)
	}
	if got := mappingByID(compiled.Mapping, "@city").BindingPath; got != "@root.patient.address.city" {
		t.Fatalf("custom object binding = %q", got)
	}
}

func TestCompileRejectsUnknownAndRecursiveCustomObjects(t *testing.T) {
	tests := []struct {
		name   string
		source string
		code   string
	}{
		{
			name: "unknown",
			source: `document:
  schema:
    Address address
  page:
    body:
      text: "x"
`,
			code: "PAPER_SCHEMA_OBJECT_UNKNOWN",
		},
		{
			name: "cycle",
			source: `document:
  object A:
    B b
  object B:
    A a
  schema:
    A root
  page:
    body:
      text: "x"
`,
			code: "PAPER_SCHEMA_OBJECT_CYCLE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parsed := paperlang.Parse(test.name+".paper", test.source)
			compiled := Compile(parsed.AST)
			if !parsed.OK() || compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, test.code) {
				t.Fatalf("diagnostics = %+v / %+v, want %s", parsed.Diagnostics, compiled.Diagnostics, test.code)
			}
		})
	}
}

func TestCompileRelativeBindingRequiresExactlyOneSchema(t *testing.T) {
	source := `document:
  schema first:
    string value
  schema second:
    string value
  page:
    body:
      paragraph:
        bind: "value"
        text: "Value"
`
	parsed := paperlang.Parse("ambiguous-binding.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, "PAPER_BIND_PATH") {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	found := false
	for _, diagnostic := range compiled.Diagnostics {
		if diagnostic.Code == "PAPER_BIND_PATH" && strings.Contains(diagnostic.Message, "ambiguous") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v, want relative binding ambiguity", compiled.Diagnostics)
	}
}

func TestCompileComponentRelativeBindingPreservesInstanceProvenance(t *testing.T) {
	source := schemaFixture + "  component @customer-card:\n    paragraph @name:\n      bind: \"name\"\n      text: \"Name\"\n  page:\n    body:\n      use @card-one:\n        component: \"@customer-card\"\n        bind: \"customer\"\n        bind-required: false\n"
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
		{"unknown field", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"missing\"\n        text: \"x\"\n", "PAPER_BIND_PATH"},
		{"list traversal", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"items.price\"\n        text: \"x\"\n", "PAPER_BIND_PATH"},
		{"collection text", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"items[].price\"\n        bind-required: false\n        text: \"x\"\n", "PAPER_BIND_COLLECTION"},
		{"object terminal", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"customer\"\n        bind-required: false\n        text: \"x\"\n", "PAPER_BIND_TARGET_TYPE"},
		{"list terminal", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"items\"\n        text: \"x\"\n", "PAPER_BIND_TARGET_TYPE"},
		{"nullable required", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"customer.name\"\n        text: \"x\"\n", "PAPER_BIND_NULLABLE"},
		{"expression", schemaFixture + "  page:\n    body:\n      paragraph:\n        bind: \"items + total\"\n        text: \"x\"\n", "PAPER_BIND_PATH"},
		{"duplicate field", `document:
  schema s:
    string x
    number x
  page:
    body:
      text: "x"
`, "PAPER_FIELD_DUPLICATE"},
		{"unbounded list", `document:
  schema s:
    list string xs:
  page:
    body:
      text: "x"
`, "PAPER_FIELD_LIST_BOUND"},
		{"computed field", `document:
  schema s:
    object x:
      expression: "1 + 2"
      string value
  page:
    body:
      text: "x"
`, "PAPER_SCHEMA_EXPRESSION_UNSUPPORTED"},
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
