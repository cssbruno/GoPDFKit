// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"slices"
	"sort"
	"strconv"
	"strings"
)

type htmlFlexStyle struct {
	direction      string
	wrap           bool
	gap            float64
	rowGap         float64
	columnGap      float64
	justifyContent string
	alignItems     string
	alignContent   string
}

type htmlFlexItem struct {
	el        HTMLSegmentType
	tokens    []HTMLSegmentType
	start     int
	end       int
	text      string
	style     htmlTextStyle
	box       htmlBlockBoxStyle
	outerWd   float64
	outerHt   float64
	textHt    float64
	lines     []string
	grow      float64
	shrink    float64
	minWd     float64
	maxWd     float64
	minHt     float64
	maxHt     float64
	height    float64
	hasHeight bool
	order     int
	alignSelf string
	hasWidth  bool
	lineHt    float64
}

type htmlFlexLine struct {
	items []htmlFlexItem
	wd    float64
	ht    float64
}

func htmlDisplayFlex(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return value == "flex" || value == "inline-flex"
}

func (html *HTML) elementDisplayFlex(el HTMLSegmentType, cssRules []htmlCSSRule, ancestors ...HTMLSegmentType) bool {
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	return htmlDisplayFlex(decl["display"])
}

func (html *HTML) writeCompiledFlexBox(compiled *CompiledHTML, tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
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
	container := tokens[start]
	containerAncestors := appendHTMLAncestors(ancestors, container)
	containerBox := html.blockBox(container, cssRules, pdf, availableWd, ancestors...)
	if containerBox.breakBefore {
		if !html.addPageFormat() {
			return end
		}
	}
	containerStyle := inherited
	applyHTMLCSSRules(&containerStyle, container, cssRules, inherited.fontSize, inherited.lineHeight, pdf, ancestors...)
	html.applyAttrs(&containerStyle, container.Attr, inherited.fontSize, inherited.lineHeight, pdf)
	flexStyle := html.flexStyle(container, cssRules, availableWd, ancestors...)
	if flexStyle.direction == "column" {
		return html.writeFlexColumn(compiled, blockTokens, start, end, lineHt, containerStyle, fallback, cssRules, ancestors, containerAncestors, containerBox, flexStyle)
	}

	boxWd := htmlMaxFloat(availableWd-containerBox.margin.left-containerBox.margin.right, 0)
	contentWd := htmlMaxFloat(boxWd-containerBox.padding.left-containerBox.padding.right, 0)
	items := html.collectFlexItems(compiled, blockTokens, start, containerStyle, fallback, cssRules, containerAncestors, contentWd, lineHt)
	if len(items) == 0 {
		return end
	}
	lines := html.flexLines(items, contentWd, flexStyle, fallback, cssRules, containerAncestors)
	linesContentHt := htmlFlexLinesCrossSize(lines, flexStyle)
	contentHt := linesContentHt
	if explicitHt, ok := html.flexContainerContentHeight(container, cssRules, contentWd, ancestors...); ok && explicitHt > contentHt {
		contentHt = explicitHt
		html.distributeFlexLineCrossSpace(lines, contentHt, linesContentHt, flexStyle)
		linesContentHt = htmlFlexLinesCrossSize(lines, flexStyle)
	}
	totalHt := containerBox.padding.top + containerBox.padding.bottom + contentHt

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
	if containerBox.margin.top > 0 {
		pdf.Ln(containerBox.margin.top)
	}
	if pdf.y+totalHt > pdf.pageBreakTrigger && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
		if !html.addPageFormat() {
			return end
		}
	}
	x, y := pdf.GetXY()
	x += containerBox.margin.left
	fillBox := containerBox.background.Set && !containerBox.background.None
	if fillBox {
		pdf.SetFillColor(containerBox.background.R, containerBox.background.G, containerBox.background.B)
	}
	htmlDrawBoxShadow(pdf, x, y, boxWd, totalHt, containerBox.radius, containerBox.shadow)
	htmlDrawBorderedRect(pdf, x, y, boxWd, totalHt, containerBox.border, containerBox.radius, fillBox, drawR, drawG, drawB, lineWidth)
	pdf.SetCellMargin(0)

	lineOffset, lineGap := htmlFlexAlignContentPlacement(len(lines), contentHt, linesContentHt, flexStyle)
	lineY := y + containerBox.padding.top + lineOffset
	for _, line := range lines {
		itemX, itemGap := html.flexLinePlacement(line, contentWd, flexStyle)
		for _, item := range line.items {
			alignItems := htmlFlexItemAlign(item, flexStyle)
			itemY := lineY + htmlFlexItemCrossOffset(line.ht, item.outerHt, alignItems)
			itemHt := item.outerHt
			if alignItems == "stretch" {
				itemHt = line.ht
			}
			html.renderFlexItem(item, x+containerBox.padding.left+itemX, itemY, item.outerWd, itemHt, lineHt, fallback, cssRules, containerAncestors, drawR, drawG, drawB, lineWidth)
			itemX += item.outerWd + itemGap
		}
		lineY += line.ht + lineGap
	}
	pdf.SetXY(pdf.lMargin, y+totalHt+containerBox.margin.bottom)
	if containerBox.breakAfter {
		if !html.addPageFormat() {
			return end
		}
	}
	return end
}

func (html *HTML) writeFlexColumn(compiled *CompiledHTML, blockTokens []HTMLSegmentType, start, end int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors, containerAncestors []HTMLSegmentType, containerBox htmlBlockBoxStyle, flexStyle htmlFlexStyle) int {
	pdf := html.pdf
	availableWd := pdf.w - pdf.rMargin - pdf.lMargin
	boxWd := htmlMaxFloat(availableWd-containerBox.margin.left-containerBox.margin.right, 0)
	contentWd := htmlMaxFloat(boxWd-containerBox.padding.left-containerBox.padding.right, 0)
	items := html.collectFlexItems(compiled, blockTokens, start, inherited, fallback, cssRules, containerAncestors, contentWd, lineHt)
	if len(items) == 0 {
		return end
	}
	itemsContentHt := 0.0
	for i := range items {
		if i > 0 {
			itemsContentHt += flexMainGap(flexStyle)
		}
		if htmlFlexItemAlign(items[i], flexStyle) == "stretch" {
			items[i] = html.remeasureFlexItemWidth(items[i], contentWd, fallback, cssRules, containerAncestors)
		} else if items[i].outerWd > contentWd {
			items[i] = html.remeasureFlexItemWidth(items[i], contentWd, fallback, cssRules, containerAncestors)
		}
		itemsContentHt += items[i].outerHt
	}
	contentHt := itemsContentHt
	if explicitHt, ok := html.flexContainerContentHeight(blockTokens[0], cssRules, contentWd, ancestors...); ok && explicitHt > contentHt {
		contentHt = explicitHt
	}
	totalHt := containerBox.padding.top + containerBox.padding.bottom + contentHt
	itemOffset, itemGap := htmlFlexMainPlacement(len(items), contentHt, itemsContentHt, flexStyle.justifyContent, flexMainGap(flexStyle))
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
	if containerBox.margin.top > 0 {
		pdf.Ln(containerBox.margin.top)
	}
	if pdf.y+totalHt > pdf.pageBreakTrigger && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
		if !html.addPageFormat() {
			return end
		}
	}
	x, y := pdf.GetXY()
	x += containerBox.margin.left
	fillBox := containerBox.background.Set && !containerBox.background.None
	if fillBox {
		pdf.SetFillColor(containerBox.background.R, containerBox.background.G, containerBox.background.B)
	}
	htmlDrawBoxShadow(pdf, x, y, boxWd, totalHt, containerBox.radius, containerBox.shadow)
	htmlDrawBorderedRect(pdf, x, y, boxWd, totalHt, containerBox.border, containerBox.radius, fillBox, drawR, drawG, drawB, lineWidth)
	pdf.SetCellMargin(0)
	itemY := y + containerBox.padding.top + itemOffset
	for i, item := range items {
		if i > 0 {
			itemY += itemGap
		}
		itemX := htmlFlexColumnItemX(contentWd, item.outerWd, htmlFlexItemAlign(item, flexStyle))
		html.renderFlexItem(item, x+containerBox.padding.left+itemX, itemY, item.outerWd, item.outerHt, lineHt, fallback, cssRules, containerAncestors, drawR, drawG, drawB, lineWidth)
		itemY += item.outerHt
	}
	pdf.SetXY(pdf.lMargin, y+totalHt+containerBox.margin.bottom)
	if containerBox.breakAfter {
		if !html.addPageFormat() {
			return end
		}
	}
	return end
}

func (html *HTML) flexStyle(el HTMLSegmentType, cssRules []htmlCSSRule, relative float64, ancestors ...HTMLSegmentType) htmlFlexStyle {
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	style := htmlFlexStyle{
		direction:      "row",
		justifyContent: "flex-start",
		alignItems:     "stretch",
		alignContent:   "stretch",
	}
	switch strings.ToLower(strings.TrimSpace(decl["flex-direction"])) {
	case "column", "column-reverse":
		style.direction = "column"
	case "row-reverse":
		style.direction = "row-reverse"
	default:
		style.direction = "row"
	}
	switch strings.ToLower(strings.TrimSpace(decl["flex-wrap"])) {
	case "wrap", "wrap-reverse":
		style.wrap = true
	}
	style.gap, _ = parseHTMLBoxLength(decl["gap"], html.pdf, relative)
	if style.gap < 0 {
		style.gap = 0
	}
	style.rowGap = style.gap
	style.columnGap = style.gap
	if rowGap, ok := parseHTMLBoxLength(decl["row-gap"], html.pdf, relative); ok {
		style.rowGap = htmlMaxFloat(rowGap, 0)
	}
	if columnGap, ok := parseHTMLBoxLength(decl["column-gap"], html.pdf, relative); ok {
		style.columnGap = htmlMaxFloat(columnGap, 0)
	}
	switch strings.ToLower(strings.TrimSpace(decl["justify-content"])) {
	case "center", "flex-end", "end", "space-between", "space-around", "space-evenly":
		style.justifyContent = strings.ToLower(strings.TrimSpace(decl["justify-content"]))
	case "right":
		style.justifyContent = "flex-end"
	}
	if alignItems := htmlFlexAlignValue(decl["align-items"]); alignItems != "" && alignItems != "auto" {
		style.alignItems = alignItems
	}
	switch strings.ToLower(strings.TrimSpace(decl["align-content"])) {
	case "flex-start", "start":
		style.alignContent = "flex-start"
	case "center":
		style.alignContent = "center"
	case "flex-end", "end":
		style.alignContent = "flex-end"
	case "stretch", "space-between", "space-around", "space-evenly":
		style.alignContent = strings.ToLower(strings.TrimSpace(decl["align-content"]))
	}
	return style
}

func (html *HTML) collectFlexItems(compiled *CompiledHTML, blockTokens []HTMLSegmentType, containerStart int, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType, contentWd, lineHt float64) []htmlFlexItem {
	var items []htmlFlexItem
	for i := 1; i < len(blockTokens)-1; i++ {
		token := blockTokens[i]
		if token.Cat == 'T' {
			text := strings.TrimSpace(collapseHTMLWhitespace(token.Str))
			if text == "" {
				continue
			}
			el := HTMLSegmentType{Cat: 'O', Str: "div", Attr: map[string]string{}}
			items = append(items, html.measureFlexItem(compiled, el, nil, -1, inherited, fallback, cssRules, ancestors, contentWd, lineHt, text))
			continue
		}
		if token.Cat != 'O' {
			continue
		}
		switch token.Str {
		case "style", "script", "head":
			_, end := htmlCollectElementTokens(blockTokens, i, token.Str)
			i = end
			continue
		}
		childTokens, end := htmlCollectElementTokens(blockTokens, i, token.Str)
		absoluteStart := containerStart + i
		items = append(items, html.measureFlexItem(compiled, token, childTokens, absoluteStart, inherited, fallback, cssRules, ancestors, contentWd, lineHt, ""))
		i = end
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].order < items[j].order
	})
	return items
}

func (html *HTML) measureFlexItem(compiled *CompiledHTML, el HTMLSegmentType, tokens []HTMLSegmentType, start int, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType, contentWd, lineHt float64, textOverride string) htmlFlexItem {
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	style := inherited
	applyHTMLStyleDeclarations(&style, decl, inherited.fontSize, inherited.lineHeight, html.pdf)
	html.applyAttrs(&style, el.Attr, inherited.fontSize, inherited.lineHeight, html.pdf)
	box := htmlBlockBoxFromDeclarations(el, decl, html.pdf, contentWd)
	text := textOverride
	if text == "" {
		if compiled != nil && start >= 0 {
			if cached, ok := compiled.text(start, style.preserveWhitespace); ok {
				text = cached
			}
		}
		if text == "" && len(tokens) > 1 {
			text = htmlPlainTextWithMode(tokens[1:len(tokens)-1], style.preserveWhitespace)
		}
	}
	basis, hasBasis := htmlFlexBasis(decl, html.pdf, contentWd)
	grow := htmlFlexGrow(decl)
	shrink := htmlFlexShrink(decl)
	minWd, _ := parseHTMLBoxLength(decl["min-width"], html.pdf, contentWd)
	maxWd, _ := parseHTMLBoxLength(decl["max-width"], html.pdf, contentWd)
	minHt, _ := parseHTMLBoxLength(decl["min-height"], html.pdf, html.pdf.pageBreakTrigger-html.pdf.GetY())
	maxHt, _ := parseHTMLBoxLength(decl["max-height"], html.pdf, html.pdf.pageBreakTrigger-html.pdf.GetY())
	height, hasHeight := parseHTMLBoxLength(decl["height"], html.pdf, html.pdf.pageBreakTrigger-html.pdf.GetY())
	outerWd := basis
	if !hasBasis || outerWd <= 0 {
		if grow > 0 {
			outerWd = 0
		} else {
			outerWd = htmlMaxFloat(htmlMaxWordWidth(html.pdf, text)+box.padding.left+box.padding.right, contentWd*0.2)
		}
	}
	outerWd = htmlClampFlexSize(outerWd, minWd, maxWd, contentWd)
	innerWd := htmlMaxFloat(outerWd-box.margin.left-box.margin.right-box.padding.left-box.padding.right, 1)
	html.applyTextStyle(style, fallback)
	lines := htmlSplitLines(html.pdf, text, innerWd)
	styleLineHt := htmlEffectiveLineHeight(style, lineHt)
	textHt := float64(len(lines)) * styleLineHt
	if len(lines) == 0 {
		textHt = styleLineHt
	}
	contentHt := textHt
	item := htmlFlexItem{
		el:        el,
		tokens:    tokens,
		start:     start,
		end:       start + len(tokens) - 1,
		text:      text,
		style:     style,
		box:       box,
		outerWd:   outerWd,
		textHt:    textHt,
		lines:     lines,
		grow:      grow,
		shrink:    shrink,
		minWd:     minWd,
		maxWd:     maxWd,
		minHt:     minHt,
		maxHt:     maxHt,
		height:    height,
		hasHeight: hasHeight,
		order:     htmlFlexOrder(decl),
		alignSelf: htmlFlexAlignValue(decl["align-self"]),
		hasWidth:  hasBasis,
		lineHt:    lineHt,
	}
	if len(tokens) > 1 && html.flexItemHasStructuredContent(item, cssRules, ancestors) {
		contentHt += html.flexItemStructuredHeight(item, innerWd, lineHt, fallback, cssRules, ancestors)
	}
	item.outerHt = html.flexItemOuterHeight(item, contentHt)
	if item.end < item.start {
		item.end = item.start
	}
	return item
}

func htmlFlexBasis(decl map[string]string, pdf *Document, relative float64) (float64, bool) {
	if wd, ok := htmlParseFlexBasisValue(decl["flex-basis"], pdf, relative); ok {
		return wd, true
	}
	fields := htmlFlexShorthandFields(decl["flex"])
	if len(fields) == 1 {
		switch fields[0] {
		case "none", "auto", "initial":
		default:
			if wd, ok := htmlParseFlexBasisValue(fields[0], pdf, relative); ok && !htmlFlexTokenIsNumber(fields[0]) {
				return wd, true
			}
		}
	}
	if len(fields) == 2 {
		if wd, ok := htmlParseFlexBasisValue(fields[1], pdf, relative); ok && !htmlFlexTokenIsNumber(fields[1]) {
			return wd, true
		}
	}
	if len(fields) >= 3 {
		if wd, ok := htmlParseFlexBasisValue(fields[2], pdf, relative); ok {
			return wd, true
		}
	}
	if wd, ok := htmlParseFlexBasisValue(decl["width"], pdf, relative); ok {
		return wd, true
	}
	return 0, false
}

func htmlFlexGrow(decl map[string]string) float64 {
	if grow, ok := parsePositiveFloat(decl["flex-grow"]); ok {
		return grow
	}
	fields := htmlFlexShorthandFields(decl["flex"])
	if len(fields) == 0 {
		return 0
	}
	switch fields[0] {
	case "auto":
		return 1
	case "none", "initial":
		return 0
	}
	grow, ok := parsePositiveFloat(fields[0])
	if !ok {
		return 0
	}
	return grow
}

func htmlFlexShrink(decl map[string]string) float64 {
	if shrink, ok := parsePositiveFloat(decl["flex-shrink"]); ok {
		return shrink
	}
	fields := htmlFlexShorthandFields(decl["flex"])
	if len(fields) == 0 {
		return 1
	}
	switch fields[0] {
	case "none":
		return 0
	case "auto", "initial":
		return 1
	}
	if len(fields) < 2 || !htmlFlexTokenIsNumber(fields[1]) {
		return 1
	}
	shrink, ok := parsePositiveFloat(fields[1])
	if !ok {
		return 1
	}
	return shrink
}

func htmlFlexOrder(decl map[string]string) int {
	value := strings.TrimSpace(decl["order"])
	if value == "" {
		return 0
	}
	order, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return order
}

func htmlFlexShorthandFields(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	return strings.Fields(value)
}

func htmlParseFlexBasisValue(value string, pdf *Document, relative float64) (float64, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "auto" {
		return 0, false
	}
	return parseHTMLBoxLength(value, pdf, relative)
}

func htmlFlexTokenIsNumber(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasSuffix(value, "%") {
		return false
	}
	for _, suffix := range []string{"px", "pt", "mm", "cm", "in"} {
		if strings.HasSuffix(value, suffix) {
			return false
		}
	}
	_, ok := parsePositiveFloat(value)
	return ok
}

func parsePositiveFloat(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !isFiniteFloat(n) || n < 0 {
		return 0, false
	}
	return n, true
}

func flexMainGap(style htmlFlexStyle) float64 {
	if style.direction == "column" {
		return style.rowGap
	}
	return style.columnGap
}

func flexCrossGap(style htmlFlexStyle) float64 {
	if style.direction == "column" {
		return style.columnGap
	}
	return style.rowGap
}

func htmlFlexAlignValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto":
		return "auto"
	case "flex-start", "start":
		return "flex-start"
	case "center":
		return "center"
	case "flex-end", "end":
		return "flex-end"
	case "stretch":
		return "stretch"
	default:
		return ""
	}
}

func htmlFlexItemAlign(item htmlFlexItem, style htmlFlexStyle) string {
	if item.alignSelf != "" && item.alignSelf != "auto" {
		return item.alignSelf
	}
	if style.alignItems == "" {
		return "stretch"
	}
	return style.alignItems
}

func htmlClampFlexSize(value, minValue, maxValue, limit float64) float64 {
	if maxValue > 0 && value > maxValue {
		value = maxValue
	}
	if minValue > 0 && value < minValue {
		value = minValue
	}
	if limit > 0 && value > limit {
		value = limit
	}
	if value < 0 {
		return 0
	}
	return value
}

func htmlFlexItemsMainSize(items []htmlFlexItem, gap float64) float64 {
	used := 0.0
	for i, item := range items {
		if i > 0 {
			used += gap
		}
		used += item.outerWd
	}
	return used
}

func (html *HTML) flexLines(items []htmlFlexItem, contentWd float64, style htmlFlexStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) []htmlFlexLine {
	if style.direction == "row-reverse" {
		slices.Reverse(items)
	}
	if contentWd <= 0 {
		return nil
	}
	gap := flexMainGap(style)
	if !style.wrap {
		html.distributeFlexGrow(items, contentWd, gap, fallback, cssRules, ancestors)
		line := htmlFlexLine{items: items}
		line.wd, line.ht = html.flexLineSize(items, gap)
		if line.wd > contentWd && line.wd > 0 {
			scale := contentWd / line.wd
			for i := range line.items {
				line.items[i].outerWd *= scale
				line.items[i] = html.remeasureFlexItemWidth(line.items[i], line.items[i].outerWd, fallback, cssRules, ancestors)
			}
			line.wd, line.ht = html.flexLineSize(line.items, gap)
		}
		return []htmlFlexLine{line}
	}
	var lines []htmlFlexLine
	current := htmlFlexLine{}
	for _, item := range items {
		nextWd := item.outerWd
		if len(current.items) > 0 {
			nextWd += gap
		}
		if len(current.items) > 0 && current.wd+nextWd > contentWd {
			html.distributeFlexGrow(current.items, contentWd, gap, fallback, cssRules, ancestors)
			current.wd, current.ht = html.flexLineSize(current.items, gap)
			lines = append(lines, current)
			current = htmlFlexLine{}
		}
		current.items = append(current.items, item)
		current.wd += nextWd
		if item.outerHt > current.ht {
			current.ht = item.outerHt
		}
	}
	if len(current.items) > 0 {
		html.distributeFlexGrow(current.items, contentWd, gap, fallback, cssRules, ancestors)
		current.wd, current.ht = html.flexLineSize(current.items, gap)
		lines = append(lines, current)
	}
	return lines
}

func (html *HTML) distributeFlexGrow(items []htmlFlexItem, contentWd, gap float64, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) {
	if len(items) == 0 {
		return
	}
	used := 0.0
	totalGrow := 0.0
	for i := range items {
		if i > 0 {
			used += gap
		}
		used += items[i].outerWd
		totalGrow += items[i].grow
	}
	if used > contentWd {
		html.distributeFlexShrink(items, contentWd, gap, fallback, cssRules, ancestors)
		return
	}
	if totalGrow <= 0 {
		flexible := 0
		for i := range items {
			if !items[i].hasWidth {
				flexible++
			}
		}
		if flexible == 0 || used >= contentWd {
			return
		}
		extra := (contentWd - used) / float64(flexible)
		for i := range items {
			if !items[i].hasWidth {
				items[i] = html.remeasureFlexItemWidth(items[i], items[i].outerWd+extra, fallback, cssRules, ancestors)
			}
		}
		return
	}
	if used >= contentWd {
		return
	}
	remaining := contentWd - used
	for i := range items {
		if items[i].grow <= 0 {
			continue
		}
		items[i] = html.remeasureFlexItemWidth(items[i], items[i].outerWd+remaining*items[i].grow/totalGrow, fallback, cssRules, ancestors)
	}
}

func (html *HTML) distributeFlexShrink(items []htmlFlexItem, contentWd, gap float64, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) {
	for iter := 0; iter < len(items); iter++ {
		used := htmlFlexItemsMainSize(items, gap)
		overflow := used - contentWd
		if overflow <= 0 {
			return
		}
		totalShrink := 0.0
		for i := range items {
			minWd := items[i].minWd
			if items[i].shrink <= 0 || items[i].outerWd <= minWd {
				continue
			}
			totalShrink += items[i].shrink * items[i].outerWd
		}
		if totalShrink <= 0 {
			return
		}
		changed := false
		for i := range items {
			minWd := items[i].minWd
			if items[i].shrink <= 0 || items[i].outerWd <= minWd {
				continue
			}
			reduction := overflow * (items[i].shrink * items[i].outerWd) / totalShrink
			next := items[i].outerWd - reduction
			if next < minWd {
				next = minWd
			}
			if next < 0 {
				next = 0
			}
			if next < items[i].outerWd {
				items[i] = html.remeasureFlexItemWidth(items[i], next, fallback, cssRules, ancestors)
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func (html *HTML) remeasureFlexItemWidth(item htmlFlexItem, outerWd float64, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) htmlFlexItem {
	item.outerWd = htmlClampFlexSize(outerWd, item.minWd, item.maxWd, 0)
	innerWd := htmlMaxFloat(item.outerWd-item.box.margin.left-item.box.margin.right-item.box.padding.left-item.box.padding.right, 1)
	html.applyTextStyle(item.style, fallback)
	item.lines = htmlSplitLines(html.pdf, item.text, innerWd)
	styleLineHt := htmlEffectiveLineHeight(item.style, item.lineHt)
	if styleLineHt <= 0 {
		styleLineHt = html.pdf.fontSizePt
	}
	item.textHt = float64(len(item.lines)) * styleLineHt
	contentHt := item.textHt
	if len(item.lines) == 0 {
		contentHt = styleLineHt
	}
	if len(item.tokens) > 1 && html.flexItemHasStructuredContent(item, cssRules, ancestors) {
		contentHt += html.flexItemStructuredHeight(item, innerWd, item.lineHt, fallback, cssRules, ancestors)
	}
	item.outerHt = html.flexItemOuterHeight(item, contentHt)
	return item
}

func (html *HTML) flexLineSize(items []htmlFlexItem, gap float64) (float64, float64) {
	wd, ht := 0.0, 0.0
	for i, item := range items {
		if i > 0 {
			wd += gap
		}
		wd += item.outerWd
		if item.outerHt > ht {
			ht = item.outerHt
		}
	}
	return wd, ht
}

func (html *HTML) flexLinePlacement(line htmlFlexLine, contentWd float64, style htmlFlexStyle) (float64, float64) {
	gap := flexMainGap(style)
	free := contentWd - line.wd
	if free < 0 {
		free = 0
	}
	switch style.justifyContent {
	case "center":
		return free / 2, gap
	case "flex-end", "end":
		return free, gap
	case "space-between":
		if len(line.items) > 1 {
			return 0, gap + free/float64(len(line.items)-1)
		}
	case "space-around":
		if len(line.items) > 0 {
			space := free / float64(len(line.items))
			return space / 2, gap + space
		}
	case "space-evenly":
		if len(line.items) > 0 {
			space := free / float64(len(line.items)+1)
			return space, gap + space
		}
	}
	return 0, gap
}

func htmlFlexLinesCrossSize(lines []htmlFlexLine, style htmlFlexStyle) float64 {
	if len(lines) == 0 {
		return 0
	}
	total := 0.0
	gap := flexCrossGap(style)
	for i, line := range lines {
		if i > 0 {
			total += gap
		}
		total += line.ht
	}
	return total
}

func (html *HTML) distributeFlexLineCrossSpace(lines []htmlFlexLine, contentHt, usedHt float64, style htmlFlexStyle) {
	if len(lines) == 0 || style.alignContent != "stretch" {
		return
	}
	extra := contentHt - usedHt
	if extra <= 0 {
		return
	}
	add := extra / float64(len(lines))
	for i := range lines {
		lines[i].ht += add
	}
}

func htmlFlexAlignContentPlacement(lineCount int, contentHt, usedHt float64, style htmlFlexStyle) (float64, float64) {
	gap := flexCrossGap(style)
	free := contentHt - usedHt
	if free < 0 {
		free = 0
	}
	switch style.alignContent {
	case "center":
		return free / 2, gap
	case "flex-end", "end":
		return free, gap
	case "space-between":
		if lineCount > 1 {
			return 0, gap + free/float64(lineCount-1)
		}
	case "space-around":
		if lineCount > 0 {
			space := free / float64(lineCount)
			return space / 2, gap + space
		}
	case "space-evenly":
		if lineCount > 0 {
			space := free / float64(lineCount+1)
			return space, gap + space
		}
	}
	return 0, gap
}

func htmlFlexMainPlacement(itemCount int, contentHt, usedHt float64, justifyContent string, gap float64) (float64, float64) {
	free := contentHt - usedHt
	if free < 0 {
		free = 0
	}
	switch justifyContent {
	case "center":
		return free / 2, gap
	case "flex-end", "end":
		return free, gap
	case "space-between":
		if itemCount > 1 {
			return 0, gap + free/float64(itemCount-1)
		}
	case "space-around":
		if itemCount > 0 {
			space := free / float64(itemCount)
			return space / 2, gap + space
		}
	case "space-evenly":
		if itemCount > 0 {
			space := free / float64(itemCount+1)
			return space, gap + space
		}
	}
	return 0, gap
}

func (html *HTML) flexContainerContentHeight(el HTMLSegmentType, cssRules []htmlCSSRule, relative float64, ancestors ...HTMLSegmentType) (float64, bool) {
	decl := html.elementDeclarations(el, cssRules, ancestors...)
	if height, ok := parseHTMLBoxLength(decl["height"], html.pdf, html.pdf.pageBreakTrigger-html.pdf.GetY()); ok && height > 0 {
		return height, true
	}
	return 0, false
}

func (html *HTML) flexItemOuterHeight(item htmlFlexItem, contentHt float64) float64 {
	if item.hasHeight && item.height > contentHt {
		contentHt = item.height
	}
	contentHt = htmlClampFlexSize(contentHt, item.minHt, item.maxHt, 0)
	return contentHt + item.box.margin.top + item.box.margin.bottom + item.box.padding.top + item.box.padding.bottom
}

func (html *HTML) flexItemHasStructuredContent(item htmlFlexItem, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) bool {
	if len(item.tokens) < 3 {
		return false
	}
	elementStack := appendHTMLAncestors(ancestors, item.el)
	for _, token := range item.tokens[1 : len(item.tokens)-1] {
		if token.Cat != 'O' {
			continue
		}
		switch token.Str {
		case "table", "ul", "ol":
			return true
		case "div", "section", "article", "header", "footer", "figure", "figcaption":
			if html.elementDisplayFlex(token, cssRules, elementStack...) {
				return true
			}
		}
	}
	return false
}

func (html *HTML) flexItemStructuredHeight(item htmlFlexItem, wd, lineHt float64, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) float64 {
	if len(item.tokens) < 3 {
		return 0
	}
	pdf := html.pdf
	oldLeft, oldRight := pdf.lMargin, pdf.rMargin
	oldX, oldY := pdf.GetXY()
	textR, textG, textB := pdf.GetTextColor()
	fillR, fillG, fillB := pdf.GetFillColor()
	drawR, drawG, drawB := pdf.GetDrawColor()
	lineWidth := pdf.GetLineWidth()
	cellMargin := pdf.GetCellMargin()
	defer func() {
		pdf.lMargin, pdf.rMargin = oldLeft, oldRight
		pdf.SetXY(oldX, oldY)
		pdf.SetTextColor(textR, textG, textB)
		pdf.SetFillColor(fillR, fillG, fillB)
		pdf.SetDrawColor(drawR, drawG, drawB)
		pdf.SetLineWidth(lineWidth)
		pdf.SetCellMargin(cellMargin)
	}()
	pdf.lMargin = 0
	pdf.rMargin = pdf.w - wd
	pdf.SetXY(0, oldY)
	total := 0.0
	effectiveLineHt := htmlEffectiveLineHeight(item.style, lineHt)
	inner := item.tokens[1 : len(item.tokens)-1]
	elementStack := appendHTMLAncestors(ancestors, item.el)
	for i := 0; i < len(inner); i++ {
		token := inner[i]
		if token.Cat != 'O' {
			continue
		}
		switch token.Str {
		case "table":
			total += html.tableHeight(inner, i, effectiveLineHt, item.style, fallback, cssRules, elementStack)
			i = htmlSkipElement(inner, i, token.Str)
		case "ul", "ol":
			total += effectiveLineHt
		case "div", "section", "article", "header", "footer", "figure", "figcaption":
			if html.elementDisplayFlex(token, cssRules, elementStack...) {
				childTokens, end := htmlCollectElementTokens(inner, i, token.Str)
				if len(childTokens) > 2 {
					childItems := html.collectFlexItems(nil, childTokens, 0, item.style, fallback, cssRules, appendHTMLAncestors(elementStack, token), wd, effectiveLineHt)
					childStyle := html.flexStyle(token, cssRules, wd, elementStack...)
					if childStyle.direction == "column" {
						for childIndex, child := range childItems {
							if childIndex > 0 {
								total += flexMainGap(childStyle)
							}
							total += child.outerHt
						}
					} else {
						lines := html.flexLines(childItems, wd, childStyle, fallback, cssRules, appendHTMLAncestors(elementStack, token))
						total += htmlFlexLinesCrossSize(lines, childStyle)
					}
				}
				i = end
			}
		}
	}
	return total
}

func htmlFlexItemCrossOffset(lineHt, itemHt float64, alignItems string) float64 {
	free := lineHt - itemHt
	if free <= 0 {
		return 0
	}
	switch alignItems {
	case "center":
		return free / 2
	case "flex-end", "end":
		return free
	default:
		return 0
	}
}

func htmlFlexColumnItemX(contentWd, itemWd float64, alignItems string) float64 {
	free := contentWd - itemWd
	if free <= 0 {
		return 0
	}
	switch alignItems {
	case "center":
		return free / 2
	case "flex-end", "end":
		return free
	default:
		return 0
	}
}

func (html *HTML) renderFlexItem(item htmlFlexItem, x, y, outerWd, outerHt, lineHt float64, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType, drawR, drawG, drawB int, lineWidth float64) {
	pdf := html.pdf
	box := item.box
	boxX := x + box.margin.left
	boxY := y + box.margin.top
	boxWd := htmlMaxFloat(outerWd-box.margin.left-box.margin.right, 0)
	boxHt := htmlMaxFloat(outerHt-box.margin.top-box.margin.bottom, 0)
	fillBox := box.background.Set && !box.background.None
	if fillBox {
		pdf.SetFillColor(box.background.R, box.background.G, box.background.B)
	}
	htmlDrawBoxShadow(pdf, boxX, boxY, boxWd, boxHt, box.radius, box.shadow)
	htmlDrawBorderedRect(pdf, boxX, boxY, boxWd, boxHt, box.border, box.radius, fillBox, drawR, drawG, drawB, lineWidth)
	contentWd := htmlMaxFloat(boxWd-box.padding.left-box.padding.right, 1)
	contentX := boxX + box.padding.left
	contentY := boxY + box.padding.top
	html.applyTextStyle(item.style, fallback)
	pdf.SetXY(contentX, contentY)
	if len(item.tokens) > 1 {
		html.renderFlexItemInlineContent(item, contentX, contentY, contentWd, lineHt, fallback, cssRules, ancestors)
		return
	}
	htmlRenderSplitLines(pdf, contentWd, htmlEffectiveLineHeight(item.style, lineHt), item.lines, item.style.align)
}

func (html *HTML) renderFlexItemInlineContent(item htmlFlexItem, x, y, wd, lineHt float64, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) {
	pdf := html.pdf
	oldLeft, oldRight := pdf.lMargin, pdf.rMargin
	defer func() {
		pdf.lMargin, pdf.rMargin = oldLeft, oldRight
	}()
	pdf.lMargin = x
	pdf.rMargin = pdf.w - x - wd
	pdf.SetXY(x, y)

	stack := []htmlTextStyle{item.style}
	tagStack := []string{item.el.Str}
	elementStack := appendHTMLAncestors(ancestors, item.el)
	current := func() htmlTextStyle {
		return stack[len(stack)-1]
	}
	push := func(tag string, st htmlTextStyle) {
		stack = append(stack, st)
		tagStack = append(tagStack, tag)
	}
	pop := func(tag string) {
		for len(stack) > 1 {
			top := tagStack[len(tagStack)-1]
			stack = stack[:len(stack)-1]
			tagStack = tagStack[:len(tagStack)-1]
			if top == tag {
				return
			}
		}
	}
	popElement := func(tag string) {
		for len(elementStack) > 0 {
			top := elementStack[len(elementStack)-1]
			elementStack = elementStack[:len(elementStack)-1]
			if top.Str == tag {
				return
			}
		}
	}
	writeText := func(text string) {
		if text == "" {
			return
		}
		st := current()
		if !st.preserveWhitespace {
			text = collapseHTMLWhitespace(text)
		}
		if text == "" {
			return
		}
		html.applyTextStyle(st, fallback)
		textLineHt := htmlEffectiveLineHeight(st, lineHt)
		if st.href != "" {
			pdf.WriteLinkString(textLineHt, text, st.href)
			return
		}
		if st.script != 0 {
			pdf.SubWrite(textLineHt, text, st.fontSize*0.75, float64(st.script)*st.fontSize*0.35, 0, "")
			return
		}
		pdf.Write(textLineHt, text)
	}

	inner := item.tokens[1 : len(item.tokens)-1]
	for i := 0; i < len(inner); i++ {
		token := inner[i]
		switch token.Cat {
		case 'T':
			writeText(token.Str)
		case 'O':
			st := current()
			pushStyle := true
			switch token.Str {
			case "b", "strong":
				st.bold = true
			case "i", "em":
				st.italic = true
			case "u", "ins":
				st.underline = true
			case "s", "strike", "del":
				st.strike = true
			case "sup":
				st.script = 1
			case "sub":
				st.script = -1
			case "code", "kbd", "samp":
				st.fontFamily = "Courier"
			case "a":
				href, err := htmlLinkTarget(token.Attr["href"])
				if err != nil {
					html.pdf.SetError(err)
					pushStyle = false
				} else {
					st.href = href
				}
			case "br":
				pdf.Ln(htmlEffectiveLineHeight(st, lineHt))
				pushStyle = false
			case "table":
				if pdf.GetX() != pdf.lMargin {
					pdf.Ln(htmlEffectiveLineHeight(st, lineHt))
				}
				i = html.writeTable(inner, i, htmlEffectiveLineHeight(st, lineHt), st, fallback, cssRules, elementStack)
				pdf.SetX(pdf.lMargin)
				pushStyle = false
			case "p", "div", "section", "article", "header", "footer", "figure", "figcaption":
				if html.elementDisplayFlex(token, cssRules, elementStack...) {
					if pdf.GetX() != pdf.lMargin {
						pdf.Ln(htmlEffectiveLineHeight(st, lineHt))
					}
					i = html.writeCompiledFlexBox(nil, inner, i, htmlEffectiveLineHeight(st, lineHt), st, fallback, cssRules, elementStack)
					pdf.SetX(pdf.lMargin)
					pushStyle = false
				} else if pdf.GetX() != pdf.lMargin {
					pdf.Ln(htmlEffectiveLineHeight(st, lineHt))
				}
			case "style", "script", "head":
				i = htmlSkipElement(inner, i, token.Str)
				pushStyle = false
			case "img":
				writeText(token.Attr["alt"])
				pushStyle = false
			}
			applyHTMLCSSRules(&st, token, cssRules, current().fontSize, current().lineHeight, pdf, elementStack...)
			html.applyAttrs(&st, token.Attr, current().fontSize, current().lineHeight, pdf)
			if pushStyle {
				push(token.Str, st)
				elementStack = append(elementStack, token)
			}
		case 'C':
			if htmlClosePops(token.Str) {
				pop(token.Str)
				popElement(token.Str)
			}
			switch token.Str {
			case "p", "div", "section", "article", "header", "footer", "figure", "figcaption", "pre", "h1", "h2", "h3", "h4", "h5", "h6", "dt", "dd":
				pdf.Ln(htmlEffectiveLineHeight(current(), lineHt))
			}
			html.applyTextStyle(current(), fallback)
		}
	}
}
