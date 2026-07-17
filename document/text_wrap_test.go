// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"reflect"
	"testing"
)

func TestCoreMultiCellWrapOwnsNormalizedTextAndTrailingLF(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint))
	pdf.SetFont("Courier", "", 10)
	tests := []struct {
		name        string
		text        string
		wantText    string
		wantContent []string
		wantRanges  [][3]int
	}{
		{"empty", "", "", []string{""}, [][3]int{{0, 0, 0}}},
		{"one terminal LF", "\n", "", []string{""}, [][3]int{{0, 0, 0}}},
		{"text terminal LF", "A\n", "A", []string{"A"}, [][3]int{{0, 1, 1}}},
		{"leading LF", "\nA", "\nA", []string{"", "A"}, [][3]int{{0, 0, 1}, {1, 2, 2}}},
		{"two terminal LF", "A\n\n", "A\n", []string{"A", ""}, [][3]int{{0, 1, 2}, {2, 2, 2}}},
		{"only two LF", "\n\n", "\n", []string{"", ""}, [][3]int{{0, 0, 1}, {1, 1, 1}}},
		{"interior empty", "A\n\nB", "A\n\nB", []string{"A", "", "B"}, [][3]int{{0, 1, 2}, {2, 2, 3}, {3, 4, 4}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wrapped, err := pdf.wrapCoreMultiCellText(test.text, 1000, "L")
			if err != nil {
				t.Fatalf("wrapCoreMultiCellText() = %v", err)
			}
			if wrapped.Text != test.wantText {
				t.Fatalf("normalized text = %q, want %q", wrapped.Text, test.wantText)
			}
			content := make([]string, len(wrapped.Lines))
			ranges := make([][3]int, len(wrapped.Lines))
			for index, line := range wrapped.Lines {
				content[index] = wrapped.Text[line.StartByte:line.EndByte]
				ranges[index] = [3]int{line.StartByte, line.EndByte, line.NextByte}
				if index > 0 && wrapped.Lines[index-1].NextByte != line.StartByte {
					t.Fatalf("line ownership has a gap at %d: %#v", index, wrapped.Lines)
				}
			}
			if !reflect.DeepEqual(content, test.wantContent) || !reflect.DeepEqual(ranges, test.wantRanges) {
				t.Fatalf("content/ranges = %#v/%#v, want %#v/%#v", content, ranges, test.wantContent, test.wantRanges)
			}
		})
	}
}

func TestWrappedTextStrictWidthAndConsumedSpaceBoundary(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint))
	pdf.SetFont("Courier", "", 10)
	collect := func(maxWidth float64) ([]string, []wrappedTextLine) {
		text := "AA BB"
		var lines []wrappedTextLine
		pdf.walkWrappedText(text, wrappedTextOptions{
			Mode: wrappedTextCoreMultiCell, MaxWidth: maxWidth, AlwaysFinal: true,
		}, func(line wrappedTextLine) bool {
			lines = append(lines, line)
			return true
		})
		content := make([]string, len(lines))
		for index, line := range lines {
			content[index] = text[line.StartByte:line.EndByte]
		}
		return content, lines
	}
	content, _ := collect(3000)
	if want := []string{"AA BB"}; !reflect.DeepEqual(content, want) {
		t.Fatalf("exact width content = %#v, want %#v", content, want)
	}
	content, lines := collect(2999)
	if want := []string{"AA", "BB"}; !reflect.DeepEqual(content, want) {
		t.Fatalf("one-unit-short content = %#v, want %#v", content, want)
	}
	if lines[0].Break != wrappedBreakSoftSpace || lines[0].EndByte != 2 || lines[0].NextByte != 3 {
		t.Fatalf("space break = %#v, want visible AA and consumed separator", lines[0])
	}
}

func TestCoreMultiCellWrapAlignmentControlsConfiguredWordSpacing(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint))
	pdf.SetFont("Courier", "", 10)
	pdf.ws = pdf.fontSize * 600 / 1000
	width := float64(1800)*pdf.fontSize/1000 + 2*pdf.cMargin

	left, err := pdf.wrapCoreMultiCellText("A A", width, "L")
	if err != nil {
		t.Fatalf("left wrap = %v", err)
	}
	justified, err := pdf.wrapCoreMultiCellText("A A", width, "J")
	if err != nil {
		t.Fatalf("justified wrap = %v", err)
	}
	if len(left.Lines) != 2 || len(justified.Lines) != 1 {
		t.Fatalf("line counts L/J = %d/%d, want 2/1", len(left.Lines), len(justified.Lines))
	}
}

func TestSharedSplitScannerCountAndCollectionStayEquivalent(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint))
	pdf.SetFont("Courier", "", 10)
	texts := []string{"", "A", "AA BB", "A\n\nB", " leading", "trailing ", "中 文", string([]byte{'A', 0xff, 'B'})}
	for _, text := range texts {
		for _, width := range []float64{0, 8, 20, 100} {
			if got, want := pdf.SplitTextCount(text, width), len(pdf.SplitText(text, width)); got != want {
				t.Fatalf("SplitTextCount(%q, %.1f) = %d, want %d", text, width, got, want)
			}
			bytesText := []byte(text)
			if got, want := pdf.SplitLineCount(bytesText, width), len(pdf.SplitLines(bytesText, width)); got != want {
				t.Fatalf("SplitLineCount(%q, %.1f) = %d, want %d", text, width, got, want)
			}
		}
	}
}
