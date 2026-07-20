// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

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
	"image/draw"
	"image/png"
	"sort"
)

const (
	ReviewBundleManifestVersion uint16 = 1
	ReviewBundleRendererVersion        = "layoutengine/review-bundle@1"

	ReviewBundleHardMaxArtifacts     uint32 = 1024
	ReviewBundleHardMaxPages         uint32 = 64
	ReviewBundleHardMaxBytes         uint64 = 128 << 20
	ReviewBundleHardMaxArtifactBytes uint64 = 64 << 20
	ReviewBundleHardMaxManifestBytes uint64 = 2 << 20
)

var (
	ErrReviewBundleRequest = errors.New("layoutengine: invalid review bundle request")
	ErrReviewBundleLimit   = errors.New("layoutengine: review bundle limit exceeded")
)

// ReviewBundleLimits are enforced before a bundle is returned. Bundle
// construction is failure atomic: callers never receive a partial review.
type ReviewBundleLimits struct {
	MaxPages         uint32 `json:"max_pages"`
	MaxArtifacts     uint32 `json:"max_artifacts"`
	MaxArtifactBytes uint64 `json:"max_artifact_bytes"`
	MaxTotalBytes    uint64 `json:"max_total_bytes"`
	MaxManifestBytes uint64 `json:"max_manifest_bytes"`
	MaxDiffChanges   uint64 `json:"max_diff_changes"`
}

func DefaultReviewBundleLimits() ReviewBundleLimits {
	return ReviewBundleLimits{MaxPages: 32, MaxArtifacts: 512, MaxArtifactBytes: 32 << 20,
		MaxTotalBytes: 64 << 20, MaxManifestBytes: 1 << 20, MaxDiffChanges: 100_000}
}

func (limits ReviewBundleLimits) validate() error {
	if limits.MaxPages == 0 || limits.MaxPages > ReviewBundleHardMaxPages ||
		limits.MaxArtifacts == 0 || limits.MaxArtifacts > ReviewBundleHardMaxArtifacts ||
		limits.MaxArtifactBytes == 0 || limits.MaxArtifactBytes > ReviewBundleHardMaxArtifactBytes ||
		limits.MaxTotalBytes == 0 || limits.MaxTotalBytes > ReviewBundleHardMaxBytes ||
		limits.MaxManifestBytes == 0 || limits.MaxManifestBytes > ReviewBundleHardMaxManifestBytes ||
		limits.MaxDiffChanges == 0 || limits.MaxArtifactBytes > limits.MaxTotalBytes {
		return fmt.Errorf("%w: limits must be positive and within published hard caps", ErrReviewBundleRequest)
	}
	return nil
}

// ReviewBundleRequest binds before/after evidence to exact revision,
// scenario, policy, page-profile, renderer, and resource identities.
type ReviewBundleRequest struct {
	BeforeRevisions     ViewerRevisionIdentityInput `json:"before_revisions"`
	AfterRevisions      ViewerRevisionIdentityInput `json:"after_revisions"`
	PageProfile         string                      `json:"page_profile"`
	RasterProfile       DisplayRasterProfile        `json:"raster_profile"`
	RasterLimits        DisplayRasterLimits         `json:"raster_limits"`
	Limits              ReviewBundleLimits          `json:"limits"`
	IncludeContactSheet bool                        `json:"include_contact_sheet"`
	ContactSheetColumns uint32                      `json:"contact_sheet_columns"`
	SourceDiff          []byte                      `json:"-"`
}

type ReviewArtifactKind string

const (
	ReviewArtifactCleanPage         ReviewArtifactKind = "clean_page"
	ReviewArtifactOverlayPage       ReviewArtifactKind = "overlay_page"
	ReviewArtifactBeforeCrop        ReviewArtifactKind = "before_crop"
	ReviewArtifactAfterCrop         ReviewArtifactKind = "after_crop"
	ReviewArtifactRasterDiff        ReviewArtifactKind = "raster_diff"
	ReviewArtifactContactSheet      ReviewArtifactKind = "contact_sheet"
	ReviewArtifactSourceDiff        ReviewArtifactKind = "source_diff"
	ReviewArtifactSemanticDiff      ReviewArtifactKind = "semantic_diff"
	ReviewArtifactPlanDiff          ReviewArtifactKind = "plan_diff"
	ReviewArtifactAccessibilityDiff ReviewArtifactKind = "accessibility_diff"
	ReviewArtifactDiagnostics       ReviewArtifactKind = "diagnostics"
)

type ReviewArtifactLayer string

const (
	ReviewLayerClean    ReviewArtifactLayer = "clean"
	ReviewLayerOverlay  ReviewArtifactLayer = "overlay"
	ReviewLayerEvidence ReviewArtifactLayer = "evidence"
)

type ReviewFragmentContext struct {
	Node       NodeID             `json:"node"`
	Key        NodeKey            `json:"key"`
	Instance   InstanceID         `json:"instance"`
	Fragment   FragmentID         `json:"fragment"`
	Page       uint32             `json:"page"`
	Bounds     Rect               `json:"bounds"`
	Source     SourceSpan         `json:"source"`
	Semantic   SemanticNodeID     `json:"semantic,omitempty"`
	Role       SemanticRole       `json:"role,omitempty"`
	Attributes SemanticAttributes `json:"attributes,omitzero"`
}

type ReviewPagePixelTransform struct {
	Page        uint32                 `json:"page"`
	Bounds      Rect                   `json:"bounds"`
	PixelX      uint32                 `json:"pixel_x"`
	PixelY      uint32                 `json:"pixel_y"`
	PixelWidth  uint32                 `json:"pixel_width"`
	PixelHeight uint32                 `json:"pixel_height"`
	Transform   DisplayRasterTransform `json:"transform"`
}

type ReviewArtifactMetadata struct {
	Index          uint32                     `json:"index"`
	Name           string                     `json:"name"`
	Kind           ReviewArtifactKind         `json:"kind"`
	Layer          ReviewArtifactLayer        `json:"layer"`
	MediaType      string                     `json:"media_type"`
	Page           uint32                     `json:"page,omitempty"`
	BeforePlanHash string                     `json:"before_plan_hash,omitempty"`
	AfterPlanHash  string                     `json:"after_plan_hash,omitempty"`
	CropBounds     Rect                       `json:"crop_bounds"`
	PixelWidth     uint32                     `json:"pixel_width,omitempty"`
	PixelHeight    uint32                     `json:"pixel_height,omitempty"`
	ChangedPixels  uint64                     `json:"changed_pixels,omitempty"`
	PixelTransform *DisplayRasterTransform    `json:"pixel_transform,omitempty"`
	PageTransforms []ReviewPagePixelTransform `json:"page_transforms,omitempty"`
	Fragments      []ReviewFragmentContext    `json:"fragments,omitempty"`
	ByteLength     uint64                     `json:"byte_length"`
	SHA256         string                     `json:"sha256"`
}

type ReviewSemanticRoleCount struct {
	Role  SemanticRole `json:"role"`
	Count uint64       `json:"count"`
}

type ReviewSemanticSnapshot struct {
	NodeCount        uint64                    `json:"node_count"`
	AssociationCount uint64                    `json:"association_count"`
	ReadingCount     uint64                    `json:"reading_count"`
	Roles            []ReviewSemanticRoleCount `json:"roles,omitempty"`
	ReadingSHA256    string                    `json:"reading_sha256"`
	SHA256           string                    `json:"sha256"`
}

type ReviewAccessibilitySnapshot struct {
	SemanticNodeCount   uint64 `json:"semantic_node_count"`
	ReadingCount        uint64 `json:"reading_count"`
	FigureCount         uint64 `json:"figure_count"`
	MissingAltCount     uint64 `json:"missing_alt_count"`
	HeadingCount        uint64 `json:"heading_count"`
	InvalidHeadingCount uint64 `json:"invalid_heading_count"`
	LanguageCount       uint64 `json:"language_count"`
	SHA256              string `json:"sha256"`
}

type ReviewSemanticDiff struct {
	Equal               bool                   `json:"equal"`
	ReadingOrderChanged bool                   `json:"reading_order_changed"`
	RolesChanged        bool                   `json:"roles_changed"`
	Before              ReviewSemanticSnapshot `json:"before"`
	After               ReviewSemanticSnapshot `json:"after"`
}

type ReviewAccessibilityDiff struct {
	Equal  bool                        `json:"equal"`
	Before ReviewAccessibilitySnapshot `json:"before"`
	After  ReviewAccessibilitySnapshot `json:"after"`
}

type ReviewBundleManifest struct {
	FormatVersion          uint16                      `json:"format_version"`
	PlanSchemaVersion      uint16                      `json:"plan_schema_version"`
	RendererVersion        string                      `json:"renderer_version"`
	BeforePlanHash         string                      `json:"before_plan_hash"`
	AfterPlanHash          string                      `json:"after_plan_hash"`
	BeforeIdentity         ViewerIdentity              `json:"before_identity"`
	AfterIdentity          ViewerIdentity              `json:"after_identity"`
	BeforeScenarioRevision string                      `json:"before_scenario_revision"`
	AfterScenarioRevision  string                      `json:"after_scenario_revision"`
	BeforePolicyRevision   string                      `json:"before_policy_revision"`
	AfterPolicyRevision    string                      `json:"after_policy_revision"`
	PageProfile            string                      `json:"page_profile"`
	RasterProfile          DisplayRasterProfile        `json:"raster_profile"`
	Limits                 ReviewBundleLimits          `json:"limits"`
	BeforeSemantics        ReviewSemanticSnapshot      `json:"before_semantics"`
	AfterSemantics         ReviewSemanticSnapshot      `json:"after_semantics"`
	BeforeAccessibility    ReviewAccessibilitySnapshot `json:"before_accessibility"`
	AfterAccessibility     ReviewAccessibilitySnapshot `json:"after_accessibility"`
	ChangedPages           []uint32                    `json:"changed_pages"`
	ArtifactCount          uint32                      `json:"artifact_count"`
	TotalBytes             uint64                      `json:"total_bytes"`
	Artifacts              []ReviewArtifactMetadata    `json:"artifacts"`
}

func (manifest ReviewBundleManifest) CanonicalJSON() ([]byte, error) {
	if err := manifest.Limits.validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: marshal review manifest: %w", err)
	}
	if uint64(len(encoded)) > manifest.Limits.MaxManifestBytes {
		return nil, fmt.Errorf("%w: manifest exceeds %d bytes", ErrReviewBundleLimit, manifest.Limits.MaxManifestBytes)
	}
	return encoded, nil
}

type ReviewArtifact struct {
	Metadata ReviewArtifactMetadata
	Bytes    []byte
}

type ReviewBundle struct {
	manifest  ReviewBundleManifest
	artifacts []ReviewArtifact
}

type reviewPagePair struct {
	before DisplayRasterArtifact
	after  DisplayRasterArtifact
}

func (bundle ReviewBundle) Manifest() ReviewBundleManifest {
	return cloneReviewManifest(bundle.manifest)
}
func (bundle ReviewBundle) Artifacts() []ReviewArtifact {
	result := make([]ReviewArtifact, len(bundle.artifacts))
	for i := range bundle.artifacts {
		result[i] = ReviewArtifact{Metadata: cloneReviewMetadata(bundle.artifacts[i].Metadata), Bytes: append([]byte(nil), bundle.artifacts[i].Bytes...)}
	}
	return result
}
func (bundle ReviewBundle) CanonicalJSON() ([]byte, error) { return bundle.manifest.CanonicalJSON() }

func cloneReviewManifest(value ReviewBundleManifest) ReviewBundleManifest {
	value.ChangedPages = append([]uint32(nil), value.ChangedPages...)
	artifacts := value.Artifacts
	value.Artifacts = make([]ReviewArtifactMetadata, len(artifacts))
	for i := range artifacts {
		value.Artifacts[i] = cloneReviewMetadata(artifacts[i])
	}
	value.BeforeSemantics.Roles = append([]ReviewSemanticRoleCount(nil), value.BeforeSemantics.Roles...)
	value.AfterSemantics.Roles = append([]ReviewSemanticRoleCount(nil), value.AfterSemantics.Roles...)
	return value
}

func cloneReviewMetadata(value ReviewArtifactMetadata) ReviewArtifactMetadata {
	if value.PixelTransform != nil {
		copy := *value.PixelTransform
		value.PixelTransform = &copy
	}
	value.PageTransforms = append([]ReviewPagePixelTransform(nil), value.PageTransforms...)
	value.Fragments = append([]ReviewFragmentContext(nil), value.Fragments...)
	return value
}

// BuildReviewBundle creates a deterministic, content-addressed before/after
// evidence bundle directly from retained display lists. Clean and overlay
// images are always separate artifacts.
func BuildReviewBundle(ctx context.Context, before, after LayoutPlan, beforeSources, afterSources DisplayRasterSources, request ReviewBundleRequest) (ReviewBundle, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ReviewBundle{}, err
	}
	if err := request.Limits.validate(); err != nil {
		return ReviewBundle{}, err
	}
	if err := request.RasterProfile.validate(); err != nil {
		return ReviewBundle{}, err
	}
	if err := request.RasterLimits.validate(); err != nil {
		return ReviewBundle{}, err
	}
	if err := request.BeforeRevisions.validate(); err != nil {
		return ReviewBundle{}, err
	}
	if err := request.AfterRevisions.validate(); err != nil {
		return ReviewBundle{}, err
	}
	if request.BeforeRevisions.ScenarioRevision == "" || request.AfterRevisions.ScenarioRevision == "" {
		return ReviewBundle{}, fmt.Errorf("%w: review requires explicit scenario and policy revisions", ErrReviewBundleRequest)
	}
	if err := validateDigestString(request.PageProfile); err != nil {
		return ReviewBundle{}, fmt.Errorf("%w: page profile: %w", ErrReviewBundleRequest, err)
	}
	if request.IncludeContactSheet && request.ContactSheetColumns == 0 {
		return ReviewBundle{}, fmt.Errorf("%w: contact-sheet columns must be positive", ErrReviewBundleRequest)
	}
	if err := before.Validate(); err != nil {
		return ReviewBundle{}, fmt.Errorf("layoutengine: invalid review base plan: %w", err)
	}
	if err := after.Validate(); err != nil {
		return ReviewBundle{}, fmt.Errorf("layoutengine: invalid review candidate plan: %w", err)
	}

	diff, err := DiffLayoutPlansWithLimits(before, after, PlanDiffLimits{MaxPageChanges: request.Limits.MaxDiffChanges, MaxFragmentChanges: request.Limits.MaxDiffChanges})
	if err != nil {
		return ReviewBundle{}, err
	}
	if diff.PageChangesTruncated || diff.FragmentChangesTruncated {
		return ReviewBundle{}, fmt.Errorf("%w: structural change evidence was truncated", ErrReviewBundleLimit)
	}
	beforeHash, _ := before.Hash()
	afterHash, _ := after.Hash()
	beforeIdentity, err := viewerIdentityForPlan(before, ReviewBundleRendererVersion, request.BeforeRevisions)
	if err != nil {
		return ReviewBundle{}, err
	}
	afterIdentity, err := viewerIdentityForPlan(after, ReviewBundleRendererVersion, request.AfterRevisions)
	if err != nil {
		return ReviewBundle{}, err
	}
	changedPages := reviewChangedPages(diff)
	if uint32(len(changedPages)) > request.Limits.MaxPages { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return ReviewBundle{}, fmt.Errorf("%w: %d changed pages", ErrReviewBundleLimit, len(changedPages))
	}
	estimatedArtifacts := uint64(4) // semantic, plan, accessibility, diagnostics
	if len(request.SourceDiff) != 0 {
		estimatedArtifacts++
	}
	for _, page := range changedPages {
		hasBefore, hasAfter := int(page) <= len(before.pages), int(page) <= len(after.pages)
		if hasBefore {
			estimatedArtifacts++
		}
		if hasAfter {
			estimatedArtifacts += 2
		} // clean candidate and separate overlay
		if hasBefore && hasAfter {
			estimatedArtifacts++
		}
	}
	for _, change := range diff.FragmentChanges {
		if change.Before != nil {
			estimatedArtifacts++
		}
		if change.After != nil {
			estimatedArtifacts++
		}
	}
	if request.IncludeContactSheet && len(changedPages) != 0 {
		estimatedArtifacts++
	}
	if estimatedArtifacts > uint64(request.Limits.MaxArtifacts) {
		return ReviewBundle{}, fmt.Errorf("%w: %d artifacts exceed limit %d", ErrReviewBundleLimit, estimatedArtifacts, request.Limits.MaxArtifacts)
	}
	beforeSemantic, beforeAccess, err := reviewSemanticEvidence(before)
	if err != nil {
		return ReviewBundle{}, err
	}
	afterSemantic, afterAccess, err := reviewSemanticEvidence(after)
	if err != nil {
		return ReviewBundle{}, err
	}
	manifest := ReviewBundleManifest{FormatVersion: ReviewBundleManifestVersion, PlanSchemaVersion: LayoutPlanSchemaVersion,
		RendererVersion: ReviewBundleRendererVersion, BeforePlanHash: beforeHash.String(), AfterPlanHash: afterHash.String(),
		BeforeIdentity: beforeIdentity, AfterIdentity: afterIdentity,
		BeforeScenarioRevision: request.BeforeRevisions.ScenarioRevision, AfterScenarioRevision: request.AfterRevisions.ScenarioRevision,
		BeforePolicyRevision: request.BeforeRevisions.PolicyRevision, AfterPolicyRevision: request.AfterRevisions.PolicyRevision,
		PageProfile: request.PageProfile, RasterProfile: request.RasterProfile,
		Limits: request.Limits, BeforeSemantics: beforeSemantic, AfterSemantics: afterSemantic,
		BeforeAccessibility: beforeAccess, AfterAccessibility: afterAccess, ChangedPages: changedPages}
	artifacts := make([]ReviewArtifact, 0)
	appendJSON := func(name string, kind ReviewArtifactKind, value any) error {
		payload, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return marshalErr
		}
		return appendReviewArtifact(&manifest, &artifacts, ReviewArtifactMetadata{Name: name, Kind: kind, Layer: ReviewLayerEvidence, MediaType: "application/json", BeforePlanHash: beforeHash.String(), AfterPlanHash: afterHash.String()}, payload)
	}
	if len(request.SourceDiff) != 0 {
		if err := appendReviewArtifact(&manifest, &artifacts, ReviewArtifactMetadata{Name: "source.diff", Kind: ReviewArtifactSourceDiff, Layer: ReviewLayerEvidence, MediaType: "text/x-diff", BeforePlanHash: beforeHash.String(), AfterPlanHash: afterHash.String()}, request.SourceDiff); err != nil {
			return ReviewBundle{}, err
		}
	}
	semanticDiff := ReviewSemanticDiff{Equal: beforeSemantic.SHA256 == afterSemantic.SHA256,
		ReadingOrderChanged: beforeSemantic.ReadingSHA256 != afterSemantic.ReadingSHA256,
		RolesChanged:        !jsonEqual(beforeSemantic.Roles, afterSemantic.Roles), Before: beforeSemantic, After: afterSemantic}
	if err := appendJSON("semantic-diff.json", ReviewArtifactSemanticDiff, semanticDiff); err != nil {
		return ReviewBundle{}, err
	}
	if err := appendJSON("plan-diff.json", ReviewArtifactPlanDiff, diff); err != nil {
		return ReviewBundle{}, err
	}
	accessibilityDiff := ReviewAccessibilityDiff{Equal: beforeAccess.SHA256 == afterAccess.SHA256, Before: beforeAccess, After: afterAccess}
	if err := appendJSON("accessibility-diff.json", ReviewArtifactAccessibilityDiff, accessibilityDiff); err != nil {
		return ReviewBundle{}, err
	}
	if err := appendJSON("diagnostics.json", ReviewArtifactDiagnostics, reviewDiagnostics(before, after)); err != nil {
		return ReviewBundle{}, err
	}

	pageRasters := make(map[uint32]reviewPagePair, len(changedPages))
	for _, page := range changedPages {
		if err := ctx.Err(); err != nil {
			return ReviewBundle{}, err
		}
		var pair reviewPagePair
		if int(page) <= len(before.pages) {
			pair.before, err = CaptureDisplayPlanPNGContext(ctx, before, beforeSources, reviewRasterRequest(page, nil, request.BeforeRevisions, request))
			if err != nil {
				return ReviewBundle{}, fmt.Errorf("layoutengine: review before page %d: %w", page, err)
			}
			metadata := reviewRasterMetadata(fmt.Sprintf("changed-page-%04d-before.png", page), ReviewArtifactCleanPage, ReviewLayerClean, beforeHash.String(), pair.before)
			metadata.BeforePlanHash, metadata.AfterPlanHash = beforeHash.String(), ""
			metadata.Page = page
			metadata.Fragments = reviewPageContexts(before, page)
			if err := appendReviewArtifact(&manifest, &artifacts, metadata, pair.before.PNG()); err != nil {
				return ReviewBundle{}, err
			}
		}
		if int(page) <= len(after.pages) {
			pair.after, err = CaptureDisplayPlanPNGContext(ctx, after, afterSources, reviewRasterRequest(page, nil, request.AfterRevisions, request))
			if err != nil {
				return ReviewBundle{}, fmt.Errorf("layoutengine: review after page %d: %w", page, err)
			}
			metadata := reviewRasterMetadata(fmt.Sprintf("changed-page-%04d-after.png", page), ReviewArtifactCleanPage, ReviewLayerClean, afterHash.String(), pair.after)
			metadata.Page = page
			metadata.Fragments = reviewPageContexts(after, page)
			if err := appendReviewArtifact(&manifest, &artifacts, metadata, pair.after.PNG()); err != nil {
				return ReviewBundle{}, err
			}
			overlay, overlayMeta, overlayErr := reviewOverlayPNG(after, page, diff, pair.after.Manifest())
			if overlayErr != nil {
				return ReviewBundle{}, overlayErr
			}
			overlayMeta.BeforePlanHash, overlayMeta.AfterPlanHash = beforeHash.String(), afterHash.String()
			if err := appendReviewArtifact(&manifest, &artifacts, overlayMeta, overlay); err != nil {
				return ReviewBundle{}, err
			}
		}
		if len(pair.before.PNG()) != 0 && len(pair.after.PNG()) != 0 {
			heatmap, width, height, changedPixels, diffErr := reviewPixelDiff(ctx, pair.before.PNG(), pair.after.PNG())
			if diffErr != nil {
				return ReviewBundle{}, diffErr
			}
			meta := ReviewArtifactMetadata{Name: fmt.Sprintf("raster-diff-page-%04d.png", page), Kind: ReviewArtifactRasterDiff, Layer: ReviewLayerEvidence, MediaType: "image/png", Page: page, BeforePlanHash: beforeHash.String(), AfterPlanHash: afterHash.String(), PixelWidth: width, PixelHeight: height, ChangedPixels: changedPixels}
			if err := appendReviewArtifact(&manifest, &artifacts, meta, heatmap); err != nil {
				return ReviewBundle{}, err
			}
		}
		pageRasters[page] = pair
	}
	for index, change := range diff.FragmentChanges {
		if err := ctx.Err(); err != nil {
			return ReviewBundle{}, err
		}
		name := fmt.Sprintf("changed-fragment-%04d", index+1)
		if change.Before != nil {
			crop := change.Before.BorderBox
			artifact, captureErr := CaptureDisplayPlanPNGContext(ctx, before, beforeSources, reviewRasterRequest(change.Before.Page, &crop, request.BeforeRevisions, request))
			if captureErr != nil {
				return ReviewBundle{}, captureErr
			}
			meta := reviewRasterMetadata(name+"-before.png", ReviewArtifactBeforeCrop, ReviewLayerClean, beforeHash.String(), artifact)
			meta.BeforePlanHash, meta.AfterPlanHash = beforeHash.String(), ""
			meta.Fragments = []ReviewFragmentContext{reviewFragmentContext(before, *change.Before)}
			if err := appendReviewArtifact(&manifest, &artifacts, meta, artifact.PNG()); err != nil {
				return ReviewBundle{}, err
			}
		}
		if change.After != nil {
			crop := change.After.BorderBox
			artifact, captureErr := CaptureDisplayPlanPNGContext(ctx, after, afterSources, reviewRasterRequest(change.After.Page, &crop, request.AfterRevisions, request))
			if captureErr != nil {
				return ReviewBundle{}, captureErr
			}
			meta := reviewRasterMetadata(name+"-after.png", ReviewArtifactAfterCrop, ReviewLayerClean, afterHash.String(), artifact)
			meta.Fragments = []ReviewFragmentContext{reviewFragmentContext(after, *change.After)}
			if err := appendReviewArtifact(&manifest, &artifacts, meta, artifact.PNG()); err != nil {
				return ReviewBundle{}, err
			}
		}
	}
	if request.IncludeContactSheet && len(changedPages) != 0 {
		pngBytes, metadata, sheetErr := reviewContactSheet(ctx, changedPages, pageRasters, request.ContactSheetColumns, request.RasterLimits.MaxPixels)
		if sheetErr != nil {
			return ReviewBundle{}, sheetErr
		}
		metadata.BeforePlanHash, metadata.AfterPlanHash = beforeHash.String(), afterHash.String()
		if err := appendReviewArtifact(&manifest, &artifacts, metadata, pngBytes); err != nil {
			return ReviewBundle{}, err
		}
	}
	manifest.ArtifactCount = uint32(len(artifacts)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if _, err := manifest.CanonicalJSON(); err != nil {
		return ReviewBundle{}, err
	}
	return ReviewBundle{manifest: manifest, artifacts: artifacts}, nil
}

func reviewRasterRequest(page uint32, crop *Rect, revisions ViewerRevisionIdentityInput, request ReviewBundleRequest) DisplayRasterRequest {
	return DisplayRasterRequest{Page: page, Crop: crop, Profile: request.RasterProfile, Limits: request.RasterLimits, Revisions: revisions, PageProfile: request.PageProfile}
}

func appendReviewArtifact(manifest *ReviewBundleManifest, artifacts *[]ReviewArtifact, metadata ReviewArtifactMetadata, payload []byte) error {
	if uint32(len(*artifacts)) >= manifest.Limits.MaxArtifacts { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return fmt.Errorf("%w: artifact count", ErrReviewBundleLimit)
	}
	if uint64(len(payload)) > manifest.Limits.MaxArtifactBytes {
		return fmt.Errorf("%w: artifact %q", ErrReviewBundleLimit, metadata.Name)
	}
	next, ok := addBoundedBytes(manifest.TotalBytes, uint64(len(payload)), manifest.Limits.MaxTotalBytes)
	if !ok {
		return fmt.Errorf("%w: total bytes", ErrReviewBundleLimit)
	}
	metadata.Index = uint32(len(*artifacts)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	metadata.ByteLength = uint64(len(payload))
	digest := sha256.Sum256(payload)
	metadata.SHA256 = hex.EncodeToString(digest[:])
	manifest.TotalBytes = next
	manifest.Artifacts = append(manifest.Artifacts, cloneReviewMetadata(metadata))
	*artifacts = append(*artifacts, ReviewArtifact{Metadata: cloneReviewMetadata(metadata), Bytes: append([]byte(nil), payload...)})
	return nil
}

func reviewChangedPages(diff LayoutPlanDiff) []uint32 {
	set := make(map[uint32]bool)
	for _, change := range diff.PageChanges {
		set[change.Page] = true
	}
	for _, change := range diff.FragmentChanges {
		if change.Before != nil {
			set[change.Before.Page] = true
		}
		if change.After != nil {
			set[change.After.Page] = true
		}
	}
	// Display-list/resource changes do not necessarily alter the page or
	// fragment structs. Conservatively review every represented page rather
	// than allowing a paint-only change to escape visual evidence.
	if diff.DisplayListChanged || diff.FontCatalogChanged || diff.ImageCatalogChanged {
		count := diff.Pages.Before
		if diff.Pages.After > count {
			count = diff.Pages.After
		}
		for page := uint64(1); page <= count; page++ {
			set[uint32(page)] = true
		}
	}
	if !diff.Equal && len(set) == 0 {
		count := diff.Pages.Before
		if diff.Pages.After > count {
			count = diff.Pages.After
		}
		for page := uint64(1); page <= count; page++ {
			set[uint32(page)] = true
		}
	}
	pages := make([]uint32, 0, len(set))
	for page := range set {
		pages = append(pages, page)
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i] < pages[j] })
	return pages
}

func reviewRasterMetadata(name string, kind ReviewArtifactKind, layer ReviewArtifactLayer, planHash string, artifact DisplayRasterArtifact) ReviewArtifactMetadata {
	m := artifact.Manifest()
	transform := m.PixelTransform
	return ReviewArtifactMetadata{Name: name, Kind: kind, Layer: layer, MediaType: "image/png", Page: m.Page, AfterPlanHash: planHash, CropBounds: m.CaptureBounds, PixelWidth: m.PixelWidth, PixelHeight: m.PixelHeight, PixelTransform: &transform}
}

func reviewSemanticEvidence(plan LayoutPlan) (ReviewSemanticSnapshot, ReviewAccessibilitySnapshot, error) {
	p := plan.Projection()
	counts := make(map[SemanticRole]uint64)
	access := ReviewAccessibilitySnapshot{SemanticNodeCount: uint64(len(p.SemanticNodes)), ReadingCount: uint64(len(p.ReadingOrder))}
	languages := make(map[string]bool)
	for _, node := range p.SemanticNodes {
		counts[node.Role]++
		if node.Attributes.Language != "" {
			languages[node.Attributes.Language] = true
		}
		switch node.Role {
		case SemanticRoleFigure:
			access.FigureCount++
			if node.Attributes.AlternateText == "" {
				access.MissingAltCount++
			}
		case SemanticRoleHeading:
			access.HeadingCount++
			if node.Attributes.HeadingLevel == 0 {
				access.InvalidHeadingCount++
			}
		}
	}
	roles := make([]ReviewSemanticRoleCount, 0, len(counts))
	for role, count := range counts {
		roles = append(roles, ReviewSemanticRoleCount{role, count})
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].Role < roles[j].Role })
	semanticPayload, err := json.Marshal(struct {
		Nodes        []SemanticNode                `json:"nodes"`
		Associations []SemanticFragmentAssociation `json:"associations"`
		Reading      []ReadingOccurrence           `json:"reading"`
	}{p.SemanticNodes, p.SemanticFragments, p.ReadingOrder})
	if err != nil {
		return ReviewSemanticSnapshot{}, ReviewAccessibilitySnapshot{}, err
	}
	semanticHash := sha256.Sum256(semanticPayload)
	readingPayload, err := json.Marshal(p.ReadingOrder)
	if err != nil {
		return ReviewSemanticSnapshot{}, ReviewAccessibilitySnapshot{}, err
	}
	readingHash := sha256.Sum256(readingPayload)
	semantic := ReviewSemanticSnapshot{NodeCount: uint64(len(p.SemanticNodes)), AssociationCount: uint64(len(p.SemanticFragments)), ReadingCount: uint64(len(p.ReadingOrder)), Roles: roles, ReadingSHA256: hex.EncodeToString(readingHash[:]), SHA256: hex.EncodeToString(semanticHash[:])}
	access.LanguageCount = uint64(len(languages))
	accessPayload, err := json.Marshal(access)
	if err != nil {
		return ReviewSemanticSnapshot{}, ReviewAccessibilitySnapshot{}, err
	}
	accessHash := sha256.Sum256(accessPayload)
	access.SHA256 = hex.EncodeToString(accessHash[:])
	return semantic, access, nil
}

func reviewFragmentContext(plan LayoutPlan, fragment Fragment) ReviewFragmentContext {
	result := ReviewFragmentContext{Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Fragment: fragment.ID, Page: fragment.Page, Bounds: fragment.BorderBox, Source: fragment.Source}
	p := plan.Projection()
	for _, association := range p.SemanticFragments {
		if association.Fragment != fragment.ID {
			continue
		}
		result.Semantic = association.Semantic
		if int(association.Semantic) <= len(p.SemanticNodes) {
			node := p.SemanticNodes[association.Semantic-1]
			result.Role = node.Role
			result.Attributes = node.Attributes
		}
		break
	}
	return result
}

func reviewPageContexts(plan LayoutPlan, page uint32) []ReviewFragmentContext {
	p := plan.Projection()
	result := make([]ReviewFragmentContext, 0)
	for _, fragment := range p.Fragments {
		if fragment.Page == page {
			result = append(result, reviewFragmentContext(plan, fragment))
		}
	}
	return result
}

func reviewDiagnostics(before, after LayoutPlan) any {
	b := before.Projection()
	a := after.Projection()
	return struct{ Before, After []Diagnostic }{b.Diagnostics, a.Diagnostics}
}

func reviewOverlayPNG(plan LayoutPlan, page uint32, diff LayoutPlanDiff, raster DisplayRasterManifest) ([]byte, ReviewArtifactMetadata, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, int(raster.PixelWidth), int(raster.PixelHeight)))
	contexts := make([]ReviewFragmentContext, 0)
	for _, change := range diff.FragmentChanges {
		if change.After == nil || change.After.Page != page {
			continue
		}
		fragment := *change.After
		contexts = append(contexts, reviewFragmentContext(plan, fragment))
		rect := reviewPixelRect(fragment.BorderBox, raster)
		if rect.Empty() {
			continue
		}
		reviewStrokeRect(canvas, rect, color.RGBA{R: 255, G: 40, B: 40, A: 220}, 2)
	}
	payload, err := encodeReviewPNG(canvas)
	if err != nil {
		return nil, ReviewArtifactMetadata{}, err
	}
	transform := raster.PixelTransform
	return payload, ReviewArtifactMetadata{Name: fmt.Sprintf("changed-page-%04d-overlay.png", page), Kind: ReviewArtifactOverlayPage, Layer: ReviewLayerOverlay, MediaType: "image/png", Page: page, CropBounds: raster.CaptureBounds, PixelWidth: raster.PixelWidth, PixelHeight: raster.PixelHeight, PixelTransform: &transform, Fragments: contexts}, nil
}

func reviewPixelRect(bounds Rect, raster DisplayRasterManifest) image.Rectangle {
	scale := func(value, origin Fixed, n uint32, d Fixed) int {
		delta := int64(value - origin)
		return int((delta*int64(n) + int64(d)/2) / int64(d))
	}
	x0 := scale(bounds.X, raster.PixelTransform.OriginX, raster.PixelTransform.XNumerator, raster.PixelTransform.XDenominator)
	y0 := scale(bounds.Y, raster.PixelTransform.OriginY, raster.PixelTransform.YNumerator, raster.PixelTransform.YDenominator)
	x1 := scale(bounds.X+bounds.Width, raster.PixelTransform.OriginX, raster.PixelTransform.XNumerator, raster.PixelTransform.XDenominator)
	y1 := scale(bounds.Y+bounds.Height, raster.PixelTransform.OriginY, raster.PixelTransform.YNumerator, raster.PixelTransform.YDenominator)
	return image.Rect(x0, y0, x1, y1).Intersect(image.Rect(0, 0, int(raster.PixelWidth), int(raster.PixelHeight)))
}

func reviewStrokeRect(dst *image.RGBA, rect image.Rectangle, ink color.RGBA, width int) {
	for i := 0; i < width; i++ {
		r := rect.Inset(-i)
		if r.Empty() {
			continue
		}
		draw.Draw(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), image.NewUniform(ink), image.Point{}, draw.Over)
		draw.Draw(dst, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), image.NewUniform(ink), image.Point{}, draw.Over)
		draw.Draw(dst, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), image.NewUniform(ink), image.Point{}, draw.Over)
		draw.Draw(dst, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), image.NewUniform(ink), image.Point{}, draw.Over)
	}
}

func reviewPixelDiff(ctx context.Context, beforePNG, afterPNG []byte) ([]byte, uint32, uint32, uint64, error) {
	before, err := png.Decode(bytes.NewReader(beforePNG))
	if err != nil {
		return nil, 0, 0, 0, err
	}
	after, err := png.Decode(bytes.NewReader(afterPNG))
	if err != nil {
		return nil, 0, 0, 0, err
	}
	w := before.Bounds().Dx()
	if after.Bounds().Dx() > w {
		w = after.Bounds().Dx()
	}
	h := before.Bounds().Dy()
	if after.Bounds().Dy() > h {
		h = after.Bounds().Dy()
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	white := color.RGBA{255, 255, 255, 255}
	var changed uint64
	for y := 0; y < h; y++ {
		if y&127 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, 0, 0, 0, err
			}
		}
		for x := 0; x < w; x++ {
			bc, ac := white, white
			if image.Pt(x, y).In(before.Bounds()) {
				bc = color.RGBAModel.Convert(before.At(x, y)).(color.RGBA)
			}
			if image.Pt(x, y).In(after.Bounds()) {
				ac = color.RGBAModel.Convert(after.At(x, y)).(color.RGBA)
			}
			d := max8(abs8(bc.R, ac.R), max8(abs8(bc.G, ac.G), abs8(bc.B, ac.B)))
			if d != 0 {
				changed++
				dst.SetRGBA(x, y, color.RGBA{R: 255, G: 255 - d/2, B: 0, A: 255})
			} else {
				dst.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}
	payload, err := encodeReviewPNG(dst)
	return payload, uint32(w), uint32(h), changed, err // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
}
func abs8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
func max8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}

func reviewContactSheet(ctx context.Context, pages []uint32, rasters map[uint32]reviewPagePair, columns uint32, maxPixels uint64) ([]byte, ReviewArtifactMetadata, error) {
	if len(pages) == 0 || columns == 0 {
		return nil, ReviewArtifactMetadata{}, fmt.Errorf("%w: empty contact sheet", ErrReviewBundleRequest)
	}
	if columns > uint32(len(pages)) { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		columns = uint32(len(pages)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	rows := (uint32(len(pages)) + columns - 1) / columns // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	columnWidths := make([]int, columns)
	rowHeights := make([]int, rows)
	decoded := make([]image.Image, len(pages))
	for index, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, ReviewArtifactMetadata{}, err
		}
		payload := rasters[page].after.PNG()
		if len(payload) == 0 {
			continue
		}
		img, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			return nil, ReviewArtifactMetadata{}, fmt.Errorf("layoutengine: decode contact-sheet page %d: %w", page, err)
		}
		decoded[index] = img
		column, row := index%int(columns), index/int(columns)
		if img.Bounds().Dx() > columnWidths[column] {
			columnWidths[column] = img.Bounds().Dx()
		}
		if img.Bounds().Dy() > rowHeights[row] {
			rowHeights[row] = img.Bounds().Dy()
		}
	}
	const gap = 8
	width, height := gap*(int(columns)-1), gap*(int(rows)-1)
	for _, extent := range columnWidths {
		width += extent
	}
	for _, extent := range rowHeights {
		height += extent
	}
	if width <= 0 || height <= 0 || uint64(width) > maxPixels/uint64(height) {
		return nil, ReviewArtifactMetadata{}, fmt.Errorf("%w: contact-sheet pixels", ErrReviewBundleLimit)
	}
	columnX, rowY := make([]int, columns), make([]int, rows)
	for index := 1; index < len(columnX); index++ {
		columnX[index] = columnX[index-1] + columnWidths[index-1] + gap
	}
	for index := 1; index < len(rowY); index++ {
		rowY[index] = rowY[index-1] + rowHeights[index-1] + gap
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	transforms := make([]ReviewPagePixelTransform, 0, len(pages))
	for index, page := range pages {
		if err := ctx.Err(); err != nil {
			return nil, ReviewArtifactMetadata{}, err
		}
		img := decoded[index]
		if img == nil {
			continue
		}
		x, y := columnX[index%int(columns)], rowY[index/int(columns)]
		draw.Draw(canvas, image.Rect(x, y, x+img.Bounds().Dx(), y+img.Bounds().Dy()), img, img.Bounds().Min, draw.Src)
		manifest := rasters[page].after.Manifest()
		transforms = append(transforms, ReviewPagePixelTransform{Page: page, Bounds: manifest.CaptureBounds,
			PixelX: uint32(x), PixelY: uint32(y), PixelWidth: manifest.PixelWidth, PixelHeight: manifest.PixelHeight, // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			Transform: manifest.PixelTransform})
	}
	payload, err := encodeReviewPNG(canvas)
	if err != nil {
		return nil, ReviewArtifactMetadata{}, err
	}
	return payload, ReviewArtifactMetadata{Name: "contact-sheet.png", Kind: ReviewArtifactContactSheet,
		Layer: ReviewLayerClean, MediaType: "image/png", PixelWidth: uint32(width), PixelHeight: uint32(height), PageTransforms: transforms}, nil // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
}

func encodeReviewPNG(image image.Image) ([]byte, error) {
	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&out, image); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
