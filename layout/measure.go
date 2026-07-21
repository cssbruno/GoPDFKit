// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

import (
	"strings"
	"unicode"

	"github.com/cssbruno/gopdfkit/internal/layoutgeom"
)

const (
	paragraphSpacing = 2.0
	headingTopSpace  = 2.5
	headingBotSpace  = 1.5
)

// TextMeasurer measures wrapped text for layout estimation.
type TextMeasurer interface {
	TextLineCount(text string, style TextStyle, width float64) int
}

// MeasureContext contains the renderer state needed to estimate block layout.
type MeasureContext struct {
	Width        float64      // Available content width.
	DefaultStyle TextStyle    // Default text style for measurement.
	CellPadding  float64      // Default table cell padding.
	TextMeasurer TextMeasurer // Optional renderer-specific text measurer.
}

// NewMeasureContext creates a renderer-independent measurement context.
func NewMeasureContext(width float64, defaultStyle TextStyle) MeasureContext {
	if width < 0 {
		width = 0
	}
	if defaultStyle.FontFamily == "" {
		defaultStyle.FontFamily = "Helvetica"
	}
	if defaultStyle.FontSize <= 0 {
		defaultStyle.FontSize = 12
	}
	if defaultStyle.LineHeight <= 0 {
		defaultStyle.LineHeight = 5
	}
	return MeasureContext{
		Width:        width,
		DefaultStyle: defaultStyle,
		CellPadding:  1,
	}
}

// BlockMeasurement is the estimated layout footprint of a block.
type BlockMeasurement struct {
	Kind          BlockKind          // Block kind measured.
	Width         float64            // Estimated block width.
	Height        float64            // Estimated full block height.
	MinHeight     float64            // Minimum height required to start rendering.
	Splittable    bool               // Whether the block can split across pages.
	KeepTogether  bool               // Whether the block prefers one page.
	KeepWithNext  bool               // Whether the block should stay with the next one.
	BreakBefore   bool               // Whether rendering requires a prior page break.
	BreakAfter    bool               // Whether rendering requires a following page break.
	ChildMeasures []BlockMeasurement // Measurements for nested child blocks.
}

// Fits reports whether the whole block fits in the available height.
func (m BlockMeasurement) Fits(availableHeight float64) bool {
	return m.Height <= availableHeight
}

// CanStart reports whether the block can start in the available height.
func (m BlockMeasurement) CanStart(availableHeight float64) bool {
	if m.MinHeight <= 0 {
		return m.Fits(availableHeight)
	}
	return m.MinHeight <= availableHeight
}

// RequiredStartHeight reports the vertical space needed to start this block.
// When next is supplied, KeepWithNext includes enough space to start the next
// block as well. It intentionally does not require the whole next block unless
// that block is itself kept together.
func (m BlockMeasurement) RequiredStartHeight(next *BlockMeasurement) float64 {
	required := m.MinHeight
	if required <= 0 || m.KeepTogether {
		required = m.Height
	}
	if m.KeepWithNext && next != nil {
		if withNext := m.Height + next.RequiredStartHeight(nil); withNext > required {
			required = withNext
		}
	}
	return required
}

// ShouldMoveToNextPage reports whether the block should move before drawing.
func (m BlockMeasurement) ShouldMoveToNextPage(availableHeight float64) bool {
	if m.BreakBefore {
		return true
	}
	return m.RequiredStartHeight(nil) > availableHeight
}

// MeasureBlocks estimates a sequence of blocks.
func MeasureBlocks(ctx MeasureContext, blocks []Block) []BlockMeasurement {
	blocks = NormalizeBlocks(blocks)
	measures := make([]BlockMeasurement, 0, len(blocks))
	for _, block := range blocks {
		measures = append(measures, MeasureBlock(ctx, block))
	}
	return measures
}

// MeasureBlock estimates the layout footprint for one block.
func MeasureBlock(ctx MeasureContext, block Block) BlockMeasurement {
	block, ok := NormalizeBlock(block)
	if !ok {
		return BlockMeasurement{}
	}
	switch b := block.(type) {
	case ParagraphBlock:
		return measureParagraphBlock(ctx, b)
	case HeadingBlock:
		return measureHeadingBlock(ctx, b)
	case ListBlock:
		return measureListBlock(ctx, b)
	case TableBlock:
		return measureTableBlock(ctx, b)
	case ImageBlock:
		return measureImageBlock(ctx, b)
	case SignatureRowBlock:
		return measureSignatureRowBlock(ctx, b)
	case MetadataGridBlock:
		return measureMetadataGridBlock(ctx, b)
	case QRVerificationBlock:
		return measureQRVerificationBlock(ctx, b)
	case NoteBoxBlock:
		return measureNoteBoxBlock(ctx, b)
	case SectionBlock:
		return measureSectionBlock(ctx, b)
	case ClauseBlock:
		return measureClauseBlock(ctx, b)
	case PageBreakBlock:
		return BlockMeasurement{Kind: b.DocumentBlockKind(), BreakBefore: b.Before, BreakAfter: b.After}
	default:
		return BlockMeasurement{Kind: block.DocumentBlockKind(), Width: ctx.Width}
	}
}

func measureNoteBoxBlock(ctx MeasureContext, block NoteBoxBlock) BlockMeasurement {
	children := make([]Block, 0, len(block.Body)+1)
	if strings.TrimSpace(block.Title) != "" {
		children = append(children, HeadingBlock{
			Level:    4,
			Segments: []TextSegment{{Text: block.Title}},
			Style:    block.EffectiveStyle(),
		})
	}
	children = append(children, block.Body...)
	return measureContainerBlock(ctx, block.DocumentBlockKind(), children, block.EffectiveBox())
}

func measureParagraphBlock(ctx MeasureContext, block ParagraphBlock) BlockMeasurement {
	box := ParagraphBox(block.EffectiveBox())
	style := MergedTextStyle(ctx.DefaultStyle, block.EffectiveStyle())
	contentWidth := InnerWidth(ctx.Width, box)
	height := measureTextSegments(ctx, block.Segments, style, contentWidth)
	height += VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	lineHeight := ResolvedLineHeight(style)
	if height < lineHeight {
		height = lineHeight
	}
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    lineHeight + box.Padding.Top + box.Padding.Bottom + BorderVertical(box.Border),
		Splittable:   !box.KeepTogether,
		KeepTogether: box.KeepTogether,
		KeepWithNext: box.KeepWithNext,
	}
}

func measureHeadingBlock(ctx MeasureContext, block HeadingBlock) BlockMeasurement {
	box := HeadingBox(block.EffectiveBox())
	blockStyle := block.EffectiveStyle()
	style := MergedTextStyle(ctx.DefaultStyle, blockStyle)
	if blockStyle.FontSize <= 0 {
		style.FontSize = HeadingFontSize(ctx.DefaultStyle.FontSize, block.Level)
	}
	if blockStyle.LineHeight <= 0 {
		style.LineHeight = scaledLineHeight(ctx.DefaultStyle, style.FontSize)
	}
	height := measureTextSegments(ctx, block.Segments, style, InnerWidth(ctx.Width, box))
	height += ResolvedLineHeight(style) * 0.25
	height += VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	minHeight := ResolvedLineHeight(style) + box.Padding.Top + box.Padding.Bottom + BorderVertical(box.Border)
	return BlockMeasurement{
		Kind:          block.DocumentBlockKind(),
		Width:         ctx.Width,
		Height:        height,
		MinHeight:     minHeight,
		Splittable:    false,
		KeepTogether:  true,
		KeepWithNext:  true,
		ChildMeasures: nil,
	}
}

func measureListBlock(ctx MeasureContext, block ListBlock) BlockMeasurement {
	box := block.EffectiveBox()
	style := block.EffectiveStyle()
	childCtx := ctx
	childCtx.Width = InnerWidth(ctx.Width, box)
	measures := make([]BlockMeasurement, 0, len(block.Items))
	total := VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	minHeight := 0.0
	for _, item := range block.Items {
		itemMeasure := measureBlockSequence(childCtx, item.Blocks)
		if itemMeasure.Height <= 0 {
			itemMeasure.Height = ResolvedLineHeight(MergedTextStyle(ctx.DefaultStyle, style))
			itemMeasure.MinHeight = itemMeasure.Height
		}
		measures = append(measures, itemMeasure)
		total += itemMeasure.Height
		if minHeight == 0 {
			minHeight = itemMeasure.MinHeight
		}
	}
	return BlockMeasurement{
		Kind:          block.DocumentBlockKind(),
		Width:         ctx.Width,
		Height:        total,
		MinHeight:     minHeight + box.Padding.Top + BorderVertical(box.Border),
		Splittable:    !box.KeepTogether,
		KeepTogether:  box.KeepTogether,
		KeepWithNext:  box.KeepWithNext,
		ChildMeasures: measures,
	}
}

func measureTableBlock(ctx MeasureContext, block TableBlock) BlockMeasurement {
	box := block.EffectiveBox()
	rowCount := len(block.Header) + len(block.Body) + len(block.Footer)
	measures := make([]BlockMeasurement, 0, rowCount)
	total := VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	if block.Caption != "" {
		total += ResolvedLineHeight(ctx.DefaultStyle)
	}
	minHeight := 0.0
	for _, row := range block.Header {
		rowMeasure := measureTableRow(ctx, row, block)
		measures = append(measures, rowMeasure)
		total += rowMeasure.Height
		minHeight += rowMeasure.Height
	}
	for i, row := range block.Body {
		rowMeasure := measureTableRow(ctx, row, block)
		measures = append(measures, rowMeasure)
		total += rowMeasure.Height
		if minHeight == 0 || i == 0 {
			minHeight += rowMeasure.Height
		}
	}
	for _, row := range block.Footer {
		rowMeasure := measureTableRow(ctx, row, block)
		measures = append(measures, rowMeasure)
		total += rowMeasure.Height
	}
	return BlockMeasurement{
		Kind:          block.DocumentBlockKind(),
		Width:         ctx.Width,
		Height:        total,
		MinHeight:     minHeight + box.Padding.Top + BorderVertical(box.Border),
		Splittable:    !block.Style.KeepRows && !box.KeepTogether,
		KeepTogether:  block.Style.KeepRows || box.KeepTogether,
		KeepWithNext:  box.KeepWithNext,
		ChildMeasures: measures,
	}
}

func measureImageBlock(ctx MeasureContext, block ImageBlock) BlockMeasurement {
	box := block.EffectiveBox()
	width := FirstPositive(block.Width, block.MaxWidth, ctx.Width)
	if width > ctx.Width && ctx.Width > 0 {
		width = ctx.Width
	}
	height := block.Height
	if height <= 0 {
		height = block.MaxHeight
	}
	if height <= 0 {
		height = width * 0.75
	}
	if block.MaxHeight > 0 && height > block.MaxHeight {
		height = block.MaxHeight
	}
	if len(block.Caption) > 0 {
		height += ResolvedLineHeight(ctx.DefaultStyle)
	}
	height += VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    height,
		Splittable:   false,
		KeepTogether: true,
		KeepWithNext: box.KeepWithNext,
	}
}

func measureSignatureRowBlock(ctx MeasureContext, block SignatureRowBlock) BlockMeasurement {
	box := block.EffectiveBox()
	lineHeight := ResolvedLineHeight(ctx.DefaultStyle)
	height := lineHeight*3 + VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    height,
		Splittable:   false,
		KeepTogether: true,
		KeepWithNext: box.KeepWithNext,
	}
}

func measureMetadataGridBlock(ctx MeasureContext, block MetadataGridBlock) BlockMeasurement {
	box := block.EffectiveBox()
	style := block.EffectiveStyle()
	columns := block.Columns
	if columns <= 0 {
		columns = 2
	}
	rows := (len(block.Fields) + columns - 1) / columns
	lineHeight := ResolvedLineHeight(MergedTextStyle(ctx.DefaultStyle, style))
	height := float64(rows)*lineHeight + VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    lineHeight + box.Padding.Top + BorderVertical(box.Border),
		Splittable:   !box.KeepTogether,
		KeepTogether: box.KeepTogether,
		KeepWithNext: box.KeepWithNext,
	}
}

func measureQRVerificationBlock(ctx MeasureContext, block QRVerificationBlock) BlockMeasurement {
	box := block.EffectiveBox()
	style := block.EffectiveStyle()
	qrSize := block.QR.Size
	if qrSize <= 0 {
		qrSize = 25
	}
	textWidth := ctx.Width - qrSize
	if strings.EqualFold(block.QR.Align, "center") || strings.EqualFold(block.QR.Align, "c") {
		textWidth = ctx.Width
	}
	textHeight := measureTextSegments(ctx, block.Text, MergedTextStyle(ctx.DefaultStyle, style), textWidth)
	contentHeight := measureMaxFloat(qrSize, textHeight)
	if strings.EqualFold(block.QR.Align, "center") || strings.EqualFold(block.QR.Align, "c") {
		contentHeight = qrSize + 2 + textHeight
	}
	height := contentHeight + VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    height,
		Splittable:   false,
		KeepTogether: true,
		KeepWithNext: box.KeepWithNext,
	}
}

func measureSectionBlock(ctx MeasureContext, block SectionBlock) BlockMeasurement {
	box := block.EffectiveBox()
	body := measureContainerBlock(ctx, block.DocumentBlockKind(), block.Blocks, box)
	var title BlockMeasurement
	if strings.TrimSpace(block.Title) != "" {
		title = measureHeadingBlock(ctx, HeadingBlock{
			Level:    2,
			Segments: []TextSegment{{Text: block.Title}},
		})
	}

	measure := body
	measure.Height += title.Height
	if title.Height > 0 {
		measure.MinHeight = title.RequiredStartHeight(nil)
		if block.KeepTitleWithBody && len(body.ChildMeasures) > 0 {
			measure.MinHeight = title.Height + body.RequiredStartHeight(nil)
		}
	}
	measure.KeepTogether = box.KeepTogether
	if measure.KeepTogether {
		measure.MinHeight = measure.Height
	}
	return measure
}

func measureClauseBlock(ctx MeasureContext, block ClauseBlock) BlockMeasurement {
	box := block.EffectiveBox()
	measure := measureContainerBlock(ctx, block.DocumentBlockKind(), block.Blocks, box)
	titleText := strings.TrimSpace(strings.TrimSpace(block.Number + " " + block.Title))
	if titleText != "" {
		title := measureHeadingBlock(ctx, HeadingBlock{
			Level:    3,
			Segments: []TextSegment{{Text: titleText}},
		})
		measure.Height += title.Height
		measure.MinHeight = title.Height + measure.RequiredStartHeight(nil)
	}
	measure.BreakBefore = block.BreakBefore
	measure.BreakAfter = block.BreakAfter
	measure.KeepTogether = block.KeepTogether || box.KeepTogether
	measure.Splittable = !measure.KeepTogether
	if measure.KeepTogether {
		measure.MinHeight = measure.Height
	}
	return measure
}

func measureContainerBlock(ctx MeasureContext, kind BlockKind, blocks []Block, box BoxStyle) BlockMeasurement {
	childCtx := ctx
	childCtx.Width = InnerWidth(ctx.Width, box)
	measure := measureBlockSequence(childCtx, blocks)
	measure.Kind = kind
	measure.Width = ctx.Width
	measure.Height += VerticalSpacing(box.Margin) + VerticalSpacing(box.Padding) + BorderVertical(box.Border)
	measure.MinHeight += box.Padding.Top + BorderVertical(box.Border)
	measure.KeepTogether = box.KeepTogether
	measure.KeepWithNext = box.KeepWithNext
	measure.Splittable = !box.KeepTogether
	return measure
}

func measureBlockSequence(ctx MeasureContext, blocks []Block) BlockMeasurement {
	children := MeasureBlocks(ctx, blocks)
	total := 0.0
	minHeight := 0.0
	for i, child := range children {
		total += child.Height
		if i == 0 {
			if len(children) > 1 {
				minHeight = child.RequiredStartHeight(&children[1])
			} else {
				minHeight = child.RequiredStartHeight(nil)
			}
		}
	}
	return BlockMeasurement{
		Width:         ctx.Width,
		Height:        total,
		MinHeight:     minHeight,
		Splittable:    len(children) > 1,
		ChildMeasures: children,
	}
}

func measureTableRow(ctx MeasureContext, row TableRow, table TableBlock) BlockMeasurement {
	columnCount := measureMaxInt(measureTableRowColumnCount(row), len(table.Columns))
	if columnCount <= 0 {
		columnCount = 1
	}
	tableBox := table.EffectiveBox()
	widths := measureTableColumnWidths(InnerWidth(ctx.Width, tableBox), columnCount, table.Columns)
	maxHeight := 0.0
	col := 0
	for _, cell := range row.Cells {
		if col >= len(widths) {
			break
		}
		span := cell.ColSpan
		if span <= 0 {
			span = 1
		}
		cellWidth := sumMeasureFloat64(widths[col:measureMinInt(col+span, len(widths))])
		childCtx := ctx
		cellBox := tableCellBox(cell.EffectiveBox(), ctx.CellPadding)
		cellStyle := cell.EffectiveStyle()
		childCtx.Width = cellWidth - horizontalSpacing(cellBox.Padding) - borderHorizontal(cellBox.Border)
		cellMeasure := measureBlockSequence(childCtx, cell.Blocks)
		if cellMeasure.Height <= 0 {
			cellMeasure.Height = ResolvedLineHeight(MergedTextStyle(ctx.DefaultStyle, cellStyle))
		}
		cellMeasure.Height += VerticalSpacing(cellBox.Padding) + BorderVertical(cellBox.Border)
		maxHeight = measureMaxFloat(maxHeight, cellMeasure.Height)
		col += span
	}
	if maxHeight <= 0 {
		maxHeight = ResolvedLineHeight(ctx.DefaultStyle)
	}
	return BlockMeasurement{
		Kind:         BlockKindTable,
		Width:        ctx.Width,
		Height:       maxHeight,
		MinHeight:    maxHeight,
		Splittable:   false,
		KeepTogether: true,
	}
}

func measureTableRowColumnCount(row TableRow) int {
	count := 0
	for _, cell := range row.Cells {
		span := cell.ColSpan
		if span <= 0 {
			span = 1
		}
		count += span
	}
	return count
}

func measureTextSegments(ctx MeasureContext, segments []TextSegment, style TextStyle, width float64) float64 {
	lineHeight := ResolvedLineHeight(style)
	for _, segment := range segments {
		segmentHeight := ResolvedLineHeight(MergedTextStyle(style, segment.EffectiveStyle()))
		if segmentHeight > lineHeight {
			lineHeight = segmentHeight
		}
	}
	text := TextSegmentsPlainText(segments)
	if text == "" {
		return lineHeight
	}
	if width <= 0 {
		return lineHeight * float64(strings.Count(text, "\n")+1)
	}
	if ctx.TextMeasurer == nil {
		return lineHeight * float64(estimatedTextLineCount(text, style, width))
	}
	lineCount := ctx.TextMeasurer.TextLineCount(text, style, width)
	if lineCount <= 0 {
		return lineHeight
	}
	return float64(lineCount) * lineHeight
}

// TextSegmentsPlainText joins styled text segments into plain text.
func TextSegmentsPlainText(segments []TextSegment) string {
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(segment.Text)
	}
	return builder.String()
}

// MergedTextStyle overlays override onto base.
func MergedTextStyle(base, override TextStyle) TextStyle {
	if override.FontFamily != "" {
		base.FontFamily = override.FontFamily
	}
	if override.FontSize > 0 {
		if override.LineHeight <= 0 {
			base.LineHeight = scaledLineHeight(base, override.FontSize)
		}
		base.FontSize = override.FontSize
	}
	if override.LineHeight > 0 {
		base.LineHeight = override.LineHeight
	}
	base.Bold = base.Bold || override.Bold
	base.Italic = base.Italic || override.Italic
	base.Underline = base.Underline || override.Underline
	base.StrikeThrough = base.StrikeThrough || override.StrikeThrough
	if override.Color.Set {
		base.Color = override.Color
	}
	if override.Align != "" {
		base.Align = override.Align
	}
	return base
}

// ResolvedLineHeight returns the concrete line height for style.
func ResolvedLineHeight(style TextStyle) float64 {
	if style.LineHeight > 0 {
		return style.LineHeight
	}
	if style.FontSize > 0 {
		return style.FontSize * 1.2
	}
	return 5
}

func scaledLineHeight(base TextStyle, fontSize float64) float64 {
	if fontSize <= 0 {
		return ResolvedLineHeight(base)
	}
	if base.LineHeight > 0 && base.FontSize > 0 {
		return base.LineHeight * fontSize / base.FontSize
	}
	return fontSize * 1.2
}

// HeadingFontSize returns the default heading size for level.
func HeadingFontSize(base float64, level int) float64 {
	if base <= 0 {
		base = 12
	}
	switch level {
	case 1:
		return base * 1.8
	case 2:
		return base * 1.5
	case 3:
		return base * 1.25
	default:
		return base * 1.1
	}
}

// ParagraphBox returns paragraph box defaults applied to box.
func ParagraphBox(box BoxStyle) BoxStyle {
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = paragraphSpacing
	}
	return box
}

// HeadingBox returns heading box defaults applied to box.
func HeadingBox(box BoxStyle) BoxStyle {
	if box.Margin.Top == 0 {
		box.Margin.Top = headingTopSpace
	}
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = headingBotSpace
	}
	return box
}

// InnerWidth returns the content width inside padding and borders.
func InnerWidth(width float64, box BoxStyle) float64 {
	inner := width - horizontalSpacing(box.Padding) - borderHorizontal(box.Border)
	if inner < 0 {
		return 0
	}
	return inner
}

// VerticalSpacing returns top plus bottom spacing.
func VerticalSpacing(spacing Spacing) float64 {
	return spacing.Top + spacing.Bottom
}

func horizontalSpacing(spacing Spacing) float64 {
	return spacing.Left + spacing.Right
}

// BorderVertical returns top plus bottom border width.
func BorderVertical(border BorderStyle) float64 {
	return border.Top.Width + border.Bottom.Width
}

func borderHorizontal(border BorderStyle) float64 {
	return border.Left.Width + border.Right.Width
}

// FirstPositive returns the first positive value.
func FirstPositive(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func measureMaxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func measureMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func measureMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func measureTableColumnWidths(total float64, count int, columns []TableColumn) []float64 {
	constraints := make([]layoutgeom.TrackConstraint, len(columns))
	for i, column := range columns {
		constraints[i] = layoutgeom.TrackConstraint{Preferred: column.Width, Min: column.MinWidth, Max: column.MaxWidth}
	}
	return layoutgeom.ResolveTracks(total, count, constraints)
}

func tableCellBox(box BoxStyle, fallback float64) BoxStyle {
	if box.Padding == (Spacing{}) && fallback > 0 {
		box.Padding = Spacing{Top: fallback, Right: fallback, Bottom: fallback, Left: fallback}
	}
	return box
}

func sumMeasureFloat64(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func estimatedTextLineCount(text string, style TextStyle, width float64) int {
	if width <= 0 {
		return strings.Count(text, "\n") + 1
	}
	lines := 1
	lineWidth := 0.0
	for _, char := range strings.TrimRight(text, "\n") {
		if char == '\n' {
			lines++
			lineWidth = 0
			continue
		}
		charWidth := estimatedRuneWidth(char, style)
		if lineWidth > 0 && lineWidth+charWidth > width {
			lines++
			lineWidth = 0
		}
		lineWidth += charWidth
	}
	return lines
}

func estimatedRuneWidth(char rune, style TextStyle) float64 {
	if unicode.IsControl(char) {
		return 0
	}
	fontSize := style.FontSize
	if fontSize <= 0 {
		fontSize = 12
	}
	em := fontSize * 0.3527777778
	factor := 0.5
	family := strings.ToLower(style.FontFamily)
	switch {
	case strings.Contains(family, "courier") || strings.Contains(family, "mono"):
		factor = 0.6
	case strings.Contains(family, "times") || strings.Contains(family, "serif"):
		factor = 0.47
	}
	switch {
	case unicode.Is(unicode.Mn, char) || unicode.Is(unicode.Me, char):
		factor = 0
	case char == '\t':
		factor = 1.2
	case unicode.IsSpace(char):
		factor = 0.28
	case isWideRune(char):
		factor = 0.95
	case strings.ContainsRune(".,;:!|'`iIl1[](){}", char):
		factor *= 0.55
	case strings.ContainsRune("mwMW@#%&", char):
		factor *= 1.35
	}
	if style.Bold && factor > 0 {
		factor *= 1.04
	}
	return em * factor
}

func isWideRune(char rune) bool {
	return (char >= 0x1100 && char <= 0x11FF) ||
		(char >= 0x2E80 && char <= 0xA4CF) ||
		(char >= 0xAC00 && char <= 0xD7AF) ||
		(char >= 0xF900 && char <= 0xFAFF) ||
		(char >= 0xFE10 && char <= 0xFE6F) ||
		(char >= 0xFF00 && char <= 0xFF60) ||
		(char >= 0xFFE0 && char <= 0xFFE6) ||
		(char >= 0x1F300 && char <= 0x1FAFF)
}
