// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math"
)

const svgMaxPatternTiles = 4096

// SVGWrite renders the SVG image described by sb. The scale value converts SVG
// coordinates to the unit of measure specified in New(). The current position,
// as set with SetXY(), is used as the image origin.
func (f *Fpdf) SVGWrite(sb *SVG, scale float64) {
	originX, originY := f.GetXY()
	drawR, drawG, drawB := f.GetDrawColor()
	fillR, fillG, fillB := f.GetFillColor()
	textR, textG, textB := f.GetTextColor()
	_, fontUnitSize := f.GetFontSize()
	lineWidth := f.GetLineWidth()
	capStyle := f.capStyle
	joinStyle := f.joinStyle
	dashArray := append([]float64(nil), f.dashArray...)
	dashPhase := f.dashPhase
	alpha, blendMode := f.GetAlpha()
	defer func() {
		f.SetDrawColor(drawR, drawG, drawB)
		f.SetFillColor(fillR, fillG, fillB)
		f.SetTextColor(textR, textG, textB)
		f.SetLineWidth(lineWidth)
		f.SetLineCapStyle(svgLineCapName(capStyle))
		f.SetLineJoinStyle(svgLineJoinName(joinStyle))
		f.dashArray = dashArray
		f.dashPhase = dashPhase
		if f.page > 0 {
			f.outputDashPattern()
		}
		f.SetAlpha(alpha, blendMode)
		if fontUnitSize > 0 {
			f.SetFontUnitSize(fontUnitSize)
		}
	}()

	if len(sb.Elements) > 0 {
		for _, element := range sb.Elements {
			f.svgWriteElement(originX, originY, scale, element)
			f.SetAlpha(alpha, blendMode)
		}
		return
	}

	paths := sb.Paths
	if len(paths) == 0 {
		paths = make([]SVGPath, 0, len(sb.Segments))
		for _, segs := range sb.Segments {
			paths = append(paths, SVGPath{Segments: segs})
		}
	}
	for _, image := range sb.Images {
		f.svgWriteImage(originX, originY, scale, image)
		f.SetAlpha(alpha, blendMode)
	}
	for _, path := range paths {
		f.svgWritePath(originX, originY, scale, path)
		f.SetAlpha(alpha, blendMode)
	}
	for _, text := range sb.Texts {
		f.svgWriteText(originX, originY, scale, text)
		f.SetAlpha(alpha, blendMode)
	}
}

func (f *Fpdf) svgWriteElement(originX, originY, scale float64, element SVGElement) {
	switch element.Kind {
	case "path":
		f.svgWritePath(originX, originY, scale, element.Path)
	case "text":
		f.svgWriteText(originX, originY, scale, element.Text)
	case "image":
		f.svgWriteImage(originX, originY, scale, element.Image)
	}
}

func (f *Fpdf) svgWritePath(originX, originY, scale float64, path SVGPath) {
	if clipped := f.svgClipStart(originX, originY, scale, path.Style); clipped {
		defer f.svgClipEnd()
	}
	if path.Style.FillPattern.Set && svgPathHasFill(path.Style, path.Segments) {
		f.svgWritePatternFill(originX, originY, scale, path)
		if !svgPathHasStroke(path.Style, path.Segments) {
			return
		}
		path.Style.Fill = CSSColorType{Set: true, None: true}
		path.Style.FillPattern = SVGPattern{}
	}
	if path.Style.FillGradient.Set && svgPathHasFill(path.Style, path.Segments) {
		f.svgWriteGradientFill(originX, originY, scale, path)
		if !svgPathHasStroke(path.Style, path.Segments) {
			return
		}
		path.Style.Fill = CSSColorType{Set: true, None: true}
		path.Style.FillGradient = SVGGradient{}
	}
	if style := f.svgApplyPathStyle(path, scale); style != "" {
		f.svgEmitPath(originX, originY, scale, path.Segments)
		f.DrawPath(style)
	}
}

func (f *Fpdf) svgWriteImage(originX, originY, scale float64, image SVGImage) {
	if len(image.Data) == 0 || image.ImageType == "" || image.Wd <= 0 || image.Ht <= 0 || image.Style.Hidden {
		return
	}
	if clipped := f.svgClipStart(originX, originY, scale, image.Style); clipped {
		defer f.svgClipEnd()
	}
	if opacity := svgStyleOpacity(image.Style, false, true); opacity < 1 {
		f.SetAlpha(opacity, "Normal")
	}
	sum := sha256.Sum256(image.Data)
	name := fmt.Sprintf("svg-image-%s-%x", image.ImageType, sum)
	options := ImageOptions{ImageType: image.ImageType, ReadDpi: true}
	f.RegisterImageOptionsReader(name, options, bytes.NewReader(image.Data))
	if !f.Ok() {
		return
	}
	f.ImageOptions(name,
		originX+image.X*scale,
		originY+image.Y*scale,
		image.Wd*scale,
		image.Ht*scale,
		false,
		options,
		0,
		"")
}

func (f *Fpdf) svgEmitPath(originX, originY, scale float64, segments []SVGSegment) {
	var x, y, newX, newY float64
	var cx0, cy0, cx1, cy1 float64
	var seg SVGSegment
	var startX, startY float64
	sval := func(origin float64, arg int) float64 {
		return origin + scale*seg.Arg[arg]
	}
	xval := func(arg int) float64 {
		return sval(originX, arg)
	}
	yval := func(arg int) float64 {
		return sval(originY, arg)
	}
	val := func(arg int) (float64, float64) {
		return xval(arg), yval(arg + 1)
	}
	for k := 0; k < len(segments) && f.Ok(); k++ {
		seg = segments[k]
		switch seg.Cmd {
		case 'M':
			x, y = val(0)
			startX, startY = x, y
			f.MoveTo(x, y)
		case 'L':
			newX, newY = val(0)
			f.LineTo(newX, newY)
			x, y = newX, newY
		case 'C':
			cx0, cy0 = val(0)
			cx1, cy1 = val(2)
			newX, newY = val(4)
			f.CurveBezierCubicTo(cx0, cy0, cx1, cy1, newX, newY)
			x, y = newX, newY
		case 'Q':
			cx0, cy0 = val(0)
			newX, newY = val(2)
			f.CurveTo(cx0, cy0, newX, newY)
			x, y = newX, newY
		case 'H':
			newX = xval(0)
			f.LineTo(newX, y)
			x = newX
		case 'V':
			newY = yval(0)
			f.LineTo(x, newY)
			y = newY
		case 'Z':
			f.ClosePath()
			x, y = startX, startY
		default:
			f.SetErrorf("Unexpected path command '%c'", seg.Cmd)
		}
	}
}

func svgLineCapName(style int) string {
	switch style {
	case 1:
		return "round"
	case 2:
		return "square"
	default:
		return "butt"
	}
}

func svgLineJoinName(style int) string {
	switch style {
	case 1:
		return "round"
	case 2:
		return "bevel"
	default:
		return "miter"
	}
}

func svgPathOpen(segs []SVGSegment) bool {
	for j := len(segs) - 1; j >= 0; j-- {
		if segs[j].Cmd == 'Z' {
			return false
		}
		if segs[j].Cmd == 'M' || segs[j].Cmd == 'L' || segs[j].Cmd == 'H' ||
			segs[j].Cmd == 'V' || segs[j].Cmd == 'C' || segs[j].Cmd == 'Q' {
			return true
		}
	}
	return true
}

func svgPathHasStroke(style SVGStyle, segs []SVGSegment) bool {
	explicitPaint := style.Stroke.Set || style.Fill.Set || style.FillGradient.Set || style.FillPattern.Set
	if explicitPaint {
		return style.Stroke.Set && !style.Stroke.None
	}
	return svgPathOpen(segs)
}

func svgPathHasFill(style SVGStyle, segs []SVGSegment) bool {
	explicitPaint := style.Stroke.Set || style.Fill.Set || style.FillGradient.Set || style.FillPattern.Set
	if explicitPaint {
		return style.FillGradient.Set || style.FillPattern.Set || (style.Fill.Set && !style.Fill.None)
	}
	return !svgPathOpen(segs)
}

func (f *Fpdf) svgClipStart(originX, originY, scale float64, style SVGStyle) bool {
	if len(style.ClipPath) == 0 {
		return false
	}
	f.out("q")
	f.svgEmitPath(originX, originY, scale, style.ClipPath)
	if style.ClipRule == "evenodd" {
		f.out("W* n")
	} else {
		f.out("W n")
	}
	return true
}

func (f *Fpdf) svgClipEnd() {
	f.out("Q")
}

func svgStyleOpacity(style SVGStyle, stroke, fill bool) float64 {
	opacity := 1.0
	if style.OpacitySet {
		opacity *= style.Opacity
	}
	if stroke && !fill && style.StrokeOpacitySet {
		opacity *= style.StrokeOpacity
	}
	if fill && !stroke && style.FillOpacitySet {
		opacity *= style.FillOpacity
	}
	return opacity
}

func svgScaledValues(values []float64, scale float64) []float64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]float64, len(values))
	for j, value := range values {
		out[j] = value * scale
	}
	return out
}

func svgPathBounds(segs []SVGSegment) (minX, minY, maxX, maxY float64, ok bool) {
	include := func(x, y float64) {
		if !ok {
			minX, minY, maxX, maxY = x, y, x, y
			ok = true
			return
		}
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}
	var x, y, startX, startY float64
	for _, seg := range segs {
		switch seg.Cmd {
		case 'M':
			x, y = seg.Arg[0], seg.Arg[1]
			startX, startY = x, y
			include(x, y)
		case 'L':
			x, y = seg.Arg[0], seg.Arg[1]
			include(x, y)
		case 'H':
			x = seg.Arg[0]
			include(x, y)
		case 'V':
			y = seg.Arg[0]
			include(x, y)
		case 'C':
			include(seg.Arg[0], seg.Arg[1])
			include(seg.Arg[2], seg.Arg[3])
			x, y = seg.Arg[4], seg.Arg[5]
			include(x, y)
		case 'Q':
			include(seg.Arg[0], seg.Arg[1])
			x, y = seg.Arg[2], seg.Arg[3]
			include(x, y)
		case 'Z':
			x, y = startX, startY
			include(x, y)
		}
	}
	return minX, minY, maxX, maxY, ok
}

func svgGradientPDFStops(gradient SVGGradient) ([]gradientStopType, bool) {
	if len(gradient.Stops) < 2 {
		return nil, false
	}
	stops := make([]gradientStopType, 0, len(gradient.Stops))
	for _, stop := range gradient.Stops {
		if !stop.Color.Set || stop.Color.None {
			continue
		}
		clr := rgbColorValue(stop.Color.R, stop.Color.G, stop.Color.B, "", "")
		stops = append(stops, gradientStopType{offset: stop.Offset, clrStr: clr.str})
	}
	return stops, len(stops) >= 2
}

func svgGradientUnit(value, min, max float64, units string) float64 {
	if units == "userSpaceOnUse" {
		size := max - min
		if size == 0 {
			return 0
		}
		return (value - min) / size
	}
	return value
}

func svgGradientY(value, min, max float64, units string) float64 {
	return 1 - svgGradientUnit(value, min, max, units)
}

func svgPatternTile(pattern SVGPattern, minX, minY, maxX, maxY float64) (x, y, wd, ht float64) {
	if pattern.Units == "userSpaceOnUse" {
		return pattern.X, pattern.Y, pattern.Wd, pattern.Ht
	}
	bboxWd := maxX - minX
	bboxHt := maxY - minY
	return minX + pattern.X*bboxWd, minY + pattern.Y*bboxHt, pattern.Wd * bboxWd, pattern.Ht * bboxHt
}

func (f *Fpdf) svgWritePatternFill(originX, originY, scale float64, path SVGPath) {
	minX, minY, maxX, maxY, ok := svgPathBounds(path.Segments)
	if !ok || maxX <= minX || maxY <= minY {
		return
	}
	pattern := path.Style.FillPattern
	if len(pattern.Elements) == 0 {
		return
	}
	tileX, tileY, tileWd, tileHt := svgPatternTile(pattern, minX, minY, maxX, maxY)
	if tileWd <= 0 || tileHt <= 0 {
		return
	}
	alpha, blendMode := f.GetAlpha()
	defer f.SetAlpha(alpha, blendMode)
	if opacity := svgStyleOpacity(path.Style, false, true); opacity < 1 {
		f.SetAlpha(opacity, "Normal")
	}
	minCol := int(math.Floor((minX-tileX)/tileWd)) - 1
	maxCol := int(math.Ceil((maxX-tileX)/tileWd)) + 1
	minRow := int(math.Floor((minY-tileY)/tileHt)) - 1
	maxRow := int(math.Ceil((maxY-tileY)/tileHt)) + 1
	cols := maxCol - minCol + 1
	rows := maxRow - minRow + 1
	if cols <= 0 || rows <= 0 {
		return
	}
	if cols > svgMaxPatternTiles || rows > svgMaxPatternTiles || cols*rows > svgMaxPatternTiles {
		f.SetErrorf("SVG pattern tile count exceeds %d", svgMaxPatternTiles)
		return
	}

	f.out("q")
	f.svgEmitPath(originX, originY, scale, path.Segments)
	if path.Style.FillRule == "evenodd" {
		f.out("W* n")
	} else {
		f.out("W n")
	}
	tileClip := svgRectSegments(0, 0, tileWd, tileHt, 0, 0)
	for row := minRow; row <= maxRow && f.Ok(); row++ {
		for col := minCol; col <= maxCol && f.Ok(); col++ {
			x := originX + (tileX+float64(col)*tileWd)*scale
			y := originY + (tileY+float64(row)*tileHt)*scale
			f.out("q")
			f.svgEmitPath(x, y, scale, tileClip)
			f.out("W n")
			for _, element := range pattern.Elements {
				f.svgWriteElement(x, y, scale, element)
				if opacity := svgStyleOpacity(path.Style, false, true); opacity < 1 {
					f.SetAlpha(opacity, "Normal")
				} else {
					f.SetAlpha(alpha, blendMode)
				}
			}
			f.out("Q")
		}
	}
	f.out("Q")
}

func (f *Fpdf) svgWriteGradientFill(originX, originY, scale float64, path SVGPath) {
	minX, minY, maxX, maxY, ok := svgPathBounds(path.Segments)
	if !ok || maxX <= minX || maxY <= minY {
		return
	}
	stops, ok := svgGradientPDFStops(path.Style.FillGradient)
	if !ok {
		return
	}
	x := originX + minX*scale
	y := originY + minY*scale
	wd := (maxX - minX) * scale
	ht := (maxY - minY) * scale
	if wd <= 0 || ht <= 0 {
		return
	}
	f.out("q")
	f.svgEmitPath(originX, originY, scale, path.Segments)
	if path.Style.FillRule == "evenodd" {
		f.out("W* n")
	} else {
		f.out("W n")
	}
	f.outf("%.5f 0 0 %.5f %.5f %.5f cm", wd*f.k, ht*f.k, x*f.k, (f.h-(y+ht))*f.k)
	gradient := path.Style.FillGradient
	if gradient.Kind == "radial" {
		cx := svgGradientUnit(gradient.CX, minX, maxX, gradient.Units)
		cy := svgGradientY(gradient.CY, minY, maxY, gradient.Units)
		fx := svgGradientUnit(gradient.FX, minX, maxX, gradient.Units)
		fy := svgGradientY(gradient.FY, minY, maxY, gradient.Units)
		r := gradient.R
		if gradient.Units == "userSpaceOnUse" {
			if size := maxX - minX; size > 0 {
				r = r / size
			}
		}
		f.gradientWithStops(3, stops, fx, fy, cx, cy, r)
	} else {
		x1 := svgGradientUnit(gradient.X1, minX, maxX, gradient.Units)
		y1 := svgGradientY(gradient.Y1, minY, maxY, gradient.Units)
		x2 := svgGradientUnit(gradient.X2, minX, maxX, gradient.Units)
		y2 := svgGradientY(gradient.Y2, minY, maxY, gradient.Units)
		f.gradientWithStops(2, stops, x1, y1, x2, y2, 0)
	}
	f.out("Q")
}

func (f *Fpdf) svgApplyPathStyle(path SVGPath, scale float64) string {
	style := path.Style
	if style.Hidden {
		return ""
	}
	explicitPaint := style.Stroke.Set || style.Fill.Set || style.FillGradient.Set || style.FillPattern.Set
	stroke := svgPathHasStroke(style, path.Segments)
	fill := svgPathHasFill(style, path.Segments)
	if !stroke && !fill {
		return ""
	}
	if stroke && style.Stroke.Set {
		f.SetDrawColor(style.Stroke.R, style.Stroke.G, style.Stroke.B)
	}
	if fill {
		if style.Fill.Set {
			f.SetFillColor(style.Fill.R, style.Fill.G, style.Fill.B)
		} else {
			f.SetFillColor(0, 0, 0)
		}
	}
	if style.StrokeWidth > 0 {
		f.SetLineWidth(style.StrokeWidth * scale)
	} else if stroke && explicitPaint {
		f.SetLineWidth(scale)
	}
	if stroke {
		lineCap := style.StrokeLineCap
		if lineCap == "" && explicitPaint {
			lineCap = "butt"
		}
		if lineCap != "" {
			f.SetLineCapStyle(lineCap)
		}
		lineJoin := style.StrokeLineJoin
		if lineJoin == "" && explicitPaint {
			lineJoin = "miter"
		}
		if lineJoin != "" {
			f.SetLineJoinStyle(lineJoin)
		}
		if style.StrokeDashSet {
			f.SetDashPattern(svgScaledValues(style.StrokeDashArray, scale), style.StrokeDashOffset*scale)
		} else if explicitPaint {
			f.SetDashPattern(nil, 0)
		}
	}
	if opacity := svgStyleOpacity(style, stroke, fill); opacity < 1 {
		f.SetAlpha(opacity, "Normal")
	}
	switch {
	case stroke && fill:
		if style.FillRule == "evenodd" {
			return "DF*"
		}
		return "DF"
	case fill:
		if style.FillRule == "evenodd" {
			return "F*"
		}
		return "F"
	default:
		return "D"
	}
}

func (f *Fpdf) svgWriteText(originX, originY, scale float64, text SVGText) {
	if text.Text == "" || text.Style.Hidden || text.Style.Fill.None {
		return
	}
	if clipped := f.svgClipStart(originX, originY, scale, text.Style); clipped {
		defer f.svgClipEnd()
	}
	if f.currentFont.Name == "" {
		f.SetFont("Helvetica", "", 12)
	}
	x := originX + text.X*scale
	y := originY + text.Y*scale
	if text.Style.Fill.Set && !text.Style.Fill.None {
		f.SetTextColor(text.Style.Fill.R, text.Style.Fill.G, text.Style.Fill.B)
	} else if !text.Style.Fill.Set {
		f.SetTextColor(0, 0, 0)
	}
	if text.Style.FontSize > 0 {
		f.SetFontUnitSize(text.Style.FontSize * scale)
	}
	if opacity := svgStyleOpacity(text.Style, false, true); opacity < 1 {
		f.SetAlpha(opacity, "Normal")
	}
	switch text.Style.TextAnchor {
	case "middle":
		x -= f.GetStringWidth(text.Text) / 2
	case "end":
		x -= f.GetStringWidth(text.Text)
	}
	f.Text(x, y, text.Text)
}
