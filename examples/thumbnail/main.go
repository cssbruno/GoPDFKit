// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"os"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/assets"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	file, err := os.Open(assets.File("image", "logo.png"))
	if err != nil {
		log.Fatal(err)
	}
	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(0, 10, "Generated thumbnail")
	pdf.Ln(14)

	_, err = pdf.RegisterThumbnail("logo-thumb", file, document.ThumbnailOptions{
		MaxWidth:  96,
		MaxHeight: 96,
		Format:    document.ThumbnailFormatPNG,
	})
	if closeErr := file.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		log.Fatal(err)
	}
	pdf.ImageOptions("logo-thumb", 20, 35, 36, 0, false, document.ImageOptions{ImageType: "png"}, 0, "")

	if err := pdf.OutputFileAndClose(outpath.File("thumbnail.pdf")); err != nil {
		log.Fatal(err)
	}
}
