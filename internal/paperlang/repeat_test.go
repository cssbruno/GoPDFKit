// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestRepeatSyntaxParseFormatParseIsStable(t *testing.T) {
	t.Parallel()

	source := "document @invoice:\n" +
		"    page:\n" +
		"        body:\n" +
		"            repeat @visible-lines:\n" +
		"                when: \"active && quantity == 1\"\n" +
		"                max-items: 50\n" +
		"                source: \"@invoice.items\"\n" +
		"                instance-prefix: \"invoice-lines\"\n" +
		"                paragraph @line:\n" +
		"                    text: \"Line\"\n"
	parsed := Parse("repeat.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	want := "document @invoice:\n" +
		"  page:\n" +
		"    body:\n" +
		"      repeat @visible-lines:\n" +
		"        instance-prefix: \"invoice-lines\"\n" +
		"        max-items: 50\n" +
		"        source: \"@invoice.items\"\n" +
		"        when: \"active && quantity == 1\"\n" +
		"        paragraph @line:\n" +
		"          text: \"Line\"\n"
	if string(formatted) != want {
		t.Fatalf("formatted =\n%s\nwant:\n%s", formatted, want)
	}
	reparsed := Parse("repeat-formatted.paper", string(formatted))
	if !reparsed.OK() {
		t.Fatalf("reparse diagnostics = %#v", reparsed.Diagnostics)
	}
	again, err := Format(reparsed.AST)
	if err != nil || !bytes.Equal(formatted, again) {
		t.Fatalf("format stability = %q, %v", again, err)
	}
}

func TestRepeatSyntaxRequiresNameAndTemplateHierarchy(t *testing.T) {
	t.Parallel()

	source := "document:\n  page:\n    body:\n      repeat:\n        source: \"@s.items\"\n        paragraph:\n          text: \"a\"\n        paragraph:\n          text: \"b\"\n"
	parsed := Parse("bad-repeat.paper", source)
	if parsed.OK() || !diagnosticCodes(parsed.Diagnostics)["PAPER_REPEAT_NAME"] {
		t.Fatalf("diagnostics = %#v", parsed.Diagnostics)
	}
}
