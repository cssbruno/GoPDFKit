// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// PointConvert returns the value of pt, expressed in points (1/72 inch), as a
// value expressed in the unit of measure specified in New(). Since font
// management in Document uses points, this method can help with line height
// calculations and other methods that require user units.
func (f *Document) PointConvert(pt float64) (u float64) {
	return pt / f.k
}

// PointToUnitConvert is an alias for PointConvert.
func (f *Document) PointToUnitConvert(pt float64) (u float64) {
	return pt / f.k
}

// UnitToPointConvert returns the value of u, expressed in the unit of measure
// specified in New(), as a value expressed in points (1/72 inch). Since font
// management in Document uses points, this method can help with setting font sizes
// based on the sizes of other non-font page elements.
func (f *Document) UnitToPointConvert(u float64) (pt float64) {
	return u * f.k
}

// Transform returns a point moved by the given X and Y offsets.
func (p *Point) Transform(x, y float64) Point {
	return Point{p.X + x, p.Y + y}
}

// Orientation returns "P" for portrait sizes, "L" for landscape sizes, and an
// empty string for nil or square sizes.
func (s *Size) Orientation() string {
	if s == nil || s.Ht == s.Wd {
		return ""
	}
	if s.Wd > s.Ht {
		return "L"
	}
	return "P"
}

// ScaleBy scales a size by factor.
func (s *Size) ScaleBy(factor float64) Size {
	return Size{s.Wd * factor, s.Ht * factor}
}

// ScaleToWidth adjusts the height of a size to match width.
func (s *Size) ScaleToWidth(width float64) Size {
	height := s.Ht * width / s.Wd
	return Size{width, height}
}

// ScaleToHeight adjusts the width of a size to match height.
func (s *Size) ScaleToHeight(height float64) Size {
	width := s.Wd * height / s.Ht
	return Size{width, height}
}
