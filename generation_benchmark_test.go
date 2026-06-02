/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit_test

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/testsupport/example"
)

func benchmarkGeneratedPDF(b *testing.B, build func(*gopdfkit.Fpdf)) {
	b.Helper()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pdf := gopdfkit.New("P", "mm", "A4", "")
		pdf.SetCompression(false)
		build(pdf)

		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			b.Fatalf("Output() error = %v", err)
		}
		if output.Len() == 0 {
			b.Fatal("generated empty PDF")
		}
	}
}

func BenchmarkGenerationText(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for row := 0; row < 180; row++ {
			pdf.CellFormat(40, 6, fmt.Sprintf("Row %03d", row), "1", 0, "L", false, 0, "")
			pdf.CellFormat(80, 6, "Operational PDF generation benchmark", "1", 0, "L", false, 0, "")
			pdf.CellFormat(40, 6, fmt.Sprintf("%0.2f", float64(row)*1.25), "1", 1, "R", false, 0, "")
		}
	})
}

func BenchmarkGenerationLongText(b *testing.B) {
	text, err := os.ReadFile(example.TextFile("20k_c1.txt"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Times", "", 11)
		pdf.MultiCell(0, 5, string(text), "", "J", false)
	})
}

func BenchmarkGenerationHTMLText(b *testing.B) {
	htmlStr := `<style>
		p { margin: 0 0 4px 0; line-height: 1.25; }
		.note { color: #123456; font-weight: bold; }
	</style>`
	for i := 0; i < 80; i++ {
		htmlStr += fmt.Sprintf(`<p><strong>Section %03d</strong> Operational HTML paragraph with `+
			`<em>inline emphasis</em>, <u>underlined text</u>, and `+
			`<span class="note">styled text</span>.</p>`, i)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, htmlStr)
	})
}

func BenchmarkGenerationHTMLTable(b *testing.B) {
	var rows strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&rows, `<tr><td>Item %03d</td><td>Generated benchmark row</td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*1.25)
	}
	htmlStr := `<style>
		th { background-color: #eeeeee; font-weight: bold; }
		td, th { padding: 3px; border: 1px solid #555555; }
	</style>` +
		`<table border="1" cellpadding="3" width="100%">` +
		`<thead><tr><th width="22%">Code</th><th>Description</th><th width="18%">Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>`

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 9)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, htmlStr)
	})
}

func BenchmarkGenerationHTMLDataPNG(b *testing.B) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	var body strings.Builder
	for i := 0; i < 24; i++ {
		fmt.Fprintf(&body, `<p>PNG block %02d</p><img src="%s" width="24" height="24"/>`, i, pngDataURI)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, body.String())
	})
}

func BenchmarkGenerationHTMLInlineSVG(b *testing.B) {
	const svgFragment = `<svg width="80" height="36" viewBox="0 0 80 36">` +
		`<rect x="1" y="1" width="78" height="34" fill="#e8f4ff" stroke="#123456"/>` +
		`<circle cx="18" cy="18" r="8" fill="#2a8fbd"/>` +
		`<path d="M34 24 L46 10 L58 24 Z" fill="#50a060" stroke="#000000"/>` +
		`<text x="70" y="22" text-anchor="middle" font-size="8">SVG</text>` +
		`</svg>`
	var body strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&body, `<p>SVG block %02d</p>%s`, i, svgFragment)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, body.String())
	})
}

func BenchmarkGenerationHTMLMixedDocument(b *testing.B) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	const svgFragment = `<svg width="96" height="32" viewBox="0 0 96 32">` +
		`<rect x="1" y="1" width="94" height="30" fill="#f3f3f3" stroke="#333333"/>` +
		`<path d="M8 24 C20 4 32 4 44 24 S68 44 88 8" fill="none" stroke="#006699"/>` +
		`<text x="48" y="20" text-anchor="middle" font-size="9">mixed</text>` +
		`</svg>`
	var rows strings.Builder
	for i := 0; i < 36; i++ {
		fmt.Fprintf(&rows, `<tr><td>Line %02d</td><td>Nested content and table text</td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*3.75)
	}
	htmlStr := `<style>
		h1 { font-size: 18pt; color: #123456; }
		.summary { background-color: #eeeeee; border: 1px solid #999999; padding: 5px; }
		td, th { padding: 3px; border: 1px solid #666666; }
		th { background-color: #dddddd; }
	</style>` +
		`<h1>Mixed HTML PDF Benchmark</h1>` +
		`<div class="summary"><p><strong>Summary</strong> with paragraph text, inline styles, and image output.</p></div>` +
		`<ul><li>First list item</li><li>Second list item</li><li>Third list item</li></ul>` +
		`<img src="` + pngDataURI + `" width="32" height="32"/>` +
		svgFragment +
		`<table border="1" cellpadding="3" width="100%">` +
		`<thead><tr><th>Code</th><th>Description</th><th>Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>` +
		`<p style="break-before: page">Second page paragraph with <em>emphasis</em> and <u>decoration</u>.</p>` +
		svgFragment

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, htmlStr)
	})
}

func BenchmarkGenerationUTF8Text(b *testing.B) {
	text, err := os.ReadFile(example.TextFile("utf-8test.txt"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 11)
		pdf.MultiCell(0, 5, string(text), "", "L", false)
	})
}

func BenchmarkGenerationUTF8TextCachedFont(b *testing.B) {
	text, err := os.ReadFile(example.TextFile("utf-8test.txt"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}
	fontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed.ttf"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}
	cache := gopdfkit.NewFontCache()
	if err := cache.AddUTF8FontFromBytes("DejaVu", "", fontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 11)
		pdf.MultiCell(0, 5, string(text), "", "L", false)
	})
}

func BenchmarkGenerationTextCompressionLevel(b *testing.B) {
	for _, tc := range []struct {
		name  string
		level int
	}{
		{name: "BestSpeed", level: zlib.BestSpeed},
		{name: "BestCompression", level: zlib.BestCompression},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				pdf := gopdfkit.New("P", "mm", "A4", "")
				pdf.SetCompressionLevel(tc.level)
				pdf.AddPage()
				pdf.SetFont("Arial", "", 10)
				for row := 0; row < 180; row++ {
					pdf.CellFormat(40, 6, fmt.Sprintf("Row %03d", row), "1", 0, "L", false, 0, "")
					pdf.CellFormat(80, 6, "Operational PDF generation benchmark", "1", 0, "L", false, 0, "")
					pdf.CellFormat(40, 6, fmt.Sprintf("%0.2f", float64(row)*1.25), "1", 1, "R", false, 0, "")
				}
				var output bytes.Buffer
				if err := pdf.Output(&output); err != nil {
					b.Fatalf("Output() error = %v", err)
				}
				if output.Len() == 0 {
					b.Fatal("generated empty PDF")
				}
			}
		})
	}
}

func BenchmarkGenerationImages(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
			x := 10 + float64(i%2)*90
			y := 15 + float64(i/2)*70
			pdf.ImageOptions(example.ImageFile(image), x, y, 60, 0, false, gopdfkit.ImageOptions{}, 0, "")
			pdf.Text(x, y+50, image)
		}
	})
}

func BenchmarkGenerationSVG(b *testing.B) {
	svg, err := gopdfkit.SVGParse([]byte(`<svg width="240" height="160" viewBox="0 0 240 160">
		<rect x="12" y="10" width="216" height="140" rx="8" fill="none" stroke="#3c5a8c" stroke-width="4"/>
		<path d="M42 46h156M42 76h156M42 106h108" fill="none" stroke="#3c5a8c" stroke-width="8" stroke-linecap="round"/>
		<circle cx="188" cy="112" r="18" fill="none" stroke="#3c5a8c" stroke-width="6"/>
	</svg>`))
	if err != nil {
		b.Fatalf("SVGParse() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.AddPage()
		pdf.SetDrawColor(60, 90, 140)
		pdf.SetLineWidth(0.4)
		for i := 0; i < 8; i++ {
			pdf.SetXY(10+float64(i%2)*85, 12+float64(i/2)*60)
			pdf.SVGWrite(&svg, 0.18)
		}
	})
}

func BenchmarkGenerationTemplates(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		template := pdf.CreateTemplate(func(tpl *gopdfkit.Tpl) {
			tpl.ImageOptions(example.ImageFile("logo.png"), 6, 6, 28, 0, false, gopdfkit.ImageOptions{}, 0, "")
			tpl.SetFont("Arial", "B", 14)
			tpl.Text(40, 20, "Template benchmark")
			tpl.SetDrawColor(0, 100, 200)
			tpl.SetLineWidth(1.8)
			tpl.Line(95, 12, 105, 22)
		})

		pdf.AddPage()
		for y := 0; y < 5; y++ {
			for x := 0; x < 2; x++ {
				pdf.SetXY(5+float64(x)*95, 10+float64(y)*45)
				pdf.UseTemplate(template)
			}
		}
	})
}

func BenchmarkGenerationImportedPDFPages(b *testing.B) {
	source := func() []byte {
		pdf := gopdfkit.New("P", "pt", "A4", "")
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 16)
		pdf.Text(72, 96, "Imported benchmark page")
		pdf.Rect(70, 110, 220, 60, "D")
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			b.Fatalf("source Output() error = %v", err)
		}
		return out.Bytes()
	}()

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		imported := pdf.ImportPageStream(bytes.NewReader(source), 1, "MediaBox")
		pdf.AddPage()
		for row := 0; row < 4; row++ {
			for col := 0; col < 2; col++ {
				pdf.UseImportedPage(imported, 8+float64(col)*100, 10+float64(row)*62, 88, 0)
			}
		}
	})
}

func BenchmarkGenerationProtection(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		pdf.SetProtection(gopdfkit.CnProtectPrint, "reader", "owner")
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i := 0; i < 80; i++ {
			pdf.CellFormat(0, 5, fmt.Sprintf("Protected line %02d", i), "", 1, "L", false, 0, "")
		}
	})
}

func BenchmarkGenerationAttachments(b *testing.B) {
	grid, err := os.ReadFile("grid.go")
	if err != nil {
		b.Fatalf("ReadFile(grid.go) error = %v", err)
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		b.Fatalf("ReadFile(LICENSE) error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *gopdfkit.Fpdf) {
		attachments := []gopdfkit.Attachment{
			{Content: grid, Filename: "grid.go", Description: "Grid example source"},
			{Content: license, Filename: "LICENSE", Description: "License text"},
		}
		pdf.SetAttachments(attachments)
		pdf.AddPage()
		pdf.SetFont("Arial", "", 12)
		for i, attachment := range attachments {
			y := 20 + float64(i)*30
			pdf.SetXY(15, y)
			pdf.Cell(70, 10, strings.TrimSpace(attachment.Description))
			pdf.Rect(12, y-2, 80, 14, "D")
			pdf.AddAttachmentAnnotation(&attachments[i], 12, y-2, 80, 14)
		}
	})
}
