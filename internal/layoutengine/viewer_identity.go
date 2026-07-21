// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const ViewerIdentityFormatVersion uint16 = 1

var ErrViewerIdentityInvalid = errors.New("layoutengine: viewer identity is invalid")

// ViewerRevisionIdentityInput binds visual evidence to the exact source,
// scenario, and policy revisions used to produce it. The zero value means an
// internal geometry-only caller did not provide revision context; partial
// identity tuples are rejected.
type ViewerRevisionIdentityInput struct {
	SourceRevision   string `json:"source_revision,omitempty"`
	ScenarioRevision string `json:"scenario_revision,omitempty"`
	PolicyRevision   string `json:"policy_revision,omitempty"`
}

func (input ViewerRevisionIdentityInput) validate() error {
	provided := input.SourceRevision != "" || input.ScenarioRevision != "" || input.PolicyRevision != ""
	if !provided {
		return nil
	}
	if input.SourceRevision == "" || input.ScenarioRevision == "" || input.PolicyRevision == "" {
		return fmt.Errorf("%w: revision tuple is partial", ErrViewerIdentityInvalid)
	}
	if _, err := ParseSourceRevisionID(input.SourceRevision); err != nil {
		return fmt.Errorf("%w: source revision: %v", ErrViewerIdentityInvalid, err)
	}
	if _, err := ParseScenarioRevisionID(input.ScenarioRevision); err != nil {
		return fmt.Errorf("%w: scenario revision: %v", ErrViewerIdentityInvalid, err)
	}
	if _, err := ParsePolicyRevisionID(input.PolicyRevision); err != nil {
		return fmt.Errorf("%w: policy revision: %v", ErrViewerIdentityInvalid, err)
	}
	return nil
}

// ViewerIdentity is the exact engine/resource/revision identity displayed in
// a visual manifest. ResourceSetHash covers the immutable font and image
// resources actually referenced by the plan, independently from geometry.
type ViewerIdentity struct {
	FormatVersion          uint16 `json:"format_version"`
	PlannerVersion         string `json:"planner_version"`
	PainterContractVersion string `json:"painter_contract_version"`
	RendererVersion        string `json:"renderer_version"`
	ResourceSetHash        string `json:"resource_set_hash"`
	SourceRevision         string `json:"source_revision,omitempty"`
	ScenarioRevision       string `json:"scenario_revision,omitempty"`
	PolicyRevision         string `json:"policy_revision,omitempty"`
}

func viewerIdentityForPlan(plan LayoutPlan, renderer string, revisions ViewerRevisionIdentityInput) (ViewerIdentity, error) {
	if renderer == "" {
		return ViewerIdentity{}, fmt.Errorf("%w: renderer version is empty", ErrViewerIdentityInvalid)
	}
	if err := revisions.validate(); err != nil {
		return ViewerIdentity{}, err
	}
	resourceHash, err := planResourceSetHash(plan)
	if err != nil {
		return ViewerIdentity{}, err
	}
	return ViewerIdentity{
		FormatVersion: ViewerIdentityFormatVersion, PlannerVersion: PlannerVersion,
		PainterContractVersion: PainterContractVersion, RendererVersion: renderer,
		ResourceSetHash: resourceHash, SourceRevision: revisions.SourceRevision,
		ScenarioRevision: revisions.ScenarioRevision, PolicyRevision: revisions.PolicyRevision,
	}, nil
}

func planResourceSetHash(plan LayoutPlan) (string, error) {
	projection := struct {
		Version uint16             `json:"version"`
		Fonts   []CoreFontResource `json:"fonts,omitempty"`
		Images  []ImageResource    `json:"images,omitempty"`
	}{Version: 1, Fonts: cloneSlice(plan.fonts), Images: cloneSlice(plan.imageResources)}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("layoutengine: encode plan resource set: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func agentVisualRendererVersion(mode AICaptureMode) string {
	if mode == AICaptureGeometry {
		return fmt.Sprintf("layoutengine/geometry-svg@%d", DebugGeometrySVGFormatVersion)
	}
	return fmt.Sprintf("layoutengine/display-svg@%d", DisplayPlanSVGFormatVersion)
}
