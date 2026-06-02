/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

// Version of FPDF from which this package is derived
const (
	cnFpdfVersion = "1.9"
)

const (
	// OrientationPortrait represents the portrait orientation.
	OrientationPortrait = "portrait"

	// OrientationLandscape represents the landscape orientation.
	OrientationLandscape = "landscape"
)

const (
	// UnitPoint represents the size unit point
	UnitPoint = "pt"
	// UnitMillimeter represents the size unit millimeter
	UnitMillimeter = "mm"
	// UnitCentimeter represents the size unit centimeter
	UnitCentimeter = "cm"
	// UnitInch represents the size unit inch
	UnitInch = "inch"
)

const (
	// PageSizeA3 represents DIN/ISO A3 page size
	PageSizeA3 = "A3"
	// PageSizeA4 represents DIN/ISO A4 page size
	PageSizeA4 = "A4"
	// PageSizeA5 represents DIN/ISO A5 page size
	PageSizeA5 = "A5"
	// PageSizeLetter represents US Letter page size
	PageSizeLetter = "Letter"
	// PageSizeLegal represents US Legal page size
	PageSizeLegal = "Legal"
)

const (
	// BorderNone set no border
	BorderNone = ""
	// BorderFull sets a full border
	BorderFull = "1"
	// BorderLeft sets the border on the left side
	BorderLeft = "L"
	// BorderTop sets the border at the top
	BorderTop = "T"
	// BorderRight sets the border on the right side
	BorderRight = "R"
	// BorderBottom sets the border on the bottom
	BorderBottom = "B"
)

const (
	// LineBreakNone disables linebreak
	LineBreakNone = 0
	// LineBreakNormal enables normal linebreak
	LineBreakNormal = 1
	// LineBreakBelow enables linebreak below
	LineBreakBelow = 2
)

const (
	// AlignLeft left aligns the cell
	AlignLeft = "L"
	// AlignRight right aligns the cell
	AlignRight = "R"
	// AlignCenter centers the cell
	AlignCenter = "C"
	// AlignTop aligns the cell to the top
	AlignTop = "T"
	// AlignBottom aligns the cell to the bottom
	AlignBottom = "B"
	// AlignMiddle aligns the cell to the middle
	AlignMiddle = "M"
	// AlignBaseline aligns the cell to the baseline
	AlignBaseline = "B"
)

// Size fields Wd and Ht specify the horizontal and vertical extents of a
// document element such as a page.
type Size struct {
	Wd, Ht float64
}

// Point fields X and Y specify the horizontal and vertical coordinates of
// a point, typically used in drawing.
type Point struct {
	X, Y float64
}

// XY returns the X and Y components of the receiver point.
func (p Point) XY() (float64, float64) {
	return p.X, p.Y
}

// InitType is used with NewCustom() to customize an Fpdf instance.
// OrientationStr, UnitStr, SizeStr and FontDirStr correspond to the arguments
// accepted by New(). If the Wd and Ht fields of Size are each greater than
// zero, Size will be used to set the default page size rather than SizeStr. Wd
// and Ht are specified in the units of measure indicated by UnitStr.
type InitType struct {
	OrientationStr string
	UnitStr        string
	SizeStr        string
	Size           Size
	FontDirStr     string
}

// PageBox defines the coordinates and extent of the various page box types
type PageBox struct {
	Size
	Point
}
