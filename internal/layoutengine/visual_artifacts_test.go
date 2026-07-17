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
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestCaptureAgentVisualArtifactsIsDeterministicExactAndDetached(t *testing.T) {
	plan := testAIGeometryPlan(t)
	projection := plan.Projection()
	limits := DefaultAgentVisualLimits()
	request := AgentVisualRequest{
		Mode: AICaptureGeometry, IncludeContactSheet: true, ContactSheetColumns: 1,
		Nodes: []NodeID{projection.Fragments[0].Node}, Limits: limits,
		Revisions: ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64),
			ScenarioRevision: strings.Repeat("2", 64), PolicyRevision: strings.Repeat("3", 64)},
	}
	first, err := CaptureAgentVisualArtifacts(plan, request)
	if err != nil {
		t.Fatalf("CaptureAgentVisualArtifacts(first) = %v", err)
	}
	second, err := CaptureAgentVisualArtifacts(plan, request)
	if err != nil {
		t.Fatalf("CaptureAgentVisualArtifacts(second) = %v", err)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("manifests differ:\n%s\n%s", firstJSON, secondJSON)
	}

	manifest := first.Manifest()
	planHash, _ := plan.Hash()
	if manifest.FormatVersion != AgentVisualManifestFormatVersion ||
		manifest.PlanSchemaVersion != LayoutPlanSchemaVersion || manifest.PlanHash != planHash.String() ||
		manifest.Identity.FormatVersion != ViewerIdentityFormatVersion ||
		manifest.Identity.PlannerVersion != PlannerVersion ||
		manifest.Identity.PainterContractVersion != PainterContractVersion ||
		manifest.Identity.RendererVersion != "layoutengine/geometry-svg@2" ||
		len(manifest.Identity.ResourceSetHash) != 64 ||
		manifest.Identity.SourceRevision != request.Revisions.SourceRevision ||
		manifest.Identity.ScenarioRevision != request.Revisions.ScenarioRevision ||
		manifest.Identity.PolicyRevision != request.Revisions.PolicyRevision ||
		manifest.Mode != AICaptureGeometry || manifest.Disclosure != AIDisclosureGeometryOnly ||
		manifest.ContainsUserText || manifest.Limits != limits ||
		manifest.ArtifactCount != uint32(len(projection.Fragments)+1) {
		t.Fatalf("manifest = %+v", manifest)
	}
	artifacts := first.Artifacts()
	if artifacts[0].Metadata.Kind != AgentVisualContactSheet ||
		len(artifacts[0].Metadata.PageTransforms) != len(projection.Pages) ||
		!bytes.Contains(artifacts[0].SVG, []byte("data-format=\"contact-sheet\"")) ||
		!bytes.Contains(artifacts[0].SVG, []byte("no browser layout")) {
		t.Fatalf("contact sheet = %+v\n%s", artifacts[0].Metadata, artifacts[0].SVG)
	}
	var total uint64
	for index, artifact := range artifacts {
		metadata := manifest.Artifacts[index]
		digest := sha256.Sum256(artifact.SVG)
		if artifact.Metadata.Index != uint32(index) || artifact.Metadata.Name != metadata.Name ||
			metadata.ByteLength != uint64(len(artifact.SVG)) || metadata.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("artifact %d metadata = %+v / %+v", index, artifact.Metadata, metadata)
		}
		total += metadata.ByteLength
	}
	if manifest.TotalBytes != total {
		t.Fatalf("total bytes = %d, want %d", manifest.TotalBytes, total)
	}
	for index, fragment := range projection.Fragments {
		artifact := artifacts[index+1]
		metadata := artifact.Metadata
		if metadata.Kind != AgentVisualNodeCrop || metadata.Page != fragment.Page ||
			metadata.Node != fragment.Node || metadata.Fragment != fragment.ID ||
			metadata.CropBounds != fragment.BorderBox ||
			metadata.ArtifactBounds != (Rect{Width: fragment.BorderBox.Width, Height: fragment.BorderBox.Height}) ||
			len(metadata.PageTransforms) != 1 {
			t.Fatalf("crop %d metadata = %+v", index, metadata)
		}
		tx, _ := fragment.BorderBox.X.Neg()
		ty, _ := fragment.BorderBox.Y.Neg()
		if metadata.PageTransforms[0].Transform != (AgentVisualTransform{
			TranslateX: tx, TranslateY: ty, ScaleNumerator: 1, ScaleDenominator: 1,
		}) {
			t.Fatalf("crop %d transform = %+v", index, metadata.PageTransforms[0].Transform)
		}
		viewBox := []byte("viewBox=\"0 0 " + strconv.FormatInt(int64(fragment.BorderBox.Width), 10) + " " + strconv.FormatInt(int64(fragment.BorderBox.Height), 10) + "\"")
		if !bytes.Contains(artifact.SVG, viewBox) {
			t.Fatalf("crop %d lacks exact viewBox %q:\n%s", index, viewBox, artifact.SVG)
		}
	}

	var decoded AgentVisualManifest
	if err := json.Unmarshal(firstJSON, &decoded); err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest round trip = %+v, %v; want %+v", decoded, err, manifest)
	}
	manifest.Artifacts[0].PageTransforms[0].Transform.TranslateX++
	artifacts[0].SVG[0] ^= 0xff
	if reflect.DeepEqual(manifest, first.Manifest()) || artifacts[0].SVG[0] == first.Artifacts()[0].SVG[0] {
		t.Fatal("bundle exposed mutable manifest or SVG storage")
	}
}

func TestCaptureAgentVisualArtifactsRejectsPartialRevisionIdentity(t *testing.T) {
	_, err := CaptureAgentVisualArtifacts(testAIGeometryPlan(t), AgentVisualRequest{
		Mode: AICaptureGeometry, IncludeContactSheet: true, ContactSheetColumns: 1,
		Limits:    DefaultAgentVisualLimits(),
		Revisions: ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64)},
	})
	if !errors.Is(err, ErrViewerIdentityInvalid) {
		t.Fatalf("partial revision identity error = %v", err)
	}
}

func TestCaptureAgentVisualArtifactsGeometryEmbedsBoundsAndBreakLabels(t *testing.T) {
	plan, err := NewLayoutPlan(testPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	bundle, err := CaptureAgentVisualArtifacts(plan, AgentVisualRequest{
		Mode: AICaptureGeometry, IncludeContactSheet: true, ContactSheetColumns: 1,
		Limits: DefaultAgentVisualLimits(),
	})
	if err != nil {
		t.Fatalf("CaptureAgentVisualArtifacts() = %v", err)
	}
	artifacts := bundle.Artifacts()
	if len(artifacts) != 1 || artifacts[0].Metadata.Kind != AgentVisualContactSheet {
		t.Fatalf("artifacts = %+v", bundle.Manifest().Artifacts)
	}
	for page := uint32(1); page <= 2; page++ {
		capture, err := plan.CaptureDebugGeometrySVGPage(page)
		if err != nil {
			t.Fatalf("CaptureDebugGeometrySVGPage(%d) = %v", page, err)
		}
		for _, want := range [][]byte{
			[]byte(`class="page"`),
			[]byte(`class="fragment-border"`),
			[]byte(`class="break-label"`),
		} {
			if !bytes.Contains(capture.SVG, want) {
				t.Fatalf("page %d capture lacks %q:\n%s", page, want, capture.SVG)
			}
		}
		if !bytes.Contains(artifacts[0].SVG, []byte(base64.StdEncoding.EncodeToString(capture.SVG))) {
			t.Fatalf("contact sheet does not embed annotated page %d capture", page)
		}
	}
}

func TestCaptureAgentVisualArtifactsEmitsSeparateExactCrossPageStrip(t *testing.T) {
	plan := testAIGeometryPlan(t)
	request := AgentVisualRequest{
		Mode: AICaptureGeometry, IncludeCrossPageStrip: true,
		Limits: DefaultAgentVisualLimits(),
	}
	first, err := CaptureAgentVisualArtifacts(plan, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CaptureAgentVisualArtifacts(plan, request)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, _ := first.CanonicalJSON()
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) || !reflect.DeepEqual(first.Artifacts(), second.Artifacts()) {
		t.Fatal("cross-page strip is not deterministic")
	}
	artifacts := first.Artifacts()
	if len(artifacts) != 1 || artifacts[0].Metadata.Kind != AgentVisualCrossPageStrip || artifacts[0].Metadata.Name != "cross-page-strip.svg" {
		t.Fatalf("cross-page strip artifacts = %+v", first.Manifest().Artifacts)
	}
	metadata := artifacts[0].Metadata
	if len(metadata.PageTransforms) != len(plan.Projection().Pages) ||
		!bytes.Contains(artifacts[0].SVG, []byte(`data-format="cross-page-strip"`)) ||
		!bytes.Contains(artifacts[0].SVG, []byte("exact page-local transforms")) {
		t.Fatalf("cross-page strip metadata/SVG = %+v\n%s", metadata, artifacts[0].SVG)
	}
	for index, placement := range metadata.PageTransforms {
		left, err := placement.CaptureBounds.X.Add(placement.Transform.TranslateX)
		if err != nil || left != 0 {
			t.Fatalf("strip page %d placed left = %d, %v", placement.Page, left, err)
		}
		top, err := placement.CaptureBounds.Y.Add(placement.Transform.TranslateY)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 && top != 0 {
			t.Fatalf("first strip page top = %d", top)
		}
		if index > 0 {
			previous := metadata.PageTransforms[index-1]
			previousTop, _ := previous.CaptureBounds.Y.Add(previous.Transform.TranslateY)
			want, _ := previousTop.Add(previous.CaptureBounds.Height)
			want, _ = want.Add(agentVisualSheetGap)
			if top != want {
				t.Fatalf("strip page %d top = %d, want %d", placement.Page, top, want)
			}
		}
	}
	limited := request
	limited.Limits.MaxPages = 1
	if _, err := CaptureAgentVisualArtifacts(plan, limited); !errors.Is(err, ErrAgentVisualLimit) {
		t.Fatalf("cross-page strip page limit = %v", err)
	}
}

func TestCaptureAgentVisualArtifactsUsesCanonicalFragmentOrderAndExplicitKind(t *testing.T) {
	plan := testAIGeometryPlan(t)
	fragments := plan.Projection().Fragments
	request := AgentVisualRequest{
		Mode:      AICaptureGeometry,
		Fragments: []FragmentID{fragments[len(fragments)-1].ID, fragments[0].ID, fragments[0].ID},
		Limits:    DefaultAgentVisualLimits(),
	}
	bundle, err := CaptureAgentVisualArtifacts(plan, request)
	if err != nil {
		t.Fatalf("CaptureAgentVisualArtifacts() = %v", err)
	}
	artifacts := bundle.Artifacts()
	if len(artifacts) != 2 || artifacts[0].Metadata.Fragment != fragments[0].ID ||
		artifacts[1].Metadata.Fragment != fragments[len(fragments)-1].ID ||
		artifacts[0].Metadata.Kind != AgentVisualFragmentCrop || artifacts[1].Metadata.Kind != AgentVisualFragmentCrop {
		t.Fatalf("artifact order/kinds = %+v", bundle.Manifest().Artifacts)
	}
}

func TestCaptureAgentVisualArtifactsCoreDisclosureEmbedsExistingPreview(t *testing.T) {
	plan, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	fragment := plan.Projection().Fragments[0]
	bundle, err := CaptureAgentVisualArtifacts(plan, AgentVisualRequest{
		Mode: AICaptureCoreText, IncludeContactSheet: true, ContactSheetColumns: 1,
		Fragments: []FragmentID{fragment.ID}, Limits: DefaultAgentVisualLimits(),
	})
	if err != nil {
		t.Fatalf("CaptureAgentVisualArtifacts() = %v", err)
	}
	manifest := bundle.Manifest()
	if !manifest.ContainsUserText || manifest.Disclosure != AIDisclosureContainsUserText {
		t.Fatalf("core disclosure = %+v", manifest)
	}
	page, err := CaptureCorePlanSVG(plan, 1)
	if err != nil {
		t.Fatalf("CaptureCorePlanSVG() = %v", err)
	}
	embedded := base64.StdEncoding.EncodeToString(page.SVG)
	artifacts := bundle.Artifacts()
	if !bytes.Contains(artifacts[0].SVG, []byte(embedded)) {
		t.Fatal("contact sheet does not embed the existing core preview")
	}
	for index, artifact := range artifacts {
		if !artifact.Metadata.ContainsUserText {
			t.Fatalf("artifact %d lacks text disclosure", index)
		}
	}
	croppedSource, err := agentVisualSVGWithViewBox(page.SVG, fragment.BorderBox)
	if err != nil {
		t.Fatalf("agentVisualSVGWithViewBox() = %v", err)
	}
	if !bytes.Contains(artifacts[1].SVG, []byte(base64.StdEncoding.EncodeToString(croppedSource))) {
		t.Fatal("crop does not embed the existing core preview with the exact fragment viewport")
	}
}

func TestCaptureAgentVisualArtifactsEnforcesSelectorsAndLimits(t *testing.T) {
	plan := testAIGeometryPlan(t)
	projection := plan.Projection()
	valid := AgentVisualRequest{
		Mode: AICaptureGeometry, Fragments: []FragmentID{projection.Fragments[0].ID},
		Limits: DefaultAgentVisualLimits(),
	}
	tests := []struct {
		name    string
		request AgentVisualRequest
		want    error
	}{
		{name: "zero limits", request: AgentVisualRequest{Mode: AICaptureGeometry, IncludeContactSheet: true, ContactSheetColumns: 1}, want: ErrAgentVisualRequest},
		{name: "no selection", request: AgentVisualRequest{Mode: AICaptureGeometry, Limits: DefaultAgentVisualLimits()}, want: ErrAgentVisualRequest},
		{name: "zero columns", request: AgentVisualRequest{Mode: AICaptureGeometry, IncludeContactSheet: true, Limits: DefaultAgentVisualLimits()}, want: ErrAgentVisualRequest},
		{name: "unknown node", request: AgentVisualRequest{Mode: AICaptureGeometry, Nodes: []NodeID{999}, Limits: DefaultAgentVisualLimits()}, want: ErrAgentVisualSelector},
		{name: "unknown fragment", request: AgentVisualRequest{Mode: AICaptureGeometry, Fragments: []FragmentID{999}, Limits: DefaultAgentVisualLimits()}, want: ErrAgentVisualSelector},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CaptureAgentVisualArtifacts(plan, test.request); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	pageLimited := valid
	pageLimited.IncludeContactSheet = true
	pageLimited.ContactSheetColumns = 1
	pageLimited.Limits.MaxPages = 1
	if _, err := CaptureAgentVisualArtifacts(plan, pageLimited); !errors.Is(err, ErrAgentVisualLimit) {
		t.Fatalf("page limit error = %v, want ErrAgentVisualLimit", err)
	}
	cropLimited := valid
	cropLimited.Nodes = []NodeID{projection.Fragments[0].Node}
	cropLimited.Fragments = nil
	cropLimited.Limits.MaxCrops = 1
	if _, err := CaptureAgentVisualArtifacts(plan, cropLimited); !errors.Is(err, ErrAgentVisualLimit) {
		t.Fatalf("crop limit error = %v, want ErrAgentVisualLimit", err)
	}
	byteLimited := valid
	byteLimited.Limits.MaxArtifactBytes = 1
	if _, err := CaptureAgentVisualArtifacts(plan, byteLimited); !errors.Is(err, ErrAgentVisualLimit) {
		t.Fatalf("byte limit error = %v, want ErrAgentVisualLimit", err)
	}
}

func TestCaptureAgentVisualArtifactsRejectsEmptyExactCrop(t *testing.T) {
	plan, err := NewLayoutPlan(LayoutPlanInput{
		Pages: []PlannedPage{{Number: 1, Size: Size{Width: 100, Height: 100}, Fragments: IndexRange{Count: 1}}},
		Fragments: []Fragment{{
			ID: 1, Node: 1, Key: "@empty", Instance: "@empty", Page: 1, Region: RegionBody,
			Continuation: ContinuationWhole,
		}},
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	_, err = CaptureAgentVisualArtifacts(plan, AgentVisualRequest{
		Mode: AICaptureGeometry, Fragments: []FragmentID{1}, Limits: DefaultAgentVisualLimits(),
	})
	if !errors.Is(err, ErrAgentVisualEmpty) {
		t.Fatalf("error = %v, want ErrAgentVisualEmpty", err)
	}
}
