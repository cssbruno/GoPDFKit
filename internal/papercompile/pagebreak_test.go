// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/layout"
)

func TestCompileLowersPageBreakInBodySourceOrder(t *testing.T) {
	parsed := paperlang.Parse("break.paper", "document:\n  page:\n    body:\n      text: \"before\"\n      page-break @next:\n      text: \"after\"\n")
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	compiled := Compile(parsed.AST)
	if !compiled.OK() {
		t.Fatalf("Compile() diagnostics = %+v", compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 3 {
		t.Fatalf("body length = %d, want 3", len(compiled.Document.Body))
	}
	pageBreak, ok := compiled.Document.Body[1].(layout.PageBreakBlock)
	if !ok || !pageBreak.After {
		t.Fatalf("body[1] = %#v, want explicit PageBreakBlock", compiled.Document.Body[1])
	}
	found := false
	for _, mapping := range compiled.Mapping.Nodes {
		if mapping.ID == "@next" && mapping.Kind == paperlang.NodePageBreak && mapping.BodyIndex == 1 && mapping.SegmentIndex == -1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("mapping = %+v", compiled.Mapping.Nodes)
	}
}
