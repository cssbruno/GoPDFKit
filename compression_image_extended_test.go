// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit_test

import (
	"bytes"
	"compress/zlib"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/testsupport/example"
)

func TestCompressionLevelControlsPageCompression(t *testing.T) {
	build := func(configure func(*gopdfkit.Fpdf)) []byte {
		pdf := gopdfkit.New("P", "mm", "A4", "")
		configure(pdf)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		for i := 0; i < 120; i++ {
			pdf.CellFormat(0, 4, strings.Repeat("compression-level ", 8), "", 1, "L", false, 0, "")
		}
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		return out.Bytes()
	}

	uncompressed := build(func(pdf *gopdfkit.Fpdf) {
		pdf.SetNoCompression()
	})
	compressed := build(func(pdf *gopdfkit.Fpdf) {
		pdf.SetCompressionLevel(zlib.BestCompression)
	})
	if !bytes.Contains(compressed, []byte("/Filter /FlateDecode")) {
		t.Fatal("compressed PDF missing FlateDecode filter")
	}
	if len(compressed) >= len(uncompressed) {
		t.Fatalf("compressed PDF size = %d, want smaller than uncompressed %d", len(compressed), len(uncompressed))
	}

	pdf := gopdfkit.New("P", "mm", "A4", "")
	pdf.SetCompressionLevel(zlib.BestCompression + 1)
	if pdf.Ok() {
		t.Fatal("SetCompressionLevel accepted invalid level")
	}
}

func TestImageOptionsExtendedCropRotateFlipAndMask(t *testing.T) {
	maskPath := testMaskForImage(t, example.ImageFile("logo.png"))
	pdf := gopdfkit.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.ImageOptionsExtended(example.ImageFile("logo.png"), gopdfkit.ExtendedImageOptions{
		X:              20,
		Y:              25,
		W:              50,
		H:              0,
		Rotation:       12,
		HorizontalFlip: true,
		Crop:           &gopdfkit.ImageCrop{X: 4, Y: 3, W: 32, H: 24},
		MaskImage:      maskPath,
	})
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	for _, want := range [][]byte{
		[]byte("/SMask"),
		[]byte(" re W n"),
		[]byte(" cm /I"),
	} {
		if !bytes.Contains(out.Bytes(), want) {
			t.Fatalf("extended image output missing %q", want)
		}
	}
}

func testMaskForImage(t *testing.T, imagePath string) string {
	t.Helper()
	file, err := os.Open(imagePath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = file.Close() }()
	cfg, _, err := image.DecodeConfig(file)
	if err != nil {
		t.Fatalf("DecodeConfig() error = %v", err)
	}
	mask := image.NewGray(image.Rect(0, 0, cfg.Width, cfg.Height))
	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			mask.SetGray(x, y, color.Gray{Y: uint8((x + y) % 256)})
		}
	}
	maskPath := filepath.Join(t.TempDir(), "mask.png")
	out, err := os.Create(maskPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := png.Encode(out, mask); err != nil {
		_ = out.Close()
		t.Fatalf("png.Encode() error = %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return maskPath
}
