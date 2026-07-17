// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"strconv"
	"strings"
)

func svgPaintRef(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(value), "url(") {
		return ""
	}
	close := strings.IndexByte(value, ')')
	if close < 0 {
		return ""
	}
	ref := strings.TrimSpace(value[4:close])
	ref = strings.Trim(ref, `"'`)
	if strings.HasPrefix(ref, "#") {
		return strings.TrimPrefix(ref, "#")
	}
	return ""
}

func svgGradientUnits(value string, fallback string) string {
	switch strings.TrimSpace(value) {
	case "userSpaceOnUse":
		return "userSpaceOnUse"
	case "objectBoundingBox":
		return "objectBoundingBox"
	default:
		if fallback != "" {
			return fallback
		}
		return "objectBoundingBox"
	}
}

func svgGradientOffset(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if n, ok := svgPercentage(value); ok {
		return svgClamp01(n), true
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !svgFinite(n) {
		return 0, false
	}
	return svgClamp01(n), true
}

func svgGradientKind(name string) string {
	if name == "radialGradient" {
		return "radial"
	}
	return "linear"
}

func svgPatternUnits(value string, fallback string) string {
	switch strings.TrimSpace(value) {
	case "userSpaceOnUse":
		return "userSpaceOnUse"
	case "objectBoundingBox":
		return "objectBoundingBox"
	default:
		if fallback != "" {
			return fallback
		}
		return "objectBoundingBox"
	}
}

func svgNodeStyle(node svgNode, inherited SVGStyle, rules []htmlCSSRule, ancestors []HTMLSegmentType) SVGStyle {
	style := inherited
	el := svgHTMLSegment(node)
	attrs := el.Attr
	declarations := make(map[string]string, len(attrs))
	for name, value := range attrs {
		if name != "style" {
			declarations[name] = value
		}
	}
	for name, value := range htmlElementDeclarationsWithStyle(el, rules, nil, ancestors...) {
		declarations[name] = value
	}
	for name, value := range parseStyleDeclarations(attrs["style"]) {
		declarations[name] = value
	}
	if color, ok := parseCSSColor(declarations["color"]); ok {
		style.Color = color
	}
	if stroke, ok := parseSVGPaint(declarations["stroke"], style); ok {
		style.Stroke = stroke
	}
	if fillValue, ok := declarations["fill"]; ok {
		style.FillRef = ""
		style.FillGradient = SVGGradient{}
		style.FillPattern = SVGPattern{}
		if ref := svgPaintRef(fillValue); ref != "" {
			style.FillRef = ref
			style.Fill = CSSColorType{}
		}
		if fill, ok := parseSVGPaint(fillValue, style); ok {
			style.Fill = fill
		}
	}
	if ref := svgPaintRef(declarations["clip-path"]); ref != "" {
		style.ClipRef = ref
	}
	if clipRule := parseSVGFillRule(declarations["clip-rule"]); clipRule != "" {
		style.ClipRule = clipRule
	}
	if fillRule := parseSVGFillRule(declarations["fill-rule"]); fillRule != "" {
		style.FillRule = fillRule
	}
	if hidden, ok := parseSVGVisibility(declarations["display"], declarations["visibility"]); ok {
		style.Hidden = hidden
	}
	if n, ok := parseSVGStyleNumber(declarations["stroke-width"]); ok {
		style.StrokeWidth = n
	}
	if n, ok := parseSVGStyleNumber(declarations["font-size"]); ok {
		style.FontSize = n
	}
	if anchor := strings.TrimSpace(strings.ToLower(declarations["text-anchor"])); anchor != "" {
		style.TextAnchor = anchor
	}
	if capStyle := parseSVGLineCap(declarations["stroke-linecap"]); capStyle != "" {
		style.StrokeLineCap = capStyle
	}
	if joinStyle := parseSVGLineJoin(declarations["stroke-linejoin"]); joinStyle != "" {
		style.StrokeLineJoin = joinStyle
	}
	if opacity, ok := parseSVGOpacity(declarations["opacity"]); ok {
		style.Opacity = opacity
		style.OpacitySet = true
	}
	if opacity, ok := parseSVGOpacity(declarations["fill-opacity"]); ok {
		style.FillOpacity = opacity
		style.FillOpacitySet = true
	}
	if opacity, ok := parseSVGOpacity(declarations["stroke-opacity"]); ok {
		style.StrokeOpacity = opacity
		style.StrokeOpacitySet = true
	}
	if dash, ok := parseSVGDashArray(declarations["stroke-dasharray"]); ok {
		style.StrokeDashArray = dash
		style.StrokeDashSet = true
	}
	if offset, ok := svgLength(declarations["stroke-dashoffset"]); ok {
		style.StrokeDashOffset = offset
	}
	return style
}

func parseSVGPaint(value string, style SVGStyle) (CSSColorType, bool) {
	if strings.EqualFold(strings.TrimSpace(value), "currentColor") {
		if style.Color.Set && !style.Color.None {
			return style.Color, true
		}
		return CSSColorType{Set: true}, true
	}
	return parseCSSPaint(value)
}

func parseSVGVisibility(display, visibility string) (bool, bool) {
	display = svgLowerTrim(display)
	visibility = svgLowerTrim(visibility)
	if display == "none" || visibility == "hidden" || visibility == "collapse" {
		return true, true
	}
	return false, false
}

func parseSVGFillRule(value string) string {
	switch svgLowerTrim(value) {
	case "evenodd":
		return "evenodd"
	case "nonzero":
		return "nonzero"
	default:
		return ""
	}
}

func parseSVGLineCap(value string) string {
	value = svgLowerTrim(value)
	switch value {
	case "butt", "round", "square":
		return value
	default:
		return ""
	}
}

func parseSVGLineJoin(value string) string {
	value = svgLowerTrim(value)
	switch value {
	case "miter", "round", "bevel":
		return value
	default:
		return ""
	}
}

func parseSVGOpacity(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var n float64
	var err error
	if strings.HasSuffix(value, "%") {
		n, err = strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
		n /= 100
	} else {
		n, err = strconv.ParseFloat(value, 64)
	}
	if err != nil || !svgFinite(n) {
		return 0, false
	}
	if n < 0 {
		n = 0
	} else if n > 1 {
		n = 1
	}
	return n, true
}

func parseSVGDashArray(value string) ([]float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if strings.EqualFold(value, "none") {
		return nil, true
	}
	fields := svgNumberFields(value)
	if len(fields) == 0 {
		return nil, false
	}
	dash := make([]float64, 0, len(fields))
	hasPositive := false
	for _, field := range fields {
		n, ok := svgLength(field)
		if !ok || n < 0 {
			return nil, false
		}
		if n > 0 {
			hasPositive = true
		}
		dash = append(dash, n)
	}
	if !hasPositive {
		return nil, true
	}
	return dash, true
}

func svgLowerTrim(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseSVGStyleNumber(value string) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, false
	}
	n, ok := svgLength(value)
	return n, ok && n > 0
}
