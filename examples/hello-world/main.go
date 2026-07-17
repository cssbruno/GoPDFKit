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
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(40, 10, "Hello from GoPDFKit")

	if err := pdf.OutputFileAndClose(outpath.File("hello-world.pdf")); err != nil {
		log.Fatal(err)
	}
}
