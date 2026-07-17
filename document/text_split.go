// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
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
	if !f.requireCurrentFont("measuring text") {
		return nil
	}
	lines := [][]byte{}
	s := txt
	if bytes.Contains(s, []byte("\r")) {
		s = bytes.ReplaceAll(s, []byte("\r"), []byte{})
	}
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	normalized := string(s)
	f.walkWrappedText(normalized, wrappedTextOptions{
		Mode: wrappedTextBytes, MaxWidth: f.wrappedTextMaxWidth(w), WordSpacing: f.wordSpacingFontUnits(),
	}, func(line wrappedTextLine) bool {
		lines = append(lines, s[line.StartByte:line.EndByte])
		return true
	})
	return lines
}

// SplitText splits UTF-8 encoded text into several lines using the current
// font. Each line has its length limited to a maximum width given by w. This
// function can be used to determine the total height of wrapped text for
// vertical placement purposes.
func (f *Document) SplitText(txt string, w float64) (lines []string) {
	if !f.requireCurrentFont("measuring text") {
		return nil
	}
	txt = trimAllTrailingLF(txt)
	f.walkWrappedText(txt, wrappedTextOptions{
		Mode: wrappedTextUTF8, MaxWidth: f.wrappedTextMaxWidth(w), WordSpacing: f.wordSpacingFontUnits(),
	}, func(line wrappedTextLine) bool {
		lines = append(lines, txt[line.StartByte:line.EndByte])
		return true
	})
	return lines
}

// SplitTextCount returns the number of lines SplitText would produce without
// allocating the line strings.
func (f *Document) SplitTextCount(txt string, w float64) int {
	if !f.requireCurrentFont("measuring text") {
		return 0
	}
	txt = trimAllTrailingLF(txt)
	return f.walkWrappedText(txt, wrappedTextOptions{
		Mode: wrappedTextUTF8, MaxWidth: f.wrappedTextMaxWidth(w), WordSpacing: f.wordSpacingFontUnits(),
	}, nil)
}

// SplitLineCount returns the number of lines SplitLines would produce without
// allocating a slice of line spans.
func (f *Document) SplitLineCount(txt []byte, w float64) int {
	if !f.requireCurrentFont("measuring text") {
		return 0
	}
	s := txt
	if bytes.Contains(s, []byte("\r")) {
		s = bytes.ReplaceAll(s, []byte("\r"), []byte{})
	}
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return f.walkWrappedText(string(s), wrappedTextOptions{
		Mode: wrappedTextBytes, MaxWidth: f.wrappedTextMaxWidth(w), WordSpacing: f.wordSpacingFontUnits(),
	}, nil)
}
