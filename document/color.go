// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
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
			buf := make([]byte, 0, len(grayStr)+8)
			buf = appendPDFNumberSpace(buf, clr.r, 3)
			buf = append(buf, grayStr...)
			clr.str = string(buf)
		} else {
			buf := make([]byte, 0, len(fullStr)+22)
			buf = appendPDFNumberSpace(buf, clr.r, 3)
			buf = appendPDFNumberSpace(buf, clr.g, 3)
			buf = appendPDFNumberSpace(buf, clr.b, 3)
			buf = append(buf, fullStr...)
			clr.str = string(buf)
		}
	} else {
		buf := make([]byte, 0, 20)
		buf = appendPDFNumberSpace(buf, clr.r, 3)
		buf = appendPDFNumberSpace(buf, clr.g, 3)
		buf = appendPDFNumber(buf, clr.b, 3)
		clr.str = string(buf)
	}
	return
}

func sameRGBColor(clr pdfColor, r, g, b int) bool {
	ir, _ := colorComp(r)
	ig, _ := colorComp(g)
	ib, _ := colorComp(b)
	return clr.str != "" && clr.mode == colorModeRGB && clr.ir == ir && clr.ig == ig && clr.ib == ib
}

// SetDrawColor defines the color used for all drawing operations (lines,
// rectangles and cell borders). It is expressed in RGB components (0-255).
// The method can be called before the first page is created. The value is
// retained from page to page.
func (f *Document) SetDrawColor(r, g, b int) {
	f.setDrawColor(r, g, b)
}

func (f *Document) setDrawColor(r, g, b int) {
	if !sameRGBColor(f.color.draw, r, g, b) {
		f.color.draw = rgbColorValue(r, g, b, "G", "RG")
	}
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
	if !sameRGBColor(f.color.fill, r, g, b) {
		f.color.fill = rgbColorValue(r, g, b, "g", "rg")
	}
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
	if !sameRGBColor(f.color.text, r, g, b) {
		f.color.text = rgbColorValue(r, g, b, "g", "rg")
	}
	f.colorFlag = f.color.fill.str != f.color.text.str
}

// GetTextColor returns the most recently set text color as RGB components
// (0-255). This will not be the current value if a text color of some other type
// (for example, spot) has been more recently set.
func (f *Document) GetTextColor() (int, int, int) {
	return f.color.text.ir, f.color.text.ig, f.color.text.ib
}
