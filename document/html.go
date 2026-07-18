// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
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
	DebugLog             func(message string)
	renderStartPageCount int
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
	_ = html.writeContextEntry(context.Background(), lineHt, htmlStr, "HTML.Write")
}

// WriteContext gets or compiles an HTML fragment render plan, renders it, and
// checks ctx before compile and render. Deeper cancellation within long
// table/image rendering remains best effort until those internals accept
// context directly.
func (html *HTML) WriteContext(ctx context.Context, lineHt float64, htmlStr string) error {
	return html.writeContextEntry(ctx, lineHt, htmlStr, "HTML.WriteContext")
}

func (html *HTML) writeContextEntry(ctx context.Context, lineHt float64, htmlStr, entryPoint string) error {
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
	if err := html.ensureCurrentFont(); err != nil {
		return err
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
	html.writeCompiledContext(ctx, lineHt, compiled, entryPoint)
	return html.pdf.Error()
}

// WriteCompiled renders a precompiled HTML fragment. Use CompileHTML when the
// same HTML is rendered repeatedly across documents.
func (html *HTML) WriteCompiled(lineHt float64, compiled *CompiledHTML) {
	html.writeCompiledContext(context.Background(), lineHt, compiled, "HTML.WriteCompiled")
}

func (html *HTML) writeCompiledContext(ctx context.Context, lineHt float64, compiled *CompiledHTML, entryPoint string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := outputCanceledError(ctx); err != nil {
		html.pdf.SetError(err)
		return
	}
	if err := html.ensureCurrentFont(); err != nil {
		return
	}
	if compiled == nil {
		html.pdf.SetError(errors.New("compiled HTML is nil"))
		return
	}
	if compiled.sourceBytes > html.maxHTMLBytes() {
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
	if err := html.validateCompiledTableRowLimit(compiled); err != nil {
		html.pdf.SetError(err)
		return
	}
	if err := html.validateCompiledLinkSafety(compiled); err != nil {
		html.pdf.SetError(err)
		return
	}
	handled, routeErr := html.writeCompiledUnifiedFragmentContext(ctx, lineHt, compiled)
	if handled {
		if routeErr != nil {
			html.pdf.SetError(routeErr)
			return
		}
		html.pdf.observeLayoutEngineRoute(entryPoint, "unified", "")
		return
	} else if !errors.Is(routeErr, ErrHTMLPlanUnsupported) {
		html.pdf.SetError(routeErr)
		return
	}
	if err := outputCanceledError(ctx); err != nil {
		html.pdf.SetError(err)
		return
	}
	// Unsupported fragments fail atomically in the deletion release. There is
	// no automatic compatibility renderer or hidden pagination fallback.
	html.pdf.SetError(routeErr)
}

func (html *HTML) ensureCurrentFont() error {
	if html == nil || html.pdf == nil {
		return errors.New("document: HTML receiver is nil")
	}
	if html.pdf.currentFont.Name == "" {
		html.pdf.SetFont("Helvetica", "", 12)
	}
	return html.pdf.Error()
}

func (html *HTML) validateCompiledLinkSafety(compiled *CompiledHTML) error {
	if compiled == nil {
		return errors.New("compiled HTML is nil")
	}
	for _, token := range compiled.tokens {
		if token.Cat != 'O' || token.Str != "a" {
			continue
		}
		href := token.Attr["href"]
		if href == "" {
			href = token.Attr["xlink:href"]
		}
		if _, err := htmlLinkTarget(href); err != nil {
			return err
		}
	}
	return nil
}

func (html *HTML) validateCompiledTableRowLimit(compiled *CompiledHTML) error {
	if compiled == nil {
		return errors.New("compiled HTML is nil")
	}
	limit := html.maxTableRows()
	rows := make([]int, 0, 4)
	for _, token := range compiled.tokens {
		switch {
		case token.Cat == 'O' && token.Str == "table":
			rows = append(rows, 0)
		case token.Cat == 'O' && token.Str == "tr" && len(rows) != 0:
			rows[len(rows)-1]++
			if rows[len(rows)-1] > limit {
				return fmt.Errorf("%w: HTML table row count exceeds maximum size", ErrHTMLLimitExceeded)
			}
		case token.Cat == 'C' && token.Str == "table" && len(rows) != 0:
			rows = rows[:len(rows)-1]
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func htmlStyleValue(attrs map[string]string, name string) string {
	if attrs == nil {
		return ""
	}
	return parseStyleDeclarations(attrs["style"])[strings.ToLower(name)]
}

func htmlBreakForcesPage(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always", "page", "left", "right":
		return true
	default:
		return false
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
	unitScale := 1.0
	if pdf != nil {
		unitScale = pdf.k
		if !isFiniteFloat(unitScale) || unitScale <= 0 {
			return 0, false
		}
	}
	var result float64
	switch unit {
	case "pt":
		result = n / unitScale
	case "mm":
		result = n * 72 / 25.4 / unitScale
	case "cm":
		result = n * 72 / 2.54 / unitScale
	case "in":
		result = n * 72 / unitScale
	default:
		result = n * 72 / 96 / unitScale
	}
	return result, isFiniteFloat(result)
}

func isFiniteFloat(n float64) bool {
	return !math.IsNaN(n) && !math.IsInf(n, 0)
}
