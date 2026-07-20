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
	pdf.SetTitle("HTML CSS Styles", false)
	pdf.SetCreator("examples/html-css-styles", false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 11)

	html := pdf.HTMLNew()
	html.Write(6, `
		<style>
			h1 { color: #20324a; margin-bottom: 8px; }
			p { font-size: 11pt; line-height: 1.35; }
			.card {
				background: #f7fbff;
				border: 1px solid #8ca9c7;
				border-radius: 10px;
				box-shadow: 4px 5px 10px rgba(0, 0, 0, 0.22);
				padding: 10px;
				margin: 8px 0;
			}
			.notice {
				background-color: #fff8e6;
				border-left: 3px solid #d19219;
				border-top-right-radius: 8px;
				border-bottom-right-radius: 8px;
				padding: 8px 10px;
				margin: 8px 0;
			}
			.small { color: #596575; font-size: 9pt; }
		</style>
		<h1>HTML CSS Styles</h1>
		<p class="card"><strong>Rounded card:</strong> background color, border, border-radius, padding, margin, and box-shadow are rendered with PDF drawing operations.</p>
		<p class="notice"><strong>Side border:</strong> longhand border and per-corner radius declarations can be mixed for report callouts.</p>
		<p class="small">The renderer remains a bounded HTML fragment renderer, not a browser layout engine.</p>
	`)

	if err := pdf.OutputFileAndClose(outpath.File("html-css-styles.pdf")); err != nil {
		log.Fatal(err)
	}
}
