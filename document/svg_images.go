// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// SVGImage describes one embedded raster image from an SVG image element.
type SVGImage struct {
	X         float64  // Image X coordinate.
	Y         float64  // Image Y coordinate.
	Wd        float64  // Image width.
	Ht        float64  // Image height.
	ImageType string   // Embedded image type, such as png or jpg.
	Data      []byte   // Encoded image bytes.
	Style     SVGStyle // Image style.
}

func svgImageTypeFromMime(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	case "image/gif":
		return "gif"
	default:
		return ""
	}
}

func svgImageDataURI(href string) ([]byte, string, bool, error) {
	href = strings.TrimSpace(href)
	if !strings.HasPrefix(strings.ToLower(href), "data:") {
		return nil, "", false, nil
	}
	media, data, ok := strings.Cut(href[5:], ",")
	if !ok {
		return nil, "", false, errors.New("invalid SVG image data URI")
	}
	parts := strings.Split(media, ";")
	imageType := svgImageTypeFromMime(parts[0])
	if imageType == "" {
		return nil, "", false, fmt.Errorf("unsupported SVG image type: %s", parts[0])
	}
	base64Encoded := false
	for _, part := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			base64Encoded = true
			break
		}
	}
	if !base64Encoded {
		return nil, "", false, errors.New("SVG image data URI must be base64 encoded")
	}
	data = strings.TrimSpace(data)
	if base64.StdEncoding.DecodedLen(len(data)) > maxImageSourceBytes {
		return nil, "", false, errors.New("SVG image data URI exceeds maximum size")
	}
	buf, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, "", false, fmt.Errorf("invalid SVG image data URI: %w", err)
	}
	if len(buf) > maxImageSourceBytes {
		return nil, "", false, errors.New("SVG image data URI exceeds maximum size")
	}
	return buf, imageType, true, nil
}

func svgImageBounds(x, y, wd, ht float64, transform svgMatrix) (float64, float64, float64, float64) {
	points := [4][2]float64{{x, y}, {x + wd, y}, {x, y + ht}, {x + wd, y + ht}}
	minX, minY := transform.apply(points[0][0], points[0][1])
	maxX, maxY := minX, minY
	for j := 1; j < len(points); j++ {
		px, py := transform.apply(points[j][0], points[j][1])
		if px < minX {
			minX = px
		}
		if px > maxX {
			maxX = px
		}
		if py < minY {
			minY = py
		}
		if py > maxY {
			maxY = py
		}
	}
	return minX, minY, maxX - minX, maxY - minY
}

func svgImage(node svgNode, style SVGStyle, transform svgMatrix) (SVGImage, bool, error) {
	if node.XMLName.Local != "image" {
		return SVGImage{}, false, nil
	}
	if style.Hidden {
		return SVGImage{}, false, nil
	}
	href := node.attr("href")
	data, imageType, ok, err := svgImageDataURI(href)
	if err != nil || !ok {
		return SVGImage{}, false, err
	}
	x, err := svgOptionalLength(node, "x", 0)
	if err != nil {
		return SVGImage{}, false, err
	}
	y, err := svgOptionalLength(node, "y", 0)
	if err != nil {
		return SVGImage{}, false, err
	}
	wd, err := svgRequiredPositiveLength(node, "width")
	if err != nil {
		return SVGImage{}, false, err
	}
	ht, err := svgRequiredPositiveLength(node, "height")
	if err != nil {
		return SVGImage{}, false, err
	}
	x, y, wd, ht = svgImageBounds(x, y, wd, ht, transform)
	style = svgRenderedStyle(style, transform)
	return SVGImage{X: x, Y: y, Wd: wd, Ht: ht, ImageType: imageType, Data: data, Style: style}, true, nil
}
