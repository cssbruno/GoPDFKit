// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

var errCoreTextWrapUnsupported = errors.New("document: core text wrap unsupported")

type wrappedTextBreak string

const (
	wrappedBreakFinal      wrappedTextBreak = "final"
	wrappedBreakExplicitLF wrappedTextBreak = "explicit_lf"
	wrappedBreakSoftSpace  wrappedTextBreak = "soft_space"
	wrappedBreakSoftCJK    wrappedTextBreak = "soft_cjk"
	wrappedBreakForced     wrappedTextBreak = "forced"
)

type wrappedTextMode uint8

const (
	wrappedTextBytes wrappedTextMode = iota + 1
	wrappedTextUTF8
	wrappedTextCoreMultiCell
)

// wrappedTextLine owns [StartByte, NextByte) of the normalized scanner input.
// [StartByte, EndByte) is visible content; the difference up to NextByte is a
// consumed separator. Keeping both ranges makes spaces and empty LF lines
// unambiguous for future source provenance.
type wrappedTextLine struct {
	StartByte      int
	EndByte        int
	NextByte       int
	Break          wrappedTextBreak
	WidthFontUnits float64
	JustifyGaps    int
}

type wrappedTextOptions struct {
	Mode        wrappedTextMode
	MaxWidth    float64
	WordSpacing float64
	AlwaysFinal bool
}

type wrappedCoreText struct {
	Text  string
	Lines []wrappedTextLine
}

// walkWrappedText is the shared streaming line scanner. The callback runs
// synchronously before scanning resumes so a future MultiCell migration can
// preserve lifecycle callbacks that change document state between lines.
func (f *Document) walkWrappedText(text string, options wrappedTextOptions, yield func(wrappedTextLine) bool) int {
	separator := -1
	separatorSize := 0
	separatorInclude := false
	separatorWidth := 0.0
	separatorBreak := wrappedBreakSoftSpace
	i, lineStart := 0, 0
	width := 0.0
	spaceCount := 0
	count := 0

	emit := func(line wrappedTextLine) bool {
		count++
		return yield == nil || yield(line)
	}
	reset := func(next int) {
		separator = -1
		separatorSize = 0
		separatorInclude = false
		separatorWidth = 0
		separatorBreak = wrappedBreakSoftSpace
		i = next
		lineStart = next
		width = 0
		spaceCount = 0
	}

	for i < len(text) {
		characterStart := i
		character, size := rune(text[i]), 1
		if options.Mode == wrappedTextUTF8 {
			character, size = utf8.DecodeRuneInString(text[i:])
		}
		next := i + size
		widthBeforeCharacter := width

		if character == '\n' {
			if !emit(wrappedTextLine{
				StartByte: lineStart, EndByte: characterStart, NextByte: next,
				Break: wrappedBreakExplicitLF, WidthFontUnits: widthBeforeCharacter,
				JustifyGaps: maxInt(spaceCount-1, 0),
			}) {
				return count
			}
			reset(next)
			continue
		}

		characterWidth := float64(f.currentFontRuneWidth(character))
		width += characterWidth
		if character == ' ' {
			width += options.WordSpacing
			spaceCount++
		}

		isSeparator := false
		includeSeparator := false
		separatorKind := wrappedBreakSoftSpace
		switch options.Mode {
		case wrappedTextBytes:
			isSeparator = character == ' ' || character == '\t'
		case wrappedTextUTF8:
			isSeparator = unicode.IsSpace(character)
			if !isSeparator && isChinese(character) {
				isSeparator = true
				includeSeparator = true
				separatorKind = wrappedBreakSoftCJK
			}
		case wrappedTextCoreMultiCell:
			isSeparator = character == ' '
		}
		if isSeparator {
			separator = characterStart
			separatorSize = size
			separatorInclude = includeSeparator
			separatorBreak = separatorKind
			if includeSeparator {
				separatorWidth = width
			} else {
				separatorWidth = widthBeforeCharacter
			}
		}

		if width > options.MaxWidth {
			lineEnd, nextByte := separator, separator+separatorSize
			lineWidth := separatorWidth
			breakKind := separatorBreak
			if separator == -1 {
				breakKind = wrappedBreakForced
				lineEnd = characterStart
				nextByte = characterStart
				lineWidth = widthBeforeCharacter
				if characterStart == lineStart {
					lineEnd = next
					nextByte = next
					lineWidth = width
				}
			} else if separatorInclude {
				lineEnd += separatorSize
			}
			if !emit(wrappedTextLine{
				StartByte: lineStart, EndByte: lineEnd, NextByte: nextByte,
				Break: breakKind, WidthFontUnits: lineWidth,
				JustifyGaps: maxInt(spaceCount-1, 0),
			}) {
				return count
			}
			reset(nextByte)
			continue
		}
		i = next
	}

	if i != lineStart || options.AlwaysFinal {
		emit(wrappedTextLine{
			StartByte: lineStart, EndByte: i, NextByte: i, Break: wrappedBreakFinal,
			WidthFontUnits: width, JustifyGaps: maxInt(spaceCount-1, 0),
		})
	}
	return count
}

func maxInt(first, second int) int {
	if first > second {
		return first
	}
	return second
}

func trimAllTrailingLF(text string) string {
	for len(text) > 0 && text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
	}
	return text
}

func normalizeCoreMultiCellText(text string) string {
	text = strings.ReplaceAll(text, "\r", "")
	if len(text) > 0 && text[len(text)-1] == '\n' {
		text = text[:len(text)-1]
	}
	return text
}

func (f *Document) wrappedTextMaxWidth(width float64) float64 {
	return math.Ceil((width - 2*f.cMargin) * 1000 / f.fontSize)
}

func isPlannerCoreText(text string) bool {
	for index := 0; index < len(text); index++ {
		character := text[index]
		if character == '\n' || character >= 0x20 && character <= 0x7e {
			continue
		}
		return false
	}
	return true
}

func (f *Document) wrapCoreMultiCellText(text string, width float64, align string) (wrappedCoreText, error) {
	if f == nil || f.err != nil || f.currentFont.Name == "" || f.isCurrentUTF8 ||
		f.fontSize <= 0 || !isFiniteFloat(f.fontSize) || !isFiniteFloat(width) || !isFiniteFloat(f.cMargin) {
		return wrappedCoreText{}, errCoreTextWrapUnsupported
	}
	normalized := normalizeCoreMultiCellText(text)
	wordSpacing := 0.0
	if align != "J" {
		wordSpacing = f.wordSpacingFontUnits()
	}
	result := wrappedCoreText{Text: normalized, Lines: make([]wrappedTextLine, 0, 1)}
	f.walkWrappedText(normalized, wrappedTextOptions{
		Mode: wrappedTextCoreMultiCell, MaxWidth: f.wrappedTextMaxWidth(width),
		WordSpacing: wordSpacing, AlwaysFinal: true,
	}, func(line wrappedTextLine) bool {
		result.Lines = append(result.Lines, line)
		return true
	})
	return result, nil
}
