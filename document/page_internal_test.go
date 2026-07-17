// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestAddPageFormatRotationWritesRotateEntry(t *testing.T) {
	pdf := MustNew()
	pdf.SetCompression(false)
	pdf.AddPageFormatRotation("P", Size{Wd: 210, Ht: 297}, 90)
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(10, 10, "rotated")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(out.String(), "/Rotate 90") {
		t.Fatal("generated PDF does not contain /Rotate 90")
	}
}

func TestAddPageFormatRotationRejectsInvalidRotation(t *testing.T) {
	pdf := MustNew()
	pdf.AddPageFormatRotation("P", Size{Wd: 210, Ht: 297}, 45)
	if pdf.Ok() {
		t.Fatal("invalid page rotation did not set an error")
	}
}

func TestSetYWithResetXCanPreserveX(t *testing.T) {
	pdf := MustNew()
	pdf.AddPage()
	pdf.SetX(42)
	pdf.SetYWithResetX(30, false)

	x, y := pdf.GetXY()
	if x != 42 || y != 30 {
		t.Fatalf("position = %.2f,%.2f; want 42.00,30.00", x, y)
	}
}

func TestGetPageWidthAndHeight(t *testing.T) {
	pdf := MustNew(WithOrientation(OrientationLandscape))
	pdf.AddPage()
	if got, want := pdf.GetPageWidth(), 297.0000833333333; !floatEquals(got, want) {
		t.Fatalf("GetPageWidth() = %.8f, want %.8f", got, want)
	}
	if got, want := pdf.GetPageHeight(), 210.0015555555555; !floatEquals(got, want) {
		t.Fatalf("GetPageHeight() = %.8f, want %.8f", got, want)
	}
}

func floatEquals(a, b float64) bool {
	const epsilon = 1e-9
	if a < b {
		return b-a < epsilon
	}
	return a-b < epsilon
}
