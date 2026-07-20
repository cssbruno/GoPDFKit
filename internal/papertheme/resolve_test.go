// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papertheme

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"testing"
)

func TestResolveTypedInheritanceLexicalScopesAndProvenance(t *testing.T) {
	base := Theme{
		Name: "base", Source: testSource("theme.paper", 1),
		Tokens: []Token{
			literal("brand", Color, Value{Kind: Color, Color: "#AABBCC"}, 2),
			literal("label", String, Value{Kind: String, String: "Invoice"}, 3),
			literal("gap", Length, Value{Kind: Length, Length: LengthValue{Number: "12.5", Unit: "pt"}}, 4),
			literal("ratio", Number, Value{Kind: Number, Number: "1.25"}, 5),
			literal("enabled", Bool, Value{Kind: Bool, Bool: true}, 6),
		},
	}
	child := Theme{
		Name: "print", Parent: "base", Source: testSource("theme.paper", 10),
		Tokens: []Token{{Name: "inherited-gap", Kind: Length, Reference: "gap", Source: testSource("theme.paper", 11)}},
		Scopes: []Scope{{
			Name: "invoice", Source: testSource("theme.paper", 12),
			Tokens: []Token{
				literal("brand", Color, Value{Kind: Color, Color: "#112233"}, 13),
				{Name: "heading", Kind: Color, Reference: "brand", Source: testSource("theme.paper", 14)},
			},
			Scopes: []Scope{{Name: "line", Source: testSource("theme.paper", 15)}},
		}},
	}
	input := Input{
		Themes: []Theme{child, base}, // Forward parent references are supported.
		Properties: []Property{
			{Name: "z-heading", Theme: "print", Scope: []string{"invoice", "line"}, Token: "heading", Kind: Color, Source: testSource("consumer.paper", 2)},
			{Name: "a-gap", Theme: "print", Token: "inherited-gap", Kind: Length, Source: testSource("consumer.paper", 1)},
			{Name: "label", Theme: "print", Token: "label", Kind: String, Source: testSource("consumer.paper", 3)},
			{Name: "ratio", Theme: "base", Token: "ratio", Kind: Number, Source: testSource("consumer.paper", 4)},
			{Name: "enabled", Theme: "base", Token: "enabled", Kind: Bool, Source: testSource("consumer.paper", 5)},
		},
	}
	result := Resolve(input, Limits{})
	if !result.OK() {
		t.Fatalf("Resolve() diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Output.Themes) != 2 || result.Output.Themes[0].Name != "base" || result.Output.Themes[1].Name != "print" {
		t.Fatalf("resolved themes = %#v", result.Output.Themes)
	}
	if len(result.Output.Properties) != 5 || result.Output.Properties[0].Name != "a-gap" || result.Output.Properties[4].Name != "z-heading" {
		t.Fatalf("canonical property order = %#v", result.Output.Properties)
	}
	heading := result.Output.Properties[4]
	if heading.Value != (Value{Kind: Color, Color: "#112233"}) {
		t.Fatalf("nearest lexical value = %#v", heading.Value)
	}
	if heading.Provenance.Property.File != "consumer.paper" || len(heading.Provenance.Chain) != 2 ||
		heading.Provenance.Chain[0].Token != "heading" || !reflect.DeepEqual(heading.Provenance.Chain[0].Scope, []string{"invoice"}) ||
		heading.Provenance.Chain[1].Token != "brand" || heading.Provenance.Chain[1].Theme != "print" {
		t.Fatalf("heading provenance = %#v", heading.Provenance)
	}
	gap := result.Output.Properties[0]
	if gap.Value != (Value{Kind: Length, Length: LengthValue{Number: "12.5", Unit: "pt"}}) || len(gap.Provenance.Chain) != 2 || gap.Provenance.Chain[1].Theme != "base" {
		t.Fatalf("inherited gap/provenance = %#v / %#v", gap.Value, gap.Provenance)
	}
	if got := result.Output.Themes[0].Tokens[0].Value.Color; got != "#aabbcc" {
		t.Fatalf("color was not canonicalized: %q", got)
	}
	encoded, err := result.Output.CanonicalJSON()
	if err != nil || !bytes.Equal(encoded, result.Canonical) || len(result.Digest) != 64 {
		t.Fatalf("canonical output/digest = %q, %q, %v", result.Canonical, result.Digest, err)
	}
	sum := sha256.Sum256(result.Canonical)
	if result.Digest != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest = %q", result.Digest)
	}

	// Resolution must not retain caller-owned slices.
	input.Themes[0].Scopes[0].Name = "mutated"
	input.Properties[0].Scope[0] = "mutated"
	if heading.Provenance.Chain[0].Scope[0] != "invoice" || heading.Scope[0] != "invoice" {
		t.Fatal("resolved output aliases mutable input storage")
	}
}

func TestResolveCanonicalDigestIgnoresAuthoredDeclarationOrder(t *testing.T) {
	one := literal("one", Number, Value{Kind: Number, Number: "1"}, 1)
	two := literal("two", Number, Value{Kind: Number, Number: "2"}, 2)
	left := Input{Themes: []Theme{{Name: "z", Tokens: []Token{two, one}}, {Name: "a"}}, Properties: []Property{
		{Name: "second", Theme: "z", Token: "two", Kind: Number},
		{Name: "first", Theme: "z", Token: "one", Kind: Number},
	}}
	right := Input{Themes: []Theme{{Name: "a"}, {Name: "z", Tokens: []Token{one, two}}}, Properties: []Property{
		{Name: "first", Theme: "z", Token: "one", Kind: Number},
		{Name: "second", Theme: "z", Token: "two", Kind: Number},
	}}
	first, second := Resolve(left, Limits{}), Resolve(right, Limits{})
	if !first.OK() || !second.OK() || first.Digest != second.Digest || !bytes.Equal(first.Canonical, second.Canonical) {
		t.Fatalf("canonical results differ:\n%#v\n%#v", first, second)
	}
}

func TestResolveDiagnosticsAreDeterministicLocatedAndComprehensive(t *testing.T) {
	source := testSource("broken.theme", 1)
	input := Input{Themes: []Theme{
		{Name: "base", Source: source, Tokens: []Token{
			literal("same", Number, Value{Kind: Number, Number: "1"}, 2),
			literal("same", Number, Value{Kind: Number, Number: "2"}, 3),
			{Name: "unknown", Kind: Color, Reference: "missing", Source: testSource("broken.theme", 4)},
			{Name: "a", Kind: Color, Reference: "b", Source: testSource("broken.theme", 5)},
			{Name: "b", Kind: Color, Reference: "a", Source: testSource("broken.theme", 6)},
			{Name: "wrong", Kind: Number, Reference: "color", Source: testSource("broken.theme", 7)},
			literal("color", Color, Value{Kind: Color, Color: "#abcdef"}, 8),
			literal("bad-number", Number, Value{Kind: Number, Number: "01"}, 9),
		}, Scopes: []Scope{{Name: "repeat", Source: testSource("broken.theme", 10)}, {Name: "repeat", Source: testSource("broken.theme", 11)}}},
		{Name: "base", Source: testSource("broken.theme", 12)},
		{Name: "orphan", Parent: "missing-theme", Source: testSource("broken.theme", 13)},
		{Name: "cycle-a", Parent: "cycle-b", Source: testSource("broken.theme", 14)},
		{Name: "cycle-b", Parent: "cycle-a", Source: testSource("broken.theme", 15)},
	}, Properties: []Property{
		{Name: "unknown-theme", Theme: "nope", Token: "x", Kind: Color, Source: testSource("consumer.theme", 1)},
		{Name: "unknown-scope", Theme: "base", Scope: []string{"nope"}, Token: "same", Kind: Number, Source: testSource("consumer.theme", 2)},
		{Name: "bad-type", Theme: "base", Token: "same", Kind: Color, Source: testSource("consumer.theme", 3)},
		{Name: "unknown-token", Theme: "base", Token: "nope", Kind: Color, Source: testSource("consumer.theme", 4)},
		{Name: "unknown-token", Theme: "base", Token: "same", Kind: Number, Source: testSource("consumer.theme", 5)},
	}}
	first, second := Resolve(input, Limits{}), Resolve(input, Limits{})
	if first.OK() || !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("diagnostics are not deterministic:\n%#v\n%#v", first.Diagnostics, second.Diagnostics)
	}
	codes := make(map[string]bool)
	for _, diagnostic := range first.Diagnostics {
		codes[diagnostic.Code] = true
		if diagnostic.Message == "" || diagnostic.Severity != Error {
			t.Fatalf("unhelpful diagnostic = %#v", diagnostic)
		}
	}
	for _, code := range []string{
		"PAPER_THEME_DUPLICATE", "PAPER_THEME_PARENT_UNKNOWN", "PAPER_THEME_PARENT_CYCLE",
		"PAPER_THEME_TOKEN_DUPLICATE", "PAPER_THEME_SCOPE_DUPLICATE", "PAPER_THEME_TOKEN_UNKNOWN",
		"PAPER_THEME_TOKEN_CYCLE", "PAPER_THEME_TOKEN_TYPE", "PAPER_THEME_UNKNOWN",
		"PAPER_THEME_SCOPE_UNKNOWN", "PAPER_THEME_PROPERTY_TYPE", "PAPER_THEME_PROPERTY_DUPLICATE",
	} {
		if !codes[code] {
			t.Fatalf("diagnostic codes = %#v, want %s; diagnostics=%#v", codes, code, first.Diagnostics)
		}
	}
}

func TestResolveEnforcesHardLimits(t *testing.T) {
	base := Input{Themes: []Theme{{Name: "a", Tokens: []Token{
		literal("one", Number, Value{Kind: Number, Number: "1"}, 1),
		literal("two", Number, Value{Kind: Number, Number: "2"}, 2),
	}}}}
	tests := []struct {
		name   string
		input  Input
		limits Limits
		code   string
	}{
		{"invalid", base, Limits{MaxThemes: 1}, "PAPER_THEME_LIMITS"},
		{"themes", Input{Themes: []Theme{{Name: "a"}, {Name: "b"}}}, withLimit(func(l *Limits) { l.MaxThemes = 1 }), "PAPER_THEME_COUNT_LIMIT"},
		{"tokens", base, withLimit(func(l *Limits) { l.MaxTokens = 1 }), "PAPER_THEME_TOKEN_LIMIT"},
		{"depth", Input{Themes: []Theme{{Name: "a", Scopes: []Scope{{Name: "nested"}}}}}, withLimit(func(l *Limits) { l.MaxDepth = 1 }), "PAPER_THEME_DEPTH_LIMIT"},
		{"work", base, withLimit(func(l *Limits) { l.MaxWork = 1 }), "PAPER_THEME_WORK_LIMIT"},
		{"bytes", base, withLimit(func(l *Limits) { l.MaxSourceBytes = 1 }), "PAPER_THEME_BYTE_LIMIT"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Resolve(test.input, test.limits)
			found := false
			for _, diagnostic := range result.Diagnostics {
				found = found || diagnostic.Code == test.code
			}
			if result.OK() || !found {
				t.Fatalf("Resolve() = %#v, want %s", result, test.code)
			}
		})
	}
}

func literal(name string, kind Kind, value Value, line uint32) Token {
	return Token{Name: name, Kind: kind, Value: value, Source: testSource("theme.paper", line)}
}

func testSource(file string, line uint32) Source {
	return Source{File: file, StartOffset: uint64(line * 10), EndOffset: uint64(line*10 + 5), Line: line, Column: 1}
}

func withLimit(change func(*Limits)) Limits {
	limits := DefaultLimits()
	change(&limits)
	return limits
}
