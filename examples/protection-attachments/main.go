// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"log"
	"os"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		log.Fatal(err)
	}

	pdf := gopdfkit.New()
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
