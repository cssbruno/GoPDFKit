// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"strings"
	"testing"
)

func TestPageBreakIsAReadableBodyLeafAndFormatsCanonically(t *testing.T) {
	source := "document @doc:\n  page:\n    body:\n      text: \"before\"\n      page-break @chapter-two:\n      text: \"after\"\n"
	parsed := Parse("break.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	body := parsed.AST.Root.Members[0].Node.Members[0].Node
	if len(body.Members) != 3 || body.Members[1].Node == nil ||
		body.Members[1].Node.Kind != NodePageBreak || body.Members[1].Node.ID != "@chapter-two" {
		t.Fatalf("body members = %#v", body.Members)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	if string(formatted) != source {
		t.Fatalf("Format() =\n%s\nwant:\n%s", formatted, source)
	}
}

func TestPageBreakRejectsValuesAndIndentedContent(t *testing.T) {
	for _, source := range []string{
		"document:\n  page:\n    body:\n      page-break: true\n",
		"document:\n  page:\n    body:\n      page-break:\n        note: true\n",
	} {
		parsed := Parse("invalid-break.paper", source)
		if parsed.OK() {
			t.Fatalf("Parse(%q) unexpectedly succeeded", source)
		}
		found := false
		for _, diagnostic := range parsed.Diagnostics {
			if strings.HasPrefix(diagnostic.Code, "PAPER_PAGE_BREAK") || diagnostic.Code == "PAPER_NODE_VALUE" {
				found = true
			}
		}
		if !found {
			t.Fatalf("diagnostics = %+v", parsed.Diagnostics)
		}
	}
}
