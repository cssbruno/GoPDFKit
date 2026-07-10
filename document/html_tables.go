// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/layoutgeom"
)

type htmlTableType struct {
	attrs            map[string]string
	start            int
	end              int
	captionStart     int
	captionEnd       int
	captionAttrs     map[string]string
	captionTokens    []HTMLSegmentType
	captionText      string
	captionPreserved string
	rows             []htmlTableRow
}

type htmlTableRow struct {
	attrs  map[string]string
	cells  []htmlTableCell
	header bool
	footer bool
	start  int
	end    int
}

type htmlTableCell struct {
	attrs         map[string]string
	tokens        []HTMLSegmentType
	text          string
	textPreserved string
	tag           string
	header        bool
	start         int
	end           int
	colspan       int
	rowspan       int
	widthHint     string
	alignHint     string
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
	placement  htmlTableCellPlacement
	appearance *htmlTableMeasuredCellAppearance
	contentWd  float64
	text       string
	lines      []string
	textHt     float64
}

type htmlTableMeasuredCellAppearance struct {
	style   htmlTextStyle
	align   string
	fill    CSSColorType
	border  htmlBorderStyle
	padding htmlBoxEdges
}

type htmlTableMeasuredCellAppearanceCache struct {
	lastValue htmlTableMeasuredCellAppearance
	last      *htmlTableMeasuredCellAppearance
	values    map[htmlTableMeasuredCellAppearance]*htmlTableMeasuredCellAppearance
}

func (cache *htmlTableMeasuredCellAppearanceCache) intern(value htmlTableMeasuredCellAppearance) *htmlTableMeasuredCellAppearance {
	if cache.last != nil && cache.lastValue == value {
		return cache.last
	}
	if cached := cache.values[value]; cached != nil {
		cache.lastValue = value
		cache.last = cached
		return cached
	}
	appearance := new(htmlTableMeasuredCellAppearance)
	*appearance = value
	if len(cache.values) >= htmlTableMeasuredCellAppearanceCacheLimit {
		cache.lastValue = value
		cache.last = appearance
		return appearance
	}
	if cache.values == nil {
		cache.values = make(map[htmlTableMeasuredCellAppearance]*htmlTableMeasuredCellAppearance, 4)
	}
	cache.values[value] = appearance
	cache.lastValue = value
	cache.last = appearance
	return appearance
}

type htmlTableCellStyleCacheKey struct {
	cellStyle       string
	cellDecl        string
	cellAlign       string
	cellBgColor     string
	cellBorder      string
	cellBorderColor string
	rowStyle        string
	rowDecl         string
	rowBgColor      string
	rowBorder       string
	rowBorderColor  string
	alignFallback   string
	paddingFallback float64
	relative        float64
	rowFill         CSSColorType
	tableFill       CSSColorType
}

type htmlTableCellStyleCacheValue struct {
	align   string
	fill    CSSColorType
	border  htmlBorderStyle
	padding htmlBoxEdges
}

type htmlTableCellStyleCache struct {
	lastKey   htmlTableCellStyleCacheKey
	lastValue htmlTableCellStyleCacheValue
	hasLast   bool
	values    map[htmlTableCellStyleCacheKey]htmlTableCellStyleCacheValue
}

const (
	htmlMaxTableColumns                       = 1024
	htmlTableMinWidthCacheLimit               = 256
	htmlTableCellStyleCacheLimit              = 256
	htmlTableMeasuredCellAppearanceCacheLimit = 256
	htmlTableStreamingRowLimit                = 2048
)

func (html *HTML) writeTable(tokens []HTMLSegmentType, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
	table, end := parseHTMLTable(tokens, start)
	return html.writeParsedTable(nil, table, end, lineHt, inherited, fallback, cssRules, ancestors)
}

func (html *HTML) writeCompiledTable(compiled *CompiledHTML, start int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
	table, end, ok := compiled.table(start)
	if !ok {
		return html.writeTable(compiled.tokens, start, lineHt, inherited, fallback, cssRules, ancestors)
	}
	return html.writeParsedTable(compiled, table, end, lineHt, inherited, fallback, cssRules, ancestors)
}

func (html *HTML) tableElementDeclarations(compiled *CompiledHTML, tokenIndex int, el HTMLSegmentType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) map[string]string {
	if declarations, ok := compiled.declarations(tokenIndex); ok {
		return declarations
	}
	return html.elementDeclarations(el, cssRules, ancestors...)
}

func (html *HTML) writeParsedTable(compiled *CompiledHTML, table htmlTableType, end int, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, ancestors []HTMLSegmentType) int {
	if len(table.rows) == 0 {
		return end
	}
	pdf := html.pdf
	pdf.BeginStructure(taggedRoleTable)
	defer pdf.EndStructure()
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
	colOffsets := layoutgeom.TrackOffsets(colWidths)
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
	tableDecl := html.tableElementDeclarations(compiled, table.start, tableEl, cssRules, ancestors)
	tableBreakBefore := htmlBreakForcesPage(tableDecl["break-before"]) || htmlBreakForcesPage(tableDecl["page-break-before"])
	tableBreakAfter := htmlBreakForcesPage(tableDecl["break-after"]) || htmlBreakForcesPage(tableDecl["page-break-after"])
	tableBreakInsideAvoid := htmlBreakAvoidsInside(tableDecl["break-inside"]) || htmlBreakAvoidsInside(tableDecl["page-break-inside"])
	tableBorderCollapse := html.tableBorderCollapse(tableDecl, table.attrs)
	tableAncestors := appendHTMLAncestors(ancestors, tableEl)
	headerRows := htmlTableHeaderRowIndexes(layoutRows)
	captionHt := html.tableCaptionHeight(compiled, table, tableWd, lineHt, inherited, fallback, cssRules, tableAncestors)
	var measuredRows []htmlTableMeasuredRow
	var rowHeights []float64
	streamRows := html.shouldStreamTableRows(layoutRows, headerRows, captionHt, tableBreakInsideAvoid)
	if streamRows {
		rowHeights = html.measureTableRowHeights(compiled, layoutRows, colOffsets, padding, lineHt, inherited, fallback, cssRules, tableAncestors, tableBorder, table.attrs)
	} else {
		measuredRows, rowHeights = html.measureTableRows(compiled, layoutRows, colOffsets, padding, lineHt, inherited, fallback, cssRules, tableAncestors, tableBorder, table.attrs)
	}
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
		html.renderTableCaption(compiled, table, startX, tableWd, lineHt, inherited, fallback, cssRules, tableAncestors)
	}
	tableFill := html.cellBackground(table.attrs)
	var renderCellStyleCache htmlTableCellStyleCache
	var renderAppearanceCache htmlTableMeasuredCellAppearanceCache
	renderRow := func(rowIndex int, layoutRow htmlTableLayoutRow, rowHt float64, forceTopBorder bool) float64 {
		var measuredRow htmlTableMeasuredRow
		if measuredRows != nil {
			measuredRow = measuredRows[rowIndex]
		} else {
			measuredRow = html.measureTableRow(compiled, rowIndex, layoutRow, colOffsets, padding, lineHt, inherited, fallback, cssRules, tableAncestors, tableBorder, tableFill, &renderCellStyleCache, &renderAppearanceCache)
		}
		pdf.BeginStructure(taggedRoleTR)
		defer pdf.EndStructure()
		y := pdf.GetY()
		for _, measuredCell := range measuredRow.cells {
			appearance := measuredCell.appearance
			cellRole := taggedRoleTD
			if measuredCell.placement.cellIndex >= 0 && measuredCell.placement.cellIndex < len(measuredRow.row.row.cells) && measuredRow.row.row.cells[measuredCell.placement.cellIndex].header {
				cellRole = taggedRoleTH
			}
			placement := measuredCell.placement
			pdf.beginTableCellStructure(cellRole, taggedTableAttributes{
				Scope:   htmlTableCellScope(cellRole, measuredRow.row.row, placement),
				RowSpan: placement.rowspan,
				ColSpan: placement.colspan,
			})
			x := startX + layoutgeom.SpanSize(colOffsets, 0, placement.col)
			wd := layoutgeom.SpanSize(colOffsets, placement.col, placement.colspan)
			cellHt := rowHt
			if placement.rowspan > 1 {
				cellHt = layoutgeom.SumSpan(rowHeights, placement.row, placement.rowspan)
			}
			cellBorder := appearance.border
			if tableBorderCollapse {
				cellBorder = htmlCollapsedTableCellBorder(cellBorder, placement, forceTopBorder)
			}
			if appearance.fill.Set {
				pdf.SetFillColor(appearance.fill.R, appearance.fill.G, appearance.fill.B)
			}
			htmlDrawBorderedRect(pdf, x, y, wd, cellHt, cellBorder, htmlBorderRadiusStyle{}, appearance.fill.Set, drawR, drawG, drawB, lineWidth)
			html.applyTextStyle(appearance.style, fallback)
			textY := y + appearance.padding.top + htmlTableVerticalOffset(htmlMaxFloat(cellHt-appearance.padding.top-appearance.padding.bottom, 0), measuredCell.textHt, appearance.style.verticalAlign)
			pdf.SetXY(x+appearance.padding.left, textY)
			cell := measuredRow.row.row.cells[placement.cellIndex]
			html.renderTableCellContent(cell, measuredCell, cellRole, lineHt, fallback)
			pdf.SetXY(x, y)
			pdf.EndStructure()
		}
		pdf.SetXY(startX, y+rowHt)
		return rowHt
	}
	renderRepeatedHeaders := func() bool {
		for headerIndex, headerRowIndex := range headerRows {
			headerHt := rowHeights[headerRowIndex]
			if pdf.y+headerHt > pdf.pageBreakTrigger {
				return false
			}
			renderRow(headerRowIndex, layoutRows[headerRowIndex], headerHt, headerIndex == 0)
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
		if layoutgeom.ExceedsAvailableHeight(rowHt, pdf.pageBreakTrigger-pdf.y) && !pdf.inHeader && !pdf.inFooter && pdf.acceptPageBreak() {
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
		renderRow(rowIndex, layoutRow, rowHt, forceTopBorder)
	}
	pdf.Ln(lineHt)
	if tableBreakAfter {
		if !html.addPageFormat() {
			return end
		}
	}
	return end
}

func (html *HTML) renderTableCellContent(cell htmlTableCell, measuredCell htmlTableMeasuredCell, cellRole string, lineHt float64, fallback CSSColorType) {
	if !htmlTableCellContainsStructuredContent(cell.tokens) {
		html.renderMeasuredTableCellText(measuredCell, cellRole, lineHt)
		return
	}
	html.renderTableCellStructuredTokens(cell.tokens, measuredCell, cellRole, lineHt, fallback)
}

func (html *HTML) renderMeasuredTableCellText(measuredCell htmlTableMeasuredCell, cellRole string, lineHt float64) {
	appearance := measuredCell.appearance
	lines := measuredCell.lines
	if len(lines) == 0 {
		html.pdf.SetNextTextRole(cellRole)
		html.pdf.CellFormat(measuredCell.contentWd, htmlEffectiveLineHeight(appearance.style, lineHt), measuredCell.text, "", 2, appearance.align, false, 0, "")
		return
	}
	effectiveLineHt := htmlEffectiveLineHeight(appearance.style, lineHt)
	for i, line := range lines {
		if i == 0 {
			html.pdf.SetNextTextRole(cellRole)
		}
		html.pdf.CellFormat(measuredCell.contentWd, effectiveLineHt, line, "", 2, appearance.align, false, 0, "")
	}
}

type htmlTableCellListContext struct {
	bodyX  float64
	bodyWd float64
	startY float64
}

func (html *HTML) renderTableCellStructuredTokens(tokens []HTMLSegmentType, measuredCell htmlTableMeasuredCell, cellRole string, lineHt float64, fallback CSSColorType) {
	pdf := html.pdf
	appearance := measuredCell.appearance
	baseX := pdf.GetX()
	baseWd := measuredCell.contentWd
	effectiveLineHt := htmlEffectiveLineHeight(appearance.style, lineHt)
	oldLeft, oldRight := pdf.lMargin, pdf.rMargin
	defer func() {
		pdf.lMargin, pdf.rMargin = oldLeft, oldRight
	}()
	writeText := func(role, text string, x, wd float64) {
		text = strings.TrimSpace(collapseHTMLWhitespace(text))
		if text == "" || wd <= 0 {
			return
		}
		pdf.lMargin = x
		pdf.rMargin = pdf.w - x - wd
		pdf.SetX(x)
		pdf.SetNextTextRole(role)
		pdf.MultiCell(wd, effectiveLineHt, text, "", appearance.align, false)
	}
	var lists []htmlListState
	var items []htmlTableCellListContext
	var blockRoles []string
	currentTextRole := func() string {
		if len(blockRoles) > 0 {
			return blockRoles[len(blockRoles)-1]
		}
		return cellRole
	}
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		switch token.Cat {
		case 'T':
			if len(items) > 0 {
				item := items[len(items)-1]
				writeText(taggedRoleLBody, token.Str, item.bodyX, item.bodyWd)
			} else {
				writeText(currentTextRole(), token.Str, baseX, baseWd)
			}
		case 'O':
			switch token.Str {
			case "table":
				pdf.lMargin = baseX
				pdf.rMargin = pdf.w - baseX - baseWd
				pdf.SetX(baseX)
				i = html.writeTable(tokens, i, effectiveLineHt, appearance.style, fallback, nil, nil)
				pdf.SetX(baseX)
			case "p", "div", "section", "article", "header", "footer":
				pdf.BeginStructure(taggedRoleP)
				blockRoles = append(blockRoles, taggedRoleP)
			case "ul", "ol":
				st := appearance.style
				st.list = token.Str
				lists = append(lists, htmlListStateFromElement(st, token.Attr, effectiveLineHt))
				pdf.BeginStructure(taggedRoleL)
			case "li":
				if len(lists) == 0 {
					continue
				}
				list := &lists[len(lists)-1]
				list.counter++
				indent := list.indent
				if indent <= 0 {
					indent = effectiveLineHt * 1.5
				}
				depth := len(lists) - 1
				markerX := baseX + float64(depth)*indent
				if markerX > baseX+baseWd {
					markerX = baseX + baseWd
				}
				markerWd := minFloat(indent-1, baseWd*0.4)
				if markerWd < 0 {
					markerWd = 0
				}
				bodyX := markerX + markerWd + 1
				bodyWd := baseWd - (bodyX - baseX)
				if bodyWd < effectiveLineHt {
					bodyX = markerX
					bodyWd = baseWd - (bodyX - baseX)
				}
				y := pdf.GetY()
				pdf.BeginStructure(taggedRoleLI)
				pdf.SetXY(markerX, y)
				pdf.SetNextTextRole(taggedRoleLbl)
				pdf.CellFormat(markerWd, effectiveLineHt, list.marker(), "", 0, "R", false, 0, "")
				pdf.BeginStructure(taggedRoleLBody)
				pdf.SetXY(bodyX, y)
				items = append(items, htmlTableCellListContext{bodyX: bodyX, bodyWd: bodyWd, startY: y})
			case "br":
				pdf.Ln(effectiveLineHt)
			}
		case 'C':
			switch token.Str {
			case "p", "div", "section", "article", "header", "footer":
				if len(blockRoles) > 0 {
					blockRoles = blockRoles[:len(blockRoles)-1]
					pdf.EndStructure()
				}
				pdf.Ln(effectiveLineHt)
			case "li":
				if len(items) == 0 {
					continue
				}
				item := items[len(items)-1]
				items = items[:len(items)-1]
				if pdf.GetY() <= item.startY {
					pdf.SetY(item.startY + effectiveLineHt)
				}
				pdf.EndStructure()
				pdf.EndStructure()
				if len(items) > 0 {
					parent := items[len(items)-1]
					pdf.SetX(parent.bodyX)
				} else {
					pdf.SetX(baseX)
				}
			case "ul", "ol":
				if len(lists) > 0 {
					lists = lists[:len(lists)-1]
				}
				pdf.EndStructure()
			case "figure", "figcaption", "dt", "dd":
				pdf.Ln(effectiveLineHt)
			}
		}
	}
	for len(blockRoles) > 0 {
		blockRoles = blockRoles[:len(blockRoles)-1]
		pdf.EndStructure()
	}
	for len(items) > 0 {
		item := items[len(items)-1]
		items = items[:len(items)-1]
		if pdf.GetY() <= item.startY {
			pdf.SetY(item.startY + effectiveLineHt)
		}
		pdf.EndStructure()
		pdf.EndStructure()
	}
	for len(lists) > 0 {
		lists = lists[:len(lists)-1]
		pdf.EndStructure()
	}
}

func htmlTableCellContainsStructuredContent(tokens []HTMLSegmentType) bool {
	for _, token := range tokens {
		if token.Cat == 'O' && htmlTableCellStructuredTag(token.Str) {
			return true
		}
	}
	return false
}

func htmlTableCellStructuredTag(tag string) bool {
	switch tag {
	case "ul", "ol", "table", "p", "div", "section", "article", "header", "footer":
		return true
	default:
		return false
	}
}

func htmlTableCellScope(role string, row htmlTableRow, placement htmlTableCellPlacement) string {
	if role != taggedRoleTH {
		return ""
	}
	if scope := strings.ToLower(strings.TrimSpace(row.cells[placement.cellIndex].attrs["scope"])); scope != "" {
		switch scope {
		case "row":
			return "Row"
		case "col", "column":
			return "Column"
		case "rowgroup", "colgroup":
			return "Both"
		}
	}
	if row.header || placement.row == 0 {
		return "Column"
	}
	return "Row"
}

func parseHTMLTable(tokens []HTMLSegmentType, start int) (htmlTableType, int) {
	table := htmlTableType{attrs: tokens[start].Attr, start: start, end: start, rows: make([]htmlTableRow, 0, htmlTableRowCount(tokens, start+1))}
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
			table.captionStart = i
			table.captionEnd = end
			table.captionAttrs = el.Attr
			table.captionTokens = captionTokens
			table.captionText = htmlPlainTextWithMode(captionTokens, false)
			table.captionPreserved = htmlPlainTextWithMode(captionTokens, true)
			i = end
		case el.Cat == 'O' && el.Str == "tr":
			row = &htmlTableRow{attrs: el.Attr, cells: make([]htmlTableCell, 0, htmlTableRowCellCount(tokens, i+1)), header: section == "thead", footer: section == "tfoot", start: i}
		case el.Cat == 'C' && el.Str == "tr":
			if row != nil {
				row.end = i
				table.rows = append(table.rows, *row)
				row = nil
			}
		case el.Cat == 'O' && (el.Str == "td" || el.Str == "th"):
			if row == nil {
				row = &htmlTableRow{cells: make([]htmlTableCell, 0, 1)}
			}
			cellTokens, end := htmlCollectCellTokens(tokens, i+1)
			row.cells = append(row.cells, htmlTableCell{
				attrs:         el.Attr,
				tokens:        cellTokens,
				text:          htmlPlainTextWithMode(cellTokens, false),
				textPreserved: htmlPlainTextWithMode(cellTokens, true),
				tag:           el.Str,
				header:        el.Str == "th",
				start:         i,
				end:           end,
				colspan:       htmlTableColspan(el.Attr),
				rowspan:       htmlTableRowspan(el.Attr),
				widthHint:     firstNonEmpty(htmlStyleValue(el.Attr, "width"), el.Attr["width"]),
				alignHint:     firstNonEmpty(htmlStyleValue(el.Attr, "text-align"), el.Attr["align"]),
			})
			i = end
		case el.Cat == 'C' && el.Str == "table":
			if row != nil {
				row.end = i
				table.rows = append(table.rows, *row)
			}
			table.end = i
			table.rows = htmlTableRowsWithFooterLast(table.rows)
			return table, i
		}
	}
	table.end = len(tokens) - 1
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

func htmlTableHeaderRowIndexes(rows []htmlTableLayoutRow) []int {
	var headers []int
	for index, row := range rows {
		if row.row.header {
			headers = append(headers, index)
		}
	}
	if len(headers) > 0 {
		return headers
	}
	for index, row := range rows {
		if !htmlTableRowHasOnlyHeaderCells(row.row) {
			break
		}
		headers = append(headers, index)
	}
	return headers
}

func (html *HTML) shouldStreamTableRows(rows []htmlTableLayoutRow, headerRows []int, captionHt float64, breakInsideAvoid bool) bool {
	if len(rows) < htmlTableStreamingRowLimit || len(headerRows) > 0 || captionHt > 0 || breakInsideAvoid {
		return false
	}
	for _, row := range rows {
		for _, placement := range row.cells {
			if placement.rowspan > 1 || placement.colspan > 1 || htmlTableCellContainsStructuredContent(row.row.cells[placement.cellIndex].tokens) {
				return false
			}
		}
	}
	return true
}

func htmlCollectCellTokens(tokens []HTMLSegmentType, start int) ([]HTMLSegmentType, int) {
	tableDepth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			tableDepth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" && tableDepth > 0 {
			tableDepth--
			continue
		}
		if tableDepth == 0 && tokens[i].Cat == 'C' && (tokens[i].Str == "td" || tokens[i].Str == "th") {
			return tokens[start:i], i
		}
	}
	return tokens[start:], len(tokens) - 1
}

func htmlTableRowCellCount(tokens []HTMLSegmentType, start int) int {
	count := 0
	tableDepth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			tableDepth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" && tableDepth > 0 {
			tableDepth--
			continue
		}
		if tableDepth == 0 && tokens[i].Cat == 'C' && tokens[i].Str == "tr" {
			return count
		}
		if tableDepth == 0 && tokens[i].Cat == 'O' && (tokens[i].Str == "td" || tokens[i].Str == "th") {
			count++
		}
	}
	return count
}

func htmlTableRowCount(tokens []HTMLSegmentType, start int) int {
	count := 0
	tableDepth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			tableDepth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" {
			if tableDepth == 0 {
				return count
			}
			tableDepth--
			continue
		}
		if tableDepth == 0 && tokens[i].Cat == 'C' && tokens[i].Str == "table" {
			return count
		}
		if tableDepth == 0 && tokens[i].Cat == 'O' && tokens[i].Str == "tr" {
			count++
		}
	}
	return count
}

func htmlCollectCaptionTokens(tokens []HTMLSegmentType, start int) ([]HTMLSegmentType, int) {
	tableDepth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			tableDepth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" && tableDepth > 0 {
			tableDepth--
			continue
		}
		if tableDepth == 0 && tokens[i].Cat == 'C' && tokens[i].Str == "caption" {
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
			colspan := cell.colspan
			if col+colspan > htmlMaxTableColumns {
				colspan = htmlMaxTableColumns - col
			}
			rowspan := cell.rowspan
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
	minWidthCache := make(map[string]float64)
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
			minWd := htmlTableCellMinWidthCached(cellDef, pdf, minWidthCache)
			htmlApplyTableSpanMinimum(minWidths, cell.col, span, minWd)
			widthHint := cellDef.widthHint
			if widthHint == "" {
				widthHint = firstNonEmpty(styleValue(cellDef.attrs, "width"), cellDef.attrs["width"])
			}
			if wd, ok := parseHTMLBoxLength(widthHint, pdf, tableWd); ok {
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
	return !layoutgeom.ExceedsAvailableHeight(rowHt, remaining) && layoutgeom.ExceedsAvailableHeight(pairHt, remaining) && !layoutgeom.ExceedsAvailableHeight(pairHt, pageContentHt) && !html.pdf.inHeader && !html.pdf.inFooter && html.pdf.acceptPageBreak()
}

func htmlTableCellMinWidth(cell htmlTableCell, pdf *Document) float64 {
	if pdf == nil {
		return 0
	}
	return htmlMaxWordWidth(pdf, cell.text)
}

func htmlTableCellMinWidthCached(cell htmlTableCell, pdf *Document, cache map[string]float64) float64 {
	if pdf == nil {
		return 0
	}
	if cache == nil {
		return htmlTableCellMinWidth(cell, pdf)
	}
	key := cell.text
	if wd, ok := cache[key]; ok {
		return wd
	}
	wd := htmlMaxWordWidth(pdf, key)
	if len(cache) < htmlTableMinWidthCacheLimit {
		cache[key] = wd
	}
	return wd
}

func htmlTableCellText(cell htmlTableCell, preserveWhitespace bool) string {
	if preserveWhitespace {
		return cell.textPreserved
	}
	return cell.text
}

func htmlMaxWordWidth(pdf *Document, text string) float64 {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return htmlMaxWordWidthUnicode(pdf, text)
		}
	}
	return htmlMaxWordWidthASCII(pdf, text)
}

func htmlMaxWordWidthASCII(pdf *Document, text string) float64 {
	maxWd := 0.0
	start := -1
	for i := 0; i < len(text); i++ {
		if htmlIsASCIISpace(text[i]) {
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

func htmlMaxWordWidthUnicode(pdf *Document, text string) float64 {
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

func htmlIsASCIISpace(ch byte) bool {
	return ch == ' ' || (ch >= '\t' && ch <= '\r')
}

func htmlApplyTableSpanMinimum(widths []float64, start, span int, target float64) {
	if target <= 0 || span <= 0 {
		return
	}
	current := layoutgeom.SumSpan(widths, start, span)
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
	current := layoutgeom.SumSpan(widths, start, span)
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

func (html *HTML) measureTableRows(compiled *CompiledHTML, rows []htmlTableLayoutRow, colOffsets []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, tableAttrs map[string]string) ([]htmlTableMeasuredRow, []float64) {
	measuredRows := make([]htmlTableMeasuredRow, len(rows))
	heights := make([]float64, len(rows))
	for i := range heights {
		heights[i] = lineHt
	}
	tableFill := html.cellBackground(tableAttrs)
	var cellStyleCache htmlTableCellStyleCache
	var appearanceCache htmlTableMeasuredCellAppearanceCache
	for rowIndex, row := range rows {
		measuredRow := html.measureTableRow(compiled, rowIndex, row, colOffsets, padding, lineHt, inherited, fallback, cssRules, tableAncestors, tableBorder, tableFill, &cellStyleCache, &appearanceCache)
		for _, measuredCell := range measuredRow.cells {
			required := measuredCell.textHt + measuredCell.appearance.padding.top + measuredCell.appearance.padding.bottom
			span := measuredCell.placement.rowspan
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
			current := layoutgeom.SumSpan(heights, rowIndex, span)
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

func (html *HTML) measureTableRow(compiled *CompiledHTML, rowIndex int, row htmlTableLayoutRow, colOffsets []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, tableFill CSSColorType, cellStyleCache *htmlTableCellStyleCache, appearanceCache *htmlTableMeasuredCellAppearanceCache) htmlTableMeasuredRow {
	measuredRow := htmlTableMeasuredRow{index: rowIndex, row: row, cells: make([]htmlTableMeasuredCell, 0, len(row.cells))}
	rowEl := HTMLSegmentType{Cat: 'O', Str: "tr", Attr: row.row.attrs}
	rowAncestors := appendHTMLAncestors(tableAncestors, rowEl)
	rowDecl := html.tableElementDeclarations(compiled, row.row.start, rowEl, cssRules, tableAncestors)
	rowDeclKey := htmlTableStyleDeclarationKey(compiled, row.row.start, rowDecl)
	rowFill := html.cellBackgroundFromDeclarations(row.row.attrs, rowDecl)
	for _, placement := range row.cells {
		measuredRow.cells = append(measuredRow.cells, html.measureTableCell(compiled, row, placement, colOffsets, padding, lineHt, inherited, fallback, cssRules, rowAncestors, tableBorder, rowDecl, rowDeclKey, rowFill, tableFill, cellStyleCache, appearanceCache))
	}
	return measuredRow
}

func (html *HTML) measureTableRowHeights(compiled *CompiledHTML, rows []htmlTableLayoutRow, colOffsets []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, tableAttrs map[string]string) []float64 {
	heights := make([]float64, len(rows))
	for i := range heights {
		heights[i] = lineHt
	}
	tableFill := html.cellBackground(tableAttrs)
	var cellStyleCache htmlTableCellStyleCache
	for rowIndex, row := range rows {
		rowEl := HTMLSegmentType{Cat: 'O', Str: "tr", Attr: row.row.attrs}
		rowAncestors := appendHTMLAncestors(tableAncestors, rowEl)
		rowDecl := html.tableElementDeclarations(compiled, row.row.start, rowEl, cssRules, tableAncestors)
		rowDeclKey := htmlTableStyleDeclarationKey(compiled, row.row.start, rowDecl)
		rowFill := html.cellBackgroundFromDeclarations(row.row.attrs, rowDecl)
		for _, placement := range row.cells {
			required := html.measureTableCellRequiredHeight(compiled, row, placement, colOffsets, padding, lineHt, inherited, fallback, cssRules, rowAncestors, tableBorder, rowDecl, rowDeclKey, rowFill, tableFill, &cellStyleCache)
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
			current := layoutgeom.SumSpan(heights, rowIndex, span)
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

func (html *HTML) measureTableCellRequiredHeight(compiled *CompiledHTML, row htmlTableLayoutRow, placement htmlTableCellPlacement, colOffsets []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, rowAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, rowDecl map[string]string, rowDeclKey string, rowFill, tableFill CSSColorType, cellStyleCache *htmlTableCellStyleCache) float64 {
	cell := row.row.cells[placement.cellIndex]
	style := inherited
	if cell.header {
		style.bold = true
		if style.align == "" || style.align == "L" {
			style.align = "C"
		}
	}
	cellEl := HTMLSegmentType{Cat: 'O', Str: cell.tag, Attr: cell.attrs}
	html.applyCompiledElementStyle(compiled, cell.start, &style, cellEl, cssRules, inherited.fontSize, inherited.lineHeight, rowAncestors...)
	html.applyTextStyle(style, fallback)
	cellDecl := html.tableElementDeclarations(compiled, cell.start, cellEl, cssRules, rowAncestors)
	cellDeclKey := htmlTableStyleDeclarationKey(compiled, cell.start, cellDecl)
	wd := layoutgeom.SpanSize(colOffsets, placement.col, placement.colspan)
	cellStyle := html.cachedTableCellStyle(cell.attrs, row.row.attrs, cellDecl, rowDecl, cellDeclKey, rowDeclKey, style.align, padding, wd, tableBorder, rowFill, tableFill, cellStyleCache)
	contentWd := htmlMaxFloat(wd-cellStyle.padding.left-cellStyle.padding.right, 0)
	text := htmlTableCellText(cell, style.preserveWhitespace)
	return html.tableCellTextHeight(text, contentWd, style, lineHt) + cellStyle.padding.top + cellStyle.padding.bottom
}

func (html *HTML) measureTableCell(compiled *CompiledHTML, row htmlTableLayoutRow, placement htmlTableCellPlacement, colOffsets []float64, padding, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, rowAncestors []HTMLSegmentType, tableBorder htmlBorderStyle, rowDecl map[string]string, rowDeclKey string, rowFill, tableFill CSSColorType, cellStyleCache *htmlTableCellStyleCache, appearanceCache *htmlTableMeasuredCellAppearanceCache) htmlTableMeasuredCell {
	cell := row.row.cells[placement.cellIndex]
	style := inherited
	if cell.header {
		style.bold = true
		if style.align == "" || style.align == "L" {
			style.align = "C"
		}
	}
	cellEl := HTMLSegmentType{Cat: 'O', Str: cell.tag, Attr: cell.attrs}
	html.applyCompiledElementStyle(compiled, cell.start, &style, cellEl, cssRules, inherited.fontSize, inherited.lineHeight, rowAncestors...)
	html.applyTextStyle(style, fallback)
	cellDecl := html.tableElementDeclarations(compiled, cell.start, cellEl, cssRules, rowAncestors)
	cellDeclKey := htmlTableStyleDeclarationKey(compiled, cell.start, cellDecl)
	wd := layoutgeom.SpanSize(colOffsets, placement.col, placement.colspan)
	cellStyle := html.cachedTableCellStyle(cell.attrs, row.row.attrs, cellDecl, rowDecl, cellDeclKey, rowDeclKey, style.align, padding, wd, tableBorder, rowFill, tableFill, cellStyleCache)
	contentWd := htmlMaxFloat(wd-cellStyle.padding.left-cellStyle.padding.right, 0)
	text := htmlTableCellText(cell, style.preserveWhitespace)
	lineCount := htmlSplitLineCount(html.pdf, text, contentWd)
	var lines []string
	if lineCount > 1 {
		lines = htmlSplitLines(html.pdf, text, contentWd)
		lineCount = len(lines)
	}
	return htmlTableMeasuredCell{
		placement: placement,
		appearance: appearanceCache.intern(htmlTableMeasuredCellAppearance{
			style:   style,
			align:   cellStyle.align,
			fill:    cellStyle.fill,
			border:  cellStyle.border,
			padding: cellStyle.padding,
		}),
		contentWd: contentWd,
		text:      text,
		lines:     lines,
		textHt:    html.tableCellLineCountHeight(lineCount, style, lineHt),
	}
}

func (html *HTML) tableCaptionHeight(compiled *CompiledHTML, table htmlTableType, tableWd, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType) float64 {
	if len(table.captionTokens) == 0 {
		return 0
	}
	style := inherited
	if style.align == "" || style.align == "L" {
		style.align = "C"
	}
	captionEl := HTMLSegmentType{Cat: 'O', Str: "caption", Attr: table.captionAttrs}
	html.applyCompiledElementStyle(compiled, table.captionStart, &style, captionEl, cssRules, inherited.fontSize, inherited.lineHeight, tableAncestors...)
	html.applyTextStyle(style, fallback)
	text := table.captionText
	if style.preserveWhitespace {
		text = table.captionPreserved
	}
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return html.tableCellTextHeight(text, tableWd, style, lineHt) + htmlEffectiveLineHeight(style, lineHt)*0.5
}

func (html *HTML) renderTableCaption(compiled *CompiledHTML, table htmlTableType, x, tableWd, lineHt float64, inherited htmlTextStyle, fallback CSSColorType, cssRules []htmlCSSRule, tableAncestors []HTMLSegmentType) {
	style := inherited
	if style.align == "" || style.align == "L" {
		style.align = "C"
	}
	captionEl := HTMLSegmentType{Cat: 'O', Str: "caption", Attr: table.captionAttrs}
	html.applyCompiledElementStyle(compiled, table.captionStart, &style, captionEl, cssRules, inherited.fontSize, inherited.lineHeight, tableAncestors...)
	html.applyTextStyle(style, fallback)
	text := table.captionText
	if style.preserveWhitespace {
		text = table.captionPreserved
	}
	if strings.TrimSpace(text) == "" {
		return
	}
	html.pdf.SetX(x)
	html.pdf.SetNextTextRole(taggedRoleCaption)
	html.pdf.MultiCell(tableWd, htmlEffectiveLineHeight(style, lineHt), text, "", style.align, false)
	html.pdf.Ln(htmlEffectiveLineHeight(style, lineHt) * 0.5)
}

func (html *HTML) tableCellTextHeight(text string, wd float64, style htmlTextStyle, lineHt float64) float64 {
	lineCount := htmlSplitLineCount(html.pdf, text, wd)
	return float64(lineCount) * htmlEffectiveLineHeight(style, lineHt)
}

func (html *HTML) tableCellLinesHeight(lines []string, style htmlTextStyle, lineHt float64) float64 {
	lineCount := len(lines)
	if lineCount == 0 {
		lineCount = 1
	}
	return float64(lineCount) * htmlEffectiveLineHeight(style, lineHt)
}

func (html *HTML) tableCellLineCountHeight(lineCount int, style htmlTextStyle, lineHt float64) float64 {
	if lineCount < 1 {
		lineCount = 1
	}
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
	if pdf.isCurrentUTF8 {
		if count := htmlWrappedSplitTextLineCount(pdf, text, wd); count > 0 {
			return count
		}
		return 1
	}
	if strings.Contains(text, "\r") {
		if count := pdf.SplitLineCount([]byte(text), wd); count > 0 {
			return len(htmlSplitLines(pdf, text, wd))
		}
		return 1
	}
	count := htmlSplitStringLineCount(pdf, text, wd)
	if count == 0 {
		return 1
	}
	return count
}

func htmlWrappedSplitTextLineCount(pdf *Document, text string, wd float64) int {
	count := 0
	for _, line := range pdf.SplitText(text, wd) {
		count += htmlWrappedLineCount(pdf, line, wd)
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
		if line == "" || htmlTextWidth(pdf, line) <= wd {
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
			runeWidth := htmlTextSegmentWidth(pdf, line, offset, nextEnd, r)
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
	if wd <= 0 || line == "" || htmlTextWidth(pdf, line) <= wd {
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
		runeWidth := htmlTextSegmentWidth(pdf, line, offset, nextEnd, r)
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

func htmlTextWidth(pdf *Document, text string) float64 {
	if pdf == nil || pdf.err != nil {
		return 0
	}
	if pdf.currentFont.Name == "" {
		return pdf.GetStringWidth(text)
	}
	return float64(htmlTextSymbolWidth(pdf, text)) * pdf.fontSize / 1000
}

func htmlTextSegmentWidth(pdf *Document, text string, start, end int, r rune) float64 {
	if pdf == nil || pdf.err != nil {
		return 0
	}
	if pdf.currentFont.Name == "" {
		return pdf.GetStringWidth(text[start:end])
	}
	width := 0
	if pdf.isCurrentUTF8 {
		width = pdf.currentFontRuneWidth(r)
	} else {
		for i := start; i < end && i < len(text); i++ {
			ch := text[i]
			if ch == 0 {
				break
			}
			width += pdf.currentFont.Cw[ch]
		}
	}
	return float64(width) * pdf.fontSize / 1000
}

func htmlTextSymbolWidth(pdf *Document, text string) int {
	width := 0
	if pdf.isCurrentUTF8 {
		for _, r := range text {
			width += pdf.currentFontRuneWidth(r)
		}
		return width
	}
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if ch == 0 {
			break
		}
		width += pdf.currentFont.Cw[ch]
	}
	return width
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
	return html.cellAlignFromDeclarations(attrs, html.styleDeclarations(attrs), fallback)
}

func (html *HTML) cellAlignFromDeclarations(attrs map[string]string, decl map[string]string, fallback string) string {
	align := strings.ToLower(firstNonEmpty(decl["text-align"], attrs["align"]))
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

func (html *HTML) cachedTableCellStyle(cellAttrs, rowAttrs map[string]string, cellDecl, rowDecl map[string]string, cellDeclKey, rowDeclKey, alignFallback string, paddingFallback, relative float64, tableBorder htmlBorderStyle, rowFill, tableFill CSSColorType, cache *htmlTableCellStyleCache) htmlTableCellStyleCacheValue {
	key := htmlTableCellStyleCacheKey{
		cellStyle:       cellAttrs["style"],
		cellDecl:        cellDeclKey,
		cellAlign:       cellAttrs["align"],
		cellBgColor:     cellAttrs["bgcolor"],
		cellBorder:      cellAttrs["border"],
		cellBorderColor: cellAttrs["bordercolor"],
		rowStyle:        rowAttrs["style"],
		rowDecl:         rowDeclKey,
		rowBgColor:      rowAttrs["bgcolor"],
		rowBorder:       rowAttrs["border"],
		rowBorderColor:  rowAttrs["bordercolor"],
		alignFallback:   alignFallback,
		paddingFallback: paddingFallback,
		relative:        relative,
		rowFill:         rowFill,
		tableFill:       tableFill,
	}
	if cache != nil {
		if cache.values != nil {
			if cached, ok := cache.values[key]; ok {
				return cached
			}
		} else if cache.hasLast && cache.lastKey == key {
			return cache.lastValue
		}
	}
	value := htmlTableCellStyleCacheValue{
		align:   html.cellAlignFromDeclarations(cellAttrs, cellDecl, alignFallback),
		fill:    htmlTableBackground(firstColor(html.cellBackgroundFromDeclarations(cellAttrs, cellDecl), rowFill, tableFill)),
		border:  html.tableCellBorder(tableBorder, cellAttrs, rowAttrs, cellDecl, rowDecl, html.pdf, relative),
		padding: html.cellPaddingFromDeclarations(cellAttrs, cellDecl, html.pdf, paddingFallback, relative),
	}
	if cache != nil {
		if !cache.hasLast {
			cache.lastKey = key
			cache.lastValue = value
			cache.hasLast = true
			return value
		}
		if cache.values == nil {
			cache.values = make(map[htmlTableCellStyleCacheKey]htmlTableCellStyleCacheValue, 2)
			cache.values[cache.lastKey] = cache.lastValue
		}
		if len(cache.values) < htmlTableCellStyleCacheLimit {
			cache.values[key] = value
		}
	}
	return value
}

func htmlTableStyleDeclarationKey(compiled *CompiledHTML, tokenIndex int, declarations map[string]string) string {
	if key, ok := compiled.tableStyleKey(tokenIndex); ok {
		return key
	}
	return htmlTableCellStyleDeclarationKey(declarations)
}

func htmlTableCellStyleDeclarationKey(decl map[string]string) string {
	if len(decl) == 0 {
		return ""
	}
	names := [...]string{
		"text-align",
		"background", "background-color",
		"border", "border-width", "border-style", "border-color",
		"border-top", "border-top-width", "border-top-style", "border-top-color",
		"border-right", "border-right-width", "border-right-style", "border-right-color",
		"border-bottom", "border-bottom-width", "border-bottom-style", "border-bottom-color",
		"border-left", "border-left-width", "border-left-style", "border-left-color",
		"padding", "padding-top", "padding-right", "padding-bottom", "padding-left",
	}
	var b strings.Builder
	for _, name := range names {
		value := strings.TrimSpace(decl[name])
		if value == "" {
			continue
		}
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(value)
		b.WriteByte(';')
	}
	return b.String()
}

func (html *HTML) cellBackground(attrs map[string]string) CSSColorType {
	return html.cellBackgroundFromDeclarations(attrs, html.styleDeclarations(attrs))
}

func (html *HTML) cellBackgroundFromDeclarations(attrs map[string]string, decl map[string]string) CSSColorType {
	if color, ok := parseCSSColor(firstNonEmpty(decl["background-color"], decl["background"], attrs["bgcolor"])); ok {
		return color
	}
	return CSSColorType{}
}

func (html *HTML) tableCellBorder(fallback htmlBorderStyle, cellAttrs, rowAttrs map[string]string, cellDecl, rowDecl map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	border := fallback
	border = html.tableCellBorderFromDeclarations(border, rowAttrs, rowDecl, pdf, relative)
	border = html.tableCellBorderFromDeclarations(border, cellAttrs, cellDecl, pdf, relative)
	return border
}

func (html *HTML) tableCellBorderFromAttrs(fallback htmlBorderStyle, attrs map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	return html.tableCellBorderFromDeclarations(fallback, attrs, html.styleDeclarations(attrs), pdf, relative)
}

func (html *HTML) tableCellBorderFromDeclarations(fallback htmlBorderStyle, attrs map[string]string, decl map[string]string, pdf *Document, relative float64) htmlBorderStyle {
	if !htmlAttrsMayAffectCellBorder(decl, attrs) {
		return fallback
	}
	next := htmlBorderFromStyle(attrs, decl, pdf, relative)
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

func (html *HTML) cellPaddingFromDeclarations(attrs map[string]string, decl map[string]string, pdf *Document, fallback, relative float64) htmlBoxEdges {
	return htmlTableCellPaddingFromStyle(decl, pdf, fallback, relative)
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
