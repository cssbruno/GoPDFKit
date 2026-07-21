// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"
)

func TestAddTextWatermarkDrawsAndRestoresState(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint))
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetTextColor(11, 22, 33)
	pdf.SetAlpha(0.5, "Multiply")

	pdf.AddTextWatermark("CONFIDENTIAL")

	if pdf.transformNest != 0 {
		t.Fatalf("transformNest = %d, want 0", pdf.transformNest)
	}
	if alpha, blendMode := pdf.GetAlpha(); alpha != 0.5 || blendMode != "Multiply" {
		t.Fatalf("alpha state = %.2f %s, want 0.50 Multiply", alpha, blendMode)
	}
	if r, g, b := pdf.GetTextColor(); r != 11 || g != 22 || b != 33 {
		t.Fatalf("text color = %d %d %d, want 11 22 33", r, g, b)
	}
	if pdf.fontFamily != "helvetica" || pdf.fontStyle != "" || pdf.fontSizePt != 12 {
		t.Fatalf("font state = %s %s %.1f, want helvetica regular 12", pdf.fontFamily, pdf.fontStyle, pdf.fontSizePt)
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("CONFIDENTIAL")) {
		t.Fatal("watermark text missing from generated PDF")
	}
}
