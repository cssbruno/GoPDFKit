// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"testing"
)

func TestLoopSyntaxParseFormatParseIsStableAndLossless(t *testing.T) {
	t.Parallel()
	const source = "# bounded range\r\ndocument:\r\n  page:\r\n    body:\r\n      loop @copies: # stable value identity\r\n        through: 3\r\n        from: 1\r\n        max-iterations: 3\r\n        step: 1\r\n        when: \"enabled && (loop.first || loop.last)\"\r\n        instance-prefix: \"copies\"\r\n        paragraph @copy:\r\n          text: \"Copy\"\r\n"
	parsed := ParseLossless("loop.paper", source)
	if !parsed.OK() || !parsed.Semantic.OK() {
		t.Fatalf("parse = %v / %+v", parsed.Err, parsed.Semantic.Diagnostics)
	}
	printed, err := PrintLossless(parsed.CST)
	if err != nil || string(printed) != source {
		t.Fatalf("lossless = %q, %v", printed, err)
	}
	formatted, err := Format(parsed.Semantic.AST)
	if err != nil {
		t.Fatal(err)
	}
	reparsed := Parse("loop.paper", string(formatted))
	again, secondErr := Format(reparsed.AST)
	if !reparsed.OK() || secondErr != nil || !bytes.Equal(formatted, again) {
		t.Fatalf("round trip = %+v / %v\n%s", reparsed.Diagnostics, secondErr, formatted)
	}
}

func TestLoopSyntaxRequiresNameAndOneTemplate(t *testing.T) {
	t.Parallel()
	parsed := Parse("bad-loop.paper", "document:\n  page:\n    body:\n      loop:\n        from: 1\n        through: 2\n        step: 1\n        max-iterations: 2\n        instance-prefix: \"x\"\n")
	if parsed.OK() || !diagnosticCodes(parsed.Diagnostics)["PAPER_LOOP_NAME"] {
		t.Fatalf("diagnostics = %+v", parsed.Diagnostics)
	}
}
