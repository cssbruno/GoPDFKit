// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"strings"

	"github.com/cssbruno/gopdfkit/layout"
)

// LongFormHTMLDocumentModel converts supported long-form HTML into a shared
// document model with extracted footer configuration.
func LongFormHTMLDocumentModel(title, htmlStr string) (*layout.LayoutDocument, []string) {
	bodyHTML, footer := ExtractHTMLFooterBlock(htmlStr)
	pdf := MustNew()
	html := pdf.HTMLNew()
	messages := html.ValidateHTML(bodyHTML)
	doc := layout.NewLayoutDocument()
	doc.Title = strings.TrimSpace(title)
	if doc.Title != "" {
		doc.Body = append(doc.Body, layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: doc.Title}}})
	}
	doc.Body = append(doc.Body, htmlBlocksFromTokens(HTMLTokenize(bodyHTML))...)
	doc.PageTemplate.Footer = footer
	return doc, messages
}

func htmlBlocksFromTokens(tokens []HTMLSegmentType) []layout.Block {
	blocks := make([]layout.Block, 0)
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token.Cat == 'T' {
			text := strings.TrimSpace(collapseHTMLWhitespace(token.Str))
			if text != "" {
				blocks = append(blocks, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}})
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
				blocks = append(blocks, layout.HeadingBlock{Level: htmlHeadingLevel(token.Str), Segments: []layout.TextSegment{{Text: content}}})
			}
		case "p", "div", "section", "article":
			if content != "" {
				blocks = append(blocks, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: content}}})
			}
		case "ul", "ol":
			items := htmlListItems(collected)
			if len(items) > 0 {
				blocks = append(blocks, layout.ListBlock{Ordered: token.Str == "ol", Items: items})
			}
		case "table":
			table, _ := parseHTMLTable(tokens, i)
			if tableBlock := tableBlockFromHTMLTable(table); len(tableBlock.Body) > 0 || len(tableBlock.Header) > 0 {
				blocks = append(blocks, tableBlock)
			}
		case "br":
			blocks = append(blocks, layout.ParagraphBlock{})
		}
		i = end
	}
	return blocks
}

func htmlListItems(tokens []HTMLSegmentType) []layout.ListItem {
	items := make([]layout.ListItem, 0)
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
			items = append(items, layout.ListItem{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}}}})
		}
		i = end
	}
	return items
}

func tableBlockFromHTMLTable(table htmlTableType) layout.TableBlock {
	block := layout.TableBlock{Caption: strings.TrimSpace(htmlPlainText(table.captionTokens))}
	for _, row := range table.rows {
		tableRow := layout.TableRow{KeepTogether: true}
		for _, cell := range row.cells {
			text := strings.TrimSpace(htmlPlainText(cell.tokens))
			tableRow.Cells = append(tableRow.Cells, layout.TableCell{
				Blocks:  []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}}},
				ColSpan: htmlTableColspan(cell.attrs),
				RowSpan: htmlTableRowspan(cell.attrs),
			})
		}
		switch {
		case row.header:
			block.Header = append(block.Header, tableRow)
		case row.footer:
			block.Footer = append(block.Footer, tableRow)
		default:
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
