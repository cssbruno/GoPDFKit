// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

// Command compliance-fixtures generates candidate PDFs for external standards
// validators. Unsigned fixtures use deterministic document metadata. Signed
// fixtures intentionally use fresh fixture-only key material, so their bytes
// differ between runs even though their document structure is stable.
package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"flag"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/sign"
)

func main() {
	outDir := flag.String("out", filepath.Join("artifacts", "compliance"), "directory for generated compliance candidate PDFs")
	iccPath := flag.String("icc", "", "path to an sRGB ICC profile for PDF/A output intents")
	flag.Parse()

	// #nosec G301 -- compliance fixtures are deliberately readable by validator containers.
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
	if err := makeArtifactReadable(path); err != nil {
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
		if err := makeArtifactReadable(path); err != nil {
			exitErr(err)
		}
		fmt.Printf("generated %s\n", path)
	}

	path = filepath.Join(*outDir, "pdfa4f-pdfua2-arlington-signed.pdf")
	if err := generateSignedComplianceFoundation(path, root, fontPath, boldFontPath, icc); err != nil {
		exitErr(err)
	}
	if err := makeArtifactReadable(path); err != nil {
		exitErr(err)
	}
	fmt.Printf("generated %s\n", path)
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
	html.Write(6, complianceTaggedHTMLFragment())
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

func generateSignedComplianceFoundation(path, root, fontPath, boldFontPath string, icc []byte) error {
	pdf := baseDocument(fontPath, boldFontPath)
	pdf.SetTitle("Signed PDF/A-4f PDF/UA-2 Arlington metadata foundation", false)
	pdf.SetSubject("Generated signed compliance fixture for PDF/A-4f, PDF/UA-2, Arlington, and XMP metadata validation", false)
	pdf.SetAuthor("GoPDFKit compliance fixtures", false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{
		PDFA:       document.PDFAMode4F,
		PDFUA2:     true,
		Arlington:  true,
		Lang:       "en-US",
		Title:      "Signed PDF/A-4f PDF/UA-2 Arlington metadata foundation",
		Identifier: "urn:uuid:gopdfkit-signed-pdfa4f-pdfua2-arlington-foundation",
	})
	if err := pdf.SetOutputIntent(icc, "sRGB IEC61966-2.1"); err != nil {
		return err
	}
	pdf.SetAttachments([]document.Attachment{{
		Filename:       "signed-note.txt",
		Description:    "Signed PDF/A-4f attachment fixture",
		MIMEType:       "text/plain",
		AFRelationship: "Data",
		Content:        []byte("Attachment used to exercise signed PDF/A-4f compliance generation."),
	}})
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.SetNextTextRole("H1")
	pdf.CellFormat(0, 8, "Signed compliance fixture", "", 1, "L", false, 0, "")
	pdf.SetNextTextRole("P")
	pdf.MultiCell(0, 6, "This signed fixture exercises PDF/A-4f metadata, PDF/UA-2 tagged content, Arlington model checks, XMP metadata, attachments, output intents, and detached CMS signing.", "", "L", false)
	pdf.SetNextTextRole("Link")
	pdf.CellFormat(0, 7, "Signed fixture reference link", "", 1, "L", false, 0, "https://example.com/gopdfkit/signed")
	pdf.ImageOptions(filepath.Join(root, "assets", "static", "image", "logo.png"), 10, pdf.GetY()+2, 20, 0, false, document.ImageOptions{
		ImageType: "png",
		AltText:   "GoPDFKit logo",
	}, 0, "")
	pdf.Ln(16)
	html := pdf.HTMLNew()
	html.Write(6, complianceTaggedHTMLFragment())
	pdf.Line(10, pdf.GetY()+4, 80, pdf.GetY()+4)

	cert, signer, err := complianceSigner()
	if err != nil {
		return err
	}
	return pdf.OutputSignedFile(path, sign.Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		SubFilter:       sign.SubFilterAdobePKCS7Detached,
		Name:            "GoPDFKit Compliance Signer",
		Reason:          "Compliance fixture",
		Location:        "CI",
		SigningTime:     time.Unix(1_704_067_200, 0).UTC(),
		SignatureSize:   64 << 10,
	})
}

func complianceSigner() (*x509.Certificate, crypto.Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate signing key: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GoPDFKit Compliance Signer"},
		NotBefore:             time.Unix(1_704_067_200, 0).UTC().Add(-time.Hour),
		NotAfter:              time.Unix(1_704_067_200, 0).UTC().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		UnknownExtKeyUsage:    []asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 36}},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, nil, fmt.Errorf("create signing certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse signing certificate: %w", err)
	}
	return cert, key, nil
}

func complianceTaggedHTMLFragment() string {
	return `<ul><li>Tagged list label and body</li><li>Second semantic item</li></ul><table border="1"><caption>Tagged table caption</caption><tr><th>Name</th><th>Status</th><th>Detail</th></tr><tr><th scope="row" rowspan="2">Structure tree</th><td colspan="2"><p>Generated</p><div>Mixed block content</div><ul><li>Generated<ul><li>Nested table-cell list</li></ul></li></ul><table border="1"><tr><td>Nested table cell</td></tr></table></td></tr><tr><td>Parent tree</td><td>OK</td></tr></table>`
}

func baseDocument(fontPath, boldFontPath string) *document.Document {
	pdf := document.MustNew(document.WithDeterministicOutput())
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

func makeArtifactReadable(path string) error {
	// #nosec G302 -- compliance artifacts must be readable by external validator containers.
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("make artifact readable %s: %w", path, err)
	}
	return nil
}
