// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

type PaperPlanRasterRequest struct {
	PageProfile     string
	DPI             uint32
	MaxPixels       uint64
	MaxSourceBytes  uint64
	MaxPNGBytes     uint64
	CoreFontProgram []byte
	FontPrograms    map[string][]byte
	Images          map[string][]byte
}

func DefaultPaperPlanRasterRequest() PaperPlanRasterRequest {
	profile := layoutengine.DefaultDisplayRasterProfile()
	limits := layoutengine.DefaultDisplayRasterLimits()
	return PaperPlanRasterRequest{DPI: profile.DPI, MaxPixels: limits.MaxPixels, MaxSourceBytes: limits.MaxSourceBytes, MaxPNGBytes: limits.MaxPNGBytes}
}

type PaperPlanRasterPage struct {
	Page           uint32 `json:"page"`
	PNG            []byte `json:"-"`
	PNGSHA256      string `json:"png_sha256"`
	ManifestJSON   []byte `json:"-"`
	ManifestSHA256 string `json:"manifest_sha256"`
}

type PaperPlanRasterBundle struct {
	PlanHash string                `json:"plan_hash"`
	Renderer string                `json:"renderer"`
	DPI      uint32                `json:"dpi"`
	Pages    []PaperPlanRasterPage `json:"pages"`
}

// CaptureRasterPages paints each immutable page display list directly. It does
// no layout and explicitly remains a plan preview; final serialized-PDF
// authority is established only by an independent PDF consumer comparison.
func (p PaperPlan) CaptureRasterPages(ctx context.Context, request PaperPlanRasterRequest) (PaperPlanRasterBundle, error) {
	if ctx == nil {
		return PaperPlanRasterBundle{}, errors.New("document: nil raster context")
	}
	if p.hash == "" || p.pages <= 0 {
		return PaperPlanRasterBundle{}, errors.New("document: empty paper plan")
	}
	profile := layoutengine.DefaultDisplayRasterProfile()
	profile.DPI = request.DPI
	limits := layoutengine.DisplayRasterLimits{MaxPixels: request.MaxPixels, MaxSourceBytes: request.MaxSourceBytes, MaxPNGBytes: request.MaxPNGBytes, Paint: layoutengine.DefaultDisplayPaintLimits()}
	pageProfile := request.PageProfile
	if pageProfile == "" {
		if inputs, ok := p.plan.DeterministicInputs(); ok {
			pageProfile = inputs.PageProfile.ID
		}
	}
	images := make(map[string][]byte, len(request.Images)+len(p.imageSources))
	for digest, encoded := range p.imageSources {
		images[string(digest)] = append([]byte(nil), encoded...)
	}
	for digest, encoded := range request.Images {
		images[digest] = append([]byte(nil), encoded...)
	}
	sources := paperReviewSources(p.plan, request.CoreFontProgram, request.FontPrograms, images)
	bundle := PaperPlanRasterBundle{PlanHash: p.hash, Renderer: layoutengine.DisplayRasterRendererVersion, DPI: request.DPI, Pages: make([]PaperPlanRasterPage, 0, p.pages)}
	for page := uint32(1); page <= uint32(p.pages); page++ { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		artifact, err := layoutengine.CaptureDisplayPlanPNGContext(ctx, p.plan, sources, layoutengine.DisplayRasterRequest{Page: page, Profile: profile, Limits: limits, Revisions: p.revisions, PageProfile: pageProfile})
		if err != nil {
			return PaperPlanRasterBundle{}, err
		}
		manifestJSON, err := artifact.CanonicalManifestJSON()
		if err != nil {
			return PaperPlanRasterBundle{}, err
		}
		manifestHash := sha256.Sum256(manifestJSON)
		pngBytes := artifact.PNG()
		manifest := artifact.Manifest()
		bundle.Pages = append(bundle.Pages, PaperPlanRasterPage{Page: page, PNG: pngBytes, PNGSHA256: manifest.PNGSHA256,
			ManifestJSON: append([]byte(nil), manifestJSON...), ManifestSHA256: hex.EncodeToString(manifestHash[:])})
	}
	return bundle, nil
}

// PaperReviewRequest is the public, transport-neutral request for one
// deterministic before/after visual review. Font and image bytes are explicit
// immutable renderer inputs; their hashes are recorded by the bundle.
type PaperReviewRequest struct {
	PageProfile         string            `json:"page_profile,omitempty"`
	DPI                 uint32            `json:"dpi"`
	MaxPixels           uint64            `json:"max_pixels"`
	MaxSourceBytes      uint64            `json:"max_source_bytes"`
	MaxPNGBytes         uint64            `json:"max_png_bytes"`
	MaxPages            uint32            `json:"max_pages"`
	MaxArtifacts        uint32            `json:"max_artifacts"`
	MaxArtifactBytes    uint64            `json:"max_artifact_bytes"`
	MaxTotalBytes       uint64            `json:"max_total_bytes"`
	MaxManifestBytes    uint64            `json:"max_manifest_bytes"`
	MaxDiffChanges      uint64            `json:"max_diff_changes"`
	IncludeContactSheet bool              `json:"include_contact_sheet"`
	ContactSheetColumns uint32            `json:"contact_sheet_columns,omitempty"`
	SourceDiff          []byte            `json:"-"`
	CoreFontProgram     []byte            `json:"-"`
	BeforeFontPrograms  map[string][]byte `json:"-"`
	AfterFontPrograms   map[string][]byte `json:"-"`
	BeforeImages        map[string][]byte `json:"-"`
	AfterImages         map[string][]byte `json:"-"`
}

// DefaultPaperReviewRequest returns bounded interactive defaults. The caller
// still supplies exact font/image bytes required by its plans.
func DefaultPaperReviewRequest() PaperReviewRequest {
	raster := layoutengine.DefaultDisplayRasterLimits()
	bundle := layoutengine.DefaultReviewBundleLimits()
	return PaperReviewRequest{DPI: layoutengine.DefaultDisplayRasterProfile().DPI,
		MaxPixels: raster.MaxPixels, MaxSourceBytes: raster.MaxSourceBytes, MaxPNGBytes: raster.MaxPNGBytes,
		MaxPages: bundle.MaxPages, MaxArtifacts: bundle.MaxArtifacts, MaxArtifactBytes: bundle.MaxArtifactBytes,
		MaxTotalBytes: bundle.MaxTotalBytes, MaxManifestBytes: bundle.MaxManifestBytes, MaxDiffChanges: bundle.MaxDiffChanges,
		IncludeContactSheet: true, ContactSheetColumns: 2}
}

type PaperReviewArtifact struct {
	MetadataJSON []byte `json:"metadata_json"`
	Bytes        []byte `json:"bytes"`
}

type PaperReviewBundle struct {
	BeforePlanHash string                `json:"before_plan_hash"`
	AfterPlanHash  string                `json:"after_plan_hash"`
	ManifestJSON   []byte                `json:"manifest_json"`
	Artifacts      []PaperReviewArtifact `json:"artifacts"`
}

// ReviewAgainst compares candidate against base without parsing, measuring,
// wrapping, paginating, PDF serialization, browser layout, or GUI capture.
func (candidate PaperPlan) ReviewAgainst(ctx context.Context, base PaperPlan, request PaperReviewRequest) (PaperReviewBundle, error) {
	profile := layoutengine.DefaultDisplayRasterProfile()
	profile.DPI = request.DPI
	pageProfile := request.PageProfile
	if pageProfile == "" {
		if inputs, ok := candidate.plan.DeterministicInputs(); ok {
			pageProfile = inputs.PageProfile.ID
		}
	}
	beforeSources := paperReviewSources(base.plan, request.CoreFontProgram, request.BeforeFontPrograms, request.BeforeImages)
	afterSources := paperReviewSources(candidate.plan, request.CoreFontProgram, request.AfterFontPrograms, request.AfterImages)
	bundle, err := layoutengine.BuildReviewBundle(ctx, base.plan, candidate.plan, beforeSources, afterSources, layoutengine.ReviewBundleRequest{
		BeforeRevisions: base.revisions, AfterRevisions: candidate.revisions, PageProfile: pageProfile,
		RasterProfile: profile, RasterLimits: layoutengine.DisplayRasterLimits{MaxPixels: request.MaxPixels,
			MaxSourceBytes: request.MaxSourceBytes, MaxPNGBytes: request.MaxPNGBytes, Paint: layoutengine.DefaultDisplayPaintLimits()},
		Limits: layoutengine.ReviewBundleLimits{MaxPages: request.MaxPages, MaxArtifacts: request.MaxArtifacts,
			MaxArtifactBytes: request.MaxArtifactBytes, MaxTotalBytes: request.MaxTotalBytes,
			MaxManifestBytes: request.MaxManifestBytes, MaxDiffChanges: request.MaxDiffChanges},
		IncludeContactSheet: request.IncludeContactSheet, ContactSheetColumns: request.ContactSheetColumns,
		SourceDiff: append([]byte(nil), request.SourceDiff...),
	})
	if err != nil {
		return PaperReviewBundle{}, err
	}
	manifest, err := bundle.CanonicalJSON()
	if err != nil {
		return PaperReviewBundle{}, err
	}
	artifacts := bundle.Artifacts()
	result := PaperReviewBundle{BeforePlanHash: base.hash, AfterPlanHash: candidate.hash,
		ManifestJSON: append([]byte(nil), manifest...), Artifacts: make([]PaperReviewArtifact, len(artifacts))}
	for index, artifact := range artifacts {
		metadata, marshalErr := json.Marshal(artifact.Metadata)
		if marshalErr != nil {
			return PaperReviewBundle{}, fmt.Errorf("document: encode paper review artifact: %w", marshalErr)
		}
		result.Artifacts[index] = PaperReviewArtifact{MetadataJSON: metadata, Bytes: append([]byte(nil), artifact.Bytes...)}
	}
	return result, nil
}

func paperReviewSources(plan layoutengine.LayoutPlan, defaultFont []byte, fonts, images map[string][]byte) layoutengine.DisplayRasterSources {
	fontPrograms := make(map[layoutengine.CoreFontMetricsDigest][]byte, len(fonts)+len(plan.Projection().Fonts))
	for digest, payload := range fonts {
		fontPrograms[layoutengine.CoreFontMetricsDigest(digest)] = append([]byte(nil), payload...)
	}
	if len(defaultFont) != 0 {
		for _, font := range plan.Projection().Fonts {
			if _, exists := fontPrograms[font.MetricsDigest]; !exists {
				fontPrograms[font.MetricsDigest] = append([]byte(nil), defaultFont...)
			}
		}
	}
	imageSources := make(layoutengine.DisplaySVGImageSources, len(images))
	for digest, payload := range images {
		imageSources[layoutengine.ImageContentDigest(digest)] = append([]byte(nil), payload...)
	}
	return layoutengine.DisplayRasterSources{FontPrograms: fontPrograms, Images: imageSources}
}
