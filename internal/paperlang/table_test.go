// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestTableSyntaxIsLosslessAndFormatsStably(t *testing.T) {
	const source = "# retained table note\n" +
		"document @report:\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      table @ledger:\n" +
		"        repeat-header: true\n" +
		"        table-track @name:\n" +
		"          width: 90pt\n" +
		"        table-header @head:\n" +
		"          table-row @head-row:\n" +
		"            cell @head-cell:\n" +
		"              text: \"Name\"\n" +
		"        table-row @row:\n" +
		"          cell @cell:\n" +
		"            paragraph:\n" +
		"              text: \"Alpha\"\n"
	parsed := ParseLossless("table.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Semantic.Diagnostics)
	}
	lossless, err := PrintLossless(parsed.CST)
	if err != nil || !bytes.Equal(lossless, []byte(source)) {
		t.Fatalf("lossless table = %q, %v", lossless, err)
	}
	formatted, err := Format(parsed.Semantic.AST)
	if err != nil {
		t.Fatal(err)
	}
	reparsed := Parse("table.paper", string(formatted))
	second, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(second, formatted) {
		t.Fatalf("table format round trip = %#v, %v\nfirst:\n%s\nsecond:\n%s", reparsed.Diagnostics, secondErr, formatted, second)
	}
}
