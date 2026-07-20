// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestFitImageResolvesAutomaticAspectDimension(t *testing.T) {
	input := imageFitFixture()
	input.Width = ImageLength{Kind: ImageLengthFixed, Value: 300}
	input.Height = ImageLength{Kind: ImageLengthAuto}
	result, err := FitImage(context.Background(), input, ImageFitLimits{})
	if err != nil {
		t.Fatalf("FitImage() = %v", err)
	}
	want := Rect{X: 10, Y: 20, Width: 300, Height: 150}
	if result.Box != want || result.ObjectBounds != want || result.Destination != want {
		t.Fatalf("resolved geometry = box %+v object %+v destination %+v, want %+v", result.Box, result.ObjectBounds, result.Destination, want)
	}
	if result.SourceCrop != (Rect{Width: 400, Height: 200}) || result.RequiresCrop {
		t.Fatalf("source crop = %+v, requires crop %v", result.SourceCrop, result.RequiresCrop)
	}
	if result.Work != 4 {
		t.Fatalf("work = %d, want 4", result.Work)
	}
}

func TestFitImagePoliciesProduceCanonicalGeometry(t *testing.T) {
	tests := []struct {
		name          string
		policy        ImageFitPolicy
		intrinsic     Size
		box           Size
		wantObject    Rect
		wantDest      Rect
		wantCrop      Rect
		wantEffective ImageFitPolicy
	}{
		{
			name: "contain", policy: ImageFitContain, intrinsic: Size{Width: 400, Height: 200}, box: Size{Width: 200, Height: 200},
			wantObject: Rect{X: 10, Y: 70, Width: 200, Height: 100}, wantDest: Rect{X: 10, Y: 70, Width: 200, Height: 100},
			wantCrop: Rect{Width: 400, Height: 200}, wantEffective: ImageFitContain,
		},
		{
			name: "cover", policy: ImageFitCover, intrinsic: Size{Width: 400, Height: 200}, box: Size{Width: 200, Height: 200},
			wantObject: Rect{X: -90, Y: 20, Width: 400, Height: 200}, wantDest: Rect{X: 10, Y: 20, Width: 200, Height: 200},
			wantCrop: Rect{X: 100, Width: 200, Height: 200}, wantEffective: ImageFitCover,
		},
		{
			name: "fill", policy: ImageFitFill, intrinsic: Size{Width: 400, Height: 200}, box: Size{Width: 200, Height: 200},
			wantObject: Rect{X: 10, Y: 20, Width: 200, Height: 200}, wantDest: Rect{X: 10, Y: 20, Width: 200, Height: 200},
			wantCrop: Rect{Width: 400, Height: 200}, wantEffective: ImageFitFill,
		},
		{
			name: "none clips", policy: ImageFitNone, intrinsic: Size{Width: 400, Height: 200}, box: Size{Width: 200, Height: 100},
			wantObject: Rect{X: -90, Y: -30, Width: 400, Height: 200}, wantDest: Rect{X: 10, Y: 20, Width: 200, Height: 100},
			wantCrop: Rect{X: 100, Y: 50, Width: 200, Height: 100}, wantEffective: ImageFitNone,
		},
		{
			name: "scale down chooses none", policy: ImageFitScaleDown, intrinsic: Size{Width: 100, Height: 50}, box: Size{Width: 200, Height: 200},
			wantObject: Rect{X: 60, Y: 95, Width: 100, Height: 50}, wantDest: Rect{X: 60, Y: 95, Width: 100, Height: 50},
			wantCrop: Rect{Width: 100, Height: 50}, wantEffective: ImageFitNone,
		},
		{
			name: "scale down chooses contain", policy: ImageFitScaleDown, intrinsic: Size{Width: 400, Height: 200}, box: Size{Width: 200, Height: 100},
			wantObject: Rect{X: 10, Y: 20, Width: 200, Height: 100}, wantDest: Rect{X: 10, Y: 20, Width: 200, Height: 100},
			wantCrop: Rect{Width: 400, Height: 200}, wantEffective: ImageFitContain,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := imageFitFixture()
			input.Fit, input.Intrinsic = test.policy, test.intrinsic
			input.Width = ImageLength{Kind: ImageLengthFixed, Value: test.box.Width}
			input.Height = ImageLength{Kind: ImageLengthFixed, Value: test.box.Height}
			first, err := FitImage(context.Background(), input, ImageFitLimits{})
			if err != nil {
				t.Fatalf("FitImage() = %v", err)
			}
			second, err := FitImage(context.Background(), input, ImageFitLimits{})
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("second FitImage() = %+v, %v; first %+v", second, err, first)
			}
			if first.ObjectBounds != test.wantObject || first.Destination != test.wantDest || first.SourceCrop != test.wantCrop || first.EffectiveFit != test.wantEffective {
				t.Fatalf("result = object %+v destination %+v crop %+v effective %q", first.ObjectBounds, first.Destination, first.SourceCrop, first.EffectiveFit)
			}
		})
	}
}

func TestFitImageAlignmentSelectsCoverCrop(t *testing.T) {
	input := imageFitFixture()
	input.Width = ImageLength{Kind: ImageLengthFixed, Value: 200}
	input.Height = ImageLength{Kind: ImageLengthFixed, Value: 200}
	input.Fit = ImageFitCover
	input.Alignment.Horizontal = ImageAlignEnd
	result, err := FitImage(context.Background(), input, ImageFitLimits{})
	if err != nil {
		t.Fatalf("FitImage() = %v", err)
	}
	if result.SourceCrop != (Rect{X: 200, Width: 200, Height: 200}) {
		t.Fatalf("end-aligned crop = %+v", result.SourceCrop)
	}
	input.Alignment.Horizontal = ImageAlignStart
	result, err = FitImage(context.Background(), input, ImageFitLimits{})
	if err != nil || result.SourceCrop != (Rect{Width: 200, Height: 200}) {
		t.Fatalf("start-aligned crop = %+v, %v", result.SourceCrop, err)
	}
}

func TestFitImagePlacementPreservesProvenance(t *testing.T) {
	input := imageFitFixture()
	result, err := FitImage(context.Background(), input, ImageFitLimits{})
	if err != nil {
		t.Fatalf("FitImage() = %v", err)
	}
	placement := result.PlannedImage()
	if placement.Resource != input.Resource || placement.Fragment != input.Fragment || placement.Source != input.Source || placement.Bounds != result.Destination {
		t.Fatalf("planned image = %+v", placement)
	}
	if result.Node != input.Node || result.Key != input.Key || result.Instance != input.Instance {
		t.Fatalf("result provenance = %+v", result)
	}
}

func TestFitImageLowersExactCropPayload(t *testing.T) {
	input := imageFitFixture()
	input.Width = ImageLength{Kind: ImageLengthFixed, Value: 200}
	input.Height = ImageLength{Kind: ImageLengthFixed, Value: 200}
	input.Fit = ImageFitCover
	result, err := FitImage(context.Background(), input, ImageFitLimits{})
	if err != nil {
		t.Fatalf("FitImage() = %v", err)
	}
	if !result.RequiresCrop {
		t.Fatal("cover result unexpectedly claims no crop")
	}
	placement := result.PlannedImage()
	if placement.Crop == nil || *placement.Crop != (ImageCrop{Intrinsic: result.Intrinsic, Source: result.SourceCrop, Clip: result.Destination}) {
		t.Fatalf("planned crop = %+v", placement.Crop)
	}
}

func TestFitImageReportsMissingAndInvalidDimensions(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ImageFitInput)
		cause  error
		code   DiagnosticCode
	}{
		{"missing intrinsic", func(input *ImageFitInput) { input.Intrinsic.Width = 0 }, ErrImageIntrinsicMissing, DiagnosticImageMissing},
		{"negative intrinsic", func(input *ImageFitInput) { input.Intrinsic.Width = -1 }, ErrImageDimensionInvalid, DiagnosticImageDimensionInvalid},
		{"invalid fixed width", func(input *ImageFitInput) { input.Width = ImageLength{Kind: ImageLengthFixed} }, ErrImageDimensionInvalid, DiagnosticImageDimensionInvalid},
		{"auto with value", func(input *ImageFitInput) { input.Width = ImageLength{Kind: ImageLengthAuto, Value: 1} }, ErrImageDimensionInvalid, DiagnosticImageDimensionInvalid},
		{"invalid policy", func(input *ImageFitInput) { input.Fit = "zoom" }, ErrImageFitPolicyInvalid, DiagnosticImageFitInvalid},
		{"invalid alignment", func(input *ImageFitInput) { input.Alignment.Horizontal = "baseline" }, ErrImageAlignmentInvalid, DiagnosticImageFitInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := imageFitFixture()
			test.mutate(&input)
			_, err := FitImage(context.Background(), input, ImageFitLimits{})
			if !errors.Is(err, test.cause) {
				t.Fatalf("FitImage() error = %v, want %v", err, test.cause)
			}
			var planning *PlanningError
			if !errors.As(err, &planning) || planning.Diagnostic.Code != test.code {
				t.Fatalf("diagnostic = %#v", err)
			}
		})
	}
}

func TestFitImageEnforcesCancellationWorkAndStateLimits(t *testing.T) {
	input := imageFitFixture()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := FitImage(canceled, input, ImageFitLimits{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled FitImage() = %v", err)
	}
	var planning *PlanningError
	if !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticCanceled {
		t.Fatalf("canceled diagnostic = %#v", err)
	}

	limits := DefaultImageFitLimits()
	limits.MaxWork = 3
	_, err = FitImage(context.Background(), input, limits)
	if !errors.Is(err, ErrImageFitWorkLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticWorkLimit {
		t.Fatalf("work-limited FitImage() = %#v", err)
	}

	limits = DefaultImageFitLimits()
	limits.MaxDimension = 399
	_, err = FitImage(context.Background(), input, limits)
	if !errors.Is(err, ErrImageFitStateLimit) || !errors.As(err, &planning) || planning.Diagnostic.Code != DiagnosticResourceLimit {
		t.Fatalf("state-limited FitImage() = %#v", err)
	}

	limits = DefaultImageFitLimits()
	limits.MaxWork = hardMaxImageFitWork + 1
	_, err = FitImage(context.Background(), input, limits)
	if !errors.Is(err, ErrImageFitLimitsInvalid) {
		t.Fatalf("invalid limits FitImage() = %v", err)
	}
}

func TestFitImageUsesExactWideFixedArithmetic(t *testing.T) {
	input := imageFitFixture()
	input.Intrinsic = Size{Width: 1 << 50, Height: 1 << 49}
	input.Width = ImageLength{Kind: ImageLengthFixed, Value: 1 << 51}
	input.Height = ImageLength{Kind: ImageLengthAuto}
	result, err := FitImage(context.Background(), input, ImageFitLimits{})
	if err != nil {
		t.Fatalf("FitImage() = %v", err)
	}
	if result.Box.Size() != (Size{Width: 1 << 51, Height: 1 << 50}) {
		t.Fatalf("wide resolved box = %+v", result.Box.Size())
	}
}

func imageFitFixture() ImageFitInput {
	return ImageFitInput{
		Resource: 1, Fragment: 7, Node: 9, Key: "@hero-image", Instance: "@hero-image",
		Source: SourceSpan{
			File:  "invoice.paper",
			Start: SourcePosition{Offset: 10, Line: 2, Column: 3},
			End:   SourcePosition{Offset: 20, Line: 2, Column: 13},
		},
		Position: Point{X: 10, Y: 20}, Intrinsic: Size{Width: 400, Height: 200},
		Width: ImageLength{Kind: ImageLengthAuto}, Height: ImageLength{Kind: ImageLengthAuto},
		Fit:       ImageFitContain,
		Alignment: ImageAlignment{Horizontal: ImageAlignCenter, Vertical: ImageAlignCenter},
	}
}
