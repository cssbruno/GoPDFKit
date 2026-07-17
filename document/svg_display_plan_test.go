// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

func TestAttachSVGDisplayPlanCanonicalPlanCaptureRasterAndPDF(t *testing.T) {
	svg, err := SVGParse([]byte(`<svg width="40" height="30" viewBox="0 0 40 30"><g transform="translate(2 3)"><rect x="1" y="2" width="12" height="8" fill="#112233" stroke="#c86432" stroke-width="2"/><path d="M20 4 Q28 4 28 12 Z" fill="#abcdef" fill-rule="evenodd" stroke="none"/></g></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{
		Number: 1, Size: layoutengine.Size{Width: 100 * layoutengine.Fixed(layoutengine.FixedScale), Height: 80 * layoutengine.Fixed(layoutengine.FixedScale)},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	placement := SVGDisplayPlanPlacement{Page: 1, X: 10, Y: 5, Scale: 2}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, placement)
	if err != nil {
		t.Fatal(err)
	}
	again, err := AttachSVGDisplayPlan(geometry, &svg, placement)
	if err != nil {
		t.Fatal(err)
	}
	firstHash, _ := plan.Hash()
	secondHash, _ := again.Hash()
	if firstHash != secondHash {
		t.Fatalf("canonical hashes differ: %s != %s", firstHash, secondHash)
	}
	projection := plan.Projection()
	if len(projection.Paths) != 2 || len(projection.Fills) != 2 || len(projection.Strokes) != 1 || len(projection.Commands) != 3 {
		t.Fatalf("lowered resources = paths %d fills %d strokes %d commands %d", len(projection.Paths), len(projection.Fills), len(projection.Strokes), len(projection.Commands))
	}
	wantBounds := layoutengine.Rect{X: 16 * 1024, Y: 15 * 1024, Width: 24 * 1024, Height: 16 * 1024}
	if projection.Paths[0].Bounds != wantBounds || projection.Fills[0].Color != (layoutengine.CoreRGBColor{R: 17, G: 34, B: 51, Set: true}) || projection.Strokes[0].Width != 4*1024 {
		t.Fatalf("first path = %+v fill=%+v stroke=%+v", projection.Paths[0], projection.Fills[0], projection.Strokes[0])
	}
	if projection.Paths[1].Segments[1].Kind != layoutengine.PathCubicTo || projection.Fills[1].Rule != layoutengine.FillEvenOdd {
		t.Fatalf("quadratic/fill lowering = %+v %+v", projection.Paths[1].Segments, projection.Fills[1])
	}
	svg.Elements[0].Path.Segments[0].Arg[0] = 999
	if detached := plan.Projection().Paths[0].Segments[0].Point.X; detached != 16*1024 {
		t.Fatalf("plan retained mutable SVG storage: x=%d", detached)
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range [][]byte{[]byte(`fill="#112233"`), []byte(`stroke="#c86432"`), []byte(`fill-rule="evenodd"`)} {
		if !bytes.Contains(capture.SVG, token) {
			t.Fatalf("SVG capture lacks %q:\n%s", token, capture.SVG)
		}
	}
	rasterSVG, err := SVGParse([]byte(`<svg width="20" height="20"><rect x="2" y="3" width="12" height="10" fill="#112233"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	rasterPlan, err := AttachSVGDisplayPlan(geometry, &rasterSVG, SVGDisplayPlanPlacement{Page: 1, X: 10, Y: 5, Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	raster, err := layoutengine.CaptureDisplayPlanPNG(rasterPlan, layoutengine.DisplayRasterSources{}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(),
		PageProfile: strings.Repeat("a", 64), Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	})
	if err != nil || len(raster.PNG()) == 0 {
		t.Fatalf("raster = %d bytes, %v", len(raster.PNG()), err)
	}
	rasterAgain, err := layoutengine.CaptureDisplayPlanPNG(rasterPlan, layoutengine.DisplayRasterSources{}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(),
		PageProfile: strings.Repeat("a", 64), Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	})
	if err != nil || !bytes.Equal(raster.PNG(), rasterAgain.PNG()) {
		t.Fatalf("raster determinism = %v, equal=%t", err, bytes.Equal(raster.PNG(), rasterAgain.PNG()))
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	for _, token := range [][]byte{[]byte("0.0666666667 0.1333333333 0.2000000000 rg"), []byte("0.7843137255 0.3921568627 0.1960784314 RG"), []byte(" f*\n")} {
		if !bytes.Contains(content, token) {
			t.Fatalf("PDF content lacks %q:\n%s", token, content)
		}
	}
	var output bytes.Buffer
	if err := target.Output(&output); err != nil || !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatalf("PDF = %d bytes, %v", output.Len(), err)
	}
}

func TestAttachSVGDisplayPlanRejectsUnsupportedAndLimitsAtomically(t *testing.T) {
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{
		Number: 1, Size: layoutengine.Size{Width: 100 * 1024, Height: 100 * 1024},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	originalHash, _ := geometry.Hash()
	cases := []struct {
		name   string
		source string
		limits SVGDisplayPlanLimits
		want   error
	}{
		{name: "text", source: `<svg width="10" height="10"><text x="1" y="8">x</text></svg>`, limits: DefaultSVGDisplayPlanLimits(), want: ErrSVGDisplayPlanUnsupported},
		{name: "off-center-radial-gradient", source: `<svg width="10" height="10"><defs><radialGradient id="g" fx="20%"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="blue"/></radialGradient></defs><rect width="10" height="10" fill="url(#g)"/></svg>`, limits: DefaultSVGDisplayPlanLimits(), want: ErrSVGDisplayPlanUnsupported},
		{name: "implicit paint", source: `<svg width="10" height="10"><rect width="10" height="10"/></svg>`, limits: DefaultSVGDisplayPlanLimits(), want: ErrSVGDisplayPlanUnsupported},
		{name: "invalid-image", source: `<svg width="10" height="10"><image width="8" height="8" href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9WlYcYQAAAAASUVORK5CYII="/></svg>`, limits: DefaultSVGDisplayPlanLimits(), want: ErrSVGDisplayPlanUnsupported},
		{name: "segment bound", source: `<svg width="10" height="10"><path d="M0 0 L5 0 L5 5 Z" fill="red"/></svg>`, limits: SVGDisplayPlanLimits{MaxPaths: 1, MaxSegments: 2, MaxPaintItems: 1}, want: ErrSVGDisplayPlanLimit},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			svg, parseErr := SVGParse([]byte(test.source))
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			result, attachErr := AttachSVGDisplayPlanContext(t.Context(), geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1}, test.limits)
			if !errors.Is(attachErr, test.want) {
				t.Fatalf("error = %v, want %v", attachErr, test.want)
			}
			projection := result.Projection()
			if len(projection.Pages) != 0 || len(projection.Paths) != 0 || len(projection.Fills) != 0 || len(projection.Strokes) != 0 || len(projection.Commands) != 0 {
				t.Fatalf("atomic rejection returned partial resources: %+v", projection)
			}
			if got, _ := geometry.Hash(); got != originalHash {
				t.Fatal("atomic rejection mutated source plan")
			}
		})
	}
}

func TestAttachSVGDisplayPlanBoundedLinearGradientParity(t *testing.T) {
	svg, err := SVGParse([]byte(`<svg width="32" height="16"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="#ff0000"/><stop offset="1" stop-color="#0000ff"/></linearGradient></defs><rect x="2" y="2" width="28" height="12" fill="url(#g)" fill-opacity="0.75"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{Number: 1, Size: layoutengine.Size{Width: 40 * 1024, Height: 24 * 1024}}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Paths) != 1+svgDisplayGradientBands || len(projection.Clips) != 1 || len(projection.Fills) != svgDisplayGradientBands || len(projection.Commands) != svgDisplayGradientBands+3 {
		t.Fatalf("gradient resources = paths %d clips %d fills %d commands %d", len(projection.Paths), len(projection.Clips), len(projection.Fills), len(projection.Commands))
	}
	if projection.Fills[0].Opacity != 768 || projection.Fills[0].Color.R <= projection.Fills[0].Color.B || projection.Fills[len(projection.Fills)-1].Color.B <= projection.Fills[len(projection.Fills)-1].Color.R {
		t.Fatalf("gradient endpoints = first %+v last %+v", projection.Fills[0], projection.Fills[len(projection.Fills)-1])
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(capture.SVG, []byte(`fill-opacity="0.75"`)) != svgDisplayGradientBands || !bytes.Contains(capture.SVG, []byte("<clipPath")) {
		t.Fatalf("gradient capture does not contain the canonical clipped bands")
	}
	raster, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: strings.Repeat("a", 64),
		Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	})
	if err != nil || len(raster.PNG()) == 0 {
		t.Fatalf("gradient raster = %d bytes, %v", len(raster.PNG()), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(target.pages[1].Bytes(), []byte(" gs\n")) {
		t.Fatal("gradient PDF lacks bounded fill opacity graphics state")
	}
	_, err = AttachSVGDisplayPlanContext(t.Context(), geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1}, SVGDisplayPlanLimits{MaxPaths: svgDisplayGradientBands, MaxSegments: 1 << 12, MaxPaintItems: 1 << 12})
	if !errors.Is(err, ErrSVGDisplayPlanLimit) {
		t.Fatalf("gradient limit error = %v", err)
	}
}

func TestAttachSVGDisplayPlanDiagonalTranslucentGradientParity(t *testing.T) {
	svg, err := SVGParse([]byte(`<svg width="32" height="20"><defs><linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0" stop-color="#ff2000" stop-opacity=".25"/><stop offset=".5" stop-color="#20c040" stop-opacity=".6"/><stop offset="1" stop-color="#2040ff" stop-opacity=".9"/></linearGradient></defs><rect x="2" y="2" width="28" height="16" fill="url(#g)" opacity=".8"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{Number: 1, Size: layoutengine.Size{Width: 36 * 1024, Height: 24 * 1024}}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Paths) != 1+svgDisplayGradientBands || len(projection.Clips) != 1 || len(projection.Fills) != svgDisplayGradientBands {
		t.Fatalf("diagonal gradient resources paths=%d clips=%d fills=%d", len(projection.Paths), len(projection.Clips), len(projection.Fills))
	}
	first, middle, last := projection.Fills[0], projection.Fills[len(projection.Fills)/2], projection.Fills[len(projection.Fills)-1]
	if first.Opacity <= 0 || middle.Opacity <= first.Opacity || last.Opacity <= middle.Opacity || first.Color.R <= first.Color.B || last.Color.B <= last.Color.R {
		t.Fatalf("diagonal translucent bands first=%+v middle=%+v last=%+v", first, middle, last)
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil || bytes.Count(capture.SVG, []byte(`fill-opacity=`)) != svgDisplayGradientBands || bytes.Count(capture.SVG, []byte(`<path `)) < svgDisplayGradientBands {
		t.Fatalf("diagonal capture err=%v\n%s", err, capture.SVG)
	}
	request := layoutengine.DisplayRasterRequest{Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: strings.Repeat("d", 64), Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("4", 64), ScenarioRevision: strings.Repeat("5", 64), PolicyRevision: strings.Repeat("6", 64)}}
	firstRaster, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, request)
	if err != nil {
		t.Fatal(err)
	}
	secondRaster, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, request)
	if err != nil || !bytes.Equal(firstRaster.PNG(), secondRaster.PNG()) {
		t.Fatalf("diagonal raster is not deterministic: %v", err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(target.pages[1].Bytes(), []byte(" gs\n")) < svgDisplayGradientBands {
		t.Fatal("diagonal translucent PDF lacks per-band opacity graphics state")
	}
}

func TestAttachSVGDisplayPlanCenteredRadialGradientParity(t *testing.T) {
	svg, err := SVGParse([]byte(`<svg width="30" height="20"><defs><radialGradient id="g"><stop offset="0" stop-color="#fff080" stop-opacity=".9"/><stop offset=".55" stop-color="#e04040" stop-opacity=".65"/><stop offset="1" stop-color="#202060" stop-opacity=".35"/></radialGradient></defs><rect x="2" y="2" width="26" height="16" fill="url(#g)"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{Number: 1, Size: layoutengine.Size{Width: 34 * 1024, Height: 24 * 1024}}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, X: 1, Y: 1, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Paths) != 1+svgDisplayGradientBands || len(projection.Clips) != 1 || len(projection.Fills) != svgDisplayGradientBands {
		t.Fatalf("radial resources paths=%d clips=%d fills=%d", len(projection.Paths), len(projection.Clips), len(projection.Fills))
	}
	outer, inner := projection.Fills[0], projection.Fills[len(projection.Fills)-1]
	if outer.Rule != layoutengine.FillNonZero || inner.Rule != layoutengine.FillNonZero || outer.Color.B <= outer.Color.R || inner.Color.R < inner.Color.B || outer.Opacity <= 0 || inner.Opacity <= outer.Opacity {
		t.Fatalf("radial endpoint bands outer=%+v inner=%+v", outer, inner)
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil || bytes.Count(capture.SVG, []byte(`fill-rule="nonzero"`)) != svgDisplayGradientBands {
		t.Fatalf("radial capture err=%v\n%s", err, capture.SVG)
	}
	request := layoutengine.DisplayRasterRequest{Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: strings.Repeat("a", 64), Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("7", 64), ScenarioRevision: strings.Repeat("8", 64), PolicyRevision: strings.Repeat("9", 64)}}
	first, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, request)
	if err != nil || !bytes.Equal(first.PNG(), second.PNG()) {
		t.Fatalf("radial raster is not deterministic: %v", err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(target.pages[1].Bytes(), []byte(" gs\n")) < svgDisplayGradientBands {
		t.Fatal("radial translucent PDF lacks per-band opacity graphics state")
	}
}

func TestAttachSVGDisplayPlanBoundedPatternParity(t *testing.T) {
	svg, err := SVGParse([]byte(`<svg width="20" height="12"><defs><pattern id="p" width="4" height="4" patternUnits="userSpaceOnUse"><rect width="2" height="4" fill="#ffcc00"/><rect x="2" width="2" height="4" fill="#204080"/></pattern></defs><rect x="2" y="2" width="16" height="8" fill="url(#p)" opacity=".5"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{Number: 1, Size: layoutengine.Size{Width: 24 * 1024, Height: 16 * 1024}}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Paths) != 31 || len(projection.Clips) != 1 || len(projection.Fills) != 30 || len(projection.Commands) != 33 {
		t.Fatalf("pattern resources = paths %d clips %d fills %d commands %d", len(projection.Paths), len(projection.Clips), len(projection.Fills), len(projection.Commands))
	}
	if projection.Fills[0].Opacity != 512 || projection.Fills[0].Color != (layoutengine.CoreRGBColor{R: 255, G: 204, Set: true}) || projection.Fills[1].Color != (layoutengine.CoreRGBColor{R: 32, G: 64, B: 128, Set: true}) {
		t.Fatalf("pattern fills = %+v %+v", projection.Fills[0], projection.Fills[1])
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil || bytes.Count(capture.SVG, []byte(`fill-opacity="0.5"`)) != 30 || !bytes.Contains(capture.SVG, []byte("<clipPath")) {
		t.Fatalf("pattern capture err=%v", err)
	}
	raster, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: strings.Repeat("a", 64),
		Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	})
	if err != nil || len(raster.PNG()) == 0 {
		t.Fatalf("pattern raster = %d bytes, %v", len(raster.PNG()), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil || !bytes.Contains(target.pages[1].Bytes(), []byte(" gs\n")) {
		t.Fatalf("pattern PDF err=%v", err)
	}
	_, err = AttachSVGDisplayPlanContext(t.Context(), geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1}, SVGDisplayPlanLimits{MaxPaths: 30, MaxSegments: 1 << 12, MaxPaintItems: 1 << 12})
	if !errors.Is(err, ErrSVGDisplayPlanLimit) {
		t.Fatalf("pattern limit error = %v", err)
	}
}

func TestAttachSVGDisplayPlanTextImageAndClip(t *testing.T) {
	const source = `<svg width="48" height="24" viewBox="0 0 48 24">
<defs><clipPath id="cut"><rect x="1" y="1" width="14" height="10"/></clipPath></defs>
<rect x="0" y="0" width="18" height="14" fill="#336699" clip-path="url(#cut)"/>
<text x="20" y="10" font-size="8" fill="#112233" clip-path="url(#cut)">OK</text>
<image x="20" y="12" width="8" height="8" clip-path="url(#cut)" href="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="/>
</svg>`
	svg, err := SVGParse([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	pageSize := layoutengine.Size{Width: 100 * 1024, Height: 80 * 1024}
	box, _ := layoutengine.NewRect(5*1024, 6*1024, 48*1024, 24*1024)
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages:     []layoutengine.PlannedPage{{Number: 1, Size: pageSize, Fragments: layoutengine.IndexRange{Count: 1}}},
		Fragments: []layoutengine.Fragment{{ID: 1, Node: 1, Key: "svg", Instance: "svg", Page: 1, Region: layoutengine.RegionBody, BorderBox: box, ContentBox: box, Continuation: layoutengine.ContinuationWhole}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Fragment: 1, X: 5, Y: 6, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.Projection()
	if len(projection.Clips) != 3 || len(projection.GlyphRuns) != 1 || len(projection.Images) != 1 || len(projection.ImageResources) != 1 {
		t.Fatalf("rich resources: clips=%d glyphs=%d images=%d resources=%d", len(projection.Clips), len(projection.GlyphRuns), len(projection.Images), len(projection.ImageResources))
	}
	if got := projection.GlyphRuns[0]; got.Codes != "OK" || got.Color != (layoutengine.CoreRGBColor{R: 17, G: 34, B: 51, Set: true}) {
		t.Fatalf("glyph run = %+v", got)
	}
	images := svgDisplayImageSources(&svg)
	captureImages := make(layoutengine.DisplaySVGImageSources, len(images))
	for digest, encoded := range images {
		captureImages[digest] = encoded
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, captureImages)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range [][]byte{[]byte("<clipPath"), []byte(">O</text>"), []byte(">K</text>"), []byte("data:image/png;base64,")} {
		if !bytes.Contains(capture.SVG, token) {
			t.Fatalf("rich SVG capture lacks %q:\n%s", token, capture.SVG)
		}
	}
	fontPrograms := make(map[layoutengine.CoreFontMetricsDigest][]byte, len(projection.Fonts))
	for _, font := range projection.Fonts {
		fontPrograms[font.MetricsDigest] = goregular.TTF
	}
	raster, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{FontPrograms: fontPrograms, Images: captureImages}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(), PageProfile: strings.Repeat("a", 64),
		Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	})
	if err != nil || len(raster.PNG()) == 0 {
		t.Fatalf("clipped text/image direct raster = %d bytes, %v", len(raster.PNG()), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, images); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	for _, token := range [][]byte{[]byte(" W n\n"), []byte("(O) Tj"), []byte("(K) Tj"), []byte(" Do Q")} {
		if !bytes.Contains(content, token) {
			t.Fatalf("rich PDF content lacks %q:\n%s", token, content)
		}
	}
}

func TestAttachSVGDisplayPlanStyledStrokeParity(t *testing.T) {
	svg, err := SVGParse([]byte(`<svg width="40" height="20"><path d="M2 10 L38 10" fill="none" stroke="#2468ac" stroke-width="2" stroke-linecap="round" stroke-linejoin="bevel" stroke-dasharray="2 3" stroke-dashoffset="1" stroke-opacity="0.5"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{Pages: []layoutengine.PlannedPage{{
		Number: 1, Size: layoutengine.Size{Width: 50 * 1024, Height: 30 * 1024},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Scale: 1})
	if err != nil {
		t.Fatal(err)
	}
	stroke := plan.Projection().Strokes[0]
	if stroke.LineCap != layoutengine.StrokeCapRound || stroke.LineJoin != layoutengine.StrokeJoinBevel || stroke.DashOffset != 1024 || stroke.Opacity != 512 || len(stroke.Dash) != 2 || stroke.Dash[0] != 2*1024 || stroke.Dash[1] != 3*1024 {
		t.Fatalf("stroke = %+v", stroke)
	}
	stroke.Dash[0] = 99
	if plan.Projection().Strokes[0].Dash[0] != 2*1024 {
		t.Fatal("projection exposed mutable dash storage")
	}
	capture, err := layoutengine.CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range [][]byte{[]byte(`stroke-linecap="round"`), []byte(`stroke-linejoin="bevel"`), []byte(`stroke-dasharray="2048 3072"`), []byte(`stroke-dashoffset="1024"`), []byte(`stroke-opacity="0.5"`)} {
		if !bytes.Contains(capture.SVG, token) {
			t.Fatalf("SVG capture lacks %q:\n%s", token, capture.SVG)
		}
	}
	raster, err := layoutengine.CaptureDisplayPlanPNG(plan, layoutengine.DisplayRasterSources{}, layoutengine.DisplayRasterRequest{
		Page: 1, Profile: layoutengine.DefaultDisplayRasterProfile(), Limits: layoutengine.DefaultDisplayRasterLimits(),
		PageProfile: strings.Repeat("a", 64), Revisions: layoutengine.ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	})
	if err != nil || len(raster.PNG()) == 0 {
		t.Fatalf("styled stroke raster = %d bytes, %v", len(raster.PNG()), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	content := target.pages[1].Bytes()
	for _, token := range [][]byte{[]byte("1 J 2 j"), []byte("[2.0000000000 3.0000000000] 1.0000000000 d"), []byte(" gs\n")} {
		if !bytes.Contains(content, token) {
			t.Fatalf("PDF content lacks %q:\n%s", token, content)
		}
	}
}

func BenchmarkAttachSVGDisplayPlanRich(b *testing.B) {
	svg, err := SVGParse([]byte(`<svg width="80" height="30"><rect x="1" y="1" width="20" height="12" fill="#336699"/><text x="24" y="12" font-size="9" fill="#112233">benchmark</text></svg>`))
	if err != nil {
		b.Fatal(err)
	}
	box, _ := layoutengine.NewRect(0, 0, 80*1024, 30*1024)
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages:     []layoutengine.PlannedPage{{Number: 1, Size: layoutengine.Size{Width: 100 * 1024, Height: 80 * 1024}, Fragments: layoutengine.IndexRange{Count: 1}}},
		Fragments: []layoutengine.Fragment{{ID: 1, Node: 1, Key: "svg", Instance: "svg", Page: 1, Region: layoutengine.RegionBody, BorderBox: box, ContentBox: box, Continuation: layoutengine.ContinuationWhole}},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, err := AttachSVGDisplayPlan(geometry, &svg, SVGDisplayPlanPlacement{Page: 1, Fragment: 1, Scale: 1}); err != nil {
			b.Fatal(err)
		}
	}
}
