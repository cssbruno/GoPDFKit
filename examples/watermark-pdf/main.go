// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
	"github.com/cssbruno/paperrune/examples/internal/samplepdf"
)

func main() {
	source := samplepdf.Build("Watermark Source", 3)

	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetTitle("Watermarked PDF", false)
	pdf.SetCreator("examples/watermark-pdf", false)

	for _, id := range pdf.ImportPagesFromSource(source, "MediaBox") {
		pdf.AddPage()
		pdf.UseImportedPage(id, 0, 0, 0, 0)
		pdf.AddTextWatermark("CONFIDENTIAL")
	}

	if err := pdf.OutputFileAndClose(outpath.File("watermarked.pdf")); err != nil {
		log.Fatal(err)
	}
}
