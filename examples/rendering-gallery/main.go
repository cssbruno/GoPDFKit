// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	"math"
	"os"
	"strings"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	generateDashboardMetrics()
	generateInvoiceLayout()
	generateTablePagination()
	generateFormChecklist()
	generateImageCatalog()
	generateLinksBookmarks()
	generatePageSizes()
	generateTransformsClipping()
	generateUnicodeScripts()
	generateHTMLReport()
	generateLongFormContract()
	generateTypographyShowcase()
	generateLineStylesPaths()
	generateSpotColors()
	generateLayersShowcase()
	generateAttachmentsAnnotations()
	generateMetadataXMP()
	generatePageBoxesShowcase()
	generateSVGVector()
	generateCompressionVariants()
	generateColumnsLayout()
	generateCertificateAward()
	generateShippingLabel()
	generateReceiptRoll()
	generateResumeCV()
	generateFinancialStatement()
	generateProjectProposal()
	generateCalendarMonth()
	generateEventTicket()
	generateMeetingMinutes()
	generateLabelSheet()
}

func newPDF(title string) *gopdfkit.Document {
	pdf := gopdfkit.New()
	pdf.SetTitle(title, false)
	pdf.SetCreator("examples/rendering-gallery", false)
	pdf.SetAuthor("GoPDFKit", false)
	return pdf
}

func save(pdf *gopdfkit.Document, name string) {
	if err := pdf.OutputFileAndClose(outpath.File(name)); err != nil {
		panic(err)
	}
}

func generateDashboardMetrics() {
	pdf := newPDF("Dashboard Metrics")
	pdf.AddPage()
	title(pdf, "Dashboard Metrics", "Cards, bars, line chart, alpha, and gradients")

	metricCard(pdf, 16, 38, "Revenue", "$128.4K", "+12.7%")
	metricCard(pdf, 75, 38, "Orders", "3,482", "+8.1%")
	metricCard(pdf, 134, 38, "Errors", "0.18%", "-2.4%")

	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(35, 45, 55)
	pdf.Text(16, 92, "Weekly volume")
	values := []float64{42, 65, 50, 78, 91, 74, 88}
	for i, v := range values {
		x := 20 + float64(i)*22
		h := v * 0.55
		pdf.SetFillColor(85, 135, 190)
		pdf.Rect(x, 156-h, 13, h, "F")
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(85, 95, 105)
		pdf.Text(x+2, 163, fmt.Sprintf("D%d", i+1))
	}

	pdf.SetDrawColor(35, 95, 160)
	pdf.SetLineWidth(0.7)
	points := []document.Point{{X: 20, Y: 210}, {X: 45, Y: 198}, {X: 70, Y: 203}, {X: 95, Y: 181}, {X: 120, Y: 187}, {X: 145, Y: 172}, {X: 170, Y: 178}}
	for i := 1; i < len(points); i++ {
		pdf.Line(points[i-1].X, points[i-1].Y, points[i].X, points[i].Y)
	}
	for _, pt := range points {
		pdf.SetFillColor(255, 255, 255)
		pdf.Circle(pt.X, pt.Y, 2.4, "DF")
	}
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(35, 45, 55)
	pdf.Text(16, 184, "Latency trend")

	save(pdf, "dashboard-metrics.pdf")
}

func generateInvoiceLayout() {
	pdf := newPDF("Invoice Layout")
	pdf.AddPage()
	title(pdf, "Invoice INV-2026-0042", "Transactional layout with totals and table rows")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(16, 48, "Bill To")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(16, 53)
	pdf.MultiCell(75, 5, "Northwind Operations\n22 Market Street\nSeattle, WA 98101", "", "L", false)

	drawField(pdf, 126, 46, "Invoice Date", "2026-01-01")
	drawField(pdf, 126, 70, "Due Date", "2026-01-31")

	headers := []string{"Description", "Qty", "Rate", "Amount"}
	widths := []float64{92, 20, 30, 34}
	drawHeaderRow(pdf, 16, 104, widths, headers)
	rows := [][]string{
		{"PDF generation platform", "1", "$800.00", "$800.00"},
		{"Template implementation", "6", "$95.00", "$570.00"},
		{"Automated document checks", "4", "$75.00", "$300.00"},
		{"Support package", "1", "$180.00", "$180.00"},
	}
	y := 112.0
	for i, row := range rows {
		drawDataRow(pdf, 16, y+float64(i)*9, widths, row)
	}

	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetFillColor(245, 248, 251)
	pdf.Rect(126, 162, 66, 24, "F")
	pdf.SetXY(130, 169)
	pdf.CellFormat(28, 6, "Total", "", 0, "L", false, 0, "")
	pdf.CellFormat(30, 6, "$1,850.00", "", 1, "R", false, 0, "")

	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 100, 110)
	pdf.SetXY(16, 210)
	pdf.MultiCell(176, 5, "Payment terms, remittance information, and invoice notes can be rendered as regular wrapped text below the totals.", "", "L", false)

	save(pdf, "invoice-layout.pdf")
}

func generateTablePagination() {
	pdf := newPDF("Table Pagination")
	pdf.AliasNbPages("{total}")
	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 13)
		pdf.CellFormat(0, 8, "Paginated Data Table", "B", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "B", 9)
		drawHeaderRow(pdf, 14, 24, []float64{18, 54, 32, 66}, []string{"#", "Customer", "Status", "Notes"})
		pdf.SetY(32)
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 7, fmt.Sprintf("Page %d / {total}", pdf.PageNo()), "T", 0, "R", false, 0, "")
	})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 9)
	statuses := []string{"Ready", "Review", "Blocked", "Done"}
	for i := 1; i <= 72; i++ {
		row := []string{
			fmt.Sprintf("%03d", i),
			fmt.Sprintf("Customer Account %02d", i),
			statuses[i%len(statuses)],
			fmt.Sprintf("Generated row %02d with enough content to verify multi-page rendering.", i),
		}
		drawDataRow(pdf, 14, pdf.GetY(), []float64{18, 54, 32, 66}, row)
		pdf.Ln(8)
	}

	save(pdf, "table-pagination.pdf")
}

func generateFormChecklist() {
	pdf := newPDF("Form Checklist")
	pdf.AddPage()
	title(pdf, "Intake Form", "Lines, checkboxes, grouped sections, and signature areas")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(16, 46, "Applicant")
	formLine(pdf, 16, 58, 82, "Name")
	formLine(pdf, 108, 58, 82, "Email")
	formLine(pdf, 16, 78, 174, "Address")

	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(16, 106, "Checklist")
	check(pdf, 18, 118, true, "Identity verified")
	check(pdf, 18, 130, false, "Supporting document attached")
	check(pdf, 18, 142, true, "Terms accepted")
	check(pdf, 18, 154, false, "Manual review required")

	pdf.SetFillColor(247, 249, 251)
	pdf.SetDrawColor(210, 218, 226)
	pdf.RoundedRect(16, 182, 178, 42, 3, "1234", "DF")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(22, 191)
	pdf.MultiCell(166, 6, "Reviewer notes can be captured in a reserved area with background fill and border treatment.", "", "L", false)

	pdf.Line(24, 255, 92, 255)
	pdf.Line(118, 255, 186, 255)
	pdf.SetFont("Helvetica", "", 9)
	pdf.Text(40, 262, "Applicant")
	pdf.Text(138, 262, "Reviewer")

	save(pdf, "form-checklist.pdf")
}

func generateImageCatalog() {
	pdf := newPDF("Image Catalog")
	pdf.AddPage()
	title(pdf, "Image Catalog", "Static assets, captions, and repeated image placement")

	images := []struct {
		File    string
		Caption string
	}{
		{"logo.png", "PNG logo"},
		{"logo.jpg", "JPEG logo"},
		{"golang-gopher.png", "PNG illustration"},
		{"sweden.png", "Flag image"},
	}
	for i, item := range images {
		x := 20 + float64(i%2)*88
		y := 48 + float64(i/2)*82
		pdf.SetDrawColor(215, 222, 230)
		pdf.Rect(x, y, 74, 58, "D")
		pdf.ImageOptions(assets.File("image", item.File), x+7, y+8, 60, 0, false, document.ImageOptions{}, 0, "")
		pdf.SetFont("Helvetica", "", 9)
		pdf.SetTextColor(75, 85, 95)
		pdf.SetXY(x, y+63)
		pdf.CellFormat(74, 5, item.Caption, "", 0, "C", false, 0, "")
	}

	save(pdf, "image-catalog.pdf")
}

func generateLinksBookmarks() {
	pdf := newPDF("Links and Bookmarks")
	detailsLink := pdf.AddLink()

	pdf.AddPage()
	pdf.Bookmark("Overview", 0, -1)
	title(pdf, "Links and Bookmarks", "Document outlines, internal links, and external link rectangles")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(16, 50)
	pdf.MultiCell(176, 6, "This page contains a link to the details page and a link rectangle to external documentation.", "", "L", false)
	pdf.SetFont("Helvetica", "BU", 11)
	pdf.SetTextColor(30, 90, 170)
	pdf.SetXY(16, 76)
	pdf.CellFormat(58, 7, "Jump to details", "", 1, "L", false, detailsLink, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetDrawColor(30, 90, 170)
	pdf.RoundedRect(16, 94, 82, 18, 3, "1234", "D")
	pdf.LinkString(16, 94, 82, 18, "https://pkg.go.dev/github.com/cssbruno/gopdfkit")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(22, 106, "Open Go package docs")

	pdf.AddPage()
	pdf.SetLink(detailsLink, -1, -1)
	pdf.Bookmark("Details", 0, -1)
	title(pdf, "Details", "The internal link on page one lands here")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(16, 50)
	pdf.MultiCell(176, 6, strings.Repeat("Details content demonstrates a multi-page document with outline entries. ", 8), "", "L", false)

	save(pdf, "links-bookmarks.pdf")
}

func generatePageSizes() {
	pdf := gopdfkit.NewWithOptions(gopdfkit.Options{UnitStr: "mm", Size: document.Size{Wd: 100, Ht: 148}})
	pdf.SetTitle("Mixed Page Sizes", false)
	pdf.SetCreator("examples/rendering-gallery", false)

	pdf.AddPage()
	centerLabel(pdf, "Custom 100 x 148 mm page")
	pdf.AddPageFormat("L", document.Size{Wd: 210, Ht: 99})
	centerLabel(pdf, "Landscape ticket page")
	pdf.AddPageFormat("P", document.Size{Wd: 80, Ht: 120})
	centerLabel(pdf, "Narrow receipt page")
	pdf.AddPageFormat("P", document.Size{Wd: 210, Ht: 297})
	centerLabel(pdf, "Back to A4")

	save(pdf, "page-sizes.pdf")
}

func generateTransformsClipping() {
	pdf := newPDF("Transforms and Clipping")
	pdf.AddPage()
	title(pdf, "Transforms and Clipping", "Rotation, skew, text clipping, alpha, and gradients")

	pdf.SetFont("Helvetica", "B", 48)
	pdf.SetDrawColor(35, 70, 120)
	pdf.ClipText(18, 88, "CLIP", true)
	pdf.LinearGradient(15, 42, 120, 60, 250, 90, 70, 60, 120, 210, 0, 0, 1, 0)
	pdf.ClipEnd()

	pdf.SetFont("Helvetica", "B", 16)
	pdf.TransformBegin()
	pdf.TransformRotate(18, 92, 144)
	pdf.SetFillColor(235, 242, 250)
	pdf.RoundedRect(54, 126, 88, 30, 4, "1234", "DF")
	pdf.SetTextColor(35, 70, 120)
	pdf.Text(62, 145, "Rotated card")
	pdf.TransformEnd()

	pdf.TransformBegin()
	pdf.TransformSkewX(16, 100, 205)
	pdf.SetFillColor(236, 248, 238)
	pdf.Rect(36, 190, 120, 34, "DF")
	pdf.SetTextColor(30, 105, 70)
	pdf.Text(48, 211, "Skewed content block")
	pdf.TransformEnd()

	pdf.SetAlpha(0.35, "Normal")
	pdf.SetFillColor(255, 120, 90)
	pdf.Circle(126, 242, 22, "F")
	pdf.SetFillColor(60, 130, 220)
	pdf.Circle(154, 242, 22, "F")
	pdf.SetAlpha(1, "Normal")
	pdf.SetFillColor(245, 190, 55)
	pdf.SetDrawColor(140, 95, 20)
	pdf.Polygon(star(54, 242, 18, 8, 5), "DF")

	save(pdf, "transforms-clipping.pdf")
}

func generateUnicodeScripts() {
	pdf := newPDF("Unicode Scripts")
	pdf.AddUTF8FontFromBytes("dejavu", "", mustRead(assets.File("font", "DejaVuSansCondensed.ttf")))
	pdf.AddUTF8FontFromBytes("dejavu", "B", mustRead(assets.File("font", "DejaVuSansCondensed-Bold.ttf")))
	pdf.AddUTF8FontFromBytes("naskh", "", mustRead(assets.File("font", "NotoNaskhArabic-Regular.ttf")))
	pdf.AddUTF8FontFromBytes("hebrew", "", mustRead(assets.File("font", "NotoSansHebrew-Regular.ttf")))
	pdf.AddUTF8FontFromBytes("devanagari", "", mustRead(assets.File("font", "NotoSansDevanagari-Regular.ttf")))

	pdf.AddPage()
	title(pdf, "Unicode Scripts", "Embedded UTF-8 fonts for multiple writing systems")

	pdf.SetFont("dejavu", "B", 14)
	pdf.Text(16, 50, "Latin, Greek, and Cyrillic")
	pdf.SetFont("dejavu", "", 12)
	pdf.SetXY(16, 58)
	pdf.MultiCell(176, 7, "Cafe, Sao Paulo, Ελληνικά, Русский, math symbols: ∑ ∫ √ ≤ ≥", "", "L", false)

	pdf.SetFont("naskh", "", 15)
	pdf.Text(16, 88, "مرحبا بالعالم")
	pdf.SetFont("hebrew", "", 15)
	pdf.Text(16, 110, "שלום עולם")
	pdf.SetFont("devanagari", "", 15)
	pdf.Text(16, 132, "नमस्ते दुनिया")

	pdf.SetFont("dejavu", "", 11)
	pdf.SetXY(16, 156)
	pdf.MultiCell(176, 6, "Use AddUTF8FontFromBytes when the application embeds font bytes and wants deterministic example output.", "", "L", false)

	save(pdf, "unicode-scripts.pdf")
}

func generateHTMLReport() {
	pdf := newPDF("HTML Report")
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 11)
	html := pdf.HTMLNew()
	html.Write(5, `
		<h1>HTML Report</h1>
		<p><strong>Purpose:</strong> Render a controlled HTML fragment into a PDF page.</p>
		<p>The subset covers headings, paragraphs, emphasis, lists, tables, borders, and background colors.</p>
		<table border="1" cellpadding="5">
			<thead>
				<tr><th>Feature</th><th>Status</th><th>Notes</th></tr>
			</thead>
			<tbody>
				<tr><td>Headings</td><td>Ready</td><td>h1 through lower heading levels</td></tr>
				<tr><td>Lists</td><td>Ready</td><td>ordered and unordered content</td></tr>
				<tr><td>Tables</td><td>Ready</td><td>cell borders and padding</td></tr>
			</tbody>
		</table>
		<ul>
			<li>Useful for generated reports and letters.</li>
			<li>Still deterministic enough for fixture PDFs.</li>
		</ul>
	`)

	save(pdf, "html-report.pdf")
}

func generateLongFormContract() {
	pdf := newPDF("Long Form Contract")
	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 10)
		pdf.CellFormat(0, 7, "Service Agreement", "B", 1, "L", false, 0, "")
	})
	pdf.SetFooterFunc(func() {
		pdf.SetY(-14)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 7, fmt.Sprintf("Page %d", pdf.PageNo()), "T", 0, "R", false, 0, "")
	})
	pdf.AddPage()
	pdf.Bookmark("Agreement", 0, -1)
	pdf.SetFont("Helvetica", "B", 18)
	pdf.CellFormat(0, 12, "Service Agreement", "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 11)
	intro := "This agreement demonstrates long-form text, numbered clauses, page breaks, bookmarks, and signature lines in a generated PDF."
	pdf.MultiCell(0, 6, intro, "", "L", false)
	for i := 1; i <= 9; i++ {
		if i == 4 || i == 7 {
			pdf.AddPage()
		}
		pdf.Bookmark(fmt.Sprintf("Clause %d", i), 1, -1)
		pdf.SetFont("Helvetica", "B", 12)
		pdf.CellFormat(0, 8, fmt.Sprintf("%d. Clause Heading", i), "", 1, "L", false, 0, "")
		pdf.SetFont("Helvetica", "", 10)
		text := strings.Repeat("The parties agree that generated PDF content should remain readable, explicit, and suitable for review. ", 3)
		pdf.MultiCell(0, 5, text, "", "J", false)
		pdf.Ln(2)
	}
	pdf.Ln(12)
	pdf.Line(24, pdf.GetY(), 88, pdf.GetY())
	pdf.Line(122, pdf.GetY(), 186, pdf.GetY())
	pdf.Ln(4)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(98, 5, "Provider", "", 0, "C", false, 0, "")
	pdf.CellFormat(80, 5, "Customer", "", 1, "C", false, 0, "")

	save(pdf, "long-form-contract.pdf")
}

func generateTypographyShowcase() {
	pdf := newPDF("Typography Showcase")
	pdf.AddPage()
	title(pdf, "Typography Showcase", "Font styles, alignment, spacing, rendering modes, and links")

	pdf.SetFont("Times", "B", 18)
	pdf.Text(16, 48, "Serif Bold Heading")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(16, 58)
	pdf.MultiCell(176, 6, "This PDF exercises common text operations used by reports, letters, labels, and formatted forms.", "", "L", false)

	styles := []struct {
		Style string
		Label string
	}{
		{"", "regular"},
		{"B", "bold"},
		{"I", "italic"},
		{"U", "underline"},
		{"S", "strike-through"},
		{"BU", "bold underline"},
	}
	y := 84.0
	for _, style := range styles {
		pdf.SetFont("Helvetica", style.Style, 12)
		pdf.Text(20, y, "Text style: "+style.Label)
		y += 10
	}

	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(16, 154)
	pdf.WriteAligned(176, 6, "Left aligned text demonstrates WriteAligned with a fixed width.", "L")
	pdf.WriteAligned(176, 6, "Centered text demonstrates the same width with center alignment.", "C")
	pdf.WriteAligned(176, 6, "Right aligned text demonstrates right-side alignment.", "R")

	pdf.SetXY(16, 188)
	pdf.SetWordSpacing(2.5)
	pdf.Write(6, "Increased word spacing across this sentence.")
	pdf.SetWordSpacing(0)

	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetDrawColor(35, 70, 120)
	pdf.SetTextColor(35, 70, 120)
	pdf.SetTextRenderingMode(1)
	pdf.Text(20, 224, "Stroked text")
	pdf.SetTextRenderingMode(2)
	pdf.Text(20, 244, "Fill and stroke")
	pdf.SetTextRenderingMode(0)

	save(pdf, "typography-showcase.pdf")
}

func generateLineStylesPaths() {
	pdf := newPDF("Line Styles and Paths")
	pdf.AddPage()
	title(pdf, "Line Styles and Paths", "Caps, joins, dash patterns, polygons, and bezier paths")

	pdf.SetLineWidth(3)
	caps := []string{"butt", "round", "square"}
	for i, capStyle := range caps {
		y := 55 + float64(i)*18
		pdf.SetLineCapStyle(capStyle)
		pdf.SetDrawColor(35, 70, 120)
		pdf.Line(34, y, 120, y)
		pdf.SetFont("Helvetica", "", 9)
		pdf.Text(128, y+1, "cap: "+capStyle)
	}

	pdf.SetLineWidth(1.2)
	pdf.SetLineCapStyle("butt")
	pdf.SetLineJoinStyle("round")
	pdf.SetDashPattern([]float64{3, 2}, 0)
	pdf.SetDrawColor(165, 70, 45)
	pdf.Polygon(star(62, 132, 34, 14, 6), "D")
	pdf.SetDashPattern(nil, 0)

	pdf.SetLineJoinStyle("bevel")
	pdf.SetFillColor(232, 242, 250)
	pdf.SetDrawColor(30, 100, 160)
	pdf.Polygon([]document.Point{
		{X: 128, Y: 105},
		{X: 178, Y: 116},
		{X: 164, Y: 164},
		{X: 118, Y: 150},
	}, "DF")

	pdf.SetLineJoinStyle("miter")
	pdf.SetDrawColor(45, 115, 80)
	pdf.Beziergon([]document.Point{
		{X: 34, Y: 204},
		{X: 64, Y: 174},
		{X: 92, Y: 236},
		{X: 132, Y: 188},
		{X: 172, Y: 224},
	}, "D")

	save(pdf, "line-styles-paths.pdf")
}

func generateSpotColors() {
	pdf := newPDF("Spot Colors")
	pdf.AddSpotColor("Brand Blue", 88, 42, 0, 12)
	pdf.AddSpotColor("Warm Accent", 0, 55, 95, 5)
	pdf.AddSpotColor("Deep Ink", 72, 66, 58, 84)
	pdf.AddPage()
	title(pdf, "Spot Colors", "Named CMYK spot colors with different tints")

	swatches := []struct {
		Name string
		Y    float64
	}{
		{"Brand Blue", 52},
		{"Warm Accent", 92},
		{"Deep Ink", 132},
	}
	for _, swatch := range swatches {
		pdf.SetFont("Helvetica", "B", 11)
		pdf.SetTextColor(35, 45, 55)
		pdf.Text(16, swatch.Y+8, swatch.Name)
		for i, tint := range []byte{30, 60, 90, 100} {
			x := 72 + float64(i)*28
			pdf.SetFillSpotColor(swatch.Name, tint)
			pdf.Rect(x, swatch.Y, 20, 20, "F")
			pdf.SetFont("Helvetica", "", 7)
			pdf.SetTextColor(70, 80, 90)
			pdf.Text(x+3, swatch.Y+27, fmt.Sprintf("%d%%", tint))
		}
	}
	pdf.SetTextSpotColor("Brand Blue", 100)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(16, 204, "Spot color text")
	pdf.SetDrawSpotColor("Warm Accent", 100)
	pdf.SetLineWidth(2)
	pdf.Line(16, 212, 132, 212)

	save(pdf, "spot-colors.pdf")
}

func generateLayersShowcase() {
	pdf := newPDF("Layers Showcase")
	background := pdf.AddLayer("Background grid", true)
	notes := pdf.AddLayer("Review notes", false)
	main := pdf.AddLayer("Main content", true)
	pdf.OpenLayerPane()
	pdf.AddPage()
	title(pdf, "Layers Showcase", "Optional content groups: visible and hidden layers")

	pdf.BeginLayer(background)
	pdf.SetDrawColor(225, 232, 239)
	for x := 20.0; x <= 190; x += 10 {
		pdf.Line(x, 42, x, 252)
	}
	for y := 42.0; y <= 252; y += 10 {
		pdf.Line(20, y, 190, y)
	}
	pdf.EndLayer()

	pdf.BeginLayer(main)
	pdf.SetFillColor(236, 246, 240)
	pdf.SetDrawColor(70, 140, 90)
	pdf.RoundedRect(38, 78, 134, 64, 5, "1234", "DF")
	pdf.SetFont("Helvetica", "B", 17)
	pdf.SetTextColor(35, 80, 55)
	pdf.Text(54, 112, "Main visible layer")
	pdf.EndLayer()

	pdf.BeginLayer(notes)
	pdf.SetFillColor(255, 245, 190)
	pdf.SetDrawColor(185, 150, 45)
	pdf.RoundedRect(48, 160, 116, 32, 4, "1234", "DF")
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetTextColor(95, 75, 20)
	pdf.Text(56, 179, "Hidden review note layer")
	pdf.EndLayer()

	save(pdf, "layers-showcase.pdf")
}

func generateAttachmentsAnnotations() {
	pdf := newPDF("Attachments and Annotations")
	readme := mustRead("README.md")
	license := mustRead("LICENSE")
	pdf.SetAttachments([]document.Attachment{
		{Content: readme, Filename: "README.md", Description: "Repository README"},
		{Content: license, Filename: "LICENSE", Description: "Project license"},
	})

	pdf.AddPage()
	title(pdf, "Attachments and Annotations", "Document-level attachments and page attachment links")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(16, 48)
	pdf.MultiCell(176, 6, "This PDF embeds README.md and LICENSE globally, then adds two visible attachment annotation rectangles on the page.", "", "L", false)

	annotated := document.Attachment{Content: []byte("review-note: generated by rendering-gallery\n"), Filename: "review-note.txt", Description: "Review note"}
	for i, label := range []string{"Open attached review note", "Same attachment reused"} {
		y := 88 + float64(i)*38
		pdf.SetDrawColor(35, 70, 120)
		pdf.SetFillColor(245, 248, 251)
		pdf.RoundedRect(22, y, 92, 20, 3, "1234", "DF")
		pdf.SetFont("Helvetica", "B", 10)
		pdf.SetTextColor(35, 70, 120)
		pdf.Text(28, y+13, label)
		pdf.AddAttachmentAnnotation(&annotated, 22, y, 92, 20)
	}

	save(pdf, "attachments-annotations.pdf")
}

func generateMetadataXMP() {
	pdf := newPDF("Metadata and XMP")
	pdf.SetSubject("Generated metadata showcase", false)
	pdf.SetKeywords("pdf,metadata,xmp,javascript,generated", false)
	pdf.SetProducer("GoPDFKit metadata example", false)
	pdf.SetXmpMetadata([]byte(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
  <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
    <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">
      <dc:title><rdf:Alt><rdf:li xml:lang="x-default">Metadata and XMP</rdf:li></rdf:Alt></dc:title>
    </rdf:Description>
  </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`))
	pdf.SetJavascript("this.info.Subject = 'Generated metadata showcase';")
	pdf.AddPage()
	title(pdf, "Metadata and XMP", "Info dictionary, XMP packet, keywords, and JavaScript action")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(16, 52)
	pdf.MultiCell(176, 6, "This example sets PDF metadata fields, embeds an XMP metadata packet, and includes a small JavaScript action for readers that support it.", "", "L", false)

	save(pdf, "metadata-xmp.pdf")
}

func generatePageBoxesShowcase() {
	pdf := newPDF("Page Boxes Showcase")
	pdf.AddPage()
	pdf.SetPageBox("trim", 12, 12, 186, 273)
	pdf.SetPageBox("bleed", 6, 6, 198, 285)
	pdf.SetPageBox("art", 28, 42, 154, 188)
	title(pdf, "Page Boxes Showcase", "Trim, bleed, and art boxes are written into page dictionaries")
	pdf.SetDrawColor(220, 85, 70)
	pdf.Rect(12, 36, 186, 237, "D")
	pdf.SetDrawColor(70, 130, 190)
	pdf.Rect(6, 30, 198, 249, "D")
	pdf.SetDrawColor(70, 150, 90)
	pdf.Rect(28, 60, 154, 188, "D")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(60, 70, 80)
	pdf.Text(18, 286, "Red: TrimBox, Blue: BleedBox, Green: ArtBox")

	save(pdf, "page-boxes-showcase.pdf")
}

func generateSVGVector() {
	pdf := newPDF("SVG Vector")
	pdf.AddPage()
	title(pdf, "SVG Vector", "Parsed SVG paths rendered as PDF vector operations")

	svg, err := document.SVGParse([]byte(`<svg width="300" height="180" viewBox="0 0 300 180">
		<path d="M20 140 C70 20 112 30 148 112 S238 170 280 44" fill="none" stroke="#245c9c" stroke-width="8" stroke-linecap="round"/>
		<path d="M48 132 L92 58 L136 132 Z" fill="#e8f1fa" stroke="#245c9c" stroke-width="4"/>
		<path d="M180 54 L250 54 L250 124 L180 124 Z" fill="#f5d76e" stroke="#8a6415" stroke-width="4"/>
		<text x="42" y="164" font-size="18" fill="#334455">SVG parsed into PDF</text>
	</svg>`))
	if err != nil {
		panic(err)
	}
	pdf.SetXY(18, 58)
	pdf.SVGWrite(&svg, 0.58)

	save(pdf, "svg-vector.pdf")
}

func generateCompressionVariants() {
	compressed := newPDF("Compression Default")
	compressed.AddPage()
	title(compressed, "Compression Default", "Default compressed PDF streams")
	fillCompressionBody(compressed)
	save(compressed, "compression-default.pdf")

	uncompressed := newPDF("Compression Disabled")
	uncompressed.SetNoCompression()
	uncompressed.AddPage()
	title(uncompressed, "Compression Disabled", "Uncompressed PDF streams for debugging")
	fillCompressionBody(uncompressed)
	save(uncompressed, "compression-none.pdf")
}

func fillCompressionBody(pdf *gopdfkit.Document) {
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetXY(16, 44)
	for i := 1; i <= 36; i++ {
		pdf.CellFormat(0, 5, fmt.Sprintf("Repeated deterministic content row %02d: %s", i, strings.Repeat("data ", 12)), "", 1, "L", false, 0, "")
	}
}

func generateColumnsLayout() {
	pdf := newPDF("Columns Layout")
	pdf.AddPage()
	title(pdf, "Columns Layout", "Manual multi-column text flow with gutters")
	pdf.SetFont("Helvetica", "", 9)

	columnWidth := 54.0
	gutter := 8.0
	startX := 16.0
	y := 46.0
	text := strings.Repeat("Column text flows through narrow measures with explicit x and y placement. ", 5)
	for col := 0; col < 3; col++ {
		x := startX + float64(col)*(columnWidth+gutter)
		pdf.SetXY(x, y)
		pdf.SetFillColor(248, 250, 252)
		pdf.Rect(x-2, y-4, columnWidth+4, 190, "F")
		pdf.SetXY(x, y)
		for section := 1; section <= 4; section++ {
			pdf.SetFont("Helvetica", "B", 10)
			pdf.MultiCell(columnWidth, 5, fmt.Sprintf("Column %d.%d", col+1, section), "", "L", false)
			pdf.SetFont("Helvetica", "", 8)
			pdf.MultiCell(columnWidth, 4.5, text, "", "J", false)
			pdf.Ln(2)
		}
	}

	save(pdf, "columns-layout.pdf")
}

func generateCertificateAward() {
	pdf := newPDF("Certificate Award")
	pdf.AddPageFormat("L", document.Size{Wd: 297, Ht: 210})
	pdf.SetDrawColor(35, 70, 120)
	pdf.SetLineWidth(1.4)
	pdf.Rect(12, 12, 273, 186, "D")
	pdf.SetLineWidth(0.4)
	pdf.Rect(18, 18, 261, 174, "D")

	pdf.SetFont("Times", "B", 30)
	pdf.SetTextColor(35, 70, 120)
	pdf.SetXY(20, 42)
	pdf.CellFormat(257, 16, "Certificate of Completion", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetTextColor(70, 80, 90)
	pdf.SetXY(20, 70)
	pdf.CellFormat(257, 8, "Presented to", "", 1, "C", false, 0, "")
	pdf.SetFont("Times", "BI", 28)
	pdf.SetTextColor(30, 40, 50)
	pdf.SetXY(20, 90)
	pdf.CellFormat(257, 15, "Alex Morgan", "", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetXY(48, 122)
	pdf.MultiCell(201, 7, "For successfully completing the GoPDFKit document generation course and demonstrating practical PDF rendering workflows.", "", "C", false)
	pdf.Line(58, 170, 118, 170)
	pdf.Line(180, 170, 240, 170)
	pdf.SetFont("Helvetica", "", 9)
	pdf.Text(75, 177, "Instructor")
	pdf.Text(199, 177, "Date")

	save(pdf, "certificate-award.pdf")
}

func generateShippingLabel() {
	pdf := gopdfkit.NewWithOptions(gopdfkit.Options{UnitStr: "mm", Size: document.Size{Wd: 102, Ht: 152}})
	pdf.SetTitle("Shipping Label", false)
	pdf.SetCreator("examples/rendering-gallery", false)
	pdf.AddPage()
	pdf.SetLineWidth(0.6)
	pdf.Rect(4, 4, 94, 144, "D")
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetXY(8, 10)
	pdf.CellFormat(86, 10, "PRIORITY", "1", 1, "C", false, 0, "")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Text(8, 32, "SHIP TO")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(8, 38)
	pdf.MultiCell(86, 6, "Northwind Operations\n22 Market Street\nSeattle, WA 98101", "", "L", false)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Text(8, 70, "FROM")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetXY(8, 76)
	pdf.MultiCell(86, 5, "GoPDFKit Fulfillment\n100 Example Avenue\nAustin, TX 73301", "", "L", false)
	pdf.SetFont("Courier", "B", 22)
	for i := 0; i < 14; i++ {
		x := 10 + float64(i)*6
		h := 22.0
		if i%3 == 0 {
			h = 30
		}
		pdf.Rect(x, 108, 2.5, h, "F")
	}
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(8, 140)
	pdf.CellFormat(86, 5, "TRACKING 9400 1000 0000 2026 0001", "", 0, "C", false, 0, "")

	save(pdf, "shipping-label.pdf")
}

func generateReceiptRoll() {
	pdf := gopdfkit.NewWithOptions(gopdfkit.Options{UnitStr: "mm", Size: document.Size{Wd: 80, Ht: 220}})
	pdf.SetTitle("Receipt Roll", false)
	pdf.SetCreator("examples/rendering-gallery", false)
	pdf.AddPage()
	pdf.SetFont("Courier", "B", 12)
	pdf.SetXY(5, 10)
	pdf.CellFormat(70, 6, "GOPDFKIT STORE", "", 1, "C", false, 0, "")
	pdf.SetFont("Courier", "", 8)
	pdf.CellFormat(70, 5, "2026-01-01 10:30", "", 1, "C", false, 0, "")
	pdf.Line(5, 25, 75, 25)
	items := []struct {
		Name  string
		Price string
	}{
		{"PDF TEMPLATE", "24.00"},
		{"REPORT BUNDLE", "18.50"},
		{"FONT EMBED", "09.75"},
		{"SUPPORT PLAN", "12.00"},
		{"TAX", "04.50"},
	}
	y := 34.0
	for _, item := range items {
		pdf.Text(7, y, item.Name)
		pdf.Text(58, y, item.Price)
		y += 8
	}
	pdf.Line(5, y, 75, y)
	pdf.SetFont("Courier", "B", 10)
	pdf.Text(7, y+10, "TOTAL")
	pdf.Text(54, y+10, "68.75")
	pdf.SetFont("Courier", "", 8)
	pdf.SetXY(5, y+25)
	pdf.MultiCell(70, 5, "Thank you for testing generated receipt layouts with custom page sizes.", "", "C", false)

	save(pdf, "receipt-roll.pdf")
}

func generateResumeCV() {
	pdf := newPDF("Resume CV")
	pdf.AddPage()
	pdf.SetFillColor(35, 70, 120)
	pdf.Rect(0, 0, 62, 297, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Text(10, 24, "Jordan Lee")
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetXY(10, 36)
	pdf.MultiCell(44, 5, "PDF Engineer\njordan@example.test\n+1 555 0100", "", "L", false)
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(10, 76, "Skills")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetXY(10, 84)
	pdf.MultiCell(44, 5, "Go\nPDF layout\nDocument automation\nTesting\nTypography", "", "L", false)

	pdf.SetTextColor(35, 45, 55)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(74, 28, "Profile")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(74, 36)
	pdf.MultiCell(116, 6, "Pragmatic engineer focused on deterministic PDF generation, reusable document templates, and maintainable rendering pipelines.", "", "L", false)
	resumeSection(pdf, 74, 74, "Experience", []string{
		"Senior PDF Engineer - Document Systems",
		"Built rendering workflows for reports, forms, labels, and statements.",
		"Implemented generated fixture PDFs for visual review and regression checks.",
	})
	resumeSection(pdf, 74, 132, "Education", []string{
		"B.S. Computer Science",
		"Coursework in graphics, document systems, and backend services.",
	})
	resumeSection(pdf, 74, 178, "Projects", []string{
		"GoPDFKit examples gallery",
		"Generated assets covering typography, tables, images, SVG, and metadata.",
	})

	save(pdf, "resume-cv.pdf")
}

func generateFinancialStatement() {
	pdf := newPDF("Financial Statement")
	pdf.AddPage()
	title(pdf, "Financial Statement", "Account summary with transaction ledger")
	drawField(pdf, 16, 44, "Account", "ACCT-2026-001")
	drawField(pdf, 84, 44, "Period", "Jan 2026")
	drawField(pdf, 152, 44, "Balance", "$12,480.44")

	drawHeaderRow(pdf, 16, 88, []float64{30, 78, 34, 34}, []string{"Date", "Description", "Debit", "Credit"})
	rows := [][]string{
		{"Jan 02", "Opening balance", "", "$9,950.00"},
		{"Jan 04", "Subscription revenue", "", "$1,200.00"},
		{"Jan 08", "Cloud services", "$320.40", ""},
		{"Jan 12", "Consulting invoice", "", "$1,850.00"},
		{"Jan 19", "Payment processor", "$199.16", ""},
	}
	for i, row := range rows {
		drawDataRow(pdf, 16, 96+float64(i)*9, []float64{30, 78, 34, 34}, row)
	}
	pdf.SetFont("Helvetica", "B", 11)
	pdf.SetXY(118, 158)
	pdf.CellFormat(38, 8, "Ending Balance", "", 0, "R", false, 0, "")
	pdf.CellFormat(36, 8, "$12,480.44", "1", 1, "R", false, 0, "")

	save(pdf, "financial-statement.pdf")
}

func generateProjectProposal() {
	pdf := newPDF("Project Proposal")
	pdf.AddPage()
	title(pdf, "Project Proposal", "Milestones, scope, estimate, and acceptance")
	pdf.SetFont("Helvetica", "B", 14)
	pdf.Text(16, 48, "Scope")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(16, 56)
	pdf.MultiCell(176, 6, "Build a deterministic PDF generation library with examples, fixtures, documentation, and rendering coverage for common business documents.", "", "L", false)

	pdf.SetFont("Helvetica", "B", 14)
	pdf.Text(16, 88, "Milestones")
	drawHeaderRow(pdf, 16, 98, []float64{64, 36, 76}, []string{"Milestone", "Date", "Deliverable"})
	rows := [][]string{
		{"Library API cleanup", "Week 1", "Public package surface"},
		{"Examples and fixtures", "Week 2", "Generated PDF assets"},
		{"Validation", "Week 3", "Tests and references"},
	}
	for i, row := range rows {
		drawDataRow(pdf, 16, 106+float64(i)*9, []float64{64, 36, 76}, row)
	}

	pdf.SetFillColor(245, 248, 251)
	pdf.SetDrawColor(210, 218, 226)
	pdf.RoundedRect(16, 160, 176, 34, 4, "1234", "DF")
	pdf.SetFont("Helvetica", "B", 11)
	pdf.Text(22, 174, "Estimated total")
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Text(146, 174, "$8,400")

	save(pdf, "project-proposal.pdf")
}

func generateCalendarMonth() {
	pdf := newPDF("Calendar Month")
	pdf.AddPageFormat("L", document.Size{Wd: 297, Ht: 210})
	pdf.SetFont("Helvetica", "B", 20)
	pdf.SetTextColor(35, 70, 120)
	pdf.Text(18, 22, "January 2026")
	pdf.SetTextColor(0, 0, 0)

	weekdays := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	cellW, cellH := 36.5, 22.0
	startX, startY := 18.0, 36.0
	pdf.SetFont("Helvetica", "B", 9)
	for i, day := range weekdays {
		pdf.SetXY(startX+float64(i)*cellW, startY)
		pdf.CellFormat(cellW, 8, day, "1", 0, "C", true, 0, "")
	}
	pdf.SetFont("Helvetica", "", 9)
	date := 1
	for row := 0; row < 5; row++ {
		for col := 0; col < 7; col++ {
			x := startX + float64(col)*cellW
			y := startY + 8 + float64(row)*cellH
			pdf.Rect(x, y, cellW, cellH, "D")
			if row == 0 && col < 3 || date > 31 {
				continue
			}
			pdf.Text(x+3, y+6, fmt.Sprintf("%d", date))
			if date == 15 {
				pdf.SetFillColor(232, 242, 250)
				pdf.Rect(x+2, y+9, cellW-4, 8, "F")
				pdf.Text(x+4, y+15, "Review")
			}
			date++
		}
	}

	save(pdf, "calendar-month.pdf")
}

func generateEventTicket() {
	pdf := gopdfkit.NewWithOptions(gopdfkit.Options{UnitStr: "mm", Size: document.Size{Wd: 180, Ht: 80}})
	pdf.SetTitle("Event Ticket", false)
	pdf.SetCreator("examples/rendering-gallery", false)
	pdf.AddPage()
	pdf.SetFillColor(35, 70, 120)
	pdf.Rect(0, 0, 180, 80, "F")
	pdf.SetFillColor(255, 255, 255)
	pdf.RoundedRect(8, 8, 164, 64, 5, "1234", "F")
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(35, 70, 120)
	pdf.Text(18, 26, "PDF Summit 2026")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(70, 80, 90)
	pdf.Text(18, 38, "Seat A12  |  Track: Rendering")
	pdf.Text(18, 48, "2026-01-01 09:00")
	for i := 0; i < 12; i++ {
		x := 120 + float64(i)*3
		h := 34.0
		if i%2 == 0 {
			h = 44
		}
		pdf.SetFillColor(35, 70, 120)
		pdf.Rect(x, 20, 1.5, h, "F")
	}
	pdf.SetFont("Helvetica", "", 8)
	pdf.Text(116, 62, "TICKET-2026-0001")

	save(pdf, "event-ticket.pdf")
}

func generateMeetingMinutes() {
	pdf := newPDF("Meeting Minutes")
	pdf.AddPage()
	title(pdf, "Meeting Minutes", "Agenda, decisions, action items, and attendees")
	drawField(pdf, 16, 44, "Date", "2026-01-01")
	drawField(pdf, 84, 44, "Team", "PDF Services")
	drawField(pdf, 152, 44, "Owner", "Operations")
	pdf.SetFont("Helvetica", "B", 13)
	pdf.Text(16, 84, "Decisions")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(16, 92)
	pdf.MultiCell(176, 6, "- Keep generated PDF fixtures tracked\n- Expand examples for real document workflows\n- Use binary diffs for committed PDFs", "", "L", false)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.Text(16, 132, "Action Items")
	drawHeaderRow(pdf, 16, 142, []float64{82, 46, 48}, []string{"Task", "Owner", "Due"})
	rows := [][]string{
		{"Review generated assets", "Maintainer", "Jan 05"},
		{"Update package docs", "Docs", "Jan 08"},
		{"Run release checks", "CI", "Jan 10"},
	}
	for i, row := range rows {
		drawDataRow(pdf, 16, 150+float64(i)*9, []float64{82, 46, 48}, row)
	}

	save(pdf, "meeting-minutes.pdf")
}

func generateLabelSheet() {
	pdf := newPDF("Label Sheet")
	pdf.AddPage()
	title(pdf, "Label Sheet", "Grid of repeated mailing labels")
	pdf.SetFont("Helvetica", "", 8)
	labelW, labelH := 56.0, 32.0
	startX, startY := 16.0, 44.0
	for row := 0; row < 6; row++ {
		for col := 0; col < 3; col++ {
			x := startX + float64(col)*60
			y := startY + float64(row)*36
			pdf.SetDrawColor(210, 218, 226)
			pdf.RoundedRect(x, y, labelW, labelH, 2, "1234", "D")
			pdf.SetXY(x+4, y+6)
			pdf.MultiCell(labelW-8, 4.5, fmt.Sprintf("Recipient %02d\n%d Example Way\nCity, ST 000%02d", row*3+col+1, 100+row*3+col, row*3+col), "", "L", false)
		}
	}

	save(pdf, "label-sheet.pdf")
}

func title(pdf *gopdfkit.Document, heading, subheading string) {
	pdf.SetFillColor(35, 70, 120)
	pdf.Rect(0, 0, 210, 28, "F")
	pdf.SetFont("Helvetica", "B", 18)
	pdf.SetTextColor(255, 255, 255)
	pdf.Text(16, 17, heading)
	pdf.SetFont("Helvetica", "", 9)
	pdf.Text(16, 24, subheading)
	pdf.SetTextColor(0, 0, 0)
}

func metricCard(pdf *gopdfkit.Document, x, y float64, label, value, delta string) {
	pdf.SetFillColor(245, 248, 251)
	pdf.SetDrawColor(213, 223, 233)
	pdf.RoundedRect(x, y, 48, 30, 3, "1234", "DF")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(80, 90, 100)
	pdf.Text(x+5, y+8, label)
	pdf.SetFont("Helvetica", "B", 15)
	pdf.SetTextColor(30, 70, 120)
	pdf.Text(x+5, y+20, value)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(45, 130, 80)
	pdf.Text(x+5, y+27, delta)
	pdf.SetTextColor(0, 0, 0)
}

func drawField(pdf *gopdfkit.Document, x, y float64, label, value string) {
	pdf.SetFillColor(245, 248, 251)
	pdf.SetDrawColor(215, 222, 230)
	pdf.RoundedRect(x, y, 62, 20, 2, "1234", "DF")
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(100, 110, 120)
	pdf.Text(x+4, y+7, label)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(25, 35, 45)
	pdf.Text(x+4, y+15, value)
	pdf.SetTextColor(0, 0, 0)
}

func drawHeaderRow(pdf *gopdfkit.Document, x, y float64, widths []float64, values []string) {
	pdf.SetFillColor(45, 82, 130)
	pdf.SetDrawColor(45, 82, 130)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 8)
	for i, value := range values {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 8, value, "1", 0, "L", true, 0, "")
		x += widths[i]
	}
	pdf.SetTextColor(0, 0, 0)
}

func drawDataRow(pdf *gopdfkit.Document, x, y float64, widths []float64, values []string) {
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(220, 226, 232)
	pdf.SetTextColor(30, 40, 50)
	pdf.SetFont("Helvetica", "", 8)
	for i, value := range values {
		pdf.SetXY(x, y)
		pdf.CellFormat(widths[i], 8, value, "1", 0, "L", false, 0, "")
		x += widths[i]
	}
}

func resumeSection(pdf *gopdfkit.Document, x, y float64, heading string, lines []string) {
	pdf.SetFont("Helvetica", "B", 13)
	pdf.SetTextColor(35, 70, 120)
	pdf.Text(x, y, heading)
	pdf.SetDrawColor(210, 218, 226)
	pdf.Line(x, y+3, x+116, y+3)
	pdf.SetTextColor(35, 45, 55)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.Text(x, y+14, lines[0])
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetXY(x, y+20)
	pdf.MultiCell(116, 5, strings.Join(lines[1:], "\n"), "", "L", false)
}

func formLine(pdf *gopdfkit.Document, x, y, width float64, label string) {
	pdf.SetDrawColor(120, 130, 140)
	pdf.Line(x, y, x+width, y)
	pdf.SetFont("Helvetica", "", 8)
	pdf.SetTextColor(90, 100, 110)
	pdf.Text(x, y+5, label)
	pdf.SetTextColor(0, 0, 0)
}

func check(pdf *gopdfkit.Document, x, y float64, checked bool, label string) {
	pdf.SetDrawColor(70, 90, 110)
	pdf.Rect(x, y-4, 5, 5, "D")
	if checked {
		pdf.SetFont("Helvetica", "B", 8)
		pdf.Text(x+1.2, y, "X")
	}
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(x+8, y, label)
}

func centerLabel(pdf *gopdfkit.Document, label string) {
	w, h := pdf.GetPageSize()
	pdf.SetDrawColor(35, 70, 120)
	pdf.Rect(8, 8, w-16, h-16, "D")
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetXY(8, h/2-5)
	pdf.CellFormat(w-16, 10, label, "", 0, "C", false, 0, "")
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}

func star(cx, cy, outer, inner float64, count int) []document.Point {
	points := make([]document.Point, 0, count*2)
	for i := 0; i < count*2; i++ {
		radius := outer
		if i%2 == 1 {
			radius = inner
		}
		angle := -math.Pi/2 + float64(i)*math.Pi/float64(count)
		points = append(points, document.Point{
			X: cx + math.Cos(angle)*radius,
			Y: cy + math.Sin(angle)*radius,
		})
	}
	return points
}
