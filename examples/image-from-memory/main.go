// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			img.Set(x, y, color.RGBA{
				R: uint8(30 + x/2),
				G: uint8(80 + y/2),
				B: uint8(180 - y/3),
				A: 255,
			})
		}
	}

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		log.Fatal(err)
	}

	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(40, 10, "Image from memory")
	pdf.Ln(14)

	options := document.ImageOptions{ImageType: "png"}
	pdf.RegisterImageOptionsReader("gradient", options, bytes.NewReader(pngData.Bytes()))
	pdf.ImageOptions("gradient", 20, 30, 100, 0, false, options, 0, "")

	if err := pdf.OutputFileAndClose(outpath.File("image-from-memory.pdf")); err != nil {
		log.Fatal(err)
	}
}
