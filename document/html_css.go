// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
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
	index        *htmlCSSRuleIndex
}

type htmlCSSSelector struct {
	parts       []htmlCSSSelectorPart
	specificity int
}

type htmlCSSSelectorPart struct {
	tag    string
	id     string
	class  string
	direct bool
}

type htmlCSSRuleIndex struct {
	buckets map[string][]htmlCSSSelectorRef
}

type htmlCSSSelectorRef struct {
	rule     int
	selector int
}

type htmlElementMetadata struct {
	tag     string
	id      string
	classes []string
}

func htmlElementDeclarationsWithStyle(el HTMLSegmentType, cssRules []htmlCSSRule, style map[string]string, ancestors ...HTMLSegmentType) map[string]string {
	if ancestors == nil {
		ancestors = []HTMLSegmentType{}
	}
	if len(cssRules) == 0 {
		return style
	}
	if index := htmlCSSRulesIndex(cssRules); index != nil {
		return htmlElementDeclarationsWithStyleIndex(el, cssRules, style, ancestors, index)
	}
	return htmlElementDeclarationsWithStyleScan(el, cssRules, style, ancestors)
}

func htmlElementDeclarationsWithStyleMeta(el HTMLSegmentType, meta htmlElementMetadata, cssRules []htmlCSSRule, style map[string]string, ancestors []HTMLSegmentType, ancestorMeta []htmlElementMetadata) map[string]string {
	if len(cssRules) == 0 {
		return style
	}
	if index := htmlCSSRulesIndex(cssRules); index != nil {
		return htmlElementDeclarationsWithStyleMetaIndex(cssRules, style, meta, ancestorMeta, index)
	}
	return htmlElementDeclarationsWithStyleScan(el, cssRules, style, ancestors)
}

func htmlElementDeclarationsWithStyleScan(el HTMLSegmentType, cssRules []htmlCSSRule, style map[string]string, ancestors []HTMLSegmentType) map[string]string {
	type appliedDeclaration struct {
		value       string
		specificity int
		order       int
	}
	var applied map[string]appliedDeclaration
	for order, rule := range cssRules {
		specificity := -1
		for _, selector := range rule.selectors {
			if htmlCSSSelectorMatches(selector, el, ancestors) {
				if selector.specificity > specificity {
					specificity = selector.specificity
				}
			}
		}
		if specificity < 0 {
			continue
		}
		if applied == nil {
			applied = make(map[string]appliedDeclaration)
		}
		for name, value := range rule.declarations {
			current, ok := applied[name]
			if !ok || specificity > current.specificity || specificity == current.specificity && order >= current.order {
				applied[name] = appliedDeclaration{value: value, specificity: specificity, order: order}
			}
		}
	}
	if len(applied) == 0 {
		return style
	}
	decl := make(map[string]string, len(applied)+len(style))
	for name, applied := range applied {
		decl[name] = applied.value
	}
	for name, value := range style {
		decl[name] = value
	}
	return decl
}

func htmlElementDeclarationsWithStyleMetaIndex(cssRules []htmlCSSRule, style map[string]string, meta htmlElementMetadata, ancestors []htmlElementMetadata, index *htmlCSSRuleIndex) map[string]string {
	if index == nil || len(index.buckets) == 0 {
		return style
	}
	ruleSpecificities := make([]int, len(cssRules))
	matched := false
	index.visitSelectorsForElementMeta(meta, func(candidate htmlCSSSelectorRef) {
		if candidate.rule < 0 || candidate.rule >= len(cssRules) {
			return
		}
		rule := cssRules[candidate.rule]
		if candidate.selector < 0 || candidate.selector >= len(rule.selectors) {
			return
		}
		selector := rule.selectors[candidate.selector]
		if !htmlCSSSelectorMatchesMeta(selector, meta, ancestors) {
			return
		}
		encodedSpecificity := selector.specificity + 1
		if encodedSpecificity > ruleSpecificities[candidate.rule] {
			ruleSpecificities[candidate.rule] = encodedSpecificity
		}
		matched = true
	})
	return htmlElementDeclarationsFromRuleSpecificities(cssRules, style, ruleSpecificities, matched)
}

func htmlElementDeclarationsWithStyleIndex(el HTMLSegmentType, cssRules []htmlCSSRule, style map[string]string, ancestors []HTMLSegmentType, index *htmlCSSRuleIndex) map[string]string {
	if index == nil || len(index.buckets) == 0 {
		return style
	}
	ruleSpecificities := make([]int, len(cssRules))
	matched := false
	index.visitSelectorsForElement(el, func(candidate htmlCSSSelectorRef) {
		if candidate.rule < 0 || candidate.rule >= len(cssRules) {
			return
		}
		rule := cssRules[candidate.rule]
		if candidate.selector < 0 || candidate.selector >= len(rule.selectors) {
			return
		}
		selector := rule.selectors[candidate.selector]
		if !htmlCSSSelectorMatches(selector, el, ancestors) {
			return
		}
		encodedSpecificity := selector.specificity + 1
		if encodedSpecificity > ruleSpecificities[candidate.rule] {
			ruleSpecificities[candidate.rule] = encodedSpecificity
		}
		matched = true
	})
	return htmlElementDeclarationsFromRuleSpecificities(cssRules, style, ruleSpecificities, matched)
}

func htmlElementDeclarationsFromRuleSpecificities(cssRules []htmlCSSRule, style map[string]string, ruleSpecificities []int, matched bool) map[string]string {
	if !matched {
		return style
	}

	type appliedDeclaration struct {
		value       string
		specificity int
		order       int
	}
	applied := make(map[string]appliedDeclaration)
	for ruleIndex, rule := range cssRules {
		encodedSpecificity := ruleSpecificities[ruleIndex]
		if encodedSpecificity == 0 {
			continue
		}
		specificity := encodedSpecificity - 1
		for name, value := range rule.declarations {
			current, ok := applied[name]
			if !ok || specificity > current.specificity || specificity == current.specificity && ruleIndex >= current.order {
				applied[name] = appliedDeclaration{value: value, specificity: specificity, order: ruleIndex}
			}
		}
	}
	if len(applied) == 0 {
		return style
	}
	decl := make(map[string]string, len(applied)+len(style))
	for name, applied := range applied {
		decl[name] = applied.value
	}
	for name, value := range style {
		decl[name] = value
	}
	return decl
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

func parseHTMLCSSRules(css string) []htmlCSSRule {
	if len(css) > htmlMaxCSSBytes {
		css = css[:htmlMaxCSSBytes]
	}
	css = stripHTMLCSSComments(css)
	var rules []htmlCSSRule
	for {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			return htmlIndexCSSRules(rules)
		}
		close := strings.IndexByte(css[open+1:], '}')
		if close < 0 {
			return htmlIndexCSSRules(rules)
		}
		close += open + 1
		selectors := parseHTMLCSSSelectors(css[:open])
		declarations := parseStyleDeclarations(css[open+1 : close])
		if len(selectors) > 0 && len(declarations) > 0 {
			rules = append(rules, htmlCSSRule{selectors: selectors, declarations: declarations})
			if len(rules) >= htmlMaxCSSRules {
				return htmlIndexCSSRules(rules)
			}
		}
		css = css[close+1:]
	}
}

func htmlIndexCSSRules(rules []htmlCSSRule) []htmlCSSRule {
	if len(rules) == 0 {
		return rules
	}
	index := &htmlCSSRuleIndex{buckets: make(map[string][]htmlCSSSelectorRef)}
	for ruleIndex, rule := range rules {
		for selectorIndex, selector := range rule.selectors {
			if len(selector.parts) == 0 {
				continue
			}
			key := htmlCSSSelectorBucketKey(selector.parts[len(selector.parts)-1])
			index.buckets[key] = append(index.buckets[key], htmlCSSSelectorRef{rule: ruleIndex, selector: selectorIndex})
		}
	}
	for i := range rules {
		rules[i].index = index
	}
	return rules
}

func htmlCSSRulesIndex(rules []htmlCSSRule) *htmlCSSRuleIndex {
	if len(rules) == 0 {
		return nil
	}
	return rules[0].index
}

func htmlCSSSelectorBucketKey(part htmlCSSSelectorPart) string {
	if part.id != "" {
		return "id:" + part.id
	}
	if part.class != "" {
		return "class:" + part.class
	}
	if part.tag != "" {
		return "tag:" + part.tag
	}
	return "*"
}

func (index *htmlCSSRuleIndex) visitSelectorsForElement(el HTMLSegmentType, visit func(htmlCSSSelectorRef)) {
	if index == nil || len(index.buckets) == 0 {
		return
	}
	visitBucket := func(key string) {
		for _, ref := range index.buckets[key] {
			visit(ref)
		}
	}
	if id := strings.ToLower(strings.TrimSpace(el.Attr["id"])); id != "" {
		visitBucket("id:" + id)
	}
	for _, className := range htmlClassNames(el.Attr["class"]) {
		visitBucket("class:" + className)
	}
	if el.Str != "" {
		visitBucket("tag:" + el.Str)
	}
	visitBucket("*")
}

func (index *htmlCSSRuleIndex) visitSelectorsForElementMeta(meta htmlElementMetadata, visit func(htmlCSSSelectorRef)) {
	if index == nil || len(index.buckets) == 0 {
		return
	}
	visitBucket := func(key string) {
		for _, ref := range index.buckets[key] {
			visit(ref)
		}
	}
	if meta.id != "" {
		visitBucket("id:" + meta.id)
	}
	for _, className := range meta.classes {
		visitBucket("class:" + className)
	}
	if meta.tag != "" {
		visitBucket("tag:" + meta.tag)
	}
	visitBucket("*")
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
	selector.specificity = htmlCSSSelectorSpecificity(selector)
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

func htmlCSSSelectorMatchesMeta(selector htmlCSSSelector, el htmlElementMetadata, ancestors []htmlElementMetadata) bool {
	if len(selector.parts) == 0 {
		return false
	}
	if !el.matches(selector.parts[len(selector.parts)-1]) {
		return false
	}
	ancestorIndex := len(ancestors) - 1
	for partIndex := len(selector.parts) - 2; partIndex >= 0; partIndex-- {
		part := selector.parts[partIndex]
		if selector.parts[partIndex+1].direct {
			if ancestorIndex < 0 || !ancestors[ancestorIndex].matches(part) {
				return false
			}
			ancestorIndex--
			continue
		}
		found := false
		for ancestorIndex >= 0 {
			if ancestors[ancestorIndex].matches(part) {
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

func htmlCSSSelectorSpecificity(selector htmlCSSSelector) int {
	specificity := 0
	for _, part := range selector.parts {
		if part.id != "" {
			specificity += 100
		}
		if part.class != "" {
			specificity += 10
		}
		if part.tag != "" {
			specificity++
		}
	}
	return specificity
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

func htmlElementMetadataFromSegment(el HTMLSegmentType) htmlElementMetadata {
	return htmlElementMetadata{
		tag:     el.Str,
		id:      strings.ToLower(strings.TrimSpace(el.Attr["id"])),
		classes: htmlClassNames(el.Attr["class"]),
	}
}

func (meta htmlElementMetadata) matches(part htmlCSSSelectorPart) bool {
	if part.tag != "" && part.tag != meta.tag {
		return false
	}
	if part.id != "" && part.id != meta.id {
		return false
	}
	if part.class != "" {
		for _, className := range meta.classes {
			if className == part.class {
				return true
			}
		}
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

func htmlClassNames(classAttr string) []string {
	var names []string
	start := -1
	for i, r := range classAttr {
		if unicode.IsSpace(r) {
			if start >= 0 {
				names = append(names, strings.ToLower(classAttr[start:i]))
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		names = append(names, strings.ToLower(classAttr[start:]))
	}
	return names
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
		return base * 1.5
	case "h3":
		return base * 1.17
	case "h4":
		return base
	case "h5":
		return base * 0.83
	default:
		return base * 0.67
	}
}
