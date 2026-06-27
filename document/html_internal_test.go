// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestHTMLParseLineHeight(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	tests := []struct {
		value string
		base  float64
		want  float64
	}{
		{value: "1.5", base: 10, want: 15},
		{value: "150%", base: 10, want: 15},
		{value: "10mm", base: 5, want: 10},
	}
	for _, tt := range tests {
		got, ok := parseHTMLLineHeight(tt.value, tt.base, pdf)
		if !ok {
			t.Fatalf("parseHTMLLineHeight(%q) ok = false", tt.value)
		}
		if !almostEqual(got, tt.want) {
			t.Fatalf("parseHTMLLineHeight(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestHTMLCollapseWhitespace(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "empty", text: "", want: ""},
		{name: "single-space", text: " ", want: " "},
		{name: "only-space-run", text: " \n\t ", want: " "},
		{name: "already-normalized", text: "alpha beta", want: "alpha beta"},
		{name: "leading-and-trailing", text: " alpha beta ", want: " alpha beta "},
		{name: "internal-run", text: "alpha   beta", want: "alpha beta"},
		{name: "tabs-and-newlines", text: "alpha\tbeta\n gamma", want: "alpha beta gamma"},
		{name: "unicode-space", text: "alpha\u00a0beta", want: "alpha beta"},
	}
	for _, tt := range tests {
		if got := collapseHTMLWhitespace(tt.text); got != tt.want {
			t.Fatalf("%s: collapseHTMLWhitespace(%q) = %q, want %q", tt.name, tt.text, got, tt.want)
		}
	}
}

func TestParseStyleDeclarations(t *testing.T) {
	got := parseStyleDeclarations(" color : #123456 ; font-weight:bold; ; text-align : right ")
	want := map[string]string{
		"color":       "#123456",
		"font-weight": "bold",
		"text-align":  "right",
	}
	for name, value := range want {
		if got[name] != value {
			t.Fatalf("parseStyleDeclarations()[%q] = %q, want %q in %#v", name, got[name], value, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("parseStyleDeclarations() len = %d, want %d: %#v", len(got), len(want), got)
	}
	if got := parseStyleDeclarations("color"); got != nil {
		t.Fatalf("parseStyleDeclarations(no colon) = %#v, want nil", got)
	}
}

func TestHTMLCSSSpecificityAndInlinePrecedence(t *testing.T) {
	tokens := HTMLTokenize(`<style>
		p { color: #111111; text-align: left; line-height: 1.1; }
		.note { color: #222222; }
		div p.note { color: #333333; text-align: center; }
		#target { color: #444444; }
		p.note { line-height: 1.4; }
	</style>`)
	rules := htmlCollectCSSRules(tokens)
	el := HTMLSegmentType{Cat: 'O', Str: "p", Attr: map[string]string{
		"id":    "target",
		"class": "note",
		"style": "color: #555555",
	}}
	ancestor := HTMLSegmentType{Cat: 'O', Str: "div", Attr: map[string]string{}}
	decl := htmlElementDeclarationsWithStyle(el, rules, parseStyleDeclarations(el.Attr["style"]), ancestor)

	if got := decl["color"]; got != "#555555" {
		t.Fatalf("inline color = %q, want #555555 over id/class/tag rules", got)
	}
	if got := decl["text-align"]; got != "center" {
		t.Fatalf("descendant text-align = %q, want center", got)
	}
	if got := decl["line-height"]; got != "1.4" {
		t.Fatalf("class+tag line-height = %q, want 1.4", got)
	}
}

func TestHTMLCSSIndexedDeclarationsMatchScan(t *testing.T) {
	tokens := HTMLTokenize(`<style>
		p { color: #111111; text-align: left; }
		.notice { color: #222222; }
		.report .group > p.notice { text-align: center; line-height: 1.2; }
		#target { color: #333333; }
		p.notice { line-height: 1.4; }
	</style>`)
	rules := htmlCollectCSSRules(tokens)
	el := HTMLSegmentType{Cat: 'O', Str: "p", Attr: map[string]string{
		"id":    "target",
		"class": "notice",
		"style": "color: #444444",
	}}
	ancestors := []HTMLSegmentType{
		{Cat: 'O', Str: "section", Attr: map[string]string{"class": "report"}},
		{Cat: 'O', Str: "div", Attr: map[string]string{"class": "group"}},
	}
	style := parseStyleDeclarations(el.Attr["style"])
	indexed := htmlElementDeclarationsWithStyle(el, rules, style, ancestors...)
	scanned := htmlElementDeclarationsWithStyleScan(el, rules, style, ancestors)

	if len(indexed) != len(scanned) {
		t.Fatalf("indexed declarations len = %d, scan len = %d: indexed=%#v scan=%#v", len(indexed), len(scanned), indexed, scanned)
	}
	for name, want := range scanned {
		if got := indexed[name]; got != want {
			t.Fatalf("indexed declarations[%q] = %q, scan = %q; indexed=%#v scan=%#v", name, got, want, indexed, scanned)
		}
	}
}

func TestHTMLTextStyleInheritanceAndOverrides(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	inherited := htmlTextStyle{
		fontFamily: "Helvetica",
		fontSize:   12,
		lineHeight: 6,
		align:      "R",
		color:      CSSColorType{R: 1, G: 2, B: 3, Set: true},
	}
	child := inherited
	applyHTMLStyleDeclarations(&child, map[string]string{
		"font-family": "monospace",
		"color":       "#112233",
		"line-height": "2",
		"text-align":  "center",
	}, inherited.fontSize, inherited.lineHeight, pdf)

	if child.fontFamily != "Courier" {
		t.Fatalf("font family = %q, want Courier", child.fontFamily)
	}
	if child.color.R != 0x11 || child.color.G != 0x22 || child.color.B != 0x33 || !child.color.Set {
		t.Fatalf("color = %#v, want #112233", child.color)
	}
	if !almostEqual(child.lineHeight, 12) {
		t.Fatalf("line height = %.2f, want 12", child.lineHeight)
	}
	if child.align != "C" {
		t.Fatalf("align = %q, want C", child.align)
	}
}

func TestHTMLSplitLinesWrapsLongWords(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	lines := htmlSplitLines(pdf, "Supercalifragilisticexpialidocious", 10)
	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want long word split across lines: %#v", len(lines), lines)
	}
	for _, line := range lines {
		if line == "" {
			t.Fatalf("empty wrapped line in %#v", lines)
		}
		if pdf.GetStringWidth(line) > 10 && len([]rune(line)) > 1 {
			t.Fatalf("wrapped line %q width %.2f exceeds limit", line, pdf.GetStringWidth(line))
		}
	}
}

func TestHTMLTypographyStyleDeclarations(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	st := htmlTextStyle{fontFamily: "Helvetica", fontSize: 12, lineHeight: 5, align: "L"}
	applyHTMLStyleDeclarations(&st, map[string]string{
		"font-size":       "14pt",
		"font-family":     "monospace",
		"font-weight":     "700",
		"font-style":      "italic",
		"text-decoration": "underline line-through",
		"text-align":      "right",
	}, 12, 5, pdf)

	if !almostEqual(st.fontSize, 14) {
		t.Fatalf("font size = %.2f, want 14", st.fontSize)
	}
	if st.fontFamily != "Courier" {
		t.Fatalf("font family = %q, want Courier", st.fontFamily)
	}
	if !st.bold || !st.italic || !st.underline || !st.strike {
		t.Fatalf("text style = %#v, want bold italic underline strike", st)
	}
	if st.align != "R" {
		t.Fatalf("align = %q, want R", st.align)
	}
}

func TestHTMLBoxEdgesFromDeclarations(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	edges := htmlBoxEdgesFromDeclarations(map[string]string{
		"padding":        "1mm 2mm 3mm 4mm",
		"padding-left":   "5mm",
		"padding-bottom": "6mm",
	}, "padding", pdf, 100)

	if !almostEqual(edges.top, 1) || !almostEqual(edges.right, 2) || !almostEqual(edges.bottom, 6) || !almostEqual(edges.left, 5) {
		t.Fatalf("edges = %#v, want top=1 right=2 bottom=6 left=5", edges)
	}
}

func TestHTMLBorderRadiusFromDeclarations(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	radius := htmlBorderRadiusFromDeclarations(map[string]string{
		"border-radius":              "2mm 3mm 4mm 5mm",
		"border-top-right-radius":    "6mm",
		"border-bottom-left-radius":  "7mm",
		"border-bottom-right-radius": "8mm / 9mm",
	}, pdf, 100)

	if !almostEqual(radius.topLeft, 2) || !almostEqual(radius.topRight, 6) || !almostEqual(radius.bottomRight, 8) || !almostEqual(radius.bottomLeft, 7) {
		t.Fatalf("radius = %#v, want top-left=2 top-right=6 bottom-right=8 bottom-left=7", radius)
	}
}

func TestHTMLBoxShadowFromDeclarations(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	shadow := htmlBoxShadowFromDeclarations(map[string]string{
		"box-shadow": "2mm 3mm 4mm 1mm rgba(10, 20, 30, 0.35)",
	}, pdf, 100)

	if !shadow.enabled {
		t.Fatal("box shadow disabled, want enabled")
	}
	if !almostEqual(shadow.offsetX, 2) || !almostEqual(shadow.offsetY, 3) || !almostEqual(shadow.blur, 4) || !almostEqual(shadow.spread, 1) {
		t.Fatalf("box shadow dimensions = %#v, want 2/3/4/1mm", shadow)
	}
	if shadow.color.R != 10 || shadow.color.G != 20 || shadow.color.B != 30 || !almostEqual(shadow.alpha, 0.35) {
		t.Fatalf("box shadow color/alpha = %#v alpha %.2f, want rgba(10,20,30,.35)", shadow.color, shadow.alpha)
	}
}

func TestHTMLTableCellPadding(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	edges := htmlTableCellPadding(map[string]string{
		"style": "padding: 1mm 2mm; padding-left: 4mm",
	}, pdf, 3, 100)
	if !almostEqual(edges.top, 1) || !almostEqual(edges.right, 2) || !almostEqual(edges.bottom, 1) || !almostEqual(edges.left, 4) {
		t.Fatalf("cell padding = %#v, want top=1 right=2 bottom=1 left=4", edges)
	}

	fallback := htmlTableCellPadding(nil, pdf, 3, 100)
	if !almostEqual(fallback.top, 3) || !almostEqual(fallback.right, 3) || !almostEqual(fallback.bottom, 3) || !almostEqual(fallback.left, 3) {
		t.Fatalf("fallback padding = %#v, want all sides 3", fallback)
	}
}

func TestHTMLResolvedImageSize(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	info := &ImageInfo{w: 96, h: 48, scale: pdf.k, dpi: 72}

	wd, ht := htmlResolvedImageSize(info, pdf, 0, 0)
	if !almostEqual(wd, 25.4) || !almostEqual(ht, 12.7) {
		t.Fatalf("default image size = %.2f x %.2f, want 25.4 x 12.7", wd, ht)
	}

	wd, ht = htmlResolvedImageSize(info, pdf, 20, 0)
	if !almostEqual(wd, 20) || !almostEqual(ht, 10) {
		t.Fatalf("width-constrained image size = %.2f x %.2f, want 20 x 10", wd, ht)
	}

	wd, ht = htmlResolvedImageSize(info, pdf, 0, 8)
	if !almostEqual(wd, 16) || !almostEqual(ht, 8) {
		t.Fatalf("height-constrained image size = %.2f x %.2f, want 16 x 8", wd, ht)
	}
}

func TestHTMLImageAlign(t *testing.T) {
	if got := htmlImageAlign(map[string]string{"style": "text-align:right"}, "L"); got != "R" {
		t.Fatalf("style text-align right = %q, want R", got)
	}
	if got := htmlImageAlign(map[string]string{"align": "middle"}, "L"); got != "C" {
		t.Fatalf("align middle = %q, want C", got)
	}
	if got := htmlImageAlign(nil, "R"); got != "R" {
		t.Fatalf("fallback align = %q, want R", got)
	}
}

func TestHTMLImageObjectFit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	info := &ImageInfo{w: 96, h: 48, scale: pdf.k, dpi: 72}

	drawX, drawY, drawWd, drawHt, flowWd, flowHt := htmlImageFitBox(info, pdf, 20, 20, 20, 20, "contain")
	if !almostEqual(drawX, 0) || !almostEqual(drawY, 5) || !almostEqual(drawWd, 20) || !almostEqual(drawHt, 10) || !almostEqual(flowWd, 20) || !almostEqual(flowHt, 20) {
		t.Fatalf("contain fit = x %.2f y %.2f %.2fx%.2f flow %.2fx%.2f, want centered 20x10 in 20x20", drawX, drawY, drawWd, drawHt, flowWd, flowHt)
	}

	drawX, drawY, drawWd, drawHt, flowWd, flowHt = htmlImageFitBox(info, pdf, 20, 20, 20, 20, "cover")
	if !almostEqual(drawX, -10) || !almostEqual(drawY, 0) || !almostEqual(drawWd, 40) || !almostEqual(drawHt, 20) || !almostEqual(flowWd, 20) || !almostEqual(flowHt, 20) {
		t.Fatalf("cover fit = x %.2f y %.2f %.2fx%.2f flow %.2fx%.2f, want centered 40x20 clipped to 20x20", drawX, drawY, drawWd, drawHt, flowWd, flowHt)
	}

	if got := htmlImageObjectFit(map[string]string{"style": "object-fit: cover"}); got != "cover" {
		t.Fatalf("object-fit cover = %q", got)
	}
}

func TestHTMLImageSourceReadsDPI(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	html := pdf.HTMLNew()
	_, options, err := html.htmlImageSource(`data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ` +
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==`)
	if err != nil {
		t.Fatalf("htmlImageSource(data URL) error = %v", err)
	}
	if !options.ReadDpi {
		t.Fatal("data URL image ReadDpi = false, want true")
	}

	html.AllowLocalImages = true
	_, options, err = html.htmlImageSource("/tmp/example.png")
	if err != nil {
		t.Fatalf("htmlImageSource(local path) error = %v", err)
	}
	if !options.ReadDpi {
		t.Fatal("local image ReadDpi = false, want true")
	}
}

func TestHTMLFigureKeepsImageWithCaption(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	pdf.SetY(pdf.pageBreakTrigger - 25)

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<figure><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ`+
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==" width="20mm" height="20mm"/>`+
		`<figcaption>Kept caption</figcaption></figure>`)

	if pdf.Error() != nil {
		t.Fatalf("Write() error = %v", pdf.Error())
	}
	if pdf.page < 2 {
		t.Fatalf("figure stayed on page %d, want moved to next page with caption", pdf.page)
	}
}

func TestHTMLCellBorderColor(t *testing.T) {
	color := htmlCellBorderColor(
		map[string]string{"style": "border-color: #123456"},
		map[string]string{"style": "border-color: red"},
	)
	if !color.Set || color.R != 0x12 || color.G != 0x34 || color.B != 0x56 {
		t.Fatalf("cell border color = %#v, want #123456", color)
	}

	color = htmlCellBorderColor(
		map[string]string{},
		map[string]string{"bordercolor": "orange"},
	)
	if !color.Set || color.R != 255 || color.G != 165 || color.B != 0 {
		t.Fatalf("row border color = %#v, want orange", color)
	}
}

func TestHTMLBorderFromDeclarations(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	border := htmlBorderFromDeclarations(map[string]string{
		"border": "2mm solid #123456",
	}, pdf, 100)
	if !border.enabled || !almostEqual(border.width, 2) || !border.color.Set || border.color.R != 0x12 || border.color.G != 0x34 || border.color.B != 0x56 {
		t.Fatalf("border shorthand = %#v, want enabled 2mm #123456", border)
	}

	border = htmlBorderFromDeclarations(map[string]string{
		"border-style": "solid",
		"border-width": "thick",
		"border-color": "orange",
	}, pdf, 100)
	if !border.enabled || !almostEqual(border.width, 1.5) || !border.color.Set || border.color.R != 255 || border.color.G != 165 || border.color.B != 0 {
		t.Fatalf("border longhand = %#v, want enabled thick orange", border)
	}

	border = htmlBorderFromDeclarations(map[string]string{
		"border": "2mm none #123456",
	}, pdf, 100)
	if border.enabled {
		t.Fatalf("border none enabled = true, want false")
	}
}

func TestHTMLBorderFromDeclarationsPerSide(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	border := htmlBorderFromDeclarations(map[string]string{
		"border":              "1mm solid #111111",
		"border-left":         "2mm solid #123456",
		"border-top-style":    "none",
		"border-bottom-width": "3mm",
		"border-bottom-color": "orange",
	}, pdf, 100)

	if !border.enabled || !border.sideSpecific {
		t.Fatalf("border = %#v, want enabled side-specific border", border)
	}
	if border.top.enabled {
		t.Fatalf("top border enabled = true, want false")
	}
	if !border.left.enabled || !almostEqual(border.left.width, 2) || !border.left.color.Set || border.left.color.R != 0x12 || border.left.color.G != 0x34 || border.left.color.B != 0x56 {
		t.Fatalf("left border = %#v, want 2mm #123456", border.left)
	}
	if !border.bottom.enabled || !almostEqual(border.bottom.width, 3) || !border.bottom.color.Set || border.bottom.color.R != 255 || border.bottom.color.G != 165 || border.bottom.color.B != 0 {
		t.Fatalf("bottom border = %#v, want 3mm orange", border.bottom)
	}
	if !border.right.enabled || !almostEqual(border.right.width, 1) {
		t.Fatalf("right border = %#v, want inherited 1mm border", border.right)
	}
}

func TestHTMLBorderFromAttrs(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	border := htmlBorderFromAttrs(map[string]string{
		"border":      "1",
		"bordercolor": "#123456",
	}, pdf, 100)
	if !border.enabled || !border.color.Set || border.color.R != 0x12 || border.color.G != 0x34 || border.color.B != 0x56 {
		t.Fatalf("legacy border attrs = %#v, want enabled #123456", border)
	}
}

func TestHTMLBlockHasBoxStyleForBorderLonghands(t *testing.T) {
	el := HTMLSegmentType{
		Cat:  'O',
		Str:  "div",
		Attr: map[string]string{"style": "border-width: 1mm; border-color: #123456; border-style: solid"},
	}
	if !htmlBlockHasBoxStyle(el, nil) {
		t.Fatal("expected border longhands to require block box rendering")
	}
}

func TestHTMLBreakDeclarationParsing(t *testing.T) {
	if !htmlBreakForcesPage("page") || !htmlBreakForcesPage("always") {
		t.Fatal("expected page/always to force a page break")
	}
	if htmlBreakForcesPage("avoid") {
		t.Fatal("avoid should not force a page break")
	}
	if !htmlBreakAvoidsInside("avoid") || !htmlBreakAvoidsInside("avoid-page") {
		t.Fatal("expected avoid values to keep content together")
	}
}

func TestHTMLTableVerticalAlignParsingAndOffset(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	st := htmlTextStyle{}
	applyHTMLStyleDeclarations(&st, map[string]string{"vertical-align": "middle"}, 12, 5, pdf)
	if st.verticalAlign != "middle" {
		t.Fatalf("CSS verticalAlign = %q, want middle", st.verticalAlign)
	}

	st = htmlTextStyle{}
	applyHTMLAttrs(&st, map[string]string{"valign": "bottom"}, 12, 5, pdf)
	if st.verticalAlign != "bottom" {
		t.Fatalf("attr verticalAlign = %q, want bottom", st.verticalAlign)
	}

	if got := htmlTableVerticalOffset(20, 8, "middle"); !almostEqual(got, 6) {
		t.Fatalf("middle offset = %v, want 6", got)
	}
	if got := htmlTableVerticalOffset(20, 8, "bottom"); !almostEqual(got, 12) {
		t.Fatalf("bottom offset = %v, want 12", got)
	}
	if got := htmlTableVerticalOffset(20, 8, "top"); got != 0 {
		t.Fatalf("top offset = %v, want 0", got)
	}
}

func TestHTMLBlockBreakBeforeAndAfter(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<p style="break-before: page">new page</p>`)
	if pdf.PageCount() != 2 {
		t.Fatalf("break-before page count = %d, want 2", pdf.PageCount())
	}

	html.Write(lineHeight, `<p style="break-after: page">next page follows</p>`)
	if pdf.PageCount() != 3 {
		t.Fatalf("break-after page count = %d, want 3", pdf.PageCount())
	}
}

func TestHTMLWriteMaxGeneratedPages(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	html := pdf.HTMLNew()
	html.MaxGeneratedPages = 1
	html.Write(lineHeight, `<p style="break-before: page">new page</p>`)

	if pdf.err == nil {
		t.Fatal("expected generated page limit error")
	}
	if got, want := pdf.err.Error(), "HTML rendering exceeded maximum generated pages: 2 > 1"; !strings.Contains(got, want) {
		t.Fatalf("error = %q, want to contain %q", got, want)
	}
}

func TestHTMLSafetyErrorsAreDeterministic(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	html := pdf.HTMLNew()
	html.MaxHTMLBytes = 4
	html.Write(lineHeight, `<p>too long</p>`)
	if got, want := pdf.err.Error(), "HTML input exceeds maximum size"; got != want {
		t.Fatalf("MaxHTMLBytes error = %q, want %q", got, want)
	}

	html = New("P", "mm", "A4", "").HTMLNew()
	html.MaxElementDepth = 1
	if got, want := html.ValidateHTML(`<div><p>nested</p></div>`), []string{"HTML element depth exceeds maximum size"}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("MaxElementDepth diagnostics = %#v, want %#v", got, want)
	}
}

func TestHTMLHeadingKeepsWithNextBlock(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	pdf.SetY(pdf.pageBreakTrigger - lineHeight*1.2)

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<h2>Heading</h2><p>Body paragraph</p>`)

	if pdf.PageCount() != 2 {
		t.Fatalf("PageCount() = %d, want 2", pdf.PageCount())
	}
	if pdf.GetY() < pdf.tMargin+lineHeight*2 {
		t.Fatalf("final Y = %.2f, want heading and paragraph rendered on new page", pdf.GetY())
	}
}

func TestHTMLTableBreakInsideAvoidKeepsSmallTableTogether(t *testing.T) {
	splitPDF := htmlTableNearPageEnd("")
	avoidPDF := htmlTableNearPageEnd(` style="break-inside: avoid"`)

	if splitPDF.PageCount() != 2 || avoidPDF.PageCount() != 2 {
		t.Fatalf("page counts = split %d avoid %d, want both 2", splitPDF.PageCount(), avoidPDF.PageCount())
	}
	if avoidPDF.GetY() <= splitPDF.GetY()+4 {
		t.Fatalf("avoid table final y = %.2f, split final y = %.2f; avoid table did not stay together", avoidPDF.GetY(), splitPDF.GetY())
	}
}

func TestHTMLTableMovesRowToNextPageWhenItDoesNotFit(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	pdf.SetY(pdf.pageBreakTrigger - 4)

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1"><tr><td>next page row</td></tr></table>`)

	if pdf.PageCount() != 2 {
		t.Fatalf("PageCount() = %d, want 2", pdf.PageCount())
	}
}

func TestHTMLTableAvoidsSingleOrphanRowAtPageBottom(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	pdf.SetY(pdf.pageBreakTrigger - lineHeight*1.5)

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1"><tr><td>row one</td></tr><tr><td>row two</td></tr></table>`)

	if pdf.PageCount() != 2 {
		t.Fatalf("PageCount() = %d, want 2", pdf.PageCount())
	}
	if pdf.GetY() < pdf.tMargin+lineHeight*2 {
		t.Fatalf("final Y = %.2f, want both rows rendered on the new page", pdf.GetY())
	}
}

func TestHTMLTableSplitsLargeTablesAcrossPages(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	var rows strings.Builder
	for i := 0; i < 80; i++ {
		rows.WriteString(`<tr><td>row `)
		rows.WriteString(strconv.Itoa(i))
		rows.WriteString(`</td></tr>`)
	}
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1"><thead><tr><th>Header</th></tr></thead>`+rows.String()+`</table>`)

	if pdf.err != nil {
		t.Fatalf("HTML Write error = %v", pdf.err)
	}
	if pdf.PageCount() < 2 {
		t.Fatalf("PageCount() = %d, want at least 2", pdf.PageCount())
	}
}

func htmlTableNearPageEnd(tableAttrs string) *Document {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	pdf.SetY(pdf.pageBreakTrigger - lineHeight*4)

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1"`+tableAttrs+`><tr><td>one</td></tr><tr><td>two</td></tr><tr><td>three</td></tr></table>`)
	return pdf
}

func TestHTMLParseTableCaptionAndFooterRows(t *testing.T) {
	tokens := HTMLTokenize(`<table><caption>Summary</caption><tfoot><tr><td>Total</td></tr></tfoot><tbody><tr><td>Body</td></tr></tbody></table>`)
	table, end := parseHTMLTable(tokens, 0)
	if end == 0 {
		t.Fatal("parseHTMLTable did not find closing table")
	}
	if got := strings.TrimSpace(htmlPlainText(table.captionTokens)); got != "Summary" {
		t.Fatalf("caption = %q, want Summary", got)
	}
	if len(table.rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(table.rows))
	}
	if table.rows[0].footer {
		t.Fatalf("first row is footer, want body row before footer")
	}
	if !table.rows[1].footer {
		t.Fatalf("second row footer = false, want true")
	}
}

func TestHTMLParseNestedTableStaysInsideParentCell(t *testing.T) {
	tokens := HTMLTokenize(`<table><tr><td>Outer<table><tr><td>Inner</td></tr></table></td><td>Sibling</td></tr></table>`)
	table, end := parseHTMLTable(tokens, 0)
	if end == 0 {
		t.Fatal("parseHTMLTable did not find closing table")
	}
	if len(table.rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(table.rows))
	}
	if len(table.rows[0].cells) != 2 {
		t.Fatalf("len(cells) = %d, want 2", len(table.rows[0].cells))
	}
	if text := htmlPlainText(table.rows[0].cells[0].tokens); !strings.Contains(text, "Outer") || !strings.Contains(text, "Inner") {
		t.Fatalf("first cell text = %q, want nested table text inside parent cell", text)
	}
	if got := strings.TrimSpace(table.rows[0].cells[1].text); got != "Sibling" {
		t.Fatalf("second cell text = %q, want Sibling", got)
	}
}

func TestHTMLWriteTableCaptionAndFooter(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1"><caption>Invoice Summary</caption><tfoot><tr><td>Total</td></tr></tfoot><tbody><tr><td>Line item</td></tr></tbody></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Invoice Summary", "Line item", "Total"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("PDF output missing %q", want)
		}
	}
	if strings.Index(pdfText, "Line item") > strings.Index(pdfText, "Total") {
		t.Fatalf("tfoot rendered before body: %q", pdfText)
	}
}

func TestHTMLTableColumnWidthsUseColspanWidthHints(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	widths := htmlTestTableColumnWidths(pdf, 120,
		`<table><tr><td colspan="2" width="90mm">wide</td><td>auto</td></tr></table>`,
	)

	if len(widths) != 3 {
		t.Fatalf("len(widths) = %d, want 3", len(widths))
	}
	if !almostEqual(widths[0], 45) || !almostEqual(widths[1], 45) || !almostEqual(widths[2], 30) {
		t.Fatalf("widths = %.2f %.2f %.2f, want 45 45 30", widths[0], widths[1], widths[2])
	}
}

func TestHTMLTableColumnWidthsUseContentMinimums(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	widths := htmlTestTableColumnWidths(pdf, 100,
		`<table><tr><td>short</td><td>Supercalifragilistic</td></tr></table>`,
	)

	if len(widths) != 2 {
		t.Fatalf("len(widths) = %d, want 2", len(widths))
	}
	if widths[1] <= widths[0] {
		t.Fatalf("widths = %.2f %.2f, want long-word column wider", widths[0], widths[1])
	}
	if !almostEqual(widths[0]+widths[1], 100) {
		t.Fatalf("total width = %.2f, want 100", widths[0]+widths[1])
	}
}

func htmlTestTableColumnWidths(pdf *Document, tableWd float64, fragment string) []float64 {
	table, _ := parseHTMLTable(HTMLTokenize(fragment), 0)
	rows := htmlTableLayoutRows(table.rows)
	return htmlTableColumnWidths(rows, htmlTableLayoutColumnCount(rows), tableWd, pdf)
}

func TestHTMLWritePerSideBorder(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<div style="border-left:2mm solid #123456;border-bottom:1mm solid orange;padding:1mm">Side border</div>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "Side border") {
		t.Fatal("generated PDF does not contain bordered text")
	}
	if !strings.Contains(pdfText, " l S") && !strings.Contains(pdfText, " l\nS") {
		t.Fatal("generated PDF does not contain side border line operations")
	}
}

func TestHTMLTableBorderCollapse(t *testing.T) {
	if !htmlTableBorderCollapse(map[string]string{"border-collapse": "collapse"}, nil) {
		t.Fatal("expected border-collapse: collapse to enable collapsed borders")
	}
	if htmlTableBorderCollapse(map[string]string{"border-collapse": "separate"}, nil) {
		t.Fatal("border-collapse: separate should not enable collapsed borders")
	}

	border := htmlBorderStyle{}
	border.setAll(htmlBorderSideStyle{enabled: true, width: 1})
	collapsed := htmlCollapsedTableCellBorder(border, htmlTableCellPlacement{row: 1, col: 1}, false)
	if collapsed.top.enabled || collapsed.left.enabled {
		t.Fatalf("collapsed internal top/left = %#v/%#v, want disabled", collapsed.top, collapsed.left)
	}
	if !collapsed.right.enabled || !collapsed.bottom.enabled {
		t.Fatalf("collapsed right/bottom = %#v/%#v, want enabled", collapsed.right, collapsed.bottom)
	}

	collapsed = htmlCollapsedTableCellBorder(border, htmlTableCellPlacement{row: 1, col: 0}, true)
	if !collapsed.top.enabled {
		t.Fatalf("forced top border disabled = %#v", collapsed.top)
	}
}

func TestHTMLDebugLogReportsUnsupportedRenderingFeatures(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	var messages []string
	html := pdf.HTMLNew()
	html.DebugLog = func(message string) {
		messages = append(messages, message)
	}
	html.Write(lineHeight, `<style>.card:hover{display:flex;color:red}</style>`+
		`<video style="float:left">clip</video>`+
		`<table><tr><td rowspan="2">x</td></tr></table>`)

	for _, want := range []string{
		`CSS selector ".card:hover" is not supported yet`,
		`CSS property "display" in style rule is not supported yet`,
		`HTML tag <video> is not supported yet`,
		`CSS property "float" in inline style is not supported yet`,
	} {
		if !containsString(messages, want) {
			t.Fatalf("debug messages = %#v, missing %q", messages, want)
		}
	}
}

func TestHTMLValidateHTMLReportsUnsupportedRenderingFeatures(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	html := pdf.HTMLNew()
	messages := html.ValidateHTML(`<style>.card:hover{display:flex}</style><video style="float:left">clip</video>`)

	for _, want := range []string{
		`CSS selector ".card:hover" is not supported yet`,
		`CSS property "display" in style rule is not supported yet`,
		`HTML tag <video> is not supported yet`,
		`CSS property "float" in inline style is not supported yet`,
	} {
		if !containsString(messages, want) {
			t.Fatalf("validate messages = %#v, missing %q", messages, want)
		}
	}
}

func TestHTMLValidateHTMLAllowsSupportedBorderLonghands(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	html := pdf.HTMLNew()
	messages := html.ValidateHTML(`<div style="border-width:1mm;border-style:solid;border-color:#123456;border-left:2mm solid orange;border-top-style:none;border-radius:4px;box-shadow:2px 3px 5px rgba(0,0,0,.2)">box</div><img src="" style="object-fit:cover;max-width:20mm;max-height:20mm"/>`)
	if len(messages) != 0 {
		t.Fatalf("validate messages = %#v, want none for supported border, shadow, and image CSS", messages)
	}
}

func TestHTMLDebugLogDeduplicatesUnsupportedFeatures(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	var messages []string
	html := pdf.HTMLNew()
	html.DebugLog = func(message string) {
		messages = append(messages, message)
	}
	html.Write(lineHeight, `<video></video><video style="float:left"></video><span style="float:right">x</span>`)

	if countString(messages, "HTML tag <video> is not supported yet") != 1 {
		t.Fatalf("debug messages = %#v, want one unsupported video message", messages)
	}
	if countString(messages, `CSS property "float" in inline style is not supported yet`) != 1 {
		t.Fatalf("debug messages = %#v, want one unsupported float message", messages)
	}
}

func TestHTMLDebugLogAllowsSupportedBorderLonghands(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	var messages []string
	html := pdf.HTMLNew()
	html.DebugLog = func(message string) {
		messages = append(messages, message)
	}
	html.Write(lineHeight, `<div style="border-width:1mm;border-style:solid;border-color:#123456">box</div>`)

	if len(messages) != 0 {
		t.Fatalf("debug messages = %#v, want none for supported border longhands", messages)
	}
}

func containsString(values []string, want string) bool {
	return countString(values, want) > 0
}

func countString(values []string, want string) int {
	count := 0
	for _, value := range values {
		if strings.Contains(value, want) {
			count++
		}
	}
	return count
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
