// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package gopdfsuitbench

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chinmay-sawant/gopdfsuit/v5/pkg/gopdflib"
	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/inspect"
)

const (
	workerCount40 = 40
	pageMarginPt  = 36.0
	rowHeightPt   = 18.0
	footerText    = "Shared PDF benchmark"
)

type workload struct {
	name        string
	title       string
	rows        []rowData
	pngData     []byte
	pngBase64   string
	pngWidthPt  float64
	pngHeightPt float64
}

type rowData struct {
	code        string
	description string
	value       string
}

type benchmarkEngine struct {
	name   string
	render func(workload, *pdfTarget) error
}

type pdfTarget struct {
	buf bytes.Buffer
}

func BenchmarkGoPDFKit(b *testing.B) {
	benchmarkEngineSuite(b, goPDFKitEngine())
}

func BenchmarkGoPDFLib(b *testing.B) {
	benchmarkEngineSuite(b, goPDFLibEngine())
}

func TestComparableOutputsArePDF(t *testing.T) {
	for _, wl := range workloads(t) {
		for _, engine := range engines() {
			var target pdfTarget
			if err := engine.render(wl, &target); err != nil {
				t.Fatalf("%s %s render error = %v", engine.name, wl.name, err)
			}
			if err := assertPDF(target.buf.Bytes()); err != nil {
				t.Fatalf("%s %s generated invalid PDF: %v", engine.name, wl.name, err)
			}
			if err := assertImportablePDF(target.buf.Bytes()); err != nil {
				t.Fatalf("%s %s generated non-importable PDF: %v", engine.name, wl.name, err)
			}
		}
	}
}

func benchmarkEngineSuite(b *testing.B, engine benchmarkEngine) {
	for _, wl := range workloads(b) {
		wl := wl
		for _, mode := range benchmarkModes() {
			mode := mode
			b.Run(fmt.Sprintf("%s/%s", wl.name, mode.name), func(b *testing.B) {
				benchmarkEngineWorkload(b, engine, wl, mode.workers)
			})
		}
	}
}

func benchmarkEngineWorkload(b *testing.B, engine benchmarkEngine, wl workload, workers int) {
	b.Helper()
	if workers < 1 {
		b.Fatalf("workers = %d, want >= 1", workers)
	}
	b.ReportAllocs()
	b.ReportMetric(float64(workers), "workers")

	var totalBytes atomic.Int64
	var firstErr error
	var errMu sync.Mutex
	setErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}
	runOne := func() {
		var target pdfTarget
		if err := engine.render(wl, &target); err != nil {
			setErr(err)
			return
		}
		if err := assertPDF(target.buf.Bytes()); err != nil {
			setErr(err)
			return
		}
		totalBytes.Add(int64(target.buf.Len()))
	}

	b.ResetTimer()
	start := time.Now()
	if workers == 1 {
		for i := 0; i < b.N; i++ {
			runOne()
		}
	} else {
		jobs := make(chan struct{}, workers)
		var wg sync.WaitGroup
		wg.Add(workers)
		for worker := 0; worker < workers; worker++ {
			go func() {
				defer wg.Done()
				for range jobs {
					runOne()
				}
			}()
		}
		for i := 0; i < b.N; i++ {
			jobs <- struct{}{}
		}
		close(jobs)
		wg.Wait()
	}
	elapsed := time.Since(start)
	b.StopTimer()

	if firstErr != nil {
		b.Fatal(firstErr)
	}
	if b.N > 0 {
		b.ReportMetric(float64(totalBytes.Load())/float64(b.N), "pdf_bytes")
		b.ReportMetric(float64(b.N)/elapsed.Seconds(), "pdf/s")
	}
}

func benchmarkModes() []struct {
	name    string
	workers int
} {
	return []struct {
		name    string
		workers int
	}{
		{name: "single", workers: 1},
		{name: "workers_40", workers: workerCount40},
	}
}

func workloads(tb testing.TB) []workload {
	tb.Helper()
	image := loadBenchmarkPNG(tb)
	return []workload{
		{name: "table_180_rows", title: "Shared table benchmark", rows: sharedRows(180)},
		{name: "table_900_rows", title: "Shared multipage table benchmark", rows: sharedRows(900)},
		{
			name:        "png_table_180_rows",
			title:       "Shared image and table benchmark",
			rows:        sharedRows(180),
			pngData:     image.data,
			pngBase64:   image.base64,
			pngWidthPt:  image.widthPt,
			pngHeightPt: image.heightPt,
		},
	}
}

func engines() []benchmarkEngine {
	return []benchmarkEngine{goPDFKitEngine(), goPDFLibEngine()}
}

func goPDFKitEngine() benchmarkEngine {
	return benchmarkEngine{name: "gopdfkit", render: renderGoPDFKit}
}

func goPDFLibEngine() benchmarkEngine {
	return benchmarkEngine{name: "gopdflib", render: renderGoPDFLib}
}

func renderGoPDFKit(wl workload, target *pdfTarget) error {
	pdf := document.New("P", "pt", "A4", "")
	pdf.SetCompression(true)
	pdf.SetMargins(pageMarginPt, pageMarginPt, pageMarginPt)
	pdf.SetAutoPageBreak(true, pageMarginPt)
	pdf.AliasNbPages("")
	pdf.SetFooterFunc(func() {
		pdf.SetY(-24)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 10, footerText, "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, fmt.Sprintf("Page %d/{nb}", pdf.PageNo()), "", 0, "R", false, 0, "")
	})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 22, wl.title, "", 1, "C", false, 0, "")

	if len(wl.pngData) > 0 {
		options := document.ImageOptions{ImageType: "png"}
		pdf.RegisterImageOptionsReader("shared-benchmark-png", options, bytes.NewReader(wl.pngData))
		pdf.ImageOptions("shared-benchmark-png", pageMarginPt, pdf.GetY()+8, wl.pngWidthPt, 0, false, options, 0, "")
		pdf.Ln(wl.pngHeightPt + 8)
	}

	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(90, rowHeightPt, "Code", "1", 0, "L", false, 0, "")
	pdf.CellFormat(330, rowHeightPt, "Description", "1", 0, "L", false, 0, "")
	pdf.CellFormat(103, rowHeightPt, "Value", "1", 1, "R", false, 0, "")
	pdf.SetFont("Helvetica", "", 9)
	for _, row := range wl.rows {
		pdf.CellFormat(90, rowHeightPt, row.code, "1", 0, "L", false, 0, "")
		pdf.CellFormat(330, rowHeightPt, row.description, "1", 0, "L", false, 0, "")
		pdf.CellFormat(103, rowHeightPt, row.value, "1", 1, "R", false, 0, "")
	}

	if err := pdf.Output(&target.buf); err != nil {
		return fmt.Errorf("gopdfkit output: %w", err)
	}
	return nil
}

func renderGoPDFLib(wl workload, target *pdfTarget) error {
	template := gopdfSuitTemplate(wl)
	pdfBytes, err := gopdflib.GeneratePDF(template)
	if err != nil {
		return fmt.Errorf("gopdflib output: %w", err)
	}
	_, err = target.buf.Write(pdfBytes)
	return err
}

func gopdfSuitTemplate(wl workload) gopdflib.PDFTemplate {
	embedFonts := false
	table := gopdflib.Table{
		MaxColumns:   3,
		ColumnWidths: []float64{90, 330, 103},
		Rows:         make([]gopdflib.Row, 0, len(wl.rows)+1),
	}
	table.Rows = append(table.Rows, gopdflib.Row{Row: []gopdflib.Cell{
		gopdfSuitCell("Helvetica:9:100:left:1:1:1:1", "Code"),
		gopdfSuitCell("Helvetica:9:100:left:1:1:1:1", "Description"),
		gopdfSuitCell("Helvetica:9:100:right:1:1:1:1", "Value"),
	}})
	for _, row := range wl.rows {
		table.Rows = append(table.Rows, gopdflib.Row{Row: []gopdflib.Cell{
			gopdfSuitCell("Helvetica:9:000:left:1:1:1:1", row.code),
			gopdfSuitCell("Helvetica:9:000:left:1:1:1:1", row.description),
			gopdfSuitCell("Helvetica:9:000:right:1:1:1:1", row.value),
		}})
	}

	template := gopdflib.PDFTemplate{
		Config: gopdflib.Config{
			Page:          "A4",
			PageAlignment: 1,
			PageMargin:    "36:36:36:36",
			PdfTitle:      wl.title,
			EmbedFonts:    &embedFonts,
		},
		Title: gopdflib.Title{
			Props: "Helvetica:16:100:center:0:0:0:0",
			Text:  wl.title,
		},
		Footer: gopdflib.Footer{
			Font: "Helvetica:8:000:left",
			Text: footerText,
		},
		Elements: []gopdflib.Element{{Type: "table", Table: &table}},
	}
	if wl.pngBase64 != "" {
		template.Elements = []gopdflib.Element{
			{
				Type: "image",
				Image: &gopdflib.Image{
					ImageName: "shared-benchmark-png",
					ImageData: wl.pngBase64,
					Width:     wl.pngWidthPt,
					Height:    wl.pngHeightPt,
				},
			},
			{Type: "table", Table: &table},
		}
	}
	return template
}

func gopdfSuitCell(props, text string) gopdflib.Cell {
	height := rowHeightPt
	return gopdflib.Cell{Props: props, Text: text, Height: &height}
}

func sharedRows(count int) []rowData {
	rows := make([]rowData, 0, count)
	for row := 0; row < count; row++ {
		rows = append(rows, rowData{
			code:        fmt.Sprintf("ROW-%03d", row),
			description: "Operational PDF generation benchmark row",
			value:       fmt.Sprintf("%0.2f", float64(row)*1.25),
		})
	}
	return rows
}

type benchmarkImage struct {
	data     []byte
	base64   string
	widthPt  float64
	heightPt float64
}

func loadBenchmarkPNG(tb testing.TB) benchmarkImage {
	tb.Helper()
	data, err := os.ReadFile("../../assets/static/image/logo.png")
	if err != nil {
		tb.Fatalf("read benchmark PNG: %v", err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		tb.Fatalf("decode benchmark PNG config: %v", err)
	}
	width := 523.0
	return benchmarkImage{
		data:     data,
		base64:   base64.StdEncoding.EncodeToString(data),
		widthPt:  width,
		heightPt: width * float64(cfg.Height) / float64(cfg.Width),
	}
}

func assertPDF(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("generated empty PDF")
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return fmt.Errorf("missing PDF header")
	}
	if !bytes.Contains(data, []byte("%%EOF")) {
		return fmt.Errorf("missing PDF EOF marker")
	}
	return nil
}

func assertImportablePDF(data []byte) error {
	if _, err := inspect.PageCount(data); err != nil {
		return fmt.Errorf("page count: %w", err)
	}
	return nil
}

func BenchmarkGoPDFKitHTMLSubset(b *testing.B) {
	if os.Getenv("GOPDFKIT_COMPARE_HTML") == "" {
		b.Skip("set GOPDFKIT_COMPARE_HTML=1 to run non-equivalent HTML comparison")
	}
	benchmarkOptionalHTML(b, "gopdfkit", renderGoPDFKitHTML)
}

func BenchmarkGoPDFLibHTMLChrome(b *testing.B) {
	if os.Getenv("GOPDFKIT_COMPARE_HTML") == "" {
		b.Skip("set GOPDFKIT_COMPARE_HTML=1 to run Chrome-backed HTML comparison")
	}
	benchmarkOptionalHTML(b, "gopdflib", renderGoPDFLibHTML)
}

func benchmarkOptionalHTML(b *testing.B, engine string, render func(*pdfTarget) error) {
	b.ReportAllocs()
	b.ReportMetric(1, "workers")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var target pdfTarget
		if err := render(&target); err != nil {
			b.Fatalf("%s HTML render error = %v", engine, err)
		}
		if err := assertPDF(target.buf.Bytes()); err != nil {
			b.Fatalf("%s HTML generated invalid PDF: %v", engine, err)
		}
	}
}

func renderGoPDFKitHTML(target *pdfTarget) error {
	pdf := document.New("P", "pt", "A4", "")
	pdf.SetCompression(true)
	pdf.SetMargins(pageMarginPt, pageMarginPt, pageMarginPt)
	pdf.SetAutoPageBreak(true, pageMarginPt)
	pdf.AddPage()
	html := pdf.HTMLNew()
	html.Write(12, benchmarkHTML())
	if err := pdf.Output(&target.buf); err != nil {
		return fmt.Errorf("gopdfkit HTML output: %w", err)
	}
	return nil
}

func renderGoPDFLibHTML(target *pdfTarget) error {
	pdfBytes, err := gopdflib.ConvertHTMLToPDF(gopdflib.HTMLToPDFRequest{
		HTML:         "<!doctype html><html><body>" + benchmarkHTML() + "</body></html>",
		PageSize:     "A4",
		Orientation:  "Portrait",
		MarginTop:    "12.7mm",
		MarginRight:  "12.7mm",
		MarginBottom: "12.7mm",
		MarginLeft:   "12.7mm",
	})
	if err != nil {
		return fmt.Errorf("gopdflib HTML output: %w", err)
	}
	_, err = target.buf.Write(pdfBytes)
	return err
}

func benchmarkHTML() string {
	var out bytes.Buffer
	out.WriteString(`<h1>Comparable HTML</h1><p>This fragment uses headings, paragraphs, lists, and a simple table.</p><ul>`)
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&out, `<li>Item %02d</li>`, i+1)
	}
	out.WriteString(`</ul><table border="1"><tr><th>Code</th><th>Status</th></tr>`)
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&out, `<tr><td>HTML-%02d</td><td>Ready</td></tr>`, i+1)
	}
	out.WriteString(`</table>`)
	return out.String()
}
