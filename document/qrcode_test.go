// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestQRCodePNGRendersPNGAtRequestedSize(t *testing.T) {
	data, err := QRCodePNG("https://example.test/verify/document-1", 96)
	if err != nil {
		t.Fatalf("QRCodePNG() error = %v", err)
	}

	if !bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatalf("QRCodePNG() did not return PNG bytes")
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != 96 || bounds.Dy() != 96 {
		t.Fatalf("QR image size = %dx%d, want 96x96", bounds.Dx(), bounds.Dy())
	}
}

func TestQRCodePNGRejectsEmptyPayload(t *testing.T) {
	if _, err := QRCodePNG("  ", 96); err == nil || !strings.Contains(err.Error(), "qr payload is empty") {
		t.Fatalf("QRCodePNG() error = %v, want empty payload error", err)
	}
}

func TestQRCodeImageNameIsStable(t *testing.T) {
	first := QRCodeImageName(" https://example.test/verify ")
	second := QRCodeImageName("https://example.test/verify")

	if first != second {
		t.Fatalf("QRCodeImageName() = %q then %q, want stable trimmed name", first, second)
	}
	if !strings.HasPrefix(first, "qr_") {
		t.Fatalf("QRCodeImageName() = %q, want qr_ prefix", first)
	}
}

func TestRegisterQRCodePNGRegistersImage(t *testing.T) {
	pdf := MustNew()
	name, err := pdf.RegisterQRCodePNG("https://example.test/verify/document-1", 64)
	if err != nil {
		t.Fatalf("RegisterQRCodePNG() error = %v", err)
	}
	again, err := pdf.RegisterQRCodePNG(" https://example.test/verify/document-1 ", 128)
	if err != nil {
		t.Fatalf("RegisterQRCodePNG() duplicate error = %v", err)
	}
	if again != name {
		t.Fatalf("duplicate RegisterQRCodePNG() name = %q, want %q", again, name)
	}
	if got := len(pdf.ensureResourceStore().images); got != 1 {
		t.Fatalf("registered QR image count = %d, want 1", got)
	}

	if want := QRCodeImageName("https://example.test/verify/document-1"); name != want {
		t.Fatalf("RegisterQRCodePNG() name = %q, want %q", name, want)
	}

	pdf.AddPage()
	pdf.ImageOptions(name, 10, 10, 20, 20, false, ImageOptions{ImageType: "png", ReadDpi: true}, 0, "")

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}

	if !strings.Contains(out.String(), "/Subtype /Image") {
		t.Fatalf("registered QR image was not written into output")
	}
}
