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

func TestHTMLUnifiedNestedFlexPlansRecursivelyWithSemanticsAndReplay(t *testing.T) {
	source := `<div style="display:flex;gap:10pt;align-items:flex-start">` +
		`<section style="display:flex;flex-direction:column;flex:0 0 90pt;gap:4pt;align-items:stretch">` +
		`<p style="flex:0 0 18pt">Nested one</p><h2 style="flex:0 0 18pt">Nested two</h2></section>` +
		`<p style="flex:1 1 0">Outer peer</p></div>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	first, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	second, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil || first.Hash() == "" || first.Hash() != second.Hash() {
		t.Fatalf("nested plan determinism = %q/%q err=%v", first.Hash(), second.Hash(), err)
	}
	projection := first.plan.Projection()
	if len(projection.Fragments) != 4 {
		t.Fatalf("nested fragments = %+v", projection.Fragments)
	}
	if projection.Fragments[0].BorderBox != (layoutengine.Rect{X: 20 * 1024, Y: 20 * 1024, Width: 90 * 1024, Height: 40 * 1024}) ||
		projection.Fragments[1].BorderBox.X != 120*1024 || projection.Fragments[2].BorderBox.X != 20*1024 || projection.Fragments[3].BorderBox.Y != 42*1024 {
		t.Fatalf("nested recursive geometry = %+v", projection.Fragments)
	}
	roles := map[layoutengine.SemanticRole]int{}
	for _, node := range projection.SemanticNodes {
		roles[node.Role]++
	}
	if roles[layoutengine.SemanticRoleSection] == 0 || roles[layoutengine.SemanticRoleParagraph] < 2 || roles[layoutengine.SemanticRoleHeading] != 1 || len(projection.ReadingOrder) != 4 {
		t.Fatalf("nested semantics roles=%+v reading=%+v", roles, projection.ReadingOrder)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-nested", first, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("nested raster = %q %+v err=%v", status, raster, err)
	}
	render := func() ([]byte, float64) {
		pdf := newHTMLFrameTestDocument(t, 160)
		pdf.SetXY(16, 42)
		html := pdf.HTMLNew()
		if err := html.WriteContext(context.Background(), 12, source); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := pdf.OutputWithOptions(&out, OutputOptions{Deterministic: true}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes(), pdf.GetY()
	}
	firstPDF, firstY := render()
	secondPDF, secondY := render()
	if firstY <= 42 || firstY != secondY || !bytes.Equal(firstPDF, secondPDF) {
		t.Fatalf("nested replay cursor/PDF drift: y %.3f/%.3f bytes %d/%d", firstY, secondY, len(firstPDF), len(secondPDF))
	}
}

func TestHTMLUnifiedWrappedColumnResolvesBoundedIntrinsicWidth(t *testing.T) {
	source := `<div style="display:flex;flex-direction:column;flex-wrap:wrap;gap:5pt 7pt;align-content:flex-start;align-items:flex-start">` +
		`<p style="flex:0 0 50pt">Alpha</p><p style="flex:0 0 50pt">Longer beta</p><p style="flex:0 0 50pt">Gamma</p></div>`
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
	if len(projection.Fragments) != 3 {
		t.Fatalf("intrinsic column fragments = %+v", projection.Fragments)
	}
	// The 80pt body height forms two items in the first column and one in the
	// second. Each column uses its measured preferred text width; the container
	// width is their exact sum plus the authored 7pt cross gap.
	first, second, third := projection.Fragments[0].BorderBox, projection.Fragments[1].BorderBox, projection.Fragments[2].BorderBox
	if first.X != 20*1024 || second.X != first.X || first.Y != 20*1024 || second.Y != 75*1024 || third.X <= first.X || third.Y != 20*1024 || third.Width <= 0 {
		t.Fatalf("intrinsic wrapped-column geometry = %+v", projection.Fragments)
	}
	if third.X+third.Width > 220*1024 {
		t.Fatalf("intrinsic wrapped column escaped body: %+v", projection.Fragments)
	}
	secondPlan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil || plan.Hash() == "" || plan.Hash() != secondPlan.Hash() || len(projection.ReadingOrder) != 3 {
		t.Fatalf("intrinsic wrapped column determinism/semantics hash=%q/%q reading=%+v err=%v", plan.Hash(), secondPlan.Hash(), projection.ReadingOrder, err)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-intrinsic-wrapped-column", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("intrinsic wrapped-column raster = %q %+v err=%v", status, raster, err)
	}
}

func TestHTMLUnifiedNestedFlexConcurrentReuseAndDepthFailureAreAtomic(t *testing.T) {
	source := `<div style="display:flex"><section style="display:flex;flex-direction:column;flex:0 0 80pt"><p style="flex:0 0 18pt">A</p></section><p style="flex:1 1 0">B</p></div>`
	compiled, err := CompileHTML(source)
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
			errs[index], hashes[index] = planErr, plan.Hash()
		}(index)
	}
	group.Wait()
	for index := range hashes {
		if errs[index] != nil || hashes[index] == "" || hashes[index] != hashes[0] {
			t.Fatalf("worker %d hash=%q want=%q err=%v", index, hashes[index], hashes[0], errs[index])
		}
	}

	leaf := layout.Block(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "leaf"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}})
	for depth := uint32(0); depth <= paperRowColumnMaxNesting+1; depth++ {
		leaf = layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{Block: leaf, Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 200}}}}
	}
	target := htmlUnifiedFlexTestPlanner()
	plan, err := target.PlanLayoutDocumentContext(context.Background(), &layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: 20, Right: 20, Bottom: 20, Left: 20}}, Body: []layout.Block{leaf}})
	if err == nil || !strings.Contains(err.Error(), "nesting exceeds the deterministic depth limit") || plan.Hash() != "" || target.PageCount() != 0 {
		t.Fatalf("nested depth plan=%#v pages=%d err=%v", plan, target.PageCount(), err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	plan, err = target.PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || target.PageCount() != 0 {
		t.Fatalf("canceled nested plan=%#v pages=%d err=%v", plan, target.PageCount(), err)
	}
}

func BenchmarkHTMLUnifiedNestedFlexPlanning(b *testing.B) {
	compiled, err := CompileHTML(`<div style="display:flex;gap:8pt"><section style="display:flex;flex-direction:column;flex:0 0 90pt;gap:3pt"><p style="flex:0 0 18pt">A</p><p style="flex:0 0 18pt">B</p></section><p style="flex:1 1 0">C</p></div>`)
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

var _ layout.Block = layout.RowColumnBlock{}
