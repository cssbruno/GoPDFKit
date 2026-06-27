// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"math"
	"strings"
)

// SetAcceptPageBreakFunc allows the application to control where page breaks
// occur.
//
// fnc is an application function (typically a closure) called by the library
// whenever a page break condition is met. The break is issued if fnc returns
// true. The default implementation returns a value according to the
// mode selected by SetAutoPageBreak. The function provided should not be
// called by the application.
//
// See the example for SetLeftMargin() to see how this function can be used to
// manage multiple columns.
func (f *Document) SetAcceptPageBreakFunc(fnc func() bool) {
	f.acceptPageBreak = fnc
}

// CellFormat prints a rectangular cell with optional borders, background color
// and character string. The upper-left corner of the cell corresponds to the
// current position. The text can be aligned or centered. After the call, the
// current position moves to the right or to the next line. It is possible to
// put a link on the text.
//
// An error will be returned if a call to SetFont() has not already taken place
// before this method is called.
//
// If automatic page breaking is enabled and the cell goes beyond the limit, a
// page break is performed before output.
//
// w and h specify the width and height of the cell. If w is 0, the cell
// extends up to the right margin. Specifying 0 for h will result in no output,
// but the current position will be advanced by w.
//
// txtStr specifies the text to display.
//
// borderStr specifies how the cell border will be drawn. An empty string
// indicates no border, "1" indicates a full border, and one or more of "L",
// "T", "R" and "B" indicate the left, top, right and bottom sides of the
// border.
//
// ln indicates where the current position should go after the call. Possible
// values are 0 (to the right), 1 (to the beginning of the next line), and 2
// (below). Using 1 is equivalent to using 0 and calling Ln() immediately after.
//
// alignStr specifies how the text is to be positioned within the cell.
// Horizontal alignment is controlled by including "L", "C" or "R" (left,
// center, right) in alignStr. Vertical alignment is controlled by including
// "T", "M", "B" or "A" (top, middle, bottom, baseline) in alignStr. The default
// alignment is left middle.
//
// fill is true to paint the cell background or false to leave it transparent.
//
// link is the identifier returned by AddLink() or 0 for no internal link.
//
// linkStr is a target URL or empty for no external link. A non-zero value for
// link takes precedence over linkStr.
func (f *Document) CellFormat(w, h float64, txtStr, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string) {
	if f.err != nil {
		return
	}
	if f.currentFont.Name == "" {
		f.err = errors.New("font has not been set; unable to render text")
		return
	}
	borderStr = strings.ToUpper(borderStr)
	k := f.k
	if f.y+h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
		x := f.x
		ws := f.ws
		if ws > 0 {
			f.ws = 0
			f.out("0 Tw")
		}
		f.addPageFormatRotation(f.curOrientation, f.curPageSize, f.curRotation)
		if f.err != nil {
			return
		}
		f.x = x
		if ws > 0 {
			f.ws = ws
			f.outf("%.3f Tw", ws*k)
		}
	}
	if w == 0 {
		w = f.w - f.rMargin - f.x
	}
	var s []byte
	if fill || borderStr == "1" {
		var op string
		if fill {
			if borderStr == "1" {
				op = "B"
			} else {
				op = "f"
			}
		} else {
			op = "S"
		}
		s = ensurePDFBuffer(s, 128+len(txtStr))
		s = appendPDFRectPaint(s, f.x*k, (f.h-f.y)*k, w*k, -h*k, op, true)
	}
	if len(borderStr) > 0 && borderStr != "1" {
		s = ensurePDFBuffer(s, 128+len(txtStr))
		x := f.x
		y := f.y
		left := x * k
		top := (f.h - y) * k
		right := (x + w) * k
		bottom := (f.h - (y + h)) * k
		if strings.Contains(borderStr, "L") {
			s = appendPDFLine(s, left, top, left, bottom, 2, true)
		}
		if strings.Contains(borderStr, "T") {
			s = appendPDFLine(s, left, top, right, top, 2, true)
		}
		if strings.Contains(borderStr, "R") {
			s = appendPDFLine(s, right, top, right, bottom, 2, true)
		}
		if strings.Contains(borderStr, "B") {
			s = appendPDFLine(s, left, bottom, right, bottom, 2, true)
		}
	}
	if len(txtStr) > 0 {
		s = ensurePDFBuffer(s, 128+len(txtStr)*2+len(f.color.text.str))
		var dx, dy float64
		var textWidth float64
		textWidthSet := false
		getTextWidth := func() float64 {
			if !textWidthSet {
				textWidth = f.GetStringWidth(txtStr)
				textWidthSet = true
			}
			return textWidth
		}
		switch {
		case strings.Contains(alignStr, "R"):
			dx = w - f.cMargin - getTextWidth()
		case strings.Contains(alignStr, "C"):
			dx = (w - getTextWidth()) / 2
		default:
			dx = f.cMargin
		}
		switch {
		case strings.Contains(alignStr, "T"):
			dy = (f.fontSize - h) / 2.0
		case strings.Contains(alignStr, "B"):
			dy = (h - f.fontSize) / 2.0
		case strings.Contains(alignStr, "A"):
			var descent float64
			d := f.currentFont.Desc
			if d.Descent == 0 {
				descent = -0.19 * f.fontSize
			} else {
				descent = float64(d.Descent) * f.fontSize / float64(d.Ascent-d.Descent)
			}
			dy = (h-f.fontSize)/2.0 - descent
		default:
			dy = 0
		}
		if f.colorFlag {
			s = append(s, "q "...)
			s = append(s, f.color.text.str...)
			s = append(s, ' ')
		}
		utf8Justify := (f.ws != 0 || alignStr == "J") && f.isCurrentUTF8
		if utf8Justify && len(strings.Fields(txtStr)) > 1 {
			if f.isRTL {
				txtStr = reverseText(txtStr)
			}
			wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
			for _, uni := range txtStr {
				f.currentFont.usedRunes[int(uni)] = int(uni)
			}
			space := f.escape(utf8toutf16(" ", false))
			strSize := f.GetStringSymbolWidth(txtStr)
			s = append(s, "BT 0 Tw "...)
			s = appendPDFNumberSpace(s, (f.x+dx)*k, 2)
			s = appendPDFNumberSpace(s, (f.h-(f.y+.5*h+.3*f.fontSize))*k, 2)
			s = append(s, "Td ["...)
			t := strings.Split(txtStr, " ")
			shift := float64((wmax - strSize)) / float64(len(t)-1)
			numt := len(t)
			for i := range numt {
				tx := t[i]
				s = append(s, '(')
				s = append(s, f.escape(utf8toutf16(tx, false))...)
				s = append(s, ") "...)
				if (i + 1) < numt {
					s = appendPDFNumber(s, -shift, 3)
					s = append(s, '(')
					s = append(s, space...)
					s = append(s, ") "...)
				}
			}
			s = append(s, "] TJ ET"...)
		} else {
			var txt2 string
			if f.isCurrentUTF8 {
				if f.isRTL {
					txtStr = reverseText(txtStr)
				}
				txt2 = f.escape(utf8toutf16(txtStr, false))
				for _, uni := range txtStr {
					f.currentFont.usedRunes[int(uni)] = int(uni)
				}
			}
			bt := (f.x + dx) * k
			td := (f.h - (f.y + dy + .5*h + .3*f.fontSize)) * k
			s = append(s, "BT "...)
			s = appendPDFNumberSpace(s, bt, 2)
			s = appendPDFNumberSpace(s, td, 2)
			s = append(s, "Td ("...)
			if f.isCurrentUTF8 {
				s = append(s, txt2...)
			} else {
				s = appendEscapedPDFCellText(s, txtStr)
			}
			s = append(s, ")Tj ET"...)
		}
		if f.underline {
			s = append(s, ' ')
			s = f.appendUnderlineRect(s, f.x+dx, f.y+dy+.5*h+.3*f.fontSize, txtStr)
		}
		if f.strikeout {
			s = append(s, ' ')
			s = f.appendStrikeoutRect(s, f.x+dx, f.y+dy+.5*h+.3*f.fontSize, txtStr)
		}
		if f.colorFlag {
			s = append(s, " Q"...)
		}
		if link > 0 || len(linkStr) > 0 {
			f.newLink(f.x+dx, f.y+dy+.5*h-.5*f.fontSize, getTextWidth(), f.fontSize, link, linkStr)
		}
	}
	if len(s) > 0 {
		f.outbytes(f.wrapTaggedContent(s, f.consumeNextTextTag(link > 0 || linkStr != "")))
	}
	f.lasth = h
	if ln > 0 {
		f.y += h
		if ln == 1 {
			f.x = f.lMargin
		}
	} else {
		f.x += w
	}
}

func appendEscapedPDFCellText(buf []byte, text string) []byte {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\\', '(', ')':
			buf = append(buf, '\\', text[i])
		default:
			buf = append(buf, text[i])
		}
	}
	return buf
}

func reverseText(text string) string {
	oldText := []rune( // Reverse string to use in RTL languages.
		text)
	newText := make([]rune, len(oldText))
	length := len(oldText) - 1
	for i, r := range oldText {
		newText[length-i] = r
	}
	return string(newText)
}

// Cell is a simpler version of CellFormat with no fill, border, links or
// special alignment. The Cell_strikeout() example demonstrates this method.
func (f *Document) Cell(w, h float64, txtStr string) {
	f.CellFormat(w, h, txtStr, "", 0, "L", false, 0, "")
}

// Cellf is a simpler printf-style version of CellFormat with no fill, border,
// links or special alignment. See documentation for the fmt package for
// details on fmtStr and args.
func (f *Document) Cellf(w, h float64, fmtStr string, args ...any) {
	f.CellFormat(w, h, sprintf(fmtStr, args...), "", 0, "L", false, 0, "")
}
