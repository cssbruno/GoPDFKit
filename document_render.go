// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

const (
	documentParagraphSpacing = 2.0
	documentHeadingTopSpace  = 2.5
	documentHeadingBotSpace  = 1.5
)

// WriteDocument renders a shared document model into the PDF.
func (f *Fpdf) WriteDocument(doc *Document) {
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
	if f.page == 0 {
		f.AddPage()
	}
	chrome := doc.PageChrome()
	if alias := chrome.pageTotalAlias(); alias != "" {
		f.AliasNbPages(alias)
	}
	renderer := documentRenderer{pdf: f, chrome: chrome}
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
	pdf    *Fpdf
	chrome PageChrome
}

func (r *documentRenderer) contentWidth() float64 {
	return r.pdf.w - r.pdf.lMargin - r.pdf.rMargin
}

func (r *documentRenderer) availableHeight() float64 {
	reservedFooter := r.chrome.FooterReservedHeightForPage(r.pdf.page)
	return r.pdf.h - r.pdf.bMargin - reservedFooter - r.pdf.GetY()
}

func (r *documentRenderer) renderHeader() {
	header := r.chrome.Header
	if r.pdf.page == 1 && r.chrome.FirstPageHeader != nil {
		header = r.chrome.FirstPageHeader
	}
	if header == nil {
		return
	}
	startY := r.pdf.GetY()
	r.renderBlocks(header.Blocks)
	if header.Height > 0 && r.pdf.GetY() < startY+header.Height {
		r.pdf.SetY(startY + header.Height)
	}
}

func (r *documentRenderer) renderFooter() {
	footer := r.chrome.FooterForPage(r.pdf.page)
	if footer == nil && r.chrome.PageNumberText(r.pdf.page) == "" {
		return
	}
	_, _, _, bottom := r.pdf.GetMargins()
	y := r.pdf.h - bottom
	if footer != nil && footer.Height > 0 {
		y -= footer.Height
	}
	if y < r.pdf.tMargin {
		y = r.pdf.tMargin
	}
	r.pdf.SetY(y)
	if footer != nil {
		r.renderBlocks(footer.Blocks)
	}
	if text := r.chrome.PageNumberText(r.pdf.page); text != "" {
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

func (r *documentRenderer) renderBlock(block Block) {
	if block == nil || r.pdf.err != nil {
		return
	}
	if pageBreak, ok := block.(PageBreakBlock); ok {
		if pageBreak.Before || pageBreak.After {
			r.pdf.AddPage()
			r.renderHeader()
		}
		return
	}
	measure := MeasureBlock(NewMeasureContext(r.pdf, r.contentWidth()), block)
	if measure.BreakBefore || measure.ShouldMoveToNextPage(r.availableHeight()) {
		r.pdf.AddPage()
		r.renderHeader()
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
		r.renderBox(b.Box, func() {
			if b.Title != "" {
				r.renderHeading(HeadingBlock{Level: 4, Segments: []TextSegment{{Text: b.Title}}, Style: b.Style})
			}
			r.renderBlocks(b.Body)
		})
	case SectionBlock:
		if b.Title != "" {
			r.renderHeading(HeadingBlock{Level: 2, Segments: []TextSegment{{Text: b.Title}}})
		}
		r.renderBox(b.Box, func() { r.renderBlocks(b.Blocks) })
	case ClauseBlock:
		title := strings.TrimSpace(strings.TrimSpace(b.Number + " " + b.Title))
		if title != "" {
			r.renderHeading(HeadingBlock{Level: 3, Segments: []TextSegment{{Text: title}}})
		}
		r.renderBox(b.Box, func() { r.renderBlocks(b.Blocks) })
	}
	if measure.BreakAfter {
		r.pdf.AddPage()
		r.renderHeader()
	}
}

func (r *documentRenderer) renderParagraph(block ParagraphBlock) {
	block.Box = documentParagraphBox(block.Box)
	r.renderBox(block.Box, func() {
		style := mergedTextStyle(NewMeasureContext(r.pdf, r.contentWidth()).DefaultStyle, block.Style)
		r.applyTextStyle(style)
		r.pdf.MultiCell(0, resolvedLineHeight(style), textSegmentsPlainText(block.Segments), "", textAlign(style.Align), false)
	})
}

func (r *documentRenderer) renderHeading(block HeadingBlock) {
	block.Box = documentHeadingBox(block.Box)
	r.renderBox(block.Box, func() {
		style := mergedTextStyle(NewMeasureContext(r.pdf, r.contentWidth()).DefaultStyle, block.Style)
		style.Bold = true
		if style.FontSize <= 0 {
			style.FontSize = documentHeadingFontSize(12, block.Level)
		}
		if style.LineHeight <= 0 {
			style.LineHeight = style.FontSize * 1.25
		}
		r.applyTextStyle(style)
		r.pdf.MultiCell(0, resolvedLineHeight(style), textSegmentsPlainText(block.Segments), "", textAlign(style.Align), false)
		r.pdf.Ln(resolvedLineHeight(style) * 0.25)
	})
}

func (r *documentRenderer) renderList(block ListBlock) {
	r.renderBox(block.Box, func() {
		markerWidth := r.listMarkerWidth(block)
		for i, item := range block.Items {
			marker := listMarker(block, i)
			x, y := r.pdf.GetXY()
			r.applyTextStyle(mergedTextStyle(NewMeasureContext(r.pdf, r.contentWidth()).DefaultStyle, block.Style))
			r.pdf.CellFormat(markerWidth, 5, marker, "", 0, "R", false, 0, "")
			r.pdf.SetXY(x+markerWidth+2, y)
			itemWidth := r.contentWidth() - markerWidth - 2
			oldRight := r.pdf.rMargin
			r.pdf.rMargin = r.pdf.w - r.pdf.GetX() - itemWidth
			r.renderBlocks(item.Blocks)
			r.pdf.rMargin = oldRight
			r.pdf.SetX(r.pdf.lMargin)
		}
	})
}

func (r *documentRenderer) listMarkerWidth(block ListBlock) float64 {
	r.applyTextStyle(mergedTextStyle(NewMeasureContext(r.pdf, r.contentWidth()).DefaultStyle, block.Style))
	wd := 5.0
	for i := range block.Items {
		if markerWd := r.pdf.GetStringWidth(listMarker(block, i)); markerWd+1 > wd {
			wd = markerWd + 1
		}
	}
	return wd
}

func (r *documentRenderer) renderTable(block TableBlock) {
	r.renderBox(block.Box, func() {
		if block.Caption != "" {
			r.renderParagraph(ParagraphBlock{Segments: []TextSegment{{Text: block.Caption}}, Style: TextStyle{Bold: true, Align: "C"}})
		}
		rows := append([]TableRow{}, block.Header...)
		rows = append(rows, block.Body...)
		rows = append(rows, block.Footer...)
		colCount := tableColumnCount(block, rows)
		if colCount <= 0 {
			return
		}
		widths := tableRenderWidths(block, rows, r.contentWidth(), colCount)
		for _, row := range rows {
			r.renderTableRow(row, widths)
		}
	})
}

func (r *documentRenderer) renderTableRow(row TableRow, widths []float64) {
	if len(widths) == 0 {
		return
	}
	x, y := r.pdf.GetXY()
	rowHeight := MeasureBlock(NewMeasureContext(r.pdf, r.contentWidth()), TableBlock{Body: []TableRow{row}}).Height
	if rowHeight <= 0 {
		rowHeight = 6
	}
	if rowHeight > r.availableHeight() && r.pdf.GetY() > r.pdf.tMargin {
		r.pdf.AddPage()
		r.renderHeader()
		x, y = r.pdf.GetXY()
	}
	col := 0
	for _, cell := range row.Cells {
		if col >= len(widths) {
			break
		}
		span := cell.ColSpan
		if span <= 0 {
			span = 1
		}
		wd := sumFloat64(widths[col:minInt(col+span, len(widths))])
		r.renderTableCell(cell, x, y, wd, rowHeight)
		x += wd
		col += span
	}
	r.pdf.SetXY(r.pdf.lMargin, y+rowHeight)
}

func (r *documentRenderer) renderTableCell(cell TableCell, x, y, wd, ht float64) {
	r.pdf.Rect(x, y, wd, ht, "D")
	r.pdf.SetXY(x+1, y+1)
	oldRight := r.pdf.rMargin
	r.pdf.rMargin = r.pdf.w - x - wd + 1
	if len(cell.Blocks) == 0 {
		r.applyTextStyle(cell.Style)
		r.pdf.MultiCell(wd-2, maxPositive(4, ht-2), "", "", textAlign(cell.Align), false)
	} else {
		r.renderBlocks(cell.Blocks)
	}
	r.pdf.rMargin = oldRight
}

func (r *documentRenderer) renderImage(block ImageBlock) {
	r.renderBox(block.Box, func() {
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
		if len(block.Data) > 0 && block.Format != "" {
			name := fmt.Sprintf("document-image-%p", &block)
			r.pdf.RegisterImageOptionsReader(name, ImageOptions{ImageType: block.Format, ReadDpi: block.DPI > 0}, bytes.NewReader(block.Data))
			r.pdf.ImageOptions(name, x, r.pdf.GetY(), wd, ht, false, ImageOptions{ImageType: block.Format}, 0, "")
		} else if block.Source != "" {
			r.pdf.ImageOptions(block.Source, x, r.pdf.GetY(), wd, ht, false, ImageOptions{ImageType: block.Format}, 0, "")
		} else if block.Alt != "" {
			r.pdf.Rect(x, r.pdf.GetY(), wd, ht, "D")
			r.pdf.MultiCell(wd, 5, block.Alt, "", "C", false)
		}
		r.pdf.SetY(r.pdf.GetY() + ht)
		if len(block.Caption) > 0 {
			r.renderParagraph(ParagraphBlock{Segments: block.Caption, Style: TextStyle{FontSize: 9, Italic: true, Align: "C"}})
		}
	})
}

func (r *documentRenderer) renderSignatureRow(block SignatureRowBlock) {
	r.renderBox(block.Box, func() {
		columns := block.Columns
		if len(columns) == 0 {
			columns = []SignatureColumn{{}}
		}
		gap := 8.0
		wd := (r.contentWidth() - gap*float64(len(columns)-1)) / float64(len(columns))
		y := r.pdf.GetY() + 12
		for i, col := range columns {
			x := r.pdf.lMargin + float64(i)*(wd+gap)
			r.pdf.Line(x, y, x+wd, y)
			r.pdf.SetXY(x, y+2)
			r.applyTextStyle(TextStyle{FontFamily: "Helvetica", FontSize: 9})
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
		if field.Label == "" {
			lines = append(lines, field.Value)
		} else if field.Value == "" {
			lines = append(lines, field.Label)
		} else {
			lines = append(lines, field.Label+": "+field.Value)
		}
	}
	return strings.Join(lines, "\n")
}

func (r *documentRenderer) renderMetadataGrid(block MetadataGridBlock) {
	r.renderBox(block.Box, func() {
		columns := block.Columns
		if columns <= 0 {
			columns = 2
		}
		wd := r.contentWidth() / float64(columns)
		r.applyTextStyle(mergedTextStyle(NewMeasureContext(r.pdf, r.contentWidth()).DefaultStyle, block.Style))
		for i, field := range block.Fields {
			if i > 0 && i%columns == 0 {
				r.pdf.Ln(6)
			}
			text := field.Label
			if field.Value != "" {
				text += ": " + field.Value
			}
			r.pdf.CellFormat(wd, 6, text, "", 0, "L", false, 0, "")
		}
		r.pdf.Ln(6)
	})
}

func (r *documentRenderer) renderQRVerification(block QRVerificationBlock) {
	r.renderBox(block.Box, func() {
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
		r.pdf.Rect(x, y, size, size, "D")
		r.pdf.Line(x, y, x+size, y+size)
		r.pdf.Line(x+size, y, x, y+size)
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
		r.applyTextStyle(mergedTextStyle(NewMeasureContext(r.pdf, r.contentWidth()).DefaultStyle, block.Style))
		r.pdf.MultiCell(r.contentWidth()-size-4, 5, textSegmentsPlainText(segments), "", "L", false)
		if r.pdf.GetY() < y+size {
			r.pdf.SetY(y + size)
		}
	})
}

func (r *documentRenderer) renderBox(box BoxStyle, render func()) {
	r.pdf.Ln(box.Margin.Top)
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
	return "•"
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

func tableColumnCount(table TableBlock, rows []TableRow) int {
	count := len(table.Columns)
	for _, row := range rows {
		rowCount := 0
		for _, cell := range row.Cells {
			span := cell.ColSpan
			if span <= 0 {
				span = 1
			}
			rowCount += span
		}
		if rowCount > count {
			count = rowCount
		}
	}
	return count
}

func tableRenderWidths(table TableBlock, rows []TableRow, total float64, count int) []float64 {
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
