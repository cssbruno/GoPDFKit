// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"unsafe"
)

const (
	documentParagraphSpacing = 2.0
	documentHeadingTopSpace  = 2.5
	documentHeadingBotSpace  = 1.5
)

// WriteDocument renders a shared document model into the PDF.
func (f *Document) WriteDocument(doc *LayoutDocument) {
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
		for _, row := range doc.Signature.Rows {
			renderer.renderBlock(row)
		}
	}
	if doc.QR != nil {
		renderer.renderBlock(QRVerificationBlock{QR: *doc.QR})
	}
	renderer.renderFooter()
}

type documentRenderer struct {
	pdf               *Document
	template          PageTemplate
	renderedFooters   map[int]bool
	renderingShell    bool
	contentWidthVal   float64
	contentWidthW     float64
	contentWidthL     float64
	contentWidthR     float64
	contentWidthOK    bool
	scopedWidth       float64
	scopedWidthOK     bool
	measureCtx        MeasureContext
	measureCtxWidth   float64
	measureCtxFont    string
	measureCtxPt      float64
	measureCtxUnit    float64
	measureCtxOK      bool
	imageNameCache    map[documentImageCacheKey]string
	plainTextCache    map[documentTextSegmentsCacheKey]string
	tableRowCache     map[documentTableRowCacheKey]documentTableRowMeasurement
	blockMeasureCache map[documentBlockMeasureCacheKey][]BlockMeasurement
}

type documentImageCacheKey struct {
	format string
	ptr    uintptr
	len    int
}

type documentTextSegmentsCacheKey struct {
	ptr uintptr
	len int
}

type documentTableRowCacheKey struct {
	rowPtr    uintptr
	widthsPtr uintptr
	widthsLen int
}

type documentTableRowMeasurement struct {
	height float64
	cells  []documentTableCellMeasurement
}

type documentTableCellMeasurement struct {
	width         float64
	height        float64
	blockMeasures []BlockMeasurement
}

type documentBlockMeasureCacheKey struct {
	ptr   uintptr
	len   int
	width float64
	font  string
	pt    float64
	unit  float64
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

func (r *documentRenderer) measureContext() MeasureContext {
	return r.measureContextForWidth(r.contentWidth())
}

func (r *documentRenderer) measureContextForWidth(width float64) MeasureContext {
	if !r.measureCtxOK || r.measureCtxWidth != width || r.measureCtxFont != r.pdf.fontFamily || r.measureCtxPt != r.pdf.fontSizePt || r.measureCtxUnit != r.pdf.fontSize {
		r.measureCtx = NewMeasureContext(r.pdf, width)
		r.measureCtxWidth = width
		r.measureCtxFont = r.pdf.fontFamily
		r.measureCtxPt = r.pdf.fontSizePt
		r.measureCtxUnit = r.pdf.fontSize
		r.measureCtxOK = true
	}
	return r.measureCtx
}

func (r *documentRenderer) mergedStyle(style TextStyle) TextStyle {
	return mergedTextStyle(r.measureContext().DefaultStyle, style)
}

func (r *documentRenderer) textSegmentsPlainText(segments []TextSegment) string {
	if len(segments) == 0 {
		return ""
	}
	key := documentTextSegmentsCacheKey{
		ptr: uintptr(unsafe.Pointer(&segments[0])),
		len: len(segments),
	}
	if text, ok := r.plainTextCache[key]; ok {
		return text
	}
	text := textSegmentsPlainText(segments)
	if r.plainTextCache == nil {
		r.plainTextCache = make(map[documentTextSegmentsCacheKey]string)
	}
	r.plainTextCache[key] = text
	return text
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
		r.applyTextStyle(TextStyle{FontFamily: "Helvetica", FontSize: 9})
		r.pdf.CellFormat(r.contentWidth(), 5, text, "", 1, "R", false, 0, "")
	}
}

func (r *documentRenderer) renderBlocks(blocks []Block) {
	for _, block := range blocks {
		r.renderBlock(block)
		if r.pdf.err != nil {
			return
		}
	}
}

func (r *documentRenderer) renderBlocksWithMeasurements(blocks []Block, measurements []BlockMeasurement) {
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

func (r *documentRenderer) renderRepeatedBlocks(blocks []Block) {
	measurements, ok := r.cachedBlockMeasurements(blocks)
	if !ok {
		r.renderBlocks(blocks)
		return
	}
	r.renderBlocksWithMeasurements(blocks, measurements)
}

func (r *documentRenderer) cachedBlockMeasurements(blocks []Block) ([]BlockMeasurement, bool) {
	if len(blocks) == 0 {
		return nil, true
	}
	key := documentBlockMeasureCacheKey{
		ptr:   uintptr(unsafe.Pointer(&blocks[0])),
		len:   len(blocks),
		width: r.contentWidth(),
		font:  r.pdf.fontFamily,
		pt:    r.pdf.fontSizePt,
		unit:  r.pdf.fontSize,
	}
	if measurements, ok := r.blockMeasureCache[key]; ok {
		return measurements, true
	}
	measurements := MeasureBlocks(r.measureContext(), blocks)
	if r.blockMeasureCache == nil {
		r.blockMeasureCache = make(map[documentBlockMeasureCacheKey][]BlockMeasurement)
	}
	r.blockMeasureCache[key] = measurements
	return measurements, true
}

func (r *documentRenderer) renderBlock(block Block) {
	r.renderBlockMeasured(block, BlockMeasurement{}, false)
}

func (r *documentRenderer) renderBlockMeasured(block Block, measure BlockMeasurement, measured bool) {
	if block == nil || r.pdf.err != nil {
		return
	}
	if pageBreak, ok := block.(PageBreakBlock); ok {
		if !r.renderingShell && (pageBreak.Before || pageBreak.After) {
			r.addPageWithTemplate()
		}
		return
	}
	if !measured {
		measure = MeasureBlock(r.measureContext(), block)
	}
	if !r.renderingShell && (measure.BreakBefore || measure.ShouldMoveToNextPage(r.availableHeight())) {
		r.addPageWithTemplate()
	}
	switch b := block.(type) {
	case ParagraphBlock:
		r.renderParagraph(b)
	case HeadingBlock:
		r.renderHeading(b)
	case ListBlock:
		r.renderList(b)
	case TableBlock:
		r.renderTable(b)
	case ImageBlock:
		r.renderImage(b)
	case SignatureRowBlock:
		r.renderSignatureRow(b)
	case MetadataGridBlock:
		r.renderMetadataGrid(b)
	case QRVerificationBlock:
		r.renderQRVerification(b)
	case NoteBoxBlock:
		r.renderBox(b.EffectiveBox(), func() {
			if b.Title != "" {
				r.renderHeading(HeadingBlock{Level: 4, Segments: []TextSegment{{Text: b.Title}}, Style: b.EffectiveStyle()})
			}
			r.renderBlocks(b.Body)
		})
	case SectionBlock:
		if b.Title != "" {
			r.renderHeading(HeadingBlock{Level: 2, Segments: []TextSegment{{Text: b.Title}}})
		}
		r.renderBox(b.EffectiveBox(), func() { r.renderBlocks(b.Blocks) })
	case ClauseBlock:
		title := strings.TrimSpace(strings.TrimSpace(b.Number + " " + b.Title))
		if title != "" {
			r.renderHeading(HeadingBlock{Level: 3, Segments: []TextSegment{{Text: title}}})
		}
		r.renderBox(b.EffectiveBox(), func() { r.renderBlocks(b.Blocks) })
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

func (r *documentRenderer) renderParagraph(block ParagraphBlock) {
	box := documentParagraphBox(block.EffectiveBox())
	r.renderBox(box, func() {
		style := r.mergedStyle(block.EffectiveStyle())
		r.applyTextStyle(style)
		r.pdf.SetNextTextRole(taggedRoleP)
		r.pdf.MultiCell(r.contentWidth(), resolvedLineHeight(style), r.textSegmentsPlainText(block.Segments), "", textAlign(style.Align), false)
	})
}

func (r *documentRenderer) renderHeading(block HeadingBlock) {
	box := documentHeadingBox(block.EffectiveBox())
	blockStyle := block.EffectiveStyle()
	r.renderBox(box, func() {
		defaultStyle := r.measureContext().DefaultStyle
		style := mergedTextStyle(defaultStyle, blockStyle)
		style.Bold = true
		if blockStyle.FontSize <= 0 {
			style.FontSize = documentHeadingFontSize(defaultStyle.FontSize, block.Level)
			style.LineHeight = firstPositive(blockStyle.LineHeight, defaultStyle.LineHeight*style.FontSize/defaultStyle.FontSize)
		}
		r.applyTextStyle(style)
		r.pdf.SetNextTextRole(documentHeadingRole(block.Level))
		r.pdf.MultiCell(r.contentWidth(), resolvedLineHeight(style), r.textSegmentsPlainText(block.Segments), "", textAlign(style.Align), false)
		r.pdf.Ln(resolvedLineHeight(style) * 0.25)
	})
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

func (r *documentRenderer) renderList(block ListBlock) {
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

func (r *documentRenderer) listMarkerWidth(block ListBlock) float64 {
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

func (r *documentRenderer) renderTable(block TableBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		r.pdf.BeginStructure(taggedRoleTable)
		defer r.pdf.EndStructure()
		if block.Caption != "" {
			r.renderParagraph(ParagraphBlock{Segments: []TextSegment{{Text: block.Caption}}, Style: TextStyle{Bold: true, Align: "C"}})
		}
		colCount := tableColumnCount(block)
		if colCount <= 0 {
			return
		}
		widths := tableRenderWidths(block, r.contentWidth(), colCount)
		widthOffsets := documentTableSpanPrefix(widths)
		renderRows := func(rows []TableRow, header bool) {
			for i := range rows {
				r.renderTableRow(&rows[i], widths, widthOffsets, header)
			}
		}
		renderRows(block.Header, true)
		renderRows(block.Body, false)
		renderRows(block.Footer, false)
	})
}

func (r *documentRenderer) renderTableRow(row *TableRow, widths, widthOffsets []float64, header bool) {
	if row == nil {
		return
	}
	if len(widths) == 0 {
		return
	}
	r.pdf.BeginStructure(taggedRoleTR)
	defer r.pdf.EndStructure()
	x, y := r.pdf.GetXY()
	rowMeasurement := r.measureRenderedTableRowCached(row, widths, widthOffsets)
	rowHeight := rowMeasurement.height
	if rowHeight <= 0 {
		rowHeight = 6
	}
	if rowHeight > r.availableHeight() && r.pdf.GetY() > r.pdf.tMargin {
		r.addPageWithTemplate()
		x, y = r.pdf.GetXY()
	}
	col := 0
	for i, cell := range row.Cells {
		if col >= len(widths) {
			break
		}
		span := cell.ColSpan
		if span <= 0 {
			span = 1
		}
		if i >= len(rowMeasurement.cells) {
			break
		}
		cellMeasurement := rowMeasurement.cells[i]
		wd := cellMeasurement.width
		r.renderTableCell(cell, cellMeasurement, x, y, wd, rowHeight, header, taggedTableAttributes{
			Scope:   tableCellHeaderScope(header),
			RowSpan: cell.RowSpan,
			ColSpan: span,
		})
		x += wd
		col += span
	}
	r.pdf.SetXY(r.pdf.lMargin, y+rowHeight)
}

func (r *documentRenderer) measureRenderedTableRow(row TableRow, widths []float64) float64 {
	return r.measureRenderedTableRowWithOffsets(row, widths, documentTableSpanPrefix(widths))
}

func (r *documentRenderer) measureRenderedTableRowWithOffsets(row TableRow, widths, widthOffsets []float64) float64 {
	return r.measureRenderedTableRowDetailed(row, widths, widthOffsets).height
}

func (r *documentRenderer) measureRenderedTableRowCached(row *TableRow, widths, widthOffsets []float64) documentTableRowMeasurement {
	key := documentTableRowCacheKey{
		rowPtr:    uintptr(unsafe.Pointer(row)),
		widthsPtr: documentFloatSlicePointer(widths),
		widthsLen: len(widths),
	}
	if cached, ok := r.tableRowCache[key]; ok {
		return cached
	}
	measurement := r.measureRenderedTableRowDetailed(*row, widths, widthOffsets)
	if r.tableRowCache == nil {
		r.tableRowCache = make(map[documentTableRowCacheKey]documentTableRowMeasurement)
	}
	r.tableRowCache[key] = measurement
	return measurement
}

func documentFloatSlicePointer(values []float64) uintptr {
	if len(values) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(&values[0]))
}

func (r *documentRenderer) measureRenderedTableRowDetailed(row TableRow, widths, widthOffsets []float64) documentTableRowMeasurement {
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
		wd := documentTablePrefixSpanWidth(widthOffsets, col, span)
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

func (r *documentRenderer) measureRenderedTableCell(cell TableCell, width float64) float64 {
	return r.measureRenderedTableCellDetailed(cell, width).height
}

func (r *documentRenderer) measureRenderedTableCellDetailed(cell TableCell, width float64) documentTableCellMeasurement {
	measurement := documentTableCellMeasurement{width: width}
	contentWidth := width - 2
	if contentWidth < 0 {
		contentWidth = 0
	}
	ctx := r.measureContextForWidth(contentWidth)
	if len(cell.Blocks) == 0 {
		style := mergedTextStyle(ctx.DefaultStyle, cell.EffectiveStyle())
		measurement.height = maxPositive(4, resolvedLineHeight(style))
		return measurement
	}
	total := 0.0
	measurement.blockMeasures = make([]BlockMeasurement, len(cell.Blocks))
	for i, block := range cell.Blocks {
		blockMeasure := MeasureBlock(ctx, block)
		measurement.blockMeasures[i] = blockMeasure
		total += blockMeasure.Height
	}
	if total <= 0 {
		total = resolvedLineHeight(mergedTextStyle(ctx.DefaultStyle, cell.EffectiveStyle()))
	}
	measurement.height = total + 2
	return measurement
}

func (r *documentRenderer) renderTableCell(cell TableCell, measurement documentTableCellMeasurement, x, y, wd, ht float64, header bool, tableAttrs taggedTableAttributes) {
	if header {
		r.pdf.beginTableCellStructure(taggedRoleTH, tableAttrs)
	} else {
		r.pdf.beginTableCellStructure(taggedRoleTD, tableAttrs)
	}
	defer r.pdf.EndStructure()
	r.pdf.BeginArtifact()
	r.pdf.Rect(x, y, wd, ht, "D")
	r.pdf.EndArtifact()
	cellX := x + 1
	r.pdf.SetXY(cellX, y+1)
	if len(cell.Blocks) > 0 {
		r.withContentWidth(wd-2, func() {
			r.renderBlocksWithMeasurements(cell.Blocks, measurement.blockMeasures)
		})
	}
}

func tableCellHeaderScope(header bool) string {
	if header {
		return "Column"
	}
	return ""
}

func (r *documentRenderer) renderImage(block ImageBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		wd := firstPositive(block.Width, block.MaxWidth, r.contentWidth())
		ht := firstPositive(block.Height, block.MaxHeight, wd*0.75)
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
			if block.Fit == ImageFitContain || block.Fit == ImageFitCover {
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
			r.renderParagraph(ParagraphBlock{Segments: block.Caption, Style: TextStyle{FontSize: 9, Italic: true, Align: "C"}})
		}
	})
}

func (r *documentRenderer) registerImageBlock(block ImageBlock) (string, ImageOptions, *ImageInfo) {
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

func (r *documentRenderer) documentImageName(block ImageBlock) string {
	data := block.ImageData()
	key := documentImageCacheKey{
		format: strings.ToLower(block.Format),
		ptr:    uintptr(unsafe.Pointer(&data[0])),
		len:    len(data),
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

func documentImageName(block ImageBlock) string {
	data := block.ImageData()
	hash := sha256.New()
	hash.Write([]byte(strings.ToLower(block.Format)))
	hash.Write([]byte{0})
	hash.Write(data)
	return "document-image-" + hex.EncodeToString(hash.Sum(nil))
}

func (r *documentRenderer) renderFittedImage(name string, options ImageOptions, info *ImageInfo, x, y, targetW, targetH float64, fit ImageFitMode) {
	if info == nil || info.Width() <= 0 || info.Height() <= 0 || targetW <= 0 || targetH <= 0 {
		r.pdf.ImageOptions(name, x, y, targetW, targetH, false, options, 0, "")
		return
	}
	imageW, imageH := info.Width(), info.Height()
	scaleX := targetW / imageW
	scaleY := targetH / imageH
	scale := scaleX
	if fit == ImageFitContain {
		if scaleY < scale {
			scale = scaleY
		}
		drawW := imageW * scale
		drawH := imageH * scale
		r.pdf.ImageOptions(name, x+(targetW-drawW)/2, y+(targetH-drawH)/2, drawW, drawH, false, options, 0, "")
		return
	}
	if scaleY > scale {
		scale = scaleY
	}
	drawW := imageW * scale
	drawH := imageH * scale
	r.pdf.ImageOptionsExtended(name, ExtendedImageOptions{
		X:       x,
		Y:       y,
		W:       drawW,
		H:       drawH,
		Options: options,
		Crop: &ImageCrop{
			X: (drawW - targetW) / 2,
			Y: (drawH - targetH) / 2,
			W: targetW,
			H: targetH,
		},
	})
}

func (r *documentRenderer) renderSignatureRow(block SignatureRowBlock) {
	r.renderBox(block.EffectiveBox(), func() {
		columns := block.Columns
		if len(columns) == 0 {
			columns = []SignatureColumn{{}}
		}
		gap := 8.0
		wd := (r.contentWidth() - gap*float64(len(columns)-1)) / float64(len(columns))
		y := r.pdf.GetY() + 12
		for i, col := range columns {
			x := r.pdf.lMargin + float64(i)*(wd+gap)
			r.pdf.BeginArtifact()
			r.pdf.Line(x, y, x+wd, y)
			r.pdf.EndArtifact()
			r.pdf.SetXY(x, y+2)
			r.applyTextStyle(TextStyle{FontFamily: "Helvetica", FontSize: 9})
			r.pdf.SetNextTextRole(taggedRoleP)
			r.pdf.MultiCell(wd, 4, signatureColumnText(col), "", "C", false)
		}
		r.pdf.SetY(maxPositive(r.pdf.GetY(), y+12))
	})
}

func signatureColumnText(col SignatureColumn) string {
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

func (r *documentRenderer) renderMetadataGrid(block MetadataGridBlock) {
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

func metadataFieldText(field MetadataField) string {
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

func (r *documentRenderer) renderQRVerification(block QRVerificationBlock) {
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
		x, y := r.pdf.GetXY()
		r.pdf.BeginArtifact()
		r.pdf.Rect(x, y, size, size, "D")
		r.pdf.Line(x, y, x+size, y+size)
		r.pdf.Line(x+size, y, x, y+size)
		r.pdf.EndArtifact()
		textX := x + size + 4
		r.pdf.SetXY(textX, y)
		segments := block.Text
		if len(segments) == 0 {
			text := firstNonEmpty(qr.Label, "Verification")
			if qr.URL != "" {
				text += "\n" + qr.URL
			} else if qr.Value != "" {
				text += "\n" + qr.Value
			}
			segments = []TextSegment{{Text: text}}
		}
		r.applyTextStyle(r.mergedStyle(block.EffectiveStyle()))
		r.pdf.SetNextTextRole(taggedRoleP)
		r.pdf.MultiCell(r.contentWidth()-size-4, 5, r.textSegmentsPlainText(segments), "", "L", false)
		if r.pdf.GetY() < y+size {
			r.pdf.SetY(y + size)
		}
	})
}

func (r *documentRenderer) renderBox(box BoxStyle, render func()) {
	if box.Margin.Top > 0 {
		r.pdf.Ln(box.Margin.Top)
	}
	x, y := r.pdf.GetXY()
	wd := r.contentWidth()
	if box.BackgroundColor.Set {
		r.pdf.SetFillColor(box.BackgroundColor.R, box.BackgroundColor.G, box.BackgroundColor.B)
		r.pdf.Rect(x, y, wd, maxPositive(1, box.Padding.Top+box.Padding.Bottom), "F")
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
		r.pdf.Rect(x, y, wd, bottomY-y, "D")
	}
}

func (r *documentRenderer) applyTextStyle(style TextStyle) {
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

func listMarker(block ListBlock, index int) string {
	if block.Ordered {
		return strconv.Itoa(index+1) + "."
	}
	return "-"
}

func documentParagraphBox(box BoxStyle) BoxStyle {
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = documentParagraphSpacing
	}
	return box
}

func documentHeadingBox(box BoxStyle) BoxStyle {
	if box.Margin.Top == 0 {
		box.Margin.Top = documentHeadingTopSpace
	}
	if box.Margin.Bottom == 0 {
		box.Margin.Bottom = documentHeadingBotSpace
	}
	return box
}

func (f *Document) applyPageTemplateMargins(template PageTemplate) {
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

func documentAttachments(blocks []AttachmentBlock) []Attachment {
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

func tableColumnCount(table TableBlock) int {
	count := len(table.Columns)
	measureRows := func(rows []TableRow) {
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

func tableRowColumnCount(row TableRow) int {
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

func tableRenderWidths(table TableBlock, total float64, count int) []float64 {
	widths := make([]float64, count)
	fixed := 0.0
	for i := 0; i < count && i < len(table.Columns); i++ {
		if table.Columns[i].Width > 0 {
			widths[i] = table.Columns[i].Width
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

func documentTableSpanPrefix(widths []float64) []float64 {
	offsets := make([]float64, len(widths)+1)
	for i, wd := range widths {
		offsets[i+1] = offsets[i] + wd
	}
	return offsets
}

func documentTablePrefixSpanWidth(offsets []float64, start, span int) float64 {
	if span <= 0 || start < 0 || start >= len(offsets)-1 {
		return 0
	}
	end := start + span
	if end > len(offsets)-1 {
		end = len(offsets) - 1
	}
	return offsets[end] - offsets[start]
}

func borderVisible(border BorderStyle) bool {
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
