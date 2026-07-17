// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/cssbruno/gopdfkit/document"
)

// PlanSnapshot is a detached identity projection for an immutable plan.
type PlanSnapshot struct {
	Handle           PlanHandle
	Revision         RevisionHandle
	Digest           string           `json:"revision"`
	Hash             string           `json:"plan_hash"`
	Pages            int              `json:"pages"`
	ExpiresAt        time.Time        `json:"expires_at"`
	Capability       CapabilityMode   `json:"capability"`
	DisclosureDomain DisclosureDomain `json:"disclosure_domain"`
}

type PlanReviewRequest struct {
	Before PlanHandle
	After  PlanHandle
	Review document.PaperReviewRequest
}

type PlanReviewResult struct {
	Before       PlanSnapshot
	After        PlanSnapshot
	ManifestJSON []byte
	Artifacts    []document.PaperReviewArtifact
}

// ReviewPlans returns one detached, bounded, deterministic visual evidence
// bundle for two exact retained plans. Both handles are independently scoped,
// authorized, revocable, and expiry checked before immutable plan use.
func (w *Workspace) ReviewPlans(ctx context.Context, request PlanReviewRequest) (PlanReviewResult, error) {
	if w == nil {
		return PlanReviewResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if request.Review.MaxPages == 0 || request.Review.MaxArtifacts == 0 ||
		request.Review.MaxTotalBytes == 0 || request.Review.MaxTotalBytes > uint64(w.limits.MaxRenderBytes) ||
		request.Review.MaxArtifactBytes == 0 || request.Review.MaxArtifactBytes > request.Review.MaxTotalBytes ||
		request.Review.MaxManifestBytes == 0 || request.Review.MaxManifestBytes > uint64(w.limits.MaxRenderBytes) {
		return PlanReviewResult{}, workspaceError("REVIEW_LIMIT", "review bounds are outside configured limits", ErrLimit)
	}
	before, err := w.plan(request.Before)
	if err != nil {
		return PlanReviewResult{}, err
	}
	after, err := w.plan(request.After)
	if err != nil {
		return PlanReviewResult{}, err
	}
	if before.disclosure != after.disclosure || before.partition != after.partition {
		return PlanReviewResult{}, workspaceError("REVIEW_PARTITION", "review plans belong to different security partitions", ErrWrongWorkspace)
	}
	bundle, err := after.plan.ReviewAgainst(ctx, before.plan, request.Review)
	if err != nil {
		return PlanReviewResult{}, workspaceError("INVALID_PLAN_REVIEW", "plan review was rejected", err)
	}
	artifacts := make([]document.PaperReviewArtifact, len(bundle.Artifacts))
	for index, artifact := range bundle.Artifacts {
		artifacts[index] = document.PaperReviewArtifact{MetadataJSON: append([]byte(nil), artifact.MetadataJSON...), Bytes: append([]byte(nil), artifact.Bytes...)}
	}
	return PlanReviewResult{Before: snapshotPlan(before), After: snapshotPlan(after),
		ManifestJSON: append([]byte(nil), bundle.ManifestJSON...), Artifacts: artifacts}, nil
}

func snapshotPlan(record *planRecord) PlanSnapshot {
	return PlanSnapshot{Handle: record.handle, Revision: record.revision,
		Digest: string(record.digest), Hash: record.plan.Hash(), Pages: record.plan.PageCount(), ExpiresAt: record.expires,
		Capability: CapabilityRender, DisclosureDomain: record.disclosure}
}

// CreatePlan derives and retains one immutable plan from the exact revision.
// Planning runs outside the workspace mutex; publication is atomic.
func (w *Workspace) CreatePlan(revision RevisionHandle) (PlanSnapshot, document.PaperPlanResult, error) {
	record, err := w.revision(revision)
	if err != nil {
		return PlanSnapshot{}, document.PaperPlanResult{}, err
	}
	if !record.parsed.OK() || !record.compiled.OK() {
		return PlanSnapshot{}, document.PaperPlanResult{}, workspaceError("INVALID_SOURCE", "source is not plannable", ErrInvalidSource)
	}
	plan, result, err := document.PlanPaperWithImports(record.file, record.source, document.PaperImportResolver(w.importResolver))
	if err != nil {
		return PlanSnapshot{}, result, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if _, lookupErr := w.revisionLocked(revision); lookupErr != nil {
		return PlanSnapshot{}, result, lookupErr
	}
	if len(w.plans) >= w.limits.MaxPlans {
		return PlanSnapshot{}, result, workspaceError("PLAN_LIMIT", "workspace plan capacity is exhausted", ErrLimit)
	}
	w.nextPlan++
	retained := &planRecord{handle: PlanHandle{value: w.newHandle(handlePlan, capabilityRender, w.nextPlan)},
		revision: revision, digest: record.revision, plan: plan, result: result, expires: w.now().Add(w.planTTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.plans[w.nextPlan] = retained
	return snapshotPlan(retained), clonePlanResult(result), nil
}

// OpenPlan returns detached plan identity metadata.
func (w *Workspace) OpenPlan(handle PlanHandle) (PlanSnapshot, error) {
	record, err := w.plan(handle)
	if err != nil {
		return PlanSnapshot{}, err
	}
	return snapshotPlan(record), nil
}

// ReleasePlan revokes one retained plan handle and immediately frees its
// workspace capacity. A concurrent operation that already acquired the
// immutable plan may finish safely; every later lookup receives
// ErrPlanNotFound. Source revisions are retained independently.
func (w *Workspace) ReleasePlan(handle PlanHandle) error {
	if w == nil {
		return workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.planLocked(handle); err != nil {
		return err
	}
	delete(w.plans, handle.value.serial)
	w.recordRevocationLocked(handle.value, revokedExplicitly, w.now())
	return nil
}

// PruneExpiredPlans removes expired capabilities and returns the number of
// reclaimed retained plans. It is deterministic under an injected clock.
func (w *Workspace) PruneExpiredPlans() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pruneExpiredPlansLocked(w.now())
}

func (w *Workspace) pruneExpiredPlansLocked(now time.Time) int {
	removed := 0
	for serial, record := range w.plans {
		if !record.expires.After(now) {
			delete(w.plans, serial)
			w.recordRevocationLocked(record.handle.value, revokedExpired, now)
			removed++
		}
	}
	return removed
}

type PlanQueryRequest struct {
	Plan     PlanHandle
	Selector document.PaperPlanSelector
}

type PlanJSONResult struct {
	Plan PlanSnapshot
	JSON []byte
}

// QueryPlan returns detached canonical structural-query JSON.
func (w *Workspace) QueryPlan(request PlanQueryRequest) (PlanJSONResult, error) {
	if request.Selector.MaxResults == 0 || int(request.Selector.MaxResults) > w.limits.MaxSearchResults {
		return PlanJSONResult{}, workspaceError("QUERY_LIMIT", "plan query result limit is outside configured bounds", ErrLimit)
	}
	record, err := w.plan(request.Plan)
	if err != nil {
		return PlanJSONResult{}, err
	}
	result, err := record.plan.Query(request.Selector)
	if err != nil {
		return PlanJSONResult{}, workspaceError("INVALID_PLAN_QUERY", "structural plan query was rejected", err)
	}
	return PlanJSONResult{Plan: snapshotPlan(record), JSON: result.JSON()}, nil
}

type PlanExplainRequest struct {
	Plan      PlanHandle
	Selectors []document.PaperPlanSelector
	MaxBytes  uint32
}

// ExplainPlan returns detached canonical causal-evidence JSON.
func (w *Workspace) ExplainPlan(request PlanExplainRequest) (PlanJSONResult, error) {
	if len(request.Selectors) == 0 || len(request.Selectors) > w.limits.MaxSearchResults ||
		request.MaxBytes == 0 || uint64(request.MaxBytes) > uint64(w.limits.MaxRenderBytes) {
		return PlanJSONResult{}, workspaceError("EXPLAIN_LIMIT", "plan explanation bounds are outside configured limits", ErrLimit)
	}
	for _, selector := range request.Selectors {
		if selector.MaxResults == 0 || int(selector.MaxResults) > w.limits.MaxSearchResults {
			return PlanJSONResult{}, workspaceError("EXPLAIN_LIMIT", "selector result limit is outside configured bounds", ErrLimit)
		}
	}
	record, err := w.plan(request.Plan)
	if err != nil {
		return PlanJSONResult{}, err
	}
	result, err := record.plan.Explain(request.Selectors, uint32(len(request.Selectors)), request.MaxBytes)
	if err != nil {
		return PlanJSONResult{}, workspaceError("INVALID_PLAN_EXPLAIN", "plan explanation was rejected", err)
	}
	return PlanJSONResult{Plan: snapshotPlan(record), JSON: result.JSON()}, nil
}

// PlanHitTestRequest selects one exact page coordinate in fixed 1/1024-point
// units. The retained plan itself remains opaque.
type PlanHitTestRequest struct {
	Plan   PlanHandle
	Page   uint32
	XFixed int64
	YFixed int64
}

// HitTestPlan returns detached, bounded geometry hits for one retained plan.
func (w *Workspace) HitTestPlan(request PlanHitTestRequest) (PlanJSONResult, error) {
	record, err := w.plan(request.Plan)
	if err != nil {
		return PlanJSONResult{}, err
	}
	result, err := record.plan.HitTest(request.Page, request.XFixed, request.YFixed)
	if err != nil {
		return PlanJSONResult{}, workspaceError("INVALID_PLAN_HIT_TEST", "plan hit test was rejected", err)
	}
	return PlanJSONResult{Plan: snapshotPlan(record), JSON: result.JSON()}, nil
}

type PlanPixelHitTestRequest struct {
	Plan   PlanHandle
	Raster document.PaperPlanPixelHitTestRequest
}

// HitTestPlanPixel maps one declared screenshot/crop pixel to exact page
// geometry without exposing or replanning the retained plan.
func (w *Workspace) HitTestPlanPixel(request PlanPixelHitTestRequest) (PlanJSONResult, error) {
	record, err := w.plan(request.Plan)
	if err != nil {
		return PlanJSONResult{}, err
	}
	result, err := record.plan.HitTestPixel(request.Raster)
	if err != nil {
		return PlanJSONResult{}, workspaceError("INVALID_PLAN_PIXEL_HIT_TEST", "plan pixel hit test was rejected", err)
	}
	return PlanJSONResult{Plan: snapshotPlan(record), JSON: result.JSON()}, nil
}

type PlanCaptureRequest struct {
	Plan    PlanHandle
	Capture document.PaperPlanCaptureRequest
}

type PlanCaptureResult struct {
	Plan         PlanSnapshot
	ManifestJSON []byte
	Artifacts    []document.PaperPlanArtifact
}

// CapturePlan returns detached deterministic SVG artifacts tied to one plan.
func (w *Workspace) CapturePlan(request PlanCaptureRequest) (PlanCaptureResult, error) {
	limits := request.Capture
	if limits.MaxCrops == 0 || int(limits.MaxCrops) > w.limits.MaxSearchResults ||
		limits.MaxTotalBytes == 0 || limits.MaxTotalBytes > uint64(w.limits.MaxRenderBytes) {
		return PlanCaptureResult{}, workspaceError("CAPTURE_LIMIT", "plan capture bounds are outside configured limits", ErrLimit)
	}
	record, err := w.plan(request.Plan)
	if err != nil {
		return PlanCaptureResult{}, err
	}
	capture, err := record.plan.Capture(request.Capture)
	if err != nil {
		return PlanCaptureResult{}, workspaceError("INVALID_PLAN_CAPTURE", "plan capture was rejected", err)
	}
	artifacts := make([]document.PaperPlanArtifact, len(capture.Artifacts))
	for index, artifact := range capture.Artifacts {
		artifacts[index] = document.PaperPlanArtifact{MetadataJSON: append([]byte(nil), artifact.MetadataJSON...), SVG: append([]byte(nil), artifact.SVG...)}
	}
	return PlanCaptureResult{Plan: snapshotPlan(record), ManifestJSON: append([]byte(nil), capture.ManifestJSON...), Artifacts: artifacts}, nil
}

// RenderPlan paints the exact retained plan without reparsing or replanning.
func (w *Workspace) RenderPlan(handle PlanHandle) (RenderResult, error) {
	record, err := w.plan(handle)
	if err != nil {
		return RenderResult{}, err
	}
	pdf, err := document.NewDocument(document.WithUnit(document.UnitPoint), document.WithDeterministicOutput())
	if err != nil {
		return RenderResult{Revision: record.revision}, fmt.Errorf("paperd: create render document: %w", err)
	}
	pipeline, err := pdf.WritePaperPlan(record.plan)
	if err != nil {
		return RenderResult{Revision: record.revision, Pipeline: pipeline}, err
	}
	var output bytes.Buffer
	bounded := &limitWriter{writer: &output, remaining: int64(w.limits.MaxRenderBytes)}
	if err := pdf.OutputWithOptions(bounded, document.OutputOptions{Deterministic: true}); err != nil {
		if errorsIsLimit(err) || bounded.exceeded {
			return RenderResult{Revision: record.revision, Pipeline: pipeline}, workspaceError("RENDER_LIMIT", "rendered PDF exceeds the configured byte limit", ErrLimit)
		}
		return RenderResult{Revision: record.revision, Pipeline: pipeline}, fmt.Errorf("paperd: output PDF: %w", err)
	}
	return RenderResult{Revision: record.revision, PDF: append([]byte(nil), output.Bytes()...), Pipeline: pipeline}, nil
}

func clonePlanResult(result document.PaperPlanResult) document.PaperPlanResult {
	result.Diagnostics = append([]document.PaperDiagnostic(nil), result.Diagnostics...)
	return result
}
