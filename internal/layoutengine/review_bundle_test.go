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
	"image/color"
	"image/png"
	"reflect"
	"strings"
	"testing"
)

func TestBuildReviewBundleDeterministicCompleteAndDetached(t *testing.T) {
	before, sources := rasterFixture(t)
	after := reviewChangedFixture(t, before)
	request := reviewRequest()
	request.SourceDiff = []byte("--- before.paper\n+++ after.paper\n@@\n-color: red\n+color: blue\n")

	first, err := BuildReviewBundle(context.Background(), before, after, sources, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildReviewBundle(context.Background(), before, after, sources, sources, request)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) || !reflect.DeepEqual(first.Artifacts(), second.Artifacts()) {
		t.Fatal("review bundle is not deterministic")
	}

	manifest := first.Manifest()
	if manifest.FormatVersion != ReviewBundleManifestVersion || manifest.RendererVersion != ReviewBundleRendererVersion ||
		manifest.BeforeScenarioRevision != request.BeforeRevisions.ScenarioRevision || manifest.AfterScenarioRevision != request.AfterRevisions.ScenarioRevision ||
		manifest.BeforePolicyRevision != request.BeforeRevisions.PolicyRevision || manifest.AfterPolicyRevision != request.AfterRevisions.PolicyRevision ||
		manifest.PageProfile != request.PageProfile || manifest.BeforeIdentity.ResourceSetHash == "" || manifest.AfterIdentity.ResourceSetHash == "" ||
		manifest.BeforeIdentity.SourceRevision != request.BeforeRevisions.SourceRevision || manifest.AfterIdentity.SourceRevision != request.AfterRevisions.SourceRevision ||
		!reflect.DeepEqual(manifest.ChangedPages, []uint32{1}) || manifest.ArtifactCount != uint32(len(first.Artifacts())) {
		t.Fatalf("manifest = %+v", manifest)
	}
	if manifest.BeforeSemantics.SHA256 == "" || manifest.AfterSemantics.SHA256 == "" ||
		manifest.BeforeAccessibility.SHA256 == "" || manifest.AfterAccessibility.SHA256 == "" {
		t.Fatalf("semantic/accessibility evidence = %+v / %+v", manifest.BeforeSemantics, manifest.BeforeAccessibility)
	}

	wantKinds := map[ReviewArtifactKind]bool{
		ReviewArtifactCleanPage: false, ReviewArtifactOverlayPage: false, ReviewArtifactBeforeCrop: false,
		ReviewArtifactAfterCrop: false, ReviewArtifactRasterDiff: false, ReviewArtifactContactSheet: false,
		ReviewArtifactSourceDiff: false, ReviewArtifactSemanticDiff: false, ReviewArtifactPlanDiff: false,
		ReviewArtifactAccessibilityDiff: false, ReviewArtifactDiagnostics: false,
	}
	var total uint64
	for index, artifact := range first.Artifacts() {
		wantKinds[artifact.Metadata.Kind] = true
		digest := sha256.Sum256(artifact.Bytes)
		if artifact.Metadata.Index != uint32(index) || artifact.Metadata.ByteLength != uint64(len(artifact.Bytes)) ||
			artifact.Metadata.SHA256 != hex.EncodeToString(digest[:]) || manifest.Artifacts[index].SHA256 != artifact.Metadata.SHA256 {
			t.Fatalf("artifact %d metadata = %+v", index, artifact.Metadata)
		}
		total += uint64(len(artifact.Bytes))
		if artifact.Metadata.Kind == ReviewArtifactBeforeCrop || artifact.Metadata.Kind == ReviewArtifactAfterCrop {
			if artifact.Metadata.CropBounds.IsEmpty() || len(artifact.Metadata.Fragments) != 1 || artifact.Metadata.PixelTransform == nil {
				t.Fatalf("crop metadata = %+v", artifact.Metadata)
			}
		}
		if artifact.Metadata.Kind == ReviewArtifactOverlayPage {
			if artifact.Metadata.Layer != ReviewLayerOverlay {
				t.Fatalf("overlay layer = %q", artifact.Metadata.Layer)
			}
			image, decodeErr := png.Decode(bytes.NewReader(artifact.Bytes))
			if decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if got := color.NRGBAModel.Convert(image.At(image.Bounds().Max.X-1, image.Bounds().Max.Y-1)).(color.NRGBA); got.A != 0 {
				t.Fatalf("overlay background is not transparent: %+v", got)
			}
		}
		if artifact.Metadata.Kind == ReviewArtifactContactSheet && len(artifact.Metadata.PageTransforms) != 1 {
			t.Fatalf("contact sheet transforms = %+v", artifact.Metadata.PageTransforms)
		}
		if artifact.Metadata.Kind == ReviewArtifactRasterDiff && artifact.Metadata.ChangedPixels == 0 {
			t.Fatal("raster diff did not record its exact changed-pixel count")
		}
	}
	for kind, found := range wantKinds {
		if !found {
			t.Errorf("missing artifact kind %q", kind)
		}
	}
	if manifest.TotalBytes != total {
		t.Fatalf("total bytes = %d, want %d", manifest.TotalBytes, total)
	}
	var decoded ReviewBundleManifest
	if err := json.Unmarshal(firstJSON, &decoded); err != nil || !reflect.DeepEqual(decoded, manifest) {
		t.Fatalf("manifest round trip = %v, %+v", err, decoded)
	}
	copyManifest := first.Manifest()
	copyArtifacts := first.Artifacts()
	copyManifest.Artifacts[0].Name = "mutated"
	copyArtifacts[0].Bytes[0] ^= 0xff
	if first.Manifest().Artifacts[0].Name == "mutated" || first.Artifacts()[0].Bytes[0] == copyArtifacts[0].Bytes[0] {
		t.Fatal("bundle exposed mutable storage")
	}
}

func TestBuildReviewBundlePaintOnlyChangeStillProducesPageEvidence(t *testing.T) {
	before, sources := rasterFixture(t)
	p := before.Projection()
	p.Fills[0].Color = CoreRGBColor{G: 255, Set: true}
	after := mustPlanFromProjection(t, p)
	bundle, err := BuildReviewBundle(context.Background(), before, after, sources, sources, reviewRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bundle.Manifest().ChangedPages, []uint32{1}) {
		t.Fatalf("paint-only changed pages = %v", bundle.Manifest().ChangedPages)
	}
	foundDiff := false
	for _, artifact := range bundle.Artifacts() {
		if artifact.Metadata.Kind == ReviewArtifactRasterDiff {
			foundDiff = true
		}
	}
	if !foundDiff {
		t.Fatal("paint-only change lacks raster diff")
	}
}

func TestBuildReviewBundleRejectsInvalidIdentityLimitsAndCancellationAtomically(t *testing.T) {
	before, sources := rasterFixture(t)
	after := reviewChangedFixture(t, before)
	tests := []struct {
		name   string
		mutate func(*ReviewBundleRequest)
		want   error
	}{
		{name: "zero limits", mutate: func(request *ReviewBundleRequest) { request.Limits = ReviewBundleLimits{} }, want: ErrReviewBundleRequest},
		{name: "partial revisions", mutate: func(request *ReviewBundleRequest) { request.AfterRevisions.PolicyRevision = "" }, want: ErrViewerIdentityInvalid},
		{name: "artifact count", mutate: func(request *ReviewBundleRequest) { request.Limits.MaxArtifacts = 1 }, want: ErrReviewBundleLimit},
		{name: "manifest bytes", mutate: func(request *ReviewBundleRequest) { request.Limits.MaxManifestBytes = 1 }, want: ErrReviewBundleLimit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := reviewRequest()
			test.mutate(&request)
			bundle, err := BuildReviewBundle(context.Background(), before, after, sources, sources, request)
			if !errors.Is(err, test.want) || len(bundle.Artifacts()) != 0 {
				t.Fatalf("bundle=%+v error=%v, want %v", bundle.Manifest(), err, test.want)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bundle, err := BuildReviewBundle(ctx, before, after, sources, sources, reviewRequest())
	if !errors.Is(err, context.Canceled) || len(bundle.Artifacts()) != 0 {
		t.Fatalf("canceled bundle=%+v error=%v", bundle.Manifest(), err)
	}
}

func reviewRequest() ReviewBundleRequest {
	return ReviewBundleRequest{
		BeforeRevisions: ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("1", 64), ScenarioRevision: strings.Repeat("3", 64), PolicyRevision: strings.Repeat("4", 64)},
		AfterRevisions:  ViewerRevisionIdentityInput{SourceRevision: strings.Repeat("2", 64), ScenarioRevision: strings.Repeat("3", 64), PolicyRevision: strings.Repeat("4", 64)},
		PageProfile:     strings.Repeat("5", 64), RasterProfile: DefaultDisplayRasterProfile(), RasterLimits: DefaultDisplayRasterLimits(),
		Limits: DefaultReviewBundleLimits(), IncludeContactSheet: true, ContactSheetColumns: 1,
	}
}

func reviewChangedFixture(t *testing.T, before LayoutPlan) LayoutPlan {
	t.Helper()
	p := before.Projection()
	p.Fragments[0].MarginBox.Width += Fixed(FixedScale)
	p.Fragments[0].BorderBox.Width += Fixed(FixedScale)
	p.Fragments[0].PaddingBox.Width += Fixed(FixedScale)
	p.Fragments[0].ContentBox.Width += Fixed(FixedScale)
	p.Fills[0].Color = CoreRGBColor{G: 255, Set: true}
	return mustPlanFromProjection(t, p)
}

func mustPlanFromProjection(t *testing.T, p LayoutPlanProjection) LayoutPlan {
	t.Helper()
	plan, err := NewLayoutPlan(LayoutPlanInput{DeterministicInputs: p.DeterministicInputs, Pages: p.Pages, Fragments: p.Fragments,
		Lines: p.Lines, PageRegions: p.PageRegions, GridTracks: p.GridTracks, Fonts: p.Fonts, GlyphRuns: p.GlyphRuns, ImageResources: p.ImageResources, Images: p.Images,
		Destinations: p.Destinations, Links: p.Links, Paths: p.Paths, Transforms: p.Transforms, Clips: p.Clips,
		Fills: p.Fills, Strokes: p.Strokes, Commands: p.Commands, Breaks: p.Breaks, Diagnostics: p.Diagnostics,
		SemanticNodes: p.SemanticNodes, SemanticFragments: p.SemanticFragments, ReadingOrder: p.ReadingOrder})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
