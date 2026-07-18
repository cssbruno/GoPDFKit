// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

// mixedParagraphUsesEmbeddedFont resolves the exact fonts used by a mixed
// paragraph before the core-only shadow is selected. A paragraph with only
// one UTF-8 style remains on the existing plain UTF-8 path; this detector is
// for metric-changing mixed runs that need one display list.
func (f *Document) mixedParagraphUsesEmbeddedFont(paragraph layout.ParagraphBlock) (bool, error) {
	base := layout.MergedTextStyle(layout.TextStyle{FontFamily: f.fontFamily, FontSize: f.fontSizePt}, paragraph.EffectiveStyle())
	seen := make(map[layout.TextStyle]struct{}, len(paragraph.Segments)+1)
	styles := make([]layout.TextStyle, 0, len(paragraph.Segments)+1)
	styles = append(styles, base)
	for _, segment := range paragraph.Segments {
		styles = append(styles, layout.MergedTextStyle(base, segment.EffectiveStyle()))
	}
	for _, style := range styles {
		if _, exists := seen[style]; exists {
			continue
		}
		seen[style] = struct{}{}
		metric, err := f.mixedTextFontMetrics(style)
		if err != nil {
			return false, err
		}
		if metric.resource.EmbeddedUTF8 != nil {
			return true, nil
		}
	}
	return false, nil
}

func (f *Document) mixedTextFontMetrics(style layout.TextStyle) (*mixedTextFontMetrics, error) {
	scratch := documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
	scratch.cMargin, scratch.ws = f.cMargin, f.ws
	scratch.fontFamily, scratch.fontStyle = f.fontFamily, f.fontStyle
	scratch.fontSizePt, scratch.fontSize = f.fontSizePt, f.fontSizePt/scratch.k
	requestedFamily := strings.ToLower(fontFamilyEscape(firstNonEmpty(style.FontFamily, f.fontFamily, "Helvetica")))
	requestedStyle := ""
	if style.Bold {
		requestedStyle += "B"
	}
	if style.Italic {
		requestedStyle += "I"
	}
	liveKey := requestedFamily + requestedStyle
	if f.resources != nil {
		if liveFont, exists := f.resources.font(liveKey); exists && liveFont.Tp == "UTF8" {
			if liveFont.utf8File == nil || liveFont.utf8File.fileReader == nil {
				return nil, newTypedShadowUnsupported(typedShadowFont, "UTF-8 font program is unavailable")
			}
			if err := scratch.AddUTF8FontFromBytesError(requestedFamily, requestedStyle, liveFont.utf8File.fileReader.array); err != nil {
				return nil, newTypedShadowUnsupported(typedShadowFont, err.Error())
			}
		}
	}
	applyPlannerTextStyle(scratch, style)
	if scratch.err != nil {
		return nil, newTypedShadowUnsupported(typedShadowFont, "font metrics could not be resolved")
	}
	var resource layoutengine.CoreFontResource
	var err error
	if scratch.isCurrentUTF8 {
		resource, _, err = typedEmbeddedUTF8FontResource(scratch.currentFont)
	} else {
		resource, err = typedCoreFontResource(scratch.currentFont)
	}
	if err != nil {
		return nil, newTypedShadowUnsupported(typedShadowFont, err.Error())
	}
	if f.resources != nil {
		for _, key := range []string{liveKey, scratch.fontFamily + scratch.fontStyle} {
			liveFont, exists := f.resources.font(key)
			if !exists {
				continue
			}
			var liveResource layoutengine.CoreFontResource
			var liveErr error
			if scratch.isCurrentUTF8 {
				liveResource, _, liveErr = typedEmbeddedUTF8FontResource(liveFont)
			} else {
				liveResource, liveErr = typedCoreFontResource(liveFont)
			}
			if liveErr != nil || liveResource.Face != resource.Face || liveResource.MetricsDigest != resource.MetricsDigest ||
				(scratch.isCurrentUTF8 && (liveResource.EmbeddedUTF8 == nil || resource.EmbeddedUTF8 == nil || liveResource.EmbeddedUTF8.Digest != resource.EmbeddedUTF8.Digest)) {
				return nil, newTypedShadowUnsupported(typedShadowFont, "a custom font shadows the canonical mixed font")
			}
		}
	}
	fontSize, err := layoutengine.FixedFromPoints(scratch.fontSizePt)
	if err != nil || fontSize <= 0 {
		return nil, newTypedShadowUnsupported(typedShadowFont, "mixed inline font size is not representable")
	}
	metric := &mixedTextFontMetrics{
		style: style, resource: resource, font: scratch.currentFont,
		fontSize: fontSize, fontSizeUser: scratch.fontSize,
		fontSizePt: scratch.fontSizePt, up: scratch.currentFont.Up, ut: scratch.currentFont.Ut,
	}
	if metric.ut <= 0 && (style.Underline || style.StrikeThrough) {
		return nil, newTypedShadowUnsupported(typedShadowFont, "mixed inline decoration has no font thickness")
	}
	return metric, nil
}

func (f *Document) planTypedParagraphMixedTextShadowContext(ctx context.Context, doc *layout.LayoutDocument, paragraph layout.ParagraphBlock) (typedLineShadowResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := layoutengine.ChargePlanningWork(ctx, "typed mixed UTF-8 paragraph measurement", uint64(maxInt(len(paragraph.Segments), 1))); err != nil {
		return typedLineShadowResult{}, err
	}
	if paragraph.BoxRef != nil || paragraph.Box != (layout.BoxStyle{}) || paragraph.StyleRef != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed UTF-8 paragraph must be a splittable block without box or style references")
	}
	if !validCoreGlyphColor(paragraph.Style.Color) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "text color must be unset or use RGB components from 0 through 255")
	}
	text := layout.TextSegmentsPlainText(paragraph.Segments)
	if strings.ContainsRune(text, '\r') {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "carriage returns are unsupported by source-stable mixed line ranges")
	}
	if !utf8.ValidString(text) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "text is not valid UTF-8")
	}
	text = normalizeCoreMultiCellText(text)
	runes := []rune(text)

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
	metrics := make([]*mixedTextFontMetrics, 0, len(paragraph.Segments)+1)
	metricForStyle := func(style layout.TextStyle) (int, error) {
		if index, exists := metricsByStyle[style]; exists {
			return index, nil
		}
		metric, metricErr := f.mixedTextFontMetrics(style)
		if metricErr != nil {
			return 0, metricErr
		}
		index := len(metrics)
		metricsByStyle[style] = index
		metrics = append(metrics, metric)
		return index, nil
	}
	if _, err := metricForStyle(base); err != nil {
		return typedLineShadowResult{}, err
	}

	styles := make([]int, 0, len(runes))
	widths := make([]float64, 0, len(runes))
	authored := 0
	rawRunes := []rune(layout.TextSegmentsPlainText(paragraph.Segments))
	for _, segment := range paragraph.Segments {
		style := layout.MergedTextStyle(base, segment.EffectiveStyle())
		if segment.StyleRef != nil || (style.Align != "" && style.Align != base.Align) || !validCoreGlyphColor(style.Color) {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed inline style changes alignment or uses an invalid color")
		}
		if f.userUnderlineThickness != 1 && (style.Underline || style.StrikeThrough) {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "custom underline thickness is outside the mixed decoration contract")
		}
		if segment.Link != "" && strings.TrimSpace(segment.Link) != segment.Link {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed link targets must be canonical")
		}
		metricIndex, metricErr := metricForStyle(style)
		if metricErr != nil {
			return typedLineShadowResult{}, metricErr
		}
		for _, character := range segment.Text {
			if authored < len(rawRunes) {
				authored++
			}
			if character == '\r' || len(styles) >= len(runes) {
				continue
			}
			if character == '\n' && authored == len(rawRunes) && len(rawRunes) > 0 && rawRunes[len(rawRunes)-1] == '\n' {
				continue
			}
			if (character < 0x20 && character != '\n') || character > 0xffff {
				return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed text supports controls other than line feeds and non-BMP scalars only on the full shaping path")
			}
			if metrics[metricIndex].resource.EmbeddedUTF8 == nil && character != '\n' && (character < 0x20 || character > 0x7e) {
				return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed core runs require printable ASCII")
			}
			width, widthOK := metrics[metricIndex].runeWidth(character)
			if !widthOK || !isFiniteFloat(width) || width < 0 {
				return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, "mixed glyph width is not representable")
			}
			if character == ' ' {
				width += f.ws
			}
			styles = append(styles, metricIndex)
			widths = append(widths, width)
		}
	}
	if len(styles) != len(runes) || len(widths) != len(runes) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed style ranges do not cover normalized text")
	}

	maxWidth := contentWidth - 2*f.cMargin
	if maxWidth <= 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "mixed paragraph has no usable line width")
	}
	for _, metric := range metrics {
		if strings.EqualFold(metric.style.WhiteSpace, "nowrap") || strings.EqualFold(metric.style.WhiteSpace, "pre") {
			natural := 0.0
			for index, width := range widths {
				if runes[index] == '\n' {
					maxWidth = maxFloat(maxWidth, natural)
					natural = 0
					continue
				}
				natural += width
			}
			maxWidth = maxFloat(maxWidth, natural)
			break
		}
	}

	lines := make([]mixedCoreLine, 0, 1)
	lineStart, lastSpace := 0, -1
	lineWidth, widthBeforeSpace := 0.0, 0.0
	emit := func(end int, width float64) {
		lines = append(lines, mixedCoreLine{start: lineStart, end: end, width: width})
	}
	isCJKBreak := func(character rune) bool {
		return unicode.Is(unicode.Han, character) || unicode.Is(unicode.Hiragana, character) ||
			unicode.Is(unicode.Katakana, character) || unicode.Is(unicode.Hangul, character)
	}
	for index, character := range runes {
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
		if lineWidth <= maxWidth {
			continue
		}
		if lastSpace >= lineStart {
			emit(lastSpace, widthBeforeSpace)
			lineStart, lineWidth, lastSpace, widthBeforeSpace = lastSpace+1, 0, -1, 0
			for reset := lineStart; reset <= index; reset++ {
				lineWidth += widths[reset]
				if runes[reset] == ' ' {
					lastSpace, widthBeforeSpace = reset, lineWidth-widths[reset]
				}
			}
			continue
		}
		if index > lineStart && isCJKBreak(character) {
			emit(index, before)
			lineStart, lineWidth, lastSpace, widthBeforeSpace = index, widths[index], -1, 0
			continue
		}
		if index == lineStart {
			emit(index+1, lineWidth)
			lineStart, lineWidth, lastSpace, widthBeforeSpace = index+1, 0, -1, 0
		} else {
			emit(index, before)
			lineStart, lineWidth, lastSpace, widthBeforeSpace = index, widths[index], -1, 0
		}
	}
	if lineStart < len(runes) || len(lines) == 0 {
		emit(len(runes), lineWidth)
	}

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
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d height is invalid", index))
		}
		width, widthErr := fixedFromDocumentUnits(f, line.width)
		if widthErr != nil || width < 0 {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d width is invalid", index))
		}
		offsetUser := f.cMargin
		align := strings.ToUpper(strings.TrimSpace(base.Align))
		switch align {
		case "C", "CENTER":
			offsetUser = (contentWidth - line.width) / 2
		case "R", "RIGHT":
			offsetUser = contentWidth - f.cMargin - line.width
		case "L", "LEFT", "J", "JUSTIFY", "":
		default:
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "mixed paragraph alignment is unsupported")
		}
		absoluteX, xErr := fixedFromDocumentUnits(f, left+offsetUser)
		absoluteTop, topErr := fixedFromDocumentUnits(f, top+lineTopUser)
		absoluteBottom, bottomErr := fixedFromDocumentUnits(f, top+lineTopUser+lineHeightUser)
		absoluteBaseline, baselineErr := fixedFromDocumentUnits(f, top+lineTopUser+0.5*lineHeightUser+0.3*maxFontSize)
		if xErr != nil || topErr != nil || bottomErr != nil || baselineErr != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d position is invalid", index))
		}
		height, heightSubErr := absoluteBottom.Sub(absoluteTop)
		baseline, baselineSubErr := absoluteBaseline.Sub(absoluteTop)
		offset, offsetErr := absoluteX.Sub(body.X)
		if heightSubErr != nil || baselineSubErr != nil || offsetErr != nil || height <= 0 || baseline < 0 || baseline > height {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d geometry is invalid", index))
		}
		lineInputs[index] = layoutengine.ParagraphLineInput{OffsetX: offset, Width: width, Height: height, Baseline: baseline}
		lineTopUser += lineHeightUser
	}
	plan, err := layoutengine.PlanParagraphFlowContext(ctx, layoutengine.ParagraphFlowInput{
		PageSize: pageSize, Body: body,
		ParagraphLinePlanInput: layoutengine.ParagraphLinePlanInput{Node: 1, Key: "@typed-mixed-utf8-line-shadow", Instance: "@typed-mixed-utf8-line-shadow", Lines: lineInputs, Orphans: 1, Widows: 1, Mode: layoutengine.ParagraphBreakPrefer},
	})
	if err != nil {
		return typedLineShadowResult{}, fmt.Errorf("document: typed mixed UTF-8 paragraph line shadow: %w", err)
	}
	projection := plan.Projection()
	fonts := make([]layoutengine.CoreFontResource, 0, len(metrics))
	fontIDs := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	runs := make([]layoutengine.CoreGlyphRun, 0, len(runes))
	paths := make([]layoutengine.PlannedPath, 0)
	strokes := make([]layoutengine.PlannedStroke, 0)
	items := make([]layoutengine.DisplayItem, 0, len(runes))
	for lineIndex, line := range lines {
		plannedLine := projection.Lines[lineIndex]
		var lineCursor layoutengine.Fixed
		for cursor := line.start; cursor < line.end; {
			metric := metrics[styles[cursor]]
			end := cursor + 1
			for end < line.end && styles[end] == styles[cursor] {
				end++
			}
			identity := paperFontIdentity(metric.resource)
			fontID := fontIDs[identity]
			if !fontID.Valid() {
				fontID = layoutengine.FontResourceID(len(fonts) + 1)
				resource := metric.resource
				resource.ID = fontID
				fonts = append(fonts, resource)
				fontIDs[identity] = fontID
			}
			runStartCursor := lineCursor
			runOriginX, originErr := plannedLine.Bounds.X.Add(runStartCursor)
			if originErr != nil {
				return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d glyph origin overflows", lineIndex))
			}
			advances := make([]layoutengine.Fixed, end-cursor)
			previous := runStartCursor
			for characterIndex := cursor; characterIndex < end; characterIndex++ {
				target, targetErr := fixedFromDocumentUnits(f, lineWidthBefore(widths, line.start, characterIndex+1))
				if targetErr != nil {
					return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d glyph advance is invalid", lineIndex))
				}
				lineCursor = target
				advance, advanceErr := lineCursor.Sub(previous)
				if advanceErr != nil || advance < 0 {
					return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("mixed UTF-8 line %d glyph advance is invalid", lineIndex))
				}
				advances[characterIndex-cursor] = advance
				previous = lineCursor
			}
			runIndex := len(runs)
			run := layoutengine.CoreGlyphRun{Line: uint32(lineIndex), Font: fontID, FontSize: metric.fontSize, Color: coreGlyphColor(metric.style.Color), Origin: layoutengine.Point{X: runOriginX, Y: plannedLine.Baseline}, Codes: string(runes[cursor:end]), Advances: advances, Source: plannedLine.Source}
			runs = append(runs, run)
			items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(runIndex)})
			if metric.style.Underline || metric.style.StrikeThrough {
				if err := appendMixedTextDecorations(f, metric.style, metric.fontSizePt, metric.up, metric.ut, runOriginX, runStartCursor, lineCursor, plannedLine, &paths, &strokes, &items); err != nil {
					return typedLineShadowResult{}, err
				}
			}
			cursor = end
		}
	}
	plan, err = layoutengine.AttachDisplayList(plan, layoutengine.DisplayListInput{Fonts: fonts, GlyphRuns: runs, Paths: paths, Strokes: strokes, Items: items})
	if err != nil {
		return typedLineShadowResult{}, fmt.Errorf("document: attach mixed UTF-8 paragraph glyph runs: %w", err)
	}
	return typedLineShadowResult{Plan: plan, Text: text}, nil
}
