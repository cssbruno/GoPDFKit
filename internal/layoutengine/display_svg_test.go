// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestCaptureDisplayPlanSVGReplaysExactCrop(t *testing.T) {
	plan, sources := cropSVGFixture(t)
	capture, err := CaptureDisplayPlanSVG(plan, 1, sources)
	if err != nil {
		t.Fatalf("CaptureDisplayPlanSVG() = %v", err)
	}
	svg := string(capture.SVG)
	for _, value := range []string{
		`x="10" y="20" width="30" height="40"`,
		`viewBox="100 0 100 100"`,
		`<image x="0" y="0" width="200" height="100"`,
		`data:image/png;base64,`,
	} {
		if !strings.Contains(svg, value) {
			t.Fatalf("SVG missing %q:\n%s", value, svg)
		}
	}
}

func TestCaptureDisplayPlanSVGPreflightsBeforeOutput(t *testing.T) {
	plan, sources := cropSVGFixture(t)
	for digest := range sources {
		sources[digest] = []byte("wrong")
	}
	if capture, err := CaptureDisplayPlanSVG(plan, 1, sources); !errors.Is(err, ErrDisplaySVGResource) || len(capture.SVG) != 0 {
		t.Fatalf("capture = %+v, error = %v", capture, err)
	}
	limits := DefaultDisplaySVGLimits()
	limits.MaxSourceBytes = 1
	if _, err := CaptureDisplayPlanSVGWithLimits(plan, 1, sources, limits); !errors.Is(err, ErrDisplaySVGResource) {
		t.Fatalf("source limit error = %v", err)
	}
}

func TestCaptureDisplayPlanSVGContextCancellationIsAtomic(t *testing.T) {
	plan, sources := cropSVGFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	capture, err := CaptureDisplayPlanSVGContext(ctx, plan, 1, sources)
	if !errors.Is(err, context.Canceled) || len(capture.SVG) != 0 {
		t.Fatalf("canceled capture=%d err=%v", len(capture.SVG), err)
	}
}

func TestCaptureDisplayPlanSVGPreservesCoreGlyphColor(t *testing.T) {
	input := coreGlyphPlanInput()
	input.GlyphRuns[0].Color = CoreRGBColor{R: 171, G: 205, B: 239, Set: true}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	capture, err := CaptureDisplayPlanSVG(plan, 1, nil)
	if err != nil {
		t.Fatalf("CaptureDisplayPlanSVG() = %v", err)
	}
	if !bytes.Contains(capture.SVG, []byte(`fill="#abcdef"`)) {
		t.Fatalf("display SVG omitted glyph color:\n%s", capture.SVG)
	}
}

func cropSVGFixture(t *testing.T) (LayoutPlan, DisplaySVGImageSources) {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	value.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	value.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, value); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	digestBytes := sha256.Sum256(encoded.Bytes())
	digest := ImageContentDigest(hex.EncodeToString(digestBytes[:]))
	input := imagePlanInput()
	input.ImageResources[0] = ImageResource{ID: 1, Digest: digest, Format: ImagePNG, PixelWidth: 2, PixelHeight: 1}
	input.Images[0].Crop = &ImageCrop{
		Intrinsic: Size{Width: 200, Height: 100},
		Source:    Rect{X: 100, Width: 100, Height: 100},
		Clip:      input.Images[0].Bounds,
	}
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	return plan, DisplaySVGImageSources{digest: encoded.Bytes()}
}
