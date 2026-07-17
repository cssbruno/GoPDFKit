// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const studioFixture = "document @report:\n" +
	"  title: \"Studio fixture\"\n" +
	"  language: \"en\"\n" +
	"  scenario @preview:\n" +
	"  page @sheet:\n" +
	"    width: 72pt\n" +
	"    height: 50pt\n" +
	"    margin: 8pt\n" +
	"    body @content:\n" +
	"      paragraph @message:\n" +
	"        font: \"Courier\"\n" +
	"        size: 10pt\n" +
	"        line-height: 8pt\n" +
	"        text @copy: \"A\\nB\\nC\\nD\\nE\"\n"

func TestPaperStudioServesRevisionBoundWorkspacePagesAndReadTools(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fixture.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()

	response := studioRequest(t, handler, http.MethodGet, "/", nil, "")
	if response.StatusCode != http.StatusOK || !bytes.Contains(response.Body, []byte("Paper Studio")) ||
		!bytes.Contains(response.Body, []byte(`id="inspection-layer"`)) ||
		!bytes.Contains(response.Body, []byte(`id="baseline-state"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/rail-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="reading"`)) ||
		!bytes.Contains(response.Body, []byte(`id="overlap-picker"`)) ||
		!bytes.Contains(response.Body, []byte(`id="edit-controls"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/edit-model.js"`)) ||
		!strings.Contains(response.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
		!strings.Contains(response.Header.Get("Content-Security-Policy"), "img-src 'self' data: blob:") ||
		response.Header.Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatalf("index status/security/body = %d / %q / %s", response.StatusCode, response.Header, response.Body)
	}
	railModel := studioRequest(t, handler, http.MethodGet, "/rail-model.js", nil, "")
	if railModel.StatusCode != http.StatusOK || !bytes.Contains(railModel.Body, []byte("baselineLabel")) ||
		!bytes.Contains(railModel.Body, []byte("fallbackPageSummary")) {
		t.Fatalf("rail model = %d / %s", railModel.StatusCode, railModel.Body)
	}
	javascript := studioRequest(t, handler, http.MethodGet, "/studio.js", nil, "")
	if javascript.StatusCode != http.StatusOK || !bytes.Contains(javascript.Body, []byte("renderSelectionRects")) ||
		!bytes.Contains(javascript.Body, []byte("loadInspection")) ||
		!bytes.Contains(javascript.Body, []byte("renderInspectionOverlays")) ||
		!bytes.Contains(javascript.Body, []byte("renderBaseline")) ||
		!bytes.Contains(javascript.Body, []byte("selectRailIssue")) ||
		!bytes.Contains(javascript.Body, []byte("summary.change_kind")) ||
		!bytes.Contains(javascript.Body, []byte("topmost first")) ||
		!bytes.Contains(javascript.Body, []byte("await showPage(fragment.page)")) ||
		!bytes.Contains(javascript.Body, []byte("markOutlineKey(fragment.Key)")) {
		t.Fatalf("studio synchronization script = %d / %s", javascript.StatusCode, javascript.Body)
	}
	editModel := studioRequest(t, handler, http.MethodGet, "/edit-model.js", nil, "")
	if editModel.StatusCode != http.StatusOK || !bytes.Contains(editModel.Body, []byte("buildPayload")) ||
		!bytes.Contains(editModel.Body, []byte("source_revision")) || !bytes.Contains(editModel.Body, []byte("plan_revision")) {
		t.Fatalf("edit model = %d / %s", editModel.StatusCode, editModel.Body)
	}
	stylesheet := studioRequest(t, handler, http.MethodGet, "/studio.css", nil, "")
	if stylesheet.StatusCode != http.StatusOK || !bytes.Contains(stylesheet.Body, []byte("STALE PREVIEW")) ||
		!bytes.Contains(stylesheet.Body, []byte(".inspection-mark.is-reading")) ||
		!bytes.Contains(stylesheet.Body, []byte(".overlap-picker")) ||
		!bytes.Contains(stylesheet.Body, []byte(".region-state.is-repeated")) ||
		!bytes.Contains(stylesheet.Body, []byte(".rail-badge.is-change")) ||
		!bytes.Contains(stylesheet.Body, []byte("pointer-events: none")) {
		t.Fatalf("studio stale-state stylesheet = %d / %s", stylesheet.StatusCode, stylesheet.Body)
	}

	workspace := fetchStudioWorkspace(t, handler)
	if workspace.Pages != 2 || workspace.PlanHash == "" || workspace.Revision != workspace.PlanHash || workspace.SourceRevision == "" || workspace.Preview != "plan_preview" ||
		!strings.Contains(workspace.Source, "@message") || len(workspace.AST) == 0 || len(workspace.Diagnostics) != 0 ||
		len(workspace.PageRail) != 2 || workspace.PageRail[0].Selector != "first" || workspace.PageRail[1].Selector != "even" ||
		workspace.Baseline.Status != "none" {
		t.Fatalf("workspace = %+v", workspace)
	}
	scenarioResponse := studioRequest(t, handler, http.MethodGet, "/api/workspace?scenario=%40preview", nil, "")
	var scenarioWorkspace studioWorkspaceResponse
	if scenarioResponse.StatusCode != http.StatusOK || json.Unmarshal(scenarioResponse.Body, &scenarioWorkspace) != nil ||
		scenarioWorkspace.Scenario != "@preview" || scenarioWorkspace.Pages != workspace.Pages || scenarioWorkspace.Revision == "" {
		t.Fatalf("scenario workspace = %d / %+v / %s", scenarioResponse.StatusCode, scenarioWorkspace, scenarioResponse.Body)
	}
	scenarioPage := studioRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/page/1.svg?revision=%s&scenario=%%40preview", scenarioWorkspace.Revision), nil, "")
	if scenarioPage.StatusCode != http.StatusOK || !bytes.Contains(scenarioPage.Body, []byte("<svg")) {
		t.Fatalf("scenario page = %d / %s", scenarioPage.StatusCode, scenarioPage.Body)
	}
	for _, suffix := range []string{".svg", ".geometry.svg"} {
		page := studioRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/page/1%s?revision=%s", suffix, workspace.Revision), nil, "")
		if page.StatusCode != http.StatusOK || page.Header.Get("Content-Type") != "image/svg+xml; charset=utf-8" || len(page.Body) == 0 || !bytes.Contains(page.Body, []byte("<svg")) {
			t.Fatalf("page %s = %d %q %d", suffix, page.StatusCode, page.Header, len(page.Body))
		}
	}

	hit := postStudioJSON(t, handler, "/api/hit", map[string]any{"revision": workspace.Revision, "page": 1, "x_fixed": 10 * 1024, "y_fixed": 10 * 1024})
	if hit.StatusCode != http.StatusOK || !strings.Contains(hit.Body, `"Page":1`) {
		t.Fatalf("hit = %d %s", hit.StatusCode, hit.Body)
	}
	explain := postStudioJSON(t, handler, "/api/explain", map[string]any{"revision": workspace.Revision, "selector": map[string]any{"key": "@message"}})
	if explain.StatusCode != http.StatusOK || !strings.Contains(explain.Body, `"key":"@message"`) ||
		!strings.Contains(explain.Body, `"fragments":[`) || !strings.Contains(explain.Body, `"page":1`) {
		t.Fatalf("explain = %d %s", explain.StatusCode, explain.Body)
	}
	inspection := postStudioJSON(t, handler, "/api/inspect", map[string]any{"revision": workspace.Revision, "page": 1})
	if inspection.StatusCode != http.StatusOK || !strings.Contains(inspection.Body, `"plan_hash":"`+workspace.Revision+`"`) ||
		!strings.Contains(inspection.Body, `"selector":{"page":1,"max_results":128}`) ||
		!strings.Contains(inspection.Body, `"border_box"`) || !strings.Contains(inspection.Body, `"content_box"`) ||
		!strings.Contains(inspection.Body, `"reading_order"`) || !strings.Contains(inspection.Body, `"semantics"`) {
		t.Fatalf("inspection = %d %s", inspection.StatusCode, inspection.Body)
	}
	staleInspection := postStudioJSON(t, handler, "/api/inspect", map[string]any{"revision": "wrong", "page": 1})
	if staleInspection.StatusCode != http.StatusConflict {
		t.Fatalf("stale inspection status = %d", staleInspection.StatusCode)
	}
	badInspection := postStudioJSON(t, handler, "/api/inspect", map[string]any{"revision": workspace.Revision, "page": 0})
	if badInspection.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad inspection status = %d", badInspection.StatusCode)
	}
	stale := studioRequest(t, handler, http.MethodGet, "/api/page/1.svg?revision=wrong", nil, "")
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale status = %d", stale.StatusCode)
	}
}

func TestPaperStudioListenAddressIsLoopbackOnly(t *testing.T) {
	for _, address := range []string{"127.0.0.1:7331", "localhost:7331", "[::1]:7331"} {
		if err := validateStudioListenAddress(address); err != nil {
			t.Errorf("validateStudioListenAddress(%q) = %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:7331", ":7331", "192.0.2.1:7331", "invalid"} {
		if err := validateStudioListenAddress(address); err == nil {
			t.Errorf("validateStudioListenAddress(%q) accepted a non-loopback address", address)
		}
	}
}

func TestPaperStudioReloadsAtomicallyAndRejectsMalformedOrConcurrentRequests(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fixture.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	first := fetchStudioWorkspace(t, handler)
	for index := 0; index < studioScenarioCacheLimit+8; index++ {
		if _, err := studio.current(context.Background(), fmt.Sprintf("@untrusted-%d", index)); err != nil {
			t.Fatal(err)
		}
	}
	if cached := len(studio.snapshots); cached > studioScenarioCacheLimit {
		t.Fatalf("scenario cache retained %d snapshots, limit %d", cached, studioScenarioCacheLimit)
	}

	const workers = 16
	var wait sync.WaitGroup
	errorsFound := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			workspace, fetchErr := fetchStudioWorkspaceResult(handler)
			if fetchErr != nil || workspace.Revision != first.Revision || workspace.Pages != first.Pages {
				errorsFound <- fmt.Errorf("workspace=%+v error=%v", workspace, fetchErr)
			}
		}()
	}
	wait.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Error(err)
	}

	changedSource := strings.Replace(studioFixture, "A\\nB", "Z\\nB", 1)
	if err := os.WriteFile(file, []byte(changedSource), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := fetchStudioWorkspace(t, handler)
	if changed.Revision == first.Revision || changed.Baseline.Status != "available" || changed.Baseline.Revision != first.Revision ||
		changed.Baseline.Scenario != "" || changed.Baseline.ChangedPageCount != 2 || changed.Baseline.RemovedPageCount != 0 ||
		len(changed.PageRail) != 2 || !changed.PageRail[0].Changed || changed.PageRail[0].ChangeKind != "modified" ||
		!changed.PageRail[1].Changed || changed.PageRail[1].ChangeKind != "modified" {
		t.Fatalf("changed page baseline = %+v", changed)
	}
	if studio.previous == nil || studio.previous.revision != first.Revision || studio.previous.plan.Hash() != first.Revision || len(studio.previous.pageSummary) != 2 {
		t.Fatalf("detached previous plan = %+v", studio.previous)
	}
	mismatchedResponse := studioRequest(t, handler, http.MethodGet, "/api/workspace?scenario=%40preview", nil, "")
	var mismatched studioWorkspaceResponse
	if mismatchedResponse.StatusCode != http.StatusOK || json.Unmarshal(mismatchedResponse.Body, &mismatched) != nil ||
		mismatched.Baseline.Status != "scenario_mismatch" || mismatched.Baseline.Revision != first.Revision ||
		mismatched.Baseline.Scenario != "" {
		t.Fatalf("scenario-mismatched baseline = %d / %+v / %s", mismatchedResponse.StatusCode, mismatched, mismatchedResponse.Body)
	}

	if err := os.WriteFile(file, []byte("document:\n  page\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	broken := fetchStudioWorkspace(t, handler)
	if broken.Revision == changed.Revision || broken.Pages != 0 || broken.Preview != "unavailable" || len(broken.Diagnostics) == 0 ||
		broken.Baseline.Status != "current_unavailable" || broken.Baseline.Revision != changed.Revision || len(broken.PageRail) != 0 {
		t.Fatalf("broken workspace = %+v", broken)
	}
	oldPage := studioRequest(t, handler, http.MethodGet, "/api/page/1.svg?revision="+first.Revision, nil, "")
	if oldPage.StatusCode != http.StatusConflict {
		t.Fatalf("old page after reload = %d", oldPage.StatusCode)
	}

	unknown := postStudioJSON(t, handler, "/api/hit", map[string]any{"revision": broken.Revision, "page": 1, "x_fixed": 1, "y_fixed": 1, "injected": true})
	if unknown.StatusCode != http.StatusBadRequest || !strings.Contains(unknown.Body, "unknown field") {
		t.Fatalf("unknown-field request = %d %s", unknown.StatusCode, unknown.Body)
	}
	method := studioRequest(t, handler, http.MethodPost, "/api/workspace", nil, "")
	if method.StatusCode != http.StatusMethodNotAllowed || method.Header.Get("Allow") != http.MethodGet {
		t.Fatalf("method response = %d allow=%q", method.StatusCode, method.Header.Get("Allow"))
	}
}

func BenchmarkPaperStudioWarmWorkspaceWithPageRailBaseline(b *testing.B) {
	file := filepath.Join(b.TempDir(), "fixture.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		b.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		b.Fatal(err)
	}
	handler := studio.routes()
	first, err := fetchStudioWorkspaceResult(handler)
	if err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(studioFixture, "A\\nB", "Z\\nB", 1)), 0o600); err != nil {
		b.Fatal(err)
	}
	changed, err := fetchStudioWorkspaceResult(handler)
	if err != nil || changed.Baseline.Status != "available" || changed.Baseline.Revision != first.Revision {
		b.Fatalf("warm baseline = %+v, %v", changed.Baseline, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		workspace, fetchErr := fetchStudioWorkspaceResult(handler)
		if fetchErr != nil || workspace.Baseline.Status != "available" || workspace.Baseline.ChangedPageCount != 2 {
			b.Fatalf("workspace baseline = %+v, %v", workspace.Baseline, fetchErr)
		}
	}
}

func BenchmarkPaperStudioWarmWorkspaceAndVisiblePage(b *testing.B) {
	file := filepath.Join(b.TempDir(), "fixture.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		b.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		b.Fatal(err)
	}
	handler := studio.routes()
	workspace, err := fetchStudioWorkspaceResult(handler)
	if err != nil {
		b.Fatal(err)
	}
	target := fmt.Sprintf("/api/page/1.svg?revision=%s", workspace.Revision)
	if response := studioRequest(nil, handler, http.MethodGet, target, nil, ""); response.StatusCode != http.StatusOK {
		b.Fatalf("warm page status = %d", response.StatusCode)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		workspaceResponse := studioRequest(nil, handler, http.MethodGet, "/api/workspace", nil, "")
		pageResponse := studioRequest(nil, handler, http.MethodGet, target, nil, "")
		if workspaceResponse.StatusCode != http.StatusOK || pageResponse.StatusCode != http.StatusOK {
			b.Fatalf("warm update status = workspace %d page %d", workspaceResponse.StatusCode, pageResponse.StatusCode)
		}
	}
}

type studioTestResponse struct {
	StatusCode int
	Body       string
}

func postStudioJSON(t *testing.T, handler http.Handler, path string, value any) studioTestResponse {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	response := studioRequest(t, handler, http.MethodPost, path, bytes.NewReader(encoded), "application/json")
	return studioTestResponse{response.StatusCode, string(response.Body)}
}

func fetchStudioWorkspace(t *testing.T, handler http.Handler) studioWorkspaceResponse {
	t.Helper()
	workspace, err := fetchStudioWorkspaceResult(handler)
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func fetchStudioWorkspaceResult(handler http.Handler) (studioWorkspaceResponse, error) {
	response := studioRequest(nil, handler, http.MethodGet, "/api/workspace", nil, "")
	if response.StatusCode != http.StatusOK {
		return studioWorkspaceResponse{}, fmt.Errorf("workspace status %d: %s", response.StatusCode, response.Body)
	}
	var workspace studioWorkspaceResponse
	if err := json.NewDecoder(bytes.NewReader(response.Body)).Decode(&workspace); err != nil {
		return studioWorkspaceResponse{}, err
	}
	return workspace, nil
}

type studioRecordedResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func studioRequest(t *testing.T, handler http.Handler, method, target string, body io.Reader, contentType string) studioRecordedResponse {
	request, err := http.NewRequest(method, "http://paper-studio.local"+target, body)
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		return studioRecordedResponse{StatusCode: http.StatusInternalServerError, Body: []byte(err.Error())}
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	recorder := &memoryResponseWriter{header: make(http.Header)}
	handler.ServeHTTP(recorder, request)
	return studioRecordedResponse{StatusCode: recorder.status, Header: recorder.header.Clone(), Body: append([]byte(nil), recorder.body.Bytes()...)}
}

type memoryResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *memoryResponseWriter) Header() http.Header { return w.header }
func (w *memoryResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *memoryResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}
