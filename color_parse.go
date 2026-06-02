/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"strconv"
	"strings"
)

// CSSColorType describes a parsed CSS color or paint value.
type CSSColorType struct {
	R, G, B int
	Set     bool
	None    bool
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
		r, okR := parseHexByte(strings.Repeat(hex[0:1], 2))
		g, okG := parseHexByte(strings.Repeat(hex[1:2], 2))
		b, okB := parseHexByte(strings.Repeat(hex[2:3], 2))
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

func parseHexByte(value string) (int, bool) {
	n, err := strconv.ParseInt(value, 16, 16)
	return int(n), err == nil
}

func parseRGBColor(value string) (CSSColorType, bool) {
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return CSSColorType{}, false
	}
	var rgb [3]int
	for j, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasSuffix(part, "%") {
			n, err := strconv.ParseFloat(strings.TrimSuffix(part, "%"), 64)
			if err != nil {
				return CSSColorType{}, false
			}
			rgb[j] = clampColor(int(n * 255 / 100))
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return CSSColorType{}, false
		}
		rgb[j] = clampColor(n)
	}
	return CSSColorType{R: rgb[0], G: rgb[1], B: rgb[2], Set: true}, true
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
	for _, declaration := range strings.Split(style, ";") {
		name, value, ok := strings.Cut(declaration, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(strings.ToLower(name))
		value = strings.TrimSpace(value)
		if name != "" && value != "" {
			declarations[name] = value
		}
	}
	return declarations
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
