// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Images on Pages", false)
	pdf.SetCreator("examples/add-images-to-pages", false)

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Text(18, 22, "Add Images to Pages")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(18, 31, "PNG, JPEG, and GIF assets placed with explicit dimensions.")

	drawImageCard(pdf, 18, 48, "PNG logo", assets.File("image", "logo.png"), document.ImageOptions{})
	drawImageCard(pdf, 112, 48, "JPEG logo", assets.File("image", "logo.jpg"), document.ImageOptions{ImageType: "jpg"})
	drawImageCard(pdf, 18, 134, "GIF logo", assets.File("image", "logo.gif"), document.ImageOptions{ImageType: "gif"})
	drawImageCard(pdf, 112, 134, "Gopher PNG", assets.File("image", "golang-gopher.png"), document.ImageOptions{})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(18, 22, "Repeated page image")
	pdf.ImageOptions(assets.File("image", "paperrune-kit.png"), 18, 36, 174, 0, false, document.ImageOptions{}, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.SetXY(18, 205)
	pdf.MultiCell(174, 6, "The same ImageOptions API can place registered images on any page. Width or height may be set to zero to preserve aspect ratio.", "", "L", false)

	if err := pdf.OutputFileAndClose(outpath.File("images-on-pages.pdf")); err != nil {
		log.Fatal(err)
	}
}

func drawImageCard(pdf *document.Document, x, y float64, label, imagePath string, options document.ImageOptions) {
	pdf.SetDrawColor(210, 218, 226)
	pdf.SetFillColor(248, 250, 252)
	pdf.RoundedRect(x, y, 80, 68, 3, "1234", "DF")
	pdf.ImageOptions(imagePath, x+10, y+9, 60, 0, false, options, 0, "")
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetTextColor(35, 45, 55)
	pdf.Text(x+10, y+58, label)
}
