// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
)

// HTML is used for rendering a controlled subset of HTML fragments. It
// supports common text tags, links, paragraphs, headings, lists, tables, images,
// inline SVG, alignment, font color, font size, and CSS declarations that map
// directly to gopdfkit text and drawing operations.
//
// HTML is not a browser engine. It does not implement the full HTML parsing
// algorithm, CSS cascade, browser layout model, grid, floats, positioning,
// paged media, or browser-grade typography. It supports a bounded flexbox
// subset for direct child blocks. For predictable output, generate simple
// content that stays within the documented subset.
type HTML struct {
	pdf *Document
	// Link controls the style applied to rendered anchor text.
	Link struct {
		// ClrR defines the red component of rendered link text.
		ClrR int
		// ClrG defines the green component of rendered link text.
		ClrG int
		// ClrB defines the blue component of rendered link text.
		ClrB int
		// Bold renders link text with a bold font style.
		Bold bool
		// Italic renders link text with an italic font style.
		Italic bool
		// Underscore underlines rendered link text.
		Underscore bool
	}
	// AllowLocalImages permits img src values that reference local file paths.
	AllowLocalImages bool
	// MaxDataImageBytes limits the decoded size of data URI images.
	MaxDataImageBytes int
	// MaxHTMLBytes limits the size of one HTML fragment.
	MaxHTMLBytes int
	// MaxElementDepth limits nested HTML element depth.
	MaxElementDepth int
	// MaxGeneratedPages limits the number of pages one Write call may create.
	MaxGeneratedPages int
	// MaxTableRows limits the number of rows in one rendered HTML table.
	MaxTableRows int
	// DebugLog receives best-effort diagnostics for unsupported HTML or CSS.
	// Leave nil to keep rendering quiet.
	DebugLog              func(message string)
	renderStartPageCount  int
	renderCacheActive     bool
	dataImageCache        map[string]htmlImageSource
	styleDeclarationCache map[string]map[string]string
	compiledStyleCache    map[string]map[string]string
	inlineSVGCache        map[string]*SVG
}

type htmlImageSource struct {
	name    string
	options ImageOptions
}

const (
	htmlDefaultMaxDataImageBytes = 16 * 1024 * 1024
	htmlDefaultMaxHTMLBytes      = 4 * 1024 * 1024
	htmlDefaultMaxElementDepth   = 512
	htmlDefaultMaxTableRows      = 10000
	htmlDefaultMaxGeneratedPages = 1000
	htmlMaxCSSBytes              = 1 * 1024 * 1024
	htmlMaxCSSRules              = 2048
	htmlMaxCSSSelectors          = 4096
	htmlMaxTokenCount            = 100000
)

// HTMLNew returns an instance that writes HTML into the current PDF document.
func (f *Document) HTMLNew() (html HTML) {
	html.pdf = f
	html.Link.ClrR, html.Link.ClrG, html.Link.ClrB = 0, 0, 128
	html.Link.Bold, html.Link.Italic, html.Link.Underscore = false, false, true
	html.MaxDataImageBytes = htmlDefaultMaxDataImageBytes
	html.MaxHTMLBytes = htmlDefaultMaxHTMLBytes
	html.MaxElementDepth = htmlDefaultMaxElementDepth
	html.MaxTableRows = htmlDefaultMaxTableRows
	html.MaxGeneratedPages = htmlDefaultMaxGeneratedPages
	if f != nil {
		if f.limits.MaxHTMLBytes > 0 {
			html.MaxHTMLBytes = f.limits.MaxHTMLBytes
		}
		if f.limits.MaxHTMLGeneratedPages > 0 {
			html.MaxGeneratedPages = f.limits.MaxHTMLGeneratedPages
		}
	}
	return
}

// Write prints text from the current position using the currently selected
// font. See HTMLNew to create a receiver associated with the PDF document
// instance. The text can use common HTML text tags and inline style
// declarations. When the right margin is reached, a line break occurs and text
// continues from the left margin. Upon method exit, the current position is
// left at the end of the text.
//
// lineHt indicates the line height in the unit of measure specified in New.
func (html *HTML) Write(lineHt float64, htmlStr string) {
	_ = html.WriteContext(context.Background(), lineHt, htmlStr)
}

// WriteContext gets or compiles an HTML fragment render plan, renders it, and
// checks ctx before compile and render. Deeper cancellation within long
// table/image rendering remains best effort until those internals accept
// context directly.
func (html *HTML) WriteContext(ctx context.Context, lineHt float64, htmlStr string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := outputCanceledError(ctx); err != nil {
		html.pdf.SetError(err)
		return err
	}
	if len(htmlStr) > html.maxHTMLBytes() {
		html.pdf.SetError(ErrHTMLLimitExceeded)
		return html.pdf.Error()
	}
	useSharedCache := html.pdf != nil && html.pdf.resourceCachePolicy == ResourceCacheShared
	compiled, err := compileHTMLForWriteContext(ctx, htmlStr, html.maxDataImageBytes(), useSharedCache)
	if err != nil {
		html.pdf.SetError(err)
		return err
	}
	if err := outputCanceledError(ctx); err != nil {
		html.pdf.SetError(err)
		return err
	}
	html.WriteCompiled(lineHt, compiled)
	return html.pdf.Error()
}

// WriteCompiled renders a precompiled HTML fragment. Use CompileHTML when the
// same HTML is rendered repeatedly across documents.
func (html *HTML) WriteCompiled(lineHt float64, compiled *CompiledHTML) {
	if compiled != nil && compiled.sourceBytes > html.maxHTMLBytes() {
		html.pdf.SetError(ErrHTMLLimitExceeded)
		return
	}
	if err := compiled.validate(); err != nil {
		html.pdf.SetError(err)
		return
	}
	if compiled.maxDepth > html.maxElementDepth() {
		html.pdf.SetErrorf("HTML element depth exceeds maximum size")
		return
	}
	if err := compiled.validateDataImageLimit(html.maxDataImageBytes()); err != nil {
		html.pdf.SetError(err)
		return
	}
	previousStartPageCount := html.renderStartPageCount
	previousRenderCacheActive := html.renderCacheActive
	previousDataImageCache := html.dataImageCache
	previousStyleDeclarationCache := html.styleDeclarationCache
	previousCompiledStyleCache := html.compiledStyleCache
	previousInlineSVGCache := html.inlineSVGCache
	previousPageAddGuard := html.pdf.pageAddGuard
	html.renderStartPageCount = html.pdf.PageCount()
	html.pdf.pageAddGuard = func() error {
		if previousPageAddGuard != nil {
			if err := previousPageAddGuard(); err != nil {
				return err
			}
		}
		return html.checkGeneratedPageLimitForAdd()
	}
	html.renderCacheActive = true
	html.dataImageCache = make(map[string]htmlImageSource)
	html.styleDeclarationCache = nil
	html.compiledStyleCache = compiled.styleDeclarations
	html.inlineSVGCache = nil
	defer func() {
		pageCount := html.generatedPageCount()
		maxPages := html.maxGeneratedPages()
		if pageCount > maxPages {
			html.pdf.SetErrorf("HTML rendering exceeded maximum generated pages: %d > %d", pageCount, maxPages)
		}
		html.renderStartPageCount = previousStartPageCount
		html.renderCacheActive = previousRenderCacheActive
		html.dataImageCache = previousDataImageCache
		html.styleDeclarationCache = previousStyleDeclarationCache
		html.compiledStyleCache = previousCompiledStyleCache
		html.inlineSVGCache = previousInlineSVGCache
		html.pdf.pageAddGuard = previousPageAddGuard
	}()
	newHTMLRenderSession(html, compiled, lineHt).render()
}

func (html *HTML) applyTextStyle(st htmlTextStyle, fallback CSSColorType) {
	styleStr := htmlTextStyleFontStyle(st)
	fontStyle := htmlTextStyleBaseFontStyle(st)
	fontFamilyChanged := st.fontFamily != "" && !strings.EqualFold(fontFamilyEscape(st.fontFamily), html.pdf.fontFamily)
	fontSizeChanged := st.fontSize != 0 && html.pdf.fontSizePt != st.fontSize
	if html.pdf.currentFont.Name == "" || fontFamilyChanged || html.pdf.fontStyle != fontStyle || html.pdf.underline != st.underline || html.pdf.strikeout != st.strike || fontSizeChanged {
		html.pdf.SetFont(st.fontFamily, styleStr, st.fontSize)
	}
	color := fallback
	if st.color.Set {
		color = st.color
	}
	if html.pdf.color.text.mode != colorModeRGB || html.pdf.color.text.ir != color.R || html.pdf.color.text.ig != color.G || html.pdf.color.text.ib != color.B {
		html.pdf.SetTextColor(color.R, color.G, color.B)
	}
}

func htmlTextStyleMask(st htmlTextStyle) int {
	mask := 0
	if st.bold {
		mask |= 1
	}
	if st.italic {
		mask |= 2
	}
	if st.underline {
		mask |= 4
	}
	if st.strike {
		mask |= 8
	}
	return mask
}

func htmlTextStyleFontStyle(st htmlTextStyle) string {
	return [...]string{"", "B", "I", "BI", "U", "BU", "IU", "BIU", "S", "BS", "IS", "BIS", "US", "BUS", "IUS", "BIUS"}[htmlTextStyleMask(st)]
}

func htmlTextStyleBaseFontStyle(st htmlTextStyle) string {
	return [...]string{"", "B", "I", "BI"}[htmlTextStyleMask(st)&3]
}

func htmlListStateFromElement(st htmlTextStyle, attrs map[string]string, lineHt float64) htmlListState {
	state := htmlListState{kind: st.list, styleType: st.listStyleType, counter: htmlListStart(attrs) - 1, indent: lineHt * 1.5}
	if state.styleType == "" {
		state.styleType = htmlListTypeAttr(attrs, state.kind)
	}
	if state.styleType == "" {
		if state.kind == "ol" {
			state.styleType = "decimal"
		} else {
			state.styleType = "disc"
		}
	}
	return state
}

func htmlListStart(attrs map[string]string) int {
	start, err := strconv.Atoi(strings.TrimSpace(attrs["start"]))
	if err != nil || start < 1 {
		return 1
	}
	return start
}

func htmlListTypeAttr(attrs map[string]string, kind string) string {
	raw := strings.TrimSpace(attrs["type"])
	value := strings.ToLower(raw)
	switch raw {
	case "1":
		return "decimal"
	case "a":
		return "lower-alpha"
	case "A":
		return "upper-alpha"
	case "i":
		return "lower-roman"
	case "I":
		return "upper-roman"
	}
	switch value {
	case "disc", "circle", "square":
		if kind == "ul" {
			return value
		}
	}
	return ""
}

func (state htmlListState) marker() string {
	if state.styleType == "none" {
		return ""
	}
	if state.kind != "ol" {
		switch state.styleType {
		case "circle":
			return "o "
		case "square":
			return "* "
		default:
			return "- "
		}
	}
	switch state.styleType {
	case "lower-alpha":
		return strings.ToLower(htmlAlphaCounter(state.counter)) + ". "
	case "upper-alpha":
		return htmlAlphaCounter(state.counter) + ". "
	case "lower-roman":
		return strings.ToLower(htmlRomanCounter(state.counter)) + ". "
	case "upper-roman":
		return htmlRomanCounter(state.counter) + ". "
	default:
		return strconv.Itoa(state.counter) + ". "
	}
}

func htmlAlphaCounter(n int) string {
	if n <= 0 {
		return strconv.Itoa(n)
	}
	var chars []byte
	for n > 0 {
		n--
		chars = append([]byte{byte('A' + n%26)}, chars...)
		n /= 26
	}
	return string(chars)
}

func htmlRomanCounter(n int) string {
	if n <= 0 || n > 3999 {
		return strconv.Itoa(n)
	}
	values := []struct {
		value int
		text  string
	}{{1000, "M"}, {900, "CM"}, {500, "D"}, {400, "CD"}, {100, "C"}, {90, "XC"}, {50, "L"}, {40, "XL"}, {10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}}
	var out strings.Builder
	for _, item := range values {
		for n >= item.value {
			out.WriteString(item.text)
			n -= item.value
		}
	}
	return out.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func appendHTMLAncestors(ancestors []HTMLSegmentType, elements ...HTMLSegmentType) []HTMLSegmentType {
	out := make([]HTMLSegmentType, 0, len(ancestors)+len(elements))
	out = append(out, ancestors...)
	out = append(out, elements...)
	return out
}

func htmlEffectiveLineHeight(st htmlTextStyle, fallback float64) float64 {
	if st.lineHeight > 0 {
		return st.lineHeight
	}
	return fallback
}

func htmlStyleValue(attrs map[string]string, name string) string {
	if attrs == nil {
		return ""
	}
	return parseStyleDeclarations(attrs["style"])[strings.ToLower(name)]
}

func htmlHasBoxEdgeDeclaration(decl map[string]string, name string) bool {
	if strings.TrimSpace(decl[name]) != "" {
		return true
	}
	for _, side := range []string{"top", "right", "bottom", "left"} {
		if strings.TrimSpace(decl[name+"-"+side]) != "" {
			return true
		}
	}
	return false
}

func htmlHasBreakDeclaration(decl map[string]string) bool {
	for _, name := range []string{"break-before", "page-break-before", "break-after", "page-break-after", "break-inside", "page-break-inside"} {
		if strings.TrimSpace(decl[name]) != "" {
			return true
		}
	}
	return false
}

func htmlBreakForcesPage(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always", "page", "left", "right":
		return true
	default:
		return false
	}
}

func htmlBreakAvoidsInside(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "avoid", "avoid-page":
		return true
	default:
		return false
	}
}

func htmlBoxEdgesFromDeclarations(decl map[string]string, name string, pdf *Document, relative float64) htmlBoxEdges {
	var edges htmlBoxEdges
	if values := strings.Fields(decl[name]); len(values) > 0 && len(values) <= 4 {
		if parsed, ok := parseHTMLBoxEdgeValues(values, pdf, relative); ok {
			edges = parsed
		}
	}
	for _, side := range []struct {
		name string
		set  func(float64)
	}{{"top", func(v float64) {
		edges.top = v
	}}, {"right", func(v float64) {
		edges.right = v
	}}, {"bottom", func(v float64) {
		edges.bottom = v
	}}, {"left", func(v float64) {
		edges.left = v
	}}} {
		if value, ok := parseHTMLBoxLength(decl[name+"-"+side.name], pdf, relative); ok {
			side.set(value)
		}
	}
	return edges
}

func parseHTMLBoxEdgeValues(values []string, pdf *Document, relative float64) (htmlBoxEdges, bool) {
	parsed := make([]float64, len(values))
	for i, value := range values {
		n, ok := parseHTMLBoxLength(value, pdf, relative)
		if !ok {
			return htmlBoxEdges{}, false
		}
		parsed[i] = n
	}
	switch len(parsed) {
	case 1:
		return htmlBoxEdges{top: parsed[0], right: parsed[0], bottom: parsed[0], left: parsed[0]}, true
	case 2:
		return htmlBoxEdges{top: parsed[0], right: parsed[1], bottom: parsed[0], left: parsed[1]}, true
	case 3:
		return htmlBoxEdges{top: parsed[0], right: parsed[1], bottom: parsed[2], left: parsed[1]}, true
	case 4:
		return htmlBoxEdges{top: parsed[0], right: parsed[1], bottom: parsed[2], left: parsed[3]}, true
	default:
		return htmlBoxEdges{}, false
	}
}

func parseHTMLBoxLength(value string, pdf *Document, relative float64) (float64, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" || value == "auto" {
		return 0, false
	}
	if strings.HasSuffix(value, "%") {
		n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
		if err != nil || !isFiniteFloat(n) || n < 0 {
			return 0, false
		}
		result := relative * n / 100
		return result, isFiniteFloat(result)
	}
	unit := "px"
	for _, suffix := range []string{"px", "pt", "mm", "cm", "in"} {
		if strings.HasSuffix(value, suffix) {
			unit = suffix
			value = strings.TrimSpace(strings.TrimSuffix(value, suffix))
			break
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || !isFiniteFloat(n) || n < 0 {
		return 0, false
	}
	switch unit {
	case "pt":
		return n / pdf.k, true
	case "mm":
		return n * 72 / 25.4 / pdf.k, true
	case "cm":
		return n * 72 / 2.54 / pdf.k, true
	case "in":
		return n * 72 / pdf.k, true
	default:
		return n * 72 / 96 / pdf.k, true
	}
}

func isFiniteFloat(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0)
}

func (html *HTML) htmlImageSource(src string) (string, ImageOptions, error) {
	options := ImageOptions{ReadDpi: true}
	if !strings.HasPrefix(strings.ToLower(src), "data:") {
		if err := validateHTMLImageSource(src); err != nil {
			return "", options, err
		}
		if !html.AllowLocalImages {
			return "", options, errors.New("local HTML images are disabled")
		}
		if html.pdf != nil {
			if err := html.pdf.requireSecurityFeature("local HTML images", html.pdf.securityPolicy.AllowLocalHTMLImages); err != nil {
				return "", options, err
			}
		}
		return src, options, nil
	}
	if cached, ok := html.dataImageCache[src]; ok {
		return cached.name, cached.options, nil
	}
	img, ok, err := compileHTMLDataImageSource(src, html.maxDataImageBytes())
	if err != nil {
		return "", options, err
	}
	if !ok {
		return "", options, errors.New("invalid HTML image data URI")
	}
	name, options, err := img.register(html.pdf)
	if err != nil {
		return "", options, err
	}
	if html.dataImageCache != nil {
		html.dataImageCache[src] = htmlImageSource{name: name, options: options}
	}
	return name, options, nil
}

func (html *HTML) compiledHTMLImageSource(compiled *CompiledHTML, tokenIndex int, src string) (string, ImageOptions, error) {
	if img, ok := compiled.dataImage(tokenIndex); ok {
		return img.register(html.pdf)
	}
	return html.htmlImageSource(src)
}
