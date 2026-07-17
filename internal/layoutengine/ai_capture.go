// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	// AICaptureManifestFormatVersion pins the canonical manifest envelope.
	AICaptureManifestFormatVersion uint16 = 1
	// AICaptureMaxPages bounds one multi-page capture request.
	AICaptureMaxPages = 64
	// AICaptureMaxTotalArtifactBytes bounds all detached SVG artifacts together.
	AICaptureMaxTotalArtifactBytes uint64 = 16 << 20
	// AICaptureMaxManifestBytes bounds canonical manifest serialization.
	AICaptureMaxManifestBytes = 256 << 10
)

var (
	ErrAICaptureMode  = errors.New("layoutengine: unsupported AI capture mode")
	ErrAICaptureLimit = errors.New("layoutengine: AI capture limit exceeded")
	ErrAICaptureEmpty = errors.New("layoutengine: AI capture plan has no pages")
)

// AICaptureMode selects either the non-disclosing geometry artifact or the
// paint-ready core-text preview artifact for every page in one bundle.
type AICaptureMode string

const (
	AICaptureGeometry AICaptureMode = "geometry_svg"
	AICaptureCoreText AICaptureMode = "core_text_svg"
)

func (mode AICaptureMode) valid() bool {
	return mode == AICaptureGeometry || mode == AICaptureCoreText
}

// AICaptureDisclosure is an explicit artifact disclosure classification.
type AICaptureDisclosure string

const (
	AIDisclosureGeometryOnly     AICaptureDisclosure = "geometry_only"
	AIDisclosureContainsUserText AICaptureDisclosure = "contains_user_text"
)

// AIPageArtifactMetadata is the canonical manifest entry for one page SVG.
// SHA256 is the lowercase digest of the exact detached artifact bytes.
type AIPageArtifactMetadata struct {
	Page             uint32              `json:"page"`
	Name             string              `json:"name"`
	Kind             AICaptureMode       `json:"kind"`
	MediaType        string              `json:"media_type"`
	FormatVersion    uint16              `json:"format_version"`
	PageBounds       Rect                `json:"page_bounds"`
	CaptureBounds    Rect                `json:"capture_bounds"`
	FixedScale       int64               `json:"fixed_scale"`
	Disclosure       AICaptureDisclosure `json:"disclosure"`
	ContainsUserText bool                `json:"contains_user_text"`
	ByteLength       uint64              `json:"byte_length"`
	SHA256           string              `json:"sha256"`
}

// AICaptureManifest is a map-free deterministic description of a bounded
// multi-page capture. It contains digests and metadata, never artifact bytes.
type AICaptureManifest struct {
	FormatVersion     uint16                   `json:"format_version"`
	PlanSchemaVersion uint16                   `json:"plan_schema_version"`
	PlanHash          string                   `json:"plan_hash"`
	Mode              AICaptureMode            `json:"mode"`
	Disclosure        AICaptureDisclosure      `json:"disclosure"`
	ContainsUserText  bool                     `json:"contains_user_text"`
	PageCount         uint32                   `json:"page_count"`
	TotalBytes        uint64                   `json:"total_bytes"`
	Artifacts         []AIPageArtifactMetadata `json:"artifacts"`
}

// CanonicalJSON serializes the fixed-order, map-free manifest and enforces its
// independent output budget.
func (manifest AICaptureManifest) CanonicalJSON() ([]byte, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: marshal AI capture manifest: %w", err)
	}
	if len(encoded) > AICaptureMaxManifestBytes {
		return nil, fmt.Errorf("%w: manifest exceeds %d bytes", ErrAICaptureLimit, AICaptureMaxManifestBytes)
	}
	return encoded, nil
}

// AIPageArtifact holds one detached SVG. Metadata is repeated from the
// manifest so callers never need positional assumptions when storing files.
type AIPageArtifact struct {
	Metadata AIPageArtifactMetadata
	SVG      []byte
}

// AICaptureBundle owns a manifest and its detached per-page SVG artifacts.
// Accessors always return copies.
type AICaptureBundle struct {
	manifest  AICaptureManifest
	artifacts []AIPageArtifact
}

// Manifest returns a detached manifest projection.
func (bundle AICaptureBundle) Manifest() AICaptureManifest {
	manifest := bundle.manifest
	manifest.Artifacts = cloneSlice(bundle.manifest.Artifacts)
	return manifest
}

// Artifacts returns detached metadata and SVG byte slices.
func (bundle AICaptureBundle) Artifacts() []AIPageArtifact {
	if len(bundle.artifacts) == 0 {
		return nil
	}
	artifacts := append([]AIPageArtifact(nil), bundle.artifacts...)
	for index := range artifacts {
		artifacts[index].SVG = append([]byte(nil), bundle.artifacts[index].SVG...)
	}
	return artifacts
}

// CanonicalJSON returns the bundle's detached canonical manifest JSON.
func (bundle AICaptureBundle) CanonicalJSON() ([]byte, error) {
	return bundle.manifest.CanonicalJSON()
}

// CaptureAIPlan builds a deterministic all-page capture using the existing
// single-page geometry or paint-ready core preview API. Any failure returns an
// empty bundle; callers never receive a partially successful manifest.
func CaptureAIPlan(plan LayoutPlan, mode AICaptureMode) (AICaptureBundle, error) {
	if !mode.valid() {
		return AICaptureBundle{}, fmt.Errorf("%w: %q", ErrAICaptureMode, mode)
	}
	if err := plan.Validate(); err != nil {
		return AICaptureBundle{}, fmt.Errorf("layoutengine: capture AI plan: %w", err)
	}
	if len(plan.pages) == 0 {
		return AICaptureBundle{}, ErrAICaptureEmpty
	}
	if len(plan.pages) > AICaptureMaxPages {
		return AICaptureBundle{}, fmt.Errorf("%w: plan has %d pages, maximum is %d", ErrAICaptureLimit, len(plan.pages), AICaptureMaxPages)
	}
	if mode == AICaptureCoreText {
		if err := plan.ValidatePaintReady(); err != nil {
			return AICaptureBundle{}, fmt.Errorf("layoutengine: capture AI core preview: %w", err)
		}
	}

	planHash, err := plan.Hash()
	if err != nil {
		return AICaptureBundle{}, fmt.Errorf("layoutengine: hash AI capture plan: %w", err)
	}
	disclosure := AIDisclosureGeometryOnly
	containsUserText := false
	if mode == AICaptureCoreText {
		disclosure = AIDisclosureContainsUserText
		containsUserText = true
	}
	manifest := AICaptureManifest{
		FormatVersion: AICaptureManifestFormatVersion, PlanSchemaVersion: LayoutPlanSchemaVersion,
		PlanHash: planHash.String(), Mode: mode, Disclosure: disclosure,
		ContainsUserText: containsUserText, PageCount: uint32(len(plan.pages)),
		Artifacts: make([]AIPageArtifactMetadata, 0, len(plan.pages)),
	}
	artifacts := make([]AIPageArtifact, 0, len(plan.pages))
	for pageIndex := range plan.pages {
		pageNumber := uint32(pageIndex + 1)
		metadata, svg, err := captureAIPlanPage(plan, pageNumber, mode, disclosure, containsUserText)
		if err != nil {
			return AICaptureBundle{}, err
		}
		nextTotal, ok := addAICaptureBytes(manifest.TotalBytes, uint64(len(svg)))
		if !ok {
			return AICaptureBundle{}, fmt.Errorf("%w: artifacts exceed %d bytes", ErrAICaptureLimit, AICaptureMaxTotalArtifactBytes)
		}
		manifest.TotalBytes = nextTotal
		manifest.Artifacts = append(manifest.Artifacts, metadata)
		artifacts = append(artifacts, AIPageArtifact{Metadata: metadata, SVG: append([]byte(nil), svg...)})
	}
	if _, err := manifest.CanonicalJSON(); err != nil {
		return AICaptureBundle{}, err
	}
	return AICaptureBundle{manifest: manifest, artifacts: artifacts}, nil
}

func captureAIPlanPage(plan LayoutPlan, page uint32, mode AICaptureMode, disclosure AICaptureDisclosure, containsUserText bool) (AIPageArtifactMetadata, []byte, error) {
	var (
		formatVersion uint16
		pageBounds    Rect
		captureBounds Rect
		svg           []byte
		err           error
	)
	switch mode {
	case AICaptureGeometry:
		capture, captureErr := plan.CaptureDebugGeometrySVGPage(page)
		if captureErr != nil {
			err = captureErr
			break
		}
		formatVersion, pageBounds, captureBounds, svg = capture.FormatVersion, capture.PageBounds, capture.CanvasBounds, capture.SVG
	case AICaptureCoreText:
		capture, captureErr := CaptureCorePlanSVG(plan, page)
		if captureErr != nil {
			err = captureErr
			break
		}
		formatVersion, pageBounds, captureBounds, svg = capture.FormatVersion, capture.PageBounds, capture.PageBounds, capture.SVG
	}
	if err != nil {
		return AIPageArtifactMetadata{}, nil, fmt.Errorf("layoutengine: capture AI page %d: %w", page, err)
	}
	digest := sha256.Sum256(svg)
	metadata := AIPageArtifactMetadata{
		Page: page, Name: fmt.Sprintf("page-%04d.%s.svg", page, mode), Kind: mode,
		MediaType: "image/svg+xml", FormatVersion: formatVersion,
		PageBounds: pageBounds, CaptureBounds: captureBounds, FixedScale: FixedScale,
		Disclosure: disclosure, ContainsUserText: containsUserText,
		ByteLength: uint64(len(svg)), SHA256: hex.EncodeToString(digest[:]),
	}
	return metadata, svg, nil
}

func addAICaptureBytes(total, next uint64) (uint64, bool) {
	if total > AICaptureMaxTotalArtifactBytes || next > AICaptureMaxTotalArtifactBytes-total {
		return 0, false
	}
	return total + next, true
}
