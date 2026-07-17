// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompileLowersReadableBoxProperties(t *testing.T) {
	const source = "document @report:\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      paragraph @box:\n" +
		"        padding: 8pt\n" +
		"        padding-left: 12pt\n" +
		"        border-width: 1pt\n" +
		"        border-bottom-width: 2pt\n" +
		"        border-color: \"#A1b2C3\"\n" +
		"        border-radius: 4pt\n" +
		"        background: \"#f2f4f8\"\n" +
		"        text @copy: \"Readable box\"\n"
	parsed := paperlang.Parse("box.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	result := Compile(parsed.AST)
	if !result.OK() {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	box := result.Document.Body[0].(layout.ParagraphBlock).Box
	if box.Padding != (layout.Spacing{Top: 8, Right: 8, Bottom: 8, Left: 12}) {
		t.Fatalf("padding = %#v", box.Padding)
	}
	wantColor := layout.DocumentColor{R: 0xa1, G: 0xb2, B: 0xc3, Set: true}
	if box.Border.Top.Width != 1 || box.Border.Right.Width != 1 || box.Border.Bottom.Width != 2 || box.Border.Left.Width != 1 ||
		box.Border.Top.Style != "solid" || box.Border.Bottom.Color != wantColor {
		t.Fatalf("border = %#v", box.Border)
	}
	if box.BorderRadius != 4 || box.BackgroundColor != (layout.DocumentColor{R: 0xf2, G: 0xf4, B: 0xf8, Set: true}) {
		t.Fatalf("decoration = %#v", box)
	}
}

func TestCompileRejectsInvalidReadableBoxProperties(t *testing.T) {
	for _, source := range []string{
		"document:\n  page:\n    body:\n      paragraph:\n        padding: -1pt\n        text: \"x\"\n",
		"document:\n  page:\n    body:\n      paragraph:\n        background: \"red\"\n        text: \"x\"\n",
	} {
		parsed := paperlang.Parse("invalid-box.paper", source)
		if !parsed.OK() {
			t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
		}
		if result := Compile(parsed.AST); result.OK() {
			t.Fatalf("Compile() accepted invalid box source: %s", source)
		}
	}
}
