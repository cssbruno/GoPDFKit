// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestScenarioSyntaxParseFormatParseIsStable(t *testing.T) {
	source := "document @invoice:\n" +
		"    scenario @child:\n" +
		"        parent: \"@base\"\n" +
		"        locale: \"pt-BR\"\n" +
		"        value @paid: true\n" +
		"        value @note: null\n" +
		"        object @customer:\n" +
		"            value @name: \"Ada\"\n" +
		"            value @score: 12.50\n" +
		"        keyed-list @lines:\n" +
		"            object @line-a:\n" +
		"                value @sku: \"A-1\"\n" +
		"                value @quantity: 2\n" +
		"            value @summary: \"one line\"\n" +
		"    scenario @base:\n"
	parsed := Parse("scenarios.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	want := "document @invoice:\n" +
		"  scenario @child:\n" +
		"    locale: \"pt-BR\"\n" +
		"    parent: \"@base\"\n" +
		"    value @paid: true\n" +
		"    value @note: null\n" +
		"    object @customer:\n" +
		"      value @name: \"Ada\"\n" +
		"      value @score: 12.5\n" +
		"    keyed-list @lines:\n" +
		"      object @line-a:\n" +
		"        value @sku: \"A-1\"\n" +
		"        value @quantity: 2\n" +
		"      value @summary: \"one line\"\n" +
		"  scenario @base:\n"
	if string(formatted) != want {
		t.Fatalf("formatted =\n%s\nwant:\n%s", formatted, want)
	}
	reparsed := Parse("formatted.paper", string(formatted))
	if !reparsed.OK() {
		t.Fatalf("formatted Parse() diagnostics = %#v", reparsed.Diagnostics)
	}
	formattedAgain, err := Format(reparsed.AST)
	if err != nil || !bytes.Equal(formattedAgain, formatted) {
		t.Fatalf("second Format() = %s, %v", formattedAgain, err)
	}
	if !semanticASTEqual(parsed.AST, reparsed.AST) {
		t.Fatal("semantic AST changed across scenario formatting")
	}
}

func TestScenarioFixtureWordsRemainContextualProperties(t *testing.T) {
	source := "document:\n  value: \"ordinary property\"\n  object: true\n  page:\n    body:\n      text: \"ok\"\n"
	parsed := Parse("contextual.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	if parsed.AST.Root.Members[0].Property == nil || parsed.AST.Root.Members[1].Property == nil {
		t.Fatalf("contextual names parsed as nodes: %#v", parsed.AST.Root.Members[:2])
	}
	if _, err := Format(parsed.AST); err != nil {
		t.Fatalf("Format() rejected contextual properties: %v", err)
	}
}

func TestScenarioGrammarRejectsInvalidHierarchyAndMissingFixtureName(t *testing.T) {
	source := "document:\n  scenario @bad:\n    page:\n      body:\n        text: \"no\"\n    value:\n"
	parsed := Parse("bad-scenario.paper", source)
	if parsed.OK() {
		t.Fatal("invalid scenario syntax unexpectedly parsed")
	}
	codes := diagnosticCodes(parsed.Diagnostics)
	if !codes["PAPER_INVALID_CHILD"] || !codes["PAPER_EXPECTED_VALUE"] {
		t.Fatalf("diagnostics = %#v", parsed.Diagnostics)
	}
}
