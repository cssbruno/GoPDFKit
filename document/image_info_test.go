// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateImageIDRejectsNilInfo(t *testing.T) {
	if _, err := generateImageID(nil); err == nil {
		t.Fatal("generateImageID(nil) returned nil error")
	}
}

func TestGenerateImageIDUsesSHA256(t *testing.T) {
	info := &ImageInfo{data: []byte("image"), w: 1, h: 1, cs: "DeviceRGB", bpc: 8, f: "DCTDecode", dpi: 72}
	resourceID, err := generateImageResourceID(info)
	if err != nil {
		t.Fatalf("generateImageResourceID() error = %v", err)
	}
	id, err := generateImageID(info)
	if err != nil {
		t.Fatalf("generateImageID() error = %v", err)
	}
	if len(id) != 64 {
		t.Fatalf("image ID length = %d, want SHA-256 hex length", len(id))
	}
	if got := base64.StdEncoding.EncodeToString(resourceID[:]); got == "" {
		t.Fatal("typed image resource ID is empty")
	}
}

func TestGenerateImageIDIgnoresRuntimeState(t *testing.T) {
	base := &ImageInfo{data: []byte("image"), w: 1, h: 1, cs: "DeviceRGB", bpc: 8, f: "DCTDecode", dpi: 72, n: 1, scale: 1}
	other := base.clone()
	other.n = 99
	other.scale = 72

	baseID, err := generateImageID(base)
	if err != nil {
		t.Fatalf("generateImageID(base) error = %v", err)
	}
	otherID, err := generateImageID(other)
	if err != nil {
		t.Fatalf("generateImageID(other) error = %v", err)
	}
	if baseID != otherID {
		t.Fatalf("image IDs differ for runtime-only fields: %s != %s", baseID, otherID)
	}
}

func TestGenerateImageIDIncludesDPI(t *testing.T) {
	base := &ImageInfo{data: []byte("image"), w: 1, h: 1, cs: "DeviceRGB", bpc: 8, f: "DCTDecode", dpi: 72}
	other := base.clone()
	other.dpi = 144

	baseID, err := generateImageID(base)
	if err != nil {
		t.Fatalf("generateImageID(base) error = %v", err)
	}
	otherID, err := generateImageID(other)
	if err != nil {
		t.Fatalf("generateImageID(other) error = %v", err)
	}
	if baseID == otherID {
		t.Fatal("image IDs are equal after changing DPI")
	}
}

func TestRegisteredImageIDStableAcrossUnitsAndOutputState(t *testing.T) {
	register := func(unit string) (*Document, *ImageInfo) {
		t.Helper()
		pdf := New("P", unit, "A4", "")
		info, err := pdf.RegisterImageOptionsReaderError("pixel", ImageOptions{ImageType: "png"}, bytes.NewReader(decodeTinyPNG(t)))
		if err != nil {
			t.Fatalf("RegisterImageOptionsReaderError(%s) error = %v", unit, err)
		}
		return pdf, info
	}

	mmDoc, mmInfo := register("mm")
	_, ptInfo := register("pt")
	if mmInfo.i != ptInfo.i {
		t.Fatalf("image ID differs by document unit: mm=%s pt=%s", mmInfo.i, ptInfo.i)
	}

	beforeOutputID := mmInfo.i
	mmDoc.SetCompression(false)
	mmDoc.AddPage()
	mmDoc.ImageOptions("pixel", 10, 10, 5, 5, false, ImageOptions{ImageType: "png"}, 0, "")
	var out bytes.Buffer
	if err := mmDoc.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if mmInfo.n == 0 {
		t.Fatal("output did not assign an image object number")
	}
	if mmInfo.i != beforeOutputID {
		t.Fatalf("image ID changed after output object assignment: %s != %s", mmInfo.i, beforeOutputID)
	}
}

func TestRegisteredImageIDsUseSHA256AcrossFormats(t *testing.T) {
	cases := []struct {
		name      string
		imageType string
		data      []byte
	}{
		{name: "png-alpha", imageType: "png", data: encodeAlphaPNG(t)},
		{name: "jpeg", imageType: "jpg", data: encodeTinyJPEG(t)},
		{name: "gif", imageType: "gif", data: encodeTinyGIF(t)},
		{name: "webp", imageType: "webp", data: decodeTinyWebP(t)},
		{name: "indexed-png", imageType: "png", data: encodeIndexedPNG(t)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pdf := New("P", "mm", "A4", "")
			info, err := pdf.RegisterImageOptionsReaderError(tc.name, ImageOptions{ImageType: tc.imageType}, bytes.NewReader(tc.data))
			if err != nil {
				t.Fatalf("RegisterImageOptionsReaderError() error = %v", err)
			}
			if len(info.i) != 64 {
				t.Fatalf("image ID length = %d, want SHA-256 hex length", len(info.i))
			}
		})
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

func TestImageCacheRegistersImageAcrossDocuments(t *testing.T) {
	cache := NewImageCache()
	if _, err := cache.RegisterImageOptionsReader("pixel", ImageOptions{ImageType: "png"}, bytes.NewReader(decodeTinyPNG(t))); err != nil {
		t.Fatalf("RegisterImageOptionsReader(cache) error = %v", err)
	}

	for i := 0; i < 2; i++ {
		pdf := New("P", "mm", "A4", "")
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.ImageFromCache("pixel", cache, 10, 10, 5, 5, false, ImageOptions{}, 0, "")
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			t.Fatalf("Output(%d) error = %v", i, err)
		}
		if !strings.Contains(out.String(), "/Subtype /Image") {
			t.Fatalf("generated PDF %d missing image object", i)
		}
	}
}

func TestImageCacheReusesFileRegistrationByPathStatAndOptions(t *testing.T) {
	cache := NewImageCache()
	imagePath := filepath.Join(t.TempDir(), "pixel.PNG")
	if err := os.WriteFile(imagePath, decodeTinyPNG(t), 0o600); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	first, err := cache.RegisterImageOptions("first", imagePath, ImageOptions{})
	if err != nil {
		t.Fatalf("first RegisterImageOptions(cache) error = %v", err)
	}
	second, err := cache.RegisterImageOptions("second", imagePath, ImageOptions{})
	if err != nil {
		t.Fatalf("second RegisterImageOptions(cache) error = %v", err)
	}
	if first.Width() != second.Width() || first.Height() != second.Height() {
		t.Fatalf("cached dimensions differ: %.2fx%.2f vs %.2fx%.2f", first.Width(), first.Height(), second.Width(), second.Height())
	}
	if got := len(cache.fileImages); got != 1 {
		t.Fatalf("cached file images = %d, want 1", got)
	}
	if got := len(cache.fileTypes); got != 1 {
		t.Fatalf("cached file types = %d, want 1", got)
	}
}

func TestRegisterImageOptionsUsesSharedFileCacheByDefault(t *testing.T) {
	previousCache := sharedImageFileCache
	sharedImageFileCache = newImageCache(maxSharedImageFileCacheBytes)
	defer func() { sharedImageFileCache = previousCache }()

	imagePath := filepath.Join(t.TempDir(), "pixel.PNG")
	if err := os.WriteFile(imagePath, decodeTinyPNG(t), 0o600); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	relPath, err := filepath.Rel(wd, imagePath)
	if err != nil {
		t.Fatalf("relative image path: %v", err)
	}

	for i, path := range []string{imagePath, relPath} {
		pdf := New("P", "mm", "A4", "")
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.ImageOptions(path, 10, 10, 5, 5, false, ImageOptions{}, 0, "")
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			t.Fatalf("Output(%d) error = %v", i, err)
		}
		if !strings.Contains(out.String(), "/Subtype /Image") {
			t.Fatalf("generated PDF %d missing image object", i)
		}
	}

	sharedImageFileCache.mu.RLock()
	fileImages := len(sharedImageFileCache.fileImages)
	fileTypes := len(sharedImageFileCache.fileTypes)
	sharedImageFileCache.mu.RUnlock()
	if fileImages != 1 {
		t.Fatalf("shared file images = %d, want 1", fileImages)
	}
	if fileTypes != 1 {
		t.Fatalf("shared file types = %d, want 1", fileTypes)
	}
}

func TestRegisterImageOptionsCanDisableSharedFileCache(t *testing.T) {
	previousCache := sharedImageFileCache
	sharedImageFileCache = newImageCache(maxSharedImageFileCacheBytes)
	defer func() { sharedImageFileCache = previousCache }()

	imagePath := filepath.Join(t.TempDir(), "pixel.PNG")
	if err := os.WriteFile(imagePath, decodeTinyPNG(t), 0o600); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	pdf := NewWithOptions(Options{CachePolicy: ResourceCacheDisabled})
	pdf.SetCompression(false)
	pdf.AddPage()
	if _, err := pdf.RegisterImageOptionsError(imagePath, ImageOptions{}); err != nil {
		t.Fatalf("RegisterImageOptionsError() error = %v", err)
	}
	pdf.ImageOptions(imagePath, 10, 10, 5, 5, false, ImageOptions{}, 0, "")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}

	sharedImageFileCache.mu.RLock()
	fileImages := len(sharedImageFileCache.fileImages)
	sharedImageFileCache.mu.RUnlock()
	if fileImages != 0 {
		t.Fatalf("shared file images = %d, want 0", fileImages)
	}
}

func TestImageFromCacheWithAlphaPromotesPDFVersion(t *testing.T) {
	cache := NewImageCache()
	if _, err := cache.RegisterImageOptionsReader("alpha", ImageOptions{ImageType: "png"}, bytes.NewReader(encodeAlphaPNG(t))); err != nil {
		t.Fatalf("RegisterImageOptionsReader(cache) error = %v", err)
	}

	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.ImageFromCache("alpha", cache, 10, 10, 5, 5, false, ImageOptions{}, 0, "")
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !bytes.HasPrefix(out.Bytes(), []byte("%PDF-1.4")) {
		t.Fatalf("PDF header = %q, want PDF 1.4 for alpha image", out.Bytes()[:8])
	}
}

func TestImageFromCacheMissingEntrySetsDocumentError(t *testing.T) {
	pdf := New("P", "mm", "A4", "")
	pdf.ImageFromCache("missing", NewImageCache(), 0, 0, 1, 1, false, ImageOptions{}, 0, "")
	if err := pdf.Error(); err == nil {
		t.Fatal("ImageFromCache missing entry error = nil")
	}
}

func encodeAlphaPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0xff, A: 0x80})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode PNG fixture: %v", err)
	}
	return buf.Bytes()
}

func encodeIndexedPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{
		color.RGBA{A: 0xff},
		color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff},
	})
	img.SetColorIndex(0, 0, 1)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode indexed PNG fixture: %v", err)
	}
	return buf.Bytes()
}

func encodeTinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 0xff, G: 0x80, A: 0xff})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode JPEG fixture: %v", err)
	}
	return buf.Bytes()
}

func encodeTinyGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{
		color.RGBA{A: 0xff},
		color.RGBA{B: 0xff, A: 0xff},
	})
	img.SetColorIndex(0, 0, 1)
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatalf("encode GIF fixture: %v", err)
	}
	return buf.Bytes()
}

func decodeTinyPNG(t *testing.T) []byte {
	t.Helper()
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	data, err := base64.StdEncoding.DecodeString(tinyPNG)
	if err != nil {
		t.Fatalf("decode PNG fixture: %v", err)
	}
	return data
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
