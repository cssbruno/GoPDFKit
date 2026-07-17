// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"math"
	"testing"
)

func TestFixedFromPointsUsesOneOver1024PointResolution(t *testing.T) {
	tests := []struct {
		name   string
		points float64
		want   Fixed
	}{
		{name: "zero", points: 0, want: 0},
		{name: "one point", points: 1, want: 1024},
		{name: "negative points", points: -2.5, want: -2560},
		{name: "one fixed unit", points: 1.0 / 1024, want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := FixedFromPoints(test.points)
			if err != nil {
				t.Fatalf("FixedFromPoints(%g) error = %v", test.points, err)
			}
			if got != test.want {
				t.Fatalf("FixedFromPoints(%g) = %d, want %d", test.points, got, test.want)
			}
			if got.Points() != float64(got)/1024 {
				t.Fatalf("Fixed(%d).Points() = %g", got, got.Points())
			}
		})
	}
}

func TestFixedFromPointsRoundsHalfAwayFromZero(t *testing.T) {
	tests := []struct {
		name   string
		points float64
		want   Fixed
	}{
		{name: "positive below half", points: 0.499 / 1024, want: 0},
		{name: "positive half", points: 0.5 / 1024, want: 1},
		{name: "positive above half", points: 0.501 / 1024, want: 1},
		{name: "negative below half magnitude", points: -0.499 / 1024, want: 0},
		{name: "negative half", points: -0.5 / 1024, want: -1},
		{name: "negative above half magnitude", points: -0.501 / 1024, want: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := FixedFromPoints(test.points)
			if err != nil {
				t.Fatalf("FixedFromPoints(%g) error = %v", test.points, err)
			}
			if got != test.want {
				t.Fatalf("FixedFromPoints(%g) = %d, want %d", test.points, got, test.want)
			}
		})
	}
}

func TestFixedFromPointsRejectsNonFiniteAndOverflow(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := FixedFromPoints(value); !errors.Is(err, ErrNonFiniteCoordinate) {
			t.Fatalf("FixedFromPoints(%g) error = %v, want ErrNonFiniteCoordinate", value, err)
		}
	}

	positiveLimitPoints := math.Ldexp(1, 53)
	if _, err := FixedFromPoints(positiveLimitPoints); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("positive limit error = %v, want ErrGeometryOverflow", err)
	}
	minimum, err := FixedFromPoints(-positiveLimitPoints)
	if err != nil || minimum != MinFixed {
		t.Fatalf("negative limit = (%d, %v), want (%d, nil)", minimum, err, MinFixed)
	}
	if _, err := FixedFromPoints(math.Nextafter(-positiveLimitPoints, math.Inf(-1))); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("below negative limit error = %v, want ErrGeometryOverflow", err)
	}
}

func TestFixedFromIntPointsChecksOverflow(t *testing.T) {
	maximumPoints := int64(MaxFixed) / FixedScale
	got, err := FixedFromIntPoints(maximumPoints)
	if err != nil || got != Fixed(maximumPoints*FixedScale) {
		t.Fatalf("FixedFromIntPoints(maximum) = (%d, %v)", got, err)
	}
	if _, err := FixedFromIntPoints(maximumPoints + 1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("above maximum error = %v, want ErrGeometryOverflow", err)
	}

	minimumPoints := int64(MinFixed) / FixedScale
	got, err = FixedFromIntPoints(minimumPoints)
	if err != nil || got != MinFixed {
		t.Fatalf("FixedFromIntPoints(minimum) = (%d, %v), want (%d, nil)", got, err, MinFixed)
	}
	if _, err := FixedFromIntPoints(minimumPoints - 1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("below minimum error = %v, want ErrGeometryOverflow", err)
	}
}

func TestFixedCheckedArithmetic(t *testing.T) {
	if got, err := Fixed(12).Add(30); err != nil || got != 42 {
		t.Fatalf("Add() = (%d, %v), want (42, nil)", got, err)
	}
	if _, err := MaxFixed.Add(1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Add() error = %v", err)
	}
	if _, err := MinFixed.Add(-1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("underflowing Add() error = %v", err)
	}

	if got, err := Fixed(12).Sub(30); err != nil || got != -18 {
		t.Fatalf("Sub() = (%d, %v), want (-18, nil)", got, err)
	}
	if _, err := MinFixed.Sub(1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("underflowing Sub() error = %v", err)
	}
	if _, err := MaxFixed.Sub(-1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Sub() error = %v", err)
	}

	if got, err := Fixed(-7).Neg(); err != nil || got != 7 {
		t.Fatalf("Neg() = (%d, %v), want (7, nil)", got, err)
	}
	if _, err := MinFixed.Neg(); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Neg() error = %v", err)
	}

	if got, err := Fixed(-7).MulInt(3); err != nil || got != -21 {
		t.Fatalf("MulInt() = (%d, %v), want (-21, nil)", got, err)
	}
	if _, err := MaxFixed.MulInt(2); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing MulInt() error = %v", err)
	}
	if _, err := MinFixed.MulInt(-1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("MinFixed.MulInt(-1) error = %v", err)
	}

	if got, err := Fixed(-7).DivInt(2); err != nil || got != -3 {
		t.Fatalf("DivInt() = (%d, %v), want (-3, nil)", got, err)
	}
	if _, err := Fixed(1).DivInt(0); !errors.Is(err, ErrZeroDivisor) {
		t.Fatalf("DivInt(0) error = %v", err)
	}
	if _, err := MinFixed.DivInt(-1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("MinFixed.DivInt(-1) error = %v", err)
	}
}

func TestPointSizeAndInsetsInvariants(t *testing.T) {
	point, err := (Point{X: 10, Y: -5}).Translate(3, 7)
	if err != nil || point != (Point{X: 13, Y: 2}) {
		t.Fatalf("Point.Translate() = (%#v, %v)", point, err)
	}
	if _, err := (Point{X: MaxFixed}).Translate(1, 0); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Point.Translate() error = %v", err)
	}

	if _, err := NewSize(-1, 10); !errors.Is(err, ErrNegativeExtent) {
		t.Fatalf("NewSize(-1, 10) error = %v", err)
	}
	if !(Size{Width: 0, Height: 10}).IsEmpty() {
		t.Fatal("zero-width size must be empty")
	}
	if (Size{Width: 1, Height: 1}).IsEmpty() {
		t.Fatal("positive size must not be empty")
	}

	insets := Insets{Top: 1, Right: 2, Bottom: 3, Left: 4}
	if got, err := insets.Horizontal(); err != nil || got != 6 {
		t.Fatalf("Horizontal() = (%d, %v), want (6, nil)", got, err)
	}
	if got, err := insets.Vertical(); err != nil || got != 4 {
		t.Fatalf("Vertical() = (%d, %v), want (4, nil)", got, err)
	}
	if _, err := (Insets{Left: MaxFixed, Right: 1}).Horizontal(); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Horizontal() error = %v", err)
	}
}

func TestRectConstructionAndFarEdges(t *testing.T) {
	rect, err := NewRect(10, 20, 30, 40)
	if err != nil {
		t.Fatalf("NewRect() error = %v", err)
	}
	if rect.Origin() != (Point{X: 10, Y: 20}) || rect.Size() != (Size{Width: 30, Height: 40}) {
		t.Fatalf("rect accessors = origin %#v, size %#v", rect.Origin(), rect.Size())
	}
	if right, err := rect.Right(); err != nil || right != 40 {
		t.Fatalf("Right() = (%d, %v), want (40, nil)", right, err)
	}
	if bottom, err := rect.Bottom(); err != nil || bottom != 60 {
		t.Fatalf("Bottom() = (%d, %v), want (60, nil)", bottom, err)
	}
	if maximum, err := rect.Maximum(); err != nil || maximum != (Point{X: 40, Y: 60}) {
		t.Fatalf("Maximum() = (%#v, %v)", maximum, err)
	}

	fromPoints, err := RectFromPoints(Point{X: 10, Y: 20}, Point{X: 40, Y: 60})
	if err != nil || fromPoints != rect {
		t.Fatalf("RectFromPoints() = (%#v, %v), want %#v", fromPoints, err, rect)
	}
	if _, err := RectFromPoints(Point{X: 2}, Point{X: 1}); !errors.Is(err, ErrNegativeExtent) {
		t.Fatalf("reversed RectFromPoints() error = %v", err)
	}
	if _, err := NewRect(0, 0, -1, 1); !errors.Is(err, ErrNegativeExtent) {
		t.Fatalf("negative NewRect() error = %v", err)
	}
	if _, err := NewRect(MaxFixed, 0, 1, 1); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("far-edge NewRect() error = %v", err)
	}
}

func TestRectContainmentIsHalfOpen(t *testing.T) {
	rect, err := NewRect(10, 20, 30, 40)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		point Point
		want  bool
	}{
		{point: Point{X: 10, Y: 20}, want: true},
		{point: Point{X: 39, Y: 59}, want: true},
		{point: Point{X: 40, Y: 20}, want: false},
		{point: Point{X: 10, Y: 60}, want: false},
		{point: Point{X: 9, Y: 20}, want: false},
	}
	for _, test := range tests {
		got, err := rect.ContainsPoint(test.point)
		if err != nil || got != test.want {
			t.Errorf("ContainsPoint(%#v) = (%v, %v), want (%v, nil)", test.point, got, err, test.want)
		}
	}
	if got, err := (Rect{Width: -1, Height: 1}).ContainsPoint(Point{}); got || !errors.Is(err, ErrNegativeExtent) {
		t.Fatalf("invalid ContainsPoint() = (%v, %v)", got, err)
	}
}

func TestRectTranslateInsetIntersectAndUnion(t *testing.T) {
	rect, err := NewRect(10, 20, 30, 40)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := rect.Translate(-5, 10)
	if err != nil || moved != (Rect{X: 5, Y: 30, Width: 30, Height: 40}) {
		t.Fatalf("Translate() = (%#v, %v)", moved, err)
	}
	if _, err := (Rect{X: MaxFixed - 1, Width: 1, Height: 1}).Translate(1, 0); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Translate() error = %v", err)
	}

	inset, err := rect.Inset(Insets{Top: 2, Right: 3, Bottom: 4, Left: 5})
	if err != nil || inset != (Rect{X: 15, Y: 22, Width: 22, Height: 34}) {
		t.Fatalf("Inset() = (%#v, %v)", inset, err)
	}
	outset, err := rect.Inset(Insets{Top: -2, Right: -2, Bottom: -2, Left: -2})
	if err != nil || outset != (Rect{X: 8, Y: 18, Width: 34, Height: 44}) {
		t.Fatalf("negative Inset() = (%#v, %v)", outset, err)
	}
	if _, err := rect.Inset(Insets{Left: 20, Right: 11}); !errors.Is(err, ErrNegativeExtent) {
		t.Fatalf("over-inset error = %v", err)
	}

	overlap, err := rect.Intersect(Rect{X: 25, Y: 10, Width: 30, Height: 30})
	if err != nil || overlap != (Rect{X: 25, Y: 20, Width: 15, Height: 20}) {
		t.Fatalf("Intersect() = (%#v, %v)", overlap, err)
	}
	disjoint, err := rect.Intersect(Rect{X: 100, Y: 100, Width: 1, Height: 1})
	if err != nil || !disjoint.IsEmpty() || disjoint.X != 100 || disjoint.Y != 100 {
		t.Fatalf("disjoint Intersect() = (%#v, %v)", disjoint, err)
	}

	union, err := rect.Union(Rect{X: 5, Y: 10, Width: 10, Height: 20})
	if err != nil || union != (Rect{X: 5, Y: 10, Width: 35, Height: 50}) {
		t.Fatalf("Union() = (%#v, %v)", union, err)
	}
	if got, err := rect.Union(Rect{X: -100, Y: -100}); err != nil || got != rect {
		t.Fatalf("Union(empty) = (%#v, %v), want %#v", got, err, rect)
	}
	if _, err := (Rect{X: MinFixed, Width: 1, Height: 1}).Union(Rect{X: MaxFixed - 1, Width: 1, Height: 1}); !errors.Is(err, ErrGeometryOverflow) {
		t.Fatalf("overflowing Union() error = %v", err)
	}
}
