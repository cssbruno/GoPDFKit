// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"math"
	"strings"
	"unicode"
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
	f.acceptPageBreakSet = true
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
	switch borderStr {
	case "", "1":
	default:
		borderStr = strings.ToUpper(borderStr)
	}
	alignRight := strings.Contains(alignStr, "R")
	alignCenter := strings.Contains(alignStr, "C")
	alignTop := strings.Contains(alignStr, "T")
	alignBottom := strings.Contains(alignStr, "B")
	alignBaseline := strings.Contains(alignStr, "A")
	k := f.k
	if f.y+h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
		x := f.x
		ws := f.ws
		if ws != 0 {
			f.SetWordSpacing(0)
		}
		f.addPageFormatRotation(f.curOrientation, f.curPageSize, f.curRotation)
		if f.err != nil {
			return
		}
		f.x = x
		if ws != 0 {
			f.SetWordSpacing(ws)
		}
	}
	if w == 0 {
		w = f.w - f.rMargin - f.x
	}
	var s []byte
	if capacity := f.estimateCellFormatBufferCapacity(txtStr, borderStr, fill); capacity > 0 {
		s = f.contentCommandBuffer(capacity)
	}
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
		s = appendPDFRectPaint(s, f.x*k, (f.h-f.y)*k, w*k, -h*k, op, true)
	}
	if len(borderStr) > 0 && borderStr != "1" {
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
		var dx, dy float64
		var textWidth float64
		textWidthSet := false
		getTextWidth := func() float64 {
			if !textWidthSet {
				textWidth = f.textWidthWithWordSpacing(txtStr)
				textWidthSet = true
			}
			return textWidth
		}
		switch {
		case alignRight:
			dx = w - f.cMargin - getTextWidth()
		case alignCenter:
			dx = (w - getTextWidth()) / 2
		default:
			dx = f.cMargin
		}
		switch {
		case alignTop:
			dy = (f.fontSize - h) / 2.0
		case alignBottom:
			dy = (h - f.fontSize) / 2.0
		case alignBaseline:
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
		renderedJustified := false
		if utf8Justify {
			justifyText := txtStr
			if f.isRTL {
				justifyText = reverseText(justifyText)
			}
			t, justify := utf8JustificationWords(justifyText)
			manualWordSpacing := f.ws != 0 && strings.Contains(justifyText, " ")
			if justify || manualWordSpacing {
				txtStr = justifyText
				wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
				space := appendEscapedUTF16BE(nil, " ", false, f.currentFont.usedRunes)
				strSize := f.GetStringSymbolWidth(txtStr)
				s = append(s, "BT 0 Tw "...)
				s = appendPDFNumberSpace(s, (f.x+dx)*k, 2)
				s = appendPDFNumberSpace(s, (f.h-(f.y+.5*h+.3*f.fontSize))*k, 2)
				s = append(s, "Td ["...)
				shift := f.wordSpacingFontUnits()
				if alignStr == "J" && justify {
					shift = float64(wmax-strSize) / float64(len(t)-1)
				}
				numt := len(t)
				var smallEncodedWords [4]struct {
					word    string
					encoded []byte
				}
				smallEncodedWordCount := 0
				var encodedWords map[string][]byte
				appendEncodedWord := func(dst []byte, word string) []byte {
					if word == "" {
						return dst
					}
					if encodedWords != nil {
						if encoded, ok := encodedWords[word]; ok {
							return append(dst, encoded...)
						}
					} else {
						for i := 0; i < smallEncodedWordCount; i++ {
							if smallEncodedWords[i].word == word {
								encodedWords = make(map[string][]byte, smallEncodedWordCount)
								for j := 0; j < smallEncodedWordCount; j++ {
									encodedWords[smallEncodedWords[j].word] = smallEncodedWords[j].encoded
								}
								return append(dst, smallEncodedWords[i].encoded...)
							}
						}
					}
					start := len(dst)
					dst = appendEscapedUTF16BE(dst, word, false, f.currentFont.usedRunes)
					encoded := dst[start:len(dst):len(dst)]
					if encodedWords != nil {
						encodedWords[word] = encoded
					} else if smallEncodedWordCount < len(smallEncodedWords) {
						smallEncodedWords[smallEncodedWordCount].word = word
						smallEncodedWords[smallEncodedWordCount].encoded = encoded
						smallEncodedWordCount++
					}
					return dst
				}
				for i, tx := range t {
					s = append(s, '(')
					s = appendEncodedWord(s, tx)
					s = append(s, ") "...)
					if (i + 1) < numt {
						s = appendPDFNumber(s, -shift, 3)
						s = append(s, '(')
						s = append(s, space...)
						s = append(s, ") "...)
					}
				}
				s = append(s, "] TJ ET "...)
				s = appendPDFNumberSpace(s, f.ws*k, 5)
				s = append(s, "Tw"...)
				if alignStr == "J" && justify {
					textWidth = w - 2*f.cMargin
					textWidthSet = true
				}
				renderedJustified = true
			}
		}
		if !renderedJustified {
			reverseUTF8 := f.isCurrentUTF8 && f.isRTL
			bt := (f.x + dx) * k
			td := (f.h - (f.y + dy + .5*h + .3*f.fontSize)) * k
			s = append(s, "BT "...)
			s = appendPDFNumberSpace(s, bt, 2)
			s = appendPDFNumberSpace(s, td, 2)
			s = append(s, "Td ("...)
			if f.isCurrentUTF8 {
				if reverseUTF8 {
					s = appendEscapedUTF16BEReverse(s, txtStr, false, f.currentFont.usedRunes)
				} else {
					s = appendEscapedUTF16BE(s, txtStr, false, f.currentFont.usedRunes)
				}
			} else {
				s = appendEscapedPDFCellText(s, txtStr)
			}
			s = append(s, ")Tj ET"...)
		}
		if f.underline {
			s = append(s, ' ')
			s = f.appendUnderlineRectWidth(s, f.x+dx, f.y+dy+.5*h+.3*f.fontSize, getTextWidth())
		}
		if f.strikeout {
			s = append(s, ' ')
			s = f.appendStrikeoutRectWidth(s, f.x+dx, f.y+dy+.5*h+.3*f.fontSize, getTextWidth())
		}
		if f.colorFlag {
			s = append(s, " Q"...)
		}
		if link > 0 || len(linkStr) > 0 {
			f.newLink(f.x+dx, f.y+dy+.5*h-.5*f.fontSize, getTextWidth(), f.fontSize, link, linkStr)
		}
	}
	if len(s) > 0 {
		f.outTaggedContent(s, f.consumeNextTextTag(link > 0 || linkStr != ""))
	}
	f.retainContentCommandBuffer(s)
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

func (f *Document) estimateCellFormatBufferCapacity(txtStr, borderStr string, fill bool) int {
	capacity := 0
	if fill || borderStr == "1" {
		capacity += 96
	}
	if len(borderStr) > 0 && borderStr != "1" {
		capacity += 44 * len(borderStr)
	}
	if len(txtStr) > 0 {
		textCapacity := 64 + len(txtStr)
		if f.isCurrentUTF8 {
			textCapacity += len(txtStr) * 2
		}
		if f.colorFlag {
			textCapacity += len(f.color.text.str) + 4
		}
		if f.underline {
			textCapacity += 64
		}
		if f.strikeout {
			textCapacity += 64
		}
		capacity += textCapacity
	}
	if capacity == 0 {
		return 0
	}
	return capacity + 16
}

func utf8JustificationWords(text string) ([]string, bool) {
	words := make([]string, 0, 4)
	fieldCount := 0
	inField := false
	start := 0
	for i, r := range text {
		if r == ' ' {
			words = append(words, text[start:i])
			start = i + 1
		}
		if unicode.IsSpace(r) {
			inField = false
			continue
		}
		if !inField {
			fieldCount++
			inField = true
		}
	}
	words = append(words, text[start:])
	return words, fieldCount > 1 && len(words) > 1
}

func (f *Document) cellSimple(w, h float64, txtStr string) {
	if f.err != nil {
		return
	}
	if f.currentFont.Name == "" {
		f.err = errors.New("font has not been set; unable to render text")
		return
	}
	k := f.k
	if f.y+h > f.pageBreakTrigger && !f.inHeader && !f.inFooter && f.acceptPageBreak() {
		x := f.x
		f.addPageFormatRotation(f.curOrientation, f.curPageSize, f.curRotation)
		if f.err != nil {
			return
		}
		f.x = x
	}
	if w == 0 {
		w = f.w - f.rMargin - f.x
	}
	if len(txtStr) > 0 {
		drawText := txtStr
		reverseUTF8 := f.isCurrentUTF8 && f.isRTL
		capacity := 64 + len(drawText) + 16
		if f.isCurrentUTF8 {
			capacity += len(drawText) * 2
		}
		if f.colorFlag {
			capacity += len(f.color.text.str) + 4
		}
		if f.underline {
			capacity += 64
		}
		if f.strikeout {
			capacity += 64
		}
		s := make([]byte, 0, capacity)
		if f.colorFlag {
			s = append(s, "q "...)
			s = append(s, f.color.text.str...)
			s = append(s, ' ')
		}
		x := f.x + f.cMargin
		y := f.y + .5*h + .3*f.fontSize
		s = append(s, "BT "...)
		s = appendPDFNumberSpace(s, x*k, 2)
		s = appendPDFNumberSpace(s, (f.h-y)*k, 2)
		s = append(s, "Td ("...)
		if f.isCurrentUTF8 {
			if reverseUTF8 {
				s = appendEscapedUTF16BEReverse(s, drawText, false, f.currentFont.usedRunes)
			} else {
				s = appendEscapedUTF16BE(s, drawText, false, f.currentFont.usedRunes)
			}
		} else {
			s = appendEscapedPDFCellText(s, drawText)
		}
		s = append(s, ")Tj ET"...)
		if f.underline || f.strikeout {
			textWidth := f.GetStringWidth(txtStr)
			if f.underline {
				s = append(s, ' ')
				s = f.appendUnderlineRectWidth(s, x, y, textWidth)
			}
			if f.strikeout {
				s = append(s, ' ')
				s = f.appendStrikeoutRectWidth(s, x, y, textWidth)
			}
		}
		if f.colorFlag {
			s = append(s, " Q"...)
		}
		f.outTaggedContent(s, f.consumeNextTextTag(false))
	}
	f.lasth = h
	f.x += w
}

func appendEscapedPDFCellText(buf []byte, text string) []byte {
	for i := 0; i < len(text); i++ {
		buf = appendEscapedPDFLiteralByte(buf, text[i])
	}
	return buf
}

func reverseText(text string) string {
	runes := []rune(text)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// Cell is a simpler version of CellFormat with no fill, border, links or
// special alignment. The Cell_strikeout() example demonstrates this method.
func (f *Document) Cell(w, h float64, txtStr string) {
	if f.ws == 0 {
		f.cellSimple(w, h, txtStr)
		return
	}
	f.CellFormat(w, h, txtStr, "", 0, "L", false, 0, "")
}

// Cellf is a simpler printf-style version of CellFormat with no fill, border,
// links or special alignment. See documentation for the fmt package for
// details on fmtStr and args.
func (f *Document) Cellf(w, h float64, fmtStr string, args ...any) {
	if f.ws == 0 {
		f.cellSimple(w, h, sprintf(fmtStr, args...))
		return
	}
	f.CellFormat(w, h, sprintf(fmtStr, args...), "", 0, "L", false, 0, "")
}
