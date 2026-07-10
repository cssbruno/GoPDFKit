// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	stdhtml "html"
	"sort"
	"strings"
	"unicode"

	"github.com/cssbruno/gopdfkit/layout"
)

type htmlTextStyle struct {
	bold               bool
	italic             bool
	underline          bool
	strike             bool
	preserveWhitespace bool
	href               string
	align              string
	verticalAlign      string
	fontFamily         string
	fontSize           float64
	lineHeight         float64
	color              CSSColorType
	role               string
	list               string
	listStyleType      string
	script             int
}

type htmlListState struct {
	kind      string
	styleType string
	counter   int
	indent    float64
}

func htmlClosePops(tag string) bool {
	switch tag {
	case "br", "img", "hr", "meta", "link", "input":
		return false
	default:
		return true
	}
}

func (html *HTML) writeHorizontalRule(el HTMLSegmentType, cssRules []htmlCSSRule, lineHt float64, ancestors []HTMLSegmentType) {
	pdf := html.pdf
	if pdf.GetX() != pdf.lMargin {
		pdf.Ln(lineHt)
	}
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	availableWd := pdf.w - pdf.rMargin - pdf.lMargin
	wd, ok := parseHTMLBoxLength(firstNonEmpty(decl["width"], el.Attr["width"]), pdf, availableWd)
	if !ok || wd <= 0 || wd > availableWd {
		wd = availableWd
	}
	thickness, ok := parseHTMLBoxLength(firstNonEmpty(decl["height"], decl["border-width"], el.Attr["size"]), pdf, lineHt)
	if !ok || thickness <= 0 {
		thickness = pdf.GetLineWidth()
	}
	color := htmlDeclarationColor(decl, "border-color", "color", "background-color", "background")
	drawR, drawG, drawB := pdf.GetDrawColor()
	lineWidth := pdf.GetLineWidth()
	defer func() {
		pdf.SetDrawColor(drawR, drawG, drawB)
		pdf.SetLineWidth(lineWidth)
	}()
	if color.Set && !color.None {
		pdf.SetDrawColor(color.R, color.G, color.B)
	}
	pdf.SetLineWidth(thickness)
	y := pdf.GetY() + lineHt/2
	pdf.Line(pdf.lMargin, y, pdf.lMargin+wd, y)
	pdf.Ln(lineHt)
}

func htmlBlockHasBoxStyle(el HTMLSegmentType, cssRules []htmlCSSRule, ancestors ...HTMLSegmentType) bool {
	decl := htmlElementDeclarations(el, cssRules, ancestors...)
	if htmlHasBoxEdgeDeclaration(decl, "padding") || htmlHasBoxEdgeDeclaration(decl, "margin") || htmlHasBreakDeclaration(decl) || htmlHasBorderDeclaration(decl) {
		return true
	}
	style := htmlBlockBoxFromDeclarations(el, decl, nil, 0)
	return style.border.enabled || style.background.Set || style.radius.hasAny() || style.shadow.enabled || style.padding.hasAny() || style.margin.hasAny() || style.breakBefore || style.breakAfter || style.breakInsideAvoid
}

func (html *HTML) blockHasBoxStyle(el HTMLSegmentType, cssRules []htmlCSSRule, ancestors ...HTMLSegmentType) bool {
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	if htmlHasBoxEdgeDeclaration(decl, "padding") || htmlHasBoxEdgeDeclaration(decl, "margin") || htmlHasBreakDeclaration(decl) || htmlHasBorderDeclaration(decl) {
		return true
	}
	style := htmlBlockBoxFromDeclarations(el, decl, nil, 0)
	return style.border.enabled || style.background.Set || style.radius.hasAny() || style.shadow.enabled || style.padding.hasAny() || style.margin.hasAny() || style.breakBefore || style.breakAfter || style.breakInsideAvoid
}

func (html *HTML) writeBlockBox(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
	return html.writeCompiledBlockBox(nil, tokens, start, lineHt, inherited, fallback, cssRules, ancestors)
}

func (html *HTML) writeCompiledBlockBox(compiled *CompiledHTML, tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
	var blockTokens []HTMLSegmentType
	var end int
	if compiled != nil {
		blockTokens, end = compiled.collectElementTokens(start, tokens[start].Str)
	} else {
		blockTokens, end = htmlCollectElementTokens(tokens, start, tokens[start].Str)
	}
	if len(blockTokens) == 0 {
		return end
	}
	pdf := html.pdf
	if pdf.GetX() != pdf.lMargin {
		pdf.Ln(lineHt)
	}
	availableWd := pdf.w - pdf.rMargin - pdf.lMargin
	box := html.blockBox(tokens[start], cssRules, pdf, availableWd, ancestors...)
	if box.breakBefore {
		if !html.addPageFormat() {
			return end
		}
	}
	style := inherited
	applyHTMLCSSRules(&style, tokens[start], cssRules, inherited.fontSize, inherited.lineHeight, pdf, ancestors...)
	html.applyAttrs(&style, tokens[start].Attr, inherited.fontSize, inherited.lineHeight, pdf)
	html.applyTextStyle(style, fallback)
	text, textCached := "", false
	if compiled != nil {
		text, textCached = compiled.text(start, style.preserveWhitespace)
	}
	if !textCached {
		text = htmlPlainTextWithMode(blockTokens[1:len(blockTokens)-1], style.preserveWhitespace)
	}
	boxWd := htmlMaxFloat(availableWd-box.margin.left-box.margin.right, 0)
	contentWd := htmlMaxFloat(boxWd-box.padding.left-box.padding.right, 0)
	styleLineHt := htmlEffectiveLineHeight(style, lineHt)
	lines := htmlSplitLines(pdf, text, contentWd)
	ht := float64(len(lines))*styleLineHt + box.padding.top + box.padding.bottom
	textR, textG, textB := pdf.GetTextColor()
	fillR, fillG, fillB := pdf.GetFillColor()
	drawR, drawG, drawB := pdf.GetDrawColor()
	lineWidth := pdf.GetLineWidth()
	cellMargin := pdf.GetCellMargin()
	defer func() {
		pdf.SetTextColor(textR, textG, textB)
		pdf.SetFillColor(fillR, fillG, fillB)
		pdf.SetDrawColor(drawR, drawG, drawB)
		pdf.SetLineWidth(lineWidth)
		pdf.SetCellMargin(cellMargin)
		html.applyTextStyle(inherited, fallback)
	}()
	if box.margin.top > 0 {
		pdf.Ln(box.margin.top)
	}
	if pdf.y+ht > pdf.pageBreakTrigger && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
		if !html.addPageFormat() {
			return end
		}
	}
	x, y := pdf.GetXY()
	x += box.margin.left
	fillBox := box.background.Set && !box.background.None
	if fillBox {
		pdf.SetFillColor(box.background.R, box.background.G, box.background.B)
	}
	htmlDrawBoxShadow(pdf, x, y, boxWd, ht, box.radius, box.shadow)
	htmlDrawBorderedRect(pdf, x, y, boxWd, ht, box.border, box.radius, fillBox, drawR, drawG, drawB, lineWidth)
	pdf.SetCellMargin(0)
	pdf.SetXY(x+box.padding.left, y+box.padding.top)
	html.applyTextStyle(style, fallback)
	htmlRenderSplitLines(pdf, contentWd, styleLineHt, lines, style.align)
	pdf.SetXY(pdf.lMargin, y+ht+box.margin.bottom)
	if box.breakAfter {
		if !html.addPageFormat() {
			return end
		}
	}
	return end
}

func htmlRenderSplitLines(pdf *Document, width, lineHeight float64, lines []string, align string) {
	for i, line := range lines {
		lineAlign := align
		if i == len(lines)-1 && lineAlign == "J" {
			if pdf.isRTL {
				lineAlign = "R"
			} else {
				lineAlign = ""
			}
		}
		pdf.CellFormat(width, lineHeight, line, "", 2, lineAlign, false, 0, "")
	}
}

func htmlBlockBox(el HTMLSegmentType, cssRules []htmlCSSRule, pdf *Document, relative float64, ancestors ...HTMLSegmentType) htmlBlockBoxStyle {
	decl := htmlElementDeclarations(el, cssRules, ancestors...)
	return htmlBlockBoxFromDeclarations(el, decl, pdf, relative)
}

func (html *HTML) blockBox(el HTMLSegmentType, cssRules []htmlCSSRule, pdf *Document, relative float64, ancestors ...HTMLSegmentType) htmlBlockBoxStyle {
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	return htmlBlockBoxFromDeclarations(el, decl, pdf, relative)
}

func htmlBlockBoxFromDeclarations(el HTMLSegmentType, decl map[string]string, pdf *Document, relative float64) htmlBlockBoxStyle {
	box := htmlBlockBoxStyle{}
	box.background = firstColor(htmlDeclarationColor(decl, "background-color", "background"), htmlAttrColor(el.Attr, "bgcolor"))
	box.border = htmlBorderFromDeclarations(decl, pdf, relative)
	box.radius = htmlBorderRadiusFromDeclarations(decl, pdf, relative)
	box.shadow = htmlBoxShadowFromDeclarations(decl, pdf, relative)
	if !box.border.hasAny() && htmlBorderEnabled(el.Attr["border"]) {
		box.border.setAll(htmlBorderSideStyle{enabled: true})
	}
	box.breakBefore = htmlBreakForcesPage(decl["break-before"]) || htmlBreakForcesPage(decl["page-break-before"])
	box.breakAfter = htmlBreakForcesPage(decl["break-after"]) || htmlBreakForcesPage(decl["page-break-after"])
	box.breakInsideAvoid = htmlBreakAvoidsInside(decl["break-inside"]) || htmlBreakAvoidsInside(decl["page-break-inside"])
	if pdf != nil {
		box.padding = htmlBoxEdgesFromDeclarations(decl, "padding", pdf, relative)
		box.margin = htmlBoxEdgesFromDeclarations(decl, "margin", pdf, relative)
	}
	return box
}

func htmlCollectElementTokens(tokens []HTMLSegmentType, start int, tag string) ([]HTMLSegmentType, int) {
	if start < 0 || start >= len(tokens) {
		return nil, len(tokens) - 1
	}
	depth := 0
	for i := start; i < len(tokens); i++ {
		el := tokens[i]
		if el.Cat == 'O' && el.Str == tag {
			depth++
		}
		if el.Cat == 'C' && el.Str == tag {
			depth--
			if depth == 0 {
				return tokens[start : i+1], i
			}
		}
	}
	return tokens[start:], len(tokens) - 1
}

func htmlSkipElement(tokens []HTMLSegmentType, start int, tag string) int {
	_, end := htmlCollectElementTokens(tokens, start, tag)
	return end
}

func htmlSerializeTokens(tokens []HTMLSegmentType) string {
	var out strings.Builder
	for _, token := range tokens {
		switch token.Cat {
		case 'O':
			out.WriteByte('<')
			out.WriteString(token.Str)
			if len(token.Attr) > 0 {
				keys := make([]string, 0, len(token.Attr))
				for key := range token.Attr {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					out.WriteByte(' ')
					out.WriteString(key)
					out.WriteString(`="`)
					out.WriteString(stdhtml.EscapeString(token.Attr[key]))
					out.WriteByte('"')
				}
			}
			out.WriteByte('>')
		case 'C':
			out.WriteString("</")
			out.WriteString(token.Str)
			out.WriteByte('>')
		case 'T':
			out.WriteString(stdhtml.EscapeString(token.Str))
		}
	}
	return out.String()
}

func htmlCollectCSSRules(tokens []HTMLSegmentType) []htmlCSSRule {
	var rules []htmlCSSRule
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Cat != 'O' || tokens[i].Str != "style" {
			continue
		}
		styleTokens, end := htmlCollectElementTokens(tokens, i, "style")
		rules = append(rules, parseHTMLCSSRules(htmlTokenText(styleTokens))...)
		if len(rules) > htmlMaxCSSRules {
			rules = rules[:htmlMaxCSSRules]
		}
		i = end
	}
	return htmlIndexCSSRules(rules)
}

func htmlTokenText(tokens []HTMLSegmentType) string {
	var out strings.Builder
	for _, token := range tokens {
		if token.Cat == 'T' {
			out.WriteString(token.Str)
		}
	}
	return out.String()
}

func htmlPlainText(tokens []HTMLSegmentType) string {
	var out strings.Builder
	needSpace := false
	lastWasNewline := false
	for _, token := range tokens {
		switch token.Cat {
		case 'T':
			text := collapseHTMLWhitespace(token.Str)
			trimmed := strings.TrimSpace(text)
			if trimmed == "" {
				needSpace = out.Len() > 0
				continue
			}
			if needSpace && out.Len() > 0 && !lastWasNewline {
				out.WriteByte(' ')
			}
			out.WriteString(trimmed)
			lastWasNewline = false
			needSpace = unicode.IsSpace(rune(text[len(text)-1]))
		case 'O':
			if token.Str == "br" {
				out.WriteByte('\n')
				needSpace = false
				lastWasNewline = true
			}
		case 'C':
			switch token.Str {
			case "p", "div", "section", "article", "header", "footer", "figure", "figcaption", "li", "dt", "dd":
				out.WriteByte('\n')
				needSpace = false
				lastWasNewline = true
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func htmlPlainTextWithMode(tokens []HTMLSegmentType, preserveWhitespace bool) string {
	if !preserveWhitespace {
		return htmlPlainText(tokens)
	}
	var out strings.Builder
	for _, token := range tokens {
		switch token.Cat {
		case 'T':
			out.WriteString(token.Str)
		case 'O':
			if token.Str == "br" {
				out.WriteByte('\n')
			}
		case 'C':
			switch token.Str {
			case "p", "div", "section", "article", "header", "footer", "figure", "figcaption", "li", "dt", "dd":
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}

func htmlMaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func clampFloat(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (html *HTML) keepHeadingWithNext(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) {
	html.keepCompiledHeadingWithNext(nil, tokens, start, lineHt, inherited, fallback, cssRules, ancestors)
}

func (html *HTML) keepCompiledHeadingWithNext(compiled *CompiledHTML, tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) {
	if html == nil || html.pdf == nil {
		return
	}
	defer html.applyTextStyle(inherited, fallback)
	var headingTokens []HTMLSegmentType
	var end int
	if compiled != nil {
		headingTokens, end = compiled.collectElementTokens(start, tokens[start].Str)
	} else {
		headingTokens, end = htmlCollectElementTokens(tokens, start, tokens[start].Str)
	}
	if len(headingTokens) == 0 {
		return
	}
	style := inherited
	style.bold = true
	style.fontSize = htmlHeadingFontSize(inherited.fontSize, tokens[start].Str)
	applyHTMLCSSRules(&style, tokens[start], cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, ancestors...)
	html.applyAttrs(&style, tokens[start].Attr, inherited.fontSize, inherited.lineHeight, html.pdf)
	headingHt := html.compiledTextBlockHeight(compiled, start, headingTokens[1:len(headingTokens)-1], style, lineHt, fallback)
	nextHt := html.nextCompiledBlockHeight(compiled, tokens, end+1, lineHt, inherited, fallback, cssRules, ancestors)
	if headingHt <= 0 || nextHt <= 0 {
		return
	}
	currentY := html.pdf.GetY()
	if html.pdf.GetX() != html.pdf.lMargin {
		currentY += lineHt
	}
	needed := headingHt + nextHt
	pageContentHt := html.pdf.pageBreakTrigger - html.pdf.tMargin
	if needed <= pageContentHt && currentY+needed > html.pdf.pageBreakTrigger && !html.pdf.inHeader && !html.pdf.inFooter && html.pdf.acceptPageBreak() {
		html.addPageFormat()
	}
}

func (html *HTML) nextBlockHeight(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) float64 {
	return html.nextCompiledBlockHeight(nil, tokens, start, lineHt, inherited, fallback, cssRules, ancestors)
}

func (html *HTML) nextCompiledBlockHeight(compiled *CompiledHTML, tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) float64 {
	for i := start; i < len(tokens); i++ {
		token := tokens[i]
		if token.Cat == 'T' && strings.TrimSpace(token.Str) == "" {
			continue
		}
		if token.Cat != 'O' {
			continue
		}
		switch token.Str {
		case "p", "div", "section", "article", "header", "footer":
			var blockTokens []HTMLSegmentType
			if compiled != nil {
				blockTokens, _ = compiled.collectElementTokens(i, token.Str)
			} else {
				blockTokens, _ = htmlCollectElementTokens(tokens, i, token.Str)
			}
			if len(blockTokens) < 2 {
				return 0
			}
			style := inherited
			applyHTMLCSSRules(&style, token, cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, ancestors...)
			html.applyAttrs(&style, token.Attr, inherited.fontSize, inherited.lineHeight, html.pdf)
			if html.blockHasBoxStyle(token, cssRules, ancestors...) {
				box := html.blockBox(token, cssRules, html.pdf, html.pdf.w-html.pdf.rMargin-html.pdf.lMargin, ancestors...)
				return html.compiledTextBlockHeight(compiled, i, blockTokens[1:len(blockTokens)-1], style, lineHt, fallback) + box.padding.top + box.padding.bottom + box.margin.top + box.margin.bottom
			}
			return html.compiledTextBlockHeight(compiled, i, blockTokens[1:len(blockTokens)-1], style, lineHt, fallback)
		case "h1", "h2", "h3", "h4", "h5", "h6":
			var blockTokens []HTMLSegmentType
			if compiled != nil {
				blockTokens, _ = compiled.collectElementTokens(i, token.Str)
			} else {
				blockTokens, _ = htmlCollectElementTokens(tokens, i, token.Str)
			}
			if len(blockTokens) < 2 {
				return 0
			}
			style := inherited
			style.bold = true
			style.fontSize = htmlHeadingFontSize(inherited.fontSize, token.Str)
			applyHTMLCSSRules(&style, token, cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, ancestors...)
			html.applyAttrs(&style, token.Attr, inherited.fontSize, inherited.lineHeight, html.pdf)
			return html.compiledTextBlockHeight(compiled, i, blockTokens[1:len(blockTokens)-1], style, lineHt, fallback)
		case "table":
			return html.compiledTableHeight(compiled, tokens, i, lineHt, inherited, fallback, cssRules, ancestors)
		case "figure":
			return html.figureHeight(tokens, i, lineHt, inherited, fallback)
		case "img":
			return html.imageFlowHeight(token.Attr, lineHt)
		}
	}
	return 0
}

func (html *HTML) textBlockHeight(tokens []HTMLSegmentType, style htmlTextStyle, lineHt float64, fallback CSSColorType) float64 {
	return html.compiledTextBlockHeight(nil, -1, tokens, style, lineHt, fallback)
}

func (html *HTML) compiledTextBlockHeight(compiled *CompiledHTML, start int, tokens []HTMLSegmentType, style htmlTextStyle, lineHt float64, fallback CSSColorType) float64 {
	if html == nil || html.pdf == nil {
		return 0
	}
	html.applyTextStyle(style, fallback)
	text := ""
	if compiled != nil {
		if cachedText, ok := compiled.text(start, style.preserveWhitespace); ok {
			text = cachedText
		}
	}
	if text == "" {
		text = htmlPlainTextWithMode(tokens, style.preserveWhitespace)
	}
	if text == "" {
		return htmlEffectiveLineHeight(style, lineHt)
	}
	availableWd := html.pdf.w - html.pdf.rMargin - html.pdf.lMargin
	lineCount := htmlSplitLineCount(html.pdf, text, availableWd)
	return float64(lineCount) * htmlEffectiveLineHeight(style, lineHt)
}

func (html *HTML) tableHeight(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) float64 {
	return html.compiledTableHeight(nil, tokens, start, lineHt, inherited, fallback, cssRules, ancestors)
}

func (html *HTML) compiledTableHeight(compiled *CompiledHTML, tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) float64 {
	var table htmlTableType
	if compiled != nil {
		var ok bool
		table, _, ok = compiled.table(start)
		if !ok {
			table, _ = parseHTMLTable(tokens, start)
		}
	} else {
		table, _ = parseHTMLTable(tokens, start)
	}
	if len(table.rows) == 0 {
		return 0
	}
	availableWd := html.pdf.w - html.pdf.rMargin - html.pdf.lMargin
	tableWd, ok := parseHTMLBoxLength(firstNonEmpty(html.styleValue(table.attrs, "width"), table.attrs["width"]), html.pdf, availableWd)
	if !ok || tableWd <= 0 || tableWd > availableWd {
		tableWd = availableWd
	}
	padding := html.tablePadding(table.attrs, html.pdf)
	layoutRows := htmlTableLayoutRows(table.rows)
	colCount := htmlTableLayoutColumnCount(layoutRows)
	if colCount == 0 {
		return 0
	}
	tableEl := HTMLSegmentType{Cat: 'O', Str: "table", Attr: table.attrs}
	tableAncestors := appendHTMLAncestors(ancestors, tableEl)
	colWidths := html.tableColumnWidths(layoutRows, colCount, tableWd, html.pdf)
	colOffsets := layout.TrackOffsets(colWidths)
	rowHeights := html.measureTableRowHeights(compiled, layoutRows, colOffsets, padding, lineHt, inherited, fallback, cssRules, tableAncestors, htmlBorderStyle{}, table.attrs)
	return html.tableCaptionHeight(compiled, table, tableWd, lineHt, inherited, fallback, cssRules, tableAncestors) + sumFloat64(rowHeights) + lineHt
}

func collapseHTMLWhitespace(text string) string {
	if text == "" {
		return ""
	}
	needsCollapse := false
	previousSpace := false
	textSeen := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if r != ' ' || previousSpace {
				needsCollapse = true
				break
			}
			previousSpace = true
			continue
		}
		textSeen = true
		previousSpace = false
	}
	if !textSeen && !needsCollapse {
		if text == " " {
			return text
		}
		return " "
	}
	if !needsCollapse {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	leadingSpace := false
	pendingSpace := false
	wroteText := false
	textStart := -1
	for i, r := range text {
		if unicode.IsSpace(r) {
			if textStart >= 0 {
				if !wroteText {
					if leadingSpace {
						out.WriteByte(' ')
					}
				} else if pendingSpace {
					out.WriteByte(' ')
				}
				out.WriteString(text[textStart:i])
				wroteText = true
				pendingSpace = false
				textStart = -1
			}
			if wroteText {
				pendingSpace = true
			} else {
				leadingSpace = true
			}
			continue
		}
		if textStart < 0 {
			textStart = i
		}
	}
	if textStart >= 0 {
		if !wroteText {
			if leadingSpace {
				out.WriteByte(' ')
			}
		} else if pendingSpace {
			out.WriteByte(' ')
		}
		out.WriteString(text[textStart:])
		wroteText = true
		pendingSpace = false
	}
	if !wroteText {
		return " "
	}
	if pendingSpace {
		out.WriteByte(' ')
	}
	return out.String()
}
