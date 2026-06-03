// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	stdhtml "html"
	"regexp"
	"sort"
	"strings"
)

var htmlTemplatePlaceholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// HTMLTemplateValues stores values used by RenderHTMLTemplate. Plain values are
// HTML-escaped before insertion. Use HTMLTemplateRaw only for trusted HTML.
type HTMLTemplateValues map[string]any

// HTMLTemplateRaw inserts trusted HTML without escaping.
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
		return "", fmt.Errorf("image source is required")
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
