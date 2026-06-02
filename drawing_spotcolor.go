/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"fmt"
	"strings"
)

func clampSpotColorPercent(percent byte) byte {
	const maxPercent = 100
	if percent > maxPercent {
		return maxPercent
	}
	return percent
}

// AddSpotColor adds an ink-based CMYK color to the gopdfkit instance and
// associates it with the specified name. The individual components specify
// percentages ranging from 0 to 100. Values above this are quietly capped to
// 100. An error occurs if the specified name is already associated with a
// color.
func (f *Fpdf) AddSpotColor(name string, cyan, magenta, yellow, black byte) {
	if f.err != nil {
		return
	}
	if _, exists := f.spotColorMap[name]; exists {
		f.err = fmt.Errorf("name \"%s\" is already associated with a spot color", name)
		return
	}
	f.spotColorMap[name] = spotColorType{
		id: len(f.spotColorMap) + 1,
		cmyk: cmykColorType{
			c: clampSpotColorPercent(cyan),
			m: clampSpotColorPercent(magenta),
			y: clampSpotColorPercent(yellow),
			k: clampSpotColorPercent(black),
		},
	}
}

func (f *Fpdf) lookupSpotColor(name string) (spotColorType, bool) {
	if f.err != nil {
		return spotColorType{}, false
	}
	spotColor, exists := f.spotColorMap[name]
	if !exists {
		f.err = fmt.Errorf("spot color name \"%s\" is not registered", name)
		return spotColorType{}, false
	}
	return spotColor, true
}

// SetDrawSpotColor sets the current draw color to the spot color associated
// with name. An error occurs if the name is not associated with a color.
// The value for tint ranges from 0 (no intensity) to 100 (full intensity). It
// is quietly bounded to this range.
func (f *Fpdf) SetDrawSpotColor(name string, tint byte) {
	spotColor, ok := f.lookupSpotColor(name)
	if !ok {
		return
	}
	f.color.draw.mode = colorModeSpot
	f.color.draw.spotStr = name
	f.color.draw.str = spotColorDrawCommand(spotColor.id, tint)
	if f.page > 0 {
		f.out(f.color.draw.str)
	}
}

// SetFillSpotColor sets the current fill color to the spot color associated
// with name. An error occurs if the name is not associated with a color.
// The value for tint ranges from 0 (no intensity) to 100 (full intensity). It
// is quietly bounded to this range.
func (f *Fpdf) SetFillSpotColor(name string, tint byte) {
	spotColor, ok := f.lookupSpotColor(name)
	if !ok {
		return
	}
	f.color.fill.mode = colorModeSpot
	f.color.fill.spotStr = name
	f.color.fill.str = spotColorFillCommand(spotColor.id, tint)
	f.colorFlag = f.color.fill.str != f.color.text.str
	if f.page > 0 {
		f.out(f.color.fill.str)
	}
}

// SetTextSpotColor sets the current text color to the spot color associated
// with name. An error occurs if the name is not associated with a color.
// The value for tint ranges from 0 (no intensity) to 100 (full intensity). It
// is quietly bounded to this range.
func (f *Fpdf) SetTextSpotColor(name string, tint byte) {
	spotColor, ok := f.lookupSpotColor(name)
	if !ok {
		return
	}
	f.color.text.mode = colorModeSpot
	f.color.text.spotStr = name
	f.color.text.str = spotColorFillCommand(spotColor.id, tint)
	f.colorFlag = f.color.fill.str != f.color.text.str
}

func spotColorDrawCommand(colorID int, tint byte) string {
	return sprintf("/CS%d CS %.3f SCN", colorID, float64(clampSpotColorPercent(tint))/100)
}

func spotColorFillCommand(colorID int, tint byte) string {
	return sprintf("/CS%d cs %.3f scn", colorID, float64(clampSpotColorPercent(tint))/100)
}

func (f *Fpdf) currentSpotColorComponents(color pdfColor) (name string, cyan, magenta, yellow, black byte) {
	name = color.spotStr
	if name == "" {
		return
	}
	spotColor, ok := f.lookupSpotColor(name)
	if !ok {
		return
	}
	cyan = spotColor.cmyk.c
	magenta = spotColor.cmyk.m
	yellow = spotColor.cmyk.y
	black = spotColor.cmyk.k
	return
}

// GetDrawSpotColor returns the most recently used spot color information for
// drawing. This will not be the current drawing color if some other color type
// such as RGB is active. If no spot color has been set for drawing, zero
// values are returned.
func (f *Fpdf) GetDrawSpotColor() (name string, c, m, y, k byte) {
	return f.currentSpotColorComponents(f.color.draw)
}

// GetTextSpotColor returns the most recently used spot color information for
// text output. This will not be the current text color if some other color
// type such as RGB is active. If no spot color has been set for text, zero
// values are returned.
func (f *Fpdf) GetTextSpotColor() (name string, c, m, y, k byte) {
	return f.currentSpotColorComponents(f.color.text)
}

// GetFillSpotColor returns the most recently used spot color information for
// fill output. This will not be the current fill color if some other color
// type such as RGB is active. If no fill spot color has been set, zero values
// are returned.
func (f *Fpdf) GetFillSpotColor() (name string, c, m, y, k byte) {
	return f.currentSpotColorComponents(f.color.fill)
}

func (f *Fpdf) putSpotColors() {
	for name, spotColor := range f.spotColorMap {
		f.newobj()
		f.outf("[/Separation /%s", strings.Replace(name, " ", "#20", -1))
		f.out("/DeviceCMYK <<")
		f.out("/Range [0 1 0 1 0 1 0 1] /C0 [0 0 0 0] ")
		f.outf("/C1 [%.3f %.3f %.3f %.3f] ", float64(spotColor.cmyk.c)/100, float64(spotColor.cmyk.m)/100,
			float64(spotColor.cmyk.y)/100, float64(spotColor.cmyk.k)/100)
		f.out("/FunctionType 2 /Domain [0 1] /N 1>>]")
		f.out("endobj")
		spotColor.objID = f.n
		f.spotColorMap[name] = spotColor
	}
}

func (f *Fpdf) putSpotColorResourceDict() {
	f.out("/ColorSpace <<")
	for _, spotColor := range f.spotColorMap {
		f.outf("/CS%d %d 0 R", spotColor.id, spotColor.objID)
	}
	f.out(">>")
}
