// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/internal/browseroracle"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

const htmlUnifiedTableRichCellFixture = `<style>
table{width:100%;border-collapse:collapse}
td{padding:2pt;border:1pt solid #405060}
.matrix{display:flex;flex-wrap:wrap;height:34pt;gap:2pt;align-content:space-between}
.tile{flex:0 0 48%;height:16pt;margin:0;font-size:8pt;line-height:9pt}
.callout{padding:2pt;border:1pt solid #aa3300;background-color:#fff2dd;break-inside:avoid}
</style><table><tr><td width="140pt">
<div class="matrix"><p class="tile">A</p><p class="tile">B</p><p class="tile">C</p><p class="tile">D</p></div>
<section class="callout"><p>Decorated note</p></section>
</td><td width="60pt">side</td></tr></table>`

func TestHTMLUnifiedTableCellLowersWrappedFlexAndDecoratedBox(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedTableRichCellFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 10)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohortUnitsWidth(context.Background(), resolved, 10, planner.PointConvert, 200)
	if err != nil {
		t.Fatal(err)
	}
	table, ok := model.Body[0].(layout.TableBlock)
	if !ok || len(table.Body) != 1 || len(table.Body[0].Cells) != 2 {
		t.Fatalf("rich table model = %#v", model.Body)
	}
	blocks := table.Body[0].Cells[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("rich cell blocks = %#v", blocks)
	}
	flex, ok := blocks[0].(layout.RowColumnBlock)
	if !ok || flex.Wrap != "wrap" || len(flex.Items) != 4 {
		t.Fatalf("cell flex = %#v", blocks[0])
	}
	box, ok := blocks[1].(layout.SectionBlock)
	if !ok || box.Box.Padding.Left != 2 || box.Box.Border.Left.Width != 1 || !box.Box.BackgroundColor.Set || !box.Box.KeepTogether {
		t.Fatalf("cell decorated box = %#v", blocks[1])
	}
	plan, err := planner.PlanCompiledHTMLContext(context.Background(), 10, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Pages) != 1 || len(projection.Fragments) < 8 || len(projection.Fills) == 0 || len(projection.Strokes) == 0 {
		t.Fatalf("rich cell projection pages=%d fragments=%d fills=%d strokes=%d", len(projection.Pages), len(projection.Fragments), len(projection.Fills), len(projection.Strokes))
	}
	roles := make(map[layoutengine.SemanticRole]int)
	for _, node := range projection.SemanticNodes {
		roles[node.Role]++
	}
	if roles[layoutengine.SemanticRoleTable] != 1 || roles[layoutengine.SemanticRoleSection] < 1 || roles[layoutengine.SemanticRoleParagraph] < 5 || roles[layoutengine.SemanticRoleArtifact] < 1 {
		t.Fatalf("rich cell semantics = %+v", roles)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-table-rich-cell", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("rich cell raster status=%q evidence=%+v err=%v", status, raster, err)
	}
	taggedPlanner := htmlUnifiedFlexTestPlanner()
	taggedPlanner.EnableTaggedPDF()
	taggedPlanner.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Rich HTML table cell", Lang: "en-US"})
	taggedPlan, err := taggedPlanner.PlanCompiledHTMLContext(context.Background(), 10, compiled)
	if err != nil {
		t.Fatal(err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if _, err := target.WriteLayoutDocumentPlan(taggedPlan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"/S /Table", "/S /TD", "/S /Sect", "/S /P"} {
		if !bytes.Contains(pdf.Bytes(), []byte(token)) {
			t.Fatalf("tagged rich-cell PDF lacks %q", token)
		}
	}
}

func TestHTMLUnifiedTableRichCellPinnedBrowserWrappedGeometry(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedTableRichCellFixture)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 10, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	planned := make(map[string]layoutengine.Rect)
	for _, run := range projection.GlyphRuns {
		if run.Codes == "A" || run.Codes == "B" || run.Codes == "C" || run.Codes == "D" {
			planned[run.Codes] = projection.Lines[run.Line].Bounds
		}
	}
	if len(planned) != 4 {
		t.Fatalf("planned matrix glyphs = %+v", planned)
	}
	browser := `<style>html,body{margin:0;width:320px;height:220px;overflow:hidden}` +
		`table{position:absolute;left:26.6666667px;top:26.6666667px;width:266.6666667px;border-collapse:collapse;table-layout:fixed}` +
		`td{box-sizing:border-box;padding:2.6666667px;border:1.3333333px solid #405060}` +
		`.matrix{display:flex;flex-wrap:wrap;height:45.3333333px;gap:2.6666667px;align-content:space-between}` +
		`.tile{box-sizing:border-box;flex:0 0 48%;height:21.3333333px;margin:0;font:10.6666667px/12px Arial}` +
		`.callout{padding:2.6666667px;border:1.3333333px solid #aa3300;background:#fff2dd;break-inside:avoid}` +
		`</style><table><tr><td style="width:186.6666667px"><div class="matrix"><p class="tile">A</p><p class="tile">B</p><p class="tile">C</p><p class="tile">D</p></div><section class="callout"><p>Decorated note</p></section></td><td style="width:80px">side</td></tr></table>`
	capture, err := browseroracle.CaptureFirefox(t.Context(), browser,
		`(()=>[...document.querySelectorAll('.tile')].map(e=>{const r=e.getBoundingClientRect();return{id:e.textContent,x:r.x,y:r.y,width:r.width,height:r.height}}))()`,
		browseroracle.Options{Width: 320, Height: 220, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var rects []browserFlexRect
	if err := json.Unmarshal(capture.DOMRects, &rects); err != nil || len(rects) != 4 {
		t.Fatalf("browser rich-cell rects=%s err=%v", capture.DOMRects, err)
	}
	byID := make(map[string]browserFlexRect, len(rects))
	for _, rect := range rects {
		byID[rect.ID] = rect
	}
	for _, pair := range [][2]string{{"A", "B"}, {"A", "C"}, {"C", "D"}} {
		planDX := planned[pair[1]].X.Points() - planned[pair[0]].X.Points()
		planDY := planned[pair[1]].Y.Points() - planned[pair[0]].Y.Points()
		browserDX := (byID[pair[1]].X - byID[pair[0]].X) * .75
		browserDY := (byID[pair[1]].Y - byID[pair[0]].Y) * .75
		// Firefox assigns collapsed half-borders to the table grid while the
		// document model owns each authored edge. Their available flex widths
		// consequently differ by less than half a point; row formation and the
		// resulting item deltas must remain inside that calibrated boundary.
		if math.Abs(planDX-browserDX) > .5 || math.Abs(planDY-browserDY) > .5 {
			t.Fatalf("matrix %s->%s plan delta=(%.4f,%.4f) browser=(%.4f,%.4f) rects=%s", pair[0], pair[1], planDX, planDY, browserDX, browserDY, capture.DOMRects)
		}
	}
}

func BenchmarkHTMLUnifiedTableRichCellPlanning(b *testing.B) {
	compiled, err := CompileHTML(htmlUnifiedTableRichCellFixture)
	if err != nil {
		b.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		if _, err := planner.PlanCompiledHTMLContext(context.Background(), 10, compiled); err != nil {
			b.Fatal(err)
		}
	}
}
