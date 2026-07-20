// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/document"
)

func TestWorkspacePlanHandlesBindRevisionAndExposeDetachedTools(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	revision, err := workspace.CreateRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	plan, pipeline, err := workspace.CreatePlan(revision.Handle)
	if err != nil || plan.Revision != revision.Handle || plan.Digest != string(revision.Revision) ||
		plan.Hash == "" || plan.Pages != 1 || pipeline.Hash != plan.Hash {
		t.Fatalf("CreatePlan() = %#v, %#v, %v", plan, pipeline, err)
	}
	opened, err := workspace.OpenPlan(plan.Handle)
	if err != nil || opened != plan {
		t.Fatalf("OpenPlan() = %#v, %v", opened, err)
	}
	query, err := workspace.QueryPlan(PlanQueryRequest{Plan: plan.Handle,
		Selector: document.PaperPlanSelector{Key: "@intro", MaxResults: 8}})
	if err != nil || query.Plan.Hash != plan.Hash || !strings.Contains(string(query.JSON), `"key":"@intro"`) {
		t.Fatalf("QueryPlan() = %#v, %v", query, err)
	}
	query.JSON[0] = 'x'
	again, err := workspace.QueryPlan(PlanQueryRequest{Plan: plan.Handle,
		Selector: document.PaperPlanSelector{Key: "@intro", MaxResults: 8}})
	if err != nil || len(again.JSON) == 0 || again.JSON[0] == 'x' {
		t.Fatal("query result was not detached")
	}
	explanation, err := workspace.ExplainPlan(PlanExplainRequest{Plan: plan.Handle, MaxBytes: 1 << 20,
		Selectors: []document.PaperPlanSelector{{Key: "@intro", MaxResults: 8}}})
	if err != nil || !strings.Contains(string(explanation.JSON), `"plan_hash":"`+plan.Hash+`"`) {
		t.Fatalf("ExplainPlan() = %#v, %v", explanation, err)
	}
	hit, err := workspace.HitTestPlan(PlanHitTestRequest{
		Plan: plan.Handle, Page: 1, XFixed: 25 * 1024, YFixed: 25 * 1024,
	})
	if err != nil || hit.Plan.Hash != plan.Hash ||
		!strings.Contains(string(hit.JSON), `"Key":"@intro"`) {
		t.Fatalf("HitTestPlan() = %#v, %v", hit, err)
	}
	pixelHit, err := workspace.HitTestPlanPixel(PlanPixelHitTestRequest{Plan: plan.Handle,
		Raster: document.PaperPlanPixelHitTestRequest{Page: 1, PixelX: 24, PixelY: 24,
			PixelWidth: 612, PixelHeight: 792, CaptureWidth: 612 * 1024, CaptureHeight: 792 * 1024}})
	if err != nil || pixelHit.Plan.Hash != plan.Hash || !strings.Contains(string(pixelHit.JSON), `"Key":"@intro"`) {
		t.Fatalf("HitTestPlanPixel() = %#v, %v", pixelHit, err)
	}
	capture, err := workspace.CapturePlan(PlanCaptureRequest{Plan: plan.Handle, Capture: document.PaperPlanCaptureRequest{
		Mode: "geometry_svg", IncludeContactSheet: true, IncludeCrossPageStrip: true, ContactSheetColumns: 1,
		MaxPages: 4, MaxCrops: 8, MaxArtifactBytes: 1 << 20, MaxTotalBytes: 2 << 20, MaxManifestBytes: 128 << 10,
	}})
	if err != nil || capture.Plan.Hash != plan.Hash || len(capture.Artifacts) != 2 ||
		!bytes.Contains(capture.Artifacts[0].SVG, []byte("<svg")) ||
		!bytes.Contains(capture.Artifacts[1].SVG, []byte(`data-format="cross-page-strip"`)) ||
		!bytes.Contains(capture.ManifestJSON, []byte(`"kind":"cross_page_strip"`)) ||
		!bytes.Contains(capture.ManifestJSON, []byte(`"source_revision":"`+plan.Digest+`"`)) ||
		!bytes.Contains(capture.ManifestJSON, []byte(`"renderer_version":"layoutengine/geometry-svg@2"`)) ||
		!bytes.Contains(capture.ManifestJSON, []byte(`"resource_set_hash":"`)) {
		t.Fatalf("CapturePlan() artifacts=%d, err=%v", len(capture.Artifacts), err)
	}
	rendered, err := workspace.RenderPlan(plan.Handle)
	if err != nil || rendered.Revision != revision.Handle || !rendered.Pipeline.OK() || !bytes.HasPrefix(rendered.PDF, []byte("%PDF-")) {
		t.Fatalf("RenderPlan() = %#v, %v", rendered, err)
	}
}

func TestWorkspaceReviewPlansReturnsBoundedDetachedHeadlessEvidence(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxPlans: 4, MaxRenderBytes: 16 << 20})
	beforeRevision, err := workspace.CreateRevision("before.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	afterSource := strings.Replace(workspaceFixture, "Hello agent", "Hello human", 1)
	afterRevision, err := workspace.CreateRevision("after.paper", afterSource)
	if err != nil {
		t.Fatal(err)
	}
	beforePlan, _, err := workspace.CreatePlan(beforeRevision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	afterPlan, _, err := workspace.CreatePlan(afterRevision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	font, err := os.ReadFile("../../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatal(err)
	}
	request := document.DefaultPaperReviewRequest()
	request.CoreFontProgram = font
	request.SourceDiff = []byte("--- before.paper\n+++ after.paper\n@@\n-Hello agent\n+Hello human\n")
	request.MaxArtifactBytes, request.MaxTotalBytes, request.MaxManifestBytes = 4<<20, 12<<20, 1<<20
	result, err := workspace.ReviewPlans(context.Background(), PlanReviewRequest{Before: beforePlan.Handle, After: afterPlan.Handle, Review: request})
	if err != nil {
		t.Fatal(err)
	}
	if result.Before != beforePlan || result.After != afterPlan || len(result.Artifacts) < 8 ||
		!bytes.Contains(result.ManifestJSON, []byte(`"kind":"clean_page"`)) ||
		!bytes.Contains(result.ManifestJSON, []byte(`"kind":"overlay_page"`)) ||
		!bytes.Contains(result.ManifestJSON, []byte(`"kind":"raster_diff"`)) ||
		!bytes.Contains(result.ManifestJSON, []byte(`"kind":"accessibility_diff"`)) ||
		!bytes.Contains(result.ManifestJSON, []byte(`"before_scenario_revision":"`)) ||
		!bytes.Contains(result.ManifestJSON, []byte(`"after_scenario_revision":"`)) ||
		!bytes.Contains(result.ManifestJSON, []byte(`"resource_set_hash":"`)) {
		t.Fatalf("ReviewPlans() artifacts=%d manifest=%s", len(result.Artifacts), result.ManifestJSON)
	}
	manifestCopy := append([]byte(nil), result.ManifestJSON...)
	artifactCopy := append([]byte(nil), result.Artifacts[0].Bytes...)
	result.ManifestJSON[0] ^= 0xff
	result.Artifacts[0].Bytes[0] ^= 0xff
	again, err := workspace.ReviewPlans(context.Background(), PlanReviewRequest{Before: beforePlan.Handle, After: afterPlan.Handle, Review: request})
	if err != nil || !bytes.Equal(again.ManifestJSON, manifestCopy) || !bytes.Equal(again.Artifacts[0].Bytes, artifactCopy) {
		t.Fatal("review result was not detached or deterministic")
	}
	request.MaxTotalBytes = uint64(workspace.limits.MaxRenderBytes + 1)
	if _, err := workspace.ReviewPlans(context.Background(), PlanReviewRequest{Before: beforePlan.Handle, After: afterPlan.Handle, Review: request}); !errors.Is(err, ErrLimit) {
		t.Fatalf("oversized review error = %v", err)
	}
}

func TestWorkspacePlanHandleExpiryAndDeterministicPruning(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	workspace, err := NewWorkspaceWithOptions(WorkspaceOptions{
		Limits: Limits{MaxPlans: 1}, PlanTTL: time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := workspace.CreateRevision("expiry.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := workspace.CreatePlan(revision.Handle)
	if err != nil || !plan.ExpiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("CreatePlan() expiry = %v, %v", plan.ExpiresAt, err)
	}
	now = now.Add(time.Minute)
	if _, err := workspace.OpenPlan(plan.Handle); !errors.Is(err, ErrPlanExpired) {
		t.Fatalf("expired OpenPlan() error = %v", err)
	}
	if removed := workspace.PruneExpiredPlans(); removed != 1 {
		t.Fatalf("PruneExpiredPlans() = %d", removed)
	}
	if _, err := workspace.OpenPlan(plan.Handle); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("pruned OpenPlan() error = %v", err)
	}
	if _, _, err := workspace.CreatePlan(revision.Handle); err != nil {
		t.Fatalf("expired capacity was not reclaimed: %v", err)
	}
}

func TestWorkspaceRejectsInvalidPlanTTL(t *testing.T) {
	if _, err := NewWorkspaceWithOptions(WorkspaceOptions{PlanTTL: -time.Second}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("negative PlanTTL error = %v", err)
	}
	if _, err := NewWorkspaceWithOptions(WorkspaceOptions{PlanTTL: MaxPlanTTLHard + time.Second}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("oversized PlanTTL error = %v", err)
	}
}

func TestWorkspacePlanHandlesAreScopedAndBounded(t *testing.T) {
	first := mustWorkspace(t, Limits{MaxPlans: 1, MaxSearchResults: 4})
	second := mustWorkspace(t, Limits{})
	revision, _ := first.CreateRevision("report.paper", workspaceFixture)
	plan, _, err := first.CreatePlan(revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.OpenPlan(plan.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("cross-workspace OpenPlan() error = %v", err)
	}
	if _, _, err := first.CreatePlan(revision.Handle); !errors.Is(err, ErrLimit) {
		t.Fatalf("plan limit error = %v", err)
	}
	if _, err := first.QueryPlan(PlanQueryRequest{Plan: plan.Handle,
		Selector: document.PaperPlanSelector{Page: 1, MaxResults: 5}}); !errors.Is(err, ErrLimit) {
		t.Fatalf("query limit error = %v", err)
	}
	if _, err := second.HitTestPlan(PlanHitTestRequest{Plan: plan.Handle, Page: 1}); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("cross-workspace HitTestPlan() error = %v", err)
	}
	if _, err := first.HitTestPlan(PlanHitTestRequest{Plan: plan.Handle}); err == nil {
		t.Fatal("zero-page hit test unexpectedly succeeded")
	}
	if err := second.ReleasePlan(plan.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("cross-workspace ReleasePlan() error = %v", err)
	}
	if err := first.ReleasePlan(plan.Handle); err != nil {
		t.Fatal(err)
	}
	if _, err := first.OpenPlan(plan.Handle); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("released OpenPlan() error = %v", err)
	}
	if _, _, err := first.CreatePlan(revision.Handle); err != nil {
		t.Fatalf("released capacity was not reclaimed: %v", err)
	}
}
