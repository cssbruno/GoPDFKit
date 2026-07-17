// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

// typedParagraphNeedsMixedCoreShadow identifies segment styles that need one
// source-stable line shadow: metric-changing core-font runs and decorations.
// Color-only runs remain on the historical exact wrapping path because their
// advances are already checked by restylePaperMeasurement.
func typedParagraphNeedsMixedCoreShadow(paragraph layout.ParagraphBlock, f *Document) bool {
	if f == nil || paragraph.StyleRef != nil || len(paragraph.Segments) == 0 {
		return false
	}
	base := layout.MergedTextStyle(layout.TextStyle{FontFamily: f.fontFamily, FontSize: f.fontSizePt}, paragraph.EffectiveStyle())
	if base.Underline || base.StrikeThrough {
		return true
	}
	for _, segment := range paragraph.Segments {
		style := layout.MergedTextStyle(base, segment.EffectiveStyle())
		if style.FontFamily != base.FontFamily || style.FontSize != base.FontSize ||
			style.LineHeight != base.LineHeight || style.Bold != base.Bold || style.Italic != base.Italic ||
			style.Underline != base.Underline || style.StrikeThrough != base.StrikeThrough {
			return true
		}
	}
	return false
}

type mixedCoreFontMetrics struct {
	style      layout.TextStyle
	resource   layoutengine.CoreFontResource
	fontSize   layoutengine.Fixed
	fontSizePt float64
	up         int
	ut         int
	charWidths [256]float64
}

type mixedTextFontMetrics struct {
	style        layout.TextStyle
	resource     layoutengine.CoreFontResource
	font         fontDefinition
	fontSize     layoutengine.Fixed
	fontSizeUser float64
	fontSizePt   float64
	up           int
	ut           int
}

func (m *mixedTextFontMetrics) runeWidth(character rune) (float64, bool) {
	if character < 0 || character > 0xffff {
		return 0, false
	}
	index := int(character)
	if index < len(m.font.Cw) {
		width := m.font.Cw[index]
		if width == 65535 {
			return 0, true
		}
		if width > 0 {
			return float64(width) * m.fontSizeUser / 1000, true
		}
	}
	if m.font.Desc.MissingWidth != 0 {
		return float64(m.font.Desc.MissingWidth) * m.fontSizeUser / 1000, true
	}
	return 500 * m.fontSizeUser / 1000, true
}

type mixedCoreLine struct {
	start int
	end   int
	width float64
}

type mixedCoreText struct {
	text    string
	styles  []int
	widths  []float64
	metrics []*mixedCoreFontMetrics
	lines   []mixedCoreLine
}

func (f *Document) planTypedParagraphMixedCoreShadowContext(ctx context.Context, doc *layout.LayoutDocument, paragraph layout.ParagraphBlock) (typedLineShadowResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := layoutengine.ChargePlanningWork(ctx, "typed mixed paragraph measurement", uint64(maxInt(len(paragraph.Segments), 1))); err != nil {
		return typedLineShadowResult{}, err
	}
	if paragraph.BoxRef != nil || paragraph.Box != (layout.BoxStyle{}) || paragraph.StyleRef != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed core paragraph must be a splittable block without box or style references")
	}
	if !validCoreGlyphColor(paragraph.Style.Color) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "text color must be unset or use RGB components from 0 through 255")
	}
	usesEmbedded, fontErr := f.mixedParagraphUsesEmbeddedFont(paragraph)
	if fontErr != nil {
		return typedLineShadowResult{}, fontErr
	}
	if usesEmbedded {
		return f.planTypedParagraphMixedTextShadowContext(ctx, doc, paragraph)
	}

	text := layout.TextSegmentsPlainText(paragraph.Segments)
	if strings.ContainsRune(text, '\r') {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "carriage returns are unsupported by source-stable mixed line ranges")
	}
	text = normalizeCoreMultiCellText(text)
	if !utf8.ValidString(text) || !isPlannerCoreText(text) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed core text requires printable ASCII and line feeds")
	}

	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	contentWidth := f.w - left - right
	bodyHeight := f.h - top - bottom
	if contentWidth <= 0 || bodyHeight <= 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "page margins leave no body area")
	}
	pageSize, body, err := typedShadowFixedGeometry(f, left, top, contentWidth, bodyHeight)
	if err != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, err.Error())
	}

	base := layout.MergedTextStyle(layout.TextStyle{FontFamily: f.fontFamily, FontSize: f.fontSizePt}, paragraph.EffectiveStyle())
	metricsByStyle := make(map[layout.TextStyle]int)
	metrics := make([]*mixedCoreFontMetrics, 0, len(paragraph.Segments))
	styles := make([]int, 0, len(text))
	widths := make([]float64, 0, len(text))
	var fullText strings.Builder
	for _, segment := range paragraph.Segments {
		style := layout.MergedTextStyle(base, segment.EffectiveStyle())
		if segment.StyleRef != nil || style.Align != "" && style.Align != base.Align || !validCoreGlyphColor(style.Color) {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed inline style changes alignment or uses an invalid color")
		}
		if f.userUnderlineThickness != 1 && (style.Underline || style.StrikeThrough) {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "custom underline thickness is outside the mixed core decoration contract")
		}
		if segment.Link != "" && strings.TrimSpace(segment.Link) != segment.Link {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed link targets must be canonical")
		}
		metricIndex, exists := metricsByStyle[style]
		if !exists {
			metric, metricErr := f.mixedCoreFontMetrics(style)
			if metricErr != nil {
				return typedLineShadowResult{}, metricErr
			}
			metricIndex = len(metrics)
			metricsByStyle[style] = metricIndex
			metrics = append(metrics, metric)
		}
		fullText.WriteString(segment.Text)
	}
	if fullText.String() != text {
		// normalizeCoreMultiCellText only removes carriage returns and one final
		// newline. The HTML adapter normally has already normalized these, but
		// keep the range contract explicit for direct typed callers.
		if !strings.HasPrefix(fullText.String(), text) || len(fullText.String())-len(text) > 1 {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed text normalization changed authored byte ranges")
		}
	}
	for _, segment := range paragraph.Segments {
		style := layout.MergedTextStyle(base, segment.EffectiveStyle())
		metricIndex := metricsByStyle[style]
		for _, character := range segment.Text {
			if character > 0x7f {
				return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed core text contains a non-ASCII scalar")
			}
			if len(styles) >= len(text) {
				break
			}
			styles = append(styles, metricIndex)
			width := metrics[metricIndex].charWidths[byte(character)]
			if character == ' ' {
				width += f.ws
			}
			widths = append(widths, width)
		}
	}
	if len(styles) != len(text) || len(widths) != len(text) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed style ranges do not cover normalized text")
	}

	maxWidth := contentWidth - 2*f.cMargin
	if maxWidth <= 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "mixed paragraph has no usable line width")
	}
	for _, metric := range metrics {
		if strings.EqualFold(metric.style.WhiteSpace, "nowrap") || strings.EqualFold(metric.style.WhiteSpace, "pre") {
			var natural float64
			for index, width := range widths {
				if text[index] == '\n' {
					if natural > maxWidth {
						maxWidth = natural
					}
					natural = 0
					continue
				}
				natural += width
			}
			if natural > maxWidth {
				maxWidth = natural
			}
			break
		}
	}

	lines := make([]mixedCoreLine, 0, 1)
	lineStart, lastSpace := 0, -1
	lineWidth, widthBeforeSpace := 0.0, 0.0
	emit := func(end int, width float64) {
		lines = append(lines, mixedCoreLine{start: lineStart, end: end, width: width})
	}
	for index := 0; index < len(text); index++ {
		character := text[index]
		if character == '\n' {
			emit(index, lineWidth)
			lineStart, lineWidth, lastSpace, widthBeforeSpace = index+1, 0, -1, 0
			continue
		}
		before := lineWidth
		lineWidth += widths[index]
		if character == ' ' {
			lastSpace, widthBeforeSpace = index, before
		}
		if lineWidth > maxWidth {
			if lastSpace >= lineStart {
				emit(lastSpace, widthBeforeSpace)
				lineStart, lineWidth, lastSpace, widthBeforeSpace = lastSpace+1, 0, -1, 0
				for reset := lineStart; reset <= index; reset++ {
					lineWidth += widths[reset]
					if text[reset] == ' ' {
						lastSpace, widthBeforeSpace = reset, lineWidth-widths[reset]
					}
				}
				continue
			}
			if index == lineStart {
				emit(index+1, lineWidth)
				lineStart, lineWidth = index+1, 0
			} else {
				emit(index, before)
				lineStart, lineWidth, lastSpace, widthBeforeSpace = index, widths[index], -1, 0
			}
		}
	}
	if lineStart < len(text) || len(lines) == 0 {
		emit(len(text), lineWidth)
	}

	resultText := mixedCoreText{text: text, styles: styles, widths: widths, metrics: metrics, lines: lines}
	lineInputs := make([]layoutengine.ParagraphLineInput, len(lines))
	lineTopUser := 0.0
	for index, line := range lines {
		lineHeightUser, maxFontSize := layout.ResolvedLineHeight(base), base.FontSize
		if maxFontSize <= 0 {
			maxFontSize = f.fontSizePt
		}
		for cursor := line.start; cursor < line.end; cursor++ {
			style := metrics[styles[cursor]].style
			lineHeightUser = maxFloat(lineHeightUser, layout.ResolvedLineHeight(style))
			maxFontSize = maxFloat(maxFontSize, style.FontSize)
		}
		lineHeight, heightErr := fixedFromDocumentUnits(f, lineHeightUser)
		if heightErr != nil || lineHeight <= 0 || lineHeight > body.Height {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d height is invalid", index))
		}
		width, widthErr := fixedFromDocumentUnits(f, line.width)
		if widthErr != nil || width < 0 {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d width is invalid", index))
		}
		offsetUser := f.cMargin
		align := base.Align
		switch align {
		case "C":
			offsetUser = (contentWidth - line.width) / 2
		case "R":
			offsetUser = contentWidth - f.cMargin - line.width
		case "L", "J", "":
		default:
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed paragraph alignment is unsupported")
		}
		absoluteX, xErr := fixedFromDocumentUnits(f, left+offsetUser)
		absoluteTop, topErr := fixedFromDocumentUnits(f, top+lineTopUser)
		absoluteBottom, bottomErr := fixedFromDocumentUnits(f, top+lineTopUser+lineHeightUser)
		absoluteBaseline, baselineErr := fixedFromDocumentUnits(f, top+lineTopUser+0.5*lineHeightUser+0.3*maxFontSize)
		if xErr != nil || topErr != nil || bottomErr != nil || baselineErr != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d position is invalid", index))
		}
		height, heightSubErr := absoluteBottom.Sub(absoluteTop)
		baseline, baselineSubErr := absoluteBaseline.Sub(absoluteTop)
		offset, offsetErr := absoluteX.Sub(body.X)
		if heightSubErr != nil || baselineSubErr != nil || offsetErr != nil || height <= 0 || baseline < 0 || baseline > height {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d geometry is invalid", index))
		}
		lineInputs[index] = layoutengine.ParagraphLineInput{OffsetX: offset, Width: width, Height: height, Baseline: baseline}
		lineTopUser += lineHeightUser
	}
	plan, err := layoutengine.PlanParagraphFlowContext(ctx, layoutengine.ParagraphFlowInput{
		PageSize: pageSize, Body: body,
		ParagraphLinePlanInput: layoutengine.ParagraphLinePlanInput{Node: 1, Key: "@typed-mixed-line-shadow", Instance: "@typed-mixed-line-shadow", Lines: lineInputs, Orphans: 1, Widows: 1, Mode: layoutengine.ParagraphBreakPrefer},
	})
	if err != nil {
		return typedLineShadowResult{}, fmt.Errorf("document: typed mixed paragraph line shadow: %w", err)
	}
	projection := plan.Projection()
	fonts := make([]layoutengine.CoreFontResource, 0, len(metrics))
	fontIDs := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	runs := make([]layoutengine.CoreGlyphRun, 0, len(text))
	paths := make([]layoutengine.PlannedPath, 0)
	strokes := make([]layoutengine.PlannedStroke, 0)
	items := make([]layoutengine.DisplayItem, 0, len(text))
	for lineIndex, line := range lines {
		plannedLine := projection.Lines[lineIndex]
		var lineCursor layoutengine.Fixed
		for cursor := line.start; cursor < line.end; {
			metricIndex := styles[cursor]
			metric := metrics[metricIndex]
			end := cursor + 1
			for end < line.end && styles[end] == metricIndex {
				end++
			}
			resourceIdentity := paperFontIdentity(metric.resource)
			fontID := fontIDs[resourceIdentity]
			if !fontID.Valid() {
				fontID = layoutengine.FontResourceID(len(fonts) + 1)
				resource := metric.resource
				resource.ID = fontID
				fonts = append(fonts, resource)
				fontIDs[resourceIdentity] = fontID
			}
			runStartCursor := lineCursor
			advances := make([]layoutengine.Fixed, end-cursor)
			previous := runStartCursor
			runOriginX, originErr := plannedLine.Bounds.X.Add(runStartCursor)
			if originErr != nil {
				return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d glyph origin overflows", lineIndex))
			}
			for characterIndex := cursor; characterIndex < end; characterIndex++ {
				target, targetErr := fixedFromDocumentUnits(f, lineWidthBefore(resultText.widths, line.start, characterIndex+1))
				if targetErr != nil {
					return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d glyph advance is invalid", lineIndex))
				}
				lineCursor = target
				advance, advanceErr := lineCursor.Sub(previous)
				if advanceErr != nil || advance < 0 {
					return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed line %d glyph advance is invalid", lineIndex))
				}
				advances[characterIndex-cursor] = advance
				previous = lineCursor
			}
			runIndex := len(runs)
			run := layoutengine.CoreGlyphRun{Line: uint32(lineIndex), Font: fontID, FontSize: metric.fontSize, Color: coreGlyphColor(metric.style.Color), Origin: layoutengine.Point{X: runOriginX, Y: plannedLine.Baseline}, Codes: text[cursor:end], Advances: advances, Source: plannedLine.Source}
			runs = append(runs, run)
			items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(runIndex)})
			if metric.style.Underline || metric.style.StrikeThrough {
				decorationErr := appendMixedCoreDecorations(f, metric, runOriginX, runStartCursor, lineCursor, plannedLine, &paths, &strokes, &items)
				if decorationErr != nil {
					return typedLineShadowResult{}, decorationErr
				}
			}
			cursor = end
		}
	}
	plan, err = layoutengine.AttachDisplayList(plan, layoutengine.DisplayListInput{Fonts: fonts, GlyphRuns: runs, Paths: paths, Strokes: strokes, Items: items})
	if err != nil {
		return typedLineShadowResult{}, fmt.Errorf("document: attach mixed typed paragraph glyph runs: %w", err)
	}
	return typedLineShadowResult{Plan: plan, Text: resultText.text}, nil
}

func (f *Document) mixedCoreFontMetrics(style layout.TextStyle) (*mixedCoreFontMetrics, error) {
	scratch := documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
	scratch.cMargin, scratch.ws = f.cMargin, f.ws
	scratch.fontFamily, scratch.fontStyle = f.fontFamily, f.fontStyle
	scratch.fontSizePt, scratch.fontSize = f.fontSizePt, f.fontSizePt/scratch.k
	applyPDFTextStyle(scratch, style)
	if scratch.err != nil || scratch.isCurrentUTF8 {
		return nil, newTypedShadowUnsupported(typedShadowFont, "mixed inline style requires a canonical core font")
	}
	resource, err := typedCoreFontResource(scratch.currentFont)
	if err != nil {
		return nil, newTypedShadowUnsupported(typedShadowFont, err.Error())
	}
	fontSize, err := layoutengine.FixedFromPoints(scratch.fontSizePt)
	if err != nil || fontSize <= 0 {
		return nil, newTypedShadowUnsupported(typedShadowFont, "mixed inline font size is not representable")
	}
	if f.resources != nil {
		requestedFamily := strings.ToLower(fontFamilyEscape(firstNonEmpty(style.FontFamily, f.fontFamily, "Helvetica")))
		requestedStyle := ""
		if style.Bold {
			requestedStyle += "B"
		}
		if style.Italic {
			requestedStyle += "I"
		}
		for _, key := range []string{requestedFamily + requestedStyle, scratch.fontFamily + scratch.fontStyle} {
			if liveFont, exists := f.resources.font(key); exists {
				liveResource, liveErr := typedCoreFontResource(liveFont)
				if liveErr != nil || liveResource.Face != resource.Face || liveResource.MetricsDigest != resource.MetricsDigest {
					return nil, newTypedShadowUnsupported(typedShadowFont, "a custom font shadows the canonical core font")
				}
			}
		}
	}
	result := &mixedCoreFontMetrics{
		style: style, resource: resource, fontSize: fontSize,
		fontSizePt: scratch.fontSizePt, up: scratch.currentFont.Up, ut: scratch.currentFont.Ut,
	}
	if result.ut <= 0 && (style.Underline || style.StrikeThrough) {
		return nil, newTypedShadowUnsupported(typedShadowFont, "mixed inline decoration has no canonical core-font thickness")
	}
	for index := range result.charWidths {
		result.charWidths[index] = float64(scratch.currentFontRuneWidth(rune(index))) * scratch.fontSize / 1000
	}
	return result, nil
}

func appendMixedCoreDecorations(
	f *Document,
	metric *mixedCoreFontMetrics,
	runOriginX, runStartCursor, runEndCursor layoutengine.Fixed,
	line layoutengine.PlannedLine,
	paths *[]layoutengine.PlannedPath,
	strokes *[]layoutengine.PlannedStroke,
	items *[]layoutengine.DisplayItem,
) error {
	return appendMixedTextDecorations(f, metric.style, metric.fontSizePt, metric.up, metric.ut,
		runOriginX, runStartCursor, runEndCursor, line, paths, strokes, items)
}

func appendMixedTextDecorations(
	f *Document,
	style layout.TextStyle,
	fontSizePt float64,
	up, ut int,
	runOriginX, runStartCursor, runEndCursor layoutengine.Fixed,
	line layoutengine.PlannedLine,
	paths *[]layoutengine.PlannedPath,
	strokes *[]layoutengine.PlannedStroke,
	items *[]layoutengine.DisplayItem,
) error {
	width, err := runEndCursor.Sub(runStartCursor)
	if err != nil || width <= 0 {
		return newTypedShadowUnsupported(typedShadowGeometry, "mixed decoration run has no representable width")
	}
	endX, err := runOriginX.Add(width)
	if err != nil {
		return newTypedShadowUnsupported(typedShadowGeometry, "mixed decoration run overflows the line")
	}
	strokeWidth, err := layoutengine.FixedFromPoints(float64(ut) / 1000 * fontSizePt * f.userUnderlineThickness)
	if err != nil || strokeWidth <= 0 {
		return newTypedShadowUnsupported(typedShadowFont, "mixed decoration thickness is not representable")
	}
	color := coreGlyphColor(style.Color)
	if !color.Set {
		color.Set = true
	}
	appendStroke := func(offsetPoints float64) error {
		offset, offsetErr := layoutengine.FixedFromPoints(offsetPoints)
		if offsetErr != nil {
			return offsetErr
		}
		y, yErr := line.Baseline.Add(offset)
		if yErr != nil {
			return yErr
		}
		start := layoutengine.Point{X: runOriginX, Y: y}
		end := layoutengine.Point{X: endX, Y: y}
		bounds, boundsErr := layoutengine.RectFromPoints(start, end)
		if boundsErr != nil {
			return boundsErr
		}
		pathIndex := uint32(len(*paths))
		*paths = append(*paths, layoutengine.PlannedPath{Segments: []layoutengine.PathSegment{
			{Kind: layoutengine.PathMoveTo, Point: start},
			{Kind: layoutengine.PathLineTo, Point: end},
		}, Bounds: bounds})
		strokeIndex := uint32(len(*strokes))
		*strokes = append(*strokes, layoutengine.PlannedStroke{
			Path: pathIndex, Color: color, Width: strokeWidth,
			LineCap: layoutengine.StrokeCapButt, Fragment: line.Fragment,
		})
		*items = append(*items, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: strokeIndex})
		return nil
	}
	if style.Underline {
		if err := appendStroke(-float64(up) / 1000 * fontSizePt); err != nil {
			return newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed underline geometry is invalid: %v", err))
		}
	}
	if style.StrikeThrough {
		if err := appendStroke(4 * float64(up) / 1000 * fontSizePt); err != nil {
			return newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed strike geometry is invalid: %v", err))
		}
	}
	return nil
}

func maxFloat(first, second float64) float64 {
	if first > second {
		return first
	}
	return second
}

// lineWidthBefore returns a user-unit prefix width. It is kept separate from
// the fixed conversion so every glyph run in a line shares one rounded cursor.
func lineWidthBefore(widths []float64, start, end int) float64 {
	var result float64
	for index := start; index < end; index++ {
		result += widths[index]
	}
	return result
}
