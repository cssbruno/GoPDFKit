/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *                     *
 ****************************************************************************/

package gopdfkit

type SVGSegment struct {
	Cmd byte
	Arg [6]float64
}

type SVGStyle struct {
	Color        CSSColorType
	Stroke       CSSColorType
	Fill         CSSColorType
	FillGradient SVGGradient
	FillPattern  SVGPattern
	StrokeWidth  float64
	FillRule     string
	ClipPath     []SVGSegment // SVGStyle describes drawing attributes parsed from SVG presentation
	// attributes or style declarations.

	ClipRule         string
	ClipRef          string
	FillRef          string
	FontSize         float64
	TextAnchor       string
	StrokeLineCap    string
	StrokeLineJoin   string
	StrokeDashArray  []float64
	StrokeDashOffset float64
	Opacity          float64
	FillOpacity      float64
	StrokeOpacity    float64
	OpacitySet       bool
	FillOpacitySet   bool
	StrokeOpacitySet bool
	StrokeDashSet    bool
	Hidden           bool
}

// SVGGradientStop describes one color stop in a parsed SVG gradient.

type SVGGradientStop struct {
	Offset  float64
	Color   CSSColorType
	Opacity float64
}

type SVGGradient struct {
	Set   bool
	Kind  string
	Units string
	X1    float64
	Y1    float64
	X2    float64
	Y2    float64
	CX    float64
	CY    float64
	FX    float64
	FY    float64
	R     float64
	Stops []SVGGradientStop // SVGGradient describes a linear or radial SVG gradient that SVGWrite can
	// render as a PDF shading fill.

}

type SVGPattern struct {
	Set      bool
	Units    string
	X, Y     float64
	Wd, Ht   float64
	Elements []SVGElement // SVGPattern describes an internal SVG pattern fill that SVGWrite can tile
	// inside a path.

}

type SVGPath struct {
	Segments []SVGSegment // SVGPath describes one styled SVG path.

	Style SVGStyle
}

// SVGText describes one SVG text element.

type SVGText struct {
	X, Y  float64
	Text  string
	Style SVGStyle
}

// SVGElement describes one drawable SVG element in document order.

type SVGElement struct {
	Kind  string
	Path  SVGPath
	Text  SVGText
	Image SVGImage
}

type SVG struct {
	Wd, Ht   float64
	Segments [][]SVGSegment // SVG aggregates the information needed to describe a multi-segment
	// vector image

	Paths    []SVGPath
	Texts    []SVGText
	Images   []SVGImage
	Elements []SVGElement
}
