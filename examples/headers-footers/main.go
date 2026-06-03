// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"fmt"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := gopdfkit.New()
	pdf.AliasNbPages("{total}")

	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 12)
		pdf.CellFormat(0, 8, "Monthly Report", "B", 1, "C", false, 0, "")
		pdf.Ln(4)
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-16)
		pdf.SetFont("Helvetica", "", 9)
		pdf.CellFormat(0, 8, fmt.Sprintf("Page %d / {total}", pdf.PageNo()), "T", 0, "C", false, 0, "")
	})

	pdf.SetFont("Helvetica", "", 11)
	for page := 1; page <= 3; page++ {
		pdf.AddPage()
		for line := 1; line <= 28; line++ {
			pdf.CellFormat(0, 7, fmt.Sprintf("Page %d, row %02d", page, line), "", 1, "L", false, 0, "")
		}
	}

	if err := pdf.OutputFileAndClose(outpath.File("headers-footers.pdf")); err != nil {
		panic(err)
	}
}
