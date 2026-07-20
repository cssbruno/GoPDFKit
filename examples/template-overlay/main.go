// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
	"github.com/cssbruno/paperrune/examples/internal/samplepdf"
)

func main() {
	template := samplepdf.Build("Template", 1)

	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetTitle("Template Overlay", false)
	pdf.SetCreator("examples/template-overlay", false)

	pageID := pdf.ImportPageStream(bytes.NewReader(template), 1, "MediaBox")
	pdf.AddPage()
	pdf.UseImportedPage(pageID, 0, 0, 0, 0)
	drawOverlay(pdf)

	if err := pdf.OutputFileAndClose(outpath.File("template-overlay.pdf")); err != nil {
		log.Fatal(err)
	}
}

func drawOverlay(pdf *document.Document) {
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(55, 120, 170)
	pdf.RoundedRect(96, 536, 403, 112, 6, "1234", "DF")

	pdf.SetTextColor(35, 45, 55)
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(120, 572, "Template populated by PaperRune")
	pdf.SetFont("Helvetica", "", 11)
	pdf.Text(120, 600, "Customer: Northwind Trading")
	pdf.Text(120, 620, "Document ID: TMP-2026-0042")
	pdf.Text(120, 640, "Status: Modified after loading source PDF page")

	pdf.SetAlpha(0.18, "Normal")
	pdf.SetTextColor(35, 120, 80)
	pdf.SetFont("Helvetica", "B", 38)
	pdf.TransformBegin()
	pdf.TransformRotate(-12, 414, 588)
	pdf.Text(330, 596, "APPROVED")
	pdf.TransformEnd()
	pdf.SetAlpha(1, "Normal")
}
