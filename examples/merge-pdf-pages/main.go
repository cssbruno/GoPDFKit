// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
	"github.com/cssbruno/gopdfkit/examples/internal/samplepdf"
)

func main() {
	first := samplepdf.Build("First Source", 2)
	second := samplepdf.Build("Second Source", 2)

	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetTitle("Merged PDF Pages", false)
	pdf.SetCreator("examples/merge-pdf-pages", false)

	appendPages(pdf, first)
	appendPages(pdf, second)

	if err := pdf.OutputFileAndClose(outpath.File("merged-pages.pdf")); err != nil {
		log.Fatal(err)
	}
}

func appendPages(pdf *document.Document, source []byte) {
	ids := pdf.ImportPagesFromSource(source, "MediaBox")
	for _, id := range ids {
		pdf.AddPage()
		pdf.UseImportedPage(id, 0, 0, 0, 0)
	}
}
