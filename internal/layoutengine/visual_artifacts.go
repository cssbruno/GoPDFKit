// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

const (
	// AgentVisualManifestFormatVersion pins the map-free manifest envelope.
	AgentVisualManifestFormatVersion uint16 = 2
	AgentVisualSVGFormatVersion      uint16 = 1

	AgentVisualHardMaxPages         uint32 = 64
	AgentVisualHardMaxCrops         uint32 = 256
	AgentVisualHardMaxArtifactBytes uint64 = 32 << 20
	AgentVisualHardMaxTotalBytes    uint64 = 64 << 20
	AgentVisualHardMaxManifestBytes uint64 = 1 << 20

	agentVisualSheetGap Fixed = 8 * Fixed(FixedScale)
)

var (
	ErrAgentVisualRequest  = errors.New("layoutengine: invalid agent visual request")
	ErrAgentVisualLimit    = errors.New("layoutengine: agent visual artifact limit exceeded")
	ErrAgentVisualSelector = errors.New("layoutengine: agent visual selector was not found")
	ErrAgentVisualEmpty    = errors.New("layoutengine: agent visual crop is empty")
)

// AgentVisualLimits is both enforced and recorded in the manifest. Hard caps
// prevent callers from turning the in-memory artifact API into an unbounded
// serialization surface.
type AgentVisualLimits struct {
	MaxPages         uint32 `json:"max_pages"`
	MaxCrops         uint32 `json:"max_crops"`
	MaxArtifactBytes uint64 `json:"max_artifact_bytes"`
	MaxTotalBytes    uint64 `json:"max_total_bytes"`
	MaxManifestBytes uint64 `json:"max_manifest_bytes"`
}

// DefaultAgentVisualLimits returns useful, bounded interactive-tool limits.
func DefaultAgentVisualLimits() AgentVisualLimits {
	return AgentVisualLimits{
		MaxPages: 32, MaxCrops: 128,
		MaxArtifactBytes: 8 << 20, MaxTotalBytes: 32 << 20, MaxManifestBytes: 256 << 10,
	}
}

func (limits AgentVisualLimits) validate() error {
	if limits.MaxPages == 0 || limits.MaxPages > AgentVisualHardMaxPages ||
		limits.MaxCrops == 0 || limits.MaxCrops > AgentVisualHardMaxCrops ||
		limits.MaxArtifactBytes == 0 || limits.MaxArtifactBytes > AgentVisualHardMaxArtifactBytes ||
		limits.MaxTotalBytes == 0 || limits.MaxTotalBytes > AgentVisualHardMaxTotalBytes ||
		limits.MaxManifestBytes == 0 || limits.MaxManifestBytes > AgentVisualHardMaxManifestBytes {
		return fmt.Errorf("%w: limits must be positive and no greater than the published hard caps", ErrAgentVisualRequest)
	}
	if limits.MaxArtifactBytes > limits.MaxTotalBytes {
		return fmt.Errorf("%w: per-artifact byte limit exceeds total byte limit", ErrAgentVisualRequest)
	}
	return nil
}

// AgentVisualRequest asks for one contact sheet and/or exact fragment crops.
// Nodes and Fragments are set selectors; duplicate or differently ordered
// selectors never affect artifact ordering or hashes. A selected node produces
// one crop for each of its fragments, in canonical plan order.
type AgentVisualRequest struct {
	Mode                  AICaptureMode
	IncludeContactSheet   bool
	IncludeCrossPageStrip bool
	ContactSheetColumns   uint32
	Nodes                 []NodeID
	Fragments             []FragmentID
	Limits                AgentVisualLimits
	Revisions             ViewerRevisionIdentityInput
}

type AgentVisualArtifactKind string

const (
	AgentVisualContactSheet   AgentVisualArtifactKind = "contact_sheet"
	AgentVisualCrossPageStrip AgentVisualArtifactKind = "cross_page_strip"
	AgentVisualNodeCrop       AgentVisualArtifactKind = "node_crop"
	AgentVisualFragmentCrop   AgentVisualArtifactKind = "fragment_crop"
)

// AgentVisualTransform maps a page-plan coordinate into an artifact
// coordinate. This initial contract is translation-only and records the unit
// scale explicitly so consumers do not infer CSS pixels or browser geometry.
type AgentVisualTransform struct {
	TranslateX       Fixed `json:"translate_x"`
	TranslateY       Fixed `json:"translate_y"`
	ScaleNumerator   int64 `json:"scale_numerator"`
	ScaleDenominator int64 `json:"scale_denominator"`
}

// AgentVisualPageTransform records one exact page placement in a contact
// sheet, including off-page geometry represented by the source capture.
type AgentVisualPageTransform struct {
	Page          uint32               `json:"page"`
	PageBounds    Rect                 `json:"page_bounds"`
	CaptureBounds Rect                 `json:"capture_bounds"`
	Transform     AgentVisualTransform `json:"transform"`
}

// AgentVisualArtifactMetadata describes either the all-page sheet or one
// exact fragment-border-box crop. PageTransforms has one entry for a crop and
// one per page for a contact sheet.
type AgentVisualArtifactMetadata struct {
	Index            uint32                     `json:"index"`
	Name             string                     `json:"name"`
	Kind             AgentVisualArtifactKind    `json:"kind"`
	MediaType        string                     `json:"media_type"`
	FormatVersion    uint16                     `json:"format_version"`
	Page             uint32                     `json:"page,omitempty"`
	Node             NodeID                     `json:"node,omitempty"`
	Fragment         FragmentID                 `json:"fragment,omitempty"`
	CropBounds       Rect                       `json:"crop_bounds"`
	ArtifactBounds   Rect                       `json:"artifact_bounds"`
	PageTransforms   []AgentVisualPageTransform `json:"page_transforms"`
	FixedScale       int64                      `json:"fixed_scale"`
	Disclosure       AICaptureDisclosure        `json:"disclosure"`
	ContainsUserText bool                       `json:"contains_user_text"`
	ByteLength       uint64                     `json:"byte_length"`
	SHA256           string                     `json:"sha256"`
}

// AgentVisualManifest is deterministic and map-free. It contains artifact
// metadata and digests, never the SVG payloads themselves.
type AgentVisualManifest struct {
	FormatVersion     uint16                        `json:"format_version"`
	PlanSchemaVersion uint16                        `json:"plan_schema_version"`
	PlanHash          string                        `json:"plan_hash"`
	Identity          ViewerIdentity                `json:"identity"`
	Mode              AICaptureMode                 `json:"mode"`
	Disclosure        AICaptureDisclosure           `json:"disclosure"`
	ContainsUserText  bool                          `json:"contains_user_text"`
	Limits            AgentVisualLimits             `json:"limits"`
	ArtifactCount     uint32                        `json:"artifact_count"`
	TotalBytes        uint64                        `json:"total_bytes"`
	Artifacts         []AgentVisualArtifactMetadata `json:"artifacts"`
}

func (manifest AgentVisualManifest) CanonicalJSON() ([]byte, error) {
	if err := manifest.Limits.validate(); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: marshal agent visual manifest: %w", err)
	}
	if uint64(len(encoded)) > manifest.Limits.MaxManifestBytes {
		return nil, fmt.Errorf("%w: manifest exceeds %d bytes", ErrAgentVisualLimit, manifest.Limits.MaxManifestBytes)
	}
	return encoded, nil
}

type AgentVisualArtifact struct {
	Metadata AgentVisualArtifactMetadata
	SVG      []byte
}

// AgentVisualBundle owns detached SVG artifacts and their canonical manifest.
type AgentVisualBundle struct {
	manifest  AgentVisualManifest
	artifacts []AgentVisualArtifact
}

func (bundle AgentVisualBundle) Manifest() AgentVisualManifest {
	manifest := bundle.manifest
	manifest.Artifacts = cloneAgentVisualMetadata(bundle.manifest.Artifacts)
	return manifest
}

func (bundle AgentVisualBundle) Artifacts() []AgentVisualArtifact {
	if len(bundle.artifacts) == 0 {
		return nil
	}
	artifacts := make([]AgentVisualArtifact, len(bundle.artifacts))
	for index, artifact := range bundle.artifacts {
		artifacts[index] = AgentVisualArtifact{
			Metadata: cloneAgentVisualMetadata([]AgentVisualArtifactMetadata{artifact.Metadata})[0],
			SVG:      append([]byte(nil), artifact.SVG...),
		}
	}
	return artifacts
}

func (bundle AgentVisualBundle) CanonicalJSON() ([]byte, error) {
	return bundle.manifest.CanonicalJSON()
}

func cloneAgentVisualMetadata(values []AgentVisualArtifactMetadata) []AgentVisualArtifactMetadata {
	if len(values) == 0 {
		return nil
	}
	result := append([]AgentVisualArtifactMetadata(nil), values...)
	for index := range result {
		result[index].PageTransforms = append([]AgentVisualPageTransform(nil), values[index].PageTransforms...) // #nosec G602 -- result and values have identical lengths from the copy above.
	}
	return result
}

type agentVisualPageCapture struct {
	metadata AIPageArtifactMetadata
	svg      []byte
}

// CaptureAgentVisualArtifacts creates agent-oriented SVGs directly from an
// immutable plan. It performs no browser layout, text measurement, wrapping,
// pagination, or coordinate rounding. Any error returns an empty bundle.
func CaptureAgentVisualArtifacts(plan LayoutPlan, request AgentVisualRequest) (AgentVisualBundle, error) {
	if err := request.Limits.validate(); err != nil {
		return AgentVisualBundle{}, err
	}
	if !request.Mode.valid() {
		return AgentVisualBundle{}, fmt.Errorf("%w: capture mode %q", ErrAgentVisualRequest, request.Mode)
	}
	if err := request.Revisions.validate(); err != nil {
		return AgentVisualBundle{}, err
	}
	if !request.IncludeContactSheet && !request.IncludeCrossPageStrip && len(request.Nodes) == 0 && len(request.Fragments) == 0 {
		return AgentVisualBundle{}, fmt.Errorf("%w: no artifact selected", ErrAgentVisualRequest)
	}
	if uint64(len(request.Nodes)) > uint64(request.Limits.MaxCrops) ||
		uint64(len(request.Fragments)) > uint64(request.Limits.MaxCrops) {
		return AgentVisualBundle{}, fmt.Errorf("%w: selector count exceeds crop limit %d", ErrAgentVisualLimit, request.Limits.MaxCrops)
	}
	if request.IncludeContactSheet && (request.ContactSheetColumns == 0 || request.ContactSheetColumns > request.Limits.MaxPages) {
		return AgentVisualBundle{}, fmt.Errorf("%w: contact-sheet columns must be within the page limit", ErrAgentVisualRequest)
	}
	if err := plan.Validate(); err != nil {
		return AgentVisualBundle{}, fmt.Errorf("layoutengine: capture agent visuals from invalid plan: %w", err)
	}
	if len(plan.pages) == 0 {
		return AgentVisualBundle{}, ErrAICaptureEmpty
	}
	if request.Mode == AICaptureCoreText {
		if err := plan.ValidatePaintReady(); err != nil {
			return AgentVisualBundle{}, fmt.Errorf("layoutengine: capture agent core preview: %w", err)
		}
	}

	selected, explicitFragments, err := selectAgentVisualFragments(plan, request)
	if err != nil {
		return AgentVisualBundle{}, err
	}
	if uint32(len(selected)) > request.Limits.MaxCrops { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return AgentVisualBundle{}, fmt.Errorf("%w: %d crops exceed limit %d", ErrAgentVisualLimit, len(selected), request.Limits.MaxCrops)
	}
	pagesNeeded := make(map[uint32]struct{})
	if request.IncludeContactSheet || request.IncludeCrossPageStrip {
		if uint32(len(plan.pages)) > request.Limits.MaxPages { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			return AgentVisualBundle{}, fmt.Errorf("%w: %d pages exceed limit %d", ErrAgentVisualLimit, len(plan.pages), request.Limits.MaxPages)
		}
		for _, page := range plan.pages {
			pagesNeeded[page.Number] = struct{}{}
		}
	}
	for _, fragment := range selected {
		pagesNeeded[fragment.Page] = struct{}{}
	}
	if uint32(len(pagesNeeded)) > request.Limits.MaxPages { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return AgentVisualBundle{}, fmt.Errorf("%w: %d selected pages exceed limit %d", ErrAgentVisualLimit, len(pagesNeeded), request.Limits.MaxPages)
	}

	disclosure, containsUserText := AIDisclosureGeometryOnly, false
	if request.Mode == AICaptureCoreText {
		disclosure, containsUserText = AIDisclosureContainsUserText, true
	}
	planHash, err := plan.Hash()
	if err != nil {
		return AgentVisualBundle{}, fmt.Errorf("layoutengine: hash agent visual plan: %w", err)
	}
	identity, err := viewerIdentityForPlan(plan, agentVisualRendererVersion(request.Mode), request.Revisions)
	if err != nil {
		return AgentVisualBundle{}, err
	}
	manifest := AgentVisualManifest{
		FormatVersion: AgentVisualManifestFormatVersion, PlanSchemaVersion: LayoutPlanSchemaVersion,
		PlanHash: planHash.String(), Identity: identity, Mode: request.Mode, Disclosure: disclosure,
		ContainsUserText: containsUserText, Limits: request.Limits,
	}

	pageCaptures := make(map[uint32]agentVisualPageCapture, len(pagesNeeded))
	for _, page := range plan.pages {
		if _, needed := pagesNeeded[page.Number]; !needed {
			continue
		}
		metadata, svg, captureErr := captureAIPlanPage(plan, page.Number, request.Mode, disclosure, containsUserText)
		if captureErr != nil {
			return AgentVisualBundle{}, captureErr
		}
		pageCaptures[page.Number] = agentVisualPageCapture{metadata: metadata, svg: svg}
	}

	artifacts := make([]AgentVisualArtifact, 0, len(selected)+1)
	if request.IncludeContactSheet {
		metadata, svg, writeErr := writeAgentVisualContactSheet(plan, pageCaptures, request, disclosure, containsUserText)
		if writeErr != nil {
			return AgentVisualBundle{}, writeErr
		}
		if err := appendAgentVisualArtifact(&manifest, &artifacts, metadata, svg); err != nil {
			return AgentVisualBundle{}, err
		}
	}
	if request.IncludeCrossPageStrip {
		metadata, svg, writeErr := writeAgentVisualCrossPageStrip(plan, pageCaptures, request, disclosure, containsUserText)
		if writeErr != nil {
			return AgentVisualBundle{}, writeErr
		}
		if err := appendAgentVisualArtifact(&manifest, &artifacts, metadata, svg); err != nil {
			return AgentVisualBundle{}, err
		}
	}
	for _, fragment := range selected {
		kind := AgentVisualNodeCrop
		if explicitFragments[fragment.ID] {
			kind = AgentVisualFragmentCrop
		}
		metadata, svg, writeErr := writeAgentVisualCrop(fragment, pageCaptures[fragment.Page], kind, request, disclosure, containsUserText)
		if writeErr != nil {
			return AgentVisualBundle{}, writeErr
		}
		if err := appendAgentVisualArtifact(&manifest, &artifacts, metadata, svg); err != nil {
			return AgentVisualBundle{}, err
		}
	}
	manifest.ArtifactCount = uint32(len(artifacts)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	if _, err := manifest.CanonicalJSON(); err != nil {
		return AgentVisualBundle{}, err
	}
	return AgentVisualBundle{manifest: manifest, artifacts: artifacts}, nil
}

func writeAgentVisualCrossPageStrip(plan LayoutPlan, captures map[uint32]agentVisualPageCapture, request AgentVisualRequest, disclosure AICaptureDisclosure, containsUserText bool) (AgentVisualArtifactMetadata, []byte, error) {
	placements := make([]AgentVisualPageTransform, len(plan.pages))
	var width, cursor Fixed
	for index, page := range plan.pages {
		capture := captures[page.Number]
		bounds := capture.metadata.CaptureBounds
		if bounds.Width > width {
			width = bounds.Width
		}
		tx, err := bounds.X.Neg()
		if err != nil {
			return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: cross-page strip page %d x: %w", page.Number, err)
		}
		ty, err := cursor.Sub(bounds.Y)
		if err != nil {
			return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: cross-page strip page %d y: %w", page.Number, err)
		}
		placements[index] = AgentVisualPageTransform{
			Page: page.Number, PageBounds: capture.metadata.PageBounds, CaptureBounds: bounds,
			Transform: AgentVisualTransform{TranslateX: tx, TranslateY: ty, ScaleNumerator: 1, ScaleDenominator: 1},
		}
		cursor, err = cursor.Add(bounds.Height)
		if err != nil {
			return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: cross-page strip height: %w", err)
		}
		if index+1 < len(plan.pages) {
			cursor, err = cursor.Add(agentVisualSheetGap)
			if err != nil {
				return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: cross-page strip gap: %w", err)
			}
		}
	}
	artifactBounds, err := NewRect(0, 0, width, cursor)
	if err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: cross-page strip bounds: %w", err)
	}
	if artifactBounds.IsEmpty() {
		return AgentVisualArtifactMetadata{}, nil, errors.New("layoutengine: cross-page strip bounds are empty")
	}
	writer := debugGeometrySVGWriter{limit: int(request.Limits.MaxArtifactBytes)} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	writeAgentVisualRoot(&writer, "cross-page-strip", artifactBounds, request.Mode, disclosure)
	writer.write("<title>Direct-plan cross-page strip</title><desc>Separate vertical page presentation with exact page-local transforms; no browser layout.</desc>")
	writer.write("<style>.page-frame{fill:white;stroke:#98a2b3;stroke-width:256}</style>")
	for index, page := range plan.pages {
		capture := captures[page.Number]
		placement := placements[index]
		writer.write("<g class=\"strip-page\" data-page=\"")
		writer.write(strconv.FormatUint(uint64(page.Number), 10))
		writer.write("\" transform=\"translate(")
		writer.write(fixedSVGDecimal(placement.Transform.TranslateX))
		writer.write(" ")
		writer.write(fixedSVGDecimal(placement.Transform.TranslateY))
		writer.write(")\"><rect class=\"page-frame\" x=\"0\" y=\"0\" width=\"")
		writer.write(fixedSVGDecimal(page.Size.Width))
		writer.write("\" height=\"")
		writer.write(fixedSVGDecimal(page.Size.Height))
		writer.write("\"/>")
		writeAgentVisualEmbeddedCapture(&writer, capture)
		writer.write("</g>")
	}
	writer.write("</svg>")
	if writer.err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("%w: cross-page strip", ErrAgentVisualLimit)
	}
	metadata := AgentVisualArtifactMetadata{
		Name: "cross-page-strip.svg", Kind: AgentVisualCrossPageStrip, MediaType: "image/svg+xml",
		FormatVersion: AgentVisualSVGFormatVersion, ArtifactBounds: artifactBounds,
		PageTransforms: placements, FixedScale: FixedScale, Disclosure: disclosure, ContainsUserText: containsUserText,
	}
	return metadata, []byte(writer.builder.String()), nil
}

func selectAgentVisualFragments(plan LayoutPlan, request AgentVisualRequest) ([]Fragment, map[FragmentID]bool, error) {
	nodes := make(map[NodeID]bool, len(request.Nodes))
	nodeOrder := make([]NodeID, 0, len(request.Nodes))
	for _, node := range request.Nodes {
		if !node.Valid() {
			return nil, nil, fmt.Errorf("%w: absent node ID", ErrAgentVisualRequest)
		}
		if _, exists := nodes[node]; exists {
			continue
		}
		nodes[node] = false
		nodeOrder = append(nodeOrder, node)
	}
	explicit := make(map[FragmentID]bool, len(request.Fragments))
	fragmentOrder := make([]FragmentID, 0, len(request.Fragments))
	for _, fragment := range request.Fragments {
		if !fragment.Valid() {
			return nil, nil, fmt.Errorf("%w: absent fragment ID", ErrAgentVisualRequest)
		}
		if _, exists := explicit[fragment]; exists {
			continue
		}
		explicit[fragment] = false
		fragmentOrder = append(fragmentOrder, fragment)
	}
	selected := make([]Fragment, 0)
	for _, fragment := range plan.fragments {
		_, byNode := nodes[fragment.Node]
		_, byFragment := explicit[fragment.ID]
		if !byNode && !byFragment {
			continue
		}
		if byNode {
			nodes[fragment.Node] = true
		}
		if byFragment {
			explicit[fragment.ID] = true
		}
		selected = append(selected, fragment)
	}
	for _, node := range nodeOrder {
		if !nodes[node] {
			return nil, nil, fmt.Errorf("%w: node %d", ErrAgentVisualSelector, node)
		}
	}
	for _, fragment := range fragmentOrder {
		if !explicit[fragment] {
			return nil, nil, fmt.Errorf("%w: fragment %d", ErrAgentVisualSelector, fragment)
		}
	}
	return selected, explicit, nil
}

func appendAgentVisualArtifact(manifest *AgentVisualManifest, artifacts *[]AgentVisualArtifact, metadata AgentVisualArtifactMetadata, svg []byte) error {
	if uint64(len(svg)) > manifest.Limits.MaxArtifactBytes {
		return fmt.Errorf("%w: artifact %q exceeds %d bytes", ErrAgentVisualLimit, metadata.Name, manifest.Limits.MaxArtifactBytes)
	}
	next, ok := addBoundedBytes(manifest.TotalBytes, uint64(len(svg)), manifest.Limits.MaxTotalBytes)
	if !ok {
		return fmt.Errorf("%w: artifacts exceed %d total bytes", ErrAgentVisualLimit, manifest.Limits.MaxTotalBytes)
	}
	metadata.Index = uint32(len(*artifacts)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	metadata.ByteLength = uint64(len(svg))
	digest := sha256.Sum256(svg)
	metadata.SHA256 = hex.EncodeToString(digest[:])
	manifest.TotalBytes = next
	manifest.Artifacts = append(manifest.Artifacts, metadata)
	*artifacts = append(*artifacts, AgentVisualArtifact{Metadata: metadata, SVG: append([]byte(nil), svg...)})
	return nil
}

func addBoundedBytes(total, next, limit uint64) (uint64, bool) {
	if total > limit || next > limit-total {
		return 0, false
	}
	return total + next, true
}

func writeAgentVisualContactSheet(plan LayoutPlan, captures map[uint32]agentVisualPageCapture, request AgentVisualRequest, disclosure AICaptureDisclosure, containsUserText bool) (AgentVisualArtifactMetadata, []byte, error) {
	columns := int(request.ContactSheetColumns)
	if columns > len(plan.pages) {
		columns = len(plan.pages)
	}
	rows := (len(plan.pages) + columns - 1) / columns
	columnWidths := make([]Fixed, columns)
	rowHeights := make([]Fixed, rows)
	for index, page := range plan.pages {
		column, row := index%columns, index/columns
		if page.Size.Width > columnWidths[column] {
			columnWidths[column] = page.Size.Width
		}
		if page.Size.Height > rowHeights[row] {
			rowHeights[row] = page.Size.Height
		}
	}
	columnX, err := agentVisualOffsets(columnWidths)
	if err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: contact sheet columns: %w", err)
	}
	rowY, err := agentVisualOffsets(rowHeights)
	if err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: contact sheet rows: %w", err)
	}

	placements := make([]AgentVisualPageTransform, len(plan.pages))
	var canvas Rect
	for index, page := range plan.pages {
		capture := captures[page.Number]
		x, y := columnX[index%columns], rowY[index/columns]
		placed, translateErr := capture.metadata.CaptureBounds.Translate(x, y)
		if translateErr != nil {
			return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: contact sheet page %d: %w", page.Number, translateErr)
		}
		if index == 0 {
			canvas = placed
		} else if canvas, err = canvas.Union(placed); err != nil {
			return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: contact sheet canvas: %w", err)
		}
		placements[index] = AgentVisualPageTransform{
			Page: page.Number, PageBounds: capture.metadata.PageBounds, CaptureBounds: capture.metadata.CaptureBounds,
			Transform: AgentVisualTransform{TranslateX: x, TranslateY: y, ScaleNumerator: 1, ScaleDenominator: 1},
		}
	}
	for index := range placements {
		placements[index].Transform.TranslateX, err = placements[index].Transform.TranslateX.Sub(canvas.X)
		if err != nil {
			return AgentVisualArtifactMetadata{}, nil, err
		}
		placements[index].Transform.TranslateY, err = placements[index].Transform.TranslateY.Sub(canvas.Y)
		if err != nil {
			return AgentVisualArtifactMetadata{}, nil, err
		}
	}
	artifactBounds := Rect{Width: canvas.Width, Height: canvas.Height}
	writer := debugGeometrySVGWriter{limit: int(request.Limits.MaxArtifactBytes)} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	writeAgentVisualRoot(&writer, "contact-sheet", artifactBounds, request.Mode, disclosure)
	writer.write("<title>Direct-plan multi-page contact sheet</title><desc>Exact fixed-coordinate page translations; no browser layout.</desc>")
	writer.write("<style>.page-frame{fill:white;stroke:#98a2b3;stroke-width:256}</style>")
	for index, page := range plan.pages {
		capture := captures[page.Number]
		placement := placements[index]
		writer.write("<g class=\"sheet-page\" data-page=\"")
		writer.write(strconv.FormatUint(uint64(page.Number), 10))
		writer.write("\" transform=\"translate(")
		writer.write(fixedSVGDecimal(placement.Transform.TranslateX))
		writer.write(" ")
		writer.write(fixedSVGDecimal(placement.Transform.TranslateY))
		writer.write(")\">")
		writer.write("<rect class=\"page-frame\" x=\"0\" y=\"0\" width=\"")
		writer.write(fixedSVGDecimal(page.Size.Width))
		writer.write("\" height=\"")
		writer.write(fixedSVGDecimal(page.Size.Height))
		writer.write("\"/>")
		writeAgentVisualEmbeddedCapture(&writer, capture)
		writer.write("</g>")
	}
	writer.write("</svg>")
	if writer.err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("%w: contact sheet", ErrAgentVisualLimit)
	}
	metadata := AgentVisualArtifactMetadata{
		Name: "contact-sheet.svg", Kind: AgentVisualContactSheet, MediaType: "image/svg+xml",
		FormatVersion: AgentVisualSVGFormatVersion, ArtifactBounds: artifactBounds,
		PageTransforms: placements, FixedScale: FixedScale, Disclosure: disclosure, ContainsUserText: containsUserText,
	}
	return metadata, []byte(writer.builder.String()), nil
}

func agentVisualOffsets(extents []Fixed) ([]Fixed, error) {
	offsets := make([]Fixed, len(extents))
	var cursor Fixed
	for index, extent := range extents {
		offsets[index] = cursor
		next, err := cursor.Add(extent)
		if err != nil {
			return nil, err
		}
		if index+1 < len(extents) {
			next, err = next.Add(agentVisualSheetGap)
			if err != nil {
				return nil, err
			}
		}
		cursor = next
	}
	return offsets, nil
}

func writeAgentVisualCrop(fragment Fragment, capture agentVisualPageCapture, kind AgentVisualArtifactKind, request AgentVisualRequest, disclosure AICaptureDisclosure, containsUserText bool) (AgentVisualArtifactMetadata, []byte, error) {
	crop := fragment.BorderBox
	if crop.IsEmpty() {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("%w: fragment %d", ErrAgentVisualEmpty, fragment.ID)
	}
	tx, err := crop.X.Neg()
	if err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: crop fragment %d x transform: %w", fragment.ID, err)
	}
	ty, err := crop.Y.Neg()
	if err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: crop fragment %d y transform: %w", fragment.ID, err)
	}
	transform := AgentVisualPageTransform{
		Page: fragment.Page, PageBounds: capture.metadata.PageBounds, CaptureBounds: capture.metadata.CaptureBounds,
		Transform: AgentVisualTransform{TranslateX: tx, TranslateY: ty, ScaleNumerator: 1, ScaleDenominator: 1},
	}
	artifactBounds := Rect{Width: crop.Width, Height: crop.Height}
	writer := debugGeometrySVGWriter{limit: int(request.Limits.MaxArtifactBytes)} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	writeAgentVisualRoot(&writer, "fragment-crop", artifactBounds, request.Mode, disclosure)
	writer.write("<title>Exact plan fragment crop</title><desc>Fragment border-box crop in fixed plan coordinates; no browser layout.</desc>")
	writer.write("<g data-page=\"")
	writer.write(strconv.FormatUint(uint64(fragment.Page), 10))
	writer.write("\" data-node-id=\"")
	writer.write(strconv.FormatUint(uint64(fragment.Node), 10))
	writer.write("\" data-fragment-id=\"")
	writer.write(strconv.FormatUint(uint64(fragment.ID), 10))
	writer.write("\" transform=\"translate(")
	writer.write(fixedSVGDecimal(tx))
	writer.write(" ")
	writer.write(fixedSVGDecimal(ty))
	writer.write(")\">")
	if err := writeAgentVisualExactCrop(&writer, capture, crop); err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("layoutengine: crop fragment %d source: %w", fragment.ID, err)
	}
	writer.write("</g></svg>")
	if writer.err != nil {
		return AgentVisualArtifactMetadata{}, nil, fmt.Errorf("%w: fragment %d crop", ErrAgentVisualLimit, fragment.ID)
	}
	name := fmt.Sprintf("node-%08d-fragment-%08d.svg", fragment.Node, fragment.ID)
	if kind == AgentVisualFragmentCrop {
		name = fmt.Sprintf("fragment-%08d.svg", fragment.ID)
	}
	metadata := AgentVisualArtifactMetadata{
		Name: name, Kind: kind, MediaType: "image/svg+xml", FormatVersion: AgentVisualSVGFormatVersion,
		Page: fragment.Page, Node: fragment.Node, Fragment: fragment.ID,
		CropBounds: crop, ArtifactBounds: artifactBounds, PageTransforms: []AgentVisualPageTransform{transform},
		FixedScale: FixedScale, Disclosure: disclosure, ContainsUserText: containsUserText,
	}
	return metadata, []byte(writer.builder.String()), nil
}

func writeAgentVisualRoot(writer *debugGeometrySVGWriter, format string, bounds Rect, mode AICaptureMode, disclosure AICaptureDisclosure) {
	writer.write("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 ")
	writer.write(fixedSVGDecimal(bounds.Width))
	writer.write(" ")
	writer.write(fixedSVGDecimal(bounds.Height))
	writer.write("\" overflow=\"hidden\" data-format=\"")
	writer.attribute(format)
	writer.write("\" data-format-version=\"")
	writer.write(strconv.FormatUint(uint64(AgentVisualSVGFormatVersion), 10))
	writer.write("\" data-coordinate-space=\"pdf-fixed\" data-fixed-scale=\"")
	writer.write(strconv.FormatInt(FixedScale, 10))
	writer.write("\" data-source-mode=\"")
	writer.attribute(string(mode))
	writer.write("\" data-disclosure=\"")
	writer.attribute(string(disclosure))
	writer.write("\">")
}

func writeAgentVisualEmbeddedCapture(writer *debugGeometrySVGWriter, capture agentVisualPageCapture) {
	bounds := capture.metadata.CaptureBounds
	writer.write("<image x=\"")
	writer.write(fixedSVGDecimal(bounds.X))
	writer.write("\" y=\"")
	writer.write(fixedSVGDecimal(bounds.Y))
	writer.write("\" width=\"")
	writer.write(fixedSVGDecimal(bounds.Width))
	writer.write("\" height=\"")
	writer.write(fixedSVGDecimal(bounds.Height))
	writer.write("\" preserveAspectRatio=\"none\" href=\"data:image/svg+xml;base64,")
	writer.write(base64.StdEncoding.EncodeToString(capture.svg))
	writer.write("\"/>")
}

// writeAgentVisualExactCrop changes only the embedded capture viewport. The
// existing capture's positioned elements remain untouched, including content
// outside the physical page. Placing that viewport at the crop's plan origin
// and applying the recorded negative translation preserves a strict 1:1 map.
func writeAgentVisualExactCrop(writer *debugGeometrySVGWriter, capture agentVisualPageCapture, crop Rect) error {
	source, err := agentVisualSVGWithViewBox(capture.svg, crop)
	if err != nil {
		return err
	}
	writer.write("<image x=\"")
	writer.write(fixedSVGDecimal(crop.X))
	writer.write("\" y=\"")
	writer.write(fixedSVGDecimal(crop.Y))
	writer.write("\" width=\"")
	writer.write(fixedSVGDecimal(crop.Width))
	writer.write("\" height=\"")
	writer.write(fixedSVGDecimal(crop.Height))
	writer.write("\" preserveAspectRatio=\"none\" href=\"data:image/svg+xml;base64,")
	writer.write(base64.StdEncoding.EncodeToString(source))
	writer.write("\"/>")
	return nil
}

func agentVisualSVGWithViewBox(svg []byte, bounds Rect) ([]byte, error) {
	marker := []byte("viewBox=\"")
	start := bytes.Index(svg, marker)
	if start < 0 {
		return nil, errors.New("source SVG has no viewBox")
	}
	valueStart := start + len(marker)
	valueEndOffset := bytes.IndexByte(svg[valueStart:], '"')
	if valueEndOffset < 0 {
		return nil, errors.New("source SVG has an unterminated viewBox")
	}
	valueEnd := valueStart + valueEndOffset
	replacement := []byte(fixedSVGDecimal(bounds.X) + " " + fixedSVGDecimal(bounds.Y) + " " +
		fixedSVGDecimal(bounds.Width) + " " + fixedSVGDecimal(bounds.Height))
	result := make([]byte, 0, len(svg)-valueEnd+valueStart+len(replacement))
	result = append(result, svg[:valueStart]...)
	result = append(result, replacement...)
	result = append(result, svg[valueEnd:]...)
	return result, nil
}
