// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := gopdfkit.New()
	pdf.SetTitle("Operations Report", false)
	pdf.SetCreator("examples/structured-report", false)
	pdf.AddPage()

	pdf.SetFillColor(35, 70, 120)
	pdf.Rect(0, 0, 210, 34, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.Text(16, 19, "Operations Report")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(16, 27, "Generated PDF rendering example")

	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 10)
	drawField(pdf, 16, 46, "Report ID", "OPS-2026-001")
	drawField(pdf, 78, 46, "Status", "Ready")
	drawField(pdf, 140, 46, "Owner", "Platform Team")

	pdf.SetFont("Helvetica", "B", 15)
	pdf.Text(16, 80, "Service Metrics")
	drawMetric(pdf, 16, 90, "Uptime", "99.95%", "last 30 days")
	drawMetric(pdf, 78, 90, "Requests", "1.2M", "monthly volume")
	drawMetric(pdf, 140, 90, "Latency", "83 ms", "p95 response")

	pdf.SetFont("Helvetica", "B", 15)
	pdf.Text(16, 138, "Incident Summary")
	drawTableHeader(pdf, 16, 148, []float64{44, 40, 44, 46}, []string{"Area", "Severity", "Open", "Owner"})
	rows := [][]string{
		{"API Gateway", "Medium", "3", "Core Platform"},
		{"Billing", "Low", "1", "Revenue Systems"},
		{"Document Jobs", "High", "2", "PDF Services"},
		{"Storage", "Low", "0", "Infrastructure"},
	}
	y := 156.0
	for i, row := range rows {
		drawTableRow(pdf, 16, y+float64(i)*9, []float64{44, 40, 44, 46}, row)
	}

	pdf.SetFillColor(242, 247, 252)
	pdf.SetDrawColor(190, 205, 220)
	pdf.RoundedRect(16, 203, 178, 34, 3, "1234", "DF")
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(22, 215, "Notes")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(22, 220)
	pdf.MultiCell(166, 5, "This report combines positioned text, filled cards, rounded rectangles, table rows, borders, and page metadata in one complete PDF.", "", "L", false)

	pdf.SetDrawColor(210, 210, 210)
	pdf.Line(16, 274, 194, 274)
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(95, 95, 95)
	pdf.Text(16, 281, "GoPDFKit example - assets/generated/pdf/examples/structured-report.pdf")

	if err := pdf.OutputFileAndClose(outpath.File("structured-report.pdf")); err != nil {
		panic(err)
	}
}

func drawField(pdf *gopdfkit.Document, x, y float64, label, value string) {
	pdf.SetFillColor(245, 248, 251)
	pdf.SetDrawColor(215, 222, 230)
	pdf.RoundedRect(x, y, 54, 22, 2, "1234", "DF")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(105, 115, 125)
	pdf.Text(x+4, y+8, label)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(20, 30, 40)
	pdf.Text(x+4, y+16, value)
}

func drawMetric(pdf *gopdfkit.Document, x, y float64, label, value, note string) {
	pdf.SetFillColor(232, 241, 250)
	pdf.SetDrawColor(190, 210, 230)
	pdf.RoundedRect(x, y, 54, 31, 3, "1234", "DF")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(65, 75, 85)
	pdf.Text(x+5, y+8, label)
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(35, 70, 120)
	pdf.Text(x+5, y+20, value)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(95, 105, 115)
	pdf.Text(x+5, y+27, note)
}

func drawTableHeader(pdf *gopdfkit.Document, x, y float64, widths []float64, labels []string) {
	pdf.SetFillColor(35, 70, 120)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 9)
	for i, label := range labels {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 8, label, "1", 0, "L", true, 0, "")
		x += widths[i]
	}
	pdf.SetTextColor(0, 0, 0)
}

func drawTableRow(pdf *gopdfkit.Document, x, y float64, widths []float64, values []string) {
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(220, 226, 232)
	pdf.SetFont("Helvetica", "", 9)
	for i, value := range values {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 9, value, "1", 0, "L", false, 0, "")
		x += widths[i]
	}
}
