// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
	"github.com/cssbruno/gopdfkit/examples/internal/samplepdf"
)

func main() {
	source := samplepdf.Build("Rotate Source", 3)
	sizes, err := document.GetPageSizes(source)
	if err != nil {
		panic(err)
	}

	pdf := document.New("P", "pt", "A4", "")
	pdf.SetTitle("Rotated PDF Pages", false)
	pdf.SetCreator("examples/rotate-pages", false)

	rotations := []int{0, 90, 270}
	for pageNo, rotation := range rotations {
		pageNo++
		id := pdf.ImportPageStream(bytes.NewReader(source), pageNo, "MediaBox")
		size := sizes[pageNo]["MediaBox"]
		pdf.AddPageFormatRotation("P", size, rotation)
		pdf.UseImportedPage(id, 0, 0, 0, 0)
	}

	if err := pdf.OutputFileAndClose(outpath.File("rotated-pages.pdf")); err != nil {
		panic(err)
	}
}
