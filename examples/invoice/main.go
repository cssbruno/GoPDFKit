// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Invoice", false)
	pdf.SetCreator("examples/invoice", false)
	pdf.AddPage()

	pdf.SetFillColor(35, 70, 120)
	pdf.Rect(0, 0, 210, 34, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.Text(16, 19, "Invoice INV-2026-0042")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(16, 27, "Generated invoice example")

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(16, 50, "Bill To")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(16, 56)
	pdf.MultiCell(76, 5, "Northwind Operations\n22 Market Street\nSeattle, WA 98101", "", "L", false)

	field(pdf, 126, 48, "Invoice Date", "2026-01-01")
	field(pdf, 126, 72, "Due Date", "2026-01-31")

	widths := []float64{88, 22, 32, 34}
	headers := []string{"Description", "Qty", "Rate", "Amount"}
	headerRow(pdf, 16, 104, widths, headers)
	rows := [][]string{
		{"PDF generation platform", "1", "$800.00", "$800.00"},
		{"Template implementation", "6", "$95.00", "$570.00"},
		{"Automated document checks", "4", "$75.00", "$300.00"},
		{"Support package", "1", "$180.00", "$180.00"},
	}
	y := 112.0
	for i, row := range rows {
		dataRow(pdf, 16, y+float64(i)*9, widths, row)
	}

	pdf.SetFillColor(245, 248, 251)
	pdf.SetDrawColor(215, 222, 230)
	pdf.Rect(126, 162, 66, 24, "DF")
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetXY(130, 169)
	pdf.CellFormat(28, 6, "Total", "", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, "$1,850.00", "", 1, "R", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 100, 110)
	pdf.SetXY(16, 210)
	pdf.MultiCell(176, 5, "Payment terms, remittance information, and invoice notes can be rendered as regular wrapped text below the totals.", "", "L", false)

	if err := pdf.OutputFileAndClose(outpath.File("invoice.pdf")); err != nil {
		log.Fatal(err)
	}
}

func field(pdf *document.Document, x, y float64, label, value string) {
	pdf.SetFillColor(245, 248, 251)
	pdf.SetDrawColor(215, 222, 230)
	pdf.Rect(x, y, 60, 18, "DF")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(105, 115, 125)
	pdf.Text(x+4, y+7, label)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(20, 30, 40)
	pdf.Text(x+4, y+14, value)
}

func headerRow(pdf *document.Document, x, y float64, widths []float64, values []string) {
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(35, 70, 120)
	pdf.SetTextColor(255, 255, 255)
	for i, value := range values {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 8, value, "1", 0, "L", true, 0, "")
		x += widths[i]
	}
	pdf.SetTextColor(0, 0, 0)
}

func dataRow(pdf *document.Document, x, y float64, widths []float64, values []string) {
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(220, 226, 232)
	for i, value := range values {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 9, value, "1", 0, "L", false, 0, "")
		x += widths[i]
	}
}
