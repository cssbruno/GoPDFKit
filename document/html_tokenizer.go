// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	stdhtml "html"
	"strings"
	"unicode"
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
	return htmlTokenize(htmlStr, nil)
}

func htmlTokenize(htmlStr string, attrCache map[string]map[string]string) []HTMLSegmentType {
	list := make([]HTMLSegmentType, 0, strings.Count(htmlStr, "<")+1)
	for len(htmlStr) > 0 {
		tagStart := strings.IndexByte(htmlStr, '<')
		if tagStart < 0 {
			if htmlStr != "" {
				list = append(list, HTMLSegmentType{Cat: 'T', Str: htmlUnescapeString(htmlStr)})
			}
			break
		}
		if tagStart > 0 {
			list = append(list, HTMLSegmentType{Cat: 'T', Str: htmlUnescapeString(htmlStr[:tagStart])})
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
			list = append(list, HTMLSegmentType{Cat: 'T', Str: htmlUnescapeString(htmlStr)})
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
			list = append(list, HTMLSegmentType{Cat: 'C', Str: name})
			continue
		}
		list = append(list, HTMLSegmentType{Cat: 'O', Str: name, Attr: attrs})
		if selfClosing {
			list = append(list, HTMLSegmentType{Cat: 'C', Str: name})
		}
	}
	return list
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
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "!") || strings.HasPrefix(raw, "?") {
		return "", nil, false, false
	}
	if strings.HasPrefix(raw, "/") {
		closeTag = true
		raw = strings.TrimSpace(raw[1:])
	}
	if strings.HasSuffix(raw, "/") {
		selfClosing = true
		raw = strings.TrimSpace(raw[:len(raw)-1])
	}
	nameEnd := 0
	for nameEnd < len(raw) {
		r := rune(raw[nameEnd])
		if unicode.IsSpace(r) {
			break
		}
		nameEnd++
	}
	name = strings.ToLower(raw[:nameEnd])
	if closeTag || nameEnd >= len(raw) {
		return name, nil, closeTag, selfClosing
	}
	attrRaw := raw[nameEnd:]
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
	attrs := make(map[string]string, strings.Count(raw, "="))
	for len(raw) > 0 {
		raw = strings.TrimLeftFunc(raw, unicode.IsSpace)
		if raw == "" {
			break
		}
		nameEnd := 0
		for nameEnd < len(raw) {
			r := rune(raw[nameEnd])
			if unicode.IsSpace(r) || raw[nameEnd] == '=' {
				break
			}
			nameEnd++
		}
		name := strings.ToLower(raw[:nameEnd])
		raw = strings.TrimLeftFunc(raw[nameEnd:], unicode.IsSpace)
		value := ""
		if strings.HasPrefix(raw, "=") {
			raw = strings.TrimLeftFunc(raw[1:], unicode.IsSpace)
			if strings.HasPrefix(raw, `"`) || strings.HasPrefix(raw, `'`) {
				quote := raw[0]
				raw = raw[1:]
				valueEnd := strings.IndexByte(raw, quote)
				if valueEnd < 0 {
					value = raw
					raw = ""
				} else {
					value = raw[:valueEnd]
					raw = raw[valueEnd+1:]
				}
			} else {
				valueEnd := 0
				for valueEnd < len(raw) && !unicode.IsSpace(rune(raw[valueEnd])) {
					valueEnd++
				}
				value = raw[:valueEnd]
				raw = raw[valueEnd:]
			}
		}
		if name != "" {
			attrs[name] = htmlUnescapeString(value)
		}
	}
	return attrs
}
