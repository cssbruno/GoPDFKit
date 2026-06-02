// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit

import "strings"

// LongFormHTMLDocumentModel converts supported long-form HTML into a shared
// document model with extracted footer configuration.
func LongFormHTMLDocumentModel(title, htmlStr string) (*Document, []string) {
	bodyHTML, footer := ExtractHTMLFooterBlock(htmlStr)
	pdf := New("P", "mm", "A4", "")
	html := pdf.HTMLNew()
	messages := html.ValidateHTML(bodyHTML)
	doc := NewDocument(DocumentKindLongForm)
	doc.Title = strings.TrimSpace(title)
	if doc.Title != "" {
		doc.Body = append(doc.Body, HeadingBlock{Level: 1, Segments: []TextSegment{{Text: doc.Title}}})
	}
	doc.Body = append(doc.Body, htmlBlocksFromTokens(HTMLTokenize(bodyHTML))...)
	doc.Footer = footer
	return doc, messages
}

func htmlBlocksFromTokens(tokens []HTMLSegmentType) []Block {
	blocks := make([]Block, 0)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.Cat == 'T' {
			text := strings.TrimSpace(collapseHTMLWhitespace(token.Str))
			if text != "" {
				blocks = append(blocks, ParagraphBlock{Segments: []TextSegment{{Text: text}}})
			}
			continue
		}
		if token.Cat != 'O' {
			continue
		}
		collected, end := htmlCollectElementTokens(tokens, i, token.Str)
		content := ""
		if len(collected) >= 2 {
			content = strings.TrimSpace(htmlPlainText(collected[1 : len(collected)-1]))
		}
		switch token.Str {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			if content != "" {
				blocks = append(blocks, HeadingBlock{Level: htmlHeadingLevel(token.Str), Segments: []TextSegment{{Text: content}}})
			}
		case "p", "div", "section", "article":
			if content != "" {
				blocks = append(blocks, ParagraphBlock{Segments: []TextSegment{{Text: content}}})
			}
		case "ul", "ol":
			items := htmlListItems(collected)
			if len(items) > 0 {
				blocks = append(blocks, ListBlock{Ordered: token.Str == "ol", Items: items})
			}
		case "table":
			table, _ := parseHTMLTable(tokens, i)
			if tableBlock := tableBlockFromHTMLTable(table); len(tableBlock.Body) > 0 || len(tableBlock.Header) > 0 {
				blocks = append(blocks, tableBlock)
			}
		case "br":
			blocks = append(blocks, ParagraphBlock{})
		}
		i = end
	}
	return blocks
}

func htmlListItems(tokens []HTMLSegmentType) []ListItem {
	items := make([]ListItem, 0)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.Cat != 'O' || token.Str != "li" {
			continue
		}
		collected, end := htmlCollectElementTokens(tokens, i, "li")
		text := ""
		if len(collected) >= 2 {
			text = strings.TrimSpace(htmlPlainText(collected[1 : len(collected)-1]))
		}
		if text != "" {
			items = append(items, ListItem{Blocks: []Block{ParagraphBlock{Segments: []TextSegment{{Text: text}}}}})
		}
		i = end
	}
	return items
}

func tableBlockFromHTMLTable(table htmlTableType) TableBlock {
	block := TableBlock{Caption: strings.TrimSpace(htmlPlainText(table.captionTokens))}
	for _, row := range table.rows {
		tableRow := TableRow{KeepTogether: true}
		for _, cell := range row.cells {
			text := strings.TrimSpace(htmlPlainText(cell.tokens))
			tableRow.Cells = append(tableRow.Cells, TableCell{
				Blocks:  []Block{ParagraphBlock{Segments: []TextSegment{{Text: text}}}},
				ColSpan: htmlTableColspan(cell.attrs),
				RowSpan: htmlTableRowspan(cell.attrs),
			})
		}
		if row.header {
			block.Header = append(block.Header, tableRow)
		} else if row.footer {
			block.Footer = append(block.Footer, tableRow)
		} else {
			block.Body = append(block.Body, tableRow)
		}
	}
	return block
}

func htmlHeadingLevel(tag string) int {
	if len(tag) == 2 && tag[0] == 'h' && tag[1] >= '1' && tag[1] <= '6' {
		return int(tag[1] - '0')
	}
	return 2
}
