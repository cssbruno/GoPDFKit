// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/internal/browseroracle"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestHTMLUnifiedWhitespacePinnedBrowserLineGeometry(t *testing.T) {
	const source = `<style>p{margin:0;font-family:Helvetica;font-size:12pt;line-height:12pt}.nw{white-space:nowrap}.bs{white-space:break-spaces;tab-size:4}</style>` +
		`<p class="nw">NOWRAP alpha beta gamma delta epsilon zeta eta theta iota kappa lambda</p>` +
		`<p class="bs">BREAK A&#9;B
C   D</p>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	lineFragments := make(map[layoutengine.FragmentID]int)
	for _, line := range projection.Lines {
		lineFragments[line.Fragment]++
	}
	planned := make(map[string]int)
	for _, run := range projection.GlyphRuns {
		for _, id := range []string{"NOWRAP", "BREAK"} {
			if strings.Contains(run.Codes, id) {
				planned[id] = lineFragments[projection.Lines[run.Line].Fragment]
			}
		}
	}
	if planned["NOWRAP"] != 1 || planned["BREAK"] != 2 {
		t.Fatalf("planned whitespace line counts = %+v", planned)
	}
	browser := `<style>html,body{margin:0;width:266.666667px;overflow:visible}p{margin:0;font:16px/16px Arial}.nw{white-space:nowrap}.bs{white-space:break-spaces;tab-size:4}</style>` +
		`<p class="nw">NOWRAP alpha beta gamma delta epsilon zeta eta theta iota kappa lambda</p><p class="bs">BREAK A&#9;B
C   D</p>`
	capture, err := browseroracle.CaptureFirefox(t.Context(), browser,
		`(()=>[...document.querySelectorAll('p')].map(e=>{const r=e.getBoundingClientRect();return{id:e.className,x:r.x,y:r.y,width:r.width,height:r.height}}))()`,
		browseroracle.Options{Width: 320, Height: 120, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var rects []browserFlexRect
	if err := json.Unmarshal(capture.DOMRects, &rects); err != nil || len(rects) != 2 {
		t.Fatalf("browser whitespace rects=%s err=%v", capture.DOMRects, err)
	}
	if rects[0].Height*.75 != 12 || rects[1].Height*.75 != 24 {
		t.Fatalf("browser whitespace heights=%s", capture.DOMRects)
	}
}

const htmlUnifiedNestedListFixture = `<style>
ol.outer{list-style-type:upper-alpha;break-inside:avoid}
ul.clean{list-style-type:none}
.spaces{white-space:break-spaces;tab-size:4;font-size:10pt;line-height:12pt}
.nowrap{white-space:nowrap;font-size:10pt;line-height:12pt}
</style>
<ol class="outer" start="2"><li>Alpha <a href="https://example.test/list">link</a>
<ul class="clean"><li>Nested plain<ol type="i" start="3"><li value="4">Roman override</li><li>Roman next</li></ol></li></ul>
</li><li value="5">Outer override</li></ol>
<dl><dt>Term</dt><dd>Description<dl><dt>Nested term</dt><dd>Nested definition</dd></dl></dd></dl>
<p class="spaces">A&#9;B
C   D</p><p class="nowrap">nowrap words stay on one authored line</p>`

func TestHTMLUnifiedNestedListsCountersWhitespaceLinksAndSemantics(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedNestedListFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohortUnitsWidth(context.Background(), resolved, 12, planner.PointConvert, 200)
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := model.Body[0].(layout.ListBlock)
	if !ok || !outer.Ordered || outer.Start != 2 || outer.MarkerStyle != "upper-alpha" || len(outer.Items) != 2 || !outer.Items[1].ValueSet || outer.Items[1].Value != 5 {
		t.Fatalf("outer list = %#v", model.Body[0])
	}
	clean, ok := outer.Items[0].Blocks[1].(layout.ListBlock)
	if !ok || clean.Ordered || clean.MarkerStyle != "none" || len(clean.Items) != 1 {
		t.Fatalf("nested clean list = %#v", outer.Items[0].Blocks)
	}
	roman, ok := clean.Items[0].Blocks[1].(layout.ListBlock)
	if !ok || roman.MarkerStyle != "lower-roman" || roman.Start != 3 || !roman.Items[0].ValueSet || roman.Items[0].Value != 4 {
		t.Fatalf("nested roman list = %#v", clean.Items[0].Blocks)
	}
	var spaces, nowrap layout.ParagraphBlock
	for _, block := range model.Body {
		paragraph, isParagraph := block.(layout.ParagraphBlock)
		if !isParagraph {
			continue
		}
		text := layout.TextSegmentsPlainText(paragraph.Segments)
		if strings.Contains(text, "A   B") {
			spaces = paragraph
		}
		if strings.Contains(text, "nowrap words") {
			nowrap = paragraph
		}
	}
	if got := layout.TextSegmentsPlainText(spaces.Segments); got != "A   B\nC   D" || spaces.Style.WhiteSpace != "break-spaces" || spaces.Style.TabSize != 4 {
		t.Fatalf("break-spaces paragraph text=%q style=%+v", got, spaces.Style)
	}
	if nowrap.Style.WhiteSpace != "nowrap" {
		t.Fatalf("nowrap style = %+v", nowrap.Style)
	}

	plan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	roles := make(map[layoutengine.SemanticRole]int)
	for _, node := range projection.SemanticNodes {
		roles[node.Role]++
	}
	if roles[layoutengine.SemanticRoleList] != 3 || roles[layoutengine.SemanticRoleListItem] != 5 || roles[layoutengine.SemanticRoleHeading] < 2 {
		t.Fatalf("nested list semantics = %+v", roles)
	}
	text := strings.Builder{}
	for _, run := range projection.GlyphRuns {
		text.WriteString(run.Codes)
		text.WriteByte('|')
	}
	for _, marker := range []string{"B. ", "E. ", "iv. ", "v. "} {
		if !strings.Contains(text.String(), marker) {
			t.Fatalf("planned list glyphs lack %q: %s", marker, text.String())
		}
	}
	if len(projection.Links) != 1 || projection.Links[0].URI != "https://example.test/list" {
		t.Fatalf("nested list links = %+v", projection.Links)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-nested-lists-whitespace", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) == 0 {
		t.Fatalf("nested list raster status=%q evidence=%+v err=%v", status, raster, err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Link")) {
		t.Fatalf("nested list PDF bytes=%d err=%v", pdf.Len(), err)
	}
}

func TestHTMLUnifiedNestedListAtomicLimitsCancellationAndConcurrentReuse(t *testing.T) {
	invalid := []string{
		`<ol start="01"><li>bad</li></ol>`,
		`<ol type="z"><li>bad</li></ol>`,
		`<ul start="2"><li>bad</li></ul>`,
		`<ol type="i"><li value="0">roman zero</li></ol>`,
		`<p style="tab-size:0">bad tab</p>`,
	}
	for _, source := range invalid {
		compiled, compileErr := CompileHTML(source)
		if compileErr != nil {
			continue
		}
		planner := htmlUnifiedFlexTestPlanner()
		plan, planErr := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
		if planErr == nil || plan.Hash() != "" || planner.PageCount() != 0 || planner.Error() != nil {
			t.Fatalf("invalid nested-list source=%q hash=%q pages=%d documentErr=%v err=%v", source, plan.Hash(), planner.PageCount(), planner.Error(), planErr)
		}
	}
	compiled, err := CompileHTML(htmlUnifiedNestedListFixture)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	failed, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || failed.Hash() != "" {
		t.Fatalf("nested list cancellation hash=%q err=%v", failed.Hash(), err)
	}
	const workers = 8
	hashes := make([]string, workers)
	errs := make([]error, workers)
	var group sync.WaitGroup
	for index := range hashes {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			plan, planErr := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
			hashes[index], errs[index] = plan.Hash(), planErr
		}(index)
	}
	group.Wait()
	for index := range hashes {
		if errs[index] != nil || hashes[index] == "" || hashes[index] != hashes[0] {
			t.Fatalf("nested list worker %d hash=%q want=%q err=%v", index, hashes[index], hashes[0], errs[index])
		}
	}
}

func TestHTMLUnifiedNestedListOuterPageBreaksRemainCausal(t *testing.T) {
	compiled, err := CompileHTML(`<p>before</p><ol style="break-before:page;break-after:page"><li><a href="https://example.test/break-list">linked item</a><ul><li>nested item</li></ul></li></ol><p>after</p>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	fragmentPages := make(map[layoutengine.FragmentID]uint32, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragmentPages[fragment.ID] = fragment.Page
	}
	if len(projection.Pages) != 3 || len(projection.Breaks) != 2 || len(projection.Links) != 1 || fragmentPages[projection.Links[0].Fragment] != 2 {
		t.Fatalf("nested list page-break pages=%d breaks=%+v links=%+v", len(projection.Pages), projection.Breaks, projection.Links)
	}
	for _, decision := range projection.Breaks {
		if decision.Reason != layoutengine.BreakExplicitPageBreak {
			t.Fatalf("nested list break reason = %+v", decision)
		}
	}
}

func BenchmarkHTMLUnifiedNestedListWhitespacePlanning(b *testing.B) {
	compiled, err := CompileHTML(htmlUnifiedNestedListFixture)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if _, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled); err != nil {
			b.Fatal(err)
		}
	}
}
