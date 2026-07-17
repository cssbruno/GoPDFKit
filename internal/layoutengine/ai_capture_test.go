// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCaptureAIPlanGeometryManifestIsCanonicalAndDetached(t *testing.T) {
	plan := testAIGeometryPlan(t)
	first, err := CaptureAIPlan(plan, AICaptureGeometry)
	if err != nil {
		t.Fatalf("CaptureAIPlan() = %v", err)
	}
	second, err := CaptureAIPlan(plan, AICaptureGeometry)
	if err != nil {
		t.Fatalf("second CaptureAIPlan() = %v", err)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical manifests differ:\n%s\n%s", firstJSON, secondJSON)
	}

	manifest := first.Manifest()
	planHash, _ := plan.Hash()
	if manifest.FormatVersion != AICaptureManifestFormatVersion ||
		manifest.PlanSchemaVersion != LayoutPlanSchemaVersion || manifest.PlanHash != planHash.String() ||
		manifest.Mode != AICaptureGeometry || manifest.Disclosure != AIDisclosureGeometryOnly ||
		manifest.ContainsUserText || manifest.PageCount != 2 || len(manifest.Artifacts) != 2 {
		t.Fatalf("geometry manifest = %+v", manifest)
	}
	artifacts := first.Artifacts()
	projection := plan.Projection()
	var total uint64
	for index, artifact := range artifacts {
		metadata := manifest.Artifacts[index]
		digest := sha256.Sum256(artifact.SVG)
		wantPageBounds := Rect{Width: projection.Pages[index].Size.Width, Height: projection.Pages[index].Size.Height}
		if artifact.Metadata != metadata || metadata.Page != uint32(index+1) ||
			metadata.Kind != AICaptureGeometry || metadata.FormatVersion != DebugGeometrySVGFormatVersion ||
			metadata.Disclosure != AIDisclosureGeometryOnly || metadata.ContainsUserText ||
			metadata.PageBounds != wantPageBounds ||
			metadata.ByteLength != uint64(len(artifact.SVG)) || metadata.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("geometry artifact %d metadata = %+v", index, metadata)
		}
		total += metadata.ByteLength
	}
	if manifest.TotalBytes != total {
		t.Fatalf("total bytes = %d, want %d", manifest.TotalBytes, total)
	}
	if bytes.Contains(firstJSON, []byte("<svg")) || bytes.Contains(firstJSON, []byte("AA")) {
		t.Fatalf("manifest unexpectedly contains artifact or authored payload: %s", firstJSON)
	}
	var decoded AICaptureManifest
	if err := json.Unmarshal(firstJSON, &decoded); err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest round trip = %+v, %v; want %+v", decoded, err, manifest)
	}

	manifest.Artifacts[0].SHA256 = "mutated"
	artifacts[0].SVG[0] ^= 0xff
	if first.Manifest().Artifacts[0].SHA256 == "mutated" || first.Artifacts()[0].SVG[0] == artifacts[0].SVG[0] {
		t.Fatal("capture bundle exposed mutable manifest or artifact storage")
	}
}

func TestCaptureAIPlanSeparatesGeometryAndCoreDisclosure(t *testing.T) {
	corePlan, err := NewLayoutPlan(coreGlyphPlanInput())
	if err != nil {
		t.Fatalf("NewLayoutPlan(core) = %v", err)
	}
	core, err := CaptureAIPlan(corePlan, AICaptureCoreText)
	if err != nil {
		t.Fatalf("CaptureAIPlan(core) = %v", err)
	}
	coreManifest := core.Manifest()
	coreSVG := core.Artifacts()[0].SVG
	if !coreManifest.ContainsUserText || coreManifest.Disclosure != AIDisclosureContainsUserText ||
		coreManifest.Artifacts[0].FormatVersion != CorePlanSVGFormatVersion ||
		!bytes.Contains(coreSVG, []byte(">A</text>")) || !bytes.Contains(coreSVG, []byte(">B</text>")) {
		t.Fatalf("core capture metadata/artifact = %+v", coreManifest)
	}

	geometry, err := CaptureAIPlan(corePlan, AICaptureGeometry)
	if err != nil {
		t.Fatalf("CaptureAIPlan(core geometry) = %v", err)
	}
	if geometry.Manifest().ContainsUserText || geometry.Manifest().Disclosure != AIDisclosureGeometryOnly ||
		bytes.Contains(geometry.Artifacts()[0].SVG, []byte("AB")) {
		t.Fatalf("geometry capture disclosed core text: %+v", geometry.Manifest())
	}

	geometryOnly := testAIGeometryPlan(t)
	if _, err := CaptureAIPlan(geometryOnly, AICaptureCoreText); err == nil {
		t.Fatal("geometry-only plan unexpectedly produced a core-text capture")
	}
}

func TestCaptureAIPlanEnforcesExplicitLimitsAndSelectors(t *testing.T) {
	pages := make([]PlannedPage, AICaptureMaxPages+1)
	for index := range pages {
		pages[index] = PlannedPage{Number: uint32(index + 1), Size: Size{Width: 100, Height: 100}}
	}
	large, err := NewLayoutPlan(LayoutPlanInput{Pages: pages})
	if err != nil {
		t.Fatalf("NewLayoutPlan(large) = %v", err)
	}
	if _, err := CaptureAIPlan(large, AICaptureGeometry); !errors.Is(err, ErrAICaptureLimit) {
		t.Fatalf("large capture error = %v, want ErrAICaptureLimit", err)
	}
	empty, err := NewLayoutPlan(LayoutPlanInput{})
	if err != nil {
		t.Fatalf("NewLayoutPlan(empty) = %v", err)
	}
	if _, err := CaptureAIPlan(empty, AICaptureGeometry); !errors.Is(err, ErrAICaptureEmpty) {
		t.Fatalf("empty capture error = %v, want ErrAICaptureEmpty", err)
	}
	if _, err := CaptureAIPlan(testAIGeometryPlan(t), "unknown"); !errors.Is(err, ErrAICaptureMode) {
		t.Fatalf("mode error = %v, want ErrAICaptureMode", err)
	}

	if total, ok := addAICaptureBytes(AICaptureMaxTotalArtifactBytes-1, 1); !ok || total != AICaptureMaxTotalArtifactBytes {
		t.Fatalf("exact byte limit = %d/%t", total, ok)
	}
	if _, ok := addAICaptureBytes(AICaptureMaxTotalArtifactBytes, 1); ok {
		t.Fatal("one byte over total artifact limit unexpectedly accepted")
	}
	oversizedManifest := AICaptureManifest{Artifacts: []AIPageArtifactMetadata{{Name: strings.Repeat("x", AICaptureMaxManifestBytes)}}}
	if _, err := oversizedManifest.CanonicalJSON(); !errors.Is(err, ErrAICaptureLimit) {
		t.Fatalf("oversized manifest error = %v, want ErrAICaptureLimit", err)
	}
}

func testAIGeometryPlan(t *testing.T) LayoutPlan {
	t.Helper()
	plan, err := PlanParagraphFlow(ParagraphFlowInput{
		PageSize: Size{Width: 100, Height: 60},
		Body:     Rect{X: 10, Y: 10, Width: 80, Height: 20},
		ParagraphLinePlanInput: ParagraphLinePlanInput{
			Node: 1, Key: "@ai-capture", Instance: "@ai-capture",
			Lines: []ParagraphLineInput{
				{Width: 20, Height: 10, Baseline: 8},
				{Width: 20, Height: 10, Baseline: 8},
				{Width: 20, Height: 10, Baseline: 8},
			},
			Orphans: 1, Widows: 1, Mode: ParagraphBreakPrefer,
		},
	})
	if err != nil {
		t.Fatalf("PlanParagraphFlow() = %v", err)
	}
	return plan
}
