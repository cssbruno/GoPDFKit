// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

func colorComp(v int) (int, float64) {
	if v < 0 {
		v = 0
	} else if v > 255 {
		v = 255
	}
	return v, float64(v) / 255.0
}

func rgbColorValue(r, g, b int, grayStr, fullStr string) (clr pdfColor) {
	clr.ir, clr.r = colorComp(r)
	clr.ig, clr.g = colorComp(g)
	clr.ib, clr.b = colorComp(b)
	clr.mode = colorModeRGB
	clr.gray = clr.ir == clr.ig && clr.r == clr.b
	if len(grayStr) > 0 {
		if clr.gray {
			clr.str = sprintf("%.3f %s", clr.r, grayStr)
		} else {
			clr.str = sprintf("%.3f %.3f %.3f %s", clr.r, clr.g, clr.b, fullStr)
		}
	} else {
		clr.str = sprintf("%.3f %.3f %.3f", clr.r, clr.g, clr.b)
	}
	return
}

// SetDrawColor defines the color used for all drawing operations (lines,
// rectangles and cell borders). It is expressed in RGB components (0-255).
// The method can be called before the first page is created. The value is
// retained from page to page.
func (f *Document) SetDrawColor(r, g, b int) {
	f.setDrawColor(r, g, b)
}

func (f *Document) setDrawColor(r, g, b int) {
	f.color.draw = rgbColorValue(r, g, b, "G", "RG")
	if f.page > 0 {
		f.out(f.color.draw.str)
	}
}

// GetDrawColor returns the most recently set draw color as RGB components
// (0-255). This will not be the current value if a draw color of some other type
// (for example, spot) has been more recently set.
func (f *Document) GetDrawColor() (int, int, int) {
	return f.color.draw.ir, f.color.draw.ig, f.color.draw.ib
}

// SetFillColor defines the color used for all filling operations (filled
// rectangles and cell backgrounds). It is expressed in RGB components (0-255).
// The method can be called before the first page is created and the
// value is retained from page to page.
func (f *Document) SetFillColor(r, g, b int) {
	f.setFillColor(r, g, b)
}

func (f *Document) setFillColor(r, g, b int) {
	f.color.fill = rgbColorValue(r, g, b, "g", "rg")
	f.colorFlag = f.color.fill.str != f.color.text.str
	if f.page > 0 {
		f.out(f.color.fill.str)
	}
}

// GetFillColor returns the most recently set fill color as RGB components
// (0-255). This will not be the current value if a fill color of some other type
// (for example, spot) has been more recently set.
func (f *Document) GetFillColor() (int, int, int) {
	return f.color.fill.ir, f.color.fill.ig, f.color.fill.ib
}

// SetTextColor defines the color used for text. It is expressed in RGB
// components (0-255). The method can be called before the first page is
// created. The value is retained from page to page.
func (f *Document) SetTextColor(r, g, b int) {
	f.setTextColor(r, g, b)
}

func (f *Document) setTextColor(r, g, b int) {
	f.color.text = rgbColorValue(r, g, b, "g", "rg")
	f.colorFlag = f.color.fill.str != f.color.text.str
}

// GetTextColor returns the most recently set text color as RGB components
// (0-255). This will not be the current value if a text color of some other type
// (for example, spot) has been more recently set.
func (f *Document) GetTextColor() (int, int, int) {
	return f.color.text.ir, f.color.text.ig, f.color.text.ib
}
