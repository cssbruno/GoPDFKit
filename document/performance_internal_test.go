// SPDX-License-Identifier: MIT
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
	pdf := New("P", "mm", "A4", "")
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
	pdf := New("P", "mm", "A4", "")
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
		pdf := New("P", "mm", "A4", "")
		pdf.RegisterImageOptionsReader("alpha.png", options, bytes.NewReader(data))
		if !pdf.Ok() {
			b.Fatalf("RegisterImageOptionsReader() error = %v", pdf.Error())
		}
	}
}

func TestHTMLTablePrefixSpanWidthMatchesScan(t *testing.T) {
	widths := []float64{1.25, 2.5, 3.75, 4, 5.5}
	offsets := htmlTableSpanPrefix(widths)
	for start := 0; start <= len(widths)+1; start++ {
		for span := 0; span <= len(widths)+2; span++ {
			want := htmlTableSpanWidth(widths, start, span)
			got := htmlTablePrefixSpanWidth(offsets, start, span)
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
	offsets := htmlTableSpanPrefix(widths)

	b.Run("Scan", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			total := 0.0
			for row := 0; row < rows; row++ {
				for col := 0; col < cols; col++ {
					total += htmlTableSpanWidth(widths, 0, col)
					total += htmlTableSpanWidth(widths, col, 1)
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
					total += htmlTablePrefixSpanWidth(offsets, 0, col)
					total += htmlTablePrefixSpanWidth(offsets, col, 1)
				}
			}
			if total == 0 {
				b.Fatal("empty total")
			}
		}
	})
}

func BenchmarkHTMLTableColumnWidthsRepeatedText(b *testing.B) {
	const (
		rows = 400
		cols = 8
	)
	values := []string{
		"ready",
		"pending approval",
		"blocked by upstream dependency",
		"generated benchmark row",
		"operational status",
		"category label",
	}
	tableRows := make([]htmlTableRow, rows)
	for row := range tableRows {
		cells := make([]htmlTableCell, cols)
		for col := range cells {
			text := values[(row+col)%len(values)]
			cells[col] = htmlTableCell{
				attrs: map[string]string{},
				text:  text,
				tag:   "td",
			}
		}
		tableRows[row] = htmlTableRow{cells: cells}
	}
	layoutRows := htmlTableLayoutRows(tableRows)
	pdf := New("P", "mm", "A4", "")
	pdf.SetFont("Helvetica", "", 9)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		widths := htmlTableColumnWidths(layoutRows, cols, 180, pdf)
		if len(widths) != cols {
			b.Fatalf("column width count = %d, want %d", len(widths), cols)
		}
	}
}

func BenchmarkHTMLBlockHasBoxStyleSelectorHeavy(b *testing.B) {
	var css strings.Builder
	for i := 0; i < 64; i++ {
		fmt.Fprintf(&css, `.report .group%d > p.item%d { color: #%02x%02x%02x; }`, i%8, i, 20+i%100, 80+i%80, 140+i%60)
	}
	css.WriteString(`.report .group3 > p.item27 { background-color: #eeeeee; }`)
	rules := parseHTMLCSSRules(css.String())
	ancestors := []HTMLSegmentType{
		{Cat: 'O', Str: "section", Attr: map[string]string{"class": "report"}},
		{Cat: 'O', Str: "div", Attr: map[string]string{"class": "group3"}},
	}
	el := HTMLSegmentType{Cat: 'O', Str: "p", Attr: map[string]string{"class": "item27"}}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !htmlBlockHasBoxStyle(el, rules, ancestors...) {
			b.Fatal("expected block box style")
		}
	}
}

func BenchmarkGenerationHTMLBlockBoxesCompiled(b *testing.B) {
	var body strings.Builder
	body.WriteString(`<style>.box{background-color:#eeeeee;padding:3px;margin:0 0 2px 0}.box strong{font-weight:bold}</style>`)
	for i := 0; i < 120; i++ {
		fmt.Fprintf(&body, `<div class="box"><strong>Block %03d</strong> repeated compiled block text for report rendering.</div>`, i)
	}
	compiled, err := CompileHTML(body.String())
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		pdf := New("P", "mm", "A4", "")
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 9)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)

		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			b.Fatalf("Output() error = %v", err)
		}
		if output.Len() == 0 {
			b.Fatal("generated empty PDF")
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
		pdf := New("P", "mm", "A4", "")
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		if !pdf.Ok() {
			b.Fatalf("AddUTF8FontFromCache() error = %v", pdf.Error())
		}
	}
}
