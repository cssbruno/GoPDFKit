// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"strconv"
	"strings"
)

// CSSColorType describes a parsed CSS color or paint value.
type CSSColorType struct {
	R    int  // Red component, 0-255.
	G    int  // Green component, 0-255.
	B    int  // Blue component, 0-255.
	Set  bool // Whether a color value was explicitly parsed.
	None bool // Whether the parsed paint value is "none".
}

func parseCSSColor(value string) (CSSColorType, bool) {
	paint, ok := parseCSSPaint(value)
	if !ok || paint.None {
		return CSSColorType{}, false
	}
	return paint, true
}

func parseCSSPaint(value string) (CSSColorType, bool) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return CSSColorType{}, false
	}
	if strings.HasPrefix(value, "url(") {
		if fallback := strings.TrimSpace(value[strings.LastIndex(value, ")")+1:]); fallback != "" && fallback != value {
			return parseCSSPaint(fallback)
		}
		return CSSColorType{}, false
	}
	if value == "none" || value == "transparent" {
		return CSSColorType{Set: true, None: true}, true
	}
	if strings.HasPrefix(value, "#") {
		return parseHexColor(value)
	}
	if strings.HasPrefix(value, "rgb(") && strings.HasSuffix(value, ")") {
		return parseRGBColor(value[4 : len(value)-1])
	}
	if rgb, ok := cssNamedColors[value]; ok {
		return CSSColorType{R: rgb[0], G: rgb[1], B: rgb[2], Set: true}, true
	}
	return CSSColorType{}, false
}

func parseHexColor(value string) (CSSColorType, bool) {
	hex := strings.TrimPrefix(value, "#")
	if len(hex) == 3 || len(hex) == 4 {
		r, okR := parseShortHexByte(hex[0])
		g, okG := parseShortHexByte(hex[1])
		b, okB := parseShortHexByte(hex[2])
		return CSSColorType{R: r, G: g, B: b, Set: true}, okR && okG && okB
	}
	if len(hex) == 6 || len(hex) == 8 {
		r, okR := parseHexByte(hex[0:2])
		g, okG := parseHexByte(hex[2:4])
		b, okB := parseHexByte(hex[4:6])
		return CSSColorType{R: r, G: g, B: b, Set: true}, okR && okG && okB
	}
	return CSSColorType{}, false
}

func parseShortHexByte(value byte) (int, bool) {
	n, ok := parseHexNibble(value)
	return n<<4 | n, ok
}

func parseHexNibble(value byte) (int, bool) {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0'), true
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10, true
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10, true
	default:
		return 0, false
	}
}

func parseHexByte(value string) (int, bool) {
	n, err := strconv.ParseInt(value, 16, 16)
	return int(n), err == nil
}

func parseRGBColor(value string) (CSSColorType, bool) {
	var rgb [3]int
	for j := 0; j < 3; j++ {
		part := value
		if comma := strings.IndexByte(value, ','); comma >= 0 {
			part = value[:comma]
			value = value[comma+1:]
		} else if j < 2 {
			return CSSColorType{}, false
		} else {
			value = ""
		}
		n, ok := parseRGBComponent(part)
		if !ok {
			return CSSColorType{}, false
		}
		rgb[j] = clampColor(n)
	}
	if strings.TrimSpace(value) != "" {
		return CSSColorType{}, false
	}
	return CSSColorType{R: rgb[0], G: rgb[1], B: rgb[2], Set: true}, true
}

func parseRGBComponent(part string) (int, bool) {
	part = strings.TrimSpace(part)
	if strings.HasSuffix(part, "%") {
		n, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(part, "%")), 64)
		if err != nil {
			return 0, false
		}
		return int(n * 255 / 100), true
	}
	n, err := strconv.Atoi(part)
	return n, err == nil
}

func clampColor(n int) int {
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

func parseStyleDeclarations(style string) map[string]string {
	if strings.TrimSpace(style) == "" || !strings.Contains(style, ":") {
		return nil
	}
	declarations := make(map[string]string, strings.Count(style, ":"))
	for len(style) > 0 {
		declaration := style
		if end := strings.IndexByte(style, ';'); end >= 0 {
			declaration = style[:end]
			style = style[end+1:]
		} else {
			style = ""
		}
		name, value, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(strings.ToLower(name))
		value = strings.TrimSpace(value)
		if name != "" && value != "" {
			if name == "background" {
				if color, ok := parseCSSColor(value); ok && color.Set && !color.None {
					declarations["background-color"] = value
					delete(declarations, name)
					continue
				}
			}
			if name == "font" {
				if expanded, ok := htmlExpandFontShorthand(value); ok {
					for expandedName, expandedValue := range expanded {
						declarations[expandedName] = expandedValue
					}
					delete(declarations, name)
					continue
				}
			}
			declarations[name] = value
		}
	}
	return declarations
}

// htmlExpandFontShorthand lowers the bounded, renderer-independent subset of
// CSS font shorthand into the longhands consumed by the unified text style.
// It intentionally rejects system fonts, variation settings, and keywords
// whose metrics cannot be reproduced by the core PDF font set.
func htmlExpandFontShorthand(value string) (map[string]string, bool) {
	fields := htmlCSSValueFields(strings.TrimSpace(value))
	if len(fields) < 2 {
		return nil, false
	}
	sizeIndex := -1
	for index, field := range fields {
		size := field
		if slash := strings.IndexByte(size, '/'); slash >= 0 {
			size = size[:slash]
		}
		lowerSize := strings.ToLower(strings.TrimSpace(size))
		isLength := strings.HasSuffix(lowerSize, "pt") || strings.HasSuffix(lowerSize, "px") || strings.HasSuffix(lowerSize, "em") || strings.HasSuffix(lowerSize, "%")
		if isLength {
			if _, ok := parseHTMLFontSize(size, 12); ok {
				sizeIndex = index
				break
			}
		}
	}
	if sizeIndex < 0 || sizeIndex >= len(fields)-1 {
		return nil, false
	}
	fontStyle, fontWeight := "normal", "normal"
	styleSeen, weightSeen := false, false
	for _, field := range fields[:sizeIndex] {
		switch strings.ToLower(field) {
		case "normal":
		case "italic", "oblique":
			if styleSeen {
				return nil, false
			}
			styleSeen = true
			fontStyle = strings.ToLower(field)
		case "bold", "bolder", "lighter", "100", "200", "300", "400", "500", "600", "700", "800", "900":
			if weightSeen {
				return nil, false
			}
			weightSeen = true
			fontWeight = strings.ToLower(field)
		default:
			return nil, false
		}
	}
	size := fields[sizeIndex]
	lineHeight := "normal"
	if slash := strings.IndexByte(size, '/'); slash >= 0 {
		lineHeight = strings.TrimSpace(size[slash+1:])
		size = strings.TrimSpace(size[:slash])
	} else if sizeIndex+1 < len(fields) && fields[sizeIndex+1] == "/" {
		if sizeIndex+2 >= len(fields) {
			return nil, false
		}
		lineHeight = fields[sizeIndex+2]
		sizeIndex += 2
	}
	if lineHeight != "normal" {
		if _, ok := parseHTMLLineHeight(lineHeight, 12, nil); !ok {
			return nil, false
		}
	}
	family := strings.TrimSpace(strings.Join(fields[sizeIndex+1:], " "))
	if htmlFontFamily(family) == "" {
		return nil, false
	}
	return map[string]string{
		"font-style":  fontStyle,
		"font-weight": fontWeight,
		"font-size":   size,
		"line-height": lineHeight,
		"font-family": family,
	}, true
}

var cssNamedColors = map[string][3]int{
	"black":   {0, 0, 0},
	"silver":  {192, 192, 192},
	"gray":    {128, 128, 128},
	"white":   {255, 255, 255},
	"maroon":  {128, 0, 0},
	"red":     {255, 0, 0},
	"purple":  {128, 0, 128},
	"fuchsia": {255, 0, 255},
	"green":   {0, 128, 0},
	"lime":    {0, 255, 0},
	"olive":   {128, 128, 0},
	"yellow":  {255, 255, 0},
	"navy":    {0, 0, 128},
	"blue":    {0, 0, 255},
	"teal":    {0, 128, 128},
	"aqua":    {0, 255, 255},
	"orange":  {255, 165, 0},
}
