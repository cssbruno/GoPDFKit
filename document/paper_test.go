// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const paperPipelineFixture = "document @report:\n" +
	"  title: \"Atomic paper\"\n" +
	"  language: \"en\"\n" +
	"  page @sheet:\n" +
	"    width: 72pt\n" +
	"    height: 50pt\n" +
	"    margin: 8pt\n" +
	"    body @content:\n" +
	"      paragraph @message:\n" +
	"        font: \"Courier\"\n" +
	"        size: 10pt\n" +
	"        line-height: 8pt\n" +
	"        text @copy: \"A\\nB\\nC\\nD\\nE\"\n"

func TestWritePaperRunsParseCompilePlanAndCorePaintWithoutHTML(t *testing.T) {
	target := MustNew(WithUnit(UnitMillimeter), WithNoCompression())
	result, err := target.WritePaper("report.paper", paperPipelineFixture)
	if err != nil || !result.OK() {
		t.Fatalf("WritePaper() = %#v, %v", result, err)
	}
	if result.Pages != 2 || target.PageCount() != 2 || len(result.Diagnostics) != 0 {
		t.Fatalf("render result/pages = %#v / %d", result, target.PageCount())
	}
	for page := 1; page <= target.PageCount(); page++ {
		width, height, _ := target.PageSize(page)
		if got := target.UnitToPointConvert(width); got != 72 {
			t.Fatalf("page %d width = %gpt, want 72pt", page, got)
		}
		if got := target.UnitToPointConvert(height); got != 50 {
			t.Fatalf("page %d height = %gpt, want 50pt", page, got)
		}
		content := target.pages[page].Bytes()
		if !bytes.Contains(content, []byte(" Tm (")) || !bytes.Contains(content, []byte(") Tj")) {
			t.Fatalf("page %d lacks direct planned text operators:\n%s", page, content)
		}
		if bytes.Contains(content, []byte(" Tw")) || bytes.Contains(content, []byte(" Td")) {
			t.Fatalf("page %d used a live text layout operator:\n%s", page, content)
		}
	}
	if target.title == "" || target.compliance.Lang != "en" {
		t.Fatalf("compiled metadata was not committed: title %q lang %q", target.title, target.compliance.Lang)
	}
}

func TestWritePaperFlowsMultipleStyledBlocksInSourceOrderThroughOnePlan(t *testing.T) {
	source := "document @report:\n" +
		"  page @sheet:\n" +
		"    width: 90pt\n" +
		"    height: 52pt\n" +
		"    margin: 6pt\n" +
		"    body @content:\n" +
		"      heading @first:\n" +
		"        level: 1\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 12pt\n" +
		"        line-height: 12pt\n" +
		"        bold: true\n" +
		"        text: \"H\"\n" +
		"      paragraph @second:\n" +
		"        font: \"Courier\"\n" +
		"        size: 10pt\n" +
		"        line-height: 10pt\n" +
		"        text: \"A\\nB\"\n" +
		"      heading @third:\n" +
		"        level: 2\n" +
		"        font: \"Times\"\n" +
		"        size: 11pt\n" +
		"        line-height: 12pt\n" +
		"        italic: true\n" +
		"        text: \"T\"\n" +
		"      paragraph @fourth:\n" +
		"        font: \"Courier\"\n" +
		"        size: 10pt\n" +
		"        line-height: 10pt\n" +
		"        text: \"C\\nD\"\n"

	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	result, err := target.WritePaper("blocks.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("WritePaper() = %#v, %v", result, err)
	}
	if result.Pages != 2 || target.PageCount() != 2 {
		t.Fatalf("pages = %#v / %d, want two", result, target.PageCount())
	}
	first := target.pages[1].Bytes()
	second := target.pages[2].Bytes()
	assertPaperCodesInOrder(t, first, 'H', 'A', 'B')
	assertPaperCodesInOrder(t, second, 'T', 'C', 'D')
	if bytes.Contains(first, []byte("(T) Tj")) || bytes.Contains(second, []byte("(B) Tj")) {
		t.Fatalf("blocks were allocated to the wrong page:\npage 1 %s\npage 2 %s", first, second)
	}
	for _, key := range []string{"helveticaB", "courier", "timesI"} {
		if _, exists := target.resources.font(key); !exists {
			t.Fatalf("compiled core-font style %q was not installed", key)
		}
	}
	for _, expected := range []struct {
		page []byte
		size string
	}{
		{first, "12.0000000000 Tf"}, {first, "10.0000000000 Tf"},
		{second, "11.0000000000 Tf"}, {second, "10.0000000000 Tf"},
	} {
		if !bytes.Contains(expected.page, []byte(expected.size)) {
			t.Fatalf("page lacks compiled font size %q:\n%s", expected.size, expected.page)
		}
	}
}

func TestWritePaperPlansStyledListsWithMarkersInSourceOrderAcrossPages(t *testing.T) {
	source := "document @report:\n" +
		"  page:\n" +
		"    width: 100pt\n" +
		"    height: 40pt\n" +
		"    margin: 5pt\n" +
		"    body:\n" +
		"      list @ordered:\n" +
		"        ordered: true\n" +
		"        marker: \"decimal\"\n" +
		"        font: \"Courier\"\n" +
		"        size: 10pt\n" +
		"        line-height: 12pt\n" +
		"        bold: true\n" +
		"        item @alpha:\n" +
		"          text: \"Alpha\"\n" +
		"        item @beta:\n" +
		"          text: \"Beta\"\n" +
		"        item @gamma:\n" +
		"          text: \"Gamma\"\n" +
		"      list @unordered:\n" +
		"        marker: \"asterisk\"\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 9pt\n" +
		"        line-height: 12pt\n" +
		"        italic: true\n" +
		"        item @delta:\n" +
		"          text: \"Delta\"\n" +
		"        item @echo:\n" +
		"          text: \"Echo\"\n"

	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	result, err := target.WritePaper("lists.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("WritePaper() = %#v, %v", result, err)
	}
	if result.Pages != 3 || target.PageCount() != 3 {
		t.Fatalf("pages = %#v / %d, want three", result, target.PageCount())
	}
	assertPaperCodesInOrder(t, target.pages[1].Bytes(), '1', 'A', '2', 'B')
	assertPaperCodesInOrder(t, target.pages[2].Bytes(), '3', 'G', '*', 'D')
	assertPaperCodesInOrder(t, target.pages[3].Bytes(), '*', 'E')
	if bytes.Contains(target.pages[1].Bytes(), []byte("(3) Tj")) ||
		bytes.Contains(target.pages[2].Bytes(), []byte("(E) Tj")) {
		t.Fatalf("list items crossed their planned page boundary:\npage1 %s\npage2 %s", target.pages[1].Bytes(), target.pages[2].Bytes())
	}
	for _, key := range []string{"courierB", "helveticaI"} {
		if _, exists := target.resources.font(key); !exists {
			t.Fatalf("list style core font %q was not installed", key)
		}
	}
	for page := 1; page <= target.PageCount(); page++ {
		content := target.pages[page].Bytes()
		if bytes.Contains(content, []byte(" Tw")) || bytes.Contains(content, []byte(" Td")) {
			t.Fatalf("page %d used live text layout operators:\n%s", page, content)
		}
	}
}

func TestPaperListMarkersArePartOfTheGlyphPlan(t *testing.T) {
	source := "document:\n  page:\n    width: 200pt\n    height: 100pt\n    margin: 5pt\n    body:\n      list:\n        marker: \"dash\"\n        font: \"Courier\"\n        size: 10pt\n        line-height: 12pt\n        item:\n          text: \"First\"\n        item:\n          text: \"Second\"\n"
	parsed := paperlang.Parse("plan-list.paper", source)
	compiled := papercompile.Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("parse/compile diagnostics = %#v / %#v", parsed.Diagnostics, compiled.Diagnostics)
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
	if len(projection.GlyphRuns) != 2 || projection.GlyphRuns[0].Codes != "- First" || projection.GlyphRuns[1].Codes != "- Second" {
		t.Fatalf("planned list glyph runs = %#v", projection.GlyphRuns)
	}
}

func TestPaperPlannerRecordsCausalPageBreaks(t *testing.T) {
	parsed := paperlang.Parse("breaks.paper", paperPipelineFixture)
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
	breaks := plan.Projection().Breaks
	if len(breaks) != 1 {
		t.Fatalf("breaks = %+v, want one", breaks)
	}
	decision := breaks[0]
	if decision.Reason != layoutengine.BreakInsufficientRemainingBodySpace ||
		decision.FromPage != 1 || decision.ToPage != 2 || decision.Preceding == decision.Triggering ||
		decision.Required <= decision.Available {
		t.Fatalf("break decision = %+v", decision)
	}
}

func assertPaperCodesInOrder(t *testing.T, content []byte, codes ...byte) {
	t.Helper()
	previous := -1
	for _, code := range codes {
		needle := []byte{'(', code, ')', ' ', 'T', 'j'}
		index := bytes.Index(content, needle)
		if index < 0 || index <= previous {
			t.Fatalf("codes %q are absent or out of order in:\n%s", codes, content)
		}
		previous = index
	}
}

func TestWritePaperFailuresAreAtomicAndStructured(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		configure func(*Document)
		stage     PaperRenderStage
		code      string
	}{
		{
			name:   "parse",
			source: "document:\n  page\n",
			stage:  PaperStageParse,
			code:   "PAPER_EXPECTED_COLON",
		},
		{
			name:   "compile",
			source: "document:\n  page:\n    mystery: true\n    body:\n      text: \"ok\"\n",
			stage:  PaperStageCompile,
			code:   "PAPER_COMPILE_UNSUPPORTED_PROPERTY",
		},
		{
			name:   "plan",
			source: "document:\n  page:\n    body:\n      paragraph:\n        text: \"non-ASCII: é\"\n",
			stage:  PaperStagePlan,
			code:   "PAPER_PLAN_UNSUPPORTED",
		},
		{
			name:   "paint preflight",
			source: paperPipelineFixture,
			configure: func(pdf *Document) {
				pdf.limits.MaxPages = 1
			},
			stage: PaperStagePaint,
			code:  "PAPER_PAINT_PREFLIGHT",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := MustNew(WithUnit(UnitPoint), WithNoCompression())
			if test.configure != nil {
				test.configure(target)
			}
			target.title = "before"
			target.compliance.Lang = "pt-BR"
			before := paperAtomicSnapshotOf(target)

			result, err := target.WritePaper("failure.paper", test.source)
			if !errors.Is(err, ErrPaperRender) || result.OK() || result.Pages != 0 {
				t.Fatalf("WritePaper() = %#v, %v", result, err)
			}
			if after := paperAtomicSnapshotOf(target); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed pipeline mutated target:\nbefore %#v\nafter  %#v", before, after)
			}
			if len(result.Diagnostics) == 0 {
				t.Fatal("failed pipeline returned no structured diagnostic")
			}
			found := false
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Stage == test.stage && diagnostic.Code == test.code &&
					diagnostic.Severity == "error" && diagnostic.File != "" {
					found = true
				}
			}
			if !found {
				t.Fatalf("diagnostics = %#v, want stage %q code %q", result.Diagnostics, test.stage, test.code)
			}
		})
	}
}

type paperAtomicSnapshot struct {
	typedShadowSnapshot
	pageBuffers int
	pageSizes   int
	pageLinks   int
	title       string
	language    string
}

func paperAtomicSnapshotOf(pdf *Document) paperAtomicSnapshot {
	return paperAtomicSnapshot{
		typedShadowSnapshot: typedShadowSnapshotOf(pdf),
		pageBuffers:         len(pdf.pages),
		pageSizes:           len(pdf.pageSizes),
		pageLinks:           len(pdf.pageLinks),
		title:               pdf.title,
		language:            pdf.compliance.Lang,
	}
}
