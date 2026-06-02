// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const svgMaxNestingDepth = 512

func svgFinite(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0)
}

type svgMatrix struct{ A, B, C, D, E, F float64 }

func svgIdentityMatrix() svgMatrix {
	return svgMatrix{A: 1, D: 1}
}

func (m svgMatrix) isIdentity() bool {
	return m.A == 1 && m.B == 0 && m.C == 0 && m.D == 1 && m.E == 0 && m.F == 0
}

func (m svgMatrix) apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}

func (m svgMatrix) multiply(n svgMatrix) svgMatrix {
	return svgMatrix{A: m.A*n.A + m.C*n.B, B: m.B*n.A + m.D*n.B, C: m.A*n.C + m.C*n.D, D: m.B*n.C + m.D*n.D, E: m.A*n.E + m.C*n.F + m.E, F: m.B*n.E + m.D*n.F + m.F}
}

func (m svgMatrix) scaleFactor() float64 {
	xScale := math.Hypot(m.A, m.B)
	yScale := math.Hypot(m.C, m.D)
	if xScale == 0 {
		return yScale
	}
	if yScale == 0 {
		return xScale
	}
	return (xScale + yScale) / 2
}

func (m svgMatrix) finite() bool {
	return svgFinite(m.A) && svgFinite(m.B) && svgFinite(m.C) && svgFinite(m.D) && svgFinite(m.E) && svgFinite(m.F)
}

func svgLength(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(value, "%") {
		return 0, false
	}
	lower := strings.ToLower(value)
	multiplier := 1.0
	switch {
	case strings.HasSuffix(lower, "px"):
		value = strings.TrimSpace(value[:len(value)-2])
	case strings.HasSuffix(lower, "pt"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = 96.0 / 72.0
	case strings.HasSuffix(lower, "pc"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = 16
	case strings.HasSuffix(lower, "mm"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = 96.0 / 25.4
	case strings.HasSuffix(lower, "cm"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = 96.0 / 2.54
	case strings.HasSuffix(lower, "in"):
		value = strings.TrimSpace(value[:len(value)-2])
		multiplier = 96
	}
	n, err := strconv.ParseFloat(value, 64)
	return n * multiplier, err == nil && svgFinite(n)
}

func svgPercentage(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasSuffix(value, "%") {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
	return n / 100, err == nil && svgFinite(n)
}

func svgClamp01(n float64) float64 {
	if n < 0 {
		return 0
	}
	if n > 1 {
		return 1
	}
	return n
}

func svgGradientCoordinate(value string, fallback float64, units string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if n, ok := svgPercentage(value); ok {
		return n
	}
	if units == "userSpaceOnUse" {
		if n, ok := svgLength(value); ok {
			return n
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !svgFinite(n) {
		return fallback
	}
	return n
}

func svgPatternCoordinate(value string, fallback float64, units string) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if n, ok := svgPercentage(value); ok {
		return n
	}
	if units == "userSpaceOnUse" {
		if n, ok := svgLength(value); ok {
			return n
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !svgFinite(n) {
		return fallback
	}
	return n
}

func svgOptionalLength(node svgNode, name string, fallback float64) (float64, error) {
	value := strings.TrimSpace(node.attr(name))
	if value == "" {
		return fallback, nil
	}
	n, ok := svgLength(value)
	if !ok {
		return 0, fmt.Errorf("invalid SVG %s value: %s", name, value)
	}
	return n, nil
}

func svgRequiredPositiveLength(node svgNode, name string) (float64, error) {
	value := strings.TrimSpace(node.attr(name))
	n, ok := svgLength(value)
	if !ok || n <= 0 {
		return 0, fmt.Errorf("invalid SVG %s value: %s", name, value)
	}
	return n, nil
}

func svgViewBox(value string) (minX, minY, wd, ht float64, ok bool, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, 0, 0, false, nil
	}
	fields := svgNumberFields(value)
	if len(fields) != 4 {
		return 0, 0, 0, 0, false, fmt.Errorf("invalid SVG viewBox: %s", value)
	}
	nums := [4]float64{}
	for j, field := range fields {
		nums[j], err = svgParseNumber(field, "viewBox")
		if err != nil {
			return 0, 0, 0, 0, false, fmt.Errorf("invalid SVG viewBox: %s", value)
		}
	}
	return nums[0], nums[1], nums[2], nums[3], true, nil
}

func svgExtent(root svgNode) (float64, float64, error) {
	wd, wdOK := svgLength(root.attr("width"))
	ht, htOK := svgLength(root.attr("height"))
	if wdOK && htOK && wd > 0 && ht > 0 {
		return wd, ht, nil
	}
	_, _, viewWd, viewHt, ok, err := svgViewBox(root.attr("viewBox"))
	if err != nil {
		return 0, 0, err
	}
	if ok {
		if viewWd > 0 && viewHt > 0 {
			return viewWd, viewHt, nil
		}
	}
	return 0, 0, fmt.Errorf("unacceptable values for SVG extent: %.2f x %.2f", wd, ht)
}

func svgRootTransform(root svgNode, wd, ht float64) (svgMatrix, error) {
	return svgViewBoxTransform(root.attr("viewBox"), root.attr("preserveAspectRatio"), wd, ht)
}

func svgViewBoxTransform(viewBoxValue, preserveAspectRatioValue string, wd, ht float64) (svgMatrix, error) {
	minX, minY, viewWd, viewHt, ok, err := svgViewBox(viewBoxValue)
	if err != nil {
		return svgIdentityMatrix(), err
	}
	if !ok || viewWd <= 0 || viewHt <= 0 {
		return svgIdentityMatrix(), nil
	}
	scaleX := wd / viewWd
	scaleY := ht / viewHt
	alignX, alignY, mode := svgPreserveAspectRatio(preserveAspectRatioValue)
	if mode == "none" {
		return svgMatrix{A: scaleX, D: scaleY, E: -minX * scaleX, F: -minY * scaleY}, nil
	}
	scale := scaleX
	if mode == "slice" {
		if scaleY > scale {
			scale = scaleY
		}
	} else if scaleY < scale {
		scale = scaleY
	}
	renderedWd := viewWd * scale
	renderedHt := viewHt * scale
	offsetX := svgPreserveAspectOffset(alignX, wd-renderedWd)
	offsetY := svgPreserveAspectOffset(alignY, ht-renderedHt)
	return svgMatrix{A: scale, D: scale, E: offsetX - minX*scale, F: offsetY - minY*scale}, nil
}

func svgPreserveAspectRatio(value string) (alignX, alignY, mode string) {
	alignX, alignY, mode = "mid", "mid", "meet"
	value = strings.TrimSpace(value)
	if value == "" {
		return alignX, alignY, mode
	}
	fields := strings.Fields(value)
	if len(fields) > 0 && strings.EqualFold(fields[0], "defer") {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return alignX, alignY, mode
	}
	if strings.EqualFold(fields[0], "none") {
		return "min", "min", "none"
	}
	if len(fields[0]) == len("xMidYMid") {
		xPart := strings.ToLower(fields[0][1:4])
		yPart := strings.ToLower(fields[0][5:8])
		if xPart == "min" || xPart == "mid" || xPart == "max" {
			alignX = xPart
		}
		if yPart == "min" || yPart == "mid" || yPart == "max" {
			alignY = yPart
		}
	}
	if len(fields) > 1 {
		switch strings.ToLower(fields[1]) {
		case "slice":
			mode = "slice"
		case "meet":
			mode = "meet"
		}
	}
	return alignX, alignY, mode
}

func svgPreserveAspectOffset(align string, extra float64) float64 {
	switch align {
	case "mid":
		return extra / 2
	case "max":
		return extra
	default:
		return 0
	}
}

func svgTransform(value string) (svgMatrix, error) {
	matrix := svgIdentityMatrix()
	value = strings.TrimSpace(value)
	for value != "" {
		open := strings.IndexByte(value, '(')
		if open < 0 {
			return matrix, fmt.Errorf("invalid SVG transform: %s", value)
		}
		name := strings.ToLower(strings.TrimSpace(value[:open]))
		close := strings.IndexByte(value[open+1:], ')')
		if close < 0 {
			return matrix, fmt.Errorf("invalid SVG transform: %s", value)
		}
		close += open + 1
		nums, err := svgTransformNumbers(value[open+1 : close])
		if err != nil {
			return matrix, err
		}
		next, err := svgTransformMatrix(name, nums)
		if err != nil {
			return matrix, err
		}
		matrix = next.multiply(matrix)
		if !matrix.finite() {
			return svgIdentityMatrix(), fmt.Errorf("invalid SVG transform: non-finite result")
		}
		value = strings.TrimSpace(value[close+1:])
	}
	return matrix, nil
}

func svgTransformNumbers(value string) ([]float64, error) {
	fields := svgNumberFields(value)
	nums := make([]float64, 0, len(fields))
	for _, field := range fields {
		n, err := svgParseNumber(field, "transform")
		if err != nil {
			return nil, fmt.Errorf("invalid SVG transform value: %s", field)
		}
		nums = append(nums, n)
	}
	return nums, nil
}

func svgTransformMatrix(name string, nums []float64) (svgMatrix, error) {
	switch name {
	case "matrix":
		if len(nums) != 6 {
			return svgIdentityMatrix(), fmt.Errorf("SVG matrix transform expects 6 arguments")
		}
		return svgMatrix{A: nums[0], B: nums[1], C: nums[2], D: nums[3], E: nums[4], F: nums[5]}, nil
	case "translate":
		if len(nums) != 1 && len(nums) != 2 {
			return svgIdentityMatrix(), fmt.Errorf("SVG translate transform expects 1 or 2 arguments")
		}
		ty := 0.0
		if len(nums) == 2 {
			ty = nums[1]
		}
		return svgMatrix{A: 1, D: 1, E: nums[0], F: ty}, nil
	case "scale":
		if len(nums) != 1 && len(nums) != 2 {
			return svgIdentityMatrix(), fmt.Errorf("SVG scale transform expects 1 or 2 arguments")
		}
		sy := nums[0]
		if len(nums) == 2 {
			sy = nums[1]
		}
		return svgMatrix{A: nums[0], D: sy}, nil
	case "rotate":
		if len(nums) != 1 && len(nums) != 3 {
			return svgIdentityMatrix(), fmt.Errorf("SVG rotate transform expects 1 or 3 arguments")
		}
		radians := nums[0] * math.Pi / 180
		cosine, sine := math.Cos(radians), math.Sin(radians)
		rotation := svgMatrix{A: cosine, B: sine, C: -sine, D: cosine}
		if len(nums) == 1 {
			return rotation, nil
		}
		toOrigin := svgMatrix{A: 1, D: 1, E: -nums[1], F: -nums[2]}
		fromOrigin := svgMatrix{A: 1, D: 1, E: nums[1], F: nums[2]}
		return fromOrigin.multiply(rotation).multiply(toOrigin), nil
	case "skewx":
		if len(nums) != 1 {
			return svgIdentityMatrix(), fmt.Errorf("SVG skewX transform expects 1 argument")
		}
		return svgMatrix{A: 1, C: math.Tan(nums[0] * math.Pi / 180), D: 1}, nil
	case "skewy":
		if len(nums) != 1 {
			return svgIdentityMatrix(), fmt.Errorf("SVG skewY transform expects 1 argument")
		}
		return svgMatrix{A: 1, B: math.Tan(nums[0] * math.Pi / 180), D: 1}, nil
	default:
		return svgIdentityMatrix(), fmt.Errorf("unsupported SVG transform: %s", name)
	}
}

func svgSegment(cmd byte, args ...float64) SVGSegment {
	seg := SVGSegment{Cmd: cmd}
	for j := 0; j < len(args) && j < len(seg.Arg); j++ {
		seg.Arg[j] = args[j]
	}
	return seg
}

func svgLineSegments(x1, y1, x2, y2 float64) []SVGSegment {
	return []SVGSegment{svgSegment('M', x1, y1), svgSegment('L', x2, y2)}
}

func svgRectSegments(x, y, wd, ht, rx, ry float64) []SVGSegment {
	if wd <= 0 || ht <= 0 {
		return nil
	}
	if rx == 0 && ry > 0 {
		rx = ry
	} else if ry == 0 && rx > 0 {
		ry = rx
	}
	if rx > wd/2 {
		rx = wd / 2
	}
	if ry > ht/2 {
		ry = ht / 2
	}
	if rx <= 0 || ry <= 0 {
		return []SVGSegment{svgSegment('M', x, y), svgSegment('L', x+wd, y), svgSegment('L', x+wd, y+ht), svgSegment('L', x, y+ht), svgSegment('Z')}
	}
	const kappa = 0.5522847498307936
	kx, ky := rx*kappa, ry*kappa
	return []SVGSegment{svgSegment('M', x+rx, y), svgSegment('L', x+wd-rx, y), svgSegment('C', x+wd-rx+kx, y, x+wd, y+ry-ky, x+wd, y+ry), svgSegment('L', x+wd, y+ht-ry), svgSegment('C', x+wd, y+ht-ry+ky, x+wd-rx+kx, y+ht, x+wd-rx, y+ht), svgSegment('L', x+rx, y+ht), svgSegment('C', x+rx-kx, y+ht, x, y+ht-ry+ky, x, y+ht-ry), svgSegment('L', x, y+ry), svgSegment('C', x, y+ry-ky, x+rx-kx, y, x+rx, y), svgSegment('Z')}
}

func svgEllipseSegments(cx, cy, rx, ry float64) []SVGSegment {
	if rx <= 0 || ry <= 0 {
		return nil
	}
	const kappa = 0.5522847498307936
	kx, ky := rx*kappa, ry*kappa
	return []SVGSegment{svgSegment('M', cx+rx, cy), svgSegment('C', cx+rx, cy+ky, cx+kx, cy+ry, cx, cy+ry), svgSegment('C', cx-kx, cy+ry, cx-rx, cy+ky, cx-rx, cy), svgSegment('C', cx-rx, cy-ky, cx-kx, cy-ry, cx, cy-ry), svgSegment('C', cx+kx, cy-ry, cx+rx, cy-ky, cx+rx, cy), svgSegment('Z')}
}

func svgPointsSegments(points string, closed bool) ([]SVGSegment, error) {
	fields := svgNumberFields(points)
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("invalid SVG points value: %s", points)
	}
	segs := make([]SVGSegment, 0, len(fields)/2+1)
	for j := 0; j < len(fields); j += 2 {
		x, err := svgParseNumber(fields[j], "points")
		if err != nil {
			return nil, fmt.Errorf("invalid SVG points value: %s", points)
		}
		y, err := svgParseNumber(fields[j+1], "points")
		if err != nil {
			return nil, fmt.Errorf("invalid SVG points value: %s", points)
		}
		cmd := byte('L')
		if j == 0 {
			cmd = 'M'
		}
		segs = append(segs, svgSegment(cmd, x, y))
	}
	if closed {
		segs = append(segs, svgSegment('Z'))
	}
	return segs, nil
}

func svgElementPath(node svgNode, style SVGStyle, transform svgMatrix) (SVGPath, bool, error) {
	path := func(segs []SVGSegment, err error) (SVGPath, bool, error) {
		if err != nil {
			return SVGPath{}, false, err
		}
		if len(segs) == 0 {
			return SVGPath{}, false, nil
		}
		return SVGPath{Segments: svgTransformSegments(segs, transform), Style: svgRenderedStyle(style, transform)}, true, nil
	}
	switch node.XMLName.Local {
	case "path":
		return path(pathParse(node.attr("d")))
	case "line":
		x1, err := svgOptionalLength(node, "x1", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		y1, err := svgOptionalLength(node, "y1", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		x2, err := svgOptionalLength(node, "x2", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		y2, err := svgOptionalLength(node, "y2", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		return path(svgLineSegments(x1, y1, x2, y2), nil)
	case "rect":
		x, err := svgOptionalLength(node, "x", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		y, err := svgOptionalLength(node, "y", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		wd, err := svgRequiredPositiveLength(node, "width")
		if err != nil {
			return SVGPath{}, false, err
		}
		ht, err := svgRequiredPositiveLength(node, "height")
		if err != nil {
			return SVGPath{}, false, err
		}
		rx, err := svgOptionalLength(node, "rx", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		ry, err := svgOptionalLength(node, "ry", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		return path(svgRectSegments(x, y, wd, ht, rx, ry), nil)
	case "circle":
		cx, err := svgOptionalLength(node, "cx", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		cy, err := svgOptionalLength(node, "cy", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		r, err := svgRequiredPositiveLength(node, "r")
		if err != nil {
			return SVGPath{}, false, err
		}
		return path(svgEllipseSegments(cx, cy, r, r), nil)
	case "ellipse":
		cx, err := svgOptionalLength(node, "cx", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		cy, err := svgOptionalLength(node, "cy", 0)
		if err != nil {
			return SVGPath{}, false, err
		}
		rx, err := svgRequiredPositiveLength(node, "rx")
		if err != nil {
			return SVGPath{}, false, err
		}
		ry, err := svgRequiredPositiveLength(node, "ry")
		if err != nil {
			return SVGPath{}, false, err
		}
		return path(svgEllipseSegments(cx, cy, rx, ry), nil)
	case "polyline":
		return path(svgPointsSegments(node.attr("points"), false))
	case "polygon":
		return path(svgPointsSegments(node.attr("points"), true))
	}
	return SVGPath{}, false, nil
}

func svgTransformSegments(segs []SVGSegment, transform svgMatrix) []SVGSegment {
	if transform.isIdentity() {
		return segs
	}
	out := make([]SVGSegment, len(segs))
	var x, y, startX, startY float64
	for j, seg := range segs {
		out[j] = seg
		switch seg.Cmd {
		case 'M':
			x, y = seg.Arg[0], seg.Arg[1]
			startX, startY = x, y
			out[j].Arg[0], out[j].Arg[1] = transform.apply(x, y)
		case 'L':
			x, y = seg.Arg[0], seg.Arg[1]
			out[j].Arg[0], out[j].Arg[1] = transform.apply(x, y)
		case 'H':
			x = seg.Arg[0]
			out[j].Cmd = 'L'
			out[j].Arg[0], out[j].Arg[1] = transform.apply(x, y)
		case 'V':
			y = seg.Arg[0]
			out[j].Cmd = 'L'
			out[j].Arg[0], out[j].Arg[1] = transform.apply(x, y)
		case 'C':
			out[j].Arg[0], out[j].Arg[1] = transform.apply(seg.Arg[0], seg.Arg[1])
			out[j].Arg[2], out[j].Arg[3] = transform.apply(seg.Arg[2], seg.Arg[3])
			out[j].Arg[4], out[j].Arg[5] = transform.apply(seg.Arg[4], seg.Arg[5])
			x, y = seg.Arg[4], seg.Arg[5]
		case 'Q':
			out[j].Arg[0], out[j].Arg[1] = transform.apply(seg.Arg[0], seg.Arg[1])
			out[j].Arg[2], out[j].Arg[3] = transform.apply(seg.Arg[2], seg.Arg[3])
			x, y = seg.Arg[2], seg.Arg[3]
		case 'Z':
			x, y = startX, startY
		}
	}
	return out
}

func svgSegmentFinite(seg SVGSegment) bool {
	for _, n := range seg.Arg {
		if !svgFinite(n) {
			return false
		}
	}
	return true
}

func svgGradientFinite(gradient SVGGradient) bool {
	return svgFinite(gradient.X1) && svgFinite(gradient.Y1) && svgFinite(gradient.X2) && svgFinite(gradient.Y2) && svgFinite(gradient.CX) && svgFinite(gradient.CY) && svgFinite(gradient.FX) && svgFinite(gradient.FY) && svgFinite(gradient.R)
}

func svgPathFinite(path SVGPath) bool {
	if !svgFinite(path.Style.StrokeWidth) || !svgFinite(path.Style.FontSize) || !svgFinite(path.Style.StrokeDashOffset) || !svgFinite(path.Style.Opacity) || !svgFinite(path.Style.FillOpacity) || !svgFinite(path.Style.StrokeOpacity) {
		return false
	}
	if path.Style.FillGradient.Set && !svgGradientFinite(path.Style.FillGradient) {
		return false
	}
	for _, dash := range path.Style.StrokeDashArray {
		if !svgFinite(dash) {
			return false
		}
	}
	for _, seg := range path.Style.ClipPath {
		if !svgSegmentFinite(seg) {
			return false
		}
	}
	for _, seg := range path.Segments {
		if !svgSegmentFinite(seg) {
			return false
		}
	}
	return true
}

func svgTextFinite(text SVGText) bool {
	return svgFinite(text.X) && svgFinite(text.Y) && svgFinite(text.Style.StrokeWidth) && svgFinite(text.Style.FontSize) && svgFinite(text.Style.Opacity) && svgFinite(text.Style.FillOpacity)
}

func svgImageFinite(image SVGImage) bool {
	return svgFinite(image.X) && svgFinite(image.Y) && svgFinite(image.Wd) && svgFinite(image.Ht) && image.Wd > 0 && image.Ht > 0
}

func svgRenderedStyle(style SVGStyle, transform svgMatrix) SVGStyle {
	scale := transform.scaleFactor()
	if style.StrokeWidth > 0 {
		style.StrokeWidth *= scale
	}
	if style.FontSize > 0 {
		style.FontSize *= scale
	}
	if len(style.StrokeDashArray) > 0 {
		style.StrokeDashArray = append([]float64(nil), style.StrokeDashArray...)
		for j := range style.StrokeDashArray {
			style.StrokeDashArray[j] *= scale
		}
	}
	style.StrokeDashOffset *= scale
	return style
}

func svgTextContent(node svgNode) string {
	parts := []string{}
	if text := strings.TrimSpace(node.Text); text != "" {
		parts = append(parts, text)
	}
	for _, child := range node.Children {
		if text := svgTextContent(child); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(strings.Fields(strings.Join(parts, " ")), " ")
}

func svgText(node svgNode, style SVGStyle, transform svgMatrix) (SVGText, bool, error) {
	if node.XMLName.Local != "text" {
		return SVGText{}, false, nil
	}
	if style.Hidden {
		return SVGText{}, false, nil
	}
	x, err := svgOptionalLength(node, "x", 0)
	if err != nil {
		return SVGText{}, false, err
	}
	y, err := svgOptionalLength(node, "y", 0)
	if err != nil {
		return SVGText{}, false, err
	}
	text := svgTextContent(node)
	x, y = transform.apply(x, y)
	style = svgRenderedStyle(style, transform)
	return SVGText{X: x, Y: y, Text: text, Style: style}, text != "", nil
}
