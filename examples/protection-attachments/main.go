// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"os"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/examples/internal/outpath"
)

func main() {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		log.Fatal(err)
	}

	pdf := document.MustNew()
	if err := pdf.SetLegacyProtection(document.CnProtectPrint, "reader", "owner"); err != nil {
		log.Fatal(err)
	}
	pdf.SetAttachments([]document.Attachment{{
		Content:     readme,
		Filename:    "README.md",
		Description: "Repository README attached from an example",
	}})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.MultiCell(0, 7, "This PDF uses legacy PDF standard-security protection and has README.md embedded as a document-level attachment.", "", "L", false)

	if err := pdf.OutputFileAndClose(outpath.File("protection-attachments.pdf")); err != nil {
		log.Fatal(err)
	}
}
