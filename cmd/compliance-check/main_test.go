// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"strings"
	"testing"
)

func TestCheckPDFTextChecksEveryNamedProfile(t *testing.T) {
	t.Parallel()

	pdfaOnly := strings.Join([]string{
		"%PDF-2.0",
		"/Metadata",
		"<pdfaid:part>4</pdfaid:part>",
		"/OutputIntents",
		"/DestOutputProfile",
	}, " ")
	err := checkPDFText("combined.pdf", "pdfa4f-pdfua2-arlington-signed.pdf", pdfaOnly)
	if err == nil || !strings.Contains(err.Error(), "PDF/UA-2 XMP identifier") {
		t.Fatalf("combined profile error = %v, want missing PDF/UA-2 marker", err)
	}

	err = checkPDFText("combined.pdf", "pdfa4f-pdfua2-arlington-signed.pdf", "%PDF-2.0 /Metadata")
	if err == nil || !strings.Contains(err.Error(), "PDF/A-4 XMP identifier") {
		t.Fatalf("combined profile error = %v, want missing PDF/A-4 marker", err)
	}
}
