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

type typedLineShadowResult struct {
	Plan  layoutengine.LayoutPlan
	Text  string
	Lines []wrappedTextLine
}

// planTypedParagraphLineShadow is an observational bridge from one plain,
// splittable, core-font paragraph to exact legacy line breaks and the new
// resumable paragraph planner. Fixed command geometry is normative new-plan
// geometry; it is not byte-level parity with the legacy PDF writer's decimal
// operator quantization. This bridge does not paint or participate in
// WriteDocument.
func (f *Document) planTypedParagraphLineShadow(doc *layout.LayoutDocument) (typedLineShadowResult, error) {
	return f.planTypedParagraphLineShadowContext(context.Background(), doc)
}

func (f *Document) planTypedParagraphLineShadowContext(ctx context.Context, doc *layout.LayoutDocument) (typedLineShadowResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := layoutengine.ChargePlanningWork(ctx, "typed paragraph measurement", 1); err != nil {
		return typedLineShadowResult{}, err
	}
	if f == nil || f.err != nil ||
		(f.page != 0 || f.state != documentStateUnopened) && (f.page == 0 || f.state != documentStatePageOpen) ||
		f.clipNest != 0 || f.transformNest != 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentState, "requires an error-free unopened document or active page")
	}
	if doc == nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "layout document is nil")
	}
	if !f.autoPageBreak || f.acceptPageBreakSet || f.headerFnc != nil ||
		f.footerFnc != nil || f.footerFncLpi != nil || f.pageAddGuard != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentPolicy, "custom page lifecycle behavior is present")
	}
	if !typedShadowTemplateHasOnlyMargins(doc.PageTemplate) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowPageTemplate, "only uniform margins are supported")
	}
	if doc.Signature != nil || doc.QR != nil || len(doc.Attachments) != 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "signature, QR, or attachments are present")
	}
	if len(f.aliasMap) != 0 || f.aliasNbPagesStr != "" {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentPolicy, "deferred text aliases are present")
	}
	if f.ws < 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "negative word spacing is not represented by the initial glyph bounds")
	}

	blocks := layout.NormalizeBlocks(doc.Body)
	if len(blocks) != 1 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "requires exactly one paragraph")
	}
	paragraph, ok := blocks[0].(layout.ParagraphBlock)
	if !ok {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowBlockKind, fmt.Sprintf("body[0] is %s", blocks[0].DocumentBlockKind()))
	}
	if typedParagraphNeedsMixedCoreShadow(paragraph, f) {
		return f.planTypedParagraphMixedCoreShadowContext(ctx, doc, paragraph)
	}
	if detail := typedLineShadowParagraphUnsupported(paragraph, f.coreFonts); detail != "" {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, detail)
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

	scratch := documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
	if scratch.err != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, scratch.err.Error())
	}
	scratch.cMargin = f.cMargin
	scratch.ws = f.ws
	scratch.fontFamily = f.fontFamily
	scratch.fontStyle = f.fontStyle
	scratch.fontSizePt = f.fontSizePt
	scratch.fontSize = f.fontSizePt / scratch.k
	style := layout.MergedTextStyle(plannerDefaultTextStyle(scratch), paragraph.EffectiveStyle())
	requestedFamily := strings.ToLower(fontFamilyEscape(firstNonEmpty(style.FontFamily, f.fontFamily, "Helvetica")))
	requestedStyle := ""
	if style.Bold {
		requestedStyle += "B"
	}
	if style.Italic {
		requestedStyle += "I"
	}
	liveKey := requestedFamily + requestedStyle
	var liveFont fontDefinition
	var liveFontExists bool
	if f.resources != nil {
		liveFont, liveFontExists = f.resources.font(liveKey)
	}
	if liveFontExists && liveFont.Tp == "UTF8" {
		if liveFont.utf8File == nil || liveFont.utf8File.fileReader == nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, "UTF-8 font program is unavailable")
		}
		if err := scratch.AddUTF8FontFromBytesError(requestedFamily, requestedStyle, liveFont.utf8File.fileReader.array); err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, err.Error())
		}
	}
	applyPlannerTextStyle(scratch, style)
	if scratch.err != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, "font metrics could not be resolved")
	}
	var fontResource layoutengine.CoreFontResource
	if scratch.isCurrentUTF8 {
		fontResource, _, err = typedEmbeddedUTF8FontResource(scratch.currentFont)
	} else {
		fontResource, err = typedCoreFontResource(scratch.currentFont)
	}
	if err != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, err.Error())
	}
	if f.resources != nil {
		for _, key := range []string{liveKey, scratch.fontFamily + scratch.fontStyle} {
			if liveFont, exists := f.resources.font(key); exists {
				var liveResource layoutengine.CoreFontResource
				var liveErr error
				if scratch.isCurrentUTF8 {
					liveResource, _, liveErr = typedEmbeddedUTF8FontResource(liveFont)
				} else {
					liveResource, liveErr = typedCoreFontResource(liveFont)
				}
				if liveErr != nil || liveResource.Face != fontResource.Face || liveResource.MetricsDigest != fontResource.MetricsDigest ||
					(scratch.isCurrentUTF8 && liveResource.EmbeddedUTF8.Digest != fontResource.EmbeddedUTF8.Digest) {
					return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, "a custom font shadows the canonical core font")
				}
			}
		}
	}

	text := layout.TextSegmentsPlainText(paragraph.Segments)
	if !utf8.ValidString(text) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "text is not valid UTF-8")
	}
	if !scratch.isCurrentUTF8 && !isPlannerCoreText(text) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, "only printable ASCII and line feeds are supported by core fonts")
	}
	align := textAlign(style.Align)
	wrapWidth := contentWidth
	whiteSpace := strings.ToLower(strings.TrimSpace(style.WhiteSpace))
	if whiteSpace == "nowrap" || whiteSpace == "pre" {
		for _, line := range strings.Split(text, "\n") {
			if width := scratch.GetStringWidth(line) + 2*scratch.cMargin; width > wrapWidth {
				wrapWidth = width
			}
		}
	}
	var wrapped wrappedCoreText
	if scratch.isCurrentUTF8 {
		normalized := normalizeCoreMultiCellText(text)
		wrapped.Text = normalized
		scratch.walkWrappedText(normalized, wrappedTextOptions{Mode: wrappedTextUTF8, MaxWidth: scratch.wrappedTextMaxWidth(wrapWidth), AlwaysFinal: true}, func(line wrappedTextLine) bool {
			wrapped.Lines = append(wrapped.Lines, line)
			return true
		})
	} else {
		wrapped, err = scratch.wrapCoreMultiCellText(text, wrapWidth, align)
	}
	if err != nil {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, err.Error())
	}
	lineHeightUser := layout.ResolvedLineHeight(style)
	baselineUser := 0.5*lineHeightUser + 0.3*scratch.fontSize
	toFixed := func(userUnits float64) (layoutengine.Fixed, error) {
		return fixedFromDocumentUnits(f, userUnits)
	}
	lineHeight, err := toFixed(lineHeightUser)
	if err != nil || lineHeight <= 0 || lineHeight > body.Height {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "line height is invalid or exceeds the page body")
	}
	baseline, err := toFixed(baselineUser)
	if err != nil || baseline < 0 || baseline > lineHeight {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "compatibility baseline lies outside the line box")
	}

	legacyLineCounts := typedLineShadowLegacyPageLineCounts(len(wrapped.Lines), top, f.h-bottom, lineHeightUser)
	lines := make([]layoutengine.ParagraphLineInput, len(wrapped.Lines))
	legacyPage, lineOnPage := 0, 0
	for index, wrappedLine := range wrapped.Lines {
		widthUser := wrappedLine.WidthFontUnits * scratch.fontSize / 1000
		offsetUser := scratch.cMargin
		switch align {
		case "C":
			offsetUser = (contentWidth - widthUser) / 2
		case "R":
			offsetUser = contentWidth - scratch.cMargin - widthUser
		}
		width, err := toFixed(widthUser)
		if err != nil || width < 0 {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d width is invalid", index))
		}
		absoluteX, err := toFixed(left + offsetUser)
		if err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d offset is invalid", index))
		}
		offset, err := absoluteX.Sub(body.X)
		if err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d offset overflows", index))
		}
		absoluteTop, err := toFixed(top + float64(lineOnPage)*lineHeightUser)
		if err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d top is invalid", index))
		}
		absoluteBottom, err := toFixed(top + float64(lineOnPage+1)*lineHeightUser)
		if err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d bottom is invalid", index))
		}
		exactHeight, err := absoluteBottom.Sub(absoluteTop)
		if err != nil || exactHeight <= 0 {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d height is invalid", index))
		}
		absoluteBaseline, err := toFixed(top + float64(lineOnPage)*lineHeightUser + baselineUser)
		if err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d baseline is invalid", index))
		}
		exactBaseline, err := absoluteBaseline.Sub(absoluteTop)
		if err != nil || exactBaseline < 0 || exactBaseline > exactHeight {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d baseline lies outside its box", index))
		}
		lines[index] = layoutengine.ParagraphLineInput{
			OffsetX: offset, Width: width, Height: exactHeight, Baseline: exactBaseline,
		}
		lineOnPage++
		if legacyPage < len(legacyLineCounts) && lineOnPage == int(legacyLineCounts[legacyPage]) {
			legacyPage++
			lineOnPage = 0
		}
	}

	plan, err := layoutengine.PlanParagraphFlowContext(ctx, layoutengine.ParagraphFlowInput{
		PageSize: pageSize,
		Body:     body,
		ParagraphLinePlanInput: layoutengine.ParagraphLinePlanInput{
			Node: 1, Key: "@typed-line-shadow", Instance: "@typed-line-shadow",
			Lines: lines, Orphans: 1, Widows: 1, Mode: layoutengine.ParagraphBreakPrefer,
		},
	})
	if err != nil {
		return typedLineShadowResult{}, fmt.Errorf("document: typed paragraph line shadow: %w", err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != len(legacyLineCounts) {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "fixed rounding changes legacy line pagination")
	}
	for index, page := range projection.Pages {
		if page.Lines.Count != legacyLineCounts[index] {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "fixed rounding changes legacy line pagination")
		}
	}
	fontSize, err := layoutengine.FixedFromPoints(scratch.fontSizePt)
	if err != nil || fontSize <= 0 {
		return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowFont, "font size cannot be represented in the glyph plan")
	}
	glyphRuns := make([]layoutengine.CoreGlyphRun, 0, len(wrapped.Lines))
	for index, wrappedLine := range wrapped.Lines {
		codes := wrapped.Text[wrappedLine.StartByte:wrappedLine.EndByte]
		if codes == "" {
			continue
		}
		plannedLine := projection.Lines[index]
		var advances []layoutengine.Fixed
		if scratch.isCurrentUTF8 {
			advances, err = typedUTF8GlyphAdvances(scratch, codes, plannedLine.Bounds.Width)
		} else {
			advances, err = typedCoreGlyphAdvances(scratch, codes, plannedLine.Bounds.Width)
		}
		if err != nil {
			return typedLineShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("line %d: %v", index, err))
		}
		leadingSpace, trailingSpace := false, false
		if preservesHTMLAuthoredWhitespace(ctx) {
			if index > 0 {
				previousLine := wrapped.Lines[index-1]
				gap := wrapped.Text[previousLine.EndByte:previousLine.NextByte]
				leadingSpace = gap != "" && strings.TrimSpace(gap) == ""
			}
			trailingGap := wrapped.Text[wrappedLine.EndByte:wrappedLine.NextByte]
			trailingSpace = trailingGap != "" && strings.TrimSpace(trailingGap) == ""
		}
		glyphRuns = append(glyphRuns, layoutengine.CoreGlyphRun{
			Line: uint32(index), Font: fontResource.ID, FontSize: fontSize,
			Color:  coreGlyphColor(style.Color),
			Origin: layoutengine.Point{X: plannedLine.Bounds.X, Y: plannedLine.Baseline},
			Codes:  codes, LeadingSpace: leadingSpace, TrailingSpace: trailingSpace, Advances: advances, Source: plannedLine.Source,
		})
	}
	var fontResources []layoutengine.CoreFontResource
	if len(glyphRuns) != 0 {
		fontResources = []layoutengine.CoreFontResource{fontResource}
	}
	plan, err = layoutengine.AttachCoreGlyphRuns(plan, fontResources, glyphRuns)
	if err != nil {
		return typedLineShadowResult{}, fmt.Errorf("document: attach typed paragraph glyph runs: %w", err)
	}
	return typedLineShadowResult{
		Plan: plan, Text: wrapped.Text, Lines: append([]wrappedTextLine(nil), wrapped.Lines...),
	}, nil
}

func typedLineShadowLegacyPageLineCounts(lineCount int, top, trigger, lineHeight float64) []uint32 {
	counts := []uint32{0}
	y := top
	for range lineCount {
		if y+lineHeight > trigger {
			counts = append(counts, 0)
			y = top
		}
		counts[len(counts)-1]++
		y += lineHeight
	}
	return counts
}

func typedLineShadowParagraphUnsupported(block layout.ParagraphBlock, coreFonts map[string]bool) string {
	if block.BoxRef != nil || block.Box != (layout.BoxStyle{}) {
		return "paragraph must be a plain splittable box"
	}
	if block.StyleRef != nil {
		return "paragraph style reference is unsupported"
	}
	if block.Style.Underline || block.Style.StrikeThrough {
		return "text decorations are not represented by the line shadow"
	}
	if !validCoreGlyphColor(block.Style.Color) {
		return "text color must be unset or use RGB components from 0 through 255"
	}
	for _, segment := range block.Segments {
		if segment.StyleRef != nil || segment.Style != (layout.TextStyle{}) {
			return "segment styles and style references are unsupported"
		}
	}
	text := layout.TextSegmentsPlainText(block.Segments)
	if strings.ContainsRune(text, '\r') {
		return "carriage returns are unsupported by source-stable line ranges"
	}
	for _, character := range text {
		if character == '\n' || character >= 0x20 && character <= 0xffff {
			continue
		}
		return "text contains a control or non-BMP scalar unsupported by the exact glyph plan"
	}
	return ""
}

func validCoreGlyphColor(color layout.DocumentColor) bool {
	if !color.Set {
		return color.R == 0 && color.G == 0 && color.B == 0
	}
	return color.R >= 0 && color.R <= 255 && color.G >= 0 && color.G <= 255 && color.B >= 0 && color.B <= 255
}

func coreGlyphColor(color layout.DocumentColor) layoutengine.CoreRGBColor {
	if !color.Set {
		return layoutengine.CoreRGBColor{}
	}
	return layoutengine.CoreRGBColor{R: uint8(color.R), G: uint8(color.G), B: uint8(color.B), Set: true} // #nosec G115 -- low-width representation is explicitly normalized before packing
}
