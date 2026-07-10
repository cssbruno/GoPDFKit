// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

import "testing"

func TestFitImageParityGeometry(t *testing.T) {
	contain := FitImage(40, 20, 20, 20, ImageFitContain)
	if contain != (ImageFitResult{OffsetY: 5, Width: 20, Height: 10}) {
		t.Fatalf("contain fit = %#v", contain)
	}
	cover := FitImage(40, 20, 20, 20, ImageFitCover)
	if cover != (ImageFitResult{OffsetX: -10, Width: 40, Height: 20}) {
		t.Fatalf("cover fit = %#v", cover)
	}
}

func TestExceedsAvailableHeight(t *testing.T) {
	if ExceedsAvailableHeight(10, 10) {
		t.Fatal("equal height must fit")
	}
	if !ExceedsAvailableHeight(10.01, 10) {
		t.Fatal("larger content must move to the next page")
	}
}
