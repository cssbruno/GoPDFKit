// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"log"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/examples/internal/outpath"
)

func main() {
	form := document.FormDocument{
		Title: "Client Intake Form",
		Sections: []document.FormSection{
			{
				Title:        "Contact",
				KeepTogether: true,
				Questions: []document.FormQuestion{
					{Label: "Client name", Required: true, Answer: document.FormAnswer{Text: "Northwind Trading"}},
					{Label: "Email", Required: true, Answer: document.FormAnswer{Text: "ops@example.test"}},
					{Label: "Services requested", Answer: document.FormAnswer{Items: []string{"Monthly PDF reporting", "Invoice generation", "Watermarked document delivery"}}},
				},
			},
			{
				Title:        "Line items",
				KeepTogether: true,
				Questions: []document.FormQuestion{
					{
						Label: "Approved scope",
						Answer: document.FormAnswer{Table: [][]string{
							{"Item", "Qty", "Owner"},
							{"Reports", "12", "Analytics"},
							{"Invoices", "80", "Finance"},
							{"Templates", "4", "Operations"},
						}},
					},
				},
			},
		},
	}

	pdf := document.MustNew()
	pdf.SetCreator("examples/form-creation", false)
	pdf.WriteDocument(document.FormDocumentModel(form))

	if err := pdf.OutputFileAndClose(outpath.File("form-creation.pdf")); err != nil {
		log.Fatal(err)
	}
}
