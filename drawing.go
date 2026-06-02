// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"strconv"
)

// GetStringWidth returns the length of a string in user units. A font must be
// currently selected.
func (f *Fpdf) GetStringWidth(s string) float64 {
	if f.err != nil {
		return 0
	}
	w := f.GetStringSymbolWidth(s)
	return float64(w) * f.fontSize / 1000
}

// GetStringSymbolWidth returns the length of a string in glyph units. A font
// must be currently selected.
func (f *Fpdf) GetStringSymbolWidth(s string) int {
	if f.err != nil {
		return 0
	}
	w := 0
	if f.isCurrentUTF8 {
		for _, char := range s {
			w += f.currentFontRuneWidth(char)
		}
	} else {
		for _, ch := range []byte(s) {
			if ch == 0 {
				break
			}
			w += f.currentFont.Cw[ch]
		}
	}
	return w
}

func (f *Fpdf) currentFontRuneWidth(char rune) int {
	intChar := int(char)
	if intChar >= 0 && intChar < len(f.currentFont.Cw) {
		width := f.currentFont.Cw[intChar]
		if width == 65535 {
			return 0
		}
		if width > 0 {
			return width
		}
	}
	if f.currentFont.Desc.MissingWidth != 0 {
		return f.currentFont.Desc.MissingWidth
	}
	return 500
}

// SetLineWidth defines the line width. By default, the value equals 0.2 mm.
// The method can be called before the first page is created. The value is
// retained from page to page.
func (f *Fpdf) SetLineWidth(width float64) {
	f.setLineWidth(width)
}

func (f *Fpdf) setLineWidth(width float64) {
	f.lineWidth = width
	if f.page > 0 {
		f.outf("%.2f w", width*f.k)
	}
}

// GetLineWidth returns the current line thickness.
func (f *Fpdf) GetLineWidth() float64 {
	return f.lineWidth
}

// SetLineCapStyle defines the line cap style. styleStr should be "butt",
// "round" or "square". A square style projects from the end of the line. The
// method can be called before the first page is created. The value is
// retained from page to page.
func (f *Fpdf) SetLineCapStyle(styleStr string) {
	var capStyle int
	switch styleStr {
	case "round":
		capStyle = 1
	case "square":
		capStyle = 2
	default:
		capStyle = 0
	}
	f.capStyle = capStyle
	if f.page > 0 {
		f.outf("%d J", f.capStyle)
	}
}

// SetLineJoinStyle defines the line join style. styleStr should be "miter",
// "round" or "bevel". The method can be called before the first page is
// created. The value is retained from page to page.
func (f *Fpdf) SetLineJoinStyle(styleStr string) {
	var joinStyle int
	switch styleStr {
	case "round":
		joinStyle = 1
	case "bevel":
		joinStyle = 2
	default:
		joinStyle = 0
	}
	f.joinStyle = joinStyle
	if f.page > 0 {
		f.outf("%d j", f.joinStyle)
	}
}

// SetDashPattern sets the dash pattern that is used to draw lines. The
// dashArray elements are numbers that specify the lengths, in units
// established in New(), of alternating dashes and gaps. The dash phase
// specifies the distance into the dash pattern at which to start the dash. The
// dash pattern is retained from page to page. Call this method with an empty
// array to restore solid line drawing.
//
// The Beziergon() example demonstrates this method.
func (f *Fpdf) SetDashPattern(dashArray []float64, dashPhase float64) {
	scaled := make([]float64, len(dashArray))
	for i, value := range dashArray {
		scaled[i] = value * f.k
	}
	dashPhase *= f.k
	f.dashArray = scaled
	f.dashPhase = dashPhase
	if f.page > 0 {
		f.outputDashPattern()
	}
}

func (f *Fpdf) outputDashPattern() {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, value := range f.dashArray {
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.WriteString(strconv.FormatFloat(value, 'f', 2, 64))
	}
	buf.WriteString("] ")
	buf.WriteString(strconv.FormatFloat(f.dashPhase, 'f', 2, 64))
	buf.WriteString(" d")
	f.outbuf(&buf)
}
