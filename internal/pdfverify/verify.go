// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package pdfverify verifies committed serialized PDF bytes independently from
// layout planning and PDF painting. Raster evidence comes from an explicitly
// versioned external renderer; structural/text evidence is extracted from the
// final bytes; external compliance results are exact hash-bound inputs.
package pdfverify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/characterize"
)

const ReportVersion uint16 = 1

var (
	ErrInvalid            = errors.New("pdfverify: invalid request")
	ErrLimit              = errors.New("pdfverify: limit exceeded")
	ErrRaster             = errors.New("pdfverify: raster failure")
	ErrVerificationFailed = errors.New("pdfverify: final PDF verification failed")
)

type Limits struct {
	MaxPDFBytes         uint64
	MaxPages            uint32
	MaxPixelsPerPage    uint64
	MaxRasterBytesPage  uint64
	MaxTotalRasterBytes uint64
	MaxJSONBytes        uint64
}

func DefaultLimits() Limits {
	return Limits{MaxPDFBytes: 64 << 20, MaxPages: 1024, MaxPixelsPerPage: 32 << 20,
		MaxRasterBytesPage: 64 << 20, MaxTotalRasterBytes: 256 << 20, MaxJSONBytes: 32 << 20}
}

func (limits Limits) valid() bool {
	hard := DefaultLimits()
	return limits.MaxPDFBytes > 0 && limits.MaxPDFBytes <= hard.MaxPDFBytes && limits.MaxPages > 0 && limits.MaxPages <= hard.MaxPages &&
		limits.MaxPixelsPerPage > 0 && limits.MaxPixelsPerPage <= hard.MaxPixelsPerPage && limits.MaxRasterBytesPage > 0 && limits.MaxRasterBytesPage <= hard.MaxRasterBytesPage &&
		limits.MaxTotalRasterBytes > 0 && limits.MaxTotalRasterBytes <= hard.MaxTotalRasterBytes && limits.MaxRasterBytesPage <= limits.MaxTotalRasterBytes &&
		limits.MaxJSONBytes > 0 && limits.MaxJSONBytes <= hard.MaxJSONBytes
}

type RasterTolerance struct {
	MaxChangedPixelsPPM      uint32 `json:"max_changed_pixels_ppm"`
	MaxChannelDelta          uint8  `json:"max_channel_delta"`
	MaxMeanChannelDeltaMilli uint32 `json:"max_mean_channel_delta_milli"`
}

func (t RasterTolerance) valid() bool {
	return t.MaxChangedPixelsPPM <= 1_000_000 && t.MaxMeanChannelDeltaMilli <= 255_000
}

// ValidateConfiguration preflights non-PDF verifier inputs before a caller
// consumes an approval or invokes an external renderer.
func ValidateConfiguration(dpi uint32, tolerance RasterTolerance, limits Limits, structure StructuralExpectation, required []string, compliance []ComplianceEvidence) error {
	if dpi < 36 || dpi > 600 || !tolerance.valid() || !limits.valid() || (structure.TextSHA256 != "" && !validSHA256(structure.TextSHA256)) {
		return ErrInvalid
	}
	_, _, err := validateCompliance(required, compliance)
	return err
}

type ExpectedRasterPage struct {
	Page               uint32
	PNG                []byte
	PlanRasterManifest string
}

type StructuralExpectation struct {
	Pages               uint32
	TextSHA256          string
	MinimumLinks        uint32
	MinimumDestinations uint32
	RequireMetadata     bool
	RequireTagged       bool
	RequirePDFUA2       bool
	RequirePDFA4        bool
	RequireOutputIntent bool
}

type ComplianceEvidence struct {
	Profile    string `json:"profile"`
	Tool       string `json:"tool"`
	Version    string `json:"version"`
	PDFSHA256  string `json:"pdf_sha256"`
	ReportHash string `json:"report_hash"`
	Passed     bool   `json:"passed"`
}

type Request struct {
	PDF                []byte
	PlanHash           string
	DPI                uint32
	ExpectedPages      []ExpectedRasterPage
	Tolerance          RasterTolerance
	Structure          StructuralExpectation
	RequiredCompliance []string
	Compliance         []ComplianceEvidence
	Limits             Limits
}

type RasterOutput struct {
	Renderer string
	Version  string
	Pages    [][]byte
}

type Rasterizer interface {
	Rasterize(context.Context, []byte, uint32, []image.Point, Limits) (RasterOutput, error)
}

type DiffBounds struct {
	MinX int `json:"min_x"`
	MinY int `json:"min_y"`
	MaxX int `json:"max_x"`
	MaxY int `json:"max_y"`
}

type PageEvidence struct {
	Page                  uint32      `json:"page"`
	Width                 int         `json:"width"`
	Height                int         `json:"height"`
	ExpectedSHA256        string      `json:"expected_sha256"`
	ActualSHA256          string      `json:"actual_sha256"`
	PlanRasterManifest    string      `json:"plan_raster_manifest"`
	ChangedPixels         uint64      `json:"changed_pixels"`
	ChangedPixelsPPM      uint32      `json:"changed_pixels_ppm"`
	MaximumChannelDelta   uint8       `json:"maximum_channel_delta"`
	MeanChannelDeltaMilli uint32      `json:"mean_channel_delta_milli"`
	DiffBounds            *DiffBounds `json:"diff_bounds,omitempty"`
	Passed                bool        `json:"passed"`
}

type Report struct {
	Version         uint16                    `json:"version"`
	PDFSHA256       string                    `json:"pdf_sha256"`
	PlanHash        string                    `json:"plan_hash"`
	Renderer        string                    `json:"renderer"`
	RendererVersion string                    `json:"renderer_version"`
	DPI             uint32                    `json:"dpi"`
	Pages           []PageEvidence            `json:"pages"`
	PDFVersion      string                    `json:"pdf_version"`
	PageCount       uint32                    `json:"page_count"`
	TextSHA256      string                    `json:"text_sha256"`
	Structure       characterize.PDFStructure `json:"structure"`
	Compliance      []ComplianceEvidence      `json:"compliance"`
	Failures        []string                  `json:"failures,omitempty"`
	Passed          bool                      `json:"passed"`
}

func (report Report) CanonicalJSON(maxBytes uint64) ([]byte, error) {
	if report.Version != ReportVersion || maxBytes == 0 {
		return nil, ErrInvalid
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	if uint64(len(encoded)) > maxBytes {
		return nil, ErrLimit
	}
	return encoded, nil
}

func Verify(ctx context.Context, request Request, rasterizer Rasterizer) (Report, error) {
	if ctx == nil || rasterizer == nil || !validSHA256(request.PlanHash) || request.DPI < 36 || request.DPI > 600 || !request.Tolerance.valid() {
		return Report{}, ErrInvalid
	}
	limits := request.Limits
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if !limits.valid() || len(request.PDF) == 0 || uint64(len(request.PDF)) > limits.MaxPDFBytes || len(request.ExpectedPages) == 0 || uint32(len(request.ExpectedPages)) > limits.MaxPages {
		return Report{}, ErrLimit
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	report := Report{Version: ReportVersion, PlanHash: request.PlanHash, DPI: request.DPI}
	pdfHash := sha256.Sum256(request.PDF)
	report.PDFSHA256 = hex.EncodeToString(pdfHash[:])
	characterizeDefaults := characterize.DefaultLimits()
	maxText := min(limits.MaxPDFBytes, characterizeDefaults.MaxTextBytes)
	characterization, err := characterize.Build(ctx, []characterize.Artifact{{Name: "final.pdf", PDF: request.PDF}}, "pdfverify final serialized bytes", characterize.Fingerprint{GOOS: "detached", GOARCH: "detached", GoVersion: "detached", CPUs: 1}, characterize.Limits{MaxFixtures: 1, MaxPDFBytes: limits.MaxPDFBytes, MaxTotalBytes: limits.MaxPDFBytes, MaxTextBytes: maxText, MaxNameBytes: 4096, MaxJSONBytes: limits.MaxJSONBytes})
	if err != nil || len(characterization.Fixtures) != 1 {
		return Report{}, fmt.Errorf("%w: structural inspection: %v", ErrVerificationFailed, err)
	}
	fixture := characterization.Fixtures[0]
	report.PDFVersion, report.PageCount, report.Structure = fixture.PDFVersion, fixture.Pages, fixture.Structure
	textHash := sha256.Sum256([]byte(fixture.Text))
	report.TextSHA256 = hex.EncodeToString(textHash[:])
	if report.PageCount != uint32(len(request.ExpectedPages)) {
		report.Failures = append(report.Failures, "final PDF page count differs from expected plan raster pages")
	}
	dimensions := make([]image.Point, len(request.ExpectedPages))
	expectedImages := make([]image.Image, len(request.ExpectedPages))
	var expectedBytes uint64
	for index, expected := range request.ExpectedPages {
		if expected.Page != uint32(index+1) || !validSHA256(expected.PlanRasterManifest) || uint64(len(expected.PNG)) > limits.MaxRasterBytesPage {
			return Report{}, ErrInvalid
		}
		expectedBytes += uint64(len(expected.PNG))
		if expectedBytes > limits.MaxTotalRasterBytes {
			return Report{}, ErrLimit
		}
		decoded, err := png.Decode(bytes.NewReader(expected.PNG))
		if err != nil || decoded.Bounds().Min != (image.Point{}) || decoded.Bounds().Empty() {
			return Report{}, fmt.Errorf("%w: expected page %d PNG", ErrInvalid, expected.Page)
		}
		pixels := uint64(decoded.Bounds().Dx()) * uint64(decoded.Bounds().Dy())
		if pixels == 0 || pixels > limits.MaxPixelsPerPage {
			return Report{}, ErrLimit
		}
		dimensions[index] = decoded.Bounds().Size()
		expectedImages[index] = decoded
	}
	raster, err := rasterizer.Rasterize(ctx, append([]byte(nil), request.PDF...), request.DPI, dimensions, limits)
	if err != nil {
		return Report{}, fmt.Errorf("%w: %v", ErrRaster, err)
	}
	if !validLabel(raster.Renderer) || !validLabel(raster.Version) || len(raster.Pages) != len(request.ExpectedPages) {
		return Report{}, fmt.Errorf("%w: invalid renderer result", ErrRaster)
	}
	report.Renderer, report.RendererVersion = raster.Renderer, raster.Version
	var totalRaster uint64
	for index, actualPNG := range raster.Pages {
		if err := ctx.Err(); err != nil {
			return Report{}, err
		}
		totalRaster += uint64(len(actualPNG))
		if uint64(len(actualPNG)) > limits.MaxRasterBytesPage || totalRaster > limits.MaxTotalRasterBytes {
			return Report{}, ErrLimit
		}
		actual, err := png.Decode(bytes.NewReader(actualPNG))
		if err != nil || actual.Bounds().Min != (image.Point{}) || actual.Bounds().Size() != dimensions[index] {
			return Report{}, fmt.Errorf("%w: page %d PNG geometry", ErrRaster, index+1)
		}
		evidence, err := comparePage(ctx, uint32(index+1), request.ExpectedPages[index], expectedImages[index], actualPNG, actual, request.Tolerance)
		if err != nil {
			return Report{}, err
		}
		report.Pages = append(report.Pages, evidence)
		if !evidence.Passed {
			report.Failures = append(report.Failures, fmt.Sprintf("page %d raster exceeds pinned tolerance", index+1))
		}
	}
	checkStructure(&report, request.Structure)
	compliance, failures, err := validateCompliance(request.RequiredCompliance, request.Compliance)
	if err != nil {
		return Report{}, err
	}
	report.Compliance = compliance
	report.Failures = append(report.Failures, failures...)
	for _, item := range report.Compliance {
		if item.PDFSHA256 != report.PDFSHA256 {
			report.Failures = append(report.Failures, "compliance evidence is bound to different PDF bytes: "+item.Profile)
		}
	}
	report.Passed = len(report.Failures) == 0
	if _, err := report.CanonicalJSON(limits.MaxJSONBytes); err != nil {
		return Report{}, err
	}
	if !report.Passed {
		return report, ErrVerificationFailed
	}
	return report, nil
}

func comparePage(ctx context.Context, page uint32, expected ExpectedRasterPage, expectedImage image.Image, actualPNG []byte, actual image.Image, tolerance RasterTolerance) (PageEvidence, error) {
	expectedHash := sha256.Sum256(expected.PNG)
	actualHash := sha256.Sum256(actualPNG)
	bounds := expectedImage.Bounds()
	total := uint64(bounds.Dx()) * uint64(bounds.Dy())
	var changed, channelSum uint64
	var maximum uint8
	diff := DiffBounds{MinX: bounds.Max.X, MinY: bounds.Max.Y, MaxX: -1, MaxY: -1}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		if y&31 == 0 {
			if err := ctx.Err(); err != nil {
				return PageEvidence{}, err
			}
		}
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			a := color.NRGBAModel.Convert(expectedImage.At(x, y)).(color.NRGBA)
			b := color.NRGBAModel.Convert(actual.At(x, y)).(color.NRGBA)
			pixelChanged := false
			for _, delta := range []uint8{absByte(a.R, b.R), absByte(a.G, b.G), absByte(a.B, b.B), absByte(a.A, b.A)} {
				channelSum += uint64(delta)
				if delta > maximum {
					maximum = delta
				}
				pixelChanged = pixelChanged || delta != 0
			}
			if pixelChanged {
				changed++
				if x < diff.MinX {
					diff.MinX = x
				}
				if y < diff.MinY {
					diff.MinY = y
				}
				if x > diff.MaxX {
					diff.MaxX = x
				}
				if y > diff.MaxY {
					diff.MaxY = y
				}
			}
		}
	}
	ppm := uint32(0)
	mean := uint32(0)
	if total != 0 {
		ppm = uint32((changed*1_000_000 + total/2) / total)
		mean = uint32((channelSum*1000 + total*2) / (total * 4))
	}
	var diffBounds *DiffBounds
	if changed != 0 {
		copy := diff
		diffBounds = &copy
	}
	return PageEvidence{Page: page, Width: bounds.Dx(), Height: bounds.Dy(), ExpectedSHA256: hex.EncodeToString(expectedHash[:]), ActualSHA256: hex.EncodeToString(actualHash[:]), PlanRasterManifest: expected.PlanRasterManifest,
		ChangedPixels: changed, ChangedPixelsPPM: ppm, MaximumChannelDelta: maximum, MeanChannelDeltaMilli: mean, DiffBounds: diffBounds,
		Passed: ppm <= tolerance.MaxChangedPixelsPPM && maximum <= tolerance.MaxChannelDelta && mean <= tolerance.MaxMeanChannelDeltaMilli}, nil
}

func checkStructure(report *Report, expected StructuralExpectation) {
	if expected.Pages != 0 && report.PageCount != expected.Pages {
		report.Failures = append(report.Failures, "structural page count mismatch")
	}
	if expected.TextSHA256 != "" && report.TextSHA256 != expected.TextSHA256 {
		report.Failures = append(report.Failures, "extracted text hash mismatch")
	}
	if report.Structure.LinkAnnotations < expected.MinimumLinks {
		report.Failures = append(report.Failures, "missing required link annotations")
	}
	if report.Structure.Destinations < expected.MinimumDestinations {
		report.Failures = append(report.Failures, "missing required destinations")
	}
	if expected.RequireMetadata && !report.Structure.HasMetadata {
		report.Failures = append(report.Failures, "missing required metadata")
	}
	if expected.RequireTagged && (!report.Structure.HasTagMarkInfo || report.Structure.StructureTrees == 0) {
		report.Failures = append(report.Failures, "missing required tagged structure")
	}
	if expected.RequirePDFUA2 && !report.Structure.PDFUA2 {
		report.Failures = append(report.Failures, "missing required PDF/UA-2 marker")
	}
	if expected.RequirePDFA4 && !report.Structure.PDFA4 {
		report.Failures = append(report.Failures, "missing required PDF/A-4 marker")
	}
	if expected.RequireOutputIntent && report.Structure.OutputIntents == 0 {
		report.Failures = append(report.Failures, "missing required output intent")
	}
}

func validateCompliance(required []string, evidence []ComplianceEvidence) ([]ComplianceEvidence, []string, error) {
	result := append([]ComplianceEvidence(nil), evidence...)
	sort.Slice(result, func(i, j int) bool { return result[i].Profile < result[j].Profile })
	seen := make(map[string]ComplianceEvidence, len(result))
	for _, item := range result {
		if !validLabel(item.Profile) || !validLabel(item.Tool) || !validLabel(item.Version) || !validSHA256(item.PDFSHA256) || !validSHA256(item.ReportHash) {
			return nil, nil, ErrInvalid
		}
		if _, exists := seen[item.Profile]; exists {
			return nil, nil, ErrInvalid
		}
		seen[item.Profile] = item
	}
	var failures []string
	requiredSeen := make(map[string]struct{}, len(required))
	for _, profile := range required {
		if !validLabel(profile) {
			return nil, nil, ErrInvalid
		}
		if _, exists := requiredSeen[profile]; exists {
			return nil, nil, ErrInvalid
		}
		requiredSeen[profile] = struct{}{}
		item, exists := seen[profile]
		if !exists || !item.Passed {
			failures = append(failures, "required compliance profile did not pass: "+profile)
		}
	}
	return result, failures, nil
}

func absByte(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
func validLabel(value string) bool {
	return value != "" && len(value) <= 4096 && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n")
}
