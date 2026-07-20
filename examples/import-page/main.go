// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	source := buildSourcePDF()

	pdf := document.MustNew()
	pageID := pdf.ImportPageStream(bytes.NewReader(source), 1, "MediaBox")
	if pdf.Err() {
		log.Fatal(pdf.Error())
	}

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(0, 10, "Imported page")
	pdf.UseImportedPage(pageID, 25, 35, 150, 0)

	if err := pdf.OutputFileAndClose(outpath.File("import-page.pdf")); err != nil {
		log.Fatal(err)
	}
}

func buildSourcePDF() []byte {
	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Text(72, 96, "Source PDF page")
	pdf.SetDrawColor(40, 90, 160)
	pdf.Rect(70, 112, 220, 50, "D")
	pdf.Text(86, 144, "This page is imported into another PDF.")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		log.Fatal(err)
	}
	return out.Bytes()
}
