// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

func TestPaperThemeControlsExactPlanAndPaintTextStyle(t *testing.T) {
	source := "document @themed:\n" +
		"  theme: \"@print\"\n" +
		"  theme @print:\n" +
		"    token @font:\n      type: \"string\"\n      value: \"Courier\"\n" +
		"    token @size:\n      type: \"length\"\n      value: 13pt\n" +
		"    token @leading:\n      type: \"length\"\n      value: 15pt\n" +
		"    token @ink:\n      type: \"color\"\n      value: \"#336699\"\n" +
		"  page:\n" +
		"    width: 160pt\n" +
		"    height: 80pt\n" +
		"    margin: 8pt\n" +
		"    body:\n" +
		"      paragraph @visible:\n" +
		"        font-token: \"font\"\n" +
		"        size-token: \"size\"\n" +
		"        line-height-token: \"leading\"\n" +
		"        color-token: \"ink\"\n" +
		"        text: \"Visible\"\n"

	plan, planned, err := PlanPaper("themed.paper", source)
	if err != nil || planned.Pages == 0 || len(planned.Diagnostics) != 0 {
		t.Fatalf("PlanPaper() = %#v, %v", planned, err)
	}
	projection := plan.plan.Projection()
	if len(projection.GlyphRuns) != 1 {
		t.Fatalf("glyph runs = %#v", projection.GlyphRuns)
	}
	run := projection.GlyphRuns[0]
	if run.FontSize != layoutengine.Fixed(13*layoutengine.FixedScale) ||
		run.Color != (layoutengine.CoreRGBColor{R: 51, G: 102, B: 153, Set: true}) || run.Codes != "Visible" {
		t.Fatalf("themed glyph run = %#v", run)
	}
	if len(projection.Fonts) != 1 || projection.Fonts[0].Face != layoutengine.CoreFontCourier {
		t.Fatalf("themed font resources = %#v", projection.Fonts)
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	rendered, err := target.WritePaper("themed.paper", source)
	if err != nil || !rendered.OK() {
		t.Fatalf("WritePaper() = %#v, %v", rendered, err)
	}
	content := target.pages[1].Bytes()
	for _, want := range [][]byte{
		[]byte("13.0000000000 Tf"),
		[]byte("0.2000000000 0.4000000000 0.6000000000 rg"),
		[]byte("(V) Tj"),
	} {
		if !bytes.Contains(content, want) {
			t.Fatalf("painted page lacks %q:\n%s", want, content)
		}
	}
	for _, declaration := range [][]byte{[]byte("print"), []byte("font"), []byte("size"), []byte("ink")} {
		if bytes.Contains(content, declaration) {
			t.Fatalf("theme declaration %q leaked into visual content:\n%s", declaration, content)
		}
	}
}
