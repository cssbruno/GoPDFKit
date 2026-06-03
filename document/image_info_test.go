// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGenerateImageIDRejectsNilInfo(t *testing.T) {
	if _, err := generateImageID(nil); err == nil {
		t.Fatal("generateImageID(nil) returned nil error")
	}
}

func TestImageTypeFromMimeSupportsWebP(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	if got := pdf.ImageTypeFromMime("image/webp"); got != "webp" {
		t.Fatalf("ImageTypeFromMime(image/webp) = %q, want webp", got)
	}
}

func TestRegisterImageOptionsReaderSupportsWebP(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()

	info := pdf.RegisterImageOptionsReader("tiny-webp", ImageOptions{ImageType: "webp"}, bytes.NewReader(decodeTinyWebP(t)))
	if err := pdf.Error(); err != nil {
		t.Fatalf("RegisterImageOptionsReader error = %v", err)
	}
	if info == nil {
		t.Fatal("RegisterImageOptionsReader returned nil image info")
	}
	if info.w != 1 || info.h != 1 {
		t.Fatalf("webp dimensions = %.0fx%.0f, want 1x1", info.w, info.h)
	}

	pdf.ImageOptions("tiny-webp", 10, 10, 5, 5, false, ImageOptions{ImageType: "webp"}, 0, "")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := out.String()
	for _, want := range []string{"/Subtype /Image", "/Filter /FlateDecode"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain %q", want)
		}
	}
}

func decodeTinyWebP(t *testing.T) []byte {
	t.Helper()
	const tinyWebP = "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"
	data, err := base64.StdEncoding.DecodeString(tinyWebP)
	if err != nil {
		t.Fatalf("decode WebP fixture: %v", err)
	}
	return data
}
