// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"math"
	"unicode"
	"unicode/utf8"
)

// SplitLines splits text into several lines using the current font. Each line
// has its length limited to a maximum width given by w. This function can be
// used to determine the total height of wrapped text for vertical placement
// purposes.
//
// This method is useful for codepage-based fonts only. For UTF-8 encoded text,
// use SplitText().
//
// You can use MultiCell if you want to print text on several lines in a
// simple way.
func (f *Document) SplitLines(txt []byte, w float64) [][]byte {
	lines := [][]byte{}
	cw := f.currentFont.Cw
	wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
	s := txt
	if bytes.Contains(s, []byte("\r")) {
		s = bytes.ReplaceAll(s, []byte("\r"), []byte{})
	}
	nb := len(s)
	for nb > 0 && s[nb-1] == '\n' {
		nb--
	}
	s = s[0:nb]
	sep := -1
	i := 0
	j := 0
	l := 0
	for i < nb {
		c := s[i]
		l += cw[c]
		if c == ' ' || c == '\t' || c == '\n' {
			sep = i
		}
		if c == '\n' || l > wmax {
			if sep == -1 {
				if i == j {
					i++
				}
				sep = i
			} else {
				i = sep + 1
			}
			lines = append(lines, s[j:sep])
			sep = -1
			j = i
			l = 0
		} else {
			i++
		}
	}
	if i != j {
		lines = append(lines, s[j:i])
	}
	return lines
}

// SplitText splits UTF-8 encoded text into several lines using the current
// font. Each line has its length limited to a maximum width given by w. This
// function can be used to determine the total height of wrapped text for
// vertical placement purposes.
func (f *Document) SplitText(txt string, w float64) (lines []string) {
	wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
	nb := len(txt)
	for nb > 0 && txt[nb-1] == '\n' {
		nb--
	}
	txt = txt[:nb]
	sep := -1
	sepSize := 1
	sepInclude := false
	i := 0
	j := 0
	l := 0
	for i < nb {
		c, size := utf8.DecodeRuneInString(txt[i:])
		if size <= 0 {
			break
		}
		next := i + size
		l += f.currentFontRuneWidth(c)
		if unicode.IsSpace(c) {
			sep = i
			sepSize = size
			sepInclude = false
		} else if isChinese(c) {
			sep = i
			sepSize = size
			sepInclude = true
		}
		if c == '\n' || l > wmax {
			lineEnd := sep
			if sep == -1 {
				if i == j {
					i = next
				}
				sep = i
				lineEnd = sep
			} else {
				if sepInclude {
					lineEnd = sep + sepSize
				}
				i = sep + sepSize
			}
			lines = append(lines, txt[j:lineEnd])
			sep = -1
			sepSize = 1
			sepInclude = false
			j = i
			l = 0
		} else {
			i = next
		}
	}
	if i != j {
		lines = append(lines, txt[j:i])
	}
	return lines
}

// SplitTextCount returns the number of lines SplitText would produce without
// allocating the line strings.
func (f *Document) SplitTextCount(txt string, w float64) int {
	wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
	nb := len(txt)
	for nb > 0 && txt[nb-1] == '\n' {
		nb--
	}
	txt = txt[:nb]
	sep := -1
	sepSize := 1
	i := 0
	j := 0
	l := 0
	count := 0
	for i < nb {
		c, size := utf8.DecodeRuneInString(txt[i:])
		if size <= 0 {
			break
		}
		next := i + size
		l += f.currentFontRuneWidth(c)
		if unicode.IsSpace(c) {
			sep = i
			sepSize = size
		} else if isChinese(c) {
			sep = i
			sepSize = size
		}
		if c == '\n' || l > wmax {
			if sep == -1 {
				if i == j {
					i = next
				}
			} else {
				i = sep + sepSize
			}
			count++
			sep = -1
			sepSize = 1
			j = i
			l = 0
		} else {
			i = next
		}
	}
	if i != j {
		count++
	}
	return count
}

// SplitLineCount returns the number of lines SplitLines would produce without
// allocating a slice of line spans.
func (f *Document) SplitLineCount(txt []byte, w float64) int {
	cw := f.currentFont.Cw
	wmax := int(math.Ceil((w - 2*f.cMargin) * 1000 / f.fontSize))
	s := txt
	if bytes.Contains(s, []byte("\r")) {
		s = bytes.ReplaceAll(s, []byte("\r"), []byte{})
	}
	nb := len(s)
	for nb > 0 && s[nb-1] == '\n' {
		nb--
	}
	s = s[0:nb]
	sep := -1
	i := 0
	j := 0
	l := 0
	count := 0
	for i < nb {
		c := s[i]
		l += cw[c]
		if c == ' ' || c == '\t' || c == '\n' {
			sep = i
		}
		if c == '\n' || l > wmax {
			if sep == -1 {
				if i == j {
					i++
				}
			} else {
				i = sep + 1
			}
			count++
			sep = -1
			j = i
			l = 0
		} else {
			i++
		}
	}
	if i != j {
		count++
	}
	return count
}
