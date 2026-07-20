// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/inspect"
)

const maxEdgeRasterBytes = 32 << 20

type edgePDFRasterizer struct {
	executable  string
	environment []string
	fontCache   string
}

func newEdgePDFRasterizer() (*edgePDFRasterizer, error) {
	pdftoppm, err := exec.LookPath("pdftoppm")
	if err != nil {
		return nil, errors.New("pdftoppm is required for --edge-visual; install Poppler or run without visual evidence")
	}
	fontCache, err := os.MkdirTemp("", "paperrune-fontconfig-*")
	if err != nil {
		return nil, fmt.Errorf("create font cache: %w", err)
	}
	commandEnvironment := append(os.Environ(), "XDG_CACHE_HOME="+fontCache)
	fontConfig := filepath.Clean(filepath.Join(filepath.Dir(pdftoppm), "..", "..", "native", "poppler", "poppler", "etc", "fonts", "fonts.conf"))
	if info, statErr := os.Stat(fontConfig); statErr == nil && !info.IsDir() {
		commandEnvironment = append(commandEnvironment, "FONTCONFIG_FILE="+fontConfig)
	}
	return &edgePDFRasterizer{executable: pdftoppm, environment: commandEnvironment, fontCache: fontCache}, nil
}

func (r *edgePDFRasterizer) Close() {
	if r != nil && r.fontCache != "" {
		_ = os.RemoveAll(r.fontCache)
		r.fontCache = ""
	}
}

func (r *edgePDFRasterizer) Rasterize(pdfFile, outputDir, baseName string, pages int, dpi uint) ([]edgeCheckRasterPage, error) {
	if r == nil || r.executable == "" {
		return nil, errors.New("PDF rasterizer is not initialized")
	}
	if pages <= 0 || pages > 1000 {
		return nil, fmt.Errorf("cannot rasterize %d pages", pages)
	}
	result := make([]edgeCheckRasterPage, 0, pages)
	for page := 1; page <= pages; page++ {
		name := fmt.Sprintf("%s-page-%03d.png", baseName, page)
		finalPath := filepath.Join(outputDir, name)
		temporary, err := os.CreateTemp(outputDir, ".edge-raster-*")
		if err != nil {
			return nil, fmt.Errorf("create raster temporary file: %w", err)
		}
		temporaryPrefix := temporary.Name()
		if err := temporary.Close(); err != nil {
			return nil, fmt.Errorf("close raster temporary file: %w", err)
		}
		if err := os.Remove(temporaryPrefix); err != nil {
			return nil, fmt.Errorf("prepare raster temporary path: %w", err)
		}
		temporaryPNG := temporaryPrefix + ".png"
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		command := exec.CommandContext(ctx, r.executable,
			"-png", "-r", strconv.FormatUint(uint64(dpi), 10),
			"-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-singlefile",
			pdfFile, temporaryPrefix,
		) // #nosec G204 -- executable is resolved explicitly and every argument is passed without a shell.
		command.Env = r.environment
		commandError := edgeDiagnosticWriter{remaining: 64 << 10}
		command.Stderr = &commandError
		runErr := command.Run()
		contextErr := ctx.Err()
		cancel()
		if runErr != nil {
			_ = os.Remove(temporaryPNG)
			if contextErr != nil {
				return nil, fmt.Errorf("rasterize page %d: %w", page, contextErr)
			}
			return nil, fmt.Errorf("rasterize page %d: %w: %s", page, runErr, commandError.String())
		}
		payload, readErr := readBoundedRaster(temporaryPNG)
		_ = os.Remove(temporaryPNG)
		if readErr != nil {
			return nil, fmt.Errorf("read raster page %d: %w", page, readErr)
		}
		configuration, decodeErr := png.DecodeConfig(bytes.NewReader(payload))
		if decodeErr != nil || configuration.Width <= 0 || configuration.Height <= 0 {
			return nil, fmt.Errorf("decode raster page %d: %w", page, decodeErr)
		}
		if err := atomicWrite(finalPath, payload, 0o644); err != nil {
			return nil, fmt.Errorf("write raster page %d: %w", page, err)
		}
		result = append(result, edgeCheckRasterPage{
			Page: page, File: name, SHA256: edgeSHA256(payload), Bytes: len(payload),
			Width: configuration.Width, Height: configuration.Height,
		})
	}
	return result, nil
}

type edgeDiagnosticWriter struct {
	buffer    bytes.Buffer
	remaining int
	truncated bool
}

func (w *edgeDiagnosticWriter) Write(payload []byte) (int, error) {
	accepted := len(payload)
	if w.remaining > 0 {
		stored := payload
		if len(stored) > w.remaining {
			stored = stored[:w.remaining]
			w.truncated = true
		}
		_, _ = w.buffer.Write(stored)
		w.remaining -= len(stored)
	} else if accepted != 0 {
		w.truncated = true
	}
	return accepted, nil
}

func (w *edgeDiagnosticWriter) String() string {
	if w.truncated {
		return w.buffer.String() + "\n[diagnostics truncated]"
	}
	return w.buffer.String()
}

func readBoundedRaster(file string) ([]byte, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, err
	}
	if info.Size() <= 0 || info.Size() > maxEdgeRasterBytes {
		return nil, fmt.Errorf("raster size %d is outside 1..%d bytes", info.Size(), maxEdgeRasterBytes)
	}
	opened, err := os.Open(file) // #nosec G304 -- the path is a private temporary created by this process.
	if err != nil {
		return nil, err
	}
	defer func() { _ = opened.Close() }()
	payload, err := io.ReadAll(io.LimitReader(opened, maxEdgeRasterBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maxEdgeRasterBytes {
		return nil, fmt.Errorf("raster exceeds %d-byte limit", maxEdgeRasterBytes)
	}
	return payload, nil
}

func writeEdgeVisualReview(outputDir string, report edgeCheckResult) error {
	pdf, err := document.NewDocument(document.WithDeterministicOutput())
	if err != nil {
		return fmt.Errorf("create edge visual review: %w", err)
	}
	pdf.SetTitle("PaperRune edge-case visual review", false)
	pdf.SetCreator("PaperRune paper check", false)
	pdf.SetMargins(12, 12, 12)
	pdf.AddPage()
	pdf.SetFillColor(20, 50, 62)
	pdf.Rect(0, 0, 210, 44, "F")
	pdf.SetTextColor(255, 255, 255)
	pdf.SetFont("Helvetica", "B", 22)
	pdf.Text(14, 22, "PaperRune visual review")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(14, 32, "Raster evidence from the final generated PDF files")
	pdf.SetTextColor(20, 35, 42)
	pdf.SetFont("Helvetica", "B", 13)
	pdf.Text(14, 62, fmt.Sprintf("Schema %s", report.Schema))
	passed := 0
	pageCount := 0
	for _, item := range report.Cases {
		if item.OK {
			passed++
		}
		pageCount += len(item.RasterPages)
	}
	pdf.SetFont("Helvetica", "", 11)
	pdf.SetXY(14, 72)
	pdf.MultiCell(182, 7, fmt.Sprintf(
		"Cases: %d\nPassed: %d\nFailed: %d\nRaster pages: %d\nMax layout issues: %d\nMinimum text runes: %d\nMaximum PDF pages: %d",
		len(report.Cases), passed, len(report.Cases)-passed, pageCount,
		report.Thresholds.MaxPageIssues, report.Thresholds.MinTextRunes, report.Thresholds.MaxPages,
	), "", "L", false)

	for _, item := range report.Cases {
		for _, raster := range item.RasterPages {
			pdf.AddPage()
			if item.OK {
				pdf.SetFillColor(22, 132, 111)
			} else {
				pdf.SetFillColor(185, 65, 65)
			}
			pdf.Rect(0, 0, 210, 18, "F")
			pdf.SetTextColor(255, 255, 255)
			pdf.SetFont("Helvetica", "B", 10)
			pdf.Text(12, 11, fmt.Sprintf("%s  |  PDF page %d of %d", item.Name, raster.Page, item.Pages))
			width, height := fitEdgeRaster(raster.Width, raster.Height, 186, 265)
			x := (210 - width) / 2
			y := 23 + (265-height)/2
			pdf.ImageOptions(filepath.Join(outputDir, raster.File), x, y, width, height, false, document.ImageOptions{ImageType: "PNG", AltText: fmt.Sprintf("Rendered PDF page %d for edge case %s", raster.Page, item.Name)}, 0, "")
		}
	}
	if pdf.Err() {
		return fmt.Errorf("compose edge visual review: %w", pdf.Error())
	}
	var encoded bytes.Buffer
	limited := &limitWriter{w: &encoded, remaining: maxPDFBytes}
	if err := pdf.OutputWithOptions(limited, document.OutputOptions{Deterministic: true}); err != nil {
		return fmt.Errorf("encode edge visual review: %w", err)
	}
	if err := inspect.ValidateStructure(encoded.Bytes()); err != nil {
		return fmt.Errorf("validate edge visual review: %w", err)
	}
	pages, err := inspect.PageCount(encoded.Bytes())
	if err != nil || pages != pageCount+1 {
		return fmt.Errorf("validate edge visual review page count: got %d, want %d: %w", pages, pageCount+1, err)
	}
	if err := atomicWrite(filepath.Join(outputDir, report.VisualReviewFile), encoded.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write edge visual review: %w", err)
	}
	return nil
}

func fitEdgeRaster(pixelWidth, pixelHeight int, maxWidth, maxHeight float64) (float64, float64) {
	if pixelWidth <= 0 || pixelHeight <= 0 {
		return 0, 0
	}
	width := maxWidth
	height := width * float64(pixelHeight) / float64(pixelWidth)
	if height > maxHeight {
		height = maxHeight
		width = height * float64(pixelWidth) / float64(pixelHeight)
	}
	return width, height
}
