// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/papertheme"
)

func TestCompileThemeSourceExtractsAndResolvesTypedDeclarations(t *testing.T) {
	source := "document @invoice:\n" +
		"  theme @print:\n" +
		"    parent: \"@base\"\n" +
		"    token @inherited-gap:\n" +
		"      type: \"length\"\n" +
		"      reference: \"gap\"\n" +
		"    scope @invoice:\n" +
		"      token @brand:\n" +
		"        type: \"color\"\n" +
		"        value: \"#112233\"\n" +
		"      token @heading:\n" +
		"        type: \"color\"\n" +
		"        reference: \"brand\"\n" +
		"  theme @base:\n" +
		"    token @brand:\n" +
		"      type: \"color\"\n" +
		"      value: \"#AABBCC\"\n" +
		"    token @label:\n" +
		"      type: \"string\"\n" +
		"      value: \"Invoice\"\n" +
		"    token @gap:\n" +
		"      type: \"length\"\n" +
		"      value: 12.50pt\n" +
		"    token @ratio:\n" +
		"      type: \"number\"\n" +
		"      value: +1.25\n" +
		"    token @enabled:\n" +
		"      type: \"bool\"\n" +
		"      value: false\n" +
		"  page:\n" +
		"    body:\n" +
		"      text: \"preview\"\n"
	first := CompileThemeSource("invoice.paper", source)
	second := CompileThemeSource("invoice.paper", source)
	if !first.OK() || !reflect.DeepEqual(first, second) {
		t.Fatalf("theme compile = %#v / %#v", first, second)
	}
	if len(first.Input.Themes) != 2 || first.Input.Themes[0].Name != "print" || first.Input.Themes[0].Parent != "base" || len(first.Input.Themes[0].Scopes) != 1 {
		t.Fatalf("theme input = %#v", first.Input)
	}
	base := first.Input.Themes[1]
	if len(base.Tokens) != 5 || base.Tokens[0].Value != (papertheme.Value{Kind: papertheme.Color, Color: "#AABBCC"}) ||
		base.Tokens[2].Value != (papertheme.Value{Kind: papertheme.Length, Length: papertheme.LengthValue{Number: "12.5", Unit: "pt"}}) ||
		base.Tokens[3].Value != (papertheme.Value{Kind: papertheme.Number, Number: "1.25"}) ||
		base.Tokens[4].Value != (papertheme.Value{Kind: papertheme.Bool}) {
		t.Fatalf("typed base tokens = %#v", base.Tokens)
	}
	if len(first.Output.Themes) != 2 || first.Output.Themes[0].Name != "base" || first.Output.Themes[1].Name != "print" || first.Digest == "" {
		t.Fatalf("resolved output/digest = %#v / %q", first.Output, first.Digest)
	}
	var inherited *papertheme.ResolvedToken
	for index := range first.Output.Themes[1].Tokens {
		if first.Output.Themes[1].Tokens[index].Name == "inherited-gap" {
			inherited = &first.Output.Themes[1].Tokens[index]
		}
	}
	if inherited == nil || inherited.Value.Kind != papertheme.Length || len(inherited.Provenance.Chain) != 2 || inherited.Provenance.Chain[1].Theme != "base" || inherited.Provenance.Chain[0].Source.File != "invoice.paper" {
		t.Fatalf("inherited token/provenance = %#v", inherited)
	}
	canonical, err := first.Output.CanonicalJSON()
	if err != nil || !bytes.Equal(canonical, first.Canonical) {
		t.Fatalf("canonical output = %q / %v", first.Canonical, err)
	}

	parsed := paperlang.Parse("invoice.paper", source)
	layoutResult := Compile(parsed.AST)
	if !layoutResult.OK() {
		t.Fatalf("layout compiler did not ignore themes: %#v", layoutResult.Diagnostics)
	}
}

func TestCompileThemeSourceReportsSourceLocatedDiagnostics(t *testing.T) {
	source := "document:\n" +
		"  theme @broken:\n" +
		"    parent: \"missing\"\n" +
		"    mystery: true\n" +
		"    token @same:\n" +
		"      type: \"number\"\n" +
		"      value: 1\n" +
		"    token @same:\n" +
		"      type: \"number\"\n" +
		"      value: 2\n" +
		"    token @unknown:\n" +
		"      type: \"color\"\n" +
		"      reference: \"missing-token\"\n" +
		"    token @wrong:\n" +
		"      type: \"number\"\n" +
		"      value: \"not-a-number\"\n" +
		"    token @nullish:\n" +
		"      type: \"string\"\n" +
		"      value: null\n" +
		"    token @both:\n" +
		"      type: \"number\"\n" +
		"      value: 1\n" +
		"      reference: \"same\"\n"
	first := CompileThemeSource("broken.paper", source)
	second := CompileThemeSource("broken.paper", source)
	if first.OK() || !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("diagnostics = %#v / %#v", first.Diagnostics, second.Diagnostics)
	}
	codes := make(map[string]bool)
	for _, diagnostic := range first.Diagnostics {
		codes[diagnostic.Code] = true
		if diagnostic.Span.File != "broken.paper" || diagnostic.Span.Start.Line == 0 {
			t.Fatalf("diagnostic is not source located: %#v", diagnostic)
		}
	}
	for _, code := range []string{"PAPER_THEME_PROPERTY", "PAPER_THEME_PARENT_UNKNOWN", "PAPER_THEME_TOKEN_DUPLICATE", "PAPER_THEME_TOKEN_UNKNOWN", "PAPER_THEME_TOKEN_TYPE", "PAPER_THEME_TOKEN_VALUE"} {
		if !codes[code] {
			t.Fatalf("codes = %#v, want %s; diagnostics=%#v", codes, code, first.Diagnostics)
		}
	}
}

func TestCompileThemeSourceEnforcesDeclarationLimits(t *testing.T) {
	source := "document:\n  theme @bounded:\n    token @one:\n      type: \"number\"\n      value: 1\n    token @two:\n      type: \"number\"\n      value: 2\n"
	limits := papertheme.DefaultLimits()
	limits.MaxTokens = 1
	result := CompileThemeSourceWithLimits("bounded.paper", source, limits)
	if result.OK() {
		t.Fatal("token limit unexpectedly accepted")
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "PAPER_THEME_TOKEN_LIMIT"
	}
	if !found {
		t.Fatalf("limit diagnostics = %#v", result.Diagnostics)
	}
}
