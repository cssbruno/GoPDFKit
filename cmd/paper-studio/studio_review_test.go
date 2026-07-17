// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		len(projected.Comments) != 1 || !projected.Comments[0].Resolved || projected.Comments[0].Body != "Readable after formatting" || projected.Reference == nil || !projected.Reference.Calibrated {
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

func mustReadReviewTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Clone(data)
}
