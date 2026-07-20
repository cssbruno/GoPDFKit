// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/internal/browseroracle"
	"github.com/cssbruno/paperrune/internal/layoutengine"
)

func TestHTMLUnifiedImagesResolveIntrinsicContainCoverAlignmentReuseAndAlt(t *testing.T) {
	encoded := htmlImagePlanFixturePNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded)
	source := `<p>Before</p>` +
		`<img src="` + uri + `" alt="Intrinsic mark">` +
		`<img src="` + uri + `" alt="Contained mark" style="width:80px;height:80px;object-fit:contain;text-align:center">` +
		`<img src="` + uri + `" alt="Covered mark" style="width:80px;height:80px;object-fit:cover;text-align:right">` +
		`<img src="` + uri + `" alt="Percentage mark" style="width:50%;height:auto;max-width:100%">` +
		`<img alt="Missing source fallback">`
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
	if len(projection.ImageResources) != 1 || len(projection.Images) != 4 {
		t.Fatalf("content-addressed images = resources %d placements %d", len(projection.ImageResources), len(projection.Images))
	}
	intrinsic := projection.Images[0]
	if intrinsic.Bounds.Width.Points() != 1.5 || intrinsic.Bounds.Height.Points() != .75 || intrinsic.Crop != nil {
		t.Fatalf("intrinsic image = %+v", intrinsic)
	}
	contained := projection.Images[1]
	containedOuter := projection.Fragments[contained.Fragment-1].BorderBox
	if containedOuter.Width.Points() != 60 || containedOuter.Height.Points() != 60 || containedOuter.X.Points() != 90 ||
		contained.Bounds.Width.Points() != 60 || contained.Bounds.Height.Points() != 30 || contained.Bounds.Y != containedOuter.Y+15*1024 || contained.Crop != nil {
		t.Fatalf("contained image outer=%+v image=%+v", containedOuter, contained)
	}
	covered := projection.Images[2]
	coveredOuter := projection.Fragments[covered.Fragment-1].BorderBox
	if coveredOuter.X.Points() != 160 || covered.Bounds != coveredOuter || covered.Crop == nil || covered.Crop.Source.Width.Points() != 1 || covered.Crop.Source.Height.Points() != 1 {
		t.Fatalf("covered image outer=%+v image=%+v", coveredOuter, covered)
	}
	percentage := projection.Images[3]
	percentageOuter := projection.Fragments[percentage.Fragment-1].BorderBox
	if percentageOuter.Width.Points() != 100 || percentageOuter.Height.Points() != 50 || percentage.Bounds != percentageOuter {
		t.Fatalf("percentage image outer=%+v image=%+v", percentageOuter, percentage)
	}
	roles := map[string]bool{}
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleFigure {
			roles[node.Attributes.AlternateText] = true
		}
	}
	for _, alt := range []string{"Intrinsic mark", "Contained mark", "Covered mark", "Percentage mark"} {
		if !roles[alt] {
			t.Fatalf("missing image alternate text %q in %+v", alt, projection.SemanticNodes)
		}
	}
	var text strings.Builder
	for _, run := range projection.GlyphRuns {
		text.WriteString(run.Codes)
	}
	if !strings.Contains(text.String(), "Missing source fallback") {
		t.Fatalf("alt fallback text = %q", text.String())
	}
	firstRaster, status, err := captureCharacterizationRaster(t.Context(), "html-images-fit", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || firstRaster == nil || firstRaster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("image raster = %q %+v err=%v", status, firstRaster, err)
	}
	second, err := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil || second.Hash() != plan.Hash() {
		t.Fatalf("image plan determinism = %q/%q err=%v", plan.Hash(), second.Hash(), err)
	}
}

func TestHTMLUnifiedLocalAndCatalogImagesRequirePolicyAndSnapshotBytes(t *testing.T) {
	data := htmlImagePlanFixturePNG(t)
	directory := t.TempDir()
	path := filepath.Join(directory, "mark.png")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileHTML(`<img src="` + path + `" alt="Local" style="width:20px">`)
	if err != nil {
		t.Fatal(err)
	}
	denied := htmlUnifiedFlexTestPlanner()
	bad, err := denied.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if !errors.Is(err, ErrSecurityPolicyDenied) || bad.Hash() != "" || denied.PageCount() != 0 || denied.Error() != nil {
		t.Fatalf("denied local plan=%#v pages=%d documentErr=%v err=%v", bad, denied.PageCount(), denied.Error(), err)
	}
	allowed := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput(),
		WithSecurityPolicy(SecurityPolicy{AllowLocalHTMLImages: true}))
	allowed.SetMargins(20, 20, 20)
	plan, err := allowed.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil || len(plan.plan.Projection().ImageResources) != 1 {
		t.Fatalf("allowed local plan resources=%d err=%v", len(plan.plan.Projection().ImageResources), err)
	}
	live := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(),
		WithSecurityPolicy(SecurityPolicy{AllowLocalHTMLImages: true}))
	live.SetMargins(20, 20, 20)
	live.AddPage()
	live.SetFont("Helvetica", "", 12)
	html := live.HTMLNew()
	before := append([]byte(nil), live.pages[1].Bytes()...)
	if _, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled); !errors.Is(err, ErrSecurityPolicyDenied) || !bytes.Equal(before, live.pages[1].Bytes()) {
		t.Fatalf("live local gate mutation=%t err=%v", !bytes.Equal(before, live.pages[1].Bytes()), err)
	}
	html.AllowLocalImages = true
	if _, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled); err != nil || !bytes.Equal(before, live.pages[1].Bytes()) {
		t.Fatalf("live local allowed snapshot mutation=%t err=%v", !bytes.Equal(before, live.pages[1].Bytes()), err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, len(data)), 0o600); err != nil {
		t.Fatal(err)
	}
	target := htmlUnifiedFlexTestPlanner()
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatalf("retained local snapshot reopened changed path: %v", err)
	}

	var opens atomic.Int64
	catalog := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithSecurityPolicy(SecurityPolicy{AllowLocalHTMLImages: true}), WithResourceLoader(ResourceLoaderFunc(
		func(ctx context.Context, kind ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
			opens.Add(1)
			if kind != ResourceImage || name != "catalog-mark" {
				return nil, ResourceInfo{}, errors.New("unexpected resource")
			}
			return io.NopCloser(bytes.NewReader(data)), ResourceInfo{Size: int64(len(data)), StableID: "mark-v1"}, nil
		})))
	catalogCompiled, err := CompileHTML(`<img src="catalog-mark" alt="Catalog one"><img src="catalog-mark" alt="Catalog two">`)
	if err != nil {
		t.Fatal(err)
	}
	first, err := catalog.PlanCompiledHTMLContext(context.Background(), 12, catalogCompiled)
	if err != nil || first.Hash() == "" || opens.Load() != 1 || len(first.plan.Projection().ImageResources) != 1 || len(first.plan.Projection().Images) != 2 {
		t.Fatalf("catalog plan hash=%q opens=%d err=%v", first.Hash(), opens.Load(), err)
	}
}

func TestHTMLUnifiedJPEGDataImageUsesIntrinsicGeometry(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 1))
	canvas.Set(0, 0, color.RGBA{R: 240, G: 180, B: 30, A: 255})
	canvas.Set(1, 0, color.RGBA{R: 30, G: 120, B: 220, A: 255})
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, canvas, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	uri := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())
	compiled, err := CompileHTML(`<img src="` + uri + `" alt="JPEG swatch">`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || projection.ImageResources[0].Format != layoutengine.ImageJPEG ||
		len(projection.Images) != 1 || projection.Images[0].Bounds.Width.Points() != 1.5 || projection.Images[0].Bounds.Height.Points() != .75 {
		t.Fatalf("JPEG unified plan resources=%+v images=%+v", projection.ImageResources, projection.Images)
	}
}

func TestHTMLUnifiedFigureCaptionSemanticStructureCursorRasterAndPDF(t *testing.T) {
	data := htmlImagePlanFixturePNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	source := `<style>.hero{width:96px;height:48px;object-fit:contain;text-align:center} figcaption{font-size:10pt;color:#334455}</style>` +
		`<figure><img class="hero" src="` + uri + `" alt="Two color swatch">` +
		`<figcaption>Verified <a href="https://example.test/figure">figure caption</a></figcaption></figure>`
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
	var documentNode, figure, caption layoutengine.SemanticNodeID
	for _, node := range projection.SemanticNodes {
		switch node.Role {
		case layoutengine.SemanticRoleDocument:
			documentNode = node.ID
		case layoutengine.SemanticRoleFigure:
			if node.Attributes.AlternateText == "Two color swatch" {
				figure = node.ID
			}
		case layoutengine.SemanticRoleParagraph:
			caption = node.ID
		}
	}
	if !documentNode.Valid() || !figure.Valid() || !caption.Valid() || projection.SemanticNodes[figure-1].Parent != documentNode || projection.SemanticNodes[caption-1].Parent != figure || len(projection.Links) != 1 {
		t.Fatalf("figure semantic structure nodes=%+v links=%+v", projection.SemanticNodes, projection.Links)
	}
	captionStyled := false
	for _, run := range projection.GlyphRuns {
		if strings.Contains(run.Codes, "Verified") && run.FontSize.Points() == 10 && run.Color == (layoutengine.CoreRGBColor{R: 0x33, G: 0x44, B: 0x55, Set: true}) {
			captionStyled = true
		}
	}
	if !captionStyled {
		t.Fatalf("resolved figcaption style is absent from glyph runs: %+v", projection.GlyphRuns)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "html-figure-caption", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil {
		t.Fatalf("figure raster = %q %+v err=%v", status, raster, err)
	}
	render := func() ([]byte, float64) {
		target := htmlUnifiedFlexTestPlanner()
		if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
			t.Fatal(err)
		}
		return output.Bytes(), target.GetY()
	}
	firstPDF, firstY := render()
	secondPDF, secondY := render()
	if firstY != 20 || firstY != secondY || !bytes.Equal(firstPDF, secondPDF) || !bytes.Contains(firstPDF, []byte("/Subtype /Image")) {
		t.Fatalf("figure cursor/PDF y=%.3f/%.3f bytes=%d/%d image=%t", firstY, secondY, len(firstPDF), len(secondPDF), bytes.Contains(firstPDF, []byte("/Subtype /Image")))
	}
	live := htmlUnifiedFlexTestPlanner()
	live.AddPage()
	live.SetFont("Helvetica", "", 12)
	live.SetXY(20, 20)
	html := live.HTMLNew()
	fragment, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatalf("unified live figure plan: %v", err)
	}
	if err := html.WriteContext(context.Background(), 12, source); err != nil {
		t.Fatal(err)
	}
	var plannedBottom layoutengine.Fixed
	for _, fragment := range projection.Fragments {
		bottom, bottomErr := fragment.BorderBox.Bottom()
		if bottomErr != nil {
			t.Fatal(bottomErr)
		}
		if bottom > plannedBottom {
			plannedBottom = bottom
		}
	}
	wantY := fragment.final.y
	if live.GetY() != wantY || live.GetY() < plannedBottom.Points() || live.GetY() > plannedBottom.Points()+1.0/1024+1e-9 {
		t.Fatalf("figure cursor compatibility planned_bottom=%.3f final=%.6f live_y=%.6f", plannedBottom.Points(), wantY, live.GetY())
	}
}

func TestHTMLUnifiedFigureFrameMovesImageAndCaptionTogether(t *testing.T) {
	data := htmlImagePlanFixturePNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	source := `<figure><img src="` + uri + `" alt="Kept figure" style="width:80px;height:40px"><figcaption>Kept caption</figcaption></figure>`
	compiled, err := CompileHTML(source)
	if err != nil {
		t.Fatal(err)
	}
	pdf := newHTMLFrameTestDocument(t, 160)
	pdf.SetXY(16, 120)
	html := pdf.HTMLNew()
	fragment, err := html.planCompiledHTMLFragmentContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	if fragment.reuseCurrentPage || fragment.final.page != 2 {
		t.Fatalf("figure frame placement = reuse %t final page %d", fragment.reuseCurrentPage, fragment.final.page)
	}
	projection := fragment.plan.plan.Projection()
	if len(projection.Fragments) != 2 || projection.Fragments[0].Page != 1 || projection.Fragments[1].Page != 1 {
		t.Fatalf("kept figure fragments = %+v", projection.Fragments)
	}
	if err := html.WriteContext(context.Background(), 12, source); err != nil {
		t.Fatal(err)
	}
	if pdf.PageNo() != 2 || pdf.GetX() != 16 || pdf.GetY() != fragment.final.y {
		t.Fatalf("figure exit frame = page %d x %.3f y %.6f want page 2 x 16 y %.6f", pdf.PageNo(), pdf.GetX(), pdf.GetY(), fragment.final.y)
	}
}

func TestHTMLUnifiedImagesRejectMalformedOversizedUnsafeAndCancelAtomically(t *testing.T) {
	data := htmlImagePlanFixturePNG(t)
	validURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	tests := []string{
		`<img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString([]byte("not png")) + `" alt="bad">`,
		`<img src="https://example.test/remote.png" alt="remote">`,
		`<img src="file:///tmp/image.png" alt="file URL">`,
		`<img src="ftp://example.test/image.png" alt="other URL">`,
		`<img src="` + validURI + `" style="height:50%">`,
		`<img src="` + validURI + `" style="object-fit:scale-down">`,
		`<img>`,
	}
	for _, source := range tests {
		compiled, compileErr := CompileHTML(source)
		if compileErr != nil {
			continue
		}
		planner := htmlUnifiedFlexTestPlanner()
		plan, planErr := planner.PlanCompiledHTMLContext(context.Background(), 12, compiled)
		if planErr == nil || plan.Hash() != "" || planner.PageCount() != 0 || planner.Error() != nil {
			t.Fatalf("malformed image %q plan=%#v pages=%d documentErr=%v err=%v", source, plan, planner.PageCount(), planner.Error(), planErr)
		}
	}

	limited := MustNew(WithUnit(UnitPoint), WithLimits(Limits{MaxImageSourceBytes: 8}), WithSecurityPolicy(SecurityPolicy{AllowLocalHTMLImages: true}))
	limited.SetResourceLoader(ResourceLoaderFunc(func(context.Context, ResourceKind, string) (io.ReadCloser, ResourceInfo, error) {
		return io.NopCloser(bytes.NewReader(data)), ResourceInfo{Size: int64(len(data))}, nil
	}))
	compiled, _ := CompileHTML(`<img src="oversized.png" alt="large">`)
	plan, err := limited.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err == nil || plan.Hash() != "" || limited.PageCount() != 0 || limited.Error() != nil {
		t.Fatalf("oversized image plan=%#v pages=%d documentErr=%v err=%v", plan, limited.PageCount(), limited.Error(), err)
	}

	other := htmlImagePlanSolidPNG(t, color.RGBA{R: 40, G: 180, B: 70, A: 255})
	cumulativeLimit := len(data) + len(other) - 1
	cumulative := MustNew(WithUnit(UnitPoint), WithLimits(Limits{MaxImageSourceBytes: int64(cumulativeLimit)}),
		WithSecurityPolicy(SecurityPolicy{AllowLocalHTMLImages: true}), WithResourceLoader(ResourceLoaderFunc(
			func(_ context.Context, _ ResourceKind, name string) (io.ReadCloser, ResourceInfo, error) {
				payload := data
				if name == "second.png" {
					payload = other
				}
				return io.NopCloser(bytes.NewReader(payload)), ResourceInfo{Size: int64(len(payload))}, nil
			})))
	compiled, _ = CompileHTML(`<img src="first.png" alt="first"><img src="second.png" alt="second">`)
	plan, err = cumulative.PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err == nil || plan.Hash() != "" || cumulative.PageCount() != 0 || cumulative.Error() != nil {
		t.Fatalf("cumulative image limit plan=%#v pages=%d documentErr=%v err=%v", plan, cumulative.PageCount(), cumulative.Error(), err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := htmlUnifiedFlexTestPlanner()
	compiled, _ = CompileHTML(`<img src="` + validURI + `" alt="cancel">`)
	plan, err = planner.PlanCompiledHTMLContext(canceled, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled image plan=%#v pages=%d err=%v", plan, planner.PageCount(), err)
	}

	loadContext, cancelLoad := context.WithCancel(context.Background())
	loading := MustNew(WithUnit(UnitPoint), WithSecurityPolicy(SecurityPolicy{AllowLocalHTMLImages: true}),
		WithResourceLoader(ResourceLoaderFunc(func(context.Context, ResourceKind, string) (io.ReadCloser, ResourceInfo, error) {
			cancelLoad()
			return io.NopCloser(bytes.NewReader(data)), ResourceInfo{Size: int64(len(data))}, nil
		})))
	compiled, _ = CompileHTML(`<img src="cancel-during-load.png" alt="cancel load">`)
	plan, err = loading.PlanCompiledHTMLContext(loadContext, 12, compiled)
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || loading.PageCount() != 0 || loading.Error() != nil {
		t.Fatalf("load-canceled image plan=%#v pages=%d documentErr=%v err=%v", plan, loading.PageCount(), loading.Error(), err)
	}
}

func TestHTMLUnifiedImageCompiledReuseIsConcurrentAndDetached(t *testing.T) {
	data := htmlImagePlanFixturePNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	compiled, err := CompileHTML(`<img src="` + uri + `" alt="Concurrent" style="width:32px;object-fit:contain">`)
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
			t.Fatalf("worker %d hash=%q want=%q err=%v", index, hashes[index], hashes[0], errs[index])
		}
	}
	if source, ok := compiled.dataImage(0); !ok || len(source.data) == 0 {
		t.Fatal("compiled source image disappeared")
	}
}

func TestHTMLUnifiedImagePinnedBrowserGeometryAndRaster(t *testing.T) {
	data := htmlImagePlanFixturePNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	compiled, err := CompileHTML(`<img src="` + uri + `" alt="Browser parity swatch" style="width:80px;height:80px;object-fit:contain;text-align:center">`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	browser := `<style>html,body{margin:0;width:320px;height:213px;overflow:hidden;background:#fff}` +
		`body{position:relative;margin-left:26.6666667px;margin-top:26.6666667px;width:266.6666667px}` +
		`#target{box-sizing:border-box;display:block;width:80px;height:80px;object-fit:contain;margin:0 auto}</style>` +
		`<img id="target" src="` + uri + `" alt="Browser parity swatch">`
	capture, err := browseroracle.CaptureFirefox(t.Context(), browser,
		`(()=>{const r=document.querySelector("#target").getBoundingClientRect();return[{id:"target",x:r.x,y:r.y,width:r.width,height:r.height}]})()`,
		browseroracle.Options{Width: 320, Height: 213, Timeout: 15 * time.Second})
	if errors.Is(err, browseroracle.ErrBrowserUnavailable) {
		t.Skipf("pinned external browser oracle unavailable: %v", err)
	}
	if err != nil {
		t.Fatal(err)
	}
	var rects []browserFlexRect
	if err := json.Unmarshal(capture.DOMRects, &rects); err != nil || len(rects) != 1 {
		t.Fatalf("browser DOMRect = %s err=%v", capture.DOMRects, err)
	}
	outer := plan.plan.Projection().Fragments[0].BorderBox
	want := []float64{outer.X.Points(), outer.Y.Points(), outer.Width.Points(), outer.Height.Points()}
	got := []float64{rects[0].X * .75, rects[0].Y * .75, rects[0].Width * .75, rects[0].Height * .75}
	for axis := range want {
		if math.Abs(want[axis]-got[axis]) > 1.0/1024 {
			t.Fatalf("browser image axis %d = %.6fpt, plan %.6fpt; rect=%s", axis, got[axis], want[axis], capture.DOMRects)
		}
	}
	profile := layoutengine.DefaultDisplayRasterProfile()
	profile.DPI = 96
	images := make(layoutengine.DisplaySVGImageSources, len(plan.imageSources))
	for digest, source := range plan.imageSources {
		images[digest] = source
	}
	artifact, err := layoutengine.CaptureDisplayPlanPNGContext(t.Context(), plan.plan, layoutengine.DisplayRasterSources{Images: images}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: profile, Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: characterizationDigest("browser-image-page-profile"),
		Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: characterizationDigest("browser-image-source"), ScenarioRevision: characterizationDigest("browser-image-scenario"), PolicyRevision: characterizationDigest("browser-image-policy")},
	})
	if err != nil {
		t.Fatal(err)
	}
	changed, total, maxDelta, mean := compareFlexPNGs(t, artifact.PNG(), capture.PNG)
	t.Logf("Firefox %s image raster changed=%d/%d max_channel_delta=%d mean_channel_delta=%.4f", capture.Version, changed, total, maxDelta, mean)
	// The 2x1 source is magnified through independent browser and Go image
	// resamplers. Geometry is exact above; this separate calibrated pixel gate
	// permits their edge-filter difference while bounding whole-page drift.
	if changed*1000 > total*25 || maxDelta > 128 || mean > 1.5 {
		t.Fatalf("browser image raster parity changed=%d/%d max=%d mean=%.4f", changed, total, maxDelta, mean)
	}
}

func BenchmarkHTMLUnifiedImagePlanning(b *testing.B) {
	data := htmlImagePlanFixturePNG(b)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	compiled, err := CompileHTML(`<figure><img src="` + uri + `" alt="Benchmark" style="width:96px;height:48px;object-fit:cover"><figcaption>Caption</figcaption></figure>`)
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

func TestHTMLUnifiedImageVisualFixture(t *testing.T) {
	destination := os.Getenv("PAPERRUNE_IMAGE_FIXTURE_PDF")
	if destination == "" {
		t.Skip("set PAPERRUNE_IMAGE_FIXTURE_PDF to write the reviewed image/figure PDF")
	}
	data := htmlImagePlanFixturePNG(t)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	source := `<style>.swatch{width:120px;height:72px;object-fit:contain;text-align:center} figcaption{font-size:10pt;color:#334455}</style>` +
		`<h2>Unified image and figure</h2><p>Intrinsic, fitted, semantic, and deterministic.</p>` +
		`<figure><img class="swatch" src="` + uri + `" alt="Red and blue verification swatch">` +
		`<figcaption>Figure 1 - contained source with exact caption geometry.</figcaption></figure>`
	compiled, err := CompileHTML(source)
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

func htmlImagePlanFixturePNG(t testing.TB) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 2, 1))
	canvas.Set(0, 0, color.RGBA{R: 220, G: 40, B: 40, A: 255})
	canvas.Set(1, 0, color.RGBA{R: 20, G: 80, B: 220, A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func htmlImagePlanSolidPNG(t testing.TB, fill color.RGBA) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, fill)
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
