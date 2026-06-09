// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

// SVGSegment describes one normalized SVG path command and its arguments.
type SVGSegment struct {
	Cmd byte       // Normalized path command.
	Arg [6]float64 // Command arguments.
}

// SVGStyle describes drawing attributes parsed from SVG presentation
// attributes or style declarations.
type SVGStyle struct {
	Color            CSSColorType // Current text or inherited color.
	Stroke           CSSColorType // Stroke paint.
	Fill             CSSColorType // Fill paint.
	FillGradient     SVGGradient  // Fill gradient, when present.
	FillPattern      SVGPattern   // Fill pattern, when present.
	StrokeWidth      float64      // Stroke width.
	FillRule         string       // Fill rule, such as nonzero or evenodd.
	ClipPath         []SVGSegment // Inline clip path segments.
	ClipRule         string       // Clip rule.
	ClipRef          string       // Referenced clip path ID.
	FillRef          string       // Referenced fill paint ID.
	FontSize         float64      // Text font size.
	TextAnchor       string       // SVG text-anchor value.
	StrokeLineCap    string       // Stroke line-cap style.
	StrokeLineJoin   string       // Stroke line-join style.
	StrokeDashArray  []float64    // Stroke dash pattern.
	StrokeDashOffset float64      // Stroke dash phase.
	Opacity          float64      // Overall opacity.
	FillOpacity      float64      // Fill opacity.
	StrokeOpacity    float64      // Stroke opacity.
	OpacitySet       bool         // Whether Opacity was explicitly set.
	FillOpacitySet   bool         // Whether FillOpacity was explicitly set.
	StrokeOpacitySet bool         // Whether StrokeOpacity was explicitly set.
	StrokeDashSet    bool         // Whether a dash pattern was explicitly set.
	Hidden           bool         // Whether the element should not render.
}

// SVGGradientStop describes one color stop in a parsed SVG gradient.
type SVGGradientStop struct {
	Offset  float64      // Stop offset from 0 to 1.
	Color   CSSColorType // Stop color.
	Opacity float64      // Stop opacity.
}

// SVGGradient describes a linear or radial SVG gradient that SVGWrite can
// render as a PDF shading fill.
type SVGGradient struct {
	Set   bool              // Whether a gradient is present.
	Kind  string            // Gradient kind: linear or radial.
	Units string            // SVG gradientUnits value.
	X1    float64           // Linear gradient start X coordinate.
	Y1    float64           // Linear gradient start Y coordinate.
	X2    float64           // Linear gradient end X coordinate.
	Y2    float64           // Linear gradient end Y coordinate.
	CX    float64           // Radial gradient center X coordinate.
	CY    float64           // Radial gradient center Y coordinate.
	FX    float64           // Radial gradient focal X coordinate.
	FY    float64           // Radial gradient focal Y coordinate.
	R     float64           // Radial gradient radius.
	Stops []SVGGradientStop // Gradient stops.
}

// SVGPattern describes an internal SVG pattern fill that SVGWrite can tile
// inside a path.
type SVGPattern struct {
	Set      bool         // Whether a pattern is present.
	Units    string       // SVG patternUnits value.
	X        float64      // Pattern origin X coordinate.
	Y        float64      // Pattern origin Y coordinate.
	Wd       float64      // Pattern width.
	Ht       float64      // Pattern height.
	Elements []SVGElement // Pattern elements.
}

// SVGPath describes one styled SVG path.
type SVGPath struct {
	Segments []SVGSegment // Path segments.
	Style    SVGStyle     // Path style.
}

// SVGText describes one SVG text element.
type SVGText struct {
	X     float64  // Text X coordinate.
	Y     float64  // Text Y coordinate.
	Text  string   // Text content.
	Role  string   // Optional tagged PDF role for this text node.
	Style SVGStyle // Text style.
}

// SVGElement describes one drawable SVG element in document order.
type SVGElement struct {
	Kind  string   // Element kind: path, text, or image.
	Path  SVGPath  // Path data when Kind is path.
	Text  SVGText  // Text data when Kind is text.
	Image SVGImage // Image data when Kind is image.
}

// SVG aggregates the information needed to describe a multi-segment vector
// image.
type SVG struct {
	Wd       float64        // SVG width.
	Ht       float64        // SVG height.
	Segments [][]SVGSegment // Legacy path segment groups.
	Paths    []SVGPath      // Parsed paths.
	Texts    []SVGText      // Parsed text elements.
	Images   []SVGImage     // Embedded raster images.
	Elements []SVGElement   // Drawable elements in document order.
}
