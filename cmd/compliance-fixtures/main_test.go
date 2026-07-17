// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/characterize"
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
		"pdfa4-metadata.pdf":                       "59c329d6721f10a361b39444c9521feb6d706d51855bf58eeec4d55fd915d65a",
		"pdfa4e-attachment-metadata.pdf":           "b22383412126d7770ea9733bab35d59600f3b9402a42febddcd55f898c0ca4b4",
		"pdfa4f-attachment-metadata.pdf":           "4d565daaf4955fcdbccaef27ea70e8e0d8deadb54f409200691eb78d0b32b41f",
		"pdfua2-arlington-metadata-foundation.pdf": "2f4152a926e483833a6aef3ad72d6dd7de62ef52b6b1dfb3feb2c5afed6e17c3",
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

func TestPDFUAComplianceStructuredTableCellRasterDoesNotOverlapFollowingRow(t *testing.T) {
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
	if got := fmt.Sprintf("%x", sha256.Sum256(pngBytes)); got != "6cda70c5339f8d483fec26444f82d496d2fe69693e82006c3198183fdbf5bfd9" {
		t.Fatalf("structured-cell raster drift = %s", got)
	}
	raster, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	lines := rasterHorizontalLineGroups(raster, 500, 600)
	if len(lines) < 5 || lines[len(lines)-2]-lines[len(lines)-3] < 30 {
		t.Fatalf("table raster horizontal boundaries = %v; nested-table bottom must precede the following-row boundary", lines)
	}
}

func rasterHorizontalLineGroups(raster image.Image, minY, minimumRun int) []int {
	bounds := raster.Bounds()
	var groups []int
	for y := max(minY, bounds.Min.Y); y < bounds.Max.Y; y++ {
		run, longest := 0, 0
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := raster.At(x, y).RGBA()
			if r < 0x4000 && g < 0x4000 && b < 0x4000 {
				run++
				if run > longest {
					longest = run
				}
			} else {
				run = 0
			}
		}
		if longest < minimumRun {
			continue
		}
		if len(groups) == 0 || y-groups[len(groups)-1] > 1 {
			groups = append(groups, y)
		} else {
			groups[len(groups)-1] = y
		}
	}
	return groups
}
