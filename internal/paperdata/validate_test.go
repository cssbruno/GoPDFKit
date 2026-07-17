// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperdata

import (
	"errors"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

func fixtureSchema() papercompile.SchemaDescriptor {
	return papercompile.SchemaDescriptor{Name: "@invoice", Kind: papercompile.SchemaObject, Fields: []papercompile.FieldDescriptor{
		{Name: "customer", Kind: papercompile.SchemaObject, Required: true, Fields: []papercompile.FieldDescriptor{{Name: "name", Kind: papercompile.SchemaString, Required: true}, {Name: "note", Kind: papercompile.SchemaString}}},
		{Name: "lines", Kind: papercompile.SchemaList, Required: true, ItemKind: papercompile.SchemaObject, ItemRequired: true, MaxItems: 2, Fields: []papercompile.FieldDescriptor{{Name: "quantity", Kind: papercompile.SchemaNumber, Required: true}}},
	}}
}

func validFixture() paperscenario.Fixture {
	return paperscenario.Fixture{Name: "typical", Values: []paperscenario.Field{
		{Name: "customer", Value: paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{{Name: "name", Value: paperscenario.Value{Kind: paperscenario.String, String: "Ada"}}, {Name: "note", Value: paperscenario.Value{Kind: paperscenario.Null}}}}},
		{Name: "lines", Value: paperscenario.Value{Kind: paperscenario.List, List: []paperscenario.Item{{Key: "a", Value: paperscenario.Value{Kind: paperscenario.Object, Object: []paperscenario.Field{{Name: "quantity", Value: paperscenario.Value{Kind: paperscenario.Number, Number: "2"}}}}}}}},
	}}
}

func TestValidateFixtureAndLookupPrimitive(t *testing.T) {
	fixture := validFixture()
	if err := ValidateFixture(fixtureSchema(), fixture, Limits{}); err != nil {
		t.Fatal(err)
	}
	value, err := LookupPrimitive(fixtureSchema(), fixture, "@invoice.customer.name", Limits{})
	if err != nil || value.Kind != paperscenario.String || value.String != "Ada" {
		t.Fatalf("lookup = %+v, %v", value, err)
	}
	value.String = "changed"
	again, err := LookupPrimitive(fixtureSchema(), fixture, "customer.name", Limits{})
	if err != nil || again.String != "Ada" {
		t.Fatal("lookup result aliases fixture")
	}
}

func TestValidateFixtureRejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name string
		edit func(*paperscenario.Fixture)
	}{
		{"missing", func(f *paperscenario.Fixture) { f.Values = f.Values[1:] }},
		{"unknown", func(f *paperscenario.Fixture) {
			f.Values = append(f.Values, paperscenario.Field{Name: "unknown", Value: paperscenario.Value{Kind: paperscenario.String}})
		}},
		{"kind", func(f *paperscenario.Fixture) {
			f.Values[0].Value.Object[0].Value = paperscenario.Value{Kind: paperscenario.Bool}
		}},
		{"required-null", func(f *paperscenario.Fixture) {
			f.Values[0].Value.Object[0].Value = paperscenario.Value{Kind: paperscenario.Null}
		}},
		{"too-many", func(f *paperscenario.Fixture) {
			item := f.Values[1].Value.List[0]
			f.Values[1].Value.List = append(f.Values[1].Value.List, item, item)
		}},
		{"duplicate-key", func(f *paperscenario.Fixture) {
			f.Values[1].Value.List = append(f.Values[1].Value.List, f.Values[1].Value.List[0])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := validFixture()
			test.edit(&fixture)
			if err := ValidateFixture(fixtureSchema(), fixture, Limits{}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestLookupRejectsCollectionsAndLimits(t *testing.T) {
	fixture := validFixture()
	if _, err := LookupPrimitive(fixtureSchema(), fixture, "lines[].quantity", Limits{}); !errors.Is(err, ErrPath) {
		t.Fatalf("collection error = %v", err)
	}
	limits := DefaultLimits()
	limits.MaxNodes = 1
	if err := ValidateFixture(fixtureSchema(), fixture, limits); !errors.Is(err, ErrLimit) {
		t.Fatalf("limit error = %v", err)
	}
}

func TestLookupKeyedPrimitiveUsesStableIdentity(t *testing.T) {
	fixture := validFixture()
	value, err := LookupKeyedPrimitive(fixtureSchema(), fixture, "@invoice.lines", "a", "quantity", Limits{})
	if err != nil || value.Kind != paperscenario.Number || value.Number != "2" {
		t.Fatalf("lookup = %+v, %v", value, err)
	}
	if _, err := LookupKeyedPrimitive(fixtureSchema(), fixture, "lines", "missing", "quantity", Limits{}); !errors.Is(err, ErrPath) {
		t.Fatalf("missing key error = %v", err)
	}
}
