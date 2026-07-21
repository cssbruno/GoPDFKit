// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Package samplepdf builds small in-memory PDFs used by page-processing
// examples.
package samplepdf

import (
	"bytes"
	"fmt"

	"github.com/cssbruno/gopdfkit/document"
)

// Build returns a deterministic multi-page source PDF in points.
func Build(title string, pages int) []byte {
	pdf := document.MustNew(document.WithUnit(document.UnitPoint))
	pdf.SetTitle(title, false)
	pdf.SetCreator("examples/internal/samplepdf", false)
	pdf.SetCatalogSort(true)
	pdf.SetCompression(false)

	for page := 1; page <= pages; page++ {
		pdf.AddPage()
		drawPage(pdf, title, page)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		panic(err)
	}
	return out.Bytes()
}

func drawPage(pdf *document.Document, title string, page int) {
	r, g, b := pageColor(page)
	pdf.SetFillColor(r, g, b)
	pdf.Rect(72, 84, 451, 72, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 28)
	pdf.Text(96, 126, fmt.Sprintf("%s page %d", title, page))

	pdf.SetTextColor(35, 45, 55)
	pdf.SetDrawColor(195, 205, 215)
	pdf.Rect(72, 182, 451, 310, "D")
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(96, 224, "Reusable source page")
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetXY(96, 248)
	pdf.MultiCell(384, 18, "This page was generated in memory, then imported by the merge, split, reorder, and rotate examples. The examples do not require external fixture files.", "", "L", false)

	pdf.SetFont("Helvetica", "", 10)
	pdf.SetTextColor(95, 105, 115)
	pdf.Text(72, 760, fmt.Sprintf("Source page %d", page))
}

func pageColor(page int) (int, int, int) {
	colors := [][3]int{
		{35, 90, 150},
		{70, 125, 95},
		{145, 85, 95},
		{110, 95, 155},
	}
	c := colors[(page-1)%len(colors)]
	return c[0], c[1], c[2]
}
