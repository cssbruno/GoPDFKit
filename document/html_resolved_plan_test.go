// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
)

func TestHTMLUnifiedResolvedCSSCascadeInheritanceAndSelectorFreeBoundary(t *testing.T) {
	compiled, err := CompileHTML(`<style>
		p { color: red; font-family: serif; font-size: 9pt; line-height: 11pt }
		.wrap p.note { color: #123456 }
		#target { color: #abcdef }
	</style><main class="wrap"><p class="note" id="target">Alpha <span>Beta</span> Gamma</p></main>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 220, Ht: 140}), WithNoCompression())
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.cssRules) != 0 {
		t.Fatalf("selector rules crossed frontend boundary: %d", len(resolved.cssRules))
	}
	for _, token := range resolved.tokens {
		if token.Cat == 'O' && (token.Attr["class"] != "" || token.Attr["id"] != "" || token.Attr["style"] != "") {
			t.Fatalf("selector/style attributes crossed frontend boundary: %#v", token)
		}
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	paragraph := model.Body[0].(layout.ParagraphBlock)
	if paragraph.Style.FontFamily != "Times" || paragraph.Style.FontSize != 9 || paragraph.Style.LineHeight != 11 ||
		!paragraph.Style.Color.Set || paragraph.Style.Color.R != 0xab || paragraph.Style.Color.G != 0xcd || paragraph.Style.Color.B != 0xef {
		t.Fatalf("resolved paragraph style = %#v", paragraph.Style)
	}
	if len(paragraph.Segments) != 1 || paragraph.Segments[0].Text != "Alpha Beta Gamma" {
		t.Fatalf("resolved inline segments = %#v", paragraph.Segments)
	}
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("plan = %#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.GlyphRuns) == 0 {
		t.Fatal("resolved CSS plan has no glyph runs")
	}
	foundColor := false
	for _, run := range projection.GlyphRuns {
		foundColor = foundColor || run.Color.Set && run.Color.R == 0xab && run.Color.G == 0xcd && run.Color.B == 0xef
	}
	if !foundColor {
		t.Fatal("resolved display style lacks cascaded color")
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages == 0 {
		t.Fatalf("write pages=%d err=%v", pages, err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 || !bytes.Contains(output.Bytes(), []byte(" rg")) {
		t.Fatalf("resolved CSS PDF bytes=%d lacks color command", output.Len())
	}
}

func TestHTMLCSSCascadeScanFallback(t *testing.T) {
	el := HTMLSegmentType{Str: "span", Attr: map[string]string{"class": "note"}}
	ancestors := []HTMLSegmentType{{Str: "section", Attr: map[string]string{"class": "body"}}}
	rules := []htmlCSSRule{
		{selectors: parseHTMLCSSSelectors("span"), declarations: map[string]string{"color": "red"}},
		{selectors: parseHTMLCSSSelectors("section > span.note"), declarations: map[string]string{"font-weight": "bold"}},
	}
	declarations := htmlElementDeclarationsWithStyle(el, rules, map[string]string{"font-size": "12pt"}, ancestors...)
	if declarations["color"] != "red" || declarations["font-weight"] != "bold" || declarations["font-size"] != "12pt" {
		t.Fatalf("fallback declarations = %#v, want matched rules plus inline style", declarations)
	}
}

func TestHTMLUnifiedHeadingDefaultsMatchBrowserUserAgentSubset(t *testing.T) {
	compiled, err := CompileHTML(`<h1>Title</h1><h2>Subhead</h2>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 220, Ht: 140}), WithNoCompression())
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	var headings []htmlUnifiedResolvedElement
	for index, token := range resolved.tokens {
		if token.Cat == 'O' && (token.Str == "h1" || token.Str == "h2") {
			headings = append(headings, resolved.unifiedResolved[index])
		}
	}
	if len(headings) != 2 {
		t.Fatalf("resolved headings = %d", len(headings))
	}
	if headings[0].text.FontSize != 24 || !headings[0].text.Bold || !almostEqual(headings[0].text.LineHeight, 28.8) || !almostEqual(headings[0].box.Margin.Top, 16.08) {
		t.Fatalf("h1 defaults = %+v", headings[0])
	}
	if headings[1].text.FontSize != 18 || !headings[1].text.Bold || !almostEqual(headings[1].box.Margin.Top, 14.94) {
		t.Fatalf("h2 defaults = %+v", headings[1])
	}
}

func TestHTMLUnifiedResolvedTableBoxSubsetFromSelectors(t *testing.T) {
	compiled, err := CompileHTML(`<style>
		table.report { background-color:#eeeeee; border-collapse:collapse }
		table.report td.value { background-color:#102030; padding:2pt; border:1pt solid #405060; text-align:right; vertical-align:bottom }
	</style><table class="report"><tbody><tr><td class="value">42</td></tr></tbody></table>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := typedTableTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	table := model.Body[0].(layout.TableBlock)
	cell := table.Body[0].Cells[0]
	if !table.Style.BorderCollapse || !table.Box.BackgroundColor.Set || !cell.Box.BackgroundColor.Set ||
		cell.Box.Padding != (layout.Spacing{Top: 2, Right: 2, Bottom: 2, Left: 2}) || cell.Box.Border.Top.Width != 1 || cell.Align != "right" || cell.VerticalAlign != "bottom" {
		t.Fatalf("resolved table/cell box = %#v / %#v", table, cell)
	}
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fills) < 1 || len(projection.Strokes) < 4 {
		t.Fatalf("box display fills=%d strokes=%d", len(projection.Fills), len(projection.Strokes))
	}
}

func TestHTMLUnifiedCapabilityScanRejectsWholeFragmentBeforePlanning(t *testing.T) {
	compiled, err := CompileHTML(`<p class="ok">Supported first</p><style>.bad{float:left}</style><p class="bad">Unsupported later</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint))
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if !errors.Is(err, ErrHTMLPlanUnsupported) || !strings.Contains(err.Error(), `resolved CSS property "float"`) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("atomic capability scan plan=%#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
}

func TestHTMLUnifiedResolvedWhitespaceAndBenchmarkCohort(t *testing.T) {
	compiled, err := CompileHTML(`<style>.code{white-space:pre;font-family:monospace}</style><p class="code">A  B
 C</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := paperEngineBenchmarkDocument()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	paragraph := model.Body[0].(layout.ParagraphBlock)
	if got := layout.TextSegmentsPlainText(paragraph.Segments); got != "A  B\n C" {
		t.Fatalf("preserved whitespace = %q", got)
	}

	benchmarkCompiled, err := CompileHTML(paperEngineBenchmarkHTMLFixture())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := paperEngineBenchmarkDocument().PlanCompiledHTML(12, benchmarkCompiled)
	if err != nil || plan.PageCount() == 0 {
		t.Fatalf("benchmark cohort pages=%d err=%v", plan.PageCount(), err)
	}
}
