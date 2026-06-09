// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"strconv"
	"strings"
	"unicode"
)

type htmlCSSRule struct {
	selectors    []htmlCSSSelector
	declarations map[string]string
}

type htmlCSSSelector struct{ parts []htmlCSSSelectorPart }

type htmlCSSSelectorPart struct {
	tag    string
	id     string
	class  string
	direct bool
}

type htmlBlockBoxStyle struct {
	background       CSSColorType
	border           htmlBorderStyle
	radius           htmlBorderRadiusStyle
	shadow           htmlBoxShadowStyle
	breakBefore      bool
	breakAfter       bool
	breakInsideAvoid bool
	padding          htmlBoxEdges
	margin           htmlBoxEdges
}

type htmlBoxEdges struct {
	top    float64
	right  float64
	bottom float64
	left   float64
}

type htmlBorderStyle struct {
	enabled      bool
	width        float64
	color        CSSColorType
	sideSpecific bool
	top          htmlBorderSideStyle
	right        htmlBorderSideStyle
	bottom       htmlBorderSideStyle
	left         htmlBorderSideStyle
}

type htmlBorderSideStyle struct {
	enabled bool
	width   float64
	color   CSSColorType
}

type htmlBorderRadiusStyle struct {
	topLeft     float64
	topRight    float64
	bottomRight float64
	bottomLeft  float64
}

type htmlBoxShadowStyle struct {
	enabled bool
	offsetX float64
	offsetY float64
	blur    float64
	spread  float64
	color   CSSColorType
	alpha   float64
}

func (edges htmlBoxEdges) hasAny() bool {
	return edges.top > 0 || edges.right > 0 || edges.bottom > 0 || edges.left > 0
}

func (radius htmlBorderRadiusStyle) hasAny() bool {
	return radius.topLeft > 0 || radius.topRight > 0 || radius.bottomRight > 0 || radius.bottomLeft > 0
}

func (radius htmlBorderRadiusStyle) clamped(w, h float64) htmlBorderRadiusStyle {
	maxRadius := minFloat(w, h) / 2
	radius.topLeft = clampFloat(radius.topLeft, 0, maxRadius)
	radius.topRight = clampFloat(radius.topRight, 0, maxRadius)
	radius.bottomRight = clampFloat(radius.bottomRight, 0, maxRadius)
	radius.bottomLeft = clampFloat(radius.bottomLeft, 0, maxRadius)
	return radius
}

func (html *HTML) cachedStyleDeclarations(style string) map[string]string {
	if strings.TrimSpace(style) == "" || !strings.Contains(style, ":") {
		return nil
	}
	if html != nil && html.compiledStyleCache != nil {
		if declarations, ok := html.compiledStyleCache[style]; ok {
			return declarations
		}
	}
	if html == nil || !html.renderCacheActive {
		return parseStyleDeclarations(style)
	}
	if html.styleDeclarationCache == nil {
		html.styleDeclarationCache = make(map[string]map[string]string)
	}
	if declarations, ok := html.styleDeclarationCache[style]; ok {
		return declarations
	}
	declarations := parseStyleDeclarations(style)
	html.styleDeclarationCache[style] = declarations
	return declarations
}

func (html *HTML) styleDeclarations(attrs map[string]string) map[string]string {
	if attrs == nil {
		return nil
	}
	return html.cachedStyleDeclarations(attrs["style"])
}

func (html *HTML) styleValue(attrs map[string]string, name string) string {
	if attrs == nil {
		return ""
	}
	return html.styleDeclarations(attrs)[strings.ToLower(name)]
}

func htmlElementDeclarations(el HTMLSegmentType, cssRules []htmlCSSRule, ancestors ...HTMLSegmentType) map[string]string {
	return htmlElementDeclarationsWithStyle(el, cssRules, parseStyleDeclarations(el.Attr["style"]), ancestors...)
}

func (html *HTML) elementDeclarations(el HTMLSegmentType, cssRules []htmlCSSRule, ancestors ...HTMLSegmentType) map[string]string {
	return htmlElementDeclarationsWithStyle(el, cssRules, html.styleDeclarations(el.Attr), ancestors...)
}

func htmlElementDeclarationsWithStyle(el HTMLSegmentType, cssRules []htmlCSSRule, style map[string]string, ancestors ...HTMLSegmentType) map[string]string {
	if ancestors == nil {
		ancestors = []HTMLSegmentType{}
	}
	if len(cssRules) == 0 {
		return style
	}
	var decl map[string]string
	for _, rule := range cssRules {
		for _, selector := range rule.selectors {
			if htmlCSSSelectorMatches(selector, el, ancestors) {
				if decl == nil {
					decl = make(map[string]string, len(rule.declarations)+len(style))
				}
				for name, value := range rule.declarations {
					decl[name] = value
				}
				break
			}
		}
	}
	if decl == nil {
		return style
	}
	for name, value := range style {
		decl[name] = value
	}
	return decl
}

func htmlDeclarationColor(decl map[string]string, names ...string) CSSColorType {
	for _, name := range names {
		if color, ok := parseCSSColor(decl[name]); ok {
			return color
		}
	}
	return CSSColorType{}
}

func htmlAttrColor(attrs map[string]string, name string) CSSColorType {
	if color, ok := parseCSSColor(attrs[name]); ok {
		return color
	}
	return CSSColorType{}
}

func htmlBorderEnabled(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "0" && value != "none" && value != "hidden"
}

func (border htmlBorderStyle) hasAny() bool {
	return border.enabled || border.top.enabled || border.right.enabled || border.bottom.enabled || border.left.enabled
}

func (border *htmlBorderStyle) setAll(side htmlBorderSideStyle) {
	border.enabled = side.enabled
	border.width = side.width
	border.color = side.color
	border.top = side
	border.right = side
	border.bottom = side
	border.left = side
}

func (side htmlBorderSideStyle) withFallback(fallback htmlBorderStyle) htmlBorderSideStyle {
	if side.width <= 0 {
		side.width = fallback.width
	}
	if !side.color.Set {
		side.color = fallback.color
	}
	return side
}

func htmlHasBorderDeclaration(decl map[string]string) bool {
	for name, value := range decl {
		if strings.HasPrefix(name, "border") && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func htmlBorderFromAttrs(attrs map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	return htmlBorderFromStyle(attrs, parseStyleDeclarations(attrs["style"]), pdf, relative)
}

func (html *HTML) borderFromAttrs(attrs map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	return htmlBorderFromStyle(attrs, html.styleDeclarations(attrs), pdf, relative)
}

func htmlBorderFromStyle(attrs map[string]string, style map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	border := htmlBorderFromDeclarations(style, pdf, relative)
	if !border.hasAny() && htmlBorderEnabled(attrs["border"]) {
		border.setAll(htmlBorderSideStyle{enabled: true})
	}
	if !border.color.Set {
		if color, ok := parseCSSColor(attrs["bordercolor"]); ok {
			border.color = color
			border.top.color = color
			border.right.color = color
			border.bottom.color = color
			border.left.color = color
		}
	}
	return border
}

func htmlBorderFromDeclarations(decl map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	border := htmlBorderStyle{}
	shorthand := decl["border"]
	if htmlBorderStyleNone(firstNonEmpty(decl["border-style"], shorthand)) {
		return border
	}
	if width, ok := parseHTMLBorderWidth(firstNonEmpty(decl["border-width"], shorthand), pdf, relative); ok {
		border.width = width
		border.enabled = width > 0
	}
	if color, ok := htmlBorderColor(firstNonEmpty(decl["border-color"], shorthand)); ok {
		border.color = color
	}
	if htmlBorderEnabled(shorthand) || htmlBorderVisibleStyle(decl["border-style"]) || border.width > 0 {
		border.enabled = true
	}
	border.setAll(htmlBorderSideStyle{enabled: border.enabled, width: border.width, color: border.color})
	if sideStyle, ok := htmlBorderSideFromDeclarations(decl, "top", border, pdf, relative); ok {
		border.sideSpecific = true
		border.top = sideStyle
	}
	if sideStyle, ok := htmlBorderSideFromDeclarations(decl, "right", border, pdf, relative); ok {
		border.sideSpecific = true
		border.right = sideStyle
	}
	if sideStyle, ok := htmlBorderSideFromDeclarations(decl, "bottom", border, pdf, relative); ok {
		border.sideSpecific = true
		border.bottom = sideStyle
	}
	if sideStyle, ok := htmlBorderSideFromDeclarations(decl, "left", border, pdf, relative); ok {
		border.sideSpecific = true
		border.left = sideStyle
	}
	border.enabled = border.hasAny()
	return border
}

func htmlBorderSideFromDeclarations(decl map[string]string, side string, fallback htmlBorderStyle, pdf *Document, relative float64) (htmlBorderSideStyle, bool) {
	sideDecl := htmlBorderSideDeclarationNames(side)
	if !htmlHasSpecificBorderDeclaration(decl, sideDecl) {
		return htmlBorderSideStyle{}, false
	}
	current := htmlBorderSideStyle{enabled: fallback.enabled, width: fallback.width, color: fallback.color}
	shorthand := decl[sideDecl.shorthand]
	if htmlBorderStyleNone(firstNonEmpty(decl[sideDecl.style], shorthand)) {
		return htmlBorderSideStyle{}, true
	}
	if width, ok := parseHTMLBorderWidth(firstNonEmpty(decl[sideDecl.width], shorthand), pdf, relative); ok {
		current.width = width
		current.enabled = width > 0
	}
	if color, ok := htmlBorderColor(firstNonEmpty(decl[sideDecl.color], shorthand)); ok {
		current.color = color
	}
	if htmlBorderEnabled(shorthand) || htmlBorderVisibleStyle(decl[sideDecl.style]) || current.width > 0 {
		current.enabled = true
	}
	return current.withFallback(fallback), true
}

type htmlBorderSideDeclarationSet struct {
	shorthand string
	width     string
	style     string
	color     string
}

func htmlBorderSideDeclarationNames(side string) htmlBorderSideDeclarationSet {
	switch side {
	case "top":
		return htmlBorderSideDeclarationSet{
			shorthand: "border-top",
			width:     "border-top-width",
			style:     "border-top-style",
			color:     "border-top-color",
		}
	case "right":
		return htmlBorderSideDeclarationSet{
			shorthand: "border-right",
			width:     "border-right-width",
			style:     "border-right-style",
			color:     "border-right-color",
		}
	case "bottom":
		return htmlBorderSideDeclarationSet{
			shorthand: "border-bottom",
			width:     "border-bottom-width",
			style:     "border-bottom-style",
			color:     "border-bottom-color",
		}
	default:
		return htmlBorderSideDeclarationSet{
			shorthand: "border-left",
			width:     "border-left-width",
			style:     "border-left-style",
			color:     "border-left-color",
		}
	}
}

func htmlHasSpecificBorderDeclaration(decl map[string]string, side htmlBorderSideDeclarationSet) bool {
	return strings.TrimSpace(decl[side.shorthand]) != "" ||
		strings.TrimSpace(decl[side.width]) != "" ||
		strings.TrimSpace(decl[side.style]) != "" ||
		strings.TrimSpace(decl[side.color]) != ""
}

func htmlBorderVisibleStyle(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false
	}
	for _, token := range strings.Fields(value) {
		switch token {
		case "none", "hidden":
			return false
		case "solid", "dashed", "dotted", "double":
			return true
		}
	}
	return false
}

func htmlBorderStyleNone(value string) bool {
	for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(value))) {
		if token == "none" || token == "hidden" {
			return true
		}
	}
	return false
}

func parseHTMLBorderWidth(value string, pdf *Document, relative float64) (float64, bool) {
	for _, token := range strings.Fields(strings.ToLower(strings.TrimSpace(value))) {
		switch token {
		case "thin":
			return 0.5, true
		case "medium":
			return 1, true
		case "thick":
			return 1.5, true
		}
		if pdf != nil {
			if width, ok := parseHTMLBoxLength(token, pdf, relative); ok {
				return width, true
			}
		}
	}
	return 0, false
}

func htmlBorderColor(value string) (CSSColorType, bool) {
	for _, token := range strings.Fields(strings.TrimSpace(value)) {
		if color, ok := parseCSSColor(token); ok {
			return color, true
		}
	}
	return CSSColorType{}, false
}

func htmlBorderRadiusFromDeclarations(decl map[string]string, pdf *Document, relative float64) htmlBorderRadiusStyle {
	radius := htmlBorderRadiusStyle{}
	if values := htmlCSSValueFields(strings.Split(firstNonEmpty(decl["border-radius"]), "/")[0]); len(values) > 0 && len(values) <= 4 {
		if edges, ok := parseHTMLBoxEdgeValues(values, pdf, relative); ok {
			radius = htmlBorderRadiusStyle{
				topLeft:     edges.top,
				topRight:    edges.right,
				bottomRight: edges.bottom,
				bottomLeft:  edges.left,
			}
		}
	}
	for _, corner := range []struct {
		name string
		set  func(float64)
	}{
		{name: "top-left", set: func(v float64) { radius.topLeft = v }},
		{name: "top-right", set: func(v float64) { radius.topRight = v }},
		{name: "bottom-right", set: func(v float64) { radius.bottomRight = v }},
		{name: "bottom-left", set: func(v float64) { radius.bottomLeft = v }},
	} {
		value := strings.TrimSpace(strings.Split(decl["border-"+corner.name+"-radius"], "/")[0])
		if n, ok := parseHTMLBoxLength(value, pdf, relative); ok {
			corner.set(n)
		}
	}
	return radius
}

func htmlBoxShadowFromDeclarations(decl map[string]string, pdf *Document, relative float64) htmlBoxShadowStyle {
	value := strings.TrimSpace(decl["box-shadow"])
	if value == "" || strings.EqualFold(value, "none") || pdf == nil {
		return htmlBoxShadowStyle{}
	}
	if comma := htmlCSSTopLevelComma(value); comma >= 0 {
		value = value[:comma]
	}
	tokens := htmlCSSValueFields(value)
	lengths := make([]float64, 0, 4)
	shadow := htmlBoxShadowStyle{
		color: CSSColorType{R: 0, G: 0, B: 0, Set: true},
		alpha: 0.18,
	}
	for _, token := range tokens {
		if strings.EqualFold(token, "inset") {
			continue
		}
		if color, alpha, ok := parseCSSColorWithAlpha(token); ok {
			if !color.None {
				shadow.color = color
				shadow.alpha = alpha
			}
			continue
		}
		if len(lengths) < 4 {
			if n, ok := parseHTMLBoxLengthSigned(token, pdf, relative); ok {
				lengths = append(lengths, n)
			}
		}
	}
	if len(lengths) < 2 {
		return htmlBoxShadowStyle{}
	}
	shadow.offsetX = lengths[0]
	shadow.offsetY = lengths[1]
	if len(lengths) > 2 && lengths[2] > 0 {
		shadow.blur = lengths[2]
	}
	if len(lengths) > 3 {
		shadow.spread = lengths[3]
	}
	shadow.alpha = clampFloat(shadow.alpha, 0, 1)
	shadow.enabled = shadow.alpha > 0 && shadow.color.Set && !shadow.color.None
	return shadow
}

func htmlCSSValueFields(value string) []string {
	var fields []string
	start := -1
	depth := 0
	for i, r := range value {
		switch {
		case unicode.IsSpace(r) && depth == 0:
			if start >= 0 {
				fields = append(fields, value[start:i])
				start = -1
			}
		case r == '(':
			if start < 0 {
				start = i
			}
			depth++
		case r == ')':
			if start < 0 {
				start = i
			}
			if depth > 0 {
				depth--
			}
		default:
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		fields = append(fields, value[start:])
	}
	return fields
}

func htmlCSSTopLevelComma(value string) int {
	depth := 0
	for i, r := range value {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseCSSColorWithAlpha(value string) (CSSColorType, float64, bool) {
	color, ok := parseCSSColor(value)
	if ok {
		return color, 1, true
	}
	value = strings.TrimSpace(strings.ToLower(value))
	if !strings.HasPrefix(value, "rgba(") || !strings.HasSuffix(value, ")") {
		return CSSColorType{}, 0, false
	}
	parts := strings.Split(value[5:len(value)-1], ",")
	if len(parts) != 4 {
		return CSSColorType{}, 0, false
	}
	r, okR := parseCSSColorComponent(parts[0])
	g, okG := parseCSSColorComponent(parts[1])
	b, okB := parseCSSColorComponent(parts[2])
	alpha, okA := parseCSSAlpha(parts[3])
	if !okR || !okG || !okB || !okA {
		return CSSColorType{}, 0, false
	}
	return CSSColorType{R: r, G: g, B: b, Set: true}, alpha, true
}

func parseCSSColorComponent(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil || !isFiniteFloat(n) {
			return 0, false
		}
		return clampColor(int(n * 255 / 100)), true
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return clampColor(n), true
}

func parseCSSAlpha(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
		if err != nil || !isFiniteFloat(n) {
			return 0, false
		}
		return clampFloat(n/100, 0, 1), true
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !isFiniteFloat(n) {
		return 0, false
	}
	return clampFloat(n, 0, 1), true
}

func parseHTMLBoxLengthSigned(value string, pdf *Document, relative float64) (float64, bool) {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "-") {
		n, ok := parseHTMLBoxLength(strings.TrimSpace(value[1:]), pdf, relative)
		return -n, ok
	}
	if strings.HasPrefix(value, "+") {
		value = strings.TrimSpace(value[1:])
	}
	return parseHTMLBoxLength(value, pdf, relative)
}

func htmlApplyBorderStyle(pdf *Document, border htmlBorderStyle, fallbackR, fallbackG, fallbackB int, fallbackWidth float64) {
	if border.color.Set && !border.color.None {
		pdf.SetDrawColor(border.color.R, border.color.G, border.color.B)
	} else {
		pdf.SetDrawColor(fallbackR, fallbackG, fallbackB)
	}
	if border.width > 0 {
		pdf.SetLineWidth(border.width)
	} else {
		pdf.SetLineWidth(fallbackWidth)
	}
}

func htmlDrawBoxShadow(pdf *Document, x, y, wd, ht float64, radius htmlBorderRadiusStyle, shadow htmlBoxShadowStyle) {
	if !shadow.enabled || wd <= 0 || ht <= 0 {
		return
	}
	fillR, fillG, fillB := pdf.GetFillColor()
	alpha, blend := pdf.GetAlpha()
	defer func() {
		pdf.SetFillColor(fillR, fillG, fillB)
		pdf.SetAlpha(alpha, blend)
	}()
	pdf.SetFillColor(shadow.color.R, shadow.color.G, shadow.color.B)
	steps := 1
	if shadow.blur > 0 {
		steps = 4
	}
	for i := steps; i >= 1; i-- {
		grow := shadow.spread
		if shadow.blur > 0 {
			grow += shadow.blur * float64(i) / float64(steps)
		}
		drawX := x + shadow.offsetX - grow
		drawY := y + shadow.offsetY - grow
		drawWd := wd + 2*grow
		drawHt := ht + 2*grow
		if drawWd <= 0 || drawHt <= 0 {
			continue
		}
		layerAlpha := shadow.alpha / float64(steps)
		if steps == 1 {
			layerAlpha = shadow.alpha
		}
		pdf.SetAlpha(clampFloat(layerAlpha, 0, 1), "Normal")
		drawRadius := radius
		if grow > 0 {
			drawRadius.topLeft += grow
			drawRadius.topRight += grow
			drawRadius.bottomRight += grow
			drawRadius.bottomLeft += grow
		}
		drawRadius = drawRadius.clamped(drawWd, drawHt)
		if drawRadius.hasAny() {
			pdf.RoundedRectExt(drawX, drawY, drawWd, drawHt, drawRadius.topLeft, drawRadius.topRight, drawRadius.bottomRight, drawRadius.bottomLeft, "F")
		} else {
			pdf.Rect(drawX, drawY, drawWd, drawHt, "F")
		}
	}
}

func htmlDrawBorderedRect(pdf *Document, x, y, wd, ht float64, border htmlBorderStyle, radius htmlBorderRadiusStyle, fill bool, fallbackR, fallbackG, fallbackB int, fallbackWidth float64) {
	radius = radius.clamped(wd, ht)
	if fill {
		if radius.hasAny() {
			pdf.RoundedRectExt(x, y, wd, ht, radius.topLeft, radius.topRight, radius.bottomRight, radius.bottomLeft, "F")
		} else {
			pdf.Rect(x, y, wd, ht, "F")
		}
	}
	if !border.hasAny() {
		return
	}
	if !border.sideSpecific {
		htmlApplyBorderStyle(pdf, border, fallbackR, fallbackG, fallbackB, fallbackWidth)
		if radius.hasAny() {
			pdf.RoundedRectExt(x, y, wd, ht, radius.topLeft, radius.topRight, radius.bottomRight, radius.bottomLeft, "D")
			return
		}
		pdf.Rect(x, y, wd, ht, "D")
		return
	}
	htmlDrawBorderSide(pdf, border.top, fallbackR, fallbackG, fallbackB, fallbackWidth, x, y, x+wd, y)
	htmlDrawBorderSide(pdf, border.right, fallbackR, fallbackG, fallbackB, fallbackWidth, x+wd, y, x+wd, y+ht)
	htmlDrawBorderSide(pdf, border.bottom, fallbackR, fallbackG, fallbackB, fallbackWidth, x+wd, y+ht, x, y+ht)
	htmlDrawBorderSide(pdf, border.left, fallbackR, fallbackG, fallbackB, fallbackWidth, x, y+ht, x, y)
}

func htmlDrawBorderSide(pdf *Document, side htmlBorderSideStyle, fallbackR, fallbackG, fallbackB int, fallbackWidth, x1, y1, x2, y2 float64) {
	if !side.enabled {
		return
	}
	if side.color.Set && !side.color.None {
		pdf.SetDrawColor(side.color.R, side.color.G, side.color.B)
	} else {
		pdf.SetDrawColor(fallbackR, fallbackG, fallbackB)
	}
	if side.width > 0 {
		pdf.SetLineWidth(side.width)
	} else {
		pdf.SetLineWidth(fallbackWidth)
	}
	pdf.Line(x1, y1, x2, y2)
}

func parseHTMLCSSRules(css string) []htmlCSSRule {
	if len(css) > htmlMaxCSSBytes {
		css = css[:htmlMaxCSSBytes]
	}
	css = stripHTMLCSSComments(css)
	var rules []htmlCSSRule
	for {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			return rules
		}
		close := strings.IndexByte(css[open+1:], '}')
		if close < 0 {
			return rules
		}
		close += open + 1
		selectors := parseHTMLCSSSelectors(css[:open])
		declarations := parseStyleDeclarations(css[open+1 : close])
		if len(selectors) > 0 && len(declarations) > 0 {
			rules = append(rules, htmlCSSRule{selectors: selectors, declarations: declarations})
			if len(rules) >= htmlMaxCSSRules {
				return rules
			}
		}
		css = css[close+1:]
	}
}

func stripHTMLCSSComments(css string) string {
	start := strings.Index(css, "/*")
	if start < 0 {
		return css
	}
	var out strings.Builder
	out.Grow(len(css))
	pos := 0
	for {
		start = strings.Index(css[pos:], "/*")
		if start < 0 {
			out.WriteString(css[pos:])
			return out.String()
		}
		start += pos
		out.WriteString(css[pos:start])
		end := strings.Index(css[start+2:], "*/")
		if end < 0 {
			return out.String()
		}
		pos = start + 2 + end + 2
	}
}

func parseHTMLCSSSelectors(value string) []htmlCSSSelector {
	var selectors []htmlCSSSelector
	for _, raw := range strings.Split(value, ",") {
		selector, ok := parseHTMLCSSSelector(strings.TrimSpace(raw))
		if ok {
			selectors = append(selectors, selector)
			if len(selectors) >= htmlMaxCSSSelectors {
				return selectors
			}
		}
	}
	return selectors
}

func parseHTMLCSSSelector(value string) (htmlCSSSelector, bool) {
	if value == "" || strings.ContainsAny(value, "+~[]:*") {
		return htmlCSSSelector{}, false
	}
	tokens := htmlCSSSelectorTokens(value)
	if len(tokens) == 0 {
		return htmlCSSSelector{}, false
	}
	selector := htmlCSSSelector{}
	expectSimple := true
	nextDirect := false
	for _, token := range tokens {
		if token == ">" {
			if expectSimple || nextDirect {
				return htmlCSSSelector{}, false
			}
			nextDirect = true
			expectSimple = true
			continue
		}
		part, ok := parseHTMLCSSSelectorPart(token)
		if !ok {
			return htmlCSSSelector{}, false
		}
		part.direct = nextDirect
		selector.parts = append(selector.parts, part)
		nextDirect = false
		expectSimple = false
	}
	if expectSimple {
		return htmlCSSSelector{}, false
	}
	return selector, true
}

func htmlCSSSelectorTokens(value string) []string {
	var tokens []string
	for len(value) > 0 {
		value = strings.TrimLeftFunc(value, unicode.IsSpace)
		if value == "" {
			break
		}
		if value[0] == '>' {
			tokens = append(tokens, ">")
			value = value[1:]
			continue
		}
		end := 0
		for end < len(value) && value[end] != '>' && !unicode.IsSpace(rune(value[end])) {
			end++
		}
		tokens = append(tokens, value[:end])
		value = value[end:]
	}
	return tokens
}

func parseHTMLCSSSelectorPart(value string) (htmlCSSSelectorPart, bool) {
	part := htmlCSSSelectorPart{}
	for len(value) > 0 {
		prefix := value[0]
		switch prefix {
		case '.', '#':
			value = value[1:]
		default:
			prefix = 0
		}
		end := 0
		for end < len(value) && value[end] != '.' && value[end] != '#' {
			end++
		}
		token := strings.ToLower(value[:end])
		if token == "" {
			return htmlCSSSelectorPart{}, false
		}
		switch prefix {
		case '.':
			if part.class != "" {
				return htmlCSSSelectorPart{}, false
			}
			part.class = token
		case '#':
			if part.id != "" {
				return htmlCSSSelectorPart{}, false
			}
			part.id = token
		default:
			if part.tag != "" {
				return htmlCSSSelectorPart{}, false
			}
			part.tag = token
		}
		value = value[end:]
	}
	return part, part.tag != "" || part.id != "" || part.class != ""
}

func applyHTMLCSSRules(st *htmlTextStyle, el HTMLSegmentType, rules []htmlCSSRule, baseFontSize, baseLineHeight float64, pdf *Document, ancestors ...HTMLSegmentType) {
	if ancestors == nil {
		ancestors = []HTMLSegmentType{}
	}
	for _, rule := range rules {
		for _, selector := range rule.selectors {
			if htmlCSSSelectorMatches(selector, el, ancestors) {
				applyHTMLStyleDeclarations(st, rule.declarations, baseFontSize, baseLineHeight, pdf)
				break
			}
		}
	}
}

func htmlCSSSelectorMatches(selector htmlCSSSelector, el HTMLSegmentType, ancestors []HTMLSegmentType) bool {
	if ancestors == nil {
		ancestors = []HTMLSegmentType{}
	}
	if len(selector.parts) == 0 {
		return false
	}
	if !htmlCSSSelectorPartMatches(selector.parts[len(selector.parts)-1], el) {
		return false
	}
	ancestorIndex := len(ancestors) - 1
	for partIndex := len(selector.parts) - 2; partIndex >= 0; partIndex-- {
		part := selector.parts[partIndex]
		if selector.parts[partIndex+1].direct {
			if ancestorIndex < 0 || !htmlCSSSelectorPartMatches(part, ancestors[ancestorIndex]) {
				return false
			}
			ancestorIndex--
			continue
		}
		found := false
		for ancestorIndex >= 0 {
			if htmlCSSSelectorPartMatches(part, ancestors[ancestorIndex]) {
				found = true
				ancestorIndex--
				break
			}
			ancestorIndex--
		}
		if !found {
			return false
		}
	}
	return true
}

func htmlCSSSelectorPartMatches(part htmlCSSSelectorPart, el HTMLSegmentType) bool {
	if part.tag != "" && part.tag != el.Str {
		return false
	}
	if part.id != "" && strings.ToLower(el.Attr["id"]) != part.id {
		return false
	}
	if part.class != "" && !htmlClassContains(el.Attr["class"], part.class) {
		return false
	}
	return true
}

func htmlClassContains(classAttr, className string) bool {
	start := -1
	for i, r := range classAttr {
		if unicode.IsSpace(r) {
			if start >= 0 && strings.EqualFold(classAttr[start:i], className) {
				return true
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	return start >= 0 && strings.EqualFold(classAttr[start:], className)
}

func htmlListStyleType(value string) string {
	start := -1
	for i, r := range value {
		if unicode.IsSpace(r) {
			if start >= 0 {
				if style := htmlListStyleToken(value[start:i]); style != "" {
					return style
				}
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		return htmlListStyleToken(value[start:])
	}
	return ""
}

func htmlListStyleToken(token string) string {
	switch {
	case strings.EqualFold(token, "decimal"):
		return "decimal"
	case strings.EqualFold(token, "lower-alpha"):
		return "lower-alpha"
	case strings.EqualFold(token, "upper-alpha"):
		return "upper-alpha"
	case strings.EqualFold(token, "lower-roman"):
		return "lower-roman"
	case strings.EqualFold(token, "upper-roman"):
		return "upper-roman"
	case strings.EqualFold(token, "disc"):
		return "disc"
	case strings.EqualFold(token, "circle"):
		return "circle"
	case strings.EqualFold(token, "square"):
		return "square"
	case strings.EqualFold(token, "none"):
		return "none"
	default:
		return ""
	}
}

func applyHTMLAttrs(st *htmlTextStyle, attrs map[string]string, baseFontSize, baseLineHeight float64, pdf *Document) {
	applyHTMLAttrsWithStyle(st, attrs, parseStyleDeclarations(attrs["style"]), baseFontSize, baseLineHeight, pdf)
}

func (html *HTML) applyAttrs(st *htmlTextStyle, attrs map[string]string, baseFontSize, baseLineHeight float64, pdf *Document) {
	applyHTMLAttrsWithStyle(st, attrs, html.styleDeclarations(attrs), baseFontSize, baseLineHeight, pdf)
}

func applyHTMLAttrsWithStyle(st *htmlTextStyle, attrs map[string]string, style map[string]string, baseFontSize, baseLineHeight float64, pdf *Document) {
	if attrs == nil {
		return
	}
	if color, ok := parseCSSColor(attrs["color"]); ok {
		st.color = color
	}
	if size, ok := parseHTMLFontSize(attrs["size"], baseFontSize); ok {
		st.fontSize = size
	}
	switch strings.ToLower(strings.TrimSpace(attrs["valign"])) {
	case "top", "middle", "bottom":
		st.verticalAlign = strings.ToLower(strings.TrimSpace(attrs["valign"]))
	}
	applyHTMLStyleDeclarations(st, style, baseFontSize, baseLineHeight, pdf)
}

func applyHTMLStyleDeclarations(st *htmlTextStyle, style map[string]string, baseFontSize, baseLineHeight float64, pdf *Document) {
	if color, ok := parseCSSColor(style["color"]); ok {
		st.color = color
	}
	if size, ok := parseHTMLFontSize(style["font-size"], baseFontSize); ok {
		st.fontSize = size
	}
	if lineHeight, ok := parseHTMLLineHeight(style["line-height"], baseLineHeight, pdf); ok {
		st.lineHeight = lineHeight
	}
	switch strings.ToLower(style["font-weight"]) {
	case "bold", "bolder", "600", "700", "800", "900":
		st.bold = true
	}
	if strings.Contains(strings.ToLower(style["font-style"]), "italic") {
		st.italic = true
	}
	if strings.Contains(strings.ToLower(style["text-decoration"]), "underline") {
		st.underline = true
	}
	if strings.Contains(strings.ToLower(style["text-decoration"]), "line-through") {
		st.strike = true
	}
	if family := htmlFontFamily(style["font-family"]); family != "" {
		st.fontFamily = family
	}
	if listStyleType := htmlListStyleType(firstNonEmpty(style["list-style-type"], style["list-style"])); listStyleType != "" {
		st.listStyleType = listStyleType
	}
	switch strings.ToLower(style["vertical-align"]) {
	case "super", "sup":
		st.script = 1
	case "sub":
		st.script = -1
	case "top", "middle", "bottom":
		st.verticalAlign = strings.ToLower(style["vertical-align"])
	}
	switch strings.ToLower(style["white-space"]) {
	case "pre", "pre-wrap":
		st.preserveWhitespace = true
	}
	switch strings.ToLower(style["text-align"]) {
	case "left":
		st.align = "L"
	case "center":
		st.align = "C"
	case "right":
		st.align = "R"
	}
}

func htmlFontFamily(value string) string {
	for {
		part := value
		if comma := strings.IndexByte(value, ','); comma >= 0 {
			part = value[:comma]
			value = value[comma+1:]
		} else {
			value = ""
		}
		family := strings.Trim(strings.TrimSpace(part), `"'`)
		switch {
		case strings.EqualFold(family, "monospace"), strings.EqualFold(family, "courier"), strings.EqualFold(family, "courier new"):
			return "Courier"
		case strings.EqualFold(family, "serif"), strings.EqualFold(family, "times"), strings.EqualFold(family, "times new roman"):
			return "Times"
		case strings.EqualFold(family, "sans-serif"), strings.EqualFold(family, "sans"), strings.EqualFold(family, "helvetica"), strings.EqualFold(family, "arial"):
			return "Helvetica"
		}
		if value == "" {
			return ""
		}
	}
}

func parseHTMLFontSize(value string, base float64) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return 0, false
	}
	if strings.HasSuffix(value, "em") {
		n, err := strconv.ParseFloat(strings.TrimSuffix(value, "em"), 64)
		return base * n, err == nil && n > 0
	}
	for _, suffix := range []string{"pt", "px"} {
		if strings.HasSuffix(value, suffix) {
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
			break
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	return n, err == nil && n > 0
}

func parseHTMLLineHeight(value string, base float64, pdf *Document) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "normal" {
		return 0, false
	}
	if strings.HasSuffix(value, "%") {
		n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
		if err != nil || !isFiniteFloat(n) || n <= 0 {
			return 0, false
		}
		return base * n / 100, true
	}
	for _, suffix := range []string{"px", "pt", "mm", "cm", "in"} {
		if strings.HasSuffix(value, suffix) {
			if lineHeight, ok := parseHTMLBoxLength(value, pdf, base); ok && lineHeight > 0 {
				return lineHeight, true
			}
			return 0, false
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !isFiniteFloat(n) || n <= 0 {
		return 0, false
	}
	return base * n, true
}

func htmlHeadingFontSize(base float64, tag string) float64 {
	switch tag {
	case "h1":
		return base * 2
	case "h2":
		return base * 1.6
	case "h3":
		return base * 1.35
	case "h4":
		return base * 1.15
	case "h5":
		return base
	default:
		return base * 0.9
	}
}
