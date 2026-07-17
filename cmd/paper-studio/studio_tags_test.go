// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestPaperStudioInspectsTagsFromExactFinalPDFBytes(t *testing.T) {
	file := filepath.Join(t.TempDir(), "tags.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	workspace := fetchStudioWorkspace(t, handler)
	response := studioRequest(t, handler, http.MethodGet, "/api/pdf-tags?revision="+workspace.Revision, nil, "")
	var tags studioPDFTagsResponse
	if response.StatusCode != http.StatusOK || json.Unmarshal(response.Body, &tags) != nil {
		t.Fatalf("tag response = %d / %s", response.StatusCode, response.Body)
	}
	if tags.Evidence != "final_serialized_pdf" || tags.PlanRevision != workspace.Revision || tags.SourceRevision != workspace.SourceRevision ||
		!tags.Report.Passed || !tags.Report.Marked || tags.Report.PDFSHA256 == "" || tags.Report.MarkedContent == 0 || len(tags.Report.Nodes) == 0 {
		t.Fatalf("tag evidence = %+v", tags)
	}
	roles := map[string]bool{}
	for _, node := range tags.Report.Nodes {
		roles[node.Role] = true
	}
	if !roles["Document"] || !roles["P"] {
		t.Fatalf("final tag roles = %v", roles)
	}
	stale := studioRequest(t, handler, http.MethodGet, "/api/pdf-tags?revision=stale", nil, "")
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale tag inspection = %d / %s", stale.StatusCode, stale.Body)
	}
}
