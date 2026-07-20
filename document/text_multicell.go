// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"math"
	"strings"
	"unicode/utf8"
)

// MultiCell prints text with automatic line breaks. The text is placed in
// cells of width w and line height h. Each line is bordered according to
// borderStr, aligned according to alignStr, and optionally filled when fill is
// true. When w is 0, the cell extends to the right margin.
func (f *Document) MultiCell(w, h float64, txtStr, borderStr, alignStr string, fill bool) {
	if !f.requireCurrentFont("rendering text") {
		return
	}
	if alignStr == "" {
		alignStr = "J"
	}
	if w == 0 {
		w = f.w - f.rMargin - f.x
	}
	wmax := math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize)
	originalWordSpacing := f.ws
	restoreWordSpacing := func() {
		if f.ws == originalWordSpacing {
			return
		}
		if f.err != nil {
			f.ws = originalWordSpacing
			return
		}
		f.SetWordSpacing(originalWordSpacing)
	}
	defer restoreWordSpacing()
	wrapWordSpacing := 0.0
	if alignStr != "J" {
		wrapWordSpacing = f.wordSpacingFontUnits()
	}
	s := txtStr
	if strings.Contains(s, "\r") {
		s = strings.ReplaceAll(s, "\r", "")
	}
	var nb int
	if f.isCurrentUTF8 {
		nb = len(s)
		for nb > 0 && s[nb-1] == '\n' {
			nb--
		}
		s = s[:nb]
	} else {
		nb = len(s)
		if nb > 0 && s[nb-1] == '\n' {
			nb--
		}
		s = s[0:nb]
	}
	var b, b2 string
	b = "0"
	if len(borderStr) > 0 {
		if borderStr == "1" {
			borderStr = "LTRB"
			b = "LRT"
			b2 = "LR"
		} else {
			b2 = ""
			if strings.Contains(borderStr, "L") {
				b2 += "L"
			}
			if strings.Contains(borderStr, "R") {
				b2 += "R"
			}
			if strings.Contains(borderStr, "T") {
				b = b2 + "T"
			} else {
				b = b2
			}
		}
	}
	sep := -1
	sepInclude := false
	i := 0
	j := 0
	l := 0.0
	ls := 0.0
	ns := 0
	nl := 1
	for i < nb {
		var c rune
		var charSize int
		next := i + 1
		if f.isCurrentUTF8 {
			c, charSize = utf8.DecodeRuneInString(s[i:])
			if charSize <= 0 {
				break
			}
			next = i + charSize
		} else {
			c = rune(s[i])
		}
		if c == '\n' {
			restoreWordSpacing()
			if f.isCurrentUTF8 {
				newAlignStr := alignStr
				if newAlignStr == "J" {
					if f.isRTL {
						newAlignStr = "R"
					} else {
						newAlignStr = "L"
					}
				}
				f.CellFormat(w, h, s[j:i], b, 2, newAlignStr, fill, 0, "")
			} else {
				f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
			}
			i = next
			sep = -1
			j = i
			l = 0
			ns = 0
			nl++
			if len(borderStr) > 0 && nl == 2 {
				b = b2
			}
			continue
		}
		charWidth := float64(f.currentFontRuneWidth(c))
		l += charWidth
		if c == ' ' {
			l += wrapWordSpacing
			sep = i
			sepInclude = false
			ls = l - charWidth - wrapWordSpacing
			ns++
		} else if f.isCurrentUTF8 && isChinese(c) {
			sep = i
			sepInclude = true
			ls = l
		}
		if l > wmax {
			if sep == -1 {
				if i == j {
					i = next
				}
				restoreWordSpacing()
				f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
			} else {
				lineEnd := sep
				if sepInclude {
					_, sepSize := utf8.DecodeRuneInString(s[sep:])
					lineEnd = sep + sepSize
				}
				if alignStr == "J" {
					lineWordSpacing := 0.0
					if ns > 1 {
						lineWordSpacing = (wmax - ls) / 1000 * f.fontSize / float64(ns-1)
					}
					f.SetWordSpacing(lineWordSpacing)
				}
				f.CellFormat(w, h, s[j:lineEnd], b, 2, alignStr, fill, 0, "")
				if f.isCurrentUTF8 {
					_, sepSize := utf8.DecodeRuneInString(s[sep:])
					i = sep + sepSize
				} else {
					i = sep + 1
				}
			}
			sep = -1
			sepInclude = false
			j = i
			l = 0
			ns = 0
			nl++
			if len(borderStr) > 0 && nl == 2 {
				b = b2
			}
		} else {
			i = next
		}
	}
	restoreWordSpacing()
	if len(borderStr) > 0 && strings.Contains(borderStr, "B") {
		b += "B"
	}
	if f.isCurrentUTF8 {
		if alignStr == "J" {
			if f.isRTL {
				alignStr = "R"
			} else {
				alignStr = ""
			}
		}
		f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
	} else {
		f.CellFormat(w, h, s[j:i], b, 2, alignStr, fill, 0, "")
	}
	f.x = f.lMargin
}
