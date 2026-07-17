// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestPaintDisplayLayoutPlanPDFInterleavesPlannedImageAndText(t *testing.T) {
	plan, sources := mixedDisplayPDFPlan(t)
	target := MustNew(WithUnit(UnitMillimeter), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, sources); err != nil {
		t.Fatalf("paintDisplayLayoutPlanPDF() = %v", err)
	}
	if target.PageCount() != 1 {
		t.Fatalf("pages = %d, want 1", target.PageCount())
	}
	content := target.pages[1].Bytes()
	imageAt := bytes.Index(content, []byte(" Do Q"))
	textAt := bytes.Index(content, []byte("BT /F"))
	if imageAt < 0 || textAt < 0 || imageAt >= textAt {
		t.Fatalf("display order is not image then text:\n%s", content)
	}
	if bytes.Contains(content, []byte(" Td")) || bytes.Contains(content, []byte(" Tw")) {
		t.Fatalf("display painter used layout-capable text operators:\n%s", content)
	}
	if !bytes.Contains(content, []byte("0.0666666667 0.1333333333 0.2000000000 rg")) {
		t.Fatalf("display painter omitted planned RGB text color:\n%s", content)
	}
	if got := len(target.resources.images); got != 1 {
		t.Fatalf("registered planned images = %d, want 1", got)
	}
}

func TestPaintDisplayLayoutPlanPDFRejectsBytesBeforeMutation(t *testing.T) {
	plan, sources := mixedDisplayPDFPlan(t)
	for digest := range sources {
		sources[digest] = []byte("not the planned image")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	before := typedShadowSnapshotOf(target)
	beforeResources := target.resources
	err := target.paintDisplayLayoutPlanPDF(plan, sources)
	if !errors.Is(err, errCoreLayoutPlanPaintUnsupported) {
		t.Fatalf("paint error = %v, want unsupported preflight", err)
	}
	if after := typedShadowSnapshotOf(target); after != before || target.PageCount() != 0 || target.resources != beforeResources {
		t.Fatalf("failed image preflight mutated target: before=%#v after=%#v pages=%d resources=%v",
			before, after, target.PageCount(), target.resources != nil)
	}
}

func TestPaintDisplayLayoutPlanPDFReplaysEndAlignedCoverCrop(t *testing.T) {
	encoded := encodePlannedWidePNG(t)
	plan, sources := croppedDisplayPDFPlan(t, encoded, 2, 1)
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if err := target.paintDisplayLayoutPlanPDF(plan, sources); err != nil {
		t.Fatalf("paintDisplayLayoutPlanPDF() = %v", err)
	}
	content := target.pages[1].Bytes()
	if !bytes.Contains(content, []byte(" re W n")) {
		t.Fatalf("cropped image has no PDF clip operator:\n%s", content)
	}
	// The 12pt square destination selects the end-aligned right half of a 2:1
	// source, so replay paints the full image at 24x12pt shifted 12pt left.
	if !bytes.Contains(content, []byte("24.00000 0 0 12.00000 -6.00000")) {
		t.Fatalf("cropped image transform was not replayed exactly:\n%s", content)
	}
	if target.clipNest != 0 {
		t.Fatalf("clip nesting leaked after crop replay: %d", target.clipNest)
	}
}

func TestPaintDisplayLayoutPlanPDFCropPreflightIsAtomic(t *testing.T) {
	// The bytes are a valid 1x1 PNG and match the digest, but the immutable
	// resource/crop contract declares 2x1. Decode preflight must reject this
	// before installing resources or opening a page.
	plan, sources := croppedDisplayPDFPlan(t, decodeTinyPNG(t), 2, 1)
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	before := typedShadowSnapshotOf(target)
	beforeResources := target.resources
	err := target.paintDisplayLayoutPlanPDF(plan, sources)
	if !errors.Is(err, errCoreLayoutPlanPaintUnsupported) {
		t.Fatalf("paint error = %v, want unsupported preflight", err)
	}
	if after := typedShadowSnapshotOf(target); after != before || target.PageCount() != 0 || target.resources != beforeResources {
		t.Fatalf("crop preflight mutated target: before=%#v after=%#v pages=%d resources=%v",
			before, after, target.PageCount(), target.resources != nil)
	}
}

func mixedDisplayPDFPlan(t *testing.T) (layoutengine.LayoutPlan, plannedImageSources) {
	t.Helper()
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 96, Ht: 72}))
	source.SetMargins(8, 8, 8)
	source.SetAutoPageBreak(true, 8)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "planned"}},
		Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12,
			Color: layout.DocumentColor{R: 17, G: 34, B: 51, Set: true}},
	}}
	shadow, err := source.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("planTypedParagraphLineShadow() = %v", err)
	}
	projection := shadow.Plan.Projection()
	projection.Pages[0].Commands = layoutengine.IndexRange{}
	imageBounds := layoutengine.Rect{
		X: layoutengine.Fixed(6 * layoutengine.FixedScale), Y: layoutengine.Fixed(6 * layoutengine.FixedScale),
		Width: layoutengine.Fixed(12 * layoutengine.FixedScale), Height: layoutengine.Fixed(12 * layoutengine.FixedScale),
	}
	imageFragment := layoutengine.Fragment{
		ID: layoutengine.FragmentID(len(projection.Fragments) + 1), Node: 2,
		Key: "@planned-image", Instance: "@planned-image", Page: 1, Region: layoutengine.RegionBody,
		BorderBox: imageBounds, ContentBox: imageBounds, Continuation: layoutengine.ContinuationWhole,
	}
	projection.Fragments = append(projection.Fragments, imageFragment)
	projection.Pages[0].Fragments.Count++
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan(geometry) = %v", err)
	}
	encoded := decodeTinyPNG(t)
	digestBytes := sha256.Sum256(encoded)
	digest := layoutengine.ImageContentDigest(hex.EncodeToString(digestBytes[:]))
	resource := layoutengine.ImageResource{
		ID: 1, Digest: digest, Format: layoutengine.ImagePNG, PixelWidth: 1, PixelHeight: 1,
	}
	image := layoutengine.PlannedImage{
		Resource: 1, Fragment: imageFragment.ID, Bounds: imageBounds, Source: imageFragment.Source,
	}
	items := make([]layoutengine.DisplayItem, 0, len(projection.GlyphRuns)+1)
	items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandImage})
	for index := range projection.GlyphRuns {
		items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(index)})
	}
	plan, err := layoutengine.AttachDisplayList(geometry, layoutengine.DisplayListInput{
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		ImageResources: []layoutengine.ImageResource{resource}, Images: []layoutengine.PlannedImage{image}, Items: items,
	})
	if err != nil {
		t.Fatalf("AttachDisplayList() = %v", err)
	}
	return plan, plannedImageSources{digest: encoded}
}

func croppedDisplayPDFPlan(t *testing.T, encoded []byte, pixelWidth, pixelHeight uint32) (layoutengine.LayoutPlan, plannedImageSources) {
	t.Helper()
	plan, _ := mixedDisplayPDFPlan(t)
	projection := plan.Projection()
	digestBytes := sha256.Sum256(encoded)
	digest := layoutengine.ImageContentDigest(hex.EncodeToString(digestBytes[:]))
	projection.ImageResources[0] = layoutengine.ImageResource{
		ID: 1, Digest: digest, Format: layoutengine.ImagePNG,
		PixelWidth: pixelWidth, PixelHeight: pixelHeight,
	}
	bounds := projection.Images[0].Bounds
	projection.Images[0].Crop = &layoutengine.ImageCrop{
		Intrinsic: layoutengine.Size{Width: 200, Height: 100},
		Source:    layoutengine.Rect{X: 100, Width: 100, Height: 100},
		Clip:      bounds,
	}
	plan, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		ImageResources: projection.ImageResources, Images: projection.Images,
		Destinations: projection.Destinations, Links: projection.Links,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan(cropped) = %v", err)
	}
	return plan, plannedImageSources{digest: encoded}
}

func encodePlannedWidePNG(t *testing.T) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	value.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	value.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatalf("encode crop PNG: %v", err)
	}
	return encoded.Bytes()
}
