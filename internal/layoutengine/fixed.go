// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package layoutengine contains the private canonical layout model and planner
// contracts shared by PaperRune's automatic-layout frontends.
package layoutengine

import (
	"errors"
	"math"
)

// Fixed is a signed fixed-point distance measured in PDF points. One point is
// FixedScale units. Fixed is the canonical coordinate representation inside
// the layout engine; floating-point values belong only at adapter and painter
// boundaries.
type Fixed int64

const (
	// FixedScale is the number of fixed units in one PDF point. It gives the
	// planner a resolution of 1/1024 point (about 0.00034 mm).
	FixedScale int64 = 1024

	// MinFixed and MaxFixed are the representable fixed-coordinate limits.
	MinFixed Fixed = -1 << 63
	MaxFixed Fixed = 1<<63 - 1
)

var (
	// ErrNonFiniteCoordinate reports a NaN or infinite point conversion.
	ErrNonFiniteCoordinate = errors.New("layoutengine: coordinate is not finite")
	// ErrGeometryOverflow reports geometry outside the signed fixed range.
	ErrGeometryOverflow = errors.New("layoutengine: fixed-point geometry overflow")
	// ErrNegativeExtent reports a negative width or height. Coordinates and
	// insets may be negative, but sizes and rectangles may not.
	ErrNegativeExtent = errors.New("layoutengine: geometry extent is negative")
	// ErrZeroDivisor reports an attempted fixed-point division by zero.
	ErrZeroDivisor = errors.New("layoutengine: fixed-point divisor is zero")
)

// FixedFromPoints converts PDF points to Fixed. Conversion rounds to the
// nearest fixed unit, with exact half units rounded away from zero. NaN,
// infinity, and values that do not fit in Fixed are rejected.
func FixedFromPoints(points float64) (Fixed, error) {
	if math.IsNaN(points) || math.IsInf(points, 0) {
		return 0, ErrNonFiniteCoordinate
	}

	scaled := math.Round(points * float64(FixedScale))
	// 2^63 is exactly representable in float64. Use an exclusive upper bound
	// because float64(MaxFixed) itself rounds up to 2^63.
	limit := math.Ldexp(1, 63)
	if math.IsInf(scaled, 0) || scaled < -limit || scaled >= limit {
		return 0, ErrGeometryOverflow
	}
	return Fixed(int64(scaled)), nil
}

// FixedFromIntPoints converts an integer number of PDF points without passing
// through floating point.
func FixedFromIntPoints(points int64) (Fixed, error) {
	value, ok := checkedFixedMul(points, FixedScale)
	if !ok {
		return 0, ErrGeometryOverflow
	}
	return Fixed(value), nil
}

// Points converts f to PDF points for an adapter or painter boundary.
func (f Fixed) Points() float64 {
	return float64(f) / float64(FixedScale)
}

// Add returns f+other or ErrGeometryOverflow when the sum is not
// representable.
func (f Fixed) Add(other Fixed) (Fixed, error) {
	value, ok := checkedFixedAdd(int64(f), int64(other))
	if !ok {
		return 0, ErrGeometryOverflow
	}
	return Fixed(value), nil
}

// Sub returns f-other or ErrGeometryOverflow when the difference is not
// representable.
func (f Fixed) Sub(other Fixed) (Fixed, error) {
	value, ok := checkedFixedSub(int64(f), int64(other))
	if !ok {
		return 0, ErrGeometryOverflow
	}
	return Fixed(value), nil
}

// Neg returns -f or ErrGeometryOverflow when f is MinFixed.
func (f Fixed) Neg() (Fixed, error) {
	if f == MinFixed {
		return 0, ErrGeometryOverflow
	}
	return -f, nil
}

// MulInt multiplies f by an integer without changing its fixed-point scale.
func (f Fixed) MulInt(factor int64) (Fixed, error) {
	value, ok := checkedFixedMul(int64(f), factor)
	if !ok {
		return 0, ErrGeometryOverflow
	}
	return Fixed(value), nil
}

// DivInt divides f by an integer without changing its fixed-point scale.
// Division truncates toward zero. Layout algorithms that distribute a
// remainder must do so explicitly to keep that policy visible and testable.
func (f Fixed) DivInt(divisor int64) (Fixed, error) {
	if divisor == 0 {
		return 0, ErrZeroDivisor
	}
	if f == MinFixed && divisor == -1 {
		return 0, ErrGeometryOverflow
	}
	return Fixed(int64(f) / divisor), nil
}

// Point is a position in fixed-point PDF coordinates.
type Point struct {
	X Fixed `json:"x"`
	Y Fixed `json:"y"`
}

// Translate returns p moved by dx and dy.
func (p Point) Translate(dx, dy Fixed) (Point, error) {
	x, err := p.X.Add(dx)
	if err != nil {
		return Point{}, err
	}
	y, err := p.Y.Add(dy)
	if err != nil {
		return Point{}, err
	}
	return Point{X: x, Y: y}, nil
}

// Size is a non-negative fixed-point width and height. Its zero value is a
// valid empty size.
type Size struct {
	Width  Fixed `json:"width"`
	Height Fixed `json:"height"`
}

// NewSize constructs and validates a Size.
func NewSize(width, height Fixed) (Size, error) {
	size := Size{Width: width, Height: height}
	if err := size.Validate(); err != nil {
		return Size{}, err
	}
	return size, nil
}

// Validate rejects negative dimensions.
func (s Size) Validate() error {
	if s.Width < 0 || s.Height < 0 {
		return ErrNegativeExtent
	}
	return nil
}

// IsEmpty reports whether either dimension is zero or negative. Negative
// dimensions are invalid, but treating them as empty keeps this predicate safe
// to use before validation.
func (s Size) IsEmpty() bool {
	return s.Width <= 0 || s.Height <= 0
}

// Insets describes distances from the top, right, bottom, and left edges.
// Negative values deliberately represent outsets; consumers may impose
// stricter policy for properties such as padding.
type Insets struct {
	Top    Fixed `json:"top"`
	Right  Fixed `json:"right"`
	Bottom Fixed `json:"bottom"`
	Left   Fixed `json:"left"`
}

// Horizontal returns Left+Right with overflow checking.
func (i Insets) Horizontal() (Fixed, error) {
	return i.Left.Add(i.Right)
}

// Vertical returns Top+Bottom with overflow checking.
func (i Insets) Vertical() (Fixed, error) {
	return i.Top.Add(i.Bottom)
}

// Rect is an axis-aligned rectangle represented by its origin and non-negative
// size. The right and bottom edges are exclusive for containment tests.
type Rect struct {
	X      Fixed `json:"x"`
	Y      Fixed `json:"y"`
	Width  Fixed `json:"width"`
	Height Fixed `json:"height"`
}

// NewRect constructs and validates a rectangle. A rectangle is invalid if its
// size is negative or either far edge is outside the Fixed range.
func NewRect(x, y, width, height Fixed) (Rect, error) {
	rect := Rect{X: x, Y: y, Width: width, Height: height}
	if err := rect.Validate(); err != nil {
		return Rect{}, err
	}
	return rect, nil
}

// RectFromPoints constructs a rectangle whose origin is min and whose far
// corner is max. Either axis in max before min is a negative extent.
func RectFromPoints(minimum, maximum Point) (Rect, error) {
	width, err := maximum.X.Sub(minimum.X)
	if err != nil {
		return Rect{}, err
	}
	height, err := maximum.Y.Sub(minimum.Y)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(minimum.X, minimum.Y, width, height)
}

// Validate rejects negative extents and unrepresentable far edges.
func (r Rect) Validate() error {
	if r.Width < 0 || r.Height < 0 {
		return ErrNegativeExtent
	}
	if _, err := r.Right(); err != nil {
		return err
	}
	if _, err := r.Bottom(); err != nil {
		return err
	}
	return nil
}

// Origin returns the rectangle's top-left point.
func (r Rect) Origin() Point {
	return Point{X: r.X, Y: r.Y}
}

// Size returns the rectangle's width and height.
func (r Rect) Size() Size {
	return Size{Width: r.Width, Height: r.Height}
}

// Right returns X+Width with overflow checking.
func (r Rect) Right() (Fixed, error) {
	return r.X.Add(r.Width)
}

// Bottom returns Y+Height with overflow checking.
func (r Rect) Bottom() (Fixed, error) {
	return r.Y.Add(r.Height)
}

// Maximum returns the rectangle's exclusive bottom-right corner.
func (r Rect) Maximum() (Point, error) {
	right, err := r.Right()
	if err != nil {
		return Point{}, err
	}
	bottom, err := r.Bottom()
	if err != nil {
		return Point{}, err
	}
	return Point{X: right, Y: bottom}, nil
}

// IsEmpty reports whether either extent is zero or negative. Negative extents
// are invalid, but treating them as empty keeps this predicate safe before
// validation.
func (r Rect) IsEmpty() bool {
	return r.Width <= 0 || r.Height <= 0
}

// Translate returns r moved by dx and dy. It also verifies that the translated
// far edges remain representable.
func (r Rect) Translate(dx, dy Fixed) (Rect, error) {
	if err := r.Validate(); err != nil {
		return Rect{}, err
	}
	origin, err := r.Origin().Translate(dx, dy)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(origin.X, origin.Y, r.Width, r.Height)
}

// Inset returns r reduced by i. Negative insets expand the rectangle. Insets
// that would produce a negative extent are rejected rather than clamped.
func (r Rect) Inset(i Insets) (Rect, error) {
	if err := r.Validate(); err != nil {
		return Rect{}, err
	}
	horizontal, err := i.Horizontal()
	if err != nil {
		return Rect{}, err
	}
	vertical, err := i.Vertical()
	if err != nil {
		return Rect{}, err
	}
	x, err := r.X.Add(i.Left)
	if err != nil {
		return Rect{}, err
	}
	y, err := r.Y.Add(i.Top)
	if err != nil {
		return Rect{}, err
	}
	width, err := r.Width.Sub(horizontal)
	if err != nil {
		return Rect{}, err
	}
	height, err := r.Height.Sub(vertical)
	if err != nil {
		return Rect{}, err
	}
	return NewRect(x, y, width, height)
}

// ContainsPoint reports whether p is inside r using half-open right and bottom
// edges. Invalid rectangles return an error.
func (r Rect) ContainsPoint(p Point) (bool, error) {
	if err := r.Validate(); err != nil {
		return false, err
	}
	right, _ := r.Right()
	bottom, _ := r.Bottom()
	return p.X >= r.X && p.X < right && p.Y >= r.Y && p.Y < bottom, nil
}

// Intersect returns the overlap between r and other. Disjoint rectangles
// produce an empty rectangle positioned at the nearest candidate overlap.
func (r Rect) Intersect(other Rect) (Rect, error) {
	if err := r.Validate(); err != nil {
		return Rect{}, err
	}
	if err := other.Validate(); err != nil {
		return Rect{}, err
	}
	right, _ := r.Right()
	bottom, _ := r.Bottom()
	otherRight, _ := other.Right()
	otherBottom, _ := other.Bottom()

	x := fixedMax(r.X, other.X)
	y := fixedMax(r.Y, other.Y)
	farX := fixedMin(right, otherRight)
	farY := fixedMin(bottom, otherBottom)
	if farX < x {
		farX = x
	}
	if farY < y {
		farY = y
	}
	return RectFromPoints(Point{X: x, Y: y}, Point{X: farX, Y: farY})
}

// Union returns the smallest rectangle containing r and other. Empty
// rectangles do not enlarge a non-empty rectangle.
func (r Rect) Union(other Rect) (Rect, error) {
	if err := r.Validate(); err != nil {
		return Rect{}, err
	}
	if err := other.Validate(); err != nil {
		return Rect{}, err
	}
	if r.IsEmpty() {
		return other, nil
	}
	if other.IsEmpty() {
		return r, nil
	}
	right, _ := r.Right()
	bottom, _ := r.Bottom()
	otherRight, _ := other.Right()
	otherBottom, _ := other.Bottom()
	return RectFromPoints(
		Point{X: fixedMin(r.X, other.X), Y: fixedMin(r.Y, other.Y)},
		Point{X: fixedMax(right, otherRight), Y: fixedMax(bottom, otherBottom)},
	)
}

func checkedFixedAdd(left, right int64) (int64, bool) {
	if right > 0 && left > int64(MaxFixed)-right {
		return 0, false
	}
	if right < 0 && left < int64(MinFixed)-right {
		return 0, false
	}
	return left + right, true
}

func checkedFixedSub(left, right int64) (int64, bool) {
	if right > 0 && left < int64(MinFixed)+right {
		return 0, false
	}
	if right < 0 && left > int64(MaxFixed)+right {
		return 0, false
	}
	return left - right, true
}

func checkedFixedMul(left, right int64) (int64, bool) {
	if left == 0 || right == 0 {
		return 0, true
	}
	if left == -1 {
		if right == int64(MinFixed) {
			return 0, false
		}
		return -right, true
	}
	if right == -1 {
		if left == int64(MinFixed) {
			return 0, false
		}
		return -left, true
	}
	product := left * right
	if product/right != left {
		return 0, false
	}
	return product, true
}

func fixedMin(left, right Fixed) Fixed {
	if left < right {
		return left
	}
	return right
}

func fixedMax(left, right Fixed) Fixed {
	if left > right {
		return left
	}
	return right
}
