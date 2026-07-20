// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperedge

import (
	"bytes"
	"testing"

	"github.com/cssbruno/paperrune/internal/papercompile"
)

func TestGenerateIsDeterministicAndSchemaValid(t *testing.T) {
	t.Parallel()
	schema := papercompile.SchemaDescriptor{Name: "@lab", Kind: papercompile.SchemaObject, Fields: []papercompile.FieldDescriptor{
		{Name: "name", Kind: papercompile.SchemaString, Required: true},
		{Name: "note", Kind: papercompile.SchemaString, Required: false},
		{Name: "results", Kind: papercompile.SchemaList, Required: true, ItemKind: papercompile.SchemaObject, ItemRequired: true, MaxItems: 8, Fields: []papercompile.FieldDescriptor{
			{Name: "label", Kind: papercompile.SchemaString, Required: true},
			{Name: "value", Kind: papercompile.SchemaNumber, Required: true},
			{Name: "critical", Kind: papercompile.SchemaBool, Required: true},
		}},
	}}
	options := Options{Count: 12, Seed: 42, MaxListItems: 5}
	first, err := Generate(schema, options)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(schema, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 12 || len(second) != len(first) {
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
	checks := []struct {
		index  int
		needle []byte
	}{
		{0, []byte(`"name": ""`)},
		{2, []byte(`\n`)},
		{3, []byte(`Linha 1`)},
		{4, []byte("Long name value")},
		{5, bytes.Repeat([]byte("W"), 128)},
		{7, []byte("João")},
		{8, []byte(`\"quoted\"`)},
		{9, []byte(`-999999999999.9999`)},
	}
	for _, check := range checks {
		if !bytes.Contains(first[check.index].JSON, check.needle) {
			t.Fatalf("boundary profile %s lacks %q:\n%s", first[check.index].Name, check.needle, first[check.index].JSON)
		}
	}
	if bytes.Count(first[6].JSON, []byte(`"value"`)) != 5 {
		t.Fatalf("dense list profile lacks five values:\n%s", first[6].JSON)
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
