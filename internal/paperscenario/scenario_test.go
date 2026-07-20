// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperscenario

import (
	"errors"
	"testing"
)

func TestResolveInheritanceMutationKeysAndIdentity(t *testing.T) {
	base := Scenario{Name: "typical", Locale: "en-US", Values: []Field{
		{Name: "total", Value: Value{Kind: Number, Number: "12.5"}},
		{Name: "customer", Value: Value{Kind: Object, Object: []Field{{Name: "name", Value: Value{Kind: String, String: "Ada"}}, {Name: "active", Value: Value{Kind: Bool, Bool: true}}}}},
		{Name: "lines", Value: Value{Kind: List, List: []Item{{Key: "sku-2", Value: Value{Kind: String, String: "B"}}, {Key: "sku-1", Value: Value{Kind: String, String: "A"}}}}},
	}}
	long := Scenario{Name: "long-names", Parent: "typical", Locale: "pt-BR", Mutations: []Mutation{
		{Path: "customer.name", Value: Value{Kind: String, String: "Ada Lovelace"}},
		{Path: "customer.active", Delete: true},
		{Path: "note", Value: Value{Kind: String, String: "boundary"}},
	}}
	fixtures, err := Resolve([]Scenario{long, base}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if fixtures[0].Locale != "pt-BR" || fixtures[0].Values[0].Name != "customer" || fixtures[0].Values[0].Value.Object[0].Name != "name" {
		t.Fatalf("unexpected canonical child: %+v", fixtures[0])
	}
	if got := fixtures[0].Values[0].Value.Object[0].Value.String; got != "Ada Lovelace" {
		t.Fatalf("mutation = %q", got)
	}
	if fixtures[0].Digest == "" || fixtures[0].Digest == fixtures[1].Digest {
		t.Fatal("scenario identities must be populated and distinct")
	}
	fixturesAgain, err := Resolve([]Scenario{long, base}, Limits{})
	if err != nil || fixturesAgain[0].Digest != fixtures[0].Digest {
		t.Fatalf("identity is not deterministic: %v %+v", err, fixturesAgain)
	}
	fixtures[0].Values[0].Value.Object[0].Value.String = "mutated"
	if fixturesAgain[0].Values[0].Value.Object[0].Value.String != "Ada Lovelace" {
		t.Fatal("resolved fixtures alias across calls")
	}
}

func TestResolveRejectsCyclesMissingParentsAndUnstableListKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []Scenario
	}{
		{"cycle", []Scenario{{Name: "a", Parent: "b"}, {Name: "b", Parent: "a"}}},
		{"missing", []Scenario{{Name: "a", Parent: "missing"}}},
		{"missing-key", []Scenario{{Name: "a", Values: []Field{{Name: "items", Value: Value{Kind: List, List: []Item{{Value: Value{Kind: Null}}}}}}}}},
		{"duplicate-key", []Scenario{{Name: "a", Values: []Field{{Name: "items", Value: Value{Kind: List, List: []Item{{Key: "x", Value: Value{Kind: Null}}, {Key: "x", Value: Value{Kind: Null}}}}}}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Resolve(test.input, Limits{})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestResolveLimitsAndMutationValidation(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxNodes = 1
	_, err := Resolve([]Scenario{{Name: "a", Values: []Field{{Name: "one", Value: Value{Kind: String}}, {Name: "two", Value: Value{Kind: String}}}}}, limits)
	if !errors.Is(err, ErrLimit) {
		t.Fatalf("node limit error = %v", err)
	}
	_, err = Resolve([]Scenario{{Name: "a", Mutations: []Mutation{{Path: "missing.child", Value: Value{Kind: String}}}}}, Limits{})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("mutation error = %v", err)
	}
}

func TestResolveRejectsNonCanonicalNumbers(t *testing.T) {
	for _, number := range []string{"", "+1", "01", "1.", "1.0", "NaN", "-0"} {
		_, err := Resolve([]Scenario{{Name: "a", Values: []Field{{Name: "n", Value: Value{Kind: Number, Number: number}}}}}, Limits{})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("number %q error = %v", number, err)
		}
	}
}
