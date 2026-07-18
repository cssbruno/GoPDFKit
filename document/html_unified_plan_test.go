// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompiledHTMLUnifiedPlanLowersTextHeadingsBreaksAndFlatLists(t *testing.T) {
	compiled, err := CompileHTML("  <h2>Unified</h2><p>Hello <span>exact</span><br>planner</p><ol><li>First</li><li>Second</li></ol><ul><li>Third</li></ul>")
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), compiled, 14)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 4 {
		t.Fatalf("lowered blocks = %d, want 4", len(model.Body))
	}
	heading, ok := model.Body[0].(layout.HeadingBlock)
	if !ok || heading.Level != 2 || heading.Segments[0].Text != "Unified" {
		t.Fatalf("heading = %#v", model.Body[0])
	}
	paragraph, ok := model.Body[1].(layout.ParagraphBlock)
	if !ok || paragraph.Segments[0].Text != "Hello exact\nplanner" {
		t.Fatalf("paragraph = %#v", model.Body[1])
	}
	ordered, ok := model.Body[2].(layout.ListBlock)
	if !ok || !ordered.Ordered || ordered.MarkerStyle != "decimal" || len(ordered.Items) != 2 {
		t.Fatalf("ordered list = %#v", model.Body[2])
	}
	unordered, ok := model.Body[3].(layout.ListBlock)
	if !ok || unordered.Ordered || unordered.MarkerStyle != "dash" || len(unordered.Items) != 1 {
		t.Fatalf("unordered list = %#v", model.Body[3])
	}

	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 220, Ht: 180}), WithNoCompression())
	planner.SetMargins(18, 18, 18)
	planner.SetAutoPageBreak(true, 18)
	plan, err := planner.PlanCompiledHTML(14, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() == 0 || planner.PageCount() != 0 {
		t.Fatalf("PlanCompiledHTML() = %#v, source pages %d, %v", plan, planner.PageCount(), err)
	}
	var text strings.Builder
	for _, run := range plan.plan.Projection().GlyphRuns {
		text.WriteString(run.Codes)
		text.WriteByte('\n')
	}
	for _, want := range []string{"Unified", "Hello exact", "planner", "1. First", "2. Second", "- Third"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("planned text lacks %q:\n%s", want, text.String())
		}
	}
}

func TestCompiledHTMLUnifiedPlanPrunesDisplayNoneSubtrees(t *testing.T) {
	compiled, err := CompileHTML(`<main><p>before <span style="display:none">hidden inline</span>after</p><div style="display:none"><p>hidden block</p><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=" width="1" height="1"></div><p style="position:static;float:none;clear:both">visible</p></main>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}), WithNoCompression())
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 2 {
		t.Fatalf("display:none body blocks = %d, want 2", len(model.Body))
	}
	first, ok := model.Body[0].(layout.ParagraphBlock)
	if !ok || layout.TextSegmentsPlainText(first.Segments) != "before after" {
		t.Fatalf("display:none inline text = %#v", model.Body[0])
	}
	second, ok := model.Body[1].(layout.ParagraphBlock)
	if !ok || layout.TextSegmentsPlainText(second.Segments) != "visible" {
		t.Fatalf("normal-flow defaults = %#v", model.Body[1])
	}
	for _, block := range model.Body {
		switch value := block.(type) {
		case layout.ParagraphBlock:
			if strings.Contains(layout.TextSegmentsPlainText(value.Segments), "hidden") {
				t.Fatalf("suppressed text leaked into model: %#v", block)
			}
		}
	}

	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() == 0 || planner.PageCount() != 0 {
		t.Fatalf("display:none plan = %#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
	var plannedText strings.Builder
	for _, run := range plan.plan.Projection().GlyphRuns {
		plannedText.WriteString(run.Codes)
	}
	if strings.Contains(plannedText.String(), "hidden") || !strings.Contains(plannedText.String(), "visible") {
		t.Fatalf("display:none planned text = %q", plannedText.String())
	}
}

func TestCompiledHTMLUnifiedPlanAppliesInheritedTextTransform(t *testing.T) {
	compiled, err := CompileHTML(`<div style="text-transform:uppercase"><p>hello <span style="text-transform:lowercase">WORLD</span> café</p><p style="text-transform:capitalize">second line</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 2 {
		t.Fatalf("transformed body blocks = %d, want 2", len(model.Body))
	}
	first := model.Body[0].(layout.ParagraphBlock)
	if got := layout.TextSegmentsPlainText(first.Segments); got != "HELLO world CAFÉ" {
		t.Fatalf("inherited/nested transform = %q, want %q", got, "HELLO world CAFÉ")
	}
	second := model.Body[1].(layout.ParagraphBlock)
	if got := layout.TextSegmentsPlainText(second.Segments); got != "Second Line" {
		t.Fatalf("capitalize transform = %q, want %q", got, "Second Line")
	}
}

func TestCompiledHTMLUnifiedPlanRejectsUnsupportedTextTransformAtomically(t *testing.T) {
	compiled, err := CompileHTML(`<p style="text-transform:full-width">text</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	if _, err := planner.PlanCompiledHTML(12, compiled); err == nil || !strings.Contains(err.Error(), `text-transform value "full-width" is unsupported`) {
		t.Fatalf("unsupported text-transform error = %v", err)
	}
}

func TestCompiledHTMLUnifiedPlanPrunesDisplayNoneFlexItems(t *testing.T) {
	compiled, err := CompileHTML(`<div style="display:flex;gap:4pt"><p style="display:none;flex:0 0 20pt;line-height:12pt">hidden</p><p style="flex:0 0 30pt;line-height:12pt">shown</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	model, err := func() (*layout.LayoutDocument, error) {
		resolved, resolveErr := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
		if resolveErr != nil {
			return nil, resolveErr
		}
		return lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	}()
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 1 {
		t.Fatalf("hidden flex item model blocks = %d", len(model.Body))
	}
	container, ok := model.Body[0].(layout.RowColumnBlock)
	if !ok || len(container.Items) != 1 {
		t.Fatalf("hidden flex item lowered model = %#v", model.Body[0])
	}
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() == 0 {
		t.Fatalf("hidden flex item plan=%#v pages=%d err=%v", plan, plan.PageCount(), err)
	}
	var text strings.Builder
	for _, run := range plan.plan.Projection().GlyphRuns {
		text.WriteString(run.Codes)
	}
	if text.String() != "shown" {
		t.Fatalf("hidden flex item text = %q", text.String())
	}
}

func TestCompiledHTMLUnifiedPlanAcceptsOrdinaryDisplayModes(t *testing.T) {
	compiled, err := CompileHTML(`<header><p>Header</p></header>` +
		`<span style="display:block;margin:2pt;padding:1pt;background:#eef4fa">Block span</span>` +
		`<span style="display:inline-block">Inline block span</span><footer><p>Footer</p></footer>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), mustResolveHTMLUnifiedForNestedTableTest(t, planner, compiled), 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 4 {
		t.Fatalf("ordinary display body = %#v", model.Body)
	}
	block, ok := model.Body[1].(layout.ParagraphBlock)
	if !ok || block.Segments[0].Text != "Block span" || block.Box.Margin.Top != 2 || block.Box.Padding.Top != 1 {
		t.Fatalf("block span = %#v", model.Body[1])
	}
	if plan.PageCount() != 1 || len(plan.plan.Projection().GlyphRuns) != 4 {
		t.Fatalf("ordinary display plan pages=%d runs=%d", plan.PageCount(), len(plan.plan.Projection().GlyphRuns))
	}
}

func TestCompiledHTMLUnifiedPlanLowersStrictTableThroughExactPlanner(t *testing.T) {
	compiled, err := CompileHTML(`<table><caption>Quarterly report</caption><thead><tr><th colspan="2">Heading</th></tr></thead><tbody>` +
		`<tr><th rowspan="2">North</th><td><a href="https://example.test/value">10</a></td></tr><tr><td>11</td></tr>` +
		`</tbody><tfoot><tr><th>Total</th><td>21</td></tr></tfoot></table>`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	table, ok := model.Body[0].(layout.TableBlock)
	if !ok || layout.TextSegmentsPlainText(table.CaptionSegments) != "Quarterly report" || len(table.Columns) != 2 || len(table.Header) != 1 || len(table.Body) != 2 || len(table.Footer) != 1 || !table.Style.RepeatHeader {
		t.Fatalf("lowered table = %#v", model.Body[0])
	}
	if !table.Header[0].Cells[0].Header || table.Header[0].Cells[0].ColSpan != 2 || !table.Body[0].Cells[0].Header || table.Body[0].Cells[0].RowSpan != 2 {
		t.Fatalf("lowered header/spans = %#v %#v", table.Header, table.Body)
	}
	planner := typedTableTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() == 0 || planner.PageCount() != 0 {
		t.Fatalf("planned table = %#v pages %d, %v", plan, planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Links) != 1 || projection.Links[0].URI != "https://example.test/value" {
		t.Fatalf("table links = %#v", projection.Links)
	}
	var captionCount int
	for _, run := range projection.GlyphRuns {
		if strings.Contains(run.Codes, "Quarterly report") {
			captionCount++
		}
	}
	if captionCount != 1 {
		t.Fatalf("caption runs = %d", captionCount)
	}
	var headers int
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleCell && node.Attributes.TableHeader {
			headers++
		}
	}
	if headers < 3 {
		t.Fatalf("semantic header cells = %d", headers)
	}
}

func TestCompiledHTMLUnifiedPlanRejectsUnsupportedTableContractsAtomically(t *testing.T) {
	tests := []string{
		`<table data-x="x"><tbody><tr><td>x</td></tr></tbody></table>`,
		`<table><tbody><tr><td colspan="0">x</td></tr></tbody></table>`,
		`<table><tbody><tr><td style="float:left">x</td></tr></tbody></table>`,
		`<table><tbody><tr><td rowspan="2">x</td></tr></tbody></table>`,
		`<table style="border-collapse:merge"><tbody><tr><td>x</td></tr></tbody></table>`,
		`<table><tbody><tr><td style="padding:1em">x</td></tr></tbody></table>`,
		`<table><tbody><tr><td style="border:1pt dashed red">x</td></tr></tbody></table>`,
		`<table><tbody><tr><td style="background-color:not-a-color">x</td></tr></tbody></table>`,
	}
	for _, source := range tests {
		compiled, err := CompileHTML(source)
		if err != nil {
			t.Fatal(err)
		}
		planner := typedTableTestPlanner()
		plan, err := planner.PlanCompiledHTML(12, compiled)
		if !errors.Is(err, ErrHTMLPlanUnsupported) || plan.Hash() != "" || planner.PageCount() != 0 {
			t.Fatalf("source %q: plan %#v pages %d err %v", source, plan, planner.PageCount(), err)
		}
	}
}

func TestCompiledHTMLUnifiedPlanKeepsEmptyTableCells(t *testing.T) {
	compiled, err := CompileHTML(`<table><tbody><tr><th></th><td>value</td></tr><tr><td>label</td><td></td></tr></tbody></table>`)
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
	table, ok := model.Body[0].(layout.TableBlock)
	if !ok || len(table.Body) != 2 || len(table.Body[0].Cells) != 2 || len(table.Body[1].Cells) != 2 ||
		len(table.Body[0].Cells[0].Blocks) != 0 || len(table.Body[1].Cells[1].Blocks) != 0 {
		t.Fatalf("empty-cell table = %#v", model.Body)
	}
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() != 1 || planner.PageCount() != 0 {
		t.Fatalf("empty-cell plan = %#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
	var cells int
	for _, node := range plan.plan.Projection().SemanticNodes {
		if node.Role == layoutengine.SemanticRoleCell {
			cells++
		}
	}
	if cells != 4 {
		t.Fatalf("semantic cells = %d, want 4", cells)
	}
}

func TestCompiledHTMLUnifiedPlanStrictTableDecorations(t *testing.T) {
	compiled, err := CompileHTML(`<table style="background-color:#eeeeee;border-collapse:collapse"><thead><tr><th style="background-color:#102030;padding:2pt;border:1px solid #405060;border-right:2pt solid red;text-align:right;vertical-align:bottom">Head</th></tr></thead><tbody><tr><td style="padding-left:4pt;border-top-width:1px;border-top-style:solid;border-top-color:blue">Body</td></tr></tbody></table>`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	table := model.Body[0].(layout.TableBlock)
	cell := table.Header[0].Cells[0]
	if !table.Style.BorderCollapse || !table.Box.BackgroundColor.Set || !cell.Box.BackgroundColor.Set || cell.Box.Padding.Left != 2 || cell.Box.Border.Top.Width != .75 || cell.Box.Border.Right.Width != 2 || cell.Align != "right" || cell.VerticalAlign != "bottom" {
		t.Fatalf("strict decorations=%#v %#v", table.Box, cell)
	}
	planner := typedTableTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("decorated HTML plan=%#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
	p := plan.plan.Projection()
	if len(p.Fills) < 2 || len(p.Strokes) < 4 {
		t.Fatalf("decorated resources fills=%d strokes=%d", len(p.Fills), len(p.Strokes))
	}
}

func TestCompiledHTMLUnifiedPlanRejectsWholeUnsupportedFragmentAtomically(t *testing.T) {
	tests := []struct {
		html string
		want string
	}{
		{"<p data-x=x>styled</p>", "attributes"},
		{"<style>p{float:left}</style><p>styled</p>", "resolved CSS property"},
		{"<p><a href='https://example.test' target='_blank'>linked</a></p>", "exactly one canonical href"},
		{"<table><tbody><tr><td><table><tbody><tr><td>nested</td></tr></tbody><tbody><tr><td>duplicate</td></tr></tbody></table></td></tr></tbody></table>", "duplicate table section"},
		{"<ul><li><ul><li>nested without owner text</li></ul></li></ul>", "leading item text"},
		{"<p>unclosed", "recovered HTML"},
	}
	for _, test := range tests {
		t.Run(test.want+test.html[:2], func(t *testing.T) {
			compiled, err := CompileHTML(test.html)
			if err != nil {
				t.Fatal(err)
			}
			planner := MustNew(WithUnit(UnitPoint))
			plan, err := planner.PlanCompiledHTML(12, compiled)
			if !errors.Is(err, ErrHTMLPlanUnsupported) || !strings.Contains(err.Error(), test.want) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("unsupported plan = %#v pages %d error %v", plan, planner.PageCount(), err)
			}
		})
	}
}

func TestCompiledHTMLUnifiedPlanCancellationAndInputValidation(t *testing.T) {
	compiled, err := CompileHTML("<p>content</p>")
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planner.PlanCompiledHTMLContext(canceled, 12, compiled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled plan = %v", err)
	}
	if _, err := planner.PlanCompiledHTML(0, compiled); err == nil {
		t.Fatal("zero line height was accepted")
	}
	if _, err := planner.PlanCompiledHTML(12, nil); err == nil {
		t.Fatal("nil compiled HTML was accepted")
	}
}

func TestCompiledHTMLTextCohortPreservesHrefSegmentBoundariesAndWhitespace(t *testing.T) {
	compiled, err := CompileHTML(`<p>before<a href="https://example.test/exact">linked <span>words</span></a>after and <a href="mailto:paper@example.test">mail</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	paragraph := model.Body[0].(layout.ParagraphBlock)
	want := []layout.TextSegment{
		{Text: "before"},
		{Text: "linked words", Link: "https://example.test/exact"},
		{Text: "after and "},
		{Text: "mail", Link: "mailto:paper@example.test"},
	}
	if !reflect.DeepEqual(paragraph.Segments, want) {
		t.Fatalf("segments = %#v, want %#v", paragraph.Segments, want)
	}
}

func TestCompiledHTMLUnifiedPlanPreservesExactExternalLinksThroughPDF(t *testing.T) {
	compiled, err := CompileHTML(`<p>Read <a href="https://example.test/paper">the exact Paper plan</a> now.</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}), WithNoCompression())
	planner.SetMargins(15, 15, 15)
	planner.SetAutoPageBreak(true, 15)
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("linked HTML plan = %#v source pages %d, %v", plan, planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Links) == 0 {
		t.Fatal("linked HTML produced no exact link annotations")
	}
	for _, link := range projection.Links {
		if link.URI != "https://example.test/paper" || link.Bounds.Width <= 0 {
			t.Fatalf("planned link = %#v", link)
		}
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(pdf.Bytes(), []byte("/URI (https://example.test/paper)")); got != len(projection.Links) {
		t.Fatalf("PDF link annotations = %d, want %d", got, len(projection.Links))
	}

	unsafe, err := CompileHTML(`<p><a href="javascript:alert(1)">unsafe</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	bad, err := planner.PlanCompiledHTML(12, unsafe)
	if err == nil || bad.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("unsafe HTML link = plan %#v source pages %d, %v", bad, planner.PageCount(), err)
	}
}

func TestCompiledHTMLUnifiedPlanResolvesInternalAnchorsAndLinks(t *testing.T) {
	compiled, err := CompileHTML(`<p><span id="top">Target</span> and <a href="#top">jump back</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}), WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("internal-link plan = %#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Destinations) != 1 || len(projection.Links) != 1 || projection.Links[0].Destination != projection.Destinations[0].ID {
		t.Fatalf("internal links/destinations = %#v/%#v", projection.Links, projection.Destinations)
	}
	if projection.Destinations[0].Point.X < 0 || projection.Links[0].Bounds.Width <= 0 {
		t.Fatalf("internal geometry = %#v/%#v", projection.Destinations[0], projection.Links[0].Bounds)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf.Bytes(), []byte("/Dest [")) {
		t.Fatalf("PDF lacks internal destination annotation: %s", pdf.String())
	}
}

func TestCompiledHTMLUnifiedPlanLowersStructuralWrappersDefinitionsAndPageBreaks(t *testing.T) {
	compiled, err := CompileHTML(`<main><section><h3>Wrapped heading</h3><p>Wrapped body</p></section>` +
		`<dl><dt>Term</dt><dd>Definition</dd><dd>Second definition</dd></dl></main>` +
		`<p style="break-before:page">Second page</p>` +
		`<div style="page-break-after:always"><p>Still second</p></div><p>Third page</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 220, Ht: 180}), WithNoCompression())
	planner.SetMargins(15, 15, 15)
	planner.SetAutoPageBreak(true, 15)
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.PageCount() != 3 || planner.PageCount() != 0 {
		t.Fatalf("structural HTML plan = pages %d source pages %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var text strings.Builder
	for _, run := range projection.GlyphRuns {
		text.WriteString(run.Codes)
		text.WriteByte('\n')
	}
	for _, want := range []string{"Wrapped heading", "Wrapped body", "Term", "Definition", "Second definition", "Second page", "Still second", "Third page"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("planned structural text lacks %q:\n%s", want, text.String())
		}
	}
	if len(projection.Breaks) != 2 || projection.Breaks[0].Reason != layoutengine.BreakExplicitPageBreak ||
		projection.Breaks[1].Reason != layoutengine.BreakExplicitPageBreak {
		t.Fatalf("structural breaks = %#v", projection.Breaks)
	}

	for _, source := range []string{
		`<p style="float:left">unsupported</p>`,
		`<p style="break-before:sometimes">unsupported</p>`,
		`<dl><dd>orphan definition</dd></dl>`,
	} {
		candidate, compileErr := CompileHTML(source)
		if compileErr != nil {
			t.Fatal(compileErr)
		}
		bad, planErr := planner.PlanCompiledHTML(12, candidate)
		if !errors.Is(planErr, ErrHTMLPlanUnsupported) || bad.Hash() != "" || planner.PageCount() != 0 {
			t.Fatalf("unsupported structural HTML %q = plan %#v pages %d, %v", source, bad, planner.PageCount(), planErr)
		}
	}
}

func TestCompiledHTMLUnifiedPlanLowersBoundedDataImagesToExactDisplayAndPDF(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	compiled, err := CompileHTML(`<p>Before image</p><img src="data:image/png;base64,` + pixel + `" alt="One pixel" width="24px" height="18"><p>After image</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 1 {
		t.Fatalf("HTML data image resources/images = %d/%d", len(projection.ImageResources), len(projection.Images))
	}
	imageFragment := projection.Fragments[projection.Images[0].Fragment-1]
	if imageFragment.BorderBox.Width.Points() != 18 || imageFragment.BorderBox.Height.Points() != 13.5 || projection.Images[0].Bounds.Width.Points() != 18 || projection.Images[0].Bounds.Height.Points() != 13.5 {
		t.Fatalf("HTML data image resources/images/bounds = %d/%d/%#v", len(projection.ImageResources), len(projection.Images), projection.Images)
	}
	var figure bool
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleFigure && node.Attributes.AlternateText == "One pixel" {
			figure = true
		}
	}
	if !figure {
		t.Fatalf("HTML data image lacks figure/alt semantics: %#v", projection.SemanticNodes)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("data:image/png;base64,")) {
		t.Fatalf("HTML data image capture = %v, %s", err, capture.SVG())
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")) {
		t.Fatalf("HTML data image PDF = %v, image=%t", err, bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")))
	}

	for _, source := range []string{
		`<img src="https://example.test/image.png" alt="external" width="10" height="10">`,
		`<img src="data:image/png;base64,` + pixel + `" width="10" height="10" loading="lazy">`,
		`<img src="data:image/png;base64,` + pixel + `" width="nope" height="10">`,
	} {
		candidate, compileErr := CompileHTML(source)
		if compileErr != nil {
			t.Fatal(compileErr)
		}
		bad, planErr := planner.PlanCompiledHTML(12, candidate)
		if !errors.Is(planErr, ErrHTMLPlanUnsupported) || bad.Hash() != "" || planner.PageCount() != 0 {
			t.Fatalf("unsupported HTML image %q = plan %#v pages %d, %v", source, bad, planner.PageCount(), planErr)
		}
	}
}

func TestCompiledHTMLUnifiedPlanLowersKeepGroupedFigureAndCaption(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	compiled, err := CompileHTML(`<figure><img src="data:image/png;base64,` + pixel + `" alt="Diagram" width="30" height="20"><figcaption>Exact <a href="https://example.test/caption">caption</a></figcaption></figure>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithNoCompression())
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Images) != 1 || len(projection.Links) != 1 || projection.Links[0].URI != "https://example.test/caption" {
		t.Fatalf("figure images/links = %d/%#v", len(projection.Images), projection.Links)
	}
	var text strings.Builder
	for _, run := range projection.GlyphRuns {
		text.WriteString(run.Codes)
	}
	if !strings.Contains(text.String(), "Exact caption") {
		t.Fatalf("figure caption text = %q", text.String())
	}
	imagePage := projection.Fragments[projection.Images[0].Fragment-1].Page
	captionPage := projection.Fragments[len(projection.Fragments)-1].Page
	if imagePage != captionPage {
		t.Fatalf("keep-grouped figure image/caption pages = %d/%d", imagePage, captionPage)
	}

	for _, source := range []string{
		`<figure><figcaption>before</figcaption><img src="data:image/png;base64,` + pixel + `" width="10" height="10"></figure>`,
		`<figure><img src="data:image/png;base64,` + pixel + `" width="10" height="10"><p>not caption</p></figure>`,
		`<figure></figure>`,
	} {
		candidate, compileErr := CompileHTML(source)
		if compileErr != nil {
			t.Fatal(compileErr)
		}
		bad, planErr := planner.PlanCompiledHTML(12, candidate)
		if !errors.Is(planErr, ErrHTMLPlanUnsupported) || bad.Hash() != "" || planner.PageCount() != 0 {
			t.Fatalf("invalid figure %q = plan %#v pages %d, %v", source, bad, planner.PageCount(), planErr)
		}
	}
}
