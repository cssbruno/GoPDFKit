// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"testing"
)

func TestFixedTransformApplyComposeAndBounds(t *testing.T) {
	scale, err := ScaleTransform(2*Fixed(FixedScale), 3*Fixed(FixedScale))
	if err != nil {
		t.Fatal(err)
	}
	combined, err := scale.Then(TranslationTransform(5, 7))
	if err != nil {
		t.Fatal(err)
	}
	point, err := combined.Apply(Point{X: 10, Y: 20})
	if err != nil || point != (Point{X: 25, Y: 67}) {
		t.Fatalf("Apply() = %+v, %v", point, err)
	}
	bounds, err := QuarterTurnTransform(1).Then(TranslationTransform(100, 0))
	if err != nil {
		t.Fatal(err)
	}
	rect, err := bounds.Bounds(Rect{X: 10, Y: 20, Width: 30, Height: 40})
	if err != nil || rect != (Rect{X: 40, Y: 10, Width: 40, Height: 30}) {
		t.Fatalf("Bounds() = %+v, %v", rect, err)
	}
}

func TestFixedTransformRejectsCollapseAndOverflow(t *testing.T) {
	if _, err := ScaleTransform(0, Fixed(FixedScale)); !errors.Is(err, ErrTransformInvalid) {
		t.Fatalf("zero scale error = %v", err)
	}
	if err := (Transform{A: Fixed(FixedScale)}).Validate(); !errors.Is(err, ErrTransformInvalid) {
		t.Fatalf("collapsed transform error = %v", err)
	}
	if _, err := TranslationTransform(MaxFixed, 0).Apply(Point{X: 1}); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestFixedScalarProductRoundsHalfAwayFromZero(t *testing.T) {
	if got, err := fixedScalarProduct(1, Fixed(FixedScale/2)); err != nil || got != 1 {
		t.Fatalf("positive half = %d, %v", got, err)
	}
	if got, err := fixedScalarProduct(-1, Fixed(FixedScale/2)); err != nil || got != -1 {
		t.Fatalf("negative half = %d, %v", got, err)
	}
}
