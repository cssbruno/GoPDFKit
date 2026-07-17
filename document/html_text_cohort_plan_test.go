// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestHTMLUnifiedTextCohortResolvedInlineStylesWhitespaceDisplayRasterAndPDF(t *testing.T) {
	compiled, err := CompileHTML(`<style>
		h2,p,pre{font-family:Courier;font-size:10pt;line-height:12pt}
		.accent{font-weight:bold;color:#b02030}
		.em{font-style:italic}
	</style><h2>Styled heading</h2><p>plain <strong class="accent">bold link</strong> <em class="em">italic</em> ` +
		`<a href="https://example.test/text"><span class="accent">exact target</span></a></p>` +
		`<p style="white-space:pre-line">alpha   beta
		gamma</p><pre>  fixed
  columns</pre>`)
	if err != nil {
		t.Fatal(err)
	}
	modelPlanner := paginationTestDocument(t, 180)
	resolved, err := modelPlanner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 4 {
		t.Fatalf("lowered text blocks = %d, want 4", len(model.Body))
	}
	paragraph := model.Body[1].(layout.ParagraphBlock)
	if got := layout.TextSegmentsPlainText(paragraph.Segments); got != "plain bold link italic exact target" {
		t.Fatalf("styled paragraph text = %q", got)
	}
	var bold, italic, linked bool
	for _, segment := range paragraph.Segments {
		bold = bold || segment.Style.Bold && segment.Style.Color.Set
		italic = italic || segment.Style.Italic
		linked = linked || segment.Link == "https://example.test/text" && strings.Contains(segment.Text, "exact target")
	}
	if !bold || !italic || !linked {
		t.Fatalf("styled segments bold/italic/link = %t/%t/%t: %#v", bold, italic, linked, paragraph.Segments)
	}
	preLine := model.Body[2].(layout.ParagraphBlock)
	pre := model.Body[3].(layout.ParagraphBlock)
	if got := layout.TextSegmentsPlainText(preLine.Segments); got != "alpha beta\ngamma" {
		t.Fatalf("pre-line text = %q", got)
	}
	if got := layout.TextSegmentsPlainText(pre.Segments); got != "  fixed\n  columns" || !strings.EqualFold(pre.Style.FontFamily, "Courier") {
		t.Fatalf("pre text/style = %q / %#v", got, pre.Style)
	}

	planner := paginationTestDocument(t, 180, WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("plan = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Links) == 0 {
		t.Fatal("styled linked text produced no exact annotation")
	}
	faces := make(map[layoutengine.CoreFontFace]bool)
	for _, font := range projection.Fonts {
		faces[font.Face] = true
	}
	if !faces[layoutengine.CoreFontCourier] || !faces[layoutengine.CoreFontCourierBold] || !faces[layoutengine.CoreFontCourierOblique] {
		t.Fatalf("planned inline faces = %#v", faces)
	}
	var heading, listText bool
	for _, node := range projection.SemanticNodes {
		heading = heading || node.Role == layoutengine.SemanticRoleHeading && node.Attributes.ActualText == "Styled heading"
		listText = listText || node.Role == layoutengine.SemanticRoleParagraph && strings.Contains(node.Attributes.ActualText, "exact target")
	}
	if !heading || !listText {
		t.Fatalf("heading/paragraph semantics = %t/%t", heading, listText)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`font-family="courier_bold" fill="#b02030"`)) {
		t.Fatalf("display capture = %v, %s", err, capture.SVG())
	}
	if png := captureFlexPlanPNG(t, plan); len(png) == 0 {
		t.Fatal("direct display raster is empty")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil ||
		!bytes.Contains(output.Bytes(), []byte("/URI (https://example.test/text)")) {
		t.Fatalf("PDF replay = %v, bytes=%d", err, output.Len())
	}
}

func TestHTMLUnifiedTextCohortPlansMetricChangingInlineStyleAndDecoration(t *testing.T) {
	for _, source := range []string{
		`<p>before <span style="font-size:18pt">larger</span> after</p>`,
		`<p>before <span style="font-size:16pt;line-height:20pt">taller</span> after</p>`,
		`<p style="font-family:Helvetica">before <strong>wide MMMM</strong> after</p>`,
	} {
		compiled, err := CompileHTML(source)
		if err != nil {
			t.Fatal(err)
		}
		planner := paginationTestDocument(t, 100)
		plan, err := planner.PlanCompiledHTML(12, compiled)
		if err != nil || plan.Hash() == "" || plan.PageCount() == 0 || planner.PageCount() != 0 {
			t.Fatalf("metric-changing source %q = plan %q pages %d, %v", source, plan.Hash(), planner.PageCount(), err)
		}
		var plannedText strings.Builder
		var previousLine uint32
		for index, run := range plan.plan.Projection().GlyphRuns {
			if index != 0 && run.Line != previousLine {
				plannedText.WriteByte(' ')
			}
			plannedText.WriteString(run.Codes)
			previousLine = run.Line
		}
		wantText := "before larger after"
		if strings.Contains(source, "taller") {
			wantText = "before taller after"
		} else if strings.Contains(source, "wide") {
			wantText = "before wide MMMM after"
		}
		if got := strings.Join(strings.Fields(plannedText.String()), " "); got != wantText {
			t.Fatalf("metric-changing source %q lost wrapped text: got %q want %q", source, got, wantText)
		}
		if strings.Contains(source, "font-size") {
			var hasBaseSize, hasLargerSize bool
			largerSize := layoutengine.Fixed(18 * layoutengine.FixedScale)
			if strings.Contains(source, "16pt") {
				largerSize = layoutengine.Fixed(16 * layoutengine.FixedScale)
			}
			for _, run := range plan.plan.Projection().GlyphRuns {
				hasBaseSize = hasBaseSize || run.FontSize == layoutengine.Fixed(12*layoutengine.FixedScale)
				hasLargerSize = hasLargerSize || run.FontSize == largerSize
			}
			if !hasBaseSize || !hasLargerSize {
				t.Fatalf("metric-changing source %q did not retain mixed glyph sizes: %#v", source, plan.plan.Projection().GlyphRuns)
			}
		} else if len(plan.plan.Projection().Fonts) < 2 {
			t.Fatalf("metric-changing source %q did not retain distinct core font resources", source)
		}
		if _, err := plan.CaptureDisplayPage(1); err != nil {
			t.Fatalf("metric-changing source %q display capture: %v", source, err)
		}
	}
	compiled, err := CompileHTML(`<p><span style="text-decoration:underline">underlined</span> <span style="text-decoration:line-through">struck</span></p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := paginationTestDocument(t, 100)
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() == 0 || planner.PageCount() != 0 {
		t.Fatalf("decoration source = plan %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var strokes int
	for _, command := range projection.Commands {
		if command.Kind == layoutengine.CommandStrokePath {
			strokes++
		}
	}
	if strokes != 2 {
		t.Fatalf("decoration source stroke commands = %d, want 2", strokes)
	}
	if _, err := plan.CaptureDisplayPage(1); err != nil {
		t.Fatalf("decoration display capture: %v", err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatalf("decoration PDF replay: %v", err)
	}
}

func TestHTMLUnifiedTableCellPlansMetricChangingInlineStyle(t *testing.T) {
	compiled, err := CompileHTML(`<table><tbody><tr><td>before <span style="font-size:18pt">larger</span> after</td></tr></tbody></table>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := paginationTestDocument(t, 160)
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() == 0 || planner.PageCount() != 0 {
		t.Fatalf("metric-changing table cell = plan %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	var hasLargerSize bool
	for _, run := range plan.plan.Projection().GlyphRuns {
		hasLargerSize = hasLargerSize || run.FontSize == layoutengine.Fixed(18*layoutengine.FixedScale)
	}
	if !hasLargerSize {
		t.Fatalf("metric-changing table cell did not retain the inline font size: %#v", plan.plan.Projection().GlyphRuns)
	}
	decorated, err := CompileHTML(`<table><tbody><tr><td><span style="text-decoration:underline">underlined</span></td></tr></tbody></table>`)
	if err != nil {
		t.Fatal(err)
	}
	decoratedPlan, err := paginationTestDocument(t, 160).PlanCompiledHTML(12, decorated)
	if err != nil || decoratedPlan.Hash() == "" {
		t.Fatalf("decorated metric-changing table cell = plan %q, %v", decoratedPlan.Hash(), err)
	}
	var strokes int
	for _, command := range decoratedPlan.plan.Projection().Commands {
		if command.Kind == layoutengine.CommandStrokePath {
			strokes++
		}
	}
	if strokes != 1 {
		t.Fatalf("decorated table cell stroke commands = %d, want 1", strokes)
	}
}

func TestHTMLUnifiedListsRetainResolvedTextLinksSemanticsAndBreakPolicy(t *testing.T) {
	compiled, err := CompileHTML(`<style>ol,li,dt,dd{font-family:Courier;font-size:10pt;line-height:12pt}</style>` +
		`<p>first page</p><ol style="break-before:page;break-inside:avoid"><li>one <strong>bold</strong></li>` +
		`<li><a href="https://example.test/item">linked item</a></li></ol>` +
		`<dl><dt style="break-inside:avoid">Term</dt><dd>Definition</dd></dl>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := paginationTestDocument(t, 140)
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.PageCount() != 2 {
		t.Fatalf("list plan pages = %d, %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Breaks) != 1 || projection.Breaks[0].Reason != layoutengine.BreakExplicitPageBreak || len(projection.Links) == 0 {
		t.Fatalf("list breaks/links = %#v / %#v", projection.Breaks, projection.Links)
	}
	var list, items, term, definition bool
	for _, node := range projection.SemanticNodes {
		list = list || node.Role == layoutengine.SemanticRoleList
		if node.Role == layoutengine.SemanticRoleListItem {
			items = true
		}
		term = term || node.Role == layoutengine.SemanticRoleHeading && node.Attributes.ActualText == "Term"
		definition = definition || node.Role == layoutengine.SemanticRoleParagraph && node.Attributes.ActualText == "Definition"
	}
	if !list || !items || !term || !definition {
		t.Fatalf("list semantics list/items/term/definition = %t/%t/%t/%t", list, items, term, definition)
	}
	var text strings.Builder
	for _, run := range projection.GlyphRuns {
		text.WriteString(run.Codes)
	}
	for _, want := range []string{"1. one bold", "2. linked item", "Term", "Definition"} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("list display lacks %q: %q", want, text.String())
		}
	}
}

func TestHTMLUnifiedTextPlanIsDetachedCancelableAndConcurrent(t *testing.T) {
	compiled, err := CompileHTML(`<p style="font-family:Courier">stable <strong>content</strong> <a href="https://example.test/stable">link</a></p>`)
	if err != nil {
		t.Fatal(err)
	}
	base, err := paginationTestDocument(t, 100).PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	for index := range compiled.tokens {
		if compiled.tokens[index].Cat == 'T' {
			compiled.tokens[index].Str = "mutated"
		}
	}
	const workers = 8
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
			if _, writeErr := target.WriteLayoutDocumentPlan(base); writeErr != nil {
				errs <- writeErr
				return
			}
			var output bytes.Buffer
			if outputErr := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); outputErr != nil ||
				!bytes.Contains(output.Bytes(), []byte("stable")) || bytes.Contains(output.Bytes(), []byte("mutated")) {
				errs <- errors.New("immutable HTML text plan observed source mutation")
			}
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	plan, err := paginationTestDocument(t, 100).PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" {
		t.Fatalf("canceled plan = %q, %v", plan.Hash(), err)
	}
}
