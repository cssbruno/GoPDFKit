// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"math"
)

// ClipRect begins a rectangular clipping operation. The rectangle has width w
// and height h. Its upper-left corner is positioned at (x, y). outline
// is true to draw a border with the current draw color and line width centered
// on the rectangle's perimeter. Only the outer half of the border will be
// shown. After calling this method, all rendering operations (for example,
// ImageOptions(), LinearGradient(), etc.) will be clipped by the specified
// rectangle. Call ClipEnd() to restore unclipped operations.
//
// This ClipText() example demonstrates this method.
func (f *Document) ClipRect(x, y, w, h float64, outline bool) {
	f.clipNest++
	var scratch [96]byte
	buf := append(scratch[:0], "q "...)
	buf = appendPDFRectPaint(buf, x*f.k, (f.h-y)*f.k, w*f.k, -h*f.k, "W", true)
	buf = append(buf, strIf(outline, "S", "n")...)
	f.outbytes(buf)
}

// ClipText begins a clipping operation in which rendering is confined to the
// character string specified by txtStr. The origin (x, y) is at the left side
// of the first character's baseline. The current font is used. outline is
// true to draw a border with the current draw color and line width centered on
// the perimeters of the text characters. Only the outer half of the border
// will be shown. After calling this method, all rendering operations (for
// example, ImageOptions(), LinearGradient(), etc.) will be clipped. Call
// ClipEnd() to restore unclipped operations.
func (f *Document) ClipText(x, y float64, txtStr string, outline bool) {
	f.clipNest++
	escaped := f.escape(txtStr)
	buf := make([]byte, 0, len(escaped)+64)
	buf = append(buf, "q BT "...)
	buf = appendPDFNumberSpace(buf, x*f.k, 5)
	buf = appendPDFNumberSpace(buf, (f.h-y)*f.k, 5)
	buf = append(buf, "Td "...)
	buf = appendPDFInt(buf, intIf(outline, 5, 7))
	buf = append(buf, " Tr ("...)
	buf = append(buf, escaped...)
	buf = append(buf, ") Tj ET"...)
	f.outbytes(buf)
}

func (f *Document) clipArc(x1, y1, x2, y2, x3, y3 float64) {
	h := f.h
	var scratch [96]byte
	buf := appendPDFCubicCurve(scratch[:0], x1*f.k, (h-y1)*f.k, x2*f.k, (h-y2)*f.k, x3*f.k, (h-y3)*f.k)
	buf = append(buf, ' ')
	f.outbytes(buf)
}

// ClipRoundedRect begins a rectangular clipping operation. The rectangle has
// width w and height h. Its upper-left corner is positioned at (x, y).
// The rounded corners of the rectangle are specified by radius r. outline is
// true to draw a border with the current draw color and line width centered on
// the rectangle's perimeter. Only the outer half of the border will be shown.
// After calling this method, all rendering operations (for example,
// ImageOptions(), LinearGradient(), etc.) will be clipped by the specified
// rectangle. Call
// ClipEnd() to restore unclipped operations.
//
// This ClipText() example demonstrates this method.
func (f *Document) ClipRoundedRect(x, y, w, h, r float64, outline bool) {
	f.ClipRoundedRectExt(x, y, w, h, r, r, r, r, outline)
}

// ClipRoundedRectExt behaves the same as ClipRoundedRect() but supports a
// different radius for each corner, given by rTL (top-left), rTR (top-right),
// rBR (bottom-right), rBL (bottom-left). See ClipRoundedRect() for more
// details. This method is demonstrated in the ClipText() example.
func (f *Document) ClipRoundedRectExt(x, y, w, h, rTL, rTR, rBR, rBL float64, outline bool) {
	f.clipNest++
	f.roundedRectPath(x, y, w, h, rTL, rTR, rBR, rBL)
	f.out(" W " + strIf(outline, "S", "n"))
}

// roundedRectPath adds a rounded rectangle path. RoundedRect() and
// ClipRoundedRect() share this helper and add their own drawing operation.
func (f *Document) roundedRectPath(x, y, w, h, rTL, rTR, rBR, rBL float64) {
	k := f.k
	hp := f.h
	myArc := (4.0 / 3.0) * (math.Sqrt2 - 1.0)
	var scratch [96]byte
	buf := append(scratch[:0], "q "...)
	buf = appendPDFMoveTo(buf, (x+rTL)*k, (hp-y)*k, 5)
	f.outbytes(buf)
	xc := x + w - rTR
	yc := y + rTR
	f.outbytes(appendPDFLineTo(scratch[:0], xc*k, (hp-y)*k, 5))
	if rTR != 0 {
		f.clipArc(xc+rTR*myArc, yc-rTR, xc+rTR, yc-rTR*myArc, xc+rTR, yc)
	}
	xc = x + w - rBR
	yc = y + h - rBR
	f.outbytes(appendPDFLineTo(scratch[:0], (x+w)*k, (hp-yc)*k, 5))
	if rBR != 0 {
		f.clipArc(xc+rBR, yc+rBR*myArc, xc+rBR*myArc, yc+rBR, xc, yc+rBR)
	}
	xc = x + rBL
	yc = y + h - rBL
	f.outbytes(appendPDFLineTo(scratch[:0], xc*k, (hp-(y+h))*k, 5))
	if rBL != 0 {
		f.clipArc(xc-rBL*myArc, yc+rBL, xc-rBL, yc+rBL*myArc, xc-rBL, yc)
	}
	xc = x + rTL
	yc = y + rTL
	f.outbytes(appendPDFLineTo(scratch[:0], x*k, (hp-yc)*k, 5))
	if rTL != 0 {
		f.clipArc(xc-rTL, yc-rTL*myArc, xc-rTL*myArc, yc-rTL, xc, yc-rTL)
	}
}

// ClipEllipse begins an elliptical clipping operation. The ellipse is centered
// at (x, y). Its horizontal and vertical radii are specified by rx and ry.
// outline is true to draw a border with the current draw color and line width
// centered on the ellipse's perimeter. Only the outer half of the border will
// be shown. After calling this method, all rendering operations (for example,
// ImageOptions(), LinearGradient(), etc.) will be clipped by the specified
// ellipse. Call ClipEnd() to restore unclipped operations.
//
// This ClipText() example demonstrates this method.
func (f *Document) ClipEllipse(x, y, rx, ry float64, outline bool) {
	f.clipNest++
	lx := (4.0 / 3.0) * rx * (math.Sqrt2 - 1)
	ly := (4.0 / 3.0) * ry * (math.Sqrt2 - 1)
	k := f.k
	h := f.h
	var scratch [160]byte
	buf := append(scratch[:0], "q "...)
	buf = appendPDFMoveTo(buf, (x+rx)*k, (h-y)*k, 5)
	buf = append(buf, ' ')
	buf = appendPDFCubicCurve(buf, (x+rx)*k, (h-(y-ly))*k, (x+lx)*k, (h-(y-ry))*k, x*k, (h-(y-ry))*k)
	f.outbytes(buf)
	f.outbytes(appendPDFCubicCurve(scratch[:0], (x-lx)*k, (h-(y-ry))*k, (x-rx)*k, (h-(y-ly))*k, (x-rx)*k, (h-y)*k))
	f.outbytes(appendPDFCubicCurve(scratch[:0], (x-rx)*k, (h-(y+ly))*k, (x-lx)*k, (h-(y+ry))*k, x*k, (h-(y+ry))*k))
	buf = appendPDFCubicCurve(scratch[:0], (x+lx)*k, (h-(y+ry))*k, (x+rx)*k, (h-(y+ly))*k, (x+rx)*k, (h-y)*k)
	buf = append(buf, " W "...)
	buf = append(buf, strIf(outline, "S", "n")...)
	f.outbytes(buf)
}

// ClipCircle begins a circular clipping operation. The circle is centered at
// (x, y) and has radius r. outline is true to draw a border with the current
// draw color and line width centered on the circle's perimeter. Only the outer
// half of the border will be shown. After calling this method, all rendering
// operations (for example, ImageOptions(), LinearGradient(), etc.) will be
// clipped by the specified circle. Call ClipEnd() to restore unclipped
// operations.
//
// The ClipText() example demonstrates this method.
func (f *Document) ClipCircle(x, y, r float64, outline bool) {
	f.ClipEllipse(x, y, r, r, outline)
}

// ClipPolygon begins a clipping operation within a polygon. The figure is
// defined by a series of vertices specified by points. The x and y fields of
// the points use the units established in New(). The last point in the slice
// will be implicitly joined to the first to close the polygon. outline is true
// to draw a border with the current draw color and line width centered on the
// polygon's perimeter. Only the outer half of the border will be shown. After
// calling this method, all rendering operations (for example, ImageOptions(),
// LinearGradient(), etc.) will be clipped by the specified polygon. Call
// ClipEnd() to restore unclipped operations.
//
// The ClipText() example demonstrates this method.
func (f *Document) ClipPolygon(points []Point, outline bool) {
	f.clipNest++
	var s fmtBuffer
	h := f.h
	k := f.k
	s.printf("q ")
	for j, pt := range points {
		s.printf("%.5f %.5f %s ", pt.X*k, (h-pt.Y)*k, strIf(j == 0, "m", "l"))
	}
	s.printf("h W %s", strIf(outline, "S", "n"))
	f.out(s.String())
}

// ClipEnd ends a clipping operation that was started with a call to
// ClipRect(), ClipRoundedRect(), ClipText(), ClipEllipse(), ClipCircle() or
// ClipPolygon(). Clipping operations can be nested. The document cannot be
// output successfully while a clipping operation is active.
//
// The ClipText() example demonstrates this method.
func (f *Document) ClipEnd() {
	if f.err == nil {
		if f.clipNest > 0 {
			f.clipNest--
			f.out("Q")
		} else {
			f.err = errors.New("error attempting to end clip operation out of sequence")
		}
	}
}
