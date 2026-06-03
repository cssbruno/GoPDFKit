// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := document.New("P", "mm", "A4", "")
	pdf.SetTitle("Manual Table Pagination", false)
	pdf.SetCreator("examples/pagination-table", false)
	pdf.AliasNbPages("{total}")
	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 14)
		pdf.CellFormat(0, 9, "Manual Table Pagination", "B", 1, "L", false, 0, "")
		drawTableHeader(pdf)
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 7, fmt.Sprintf("Page %d / {total}", pdf.PageNo()), "T", 0, "R", false, 0, "")
	})

	pdf.AddPage()
	for i := 1; i <= 130; i++ {
		if pdf.GetY()+7 > 270 {
			pdf.AddPage()
		}
		drawTableRow(pdf, i)
	}

	if err := pdf.OutputFileAndClose(outpath.File("pagination-table.pdf")); err != nil {
		panic(err)
	}
}

func drawTableHeader(pdf *document.Document) {
	pdf.SetFillColor(36, 60, 82)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	for i, label := range []string{"#", "Account", "Region", "Status", "Amount"} {
		pdf.CellFormat([]float64{18, 64, 34, 34, 32}[i], 7, label, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(7)
	pdf.SetTextColor(0, 0, 0)
}

func drawTableRow(pdf *document.Document, index int) {
	statuses := []string{"Ready", "Review", "Blocked", "Done"}
	regions := []string{"North", "South", "East", "West"}
	values := []string{
		fmt.Sprintf("%03d", index),
		fmt.Sprintf("Customer Account %03d", index),
		regions[index%len(regions)],
		statuses[index%len(statuses)],
		fmt.Sprintf("$%0.2f", float64(index)*37.25),
	}
	widths := []float64{18, 64, 34, 34, 32}
	pdf.SetDrawColor(220, 226, 232)
	pdf.SetFont("Helvetica", "", 8)
	for i, value := range values {
		align := "L"
		if i == len(values)-1 {
			align = "R"
		}
		pdf.CellFormat(widths[i], 7, value, "1", 0, align, false, 0, "")
	}
	pdf.Ln(7)
}
