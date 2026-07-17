// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"math/bits"
)

var ErrTransformInvalid = errors.New("layoutengine: fixed transform is invalid")

// Transform is a fixed-point PDF-style affine transform. A/B/C/D are
// dimensionless values scaled by FixedScale; TX/TY are fixed-point distances.
// The zero value is intentionally invalid, preventing accidental collapse.
type Transform struct {
	A  Fixed `json:"a"`
	B  Fixed `json:"b"`
	C  Fixed `json:"c"`
	D  Fixed `json:"d"`
	TX Fixed `json:"tx"`
	TY Fixed `json:"ty"`
}

func IdentityTransform() Transform { return Transform{A: Fixed(FixedScale), D: Fixed(FixedScale)} }

func TranslationTransform(x, y Fixed) Transform {
	result := IdentityTransform()
	result.TX, result.TY = x, y
	return result
}

// ScaleTransform accepts fixed dimensionless scale factors: FixedScale is 1.
func ScaleTransform(x, y Fixed) (Transform, error) {
	if x == 0 || y == 0 {
		return Transform{}, ErrTransformInvalid
	}
	return Transform{A: x, D: y}, nil
}

// QuarterTurnTransform rotates clockwise by an exact multiple of 90 degrees.
func QuarterTurnTransform(turns int) Transform {
	switch ((turns % 4) + 4) % 4 {
	case 0:
		return IdentityTransform()
	case 1:
		return Transform{B: Fixed(FixedScale), C: Fixed(-FixedScale)}
	case 2:
		return Transform{A: Fixed(-FixedScale), D: Fixed(-FixedScale)}
	default:
		return Transform{B: Fixed(-FixedScale), C: Fixed(FixedScale)}
	}
}

func (transform Transform) Validate() error {
	if transform == (Transform{}) {
		return ErrTransformInvalid
	}
	// A zero determinant collapses two-dimensional geometry.
	ad, err := fixedScalarProduct(transform.A, transform.D)
	if err != nil {
		return err
	}
	bc, err := fixedScalarProduct(transform.B, transform.C)
	if err != nil {
		return err
	}
	determinant, err := ad.Sub(bc)
	if err != nil {
		return err
	}
	if determinant == 0 {
		return ErrTransformInvalid
	}
	return nil
}

func (transform Transform) Apply(point Point) (Point, error) {
	if err := transform.Validate(); err != nil {
		return Point{}, err
	}
	ax, err := fixedScalarProduct(transform.A, point.X)
	if err != nil {
		return Point{}, err
	}
	cy, err := fixedScalarProduct(transform.C, point.Y)
	if err != nil {
		return Point{}, err
	}
	x, err := addFixed(ax, cy, transform.TX)
	if err != nil {
		return Point{}, err
	}
	bx, err := fixedScalarProduct(transform.B, point.X)
	if err != nil {
		return Point{}, err
	}
	dy, err := fixedScalarProduct(transform.D, point.Y)
	if err != nil {
		return Point{}, err
	}
	y, err := addFixed(bx, dy, transform.TY)
	if err != nil {
		return Point{}, err
	}
	return Point{X: x, Y: y}, nil
}

// Then composes transforms in reading order: t.Then(next) applies t first and
// next second.
func (transform Transform) Then(next Transform) (Transform, error) {
	if err := transform.Validate(); err != nil {
		return Transform{}, err
	}
	if err := next.Validate(); err != nil {
		return Transform{}, err
	}
	a, err := transformCoefficient(next.A, transform.A, next.C, transform.B)
	if err != nil {
		return Transform{}, err
	}
	b, err := transformCoefficient(next.B, transform.A, next.D, transform.B)
	if err != nil {
		return Transform{}, err
	}
	c, err := transformCoefficient(next.A, transform.C, next.C, transform.D)
	if err != nil {
		return Transform{}, err
	}
	d, err := transformCoefficient(next.B, transform.C, next.D, transform.D)
	if err != nil {
		return Transform{}, err
	}
	origin, err := next.Apply(Point{X: transform.TX, Y: transform.TY})
	if err != nil {
		return Transform{}, err
	}
	result := Transform{A: a, B: b, C: c, D: d, TX: origin.X, TY: origin.Y}
	if err := result.Validate(); err != nil {
		return Transform{}, err
	}
	return result, nil
}

func transformCoefficient(a, b, c, d Fixed) (Fixed, error) {
	ab, err := fixedScalarProduct(a, b)
	if err != nil {
		return 0, err
	}
	cd, err := fixedScalarProduct(c, d)
	if err != nil {
		return 0, err
	}
	return ab.Add(cd)
}

// Bounds transforms all four rectangle corners and returns their exact
// axis-aligned fixed-point bounding box.
func (transform Transform) Bounds(rect Rect) (Rect, error) {
	if err := rect.Validate(); err != nil {
		return Rect{}, err
	}
	right, err := rect.Right()
	if err != nil {
		return Rect{}, err
	}
	bottom, err := rect.Bottom()
	if err != nil {
		return Rect{}, err
	}
	points := []Point{{rect.X, rect.Y}, {right, rect.Y}, {rect.X, bottom}, {right, bottom}}
	for index := range points {
		points[index], err = transform.Apply(points[index])
		if err != nil {
			return Rect{}, err
		}
	}
	minX, maxX, minY, maxY := points[0].X, points[0].X, points[0].Y, points[0].Y
	for _, point := range points[1:] {
		minX, maxX = fixedMin(minX, point.X), fixedMax(maxX, point.X)
		minY, maxY = fixedMin(minY, point.Y), fixedMax(maxY, point.Y)
	}
	width, err := maxX.Sub(minX)
	if err != nil {
		return Rect{}, err
	}
	height, err := maxY.Sub(minY)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(minX, minY, width, height)
}

func fixedScalarProduct(left, right Fixed) (Fixed, error) {
	negative := (left < 0) != (right < 0)
	leftAbs := fixedAbsUint64(left)
	rightAbs := fixedAbsUint64(right)
	high, low := bits.Mul64(leftAbs, rightAbs)
	if high >= uint64(FixedScale) {
		return 0, ErrGeometryOverflow
	}
	quotient, remainder := bits.Div64(high, low, uint64(FixedScale))
	if remainder*2 >= uint64(FixedScale) {
		quotient++
	}
	limit := uint64(MaxFixed)
	if negative {
		limit++
	}
	if quotient > limit {
		return 0, ErrGeometryOverflow
	}
	if negative {
		if quotient == uint64(MaxFixed)+1 {
			return MinFixed, nil
		}
		return -Fixed(quotient), nil
	}
	return Fixed(quotient), nil
}

func fixedAbsUint64(value Fixed) uint64 {
	if value >= 0 {
		return uint64(value)
	}
	return uint64(-(value + 1)) + 1
}
