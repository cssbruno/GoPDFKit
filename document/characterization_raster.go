// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

const (
	characterizationRasterMaxPages    uint32 = 512
	characterizationRasterMaxPNGBytes uint64 = 128 << 20
)

type CharacterizationRasterPage struct {
	Page           uint32                             `json:"page"`
	PNGSHA256      string                             `json:"png_sha256"`
	PNGBytes       uint64                             `json:"png_bytes"`
	ManifestSHA256 string                             `json:"manifest_sha256"`
	Manifest       layoutengine.DisplayRasterManifest `json:"manifest"`
}

// CharacterizationRasterEvidence pins a direct display-list preview. It is
// deliberately not described as an authoritative PDF-consumer raster.
type CharacterizationRasterEvidence struct {
	Renderer         string                            `json:"renderer"`
	AuthoritativePDF bool                              `json:"authoritative_pdf_raster"`
	Profile          layoutengine.DisplayRasterProfile `json:"profile"`
	Pages            []CharacterizationRasterPage      `json:"pages"`
}

type characterizationRasterBudget struct {
	pages uint32
	bytes uint64
}

func captureCharacterizationRaster(ctx context.Context, fixture string, plan LayoutDocumentPlan, budget *characterizationRasterBudget) (*CharacterizationRasterEvidence, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if budget == nil || plan.PageCount() <= 0 {
		return nil, "", errors.New("document: characterization raster requires a budget and non-empty plan")
	}
	if budget.pages > characterizationRasterMaxPages || uint64(plan.PageCount()) > uint64(characterizationRasterMaxPages-budget.pages) { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		return nil, "", errors.New("document: characterization raster page limit exceeded")
	}
	if budget.bytes > characterizationRasterMaxPNGBytes {
		return nil, "", errors.New("document: characterization raster byte limit exceeded")
	}
	projection := plan.plan.Projection()
	fontPrograms := make(map[layoutengine.CoreFontMetricsDigest][]byte, len(projection.Fonts))
	for _, font := range projection.Fonts {
		if font.EmbeddedUTF8 != nil {
			fontPrograms[font.MetricsDigest] = append([]byte(nil), plan.fontSources[font.EmbeddedUTF8.Digest]...)
		} else {
			fontPrograms[font.MetricsDigest] = goregular.TTF
		}
	}
	images := make(layoutengine.DisplaySVGImageSources, len(plan.imageSources))
	for digest, payload := range plan.imageSources {
		images[digest] = append([]byte(nil), payload...)
	}
	profile := layoutengine.DefaultDisplayRasterProfile()
	sources := layoutengine.DisplayRasterSources{FontPrograms: fontPrograms, Images: images}
	evidence := &CharacterizationRasterEvidence{
		Renderer: layoutengine.DisplayRasterRendererVersion, Profile: profile,
		Pages: make([]CharacterizationRasterPage, 0, plan.PageCount()),
	}
	request := layoutengine.DisplayRasterRequest{
		Profile: profile, Limits: layoutengine.DefaultDisplayRasterLimits(),
		Revisions: layoutengine.ViewerRevisionIdentityInput{
			SourceRevision:   characterizationDigest("source:" + fixture),
			ScenarioRevision: characterizationDigest("scenario:stage0-characterization-v1"),
			PolicyRevision:   characterizationDigest("policy:stage0-characterization-v1"),
		},
		PageProfile: characterizationDigest("page-profile:stage0-characterization-v1"),
	}
	for page := uint32(1); page <= uint32(plan.PageCount()); page++ { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		request.Page = page
		artifact, err := layoutengine.CaptureDisplayPlanPNGContext(ctx, plan.plan, sources, request)
		if errors.Is(err, layoutengine.ErrDisplayRasterUnsupported) {
			// The rasterizer is intentionally narrower than the display list. Do
			// not retain a supported prefix as misleading partial evidence.
			return nil, "unsupported-display-command", nil
		}
		if err != nil {
			return nil, "", fmt.Errorf("document: characterize raster %s page %d: %w", fixture, page, err)
		}
		manifestJSON, err := artifact.CanonicalManifestJSON()
		if err != nil {
			return nil, "", err
		}
		manifest := artifact.Manifest()
		if manifest.AuthoritativePDF || manifest.PNGByteLength == 0 || manifest.PNGByteLength != uint64(len(artifact.PNG())) {
			return nil, "", errors.New("document: characterization raster returned invalid evidence")
		}
		if manifest.PNGByteLength > characterizationRasterMaxPNGBytes-budget.bytes {
			return nil, "", errors.New("document: characterization raster byte limit exceeded")
		}
		manifestDigest := sha256.Sum256(manifestJSON)
		evidence.Pages = append(evidence.Pages, CharacterizationRasterPage{
			Page: page, PNGSHA256: manifest.PNGSHA256, PNGBytes: manifest.PNGByteLength,
			ManifestSHA256: hex.EncodeToString(manifestDigest[:]), Manifest: manifest,
		})
		budget.pages++
		budget.bytes += manifest.PNGByteLength
	}
	return evidence, "captured", nil
}

func characterizationDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
