// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

const htmlUnifiedFlexWrapFixture = `<div style="display:flex;flex-wrap:wrap;height:80pt;gap:4pt 10pt;` +
	`justify-content:space-between;align-items:flex-start;align-content:space-between">` +
	`<p style="flex:0 0 80pt">One</p><h2 style="flex:0 0 80pt">Two</h2><p style="flex:0 0 80pt">Three</p></div>`

func TestHTMLUnifiedFlexWrapExactPlanSemanticsRasterPDFAndCursor(t *testing.T) {
	requireDarwinRasterBaseline(t)
	compiled, err := CompileHTML(htmlUnifiedFlexWrapFixture)
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
	row, ok := model.Body[0].(layout.RowColumnBlock)
	if !ok || row.Wrap != "wrap" || row.Gap != 10 || row.CrossGap != 4 || row.CrossSize != 80 ||
		row.MainAlign != "space-between" || row.AlignContent != "space-between" {
		t.Fatalf("wrap lowering=%#v", model.Body[0])
	}

	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	want := []layoutengine.Rect{
		{X: 20 * 1024, Y: 20 * 1024, Width: 80 * 1024, Height: 12 * 1024},
		{X: 140 * 1024, Y: 20 * 1024, Width: 80 * 1024, Height: 12 * 1024},
		{X: 20 * 1024, Y: 88 * 1024, Width: 80 * 1024, Height: 12 * 1024},
	}
	if len(projection.Fragments) != len(want) {
		t.Fatalf("fragments=%+v", projection.Fragments)
	}
	for index := range want {
		if projection.Fragments[index].BorderBox != want[index] {
			t.Errorf("fragment %d=%+v want %+v", index, projection.Fragments[index].BorderBox, want[index])
		}
	}
	if len(projection.SemanticFragments) != 3 || len(projection.ReadingOrder) != 3 {
		t.Fatalf("semantics=%+v/%+v", projection.SemanticFragments, projection.ReadingOrder)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-wrap", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 {
		t.Fatalf("raster=%+v status=%q err=%v", raster, status, err)
	}
	if got := raster.Pages[0].PNGSHA256; got != "31da3bf2b6301ec43d047e18a2fc38caae7582eab8230fe9f4de4c6e03026ae2" {
		t.Fatalf("wrap raster drift: %s", got)
	}

	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("write pages=%d err=%v", pages, err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil || output.Len() == 0 || !bytes.Contains(output.Bytes(), []byte(" Tj")) {
		t.Fatalf("PDF bytes=%d err=%v", output.Len(), err)
	}

	live := newHTMLFrameTestDocument(t, 160)
	live.SetXY(16, 42)
	html := live.HTMLNew()
	if err := html.WriteContext(context.Background(), 12, htmlUnifiedFlexWrapFixture); err != nil || live.GetY() <= 42 {
		t.Fatalf("cursor=%.4f err=%v", live.GetY(), err)
	}
}

func TestHTMLUnifiedFlexWrapReverseAndAlignContentModes(t *testing.T) {
	tests := []struct {
		align               string
		wantFirst, wantLast float64
	}{
		{"start", 20, 36}, {"center", 46, 62}, {"end", 72, 88},
		{"space-between", 20, 88}, {"space-around", 33, 75},
		{"space-evenly", 37.333984375, 70.6669921875}, {"stretch", 20, 62},
	}
	for _, test := range tests {
		t.Run(test.align, func(t *testing.T) {
			source := strings.Replace(htmlUnifiedFlexWrapFixture, "align-content:space-between", "align-content:"+test.align, 1)
			compiled, err := CompileHTML(source)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
			if err != nil {
				t.Fatal(err)
			}
			fragments := plan.plan.Projection().Fragments
			if fragments[0].BorderBox.Y.Points() != test.wantFirst || fragments[2].BorderBox.Y.Points() != test.wantLast {
				t.Fatalf("y=%.12g/%.12g want %.12g/%.12g", fragments[0].BorderBox.Y.Points(), fragments[2].BorderBox.Y.Points(), test.wantFirst, test.wantLast)
			}
		})
	}

	reverse := strings.Replace(htmlUnifiedFlexWrapFixture, "flex-wrap:wrap", "flex-wrap:wrap-reverse", 1)
	compiled, err := CompileHTML(reverse)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	fragments := plan.plan.Projection().Fragments
	if fragments[0].BorderBox.Y.Points() != 88 || fragments[1].BorderBox.Y.Points() != 88 || fragments[2].BorderBox.Y.Points() != 20 ||
		fragments[0].ID != 1 || fragments[1].ID != 2 || fragments[2].ID != 3 {
		t.Fatalf("wrap-reverse geometry/order=%+v", fragments)
	}
}

func TestHTMLUnifiedColumnFlexWrapUsesDefiniteCrossSizes(t *testing.T) {
	source := `<div style="display:flex;flex-direction:column;flex-wrap:wrap;width:100pt;gap:10pt 5pt;align-content:center;align-items:flex-start">` +
		`<p style="flex:0 0 50pt;width:30pt">One</p><p style="flex:0 0 50pt;width:40pt">Two</p><p style="flex:0 0 50pt;width:30pt">Three</p></div>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	fragments := plan.plan.Projection().Fragments
	want := []layoutengine.Rect{
		{X: 32.5 * 1024, Y: 20 * 1024, Width: 30 * 1024, Height: 50 * 1024},
		{X: 32.5 * 1024, Y: 80 * 1024, Width: 40 * 1024, Height: 50 * 1024},
		{X: 77.5 * 1024, Y: 20 * 1024, Width: 30 * 1024, Height: 50 * 1024},
	}
	if len(fragments) != len(want) {
		t.Fatalf("fragments=%+v", fragments)
	}
	for index := range want {
		if fragments[index].BorderBox != want[index] {
			t.Errorf("fragment %d=%+v want %+v", index, fragments[index].BorderBox, want[index])
		}
	}
}

func TestHTMLUnifiedFlexWrapRejectsOutsideCohortAtomically(t *testing.T) {
	tests := []struct{ name, source, diagnostic string }{
		{"missing-row-height", `<div style="display:flex;flex-wrap:wrap"><p style="flex:0 0 20pt">A</p></div>`, "definite positive height"},
		{"undersized-intrinsic-column-item", `<div style="display:flex;flex-direction:column;flex-wrap:wrap"><p style="flex:0 0 20pt;width:10pt">A</p></div>`, "minimum exceeds its region"},
		{"fraction-track", `<div style="display:flex;flex-wrap:wrap;height:40pt"><p style="flex:1">A</p></div>`, "positive fixed or percentage main-axis basis"},
		{"align-content", `<div style="display:flex;flex-wrap:wrap;height:40pt;align-content:baseline"><p style="flex:0 0 20pt">A</p></div>`, "align-content"},
		{"oversized-main", `<div style="display:flex;flex-wrap:wrap;height:40pt"><p style="flex:0 0 220pt">A</p></div>`, "minimum exceeds its region"},
		{"cross-overflow", `<div style="display:flex;flex-wrap:wrap;height:10pt"><p style="flex:0 0 80pt">A</p><p style="flex:0 0 80pt">B</p><p style="flex:0 0 80pt">C</p></div>`, "plan wrapped row/column"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(test.source)
			if err != nil {
				t.Fatal(err)
			}
			planner := htmlUnifiedFlexTestPlanner()
			plan, err := planner.PlanCompiledHTML(12, compiled)
			if !errors.Is(err, ErrHTMLPlanUnsupported) || !strings.Contains(err.Error(), test.diagnostic) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("plan=%#v pages=%d err=%v want %q", plan, planner.PageCount(), err, test.diagnostic)
			}
		})
	}
}
