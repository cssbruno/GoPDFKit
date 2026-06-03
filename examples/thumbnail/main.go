// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"os"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	file, err := os.Open(assets.File("image", "logo.png"))
	if err != nil {
		panic(err)
	}
	defer func() { _ = file.Close() }()

	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Cell(0, 10, "Generated thumbnail")
	pdf.Ln(14)

	_, err = pdf.RegisterThumbnail("logo-thumb", file, document.ThumbnailOptions{
		MaxWidth:  96,
		MaxHeight: 96,
		Format:    document.ThumbnailFormatPNG,
	})
	if err != nil {
		panic(err)
	}
	pdf.ImageOptions("logo-thumb", 20, 35, 36, 0, false, document.ImageOptions{ImageType: "png"}, 0, "")

	if err := pdf.OutputFileAndClose(outpath.File("thumbnail.pdf")); err != nil {
		panic(err)
	}
}
