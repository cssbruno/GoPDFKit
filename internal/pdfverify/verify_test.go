// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package pdfverify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os/exec"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/document"
)

type rasterizerFunc func(context.Context, []byte, uint32, []image.Point, Limits) (RasterOutput, error)

func (function rasterizerFunc) Rasterize(ctx context.Context, pdf []byte, dpi uint32, dimensions []image.Point, limits Limits) (RasterOutput, error) {
	return function(ctx, pdf, dpi, dimensions, limits)
}

func blankFinalPDF(t *testing.T) ([]byte, []byte) {
	t.Helper()
	pdf, err := document.NewDocument(document.WithUnit(document.UnitPoint), document.WithCustomPageSize(document.Size{Wd: 72, Ht: 72}), document.WithDeterministicOutput())
	if err != nil {
		t.Fatal(err)
	}
	pdf.SetMargins(0, 0, 0)
	pdf.AddPage()
	var output bytes.Buffer
	if err := pdf.OutputWithOptions(&output, document.OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, 72, 72))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	var raster bytes.Buffer
	if err := png.Encode(&raster, canvas); err != nil {
		t.Fatal(err)
	}
	return output.Bytes(), raster.Bytes()
}

func verificationRequest(t *testing.T) Request {
	t.Helper()
	pdf, raster := blankFinalPDF(t)
	return Request{PDF: pdf, PlanHash: strings.Repeat("1", 64), DPI: 72,
		ExpectedPages: []ExpectedRasterPage{{Page: 1, PNG: raster, PlanRasterManifest: strings.Repeat("2", 64)}},
		Tolerance:     RasterTolerance{}, Structure: StructuralExpectation{Pages: 1}}
}

func TestVerifyBindsFinalBytesRasterStructureAndCompliance(t *testing.T) {
	request := verificationRequest(t)
	request.RequiredCompliance = []string{"layout"}
	pdfHash := sha256.Sum256(request.PDF)
	request.Compliance = []ComplianceEvidence{{Profile: "layout", Tool: "fixture-validator", Version: "1.0", PDFSHA256: hex.EncodeToString(pdfHash[:]), ReportHash: strings.Repeat("3", 64), Passed: true}}
	fake := rasterizerFunc(func(_ context.Context, pdf []byte, dpi uint32, dimensions []image.Point, _ Limits) (RasterOutput, error) {
		if !bytes.Equal(pdf, request.PDF) || dpi != 72 || len(dimensions) != 1 || dimensions[0] != (image.Point{X: 72, Y: 72}) {
			t.Fatalf("raster request mismatch")
		}
		return RasterOutput{Renderer: "test-raster", Version: "1.0", Pages: [][]byte{append([]byte(nil), request.ExpectedPages[0].PNG...)}}, nil
	})
	report, err := Verify(context.Background(), request, fake)
	if err != nil || !report.Passed || report.Pages[0].ChangedPixels != 0 || report.Renderer != "test-raster" || len(report.Compliance) != 1 {
		t.Fatalf("verification = %#v, %v", report, err)
	}
	digest := sha256.Sum256(request.PDF)
	if report.PDFSHA256 != hex.EncodeToString(digest[:]) || report.PageCount != 1 || report.Pages[0].PlanRasterManifest != strings.Repeat("2", 64) {
		t.Fatalf("bound evidence = %#v", report)
	}
	first, _ := report.CanonicalJSON(DefaultLimits().MaxJSONBytes)
	second, _ := report.CanonicalJSON(DefaultLimits().MaxJSONBytes)
	if !bytes.Equal(first, second) {
		t.Fatal("canonical report is nondeterministic")
	}
}

func TestVerifyReturnsBoundedRasterAndComplianceFailures(t *testing.T) {
	request := verificationRequest(t)
	request.RequiredCompliance = []string{"pdfua-2"}
	pdfHash := sha256.Sum256(request.PDF)
	request.Compliance = []ComplianceEvidence{{Profile: "pdfua-2", Tool: "validator", Version: "2", PDFSHA256: hex.EncodeToString(pdfHash[:]), ReportHash: strings.Repeat("4", 64), Passed: false}}
	changed, _ := png.Decode(bytes.NewReader(request.ExpectedPages[0].PNG))
	canvas := image.NewNRGBA(changed.Bounds())
	draw.Draw(canvas, canvas.Bounds(), changed, image.Point{}, draw.Src)
	canvas.Set(10, 11, color.Black)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	fake := rasterizerFunc(func(context.Context, []byte, uint32, []image.Point, Limits) (RasterOutput, error) {
		return RasterOutput{Renderer: "test-raster", Version: "1.0", Pages: [][]byte{encoded.Bytes()}}, nil
	})
	report, err := Verify(context.Background(), request, fake)
	if !errors.Is(err, ErrVerificationFailed) || report.Passed || len(report.Failures) != 2 || report.Pages[0].ChangedPixels != 1 || report.Pages[0].DiffBounds == nil || *report.Pages[0].DiffBounds != (DiffBounds{MinX: 10, MinY: 11, MaxX: 10, MaxY: 11}) {
		t.Fatalf("failed verification = %#v, %v", report, err)
	}
}

func TestPopplerRasterizerVerifiesCommittedBlankPDF(t *testing.T) {
	binary, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm is not installed")
	}
	request := verificationRequest(t)
	report, err := Verify(context.Background(), request, PopplerRasterizer{Binary: binary, Version: "26.05.0", TempRoot: t.TempDir()})
	if err != nil || !report.Passed || report.Renderer != "poppler/pdftoppm" || report.Pages[0].ChangedPixels != 0 {
		t.Fatalf("Poppler verification = %#v, %v", report, err)
	}
}
