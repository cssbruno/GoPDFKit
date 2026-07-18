// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/image/font/opentype"
)

func TestCaptureDisplayPlanPNGDeterministicLosslessPageAndManifest(t *testing.T) {
	plan, sources := rasterFixture(t)
	request := rasterRequest()
	first, err := CaptureDisplayPlanPNG(plan, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CaptureDisplayPlanPNG(plan, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := first.CanonicalManifestJSON()
	secondJSON, _ := second.CanonicalManifestJSON()
	if !bytes.Equal(first.PNG(), second.PNG()) || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("PNG capture or canonical manifest is not deterministic")
	}
	manifest := first.Manifest()
	if manifest.MediaType != "image/png" || manifest.ArtifactKind != "direct_display_list_preview" || manifest.AuthoritativePDF ||
		manifest.Disclosure != "contains_rendered_content" || !manifest.ContainsContent || manifest.Page != 1 ||
		manifest.PixelWidth != 144 || manifest.PixelHeight != 144 || manifest.CaptureBounds != (Rect{Width: 72 * Fixed(FixedScale), Height: 72 * Fixed(FixedScale)}) ||
		manifest.Profile != request.Profile || manifest.PageProfile != request.PageProfile || manifest.Identity.RendererVersion != DisplayRasterRendererVersion || len(manifest.Resources) != 1 {
		t.Fatalf("manifest = %+v", manifest)
	}
	digest := sha256.Sum256(first.PNG())
	if manifest.PNGSHA256 != hex.EncodeToString(digest[:]) || manifest.PNGByteLength != uint64(len(first.PNG())) {
		t.Fatalf("PNG evidence = %+v", manifest)
	}
	if manifest.PNGSHA256 != "45a8cc5f49a9e0f336dacc3c17377262a966f1eb16ae5914383d08a13bdd305c" {
		t.Fatalf("fixture raster hash = %s", manifest.PNGSHA256)
	}
	decoded, err := png.Decode(bytes.NewReader(first.PNG()))
	if err != nil {
		t.Fatalf("lossless PNG decode: %v", err)
	}
	if decoded.Bounds() != image.Rect(0, 0, 144, 144) {
		t.Fatalf("bounds = %v", decoded.Bounds())
	}
	if got := color.RGBAModel.Convert(decoded.At(12, 12)).(color.RGBA); got.R < 240 || got.G > 20 || got.B > 20 {
		t.Fatalf("filled display command pixel = %+v", got)
	}
	if got := color.RGBAModel.Convert(decoded.At(140, 140)).(color.RGBA); got != (color.RGBA{255, 255, 255, 255}) {
		t.Fatalf("background = %+v", got)
	}
	var decodedManifest DisplayRasterManifest
	if err := json.Unmarshal(firstJSON, &decodedManifest); err != nil || !reflect.DeepEqual(decodedManifest, manifest) {
		t.Fatalf("manifest round trip = %v, %+v", err, decodedManifest)
	}
	manifest.Resources[0].SHA256 = "changed"
	pngCopy := first.PNG()
	pngCopy[0] ^= 0xff
	if first.Manifest().Resources[0].SHA256 == "changed" || first.PNG()[0] == pngCopy[0] {
		t.Fatal("artifact exposed mutable storage")
	}
}

func TestCaptureDisplayPlanPNGBoundedExactCrop(t *testing.T) {
	plan, sources := rasterFixture(t)
	request := rasterRequest()
	crop := Rect{X: 5 * Fixed(FixedScale), Y: 5 * Fixed(FixedScale), Width: 20 * Fixed(FixedScale), Height: 10 * Fixed(FixedScale)}
	request.Crop = &crop
	request.Profile.DPI = 72
	artifact, err := CaptureDisplayPlanPNG(plan, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	manifest := artifact.Manifest()
	if manifest.CaptureBounds != crop || manifest.PixelWidth != 20 || manifest.PixelHeight != 10 || manifest.PixelTransform.OriginX != crop.X || manifest.PixelTransform.XNumerator != 20 || manifest.PixelTransform.XDenominator != crop.Width {
		t.Fatalf("crop manifest = %+v", manifest)
	}
	decoded, _ := png.Decode(bytes.NewReader(artifact.PNG()))
	if decoded.Bounds() != image.Rect(0, 0, 20, 10) {
		t.Fatalf("crop PNG = %v", decoded.Bounds())
	}
}

func TestCaptureDisplayPlanPNGPreflightIsAtomic(t *testing.T) {
	plan, sources := rasterFixture(t)
	request := rasterRequest()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	artifact, err := CaptureDisplayPlanPNGContext(ctx, plan, sources, request)
	if !errors.Is(err, context.Canceled) || len(artifact.PNG()) != 0 {
		t.Fatalf("cancel artifact=%d err=%v", len(artifact.PNG()), err)
	}
	request.Limits.MaxPixels = 1
	artifact, err = CaptureDisplayPlanPNG(plan, sources, request)
	if !errors.Is(err, ErrDisplayRasterLimit) || len(artifact.PNG()) != 0 {
		t.Fatalf("limit artifact=%d err=%v", len(artifact.PNG()), err)
	}
	request = rasterRequest()
	delete(sources.FontPrograms, CoreFontMetricsDigest(strings.Repeat("1", 64)))
	artifact, err = CaptureDisplayPlanPNG(plan, sources, request)
	if !errors.Is(err, ErrDisplayRasterResource) || len(artifact.PNG()) != 0 {
		t.Fatalf("resource artifact=%d err=%v", len(artifact.PNG()), err)
	}
	plan, sources = rasterFixture(t)
	request = rasterRequest()
	request.Limits.MaxPNGBytes = 1
	artifact, err = CaptureDisplayPlanPNG(plan, sources, request)
	if !errors.Is(err, ErrDisplayRasterLimit) || len(artifact.PNG()) != 0 {
		t.Fatalf("PNG-byte artifact=%d err=%v", len(artifact.PNG()), err)
	}
}

func TestCaptureDisplayPlanPNGRejectsUnsupportedBeforePainting(t *testing.T) {
	plan := graphicsDisplayPlan(t)
	request := rasterRequest()
	artifact, err := CaptureDisplayPlanPNG(plan, DisplayRasterSources{}, request)
	if !errors.Is(err, ErrDisplayRasterUnsupported) || len(artifact.PNG()) != 0 {
		t.Fatalf("unsupported artifact=%d err=%v", len(artifact.PNG()), err)
	}
}

func TestCaptureDisplayPlanPNGPaintsDeterministicStraightStrokes(t *testing.T) {
	unit := Fixed(FixedScale)
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: []PlannedPage{{Number: 1, Size: Size{Width: 72 * unit, Height: 72 * unit}}}})
	if err != nil {
		t.Fatal(err)
	}
	path := PlannedPath{Bounds: Rect{X: 10 * unit, Y: 20 * unit, Width: 52 * unit}, Segments: []PathSegment{
		{Kind: PathMoveTo, Point: Point{X: 10 * unit, Y: 20 * unit}},
		{Kind: PathLineTo, Point: Point{X: 62 * unit, Y: 20 * unit}},
	}}
	plan, err := AttachDisplayList(geometry, DisplayListInput{Paths: []PlannedPath{path},
		Strokes: []PlannedStroke{{Path: 0, Color: CoreRGBColor{Set: true}, Width: unit}},
		Items:   []DisplayItem{{Kind: CommandStrokePath, Payload: 0, Page: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CaptureDisplayPlanPNG(plan, DisplayRasterSources{}, rasterRequest())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(artifact.PNG()))
	if err != nil {
		t.Fatal(err)
	}
	if got := color.RGBAModel.Convert(decoded.At(72, 40)).(color.RGBA); got.R > 20 || got.G > 20 || got.B > 20 {
		t.Fatalf("stroke pixel = %+v", got)
	}
}

func TestRasterOpacityAlphaRoundsAndPreservesMaximum(t *testing.T) {
	if got := rasterOpacityAlpha(255, 0); got != 255 {
		t.Fatalf("opaque sentinel alpha = %d", got)
	}
	if got := rasterOpacityAlpha(255, Fixed(FixedScale)); got != 255 {
		t.Fatalf("maximum alpha = %d", got)
	}
	if got := rasterOpacityAlpha(255, Fixed(FixedScale/2)); got != 128 {
		t.Fatalf("half alpha = %d, want rounded 128", got)
	}
	if got := rasterOpacityAlpha(127, Fixed(FixedScale/2)); got != 64 {
		t.Fatalf("combined alpha = %d, want rounded 64", got)
	}
}

func TestRasterCoreFontSubstituteFitsPlannedRunWithoutGlyphCollisions(t *testing.T) {
	fontBytes, err := os.ReadFile("../../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := opentype.Parse(fontBytes)
	if err != nil {
		t.Fatal(err)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 144, 144))
	for y := 0; y < canvas.Bounds().Dy(); y++ {
		for x := 0; x < canvas.Bounds().Dx(); x++ {
			canvas.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	unit := Fixed(FixedScale)
	run := CoreGlyphRun{
		Font: 1, FontSize: 12 * unit, Origin: Point{X: 10 * unit, Y: 30 * unit},
		Codes: "MMMM", Advances: []Fixed{4 * unit, 4 * unit, 4 * unit, 4 * unit},
	}
	if err := (&rasterSizedFace{font: parsed}).draw(canvas, CoreFontResource{ID: 1, Face: CoreFontHelvetica}, run,
		IdentityTransform(), Rect{Width: 72 * unit, Height: 72 * unit}, 144, canvas.Bounds()); err != nil {
		t.Fatal(err)
	}
	ink := false
	for y := 20; y < 64; y++ {
		for x := 20; x < 52; x++ {
			pixel := canvas.RGBAAt(x, y)
			ink = ink || pixel.R < 240 || pixel.G < 240 || pixel.B < 240
		}
		for x := 52; x < 72; x++ {
			if pixel := canvas.RGBAAt(x, y); pixel != (color.RGBA{255, 255, 255, 255}) {
				t.Fatalf("substitute ink escaped planned run at (%d,%d): %+v", x, y, pixel)
			}
		}
	}
	if !ink {
		t.Fatal("core-font substitute painted no visible glyphs")
	}
}

func rasterRequest() DisplayRasterRequest {
	return DisplayRasterRequest{Page: 1, Profile: DefaultDisplayRasterProfile(), Limits: DefaultDisplayRasterLimits(), PageProfile: strings.Repeat("a", 64), Revisions: ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)}}
}

func rasterFixture(t *testing.T) (LayoutPlan, DisplayRasterSources) {
	t.Helper()
	unit := Fixed(FixedScale)
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: []PlannedPage{{Number: 1, Size: Size{Width: 72 * unit, Height: 72 * unit}, Fragments: IndexRange{Count: 1}, Lines: IndexRange{Count: 1}}}, Fragments: []Fragment{{ID: 1, Node: 1, Key: "@preview", Instance: "@preview", Page: 1, Region: RegionBody, BorderBox: Rect{X: 5 * unit, Y: 5 * unit, Width: 50 * unit, Height: 40 * unit}, ContentBox: Rect{X: 5 * unit, Y: 5 * unit, Width: 50 * unit, Height: 40 * unit}, Continuation: ContinuationWhole}}, Lines: []PlannedLine{{Fragment: 1, Bounds: Rect{X: 10 * unit, Y: 25 * unit, Width: 16 * unit, Height: 12 * unit}, Baseline: 35 * unit}}})
	if err != nil {
		t.Fatal(err)
	}
	path := PlannedPath{Bounds: Rect{X: 5 * unit, Y: 5 * unit, Width: 20 * unit, Height: 15 * unit}, Segments: []PathSegment{{Kind: PathMoveTo, Point: Point{X: 5 * unit, Y: 5 * unit}}, {Kind: PathLineTo, Point: Point{X: 25 * unit, Y: 5 * unit}}, {Kind: PathLineTo, Point: Point{X: 25 * unit, Y: 20 * unit}}, {Kind: PathLineTo, Point: Point{X: 5 * unit, Y: 20 * unit}}, {Kind: PathClose}}}
	digest := CoreFontMetricsDigest(strings.Repeat("1", 64))
	plan, err := AttachDisplayList(geometry, DisplayListInput{Fonts: []CoreFontResource{{ID: 1, Face: CoreFontHelvetica, MetricsDigest: digest}}, GlyphRuns: []CoreGlyphRun{{Line: 0, Font: 1, FontSize: 12 * unit, Color: CoreRGBColor{B: 180, Set: true}, Origin: Point{X: 10 * unit, Y: 35 * unit}, Codes: "AI", Advances: []Fixed{8 * unit, 8 * unit}}}, Paths: []PlannedPath{path}, Fills: []PlannedFill{{Path: 0, Rule: FillNonZero, Color: CoreRGBColor{R: 255, Set: true}, Fragment: 1}}, Items: []DisplayItem{{Kind: CommandFillPath, Payload: 0, Page: 1}, {Kind: CommandGlyphRun, Payload: 0, Page: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	fontBytes, err := os.ReadFile("../../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatal(err)
	}
	return plan, DisplayRasterSources{FontPrograms: map[CoreFontMetricsDigest][]byte{digest: fontBytes}}
}
