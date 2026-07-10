// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "strings"

// htmlRenderSession owns the mutable stacks used during one compiled HTML
// render. Keeping this state together makes nesting and cleanup explicit while
// leaving HTML reusable across render calls.
type htmlRenderSession struct {
	html         *HTML
	compiled     *CompiledHTML
	lineHt       float64
	defaultColor CSSColorType
	base         htmlTextStyle
	styles       []htmlTextStyle
	tags         []string
	elements     []HTMLSegmentType
	lists        []htmlListState
}

func newHTMLRenderSession(html *HTML, compiled *CompiledHTML, lineHt float64) *htmlRenderSession {
	textR, textG, textB := html.pdf.GetTextColor()
	fontPt, _ := html.pdf.GetFontSize()
	base := htmlTextStyle{align: "L", fontSize: fontPt, lineHeight: lineHt}
	return &htmlRenderSession{
		html:         html,
		compiled:     compiled,
		lineHt:       lineHt,
		defaultColor: CSSColorType{R: textR, G: textG, B: textB, Set: true},
		base:         base,
		styles:       []htmlTextStyle{base},
		tags:         []string{""},
	}
}

func (session *htmlRenderSession) render() {
	tokens := session.compiled.tokens
	session.html.logUnsupportedHTML(tokens)
	for index := 0; index < len(tokens); index++ {
		element := tokens[index]
		switch element.Cat {
		case 'T':
			text := element.Str
			if !session.current().preserveWhitespace {
				text = collapseHTMLWhitespace(text)
			}
			session.writeText(text)
		case 'O':
			next, abort := session.openElement(index, element)
			if abort {
				return
			}
			index = next
		case 'C':
			session.closeElement(element.Str)
		}
	}
	session.apply(session.base)
}

func (session *htmlRenderSession) current() htmlTextStyle {
	return session.styles[len(session.styles)-1]
}

func (session *htmlRenderSession) apply(style htmlTextStyle) {
	session.html.applyTextStyle(style, session.defaultColor)
}

func (session *htmlRenderSession) push(tag string, style htmlTextStyle, element HTMLSegmentType) {
	session.styles = append(session.styles, style)
	session.tags = append(session.tags, tag)
	session.elements = append(session.elements, element)
}

func (session *htmlRenderSession) pop(tag string) {
	for len(session.styles) > 1 {
		top := session.tags[len(session.tags)-1]
		session.styles = session.styles[:len(session.styles)-1]
		session.tags = session.tags[:len(session.tags)-1]
		if top == tag {
			break
		}
	}
	for len(session.elements) > 0 {
		top := session.elements[len(session.elements)-1]
		session.elements = session.elements[:len(session.elements)-1]
		if top.Str == tag {
			return
		}
	}
}

func (session *htmlRenderSession) lineBreak() {
	session.html.pdf.Ln(htmlEffectiveLineHeight(session.current(), session.lineHt))
}

func (session *htmlRenderSession) blockBreak() {
	if session.html.pdf.GetX() != session.html.pdf.lMargin {
		session.lineBreak()
	}
}

func (session *htmlRenderSession) writeText(text string) {
	if text == "" {
		return
	}
	style := session.current()
	lineHt := htmlEffectiveLineHeight(style, session.lineHt)
	session.apply(style)
	if style.href == "" {
		session.writeCurrentText(text, style, lineHt, "")
		return
	}

	linkBold, linkItalic, linkUnderline := style.bold, style.italic, style.underline
	if session.html.Link.Bold {
		linkBold = true
	}
	if session.html.Link.Italic {
		linkItalic = true
	}
	if session.html.Link.Underscore {
		linkUnderline = true
	}
	session.apply(htmlTextStyle{
		bold:               linkBold,
		italic:             linkItalic,
		underline:          linkUnderline,
		strike:             style.strike,
		preserveWhitespace: style.preserveWhitespace,
		fontFamily:         style.fontFamily,
		fontSize:           style.fontSize,
		lineHeight:         style.lineHeight,
		verticalAlign:      style.verticalAlign,
		color:              CSSColorType{R: session.html.Link.ClrR, G: session.html.Link.ClrG, B: session.html.Link.ClrB, Set: true},
		script:             style.script,
	})
	session.writeCurrentText(text, style, lineHt, style.href)
	session.apply(style)
}

func (session *htmlRenderSession) writeCurrentText(text string, style htmlTextStyle, lineHt float64, link string) {
	if style.role != "" && link == "" {
		session.html.pdf.SetNextTextRole(style.role)
	}
	if style.script != 0 {
		session.html.pdf.SubWrite(lineHt, text, style.fontSize*0.75, float64(style.script)*style.fontSize*0.35, 0, link)
		return
	}
	if link != "" {
		session.html.pdf.WriteLinkString(lineHt, text, link)
		return
	}
	if style.align == "C" || style.align == "R" {
		session.html.pdf.WriteAligned(0, lineHt, text, style.align)
		return
	}
	session.html.pdf.Write(lineHt, text)
}

func (session *htmlRenderSession) openElement(index int, element HTMLSegmentType) (int, bool) {
	html := session.html
	compiled := session.compiled
	style := session.current()
	pushStyle := true
	openedList := false

	switch element.Str {
	case "b", "strong":
		style.bold = true
	case "i", "em":
		style.italic = true
	case "u", "ins":
		style.underline = true
	case "s", "strike", "del":
		style.strike = true
	case "sup":
		style.script = 1
	case "sub":
		style.script = -1
	case "code", "kbd", "samp":
		style.fontFamily = "Courier"
	case "pre":
		session.blockBreak()
		style.fontFamily = "Courier"
		style.preserveWhitespace = true
	case "a":
		href, err := htmlLinkTarget(element.Attr["href"])
		if err != nil {
			html.pdf.SetError(err)
			pushStyle = false
		} else {
			style.href = href
		}
	case "br":
		session.lineBreak()
		pushStyle = false
	case "img":
		session.writeImage(index, element.Attr, style)
		pushStyle = false
	case "table":
		index = html.writeCompiledTable(compiled, index, session.lineHt, style, session.defaultColor, compiled.cssRules, session.elements)
		pushStyle = false
	case "svg":
		index = html.writeCompiledInlineSVG(compiled, compiled.tokens, index, session.lineHt, style)
		pushStyle = false
	case "hr":
		html.writeHorizontalRule(element, compiled.cssRules, session.lineHt, session.elements)
		pushStyle = false
	case "style", "script", "head":
		index = compiled.skipElement(index, element.Str)
		pushStyle = false
	case "p", "div", "section", "article", "header", "footer", "figure":
		if element.Str == "figure" && !session.prepareFigure(index, style) {
			return index, true
		}
		if html.elementDisplayFlex(element, compiled.cssRules, session.elements...) {
			index = html.writeCompiledFlexBox(compiled, compiled.tokens, index, session.lineHt, style, session.defaultColor, compiled.cssRules, session.elements)
			pushStyle = false
		} else if html.blockHasBoxStyle(element, compiled.cssRules, session.elements...) {
			index = html.writeCompiledBlockBox(compiled, compiled.tokens, index, session.lineHt, style, session.defaultColor, compiled.cssRules, session.elements)
			pushStyle = false
		} else {
			session.blockBreak()
		}
		if element.Str == "p" {
			style.role = taggedRoleP
		}
	case "figcaption":
		session.blockBreak()
		style.role = "Caption"
		style.italic = true
		if style.align == "" || style.align == "L" {
			style.align = "C"
		}
		if style.fontSize > 1 {
			style.fontSize *= 0.9
		}
	case "dl":
		session.blockBreak()
	case "dt":
		session.blockBreak()
		style.role = "Lbl"
		style.bold = true
	case "dd":
		session.blockBreak()
		style.role = "LBody"
		html.pdf.SetX(html.pdf.lMargin + session.lineHt*1.5)
	case "center":
		session.blockBreak()
		style.align = "C"
	case "right":
		session.blockBreak()
		style.align = "R"
	case "left":
		session.blockBreak()
		style.align = "L"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		html.keepCompiledHeadingWithNext(compiled, compiled.tokens, index, session.lineHt, style, session.defaultColor, compiled.cssRules, session.elements)
		session.blockBreak()
		style.bold = true
		style.fontSize = htmlHeadingFontSize(session.base.fontSize, element.Str)
		style.role = strings.ToUpper(element.Str)
	case "ul", "ol":
		session.blockBreak()
		html.pdf.BeginStructure("L")
		style.list = element.Str
		openedList = true
	case "li":
		session.openListItem(&style)
	}

	html.applyCompiledElementStyle(compiled, index, &style, element, compiled.cssRules, session.base.fontSize, session.base.lineHeight, session.elements...)
	if openedList {
		session.lists = append(session.lists, htmlListStateFromElement(style, element.Attr, session.lineHt))
	}
	if pushStyle {
		session.push(element.Str, style, element)
	}
	return index, false
}

func (session *htmlRenderSession) prepareFigure(index int, style htmlTextStyle) bool {
	html := session.html
	figureHt := html.figureHeight(session.compiled.tokens, index, session.lineHt, style, session.defaultColor)
	if figureHt <= 0 {
		return true
	}
	pageContentHt := html.pdf.pageBreakTrigger - html.pdf.tMargin
	if figureHt > pageContentHt || html.pdf.y+figureHt <= html.pdf.pageBreakTrigger || html.pdf.inHeader || html.pdf.inFooter || !html.pdf.acceptPageBreak() {
		return true
	}
	return html.addPageFormat()
}

func (session *htmlRenderSession) openListItem(style *htmlTextStyle) {
	html := session.html
	session.blockBreak()
	html.pdf.BeginStructure("LI")
	if len(session.lists) > 0 {
		list := &session.lists[len(session.lists)-1]
		list.counter++
		html.pdf.SetX(html.pdf.lMargin + float64(len(session.lists)-1)*list.indent)
		html.pdf.SetNextTextRole("Lbl")
		session.writeText(list.marker())
	}
	html.pdf.BeginStructure("LBody")
	style.role = "LBody"
}

func (session *htmlRenderSession) closeElement(tag string) {
	if htmlClosePops(tag) {
		session.pop(tag)
	}
	switch tag {
	case "p", "div", "section", "article", "header", "footer", "figure", "figcaption", "pre", "h1", "h2", "h3", "h4", "h5", "h6", "dt", "dd":
		session.lineBreak()
	case "li":
		session.html.pdf.EndStructure()
		session.html.pdf.EndStructure()
		session.lineBreak()
	case "ul", "ol":
		if len(session.lists) > 0 {
			session.lists = session.lists[:len(session.lists)-1]
		}
		session.html.pdf.EndStructure()
		session.lineBreak()
	case "dl":
		session.lineBreak()
	}
	session.apply(session.current())
}

func (session *htmlRenderSession) writeImage(tokenIndex int, attrs map[string]string, style htmlTextStyle) {
	html := session.html
	src := strings.TrimSpace(attrs["src"])
	if src == "" {
		session.writeText(attrs["alt"])
		return
	}
	tag := taggedContentOptions{
		Role:     taggedRoleFigure,
		AltText:  attrs["alt"],
		Artifact: strings.TrimSpace(attrs["alt"]) == "",
	}
	session.blockBreak()
	margin := htmlBoxEdgesFromDeclarations(html.styleDeclarations(attrs), "margin", html.pdf, html.pdf.w-html.pdf.lMargin-html.pdf.rMargin)
	if margin.top > 0 {
		html.pdf.Ln(margin.top)
	}
	availableWd := html.pdf.w - html.pdf.rMargin - html.pdf.GetX() - margin.left - margin.right
	if availableWd < 0 {
		availableWd = 0
	}
	pageHt := html.pdf.h - html.pdf.bMargin - html.pdf.GetY()
	wd, _ := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "width"), attrs["width"]), html.pdf, availableWd)
	ht, _ := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "height"), attrs["height"]), html.pdf, pageHt)
	boxWd, boxHt := wd, ht
	maxWd, hasMaxWd := parseHTMLBoxLength(html.styleValue(attrs, "max-width"), html.pdf, availableWd)
	maxHt, hasMaxHt := parseHTMLBoxLength(html.styleValue(attrs, "max-height"), html.pdf, pageHt)
	name, options, err := html.compiledHTMLImageSource(session.compiled, tokenIndex, src)
	if err != nil {
		html.pdf.SetError(err)
		return
	}
	info := html.pdf.RegisterImageOptions(name, options)
	if html.pdf.err != nil {
		return
	}
	wd, ht = htmlResolvedImageSize(info, html.pdf, wd, ht)
	if boxWd <= 0 {
		boxWd = wd
	}
	if boxHt <= 0 {
		boxHt = ht
	}
	if hasMaxWd && maxWd > 0 && wd > maxWd {
		ratio := maxWd / wd
		wd *= ratio
		ht *= ratio
		boxWd = minFloat(boxWd, maxWd)
		boxHt *= ratio
	}
	if hasMaxHt && maxHt > 0 && ht > maxHt {
		ratio := maxHt / ht
		wd *= ratio
		ht *= ratio
		boxHt = minFloat(boxHt, maxHt)
		boxWd *= ratio
	}
	fit := html.imageObjectFit(attrs)
	drawX, drawY, drawWd, drawHt, flowWd, flowHt := htmlImageFitBox(info, html.pdf, wd, ht, boxWd, boxHt, fit)
	x := html.pdf.GetX() + margin.left
	switch html.imageAlign(attrs, style.align) {
	case "C":
		x += htmlMaxFloat((availableWd-flowWd)/2, 0)
	case "R":
		x += htmlMaxFloat(availableWd-flowWd, 0)
	}
	if fit == "cover" {
		session.writeFittedImage(info, options, style, tag, x, drawX, drawY, drawWd, drawHt, flowWd, flowHt, margin.bottom, true)
		return
	}
	if fit == "contain" {
		session.writeFittedImage(info, options, style, tag, x, drawX, drawY, drawWd, drawHt, flowWd, flowHt, margin.bottom, false)
		return
	}
	html.pdf.imageOut(info, x+drawX, 0, drawWd, drawHt, options.AllowNegativePosition, true, 0, style.href, tag)
	if margin.bottom > 0 {
		html.pdf.Ln(margin.bottom)
	}
}

func (session *htmlRenderSession) writeFittedImage(info *ImageInfo, options ImageOptions, style htmlTextStyle, tag taggedContentOptions, x, drawX, drawY, drawWd, drawHt, flowWd, flowHt, marginBottom float64, clip bool) {
	html := session.html
	y := html.pdf.GetY()
	if y+flowHt > html.pdf.pageBreakTrigger && !html.pdf.inHeader && !html.pdf.inFooter && html.pdf.acceptPageBreak() {
		x2 := html.pdf.GetX()
		if !html.addPageFormat() {
			return
		}
		html.pdf.x = x2
		y = html.pdf.GetY()
	}
	if clip {
		html.pdf.ClipRect(x, y, flowWd, flowHt, false)
	}
	html.pdf.imageOut(info, x+drawX, y+drawY, drawWd, drawHt, options.AllowNegativePosition, false, 0, style.href, tag)
	if clip {
		html.pdf.ClipEnd()
	}
	if style.href != "" {
		html.pdf.newLink(x, y, flowWd, flowHt, 0, style.href)
	}
	html.pdf.SetY(y + flowHt + marginBottom)
	html.pdf.SetX(html.pdf.lMargin)
}
