// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	pdf := gopdfkit.New()
	pdf.SetTitle("Password Protected PDF", false)
	pdf.SetCreator("examples/protect-pdf", false)
	pdf.SetProtection(document.CnProtectPrint, "reader", "owner")

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Text(18, 24, "Protected PDF")
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(18, 40)
	pdf.MultiCell(174, 7, "This PDF requires the user password \"reader\" to open. The owner password is \"owner\" and print permission is enabled.", "", "L", false)

	if err := pdf.OutputFileAndClose(outpath.File("protected-password.pdf")); err != nil {
		log.Fatal(err)
	}
}
