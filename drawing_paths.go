// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"math"
	"strings"
)

// Line draws a line between points (x1, y1) and (x2, y2) using the current
// draw color, line width, and cap style.
func (f *Fpdf) Line(x1, y1, x2, y2 float64) {
	f.outf("%.2f %.2f m %.2f %.2f l S", x1*f.k, (f.h-y1)*f.k, x2*f.k, (f.h-y2)*f.k)
}

// fillDrawOp normalizes shorthand style values to PDF path-painting operators.
func fillDrawOp(styleStr string) (opStr string) {
	switch strings.ToUpper(styleStr) {
	case "", "D":
		opStr = "S"
	case "F":
		opStr = "f"
	case "F*":
		opStr = "f*"
	case "FD", "DF":
		opStr = "B"
	case "FD*", "DF*":
		opStr = "B*"
	default:
		opStr = styleStr
	}
	return
}

// Rect outputs a rectangle of width w and height h with the upper-left corner
// positioned at (x, y).
//
// It can be drawn (border only), filled (with no border) or both. styleStr can
// be "F" for filled, "D" for outlined only, or "DF" or "FD" for outlined and
// filled. An empty string will be replaced with "D". Drawing uses the current
// draw color and line width centered on the rectangle's perimeter. Filling
// uses the current fill color.
func (f *Fpdf) Rect(x, y, w, h float64, styleStr string) {
	f.outf("%.2f %.2f %.2f %.2f re %s", x*f.k, (f.h-y)*f.k, w*f.k, -h*f.k, fillDrawOp(styleStr))
}

// RoundedRect outputs a rectangle of width w and height h with the upper-left
// corner positioned at (x, y). It can be drawn (border only), filled
// (with no border) or both. styleStr can be "F" for filled, "D" for outlined
// only, or "DF" or "FD" for outlined and filled. An empty string will be
// replaced with "D". Drawing uses the current draw color and line width
// centered on the rectangle's perimeter. Filling uses the current fill color.
// The rounded corners of the rectangle are specified by radius r. corners is a
// string that includes "1" to round the upper-left corner, "2" to round the
// upper right corner, "3" to round the lower right corner, and "4" to round
// the lower-left corner. A zero radius means a square corner. The RoundedRect
// example demonstrates this method.
func (f *Fpdf) RoundedRect(x, y, w, h, r float64, corners string, stylestr string) {
	var rTL, rTR, rBR, rBL float64
	if strings.Contains(corners, "1") {
		rTL = r
	}
	if strings.Contains(corners, "2") {
		rTR = r
	}
	if strings.Contains(corners, "3") {
		rBR = r
	}
	if strings.Contains(corners, "4") {
		rBL = r
	}
	f.RoundedRectExt(x, y, w, h, rTL, rTR, rBR, rBL, stylestr)
}

// RoundedRectExt behaves the same as RoundedRect() but supports a different
// radius for each corner. A zero radius means a square corner. See
// RoundedRect() for more details. This method is demonstrated in the
// RoundedRect() example.
func (f *Fpdf) RoundedRectExt(x, y, w, h, rTL, rTR, rBR, rBL float64, stylestr string) {
	f.roundedRectPath(x, y, w, h, rTL, rTR, rBR, rBL)
	f.out(fillDrawOp(stylestr))
}

// Circle draws a circle centered on point (x, y) with radius r.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color and line width centered on the circle's perimeter.
// Filling uses the current fill color.
func (f *Fpdf) Circle(x, y, r float64, styleStr string) {
	f.Ellipse(x, y, r, r, 0, styleStr)
}

// Ellipse draws an ellipse centered at point (x, y). rx and ry specify its
// horizontal and vertical radii.
//
// degRotate specifies the counter-clockwise angle in degrees that the ellipse
// will be rotated.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color and line width centered on the ellipse's perimeter.
// Filling uses the current fill color.
//
// The Circle() example demonstrates this method.
func (f *Fpdf) Ellipse(x, y, rx, ry, degRotate float64, styleStr string) {
	f.arc(x, y, rx, ry, degRotate, 0, 360, styleStr, false)
}

// Polygon draws a closed figure defined by a series of vertices specified by
// points. The x and y fields of the points use the units established in New().
// The last point in the slice will be implicitly joined to the first to close
// the polygon.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color and line width centered on the polygon's perimeter.
// Filling uses the current fill color.
func (f *Fpdf) Polygon(points []Point, styleStr string) {
	if len(points) > 2 {
		for j, pt := range points {
			if j == 0 {
				f.point(pt.X, pt.Y)
			} else {
				f.outf("%.5f %.5f l ", pt.X*f.k, (f.h-pt.Y)*f.k)
			}
		}
		f.outf("%.5f %.5f l ", points[0].X*f.k, (f.h-points[0].Y)*f.k)
		f.DrawPath(styleStr)
	}
}

// Beziergon draws a closed figure defined by a series of cubic Bézier curve
// segments. The first point in the slice defines the starting point of the
// figure. Each three following points p1, p2, p3 represent a curve segment to
// the point p3 using p1 and p2 as the Bézier control points.
//
// The x and y fields of the points use the units established in New().
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color and line width centered on the path's perimeter.
// Filling uses the current fill color.
func (f *Fpdf) Beziergon(points []Point, styleStr string) {
	if len(points) < 4 {
		return
	}
	f.point(points[0].XY())
	points = points[1:]
	for len(points) >= 3 {
		cx0, cy0 := points[0].XY()
		cx1, cy1 := points[1].XY()
		x1, y1 := points[2].XY()
		f.curve(cx0, cy0, cx1, cy1, x1, y1)
		points = points[3:]
	}
	f.DrawPath(styleStr)
}

// point outputs the current point.
func (f *Fpdf) point(x, y float64) {
	f.outf("%.2f %.2f m", x*f.k, (f.h-y)*f.k)
}

// curve outputs a single cubic Bézier curve segment from the current point.
func (f *Fpdf) curve(cx0, cy0, cx1, cy1, x, y float64) {
	f.outf("%.5f %.5f %.5f %.5f %.5f %.5f c", cx0*f.k, (f.h-cy0)*f.k, cx1*f.k, (f.h-cy1)*f.k, x*f.k, (f.h-y)*f.k)
}

// Curve draws a single-segment quadratic Bézier curve. The curve starts at
// the point (x0, y0) and ends at the point (x1, y1). The control point (cx,
// cy) specifies the curvature. At the start point, the curve is tangent to the
// straight line between the start point and the control point. At the end
// point, the curve is tangent to the straight line between the end point and
// the control point.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color, line width, and cap style centered on the curve's
// path. Filling uses the current fill color.
//
// The Circle() example demonstrates this method.
func (f *Fpdf) Curve(x0, y0, cx, cy, x1, y1 float64, styleStr string) {
	f.point(x0, y0)
	f.outf("%.5f %.5f %.5f %.5f v %s", cx*f.k, (f.h-cy)*f.k, x1*f.k, (f.h-y1)*f.k, fillDrawOp(styleStr))
}

// CurveCubic draws a single-segment cubic Bézier curve. This routine performs
// the same function as CurveBezierCubic() but has a nonstandard argument order.
// It is retained to preserve backward compatibility.
func (f *Fpdf) CurveCubic(x0, y0, cx0, cy0, x1, y1, cx1, cy1 float64, styleStr string) {
	f.CurveBezierCubic(x0, y0, cx0, cy0, cx1, cy1, x1, y1, styleStr)
}

// CurveBezierCubic draws a single-segment cubic Bézier curve. The curve starts at
// the point (x0, y0) and ends at the point (x1, y1). The control points
// (cx0, cy0) and (cx1, cy1) specify the curvature. At the start point, the
// curve is tangent to the straight line between the start point and the
// control point (cx0, cy0). At the end point, the curve is tangent to the
// straight line between the end point and the control point (cx1, cy1).
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color, line width, and cap style centered on the curve's
// path. Filling uses the current fill color.
//
// This routine performs the same function as CurveCubic() but uses standard
// argument order.
//
// The Circle() example demonstrates this method.
func (f *Fpdf) CurveBezierCubic(x0, y0, cx0, cy0, cx1, cy1, x1, y1 float64, styleStr string) {
	f.point(x0, y0)
	f.outf("%.5f %.5f %.5f %.5f %.5f %.5f c %s", cx0*f.k, (f.h-cy0)*f.k, cx1*f.k, (f.h-cy1)*f.k, x1*f.k, (f.h-y1)*f.k, fillDrawOp(styleStr))
}

// Arc draws an elliptical arc centered at point (x, y). rx and ry specify its
// horizontal and vertical radii.
//
// degRotate specifies the angle that the arc will be rotated. degStart and
// degEnd specify the starting and ending angle of the arc. All angles are
// specified in degrees and measured counter-clockwise from the 3 o'clock
// position.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color, line width, and cap style centered on the arc's
// path. Filling uses the current fill color.
//
// The Circle() example demonstrates this method.
func (f *Fpdf) Arc(x, y, rx, ry, degRotate, degStart, degEnd float64, styleStr string) {
	f.arc(x, y, rx, ry, degRotate, degStart, degEnd, styleStr, false)
}

// MoveTo moves the stylus to (x, y) without drawing from the previous point.
// Paths must start with MoveTo to set the original stylus location; otherwise,
// the result is undefined.
//
// Create a path by moving a virtual stylus around the page with MoveTo(),
// LineTo(), CurveTo(), CurveBezierCubicTo(), ArcTo(), and ClosePath(), then
// draw or fill it with DrawPath(). Path drawing routines produce proper PDF
// line joins at angles instead of overlaying separate line segments.
func (f *Fpdf) MoveTo(x, y float64) {
	f.point(x, y)
	f.x, f.y = x, y
}

// LineTo creates a line from the current stylus location to (x, y), which
// becomes the new stylus location. Note that this only creates the line in
// the path; it does not actually draw the line on the page.
//
// The MoveTo() example demonstrates this method.
func (f *Fpdf) LineTo(x, y float64) {
	f.outf("%.2f %.2f l", x*f.k, (f.h-y)*f.k)
	f.x, f.y = x, y
}

// CurveTo creates a single-segment quadratic Bézier curve. The curve starts at
// the current stylus location and ends at the point (x, y). The control point
// (cx, cy) specifies the curvature. At the start point, the curve is tangent
// to the straight line between the current stylus location and the control
// point. At the end point, the curve is tangent to the straight line between
// the end point and the control point.
//
// The MoveTo() example demonstrates this method.
func (f *Fpdf) CurveTo(cx, cy, x, y float64) {
	f.outf("%.5f %.5f %.5f %.5f v", cx*f.k, (f.h-cy)*f.k, x*f.k, (f.h-y)*f.k)
	f.x, f.y = x, y
}

// CurveBezierCubicTo creates a single-segment cubic Bézier curve. The curve
// starts at the current stylus location and ends at the point (x, y). The
// control points (cx0, cy0) and (cx1, cy1) specify the curvature. At the
// current stylus location, the curve is tangent to the straight line between the
// current stylus location and the control point (cx0, cy0). At the end point,
// the curve is tangent to the straight line between the end point and the
// control point (cx1, cy1).
//
// The MoveTo() example demonstrates this method.
func (f *Fpdf) CurveBezierCubicTo(cx0, cy0, cx1, cy1, x, y float64) {
	f.curve(cx0, cy0, cx1, cy1, x, y)
	f.x, f.y = x, y
}

// ClosePath creates a line from the current location to the last MoveTo point
// (if not the same) and marks the path as closed so the first and last lines
// join nicely.
//
// The MoveTo() example demonstrates this method.
func (f *Fpdf) ClosePath() {
	f.outf("h")
}

// DrawPath actually draws the path on the page.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D".
// Path-painting operators as defined in the PDF specification are also
// allowed: "S" (Stroke the path), "s" (Close and stroke the path),
// "f" (Fill the path, using the nonzero winding number), "f*"
// (Fill the path, using the even-odd rule), "B" (Fill and then stroke
// the path, using the nonzero winding number rule), "B*" (Fill and
// then stroke the path, using the even-odd rule), "b" (Close, fill,
// and then stroke the path, using the nonzero winding number rule), and
// "b*" (Close, fill, and then stroke the path, using the even-odd
// rule).
// Drawing uses the current draw color, line width, and cap style centered on
// the path. Filling uses the current fill color.
//
// The MoveTo() example demonstrates this method.
func (f *Fpdf) DrawPath(styleStr string) {
	f.out(fillDrawOp(styleStr))
}

// ArcTo draws an elliptical arc centered at point (x, y). rx and ry specify its
// horizontal and vertical radii. If the start of the arc is not at
// the current position, a connecting line will be drawn.
//
// degRotate specifies the angle that the arc will be rotated. degStart and
// degEnd specify the starting and ending angle of the arc. All angles are
// specified in degrees and measured counter-clockwise from the 3 o'clock
// position.
//
// styleStr can be "F" for filled, "D" for outlined only, or "DF" or "FD" for
// outlined and filled. An empty string will be replaced with "D". Drawing uses
// the current draw color, line width, and cap style centered on the arc's
// path. Filling uses the current fill color.
//
// The MoveTo() example demonstrates this method.
func (f *Fpdf) ArcTo(x, y, rx, ry, degRotate, degStart, degEnd float64) {
	f.arc(x, y, rx, ry, degRotate, degStart, degEnd, "", true)
}

func (f *Fpdf) arc(x, y, rx, ry, degRotate, degStart, degEnd float64, styleStr string, path bool) {
	x *= f.k
	y = (f.h - y) * f.k
	rx *= f.k
	ry *= f.k
	segments := max(int(degEnd-degStart)/60, 2)
	angleStart := degStart * math.Pi / 180
	angleEnd := degEnd * math.Pi / 180
	angleTotal := angleEnd - angleStart
	dt := angleTotal / float64(segments)
	dtm := dt / 3
	if degRotate != 0 {
		a := -degRotate * math.Pi / 180
		f.outf("q %.5f %.5f %.5f %.5f %.5f %.5f cm", math.Cos(a), -1*math.Sin(a), math.Sin(a), math.Cos(a), x, y)
		x = 0
		y = 0
	}
	t := angleStart
	a0 := x + rx*math.Cos(t)
	b0 := y + ry*math.Sin(t)
	c0 := -rx * math.Sin(t)
	d0 := ry * math.Cos(t)
	sx := a0 / f.k
	sy := f.h - (b0 / f.k)
	if path {
		if f.x != sx || f.y != sy {
			f.LineTo(sx, sy)
		}
	} else {
		f.point(sx, sy)
	}
	for j := 1; j <= segments; j++ {
		t = (float64(j) * dt) + angleStart
		a1 := x + rx*math.Cos(t)
		b1 := y + ry*math.Sin(t)
		c1 := -rx * math.Sin(t)
		d1 := ry * math.Cos(t)
		f.curve((a0+(c0*dtm))/f.k, f.h-((b0+(d0*dtm))/f.k), (a1-(c1*dtm))/f.k, f.h-((b1-(d1*dtm))/f.k), a1/f.k, f.h-(b1/f.k))
		a0 = a1
		b0 = b1
		c0 = c1
		d0 = d1
		if path {
			f.x = a1 / f.k
			f.y = f.h - (b1 / f.k)
		}
	}
	if !path {
		f.out(fillDrawOp(styleStr))
	}
	if degRotate != 0 {
		f.out("Q")
	}
}
