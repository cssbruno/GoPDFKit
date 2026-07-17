// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
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
	if f.err != nil {
		return
	}
	if !finiteNumbers(x, y, w, h) {
		f.SetErrorf("invalid clipping rectangle")
		return
	}
	f.clipNest++
	f.outf("q %.2f %.2f %.2f %.2f re W %s", x*f.k, (f.h-y)*f.k, w*f.k, -h*f.k, strIf(outline, "S", "n"))
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
	if f.err != nil {
		return
	}
	if !finiteNumbers(x, y) {
		f.SetErrorf("invalid clipping text position")
		return
	}
	f.clipNest++
	f.outf("q BT %.5f %.5f Td %d Tr (%s) Tj ET", x*f.k, (f.h-y)*f.k, intIf(outline, 5, 7), f.escape(txtStr))
}

func (f *Document) clipArc(x1, y1, x2, y2, x3, y3 float64) {
	h := f.h
	f.outf("%.5f %.5f %.5f %.5f %.5f %.5f c ", x1*f.k, (h-y1)*f.k, x2*f.k, (h-y2)*f.k, x3*f.k, (h-y3)*f.k)
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
	if f.err != nil {
		return
	}
	if !finiteNumbers(x, y, w, h, rTL, rTR, rBR, rBL) {
		f.SetErrorf("invalid rounded clipping rectangle")
		return
	}
	f.clipNest++
	f.roundedRectPath(x, y, w, h, rTL, rTR, rBR, rBL)
	f.outf(" W %s", strIf(outline, "S", "n"))
}

// roundedRectPath adds a rounded rectangle path. RoundedRect() and
// ClipRoundedRect() share this helper and add their own drawing operation.
func (f *Document) roundedRectPath(x, y, w, h, rTL, rTR, rBR, rBL float64) {
	k := f.k
	hp := f.h
	myArc := (4.0 / 3.0) * (math.Sqrt2 - 1.0)
	f.outf("q %.5f %.5f m", (x+rTL)*k, (hp-y)*k)
	xc := x + w - rTR
	yc := y + rTR
	f.outf("%.5f %.5f l", xc*k, (hp-y)*k)
	if rTR != 0 {
		f.clipArc(xc+rTR*myArc, yc-rTR, xc+rTR, yc-rTR*myArc, xc+rTR, yc)
	}
	xc = x + w - rBR
	yc = y + h - rBR
	f.outf("%.5f %.5f l", (x+w)*k, (hp-yc)*k)
	if rBR != 0 {
		f.clipArc(xc+rBR, yc+rBR*myArc, xc+rBR*myArc, yc+rBR, xc, yc+rBR)
	}
	xc = x + rBL
	yc = y + h - rBL
	f.outf("%.5f %.5f l", xc*k, (hp-(y+h))*k)
	if rBL != 0 {
		f.clipArc(xc-rBL*myArc, yc+rBL, xc-rBL, yc+rBL*myArc, xc-rBL, yc)
	}
	xc = x + rTL
	yc = y + rTL
	f.outf("%.5f %.5f l", x*k, (hp-yc)*k)
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
	if f.err != nil {
		return
	}
	if !finiteNumbers(x, y, rx, ry) {
		f.SetErrorf("invalid clipping ellipse")
		return
	}
	f.clipNest++
	lx := (4.0 / 3.0) * rx * (math.Sqrt2 - 1)
	ly := (4.0 / 3.0) * ry * (math.Sqrt2 - 1)
	k := f.k
	h := f.h
	f.outf("q %.5f %.5f m %.5f %.5f %.5f %.5f %.5f %.5f c", (x+rx)*k, (h-y)*k, (x+rx)*k, (h-(y-ly))*k, (x+lx)*k, (h-(y-ry))*k, x*k, (h-(y-ry))*k)
	f.outf("%.5f %.5f %.5f %.5f %.5f %.5f c", (x-lx)*k, (h-(y-ry))*k, (x-rx)*k, (h-(y-ly))*k, (x-rx)*k, (h-y)*k)
	f.outf("%.5f %.5f %.5f %.5f %.5f %.5f c", (x-rx)*k, (h-(y+ly))*k, (x-lx)*k, (h-(y+ry))*k, x*k, (h-(y+ry))*k)
	f.outf("%.5f %.5f %.5f %.5f %.5f %.5f c W %s", (x+lx)*k, (h-(y+ry))*k, (x+rx)*k, (h-(y+ly))*k, (x+rx)*k, (h-y)*k, strIf(outline, "S", "n"))
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
	if f.err != nil {
		return
	}
	if len(points) < 3 {
		f.SetErrorf("clipping polygon requires at least 3 points")
		return
	}
	for _, pt := range points {
		if !finiteNumbers(pt.X, pt.Y) {
			f.SetErrorf("invalid clipping polygon point")
			return
		}
	}
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
