// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
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

func TestTrackSpanGeometryParity(t *testing.T) {
	sizes := []float64{12, 18, 30}
	offsets := TrackOffsets(sizes)
	if got := SpanSize(offsets, 1, 2); got != 48 {
		t.Fatalf("SpanSize() = %.2f, want 48", got)
	}
	if got := SumSpan(sizes, 1, 2); got != 48 {
		t.Fatalf("SumSpan() = %.2f, want 48", got)
	}
	if got := SpanSize(offsets, 2, 8); got != 30 {
		t.Fatalf("clipped SpanSize() = %.2f, want 30", got)
	}
	if got := SumSpan(sizes, -1, 2); got != 0 {
		t.Fatalf("invalid SumSpan() = %.2f, want 0", got)
	}
}
