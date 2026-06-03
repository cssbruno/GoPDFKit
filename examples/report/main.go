// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := document.NewWithOptions(document.Options{
		OrientationStr: "P",
		UnitStr:        "mm",
		SizeStr:        "A4",
		Optimize:       true,
	})
	pdf.SetTitle("GoPDFKit Operations Report", false)
	pdf.SetCreator("examples/report", false)

	pdf.AddPage()
	drawReportHeader(pdf)
	drawSummary(pdf)
	drawMetrics(pdf)
	drawIncidents(pdf)
	drawReportFooter(pdf)

	if err := pdf.OutputFileAndClose(outpath.File("gopdfkit-report.pdf")); err != nil {
		log.Fatal(err)
	}
}

func drawReportHeader(pdf *document.Document) {
	pdf.SetFillColor(35, 70, 120)
	pdf.Rect(0, 0, 210, 36, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.Text(16, 19, "Operations Report")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(16, 28, "Generated with GoPDFKit")

	drawField(pdf, 16, 48, "Report ID", "OPS-2026-001")
	drawField(pdf, 78, 48, "Status", "Ready")
	drawField(pdf, 140, 48, "Owner", "Platform Team")
}

func drawSummary(pdf *document.Document) {
	pdf.SetTextColor(30, 40, 50)
	pdf.SetFont("Helvetica", "B", 15)
	pdf.Text(16, 88, "Executive Summary")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(16, 96)
	pdf.MultiCell(178, 5.5, "Revenue, operations, and customer health remained inside target ranges. Document generation was stable across scheduled jobs, ad-hoc reports, and signed outputs.", "", "L", false)

	pdf.SetFillColor(242, 247, 252)
	pdf.SetDrawColor(190, 205, 220)
	pdf.RoundedRect(16, 116, 178, 26, 3, "1234", "DF")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Text(22, 127, "Key signal")
	pdf.SetFont("Helvetica", "", 9)
	pdf.Text(22, 136, "Customer-facing PDFs were produced without browser or office-runtime dependencies.")
}

func drawMetrics(pdf *document.Document) {
	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetTextColor(30, 40, 50)
	pdf.Text(16, 160, "Service Metrics")
	drawMetric(pdf, 16, 170, "Uptime", "99.95%", "last 30 days")
	drawMetric(pdf, 78, 170, "Requests", "1.2M", "monthly volume")
	drawMetric(pdf, 140, 170, "Latency", "83 ms", "p95 response")
}

func drawIncidents(pdf *document.Document) {
	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetTextColor(30, 40, 50)
	pdf.Text(16, 226, "Incident Summary")
	drawTableHeader(pdf, 16, 236, []float64{44, 40, 44, 46}, []string{"Area", "Severity", "Open", "Owner"})
	rows := [][]string{
		{"API Gateway", "Medium", "3", "Core Platform"},
		{"Billing", "Low", "1", "Revenue Systems"},
		{"Document Jobs", "High", "2", "PDF Services"},
	}
	y := 244.0
	for i, row := range rows {
		drawTableRow(pdf, 16, y+float64(i)*9, []float64{44, 40, 44, 46}, row)
	}
}

func drawReportFooter(pdf *document.Document) {
	pdf.SetDrawColor(210, 210, 210)
	pdf.Line(16, 282, 194, 282)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(95, 95, 95)
	pdf.Text(16, 288, "GoPDFKit example - assets/generated/pdf/examples/gopdfkit-report.pdf")
}

func drawField(pdf *document.Document, x, y float64, label, value string) {
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

func drawMetric(pdf *document.Document, x, y float64, label, value, note string) {
	pdf.SetFillColor(232, 241, 250)
	pdf.SetDrawColor(190, 210, 230)
	pdf.RoundedRect(x, y, 54, 34, 3, "1234", "DF")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(65, 75, 85)
	pdf.Text(x+5, y+9, label)
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(35, 70, 120)
	pdf.Text(x+5, y+22, value)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(95, 105, 115)
	pdf.Text(x+5, y+30, note)
}

func drawTableHeader(pdf *document.Document, x, y float64, widths []float64, labels []string) {
	pdf.SetFillColor(35, 70, 120)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	for i, label := range labels {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 8, label, "1", 0, "L", true, 0, "")
		x += widths[i]
	}
	pdf.SetTextColor(0, 0, 0)
}

func drawTableRow(pdf *document.Document, x, y float64, widths []float64, values []string) {
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(220, 226, 232)
	pdf.SetFont("Helvetica", "", 8)
	for i, value := range values {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 9, value, "1", 0, "L", false, 0, "")
		x += widths[i]
	}
}
