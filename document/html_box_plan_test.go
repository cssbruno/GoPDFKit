// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

const htmlUnifiedBoxFixture = `<p style="margin:2pt 3pt 4pt 5pt;padding:6pt 7pt 8pt 9pt;` +
	`background-color:#112233;border-top:1pt solid #ff0000;border-right:2pt solid #00ff00;` +
	`border-bottom:3pt solid #0000ff;border-left:4pt solid #010203;break-inside:avoid">Box</p>`

func TestHTMLUnifiedBoxModelExactPlanRasterPDFAndCursor(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedBoxFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 {
		t.Fatalf("fragments=%+v", projection.Fragments)
	}
	fragment := projection.Fragments[0]
	if fragment.BorderBox != (layoutengine.Rect{X: 25 * 1024, Y: 22 * 1024, Width: 192 * 1024, Height: 30 * 1024}) ||
		fragment.ContentBox != (layoutengine.Rect{X: 38 * 1024, Y: 29 * 1024, Width: 170 * 1024, Height: 12 * 1024}) {
		t.Fatalf("box geometry=%+v", fragment)
	}
	if len(projection.Commands) != 6 || len(projection.Fills) != 5 || projection.Commands[0].Kind != layoutengine.CommandFillPath ||
		projection.Commands[5].Kind != layoutengine.CommandGlyphRun {
		t.Fatalf("paint order commands=%+v fills=%d", projection.Commands, len(projection.Fills))
	}
	if len(projection.SemanticFragments) != 1 || len(projection.ReadingOrder) != 1 {
		t.Fatalf("semantics=%+v/%+v", projection.SemanticFragments, projection.ReadingOrder)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-box-model", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 {
		t.Fatalf("raster=%+v status=%q err=%v", raster, status, err)
	}
	if got := raster.Pages[0].PNGSHA256; got != "6fdd04a914c8488683fc15e7aa46572093253c71d86b97fb87694b5f82c2aa9b" {
		t.Fatalf("box raster drift: %s", got)
	}

	target := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	target.SetMargins(20, 20, 20)
	if pages, writeErr := target.WriteLayoutDocumentPlan(plan); writeErr != nil || pages != 1 {
		t.Fatalf("write pages=%d err=%v", pages, writeErr)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 || bytes.Count(output.Bytes(), []byte(" rg")) < 5 {
		t.Fatalf("decorated PDF bytes=%d lacks planned fills", output.Len())
	}

	live := newHTMLFrameTestDocument(t, 160)
	live.SetXY(16, 42)
	html := live.HTMLNew()
	if err := html.WriteContext(context.Background(), 12, htmlUnifiedBoxFixture); err != nil || live.GetY() <= 42 {
		t.Fatalf("live cursor=%.4f err=%v", live.GetY(), err)
	}
}

const htmlUnifiedRoundedShadowFixture = `<p style="margin:4pt;padding:5pt;background-color:#eef4fa;` +
	`border:2pt solid #28445f;border-radius:7pt;box-shadow:3pt 4pt 0 2pt #6c7884">Rounded shadow</p>`

func TestHTMLUnifiedRoundedShadowExactPlanSVGDirectRasterPDFAndSemantics(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedRoundedShadowFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 || len(projection.Paths) != 3 || len(projection.Fills) != 3 || len(projection.Commands) != 4 {
		t.Fatalf("rounded projection fragments=%d paths=%d fills=%d commands=%d", len(projection.Fragments), len(projection.Paths), len(projection.Fills), len(projection.Commands))
	}
	for index := 0; index < 3; index++ {
		if projection.Commands[index].Kind != layoutengine.CommandFillPath || projection.Fills[index].Rule != layoutengine.FillNonZero {
			t.Fatalf("rounded command[%d]=%+v fill=%+v", index, projection.Commands[index], projection.Fills[index])
		}
		hasCurve := false
		for _, segment := range projection.Paths[index].Segments {
			hasCurve = hasCurve || segment.Kind == layoutengine.PathCubicTo
		}
		if !hasCurve {
			t.Fatalf("rounded path[%d] has no immutable cubic corners", index)
		}
	}
	if projection.Commands[3].Kind != layoutengine.CommandGlyphRun || len(projection.SemanticFragments) != 1 || len(projection.ReadingOrder) != 1 {
		t.Fatalf("content/semantics commands=%+v semantics=%+v reading=%+v", projection.Commands, projection.SemanticFragments, projection.ReadingOrder)
	}
	shadow := projection.Commands[0].Bounds
	box := projection.Fragments[0].BorderBox
	if shadow.X != box.X+1024 || shadow.Y != box.Y+2*1024 || shadow.Width != box.Width+4*1024 || shadow.Height != box.Height+4*1024 {
		t.Fatalf("shadow bounds=%+v box=%+v", shadow, box)
	}

	svg, err := layoutengine.CaptureDisplayPlanSVG(plan.plan, 1, nil)
	if err != nil || !bytes.Contains(svg.SVG, []byte(" C")) {
		t.Fatalf("rounded SVG bytes=%d err=%v", len(svg.SVG), err)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-rounded-shadow", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 {
		t.Fatalf("direct raster=%+v status=%q err=%v", raster, status, err)
	}

	target := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	target.SetMargins(20, 20, 20)
	if pages, writeErr := target.WriteLayoutDocumentPlan(plan); writeErr != nil || pages != 1 {
		t.Fatalf("write pages=%d err=%v", pages, writeErr)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if output.Len() == 0 || bytes.Count(output.Bytes(), []byte(" rg")) < 3 || !bytes.Contains(output.Bytes(), []byte(" c")) {
		t.Fatalf("rounded planned PDF bytes=%d lacks shared curves/fills", output.Len())
	}

	live := newHTMLFrameTestDocument(t, 160)
	live.SetXY(16, 42)
	html := live.HTMLNew()
	if err := html.WriteContext(context.Background(), 12, htmlUnifiedRoundedShadowFixture); err != nil || live.GetY() <= 42 {
		t.Fatalf("live cursor=%.4f err=%v", live.GetY(), err)
	}
}

func TestHTMLUnifiedDecoratedStructuralWrapperLowersAtomically(t *testing.T) {
	compiled, err := CompileHTML(`<section style="margin:2pt;padding:3pt;background-color:#eeeeee;border:1pt solid #222222"><h2>Title</h2></section>`)
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
	if len(model.Body) != 1 {
		t.Fatalf("body=%+v", model.Body)
	}
	section, ok := model.Body[0].(layout.SectionBlock)
	if !ok || len(section.Blocks) != 1 || !section.Box.BackgroundColor.Set {
		t.Fatalf("section=%#v", model.Body[0])
	}
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.PageCount() != 1 || len(plan.plan.Projection().Fragments) != 1 {
		t.Fatalf("plan pages=%d err=%v projection=%+v", plan.PageCount(), err, plan.plan.Projection())
	}
}

func TestHTMLUnifiedStructuralBoxAutoIntrinsicAndBoundedHeight(t *testing.T) {
	planHeight := func(source string) (float64, error) {
		compiled, err := CompileHTML(source)
		if err != nil {
			return 0, err
		}
		planner := htmlUnifiedFlexTestPlanner()
		plan, err := planner.PlanCompiledHTML(12, compiled)
		if err != nil {
			return 0, err
		}
		projection := plan.plan.Projection()
		if len(projection.Fragments) != 1 {
			return 0, fmt.Errorf("got %d fragments, want one structural-box fragment", len(projection.Fragments))
		}
		for _, command := range projection.Commands {
			if command.Kind == layoutengine.CommandFillPath {
				return command.Bounds.Height.Points(), nil
			}
		}
		return 0, fmt.Errorf("structural box emitted no decoration bounds")
	}

	intrinsic, err := planHeight(`<div style="padding:2pt;background-color:#eeeeee"><p style="margin:0">Short</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	auto, err := planHeight(`<div style="padding:2pt;background-color:#eeeeee;height:auto"><p style="margin:0">Short</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if intrinsic <= 0 || auto != intrinsic {
		t.Fatalf("auto height = %.2f, intrinsic = %.2f; auto must preserve intrinsic flow height", auto, intrinsic)
	}
	percentage, err := planHeight(`<div style="background-color:#eeeeee;height:25%"><p style="margin:0">Short</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if percentage != 30 {
		t.Fatalf("percentage height border box = %.2f, want 30 (25%% of the 120pt body)", percentage)
	}

	minimum, err := planHeight(`<div style="padding:2pt;background-color:#eeeeee;min-height:30pt"><p style="margin:0">Short</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if minimum != 34 {
		t.Fatalf("min-height border box = %.2f, want 34 (30pt content + 4pt padding)", minimum)
	}

	fixed, err := planHeight(`<div style="padding:2pt;background-color:#eeeeee;height:24pt;overflow:hidden"><p style="margin:0">` + strings.Repeat("long content ", 18) + `</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if fixed != 28 {
		t.Fatalf("fixed border box = %.2f, want 28 (24pt content + 4pt padding)", fixed)
	}

	maximum, err := planHeight(`<div style="padding:2pt;background-color:#eeeeee;max-height:24pt;overflow:hidden"><p style="margin:0">` + strings.Repeat("long content ", 18) + `</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	if maximum != 28 {
		t.Fatalf("max-height border box = %.2f, want 28 (24pt content + 4pt padding)", maximum)
	}
}

func TestHTMLUnifiedBoxModelRejectsUnrepresentableCSSAtomically(t *testing.T) {
	tests := []struct{ name, style, diagnostic string }{
		{"negative-margin", `margin:-1pt`, "non-negative"},
		{"radius", `border-radius:2pt 3pt`, "border-radius"},
		{"shadow", `box-shadow:1pt 1pt 2pt #000`, "blur"},
		{"unbounded-height-percentage", `height:1001%`, "height"},
		{"mixed-overflow", `overflow-x:hidden;overflow-y:visible`, "same visible or hidden"},
		{"border-style", `border:1pt dashed #000`, "solid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(`<p style="` + test.style + `">Box</p>`)
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

	compiled, err := CompileHTML(`<div style="padding:2pt;background-color:#fff"><p>A</p><p>B</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || plan.Hash() == "" || plan.PageCount() != 1 {
		t.Fatalf("multi-child wrapper plan=%#v pages=%d err=%v", plan, planner.PageCount(), err)
	}
}

func TestHTMLUnifiedRoundedShadowRejectsUnsupportedFormsAtomically(t *testing.T) {
	tests := []struct{ name, style, diagnostic string }{
		{"elliptical-radius", `border-radius:4pt / 2pt`, "one fixed"},
		{"percentage-radius", `border-radius:10%`, "one fixed"},
		{"asymmetric-radius", `border-radius:2pt 3pt`, "one fixed"},
		{"asymmetric-rounded-border", `background-color:#fff;border-top:1pt solid #000;border-right:2pt solid #000;border-bottom:1pt solid #000;border-left:1pt solid #000;border-radius:3pt`, "equal solid"},
		{"transparent-rounded-interior", `border:1pt solid #000;border-radius:3pt`, "opaque background"},
		{"multiple-shadows", `box-shadow:1pt 1pt #000,2pt 2pt #fff`, "exactly one"},
		{"inset-shadow", `box-shadow:inset 1pt 1pt #000`, "inset"},
		{"blurred-shadow", `box-shadow:1pt 1pt 2pt #000`, "blur"},
		{"alpha-shadow", `box-shadow:1pt 1pt rgba(0,0,0,.5)`, "opaque RGB"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(`<p style="` + test.style + `">Box</p>`)
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

func TestHTMLUnifiedBoxModelPercentSizingBoxSizingAndOverflow(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedSizedBoxFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 || len(projection.Clips) != 1 {
		t.Fatalf("sized box fragments=%+v clips=%+v", projection.Fragments, projection.Clips)
	}
	fragment := projection.Fragments[0]
	if fragment.BorderBox != (layoutengine.Rect{X: 30 * 1024, Y: 30 * 1024, Width: 100 * 1024, Height: 40 * 1024}) ||
		fragment.ContentBox != (layoutengine.Rect{X: 41 * 1024, Y: 41 * 1024, Width: 78 * 1024, Height: 18 * 1024}) {
		t.Fatalf("resolved percentage/border box = %+v", fragment)
	}
	clip := projection.Clips[0]
	if projection.Paths[clip.Path].Bounds != (layoutengine.Rect{X: 31 * 1024, Y: 31 * 1024, Width: 98 * 1024, Height: 38 * 1024}) {
		t.Fatalf("overflow clip=%+v path=%+v", clip, projection.Paths[clip.Path])
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("<clipPath")) || !bytes.Contains(capture.SVG(), []byte(`fill="#ddeeff"`)) {
		t.Fatalf("sized box capture err=%v\n%s", err, capture.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-box-percent-overflow", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 {
		t.Fatalf("sized box raster=%+v status=%q err=%v", raster, status, err)
	}
	if raster.Pages[0].PNGSHA256 != "323d188d5decf114433be3eb070538a3d5e68ad3ff213018772610ad017d3ab3" {
		t.Fatalf("sized box raster drift: %s", raster.Pages[0].PNGSHA256)
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("sized box write pages=%d err=%v", pages, err)
	}
	if content := target.pages[1].Bytes(); !bytes.Contains(content, []byte(" W n\n")) {
		t.Fatalf("PDF lacks overflow clip:\n%s", content)
	}
	live := newHTMLFrameTestDocument(t, 160)
	live.SetXY(16, 42)
	html := live.HTMLNew()
	if err := html.WriteContext(context.Background(), 12, htmlUnifiedSizedBoxFixture); err != nil || live.GetY() < 89.2 || live.GetY() > 89.202 || live.PageCount() != 1 {
		t.Fatalf("sized box live cursor=%.4f pages=%d err=%v", live.GetY(), live.PageCount(), err)
	}
}

const htmlUnifiedSizedBoxFixture = `<p style="margin:5%;padding:5%;border:1pt solid #445566;` +
	`width:50%;min-width:40%;max-width:60%;height:40pt;min-height:30pt;max-height:50pt;` +
	`box-sizing:border-box;overflow:hidden;background-color:#ddeeff">Box</p>`

func TestHTMLUnifiedBoxModelCancellationConcurrentReuseAndAtomicLimits(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedSizedBoxFixture)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled box plan hash=%q pages=%d err=%v", plan.Hash(), planner.PageCount(), err)
	}

	const workers = 8
	hashes := make([]string, workers)
	errs := make([]error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer group.Done()
			candidate, candidateErr := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
			hashes[index], errs[index] = candidate.Hash(), candidateErr
		}(index)
	}
	group.Wait()
	for index := range hashes {
		if errs[index] != nil || hashes[index] == "" || hashes[index] != hashes[0] {
			t.Fatalf("box worker %d hash=%q first=%q err=%v", index, hashes[index], hashes[0], errs[index])
		}
	}

	tooWide, err := CompileHTML(`<p style="width:100%;margin:10%;padding:1pt">overflow</p>`)
	if err != nil {
		t.Fatal(err)
	}
	planner = htmlUnifiedFlexTestPlanner()
	plan, err = planner.PlanCompiledHTML(12, tooWide)
	if !errors.Is(err, ErrHTMLPlanUnsupported) || !strings.Contains(err.Error(), "exceeds its containing block") || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("over-limit box plan hash=%q pages=%d err=%v", plan.Hash(), planner.PageCount(), err)
	}
}

func TestHTMLUnifiedNestedAndMultiChildDecoratedBoxes(t *testing.T) {
	compiled, err := CompileHTML(`<section style="width:60%;padding:2%;background-color:#eeeeee">` +
		`<div style="padding:3%;border:1pt solid #222222;background-color:#dddddd"><p>A</p></div><p>B</p></section>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) < 2 || len(projection.Fills) < 4 || len(projection.SemanticFragments) < 2 || len(projection.ReadingOrder) < 2 {
		t.Fatalf("nested boxes fragments=%d fills=%d semantics=%d reading=%d", len(projection.Fragments), len(projection.Fills), len(projection.SemanticFragments), len(projection.ReadingOrder))
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`fill="#eeeeee"`)) || !bytes.Contains(capture.SVG(), []byte(`fill="#dddddd"`)) {
		t.Fatalf("nested box capture err=%v\n%s", err, capture.SVG())
	}
}

func BenchmarkHTMLUnifiedBoxModelPlanning(b *testing.B) {
	compiled, err := CompileHTML(htmlUnifiedSizedBoxFixture)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkHTMLUnifiedRoundedShadowPlanning(b *testing.B) {
	compiled, err := CompileHTML(htmlUnifiedRoundedShadowFixture)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled); err != nil {
			b.Fatal(err)
		}
	}
}
