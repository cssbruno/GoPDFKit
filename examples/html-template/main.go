// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"log"
	"path/filepath"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	template, err := document.CompileHTMLTemplate(`
		<style>
			h1 { color: #20324a; margin-bottom: 4mm; }
			p { color: #3c4652; line-height: 1.35; }
			.logo {
				width: 55mm;
				height: 22mm;
				object-fit: contain;
				margin: 5mm 0 6mm 0;
			}
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
		<img class="logo" src="{{logo}}" alt="{{logo_alt}}">
		<p class="note">{{message}}</p>
	`)
	if err != nil {
		log.Fatal(err)
	}

	pdf := document.MustNew()
	pdf.SetTitle("HTML Template", false)
	pdf.SetCreator("examples/html-template", false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 11)

	html := pdf.HTMLNew()
	html.AllowLocalImages = true
	err = html.WriteTemplateContext(context.Background(), 6, template, document.HTMLTemplateValues{
		"title":       "Compiled HTML Template",
		"customer":    "Northwind Trading",
		"document_id": "HTM-2026-0042",
		"message":     "The HTML/CSS shape was compiled once. Only text and safe attributes change at render time.",
		"logo":        filepath.ToSlash(assets.File("image", "document.png")),
		"logo_alt":    "PaperRune logo",
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := pdf.OutputFileAndClose(outpath.File("html-template.pdf")); err != nil {
		log.Fatal(err)
	}
}
