// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// CompiledHTML stores the reusable parse products for an HTML fragment. It is
// safe to reuse across documents.
type CompiledHTML struct {
	tokens            []HTMLSegmentType
	cssRules          []htmlCSSRule
	styleDeclarations map[string]map[string]string
	nodeIndexes       []compiledHTMLNode
	tokenNode         []int
	elementEnd        []int
	elementDecl       []map[string]string
	elementText       []compiledHTMLText
	elementMeta       []htmlElementMetadata
	maxDepth          int
	tables            map[int]compiledHTMLTable
	inlineSVGs        map[int]compiledInlineSVG
	dataImages        map[int]compiledHTMLDataImage
	recovery          []CompiledHTMLRecoveryIssue
}

type compiledHTMLTable struct {
	table htmlTableType
	end   int
	start int
}

type compiledInlineSVG struct {
	svg *SVG
	end int
}

type compiledHTMLText struct {
	plain     string
	preserved string
	ok        bool
}

type compiledHTMLNode struct {
	Parent      int
	FirstChild  int
	NextSibling int
	Token       int
	EndToken    int
}

type compiledHTMLDataImage struct {
	name    string
	options ImageOptions
	data    []byte
}

// CompiledHTMLStats summarizes reusable parse products in a compiled fragment.
type CompiledHTMLStats struct {
	Tokens       int
	Nodes        int
	Tables       int
	Images       int
	InlineSVGs   int
	CSSRules     int
	Recovery     int
	MaxDepth     int
	CachedText   int
	CachedStyles int
}

// CompiledHTMLRecoveryIssue describes one malformed-fragment recovery decision
// made while building the compiled node model.
type CompiledHTMLRecoveryIssue struct {
	Kind  string
	Tag   string
	Token int
}

// CompileHTML tokenizes an HTML fragment, parses CSS rules, records element
// boundaries, pre-parses tables, and pre-parses inline SVGs for repeated
// rendering.
func CompileHTML(htmlStr string) (*CompiledHTML, error) {
	return compileHTMLWithDataImageLimit(htmlStr, true, htmlDefaultMaxDataImageBytes)
}

func compileHTML(htmlStr string, cacheReusableData bool) (*CompiledHTML, error) {
	return compileHTMLWithDataImageLimit(htmlStr, cacheReusableData, htmlDefaultMaxDataImageBytes)
}

func compileHTMLWithDataImageLimit(htmlStr string, cacheReusableData bool, maxDataImageBytes int) (*CompiledHTML, error) {
	tokens := htmlTokenize(htmlStr, make(map[string]map[string]string))
	compiled := compileHTMLTokens(tokens, cacheReusableData)
	if err := compiled.compileDataImages(maxDataImageBytes); err != nil {
		return nil, err
	}
	if err := compiled.compileInlineSVGs(); err != nil {
		return nil, err
	}
	return compiled, nil
}

func compileHTMLTokens(tokens []HTMLSegmentType, cacheReusableData bool) *CompiledHTML {
	compiled := &CompiledHTML{
		tokens:     tokens,
		cssRules:   htmlCollectCSSRules(tokens),
		tokenNode:  make([]int, len(tokens)),
		elementEnd: make([]int, len(tokens)),
		tables:     make(map[int]compiledHTMLTable),
		inlineSVGs: make(map[int]compiledInlineSVG),
		dataImages: make(map[int]compiledHTMLDataImage),
	}
	if cacheReusableData {
		compiled.styleDeclarations = make(map[string]map[string]string)
		compiled.elementDecl = make([]map[string]string, len(tokens))
		compiled.elementText = make([]compiledHTMLText, len(tokens))
		compiled.elementMeta = make([]htmlElementMetadata, len(tokens))
	}
	for i := range compiled.tokenNode {
		compiled.tokenNode[i] = -1
	}
	for i := range compiled.elementEnd {
		compiled.elementEnd[i] = i
	}
	compiled.buildNodeIndexes()

	for i, token := range tokens {
		if token.Cat == 'O' && cacheReusableData {
			compiled.compileStyleDeclarations(token.Attr)
			compiled.elementMeta[i] = htmlElementMetadataFromSegment(token)
			compiled.compileElementText(i, token.Str)
		}
		if token.Cat == 'O' && token.Str == "table" {
			table, end := parseHTMLTable(tokens, i)
			compiled.tables[i] = compiledHTMLTable{table: table, end: end, start: i}
		}
	}
	if cacheReusableData {
		compiled.compileElementDeclarations()
	}
	return compiled
}

func (compiled *CompiledHTML) buildNodeIndexes() {
	compiled.nodeIndexes = make([]compiledHTMLNode, 0, len(compiled.tokens))
	stack := make([]int, 0, 16)
	lastChildByParent := make([]int, 0, len(compiled.tokens))
	for i, token := range compiled.tokens {
		switch token.Cat {
		case 'O':
			if !htmlClosePops(token.Str) {
				continue
			}
			parent := -1
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			nodeIndex := len(compiled.nodeIndexes)
			compiled.nodeIndexes = append(compiled.nodeIndexes, compiledHTMLNode{
				Parent:      parent,
				FirstChild:  -1,
				NextSibling: -1,
				Token:       i,
				EndToken:    i,
			})
			lastChildByParent = append(lastChildByParent, -1)
			compiled.tokenNode[i] = nodeIndex
			if parent >= 0 {
				if compiled.nodeIndexes[parent].FirstChild < 0 {
					compiled.nodeIndexes[parent].FirstChild = nodeIndex
				} else {
					compiled.nodeIndexes[lastChildByParent[parent]].NextSibling = nodeIndex
				}
				lastChildByParent[parent] = nodeIndex
			}
			stack = append(stack, nodeIndex)
			if len(stack) > compiled.maxDepth {
				compiled.maxDepth = len(stack)
			}
		case 'C':
			matched := false
			for len(stack) > 0 {
				nodeIndex := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				node := &compiled.nodeIndexes[nodeIndex]
				open := compiled.tokens[node.Token]
				node.EndToken = i
				compiled.elementEnd[node.Token] = i
				if open.Str != token.Str {
					compiled.recovery = append(compiled.recovery, CompiledHTMLRecoveryIssue{Kind: "misnested", Tag: open.Str, Token: node.Token})
					continue
				}
				matched = true
				break
			}
			if !matched {
				compiled.recovery = append(compiled.recovery, CompiledHTMLRecoveryIssue{Kind: "unexpected-close", Tag: token.Str, Token: i})
			}
		}
	}
	for _, nodeIndex := range stack {
		node := &compiled.nodeIndexes[nodeIndex]
		node.EndToken = len(compiled.tokens) - 1
		compiled.elementEnd[node.Token] = node.EndToken
		compiled.recovery = append(compiled.recovery, CompiledHTMLRecoveryIssue{Kind: "unclosed", Tag: compiled.tokens[node.Token].Str, Token: node.Token})
	}
}

func (compiled *CompiledHTML) compileElementDeclarations() {
	ancestors := make([]HTMLSegmentType, 0, compiled.maxDepth)
	ancestorMeta := make([]htmlElementMetadata, 0, compiled.maxDepth)
	for i, token := range compiled.tokens {
		switch token.Cat {
		case 'O':
			compiled.elementDecl[i] = htmlElementDeclarationsWithStyleMeta(token, compiled.elementMeta[i], compiled.cssRules, compiled.inlineStyleDeclarations(token.Attr), ancestors, ancestorMeta)
			if htmlClosePops(token.Str) {
				ancestors = append(ancestors, token)
				ancestorMeta = append(ancestorMeta, compiled.elementMeta[i])
			}
		case 'C':
			if !htmlClosePops(token.Str) {
				continue
			}
			for len(ancestors) > 0 {
				last := len(ancestors) - 1
				open := ancestors[last]
				ancestors = ancestors[:last]
				ancestorMeta = ancestorMeta[:last]
				if open.Str == token.Str {
					break
				}
			}
			continue
		}
	}
}

func (compiled *CompiledHTML) inlineStyleDeclarations(attrs map[string]string) map[string]string {
	if attrs == nil {
		return nil
	}
	style := attrs["style"]
	if strings.TrimSpace(style) == "" || !strings.Contains(style, ":") {
		return nil
	}
	if compiled.styleDeclarations == nil {
		return parseStyleDeclarations(style)
	}
	if declarations, ok := compiled.styleDeclarations[style]; ok {
		return declarations
	}
	declarations := parseStyleDeclarations(style)
	compiled.styleDeclarations[style] = declarations
	return declarations
}

func (compiled *CompiledHTML) ancestorsForToken(tokenIndex int) []HTMLSegmentType {
	if compiled == nil || tokenIndex < 0 || tokenIndex >= len(compiled.tokenNode) {
		return nil
	}
	nodeIndex := compiled.tokenNode[tokenIndex]
	if nodeIndex < 0 || nodeIndex >= len(compiled.nodeIndexes) {
		return nil
	}
	var rev []HTMLSegmentType
	for parent := compiled.nodeIndexes[nodeIndex].Parent; parent >= 0; parent = compiled.nodeIndexes[parent].Parent {
		rev = append(rev, compiled.tokens[compiled.nodeIndexes[parent].Token])
	}
	ancestors := make([]HTMLSegmentType, len(rev))
	for i := range rev {
		ancestors[i] = rev[len(rev)-1-i]
	}
	return ancestors
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
	if htmlCompiledTextHasNestedBlock(inner) {
		return
	}
	compiled.elementText[start] = compiledHTMLText{
		plain:     htmlPlainTextWithMode(inner, false),
		preserved: htmlPlainTextWithMode(inner, true),
		ok:        true,
	}
}

func htmlCompiledTextHasNestedBlock(tokens []HTMLSegmentType) bool {
	for _, token := range tokens {
		if token.Cat == 'O' && htmlCompiledNestedTextTag(token.Str) {
			return true
		}
	}
	return false
}

func htmlCompiledNestedTextTag(tag string) bool {
	switch tag {
	case "p", "div", "section", "article", "header", "footer", "figure", "figcaption",
		"li", "caption", "h1", "h2", "h3", "h4", "h5", "h6", "dt", "dd":
		return true
	default:
		return false
	}
}

func htmlCompiledTextTag(tag string) bool {
	switch tag {
	case "p", "figcaption", "li", "caption", "h1", "h2", "h3", "h4", "h5", "h6", "dt", "dd":
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
	var cache map[string]*SVG
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
		svgText := htmlSerializeTokens(svgTokens)
		svg, ok := cache[svgText]
		if !ok {
			parsed, err := SVGParse([]byte(svgText))
			if err != nil {
				return err
			}
			svg = &parsed
			if cache == nil {
				cache = make(map[string]*SVG)
			}
			cache[svgText] = svg
		}
		compiled.inlineSVGs[i] = compiledInlineSVG{svg: svg, end: end}
		i = end
	}
	return nil
}

func (compiled *CompiledHTML) compileDataImages(maxBytes int) error {
	if compiled == nil {
		return nil
	}
	var cache map[string]compiledHTMLDataImage
	for i, token := range compiled.tokens {
		if token.Cat != 'O' || token.Str != "img" {
			continue
		}
		src := strings.TrimSpace(token.Attr["src"])
		if cache != nil {
			if source, ok := cache[src]; ok {
				compiled.dataImages[i] = source
				continue
			}
		}
		source, ok, err := compileHTMLDataImageSource(src, maxBytes)
		if err != nil {
			return err
		}
		if ok {
			if cache == nil {
				cache = make(map[string]compiledHTMLDataImage)
			}
			cache[src] = source
			compiled.dataImages[i] = source
		}
	}
	return nil
}

func compileHTMLDataImageSource(src string, maxBytes int) (compiledHTMLDataImage, bool, error) {
	src = strings.TrimSpace(src)
	if !strings.HasPrefix(strings.ToLower(src), "data:") {
		return compiledHTMLDataImage{}, false, nil
	}
	media, data, ok := strings.Cut(src[5:], ",")
	if !ok {
		return compiledHTMLDataImage{}, false, errors.New("invalid HTML image data URI")
	}
	parts := strings.Split(media, ";")
	mimeType := strings.ToLower(strings.TrimSpace(parts[0]))
	base64Encoded := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			base64Encoded = true
		}
	}
	if !base64Encoded {
		return compiledHTMLDataImage{}, false, errors.New("HTML image data URI must be base64 encoded")
	}
	imageType := htmlImageTypeFromMime(mimeType)
	if imageType == "" {
		return compiledHTMLDataImage{}, false, fmt.Errorf("unsupported HTML image type: %s", mimeType)
	}
	if maxBytes <= 0 {
		maxBytes = htmlDefaultMaxDataImageBytes
	}
	if base64.StdEncoding.DecodedLen(len(data)) > maxBytes {
		return compiledHTMLDataImage{}, false, errors.New("HTML image data URI exceeds maximum size")
	}
	buf, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return compiledHTMLDataImage{}, false, fmt.Errorf("invalid HTML image data URI: %w", err)
	}
	if len(buf) > maxBytes {
		return compiledHTMLDataImage{}, false, errors.New("HTML image data URI exceeds maximum size")
	}
	name := compiledHTMLDataImageName(buf)
	return compiledHTMLDataImage{
		name:    name,
		options: ImageOptions{ImageType: imageType, ReadDpi: true},
		data:    buf,
	}, true, nil
}

func compiledHTMLDataImageName(data []byte) string {
	sum := sha256.Sum256(data)
	return fmt.Sprintf("html-data-image-%x", sum)
}

func (compiled *CompiledHTML) styleDeclaration(style string) (map[string]string, bool) {
	if compiled == nil {
		return nil, false
	}
	declarations, ok := compiled.styleDeclarations[style]
	return declarations, ok
}

func (compiled *CompiledHTML) declarations(start int) (map[string]string, bool) {
	if compiled == nil || start < 0 || start >= len(compiled.elementDecl) {
		return nil, false
	}
	decl := compiled.elementDecl[start]
	return decl, decl != nil
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

// Tokens returns a copy of the token stream used by the compiled HTML fragment.
func (compiled *CompiledHTML) Tokens() []HTMLSegmentType {
	if compiled == nil {
		return nil
	}
	return cloneHTMLTokens(compiled.tokens)
}

// Stats returns diagnostics for the reusable parse products stored in the
// compiled HTML fragment.
func (compiled *CompiledHTML) Stats() CompiledHTMLStats {
	if compiled == nil {
		return CompiledHTMLStats{}
	}
	stats := CompiledHTMLStats{
		Tokens:       len(compiled.tokens),
		Nodes:        len(compiled.nodeIndexes),
		Tables:       len(compiled.tables),
		Images:       len(compiled.dataImages),
		InlineSVGs:   len(compiled.inlineSVGs),
		CSSRules:     len(compiled.cssRules),
		Recovery:     len(compiled.recovery),
		MaxDepth:     compiled.maxDepth,
		CachedStyles: len(compiled.styleDeclarations),
	}
	for _, text := range compiled.elementText {
		if text.ok {
			stats.CachedText++
		}
	}
	return stats
}

// RecoveryIssues returns malformed-fragment recovery decisions from
// compilation. The returned slice is a copy.
func (compiled *CompiledHTML) RecoveryIssues() []CompiledHTMLRecoveryIssue {
	if compiled == nil || len(compiled.recovery) == 0 {
		return nil
	}
	out := make([]CompiledHTMLRecoveryIssue, len(compiled.recovery))
	copy(out, compiled.recovery)
	return out
}

// DebugDump returns a compact tree dump for diagnostics.
func (compiled *CompiledHTML) DebugDump() string {
	if compiled == nil {
		return ""
	}
	var out strings.Builder
	for i, node := range compiled.nodeIndexes {
		if node.Parent < 0 {
			compiled.debugDumpNode(&out, i, 0)
		}
	}
	return out.String()
}

func (compiled *CompiledHTML) debugDumpNode(out *strings.Builder, nodeIndex, depth int) {
	if nodeIndex < 0 || nodeIndex >= len(compiled.nodeIndexes) {
		return
	}
	node := compiled.nodeIndexes[nodeIndex]
	token := compiled.tokens[node.Token]
	out.WriteString(strings.Repeat("  ", depth))
	out.WriteString(token.Str)
	out.WriteString(" token=")
	out.WriteString(fmt.Sprint(node.Token))
	out.WriteString(" end=")
	out.WriteString(fmt.Sprint(node.EndToken))
	out.WriteByte('\n')
	for child := node.FirstChild; child >= 0; child = compiled.nodeIndexes[child].NextSibling {
		compiled.debugDumpNode(out, child, depth+1)
	}
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
	if start < len(compiled.tokenNode) {
		if nodeIndex := compiled.tokenNode[start]; nodeIndex >= 0 && nodeIndex < len(compiled.nodeIndexes) {
			end = compiled.nodeIndexes[nodeIndex].EndToken
		}
	}
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

func (compiled *CompiledHTML) inlineSVG(start int) (*SVG, int, bool) {
	if compiled == nil {
		return nil, start, false
	}
	svg, ok := compiled.inlineSVGs[start]
	return svg.svg, svg.end, ok
}

func (compiled *CompiledHTML) dataImage(start int) (compiledHTMLDataImage, bool) {
	if compiled == nil {
		return compiledHTMLDataImage{}, false
	}
	img, ok := compiled.dataImages[start]
	return img, ok
}

func (compiled *CompiledHTML) validateDataImageLimit(maxBytes int) error {
	if compiled == nil {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = htmlDefaultMaxDataImageBytes
	}
	for _, img := range compiled.dataImages {
		if len(img.data) > maxBytes {
			return errors.New("HTML image data URI exceeds maximum size")
		}
	}
	return nil
}

func (img compiledHTMLDataImage) register(pdf *Document) (string, ImageOptions, error) {
	if pdf == nil {
		return "", img.options, errors.New("PDF document is nil")
	}
	if _, ok := pdf.images[img.name]; !ok {
		pdf.RegisterImageOptionsReader(img.name, img.options, bytes.NewReader(img.data))
		if pdf.err != nil {
			return "", img.options, pdf.err
		}
	}
	return img.name, img.options, nil
}

func validateHTMLImageSource(src string) error {
	src = strings.TrimSpace(src)
	if strings.HasPrefix(strings.ToLower(src), "data:") {
		return nil
	}
	if u, err := url.Parse(src); err == nil && u.Scheme != "" {
		switch strings.ToLower(u.Scheme) {
		case "http", "https":
			return errors.New("remote HTML images are disabled")
		case "file":
			return errors.New("file URL HTML images are disabled")
		}
	}
	return nil
}

func cloneHTMLTokens(tokens []HTMLSegmentType) []HTMLSegmentType {
	if len(tokens) == 0 {
		return nil
	}
	out := make([]HTMLSegmentType, len(tokens))
	for i, token := range tokens {
		out[i] = token
		if len(token.Attr) == 0 {
			continue
		}
		out[i].Attr = make(map[string]string, len(token.Attr))
		for key, value := range token.Attr {
			out[i].Attr[key] = value
		}
	}
	return out
}
