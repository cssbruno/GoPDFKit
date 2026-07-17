// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/inspect"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestHTMLUnifiedFlexLowersExactFactorsBasisAndConstraints(t *testing.T) {
	source := `<div style="display:flex;gap:10pt">` +
		`<p style="flex:1 1 40pt;min-width:25pt;max-width:60pt">Grow</p>` +
		`<p style="flex-grow:2;flex-shrink:3;flex-basis:50%">Percent</p></div>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := htmlUnifiedFlexTestPlanner().resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	row := model.Body[0].(layout.RowColumnBlock)
	wantFirst := layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisFixed, Basis: 40, Min: 25, Max: 60, Grow: 1, Shrink: 1}
	wantSecond := layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisPercent, BasisPercent: 50_000_000, Grow: 2, Shrink: 3}
	if row.Items[0].Track != wantFirst || row.Items[1].Track != wantSecond {
		t.Fatalf("flex lowering = %+v, want %+v / %+v", row.Items, wantFirst, wantSecond)
	}
}

func TestHTMLUnifiedFlexGrowAndShrinkFreezeAtMinMax(t *testing.T) {
	tests := []struct {
		name, source string
		want         []layoutengine.Rect
	}{
		{
			name: "grow freezes maximum",
			source: `<div style="display:flex;gap:10pt"><p style="flex:1 1 40pt;max-width:60pt">A</p>` +
				`<p style="flex:2 1 20pt">B</p></div>`,
			want: []layoutengine.Rect{
				{X: 20 * 1024, Y: 20 * 1024, Width: 60 * 1024, Height: 12 * 1024},
				{X: 90 * 1024, Y: 20 * 1024, Width: 130 * 1024, Height: 12 * 1024},
			},
		},
		{
			name: "scaled shrink",
			source: `<div style="display:flex;gap:10pt"><p style="flex:0 1 120pt;min-width:80pt">A</p>` +
				`<p style="flex:0 2 100pt;min-width:40pt">B</p></div>`,
			want: []layoutengine.Rect{
				{X: 20 * 1024, Y: 20 * 1024, Width: 108.75 * 1024, Height: 12 * 1024},
				{X: 138.75 * 1024, Y: 20 * 1024, Width: 81.25 * 1024, Height: 12 * 1024},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(test.source)
			if err != nil {
				t.Fatal(err)
			}
			first, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
			if err != nil {
				t.Fatal(err)
			}
			second, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
			if err != nil || first.Hash() == "" || first.Hash() != second.Hash() {
				t.Fatalf("deterministic plans = %q / %q, %v", first.Hash(), second.Hash(), err)
			}
			fragments := first.plan.Projection().Fragments
			if len(fragments) != len(test.want) {
				t.Fatalf("fragments = %+v", fragments)
			}
			for index := range test.want {
				if fragments[index].BorderBox != test.want[index] {
					t.Errorf("fragment %d = %+v, want %+v", index, fragments[index].BorderBox, test.want[index])
				}
			}
		})
	}
}

func TestHTMLUnifiedFlexWrappedFixedAndPercentageBasesGrowPerLine(t *testing.T) {
	source := `<div style="display:flex;flex-wrap:wrap;height:40pt;gap:5pt 10pt;align-content:flex-start">` +
		`<p style="flex:1 1 60pt">A</p><p style="flex:1 1 50%">B</p><p style="flex:1 1 60pt">C</p></div>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	fragments := plan.plan.Projection().Fragments
	want := []layoutengine.Rect{
		{X: 20 * 1024, Y: 20 * 1024, Width: 75 * 1024, Height: 12 * 1024},
		{X: 105 * 1024, Y: 20 * 1024, Width: 115 * 1024, Height: 12 * 1024},
		{X: 20 * 1024, Y: 37 * 1024, Width: 200 * 1024, Height: 12 * 1024},
	}
	if len(fragments) != len(want) {
		t.Fatalf("wrapped flexible fragments = %+v", fragments)
	}
	for index := range want {
		if fragments[index].BorderBox != want[index] {
			t.Errorf("fragment %d = %+v, want %+v", index, fragments[index].BorderBox, want[index])
		}
	}
	projection := plan.plan.Projection()
	if len(projection.ReadingOrder) != 3 || projection.ReadingOrder[2].Fragment != projection.Fragments[2].ID {
		t.Fatalf("wrapped flexible reading order = %+v", projection.ReadingOrder)
	}
}

func TestHTMLUnifiedFlexReverseMainKeepsReadingOrderAndRenders(t *testing.T) {
	source := `<div style="display:flex;flex-direction:row-reverse;gap:10pt">` +
		`<p style="flex:0 0 40pt">First</p><h2 style="flex:0 0 40pt">Second</h2></div>`
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
	if len(projection.Fragments) != 2 || projection.Fragments[0].BorderBox.X != 180*1024 || projection.Fragments[1].BorderBox.X != 130*1024 {
		t.Fatalf("reverse-main geometry = %+v", projection.Fragments)
	}
	if len(projection.ReadingOrder) != 2 || projection.ReadingOrder[0].Fragment != projection.Fragments[0].ID || projection.ReadingOrder[1].Fragment != projection.Fragments[1].ID {
		t.Fatalf("reverse-main reading order = %+v", projection.ReadingOrder)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-reverse-main", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("reverse-main raster = %q %+v, %v", status, raster, err)
	}
	if got := raster.Pages[0].PNGSHA256; got != "dfac7861e26b5a00942e85919435a0aa851a064ea84ecf7429e34e57d12ef9b8" {
		t.Fatalf("reverse-main raster drift = %s", got)
	}

	renderPDF := func() ([]byte, float64) {
		pdf := newHTMLFrameTestDocument(t, 160)
		pdf.SetXY(16, 42)
		html := pdf.HTMLNew()
		if err := html.WriteContext(context.Background(), 12, source); err != nil {
			t.Fatal(err)
		}
		if pdf.GetX() != 16 || pdf.GetY() <= 42 {
			t.Fatalf("reverse-main cursor = %.3f,%.3f", pdf.GetX(), pdf.GetY())
		}
		var output bytes.Buffer
		if err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
			t.Fatal(err)
		}
		return output.Bytes(), pdf.GetY()
	}
	firstPDF, firstY := renderPDF()
	secondPDF, secondY := renderPDF()
	if firstY != secondY || !bytes.Equal(firstPDF, secondPDF) {
		t.Fatalf("reverse-main deterministic PDF drift: y %.3f/%.3f bytes %d/%d", firstY, secondY, len(firstPDF), len(secondPDF))
	}
	text, err := inspect.PageTextContext(context.Background(), firstPDF, 1)
	if err != nil {
		t.Fatal(err)
	}
	text = strings.ReplaceAll(text, "\x00", "")
	if !strings.Contains(text, "First") || !strings.Contains(text, "Second") {
		t.Fatalf("reverse-main PDF text = %q", text)
	}
}

func TestTypedRowColumnReverseMainHasCausalGeometryAndStableSemantics(t *testing.T) {
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}}
	}
	model := &layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: 20, Right: 20, Bottom: 20, Left: 20}}, Body: []layout.Block{layout.RowColumnBlock{
		Direction: layout.RowDirection, Gap: 10, ReverseMain: true,
		Items: []layout.RowColumnItem{
			{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 40}, Block: paragraph("First")},
			{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 40}, Block: paragraph("Second")},
		},
	}}}
	planner := htmlUnifiedFlexTestPlanner()
	reversed, err := planner.PlanLayoutDocumentContext(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	projection := reversed.plan.Projection()
	if len(projection.Fragments) != 2 || projection.Fragments[0].BorderBox.X != 180*1024 || projection.Fragments[1].BorderBox.X != 130*1024 ||
		len(projection.ReadingOrder) != 2 || projection.ReadingOrder[0].Fragment != projection.Fragments[0].ID {
		t.Fatalf("typed reverse-main projection = %+v / %+v", projection.Fragments, projection.ReadingOrder)
	}
	container := model.Body[0].(layout.RowColumnBlock)
	container.ReverseMain = false
	model.Body[0] = container
	forward, err := htmlUnifiedFlexTestPlanner().PlanLayoutDocumentContext(context.Background(), model)
	if err != nil {
		t.Fatal(err)
	}
	if forward.Hash() == reversed.Hash() || forward.plan.Projection().Fragments[0].BorderBox.X != 20*1024 {
		t.Fatalf("ReverseMain had no causal effect: %q / %q", forward.Hash(), reversed.Hash())
	}
}

func TestHTMLUnifiedFlexExtendedCohortCancelsAtomicallyAndRejectsRemainder(t *testing.T) {
	compiled, err := CompileHTML(`<div style="display:flex"><p style="flex:1 1 20pt">A</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTMLContext(ctx, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled flex plan = %#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
	unsupported, err := CompileHTML(`<div style="display:flex"><p style="flex:1.1234567 1 20pt">A</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = planner.PlanCompiledHTMLContext(context.Background(), 12, unsupported)
	if !errors.Is(err, ErrHTMLPlanUnsupported) || !strings.Contains(err.Error(), `flex shorthand "1.1234567 1 20pt" is unsupported`) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("unsupported flex plan = %#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
}

func TestHTMLUnifiedFlexIntrinsicFractionalPercentageBoundsAndPrecedence(t *testing.T) {
	source := `<div style="display:flex;height:30pt;gap:10pt;align-items:center">` +
		`<p style="flex:1.5 1 auto;max-width:40%;min-height:50%">Alpha</p>` +
		`<p style="flex:9 9 90pt;flex-grow:.5;flex-shrink:1.25;flex-basis:10%;min-width:20%;max-width:60%;height:12pt">Beta</p></div>`
	compiled, err := CompileHTML(source)
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
	row := model.Body[0].(layout.RowColumnBlock)
	if row.Items[0].Track.BasisKind != layout.RowColumnFlexBasisContent || row.Items[0].Track.GrowFactor != 1_500_000 || row.Items[0].Track.MaxPercent != 40_000_000 || row.Items[0].CrossMinPercent != 50_000_000 {
		t.Fatalf("intrinsic/fractional first item = %+v", row.Items[0])
	}
	second := row.Items[1].Track
	if second.BasisKind != layout.RowColumnFlexBasisPercent || second.BasisPercent != 10_000_000 || second.GrowFactor != 500_000 || second.ShrinkFactor != 1_250_000 || second.MinPercent != 20_000_000 || second.MaxPercent != 60_000_000 {
		t.Fatalf("longhand precedence/percentage second track = %+v", second)
	}
	first, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	secondPlan, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil || first.Hash() == "" || first.Hash() != secondPlan.Hash() {
		t.Fatalf("deterministic intrinsic plan = %q/%q err=%v", first.Hash(), secondPlan.Hash(), err)
	}
	projection := first.plan.Projection()
	if len(projection.Fragments) != 2 || projection.Fragments[0].BorderBox != (layoutengine.Rect{X: 20 * 1024, Y: 27.5 * 1024, Width: 80 * 1024, Height: 15 * 1024}) || projection.Fragments[1].BorderBox != (layoutengine.Rect{X: 110 * 1024, Y: 29 * 1024, Width: 110 * 1024, Height: 12 * 1024}) {
		t.Fatalf("intrinsic/fractional bounded geometry = %+v", projection.Fragments)
	}
	if len(projection.ReadingOrder) != 2 || projection.ReadingOrder[0].Fragment != projection.Fragments[0].ID {
		t.Fatalf("reading order = %+v", projection.ReadingOrder)
	}
	firstRaster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-intrinsic-fractional", first, &characterizationRasterBudget{})
	if err != nil || status != "captured" || firstRaster == nil || firstRaster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("raster = %q %+v err=%v", status, firstRaster, err)
	}
	secondRaster, _, err := captureCharacterizationRaster(t.Context(), "html-flex-intrinsic-fractional", secondPlan, &characterizationRasterBudget{})
	if err != nil || firstRaster.Pages[0].PNGSHA256 != secondRaster.Pages[0].PNGSHA256 {
		t.Fatalf("deterministic raster drift: %v", err)
	}
}

func TestHTMLUnifiedFlexStructuredTableItemPlanRasterPDFSemanticsAndCursor(t *testing.T) {
	source := `<div style="display:flex;gap:10pt;align-items:flex-start"><section style="flex:0 0 120pt"><table><tbody><tr><th>Code</th><td>OK</td></tr></tbody></table></section><p style="flex:1 1 0">Summary</p></div>`
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
	roles := map[layoutengine.SemanticRole]bool{}
	for _, node := range projection.SemanticNodes {
		roles[node.Role] = true
	}
	header := false
	for _, node := range projection.SemanticNodes {
		header = header || node.Attributes.TableHeader
	}
	if !roles[layoutengine.SemanticRoleTable] || !header || !roles[layoutengine.SemanticRoleParagraph] {
		t.Fatalf("structured flex semantics = %+v", projection.SemanticNodes)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-structured-table", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("structured raster = %q %+v err=%v", status, raster, err)
	}
	render := func() ([]byte, float64) {
		pdf := newHTMLFrameTestDocument(t, 160)
		pdf.SetXY(16, 42)
		html := pdf.HTMLNew()
		if err := html.WriteContext(context.Background(), 12, source); err != nil {
			t.Fatal(err)
		}
		if pdf.GetX() != 16 || pdf.GetY() <= 42 {
			t.Fatalf("structured cursor = %.3f,%.3f", pdf.GetX(), pdf.GetY())
		}
		var out bytes.Buffer
		if err := pdf.OutputWithOptions(&out, OutputOptions{Deterministic: true}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes(), pdf.GetY()
	}
	firstPDF, firstY := render()
	secondPDF, secondY := render()
	if firstY != secondY || !bytes.Equal(firstPDF, secondPDF) {
		t.Fatalf("structured deterministic PDF drift")
	}
	text, err := inspect.PageTextContext(context.Background(), firstPDF, 1)
	if err != nil || !strings.Contains(strings.ReplaceAll(text, "\x00", ""), "Code") || !strings.Contains(strings.ReplaceAll(text, "\x00", ""), "Summary") {
		t.Fatalf("structured PDF text = %q err=%v", text, err)
	}
}

func TestHTMLUnifiedFlexWrappedContentBasisIsDeterministicAndKeepsSourceOrder(t *testing.T) {
	source := `<div style="display:flex;flex-wrap:wrap;height:60pt;gap:4pt 10pt;align-content:flex-start">` +
		`<p style="flex:0 1 auto;max-width:45%">Alpha alpha</p><p style="flex:0 1 auto;max-width:45%">Beta beta</p><p style="flex:0 1 auto;max-width:45%">Gamma gamma</p></div>`
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
	if err != nil || first.Hash() != second.Hash() {
		t.Fatalf("wrapped content deterministic plan = %q/%q err=%v", first.Hash(), second.Hash(), err)
	}
	projection := first.plan.Projection()
	if len(projection.Fragments) != 3 || projection.Fragments[2].BorderBox.Y <= projection.Fragments[0].BorderBox.Y {
		t.Fatalf("wrapped content geometry = %+v", projection.Fragments)
	}
	for index, occurrence := range projection.ReadingOrder {
		if occurrence.Fragment != projection.Fragments[index].ID {
			t.Fatalf("wrapped content reading order = %+v", projection.ReadingOrder)
		}
	}
}

func TestHTMLUnifiedFlexIntrinsicCompiledConcurrentReuse(t *testing.T) {
	compiled, err := CompileHTML(`<div style="display:flex;height:32pt;gap:5pt"><p style="flex:.75 1 auto;min-width:15%">Alpha beta</p><p style="flex:1.25 1 auto;max-width:65%">Gamma</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 12
	hashes := make(chan string, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			plan, planErr := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
			if planErr != nil {
				errs <- planErr
				return
			}
			hashes <- plan.Hash()
		}()
	}
	group.Wait()
	close(hashes)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	want := ""
	for hash := range hashes {
		if want == "" {
			want = hash
		}
		if hash == "" || hash != want {
			t.Fatalf("concurrent intrinsic plan hash = %q, want %q", hash, want)
		}
	}
}

func BenchmarkHTMLUnifiedFlexIntrinsicFractionalPlan(b *testing.B) {
	compiled, err := CompileHTML(`<div style="display:flex;height:40pt;gap:4pt"><p style="flex:.5 1 auto;min-width:10%">Alpha beta</p><p style="flex:1.5 1 auto;max-width:70%">Gamma delta</p></div>`)
	if err != nil {
		b.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled); err != nil {
			b.Fatal(err)
		}
	}
}
