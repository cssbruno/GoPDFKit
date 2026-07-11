// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	stdhtml "html"
	"strings"
)

// HTMLSegmentType identifies one token from a supported HTML fragment: literal
// text, an opening tag, or a closing tag.
type HTMLSegmentType struct {
	// Cat identifies the token category: 'T' for text, 'O' for opening tags,
	// or 'C' for closing tags.
	Cat byte
	// Str contains text content for 'T' tokens, or the lower-case tag name for
	// 'O' and 'C' tokens.
	Str string
	// Attr contains lower-case attribute keys for opening tags.
	Attr map[string]string
}

// HTMLTokenize returns a list of supported HTML tags and literal text elements.
func HTMLTokenize(htmlStr string) []HTMLSegmentType {
	tokens, _ := HTMLTokenizeContext(context.Background(), htmlStr)
	return tokens
}

// HTMLTokenizeContext returns HTML tokens and checks ctx during tokenization.
func HTMLTokenizeContext(ctx context.Context, htmlStr string) ([]HTMLSegmentType, error) {
	if len(htmlStr) > htmlDefaultMaxHTMLBytes {
		return nil, ErrHTMLLimitExceeded
	}
	return htmlTokenizeContext(ctx, htmlStr, nil)
}

func htmlTokenize(htmlStr string, attrCache map[string]map[string]string) []HTMLSegmentType {
	tokens, _ := htmlTokenizeContext(context.Background(), htmlStr, attrCache)
	return tokens
}

func htmlTokenizeContext(ctx context.Context, htmlStr string, attrCache map[string]map[string]string) ([]HTMLSegmentType, error) {
	capacity := strings.Count(htmlStr, "<") + 1
	if capacity > htmlMaxTokenCount {
		capacity = htmlMaxTokenCount
	}
	list := make([]HTMLSegmentType, 0, capacity)
	appendToken := func(token HTMLSegmentType) error {
		if len(list) >= htmlMaxTokenCount {
			return ErrHTMLLimitExceeded
		}
		list = append(list, token)
		return nil
	}
	processed := 0
	for len(htmlStr) > 0 {
		if processed%1024 == 0 {
			if err := outputCanceledError(ctx); err != nil {
				return nil, err
			}
		}
		processed++
		tagStart := strings.IndexByte(htmlStr, '<')
		if tagStart < 0 {
			if htmlStr != "" {
				if err := appendToken(HTMLSegmentType{Cat: 'T', Str: htmlUnescapeString(htmlStr)}); err != nil {
					return nil, err
				}
			}
			break
		}
		if tagStart > 0 {
			if err := appendToken(HTMLSegmentType{Cat: 'T', Str: htmlUnescapeString(htmlStr[:tagStart])}); err != nil {
				return nil, err
			}
			htmlStr = htmlStr[tagStart:]
			continue
		}
		if strings.HasPrefix(htmlStr, "<!--") {
			commentEnd := strings.Index(htmlStr, "-->")
			if commentEnd < 0 {
				break
			}
			htmlStr = htmlStr[commentEnd+3:]
			continue
		}
		tagEnd := htmlTagEnd(htmlStr)
		if tagEnd < 0 {
			if err := appendToken(HTMLSegmentType{Cat: 'T', Str: htmlUnescapeString(htmlStr)}); err != nil {
				return nil, err
			}
			break
		}
		rawTag := htmlStr[1:tagEnd]
		htmlStr = htmlStr[tagEnd+1:]
		if strings.HasPrefix(rawTag, "!--") {
			continue
		}
		name, attrs, closeTag, selfClosing := parseHTMLTagWithAttrCache(rawTag, attrCache)
		if name == "" {
			continue
		}
		if closeTag {
			if err := appendToken(HTMLSegmentType{Cat: 'C', Str: name}); err != nil {
				return nil, err
			}
			continue
		}
		if err := appendToken(HTMLSegmentType{Cat: 'O', Str: name, Attr: attrs}); err != nil {
			return nil, err
		}
		if selfClosing {
			if err := appendToken(HTMLSegmentType{Cat: 'C', Str: name}); err != nil {
				return nil, err
			}
		}
	}
	if err := outputCanceledError(ctx); err != nil {
		return nil, err
	}
	return list, nil
}

func htmlUnescapeString(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	return stdhtml.UnescapeString(s)
}

func htmlTagEnd(s string) int {
	quote := rune(0)
	for j, r := range s {
		if j == 0 {
			continue
		}
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '>':
			return j
		}
	}
	return -1
}

func parseHTMLTagWithAttrCache(raw string, attrCache map[string]map[string]string) (name string, attrs map[string]string, closeTag, selfClosing bool) {
	raw = htmlTrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "!") || strings.HasPrefix(raw, "?") {
		return "", nil, false, false
	}
	if strings.HasPrefix(raw, "/") {
		closeTag = true
		raw = htmlTrimSpace(raw[1:])
	}
	if strings.HasSuffix(raw, "/") {
		selfClosing = true
		raw = htmlTrimSpace(raw[:len(raw)-1])
	}
	nameEnd := 0
	for nameEnd < len(raw) {
		if htmlIsSpace(raw[nameEnd]) {
			break
		}
		nameEnd++
	}
	name = internHTMLName(strings.ToLower(raw[:nameEnd]))
	if closeTag || nameEnd >= len(raw) {
		return name, nil, closeTag, selfClosing
	}
	attrRaw := htmlTrimSpace(raw[nameEnd:])
	if attrRaw == "" {
		return name, nil, closeTag, selfClosing
	}
	if attrCache != nil {
		if cached, ok := attrCache[attrRaw]; ok {
			return name, cached, closeTag, selfClosing
		}
	}
	attrs = parseHTMLAttrs(attrRaw)
	if attrCache != nil {
		attrCache[attrRaw] = attrs
	}
	return name, attrs, closeTag, selfClosing
}

func parseHTMLAttrs(raw string) map[string]string {
	raw = htmlTrimSpace(raw)
	if raw == "" {
		return nil
	}
	attrs := make(map[string]string, strings.Count(raw, "="))
	pos := 0
	for pos < len(raw) {
		pos = htmlSkipSpace(raw, pos)
		if pos >= len(raw) {
			break
		}
		nameStart := pos
		for pos < len(raw) {
			if htmlIsSpace(raw[pos]) || raw[pos] == '=' {
				break
			}
			pos++
		}
		name := internHTMLName(strings.ToLower(raw[nameStart:pos]))
		pos = htmlSkipSpace(raw, pos)
		value := ""
		if pos < len(raw) && raw[pos] == '=' {
			pos++
			pos = htmlSkipSpace(raw, pos)
			if pos < len(raw) && (raw[pos] == '"' || raw[pos] == '\'') {
				quote := raw[pos]
				pos++
				valueStart := pos
				for pos < len(raw) && raw[pos] != quote {
					pos++
				}
				value = raw[valueStart:pos]
				if pos < len(raw) {
					pos++
				}
			} else {
				valueStart := pos
				for pos < len(raw) && !htmlIsSpace(raw[pos]) {
					pos++
				}
				value = raw[valueStart:pos]
			}
		}
		if name != "" {
			attrs[name] = htmlUnescapeString(value)
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func htmlIsSpace(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '\f':
		return true
	default:
		return false
	}
}

func htmlSkipSpace(s string, pos int) int {
	for pos < len(s) && htmlIsSpace(s[pos]) {
		pos++
	}
	return pos
}

func htmlTrimSpace(s string) string {
	start := htmlSkipSpace(s, 0)
	end := len(s)
	for end > start && htmlIsSpace(s[end-1]) {
		end--
	}
	return s[start:end]
}

func internHTMLName(name string) string {
	switch name {
	case "a", "abbr", "align", "alt", "aria-label", "article", "b", "body", "border", "br", "caption", "cellpadding", "center", "class", "code", "color", "colspan", "data-pdf-footer", "dd", "del", "dir", "div", "dl", "dt", "em", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "head", "headers", "header", "height", "href", "hr", "i", "id", "img", "ins", "kbd", "lang", "li", "ol", "p", "pre", "rel", "right", "role", "rowspan", "s", "samp", "scope", "script", "section", "size", "src", "strike", "strong", "style", "sub", "sup", "svg", "table", "target", "tbody", "td", "tfoot", "th", "thead", "title", "tr", "u", "ul", "valign", "width":
		return name
	default:
		return name
	}
}
