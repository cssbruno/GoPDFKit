// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"compress/zlib"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"fmt"
	"math/big"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/testsupport/example"
	"github.com/cssbruno/gopdfkit/sign"
)

type benchmarkPDFOutput func(*document.Document, *bytes.Buffer) error

func benchmarkGeneratedPDF(b *testing.B, build func(*document.Document)) {
	b.Helper()
	benchmarkGeneratedPDFOutput(b, build, func(pdf *document.Document, output *bytes.Buffer) error {
		return pdf.Output(output)
	})
}

func benchmarkGeneratedSignedPDF(b *testing.B, build func(*document.Document), options sign.Options) {
	b.Helper()
	benchmarkGeneratedPDFOutput(b, build, func(pdf *document.Document, output *bytes.Buffer) error {
		return pdf.OutputSigned(output, options)
	})
}

func benchmarkGeneratedPDFOutput(b *testing.B, build func(*document.Document), outputPDF benchmarkPDFOutput) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		pdf := document.New("P", "mm", "A4", "")
		pdf.SetCompression(false)
		build(pdf)

		var output bytes.Buffer
		if err := outputPDF(pdf, &output); err != nil {
			b.Fatalf("output PDF error = %v", err)
		}
		if output.Len() == 0 {
			b.Fatal("generated empty PDF")
		}
	}
}

func benchmarkGeneratedPDFConcurrent(b *testing.B, workers int, build func(*document.Document)) {
	b.Helper()
	benchmarkGeneratedPDFOutputConcurrent(b, workers, build, func(pdf *document.Document, output *bytes.Buffer) error {
		return pdf.Output(output)
	})
}

func benchmarkGeneratedPDFConcurrentCPU(b *testing.B, build func(*document.Document)) {
	b.Helper()
	benchmarkGeneratedPDFConcurrent(b, benchmarkCPUWorkers(), build)
}

func benchmarkGeneratedSignedPDFConcurrent(b *testing.B, workers int, build func(*document.Document), options sign.Options) {
	b.Helper()
	benchmarkGeneratedPDFOutputConcurrent(b, workers, build, func(pdf *document.Document, output *bytes.Buffer) error {
		return pdf.OutputSigned(output, options)
	})
}

func benchmarkGeneratedSignedPDFConcurrentCPU(b *testing.B, build func(*document.Document), options sign.Options) {
	b.Helper()
	benchmarkGeneratedSignedPDFConcurrent(b, benchmarkCPUWorkers(), build, options)
}

func benchmarkCPUWorkers() int {
	if workers := runtime.GOMAXPROCS(0); workers > 0 {
		return workers
	}
	return 1
}

func benchmarkGeneratedPDFOutputConcurrent(b *testing.B, workers int, build func(*document.Document), outputPDF benchmarkPDFOutput) {
	b.Helper()
	b.ReportAllocs()
	if workers < 1 {
		b.Fatalf("workers = %d, want >= 1", workers)
	}
	b.ReportMetric(float64(workers), "workers")

	jobs := make(chan struct{}, workers)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
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

	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for range jobs {
				pdf := document.New("P", "mm", "A4", "")
				pdf.SetCompression(false)
				build(pdf)

				var output bytes.Buffer
				if err := outputPDF(pdf, &output); err != nil {
					setErr(fmt.Errorf("output PDF error = %w", err))
					continue
				}
				if output.Len() == 0 {
					setErr(fmt.Errorf("generated empty PDF"))
				}
			}
		}()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()
	b.StopTimer()

	if firstErr != nil {
		b.Fatal(firstErr)
	}
}

func benchmarkSignOptions(b *testing.B) sign.Options {
	b.Helper()
	cert, signer := benchmarkTestSigner(b)
	return sign.Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Name:            "GoPDFKit benchmark signer",
		Reason:          "PDF generation benchmark",
		SigningTime:     time.Unix(1_704_067_200, 0).UTC(),
	}
}

func benchmarkTestSigner(tb testing.TB) (*x509.Certificate, crypto.Signer) {
	tb.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		tb.Fatalf("GenerateKey() error = %v", err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GoPDFKit benchmark signer"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 36}},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		tb.Fatalf("CreateCertificate() error = %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		tb.Fatalf("ParseCertificate() error = %v", err)
	}
	return cert, key
}

func BenchmarkGenerationText(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for row := 0; row < 180; row++ {
			pdf.CellFormat(40, 6, fmt.Sprintf("Row %03d", row), "1", 0, "L", false, 0, "")
			pdf.CellFormat(80, 6, "Operational PDF generation benchmark", "1", 0, "L", false, 0, "")
			pdf.CellFormat(40, 6, fmt.Sprintf("%0.2f", float64(row)*1.25), "1", 1, "R", false, 0, "")
		}
	})
}

func BenchmarkGenerationLongText(b *testing.B) {
	text, err := os.ReadFile(example.TextFile("20k_c1.txt"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Times", "", 11)
		pdf.MultiCell(0, 5, string(text), "", "J", false)
	})
}

func BenchmarkGenerationHTMLText(b *testing.B) {
	htmlStr := `<style>
		p { margin: 0 0 4px 0; line-height: 1.25; }
		.note { color: #123456; font-weight: bold; }
	</style>`
	var htmlBuilder strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&htmlBuilder, `<p><strong>Section %03d</strong> Operational HTML paragraph with `+
			`<em>inline emphasis</em>, <u>underlined text</u>, and `+
			`<span class="note">styled text</span>.</p>`, i)
	}
	htmlStr += htmlBuilder.String()

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, htmlStr)
	})
}

func BenchmarkGenerationHTMLTextCompiled(b *testing.B) {
	htmlStr := `<style>
		p { margin: 0 0 4px 0; line-height: 1.25; }
		.note { color: #123456; font-weight: bold; }
	</style>`
	var htmlBuilder strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&htmlBuilder, `<p><strong>Section %03d</strong> Operational HTML paragraph with `+
			`<em>inline emphasis</em>, <u>underlined text</u>, and `+
			`<span class="note">styled text</span>.</p>`, i)
	}
	htmlStr += htmlBuilder.String()
	compiled, err := document.CompileHTML(htmlStr)
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
	})
}

func BenchmarkGenerationHTMLTable(b *testing.B) {
	var rows strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&rows, `<tr><td>Item %03d</td><td>Generated benchmark row</td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*1.25)
	}
	htmlStr := `<style>
		th { background-color: #eeeeee; font-weight: bold; }
		td, th { padding: 3px; border: 1px solid #555555; }
	</style>` +
		`<table border="1" cellpadding="3" width="100%">` +
		`<thead><tr><th width="22%">Code</th><th>Description</th><th width="18%">Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>`

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 9)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, htmlStr)
	})
}

func BenchmarkGenerationHTMLTableCompiled(b *testing.B) {
	var rows strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&rows, `<tr><td>Item %03d</td><td>Generated benchmark row</td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*1.25)
	}
	htmlStr := `<style>
		th { background-color: #eeeeee; font-weight: bold; }
		td, th { padding: 3px; border: 1px solid #555555; }
	</style>` +
		`<table border="1" cellpadding="3" width="100%">` +
		`<thead><tr><th width="22%">Code</th><th>Description</th><th width="18%">Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>`
	compiled, err := document.CompileHTML(htmlStr)
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 9)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
	})
}

func BenchmarkGenerationHTMLDataPNG(b *testing.B) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	var body strings.Builder
	for i := 0; i < 24; i++ {
		fmt.Fprintf(&body, `<p>PNG block %02d</p><img src="%s" width="24" height="24"/>`, i, pngDataURI)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, body.String())
	})
}

func BenchmarkGenerationHTMLDataPNGCompiled(b *testing.B) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	var body strings.Builder
	for i := 0; i < 24; i++ {
		fmt.Fprintf(&body, `<p>PNG block %02d</p><img src="%s" width="24" height="24"/>`, i, pngDataURI)
	}
	compiled, err := document.CompileHTML(body.String())
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
	})
}

func BenchmarkGenerationHTMLInlineSVG(b *testing.B) {
	const svgFragment = `<svg width="80" height="36" viewBox="0 0 80 36">` +
		`<rect x="1" y="1" width="78" height="34" fill="#e8f4ff" stroke="#123456"/>` +
		`<circle cx="18" cy="18" r="8" fill="#2a8fbd"/>` +
		`<path d="M34 24 L46 10 L58 24 Z" fill="#50a060" stroke="#000000"/>` +
		`<text x="70" y="22" text-anchor="middle" font-size="8">SVG</text>` +
		`</svg>`
	var body strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&body, `<p>SVG block %02d</p>%s`, i, svgFragment)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, body.String())
	})
}

func BenchmarkGenerationHTMLInlineSVGCompiled(b *testing.B) {
	const svgFragment = `<svg width="80" height="36" viewBox="0 0 80 36">` +
		`<rect x="1" y="1" width="78" height="34" fill="#e8f4ff" stroke="#123456"/>` +
		`<circle cx="18" cy="18" r="8" fill="#2a8fbd"/>` +
		`<path d="M34 24 L46 10 L58 24 Z" fill="#50a060" stroke="#000000"/>` +
		`<text x="70" y="22" text-anchor="middle" font-size="8">SVG</text>` +
		`</svg>`
	var body strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&body, `<p>SVG block %02d</p>%s`, i, svgFragment)
	}
	compiled, err := document.CompileHTML(body.String())
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
	})
}

func BenchmarkGenerationHTMLMixedDocument(b *testing.B) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	const svgFragment = `<svg width="96" height="32" viewBox="0 0 96 32">` +
		`<rect x="1" y="1" width="94" height="30" fill="#f3f3f3" stroke="#333333"/>` +
		`<path d="M8 24 C20 4 32 4 44 24 S68 44 88 8" fill="none" stroke="#006699"/>` +
		`<text x="48" y="20" text-anchor="middle" font-size="9">mixed</text>` +
		`</svg>`
	var rows strings.Builder
	for i := 0; i < 36; i++ {
		fmt.Fprintf(&rows, `<tr><td>Line %02d</td><td>Nested content and table text</td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*3.75)
	}
	htmlStr := `<style>
		h1 { font-size: 18pt; color: #123456; }
		.summary { background-color: #eeeeee; border: 1px solid #999999; padding: 5px; }
		td, th { padding: 3px; border: 1px solid #666666; }
		th { background-color: #dddddd; }
	</style>` +
		`<h1>Mixed HTML PDF Benchmark</h1>` +
		`<div class="summary"><p><strong>Summary</strong> with paragraph text, inline styles, and image output.</p></div>` +
		`<ul><li>First list item</li><li>Second list item</li><li>Third list item</li></ul>` +
		`<img src="` + pngDataURI + `" width="32" height="32"/>` +
		svgFragment +
		`<table border="1" cellpadding="3" width="100%">` +
		`<thead><tr><th>Code</th><th>Description</th><th>Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>` +
		`<p style="break-before: page">Second page paragraph with <em>emphasis</em> and <u>decoration</u>.</p>` +
		svgFragment

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.Write(lineHeight, htmlStr)
	})
}

func BenchmarkGenerationHTMLMixedDocumentCompiled(b *testing.B) {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	const svgFragment = `<svg width="96" height="32" viewBox="0 0 96 32">` +
		`<rect x="1" y="1" width="94" height="30" fill="#f3f3f3" stroke="#333333"/>` +
		`<path d="M8 24 C20 4 32 4 44 24 S68 44 88 8" fill="none" stroke="#006699"/>` +
		`<text x="48" y="20" text-anchor="middle" font-size="9">mixed</text>` +
		`</svg>`
	var rows strings.Builder
	for i := 0; i < 36; i++ {
		fmt.Fprintf(&rows, `<tr><td>Line %02d</td><td>Nested content and table text</td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*3.75)
	}
	htmlStr := `<style>
		h1 { font-size: 18pt; color: #123456; }
		.summary { background-color: #eeeeee; border: 1px solid #999999; padding: 5px; }
		td, th { padding: 3px; border: 1px solid #666666; }
		th { background-color: #dddddd; }
	</style>` +
		`<h1>Mixed HTML PDF Benchmark</h1>` +
		`<div class="summary"><p><strong>Summary</strong> with paragraph text, inline styles, and image output.</p></div>` +
		`<ul><li>First list item</li><li>Second list item</li><li>Third list item</li></ul>` +
		`<img src="` + pngDataURI + `" width="32" height="32"/>` +
		svgFragment +
		`<table border="1" cellpadding="3" width="100%">` +
		`<thead><tr><th>Code</th><th>Description</th><th>Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>` +
		`<p style="break-before: page">Second page paragraph with <em>emphasis</em> and <u>decoration</u>.</p>` +
		svgFragment
	compiled, err := document.CompileHTML(htmlStr)
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 10)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
	})
}

func BenchmarkGenerationBaselineNoCompliance(b *testing.B) {
	benchmarkGenerationBaselineNoCompliance(b, nil)
}

func BenchmarkGenerationBaselineNoComplianceConcurrentCPU(b *testing.B) {
	benchmarkGeneratedPDFConcurrentCPU(b, benchmarkGenerationBaselineNoComplianceBuilder(b, nil))
}

func BenchmarkGenerationBaselineNoComplianceCachedImage(b *testing.B) {
	cache := document.NewImageCache()
	if _, err := cache.RegisterImageOptions("logo.png", example.ImageFile("logo.png"), document.ImageOptions{}); err != nil {
		b.Fatalf("RegisterImageOptions(logo.png) error = %v", err)
	}
	benchmarkGenerationBaselineNoCompliance(b, cache)
}

func BenchmarkGenerationBaselineNoComplianceCachedImageConcurrentCPU(b *testing.B) {
	cache := document.NewImageCache()
	if _, err := cache.RegisterImageOptions("logo.png", example.ImageFile("logo.png"), document.ImageOptions{}); err != nil {
		b.Fatalf("RegisterImageOptions(logo.png) error = %v", err)
	}
	benchmarkGeneratedPDFConcurrentCPU(b, benchmarkGenerationBaselineNoComplianceBuilder(b, cache))
}

func BenchmarkGenerationBaselineNoComplianceSigned(b *testing.B) {
	benchmarkGeneratedSignedPDF(b, benchmarkGenerationBaselineNoComplianceBuilder(b, nil), benchmarkSignOptions(b))
}

func BenchmarkGenerationBaselineNoComplianceSignedConcurrentCPU(b *testing.B) {
	benchmarkGeneratedSignedPDFConcurrentCPU(b, benchmarkGenerationBaselineNoComplianceBuilder(b, nil), benchmarkSignOptions(b))
}

func BenchmarkGenerationBaselineNoComplianceCachedImageSigned(b *testing.B) {
	cache := document.NewImageCache()
	if _, err := cache.RegisterImageOptions("logo.png", example.ImageFile("logo.png"), document.ImageOptions{}); err != nil {
		b.Fatalf("RegisterImageOptions(logo.png) error = %v", err)
	}
	benchmarkGeneratedSignedPDF(b, benchmarkGenerationBaselineNoComplianceBuilder(b, cache), benchmarkSignOptions(b))
}

func BenchmarkGenerationBaselineNoComplianceCachedImageSignedConcurrentCPU(b *testing.B) {
	cache := document.NewImageCache()
	if _, err := cache.RegisterImageOptions("logo.png", example.ImageFile("logo.png"), document.ImageOptions{}); err != nil {
		b.Fatalf("RegisterImageOptions(logo.png) error = %v", err)
	}
	benchmarkGeneratedSignedPDFConcurrentCPU(b, benchmarkGenerationBaselineNoComplianceBuilder(b, cache), benchmarkSignOptions(b))
}

func BenchmarkGenerationPDFA4FCompliance(b *testing.B) {
	benchmarkGeneratedPDF(b, benchmarkGenerationPDFA4FComplianceBuilder(b))
}

func BenchmarkGenerationPDFA4FComplianceConcurrentCPU(b *testing.B) {
	benchmarkGeneratedPDFConcurrentCPU(b, benchmarkGenerationPDFA4FComplianceBuilder(b))
}

func BenchmarkGenerationPDFUA2ArlingtonCompiledHTML(b *testing.B) {
	benchmarkGeneratedPDF(b, benchmarkGenerationPDFUA2ArlingtonCompiledHTMLBuilder(b))
}

func BenchmarkGenerationPDFUA2ArlingtonCompiledHTMLConcurrentCPU(b *testing.B) {
	benchmarkGeneratedPDFConcurrentCPU(b, benchmarkGenerationPDFUA2ArlingtonCompiledHTMLBuilder(b))
}

func BenchmarkGenerationHTMLSelectorHeavyCompiled(b *testing.B) {
	benchmarkGeneratedPDF(b, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkSelectorHeavyHTML()))
}

func BenchmarkGenerationHTMLSelectorHeavyCompiledConcurrent40(b *testing.B) {
	benchmarkGeneratedPDFConcurrent(b, 40, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkSelectorHeavyHTML()))
}

func BenchmarkGenerationHTMLTableHeavyCompiled(b *testing.B) {
	benchmarkGeneratedPDF(b, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkTableHeavyHTML()))
}

func BenchmarkGenerationHTMLTableHeavyCompiledConcurrent40(b *testing.B) {
	benchmarkGeneratedPDFConcurrent(b, 40, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkTableHeavyHTML()))
}

func BenchmarkGenerationHTMLDataImageHeavyCompiled(b *testing.B) {
	benchmarkGeneratedPDF(b, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkDataImageHeavyHTML()))
}

func BenchmarkGenerationHTMLDataImageHeavyCompiledConcurrent40(b *testing.B) {
	benchmarkGeneratedPDFConcurrent(b, 40, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkDataImageHeavyHTML()))
}

func BenchmarkGenerationHTMLMalformedCompiled(b *testing.B) {
	benchmarkGeneratedPDF(b, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkMalformedHTML()))
}

func BenchmarkGenerationHTMLMalformedCompiledConcurrent40(b *testing.B) {
	benchmarkGeneratedPDFConcurrent(b, 40, benchmarkGenerationCompiledHTMLBuilder(b, benchmarkMalformedHTML()))
}

func BenchmarkGenerationSignedPDFA4FPDFUA2ArlingtonXMP(b *testing.B) {
	benchmarkGeneratedSignedPDF(b, benchmarkGenerationPDFA4FPDFUA2ArlingtonXMPBuilder(b), benchmarkSignOptions(b))
}

func BenchmarkGenerationSignedPDFA4FPDFUA2ArlingtonXMPConcurrentCPU(b *testing.B) {
	benchmarkGeneratedSignedPDFConcurrentCPU(b, benchmarkGenerationPDFA4FPDFUA2ArlingtonXMPBuilder(b), benchmarkSignOptions(b))
}

func benchmarkGenerationBaselineNoCompliance(b *testing.B, imageCache *document.ImageCache) {
	benchmarkGeneratedPDF(b, benchmarkGenerationBaselineNoComplianceBuilder(b, imageCache))
}

func benchmarkGenerationBaselineNoComplianceBuilder(b *testing.B, imageCache *document.ImageCache) func(*document.Document) {
	const svgFragment = `<svg width="128" height="40" viewBox="0 0 128 40">` +
		`<rect x="1" y="1" width="126" height="38" fill="#f6f8fa" stroke="#40516b"/>` +
		`<path d="M10 28 L28 12 L46 24 L64 10 L82 22 L100 14 L118 30" fill="none" stroke="#18715f" stroke-width="3"/>` +
		`<text x="64" y="25" text-anchor="middle" font-size="10">baseline</text>` +
		`</svg>`

	svg, err := document.SVGParse([]byte(svgFragment))
	if err != nil {
		b.Fatalf("SVGParse() error = %v", err)
	}
	grid, err := os.ReadFile("grid.go")
	if err != nil {
		b.Fatalf("ReadFile(grid.go) error = %v", err)
	}

	return func(pdf *document.Document) {
		pdf.SetTitle("Baseline no-compliance benchmark", false)
		pdf.SetSubject("PDF generation without PDF/A, PDF/UA, Arlington, or XMP metadata", false)
		pdf.SetAuthor("GoPDFKit benchmark", false)
		pdf.SetCreator("GoPDFKit benchmark", false)
		pdf.SetKeywords("benchmark baseline no-compliance", false)
		pdf.SetAttachments([]document.Attachment{{
			Content:     grid[:min(len(grid), 4096)],
			Filename:    "grid.go",
			Description: "Small benchmark attachment",
		}})

		template := pdf.CreateTemplate(func(tpl *document.Tpl) {
			tpl.SetFont("Helvetica", "B", 12)
			tpl.SetFillColor(235, 239, 244)
			tpl.Rect(0, 0, 180, 16, "F")
			tpl.Text(6, 10, "GoPDFKit baseline")
		})

		for page := 0; page < 3; page++ {
			pdf.AddPage()
			pdf.UseTemplate(template)
			pdf.SetXY(12, 24)
			pdf.SetFont("Helvetica", "B", 13)
			pdf.CellFormat(0, 8, fmt.Sprintf("Operational report page %d", page+1), "", 1, "L", false, 0, "")
			pdf.SetFont("Helvetica", "", 9)
			for row := 0; row < 48; row++ {
				pdf.CellFormat(26, 5, fmt.Sprintf("Item %03d", page*48+row), "1", 0, "L", false, 0, "")
				pdf.CellFormat(88, 5, "Baseline PDF output without standards validation layers", "1", 0, "L", false, 0, "")
				pdf.CellFormat(24, 5, fmt.Sprintf("%0.2f", float64(row+1)*2.35), "1", 0, "R", false, 0, "")
				pdf.CellFormat(28, 5, "ready", "1", 1, "C", false, 0, "")
			}
			pdf.SetXY(14, 270)
			if imageCache == nil {
				pdf.ImageOptions(example.ImageFile("logo.png"), 14, 270, 10, 0, false, document.ImageOptions{}, 0, "")
			} else {
				pdf.ImageFromCache("logo.png", imageCache, 14, 270, 10, 0, false, document.ImageOptions{}, 0, "")
			}
			pdf.SetXY(32, 270)
			pdf.SVGWrite(&svg, 0.45)
		}
	}
}

func benchmarkGenerationPDFA4FComplianceBuilder(b *testing.B) func(*document.Document) {
	cache := benchmarkComplianceFontCache(b)
	icc := []byte("benchmark sRGB ICC placeholder for generation-only benchmark")

	return func(pdf *document.Document) {
		pdf.SetComplianceMetadata(document.ComplianceMetadata{
			PDFA:       document.PDFAMode4F,
			Lang:       "en-US",
			Title:      "PDF/A-4f compliance benchmark",
			Identifier: "urn:uuid:gopdfkit-benchmark-pdfa4f",
		})
		if err := pdf.SetOutputIntent(icc, "sRGB IEC61966-2.1"); err != nil {
			b.Fatalf("SetOutputIntent() error = %v", err)
		}
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "B", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "I", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "BI", cache)
		pdf.SetAttachments([]document.Attachment{{
			Filename:       "benchmark.txt",
			Description:    "PDF/A-4f benchmark attachment",
			MIMEType:       "text/plain",
			AFRelationship: "Data",
			Content:        []byte("Small benchmark attachment for PDF/A-4f generation."),
		}})
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 10)
		for row := 0; row < 96; row++ {
			pdf.CellFormat(38, 5, fmt.Sprintf("Row %03d", row), "1", 0, "L", false, 0, "")
			pdf.CellFormat(104, 5, "PDF/A-4f metadata, output intent, UTF-8 font, and attachment", "1", 0, "L", false, 0, "")
			pdf.CellFormat(24, 5, fmt.Sprintf("%0.2f", float64(row)*1.7), "1", 1, "R", false, 0, "")
		}
	}
}

func benchmarkGenerationPDFUA2ArlingtonCompiledHTMLBuilder(b *testing.B) func(*document.Document) {
	cache := benchmarkComplianceFontCache(b)
	compiled, err := document.CompileHTML(benchmarkTaggedHTML())
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}

	return func(pdf *document.Document) {
		pdf.SetTitle("PDF/UA-2 Arlington compiled HTML benchmark", false)
		pdf.SetComplianceMetadata(document.ComplianceMetadata{
			PDFUA2:     true,
			Arlington:  true,
			Lang:       "en-US",
			Identifier: "urn:uuid:gopdfkit-benchmark-pdfua2-arlington",
		})
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "B", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "I", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "BI", cache)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 10)
		pdf.SetNextTextRole("H1")
		pdf.CellFormat(0, 7, "PDF/UA-2 tagged HTML benchmark", "", 1, "L", false, 0, "")
		html := pdf.HTMLNew()
		html.WriteCompiled(5, compiled)
	}
}

func benchmarkGenerationCompiledHTMLBuilder(b *testing.B, htmlStr string) func(*document.Document) {
	compiled, err := document.CompileHTML(htmlStr)
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}
	return func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 9)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
	}
}

func benchmarkGenerationPDFA4FPDFUA2ArlingtonXMPBuilder(b *testing.B) func(*document.Document) {
	cache := benchmarkComplianceFontCache(b)
	compiled, err := document.CompileHTML(benchmarkTaggedHTML())
	if err != nil {
		b.Fatalf("CompileHTML() error = %v", err)
	}
	icc := []byte("benchmark sRGB ICC placeholder for generation-only benchmark")

	return func(pdf *document.Document) {
		pdf.SetTitle("Signed PDF/A-4f PDF/UA-2 Arlington XMP benchmark", false)
		pdf.SetComplianceMetadata(document.ComplianceMetadata{
			PDFA:       document.PDFAMode4F,
			PDFUA2:     true,
			Arlington:  true,
			Lang:       "en-US",
			Title:      "Signed PDF/A-4f PDF/UA-2 Arlington XMP benchmark",
			Identifier: "urn:uuid:gopdfkit-benchmark-signed-pdfa4f-pdfua2-arlington-xmp",
		})
		if err := pdf.SetOutputIntent(icc, "sRGB IEC61966-2.1"); err != nil {
			b.Fatalf("SetOutputIntent() error = %v", err)
		}
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "B", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "I", cache)
		pdf.AddUTF8FontFromCache("DejaVu", "BI", cache)
		pdf.SetAttachments([]document.Attachment{{
			Filename:       "compliance-benchmark.txt",
			Description:    "Signed PDF/A-4f PDF/UA-2 Arlington benchmark attachment",
			MIMEType:       "text/plain",
			AFRelationship: "Data",
			Content:        []byte("Signed compliance benchmark attachment."),
		}})
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 10)
		pdf.SetNextTextRole("H1")
		pdf.CellFormat(0, 7, "Signed compliance benchmark", "", 1, "L", false, 0, "")
		html := pdf.HTMLNew()
		html.WriteCompiled(5, compiled)
	}
}

func benchmarkComplianceFontCache(b *testing.B) *document.FontCache {
	b.Helper()
	fontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed.ttf"))
	if err != nil {
		b.Fatalf("ReadFile(font) error = %v", err)
	}
	boldFontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed-Bold.ttf"))
	if err != nil {
		b.Fatalf("ReadFile(bold font) error = %v", err)
	}
	italicFontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed-Oblique.ttf"))
	if err != nil {
		b.Fatalf("ReadFile(italic font) error = %v", err)
	}
	boldItalicFontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed-BoldOblique.ttf"))
	if err != nil {
		b.Fatalf("ReadFile(bold italic font) error = %v", err)
	}
	cache := document.NewFontCache()
	if err := cache.AddUTF8FontFromBytes("DejaVu", "", fontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes() error = %v", err)
	}
	if err := cache.AddUTF8FontFromBytes("DejaVu", "B", boldFontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes(bold) error = %v", err)
	}
	if err := cache.AddUTF8FontFromBytes("DejaVu", "I", italicFontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes(italic) error = %v", err)
	}
	if err := cache.AddUTF8FontFromBytes("DejaVu", "BI", boldItalicFontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes(bold italic) error = %v", err)
	}
	return cache
}

func benchmarkSelectorHeavyHTML() string {
	var css strings.Builder
	css.WriteString(`<style>`)
	for i := 0; i < 36; i++ {
		fmt.Fprintf(&css, `.report .group%d > p.item%d { color: #%02x%02x%02x; font-weight: bold; }`, i%6, i, 20+i, 80+i, 140+i)
	}
	css.WriteString(`</style><section class="report">`)
	var body strings.Builder
	for i := 0; i < 72; i++ {
		fmt.Fprintf(&body, `<div class="group%d"><p id="row%d" class="item%d">Selector heavy row %03d with inline text.</p></div>`, i%6, i, i%36, i)
	}
	body.WriteString(`</section>`)
	return css.String() + body.String()
}

func benchmarkTableHeavyHTML() string {
	var out strings.Builder
	out.WriteString(`<style>td,th{padding:2px;border:1px solid #555}.num{text-align:right}</style>`)
	for table := 0; table < 4; table++ {
		out.WriteString(`<table border="1" width="100%"><caption>Table heavy benchmark</caption><thead><tr><th>Code</th><th>Description</th><th>Value</th></tr></thead><tbody>`)
		for row := 0; row < 36; row++ {
			fmt.Fprintf(&out, `<tr><td>T%d-%03d</td><td><p>Compiled cell paragraph</p><ul><li>Nested item</li></ul></td><td class="num">%0.2f</td></tr>`, table, row, float64(row)*1.25)
		}
		out.WriteString(`</tbody></table>`)
	}
	return out.String()
}

func benchmarkDataImageHeavyHTML() string {
	const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	var out strings.Builder
	for i := 0; i < 64; i++ {
		fmt.Fprintf(&out, `<p>Compiled data image %02d</p><img src="%s" width="16" height="16" alt="pixel"/>`, i, pngDataURI)
	}
	return out.String()
}

func benchmarkMalformedHTML() string {
	var out strings.Builder
	out.WriteString(`<section><h2>Malformed compiled HTML`)
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&out, `<p><strong>Row %03d <em>misnested</strong> text`, i)
		if i%5 == 0 {
			out.WriteString(`<ul><li>Open list item<li>Sibling`)
		}
	}
	return out.String()
}

func benchmarkTaggedHTML() string {
	var rows strings.Builder
	for i := 0; i < 32; i++ {
		fmt.Fprintf(&rows, `<tr><td>Item %03d</td><td><p>Generated tagged cell</p><ul><li>Nested list item</li></ul></td><td style="text-align:right">%0.2f</td></tr>`,
			i, float64(i)*2.45)
	}
	return `<style>
		td, th { padding: 3px; border: 1px solid #555555; }
		th { background-color: #eeeeee; font-weight: bold; }
	</style>` +
		`<p>Tagged paragraph with <strong>strong text</strong>, <em>emphasis</em>, and semantic list/table structure.</p>` +
		`<ul><li>First semantic item</li><li>Second semantic item<ul><li>Nested semantic item</li></ul></li></ul>` +
		`<table border="1" cellpadding="3" width="100%">` +
		`<caption>Benchmark tagged table</caption>` +
		`<thead><tr><th>Code</th><th>Description</th><th>Value</th></tr></thead>` +
		`<tbody>` + rows.String() + `</tbody></table>`
}

func BenchmarkGenerationUTF8Text(b *testing.B) {
	text, err := os.ReadFile(example.TextFile("utf-8test.txt"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddUTF8Font("DejaVu", "", example.FontFile("DejaVuSansCondensed.ttf"))
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 11)
		pdf.MultiCell(0, 5, string(text), "", "L", false)
	})
}

func BenchmarkGenerationUTF8TextCachedFont(b *testing.B) {
	text, err := os.ReadFile(example.TextFile("utf-8test.txt"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}
	fontBytes, err := os.ReadFile(example.FontFile("DejaVuSansCondensed.ttf"))
	if err != nil {
		b.Fatalf("ReadFile() error = %v", err)
	}
	cache := document.NewFontCache()
	if err := cache.AddUTF8FontFromBytes("DejaVu", "", fontBytes); err != nil {
		b.Fatalf("AddUTF8FontFromBytes() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddUTF8FontFromCache("DejaVu", "", cache)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 11)
		pdf.MultiCell(0, 5, string(text), "", "L", false)
	})
}

func BenchmarkGenerationTextCompressionLevel(b *testing.B) {
	for _, tc := range []struct {
		name  string
		level int
	}{
		{name: "BestSpeed", level: zlib.BestSpeed},
		{name: "BestCompression", level: zlib.BestCompression},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				pdf := document.New("P", "mm", "A4", "")
				pdf.SetCompressionLevel(tc.level)
				pdf.AddPage()
				pdf.SetFont("Arial", "", 10)
				for row := 0; row < 180; row++ {
					pdf.CellFormat(40, 6, fmt.Sprintf("Row %03d", row), "1", 0, "L", false, 0, "")
					pdf.CellFormat(80, 6, "Operational PDF generation benchmark", "1", 0, "L", false, 0, "")
					pdf.CellFormat(40, 6, fmt.Sprintf("%0.2f", float64(row)*1.25), "1", 1, "R", false, 0, "")
				}
				var output bytes.Buffer
				if err := pdf.Output(&output); err != nil {
					b.Fatalf("Output() error = %v", err)
				}
				if output.Len() == 0 {
					b.Fatal("generated empty PDF")
				}
			}
		})
	}
}

func BenchmarkGenerationImages(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
			x := 10 + float64(i%2)*90
			y := 15 + float64(i/2)*70
			pdf.ImageOptions(example.ImageFile(image), x, y, 60, 0, false, document.ImageOptions{}, 0, "")
			pdf.Text(x, y+50, image)
		}
	})
}

func BenchmarkGenerationImagesConcurrentCPU(b *testing.B) {
	benchmarkGeneratedPDFConcurrentCPU(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
			x := 10 + float64(i%2)*90
			y := 15 + float64(i/2)*70
			pdf.ImageOptions(example.ImageFile(image), x, y, 60, 0, false, document.ImageOptions{}, 0, "")
			pdf.Text(x, y+50, image)
		}
	})
}

func BenchmarkGenerationImagesCached(b *testing.B) {
	cache := document.NewImageCache()
	for _, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
		if _, err := cache.RegisterImageOptions(image, example.ImageFile(image), document.ImageOptions{}); err != nil {
			b.Fatalf("RegisterImageOptions(%s) error = %v", image, err)
		}
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
			x := 10 + float64(i%2)*90
			y := 15 + float64(i/2)*70
			pdf.ImageFromCache(image, cache, x, y, 60, 0, false, document.ImageOptions{}, 0, "")
			pdf.Text(x, y+50, image)
		}
	})
}

func BenchmarkGenerationImagesCachedConcurrentCPU(b *testing.B) {
	cache := document.NewImageCache()
	for _, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
		if _, err := cache.RegisterImageOptions(image, example.ImageFile(image), document.ImageOptions{}); err != nil {
			b.Fatalf("RegisterImageOptions(%s) error = %v", image, err)
		}
	}

	benchmarkGeneratedPDFConcurrentCPU(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i, image := range []string{"logo.png", "logo.jpg", "logo.gif", "logo-rgb.png"} {
			x := 10 + float64(i%2)*90
			y := 15 + float64(i/2)*70
			pdf.ImageFromCache(image, cache, x, y, 60, 0, false, document.ImageOptions{}, 0, "")
			pdf.Text(x, y+50, image)
		}
	})
}

func BenchmarkGenerationSVG(b *testing.B) {
	svg, err := document.SVGParse([]byte(`<svg width="240" height="160" viewBox="0 0 240 160">
		<rect x="12" y="10" width="216" height="140" rx="8" fill="none" stroke="#3c5a8c" stroke-width="4"/>
		<path d="M42 46h156M42 76h156M42 106h108" fill="none" stroke="#3c5a8c" stroke-width="8" stroke-linecap="round"/>
		<circle cx="188" cy="112" r="18" fill="none" stroke="#3c5a8c" stroke-width="6"/>
	</svg>`))
	if err != nil {
		b.Fatalf("SVGParse() error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.AddPage()
		pdf.SetDrawColor(60, 90, 140)
		pdf.SetLineWidth(0.4)
		for i := 0; i < 8; i++ {
			pdf.SetXY(10+float64(i%2)*85, 12+float64(i/2)*60)
			pdf.SVGWrite(&svg, 0.18)
		}
	})
}

func BenchmarkGenerationTemplates(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		template := pdf.CreateTemplate(func(tpl *document.Tpl) {
			tpl.ImageOptions(example.ImageFile("logo.png"), 6, 6, 28, 0, false, document.ImageOptions{}, 0, "")
			tpl.SetFont("Arial", "B", 14)
			tpl.Text(40, 20, "Template benchmark")
			tpl.SetDrawColor(0, 100, 200)
			tpl.SetLineWidth(1.8)
			tpl.Line(95, 12, 105, 22)
		})

		pdf.AddPage()
		for y := 0; y < 5; y++ {
			for x := 0; x < 2; x++ {
				pdf.SetXY(5+float64(x)*95, 10+float64(y)*45)
				pdf.UseTemplate(template)
			}
		}
	})
}

func BenchmarkGenerationImportedPDFPages(b *testing.B) {
	source := func() []byte {
		pdf := document.New("P", "pt", "A4", "")
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 16)
		pdf.Text(72, 96, "Imported benchmark page")
		pdf.Rect(70, 110, 220, 60, "D")
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			b.Fatalf("source Output() error = %v", err)
		}
		return out.Bytes()
	}()

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		imported := pdf.ImportPageStream(bytes.NewReader(source), 1, "MediaBox")
		pdf.AddPage()
		for row := 0; row < 4; row++ {
			for col := 0; col < 2; col++ {
				pdf.UseImportedPage(imported, 8+float64(col)*100, 10+float64(row)*62, 88, 0)
			}
		}
	})
}

func BenchmarkGenerationProtection(b *testing.B) {
	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		pdf.SetProtection(document.CnProtectPrint, "reader", "owner")
		pdf.AddPage()
		pdf.SetFont("Arial", "", 10)
		for i := 0; i < 80; i++ {
			pdf.CellFormat(0, 5, fmt.Sprintf("Protected line %02d", i), "", 1, "L", false, 0, "")
		}
	})
}

func BenchmarkGenerationAttachments(b *testing.B) {
	grid, err := os.ReadFile("grid.go")
	if err != nil {
		b.Fatalf("ReadFile(grid.go) error = %v", err)
	}
	license, err := os.ReadFile("../LICENSE")
	if err != nil {
		b.Fatalf("ReadFile(LICENSE) error = %v", err)
	}

	benchmarkGeneratedPDF(b, func(pdf *document.Document) {
		attachments := []document.Attachment{
			{Content: grid, Filename: "grid.go", Description: "Grid example source"},
			{Content: license, Filename: "LICENSE", Description: "License text"},
		}
		pdf.SetAttachments(attachments)
		pdf.AddPage()
		pdf.SetFont("Arial", "", 12)
		for i, attachment := range attachments {
			y := 20 + float64(i)*30
			pdf.SetXY(15, y)
			pdf.Cell(70, 10, strings.TrimSpace(attachment.Description))
			pdf.Rect(12, y-2, 80, 14, "D")
			pdf.AddAttachmentAnnotation(&attachments[i], 12, y-2, 80, 14)
		}
	})
}
