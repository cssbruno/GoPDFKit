// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"net/url"
	"strings"
)

func htmlLinkTarget(href string) (string, error) {
	href = strings.TrimSpace(href)
	if href == "" {
		return "", nil
	}
	if strings.HasPrefix(href, "#") {
		return "", fmt.Errorf("HTML fragment links are not supported: %s", href)
	}
	u, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("invalid HTML link target: %w", err)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return href, nil
	default:
		return "", fmt.Errorf("unsupported HTML link scheme: %s", u.Scheme)
	}
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

func (html *HTML) writeInlineSVG(tokens []HTMLSegmentType, start int, lineHt float64, st htmlTextStyle) int {
	svgTokens, end := htmlCollectElementTokens(tokens, start, "svg")
	if len(svgTokens) == 0 {
		return end
	}
	svgText := htmlSerializeTokens(svgTokens)
	svg, ok := html.inlineSVGCache[svgText]
	if !ok {
		parsed, err := SVGParse([]byte(svgText))
		if err != nil {
			html.pdf.SetError(err)
			return end
		}
		svg = &parsed
		if html.renderCacheActive {
			if html.inlineSVGCache == nil {
				html.inlineSVGCache = make(map[string]*SVG)
			}
			html.inlineSVGCache[svgText] = svg
		}
	}
	return html.writeSVGObject(svg, end, tokens[start].Attr, lineHt, st)
}

func (html *HTML) writeCompiledInlineSVG(compiled *CompiledHTML, tokens []HTMLSegmentType, start int, lineHt float64, st htmlTextStyle) int {
	svg, end, ok := compiled.inlineSVG(start)
	if !ok {
		return html.writeInlineSVG(tokens, start, lineHt, st)
	}
	return html.writeSVGObject(svg, end, tokens[start].Attr, lineHt, st)
}

func (html *HTML) writeSVGObject(svg *SVG, end int, attrs map[string]string, lineHt float64, st htmlTextStyle) int {
	if svg == nil || svg.Wd <= 0 || svg.Ht <= 0 {
		return end
	}
	pdf := html.pdf
	if pdf.GetX() != pdf.lMargin {
		pdf.Ln(lineHt)
	}
	availableWd := pdf.w - pdf.rMargin - pdf.GetX()
	pageHt := pdf.h - pdf.bMargin - pdf.GetY()
	targetWd, hasWd := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "width"), attrs["width"]), pdf, availableWd)
	targetHt, hasHt := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "height"), attrs["height"]), pdf, pageHt)
	if !hasWd || targetWd <= 0 {
		targetWd = svg.Wd * 72 / 96 / pdf.k
	}
	if !hasHt || targetHt <= 0 {
		targetHt = svg.Ht * 72 / 96 / pdf.k
	}
	scale := targetWd / svg.Wd
	if hasHt && targetHt/svg.Ht < scale {
		scale = targetHt / svg.Ht
	}
	actualWd := svg.Wd * scale
	actualHt := svg.Ht * scale
	x, y := pdf.GetXY()
	if pdf.y+actualHt > pdf.pageBreakTrigger && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
		if !html.addPageFormat() {
			return end
		}
		x, y = pdf.GetXY()
	}
	textRole := st.role
	linkStructure := st.href != "" && svgHasVisibleText(svg)
	if st.href != "" {
		textRole = taggedRoleLink
	}
	if linkStructure {
		pdf.BeginStructure(taggedRoleLink)
	}
	pdf.svgWriteWithOptions(svg, scale, svgWriteOptions{TextRole: normalizeTaggedRole(textRole)})
	if st.href != "" {
		pdf.LinkString(x, y, actualWd, actualHt, st.href)
	}
	if linkStructure {
		pdf.EndStructure()
	}
	pdf.SetXY(pdf.lMargin, y+actualHt)
	return end
}

func svgHasVisibleText(svg *SVG) bool {
	if svg == nil {
		return false
	}
	for _, text := range svg.Texts {
		if text.Text != "" && !text.Style.Hidden && !text.Style.Fill.None {
			return true
		}
	}
	for _, element := range svg.Elements {
		text := element.Text
		if element.Kind == "text" && text.Text != "" && !text.Style.Hidden && !text.Style.Fill.None {
			return true
		}
	}
	return false
}

func htmlResolvedImageSize(info *ImageInfo, pdf *Document, wd, ht float64) (float64, float64) {
	if wd == 0 && ht == 0 {
		wd = -96
		ht = -96
	}
	if wd == -1 {
		wd = -info.dpi
	}
	if ht == -1 {
		ht = -info.dpi
	}
	if wd < 0 {
		wd = -info.w * 72.0 / wd / pdf.k
	}
	if ht < 0 {
		ht = -info.h * 72.0 / ht / pdf.k
	}
	if wd == 0 {
		wd = ht * info.w / info.h
	}
	if ht == 0 {
		ht = wd * info.h / info.w
	}
	return wd, ht
}

func htmlImageObjectFit(attrs map[string]string) string {
	switch strings.ToLower(strings.TrimSpace(htmlStyleValue(attrs, "object-fit"))) {
	case "cover":
		return "cover"
	case "contain":
		return "contain"
	default:
		return ""
	}
}

func (html *HTML) imageObjectFit(attrs map[string]string) string {
	switch strings.ToLower(strings.TrimSpace(html.styleValue(attrs, "object-fit"))) {
	case "cover":
		return "cover"
	case "contain":
		return "contain"
	default:
		return ""
	}
}

func htmlImageFitBox(info *ImageInfo, pdf *Document, wd, ht, boxWd, boxHt float64, fit string) (drawX, drawY, drawWd, drawHt, flowWd, flowHt float64) {
	flowWd, flowHt = boxWd, boxHt
	if flowWd <= 0 || flowHt <= 0 {
		flowWd, flowHt = wd, ht
	}
	if fit == "" || flowWd <= 0 || flowHt <= 0 {
		return 0, 0, wd, ht, wd, ht
	}
	naturalWd, naturalHt := htmlResolvedImageSize(info, pdf, 0, 0)
	if naturalWd <= 0 || naturalHt <= 0 {
		return 0, 0, wd, ht, flowWd, flowHt
	}
	scaleX := flowWd / naturalWd
	scaleY := flowHt / naturalHt
	scale := minFloat(scaleX, scaleY)
	if fit == "cover" {
		scale = htmlMaxFloat(scaleX, scaleY)
	}
	drawWd = naturalWd * scale
	drawHt = naturalHt * scale
	drawX = (flowWd - drawWd) / 2
	drawY = (flowHt - drawHt) / 2
	return drawX, drawY, drawWd, drawHt, flowWd, flowHt
}

func (html *HTML) figureHeight(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType) float64 {
	figureTokens, _ := htmlCollectElementTokens(tokens, start, "figure")
	if len(figureTokens) == 0 {
		return 0
	}
	defer html.applyTextStyle(inherited, fallback)
	availableWd := html.pdf.w - html.pdf.rMargin - html.pdf.lMargin
	pageHt := html.pdf.h - html.pdf.bMargin - html.pdf.GetY()
	total := 0.0
	for i := 1; i < len(figureTokens)-1; i++ {
		token := figureTokens[i]
		if token.Cat != 'O' {
			continue
		}
		switch token.Str {
		case "img":
			wd, _ := parseHTMLBoxLength(firstNonEmpty(html.styleValue(token.Attr, "width"), token.Attr["width"]), html.pdf, availableWd)
			ht, _ := parseHTMLBoxLength(firstNonEmpty(html.styleValue(token.Attr, "height"), token.Attr["height"]), html.pdf, pageHt)
			if ht <= 0 {
				if wd > 0 {
					ht = wd
				} else {
					ht = lineHt * 3
				}
			}
			if maxHt, ok := parseHTMLBoxLength(html.styleValue(token.Attr, "max-height"), html.pdf, pageHt); ok && maxHt > 0 && ht > maxHt {
				ht = maxHt
			}
			total += ht
		case "figcaption":
			captionTokens, end := htmlCollectElementTokens(figureTokens, i, "figcaption")
			if len(captionTokens) < 2 {
				i = end
				continue
			}
			style := inherited
			style.italic = true
			if style.align == "" || style.align == "L" {
				style.align = "C"
			}
			if style.fontSize > 1 {
				style.fontSize *= 0.9
			}
			html.applyAttrs(&style, token.Attr, inherited.fontSize, inherited.lineHeight, html.pdf)
			html.applyTextStyle(style, fallback)
			text := htmlPlainText(captionTokens[1 : len(captionTokens)-1])
			if text != "" {
				total += html.tableCellTextHeight(text, availableWd, style, lineHt)
			}
			i = end
		}
	}
	if total > 0 {
		total += lineHt
	}
	return total
}

func (html *HTML) imageFlowHeight(attrs map[string]string, lineHt float64) float64 {
	if html == nil || html.pdf == nil {
		return 0
	}
	availableWd := html.pdf.w - html.pdf.rMargin - html.pdf.lMargin
	pageHt := html.pdf.h - html.pdf.bMargin - html.pdf.GetY()
	wd, _ := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "width"), attrs["width"]), html.pdf, availableWd)
	ht, _ := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "height"), attrs["height"]), html.pdf, pageHt)
	if ht <= 0 {
		if wd > 0 {
			ht = wd
		} else {
			ht = lineHt * 3
		}
	}
	if maxHt, ok := parseHTMLBoxLength(html.styleValue(attrs, "max-height"), html.pdf, pageHt); ok && maxHt > 0 && ht > maxHt {
		ht = maxHt
	}
	return ht
}

func htmlImageAlign(attrs map[string]string, fallback string) string {
	align := strings.ToLower(firstNonEmpty(htmlStyleValue(attrs, "text-align"), attrs["align"]))
	switch align {
	case "center", "middle":
		return "C"
	case "right":
		return "R"
	case "left":
		return "L"
	}
	if fallback == "C" || fallback == "R" {
		return fallback
	}
	return "L"
}

func (html *HTML) imageAlign(attrs map[string]string, fallback string) string {
	align := strings.ToLower(firstNonEmpty(html.styleValue(attrs, "text-align"), attrs["align"]))
	switch align {
	case "center", "middle":
		return "C"
	case "right":
		return "R"
	case "left":
		return "L"
	}
	if fallback == "C" || fallback == "R" {
		return fallback
	}
	return "L"
}
