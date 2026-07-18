// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	stdhtml "html"
	"net/url"
	"sort"
	"strings"
	"unicode"
)

func htmlLinkTarget(href string) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", nil
	}
	if strings.HasPrefix(href, "#") {
		name := strings.TrimPrefix(href, "#")
		if err := validateTypedDestinationName(name); err != nil {
			return "", fmt.Errorf("invalid HTML fragment link %q: %w", href, err)
		}
		return "#" + name, nil
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("invalid HTML link target: %w", err)
	}
	if allowedExternalLinkScheme(u.Scheme) {
		return href, nil
	}
	return "", fmt.Errorf("unsupported HTML link scheme: %s", u.Scheme)
}

func htmlImageTypeFromMime(mimeType string) string {
	switch mimeType {
	case "image/png":
		return "png"
	case "image/jpg", "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func htmlCollectElementTokens(tokens []HTMLSegmentType, start int, tag string) ([]HTMLSegmentType, int) {
	if start < 0 || start >= len(tokens) {
		return nil, len(tokens) - 1
	}
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == tag {
			depth++
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == tag {
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
			out.WriteString(htmlCanonicalSVGName(token.Str))
			keys := make([]string, 0, len(token.Attr))
			for key := range token.Attr {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				out.WriteByte(' ')
				out.WriteString(htmlCanonicalSVGName(key))
				out.WriteString(`="`)
				out.WriteString(stdhtml.EscapeString(token.Attr[key]))
				out.WriteByte('"')
			}
			out.WriteByte('>')
		case 'C':
			out.WriteString("</")
			out.WriteString(htmlCanonicalSVGName(token.Str))
			out.WriteByte('>')
		case 'T':
			out.WriteString(stdhtml.EscapeString(token.Str))
		}
	}
	return out.String()
}

func htmlCanonicalSVGName(name string) string {
	switch name {
	case "lineargradient":
		return "linearGradient"
	case "radialgradient":
		return "radialGradient"
	case "clippath":
		return "clipPath"
	case "viewbox":
		return "viewBox"
	case "preserveaspectratio":
		return "preserveAspectRatio"
	case "gradientunits":
		return "gradientUnits"
	case "gradienttransform":
		return "gradientTransform"
	case "patternunits":
		return "patternUnits"
	case "patterncontentunits":
		return "patternContentUnits"
	case "patterntransform":
		return "patternTransform"
	case "clippathunits":
		return "clipPathUnits"
	default:
		return name
	}
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
	needSpace, lastWasNewline := false, false
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
				needSpace, lastWasNewline = false, true
			}
		case 'C':
			switch token.Str {
			case "p", "div", "section", "article", "header", "footer", "figure", "figcaption", "li", "dt", "dd":
				out.WriteByte('\n')
				needSpace, lastWasNewline = false, true
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

func collapseHTMLWhitespace(text string) string {
	if text == "" {
		return ""
	}
	needsCollapse, previousSpace, textSeen := false, false, false
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
	leadingSpace, pendingSpace, wroteText := false, false, false
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
				wroteText, pendingSpace, textStart = true, false, -1
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
		wroteText, pendingSpace = true, false
	}
	if !wroteText {
		return " "
	}
	if pendingSpace {
		out.WriteByte(' ')
	}
	return out.String()
}
