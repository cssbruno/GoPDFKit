// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"fmt"
	"strconv"
)

// GetAlpha returns the alpha blending channel, which consists of the
// alpha transparency value and the blend mode. See SetAlpha for more
// details.
func (f *Fpdf) GetAlpha() (alpha float64, blendModeStr string) {
	return f.alpha, f.blendMode
}

// SetAlpha sets the alpha blending channel. The blending effect applies to
// text, drawings and images.
//
// alpha must be a value from 0.0 (fully transparent) to 1.0 (fully opaque).
// Values outside of this range result in an error.
//
// blendModeStr must be one of "Normal", "Multiply", "Screen", "Overlay",
// "Darken", "Lighten", "ColorDodge", "ColorBurn", "HardLight", "SoftLight",
// "Difference", "Exclusion", "Hue", "Saturation", "Color", or "Luminosity". An
// empty string is replaced with "Normal".
//
// To reset normal rendering after applying a blending mode, call this method
// with alpha set to 1.0 and blendModeStr set to "Normal".
func (f *Fpdf) SetAlpha(alpha float64, blendModeStr string) {
	if f.err != nil {
		return
	}
	var bl blendModeType
	switch blendModeStr {
	case "Normal", "Multiply", "Screen", "Overlay", "Darken", "Lighten", "ColorDodge", "ColorBurn", "HardLight", "SoftLight", "Difference", "Exclusion", "Hue", "Saturation", "Color", "Luminosity":
		bl.modeStr = blendModeStr
	case "":
		bl.modeStr = "Normal"
	default:
		f.err = fmt.Errorf("unrecognized blend mode \"%s\"", blendModeStr)
		return
	}
	if alpha < 0.0 || alpha > 1.0 {
		f.err = fmt.Errorf("alpha value (0.0 - 1.0) is out of range: %.3f", alpha)
		return
	}
	f.alpha = alpha
	f.blendMode = bl.modeStr
	alphaStr := sprintf("%.3f", alpha)
	keyStr := sprintf("%s %s", alphaStr, bl.modeStr)
	pos, ok := f.blendMap[keyStr]
	if !ok {
		pos = len(f.blendList)
		f.blendList = append(f.blendList, blendModeType{alphaStr, alphaStr, bl.modeStr, 0})
		f.blendMap[keyStr] = pos
	}
	f.outf("/GS%d gs", pos)
}

func (f *Fpdf) gradientClipStart(x, y, w, h float64) {
	f.outf("q %.2f %.2f %.2f %.2f re W n", x*f.k, (f.h-y)*f.k, w*f.k, -h*f.k)
	f.outf("%.5f 0 0 %.5f %.5f %.5f cm", w*f.k, h*f.k, x*f.k, (f.h-(y+h))*f.k)
}

func (f *Fpdf) gradientClipEnd() {
	f.out("Q")
}

func pdfGradientStopOffset(offset float64) float64 {
	if offset < 0 {
		return 0
	}
	if offset > 1 {
		return 1
	}
	return offset
}

func (f *Fpdf) gradient(tp, r1, g1, b1, r2, g2, b2 int, x1, y1, x2, y2, r float64) {
	clr1 := rgbColorValue(r1, g1, b1, "", "")
	clr2 := rgbColorValue(r2, g2, b2, "", "")
	f.gradientWithStops(tp, []gradientStopType{{offset: 0, clrStr: clr1.str}, {offset: 1, clrStr: clr2.str}}, x1, y1, x2, y2, r)
}

func (f *Fpdf) gradientWithStops(tp int, stops []gradientStopType, x1, y1, x2, y2, r float64) {
	pos := len(f.gradientList)
	if len(stops) < 2 {
		stops = []gradientStopType{{offset: 0, clrStr: "0 0 0"}, {offset: 1, clrStr: "0 0 0"}}
	}
	stops = append([]gradientStopType(nil), stops...)
	stops[0].offset = 0
	stops[len(stops)-1].offset = 1
	for j := 1; j < len(stops); j++ {
		stops[j].offset = pdfGradientStopOffset(stops[j].offset)
		if stops[j].offset < stops[j-1].offset {
			stops[j].offset = stops[j-1].offset
		}
	}
	f.gradientList = append(f.gradientList, gradientType{tp: tp, clr1Str: stops[0].clrStr, clr2Str: stops[len(stops)-1].clrStr, stops: stops, x1: x1, y1: y1, x2: x2, y2: y2, r: r})
	f.outf("/Sh%d sh", pos)
}

// LinearGradient draws a rectangular area with a blend from one color to
// another. The rectangle has width w and height h. Its upper-left corner is
// positioned at (x, y).
//
// Each color is specified with three component values, one each for red, green
// and blue. The values range from 0 to 255. The first color is specified by
// (r1, g1, b1) and the second color by (r2, g2, b2).
//
// The blending is controlled with a gradient vector that uses normalized
// coordinates in which the lower-left corner is at position (0, 0) and the
// upper-right corner is (1, 1). The vector's origin and destination are specified by
// the points (x1, y1) and (x2, y2). In a linear gradient, blending occurs
// perpendicularly to the vector. The vector does not necessarily need to be
// anchored on the rectangle edge. Color 1 is used up to the origin of the
// vector and color 2 is used beyond the vector's end point. Between the points
// the colors are gradually blended.
func (f *Fpdf) LinearGradient(x, y, w, h float64, r1, g1, b1, r2, g2, b2 int, x1, y1, x2, y2 float64) {
	f.gradientClipStart(x, y, w, h)
	f.gradient(2, r1, g1, b1, r2, g2, b2, x1, y1, x2, y2, 0)
	f.gradientClipEnd()
}

// RadialGradient draws a rectangular area with a blend from one color to
// another. The rectangle has width w and height h. Its upper-left corner is
// positioned at (x, y).
//
// Each color is specified with three component values, one each for red, green
// and blue. The values range from 0 to 255. The first color is specified by
// (r1, g1, b1) and the second color by (r2, g2, b2).
//
// The blending is controlled with a point and a circle, both specified with
// normalized coordinates in which the lower-left corner of the rendered
// rectangle is at position (0, 0) and the upper-right corner is (1, 1). Color 1
// begins at the origin point specified by (x1, y1). Color 2 begins at the
// circle specified by the center point (x2, y2) and radius r. Colors are
// gradually blended from the origin to the circle. The origin and the circle's
// center do not necessarily have to coincide, but the origin must be within
// the circle to avoid rendering problems.
//
// The LinearGradient() example demonstrates this method.
func (f *Fpdf) RadialGradient(x, y, w, h float64, r1, g1, b1, r2, g2, b2 int, x1, y1, x2, y2, r float64) {
	f.gradientClipStart(x, y, w, h)
	f.gradient(3, r1, g1, b1, r2, g2, b2, x1, y1, x2, y2, r)
	f.gradientClipEnd()
}

func (f *Fpdf) putBlendModes() {
	count := len(f.blendList)
	for j := 1; j < count; j++ {
		bl := f.blendList[j]
		f.newobj()
		f.blendList[j].objNum = f.n
		f.outf("<</Type /ExtGState /ca %s /CA %s /BM /%s>>", bl.fillStr, bl.strokeStr, bl.modeStr)
		f.out("endobj")
	}
}

func (f *Fpdf) putGradients() {
	count := len(f.gradientList)
	for j := 1; j < count; j++ {
		var f1 int
		gr := f.gradientList[j]
		if gr.tp == 2 || gr.tp == 3 {
			f1 = f.putGradientFunction(gr)
		}
		f.newobj()
		f.outf("<</ShadingType %d /ColorSpace /DeviceRGB", gr.tp)
		switch gr.tp {
		case 2:
			f.outf("/Coords [%.5f %.5f %.5f %.5f] /Function %d 0 R /Extend [true true]>>", gr.x1, gr.y1, gr.x2, gr.y2, f1)
		case 3:
			f.outf("/Coords [%.5f %.5f 0 %.5f %.5f %.5f] /Function %d 0 R /Extend [true true]>>", gr.x1, gr.y1, gr.x2, gr.y2, gr.r, f1)
		}
		f.out("endobj")
		f.gradientList[j].objNum = f.n
	}
}

func (f *Fpdf) putGradientFunction(gr gradientType) int {
	stops := gr.stops
	if len(stops) < 2 {
		stops = []gradientStopType{{offset: 0, clrStr: gr.clr1Str}, {offset: 1, clrStr: gr.clr2Str}}
	}
	if len(stops) == 2 {
		f.newobj()
		f.outf("<</FunctionType 2 /Domain [0.0 1.0] /C0 [%s] /C1 [%s] /N 1>>", stops[0].clrStr, stops[1].clrStr)
		f.out("endobj")
		return f.n
	}
	functionObjs := make([]int, 0, len(stops)-1)
	for j := 0; j < len(stops)-1; j++ {
		f.newobj()
		f.outf("<</FunctionType 2 /Domain [0.0 1.0] /C0 [%s] /C1 [%s] /N 1>>", stops[j].clrStr, stops[j+1].clrStr)
		f.out("endobj")
		functionObjs = append(functionObjs, f.n)
	}
	f.newobj()
	var buf bytes.Buffer
	buf.WriteString("<</FunctionType 3 /Domain [0.0 1.0] /Functions [")
	for j, objNum := range functionObjs {
		if j > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(strconv.Itoa(objNum))
		buf.WriteString(" 0 R")
	}
	buf.WriteString("] /Bounds [")
	for j := 1; j < len(stops)-1; j++ {
		if j > 1 {
			buf.WriteByte(' ')
		}
		buf.WriteString(strconv.FormatFloat(stops[j].offset, 'f', 5, 64))
	}
	buf.WriteString("] /Encode [")
	for j := range functionObjs {
		if j > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString("0 1")
	}
	buf.WriteString("]>>")
	f.out(buf.String())
	f.out("endobj")
	return f.n
}
