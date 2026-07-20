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
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(40, 10, "Hello from PaperRune")

	if err := pdf.OutputFileAndClose(outpath.File("hello-world.pdf")); err != nil {
		log.Fatal(err)
	}
}
