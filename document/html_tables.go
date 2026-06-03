// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type htmlTableType struct {
	attrs         map[string]string
	captionAttrs  map[string]string
	captionTokens []HTMLSegmentType
	rows          []htmlTableRow
}

type htmlTableRow struct {
	attrs  map[string]string
	cells  []htmlTableCell
	header bool
	footer bool
}

type htmlTableCell struct {
	attrs  map[string]string
	tokens []HTMLSegmentType
	text   string
	tag    string
	header bool
}

type htmlTableLayoutRow struct {
	row   htmlTableRow
	cells []htmlTableCellPlacement
}

type htmlTableCellPlacement struct {
	cellIndex int
	row       int
	col       int
	colspan   int
	rowspan   int
}

type htmlTableMeasuredRow struct {
	index int
	row   htmlTableLayoutRow
	cells []htmlTableMeasuredCell
}

type htmlTableMeasuredCell struct {
	placement htmlTableCellPlacement
	style     htmlTextStyle
	align     string
	fill      CSSColorType
	border    htmlBorderStyle
	padding   htmlBoxEdges
	contentWd float64
	text      string
	textHt    float64
}

const htmlMaxTableColumns = 1024

func (html *HTML) writeTable(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
	table, end := parseHTMLTable(tokens, start)
	if len(table.rows) == 0 {
		return end
	}
	pdf := html.pdf
	if len(table.rows) > html.maxTableRows() {
		pdf.SetErrorf("HTML table row count exceeds maximum size")
		return end
	}
	if pdf.GetX() != pdf.lMargin {
		pdf.Ln(lineHt)
	}
	startX := pdf.GetX()
	availableWd := pdf.w - pdf.rMargin - startX
	tableWd, ok := parseHTMLBoxLength(firstNonEmpty(html.styleValue(table.attrs, "width"), table.attrs["width"]), pdf, availableWd)
	if !ok || tableWd <= 0 || tableWd > availableWd {
		tableWd = availableWd
	}
	padding := html.tablePadding(table.attrs, pdf)
	tableBorder := html.borderFromAttrs(table.attrs, pdf, tableWd)
	layoutRows := htmlTableLayoutRows(table.rows)
	colCount := htmlTableLayoutColumnCount(layoutRows)
	if colCount == 0 {
		return end
	}
	colWidths := html.tableColumnWidths(layoutRows, colCount, tableWd, pdf)
	textR, textG, textB := pdf.GetTextColor()
	fillR, fillG, fillB := pdf.GetFillColor()
	drawR, drawG, drawB := pdf.GetDrawColor()
	lineWidth := pdf.GetLineWidth()
	cellMargin := pdf.GetCellMargin()
	defer func() {
		pdf.SetTextColor(textR, textG, textB)
		pdf.SetFillColor(fillR, fillG, fillB)
		pdf.SetDrawColor(drawR, drawG, drawB)
		pdf.SetLineWidth(lineWidth)
		pdf.SetCellMargin(cellMargin)
		html.applyTextStyle(inherited, fallback)
	}()
	pdf.SetCellMargin(0)
	tableEl := HTMLSegmentType{Cat: 'O', Str: "table", Attr: table.attrs}
	tableDecl := html.elementDeclarations(tableEl, cssRules, ancestors...)
	tableBreakBefore := htmlBreakForcesPage(tableDecl["break-before"]) || htmlBreakForcesPage(tableDecl["page-break-before"])
	tableBreakAfter := htmlBreakForcesPage(tableDecl["break-after"]) || htmlBreakForcesPage(tableDecl["page-break-after"])
	tableBreakInsideAvoid := htmlBreakAvoidsInside(tableDecl["break-inside"]) || htmlBreakAvoidsInside(tableDecl["page-break-inside"])
	tableBorderCollapse := html.tableBorderCollapse(tableDecl, table.attrs)
	tableAncestors := appendHTMLAncestors(ancestors, tableEl)
	measuredRows, rowHeights := html.measureTableRows(layoutRows, colWidths, padding, lineHt, inherited, fallback, cssRules, tableAncestors, tableBorder, table.attrs)
	headerRows := htmlTableHeaderMeasuredRows(measuredRows)
	captionHt := html.tableCaptionHeight(table, tableWd, lineHt, inherited, fallback, cssRules, tableAncestors)
	totalTableHt := captionHt + sumFloat64(rowHeights)
	if tableBreakBefore {
		if !html.addPageFormat() {
			return end
		}
		startX = pdf.lMargin
	}
	pageContentHt := pdf.pageBreakTrigger - pdf.tMargin
	if tableBreakInsideAvoid && totalTableHt <= pageContentHt && pdf.y+totalTableHt > pdf.pageBreakTrigger && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
		if !html.addPageFormat() {
			return end
		}
		startX = pdf.lMargin
	}
	if captionHt > 0 {
		html.renderTableCaption(table, startX, tableWd, lineHt, inherited, fallback, cssRules, tableAncestors)
	}
	renderRow := func(measuredRow htmlTableMeasuredRow, rowHt float64, forceTopBorder bool) float64 {
		y := pdf.GetY()
		for _, measuredCell := range measuredRow.cells {
			placement := measuredCell.placement
			x := startX + htmlTableSpanWidth(colWidths, 0, placement.col)
			wd := htmlTableSpanWidth(colWidths, placement.col, placement.colspan)
			cellHt := rowHt
			if placement.rowspan > 1 {
				cellHt = htmlTableSpanHeight(rowHeights, placement.row, placement.rowspan)
			}
			cellBorder := measuredCell.border
			if tableBorderCollapse {
				cellBorder = htmlCollapsedTableCellBorder(cellBorder, placement, forceTopBorder)
			}
			if measuredCell.fill.Set {
				pdf.SetFillColor(measuredCell.fill.R, measuredCell.fill.G, measuredCell.fill.B)
			}
			htmlDrawBorderedRect(pdf, x, y, wd, cellHt, cellBorder, htmlBorderRadiusStyle{}, measuredCell.fill.Set, drawR, drawG, drawB, lineWidth)
			html.applyTextStyle(measuredCell.style, fallback)
			textY := y + measuredCell.padding.top + htmlTableVerticalOffset(htmlMaxFloat(cellHt-measuredCell.padding.top-measuredCell.padding.bottom, 0), measuredCell.textHt, measuredCell.style.verticalAlign)
			pdf.SetXY(x+measuredCell.padding.left, textY)
			pdf.MultiCell(measuredCell.contentWd, htmlEffectiveLineHeight(measuredCell.style, lineHt), measuredCell.text, "", measuredCell.align, false)
			pdf.SetXY(x, y)
		}
		pdf.SetXY(startX, y+rowHt)
		return rowHt
	}
	renderRepeatedHeaders := func() bool {
		for headerIndex, headerRow := range headerRows {
			headerHt := rowHeights[headerRow.index]
			if pdf.y+headerHt > pdf.pageBreakTrigger {
				return false
			}
			renderRow(headerRow, headerHt, headerIndex == 0)
		}
		return len(headerRows) > 0
	}
	for rowIndex, layoutRow := range layoutRows {
		rowHt := rowHeights[rowIndex]
		forceTopBorder := rowIndex == 0
		if html.shouldMoveTableRowToAvoidOrphan(layoutRows, rowHeights, rowIndex, pageContentHt) {
			if !html.addPageFormat() {
				return end
			}
			startX = pdf.lMargin
			forceTopBorder = true
			if len(headerRows) > 0 && !layoutRow.row.header {
				renderRepeatedHeaders()
				forceTopBorder = false
			}
		}
		if pdf.y+rowHt > pdf.pageBreakTrigger && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
			if !html.addPageFormat() {
				return end
			}
			startX = pdf.lMargin
			forceTopBorder = true
			if len(headerRows) > 0 && !layoutRow.row.header {
				renderRepeatedHeaders()
				forceTopBorder = false
			}
		}
		renderRow(measuredRows[rowIndex], rowHt, forceTopBorder)
	}
	pdf.Ln(lineHt)
	if tableBreakAfter {
		if !html.addPageFormat() {
			return end
		}
	}
	return end
}

func parseHTMLTable(tokens []HTMLSegmentType, start int) (htmlTableType, int) {
	table := htmlTableType{attrs: tokens[start].Attr, rows: make([]htmlTableRow, 0, htmlTableRowCount(tokens, start+1))}
	var row *htmlTableRow
	section := ""
	for i := start + 1; i < len(tokens); i++ {
		el := tokens[i]
		switch {
		case el.Cat == 'O' && (el.Str == "thead" || el.Str == "tbody" || el.Str == "tfoot"):
			section = el.Str
		case el.Cat == 'C' && (el.Str == "thead" || el.Str == "tbody" || el.Str == "tfoot"):
			section = ""
		case el.Cat == 'O' && el.Str == "caption":
			captionTokens, end := htmlCollectCaptionTokens(tokens, i+1)
			table.captionAttrs = el.Attr
			table.captionTokens = captionTokens
			i = end
		case el.Cat == 'O' && el.Str == "tr":
			row = &htmlTableRow{attrs: el.Attr, cells: make([]htmlTableCell, 0, htmlTableRowCellCount(tokens, i+1)), header: section == "thead", footer: section == "tfoot"}
		case el.Cat == 'C' && el.Str == "tr":
			if row != nil {
				table.rows = append(table.rows, *row)
				row = nil
			}
		case el.Cat == 'O' && (el.Str == "td" || el.Str == "th"):
			if row == nil {
				row = &htmlTableRow{cells: make([]htmlTableCell, 0, 1)}
			}
			cellTokens, end := htmlCollectCellTokens(tokens, i+1)
			row.cells = append(row.cells, htmlTableCell{attrs: el.Attr, tokens: cellTokens, text: htmlPlainText(cellTokens), tag: el.Str, header: el.Str == "th"})
			i = end
		case el.Cat == 'C' && el.Str == "table":
			if row != nil {
				table.rows = append(table.rows, *row)
			}
			table.rows = htmlTableRowsWithFooterLast(table.rows)
			return table, i
		}
	}
	return table, start
}

func htmlTableRowsWithFooterLast(rows []htmlTableRow) []htmlTableRow {
	if len(rows) == 0 {
		return rows
	}
	hasFooter := false
	for _, row := range rows {
		if row.footer {
			hasFooter = true
			break
		}
	}
	if !hasFooter {
		return rows
	}
	ordered := make([]htmlTableRow, 0, len(rows))
	for _, row := range rows {
		if !row.footer {
			ordered = append(ordered, row)
		}
	}
	for _, row := range rows {
		if row.footer {
			ordered = append(ordered, row)
		}
	}
	return ordered
}

func htmlTableRowHasOnlyHeaderCells(row htmlTableRow) bool {
	if len(row.cells) == 0 {
		return false
	}
	for _, cell := range row.cells {
		if !cell.header {
			return false
		}
	}
	return true
}

func htmlTableHeaderMeasuredRows(rows []htmlTableMeasuredRow) []htmlTableMeasuredRow {
	var headers []htmlTableMeasuredRow
	for _, row := range rows {
		if row.row.row.header {
			headers = append(headers, row)
		}
	}
	if len(headers) > 0 {
		return headers
	}
	for _, row := range rows {
		if !htmlTableRowHasOnlyHeaderCells(row.row.row) {
			break
		}
		headers = append(headers, row)
	}
	return headers
}

func htmlCollectCellTokens(tokens []HTMLSegmentType, start int) ([]HTMLSegmentType, int) {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'C' && (tokens[i].Str == "td" || tokens[i].Str == "th") {
			return tokens[start:i], i
		}
	}
	return tokens[start:], len(tokens) - 1
}

func htmlTableRowCellCount(tokens []HTMLSegmentType, start int) int {
	count := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'C' && tokens[i].Str == "tr" {
			return count
		}
		if tokens[i].Cat == 'O' && (tokens[i].Str == "td" || tokens[i].Str == "th") {
			count++
		}
	}
	return count
}

func htmlTableRowCount(tokens []HTMLSegmentType, start int) int {
	count := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" {
			return count
		}
		if tokens[i].Cat == 'O' && tokens[i].Str == "tr" {
			count++
		}
	}
	return count
}

func htmlCollectCaptionTokens(tokens []HTMLSegmentType, start int) ([]HTMLSegmentType, int) {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'C' && tokens[i].Str == "caption" {
			return tokens[start:i], i
		}
	}
	return tokens[start:], len(tokens) - 1
}

func htmlTableLayoutRows(rows []htmlTableRow) []htmlTableLayoutRow {
	layoutRows := make([]htmlTableLayoutRow, 0, len(rows))
	var occupied []int
	for rowIndex, row := range rows {
		layoutRow := htmlTableLayoutRow{row: row, cells: make([]htmlTableCellPlacement, 0, len(row.cells))}
		col := 0
		for cellIndex, cell := range row.cells {
			for col < len(occupied) && occupied[col] > 0 {
				col++
			}
			if col >= htmlMaxTableColumns {
				break
			}
			colspan := htmlTableColspan(cell.attrs)
			if col+colspan > htmlMaxTableColumns {
				colspan = htmlMaxTableColumns - col
			}
			rowspan := htmlTableRowspan(cell.attrs)
			endCol := col + colspan
			if endCol > len(occupied) {
				oldLen := len(occupied)
				if cap(occupied) < endCol {
					next := make([]int, endCol)
					copy(next, occupied)
					occupied = next
				} else {
					occupied = occupied[:endCol]
					clear(occupied[oldLen:])
				}
			}
			layoutRow.cells = append(layoutRow.cells, htmlTableCellPlacement{cellIndex: cellIndex, row: rowIndex, col: col, colspan: colspan, rowspan: rowspan})
			for j := col; j < endCol; j++ {
				if rowspan > occupied[j] {
					occupied[j] = rowspan
				}
			}
			col = endCol
		}
		layoutRows = append(layoutRows, layoutRow)
		for j := range occupied {
			if occupied[j] > 0 {
				occupied[j]--
			}
		}
		for len(occupied) > 0 && occupied[len(occupied)-1] == 0 {
			occupied = occupied[:len(occupied)-1]
		}
	}
	return layoutRows
}

func htmlTableLayoutColumnCount(rows []htmlTableLayoutRow) int {
	count := 0
	for _, row := range rows {
		for _, cell := range row.cells {
			cellEnd := cell.col + cell.colspan
			if cellEnd >= htmlMaxTableColumns {
				cellEnd = htmlMaxTableColumns
				if cellEnd > count {
					count = cellEnd
				}
				break
			}
			if cellEnd > count {
				count = cellEnd
			}
		}
	}
	return count
}

func htmlTableColumnWidths(rows []htmlTableLayoutRow, count int, tableWd float64, pdf *Document) []float64 {
	return htmlTableColumnWidthsWithStyleValue(rows, count, tableWd, pdf, htmlStyleValue)
}

func (html *HTML) tableColumnWidths(rows []htmlTableLayoutRow, count int, tableWd float64, pdf *Document) []float64 {
	return htmlTableColumnWidthsWithStyleValue(rows, count, tableWd, pdf, html.styleValue)
}

func htmlTableColumnWidthsWithStyleValue(rows []htmlTableLayoutRow, count int, tableWd float64, pdf *Document, styleValue func(map[string]string, string) string) []float64 {
	widths := make([]float64, count)
	specified := make([]bool, count)
	minWidths := make([]float64, count)
	for _, row := range rows {
		for _, cell := range row.cells {
			if cell.col >= count {
				continue
			}
			span := minInt(cell.colspan, count-cell.col)
			if span <= 0 {
				continue
			}
			cellDef := row.row.cells[cell.cellIndex]
			minWd := htmlTableCellMinWidth(cellDef, pdf)
			htmlApplyTableSpanMinimum(minWidths, cell.col, span, minWd)
			if wd, ok := parseHTMLBoxLength(firstNonEmpty(styleValue(cellDef.attrs, "width"), cellDef.attrs["width"]), pdf, tableWd); ok {
				htmlApplyTableSpanWidth(widths, specified, cell.col, span, wd)
			}
		}
	}
	fixedWd := 0.0
	flexibleCount := 0
	flexibleMinWd := 0.0
	for j, wd := range widths {
		if specified[j] {
			if wd < minWidths[j] {
				widths[j] = minWidths[j]
			}
			fixedWd += widths[j]
		} else {
			flexibleCount++
			flexibleMinWd += minWidths[j]
		}
	}
	remaining := htmlMaxFloat(tableWd-fixedWd, 0)
	if flexibleCount > 0 {
		if flexibleMinWd > 0 && flexibleMinWd >= remaining {
			scale := remaining / flexibleMinWd
			for j := range widths {
				if !specified[j] {
					widths[j] = minWidths[j] * scale
				}
			}
		} else {
			extra := 0.0
			if remaining > flexibleMinWd {
				extra = (remaining - flexibleMinWd) / float64(flexibleCount)
			}
			for j := range widths {
				if !specified[j] {
					widths[j] = minWidths[j] + extra
				}
			}
		}
		return widths
	}
	total := sumFloat64(widths)
	if total <= 0 {
		defaultWd := 0.0
		if count > 0 {
			defaultWd = tableWd / float64(count)
		}
		for j := range widths {
			widths[j] = defaultWd
		}
		return widths
	}
	for j := range widths {
		widths[j] = widths[j] * tableWd / total
	}
	return widths
}

func (html *HTML) shouldMoveTableRowToAvoidOrphan(rows []htmlTableLayoutRow, heights []float64, index int, pageContentHt float64) bool {
	if html == nil || html.pdf == nil || index+1 >= len(rows) || index >= len(heights) || index+1 >= len(heights) {
		return false
	}
	current := rows[index]
	next := rows[index+1]
	if current.row.header || current.row.footer || next.row.header || next.row.footer {
		return false
	}
	rowHt := heights[index]
	nextHt := heights[index+1]
	pairHt := rowHt + nextHt
	remaining := html.pdf.pageBreakTrigger - html.pdf.y
	return rowHt <= remaining && pairHt > remaining && pairHt <= pageContentHt && !html.pdf.inHeader && !html.pdf.inFooter && html.pdf.acceptPageBreak()
}

func htmlTableCellMinWidth(cell htmlTableCell, pdf *Document) float64 {
	if pdf == nil {
		return 0
	}
	return htmlMaxWordWidth(pdf, cell.text)
}

func htmlTableCellText(cell htmlTableCell, preserveWhitespace bool) string {
	if preserveWhitespace {
		return htmlPlainTextWithMode(cell.tokens, true)
	}
	return cell.text
}

func htmlMaxWordWidth(pdf *Document, text string) float64 {
	maxWd := 0.0
	start := -1
	for i, r := range text {
		if unicode.IsSpace(r) {
			if start >= 0 {
				if wd := pdf.GetStringWidth(text[start:i]); wd > maxWd {
					maxWd = wd
				}
			}
			start = -1
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		if wd := pdf.GetStringWidth(text[start:]); wd > maxWd {
			maxWd = wd
		}
	}
	return maxWd
}

func htmlApplyTableSpanMinimum(widths []float64, start, span int, target float64) {
	if target <= 0 || span <= 0 {
		return
	}
	current := htmlTableSpanWidth(widths, start, span)
	if current >= target {
		return
	}
	extra := (target - current) / float64(span)
	for j := start; j < start+span && j < len(widths); j++ {
		widths[j] += extra
	}
}

func htmlApplyTableSpanWidth(widths []float64, specified []bool, start, span int, target float64) {
	if target <= 0 || span <= 0 {
		return
	}
	current := htmlTableSpanWidth(widths, start, span)
	if current >= target {
		for j := start; j < start+span && j < len(specified); j++ {
			specified[j] = true
		}
		return
	}
	var indexes []int
	for j := start; j < start+span && j < len(widths); j++ {
		if !specified[j] {
			indexes = append(indexes, j)
		}
	}
	if len(indexes) == 0 {
		for j := start; j < start+span && j < len(widths); j++ {
			indexes = append(indexes, j)
		}
	}
	extra := (target - current) / float64(len(indexes))
	for _, j := range indexes {
		widths[j] += extra
		specified[j] = true
	}
}

func (html *HTML) measureTableRows(rows []htmlTableLayoutRow, widths []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, tableAttrs map[string]string) ([]htmlTableMeasuredRow, []float64) {
	measuredRows := make([]htmlTableMeasuredRow, len(rows))
	heights := make([]float64, len(rows))
	for i := range heights {
		heights[i] = lineHt + 2*padding
	}
	tableFill := html.cellBackground(tableAttrs)
	for rowIndex, row := range rows {
		measuredRow := htmlTableMeasuredRow{index: rowIndex, row: row, cells: make([]htmlTableMeasuredCell, 0, len(row.cells))}
		rowEl := HTMLSegmentType{Cat: 'O', Str: "tr", Attr: row.row.attrs}
		rowAncestors := appendHTMLAncestors(tableAncestors, rowEl)
		rowFill := html.cellBackground(row.row.attrs)
		for _, placement := range row.cells {
			measuredCell := html.measureTableCell(row, placement, widths, padding, lineHt, inherited, fallback, cssRules, rowAncestors, tableBorder, rowFill, tableFill)
			measuredRow.cells = append(measuredRow.cells, measuredCell)
			required := measuredCell.textHt + measuredCell.padding.top + measuredCell.padding.bottom
			span := placement.rowspan
			if span < 1 {
				span = 1
			}
			if rowIndex+span > len(heights) {
				span = len(heights) - rowIndex
			}
			if span <= 1 {
				if required > heights[rowIndex] {
					heights[rowIndex] = required
				}
				continue
			}
			current := htmlTableSpanHeight(heights, rowIndex, span)
			if required > current {
				extra := (required - current) / float64(span)
				for i := rowIndex; i < rowIndex+span; i++ {
					heights[i] += extra
				}
			}
		}
		measuredRows[rowIndex] = measuredRow
	}
	return measuredRows, heights
}

func (html *HTML) measureTableCell(row htmlTableLayoutRow, placement htmlTableCellPlacement, widths []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, rowAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, rowFill, tableFill CSSColorType) htmlTableMeasuredCell {
	cell := row.row.cells[placement.cellIndex]
	style := inherited
	if cell.header {
		style.bold = true
		if style.align == "" || style.align == "L" {
			style.align = "C"
		}
	}
	applyHTMLCSSRules(&style, HTMLSegmentType{Cat: 'O', Str: cell.tag, Attr: cell.attrs}, cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, rowAncestors...)
	html.applyAttrs(&style, cell.attrs, inherited.fontSize, inherited.lineHeight, html.pdf)
	html.applyTextStyle(style, fallback)
	wd := htmlTableSpanWidth(widths, placement.col, placement.colspan)
	cellPadding := html.cellPadding(cell.attrs, html.pdf, padding, wd)
	contentWd := htmlMaxFloat(wd-cellPadding.left-cellPadding.right, 0)
	text := htmlTableCellText(cell, style.preserveWhitespace)
	return htmlTableMeasuredCell{
		placement: placement,
		style:     style,
		align:     html.cellAlign(cell.attrs, style.align),
		fill:      htmlTableBackground(firstColor(html.cellBackground(cell.attrs), rowFill, tableFill)),
		border:    html.tableCellBorder(tableBorder, cell.attrs, row.row.attrs, html.pdf, wd),
		padding:   cellPadding,
		contentWd: contentWd,
		text:      text,
		textHt:    html.tableCellTextHeight(text, contentWd, style, lineHt),
	}
}

func (html *HTML) tableRowHeights(rows []htmlTableLayoutRow, widths []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType) []float64 {
	heights := make([]float64, len(rows))
	for i := range heights {
		heights[i] = lineHt + 2*padding
	}
	for rowIndex, row := range rows {
		for _, cell := range row.cells {
			required := html.tableCellHeight(row, cell, widths, padding, lineHt, inherited, fallback, cssRules, tableAncestors)
			span := cell.rowspan
			if span < 1 {
				span = 1
			}
			if rowIndex+span > len(heights) {
				span = len(heights) - rowIndex
			}
			if span <= 1 {
				if required > heights[rowIndex] {
					heights[rowIndex] = required
				}
				continue
			}
			current := htmlTableSpanHeight(heights, rowIndex, span)
			if required > current {
				extra := (required - current) / float64(span)
				for i := rowIndex; i < rowIndex+span; i++ {
					heights[i] += extra
				}
			}
		}
	}
	return heights
}

func (html *HTML) tableCaptionHeight(table htmlTableType, tableWd, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType) float64 {
	if len(table.captionTokens) == 0 {
		return 0
	}
	style := inherited
	if style.align == "" || style.align == "L" {
		style.align = "C"
	}
	captionEl := HTMLSegmentType{Cat: 'O', Str: "caption", Attr: table.captionAttrs}
	applyHTMLCSSRules(&style, captionEl, cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, tableAncestors...)
	html.applyAttrs(&style, table.captionAttrs, inherited.fontSize, inherited.lineHeight, html.pdf)
	html.applyTextStyle(style, fallback)
	text := htmlPlainTextWithMode(table.captionTokens, style.preserveWhitespace)
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return html.tableCellTextHeight(text, tableWd, style, lineHt) + htmlEffectiveLineHeight(style, lineHt)*0.5
}

func (html *HTML) renderTableCaption(table htmlTableType, x, tableWd, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType) {
	style := inherited
	if style.align == "" || style.align == "L" {
		style.align = "C"
	}
	captionEl := HTMLSegmentType{Cat: 'O', Str: "caption", Attr: table.captionAttrs}
	applyHTMLCSSRules(&style, captionEl, cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, tableAncestors...)
	html.applyAttrs(&style, table.captionAttrs, inherited.fontSize, inherited.lineHeight, html.pdf)
	html.applyTextStyle(style, fallback)
	text := htmlPlainTextWithMode(table.captionTokens, style.preserveWhitespace)
	if strings.TrimSpace(text) == "" {
		return
	}
	html.pdf.SetX(x)
	html.pdf.MultiCell(tableWd, htmlEffectiveLineHeight(style, lineHt), text, "", style.align, false)
	html.pdf.Ln(htmlEffectiveLineHeight(style, lineHt) * 0.5)
}

func (html *HTML) tableCellHeight(row htmlTableLayoutRow, placement htmlTableCellPlacement, widths []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType) float64 {
	cell := row.row.cells[placement.cellIndex]
	style := inherited
	if cell.header {
		style.bold = true
	}
	rowEl := HTMLSegmentType{Cat: 'O', Str: "tr", Attr: row.row.attrs}
	rowAncestors := appendHTMLAncestors(tableAncestors, rowEl)
	applyHTMLCSSRules(&style, HTMLSegmentType{Cat: 'O', Str: cell.tag, Attr: cell.attrs}, cssRules, inherited.fontSize, inherited.lineHeight, html.pdf, rowAncestors...)
	html.applyAttrs(&style, cell.attrs, inherited.fontSize, inherited.lineHeight, html.pdf)
	html.applyTextStyle(style, fallback)
	cellWd := htmlTableSpanWidth(widths, placement.col, placement.colspan)
	cellPadding := html.cellPadding(cell.attrs, html.pdf, padding, cellWd)
	textWd := htmlMaxFloat(cellWd-cellPadding.left-cellPadding.right, 0)
	return html.tableCellTextHeight(htmlTableCellText(cell, style.preserveWhitespace), textWd, style, lineHt) + cellPadding.top + cellPadding.bottom
}

func (html *HTML) tableCellTextHeight(text string, wd float64, style htmlTextStyle, lineHt float64) float64 {
	lineCount := htmlSplitLineCount(html.pdf, text, wd)
	return float64(lineCount) * htmlEffectiveLineHeight(style, lineHt)
}

func htmlTableVerticalOffset(contentHt, textHt float64, verticalAlign string) float64 {
	if contentHt <= textHt {
		return 0
	}
	switch verticalAlign {
	case "middle":
		return (contentHt - textHt) / 2
	case "bottom":
		return contentHt - textHt
	default:
		return 0
	}
}

func htmlSplitLines(pdf *Document, text string, wd float64) []string {
	if text == "" {
		return []string{""}
	}
	if pdf.isCurrentUTF8 {
		lines := pdf.SplitText(text, wd)
		if len(lines) == 0 {
			return []string{""}
		}
		return htmlWrapLongLines(pdf, lines, wd)
	}
	if !strings.Contains(text, "\r") {
		lines := htmlSplitStringLines(pdf, text, wd)
		if len(lines) == 0 {
			return []string{""}
		}
		return htmlWrapLongLines(pdf, lines, wd)
	}
	lines := pdf.SplitLines([]byte(text), wd)
	if len(lines) == 0 {
		return []string{""}
	}
	out := make([]string, len(lines))
	for j, line := range lines {
		out[j] = string(line)
	}
	return htmlWrapLongLines(pdf, out, wd)
}

func htmlSplitLineCount(pdf *Document, text string, wd float64) int {
	if text == "" {
		return 1
	}
	if pdf.isCurrentUTF8 || strings.Contains(text, "\r") {
		return len(htmlSplitLines(pdf, text, wd))
	}
	count := htmlSplitStringLineCount(pdf, text, wd)
	if count == 0 {
		return 1
	}
	return count
}

func htmlSplitStringLines(pdf *Document, text string, wd float64) []string {
	lines := []string{}
	cw := pdf.currentFont.Cw
	wmax := int(math.Ceil((wd - 2*pdf.cMargin) * 1000 / pdf.fontSize))
	nb := len(text)
	for nb > 0 && text[nb-1] == '\n' {
		nb--
	}
	text = text[:nb]
	sep := -1
	i := 0
	j := 0
	l := 0
	for i < nb {
		c := text[i]
		l += cw[c]
		if c == ' ' || c == '\t' || c == '\n' {
			sep = i
		}
		if c == '\n' || l > wmax {
			if sep == -1 {
				if i == j {
					i++
				}
				sep = i
			} else {
				i = sep + 1
			}
			lines = append(lines, text[j:sep])
			sep = -1
			j = i
			l = 0
		} else {
			i++
		}
	}
	if i != j {
		lines = append(lines, text[j:i])
	}
	return lines
}

func htmlSplitStringLineCount(pdf *Document, text string, wd float64) int {
	count := 0
	cw := pdf.currentFont.Cw
	wmax := int(math.Ceil((wd - 2*pdf.cMargin) * 1000 / pdf.fontSize))
	nb := len(text)
	for nb > 0 && text[nb-1] == '\n' {
		nb--
	}
	text = text[:nb]
	sep := -1
	i := 0
	j := 0
	l := 0
	for i < nb {
		c := text[i]
		l += cw[c]
		if c == ' ' || c == '\t' || c == '\n' {
			sep = i
		}
		if c == '\n' || l > wmax {
			if sep == -1 {
				if i == j {
					i++
				}
				sep = i
			} else {
				i = sep + 1
			}
			count += htmlWrappedLineCount(pdf, text[j:sep], wd)
			sep = -1
			j = i
			l = 0
		} else {
			i++
		}
	}
	if i != j {
		count += htmlWrappedLineCount(pdf, text[j:i], wd)
	}
	return count
}

func htmlWrapLongLines(pdf *Document, lines []string, wd float64) []string {
	if wd <= 0 {
		return lines
	}
	var out []string
	for index, line := range lines {
		if line == "" || pdf.GetStringWidth(line) <= wd {
			if out != nil {
				out = append(out, line)
			}
			continue
		}
		if out == nil {
			out = make([]string, 0, len(lines)+1)
			out = append(out, lines[:index]...)
		}
		start := 0
		currentEnd := 0
		currentWidth := 0.0
		for offset, r := range line {
			size := utf8.RuneLen(r)
			if size < 0 {
				size = 1
			}
			nextEnd := offset + size
			runeWidth := pdf.GetStringWidth(line[offset:nextEnd])
			if currentEnd > start && currentWidth+runeWidth > wd {
				out = append(out, line[start:currentEnd])
				start = offset
				currentWidth = runeWidth
				currentEnd = nextEnd
				continue
			}
			currentWidth += runeWidth
			currentEnd = nextEnd
		}
		if currentEnd > start {
			out = append(out, line[start:currentEnd])
		}
	}
	if out == nil {
		return lines
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func htmlWrappedLineCount(pdf *Document, line string, wd float64) int {
	if wd <= 0 || line == "" || pdf.GetStringWidth(line) <= wd {
		return 1
	}
	count := 0
	start := 0
	currentEnd := 0
	currentWidth := 0.0
	for offset, r := range line {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		nextEnd := offset + size
		runeWidth := pdf.GetStringWidth(line[offset:nextEnd])
		if currentEnd > start && currentWidth+runeWidth > wd {
			count++
			start = offset
			currentWidth = runeWidth
			currentEnd = nextEnd
			continue
		}
		currentWidth += runeWidth
		currentEnd = nextEnd
	}
	if currentEnd > start {
		count++
	}
	if count == 0 {
		return 1
	}
	return count
}

func htmlTableColspan(attrs map[string]string) int {
	value := strings.TrimSpace(attrs["colspan"])
	if value == "" {
		return 1
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 1
	}
	if n > htmlMaxTableColumns {
		return htmlMaxTableColumns
	}
	return n
}

func htmlTableRowspan(attrs map[string]string) int {
	value := strings.TrimSpace(attrs["rowspan"])
	if value == "" {
		return 1
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 1
	}
	if n > htmlMaxTableColumns {
		return htmlMaxTableColumns
	}
	return n
}

func htmlTableSpanWidth(widths []float64, start, span int) float64 {
	wd := 0.0
	for j := start; j < start+span && j < len(widths); j++ {
		wd += widths[j]
	}
	return wd
}

func htmlTableSpanHeight(heights []float64, start, span int) float64 {
	ht := 0.0
	for j := start; j < start+span && j < len(heights); j++ {
		ht += heights[j]
	}
	return ht
}

func sumFloat64(values []float64) float64 {
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum
}

func (html *HTML) tablePadding(attrs map[string]string, pdf *Document) float64 {
	if wd, ok := parseHTMLBoxLength(firstNonEmpty(html.styleValue(attrs, "padding"), attrs["cellpadding"]), pdf, 0); ok {
		return wd
	}
	return 1.5
}

func htmlTableCellPadding(attrs map[string]string, pdf *Document, fallback, relative float64) htmlBoxEdges {
	return htmlTableCellPaddingFromStyle(parseStyleDeclarations(attrs["style"]), pdf, fallback, relative)
}

func (html *HTML) cellPadding(attrs map[string]string, pdf *Document, fallback, relative float64) htmlBoxEdges {
	return htmlTableCellPaddingFromStyle(html.styleDeclarations(attrs), pdf, fallback, relative)
}

func htmlTableCellPaddingFromStyle(decl map[string]string, pdf *Document, fallback, relative float64) htmlBoxEdges {
	edges := htmlBoxEdges{top: fallback, right: fallback, bottom: fallback, left: fallback}
	if !htmlHasBoxEdgeDeclaration(decl, "padding") {
		return edges
	}
	parsed := htmlBoxEdgesFromDeclarations(decl, "padding", pdf, relative)
	if strings.TrimSpace(decl["padding"]) != "" {
		edges = parsed
	}
	for _, side := range []struct {
		name string
		set  func(float64)
	}{{"top", func(v float64) {
		edges.top = v
	}}, {"right", func(v float64) {
		edges.right = v
	}}, {"bottom", func(v float64) {
		edges.bottom = v
	}}, {"left", func(v float64) {
		edges.left = v
	}}} {
		if value, ok := parseHTMLBoxLength(decl["padding-"+side.name], pdf, relative); ok {
			side.set(value)
		}
	}
	return edges
}

func (html *HTML) cellAlign(attrs map[string]string, fallback string) string {
	align := strings.ToLower(firstNonEmpty(html.styleValue(attrs, "text-align"), attrs["align"]))
	switch align {
	case "center":
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

func (html *HTML) cellBackground(attrs map[string]string) CSSColorType {
	if color, ok := parseCSSColor(firstNonEmpty(html.styleValue(attrs, "background-color"), html.styleValue(attrs, "background"), attrs["bgcolor"])); ok {
		return color
	}
	return CSSColorType{}
}

func (html *HTML) tableCellBorder(fallback htmlBorderStyle, cellAttrs, rowAttrs map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	border := fallback
	border = html.tableCellBorderFromAttrs(border, rowAttrs, pdf, relative)
	border = html.tableCellBorderFromAttrs(border, cellAttrs, pdf, relative)
	return border
}

func (html *HTML) tableCellBorderFromAttrs(fallback htmlBorderStyle, attrs map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	if !htmlAttrsMayAffectCellBorder(html.styleDeclarations(attrs), attrs) {
		return fallback
	}
	next := html.borderFromAttrs(attrs, pdf, relative)
	if next.hasAny() {
		return next
	}
	if next.color.Set {
		fallback.color = next.color
		fallback.top.color = next.color
		fallback.right.color = next.color
		fallback.bottom.color = next.color
		fallback.left.color = next.color
	}
	return fallback
}

func htmlAttrsMayAffectCellBorder(decl map[string]string, attrs map[string]string) bool {
	if attrs == nil {
		return false
	}
	if strings.TrimSpace(attrs["border"]) != "" || strings.TrimSpace(attrs["bordercolor"]) != "" {
		return true
	}
	for name, value := range decl {
		if strings.HasPrefix(name, "border") && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func htmlTableBorderCollapse(decl map[string]string, attrs map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(firstNonEmpty(decl["border-collapse"], htmlStyleValue(attrs, "border-collapse"))), "collapse")
}

func (html *HTML) tableBorderCollapse(decl map[string]string, attrs map[string]string) bool {
	return strings.EqualFold(strings.TrimSpace(firstNonEmpty(decl["border-collapse"], html.styleValue(attrs, "border-collapse"))), "collapse")
}

func htmlCollapsedTableCellBorder(border htmlBorderStyle, placement htmlTableCellPlacement, forceTop bool) htmlBorderStyle {
	if !border.hasAny() {
		return border
	}
	border.sideSpecific = true
	if placement.row > 0 && !forceTop {
		border.top.enabled = false
	}
	if placement.col > 0 {
		border.left.enabled = false
	}
	border.enabled = border.hasAny()
	return border
}

func htmlCellBorderColor(attrSets ...map[string]string) CSSColorType {
	for _, attrs := range attrSets {
		decl := parseStyleDeclarations(attrs["style"])
		if color, ok := htmlBorderColor(firstNonEmpty(decl["border-color"], decl["border"], attrs["bordercolor"])); ok {
			return color
		}
	}
	return CSSColorType{}
}

func firstColor(colors ...CSSColorType) CSSColorType {
	for _, color := range colors {
		if color.Set && !color.None {
			return color
		}
	}
	return CSSColorType{}
}

func htmlTableBackground(color CSSColorType) CSSColorType {
	if color.Set && !color.None {
		return color
	}
	return CSSColorType{}
}
