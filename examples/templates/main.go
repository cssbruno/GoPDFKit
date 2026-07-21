// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()

	header := pdf.CreateTemplate(func(tpl *document.Tpl) {
		tpl.ImageOptions(assets.File("image", "logo.png"), 8, 6, 22, 0, false, document.ImageOptions{}, 0, "")
		tpl.SetFont("Helvetica", "B", 16)
		tpl.Text(36, 18, "Reusable template header")
		tpl.SetDrawColor(40, 90, 160)
		tpl.Line(8, 24, 190, 24)
	})

	for i := 1; i <= 2; i++ {
		pdf.AddPage()
		pdf.UseTemplate(header)
		pdf.SetFont("Helvetica", "", 12)
		pdf.SetXY(20, 42)
		pdf.Cell(0, 8, "This page reused the same template object.")
	}

	if err := pdf.OutputFileAndClose(outpath.File("templates.pdf")); err != nil {
		log.Fatal(err)
	}
}
