// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfkit_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/testsupport/example"
)

func TestGenerateFitsWithinBounds(t *testing.T) {
	file, err := os.Open(example.ImageFile("logo.png"))
	if err != nil {
		t.Fatalf("open source image: %s", err)
	}
	defer func() { _ = file.Close() }()

	data, format, err := gopdfkit.GenerateThumbnail(file, gopdfkit.ThumbnailOptions{MaxWidth: 32, MaxHeight: 16, Format: gopdfkit.ThumbnailFormatPNG})
	if err != nil {
		t.Fatalf("generate thumbnail: %s", err)
	}
	if format != gopdfkit.ThumbnailFormatPNG {
		t.Fatalf("expected png format, got %q", format)
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode thumbnail: %s", err)
	}
	if img.Bounds().Dx() > 32 || img.Bounds().Dy() > 16 {
		t.Fatalf("thumbnail does not fit bounds: %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestGenerateDoesNotUpscaleByDefault(t *testing.T) {
	var buf bytes.Buffer
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	src.Set(0, 0, color.White)
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("encode source image: %s", err)
	}

	data, _, err := gopdfkit.GenerateThumbnail(bytes.NewReader(buf.Bytes()), gopdfkit.ThumbnailOptions{MaxWidth: 40, MaxHeight: 40})
	if err != nil {
		t.Fatalf("generate thumbnail: %s", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode thumbnail: %s", err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 2 {
		t.Fatalf("expected original size, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestGenerateUpscalesWhenRequested(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	data, _, err := gopdfkit.GenerateThumbnailImage(src, "png", gopdfkit.ThumbnailOptions{MaxWidth: 40, MaxHeight: 40, Upscale: true})
	if err != nil {
		t.Fatalf("generate thumbnail: %s", err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode thumbnail: %s", err)
	}
	if img.Bounds().Dx() != 40 || img.Bounds().Dy() != 20 {
		t.Fatalf("expected 40x20 thumbnail, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestRegisterAddsThumbnailImage(t *testing.T) {
	file, err := os.Open(example.ImageFile("logo.png"))
	if err != nil {
		t.Fatalf("open source image: %s", err)
	}
	defer func() { _ = file.Close() }()

	pdf := gopdfkit.New("P", "mm", "A4", "")
	info, err := pdf.RegisterThumbnail("logo-thumb", file, gopdfkit.ThumbnailOptions{MaxWidth: 48, MaxHeight: 48})
	if err != nil {
		t.Fatalf("register thumbnail: %s", err)
	}
	if pdf.Error() != nil {
		t.Fatalf("unexpected pdf error: %s", pdf.Error())
	}
	if info == nil {
		t.Fatal("expected image info")
	}
}

func ExampleGenerateThumbnail() {
	file, err := os.Open(example.ImageFile("logo.png"))
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = file.Close() }()

	data, format, err := gopdfkit.GenerateThumbnail(file, gopdfkit.ThumbnailOptions{MaxWidth: 64, MaxHeight: 64})
	if err != nil {
		fmt.Println(err)
		return
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(format, img.Bounds().Dx() <= 64, img.Bounds().Dy() <= 64)
	// Output:
	// png true true
}

func ExampleFpdf_RegisterThumbnail() {
	pdf := gopdfkit.New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 12)
	pdf.AddPage()

	file, err := os.Open(example.ImageFile("logo.png"))
	if err == nil {
		_, err = pdf.RegisterThumbnail("logo-thumb", file, gopdfkit.ThumbnailOptions{MaxWidth: 96, MaxHeight: 96})
		_ = file.Close()
	}
	if err == nil {
		pdf.Cell(0, 8, "Generated thumbnail")
		pdf.Ln(12)
		pdf.ImageOptions("logo-thumb", 10, 22, 32, 0, false, gopdfkit.ImageOptions{}, 0, "")
	}

	fileStr := example.Filename("thumb_Register")
	if err == nil {
		err = pdf.OutputFileAndClose(fileStr)
	}
	example.Summary(err, fileStr)
	// Output:
	// Successfully generated assets/generated/pdf/thumb_Register.pdf
}
