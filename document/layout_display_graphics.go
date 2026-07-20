// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "github.com/cssbruno/paperrune/internal/layoutengine"

func (f *Document) paintPlannedTransform(pageHeight layoutengine.Fixed, transform layoutengine.Transform) {
	h := pageHeight.Points()
	a := transform.A.Points()
	b := transform.B.Points()
	c := transform.C.Points()
	d := transform.D.Points()
	tx := transform.TX.Points()
	ty := transform.TY.Points()
	buf := f.contentCommandBuffer(128)
	// Convert the plan's top-left, Y-down affine matrix into PDF's bottom-left,
	// Y-up coordinate system: S * T * S^-1.
	for _, value := range []float64{a, -b, -c, d, c*h + tx, h*(1-d) - ty} {
		if value == 0 {
			value = 0 // canonicalize negative zero
		}
		buf = appendPDFNumberSpace(buf, value, 10)
	}
	buf = append(buf, 'c', 'm')
	f.outbytes(buf)
	f.retainContentCommandBuffer(buf)
}

func (f *Document) paintPlannedClip(pageHeight layoutengine.Fixed, path layoutengine.PlannedPath, clip layoutengine.PlannedClip) {
	buf := appendPlannedPDFPath(f.contentCommandBuffer(256), pageHeight, path)
	if clip.Rule == layoutengine.FillEvenOdd {
		buf = append(buf, ' ', 'W', '*', ' ', 'n')
	} else {
		buf = append(buf, ' ', 'W', ' ', 'n')
	}
	f.outbytes(buf)
	f.retainContentCommandBuffer(buf)
}

func (f *Document) paintPlannedFill(pageHeight layoutengine.Fixed, path layoutengine.PlannedPath, fill layoutengine.PlannedFill) {
	if fill.Opacity != 0 {
		f.out("q")
		f.SetAlpha(fill.Opacity.Points(), "Normal")
		defer f.out("Q")
	}
	buf := appendPlannedRGB(f.contentCommandBuffer(320), fill.Color, false)
	buf = appendPlannedPDFPath(buf, pageHeight, path)
	if fill.Rule == layoutengine.FillEvenOdd {
		buf = append(buf, ' ', 'f', '*')
	} else {
		buf = append(buf, ' ', 'f')
	}
	f.outbytes(buf)
	f.retainContentCommandBuffer(buf)
}

func (f *Document) paintPlannedStroke(pageHeight layoutengine.Fixed, path layoutengine.PlannedPath, stroke layoutengine.PlannedStroke) {
	if stroke.Opacity != 0 {
		f.out("q")
		f.SetAlpha(stroke.Opacity.Points(), "Normal")
		defer f.out("Q")
	}
	buf := appendPlannedRGB(f.contentCommandBuffer(320), stroke.Color, true)
	buf = appendPDFNumberSpace(buf, stroke.Width.Points(), 10)
	buf = append(buf, 'w', ' ')
	capStyle := 0
	switch stroke.LineCap {
	case layoutengine.StrokeCapRound:
		capStyle = 1
	case layoutengine.StrokeCapSquare:
		capStyle = 2
	}
	joinStyle := 0
	switch stroke.LineJoin {
	case layoutengine.StrokeJoinRound:
		joinStyle = 1
	case layoutengine.StrokeJoinBevel:
		joinStyle = 2
	}
	buf = append(buf, byte('0'+capStyle), ' ', 'J', ' ', byte('0'+joinStyle), ' ', 'j', ' ', '[')
	for index, value := range stroke.Dash {
		if index != 0 {
			buf = append(buf, ' ')
		}
		buf = appendPDFNumber(buf, value.Points(), 10)
	}
	buf = append(buf, ']', ' ')
	buf = appendPDFNumberSpace(buf, stroke.DashOffset.Points(), 10)
	buf = append(buf, 'd', ' ')
	buf = appendPlannedPDFPath(buf, pageHeight, path)
	buf = append(buf, ' ', 'S')
	f.outbytes(buf)
	f.retainContentCommandBuffer(buf)
}

func appendPlannedRGB(buf []byte, color layoutengine.CoreRGBColor, stroke bool) []byte {
	buf = appendPDFNumberSpace(buf, float64(color.R)/255, 10)
	buf = appendPDFNumberSpace(buf, float64(color.G)/255, 10)
	buf = appendPDFNumberSpace(buf, float64(color.B)/255, 10)
	if stroke {
		return append(buf, 'R', 'G', ' ')
	}
	return append(buf, 'r', 'g', ' ')
}

func appendPlannedPDFPath(buf []byte, pageHeight layoutengine.Fixed, path layoutengine.PlannedPath) []byte {
	h := pageHeight.Points()
	point := func(value layoutengine.Point) {
		buf = appendPDFNumberSpace(buf, value.X.Points(), 10)
		buf = appendPDFNumberSpace(buf, h-value.Y.Points(), 10)
	}
	for index, segment := range path.Segments {
		if index != 0 {
			buf = append(buf, ' ')
		}
		switch segment.Kind {
		case layoutengine.PathMoveTo:
			point(segment.Point)
			buf = append(buf, 'm')
		case layoutengine.PathLineTo:
			point(segment.Point)
			buf = append(buf, 'l')
		case layoutengine.PathCubicTo:
			point(segment.Control1)
			point(segment.Control2)
			point(segment.Point)
			buf = append(buf, 'c')
		case layoutengine.PathClose:
			buf = append(buf, 'h')
		}
	}
	return buf
}
