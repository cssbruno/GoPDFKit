// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/characterize"
)

func TestBaseDocumentProducesDeterministicUnsignedBytes(t *testing.T) {
	t.Parallel()

	root, err := repoRoot()
	if err != nil {
		t.Fatalf("repoRoot() error = %v", err)
	}
	fontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed.ttf")
	boldFontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed-Bold.ttf")
	render := func() []byte {
		pdf := baseDocument(fontPath, boldFontPath)
		pdf.SetTitle("deterministic compliance fixture", false)
		pdf.AddPage()
		pdf.SetFont("DejaVu", "", 12)
		pdf.Write(6, "stable content")
		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		return output.Bytes()
	}

	first := render()
	second := render()
	if !bytes.Equal(first, second) {
		t.Fatal("deterministic compliance base changed between identical renders")
	}
}

func TestUnsignedComplianceFixturesHaveDeterministicCharacterization(t *testing.T) {
	t.Parallel()

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	fontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed.ttf")
	boldFontPath := filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed-Bold.ttf")
	directory := t.TempDir()
	paths := []string{filepath.Join(directory, "pdfua2-arlington-metadata-foundation.pdf")}
	if err := generatePDFUAArlingtonFoundation(paths[0], root, fontPath, boldFontPath); err != nil {
		t.Fatal(err)
	}
	// This byte sequence is intentionally only a generation-test placeholder.
	// External PDF/A validation continues to require a real sRGB profile.
	icc := []byte("deterministic characterization ICC placeholder; not a valid color profile")
	for _, fixture := range []struct {
		name       string
		mode       document.PDFAMode
		attachment bool
	}{
		{name: "pdfa4-metadata.pdf", mode: document.PDFAMode4},
		{name: "pdfa4f-attachment-metadata.pdf", mode: document.PDFAMode4F, attachment: true},
		{name: "pdfa4e-attachment-metadata.pdf", mode: document.PDFAMode4E, attachment: true},
	} {
		path := filepath.Join(directory, fixture.name)
		if err := generatePDFAFoundation(path, fontPath, boldFontPath, icc, fixture.mode, fixture.attachment); err != nil {
			t.Fatalf("generate %s: %v", fixture.name, err)
		}
		paths = append(paths, path)
	}

	firstPath := filepath.Join(directory, "report-first.json")
	secondPath := filepath.Join(directory, "report-second.json")
	if err := writeCharacterizationReport(context.Background(), firstPath, paths, characterizationCommand); err != nil {
		t.Fatal(err)
	}
	if err := writeCharacterizationReport(context.Background(), secondPath, append([]string(nil), paths...), characterizationCommand); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("identical unsigned compliance fixtures produced different characterization reports")
	}
	var report characterize.Report
	if err := json.Unmarshal(first, &report); err != nil {
		t.Fatal(err)
	}
	if report.Command != characterizationCommand || report.Fingerprint.GOOS != runtime.GOOS ||
		report.Fingerprint.GOARCH != runtime.GOARCH || report.Fingerprint.GoVersion != runtime.Version() ||
		report.Fingerprint.CPUs != runtime.NumCPU() {
		t.Fatalf("report reproduction metadata = command %q fingerprint %#v", report.Command, report.Fingerprint)
	}
	if len(report.Fixtures) != 4 {
		t.Fatalf("fixture count = %d, want 4", len(report.Fixtures))
	}
	byName := make(map[string]characterize.FixtureEvidence, len(report.Fixtures))
	wantSHA256 := map[string]string{
		"pdfa4-metadata.pdf":                       "bd25d09c3c3e163b564aa0147150fccdf624937004d9c8f4348c04f56230752e",
		"pdfa4e-attachment-metadata.pdf":           "fd47bc71f178e9a6735e23a8e1a4d3de159e518e714eb97c7b072a03da6c21a4",
		"pdfa4f-attachment-metadata.pdf":           "5524d63395c9eb2b5e90fdb9992a9e5e854291cd727c0e1979624ea87dc6649c",
		"pdfua2-arlington-metadata-foundation.pdf": "b6e0720e3831837f57c3516801c9fad5e8b2a8622036d693d8ccec9b2486b68f",
	}
	for _, fixture := range report.Fixtures {
		byName[fixture.Name] = fixture
		if fixture.Pages != 1 || fixture.PDFVersion != "2.0" || fixture.SHA256 == "" || fixture.Bytes == 0 ||
			len(fixture.PageText) != 1 || strings.TrimSpace(fixture.Text) == "" || !fixture.Structure.HasMetadata {
			t.Errorf("incomplete baseline for %s: %#v", fixture.Name, fixture)
		}
		if fixture.SHA256 != wantSHA256[fixture.Name] {
			t.Errorf("%s SHA-256 = %s, want pinned %s", fixture.Name, fixture.SHA256, wantSHA256[fixture.Name])
		}
	}
	ua := byName["pdfua2-arlington-metadata-foundation.pdf"]
	if !ua.Structure.PDFUA2 || !ua.Structure.ArlingtonRequired || !ua.Structure.HasTagMarkInfo ||
		!ua.Structure.HasLanguage || !ua.Structure.DisplaysTitle || ua.Structure.StructureTrees == 0 ||
		ua.Structure.StructureElements == 0 || ua.Structure.ParentTrees == 0 || ua.Structure.MarkedContent == 0 ||
		ua.Structure.LinkAnnotations == 0 || ua.Structure.URIActions == 0 || ua.Structure.PDFA4 {
		t.Errorf("PDF/UA structural baseline = %#v", ua.Structure)
	}
	for name, conformance := range map[string]string{
		"pdfa4-metadata.pdf":             "",
		"pdfa4e-attachment-metadata.pdf": "E",
		"pdfa4f-attachment-metadata.pdf": "F",
	} {
		fixture := byName[name]
		if !fixture.Structure.PDFA4 || fixture.Structure.PDFUA2 || !fixture.Structure.HasEmbeddedICC ||
			fixture.Structure.OutputIntents == 0 || fixture.Structure.PDFAConformance != conformance {
			t.Errorf("%s PDF/A structural baseline = %#v", name, fixture.Structure)
		}
		wantAttachment := conformance != ""
		if got := fixture.Structure.Attachments > 0 && fixture.Structure.AssociatedFiles > 0; got != wantAttachment {
			t.Errorf("%s attachment baseline = %t, want %t: %#v", name, got, wantAttachment, fixture.Structure)
		}
	}
}

func TestPDFUAComplianceStructuredTableCellRasterIsPinned(t *testing.T) {
	binary, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm is not installed")
	}
	version, err := exec.Command(binary, "-v").CombinedOutput()
	if err != nil || !bytes.Contains(version, []byte("pdftoppm version 26.05.0")) {
		t.Skipf("requires pinned pdftoppm 26.05.0, got %q", version)
	}
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	pdfPath := filepath.Join(directory, "pdfua2-arlington-metadata-foundation.pdf")
	if err := generatePDFUAArlingtonFoundation(pdfPath, root,
		filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed.ttf"),
		filepath.Join(root, "assets", "static", "font", "DejaVuSansCondensed-Bold.ttf")); err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(directory, "structured-cell")
	if output, err := exec.Command(binary, "-png", "-r", "144", "-f", "1", "-singlefile", pdfPath, prefix).CombinedOutput(); err != nil {
		t.Fatalf("pdftoppm: %v: %s", err, output)
	}
	pngBytes, err := os.ReadFile(prefix + ".png")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(pngBytes)); got != "9f1fbfcbda9f91541aa843b9501bd67c685720ee1d7ea2429f7cc9096ef48237" {
		t.Fatalf("structured-cell raster drift = %s", got)
	}
	raster, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	if raster.Bounds().Dx() == 0 || raster.Bounds().Dy() == 0 {
		t.Fatal("structured-cell raster has empty bounds")
	}
}
