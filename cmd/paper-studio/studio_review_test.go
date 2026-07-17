// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
)

func TestPaperStudioReviewMetadataPersistsAndFollowsAuthoredIDs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "review.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	workspace := fetchStudioWorkspace(t, handler)
	transform := []float64{1, 0, 0, 1, 0, 0}
	annotation := postStudioJSON(t, handler, "/api/review", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision,
		"kind": "annotation", "target": "@copy", "page": 1,
		"x": 12, "y": 14, "width": 24, "height": 8, "transform": transform,
		"label": "check", "note": "keep this semantic target",
	})
	if annotation.StatusCode != http.StatusOK || !strings.Contains(annotation.Body, "review-") || !strings.Contains(annotation.Body, "keep this semantic target") {
		t.Fatalf("annotation = %d / %s", annotation.StatusCode, annotation.Body)
	}
	comment := postStudioJSON(t, handler, "/api/review", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision,
		"kind": "comment", "target": "@copy", "page": 1, "x": 12, "y": 14,
		"transform": transform, "author": "reviewer", "body": "Readable after formatting",
	})
	if comment.StatusCode != http.StatusOK || !strings.Contains(comment.Body, "Readable after formatting") {
		t.Fatalf("comment = %d / %s", comment.StatusCode, comment.Body)
	}
	reference := postStudioJSON(t, handler, "/api/review", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision,
		"kind": "reference", "page": 1, "width": 100, "height": 80,
		"reference_kind": "image/png", "reference_digest": strings.Repeat("a", 64), "transform": transform,
	})
	if reference.StatusCode != http.StatusOK || !strings.Contains(reference.Body, `"calibrated":true`) {
		t.Fatalf("reference = %d / %s", reference.StatusCode, reference.Body)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(mustReadReviewTestFile(t, file)), "font: \"Courier\"", "font: \"Helvetica\"", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	updated, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	updatedWorkspace := fetchStudioWorkspace(t, updated.routes())
	response := studioRequest(t, updated.routes(), http.MethodGet, "/api/review?revision="+updatedWorkspace.Revision+"&source_revision="+updatedWorkspace.SourceRevision, nil, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("review reload = %d / %s", response.StatusCode, response.Body)
	}
	var projected studioReviewResponse
	if err := json.Unmarshal(response.Body, &projected); err != nil {
		t.Fatal(err)
	}
	if len(projected.Annotations) != 1 || !projected.Annotations[0].Resolved || projected.Annotations[0].Page != 1 || len(projected.Annotations[0].Transform) != 6 ||
		len(projected.Comments) != 1 || !projected.Comments[0].Resolved || projected.Comments[0].Body != "Readable after formatting" || projected.Reference == nil || !projected.Reference.Calibrated || projected.Accessibility == nil || projected.Accessibility.Evidence != "final_serialized_pdf" {
		t.Fatalf("projected review = %+v", projected)
	}
	if projected.SourceRevision == workspace.SourceRevision || projected.Revision == workspace.Revision {
		t.Fatalf("review did not rebind to current exact revisions: %+v", projected)
	}

	stale := studioRequest(t, updated.routes(), http.MethodGet, "/api/review?revision="+workspace.Revision+"&source_revision="+workspace.SourceRevision, nil, "")
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale review status = %d / %s", stale.StatusCode, stale.Body)
	}
	if _, err := os.Stat(file + ".review.json"); err != nil {
		t.Fatalf("review sidecar = %v", err)
	}
}

func TestPaperStudioReviewCarriesExactScenarioAndAccessibilityCheck(t *testing.T) {
	file := filepath.Join(t.TempDir(), "scenario-review.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "@preview")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	workspace := fetchStudioWorkspace(t, handler)
	response := studioRequest(t, handler, http.MethodGet, "/api/review?revision="+workspace.Revision+"&source_revision="+workspace.SourceRevision+"&scenario=%40preview", nil, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("scenario review = %d / %s", response.StatusCode, response.Body)
	}
	var projected studioReviewResponse
	if err := json.Unmarshal(response.Body, &projected); err != nil {
		t.Fatal(err)
	}
	if projected.Scenario != "@preview" || projected.Accessibility == nil || projected.Accessibility.Status == "" || projected.Accessibility.Evidence != "final_serialized_pdf" {
		t.Fatalf("scenario/accessibility review = %+v", projected)
	}
}

func mustReadReviewTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(data)
}

func TestPaperStudioReviewReferenceImageDiffAndArtifacts(t *testing.T) {
	file := filepath.Join(t.TempDir(), "reference.paper")
	if err := os.WriteFile(file, []byte(studioCanvasFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	workspace := fetchStudioWorkspace(t, handler)
	snapshot, err := studio.current(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	raster, err := snapshot.plan.CaptureRasterPages(context.Background(), document.DefaultPaperPlanRasterRequest())
	if err != nil || len(raster.Pages) == 0 {
		t.Fatalf("reference raster = %d pages / %v", len(raster.Pages), err)
	}
	referencePNG := raster.Pages[0].PNG
	config, _, err := image.DecodeConfig(bytes.NewReader(referencePNG))
	if err != nil {
		t.Fatal(err)
	}
	referenceHash := sha256.Sum256(referencePNG)
	response := postStudioJSON(t, handler, "/api/review", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision,
		"kind": "reference", "page": 1, "width": config.Width, "height": config.Height,
		"reference_kind": "image/png", "reference_digest": hex.EncodeToString(referenceHash[:]),
		"reference_data_base64": base64.StdEncoding.EncodeToString(referencePNG), "transform": []float64{1, 0, 0, 1, 0, 0},
	})
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Body, `"diff_status":"verified"`) || !strings.Contains(response.Body, `"changed_pixels":0`) {
		t.Fatalf("verified reference = %d / %s", response.StatusCode, response.Body)
	}
	query := "?revision=" + workspace.Revision + "&source_revision=" + workspace.SourceRevision
	stored := studioRequest(t, handler, http.MethodGet, "/api/review/reference"+query, nil, "")
	if stored.StatusCode != http.StatusOK || !bytes.Equal(stored.Body, referencePNG) || stored.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("stored reference = %d %q %d bytes", stored.StatusCode, stored.Header, len(stored.Body))
	}
	overlay := studioRequest(t, handler, http.MethodGet, "/api/review/reference"+query+"&artifact=overlay", nil, "")
	if overlay.StatusCode != http.StatusBadRequest {
		t.Fatalf("removed reference overlay artifact = %d %q", overlay.StatusCode, overlay.Header)
	}
	diff := studioRequest(t, handler, http.MethodGet, "/api/review/reference"+query+"&artifact=diff", nil, "")
	if diff.StatusCode != http.StatusOK || len(diff.Body) == 0 || diff.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("stored reference diff = %d %q %d bytes", diff.StatusCode, diff.Header, len(diff.Body))
	}
}

func TestPaperStudioReviewReferencePDFDiffUsesPinnedRasterizer(t *testing.T) {
	binary, err := exec.LookPath("pdftoppm")
	if err != nil {
		t.Skip("pdftoppm is unavailable")
	}
	version := exec.Command(binary, "-v")
	versionOutput, err := version.CombinedOutput()
	if err != nil || !strings.Contains(string(versionOutput), "pdftoppm version 26.05.0") {
		t.Skipf("pdftoppm 26.05.0 is unavailable: %s", strings.TrimSpace(string(versionOutput)))
	}
	file := filepath.Join(t.TempDir(), "reference-pdf.paper")
	if err := os.WriteFile(file, []byte(studioCanvasFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	workspace := fetchStudioWorkspace(t, handler)
	snapshot, err := studio.current(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	pdf, err := renderStudioTaggedPDF(context.Background(), snapshot.plan)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(pdf)
	response := postStudioJSON(t, handler, "/api/review", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision,
		"kind": "reference", "page": 1, "width": 1, "height": 1,
		"reference_kind": "application/pdf", "reference_digest": hex.EncodeToString(digest[:]),
		"reference_data_base64": base64.StdEncoding.EncodeToString(pdf), "transform": []float64{1, 0, 0, 1, 0, 0},
	})
	if response.StatusCode != http.StatusOK || !strings.Contains(response.Body, `"diff_status":"verified"`) || !strings.Contains(response.Body, `"changed_pixels":0`) {
		t.Fatalf("verified PDF reference = %d / %s", response.StatusCode, response.Body)
	}
	query := "?revision=" + workspace.Revision + "&source_revision=" + workspace.SourceRevision
	stored := studioRequest(t, handler, http.MethodGet, "/api/review/reference"+query, nil, "")
	if stored.StatusCode != http.StatusOK || !bytes.Equal(stored.Body, pdf) || stored.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("stored PDF reference = %d %q %d bytes", stored.StatusCode, stored.Header, len(stored.Body))
	}
}
