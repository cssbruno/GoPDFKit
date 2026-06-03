// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	doc := document.NewGenericDocument("Document Pagination")
	doc.Footer = &document.FooterBlock{
		Height:          8,
		ShowPageNumber:  true,
		TotalPageAlias:  "{total}",
		ReservePageArea: true,
	}
	doc.Body = append(doc.Body,
		paragraph("The document model paginates long content, reserves footer space, repeats table headers, and accepts explicit page breaks."),
		document.PageBreakBlock{After: true},
		document.HeadingBlock{Level: 2, Segments: []document.TextSegment{{Text: "Automatic Pages"}}},
	)
	for i := 1; i <= 28; i++ {
		doc.Body = append(doc.Body, paragraph(fmt.Sprintf("Paragraph %02d: generated report content that flows across pages while keeping normal margins and footer space.", i)))
	}
	doc.Body = append(doc.Body,
		document.PageBreakBlock{After: true},
		document.HeadingBlock{Level: 2, Segments: []document.TextSegment{{Text: "Paginated Table"}}},
		paginatedTable(),
	)

	pdf := document.New("P", "mm", "A4", "")
	pdf.SetTitle("Document Pagination", false)
	pdf.SetCreator("examples/pagination-document", false)
	pdf.WriteDocument(doc)

	if err := pdf.OutputFileAndClose(outpath.File("pagination-document.pdf")); err != nil {
		log.Fatal(err)
	}
}

func paragraph(text string) document.ParagraphBlock {
	return document.ParagraphBlock{Segments: []document.TextSegment{{Text: text}}}
}

func paginatedTable() document.TableBlock {
	body := make([]document.TableRow, 0, 90)
	for i := 1; i <= 90; i++ {
		body = append(body, document.TableRow{Cells: []document.TableCell{
			cell(fmt.Sprintf("%03d", i), "C"),
			cell(fmt.Sprintf("Line item %03d", i), "L"),
			cell(fmt.Sprintf("$%0.2f", float64(i)*19.95), "R"),
		}})
	}
	return document.TableBlock{
		Header: []document.TableRow{{Cells: []document.TableCell{
			cell("#", "C"),
			cell("Description", "L"),
			cell("Amount", "R"),
		}}},
		Body: body,
		Style: document.TableStyle{
			BorderCollapse: true,
			RepeatHeader:   true,
			KeepRows:       true,
		},
	}
}

func cell(text, align string) document.TableCell {
	return document.TableCell{
		Align: align,
		Blocks: []document.Block{
			document.ParagraphBlock{
				Segments: []document.TextSegment{{Text: text}},
				Style:    document.TextStyle{FontSize: 8},
			},
		},
	}
}
