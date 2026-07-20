// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

const paperImageSource = "document @report:\n  language: \"en\"\n  page @sheet:\n    width: 100pt\n    height: 80pt\n    margin: 8pt\n    body @body:\n      image @hero:\n        source: \"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==\"\n        width: 40pt\n        height: 24pt\n        fit: \"cover\"\n        focus-x: 0.25\n        focus-y: 0.75\n        alt: \"Evidence pixel\"\n"

func TestPaperImagePlansRendersCapturesAndRetainsFigureSemantics(t *testing.T) {
	plan, result, err := PlanPaper("image.paper", paperImageSource)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 1 {
		t.Fatalf("image projection = resources %d images %d", len(projection.ImageResources), len(projection.Images))
	}
	encoded, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	digest := sha256.Sum256(encoded)
	if string(projection.ImageResources[0].Digest) != hex.EncodeToString(digest[:]) {
		t.Fatalf("resource digest = %s", projection.ImageResources[0].Digest)
	}
	found := false
	for _, semantic := range projection.SemanticNodes {
		if semantic.Role == layoutengine.SemanticRoleFigure && semantic.Attributes.AlternateText == "Evidence pixel" {
			found = true
		}
	}
	if !found {
		t.Fatalf("figure semantics = %#v", projection.SemanticNodes)
	}
	display, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || !bytes.Contains(display.SVG, []byte("<image")) {
		t.Fatalf("display capture = %q, %v", display.SVG, err)
	}
	rasterRequest := DefaultPaperPlanRasterRequest()
	rasterRequest.CoreFontProgram = goregular.TTF
	raster, err := plan.CaptureRasterPages(context.Background(), rasterRequest)
	if err != nil || len(raster.Pages) != 1 || len(raster.Pages[0].PNG) == 0 {
		t.Fatalf("raster = %#v, %v", raster, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	rendered, err := target.WritePaperPlan(plan)
	if err != nil || !rendered.OK() || target.PageCount() != 1 {
		t.Fatalf("WritePaperPlan() = %#v, %v", rendered, err)
	}
	changed, changedResult, err := PlanPaper("image.paper", strings.Replace(paperImageSource, "focus-x: 0.25", "focus-x: 0.75", 1))
	if err != nil || !changedResult.OK() || changed.Hash() == plan.Hash() {
		t.Fatalf("focus did not participate in plan identity: %q / %q, %v", plan.Hash(), changed.Hash(), err)
	}
}

func TestPaperImageResolvesPercentageWidthAndIntrinsicHeightInContainingBody(t *testing.T) {
	source := strings.Replace(paperImageSource, "width: 40pt\n        height: 24pt", "width: 50%\n        height: \"auto\"", 1)
	plan, result, err := PlanPaper("responsive-image.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 1 || projection.Fragments[0].BorderBox.Width.Points() != 42 || projection.Fragments[0].BorderBox.Height.Points() != 42 {
		t.Fatalf("responsive image fragments = %+v", projection.Fragments)
	}
	if len(projection.Images) != 1 || projection.Images[0].Bounds.Width.Points() != 42 || projection.Images[0].Bounds.Height.Points() != 42 {
		t.Fatalf("responsive image bounds = %+v", projection.Images)
	}
}

func TestPaperAssetReferenceIsHumanReadableContentAddressedAndAmbientFree(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	catalog, err := NewPaperAssetCatalog([]PaperAssetResource{{
		Name: "hero-image", MediaType: "image/png", Digest: hex.EncodeToString(digest[:]), Data: data,
	}})
	if err != nil || catalog.ResourceCount() != 1 {
		t.Fatalf("NewPaperAssetCatalog() = %#v, %v", catalog, err)
	}
	source := strings.Replace(paperImageSource,
		"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
		"asset:hero-image", 1)
	if strings.Contains(source, "base64") || !strings.Contains(source, `source: "asset:hero-image"`) {
		t.Fatalf("asset source is not readable: %s", source)
	}
	if missing, result, planErr := PlanPaper("asset.paper", source); planErr == nil || result.OK() || missing.Hash() != "" {
		t.Fatalf("ambient asset unexpectedly resolved: %#v/%#v, %v", missing, result, planErr)
	}
	plan, result, err := PlanPaperWithAssets("asset.paper", source, catalog)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaperWithAssets() = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || string(projection.ImageResources[0].Digest) != hex.EncodeToString(digest[:]) {
		t.Fatalf("asset plan resources = %#v", projection.ImageResources)
	}
	display, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || !bytes.Contains(display.SVG, []byte("<image")) {
		t.Fatalf("asset capture = %q, %v", display.SVG, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	rendered, err := target.WritePaperWithAssets("asset.paper", source, catalog)
	if err != nil || !rendered.OK() || target.PageCount() != 1 {
		t.Fatalf("WritePaperWithAssets() = %#v, %v", rendered, err)
	}
	data[0] = 0
	again, againResult, err := PlanPaperWithAssets("asset.paper", source, catalog)
	if err != nil || !againResult.OK() || again.Hash() != plan.Hash() {
		t.Fatalf("catalog retained caller storage: %q/%q, %v", plan.Hash(), again.Hash(), err)
	}
	scenarioSource := strings.Replace(source, "  language: \"en\"\n", "  language: \"en\"\n  scenario @preview:\n    locale: \"en-US\"\n", 1)
	scenarioPlan, scenarioResult, err := PlanPaperScenarioWithAssets("asset-scenario.paper", scenarioSource, "preview", catalog)
	if err != nil || !scenarioResult.OK() || scenarioPlan.PageCount() != 1 {
		t.Fatalf("PlanPaperScenarioWithAssets() = %#v, %v", scenarioResult, err)
	}
}

func TestPaperManifestFontIsEmbeddedAndSubsetAtPDFPaint(t *testing.T) {
	fontBytes, err := os.ReadFile("../assets/static/font/NotoNaskhArabic-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(fontBytes)
	catalog, err := NewPaperAssetCatalog([]PaperAssetResource{{Name: "body-font", MediaType: "font/ttf", Digest: hex.EncodeToString(digest[:]), Data: fontBytes, Family: "Specimen Sans", Style: "normal", Weight: 400, License: "OFL-1.1"}})
	if err != nil {
		t.Fatal(err)
	}
	source := "document @report:\n" +
		"  page @sheet:\n" +
		"    width: 180pt\n" +
		"    height: 80pt\n" +
		"    margin: 8pt\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        font: \"Specimen Sans\"\n" +
		"        text: \"Embedded custom font\"\n"
	plan, planned, err := PlanPaperWithAssets("font.paper", source, catalog)
	if err != nil || !planned.OK() {
		t.Fatalf("custom font plan = %#v, %v", planned, err)
	}
	if len(plan.plan.Projection().Fonts) != 1 || plan.plan.Projection().Fonts[0].EmbeddedUTF8 == nil || len(plan.fontSources) != 1 {
		t.Fatalf("custom font projection/sources = %#v / %d", plan.plan.Projection().Fonts, len(plan.fontSources))
	}
	pdf := MustNew(WithUnit(UnitPoint), WithNoCompression())
	rendered, err := pdf.WritePaperPlan(plan)
	if err != nil || !rendered.OK() {
		t.Fatalf("custom font PDF paint = %#v, %v", rendered, err)
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil || !bytes.Contains(output.Bytes(), []byte("/FontFile2")) {
		t.Fatalf("custom font output = %v, embedded font marker missing", err)
	}
}

func TestPaperDecorativeImageIsAnExplicitArtifact(t *testing.T) {
	source := strings.Replace(paperImageSource, `alt: "Evidence pixel"`, "decorative: true", 1)
	plan, result, err := PlanPaper("decorative-image.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	for _, semantic := range plan.plan.Projection().SemanticNodes {
		if semantic.Role == layoutengine.SemanticRoleFigure {
			t.Fatalf("decorative image exposed figure semantics: %#v", semantic)
		}
	}
}

func TestPaperImageFocusControlsExactCoverCrop(t *testing.T) {
	bitmap := image.NewRGBA(image.Rect(0, 0, 2, 1))
	bitmap.Set(0, 0, color.RGBA{R: 255, A: 255})
	bitmap.Set(1, 0, color.RGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, bitmap); err != nil {
		t.Fatal(err)
	}
	resource := base64.StdEncoding.EncodeToString(encoded.Bytes())
	build := func(focus string) PaperPlan {
		source := strings.Replace(paperImageSource,
			"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", resource, 1)
		source = strings.Replace(source, "width: 40pt", "width: 20pt", 1)
		source = strings.Replace(source, "focus-x: 0.25", "focus-x: "+focus, 1)
		plan, result, err := PlanPaper("focus.paper", source)
		if err != nil || !result.OK() {
			t.Fatalf("PlanPaper(%s) = %#v, %v", focus, result, err)
		}
		return plan
	}
	left, right := build("0"), build("1")
	leftCrop := left.plan.Projection().Images[0].Crop
	rightCrop := right.plan.Projection().Images[0].Crop
	if leftCrop == nil || rightCrop == nil || leftCrop.Source.X != 0 || rightCrop.Source.X <= leftCrop.Source.X {
		t.Fatalf("cover crops = %#v / %#v", leftCrop, rightCrop)
	}
}
