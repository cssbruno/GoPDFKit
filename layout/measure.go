// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layout

import (
	"strings"
	"unicode"
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

// ShouldMoveToNextPage reports whether the block should move before drawing.
func (m BlockMeasurement) ShouldMoveToNextPage(availableHeight float64) bool {
	if m.BreakBefore {
		return true
	}
	if m.KeepTogether {
		return !m.Fits(availableHeight)
	}
	return !m.CanStart(availableHeight)
}

// MeasureBlocks estimates a sequence of blocks.
func MeasureBlocks(ctx MeasureContext, blocks []Block) []BlockMeasurement {
	measures := make([]BlockMeasurement, 0, len(blocks))
	for _, block := range blocks {
		if block == nil {
			continue
		}
		measures = append(measures, MeasureBlock(ctx, block))
	}
	return measures
}

// MeasureBlock estimates the layout footprint for one block.
func MeasureBlock(ctx MeasureContext, block Block) BlockMeasurement {
	if block == nil {
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
		return measureContainerBlock(ctx, b.DocumentBlockKind(), b.Body, b.Box)
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

func measureParagraphBlock(ctx MeasureContext, block ParagraphBlock) BlockMeasurement {
	block.Box = ParagraphBox(block.Box)
	style := MergedTextStyle(ctx.DefaultStyle, block.Style)
	contentWidth := InnerWidth(ctx.Width, block.Box)
	height := measureTextSegments(ctx, block.Segments, style, contentWidth)
	height += VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	lineHeight := ResolvedLineHeight(style)
	if height < lineHeight {
		height = lineHeight
	}
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    lineHeight + block.Box.Padding.Top + block.Box.Padding.Bottom + BorderVertical(block.Box.Border),
		Splittable:   !block.Box.KeepTogether,
		KeepTogether: block.Box.KeepTogether,
		KeepWithNext: block.Box.KeepWithNext,
	}
}

func measureHeadingBlock(ctx MeasureContext, block HeadingBlock) BlockMeasurement {
	block.Box = HeadingBox(block.Box)
	style := MergedTextStyle(ctx.DefaultStyle, block.Style)
	if block.Style.FontSize <= 0 {
		style.FontSize = HeadingFontSize(ctx.DefaultStyle.FontSize, block.Level)
	}
	if block.Style.LineHeight <= 0 {
		style.LineHeight = scaledLineHeight(ctx.DefaultStyle, style.FontSize)
	}
	height := measureTextSegments(ctx, block.Segments, style, InnerWidth(ctx.Width, block.Box))
	height += ResolvedLineHeight(style) * 0.25
	height += VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	minHeight := ResolvedLineHeight(style) + block.Box.Padding.Top + block.Box.Padding.Bottom + BorderVertical(block.Box.Border)
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
	childCtx := ctx
	childCtx.Width = InnerWidth(ctx.Width, block.Box)
	measures := make([]BlockMeasurement, 0, len(block.Items))
	total := VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	minHeight := 0.0
	for _, item := range block.Items {
		itemMeasure := measureBlockSequence(childCtx, item.Blocks)
		if itemMeasure.Height <= 0 {
			itemMeasure.Height = ResolvedLineHeight(MergedTextStyle(ctx.DefaultStyle, block.Style))
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
		MinHeight:     minHeight + block.Box.Padding.Top + BorderVertical(block.Box.Border),
		Splittable:    !block.Box.KeepTogether,
		KeepTogether:  block.Box.KeepTogether,
		KeepWithNext:  block.Box.KeepWithNext,
		ChildMeasures: measures,
	}
}

func measureTableBlock(ctx MeasureContext, block TableBlock) BlockMeasurement {
	rowCount := len(block.Header) + len(block.Body) + len(block.Footer)
	measures := make([]BlockMeasurement, 0, rowCount)
	total := VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
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
		MinHeight:     minHeight + block.Box.Padding.Top + BorderVertical(block.Box.Border),
		Splittable:    !block.Style.KeepRows && !block.Box.KeepTogether,
		KeepTogether:  block.Style.KeepRows || block.Box.KeepTogether,
		KeepWithNext:  block.Box.KeepWithNext,
		ChildMeasures: measures,
	}
}

func measureImageBlock(ctx MeasureContext, block ImageBlock) BlockMeasurement {
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
	height += VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    height,
		Splittable:   false,
		KeepTogether: true,
		KeepWithNext: block.Box.KeepWithNext,
	}
}

func measureSignatureRowBlock(ctx MeasureContext, block SignatureRowBlock) BlockMeasurement {
	lineHeight := ResolvedLineHeight(ctx.DefaultStyle)
	height := lineHeight*3 + VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    height,
		Splittable:   false,
		KeepTogether: true,
		KeepWithNext: block.Box.KeepWithNext,
	}
}

func measureMetadataGridBlock(ctx MeasureContext, block MetadataGridBlock) BlockMeasurement {
	columns := block.Columns
	if columns <= 0 {
		columns = 2
	}
	rows := (len(block.Fields) + columns - 1) / columns
	lineHeight := ResolvedLineHeight(MergedTextStyle(ctx.DefaultStyle, block.Style))
	height := float64(rows)*lineHeight + VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    lineHeight + block.Box.Padding.Top + BorderVertical(block.Box.Border),
		Splittable:   !block.Box.KeepTogether,
		KeepTogether: block.Box.KeepTogether,
		KeepWithNext: block.Box.KeepWithNext,
	}
}

func measureQRVerificationBlock(ctx MeasureContext, block QRVerificationBlock) BlockMeasurement {
	qrSize := block.QR.Size
	if qrSize <= 0 {
		qrSize = 25
	}
	textHeight := measureTextSegments(ctx, block.Text, MergedTextStyle(ctx.DefaultStyle, block.Style), ctx.Width-qrSize)
	height := measureMaxFloat(qrSize, textHeight) + VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    height,
		Splittable:   false,
		KeepTogether: true,
		KeepWithNext: block.Box.KeepWithNext,
	}
}

func measureSectionBlock(ctx MeasureContext, block SectionBlock) BlockMeasurement {
	childCtx := ctx
	childCtx.Width = InnerWidth(ctx.Width, block.Box)
	titleHeight := 0.0
	if strings.TrimSpace(block.Title) != "" {
		titleHeight = ResolvedLineHeight(ctx.DefaultStyle)
	}
	measure := measureBlockSequence(childCtx, block.Blocks)
	measure.Kind = block.DocumentBlockKind()
	measure.Width = ctx.Width
	measure.Height += titleHeight + VerticalSpacing(block.Box.Margin) + VerticalSpacing(block.Box.Padding) + BorderVertical(block.Box.Border)
	measure.MinHeight += titleHeight + block.Box.Padding.Top + BorderVertical(block.Box.Border)
	measure.KeepTogether = block.Box.KeepTogether
	measure.KeepWithNext = block.Box.KeepWithNext
	measure.Splittable = !block.Box.KeepTogether
	return measure
}

func measureClauseBlock(ctx MeasureContext, block ClauseBlock) BlockMeasurement {
	measure := measureContainerBlock(ctx, block.DocumentBlockKind(), block.Blocks, block.Box)
	measure.BreakBefore = block.BreakBefore
	measure.BreakAfter = block.BreakAfter
	measure.KeepTogether = block.KeepTogether || block.Box.KeepTogether
	measure.Splittable = !measure.KeepTogether
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
			minHeight = child.MinHeight
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
	widths := measureTableColumnWidths(InnerWidth(ctx.Width, table.Box), columnCount, table.Columns)
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
		childCtx.Width = cellWidth - horizontalSpacing(cell.Box.Padding) - borderHorizontal(cell.Box.Border)
		cellMeasure := measureBlockSequence(childCtx, cell.Blocks)
		if cellMeasure.Height <= 0 {
			cellMeasure.Height = ResolvedLineHeight(MergedTextStyle(ctx.DefaultStyle, cell.Style))
		}
		cellMeasure.Height += VerticalSpacing(cell.Box.Padding) + BorderVertical(cell.Box.Border)
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
	if count <= 0 {
		return nil
	}
	widths := make([]float64, count)
	fixed := 0.0
	for i := 0; i < count && i < len(columns); i++ {
		if columns[i].Width > 0 {
			widths[i] = columns[i].Width
			fixed += widths[i]
		}
	}
	remainingCount := 0
	for _, width := range widths {
		if width <= 0 {
			remainingCount++
		}
	}
	fill := 0.0
	if remainingCount > 0 {
		fill = (total - fixed) / float64(remainingCount)
	}
	if fill <= 0 {
		fill = total / float64(count)
	}
	for i := range widths {
		if widths[i] <= 0 {
			widths[i] = fill
		}
	}
	return widths
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
