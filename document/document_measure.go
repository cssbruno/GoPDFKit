// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import "strings"

// MeasureContext contains the renderer state needed to estimate block layout.
type MeasureContext struct {
	PDF          *Document // PDF state used for font and unit measurements.
	Width        float64   // Available content width.
	DefaultStyle TextStyle // Default text style for measurement.
	CellPadding  float64   // Default table cell padding.
}

// NewMeasureContext creates a measurement context for the given content width.
func NewMeasureContext(pdf *Document, width float64) MeasureContext {
	fontSize := 12.0
	lineHeight := 5.0
	fontFamily := "Helvetica"
	if pdf != nil {
		fontFamily = pdf.fontFamily
		if fontFamily == "" {
			fontFamily = "Helvetica"
		}
		ptSize, unitSize := pdf.GetFontSize()
		if ptSize > 0 && isFiniteFloat(ptSize) {
			fontSize = ptSize
		}
		if unitSize > 0 && isFiniteFloat(unitSize) {
			lineHeight = unitSize * 1.2
		}
	}
	if width < 0 {
		width = 0
	}
	return MeasureContext{
		PDF:   pdf,
		Width: width,
		DefaultStyle: TextStyle{
			FontFamily: fontFamily,
			FontSize:   fontSize,
			LineHeight: lineHeight,
		},
		CellPadding: 1,
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
	block.Box = documentParagraphBox(block.Box)
	style := mergedTextStyle(ctx.DefaultStyle, block.Style)
	contentWidth := innerWidth(ctx.Width, block.Box)
	height := measureTextSegments(ctx, block.Segments, style, contentWidth)
	height += verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
	lineHeight := resolvedLineHeight(style)
	if height < lineHeight {
		height = lineHeight
	}
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    lineHeight + block.Box.Padding.Top + block.Box.Padding.Bottom + borderVertical(block.Box.Border),
		Splittable:   !block.Box.KeepTogether,
		KeepTogether: block.Box.KeepTogether,
		KeepWithNext: block.Box.KeepWithNext,
	}
}

func measureHeadingBlock(ctx MeasureContext, block HeadingBlock) BlockMeasurement {
	block.Box = documentHeadingBox(block.Box)
	style := mergedTextStyle(ctx.DefaultStyle, block.Style)
	if style.FontSize <= 0 {
		style.FontSize = documentHeadingFontSize(ctx.DefaultStyle.FontSize, block.Level)
	}
	if style.LineHeight <= 0 {
		style.LineHeight = style.FontSize * 1.25
	}
	height := measureTextSegments(ctx, block.Segments, style, innerWidth(ctx.Width, block.Box))
	height += verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
	minHeight := resolvedLineHeight(style) + block.Box.Padding.Top + block.Box.Padding.Bottom + borderVertical(block.Box.Border)
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
	childCtx.Width = innerWidth(ctx.Width, block.Box)
	measures := make([]BlockMeasurement, 0, len(block.Items))
	total := verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
	minHeight := 0.0
	for _, item := range block.Items {
		itemMeasure := measureBlockSequence(childCtx, item.Blocks)
		if itemMeasure.Height <= 0 {
			itemMeasure.Height = resolvedLineHeight(mergedTextStyle(ctx.DefaultStyle, block.Style))
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
		MinHeight:     minHeight + block.Box.Padding.Top + borderVertical(block.Box.Border),
		Splittable:    !block.Box.KeepTogether,
		KeepTogether:  block.Box.KeepTogether,
		KeepWithNext:  block.Box.KeepWithNext,
		ChildMeasures: measures,
	}
}

func measureTableBlock(ctx MeasureContext, block TableBlock) BlockMeasurement {
	rowCount := len(block.Header) + len(block.Body) + len(block.Footer)
	measures := make([]BlockMeasurement, 0, rowCount)
	total := verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
	if block.Caption != "" {
		total += resolvedLineHeight(ctx.DefaultStyle)
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
		MinHeight:     minHeight + block.Box.Padding.Top + borderVertical(block.Box.Border),
		Splittable:    !block.Style.KeepRows && !block.Box.KeepTogether,
		KeepTogether:  block.Style.KeepRows || block.Box.KeepTogether,
		KeepWithNext:  block.Box.KeepWithNext,
		ChildMeasures: measures,
	}
}

func measureImageBlock(ctx MeasureContext, block ImageBlock) BlockMeasurement {
	width := firstPositive(block.Width, block.MaxWidth, ctx.Width)
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
		height += resolvedLineHeight(ctx.DefaultStyle)
	}
	height += verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
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
	lineHeight := resolvedLineHeight(ctx.DefaultStyle)
	height := lineHeight*3 + verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
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
	lineHeight := resolvedLineHeight(mergedTextStyle(ctx.DefaultStyle, block.Style))
	height := float64(rows)*lineHeight + verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
	return BlockMeasurement{
		Kind:         block.DocumentBlockKind(),
		Width:        ctx.Width,
		Height:       height,
		MinHeight:    lineHeight + block.Box.Padding.Top + borderVertical(block.Box.Border),
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
	textHeight := measureTextSegments(ctx, block.Text, mergedTextStyle(ctx.DefaultStyle, block.Style), ctx.Width-qrSize)
	height := measureMaxFloat(qrSize, textHeight) + verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
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
	childCtx.Width = innerWidth(ctx.Width, block.Box)
	titleHeight := 0.0
	if strings.TrimSpace(block.Title) != "" {
		titleHeight = resolvedLineHeight(ctx.DefaultStyle)
	}
	measure := measureBlockSequence(childCtx, block.Blocks)
	measure.Kind = block.DocumentBlockKind()
	measure.Width = ctx.Width
	measure.Height += titleHeight + verticalSpacing(block.Box.Margin) + verticalSpacing(block.Box.Padding) + borderVertical(block.Box.Border)
	measure.MinHeight += titleHeight + block.Box.Padding.Top + borderVertical(block.Box.Border)
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
	childCtx.Width = innerWidth(ctx.Width, box)
	measure := measureBlockSequence(childCtx, blocks)
	measure.Kind = kind
	measure.Width = ctx.Width
	measure.Height += verticalSpacing(box.Margin) + verticalSpacing(box.Padding) + borderVertical(box.Border)
	measure.MinHeight += box.Padding.Top + borderVertical(box.Border)
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
	columnCount := measureMaxInt(len(row.Cells), len(table.Columns))
	if columnCount <= 0 {
		columnCount = 1
	}
	cellWidth := innerWidth(ctx.Width, table.Box) / float64(columnCount)
	maxHeight := 0.0
	for _, cell := range row.Cells {
		span := cell.ColSpan
		if span <= 0 {
			span = 1
		}
		childCtx := ctx
		childCtx.Width = cellWidth*float64(span) - horizontalSpacing(cell.Box.Padding) - borderHorizontal(cell.Box.Border)
		cellMeasure := measureBlockSequence(childCtx, cell.Blocks)
		if cellMeasure.Height <= 0 {
			cellMeasure.Height = resolvedLineHeight(mergedTextStyle(ctx.DefaultStyle, cell.Style))
		}
		cellMeasure.Height += verticalSpacing(cell.Box.Padding) + borderVertical(cell.Box.Border)
		maxHeight = measureMaxFloat(maxHeight, cellMeasure.Height)
	}
	if maxHeight <= 0 {
		maxHeight = resolvedLineHeight(ctx.DefaultStyle)
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

func measureTextSegments(ctx MeasureContext, segments []TextSegment, style TextStyle, width float64) float64 {
	lineHeight := resolvedLineHeight(style)
	text := textSegmentsPlainText(segments)
	if text == "" {
		return lineHeight
	}
	if width <= 0 || ctx.PDF == nil {
		return lineHeight * float64(strings.Count(text, "\n")+1)
	}
	state := applyPDFTextStyle(ctx.PDF, style)
	defer restorePDFTextStyle(ctx.PDF, state)
	lines := ctx.PDF.SplitText(text, width)
	if len(lines) == 0 {
		return lineHeight
	}
	return float64(len(lines)) * lineHeight
}

type pdfTextStyleState struct {
	family    string
	style     string
	sizePt    float64
	underline bool
	strikeout bool
}

func applyPDFTextStyle(pdf *Document, style TextStyle) pdfTextStyleState {
	state := pdfTextStyleState{
		family:    pdf.fontFamily,
		style:     pdf.fontStyle,
		sizePt:    pdf.fontSizePt,
		underline: pdf.underline,
		strikeout: pdf.strikeout,
	}
	if style.FontFamily == "" {
		style.FontFamily = state.family
	}
	size := style.FontSize
	if size <= 0 {
		size = state.sizePt
	}
	if size <= 0 {
		size = 12
	}
	style.FontSize = size
	family := firstNonEmpty(style.FontFamily, "Helvetica")
	fontStyle := ""
	if style.Bold {
		fontStyle += "B"
	}
	if style.Italic {
		fontStyle += "I"
	}
	if style.Underline {
		fontStyle += "U"
	}
	if style.StrikeThrough {
		fontStyle += "S"
	}
	pdf.SetFont(family, fontStyle, size)
	pdf.strikeout = style.StrikeThrough
	return state
}

func restorePDFTextStyle(pdf *Document, state pdfTextStyleState) {
	family := state.family
	if family == "" {
		family = "Helvetica"
	}
	size := state.sizePt
	if size <= 0 {
		size = 12
	}
	pdf.SetFont(family, state.style, size)
	pdf.underline = state.underline
	pdf.strikeout = state.strikeout
}

func textSegmentsPlainText(segments []TextSegment) string {
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteString(segment.Text)
	}
	return builder.String()
}

func mergedTextStyle(base, override TextStyle) TextStyle {
	if override.FontFamily != "" {
		base.FontFamily = override.FontFamily
	}
	if override.FontSize > 0 {
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

func resolvedLineHeight(style TextStyle) float64 {
	if style.LineHeight > 0 {
		return style.LineHeight
	}
	if style.FontSize > 0 {
		return style.FontSize * 1.2
	}
	return 5
}

func documentHeadingFontSize(base float64, level int) float64 {
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

func innerWidth(width float64, box BoxStyle) float64 {
	inner := width - horizontalSpacing(box.Padding) - borderHorizontal(box.Border)
	if inner < 0 {
		return 0
	}
	return inner
}

func verticalSpacing(spacing Spacing) float64 {
	return spacing.Top + spacing.Bottom
}

func horizontalSpacing(spacing Spacing) float64 {
	return spacing.Left + spacing.Right
}

func borderVertical(border BorderStyle) float64 {
	return border.Top.Width + border.Bottom.Width
}

func borderHorizontal(border BorderStyle) float64 {
	return border.Left.Width + border.Right.Width
}

func firstPositive(values ...float64) float64 {
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
