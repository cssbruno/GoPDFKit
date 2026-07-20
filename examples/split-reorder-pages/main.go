// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
	"github.com/cssbruno/paperrune/examples/internal/samplepdf"
)

func main() {
	source := samplepdf.Build("Split Source", 4)

	split := document.MustNew(document.WithUnit(document.UnitPoint))
	split.SetTitle("Split Page 2", false)
	page2 := split.ImportPageStream(bytesReader(source), 2, "MediaBox")
	split.AddPage()
	split.UseImportedPage(page2, 0, 0, 0, 0)
	if err := split.OutputFileAndClose(outpath.File("split-page-2.pdf")); err != nil {
		log.Fatal(err)
	}

	reordered := document.MustNew(document.WithUnit(document.UnitPoint))
	reordered.SetTitle("Reordered PDF Pages", false)
	for _, pageNo := range []int{4, 2, 1, 3} {
		id := reordered.ImportPageStream(bytesReader(source), pageNo, "MediaBox")
		reordered.AddPage()
		reordered.UseImportedPage(id, 0, 0, 0, 0)
	}
	if err := reordered.OutputFileAndClose(outpath.File("reordered-pages.pdf")); err != nil {
		log.Fatal(err)
	}
}

func bytesReader(data []byte) *bytes.Reader {
	return bytes.NewReader(data)
}
