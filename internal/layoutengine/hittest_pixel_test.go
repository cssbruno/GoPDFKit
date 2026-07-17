// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"testing"
)

func TestHitTestRasterPixelMapsPixelCenterToSourceGeometry(t *testing.T) {
	plan, err := NewLayoutPlan(overlappingHitTestPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	hit, err := plan.HitTestRasterPixel(RasterPixelQuery{
		Page: 1, PixelX: 3, PixelY: 3, PixelWidth: 20, PixelHeight: 30,
		CaptureBounds: Rect{Width: 200, Height: 300},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hit.PagePoint != (Point{X: 35, Y: 35}) || len(hit.Hit.Fragments) != 2 ||
		hit.Hit.Fragments[0].Key != "@child" || hit.Hit.Commands[0].Key != "@child" {
		t.Fatalf("pixel hit = %#v", hit)
	}
}

func TestHitTestRasterPixelUsesDeclaredCropWithoutDPIAssumptions(t *testing.T) {
	plan, err := NewLayoutPlan(overlappingHitTestPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	hit, err := plan.HitTestRasterPixel(RasterPixelQuery{
		Page: 1, PixelX: 0, PixelY: 0, PixelWidth: 4, PixelHeight: 4,
		CaptureBounds: Rect{X: 30, Y: 30, Width: 40, Height: 40},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hit.PagePoint != (Point{X: 35, Y: 35}) || hit.Hit.Fragments[0].Fragment != 2 {
		t.Fatalf("cropped pixel hit = %#v", hit)
	}
}

func TestHitTestRasterPixelRoundsExactHalfAwayFromZero(t *testing.T) {
	if got, err := rasterPixelCenterOffset(2, 0, 2); err != nil || got != 1 {
		t.Fatalf("half offset = %d, %v; want 1", got, err)
	}
}

func TestHitTestRasterPixelRejectsUntrustedRasterMetadata(t *testing.T) {
	plan, err := NewLayoutPlan(overlappingHitTestPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	tests := []RasterPixelQuery{
		{Page: 1, PixelHeight: 1, CaptureBounds: Rect{Width: 1, Height: 1}},
		{Page: 1, PixelWidth: HitTestMaxRasterDimension + 1, PixelHeight: 1, CaptureBounds: Rect{Width: 1, Height: 1}},
		{Page: 1, PixelX: 1, PixelWidth: 1, PixelHeight: 1, CaptureBounds: Rect{Width: 1, Height: 1}},
		{Page: 1, PixelWidth: 1, PixelHeight: 1, CaptureBounds: Rect{}},
		{Page: 1, PixelWidth: 1, PixelHeight: 1, CaptureBounds: Rect{Width: -1, Height: 1}},
	}
	for index, query := range tests {
		if _, err := plan.HitTestRasterPixel(query); !errors.Is(err, ErrHitTestRasterInvalid) {
			t.Fatalf("case %d error = %v, want ErrHitTestRasterInvalid", index, err)
		}
	}
	if _, err := plan.HitTestRasterPixel(RasterPixelQuery{
		Page: 2, PixelWidth: 1, PixelHeight: 1, CaptureBounds: Rect{Width: 1, Height: 1},
	}); !errors.Is(err, ErrHitTestPageNotFound) {
		t.Fatalf("invalid page error = %v", err)
	}
}
