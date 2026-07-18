// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperedge

import (
	"bytes"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
)

func TestGenerateIsDeterministicAndSchemaValid(t *testing.T) {
	t.Parallel()
	schema := papercompile.SchemaDescriptor{Name: "@lab", Kind: papercompile.SchemaObject, Fields: []papercompile.FieldDescriptor{
		{Name: "name", Kind: papercompile.SchemaString, Required: true},
		{Name: "note", Kind: papercompile.SchemaString, Required: false},
		{Name: "results", Kind: papercompile.SchemaList, Required: true, ItemKind: papercompile.SchemaObject, ItemRequired: true, MaxItems: 8, Fields: []papercompile.FieldDescriptor{
			{Name: "value", Kind: papercompile.SchemaNumber, Required: true},
			{Name: "critical", Kind: papercompile.SchemaBool, Required: true},
		}},
	}}
	options := Options{Count: 8, Seed: 42, MaxListItems: 5}
	first, err := Generate(schema, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(schema, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 8 || len(second) != len(first) {
		t.Fatalf("case count = %d / %d", len(first), len(second))
	}
	for index := range first {
		if first[index].Name != second[index].Name || first[index].Digest != second[index].Digest || !bytes.Equal(first[index].JSON, second[index].JSON) {
			t.Fatalf("case[%d] is not deterministic", index)
		}
		if _, err := papercompile.FixtureFromJSONData(first[index].JSON, []papercompile.SchemaDescriptor{schema}, papercompile.JSONDataOptions{}); err != nil {
			t.Fatalf("case[%d] invalid: %v\n%s", index, err, first[index].JSON)
		}
	}
	if !bytes.Contains(first[1].JSON, []byte("Long name value")) || bytes.Count(first[2].JSON, []byte(`"value"`)) != 5 || !bytes.Contains(first[3].JSON, []byte("João")) {
		t.Fatalf("boundary cases missing: %s\n%s\n%s", first[1].JSON, first[2].JSON, first[3].JSON)
	}
}

func TestGenerateRejectsUnboundedRequests(t *testing.T) {
	t.Parallel()
	schema := papercompile.SchemaDescriptor{Name: "@lab", Kind: papercompile.SchemaObject}
	if _, err := Generate(schema, Options{Count: maxCount + 1}); err == nil {
		t.Fatal("oversized count accepted")
	}
	if _, err := Generate(schema, Options{Count: 1, MaxListItems: maxListItems + 1}); err == nil {
		t.Fatal("oversized list cap accepted")
	}
}
