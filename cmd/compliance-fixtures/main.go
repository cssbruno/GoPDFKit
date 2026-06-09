// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Command compliance-fixtures generates candidate PDFs for external standards
// validators. The generated files are intentionally small and deterministic.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	outDir := flag.String("out", filepath.Join("artifacts", "compliance"), "directory for generated compliance candidate PDFs")
	iccPath := flag.String("icc", "", "path to an sRGB ICC profile for PDF/A output intents")
	flag.Parse()

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		exitErr(err)
	}
	root, err := repoRoot()
	if err != nil {
		exitErr(err)
	}
	fontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed.ttf")
	boldFontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed-Bold.ttf")

	path := filepath.Join(*outDir, "pdfua2-arlington-metadata-foundation.pdf")
	if err := generatePDFUAArlingtonFoundation(path, root, fontPath, boldFontPath); err != nil {
		exitErr(err)
	}
	fmt.Printf("generated %s\n", path)

	if *iccPath == "" {
		fmt.Fprintln(os.Stderr, "SRGB_ICC/-icc not set; skipped PDF/A fixtures that require a real ICC profile")
		return
	}
	icc, err := os.ReadFile(*iccPath)
	if err != nil {
		exitErr(fmt.Errorf("read ICC profile: %w", err))
	}
	for _, fixture := range []struct {
		name       string
		mode       document.PDFAMode
		attachment bool
	}{
		{name: "pdfa4-metadata.pdf", mode: document.PDFAMode4},
		{name: "pdfa4f-attachment-metadata.pdf", mode: document.PDFAMode4F, attachment: true},
		{name: "pdfa4e-attachment-metadata.pdf", mode: document.PDFAMode4E, attachment: true},
	} {
		path := filepath.Join(*outDir, fixture.name)
		if err := generatePDFAFoundation(path, fontPath, boldFontPath, icc, fixture.mode, fixture.attachment); err != nil {
			exitErr(err)
		}
		fmt.Printf("generated %s\n", path)
	}
}

func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("resolve command source path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("resolve repository root from %s: %w", file, err)
	}
	return root, nil
}

func generatePDFUAArlingtonFoundation(path, root, fontPath, boldFontPath string) error {
	pdf := baseDocument(fontPath, boldFontPath)
	pdf.SetTitle("PDF/UA-2 Arlington metadata foundation", false)
	pdf.SetSubject("Generated tagged PDF structure, metadata, and catalog markers for external validation workflow", false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{
		PDFUA2:     true,
		Arlington:  true,
		Lang:       "en-US",
		Identifier: "urn:uuid:gopdfkit-pdfua2-arlington-foundation",
	})
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.SetNextTextRole("H1")
	pdf.CellFormat(0, 8, "PDF/UA-2 tagged structure fixture", "", 1, "L", false, 0, "")
	pdf.SetNextTextRole("P")
	pdf.MultiCell(0, 6, "This file exercises GoPDFKit tagged PDF output, XMP metadata, catalog markers, parent tree entries, marked content IDs, links, images, lists, tables, and artifacts.", "", "L", false)
	pdf.SetNextTextRole("Link")
	pdf.CellFormat(0, 7, "External reference link", "", 1, "L", false, 0, "https://example.com/gopdfkit")
	pdf.ImageOptions(filepath.Join(root, "assets", "static", "image", "logo.png"), 10, pdf.GetY()+2, 24, 0, false, document.ImageOptions{
		ImageType: "png",
		AltText:   "GoPDFKit logo",
	}, 0, "")
	pdf.Ln(18)
	html := pdf.HTMLNew()
	html.Write(6, `<ul><li>Tagged list label and body</li><li>Second semantic item</li></ul><table border="1"><caption>Tagged table caption</caption><tr><th>Name</th><th>Status</th><th>Detail</th></tr><tr><th scope="row" rowspan="2">Structure tree</th><td colspan="2"><p>Generated</p><div>Mixed block content</div><ul><li>Generated<ul><li>Nested table-cell list</li></ul></li></ul><table border="1"><tr><td>Nested table cell</td></tr></table></td></tr><tr><td>Parent tree</td><td>OK</td></tr></table>`)
	pdf.Line(10, pdf.GetY()+4, 80, pdf.GetY()+4)
	return pdf.OutputFileAndClose(path)
}

func generatePDFAFoundation(path, fontPath, boldFontPath string, icc []byte, mode document.PDFAMode, attachment bool) error {
	pdf := baseDocument(fontPath, boldFontPath)
	pdf.SetTitle("PDF/A-4 metadata foundation", false)
	pdf.SetSubject("Generated PDF/A-4 metadata, catalog, output intent, and font embedding fixture", false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{
		PDFA:       mode,
		Lang:       "en-US",
		Identifier: "urn:uuid:gopdfkit-" + string(mode) + "-foundation",
	})
	if err := pdf.SetOutputIntent(icc, "sRGB IEC61966-2.1"); err != nil {
		return err
	}
	if attachment {
		pdf.SetAttachments([]document.Attachment{{
			Filename:    "note.txt",
			Description: "PDF/A-4f attachment fixture",
			MIMEType:    "text/plain",
			Content:     []byte("Attachment used to exercise PDF/A-4f generation."),
		}})
	}
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.MultiCell(0, 6, "This file exercises GoPDFKit PDF/A-4 metadata, catalog output intent, and embedded UTF-8 font generation.", "", "L", false)
	return pdf.OutputFileAndClose(path)
}

func baseDocument(fontPath, boldFontPath string) *document.Document {
	pdf := document.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.SetCatalogSort(true)
	pdf.AddUTF8Font("DejaVu", "", fontPath)
	pdf.AddUTF8Font("DejaVu", "B", boldFontPath)
	return pdf
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
