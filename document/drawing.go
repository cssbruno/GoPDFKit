// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strconv"
	"unicode/utf8"
)

const (
	stringWidthCacheLimit  = 512
	stringWidthCacheMaxLen = 256
)

// GetStringWidth returns the length of a string in user units. A font must be
// currently selected.
func (f *Document) GetStringWidth(s string) float64 {
	if f.err != nil {
		return 0
	}
	w := f.GetStringSymbolWidth(s)
	if f.err != nil {
		return 0
	}
	return float64(w) * f.fontSize / 1000
}

// GetStringSymbolWidth returns the length of a string in glyph units. A font
// must be currently selected.
func (f *Document) GetStringSymbolWidth(s string) int {
	if f.err != nil {
		return 0
	}
	if f.currentFont.Name == "" {
		f.SetErrorf("font must be selected before measuring text")
		return 0
	}
	if key, ok := f.stringWidthCacheKey(s); ok {
		if width, cached := f.stringWidthCache[key]; cached {
			return width
		}
		width := f.computeStringSymbolWidth(s)
		f.cacheStringSymbolWidth(key, width)
		return width
	}
	return f.computeStringSymbolWidth(s)
}

func (f *Document) computeStringSymbolWidth(s string) int {
	w := 0
	if f.isCurrentUTF8 {
		if isASCIIString(s) {
			for i := 0; i < len(s); i++ {
				w += f.currentFontRuneWidth(rune(s[i]))
			}
		} else {
			for _, char := range s {
				w += f.currentFontRuneWidth(char)
			}
		}
	} else {
		for i := 0; i < len(s); i++ {
			ch := s[i]
			if ch == 0 {
				break
			}
			w += f.currentFont.Cw[ch]
		}
	}
	return w
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func (f *Document) stringWidthCacheKey(s string) (string, bool) {
	if s == "" || len(s) > stringWidthCacheMaxLen || f.currentFont.i == "" {
		return "", false
	}
	mode := "8"
	if f.isCurrentUTF8 {
		mode = "u"
	}
	return mode + "\x00" + f.currentFont.i + "\x00" + s, true
}

func (f *Document) cacheStringSymbolWidth(key string, width int) {
	if f.stringWidthCache == nil {
		f.stringWidthCache = make(map[string]int, stringWidthCacheLimit)
	}
	if _, exists := f.stringWidthCache[key]; exists {
		f.stringWidthCache[key] = width
		return
	}
	if len(f.stringWidthCache) >= stringWidthCacheLimit {
		evict := f.stringWidthKeys[0]
		delete(f.stringWidthCache, evict)
		copy(f.stringWidthKeys, f.stringWidthKeys[1:])
		f.stringWidthKeys = f.stringWidthKeys[:len(f.stringWidthKeys)-1]
	}
	f.stringWidthCache[key] = width
	f.stringWidthKeys = append(f.stringWidthKeys, key)
}

func (f *Document) currentFontRuneWidth(char rune) int {
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
func (f *Document) SetLineWidth(width float64) {
	f.setLineWidth(width)
}

func (f *Document) setLineWidth(width float64) {
	f.lineWidth = width
	if f.page > 0 {
		f.outPDFLineWidth(width * f.k)
	}
}

// GetLineWidth returns the current line thickness.
func (f *Document) GetLineWidth() float64 {
	return f.lineWidth
}

// SetLineCapStyle defines the line cap style. styleStr should be "butt",
// "round" or "square". A square style projects from the end of the line. The
// method can be called before the first page is created. The value is
// retained from page to page.
func (f *Document) SetLineCapStyle(styleStr string) {
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
		f.outPDFIntOperator(f.capStyle, 'J')
	}
}

// SetLineJoinStyle defines the line join style. styleStr should be "miter",
// "round" or "bevel". The method can be called before the first page is
// created. The value is retained from page to page.
func (f *Document) SetLineJoinStyle(styleStr string) {
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
		f.outPDFIntOperator(f.joinStyle, 'j')
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
func (f *Document) SetDashPattern(dashArray []float64, dashPhase float64) {
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

func (f *Document) outputDashPattern() {
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
