// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"fmt"
	"strings"
)

// ValidateHTML returns best-effort diagnostics for unsupported HTML tags, CSS
// selectors, and CSS properties without writing anything to the PDF.
func (html *HTML) ValidateHTML(htmlStr string) []string {
	var messages []string
	if html == nil {
		return messages
	}
	if len(htmlStr) > html.maxHTMLBytes() {
		return []string{"HTML input exceeds maximum size"}
	}
	tokens := HTMLTokenize(htmlStr)
	if message := htmlElementDepthMessage(tokens, html.maxElementDepth()); message != "" {
		return []string{message}
	}
	validator := *html
	validator.DebugLog = func(message string) {
		messages = append(messages, message)
	}
	validator.logUnsupportedHTML(tokens)
	return messages
}

func (html *HTML) maxHTMLBytes() int {
	if html == nil || html.MaxHTMLBytes <= 0 {
		return htmlDefaultMaxHTMLBytes
	}
	return html.MaxHTMLBytes
}

func (html *HTML) maxTableRows() int {
	if html == nil || html.MaxTableRows <= 0 {
		return htmlDefaultMaxTableRows
	}
	return html.MaxTableRows
}

func (html *HTML) maxElementDepth() int {
	if html == nil || html.MaxElementDepth <= 0 {
		return htmlDefaultMaxElementDepth
	}
	return html.MaxElementDepth
}

func (html *HTML) maxGeneratedPages() int {
	if html == nil || html.MaxGeneratedPages <= 0 {
		return htmlDefaultMaxGeneratedPages
	}
	return html.MaxGeneratedPages
}

func (html *HTML) generatedPageCount() int {
	if html == nil || html.pdf == nil {
		return 0
	}
	pageCount := html.pdf.PageCount()
	if html.renderStartPageCount <= 0 {
		return pageCount
	}
	if pageCount <= html.renderStartPageCount {
		return 1
	}
	return pageCount - html.renderStartPageCount + 1
}

func (html *HTML) addPageFormat() bool {
	if html == nil || html.pdf == nil {
		return false
	}
	html.pdf.addPageFormatRotation(html.pdf.curOrientation, html.pdf.curPageSize, html.pdf.curRotation)
	pageCount := html.generatedPageCount()
	maxPages := html.maxGeneratedPages()
	if pageCount > maxPages {
		html.pdf.SetErrorf("HTML rendering exceeded maximum generated pages: %d > %d", pageCount, maxPages)
		return false
	}
	return html.pdf.err == nil
}

func htmlElementDepthMessage(tokens []HTMLSegmentType, maxDepth int) string {
	depth := 0
	for _, token := range tokens {
		switch token.Cat {
		case 'O':
			if !htmlClosePops(token.Str) {
				continue
			}
			depth++
			if depth > maxDepth {
				return "HTML element depth exceeds maximum size"
			}
		case 'C':
			if depth > 0 {
				depth--
			}
		}
	}
	return ""
}

var htmlSupportedTags = map[string]bool{"a": true, "article": true, "b": true, "br": true, "center": true, "caption": true, "code": true, "dd": true, "del": true, "div": true, "dl": true, "dt": true, "em": true, "figcaption": true, "figure": true, "footer": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "head": true, "header": true, "hr": true, "i": true, "img": true, "ins": true, "kbd": true, "left": true, "li": true, "ol": true, "p": true, "pre": true, "right": true, "s": true, "samp": true, "script": true, "section": true, "strike": true, "strong": true, "style": true, "sub": true, "sup": true, "svg": true, "table": true, "tbody": true, "td": true, "tfoot": true, "th": true, "thead": true, "tr": true, "u": true, "ul": true}

var htmlSupportedCSSProperties = map[string]bool{"background": true, "background-color": true, "border": true, "border-bottom": true, "border-bottom-color": true, "border-bottom-style": true, "border-bottom-width": true, "border-collapse": true, "border-color": true, "border-left": true, "border-left-color": true, "border-left-style": true, "border-left-width": true, "border-right": true, "border-right-color": true, "border-right-style": true, "border-right-width": true, "border-style": true, "border-top": true, "border-top-color": true, "border-top-style": true, "border-top-width": true, "border-width": true, "break-after": true, "break-before": true, "break-inside": true, "color": true, "font-family": true, "font-size": true, "font-style": true, "font-weight": true, "height": true, "line-height": true, "list-style": true, "list-style-type": true, "margin": true, "margin-bottom": true, "margin-left": true, "margin-right": true, "margin-top": true, "max-height": true, "max-width": true, "object-fit": true, "padding": true, "padding-bottom": true, "padding-left": true, "padding-right": true, "padding-top": true, "page-break-after": true, "page-break-before": true, "page-break-inside": true, "text-align": true, "text-decoration": true, "vertical-align": true, "white-space": true, "width": true}

func (html *HTML) logUnsupportedHTML(tokens []HTMLSegmentType) {
	if html.DebugLog == nil {
		return
	}
	seen := map[string]bool{}
	logOnce := func(key, message string) {
		if seen[key] {
			return
		}
		seen[key] = true
		html.DebugLog(message)
	}
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.Cat != 'O' {
			continue
		}
		if !htmlSupportedTags[token.Str] {
			logOnce("tag:"+token.Str, fmt.Sprintf("HTML tag <%s> is not supported yet", token.Str))
		}
		html.logUnsupportedStyleProperties(token.Attr["style"], seen, "inline style")
		if token.Str == "style" {
			styleTokens, end := htmlCollectElementTokens(tokens, i, "style")
			html.logUnsupportedCSSRules(htmlTokenText(styleTokens), seen)
			i = end
		}
	}
}

func (html *HTML) logUnsupportedStyleProperties(style string, seen map[string]bool, source string) {
	if html.DebugLog == nil {
		return
	}
	for name := range parseStyleDeclarations(style) {
		if htmlSupportedCSSProperties[name] {
			continue
		}
		key := "css-property:" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		html.DebugLog(fmt.Sprintf("CSS property %q in %s is not supported yet", name, source))
	}
}

func (html *HTML) logUnsupportedCSSRules(css string, seen map[string]bool) {
	if html.DebugLog == nil {
		return
	}
	if len(css) > htmlMaxCSSBytes {
		css = css[:htmlMaxCSSBytes]
	}
	css = stripHTMLCSSComments(css)
	for {
		open := strings.IndexByte(css, '{')
		if open < 0 {
			return
		}
		close := strings.IndexByte(css[open+1:], '}')
		if close < 0 {
			return
		}
		close += open + 1
		rawSelectors := strings.TrimSpace(css[:open])
		for _, raw := range strings.Split(rawSelectors, ",") {
			selectorText := strings.TrimSpace(raw)
			if selectorText == "" {
				continue
			}
			if _, ok := parseHTMLCSSSelector(selectorText); ok {
				continue
			}
			key := "css-selector:" + selectorText
			if seen[key] {
				continue
			}
			seen[key] = true
			html.DebugLog(fmt.Sprintf("CSS selector %q is not supported yet", selectorText))
		}
		html.logUnsupportedStyleProperties(css[open+1:close], seen, "style rule")
		css = css[close+1:]
	}
}
