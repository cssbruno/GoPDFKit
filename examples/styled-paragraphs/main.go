// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Styled Paragraphs", false)
	pdf.SetCreator("examples/styled-paragraphs", false)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 20)
	pdf.SetTextColor(35, 70, 120)
	pdf.CellFormat(0, 12, "Styled Paragraphs", "", 1, "L", false, 0, "")

	html := pdf.HTMLNew()
	html.Write(6, `
		<style>
			h2 { color: #234678; margin-top: 10px; }
			p { font-size: 11pt; line-height: 1.35; }
			.callout { background-color: #f2f7fc; border: 1px solid #b9c9dc; padding: 8px; margin: 8px 0; }
			.small { font-size: 9pt; color: #5f6b77; }
		</style>
		<h2>Rich text</h2>
		<p>Paragraphs can combine <strong>bold</strong>, <em>italic</em>, <u>underline</u>, links, color, and wrapping inside the built-in HTML subset.</p>
		<p class="callout">Use CSS padding, borders, and background color when the paragraph needs to read like a callout or note.</p>
		<p class="small">The renderer is intentionally bounded: predictable PDF output instead of browser-grade page layout.</p>
	`)

	if err := pdf.OutputFileAndClose(outpath.File("styled-paragraphs.pdf")); err != nil {
		log.Fatal(err)
	}
}
