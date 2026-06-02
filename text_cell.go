// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"fmt"
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
func (f *Fpdf) SetAcceptPageBreakFunc(fnc func() bool) {
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
func (f *Fpdf) CellFormat(w, h float64, txtStr, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string) {
	if f.err != nil {
		return
	}
	if f.currentFont.Name == "" {
		f.err = fmt.Errorf("font has not been set; unable to render text")
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
	var s fmtBuffer
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
		s.printf("%.2f %.2f %.2f %.2f re %s ", f.x*k, (f.h-f.y)*k, w*k, -h*k, op)
	}
	if len(borderStr) > 0 && borderStr != "1" {
		x := f.x
		y := f.y
		left := x * k
		top := (f.h - y) * k
		right := (x + w) * k
		bottom := (f.h - (y + h)) * k
		if strings.Contains(borderStr, "L") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", left, top, left, bottom)
		}
		if strings.Contains(borderStr, "T") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", left, top, right, top)
		}
		if strings.Contains(borderStr, "R") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", right, top, right, bottom)
		}
		if strings.Contains(borderStr, "B") {
			s.printf("%.2f %.2f m %.2f %.2f l S ", left, bottom, right, bottom)
		}
	}
	if len(txtStr) > 0 {
		var dx, dy float64
		switch {
		case strings.Contains(alignStr, "R"):
			dx = w - f.cMargin - f.GetStringWidth(txtStr)
		case strings.Contains(alignStr, "C"):
			dx = (w - f.GetStringWidth(txtStr)) / 2
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
			s.printf("q %s ", f.color.text.str)
		}
		if (f.ws != 0 || alignStr == "J") && f.isCurrentUTF8 {
			if f.isRTL {
				txtStr = reverseText(txtStr)
			}
			wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
			for _, uni := range txtStr {
				f.currentFont.usedRunes[int(uni)] = int(uni)
			}
			space := f.escape(utf8toutf16(" ", false))
			strSize := f.GetStringSymbolWidth(txtStr)
			s.printf("BT 0 Tw %.2f %.2f Td [", (f.x+dx)*k, (f.h-(f.y+.5*h+.3*f.fontSize))*k)
			t := strings.Split(txtStr, " ")
			shift := float64((wmax - strSize)) / float64(len(t)-1)
			numt := len(t)
			for i := range numt {
				tx := t[i]
				tx = "(" + f.escape(utf8toutf16(tx, false)) + ")"
				s.printf("%s ", tx)
				if (i + 1) < numt {
					s.printf("%.3f(%s) ", -shift, space)
				}
			}
			s.printf("] TJ ET")
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
			} else {
				txt2 = strings.ReplaceAll(txtStr, "\\", "\\\\")
				txt2 = strings.ReplaceAll(txt2, "(", "\\(")
				txt2 = strings.ReplaceAll(txt2, ")", "\\)")
			}
			bt := (f.x + dx) * k
			td := (f.h - (f.y + dy + .5*h + .3*f.fontSize)) * k
			s.printf("BT %.2f %.2f Td (%s)Tj ET", bt, td, txt2)
		}
		if f.underline {
			s.printf(" %s", f.dounderline(f.x+dx, f.y+dy+.5*h+.3*f.fontSize, txtStr))
		}
		if f.strikeout {
			s.printf(" %s", f.dostrikeout(f.x+dx, f.y+dy+.5*h+.3*f.fontSize, txtStr))
		}
		if f.colorFlag {
			s.printf(" Q")
		}
		if link > 0 || len(linkStr) > 0 {
			f.newLink(f.x+dx, f.y+dy+.5*h-.5*f.fontSize, f.GetStringWidth(txtStr), f.fontSize, link, linkStr)
		}
	}
	str := s.String()
	if len(str) > 0 {
		f.out(str)
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
func (f *Fpdf) Cell(w, h float64, txtStr string) {
	f.CellFormat(w, h, txtStr, "", 0, "L", false, 0, "")
}

// Cellf is a simpler printf-style version of CellFormat with no fill, border,
// links or special alignment. See documentation for the fmt package for
// details on fmtStr and args.
func (f *Fpdf) Cellf(w, h float64, fmtStr string, args ...any) {
	f.CellFormat(w, h, sprintf(fmtStr, args...), "", 0, "L", false, 0, "")
}
