/****************************************************************************
 * Software: GoPDFKit                                                         *
 * License:  MIT License                                                    *
 *                                                                          *
 * Copyright (c) 2026 cssBruno                                              *
 ****************************************************************************/

package gopdfkit

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"
)

func BenchmarkPerfUTF8ToUTF16(b *testing.B) {
	text := strings.Repeat("ASCII Ελληνικά こんにちは 😀 ", 64)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = utf8toutf16(text, false)
	}
}

func BenchmarkPerfReplaceAliasesManyPages(b *testing.B) {
	const pages = 50
	const aliases = 20

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pdf := benchmarkAliasPDF(pages, aliases)
		b.StartTimer()

		pdf.replaceAliases()
	}
}

func benchmarkAliasPDF(pages, aliases int) *Fpdf {
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetFont("Helvetica", "", 10)
	for i := 0; i < aliases; i++ {
		pdf.RegisterAlias(fmt.Sprintf("{mark %d}", i), fmt.Sprintf("%d", i+1))
	}
	for page := 0; page < pages; page++ {
		pdf.AddPage()
		for row := 0; row < 40; row++ {
			for i := 0; i < aliases; i++ {
				pdf.Cell(8, 4, fmt.Sprintf("{mark %d}", i))
			}
			pdf.Ln(4)
		}
	}
	return pdf
}

func BenchmarkPerfRegisterImageOptionsReaderPNGAlpha(b *testing.B) {
	data := benchmarkAlphaPNG(b, 128, 128)
	options := ImageOptions{ImageType: "png"}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pdf := New("P", "mm", "A4", "")
		pdf.RegisterImageOptionsReader("alpha.png", options, bytes.NewReader(data))
		if !pdf.Ok() {
			b.Fatalf("RegisterImageOptionsReader() error = %v", pdf.Error())
		}
	}
}

func benchmarkAlphaPNG(tb testing.TB, width, height int) []byte {
	tb.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x),
				G: uint8(y),
				B: uint8(x + y),
				A: uint8((x*y)%255 + 1),
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		tb.Fatalf("png.Encode() error = %v", err)
	}
	return buf.Bytes()
}

func BenchmarkPerfAddUTF8FontFromCache(b *testing.B) {
	fontBytes, err := os.ReadFile("assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}
	cache := NewFontCache()
	if err := cache.AddUTF8FontFromBytes("DejaVu", "", fontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes() error = %v", err)
	}

	b.SetBytes(int64(len(fontBytes)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pdf := New("P", "mm", "A4", "")
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		if !pdf.Ok() {
			b.Fatalf("AddUTF8FontFromCache() error = %v", pdf.Error())
		}
	}
}
