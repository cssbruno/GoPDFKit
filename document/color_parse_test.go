// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import "testing"

func TestCSSRGBColorParsing(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  CSSColorType
	}{
		{name: "integer components", value: "rgb(12, 34, 56)", want: CSSColorType{R: 12, G: 34, B: 56, Set: true}},
		{name: "percent and clamped components", value: "rgb(-1, 50%, 999)", want: CSSColorType{R: 0, G: 127, B: 255, Set: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := parseCSSPaint(test.value)
			if !ok || got != test.want {
				t.Fatalf("parseCSSPaint(%q) = %#v, %v; want %#v, true", test.value, got, ok, test.want)
			}
		})
	}

	for _, value := range []string{
		"rgb(1, 2)",
		"rgb(1, bad, 3)",
		"rgb(1, 2, 3, 4)",
	} {
		if _, ok := parseCSSPaint(value); ok {
			t.Errorf("parseCSSPaint(%q) accepted malformed RGB", value)
		}
	}
}

func TestHTMLFontShorthandExpansion(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  map[string]string
	}{
		{
			name:  "compact line height",
			value: "italic 700 14pt/1.5 Helvetica",
			want:  map[string]string{"font-style": "italic", "font-weight": "700", "font-size": "14pt", "line-height": "1.5", "font-family": "Helvetica"},
		},
		{
			name:  "spaced slash",
			value: "12pt / 1.4 Times",
			want:  map[string]string{"font-style": "normal", "font-weight": "normal", "font-size": "12pt", "line-height": "1.4", "font-family": "Times"},
		},
		{
			name:  "defaults",
			value: "10pt Courier",
			want:  map[string]string{"font-style": "normal", "font-weight": "normal", "font-size": "10pt", "line-height": "normal", "font-family": "Courier"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := htmlExpandFontShorthand(test.value)
			if !ok {
				t.Fatalf("htmlExpandFontShorthand(%q) rejected valid shorthand", test.value)
			}
			for key, want := range test.want {
				if got[key] != want {
					t.Errorf("%s = %q, want %q", key, got[key], want)
				}
			}
		})
	}

	for _, value := range []string{
		"italic italic 12pt Arial",
		"small-caps 12pt Arial",
		"12pt/invalid Arial",
		"12pt",
	} {
		if _, ok := htmlExpandFontShorthand(value); ok {
			t.Errorf("htmlExpandFontShorthand(%q) accepted unsupported shorthand", value)
		}
	}
}
