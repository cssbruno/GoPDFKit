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
)

const htmlUnifiedSVGFixture = `<a href="https://example.test/svg"><svg width="40" height="30" viewBox="0 0 40 30" aria-label="Status icon">` +
	`<rect x="1" y="2" width="12" height="8" fill="#112233" stroke="none"/>` +
	`<path d="M20 4 Q28 4 28 12 Z" fill="#abcdef" fill-rule="evenodd" stroke="none"/></svg></a>`

func TestHTMLUnifiedInlineSVGPlanCaptureRasterPDFLinkSemanticsAndCursor(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedSVGFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 || projection.Fragments[0].BorderBox != (layoutengine.Rect{
		X: 20 * 1024, Y: 20 * 1024, Width: 30 * 1024, Height: 23040,
	}) {
		t.Fatalf("SVG fragment=%+v", projection.Fragments)
	}
	if len(projection.Paths) != 2 || len(projection.Fills) != 2 || len(projection.Strokes) != 0 || len(projection.Commands) != 3 {
		t.Fatalf("SVG display paths=%d fills=%d strokes=%d commands=%+v", len(projection.Paths), len(projection.Fills), len(projection.Strokes), projection.Commands)
	}
	if len(projection.Links) != 1 || projection.Links[0].URI != "https://example.test/svg" || projection.Links[0].Bounds != projection.Fragments[0].BorderBox {
		t.Fatalf("SVG links=%+v", projection.Links)
	}
	if len(projection.SemanticNodes) != 3 || projection.SemanticNodes[1].Role != layoutengine.SemanticRoleLink ||
		projection.SemanticNodes[2].Role != layoutengine.SemanticRoleFigure || projection.SemanticNodes[2].Attributes.AlternateText != "Status icon" ||
		len(projection.SemanticFragments) != 1 || len(projection.ReadingOrder) != 1 {
		t.Fatalf("SVG semantics=%+v/%+v/%+v", projection.SemanticNodes, projection.SemanticFragments, projection.ReadingOrder)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`fill="#112233"`)) || !bytes.Contains(capture.SVG(), []byte(`fill-rule="evenodd"`)) {
		t.Fatalf("display capture err=%v\n%s", err, capture.SVG())
	}
	// The characterization rasterizer intentionally excludes annotations and
	// even-odd fills. Pin the supported opaque nonzero-fill visual cohort here;
	// the linked/even-odd plan above remains covered by capture and PDF evidence.
	unlinkedCompiled, err := CompileHTML(`<svg width="40" height="30" viewBox="0 0 40 30" aria-label="Status icon">` +
		`<rect x="1" y="2" width="12" height="8" fill="#112233" stroke="none"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	unlinkedPlan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, unlinkedCompiled)
	if err != nil {
		t.Fatal(err)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-inline-svg", unlinkedPlan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 {
		t.Fatalf("raster=%+v status=%q err=%v", raster, status, err)
	}
	if got := raster.Pages[0].PNGSHA256; got != "6ee944113d97e20b4593dc425fc2f8d52f04b2f8ca0779c20c7fb4f07259283d" {
		t.Fatalf("SVG raster drift: %s", got)
	}

	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("write pages=%d err=%v", pages, err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil || output.Len() == 0 ||
		!bytes.Contains(output.Bytes(), []byte("/Subtype /Link")) || !bytes.Contains(output.Bytes(), []byte(" f")) {
		t.Fatalf("SVG PDF bytes=%d err=%v", output.Len(), err)
	}

	live := newHTMLFrameTestDocument(t, 160)
	live.SetXY(16, 42)
	html := live.HTMLNew()
	if err := html.WriteContext(context.Background(), 12, htmlUnifiedSVGFixture); err != nil || live.GetY() <= 42 || live.PageCount() != 1 {
		t.Fatalf("SVG live cursor=%.4f pages=%d err=%v", live.GetY(), live.PageCount(), err)
	}
}

func TestHTMLUnifiedInlineSVGDecorativeSemantics(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="10" height="10" role="presentation" aria-hidden="true"><rect width="10" height="10" fill="red" stroke="none"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.SemanticNodes) != 2 || projection.SemanticNodes[1].Role != layoutengine.SemanticRoleArtifact ||
		len(projection.SemanticFragments) != 1 || len(projection.ReadingOrder) != 0 {
		t.Fatalf("decorative semantics=%+v/%+v/%+v", projection.SemanticNodes, projection.SemanticFragments, projection.ReadingOrder)
	}
}

func TestHTMLUnifiedMultipleSVGAndMixedFlowPlanCaptureRasterPDFAndCursor(t *testing.T) {
	source := `<p>Before</p>` +
		`<svg width="20" height="12" viewBox="0 0 20 12" aria-label="Red mark"><rect x="1" y="1" width="18" height="10" fill="#cc2030" stroke="none"/></svg>` +
		`<p>Between</p>` +
		`<svg width="16" height="10" viewBox="0 0 16 10" role="presentation" aria-hidden="true"><circle cx="8" cy="5" r="4" fill="#2050cc" stroke="none"/></svg>` +
		`<p>After</p>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	again, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil || again.Hash() != plan.Hash() {
		t.Fatalf("mixed SVG deterministic hash=%q/%q err=%v", plan.Hash(), again.Hash(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Paths) != 2 || len(projection.Fills) != 2 || len(projection.ImageResources) != 0 || len(projection.Images) != 0 {
		t.Fatalf("mixed SVG display paths=%d fills=%d image_resources=%d images=%d", len(projection.Paths), len(projection.Fills), len(projection.ImageResources), len(projection.Images))
	}
	figures, artifacts := 0, 0
	for _, semantic := range projection.SemanticNodes {
		switch semantic.Role {
		case layoutengine.SemanticRoleFigure:
			figures++
			if semantic.Attributes.AlternateText != "Red mark" {
				t.Fatalf("mixed SVG informative semantics=%+v", semantic)
			}
		case layoutengine.SemanticRoleArtifact:
			artifacts++
		}
	}
	if figures != 1 || artifacts != 1 || len(projection.ReadingOrder) != 4 {
		t.Fatalf("mixed SVG semantic figures=%d artifacts=%d reading=%+v", figures, artifacts, projection.ReadingOrder)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`fill="#cc2030"`)) || !bytes.Contains(capture.SVG(), []byte(`fill="#2050cc"`)) || bytes.Contains(capture.SVG(), []byte(`<image`)) {
		t.Fatalf("mixed SVG capture err=%v\n%s", err, capture.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-multiple-mixed-svg", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("mixed SVG raster=%+v status=%q err=%v", raster, status, err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("mixed SVG PDF pages=%d err=%v", pages, err)
	}
	if content := target.pages[1].Bytes(); !bytes.Contains(content, []byte("0.8000000000 0.1254901961 0.1882352941 rg")) || !bytes.Contains(content, []byte("0.1254901961 0.3137254902 0.8000000000 rg")) {
		t.Fatalf("mixed SVG PDF content lacks vector paints:\n%s", content)
	}

	live := newHTMLFrameTestDocument(t, 160)
	live.SetXY(16, 32)
	html := live.HTMLNew()
	if err := html.WriteContext(t.Context(), 12, source); err != nil || live.PageCount() != 1 || live.GetY() <= 32 {
		t.Fatalf("mixed SVG live pages=%d y=%.4f err=%v", live.PageCount(), live.GetY(), err)
	}
}

func TestHTMLUnifiedMixedSVGPreservesOrdinaryImageResources(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	compiled, err := CompileHTML(`<p>Mixed resources</p>` +
		`<svg width="12" height="8" aria-label="Vector mark"><rect width="12" height="8" fill="#408020" stroke="none"/></svg>` +
		`<img src="data:image/png;base64,` + pixel + `" width="8" height="6" alt="Raster mark">`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Paths) != 1 || len(projection.Fills) != 1 || len(projection.ImageResources) != 1 || len(projection.Images) != 1 || len(plan.imageSources) != 1 {
		t.Fatalf("mixed resource projection paths=%d fills=%d resources=%d images=%d sources=%d", len(projection.Paths), len(projection.Fills), len(projection.ImageResources), len(projection.Images), len(plan.imageSources))
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`fill="#408020"`)) || !bytes.Contains(capture.SVG(), []byte(`<image`)) {
		t.Fatalf("mixed resource capture err=%v\n%s", err, capture.SVG())
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("mixed resource PDF pages=%d err=%v", pages, err)
	}
}

func TestHTMLUnifiedInlineSVGRejectsUnsupportedSubsetAtomically(t *testing.T) {
	tests := []struct{ name, source, diagnostic string }{
		{"missing-accessibility", `<svg width="10" height="10"><rect width="10" height="10" fill="red"/></svg>`, "requires aria-label"},
		{"decorative-label-conflict", `<svg width="10" height="10" role="presentation" aria-label="x"><rect width="10" height="10" fill="red"/></svg>`, "cannot also"},
		{"text", `<svg width="10" height="10" aria-label="x"><text x="1" y="8">x</text></svg>`, "unsupported"},
		{"gradient", `<svg width="10" height="10" aria-label="x"><defs><linearGradient id="g"><stop offset="0" stop-color="red"/></linearGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`, "unsupported"},
		{"partial-internal-link", `<svg width="10" height="10" aria-label="x"><a href="https://example.test"><rect width="5" height="10" fill="red"/></a><rect x="5" width="5" height="10" fill="blue"/></svg>`, "every drawable"},
		{"mixed-rich-text", `<p>outside</p><svg width="10" height="10" aria-label="x"><text x="1" y="8">x</text></svg>`, "sole content"},
		{"unsafe-link", `<a href="javascript:alert(1)"><svg width="10" height="10" aria-label="x"><rect width="10" height="10" fill="red"/></svg></a>`, "external"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(test.source)
			if err != nil {
				if test.name == "unsafe-link" {
					return
				}
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

func TestHTMLUnifiedInlineSVGOpacityParity(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="18" height="12" aria-label="Translucent status"><rect x="1" y="1" width="16" height="10" fill="#204080" fill-opacity=".5"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fills) != 1 || projection.Fills[0].Opacity != 512 {
		t.Fatalf("opacity fill = %+v", projection.Fills)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`fill-opacity="0.5"`)) {
		t.Fatalf("opacity capture err=%v\n%s", err, capture.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-inline-svg-opacity", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 {
		t.Fatalf("opacity raster=%+v status=%q err=%v", raster, status, err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("opacity write pages=%d err=%v", pages, err)
	}
	if !bytes.Contains(target.pages[1].Bytes(), []byte(" gs\n")) {
		t.Fatal("opacity PDF content lacks ExtGState selection")
	}
}

func TestHTMLUnifiedInlineSVGDiagonalTranslucentGradient(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="32" height="20" aria-label="Diagonal status"><defs><linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0" stop-color="#ff2000" stop-opacity=".3"/><stop offset="1" stop-color="#2040ff" stop-opacity=".85"/></linearGradient></defs><rect x="2" y="2" width="28" height="16" fill="url(#g)"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Clips) != 1 || len(projection.Fills) != svgDisplayGradientBands || len(projection.Paths) != 1+svgDisplayGradientBands {
		t.Fatalf("HTML diagonal gradient resources paths=%d clips=%d fills=%d", len(projection.Paths), len(projection.Clips), len(projection.Fills))
	}
	if projection.Fills[0].Opacity <= 0 || projection.Fills[len(projection.Fills)-1].Opacity <= projection.Fills[0].Opacity {
		t.Fatalf("HTML diagonal gradient opacity range=%+v/%+v", projection.Fills[0], projection.Fills[len(projection.Fills)-1])
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || bytes.Count(capture.SVG(), []byte(`fill-opacity=`)) != svgDisplayGradientBands {
		t.Fatalf("HTML diagonal gradient capture err=%v\n%s", err, capture.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-inline-svg-diagonal-gradient", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("HTML diagonal gradient raster=%+v status=%q err=%v", raster, status, err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("HTML diagonal gradient write pages=%d err=%v", pages, err)
	}
	if bytes.Count(target.pages[1].Bytes(), []byte(" gs\n")) < svgDisplayGradientBands {
		t.Fatal("HTML diagonal gradient PDF lacks per-band opacity graphics state")
	}
}

func TestHTMLUnifiedInlineSVGCenteredRadialGradient(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="30" height="20" aria-label="Radial status"><defs><radialGradient id="g"><stop offset="0" stop-color="#fff080" stop-opacity=".9"/><stop offset="1" stop-color="#202060" stop-opacity=".4"/></radialGradient></defs><rect x="2" y="2" width="26" height="16" fill="url(#g)"/></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Clips) != 1 || len(projection.Fills) != svgDisplayGradientBands || len(projection.Paths) != 1+svgDisplayGradientBands {
		t.Fatalf("HTML radial resources paths=%d clips=%d fills=%d", len(projection.Paths), len(projection.Clips), len(projection.Fills))
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || bytes.Count(capture.SVG(), []byte(`fill-rule="nonzero"`)) != svgDisplayGradientBands {
		t.Fatalf("HTML radial capture err=%v\n%s", err, capture.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-inline-svg-radial-gradient", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("HTML radial raster=%+v status=%q err=%v", raster, status, err)
	}
}

func TestHTMLUnifiedInlineSVGRichContentAndInternalLink(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="48" height="24" aria-label="Rich icon"><a href="https://example.test/rich">` +
		`<rect width="18" height="14" fill="#336699"/><text x="20" y="10" font-size="8" fill="#112233">OK</text>` +
		`<image x="20" y="12" width="8" height="8" href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="/>` +
		`</a></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.GlyphRuns) != 1 || len(projection.Images) != 1 || len(projection.ImageResources) != 1 || len(plan.imageSources) != 1 ||
		len(projection.Links) != 1 || projection.Links[0].URI != "https://example.test/rich" {
		t.Fatalf("rich HTML SVG resources: glyphs=%d images=%d resources=%d sources=%d links=%+v", len(projection.GlyphRuns), len(projection.Images), len(projection.ImageResources), len(plan.imageSources), projection.Links)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`data:image/png;base64,`)) || !bytes.Contains(capture.SVG(), []byte(`>O</text>`)) {
		t.Fatalf("rich capture err=%v\n%s", err, capture.SVG())
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("rich write pages=%d err=%v", pages, err)
	}
}

func TestHTMLUnifiedInlineSVGTextAndImageOpacityParity(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="44" height="22" aria-label="Translucent content">` +
		`<rect width="44" height="22" fill="#ffffff"/>` +
		`<text x="2" y="10" font-size="8" fill="#204080" opacity=".5">soft</text>` +
		`<image x="24" y="4" width="12" height="12" opacity=".375" href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="/>` +
		`</svg>`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.GlyphRuns) != 1 || projection.GlyphRuns[0].Opacity != 512 || len(projection.Images) != 1 || projection.Images[0].Opacity != 384 {
		t.Fatalf("text/image opacity runs=%+v images=%+v", projection.GlyphRuns, projection.Images)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`fill-opacity="0.5"`)) || !bytes.Contains(capture.SVG(), []byte(`opacity="0.375"`)) {
		t.Fatalf("text/image opacity capture err=%v\n%s", err, capture.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-inline-svg-text-image-opacity", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("text/image opacity raster=%+v status=%q err=%v", raster, status, err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("text/image opacity write pages=%d err=%v", pages, err)
	}
	if bytes.Count(target.pages[1].Bytes(), []byte(" gs\n")) < 2 {
		t.Fatal("text/image opacity PDF lacks both opacity graphics states")
	}
}

func TestHTMLUnifiedInlineSVGCancellationAndConcurrentReuse(t *testing.T) {
	compiled, err := CompileHTML(`<svg width="30" height="16" aria-label="Concurrent"><text x="2" y="11" font-size="9" fill="#112233">safe</text></svg>`)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := htmlUnifiedFlexTestPlanner()
	plan, err := planner.PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled plan hash=%q pages=%d err=%v", plan.Hash(), planner.PageCount(), err)
	}

	const workers = 8
	hashes := make([]string, workers)
	errorsByWorker := make([]error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for index := 0; index < workers; index++ {
		go func(index int) {
			defer group.Done()
			candidate, candidateErr := htmlUnifiedFlexTestPlanner().PlanCompiledHTML(12, compiled)
			errorsByWorker[index] = candidateErr
			hashes[index] = candidate.Hash()
		}(index)
	}
	group.Wait()
	for index := 0; index < workers; index++ {
		if errorsByWorker[index] != nil || hashes[index] == "" || hashes[index] != hashes[0] {
			t.Fatalf("worker %d hash=%q first=%q err=%v", index, hashes[index], hashes[0], errorsByWorker[index])
		}
	}
}
