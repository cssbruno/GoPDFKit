// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	pdf := document.MustNew()
	pdf.SetTitle("Legacy Protected PDF", false)
	pdf.SetCreator("examples/protect-pdf", false)
	if err := pdf.SetLegacyProtection(document.CnProtectPrint, "reader", "owner"); err != nil {
		log.Fatal(err)
	}

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Text(18, 24, "Legacy Protected PDF")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(18, 40)
	pdf.MultiCell(174, 7, "This PDF uses the legacy PDF standard-security handler. The user password is \"reader\". The owner password is \"owner\" and print permission is enabled.", "", "L", false)

	if err := pdf.OutputFileAndClose(outpath.File("protected-password.pdf")); err != nil {
		log.Fatal(err)
	}
}
