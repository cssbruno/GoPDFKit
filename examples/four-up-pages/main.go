// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
	"github.com/cssbruno/gopdfkit/examples/internal/samplepdf"
)

func main() {
	source := samplepdf.Build("Four Up Source", 8)

	pdf := document.New("P", "pt", "A4", "")
	pdf.SetTitle("Four Up Pages", false)
	pdf.SetCreator("examples/four-up-pages", false)

	slots := []slot{
		{x: 36, y: 42},
		{x: 326, y: 42},
		{x: 36, y: 440},
		{x: 326, y: 440},
	}
	for i, id := range pdf.ImportPagesFromSource(source, "MediaBox") {
		if i%len(slots) == 0 {
			pdf.AddPage()
			drawSheetGuide(pdf)
		}
		s := slots[i%len(slots)]
		pdf.UseImportedPage(id, s.x, s.y, 0, 350)
		pdf.SetFont("Helvetica", "", 8)
		pdf.SetTextColor(80, 90, 100)
		pdf.Text(s.x, s.y+365, fmt.Sprintf("Source page %d", i+1))
	}

	if err := pdf.OutputFileAndClose(outpath.File("four-up-pages.pdf")); err != nil {
		log.Fatal(err)
	}
}

type slot struct {
	x float64
	y float64
}

func drawSheetGuide(pdf *document.Document) {
	pdf.SetDrawColor(210, 218, 226)
	pdf.SetLineWidth(0.5)
	pdf.Line(297.5, 28, 297.5, 814)
	pdf.Line(28, 421, 567, 421)
}
