// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
	stdhtml "html"
	"reflect"
	"regexp"
	"sort"
	"strings"
)

var htmlTemplatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

const (
	htmlTemplateSlotPrefix = "\x1fgopdfkit_html_template_slot_"
	htmlTemplateSlotSuffix = "\x1f"
)

// HTMLTemplateValues stores values used by RenderHTMLTemplate and
// CompiledHTMLTemplate rendering. RenderHTMLTemplate HTML-escapes plain values
// before insertion. CompiledHTMLTemplate inserts plain values directly into
// precompiled text or attribute slots without reparsing them as HTML.
type HTMLTemplateValues map[string]any

// HTMLTemplateRaw inserts trusted HTML without escaping in RenderHTMLTemplate.
// It is not supported by CompiledHTMLTemplate because raw HTML can change the
// precompiled document structure.
type HTMLTemplateRaw string

// HTMLTemplateImage renders a template value as an HTML img tag.
//
// Source is the img src value. It may be a data URL, a registered image name,
// or a local path when HTML.AllowLocalImages is enabled before rendering.
// Width, Height, MaxWidth, MaxHeight, ObjectFit, Align, Class, and Style map to
// normal HTML attributes/CSS supported by HTMLNew.
type HTMLTemplateImage struct {
	Source     string
	Alt        string
	Width      string
	Height     string
	MaxWidth   string
	MaxHeight  string
	ObjectFit  string
	Align      string
	Class      string
	Style      string
	Attributes map[string]string
}

// CompiledHTMLTemplate stores a compiled HTML template with {{key}} slots that
// can be filled at render time without reparsing the HTML/CSS structure.
//
// Template values may replace text and non-structural attribute values. They
// cannot replace tag names, CSS rules, class/style/id attributes, SVG content,
// or raw HTML structure. Typical attribute slots include href, src, alt, width,
// and height. Use RenderHTMLTemplate for templates that need trusted raw HTML
// insertion or structural changes.
type CompiledHTMLTemplate struct {
	compiled *CompiledHTML
	slots    []compiledHTMLTemplateSlot
}

type compiledHTMLTemplateSlot struct {
	key         string
	marker      string
	token       int
	attr        string
	placeholder string
}

type htmlTemplatePlaceholder struct {
	key         string
	marker      string
	placeholder string
	found       bool
}

// RenderHTMLTemplate replaces {{key}} placeholders in templateHTML with values.
//
// Placeholder keys are looked up literally, with an additional fallback that
// treats {{.key}} as {{key}}. Missing keys return an error so generated PDFs do
// not silently contain unresolved placeholders.
func RenderHTMLTemplate(templateHTML string, values HTMLTemplateValues) (string, error) {
	var err error
	out := htmlTemplatePlaceholderPattern.ReplaceAllStringFunc(templateHTML, func(match string) string {
		if err != nil {
			return ""
		}
		parts := htmlTemplatePlaceholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			err = fmt.Errorf("invalid HTML template placeholder: %s", match)
			return ""
		}
		key := parts[1]
		value, ok := values[key]
		if !ok && strings.HasPrefix(key, ".") {
			value, ok = values[strings.TrimPrefix(key, ".")]
		}
		if !ok {
			err = fmt.Errorf("missing HTML template value: %s", key)
			return ""
		}
		rendered, renderErr := renderHTMLTemplateValue(value)
		if renderErr != nil {
			err = fmt.Errorf("render HTML template value %s: %w", key, renderErr)
			return ""
		}
		return rendered
	})
	if err != nil {
		return "", err
	}
	return out, nil
}

// CompileHTMLTemplate compiles a supported HTML template once while preserving
// {{key}} placeholders as render-time text or attribute slots.
//
// Use this when the same layout is rendered many times with changing values.
// The compiled template keeps the HTML/CSS structure, selector matching, table
// parsing, inline SVG parsing, and static data-image parsing reusable. At
// render time, WriteTemplate clones the compiled plan, fills the slots, and
// renders it without reparsing the original HTML/CSS.
//
// Placeholders are accepted in text nodes and non-structural attributes. They
// are rejected in tag names, CSS rules, SVG content, style/script content,
// class/style/id attributes, and event handler attributes.
func CompileHTMLTemplate(templateHTML string) (*CompiledHTMLTemplate, error) {
	return CompileHTMLTemplateContext(context.Background(), templateHTML)
}

// CompileHTMLTemplateContext compiles a supported HTML template once while
// checking ctx during HTML compilation. See CompileHTMLTemplate for slot rules.
func CompileHTMLTemplateContext(ctx context.Context, templateHTML string) (*CompiledHTMLTemplate, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(templateHTML) > htmlDefaultMaxHTMLBytes {
		return nil, ErrHTMLLimitExceeded
	}
	if strings.Contains(templateHTML, htmlTemplateSlotPrefix) {
		return nil, errors.New("HTML template contains a reserved slot marker")
	}
	markedHTML, placeholders, err := markHTMLTemplatePlaceholders(templateHTML)
	if err != nil {
		return nil, err
	}
	if len(markedHTML) > htmlDefaultMaxHTMLBytes {
		return nil, ErrHTMLLimitExceeded
	}
	compiled, err := compileHTMLWithDataImageLimitContext(ctx, markedHTML, true, htmlDefaultMaxDataImageBytes)
	if err != nil {
		return nil, err
	}
	compiled.sourceBytes = len(templateHTML)
	slots, err := compiledHTMLTemplateSlots(compiled.tokens, placeholders)
	if err != nil {
		return nil, err
	}
	return &CompiledHTMLTemplate{compiled: compiled, slots: slots}, nil
}

// WriteTemplate fills a compiled HTML template with values and renders it.
//
// Plain values are inserted as literal text or attribute values. They are not
// reparsed as HTML, so HTMLTemplateRaw and HTMLTemplateImage are intentionally
// unsupported. For image sources, place the slot inside a static img tag, for
// example <img src="{{logo}}" alt="{{alt}}">.
func (html *HTML) WriteTemplate(lineHt float64, template *CompiledHTMLTemplate, values HTMLTemplateValues) {
	_ = html.WriteTemplateContext(context.Background(), lineHt, template, values)
}

// WriteTemplateContext fills a compiled HTML template with values, renders it,
// and checks ctx before render. See WriteTemplate for value handling.
func (html *HTML) WriteTemplateContext(ctx context.Context, lineHt float64, template *CompiledHTMLTemplate, values HTMLTemplateValues) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := outputCanceledError(ctx); err != nil {
		html.pdf.SetError(err)
		return err
	}
	compiled, err := template.render(values, html.maxHTMLBytes())
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

func markHTMLTemplatePlaceholders(templateHTML string) (string, []htmlTemplatePlaceholder, error) {
	var placeholders []htmlTemplatePlaceholder
	marked := htmlTemplatePlaceholderPattern.ReplaceAllStringFunc(templateHTML, func(match string) string {
		parts := htmlTemplatePlaceholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		marker := fmt.Sprintf("%s%d%s", htmlTemplateSlotPrefix, len(placeholders), htmlTemplateSlotSuffix)
		placeholders = append(placeholders, htmlTemplatePlaceholder{key: parts[1], marker: marker, placeholder: match})
		return marker
	})
	return marked, placeholders, nil
}

func compiledHTMLTemplateSlots(tokens []HTMLSegmentType, placeholders []htmlTemplatePlaceholder) ([]compiledHTMLTemplateSlot, error) {
	if len(placeholders) == 0 {
		return nil, nil
	}
	slots := make([]compiledHTMLTemplateSlot, 0, len(placeholders))
	stack := make([]string, 0, 8)
	markFound := func(marker string) {
		for i := range placeholders {
			if placeholders[i].marker == marker {
				placeholders[i].found = true
				return
			}
		}
	}
	for i, token := range tokens {
		switch token.Cat {
		case 'O':
			for _, placeholder := range placeholders {
				if strings.Contains(token.Str, placeholder.marker) {
					return nil, fmt.Errorf("HTML template placeholder %s cannot replace a tag name", placeholder.placeholder)
				}
			}
			for attr, value := range token.Attr {
				for _, placeholder := range placeholders {
					if !strings.Contains(value, placeholder.marker) {
						continue
					}
					if err := validateCompiledHTMLTemplateAttrSlot(token.Str, attr, placeholder.placeholder, stack); err != nil {
						return nil, err
					}
					markFound(placeholder.marker)
					slots = append(slots, compiledHTMLTemplateSlot{
						key:         placeholder.key,
						marker:      placeholder.marker,
						token:       i,
						attr:        attr,
						placeholder: placeholder.placeholder,
					})
				}
			}
			if htmlClosePops(token.Str) {
				stack = append(stack, token.Str)
			}
		case 'T':
			for _, placeholder := range placeholders {
				if !strings.Contains(token.Str, placeholder.marker) {
					continue
				}
				if contextTag := compiledHTMLTemplateDisallowedContext(stack); contextTag != "" {
					return nil, fmt.Errorf("HTML template placeholder %s cannot be used inside <%s>", placeholder.placeholder, contextTag)
				}
				markFound(placeholder.marker)
				slots = append(slots, compiledHTMLTemplateSlot{
					key:         placeholder.key,
					marker:      placeholder.marker,
					token:       i,
					placeholder: placeholder.placeholder,
				})
			}
		case 'C':
			if !htmlClosePops(token.Str) {
				continue
			}
			for len(stack) > 0 {
				last := len(stack) - 1
				open := stack[last]
				stack = stack[:last]
				if open == token.Str {
					break
				}
			}
		}
	}
	for _, placeholder := range placeholders {
		if !placeholder.found {
			return nil, fmt.Errorf("HTML template placeholder %s is not in supported text or attribute content", placeholder.placeholder)
		}
	}
	return slots, nil
}

func validateCompiledHTMLTemplateAttrSlot(tag, attr, placeholder string, stack []string) error {
	if contextTag := compiledHTMLTemplateDisallowedContext(stack); contextTag != "" {
		return fmt.Errorf("HTML template placeholder %s cannot be used inside <%s>", placeholder, contextTag)
	}
	switch tag {
	case "style", "script", "svg":
		return fmt.Errorf("HTML template placeholder %s cannot be used on <%s>", placeholder, tag)
	}
	switch attr {
	case "class", "id", "style":
		return fmt.Errorf("HTML template placeholder %s cannot replace %s attributes in compiled templates", placeholder, attr)
	default:
		if strings.HasPrefix(attr, "on") {
			return fmt.Errorf("HTML template placeholder %s cannot replace event handler attributes", placeholder)
		}
	}
	return nil
}

func compiledHTMLTemplateDisallowedContext(stack []string) string {
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case "style", "script", "svg":
			return stack[i]
		}
	}
	return ""
}

func (template *CompiledHTMLTemplate) render(values HTMLTemplateValues, maxBytes int) (*CompiledHTML, error) {
	if template == nil || template.compiled == nil {
		return nil, errors.New("compiled HTML template is nil")
	}
	if len(template.slots) == 0 {
		if template.compiled.sourceBytes > maxBytes {
			return nil, ErrHTMLLimitExceeded
		}
		return template.cloneCompiled(), nil
	}
	replacements := make(map[string]string, len(template.slots))
	for _, slot := range template.slots {
		if _, ok := replacements[slot.marker]; ok {
			continue
		}
		value, ok := htmlTemplateValue(values, slot.key)
		if !ok {
			return nil, fmt.Errorf("missing HTML template value: %s", slot.key)
		}
		renderedValue, err := renderCompiledHTMLTemplateValue(value)
		if err != nil {
			return nil, fmt.Errorf("render HTML template value %s: %w", slot.key, err)
		}
		if strings.Contains(renderedValue, htmlTemplateSlotPrefix) {
			return nil, fmt.Errorf("render HTML template value %s contains a reserved slot marker", slot.key)
		}
		replacements[slot.marker] = renderedValue
	}
	renderedBytes, err := template.renderedSize(replacements, maxBytes)
	if err != nil {
		return nil, err
	}
	rendered := template.cloneCompiled()
	for _, slot := range template.slots {
		value := replacements[slot.marker]
		if slot.token < 0 || slot.token >= len(rendered.tokens) {
			continue
		}
		if slot.attr == "" {
			rendered.tokens[slot.token].Str = strings.ReplaceAll(rendered.tokens[slot.token].Str, slot.marker, value)
			continue
		}
		attrs := rendered.tokens[slot.token].Attr
		if attrs == nil {
			continue
		}
		attrs[slot.attr] = strings.ReplaceAll(attrs[slot.attr], slot.marker, value)
	}
	rendered.sourceBytes = renderedBytes
	rendered.elementText = nil
	rendered.tables = compiledHTMLTemplateTables(rendered.tokens)
	return rendered, nil
}

func (template *CompiledHTMLTemplate) renderedSize(replacements map[string]string, maxBytes int) (int, error) {
	if maxBytes <= 0 {
		maxBytes = htmlDefaultMaxHTMLBytes
	}
	size := template.compiled.sourceBytes
	if size > maxBytes {
		return 0, ErrHTMLLimitExceeded
	}
	for _, slot := range template.slots {
		value, ok := replacements[slot.marker]
		if !ok || slot.token < 0 || slot.token >= len(template.compiled.tokens) {
			continue
		}
		token := template.compiled.tokens[slot.token]
		target := token.Str
		if slot.attr != "" {
			target = token.Attr[slot.attr]
		}
		count := strings.Count(target, slot.marker)
		if count == 0 {
			continue
		}
		delta := len(value) - len(slot.placeholder)
		if delta > 0 {
			if count > (maxBytes-size)/delta {
				return 0, ErrHTMLLimitExceeded
			}
			size += count * delta
		} else {
			size += count * delta
		}
	}
	return size, nil
}

func (template *CompiledHTMLTemplate) cloneCompiled() *CompiledHTML {
	base := template.compiled
	rendered := *base
	rendered.tokens = cloneHTMLTokens(base.tokens)
	rendered.dataImages = make(map[int]compiledHTMLDataImage, len(base.dataImages))
	for key, value := range base.dataImages {
		rendered.dataImages[key] = value
	}
	return &rendered
}

func compiledHTMLTemplateTables(tokens []HTMLSegmentType) map[int]compiledHTMLTable {
	tables := make(map[int]compiledHTMLTable)
	for i, token := range tokens {
		if token.Cat != 'O' || token.Str != "table" {
			continue
		}
		table, end := parseHTMLTable(tokens, i)
		tables[i] = compiledHTMLTable{table: table, end: end, start: i}
	}
	return tables
}

func htmlTemplateValue(values HTMLTemplateValues, key string) (any, bool) {
	value, ok := values[key]
	if !ok && strings.HasPrefix(key, ".") {
		value, ok = values[strings.TrimPrefix(key, ".")]
	}
	return value, ok
}

func renderCompiledHTMLTemplateValue(value any) (string, error) {
	if value == nil {
		return "", nil
	}
	switch v := value.(type) {
	case HTMLTemplateRaw:
		return "", errors.New("HTMLTemplateRaw is not supported by compiled HTML templates")
	case HTMLTemplateImage:
		return "", errors.New("HTMLTemplateImage is not supported by compiled HTML templates; use a static <img> tag with a src placeholder")
	case *HTMLTemplateImage:
		return "", errors.New("HTMLTemplateImage is not supported by compiled HTML templates; use a static <img> tag with a src placeholder")
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case fmt.Stringer:
		value := reflect.ValueOf(v)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
			if value.IsNil() {
				return "", nil
			}
		}
		return v.String(), nil
	default:
		return fmt.Sprint(v), nil
	}
}

func renderHTMLTemplateValue(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case HTMLTemplateRaw:
		return string(v), nil
	case HTMLTemplateImage:
		return renderHTMLTemplateImage(v)
	case *HTMLTemplateImage:
		if v == nil {
			return "", nil
		}
		return renderHTMLTemplateImage(*v)
	case string:
		return stdhtml.EscapeString(v), nil
	case []byte:
		return stdhtml.EscapeString(string(v)), nil
	case fmt.Stringer:
		return stdhtml.EscapeString(v.String()), nil
	default:
		return stdhtml.EscapeString(fmt.Sprint(v)), nil
	}
}

func renderHTMLTemplateImage(image HTMLTemplateImage) (string, error) {
	if strings.TrimSpace(image.Source) == "" {
		return "", errors.New("image source is required")
	}
	attrs := map[string]string{}
	attrs["src"] = image.Source
	setIfNotEmpty(attrs, "alt", image.Alt)
	setIfNotEmpty(attrs, "width", image.Width)
	setIfNotEmpty(attrs, "height", image.Height)
	setIfNotEmpty(attrs, "max-width", image.MaxWidth)
	setIfNotEmpty(attrs, "max-height", image.MaxHeight)
	setIfNotEmpty(attrs, "align", image.Align)
	setIfNotEmpty(attrs, "class", image.Class)
	for key, value := range image.Attributes {
		if strings.TrimSpace(key) == "" {
			continue
		}
		attrs[strings.TrimSpace(key)] = value
	}
	style := strings.TrimSpace(image.Style)
	if strings.TrimSpace(image.ObjectFit) != "" && !strings.Contains(strings.ToLower(style), "object-fit") {
		if style != "" && !strings.HasSuffix(style, ";") {
			style += ";"
		}
		style += " object-fit: " + strings.TrimSpace(image.ObjectFit)
	}
	setIfNotEmpty(attrs, "style", style)

	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var out strings.Builder
	out.WriteString("<img")
	for _, key := range keys {
		out.WriteByte(' ')
		out.WriteString(stdhtml.EscapeString(key))
		out.WriteString(`="`)
		out.WriteString(stdhtml.EscapeString(attrs[key]))
		out.WriteByte('"')
	}
	out.WriteString(">")
	return out.String(), nil
}

func setIfNotEmpty(attrs map[string]string, key, value string) {
	if strings.TrimSpace(value) != "" {
		attrs[key] = value
	}
}
