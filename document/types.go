// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// cnDocumentVersion is the producer version emitted by this package.
const (
	cnDocumentVersion = "1.9"
)

const (
	// OrientationPortrait represents the portrait orientation.
	OrientationPortrait = "portrait"

	// OrientationLandscape represents the landscape orientation.
	OrientationLandscape = "landscape"
)

const (
	// UnitPoint represents points.
	UnitPoint = "pt"
	// UnitMillimeter represents millimeters.
	UnitMillimeter = "mm"
	// UnitCentimeter represents centimeters.
	UnitCentimeter = "cm"
	// UnitInch represents inches.
	UnitInch = "inch"
)

const (
	// PageSizeA3 represents the DIN/ISO A3 page size.
	PageSizeA3 = "A3"
	// PageSizeA4 represents the DIN/ISO A4 page size.
	PageSizeA4 = "A4"
	// PageSizeA5 represents the DIN/ISO A5 page size.
	PageSizeA5 = "A5"
	// PageSizeLetter represents the US Letter page size.
	PageSizeLetter = "Letter"
	// PageSizeLegal represents the US Legal page size.
	PageSizeLegal = "Legal"
)

const (
	// BorderNone draws no border.
	BorderNone = ""
	// BorderFull draws a full border.
	BorderFull = "1"
	// BorderLeft draws the left border.
	BorderLeft = "L"
	// BorderTop draws the top border.
	BorderTop = "T"
	// BorderRight draws the right border.
	BorderRight = "R"
	// BorderBottom draws the bottom border.
	BorderBottom = "B"
)

const (
	// LineBreakNone disables line breaks.
	LineBreakNone = 0
	// LineBreakNormal enables normal line breaks.
	LineBreakNormal = 1
	// LineBreakBelow enables a line break below the current element.
	LineBreakBelow = 2
)

const (
	// AlignLeft aligns the cell content to the left.
	AlignLeft = "L"
	// AlignRight aligns the cell content to the right.
	AlignRight = "R"
	// AlignCenter centers the cell content.
	AlignCenter = "C"
	// AlignTop aligns the cell content to the top.
	AlignTop = "T"
	// AlignBottom aligns the cell content to the bottom.
	AlignBottom = "B"
	// AlignMiddle vertically centers the cell content.
	AlignMiddle = "M"
	// AlignBaseline aligns the cell content to the baseline.
	AlignBaseline = "B"
)

// Size fields Wd and Ht specify the horizontal and vertical extents of a
// document element such as a page.
type Size struct {
	Wd float64 // Width.
	Ht float64 // Height.
}

// Point fields X and Y specify the horizontal and vertical coordinates of
// a point, typically used in drawing.
type Point struct {
	X float64 // Horizontal coordinate.
	Y float64 // Vertical coordinate.
}

// XY returns the X and Y components of the receiver point.
func (p Point) XY() (float64, float64) {
	return p.X, p.Y
}

// InitType is used with NewCustom to customize a Document instance.
// OrientationStr, UnitStr, SizeStr, and FontDirStr correspond to the arguments
// accepted by New. If the Wd and Ht fields of Size are each greater than
// zero, Size will be used to set the default page size rather than SizeStr. Wd
// and Ht are specified in the units of measure indicated by UnitStr.
type InitType struct {
	OrientationStr string // Default page orientation.
	UnitStr        string // Document unit of measure.
	SizeStr        string // Named page size.
	Size           Size   // Explicit page size override.
	FontDirStr     string // Font resource directory.
}

// PageBox defines the coordinates and extent of a PDF page box.
type PageBox struct {
	Size
	Point
}
