// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"strings"
)

// getFontKey normalizes the font key used by AddFontFromReader and GetFontDesc.
func getFontKey(familyStr, styleStr string) string {
	familyStr = strings.ToLower(familyStr)
	styleStr = strings.ToUpper(styleStr)
	if styleStr == "IB" {
		styleStr = "BI"
	}
	return familyStr + styleStr
}

// GetFontDesc returns the font descriptor, which can be used, for example, to
// find the baseline of a font. If familyStr is empty, the current font
// descriptor is returned.
// See FontDescriptor for documentation about the font descriptor.
// See AddFont for details about familyStr and styleStr.
func (f *Document) GetFontDesc(familyStr, styleStr string) FontDescriptor {
	if familyStr == "" {
		return f.currentFont.Desc
	}
	return f.fonts[getFontKey(fontFamilyEscape(familyStr), styleStr)].Desc
}

// SetFont sets the font used to print character strings. It is mandatory to
// call this method at least once before printing text or the resulting
// document will not be valid.
//
// The font can be either a standard one or a font added with AddFont() or
// AddFontFromReader(). Standard fonts use Windows code page 1252 (Western
// Europe).
//
// The method can be called before the first page is created and the font is
// kept from page to page. If you just wish to change the current font size, it
// is simpler to call SetFontSize().
//
// Note: the font definition file must be accessible. An error is set if the
// file cannot be read.
//
// familyStr specifies the font family. It can be either a name defined by
// AddFont(), AddFontFromReader() or one of the standard families (case
// insensitive): "Courier" for fixed-width, "Helvetica" or "Arial" for sans
// serif, "Times" for serif, "Symbol" or "ZapfDingbats" for symbolic.
//
// styleStr can be "B" (bold), "I" (italic), "U" (underscore), "S" (strike-out)
// or any combination. The default value (specified with an empty string) is
// regular. Bold and italic styles do not apply to Symbol and ZapfDingbats.
//
// size is the font size measured in points. The default value is the current
// size. If no size has been specified since the beginning of the document, this
// uses 12.
func (f *Document) SetFont(familyStr, styleStr string, size float64) {
	if f.err != nil {
		return
	}
	familyStr = fontFamilyEscape(familyStr)
	var ok bool
	if familyStr == "" {
		familyStr = f.fontFamily
	} else {
		familyStr = strings.ToLower(familyStr)
	}
	styleStr = strings.ToUpper(styleStr)
	f.underline = strings.Contains(styleStr, "U")
	if f.underline {
		styleStr = strings.ReplaceAll(styleStr, "U", "")
	}
	f.strikeout = strings.Contains(styleStr, "S")
	if f.strikeout {
		styleStr = strings.ReplaceAll(styleStr, "S", "")
	}
	if styleStr == "IB" {
		styleStr = "BI"
	}
	if size == 0.0 {
		size = f.fontSizePt
	}
	fontKey := familyStr + styleStr
	_, ok = f.fonts[fontKey]
	if !ok {
		if familyStr == "arial" {
			familyStr = "helvetica"
		}
		_, ok = f.coreFonts[familyStr]
		if ok {
			if familyStr == "symbol" {
				familyStr = "zapfdingbats"
			}
			if familyStr == "zapfdingbats" {
				styleStr = ""
			}
			fontKey = familyStr + styleStr
			_, ok = f.fonts[fontKey]
			if !ok {
				def, err := loadCoreFontDef(fontKey)
				if err != nil {
					f.err = err
					return
				}
				f.fonts[fontKey] = def
			}
		} else {
			f.err = fmt.Errorf("undefined font: %s %s", familyStr, styleStr)
			return
		}
	}
	f.fontFamily = familyStr
	f.fontStyle = styleStr
	f.fontSizePt = size
	f.fontSize = size / f.k
	f.currentFont = f.fonts[fontKey]
	if f.currentFont.Tp == "UTF8" {
		f.isCurrentUTF8 = true
	} else {
		f.isCurrentUTF8 = false
	}
	if f.page > 0 {
		f.outPDFFontSelect()
	}
}

// SetFontStyle sets the style of the current font. See also SetFont().
func (f *Document) SetFontStyle(styleStr string) {
	f.SetFont(f.fontFamily, styleStr, f.fontSizePt)
}

// SetFontSize defines the size of the current font. Size is specified in
// points (1/72 inch). See also SetFontUnitSize().
func (f *Document) SetFontSize(size float64) {
	f.fontSizePt = size
	f.fontSize = size / f.k
	if f.page > 0 && f.currentFont.Name != "" {
		f.outPDFFontSelect()
	}
}

// SetFontUnitSize defines the size of the current font. Size is specified in
// the unit of measure specified in New(). See also SetFontSize().
func (f *Document) SetFontUnitSize(size float64) {
	f.fontSizePt = size * f.k
	f.fontSize = size
	if f.page > 0 && f.currentFont.Name != "" {
		f.outPDFFontSelect()
	}
}

// GetFontSize returns the size of the current font in points followed by the
// size in the unit of measure specified in New(). The second value can be used
// as a line height value in drawing operations.
func (f *Document) GetFontSize() (ptSize, unitSize float64) {
	return f.fontSizePt, f.fontSize
}
