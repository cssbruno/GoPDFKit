// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestHTMLUnifiedNestedTableCellPreservesImageDisplayResource(t *testing.T) {
	image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(htmlImagePlanFixturePNG(t))
	source := `<table><tr><td width="200pt"><table><tr><td width="200pt"><figure><img src="` + image + `" alt="nested swatch" width="24" height="12"><figcaption>Nested image</figcaption></figure></td></tr></table></td></tr></table>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 1 {
		t.Fatalf("nested image resources=%+v placements=%+v", projection.ImageResources, projection.Images)
	}
	figures := 0
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleFigure && node.Attributes.AlternateText == "nested swatch" {
			figures++
		}
	}
	if figures != 1 {
		t.Fatalf("nested image figure semantics=%d nodes=%+v", figures, projection.SemanticNodes)
	}
	target := htmlUnifiedFlexTestPlanner()
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")) {
		t.Fatalf("nested image PDF bytes=%d err=%v", pdf.Len(), err)
	}
}

const htmlNestedTableCellFixture = `<style>
table{border-collapse:collapse} .outer{width:100%;break-inside:avoid}
.outer-cell{width:200pt;border:1pt solid #405060}
.key{width:60pt}.value{width:140pt}
</style><table class="outer"><thead><tr><th width="200pt">Outer header</th></tr></thead><tbody><tr><td class="outer-cell">
<p>Before nested table</p>
<table border="1" cellpadding="2" bordercolor="#708090"><caption>Nested <a href="https://example.test/nested-caption">evidence</a></caption>
<thead><tr><th class="key">Key</th><th class="value">Value</th></tr></thead>
<tbody><tr><td>A</td><td><a href="https://example.test/nested-cell">linked value</a></td></tr></tbody></table>
<p>After nested table</p>
</td></tr></tbody></table>`

func TestHTMLUnifiedNestedTableCellGeometrySemanticsLinksAndPDF(t *testing.T) {
	compiled, err := CompileHTML(htmlNestedTableCellFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	model, err := lowerCompiledHTMLTextCohortUnitsWidth(context.Background(), mustResolveHTMLUnifiedForNestedTableTest(t, planner, compiled), 12, planner.PointConvert, 200)
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := model.Body[0].(layout.TableBlock)
	if !ok || len(outer.Body) != 1 || len(outer.Body[0].Cells) != 1 || len(outer.Body[0].Cells[0].Blocks) != 3 {
		t.Fatalf("outer nested-table model = %#v", model.Body)
	}
	nested, ok := outer.Body[0].Cells[0].Blocks[1].(layout.TableBlock)
	if !ok || len(nested.Columns) != 2 || nested.Columns[0].Width != 60 || nested.Columns[1].Width != 140 || len(nested.Header) != 1 || len(nested.Body) != 1 {
		t.Fatalf("nested table geometry = %#v", outer.Body[0].Cells[0].Blocks[1])
	}

	plan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	roles := make(map[layoutengine.SemanticRole]int)
	links := make(map[string]bool)
	for _, node := range projection.SemanticNodes {
		roles[node.Role]++
	}
	for _, link := range projection.Links {
		links[link.URI] = true
	}
	if roles[layoutengine.SemanticRoleTable] != 2 || roles[layoutengine.SemanticRoleRow] != 4 || roles[layoutengine.SemanticRoleCell] != 6 ||
		!links["https://example.test/nested-caption"] || !links["https://example.test/nested-cell"] {
		t.Fatalf("nested table roles=%v links=%+v", roles, projection.Links)
	}
	if len(projection.Paths) == 0 || len(projection.Strokes) == 0 || len(projection.Fragments) < 10 {
		t.Fatalf("nested table display geometry fragments=%d paths=%d strokes=%d", len(projection.Fragments), len(projection.Paths), len(projection.Strokes))
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`data-command-index=`)) || bytes.Count(capture.SVG(), []byte(`<text `)) < 20 {
		t.Fatalf("nested table display capture = %v\n%s", err, capture.SVG())
	}

	tagged := htmlUnifiedFlexTestPlanner()
	tagged.EnableTaggedPDF()
	tagged.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Nested HTML table", Lang: "en-US"})
	taggedPlan, err := tagged.PlanCompiledHTMLContext(context.Background(), 12, compiled)
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
	if bytes.Count(pdf.Bytes(), []byte("/S /Table")) < 2 || bytes.Count(pdf.Bytes(), []byte("/Subtype /Link")) < 2 {
		t.Fatalf("tagged nested table PDF tables=%d links=%d", bytes.Count(pdf.Bytes(), []byte("/S /Table")), bytes.Count(pdf.Bytes(), []byte("/Subtype /Link")))
	}
}

func TestHTMLUnifiedNestedTableCellLimitsCancellationAndConcurrentReuse(t *testing.T) {
	deep := "<p>leaf</p>"
	for index := 0; index < htmlUnifiedMaxNestedTableDepth+1; index++ {
		deep = `<table><tr><td>` + deep + `</td></tr></table>`
	}
	compiled, err := CompileHTML(deep)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	failed, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if !errors.Is(err, ErrHTMLLimitExceeded) || failed.Hash() != "" || planner.PageCount() != 0 || planner.Error() != nil {
		t.Fatalf("nested depth failure plan=%q pages=%d documentErr=%v err=%v", failed.Hash(), planner.PageCount(), planner.Error(), err)
	}

	compiled, err = CompileHTML(htmlNestedTableCellFixture)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner = htmlUnifiedFlexTestPlanner()
	failed, err = planner.PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || failed.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("nested cancellation plan=%q pages=%d err=%v", failed.Hash(), planner.PageCount(), err)
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
			t.Fatalf("nested worker %d hash=%q want=%q err=%v", index, hashes[index], hashes[0], errs[index])
		}
	}
}

func TestHTMLUnifiedNestedTableCellVisualFixture(t *testing.T) {
	destination := os.Getenv("GOPDFKIT_NESTED_TABLE_FIXTURE_PDF")
	if destination == "" {
		t.Skip("set GOPDFKIT_NESTED_TABLE_FIXTURE_PDF to write the reviewed nested-table PDF")
	}
	compiled, err := CompileHTML(htmlNestedTableCellFixture)
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

func mustResolveHTMLUnifiedForNestedTableTest(t *testing.T, planner *Document, compiled *CompiledHTML) *CompiledHTML {
	t.Helper()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err = planner.resolveCompiledHTMLImageSources(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func BenchmarkHTMLUnifiedNestedTableCellPlanning(b *testing.B) {
	compiled, err := CompileHTML(strings.TrimSpace(htmlNestedTableCellFixture))
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
