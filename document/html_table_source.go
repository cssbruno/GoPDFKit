// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"strconv"
	"strings"
)

// These table structures are source-model data used by compilation and the
// long-form HTML-to-layout adapter. Painting and table geometry belong to the
// unified HTML planner, not to this parser layer.
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

const htmlMaxTableColumns = 1024

func parseHTMLTable(tokens []HTMLSegmentType, start int) (htmlTableType, int) {
	if start < 0 || start >= len(tokens) || tokens[start].Cat != 'O' || tokens[start].Str != "table" {
		return htmlTableType{}, start
	}
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
				attrs: el.Attr, tokens: cellTokens,
				text:          htmlPlainTextWithMode(cellTokens, false),
				textPreserved: htmlPlainTextWithMode(cellTokens, true),
				tag:           el.Str, header: el.Str == "th", start: i, end: end,
				colspan: htmlTableColspan(el.Attr), rowspan: htmlTableRowspan(el.Attr),
				widthHint: firstNonEmpty(htmlStyleValue(el.Attr, "width"), el.Attr["width"]),
				alignHint: firstNonEmpty(htmlStyleValue(el.Attr, "text-align"), el.Attr["align"]),
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

func htmlCollectCaptionTokens(tokens []HTMLSegmentType, start int) ([]HTMLSegmentType, int) {
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			depth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" && depth > 0 {
			depth--
			continue
		}
		if depth == 0 && tokens[i].Cat == 'C' && tokens[i].Str == "caption" {
			return tokens[start:i], i
		}
	}
	return tokens[start:], len(tokens) - 1
}

func htmlTableRowCellCount(tokens []HTMLSegmentType, start int) int {
	count, depth := 0, 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			depth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" && depth > 0 {
			depth--
			continue
		}
		if depth == 0 && tokens[i].Cat == 'C' && tokens[i].Str == "tr" {
			return count
		}
		if depth == 0 && tokens[i].Cat == 'O' && (tokens[i].Str == "td" || tokens[i].Str == "th") {
			count++
		}
	}
	return count
}

func htmlTableRowCount(tokens []HTMLSegmentType, start int) int {
	count, depth := 0, 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Cat == 'O' && tokens[i].Str == "table" {
			depth++
			continue
		}
		if tokens[i].Cat == 'C' && tokens[i].Str == "table" {
			if depth == 0 {
				return count
			}
			depth--
			continue
		}
		if depth == 0 && tokens[i].Cat == 'O' && tokens[i].Str == "tr" {
			count++
		}
	}
	return count
}

func htmlTableColspan(attrs map[string]string) int {
	n, err := strconv.Atoi(strings.TrimSpace(attrs["colspan"]))
	if err != nil || n < 1 {
		return 1
	}
	if n > htmlMaxTableColumns {
		return htmlMaxTableColumns
	}
	return n
}

func htmlTableRowspan(attrs map[string]string) int {
	n, err := strconv.Atoi(strings.TrimSpace(attrs["rowspan"]))
	if err != nil || n < 1 {
		return 1
	}
	if n > htmlMaxTableColumns {
		return htmlMaxTableColumns
	}
	return n
}

func htmlTableCellStyleDeclarationKey(decl map[string]string) string {
	if len(decl) == 0 {
		return ""
	}
	names := [...]string{"text-align", "background", "background-color", "border", "border-width", "border-style", "border-color", "border-top", "border-top-width", "border-top-style", "border-top-color", "border-right", "border-right-width", "border-right-style", "border-right-color", "border-bottom", "border-bottom-width", "border-bottom-style", "border-bottom-color", "border-left", "border-left-width", "border-left-style", "border-left-color", "padding", "padding-top", "padding-right", "padding-bottom", "padding-left"}
	var builder strings.Builder
	for _, name := range names {
		if value := strings.TrimSpace(decl[name]); value != "" {
			builder.WriteString(name)
			builder.WriteByte(':')
			builder.WriteString(value)
			builder.WriteByte(';')
		}
	}
	return builder.String()
}
