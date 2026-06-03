// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"path/filepath"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	template := `
		<style>
			h1 { color: #20324a; margin-bottom: 4mm; }
			p { color: #3c4652; line-height: 1.35; }
			.note {
				background-color: #f4f8fb;
				border-left: 3px solid #2f80ed;
				padding: 3mm;
				margin-top: 4mm;
			}
		</style>
		<h1>{{title}}</h1>
		<p><strong>Customer:</strong> {{customer}}</p>
		<p><strong>Document:</strong> {{document_id}}</p>
		{{logo}}
		<p class="note">{{message}}</p>
	`
	fragment, err := document.RenderHTMLTemplate(template, document.HTMLTemplateValues{
		"title":       "HTML Template",
		"customer":    "Northwind Trading",
		"document_id": "HTM-2026-0042",
		"message":     "Plain values are escaped before rendering. Image placeholders insert normal HTML img tags.",
		"logo": document.HTMLTemplateImage{
			Source:    filepath.ToSlash(assets.File("image", "gopdfkit.png")),
			Alt:       "GoPDFKit logo",
			Width:     "55mm",
			Height:    "22mm",
			ObjectFit: "contain",
			Align:     "center",
			Style:     "margin: 5mm 0 6mm 0",
		},
	})
	if err != nil {
		panic(err)
	}

	pdf := gopdfkit.New()
	pdf.SetTitle("HTML Template", false)
	pdf.SetCreator("examples/html-template", false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 11)

	html := pdf.HTMLNew()
	html.AllowLocalImages = true
	html.Write(6, fragment)

	if err := pdf.OutputFileAndClose(outpath.File("html-template.pdf")); err != nil {
		panic(err)
	}
}
