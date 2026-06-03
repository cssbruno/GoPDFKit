// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"os"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/examples/internal/assets"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	text, err := os.ReadFile(assets.File("text", "utf-8test.txt"))
	if err != nil {
		log.Fatal(err)
	}
	regularFont, err := os.ReadFile(assets.File("font", "DejaVuSansCondensed.ttf"))
	if err != nil {
		log.Fatal(err)
	}
	boldFont, err := os.ReadFile(assets.File("font", "DejaVuSansCondensed-Bold.ttf"))
	if err != nil {
		log.Fatal(err)
	}

	pdf := gopdfkit.New()
	pdf.AddUTF8FontFromBytes("dejavu", "", regularFont)
	pdf.AddUTF8FontFromBytes("dejavu", "B", boldFont)

	pdf.AddPage()
	pdf.SetFont("dejavu", "B", 16)
	pdf.Cell(0, 10, "UTF-8 TrueType font")
	pdf.Ln(12)
	pdf.SetFont("dejavu", "", 12)
	pdf.MultiCell(170, 6, string(text), "", "L", false)

	if err := pdf.OutputFileAndClose(outpath.File("utf8-font.pdf")); err != nil {
		log.Fatal(err)
	}
}
