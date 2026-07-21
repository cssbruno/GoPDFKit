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
	pdf := document.MustNew()
	pdf.SetTitle("Table PDF Report", false)
	pdf.SetCreator("examples/table-report", false)
	pdf.AliasNbPages("{total}")
	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 14)
		pdf.CellFormat(0, 9, "Table PDF Report", "B", 1, "L", false, 0, "")
		drawHeader(pdf)
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 7, fmt.Sprintf("Page %d / {total}", pdf.PageNo()), "T", 0, "R", false, 0, "")
	})

	pdf.AddPage()
	statuses := []string{"Ready", "Review", "Blocked", "Done"}
	for i := 1; i <= 84; i++ {
		drawRow(pdf, i, fmt.Sprintf("Customer Account %02d", i), statuses[i%len(statuses)], fmt.Sprintf("Generated row %02d with wrapped notes.", i))
	}

	if err := pdf.OutputFileAndClose(outpath.File("gopdfkit-tables.pdf")); err != nil {
		log.Fatal(err)
	}
}

func drawHeader(pdf *document.Document) {
	pdf.SetFillColor(35, 70, 120)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	widths := []float64{18, 62, 28, 74}
	labels := []string{"#", "Customer", "Status", "Notes"}
	for i, label := range labels {
		pdf.CellFormat(widths[i], 7, label, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(7)
	pdf.SetTextColor(0, 0, 0)
}

func drawRow(pdf *document.Document, index int, customer, status, notes string) {
	if pdf.GetY() > 270 {
		pdf.AddPage()
	}
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(220, 226, 232)
	pdf.SetFont("Helvetica", "", 8)
	values := []string{fmt.Sprintf("%03d", index), customer, status, notes}
	widths := []float64{18, 62, 28, 74}
	for i, value := range values {
		pdf.CellFormat(widths[i], 7, value, "1", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
}
