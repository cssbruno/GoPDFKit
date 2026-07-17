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
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)

	html := pdf.HTMLNew()
	html.Write(6, `
		<h1>Invoice Summary</h1>
		<p><strong>Status:</strong> paid</p>
		<p>This renderer supports controlled HTML fragments for reports, letters, and forms.</p>
		<ul>
			<li>Bold, italic, underline, and links</li>
			<li>Paragraphs, headings, lists, and tables</li>
			<li>Small CSS subset for text, borders, spacing, and colors</li>
		</ul>
		<table border="1" cellpadding="4">
			<thead><tr><th>Item</th><th>Total</th></tr></thead>
			<tbody><tr><td>Consulting</td><td>$1,200.00</td></tr></tbody>
		</table>
	`)

	if err := pdf.OutputFileAndClose(outpath.File("html-fragment.pdf")); err != nil {
		log.Fatal(err)
	}
}
