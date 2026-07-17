// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestThemeSyntaxParseFormatParseIsStable(t *testing.T) {
	source := "document @invoice:\n" +
		"    theme @print:\n" +
		"        parent: \"@base\"\n" +
		"        token @brand:\n" +
		"            value: \"#AABBCC\"\n" +
		"            type: \"color\"\n" +
		"        token @gap:\n" +
		"            type: \"length\"\n" +
		"            value: 12.50pt\n" +
		"        token @alias:\n" +
		"            type: \"color\"\n" +
		"            reference: \"brand\"\n" +
		"        scope @line-items:\n" +
		"            token @brand:\n" +
		"                type: \"color\"\n" +
		"                value: \"#112233\"\n" +
		"            scope @compact:\n" +
		"                token @enabled:\n" +
		"                    type: \"bool\"\n" +
		"                    value: true\n" +
		"    theme @base:\n"
	parsed := Parse("themes.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	want := "document @invoice:\n" +
		"  theme @print:\n" +
		"    parent: \"@base\"\n" +
		"    token @brand:\n" +
		"      type: \"color\"\n" +
		"      value: \"#AABBCC\"\n" +
		"    token @gap:\n" +
		"      type: \"length\"\n" +
		"      value: 12.5pt\n" +
		"    token @alias:\n" +
		"      reference: \"brand\"\n" +
		"      type: \"color\"\n" +
		"    scope @line-items:\n" +
		"      token @brand:\n" +
		"        type: \"color\"\n" +
		"        value: \"#112233\"\n" +
		"      scope @compact:\n" +
		"        token @enabled:\n" +
		"          type: \"bool\"\n" +
		"          value: true\n" +
		"  theme @base:\n"
	if string(formatted) != want {
		t.Fatalf("formatted =\n%s\nwant:\n%s", formatted, want)
	}
	reparsed := Parse("formatted-theme.paper", string(formatted))
	if !reparsed.OK() || !semanticASTEqual(parsed.AST, reparsed.AST) {
		t.Fatalf("reparse diagnostics/AST = %#v / %#v", reparsed.Diagnostics, reparsed.AST)
	}
	formattedAgain, err := Format(reparsed.AST)
	if err != nil || !bytes.Equal(formattedAgain, formatted) {
		t.Fatalf("second Format() = %s, %v", formattedAgain, err)
	}
}

func TestThemeDeclarationWordsRemainContextualProperties(t *testing.T) {
	source := "document:\n  theme: \"ordinary\"\n  token: 2\n  scope: true\n  page:\n    body:\n      text: \"ok\"\n"
	parsed := Parse("contextual-theme.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	for index := 0; index < 3; index++ {
		if parsed.AST.Root.Members[index].Property == nil {
			t.Fatalf("member %d parsed as declaration: %#v", index, parsed.AST.Root.Members[index])
		}
	}
	if _, err := Format(parsed.AST); err != nil {
		t.Fatalf("Format() rejected contextual properties: %v", err)
	}
}

func TestThemeGrammarRejectsInvalidHierarchy(t *testing.T) {
	source := "document:\n  theme @bad:\n    page:\n      body:\n        text: \"no\"\n  page:\n    body:\n      token @orphan:\n        type: \"number\"\n        value: 1\n"
	parsed := Parse("bad-theme.paper", source)
	if parsed.OK() || !diagnosticCodes(parsed.Diagnostics)["PAPER_INVALID_CHILD"] {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
}
