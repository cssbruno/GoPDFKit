// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"strings"
	"unicode/utf8"
)

// Text prints a character string. The origin (x, y) is at the left edge of the
// first character's baseline. This method allows a string to be placed
// precisely on the page, but it is usually easier to use Cell(), MultiCell()
// or Write(), which are the standard methods for printing text.
func (f *Document) Text(x, y float64, txtStr string) {
	tag := f.consumeNextTextTag(false)
	var txt2 string
	if f.isCurrentUTF8 {
		if f.isRTL {
			txtStr = reverseText(txtStr)
			x -= f.GetStringWidth(txtStr)
		}
		txt2 = f.escape(utf8toutf16(txtStr, false))
		for _, uni := range txtStr {
			f.currentFont.usedRunes[int(uni)] = int(uni)
		}
	} else {
		txt2 = f.escape(txtStr)
	}
	buf := make([]byte, 0, len(txt2)+len(f.color.text.str)+96)
	if f.colorFlag {
		buf = append(buf, "q "...)
		buf = append(buf, f.color.text.str...)
		buf = append(buf, ' ')
	}
	buf = append(buf, "BT "...)
	buf = appendPDFNumberSpace(buf, x*f.k, 2)
	buf = appendPDFNumberSpace(buf, (f.h-y)*f.k, 2)
	buf = append(buf, "Td ("...)
	buf = append(buf, txt2...)
	buf = append(buf, ") Tj ET"...)
	if f.underline && txtStr != "" {
		buf = append(buf, ' ')
		buf = f.appendUnderlineRect(buf, x, y, txtStr)
	}
	if f.strikeout && txtStr != "" {
		buf = append(buf, ' ')
		buf = f.appendStrikeoutRect(buf, x, y, txtStr)
	}
	if f.colorFlag {
		buf = append(buf, " Q"...)
	}
	f.outbytes(f.wrapTaggedContent(buf, tag))
}

// SetWordSpacing sets spacing between words of following text. See the
// WriteAligned() example for a demonstration of its use.
func (f *Document) SetWordSpacing(space float64) {
	var scratch [32]byte
	buf := appendPDFNumber(scratch[:0], space*f.k, 5)
	buf = append(buf, " Tw"...)
	f.outbytes(buf)
}

// SetTextRenderingMode sets the rendering mode of following text.
// The mode can be as follows:
// 0: Fill text
// 1: Stroke text
// 2: Fill, then stroke text
// 3: Neither fill nor stroke text (invisible)
// 4: Fill text and add to path for clipping
// 5: Stroke text and add to path for clipping
// 6: Fill, then stroke text and add to path for clipping
// 7: Add text to path for clipping
// This method is demonstrated in the SetTextRenderingMode example.
func (f *Document) SetTextRenderingMode(mode int) {
	if mode >= 0 && mode <= 7 {
		var scratch [16]byte
		buf := appendPDFInt(scratch[:0], mode)
		buf = append(buf, " Tr"...)
		f.outbytes(buf)
	}
}

func (f *Document) write(h float64, txtStr string, link int, linkStr string) {
	cw := f.currentFont.Cw
	w := f.w - f.rMargin - f.x
	wmax := (w - 2*f.cMargin) * 1000 / f.fontSize
	s := strings.ReplaceAll(txtStr, "\r", "")
	if f.isCurrentUTF8 {
		if s == " " {
			f.x += f.GetStringWidth(s)
			return
		}
		f.writeUTF8(h, s, link, linkStr, w, wmax)
		return
	}
	nb := len(s)
	sep := -1
	i := 0
	j := 0
	l := 0.0
	nl := 1
	for i < nb {
		c := rune(s[i])
		if c == '\n' {
			f.CellFormat(w, h, s[j:i], "", 2, "", false, link, linkStr)
			i++
			sep = -1
			j = i
			l = 0.0
			if nl == 1 {
				f.x = f.lMargin
				w = f.w - f.rMargin - f.x
				wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
			}
			nl++
			continue
		}
		if c == ' ' {
			sep = i
		}
		l += float64(cw[int(c)])
		if l > wmax {
			if sep == -1 {
				if f.x > f.lMargin {
					f.x = f.lMargin
					f.y += h
					w = f.w - f.rMargin - f.x
					wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
					i++
					nl++
					continue
				}
				if i == j {
					i++
				}
				f.CellFormat(w, h, s[j:i], "", 2, "", false, link, linkStr)
			} else {
				f.CellFormat(w, h, s[j:sep], "", 2, "", false, link, linkStr)
				i = sep + 1
			}
			sep = -1
			j = i
			l = 0.0
			if nl == 1 {
				f.x = f.lMargin
				w = f.w - f.rMargin - f.x
				wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
			}
			nl++
		} else {
			i++
		}
	}
	if i != j {
		f.CellFormat(l/1000*f.fontSize, h, s[j:], "", 0, "", false, link, linkStr)
	}
}

func (f *Document) writeUTF8(h float64, s string, link int, linkStr string, w, wmax float64) {
	sep := -1
	i := 0
	j := 0
	l := 0.0
	nl := 1
	for i < len(s) {
		c, size := utf8.DecodeRuneInString(s[i:])
		if size <= 0 {
			break
		}
		next := i + size
		if c == '\n' {
			f.CellFormat(w, h, s[j:i], "", 2, "", false, link, linkStr)
			i = next
			sep = -1
			j = i
			l = 0.0
			if nl == 1 {
				f.x = f.lMargin
				w = f.w - f.rMargin - f.x
				wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
			}
			nl++
			continue
		}
		if c == ' ' {
			sep = i
		}
		l += float64(f.currentFontRuneWidth(c))
		if l > wmax {
			if sep == -1 {
				if f.x > f.lMargin {
					f.x = f.lMargin
					f.y += h
					w = f.w - f.rMargin - f.x
					wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
					i = next
					nl++
					continue
				}
				if i == j {
					i = next
				}
				f.CellFormat(w, h, s[j:i], "", 2, "", false, link, linkStr)
			} else {
				f.CellFormat(w, h, s[j:sep], "", 2, "", false, link, linkStr)
				i = sep + 1
			}
			sep = -1
			j = i
			l = 0.0
			if nl == 1 {
				f.x = f.lMargin
				w = f.w - f.rMargin - f.x
				wmax = (w - 2*f.cMargin) * 1000 / f.fontSize
			}
			nl++
		} else {
			i = next
		}
	}
	if i != j {
		f.CellFormat(l/1000*f.fontSize, h, s[j:], "", 0, "", false, link, linkStr)
	}
}

// Write prints text from the current position. When the right margin is
// reached (or the \n character is encountered), a line break occurs and text
// continues from the left margin. When the method returns, the current position
// is just after the end of the text.
//
// It is possible to put a link on the text.
//
// h indicates the line height in the unit of measure specified in New().
func (f *Document) Write(h float64, txtStr string) {
	f.write(h, txtStr, 0, "")
}

// Writef is like Write but uses printf-style formatting. See the documentation
// for package fmt for more details on fmtStr and args.
func (f *Document) Writef(h float64, fmtStr string, args ...any) {
	f.write(h, sprintf(fmtStr, args...), 0, "")
}

// WriteLinkString writes text that when clicked launches an external URL. See
// Write() for argument details.
func (f *Document) WriteLinkString(h float64, displayStr, targetStr string) {
	f.write(h, displayStr, 0, targetStr)
}

// WriteLinkID writes text that when clicked jumps to another location in the
// PDF. linkID is an identifier returned by AddLink(). See Write() for argument
// details.
func (f *Document) WriteLinkID(h float64, displayStr string, linkID int) {
	f.write(h, displayStr, linkID, "")
}

// WriteAligned is an implementation of Write that makes it possible to align
// text.
//
// width indicates the width of the box the text will be drawn in. This is in
// the unit of measure specified in New(). If it is set to 0, the bounding box
// of the page is used (pageWidth - leftMargin - rightMargin).
//
// lineHeight indicates the line height in the unit of measure specified in
// New().
//
// alignStr sets horizontal alignment of the given textStr. The options are
// "L", "C" and "R" (Left, Center, Right). The default is "L".
func (f *Document) WriteAligned(width, lineHeight float64, textStr, alignStr string) {
	lMargin, _, rMargin, _ := f.GetMargins()
	pageWidth, _ := f.GetPageSize()
	if width == 0 {
		width = pageWidth - (lMargin + rMargin)
	}
	var lines []string
	if f.isCurrentUTF8 {
		lines = f.SplitText(textStr, width)
	} else {
		for _, line := range f.SplitLines([]byte(textStr), width) {
			lines = append(lines, string(line))
		}
	}
	for _, lineStr := range lines {
		lineWidth := f.GetStringWidth(lineStr)
		switch alignStr {
		case "C":
			f.SetLeftMargin(lMargin + ((width - lineWidth) / 2))
			f.Write(lineHeight, lineStr)
			f.SetLeftMargin(lMargin)
		case "R":
			f.SetLeftMargin(lMargin + (width - lineWidth) - 2.01*f.cMargin)
			f.Write(lineHeight, lineStr)
			f.SetLeftMargin(lMargin)
		default:
			f.SetRightMargin(pageWidth - lMargin - width)
			f.Write(lineHeight, lineStr)
			f.SetRightMargin(rMargin)
		}
	}
}

// Ln performs a line break. The current abscissa goes back to the left margin
// and the ordinate increases by the amount passed in parameter. A negative
// value of h indicates the height of the last printed cell.
//
// This method is demonstrated in the example for MultiCell.
func (f *Document) Ln(h float64) {
	f.x = f.lMargin
	if h < 0 {
		f.y += f.lasth
	} else {
		f.y += h
	}
}

// Escape special characters in strings
func (f *Document) escape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

func (f *Document) textstring(s string) string {
	if f.protect.encrypted {
		b := []byte( // textstring formats a text string
			s)
		f.protect.rc4(uint32(f.n), &b)
		s = string(b)
	}
	return "(" + f.escape(s) + ")"
}

func blankCount(str string) (count int) {
	l := len(str)
	for j := range l {
		if byte(' ') == str[j] {
			count++
		}
	}
	return
}

// SetUnderlineThickness accepts a multiplier for adjusting the text underline
// thickness, defaulting to 1. See SetUnderlineThickness example.
func (f *Document) SetUnderlineThickness(thickness float64) {
	f.userUnderlineThickness = thickness
}

func (f *Document) appendUnderlineRect(buf []byte, x, y float64, txt string) []byte {
	up := float64(f.currentFont.Up)
	ut := float64(f.currentFont.Ut) * f.userUnderlineThickness
	w := f.GetStringWidth(txt) + f.ws*float64(blankCount(txt))
	return appendPDFRectPaint(buf, x*f.k, (f.h-(y-up/1000*f.fontSize))*f.k, w*f.k, -ut/1000*f.fontSizePt, "f", false)
}

func (f *Document) appendStrikeoutRect(buf []byte, x, y float64, txt string) []byte {
	up := float64(f.currentFont.Up)
	ut := float64(f.currentFont.Ut)
	w := f.GetStringWidth(txt) + f.ws*float64(blankCount(txt))
	return appendPDFRectPaint(buf, x*f.k, (f.h-(y+4*up/1000*f.fontSize))*f.k, w*f.k, -ut/1000*f.fontSizePt, "f", false)
}
