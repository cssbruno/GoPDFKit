// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const paperExplicitBreakFixture = "document:\n" +
	"  page:\n" +
	"    width: 90pt\n" +
	"    height: 90pt\n" +
	"    margin: 6pt\n" +
	"    body:\n" +
	"      page-break:\n" +
	"      text: \"A\"\n" +
	"      page-break @first:\n" +
	"      page-break @duplicate:\n" +
	"      text: \"B\"\n" +
	"      page-break:\n"

func TestPaperPlannerHonorsExplicitBreakWithoutBlankPages(t *testing.T) {
	parsed := paperlang.Parse("explicit.paper", paperExplicitBreakFixture)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %+v", parsed.Diagnostics)
	}
	compiled := papercompile.Compile(parsed.AST)
	if !compiled.OK() {
		t.Fatalf("Compile() diagnostics = %+v", compiled.Diagnostics)
	}
	planner, err := newPaperPlanner(compiled.Page)
	if err != nil {
		t.Fatalf("newPaperPlanner() = %v", err)
	}
	plan, err := planner.planPaperTextBlocks(compiled.Document)
	if err != nil {
		t.Fatalf("planPaperTextBlocks() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 2 || len(projection.Fragments) != 2 || len(projection.Breaks) != 1 {
		t.Fatalf("plan pages/fragments/breaks = %d/%d/%+v", len(projection.Pages), len(projection.Fragments), projection.Breaks)
	}
	decision := projection.Breaks[0]
	if decision.Reason != layoutengine.BreakExplicitPageBreak || decision.FromPage != 1 || decision.ToPage != 2 ||
		decision.Preceding != projection.Fragments[0].ID || decision.Triggering != projection.Fragments[1].ID ||
		decision.Required != 0 || decision.Available != 0 {
		t.Fatalf("explicit break decision = %+v", decision)
	}
}

func TestWritePaperPaintsExplicitBreakFromTheCompletedPlan(t *testing.T) {
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	result, err := target.WritePaper("explicit.paper", paperExplicitBreakFixture)
	if err != nil || !result.OK() || result.Pages != 2 || target.PageCount() != 2 {
		t.Fatalf("WritePaper() = %#v, %v; pages=%d", result, err, target.PageCount())
	}
	if !bytes.Contains(target.pages[1].Bytes(), []byte("(A) Tj")) ||
		bytes.Contains(target.pages[1].Bytes(), []byte("(B) Tj")) ||
		!bytes.Contains(target.pages[2].Bytes(), []byte("(B) Tj")) {
		t.Fatalf("explicit break did not preserve source allocation:\npage 1 %s\npage 2 %s", target.pages[1].Bytes(), target.pages[2].Bytes())
	}
}
