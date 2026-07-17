// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/internal/browseroracle"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func htmlUnifiedTableStructuredFixture(t testing.TB) string {
	t.Helper()
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(htmlImagePlanFixturePNG(t))
	return `<style>
		table{width:100%;border-collapse:collapse;break-inside:avoid}
		.code{width:25%}.body-code{width:25%;font-size:9pt;line-height:10pt}.detail{width:60pt}.visual{min-width:20pt;max-width:100pt}
		figcaption{font-size:8pt;line-height:9pt;color:#334455}
	</style><table border="1" cellpadding="2" bordercolor="#506070">` +
		`<caption>Evidence <a href="https://example.test/table-caption">table</a></caption>` +
		`<thead><tr><th class="code">Code</th><th class="detail">Details</th><th class="visual">Visual</th></tr></thead>` +
		`<tr><td class="body-code"><p>Alpha</p><ul><li>first</li><li>second</li></ul></td>` +
		`<td><a href="https://example.test/cell">linked detail</a></td>` +
		`<td><figure><img src="` + image + `" alt="Table swatch" width="32" height="16">` +
		`<figcaption>Cell figure</figcaption></figure></td></tr></table>`
}

func TestHTMLUnifiedTableTracksStructuredCellsSemanticsRasterPDFAndCursor(t *testing.T) {
	source := htmlUnifiedTableStructuredFixture(t)
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = planner.resolveCompiledHTMLImageSources(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohortUnitsWidth(context.Background(), resolved, 12, planner.PointConvert, 200)
	if err != nil {
		t.Fatal(err)
	}
	table, ok := model.Body[0].(layout.TableBlock)
	if !ok || len(table.Columns) != 3 || table.Columns[0].Width != 50 || table.Columns[1].Width != 60 || table.Columns[2].MinWidth != 20 || table.Columns[2].MaxWidth != 100 {
		t.Fatalf("resolved table tracks = %#v", model.Body[0])
	}
	if len(table.Header) != 1 || len(table.Body) != 1 || len(table.Body[0].Cells[0].Blocks) != 2 || len(table.CaptionSegments) != 2 {
		t.Fatalf("structured table shape = caption %#v header %d body %#v", table.CaptionSegments, len(table.Header), table.Body)
	}
	firstCell := table.Body[0].Cells[0]
	if firstCell.Box.Padding.Left != 1.5 || firstCell.Box.Border.Left.Width != .75 || firstCell.Box.Border.Left.Color.R != 0x50 {
		t.Fatalf("legacy table defaults = %#v", firstCell.Box)
	}
	plan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	roles := make(map[layoutengine.SemanticRole]int)
	var headerCells int
	linkTargets := make(map[string]bool)
	for _, node := range projection.SemanticNodes {
		roles[node.Role]++
		if node.Role == layoutengine.SemanticRoleCell && node.Attributes.TableHeader {
			headerCells++
		}
	}
	for _, link := range projection.Links {
		linkTargets[link.URI] = true
	}
	if roles[layoutengine.SemanticRoleTable] != 1 || roles[layoutengine.SemanticRoleList] != 1 || roles[layoutengine.SemanticRoleListItem] != 2 ||
		roles[layoutengine.SemanticRoleFigure] != 1 || headerCells != 3 || !linkTargets["https://example.test/table-caption"] || !linkTargets["https://example.test/cell"] || len(projection.ImageResources) != 1 {
		t.Fatalf("table semantics=%v headers=%d links=%+v images=%+v", roles, headerCells, projection.Links, projection.ImageResources)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-table-structured", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) == 0 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("table raster status=%q raster=%+v err=%v", status, raster, err)
	}
	render := func() []byte {
		target := htmlUnifiedFlexTestPlanner()
		if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}
	firstPDF, secondPDF := render(), render()
	if !bytes.Equal(firstPDF, secondPDF) || !bytes.Contains(firstPDF, []byte("/Subtype /Image")) || !bytes.Contains(firstPDF, []byte("/Subtype /Link")) {
		t.Fatalf("deterministic table PDF bytes=%d/%d image=%t link=%t", len(firstPDF), len(secondPDF), bytes.Contains(firstPDF, []byte("/Subtype /Image")), bytes.Contains(firstPDF, []byte("/Subtype /Link")))
	}
	tagged := htmlUnifiedFlexTestPlanner()
	tagged.EnableTaggedPDF()
	tagged.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "HTML structured table", Lang: "en-US"})
	taggedPlan, err := tagged.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	taggedTarget := htmlUnifiedFlexTestPlanner()
	if _, err := taggedTarget.WriteLayoutDocumentPlan(taggedPlan); err != nil {
		t.Fatal(err)
	}
	var taggedPDF bytes.Buffer
	if err := taggedTarget.OutputWithOptions(&taggedPDF, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"/S /Table", "/S /TR", "/S /TH", "/S /TD", "/S /L", "/S /LI", "/S /Figure"} {
		if !bytes.Contains(taggedPDF.Bytes(), []byte(token)) {
			t.Fatalf("tagged HTML table PDF lacks %q", token)
		}
	}

	live := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 220}), WithNoCompression(), WithDeterministicOutput())
	live.SetMargins(20, 20, 20)
	live.SetAutoPageBreak(true, 20)
	live.AddPage()
	live.SetFont("Helvetica", "", 12)
	live.SetXY(20, 170)
	html := live.HTMLNew()
	fragment, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.reuseCurrentPage || fragment.final.page != 2 {
		t.Fatalf("kept table frame = reuse %t final page %d", fragment.reuseCurrentPage, fragment.final.page)
	}
	if err := html.WriteContext(context.Background(), 12, source); err != nil {
		t.Fatal(err)
	}
	if live.PageNo() != 2 || live.GetX() != 20 || live.GetY() != fragment.final.y {
		t.Fatalf("table exit frame page=%d x=%.3f y=%.6f want=%+v", live.PageNo(), live.GetX(), live.GetY(), fragment.final)
	}
}

func TestHTMLUnifiedTableRepeatedHeaderPaginationAndAtomicContracts(t *testing.T) {
	var rows strings.Builder
	for index := 0; index < 18; index++ {
		rows.WriteString(`<tr><td>row `)
		rows.WriteString(string(rune('A' + index)))
		rows.WriteString(`</td><td>bounded content</td></tr>`)
	}
	source := `<table><thead><tr><th width="40%">Name</th><th width="60%">Value</th></tr></thead><tbody>` + rows.String() + `</tbody></table>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := typedTableTestPlanner().PlanCompiledHTMLContext(context.Background(), 10, compiled)
	if err != nil || plan.PageCount() < 2 {
		t.Fatalf("paginated HTML table pages=%d err=%v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var repeated int
	for _, fragment := range projection.Fragments {
		if fragment.Repeated {
			repeated++
		}
	}
	if repeated == 0 || len(projection.Breaks) == 0 {
		t.Fatalf("repeated header fragments=%d breaks=%+v", repeated, projection.Breaks)
	}

	invalid := []string{
		`<table><tr><td width="40%">a</td><td width="60%">b</td></tr><tr><td width="50%">c</td><td>d</td></tr></table>`,
		`<table><tr><td colspan="2" width="100%">ambiguous</td></tr></table>`,
		`<table><tr><td style="min-width:80%;max-width:20%">invalid</td></tr></table>`,
		`<table><tr><td><table><tbody><tr><td>nested</td></tr></tbody><tbody><tr><td>duplicate section</td></tr></tbody></table></td></tr></table>`,
	}
	for _, source := range invalid {
		compiled, compileErr := CompileHTML(source)
		if compileErr != nil {
			continue
		}
		planner := htmlUnifiedFlexTestPlanner()
		bad, planErr := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
		if planErr == nil || bad.Hash() != "" || planner.PageCount() != 0 || planner.Error() != nil {
			t.Fatalf("invalid table %q plan=%#v pages=%d documentErr=%v err=%v", source, bad, planner.PageCount(), planner.Error(), planErr)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := htmlUnifiedFlexTestPlanner()
	bad, err := planner.PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || bad.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled table plan=%#v pages=%d err=%v", bad, planner.PageCount(), err)
	}
}

func TestHTMLUnifiedTableCompiledReuseIsConcurrentAndDetached(t *testing.T) {
	compiled, err := CompileHTML(`<table><caption>Concurrent</caption><tr><th width="30%">Key</th><th width="70%">Value</th></tr><tr><td>A</td><td>Reusable</td></tr></table>`)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 16
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
			t.Fatalf("table worker %d hash=%q want=%q err=%v", index, hashes[index], hashes[0], errs[index])
		}
	}
}

func TestHTMLUnifiedTablePinnedBrowserTrackGeometry(t *testing.T) {
	const paper = `<style>table{width:100%;border-collapse:collapse}td{padding:0;border:none;font-size:12pt;line-height:12pt}.a{width:30%}.b{width:70%}</style>` +
		`<table><tr><td class="a">A</td><td class="b">B</td></tr></table>`
	compiled, err := CompileHTML(paper)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	browser := `<style>html,body{margin:0;width:320px;height:213px;overflow:hidden;background:white}` +
		`table{position:absolute;left:26.6666667px;top:26.6666667px;width:266.6666667px;border-collapse:collapse;table-layout:fixed}` +
		`td{box-sizing:border-box;padding:0;border:0;font:16px/16px Arial}.a{width:30%}.b{width:70%}</style>` +
		`<table><tr><td class="a">A</td><td class="b">B</td></tr></table>`
	capture, err := browseroracle.CaptureFirefox(t.Context(), browser,
		`(()=>[...document.querySelectorAll("td")].map((e,i)=>{const r=e.getBoundingClientRect();return{id:String(i),x:r.x,y:r.y,width:r.width,height:r.height}}))()`,
		browseroracle.Options{Width: 320, Height: 213, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var rects []browserFlexRect
	if err := json.Unmarshal(capture.DOMRects, &rects); err != nil || len(rects) != 2 {
		t.Fatalf("browser table rects=%s err=%v", capture.DOMRects, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) < 2 {
		t.Fatalf("table fragments=%+v", fragments)
	}
	for index := range rects {
		box := fragments[index].BorderBox
		want := []float64{box.X.Points(), box.Y.Points(), box.Width.Points(), box.Height.Points()}
		got := []float64{rects[index].X * .75, rects[index].Y * .75, rects[index].Width * .75, rects[index].Height * .75}
		for axis := range want {
			if math.Abs(got[axis]-want[axis]) > 1.0/1024 {
				t.Fatalf("table cell %d axis %d browser=%.6fpt plan=%.6fpt rects=%s", index, axis, got[axis], want[axis], capture.DOMRects)
			}
		}
	}
}

func BenchmarkHTMLUnifiedStructuredTablePlanning(b *testing.B) {
	source := htmlUnifiedTableStructuredFixture(b)
	compiled, err := CompileHTML(source)
	if err != nil {
		b.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if _, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled); err != nil {
			b.Fatal(err)
		}
	}
}

func TestHTMLUnifiedTableVisualFixture(t *testing.T) {
	destination := os.Getenv("GOPDFKIT_TABLE_FIXTURE_PDF")
	if destination == "" {
		t.Skip("set GOPDFKIT_TABLE_FIXTURE_PDF to write the reviewed table PDF")
	}
	compiled, err := CompileHTML(htmlUnifiedTableStructuredFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}
