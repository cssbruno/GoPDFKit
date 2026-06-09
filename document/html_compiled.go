// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"strings"
)

// CompiledHTML stores the reusable parse products for an HTML fragment.
// It is safe to reuse across documents as long as callers do not mutate values
// reachable through Tokens().
type CompiledHTML struct {
	tokens            []HTMLSegmentType
	cssRules          []htmlCSSRule
	styleDeclarations map[string]map[string]string
	elementEnd        []int
	elementText       []compiledHTMLText
	maxDepth          int
	tables            map[int]compiledHTMLTable
	inlineSVGs        map[int]compiledInlineSVG
}

type compiledHTMLTable struct {
	table htmlTableType
	end   int
}

type compiledInlineSVG struct {
	svg SVG
	end int
}

type compiledHTMLText struct {
	plain     string
	preserved string
	ok        bool
}

// CompileHTML tokenizes an HTML fragment, parses CSS rules, records element
// boundaries, pre-parses tables, and pre-parses inline SVGs for repeated
// rendering.
func CompileHTML(htmlStr string) (*CompiledHTML, error) {
	return compileHTML(htmlStr, true)
}

func compileHTML(htmlStr string, cacheReusableData bool) (*CompiledHTML, error) {
	tokens := htmlTokenize(htmlStr, make(map[string]map[string]string))
	compiled := compileHTMLTokens(tokens, cacheReusableData)
	if err := compiled.compileInlineSVGs(); err != nil {
		return nil, err
	}
	return compiled, nil
}

func compileHTMLTokens(tokens []HTMLSegmentType, cacheReusableData bool) *CompiledHTML {
	compiled := &CompiledHTML{
		tokens:     tokens,
		cssRules:   htmlCollectCSSRules(tokens),
		elementEnd: make([]int, len(tokens)),
		tables:     make(map[int]compiledHTMLTable),
		inlineSVGs: make(map[int]compiledInlineSVG),
	}
	if cacheReusableData {
		compiled.styleDeclarations = make(map[string]map[string]string)
		compiled.elementText = make([]compiledHTMLText, len(tokens))
	}
	for i := range compiled.elementEnd {
		compiled.elementEnd[i] = i
	}

	type openElement struct {
		tag   string
		index int
	}
	stack := make([]openElement, 0, 16)
	for i, token := range tokens {
		switch token.Cat {
		case 'O':
			if !htmlClosePops(token.Str) {
				continue
			}
			stack = append(stack, openElement{tag: token.Str, index: i})
			if len(stack) > compiled.maxDepth {
				compiled.maxDepth = len(stack)
			}
		case 'C':
			for len(stack) > 0 {
				top := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				compiled.elementEnd[top.index] = i
				if top.tag == token.Str {
					break
				}
			}
		}
	}
	for _, open := range stack {
		compiled.elementEnd[open.index] = len(tokens) - 1
	}

	for i, token := range tokens {
		if token.Cat == 'O' && cacheReusableData {
			compiled.compileStyleDeclarations(token.Attr)
			compiled.compileElementText(i, token.Str)
		}
		if token.Cat == 'O' && token.Str == "table" {
			table, end := parseHTMLTable(tokens, i)
			compiled.tables[i] = compiledHTMLTable{table: table, end: end}
		}
	}
	return compiled
}

func (compiled *CompiledHTML) compileElementText(start int, tag string) {
	if !htmlCompiledTextTag(tag) {
		return
	}
	tokens, _ := compiled.collectElementTokens(start, tag)
	if len(tokens) < 2 {
		return
	}
	inner := tokens[1 : len(tokens)-1]
	compiled.elementText[start] = compiledHTMLText{
		plain:     htmlPlainTextWithMode(inner, false),
		preserved: htmlPlainTextWithMode(inner, true),
		ok:        true,
	}
}

func htmlCompiledTextTag(tag string) bool {
	switch tag {
	case "p", "div", "section", "article", "header", "footer", "figure", "figcaption",
		"h1", "h2", "h3", "h4", "h5", "h6":
		return true
	default:
		return false
	}
}

func (compiled *CompiledHTML) compileStyleDeclarations(attrs map[string]string) {
	if attrs == nil {
		return
	}
	style := attrs["style"]
	if strings.TrimSpace(style) == "" || !strings.Contains(style, ":") {
		return
	}
	if _, ok := compiled.styleDeclarations[style]; ok {
		return
	}
	compiled.styleDeclarations[style] = parseStyleDeclarations(style)
}

func (compiled *CompiledHTML) compileInlineSVGs() error {
	for i := 0; i < len(compiled.tokens); i++ {
		token := compiled.tokens[i]
		if token.Cat != 'O' || token.Str != "svg" {
			if token.Cat == 'O' && (token.Str == "style" || token.Str == "script" || token.Str == "head") {
				i = compiled.skipElement(i, token.Str)
			}
			continue
		}
		svgTokens, end := compiled.collectElementTokens(i, "svg")
		if len(svgTokens) == 0 {
			continue
		}
		svg, err := SVGParse([]byte(htmlSerializeTokens(svgTokens)))
		if err != nil {
			return err
		}
		compiled.inlineSVGs[i] = compiledInlineSVG{svg: svg, end: end}
		i = end
	}
	return nil
}

func (compiled *CompiledHTML) styleDeclaration(style string) (map[string]string, bool) {
	if compiled == nil {
		return nil, false
	}
	declarations, ok := compiled.styleDeclarations[style]
	return declarations, ok
}

func (compiled *CompiledHTML) text(start int, preserveWhitespace bool) (string, bool) {
	if compiled == nil || start < 0 || start >= len(compiled.elementText) {
		return "", false
	}
	text := compiled.elementText[start]
	if !text.ok {
		return "", false
	}
	if preserveWhitespace {
		return text.preserved, true
	}
	return text.plain, true
}

// Tokens returns the token stream used by the compiled HTML fragment.
func (compiled *CompiledHTML) Tokens() []HTMLSegmentType {
	if compiled == nil {
		return nil
	}
	return compiled.tokens
}

func (compiled *CompiledHTML) validate() error {
	if compiled == nil {
		return errors.New("compiled HTML is nil")
	}
	return nil
}

func (compiled *CompiledHTML) collectElementTokens(start int, tag string) ([]HTMLSegmentType, int) {
	if compiled == nil || start < 0 || start >= len(compiled.tokens) {
		return nil, 0
	}
	end := compiled.elementEnd[start]
	if end < start || end >= len(compiled.tokens) {
		return htmlCollectElementTokens(compiled.tokens, start, tag)
	}
	return compiled.tokens[start : end+1], end
}

func (compiled *CompiledHTML) skipElement(start int, tag string) int {
	_, end := compiled.collectElementTokens(start, tag)
	return end
}

func (compiled *CompiledHTML) table(start int) (htmlTableType, int, bool) {
	if compiled == nil {
		return htmlTableType{}, start, false
	}
	table, ok := compiled.tables[start]
	return table.table, table.end, ok
}

func (compiled *CompiledHTML) inlineSVG(start int) (SVG, int, bool) {
	if compiled == nil {
		return SVG{}, start, false
	}
	svg, ok := compiled.inlineSVGs[start]
	return svg.svg, svg.end, ok
}
