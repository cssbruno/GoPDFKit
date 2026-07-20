// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/layout"
)

func TestStringWidthCacheUsesBoundedRing(t *testing.T) {
	pdf := MustNew()
	pdf.SetFont("Helvetica", "", 12)
	for i := 0; i < stringWidthCacheLimit+16; i++ {
		pdf.GetStringSymbolWidth(fmt.Sprintf("cache-key-%03d", i))
	}
	if got := len(pdf.stringWidthCache); got != stringWidthCacheLimit {
		t.Fatalf("string width cache size = %d, want %d", got, stringWidthCacheLimit)
	}
	if got := len(pdf.stringWidthKeys); got != stringWidthCacheLimit {
		t.Fatalf("string width key ring size = %d, want %d", got, stringWidthCacheLimit)
	}
	if got := pdf.stringWidthKeyNext; got != 16 {
		t.Fatalf("string width key ring next = %d, want 16", got)
	}
	first, _ := pdf.stringWidthCacheKey("cache-key-000")
	if _, ok := pdf.stringWidthCache[first]; ok {
		t.Fatal("oldest string width entry was not evicted")
	}
	recent, _ := pdf.stringWidthCacheKey(fmt.Sprintf("cache-key-%03d", stringWidthCacheLimit+15))
	if _, ok := pdf.stringWidthCache[recent]; !ok {
		t.Fatal("most recent string width entry is missing")
	}
}

func TestContentCommandBufferReuseIsBounded(t *testing.T) {
	pdf := MustNew()
	buffer := pdf.contentCommandBuffer(128)
	buffer = append(buffer, make([]byte, 128)...)
	pdf.retainContentCommandBuffer(buffer)
	retainedCapacity := cap(pdf.contentScratch)
	if retainedCapacity < 128 {
		t.Fatalf("retained content scratch capacity = %d, want at least 128", retainedCapacity)
	}
	if reused := pdf.contentCommandBuffer(64); cap(reused) != retainedCapacity {
		t.Fatalf("reused content scratch capacity = %d, want %d", cap(reused), retainedCapacity)
	}

	oversized := make([]byte, 0, maxContentScratchCapacity+1)
	pdf.retainContentCommandBuffer(oversized)
	if got := cap(pdf.contentScratch); got != retainedCapacity {
		t.Fatalf("oversized content scratch changed retained capacity to %d, want %d", got, retainedCapacity)
	}
}

func BenchmarkPerfUTF8ToUTF16(b *testing.B) {
	text := strings.Repeat("ASCII Ελληνικά こんにちは 😀 ", 64)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = utf8toutf16(text, false)
	}
}

var (
	benchmarkTaggedKidsSink []byte
	benchmarkSVGPathSink    []SVGSegment
)

func BenchmarkPerfTaggedElementKidsLarge(b *testing.B) {
	pdf := &Document{}
	pdf.tagged.pageObjNums = make([]int, 65)
	for i := range pdf.tagged.pageObjNums {
		pdf.tagged.pageObjNums[i] = 1000 + i
	}
	elem := &taggedElement{
		Page:   1,
		MCID:   0,
		ObjRef: 9001,
	}
	for i := 1; i <= 256; i++ {
		elem.Marked = append(elem.Marked, taggedMarkedContent{Page: i%64 + 1, MCID: i})
		elem.Children = append(elem.Children, &taggedElement{ObjNum: 2000 + i})
	}
	kidCount := taggedElementKidCount(elem)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		out := make([]byte, 0, 8+kidCount*32)
		out = append(out, "/K ["...)
		out = pdf.appendTaggedElementKids(out, elem, true)
		out = append(out, ']')
		benchmarkTaggedKidsSink = out
	}
}

func BenchmarkPerfSVGPathParseHeavy(b *testing.B) {
	path := benchmarkSVGHeavyPath(4000)

	b.SetBytes(int64(len(path)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		segs, err := pathParse(path)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkSVGPathSink = segs
	}
}

func benchmarkSVGHeavyPath(segments int) string {
	var out strings.Builder
	out.Grow(segments * 28)
	out.WriteString("M0 0")
	for i := 0; i < segments; i++ {
		switch i % 6 {
		case 0:
			fmt.Fprintf(&out, " l%d.%d-%d.%d", i%97+1, i%10, i%53+1, (i+3)%10)
		case 1:
			fmt.Fprintf(&out, " h%d v-%d", i%101+1, i%79+1)
		case 2:
			fmt.Fprintf(&out, " c1e-3-2e-3 3E+0-4E+0 %d-%d", i%89+1, i%67+1)
		case 3:
			fmt.Fprintf(&out, " s%d %d %d %d", i%47+1, i%43+1, i%41+1, i%37+1)
		case 4:
			fmt.Fprintf(&out, " q%d %d %d %d t%d-%d", i%31+1, i%29+1, i%23+1, i%19+1, i%17+1, i%13+1)
		case 5:
			fmt.Fprintf(&out, " a10 8 0 0 1 %d %d", i%11+1, i%7+1)
		}
	}
	out.WriteByte('z')
	return out.String()
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

func BenchmarkPerfReplaceAliasesNoMatchesManyPages(b *testing.B) {
	const pages = 50
	const aliases = 20

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		pdf := benchmarkNoMatchAliasPDF(pages, aliases)
		b.StartTimer()

		pdf.replaceAliases()
	}
}

func benchmarkAliasPDF(pages, aliases int) *Document {
	pdf := MustNew()
	pdf.SetCompression(false)
	pdf.SetFont("Helvetica", "", 10)
	for i := 0; i < aliases; i++ {
		pdf.RegisterAlias(fmt.Sprintf("{mark %d}", i), strconv.Itoa(i+1))
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

func benchmarkNoMatchAliasPDF(pages, aliases int) *Document {
	pdf := MustNew()
	pdf.SetCompression(false)
	pdf.SetFont("Helvetica", "", 10)
	for i := 0; i < aliases; i++ {
		pdf.RegisterAlias(fmt.Sprintf("{mark %d}", i), strconv.Itoa(i+1))
	}
	for page := 0; page < pages; page++ {
		pdf.AddPage()
		for row := 0; row < 40; row++ {
			pdf.Cell(80, 4, "plain report row without page markers")
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
		pdf := MustNew()
		pdf.RegisterImageOptionsReader("alpha.png", options, bytes.NewReader(data))
		if !pdf.Ok() {
			b.Fatalf("RegisterImageOptionsReader() error = %v", pdf.Error())
		}
	}
}

func TestHTMLTablePrefixSpanWidthMatchesScan(t *testing.T) {
	widths := []float64{1.25, 2.5, 3.75, 4, 5.5}
	offsets := layout.TrackOffsets(widths)
	for start := 0; start <= len(widths)+1; start++ {
		for span := 0; span <= len(widths)+2; span++ {
			want := layout.SumSpan(widths, start, span)
			got := layout.SpanSize(offsets, start, span)
			if got != want {
				t.Fatalf("span width start=%d span=%d got %v, want %v", start, span, got, want)
			}
		}
	}
}

func BenchmarkHTMLTableSpanWidthWideRows(b *testing.B) {
	const (
		cols = 1024
		rows = 100
	)
	widths := make([]float64, cols)
	for i := range widths {
		widths[i] = 1 + float64(i%7)*0.25
	}
	offsets := layout.TrackOffsets(widths)

	b.Run("Scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			total := 0.0
			for row := 0; row < rows; row++ {
				for col := 0; col < cols; col++ {
					total += layout.SumSpan(widths, 0, col)
					total += layout.SumSpan(widths, col, 1)
				}
			}
			if total == 0 {
				b.Fatal("empty total")
			}
		}
	})

	b.Run("Prefix", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			total := 0.0
			for row := 0; row < rows; row++ {
				for col := 0; col < cols; col++ {
					total += layout.SpanSize(offsets, 0, col)
					total += layout.SpanSize(offsets, col, 1)
				}
			}
			if total == 0 {
				b.Fatal("empty total")
			}
		}
	})
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
	fontBytes, err := os.ReadFile("../assets/static/font/DejaVuSansCondensed.ttf")
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
		pdf := MustNew()
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		if !pdf.Ok() {
			b.Fatalf("AddUTF8FontFromCache() error = %v", pdf.Error())
		}
	}
}
