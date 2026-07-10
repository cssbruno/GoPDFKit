// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutgeom"
	"github.com/cssbruno/gopdfkit/layout"
)

const (
	documentParagraphSpacing = 2.0
	documentHeadingTopSpace  = 2.5
	documentHeadingBotSpace  = 1.5
)

// WriteDocument renders a shared document model into the PDF.
func (f *Document) WriteDocument(doc *layout.LayoutDocument) {
	if f.err != nil {
		return
	}
	if doc == nil {
		f.SetErrorf("document is nil")
		return
	}
	if doc.Title != "" {
		f.SetTitle(doc.Title, true)
	}
	if doc.Metadata.Subject != "" {
		f.SetSubject(doc.Metadata.Subject, true)
	}
	if doc.Metadata.Author != "" {
		f.SetAuthor(doc.Metadata.Author, true)
	}
	if strings.TrimSpace(doc.Language) != "" {
		f.compliance.Lang = strings.TrimSpace(doc.Language)
	}
	template := doc.PageTemplate
	f.applyPageTemplateMargins(template)
	if len(doc.Attachments) > 0 {
		f.SetAttachments(documentAttachments(doc.Attachments))
	}
	if f.page == 0 {
		f.AddPage()
	}
	if alias := template.PageTotalAlias(); alias != "" {
		f.AliasNbPages(alias)
	}
	renderer := documentRenderer{pdf: f, template: template, renderedFooters: make(map[int]bool)}
	renderer.renderHeader()
	renderer.renderBlocks(doc.Body)
	if doc.Signature != nil {
		f.signatureFieldName = doc.Signature.PAdESFieldName()
		renderer.renderSignature(*doc.Signature)
	}
	if doc.QR != nil {
		renderer.renderBlock(layout.QRVerificationBlock{QR: *doc.QR})
	}
	renderer.renderFooter()
}

type documentRenderer struct {
	pdf             *Document
	template        layout.PageTemplate
	renderedFooters map[int]bool
	renderingShell  bool
	contentWidthVal float64
	contentWidthW   float64
	contentWidthL   float64
	contentWidthR   float64
	contentWidthOK  bool
	scopedWidth     float64
	scopedWidthOK   bool
	measureCtx      layout.MeasureContext
	measureCtxWidth float64
	measureCtxFont  string
	measureCtxPt    float64
	measureCtxUnit  float64
	measureCtxOK    bool
	imageNameCache  map[documentImageCacheKey]string
	scopedStyle     layout.TextStyle
	scopedStyleOK   bool
	boxHeight       float64
	boxHeightOK     bool
}

type documentImageCacheKey struct {
	format string
	hash   [sha256.Size]byte
}

type documentTableRowMeasurement struct {
	height float64
	cells  []documentTableCellMeasurement
	row    layout.TableRow
	header bool
}

type documentTableCellMeasurement struct {
	width         float64
	height        float64
	contentHeight float64
	blockMeasures []layout.BlockMeasurement
	cell          layout.TableCell
	column        int
	colSpan       int
	rowSpan       int
	tagRowSpan    int
}

type documentTableLayout struct {
	widths         []float64
	offsets        []float64
	rowOffsets     []float64
	rows           []documentTableRowMeasurement
	headerRows     int
	repeatHeader   bool
	borderCollapse bool
}

func (r *documentRenderer) contentWidth() float64 {
	if r.scopedWidthOK {
		return r.scopedWidth
	}
	if !r.contentWidthOK || r.contentWidthW != r.pdf.w || r.contentWidthL != r.pdf.lMargin || r.contentWidthR != r.pdf.rMargin {
		r.contentWidthW = r.pdf.w
		r.contentWidthL = r.pdf.lMargin
		r.contentWidthR = r.pdf.rMargin
		r.contentWidthVal = r.pdf.w - r.pdf.lMargin - r.pdf.rMargin
		r.contentWidthOK = true
	}
	return r.contentWidthVal
}

func (r *documentRenderer) withContentWidth(width float64, render func()) {
	if width < 0 {
		width = 0
	}
	oldWidth, oldOK := r.scopedWidth, r.scopedWidthOK
	r.scopedWidth, r.scopedWidthOK = width, true
	defer func() {
		r.scopedWidth, r.scopedWidthOK = oldWidth, oldOK
	}()
	render()
}

func (r *documentRenderer) withTextStyle(style layout.TextStyle, render func()) {
	oldStyle, oldOK := r.scopedStyle, r.scopedStyleOK
	if oldOK {
		style = layout.MergedTextStyle(oldStyle, style)
	}
	r.scopedStyle, r.scopedStyleOK = style, true
	defer func() {
		r.scopedStyle, r.scopedStyleOK = oldStyle, oldOK
	}()
	render()
}

func (r *documentRenderer) measureContext() layout.MeasureContext {
	return r.measureContextForWidth(r.contentWidth())
}

func (r *documentRenderer) measureContextForWidth(width float64) layout.MeasureContext {
	if !r.measureCtxOK || r.measureCtxWidth != width || r.measureCtxFont != r.pdf.fontFamily || r.measureCtxPt != r.pdf.fontSizePt || r.measureCtxUnit != r.pdf.fontSize {
		r.measureCtx = newMeasureContext(r.pdf, width)
		r.measureCtxWidth = width
		r.measureCtxFont = r.pdf.fontFamily
		r.measureCtxPt = r.pdf.fontSizePt
		r.measureCtxUnit = r.pdf.fontSize
		r.measureCtxOK = true
	}
	ctx := r.measureCtx
	if r.scopedStyleOK {
		ctx.DefaultStyle = layout.MergedTextStyle(ctx.DefaultStyle, r.scopedStyle)
	}
	return ctx
}

func (r *documentRenderer) mergedStyle(style layout.TextStyle) layout.TextStyle {
	return layout.MergedTextStyle(r.measureContext().DefaultStyle, style)
}

func (r *documentRenderer) textSegmentsPlainText(segments []layout.TextSegment) string {
	if len(segments) == 0 {
		return ""
	}
	return layout.TextSegmentsPlainText(segments)
}

func (r *documentRenderer) availableHeight() float64 {
	reservedFooter := r.template.FooterReservedHeightForPage(r.pdf.page)
	return r.pdf.h - r.pdf.bMargin - reservedFooter - r.pdf.GetY()
}

func (r *documentRenderer) renderHeader() {
	header := r.template.HeaderForPage(r.pdf.page)
	if header == nil {
		return
	}
	startY := r.pdf.GetY()
	wasRenderingShell := r.renderingShell
	r.renderingShell = true
	defer func() { r.renderingShell = wasRenderingShell }()
	r.renderRepeatedBlocks(header.Blocks)
	if header.Height > 0 && r.pdf.GetY() < startY+header.Height {
		r.pdf.SetY(startY + header.Height)
	}
}

func (r *documentRenderer) renderFooter() {
	if r.pdf.page <= 0 || r.renderedFooters[r.pdf.page] {
		return
	}
	footer := r.template.FooterForPage(r.pdf.page)
	if footer == nil && r.template.PageNumberText(r.pdf.page) == "" {
		return
	}
	r.renderedFooters[r.pdf.page] = true
	_, _, _, bottom := r.pdf.GetMargins()
	y := r.pdf.h - bottom
	if footer != nil && footer.Height > 0 {
		y -= footer.Height
	}
	if y < r.pdf.tMargin {
		y = r.pdf.tMargin
	}
	r.pdf.SetY(y)
	wasRenderingShell := r.renderingShell
	r.renderingShell = true
	defer func() { r.renderingShell = wasRenderingShell }()
	if footer != nil {
		r.renderRepeatedBlocks(footer.Blocks)
	}
	if text := r.template.PageNumberText(r.pdf.page); text != "" {
		r.applyTextStyle(layout.TextStyle{FontFamily: "Helvetica", FontSize: 9})
		r.pdf.CellFormat(r.contentWidth(), 5, text, "", 1, "R", false, 0, "")
	}
}

func (r *documentRenderer) renderBlocks(blocks []layout.Block) {
	for _, block := range blocks {
		r.renderBlock(block)
		if r.pdf.err != nil {
			return
		}
	}
}

func (r *documentRenderer) renderBlocksWithMeasurements(blocks []layout.Block, measurements []layout.BlockMeasurement) {
	for i, block := range blocks {
		if i < len(measurements) {
			r.renderBlockMeasured(block, measurements[i], true)
		} else {
			r.renderBlock(block)
		}
		if r.pdf.err != nil {
			return
		}
	}
}

func (r *documentRenderer) renderRepeatedBlocks(blocks []layout.Block) {
	if len(blocks) == 0 {
		r.renderBlocks(blocks)
		return
	}
	measurements := layout.MeasureBlocks(r.measureContext(), blocks)
	r.renderBlocksWithMeasurements(blocks, measurements)
}

func (r *documentRenderer) renderBlock(block layout.Block) {
	r.renderBlockMeasured(block, layout.BlockMeasurement{}, false)
}

func (r *documentRenderer) renderBlockMeasured(block layout.Block, measure layout.BlockMeasurement, measured bool) {
	if block == nil || r.pdf.err != nil {
		return
	}
	if pageBreak, ok := block.(layout.PageBreakBlock); ok {
		if !r.renderingShell && (pageBreak.Before || pageBreak.After) {
			r.addPageWithTemplate()
		}
		return
	}
	if !measured {
		measure = layout.MeasureBlock(r.measureContext(), block)
	}
	if !r.renderingShell && (measure.BreakBefore || measure.ShouldMoveToNextPage(r.availableHeight())) {
		r.addPageWithTemplate()
	}
	oldBoxHeight, oldBoxHeightOK := r.boxHeight, r.boxHeightOK
	r.boxHeight, r.boxHeightOK = measure.Height, measure.Height > 0
	defer func() {
		r.boxHeight, r.boxHeightOK = oldBoxHeight, oldBoxHeightOK
	}()
	switch b := block.(type) {
	case layout.ParagraphBlock:
		r.renderParagraph(b)
	case layout.HeadingBlock:
		r.renderHeading(b)
	case layout.ListBlock:
		r.renderList(b)
	case layout.TableBlock:
		r.renderTable(b)
	case layout.ImageBlock:
		r.renderImage(b)
	case layout.SignatureRowBlock:
		r.renderSignatureRow(b)
	case layout.MetadataGridBlock:
		r.renderMetadataGrid(b)
	case layout.QRVerificationBlock:
		r.renderQRVerification(b)
	case layout.NoteBoxBlock:
		r.renderBox(b.EffectiveBox(), func() {
			if b.Title != "" {
				r.renderHeading(layout.HeadingBlock{Level: 4, Segments: []layout.TextSegment{{Text: b.Title}}, Style: b.EffectiveStyle()})
			}
			r.renderBlocks(b.Body)
		})
	case layout.SectionBlock:
		if b.Title != "" {
			r.renderHeading(layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: b.Title}}})
		}
		r.renderBox(b.EffectiveBox(), func() { r.renderBlocks(b.Blocks) })
	case layout.ClauseBlock:
		title := strings.TrimSpace(strings.TrimSpace(b.Number + " " + b.Title))
		if title != "" {
			r.renderHeading(layout.HeadingBlock{Level: 3, Segments: []layout.TextSegment{{Text: title}}})
		}
		r.renderBox(b.EffectiveBox(), func() { r.renderBlocks(b.Blocks) })
	default:
		r.pdf.SetErrorf("unsupported document block kind: %s", block.DocumentBlockKind())
	}
	if !r.renderingShell && measure.BreakAfter {
		r.addPageWithTemplate()
	}
}

func (r *documentRenderer) addPageWithTemplate() {
	r.renderFooter()
	if r.pdf.err != nil {
		return
	}
	r.pdf.AddPage()
	r.renderHeader()
}

func (r *documentRenderer) renderParagraph(block layout.ParagraphBlock) {
	box := documentParagraphBox(block.EffectiveBox())
	r.renderBox(box, func() {
		style := r.mergedStyle(block.EffectiveStyle())
		r.pdf.SetNextTextRole(taggedRoleP)
		r.renderTextSegments(block.Segments, style, r.contentWidth())
	})
}

func (r *documentRenderer) renderHeading(block layout.HeadingBlock) {
	box := documentHeadingBox(block.EffectiveBox())
	blockStyle := block.EffectiveStyle()
	r.renderBox(box, func() {
		defaultStyle := r.measureContext().DefaultStyle
		style := layout.MergedTextStyle(defaultStyle, blockStyle)
		style.Bold = true
		if blockStyle.FontSize <= 0 {
			style.FontSize = layout.HeadingFontSize(defaultStyle.FontSize, block.Level)
			style.LineHeight = layout.FirstPositive(blockStyle.LineHeight, defaultStyle.LineHeight*style.FontSize/defaultStyle.FontSize)
		}
		r.pdf.SetNextTextRole(documentHeadingRole(block.Level))
		r.renderTextSegments(block.Segments, style, r.contentWidth())
		r.pdf.Ln(layout.ResolvedLineHeight(style) * 0.25)
	})
}

func (r *documentRenderer) renderTextSegments(segments []layout.TextSegment, style layout.TextStyle, width float64) {
	lineHeight := layout.ResolvedLineHeight(style)
	if !textSegmentsHavePresentation(segments) {
		r.applyTextStyle(style)
		r.pdf.MultiCell(width, lineHeight, r.textSegmentsPlainText(segments), "", textAlign(style.Align), false)
		return
	}

	startX, startY := r.pdf.GetXY()
	r.applyTextStyle(style)
	plainWidth := r.pdf.GetStringWidth(r.textSegmentsPlainText(segments))
	if plainWidth <= width {
		switch textAlign(style.Align) {
		case "C":
			r.pdf.SetX(startX + (width-plainWidth)/2)
		case "R":
			r.pdf.SetX(startX + width - plainWidth)
		}
	}
	for _, segment := range segments {
		segmentStyle := layout.MergedTextStyle(style, segment.EffectiveStyle())
		r.applyTextStyle(segmentStyle)
		if segment.Link != "" {
			r.pdf.WriteLinkString(layout.ResolvedLineHeight(segmentStyle), segment.Text, segment.Link)
		} else {
			r.pdf.Write(layout.ResolvedLineHeight(segmentStyle), segment.Text)
		}
		if r.pdf.err != nil {
			return
		}
	}
	endY := r.pdf.GetY()
	if endY < startY+lineHeight {
		endY = startY + lineHeight
	}
	r.pdf.SetXY(startX, endY)
}

func textSegmentsHavePresentation(segments []layout.TextSegment) bool {
	for _, segment := range segments {
		if segment.Link != "" || segment.StyleRef != nil || segment.Style != (layout.TextStyle{}) {
			return true
		}
	}
	return false
}

func documentHeadingRole(level int) string {
	switch {
	case level <= 1:
		return "H1"
	case level == 2:
		return "H2"
	case level == 3:
		return "H3"
	case level == 4:
		return "H4"
	case level == 5:
		return "H5"
	default:
		return "H6"
	}
}

func (r *documentRenderer) renderList(block layout.ListBlock) {
	blockStyle := block.EffectiveStyle()
	r.renderBox(block.EffectiveBox(), func() {
		r.pdf.BeginStructure(taggedRoleL)
		defer r.pdf.EndStructure()
		markerWidth := r.listMarkerWidth(block)
		for i, item := range block.Items {
			r.pdf.BeginStructure(taggedRoleLI)
			marker := listMarker(block, i)
			x, y := r.pdf.GetXY()
			r.applyTextStyle(r.mergedStyle(blockStyle))
			r.pdf.SetNextTextRole(taggedRoleLbl)
			r.pdf.CellFormat(markerWidth, 5, marker, "", 0, "R", false, 0, "")
			itemX := x + markerWidth + 2
			r.pdf.SetXY(itemX, y)
			itemWidth := r.contentWidth() - markerWidth - 2
			oldLeft, oldRight := r.pdf.lMargin, r.pdf.rMargin
			r.pdf.lMargin = itemX
			r.pdf.rMargin = r.pdf.w - r.pdf.GetX() - itemWidth
			r.pdf.BeginStructure(taggedRoleLBody)
			r.renderBlocks(item.Blocks)
			r.pdf.EndStructure()
			r.pdf.lMargin, r.pdf.rMargin = oldLeft, oldRight
			r.pdf.SetX(r.pdf.lMargin)
			r.pdf.EndStructure()
		}
	})
}

func (r *documentRenderer) listMarkerWidth(block layout.ListBlock) float64 {
	r.applyTextStyle(r.mergedStyle(block.EffectiveStyle()))
	wd := 5.0
	if len(block.Items) == 0 {
		return wd
	}
	markerIndex := 0
	if block.Ordered {
		markerIndex = len(block.Items) - 1
	}
	if markerWd := r.pdf.GetStringWidth(listMarker(block, markerIndex)); markerWd+1 > wd {
		wd = markerWd + 1
	}
	return wd
}

func (r *documentRenderer) renderTable(block layout.TableBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		r.pdf.BeginStructure(taggedRoleTable)
		defer r.pdf.EndStructure()
		if block.Caption != "" {
			r.renderParagraph(layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: block.Caption}}, Style: layout.TextStyle{Bold: true, Align: "C"}})
		}
		colCount := tableColumnCount(block)
		if colCount <= 0 {
			return
		}
		widths := tableRenderWidths(block, r.contentWidth(), colCount)
		tableLayout := r.measureTableLayout(block, widths)
		for rowIndex := range tableLayout.rows {
			row := &tableLayout.rows[rowIndex]
			if !row.header && layout.ExceedsAvailableHeight(tableLayout.requiredHeight(rowIndex), r.availableHeight()) && r.pdf.GetY() > r.pdf.tMargin {
				r.addPageWithTemplate()
				if tableLayout.repeatHeader {
					for headerIndex := 0; headerIndex < tableLayout.headerRows; headerIndex++ {
						r.renderTableLayoutRow(tableLayout, headerIndex)
					}
				}
			}
			r.renderTableLayoutRow(tableLayout, rowIndex)
		}
	})
}

func (r *documentRenderer) renderTableLayoutRow(tableLayout documentTableLayout, rowIndex int) {
	if rowIndex < 0 || rowIndex >= len(tableLayout.rows) {
		return
	}
	row := tableLayout.rows[rowIndex]
	r.pdf.BeginStructure(taggedRoleTR)
	defer r.pdf.EndStructure()
	x, y := r.pdf.GetXY()
	rowHeight := row.height
	if rowHeight <= 0 {
		rowHeight = 6
	}
	for _, cell := range row.cells {
		cellX := x + layoutgeom.SpanSize(tableLayout.offsets, 0, cell.column)
		cellHeight := layoutgeom.SpanSize(tableLayout.rowOffsets, rowIndex, cell.rowSpan)
		r.renderTableCell(cell, cellX, y, cell.width, cellHeight, row.header, blockTableCellEdges{
			collapse:   tableLayout.borderCollapse,
			lastColumn: cell.column+cell.colSpan >= len(tableLayout.widths),
			lastRow:    rowIndex+cell.rowSpan >= len(tableLayout.rows),
		}, taggedTableAttributes{
			Scope:   tableCellHeaderScope(row.header),
			RowSpan: cell.tagRowSpan,
			ColSpan: cell.colSpan,
		})
	}
	r.pdf.SetXY(r.pdf.lMargin, y+rowHeight)
}

func (tableLayout documentTableLayout) requiredHeight(rowIndex int) float64 {
	if rowIndex < 0 || rowIndex >= len(tableLayout.rows) {
		return 0
	}
	span := 1
	for _, cell := range tableLayout.rows[rowIndex].cells {
		if cell.rowSpan > span {
			span = cell.rowSpan
		}
	}
	return layoutgeom.SpanSize(tableLayout.rowOffsets, rowIndex, span)
}

func (r *documentRenderer) measureTableLayout(block layout.TableBlock, widths []float64) documentTableLayout {
	rows := make([]documentTableRowMeasurement, 0, len(block.Header)+len(block.Body)+len(block.Footer))
	appendRows := func(source []layout.TableRow, header bool) {
		for _, row := range source {
			rows = append(rows, documentTableRowMeasurement{row: row, header: header})
		}
	}
	appendRows(block.Header, true)
	appendRows(block.Body, false)
	appendRows(block.Footer, false)

	tableLayout := documentTableLayout{
		widths:         widths,
		offsets:        layoutgeom.TrackOffsets(widths),
		rows:           rows,
		headerRows:     len(block.Header),
		repeatHeader:   block.Style.RepeatHeader && len(block.Header) > 0,
		borderCollapse: block.Style.BorderCollapse,
	}
	if len(rows) == 0 || len(widths) == 0 {
		return tableLayout
	}

	occupied := make([][]bool, len(rows))
	for i := range occupied {
		occupied[i] = make([]bool, len(widths))
	}
	for rowIndex := range tableLayout.rows {
		column := 0
		for _, cell := range tableLayout.rows[rowIndex].row.Cells {
			for column < len(widths) && occupied[rowIndex][column] {
				column++
			}
			if column >= len(widths) {
				break
			}
			colSpan := normalizedSpan(cell.ColSpan, len(widths)-column)
			rowSpan := normalizedSpan(cell.RowSpan, len(rows)-rowIndex)
			cellWidth := layoutgeom.SpanSize(tableLayout.offsets, column, colSpan)
			measurement := r.measureRenderedTableCellDetailed(cell, cellWidth)
			measurement.column = column
			measurement.colSpan = colSpan
			measurement.rowSpan = rowSpan
			measurement.tagRowSpan = max(cell.RowSpan, 1)
			tableLayout.rows[rowIndex].cells = append(tableLayout.rows[rowIndex].cells, measurement)
			if rowSpan == 1 && measurement.height > tableLayout.rows[rowIndex].height {
				tableLayout.rows[rowIndex].height = measurement.height
			}
			for occupiedRow := rowIndex + 1; occupiedRow < rowIndex+rowSpan; occupiedRow++ {
				for occupiedColumn := column; occupiedColumn < column+colSpan; occupiedColumn++ {
					occupied[occupiedRow][occupiedColumn] = true
				}
			}
			column += colSpan
		}
		if tableLayout.rows[rowIndex].height <= 0 {
			tableLayout.rows[rowIndex].height = 6
		}
	}

	for rowIndex := range tableLayout.rows {
		for _, cell := range tableLayout.rows[rowIndex].cells {
			if cell.rowSpan <= 1 {
				continue
			}
			current := 0.0
			for spanRow := rowIndex; spanRow < rowIndex+cell.rowSpan; spanRow++ {
				current += tableLayout.rows[spanRow].height
			}
			if deficit := cell.height - current; deficit > 0 {
				share := deficit / float64(cell.rowSpan)
				for spanRow := rowIndex; spanRow < rowIndex+cell.rowSpan; spanRow++ {
					tableLayout.rows[spanRow].height += share
				}
			}
		}
	}
	heights := make([]float64, len(tableLayout.rows))
	for rowIndex := range tableLayout.rows {
		heights[rowIndex] = tableLayout.rows[rowIndex].height
	}
	tableLayout.rowOffsets = layoutgeom.TrackOffsets(heights)
	return tableLayout
}

func normalizedSpan(value, available int) int {
	if value <= 0 {
		value = 1
	}
	return min(value, available)
}

func (r *documentRenderer) measureRenderedTableRow(row layout.TableRow, widths []float64) float64 {
	return r.measureRenderedTableRowWithOffsets(row, widths, layout.TrackOffsets(widths))
}

func (r *documentRenderer) measureRenderedTableRowWithOffsets(row layout.TableRow, widths, widthOffsets []float64) float64 {
	return r.measureRenderedTableRowDetailed(row, widths, widthOffsets).height
}

func (r *documentRenderer) measureRenderedTableRowDetailed(row layout.TableRow, widths, widthOffsets []float64) documentTableRowMeasurement {
	measurement := documentTableRowMeasurement{cells: make([]documentTableCellMeasurement, 0, len(row.Cells))}
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
		wd := layout.SpanSize(widthOffsets, col, span)
		cellMeasurement := r.measureRenderedTableCellDetailed(cell, wd)
		measurement.cells = append(measurement.cells, cellMeasurement)
		if cellMeasurement.height > maxHeight {
			maxHeight = cellMeasurement.height
		}
		col += span
	}
	measurement.height = maxHeight
	return measurement
}

func (r *documentRenderer) measureRenderedTableCell(cell layout.TableCell, width float64) float64 {
	return r.measureRenderedTableCellDetailed(cell, width).height
}

func (r *documentRenderer) measureRenderedTableCellDetailed(cell layout.TableCell, width float64) documentTableCellMeasurement {
	box := documentTableCellBox(cell.EffectiveBox(), r.measureContext().CellPadding)
	cellStyle := cell.EffectiveStyle()
	if cell.Align != "" {
		cellStyle.Align = cell.Align
	}
	measurement := documentTableCellMeasurement{width: width, cell: cell}
	contentWidth := width - horizontalBoxSpacing(box)
	if contentWidth < 0 {
		contentWidth = 0
	}
	ctx := r.measureContextForWidth(contentWidth)
	ctx.DefaultStyle = layout.MergedTextStyle(ctx.DefaultStyle, cellStyle)
	if len(cell.Blocks) == 0 {
		measurement.contentHeight = maxPositive(4, layout.ResolvedLineHeight(ctx.DefaultStyle))
		measurement.height = measurement.contentHeight + verticalBoxSpacing(box)
		return measurement
	}
	total := 0.0
	measurement.blockMeasures = make([]layout.BlockMeasurement, len(cell.Blocks))
	for i, block := range cell.Blocks {
		blockMeasure := layout.MeasureBlock(ctx, block)
		measurement.blockMeasures[i] = blockMeasure
		total += blockMeasure.Height
	}
	if total <= 0 {
		total = layout.ResolvedLineHeight(layout.MergedTextStyle(ctx.DefaultStyle, cell.EffectiveStyle()))
	}
	measurement.contentHeight = total
	measurement.height = total + verticalBoxSpacing(box)
	return measurement
}

type blockTableCellEdges struct {
	collapse   bool
	lastColumn bool
	lastRow    bool
}

func (r *documentRenderer) renderTableCell(measurement documentTableCellMeasurement, x, y, wd, ht float64, header bool, edges blockTableCellEdges, tableAttrs taggedTableAttributes) {
	if header {
		r.pdf.beginTableCellStructure(taggedRoleTH, tableAttrs)
	} else {
		r.pdf.beginTableCellStructure(taggedRoleTD, tableAttrs)
	}
	defer r.pdf.EndStructure()
	cell := measurement.cell
	box := documentTableCellBox(cell.EffectiveBox(), r.measureContext().CellPadding)
	r.drawTableCellBox(x, y, wd, ht, box, edges)
	contentWidth := maxPositive(0, wd-horizontalBoxSpacing(box))
	contentHeight := maxPositive(0, ht-verticalBoxSpacing(box))
	contentY := y + box.Border.Top.Width + box.Padding.Top
	remaining := contentHeight - measurement.contentHeight
	switch strings.ToUpper(cell.VerticalAlign) {
	case "M", "MIDDLE", "CENTER", "C":
		contentY += maxPositive(0, remaining/2)
	case "B", "BOTTOM":
		contentY += maxPositive(0, remaining)
	}
	contentX := x + box.Border.Left.Width + box.Padding.Left
	r.pdf.SetXY(contentX, contentY)
	if len(cell.Blocks) > 0 {
		style := cell.EffectiveStyle()
		if cell.Align != "" {
			style.Align = cell.Align
		}
		r.withTextStyle(style, func() {
			r.withContentWidth(contentWidth, func() {
				r.renderBlocksWithMeasurements(cell.Blocks, measurement.blockMeasures)
			})
		})
	}
}

func documentTableCellBox(box layout.BoxStyle, fallback float64) layout.BoxStyle {
	if box.Padding == (layout.Spacing{}) && fallback > 0 {
		box.Padding = layout.Spacing{Top: fallback, Right: fallback, Bottom: fallback, Left: fallback}
	}
	return box
}

func horizontalBoxSpacing(box layout.BoxStyle) float64 {
	return box.Padding.Left + box.Padding.Right + box.Border.Left.Width + box.Border.Right.Width
}

func verticalBoxSpacing(box layout.BoxStyle) float64 {
	return box.Padding.Top + box.Padding.Bottom + box.Border.Top.Width + box.Border.Bottom.Width
}

func (r *documentRenderer) drawTableCellBox(x, y, width, height float64, box layout.BoxStyle, edges blockTableCellEdges) {
	pdf := r.pdf
	drawR, drawG, drawB := pdf.GetDrawColor()
	fillR, fillG, fillB := pdf.GetFillColor()
	lineWidth := pdf.GetLineWidth()
	dashArray := append([]float64(nil), pdf.dashArray...)
	dashPhase := pdf.dashPhase
	defer func() {
		pdf.SetDrawColor(drawR, drawG, drawB)
		pdf.SetFillColor(fillR, fillG, fillB)
		pdf.SetLineWidth(lineWidth)
		pdf.dashArray = dashArray
		pdf.dashPhase = dashPhase
		pdf.outputDashPattern()
	}()

	pdf.BeginArtifact()
	defer pdf.EndArtifact()
	if box.BackgroundColor.Set {
		pdf.SetFillColor(box.BackgroundColor.R, box.BackgroundColor.G, box.BackgroundColor.B)
		pdf.Rect(x, y, width, height, "F")
	}
	border := box.Border
	if !borderVisible(border) {
		border = defaultTableBorder(lineWidth)
	}
	if !edges.collapse || edges.lastRow {
		r.drawBorderSide(border.Bottom, x, y+height, x+width, y+height)
	}
	if !edges.collapse || edges.lastColumn {
		r.drawBorderSide(border.Right, x+width, y, x+width, y+height)
	}
	r.drawBorderSide(border.Top, x, y, x+width, y)
	r.drawBorderSide(border.Left, x, y, x, y+height)
}

func defaultTableBorder(width float64) layout.BorderStyle {
	if width <= 0 {
		width = 0.2
	}
	side := layout.BorderSide{Width: width, Style: "solid"}
	return layout.BorderStyle{Top: side, Right: side, Bottom: side, Left: side}
}

func (r *documentRenderer) drawBorderSide(side layout.BorderSide, x1, y1, x2, y2 float64) {
	if side.Width <= 0 {
		return
	}
	r.pdf.SetLineWidth(side.Width)
	if side.Color.Set {
		r.pdf.SetDrawColor(side.Color.R, side.Color.G, side.Color.B)
	}
	switch strings.ToLower(strings.TrimSpace(side.Style)) {
	case "dashed":
		r.pdf.SetDashPattern([]float64{3 * side.Width, 2 * side.Width}, 0)
	case "dotted":
		r.pdf.SetDashPattern([]float64{side.Width, 2 * side.Width}, 0)
	default:
		r.pdf.SetDashPattern(nil, 0)
	}
	r.pdf.Line(x1, y1, x2, y2)
}

func tableCellHeaderScope(header bool) string {
	if header {
		return "Column"
	}
	return ""
}

func (r *documentRenderer) renderImage(block layout.ImageBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		wd := layout.FirstPositive(block.Width, block.MaxWidth, r.contentWidth())
		ht := layout.FirstPositive(block.Height, block.MaxHeight, wd*0.75)
		if block.MaxWidth > 0 && wd > block.MaxWidth {
			wd = block.MaxWidth
		}
		if block.MaxHeight > 0 && ht > block.MaxHeight {
			ht = block.MaxHeight
		}
		x := r.pdf.GetX()
		switch textAlign(block.Align) {
		case "C":
			x = r.pdf.lMargin + (r.contentWidth()-wd)/2
		case "R":
			x = r.pdf.w - r.pdf.rMargin - wd
		}
		name, options, info := r.registerImageBlock(block)
		switch {
		case name != "":
			if block.Fit == layout.ImageFitContain || block.Fit == layout.ImageFitCover {
				r.renderFittedImage(name, options, info, x, r.pdf.GetY(), wd, ht, block.Fit)
			} else {
				r.pdf.ImageOptions(name, x, r.pdf.GetY(), wd, ht, false, options, 0, "")
			}
		case block.Alt != "":
			r.pdf.BeginArtifact()
			r.pdf.Rect(x, r.pdf.GetY(), wd, ht, "D")
			r.pdf.EndArtifact()
			r.pdf.SetNextTextRole(taggedRoleFigure)
			r.pdf.MultiCell(wd, 5, block.Alt, "", "C", false)
		}
		r.pdf.SetY(r.pdf.GetY() + ht)
		if len(block.Caption) > 0 {
			r.renderParagraph(layout.ParagraphBlock{Segments: block.Caption, Style: layout.TextStyle{FontSize: 9, Italic: true, Align: "C"}})
		}
	})
}

func (r *documentRenderer) registerImageBlock(block layout.ImageBlock) (string, ImageOptions, *ImageInfo) {
	options := ImageOptions{ImageType: block.Format, ReadDpi: block.DPI > 0}
	data := block.ImageData()
	switch {
	case len(data) > 0 && block.Format != "":
		name := r.documentImageName(block)
		info := r.pdf.RegisterImageOptionsReader(name, options, bytes.NewReader(data))
		if block.DPI > 0 && info != nil {
			info.SetDpi(block.DPI)
		}
		return name, options, info
	case block.Source != "":
		info := r.pdf.RegisterImageOptions(block.Source, options)
		if block.DPI > 0 && info != nil {
			info.SetDpi(block.DPI)
		}
		return block.Source, options, info
	default:
		return "", options, nil
	}
}

func (r *documentRenderer) documentImageName(block layout.ImageBlock) string {
	data := block.ImageData()
	key := documentImageCacheKey{
		format: strings.ToLower(block.Format),
		hash:   sha256.Sum256(data),
	}
	if name, ok := r.imageNameCache[key]; ok {
		return name
	}
	name := documentImageName(block)
	if r.imageNameCache == nil {
		r.imageNameCache = make(map[documentImageCacheKey]string)
	}
	r.imageNameCache[key] = name
	return name
}

func documentImageName(block layout.ImageBlock) string {
	data := block.ImageData()
	hash := sha256.New()
	hash.Write([]byte(strings.ToLower(block.Format)))
	hash.Write([]byte{0})
	hash.Write(data)
	return "document-image-" + hex.EncodeToString(hash.Sum(nil))
}

func (r *documentRenderer) renderFittedImage(name string, options ImageOptions, info *ImageInfo, x, y, targetW, targetH float64, fit layout.ImageFitMode) {
	if info == nil || info.Width() <= 0 || info.Height() <= 0 || targetW <= 0 || targetH <= 0 {
		r.pdf.ImageOptions(name, x, y, targetW, targetH, false, options, 0, "")
		return
	}
	fitted := layout.FitImage(info.Width(), info.Height(), targetW, targetH, fit)
	if fit == layout.ImageFitContain {
		r.pdf.ImageOptions(name, x+fitted.OffsetX, y+fitted.OffsetY, fitted.Width, fitted.Height, false, options, 0, "")
		return
	}
	r.pdf.ImageOptionsExtended(name, ExtendedImageOptions{
		X:       x,
		Y:       y,
		W:       fitted.Width,
		H:       fitted.Height,
		Options: options,
		Crop: &ImageCrop{
			X: -fitted.OffsetX,
			Y: -fitted.OffsetY,
			W: targetW,
			H: targetH,
		},
	})
}

func (r *documentRenderer) renderSignatureRow(block layout.SignatureRowBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		columns := block.Columns
		if len(columns) == 0 {
			columns = []layout.SignatureColumn{{}}
		}
		gap := 8.0
		available := r.contentWidth() - gap*float64(len(columns)-1)
		constraints := make([]layoutgeom.TrackConstraint, len(columns))
		for i, column := range columns {
			constraints[i].Preferred = column.Width
		}
		widths := layoutgeom.ResolveTracks(available, len(columns), constraints)
		offsets := layoutgeom.TrackOffsets(widths)
		y := r.pdf.GetY() + 12
		for i, col := range columns {
			wd := widths[i]
			x := r.pdf.lMargin + offsets[i] + float64(i)*gap
			r.pdf.BeginArtifact()
			r.pdf.Line(x, y, x+wd, y)
			r.pdf.EndArtifact()
			r.pdf.SetXY(x, y+2)
			r.applyTextStyle(layout.TextStyle{FontFamily: "Helvetica", FontSize: 9})
			r.pdf.SetNextTextRole(taggedRoleP)
			r.pdf.MultiCell(wd, 4, signatureColumnText(col), "", "C", false)
		}
		r.pdf.SetY(maxPositive(r.pdf.GetY(), y+12))
	})
}

func (r *documentRenderer) renderSignature(signature layout.SignatureBlock) {
	if signature.KeepTogether {
		total := 0.0
		for _, row := range signature.Rows {
			total += layout.MeasureBlock(r.measureContext(), row).Height
		}
		if layout.ExceedsAvailableHeight(total, r.availableHeight()) && r.pdf.GetY() > r.pdf.tMargin {
			r.addPageWithTemplate()
		}
	}
	for _, row := range signature.Rows {
		r.renderBlock(row)
	}
}

func signatureColumnText(col layout.SignatureColumn) string {
	lines := make([]string, 0, 3+len(col.Metadata))
	if col.Label != "" {
		lines = append(lines, col.Label)
	}
	if col.Name != "" && col.Name != col.Label {
		lines = append(lines, col.Name)
	}
	if col.Role != "" && col.Role != col.Label {
		lines = append(lines, col.Role)
	}
	for _, field := range col.Metadata {
		if field.Label == "" && field.Value == "" {
			continue
		}
		switch {
		case field.Label == "":
			lines = append(lines, field.Value)
		case field.Value == "":
			lines = append(lines, field.Label)
		default:
			lines = append(lines, field.Label+": "+field.Value)
		}
	}
	return strings.Join(lines, "\n")
}

func (r *documentRenderer) renderMetadataGrid(block layout.MetadataGridBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		columns := block.Columns
		if columns <= 0 {
			columns = 2
		}
		wd := r.contentWidth() / float64(columns)
		r.applyTextStyle(r.mergedStyle(block.EffectiveStyle()))
		for i, field := range block.Fields {
			if i > 0 && i%columns == 0 {
				r.pdf.Ln(6)
			}
			r.pdf.SetNextTextRole(taggedRoleLbl)
			r.pdf.CellFormat(wd, 6, metadataFieldText(field), "", 0, "L", false, 0, "")
		}
		r.pdf.Ln(6)
	})
}

func metadataFieldText(field layout.MetadataField) string {
	if field.Value == "" {
		return field.Label
	}
	var builder strings.Builder
	builder.Grow(len(field.Label) + len(field.Value) + 2)
	builder.WriteString(field.Label)
	builder.WriteString(": ")
	builder.WriteString(field.Value)
	return builder.String()
}

func (r *documentRenderer) renderQRVerification(block layout.QRVerificationBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		qr := block.QR
		if strings.TrimSpace(qr.Value) == "" && strings.TrimSpace(qr.URL) == "" {
			r.pdf.SetErrorf("QR verification block requires a value or URL")
			return
		}
		size := qr.Size
		if size <= 0 {
			size = 25
		}
		startX, y := r.pdf.GetXY()
		x := startX
		textX := x + size + 4
		textY := y
		textWidth := r.contentWidth() - size - 4
		switch textAlign(qr.Align) {
		case "C":
			x += (r.contentWidth() - size) / 2
			textX = startX
			textY = y + size + 2
			textWidth = r.contentWidth()
		case "R":
			x += r.contentWidth() - size
			textX = startX
			textWidth = r.contentWidth() - size - 4
		}
		payload := firstNonEmpty(strings.TrimSpace(qr.URL), strings.TrimSpace(qr.Value))
		name, err := r.pdf.RegisterQRCodePNG(payload, defaultQRCodeSizePx)
		if err != nil {
			r.pdf.SetError(err)
			return
		}
		r.pdf.ImageOptions(name, x, y, size, size, false, ImageOptions{ImageType: "png"}, 0, "")
		r.pdf.SetXY(textX, textY)
		segments := block.Text
		if len(segments) == 0 {
			text := firstNonEmpty(qr.Label, "Verification")
			if qr.URL != "" {
				text += "\n" + qr.URL
			} else if qr.Value != "" {
				text += "\n" + qr.Value
			}
			segments = []layout.TextSegment{{Text: text}}
		}
		r.applyTextStyle(r.mergedStyle(block.EffectiveStyle()))
		r.pdf.SetNextTextRole(taggedRoleP)
		r.pdf.MultiCell(max(textWidth, 0), 5, r.textSegmentsPlainText(segments), "", "L", false)
		if r.pdf.GetY() < y+size {
			r.pdf.SetY(y + size)
		}
	})
}

func (r *documentRenderer) renderBox(box layout.BoxStyle, render func()) {
	if box.Margin.Top > 0 {
		r.pdf.Ln(box.Margin.Top)
	}
	x, y := r.pdf.GetXY()
	wd := r.contentWidth()
	if box.BackgroundColor.Set {
		height := box.Padding.Top + box.Padding.Bottom
		if r.boxHeightOK {
			height = r.boxHeight - box.Margin.Top - box.Margin.Bottom
		}
		r.drawBoxBackground(x, y, wd, maxPositive(1, height), box.BackgroundColor)
	}
	r.pdf.SetXY(x+box.Padding.Left, y+box.Padding.Top)
	oldLeft, oldRight := r.pdf.lMargin, r.pdf.rMargin
	r.pdf.lMargin = x + box.Padding.Left
	r.pdf.rMargin = r.pdf.w - x - wd + box.Padding.Right
	render()
	r.pdf.lMargin, r.pdf.rMargin = oldLeft, oldRight
	r.pdf.SetX(oldLeft)
	r.pdf.Ln(box.Padding.Bottom + box.Margin.Bottom)
	if borderVisible(box.Border) {
		bottomY := r.pdf.GetY()
		r.drawBoxBorder(x, y, wd, bottomY-y-box.Margin.Bottom, box.Border)
	}
}

func (r *documentRenderer) drawBoxBackground(x, y, width, height float64, color layout.DocumentColor) {
	fillR, fillG, fillB := r.pdf.GetFillColor()
	r.pdf.BeginArtifact()
	r.pdf.SetFillColor(color.R, color.G, color.B)
	r.pdf.Rect(x, y, width, height, "F")
	r.pdf.SetFillColor(fillR, fillG, fillB)
	r.pdf.EndArtifact()
}

func (r *documentRenderer) drawBoxBorder(x, y, width, height float64, border layout.BorderStyle) {
	drawR, drawG, drawB := r.pdf.GetDrawColor()
	lineWidth := r.pdf.GetLineWidth()
	dashArray := append([]float64(nil), r.pdf.dashArray...)
	dashPhase := r.pdf.dashPhase
	r.pdf.BeginArtifact()
	r.drawBorderSide(border.Top, x, y, x+width, y)
	r.drawBorderSide(border.Right, x+width, y, x+width, y+height)
	r.drawBorderSide(border.Bottom, x, y+height, x+width, y+height)
	r.drawBorderSide(border.Left, x, y, x, y+height)
	r.pdf.SetDrawColor(drawR, drawG, drawB)
	r.pdf.SetLineWidth(lineWidth)
	r.pdf.dashArray = dashArray
	r.pdf.dashPhase = dashPhase
	r.pdf.outputDashPattern()
	r.pdf.EndArtifact()
}

func (r *documentRenderer) applyTextStyle(style layout.TextStyle) {
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
	size := style.FontSize
	if size <= 0 {
		size = 12
	}
	r.pdf.SetFont(family, fontStyle, size)
	if style.Color.Set {
		r.pdf.SetTextColor(style.Color.R, style.Color.G, style.Color.B)
	} else {
		r.pdf.SetTextColor(0, 0, 0)
	}
}

func textAlign(align string) string {
	switch strings.ToUpper(align) {
	case "C", "CENTER":
		return "C"
	case "R", "RIGHT":
		return "R"
	default:
		return "L"
	}
}

func listMarker(block layout.ListBlock, index int) string {
	if block.Ordered {
		return strconv.Itoa(index+1) + "."
	}
	return "-"
}

func documentParagraphBox(box layout.BoxStyle) layout.BoxStyle {
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = documentParagraphSpacing
	}
	return box
}

func documentHeadingBox(box layout.BoxStyle) layout.BoxStyle {
	if box.Margin.Top == 0 {
		box.Margin.Top = documentHeadingTopSpace
	}
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = documentHeadingBotSpace
	}
	return box
}

func (f *Document) applyPageTemplateMargins(template layout.PageTemplate) {
	margins := template.Margins
	if margins.Top <= 0 && margins.Right <= 0 && margins.Bottom <= 0 && margins.Left <= 0 {
		return
	}
	left, top, right, bottom := f.GetMargins()
	if margins.Left > 0 {
		left = margins.Left
	}
	if margins.Top > 0 {
		top = margins.Top
	}
	if margins.Right > 0 {
		right = margins.Right
	}
	if margins.Bottom > 0 {
		bottom = margins.Bottom
	}
	f.SetMargins(left, top, right)
	autoPageBreak, _ := f.GetAutoPageBreak()
	f.SetAutoPageBreak(autoPageBreak, bottom)
	if f.page > 0 {
		if f.x < left {
			f.x = left
		}
		if f.y < top {
			f.y = top
		}
	}
}

func documentAttachments(blocks []layout.AttachmentBlock) []Attachment {
	attachments := make([]Attachment, 0, len(blocks))
	for _, block := range blocks {
		if block.Name == "" && len(block.Data) == 0 {
			continue
		}
		attachments = append(attachments, Attachment{
			Content:     block.Data,
			Filename:    block.Name,
			Description: block.Description,
		})
	}
	return attachments
}

func tableColumnCount(table layout.TableBlock) int {
	count := len(table.Columns)
	measureRows := func(rows []layout.TableRow) {
		for _, row := range rows {
			rowCount := tableRowColumnCount(row)
			if rowCount > count {
				count = rowCount
			}
		}
	}
	measureRows(table.Header)
	measureRows(table.Body)
	measureRows(table.Footer)
	return count
}

func tableRowColumnCount(row layout.TableRow) int {
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

func tableRenderWidths(table layout.TableBlock, total float64, count int) []float64 {
	constraints := make([]layoutgeom.TrackConstraint, len(table.Columns))
	for i, column := range table.Columns {
		constraints[i] = layoutgeom.TrackConstraint{Preferred: column.Width, Min: column.MinWidth, Max: column.MaxWidth}
	}
	return layoutgeom.ResolveTracks(total, count, constraints)
}

func borderVisible(border layout.BorderStyle) bool {
	return border.Top.Width > 0 || border.Right.Width > 0 || border.Bottom.Width > 0 || border.Left.Width > 0
}

func maxPositive(a, b float64) float64 {
	if a > b && a > 0 {
		return a
	}
	if b > 0 {
		return b
	}
	return 0
}
