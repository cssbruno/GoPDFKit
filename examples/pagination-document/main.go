// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
	"github.com/cssbruno/gopdfkit/layout"
)

func main() {
	doc := paginationModel("Document Pagination")
	doc.Body = append(doc.Body,
		paragraph("The document model paginates long content, reserves footer space, repeats table headers, and accepts explicit page breaks."),
		layout.PageBreakBlock{After: true},
		layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: "Automatic Pages"}}},
	)
	for i := 1; i <= 28; i++ {
		doc.Body = append(doc.Body, paragraph(fmt.Sprintf("Paragraph %02d: generated report content that flows across pages while keeping normal margins and footer space.", i)))
	}
	doc.Body = append(doc.Body,
		layout.PageBreakBlock{After: true},
		layout.HeadingBlock{Level: 2, Segments: []layout.TextSegment{{Text: "Paginated Table"}}},
		paginatedTable(),
	)

	pdf := document.MustNew()
	pdf.SetTitle("Document Pagination", false)
	pdf.SetCreator("examples/pagination-document", false)
	pdf.WriteDocument(doc)

	if err := pdf.OutputFileAndClose(outpath.File("pagination-document.pdf")); err != nil {
		log.Fatal(err)
	}
}

func paginationModel(title string) *layout.LayoutDocument {
	doc := layout.NewDocumentModel(title)
	doc.PageTemplate.Footer = &layout.FooterBlock{
		Height:          8,
		ReservePageArea: true,
	}
	doc.PageTemplate.PageNumbers = layout.PageNumberOptions{Enabled: true, TotalPageAlias: "{total}"}
	return doc
}

func paragraph(text string) layout.ParagraphBlock {
	return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}}
}

func paginatedTable() layout.TableBlock {
	body := make([]layout.TableRow, 0, 90)
	for i := 1; i <= 90; i++ {
		body = append(body, layout.TableRow{Cells: []layout.TableCell{
			cell(fmt.Sprintf("%03d", i), "C"),
			cell(fmt.Sprintf("Line item %03d", i), "L"),
			cell(fmt.Sprintf("$%0.2f", float64(i)*19.95), "R"),
		}})
	}
	return layout.TableBlock{
		Header: []layout.TableRow{{Cells: []layout.TableCell{
			cell("#", "C"),
			cell("Description", "L"),
			cell("Amount", "R"),
		}}},
		Body: body,
		Style: layout.TableStyle{
			BorderCollapse: true,
			RepeatHeader:   true,
			KeepRows:       true,
		},
	}
}

func cell(text, align string) layout.TableCell {
	return layout.TableCell{
		Align: align,
		Blocks: []layout.Block{
			layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: text}},
				Style:    layout.TextStyle{FontSize: 8},
			},
		},
	}
}
