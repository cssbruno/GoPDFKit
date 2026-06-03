// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
	"github.com/cssbruno/gopdfkit/examples/internal/samplepdf"
)

func main() {
	source := samplepdf.Build("Split Source", 4)

	split := document.New("P", "pt", "A4", "")
	split.SetTitle("Split Page 2", false)
	page2 := split.ImportPageStream(bytesReader(source), 2, "MediaBox")
	split.AddPage()
	split.UseImportedPage(page2, 0, 0, 0, 0)
	if err := split.OutputFileAndClose(outpath.File("split-page-2.pdf")); err != nil {
		log.Fatal(err)
	}

	reordered := document.New("P", "pt", "A4", "")
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
