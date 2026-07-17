// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/papertheme"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompileAppliesSelectedRootThemeTokensWithProvenance(t *testing.T) {
	source := "document @doc:\n" +
		"  theme: \"@print\"\n" +
		"  theme @base:\n" +
		"    token @body-font:\n" +
		"      type: \"string\"\n" +
		"      value: \"Courier\"\n" +
		"    token @body-size:\n" +
		"      type: \"length\"\n" +
		"      value: 11pt\n" +
		"    token @leading:\n" +
		"      type: \"length\"\n" +
		"      value: 14pt\n" +
		"    token @ink:\n" +
		"      type: \"color\"\n" +
		"      value: \"#336699\"\n" +
		"  theme @print:\n" +
		"    parent: \"base\"\n" +
		"    token @print-ink:\n" +
		"      type: \"color\"\n" +
		"      reference: \"ink\"\n" +
		"  page:\n" +
		"    body:\n" +
		"      paragraph @message:\n" +
		"        font-token: \"body-font\"\n" +
		"        size-token: \"@body-size\"\n" +
		"        line-height-token: \"leading\"\n" +
		"        color-token: \"print-ink\"\n" +
		"        text: \"Themed\"\n"
	parsed := paperlang.Parse("apply-theme.paper", source)
	result := Compile(parsed.AST)
	if !parsed.OK() || !result.OK() {
		t.Fatalf("parse/compile diagnostics = %#v / %#v", parsed.Diagnostics, result.Diagnostics)
	}
	paragraph := result.Document.Body[0].(layout.ParagraphBlock)
	wantStyle := layout.TextStyle{
		FontFamily: "Courier", FontSize: 11, LineHeight: 14,
		Color: layout.DocumentColor{R: 51, G: 102, B: 153, Set: true},
	}
	if paragraph.Style != wantStyle {
		t.Fatalf("themed style = %#v, want %#v", paragraph.Style, wantStyle)
	}
	if len(result.Mapping.ThemeProperties) != 4 {
		t.Fatalf("theme mapping = %#v", result.Mapping.ThemeProperties)
	}
	properties := make(map[string]ThemePropertyMapping)
	for _, mapping := range result.Mapping.ThemeProperties {
		properties[mapping.Property] = mapping
		if mapping.NodeID != "@message" || mapping.NodeKind != paperlang.NodeParagraph || mapping.Theme != "print" || mapping.ConsumerSpan.File != "apply-theme.paper" || len(mapping.Provenance.Chain) == 0 {
			t.Fatalf("incomplete theme mapping = %#v", mapping)
		}
	}
	color := properties["color-token"]
	if color.Value != (papertheme.Value{Kind: papertheme.Color, Color: "#336699"}) || len(color.Provenance.Chain) != 2 ||
		color.Provenance.Chain[0].Theme != "print" || color.Provenance.Chain[1].Theme != "base" {
		t.Fatalf("color provenance = %#v", color)
	}

	projection := result.Mapping.ThemeProperties
	parsed.AST.Root.ID = "@mutated"
	if !reflect.DeepEqual(projection, result.Mapping.ThemeProperties) {
		t.Fatal("compile mapping changed after AST mutation")
	}
}

func TestCompileThemeLiteralPropertiesOverrideTokensWithWarning(t *testing.T) {
	source := "document:\n" +
		"  theme: \"base\"\n" +
		"  theme @base:\n" +
		"    token @font:\n      type: \"string\"\n      value: \"Courier\"\n" +
		"    token @size:\n      type: \"length\"\n      value: 20pt\n" +
		"    token @ink:\n      type: \"color\"\n      value: \"#112233\"\n" +
		"  page:\n    body:\n      paragraph @literal:\n" +
		"        font-token: \"font\"\n        font: \"Helvetica\"\n" +
		"        size-token: \"size\"\n        size: 9pt\n" +
		"        color-token: \"ink\"\n        color: \"#abcdef\"\n" +
		"        text: \"literal wins\"\n"
	parsed := paperlang.Parse("override.paper", source)
	result := Compile(parsed.AST)
	if !parsed.OK() || !result.OK() {
		t.Fatalf("parse/compile diagnostics = %#v / %#v", parsed.Diagnostics, result.Diagnostics)
	}
	style := result.Document.Body[0].(layout.ParagraphBlock).Style
	if style.FontFamily != "Helvetica" || style.FontSize != 9 || style.Color != (layout.DocumentColor{R: 171, G: 205, B: 239, Set: true}) {
		t.Fatalf("literal precedence style = %#v", style)
	}
	warnings := 0
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "PAPER_COMPILE_THEME_STYLE_OVERRIDE" && diagnostic.Severity == paperlang.SeverityWarning {
			warnings++
		}
	}
	if warnings != 3 || len(result.Mapping.ThemeProperties) != 0 {
		t.Fatalf("override diagnostics/mapping = %#v / %#v", result.Diagnostics, result.Mapping.ThemeProperties)
	}
}

func TestCompileThemeUseDiagnosticsAnchorConsumer(t *testing.T) {
	tests := []struct {
		name      string
		theme     string
		property  string
		wantCode  string
		wantValue string
	}{
		{"no selection", "", `size-token: "size"`, "PAPER_COMPILE_THEME_REQUIRED", ""},
		{"unknown", "  theme: \"base\"\n", `size-token: "missing"`, "PAPER_COMPILE_THEME_TOKEN_UNKNOWN", ""},
		{"type", "  theme: \"base\"\n", `size-token: "font"`, "PAPER_COMPILE_THEME_TOKEN_TYPE", ""},
		{"unit", "  theme: \"base\"\n", `size-token: "millimeters"`, "PAPER_COMPILE_THEME_TOKEN_VALUE", ""},
		{"alpha color", "  theme: \"base\"\n", `color-token: "alpha"`, "PAPER_COMPILE_THEME_TOKEN_VALUE", ""},
		{"bad font", "  theme: \"base\"\n", `font-token: "bad-font"`, "PAPER_COMPILE_THEME_TOKEN_VALUE", ""},
	}
	declarations := "  theme @base:\n" +
		"    token @size:\n      type: \"length\"\n      value: 10pt\n" +
		"    token @font:\n      type: \"string\"\n      value: \"Courier\"\n" +
		"    token @millimeters:\n      type: \"length\"\n      value: 2mm\n" +
		"    token @alpha:\n      type: \"color\"\n      value: \"#11223344\"\n" +
		"    token @bad-font:\n      type: \"string\"\n      value: \"Comic Sans\"\n"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := "document:\n" + test.theme + declarations +
				"  page:\n    body:\n      paragraph:\n        " + test.property + "\n        text: \"x\"\n"
			parsed := paperlang.Parse("consumer.paper", source)
			result := Compile(parsed.AST)
			if !parsed.OK() || result.OK() {
				t.Fatalf("parse/compile = %#v / %#v", parsed.Diagnostics, result.Diagnostics)
			}
			found := false
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Code == test.wantCode && diagnostic.Span.File == "consumer.paper" && diagnostic.Span.Start.Line > 10 {
					found = true
				}
			}
			if !found {
				t.Fatalf("consumer diagnostic %s not found: %#v", test.wantCode, result.Diagnostics)
			}
		})
	}
}
