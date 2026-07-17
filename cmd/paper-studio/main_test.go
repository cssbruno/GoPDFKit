// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
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

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
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
		!bytes.Contains(response.Body, []byte(`class="inspector-disclosure overlay-disclosure"`)) ||
		!bytes.Contains(response.Body, []byte(`class="inspector-disclosure authoring-disclosure"`)) ||
		!bytes.Contains(response.Body, []byte(`id="review-contract"`)) ||
		!bytes.Contains(response.Body, []byte(`id="review-notes"`)) ||
		!bytes.Contains(response.Body, []byte(`id="baseline-state"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/rail-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="reading"`)) ||
		!bytes.Contains(response.Body, []byte(`id="overlap-picker"`)) ||
		!bytes.Contains(response.Body, []byte(`id="edit-controls"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/edit-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/mutation-gate.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/instance-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/inspection-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/provenance-model.js"`)) ||
		bytes.Contains(response.Body, []byte(`experiments-disclosure`)) ||
		bytes.Contains(response.Body, []byte(`src="/typed-experiment-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/review-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/tag-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/syntax-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/page-setup-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/issue-model.js"`)) ||
		!bytes.Contains(response.Body, []byte(`id="page-setup-controls"`)) ||
		!bytes.Contains(response.Body, []byte(`id="tag-tree"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="margin"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="padding"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="overflow"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="clips"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="collisions"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="instances"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="baselines"`)) ||
		!bytes.Contains(response.Body, []byte(`data-overlay="cells"`)) ||
		!bytes.Contains(response.Body, []byte(`id="page-image" role="img"`)) ||
		!bytes.Contains(response.Body, []byte(`<footer class="statusbar">`)) ||
		!bytes.Contains(response.Body, []byte(`id="verification-state"`)) ||
		!bytes.Contains(response.Body, []byte(`id="delivery-panel"`)) ||
		!bytes.Contains(response.Body, []byte(`id="page-label" class="page-label"`)) ||
		!bytes.Contains(response.Body, []byte(`id="zoom-out" aria-label="Zoom out"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/wasm_exec.js"`)) ||
		!bytes.Contains(response.Body, []byte(`src="/wasm-renderer.js"`)) ||
		bytes.Contains(response.Body, []byte(`id="preview-status"`)) ||
		bytes.Contains(response.Body, []byte(`class="canvas-toolbar"`)) ||
		bytes.Contains(response.Body, []byte(`status-dot`)) ||
		bytes.Contains(response.Body, []byte(`WASM plan preview`)) ||
		!strings.Contains(response.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
		!strings.Contains(response.Header.Get("Content-Security-Policy"), "script-src 'self' 'wasm-unsafe-eval'") ||
		strings.Contains(response.Header.Get("Content-Security-Policy"), "'unsafe-eval'") ||
		!strings.Contains(response.Header.Get("Content-Security-Policy"), "img-src 'self' data: blob:") ||
		response.Header.Get("Cross-Origin-Resource-Policy") != "same-origin" {
		t.Fatalf("index status/security/body = %d / %q / %s", response.StatusCode, response.Header, response.Body)
	}
	canvasStart := bytes.Index(response.Body, []byte(`<section id="canvas-region"`))
	inspectorStart := bytes.Index(response.Body, []byte(`<aside id="inspector"`))
	sourceStart := bytes.Index(response.Body, []byte(`<section id="source-panel"`))
	inspectionControls := bytes.Index(response.Body, []byte(`id="inspection-controls"`))
	if canvasStart < 0 || inspectorStart < 0 || sourceStart < 0 || inspectionControls < inspectorStart || inspectionControls >= sourceStart {
		t.Fatalf("inspection controls must live in the Inspector sidebar, not the document canvas")
	}
	railModel := studioRequest(t, handler, http.MethodGet, "/rail-model.js", nil, "")
	if railModel.StatusCode != http.StatusOK || !bytes.Contains(railModel.Body, []byte("baselineLabel")) ||
		!bytes.Contains(railModel.Body, []byte("fallbackPageSummary")) {
		t.Fatalf("rail model = %d / %s", railModel.StatusCode, railModel.Body)
	}
	javascript := studioRequest(t, handler, http.MethodGet, "/studio.js", nil, "")
	if javascript.StatusCode != http.StatusOK || !bytes.Contains(javascript.Body, []byte("renderSelectionRects")) ||
		!bytes.Contains(javascript.Body, []byte("renderInspectorRows")) ||
		bytes.Contains(javascript.Body, []byte("slice(0, 120)")) ||
		!bytes.Contains(javascript.Body, []byte("loadInspection")) ||
		!bytes.Contains(javascript.Body, []byte("renderInspectionOverlays")) ||
		!bytes.Contains(javascript.Body, []byte("renderBaseline")) ||
		!bytes.Contains(javascript.Body, []byte("selectRailIssue")) ||
		!bytes.Contains(javascript.Body, []byte("summary.change_kind")) ||
		!bytes.Contains(javascript.Body, []byte("topmost first")) ||
		!bytes.Contains(javascript.Body, []byte("visualMutationsLocked")) ||
		!bytes.Contains(javascript.Body, []byte("previewRevisionLocked")) ||
		!bytes.Contains(javascript.Body, []byte("loadWASMPage")) ||
		!bytes.Contains(javascript.Body, []byte("paintWASMCanvas")) ||
		!bytes.Contains(javascript.Body, []byte("PaperStudioSyntaxModel.highlight")) ||
		!bytes.Contains(javascript.Body, []byte("PaperStudioIssueModel.sourceAnnotations")) ||
		!bytes.Contains(javascript.Body, []byte("source-diagnostic")) ||
		!bytes.Contains(javascript.Body, []byte("commitPageSetup")) ||
		!bytes.Contains(javascript.Body, []byte("refreshPromise")) ||
		!bytes.Contains(javascript.Body, []byte("loadDeliveryStatus")) ||
		!bytes.Contains(javascript.Body, []byte("loadReview")) || !bytes.Contains(javascript.Body, []byte("submitReview")) ||
		bytes.Contains(javascript.Body, []byte("draft.orientation = 'portrait'")) ||
		bytes.Contains(javascript.Body, []byte("Apply page size")) ||
		!bytes.Contains(javascript.Body, []byte(".render?revision=")) ||
		bytes.Contains(javascript.Body, []byte("preview-status")) ||
		bytes.Contains(javascript.Body, []byte("WASM plan preview")) ||
		bytes.Contains(javascript.Body, []byte("loadSVG(page, 'display')")) ||
		!bytes.Contains(javascript.Body, []byte("await showPage(fragment.page)")) ||
		!bytes.Contains(javascript.Body, []byte("markOutlineKey(fragment.Key)")) {
		t.Fatalf("studio synchronization script = %d / %s", javascript.StatusCode, javascript.Body)
	}
	stylesheetVerification := studioRequest(t, handler, http.MethodGet, "/studio.css", nil, "")
	if stylesheetVerification.StatusCode != http.StatusOK || !bytes.Contains(stylesheetVerification.Body, []byte("verification-state.is-verified")) || !bytes.Contains(stylesheetVerification.Body, []byte("verification-state.is-stale")) || !bytes.Contains(stylesheetVerification.Body, []byte("delivery-export")) {
		t.Fatalf("verification status styles = %d / %s", stylesheetVerification.StatusCode, stylesheetVerification.Body)
	}
	editModel := studioRequest(t, handler, http.MethodGet, "/edit-model.js", nil, "")
	if editModel.StatusCode != http.StatusOK || !bytes.Contains(editModel.Body, []byte("buildPayload")) ||
		!bytes.Contains(editModel.Body, []byte("source_revision")) || !bytes.Contains(editModel.Body, []byte("plan_revision")) {
		t.Fatalf("edit model = %d / %s", editModel.StatusCode, editModel.Body)
	}
	mutationGate := studioRequest(t, handler, http.MethodGet, "/mutation-gate.js", nil, "")
	if mutationGate.StatusCode != http.StatusOK || !bytes.Contains(mutationGate.Body, []byte("visualMutationsLocked")) || !bytes.Contains(mutationGate.Body, []byte("revisionsLocked")) {
		t.Fatalf("mutation gate = %d / %s", mutationGate.StatusCode, mutationGate.Body)
	}
	instanceModel := studioRequest(t, handler, http.MethodGet, "/instance-model.js", nil, "")
	if instanceModel.StatusCode != http.StatusOK || !bytes.Contains(instanceModel.Body, []byte("classifyFragments")) ||
		!bytes.Contains(instanceModel.Body, []byte("is-instance-${kind}")) {
		t.Fatalf("instance model = %d / %s", instanceModel.StatusCode, instanceModel.Body)
	}
	inspectionModel := studioRequest(t, handler, http.MethodGet, "/inspection-model.js", nil, "")
	if inspectionModel.StatusCode != http.StatusOK || !bytes.Contains(inspectionModel.Body, []byte("baselineMarks")) ||
		!bytes.Contains(inspectionModel.Body, []byte("tableCellMarks")) || !bytes.Contains(inspectionModel.Body, []byte("gridTrackMarks")) || !bytes.Contains(inspectionModel.Body, []byte("pageRegionMarks")) ||
		!bytes.Contains(inspectionModel.Body, []byte("boxModelMarks")) ||
		!bytes.Contains(inspectionModel.Body, []byte("issueMarks")) ||
		!bytes.Contains(inspectionModel.Body, []byte("baseline ${line.index + 1}")) || !bytes.Contains(inspectionModel.Body, []byte("grid ${track.group}")) {
		t.Fatalf("inspection model = %d / %s", inspectionModel.StatusCode, inspectionModel.Body)
	}
	provenanceModel := studioRequest(t, handler, http.MethodGet, "/provenance-model.js", nil, "")
	if provenanceModel.StatusCode != http.StatusOK || !bytes.Contains(provenanceModel.Body, []byte("forFragments")) || !bytes.Contains(provenanceModel.Body, []byte("tokenLabel")) {
		t.Fatalf("provenance model = %d / %s", provenanceModel.StatusCode, provenanceModel.Body)
	}
	typedModel := studioRequest(t, handler, http.MethodGet, "/typed-experiment-model.js", nil, "")
	if typedModel.StatusCode != http.StatusOK || !bytes.Contains(typedModel.Body, []byte("function normalize")) || !bytes.Contains(typedModel.Body, []byte("breakLabel")) {
		t.Fatalf("typed experiment model = %d / %s", typedModel.StatusCode, typedModel.Body)
	}
	tagModel := studioRequest(t, handler, http.MethodGet, "/tag-model.js", nil, "")
	if tagModel.StatusCode != http.StatusOK || !bytes.Contains(tagModel.Body, []byte("final_serialized_pdf")) ||
		!bytes.Contains(tagModel.Body, []byte("content_marked")) || !bytes.Contains(tagModel.Body, []byte("normalize")) {
		t.Fatalf("tag model = %d / %s", tagModel.StatusCode, tagModel.Body)
	}
	syntaxModel := studioRequest(t, handler, http.MethodGet, "/syntax-model.js", nil, "")
	if syntaxModel.StatusCode != http.StatusOK || !bytes.Contains(syntaxModel.Body, []byte("function highlight")) ||
		!bytes.Contains(syntaxModel.Body, []byte("kind = 'keyword'")) || !bytes.Contains(syntaxModel.Body, []byte("escapeHTML")) {
		t.Fatalf("syntax model = %d / %s", syntaxModel.StatusCode, syntaxModel.Body)
	}
	pageSetupModel := studioRequest(t, handler, http.MethodGet, "/page-setup-model.js", nil, "")
	if pageSetupModel.StatusCode != http.StatusOK || !bytes.Contains(pageSetupModel.Body, []byte("buildPayload")) || !bytes.Contains(pageSetupModel.Body, []byte("A3")) {
		t.Fatalf("page setup model = %d / %s", pageSetupModel.StatusCode, pageSetupModel.Body)
	}
	issueModel := studioRequest(t, handler, http.MethodGet, "/issue-model.js", nil, "")
	if issueModel.StatusCode != http.StatusOK || !bytes.Contains(issueModel.Body, []byte("function format")) {
		t.Fatalf("issue model = %d / %s", issueModel.StatusCode, issueModel.Body)
	}
	wasmBootstrap := studioRequest(t, handler, http.MethodGet, "/wasm-renderer.js", nil, "")
	if wasmBootstrap.StatusCode != http.StatusOK || !bytes.Contains(wasmBootstrap.Body, []byte("WebAssembly.instantiateStreaming")) ||
		!bytes.Contains(wasmBootstrap.Body, []byte("PaperStudioWASMRenderer")) || !bytes.Contains(wasmBootstrap.Body, []byte("renderResponse")) {
		t.Fatalf("wasm bootstrap = %d / %s", wasmBootstrap.StatusCode, wasmBootstrap.Body)
	}
	wasmRuntime := studioRequest(t, handler, http.MethodGet, "/wasm_exec.js", nil, "")
	if wasmRuntime.StatusCode != http.StatusOK || !bytes.Contains(wasmRuntime.Body, []byte("globalThis.Go")) {
		t.Fatalf("wasm runtime = %d / %d bytes", wasmRuntime.StatusCode, len(wasmRuntime.Body))
	}
	wasmModule := studioRequest(t, handler, http.MethodGet, "/paper-studio.wasm", nil, "")
	if wasmModule.StatusCode != http.StatusOK || len(wasmModule.Body) < 8 || !bytes.Equal(wasmModule.Body[:4], []byte{'\x00', 'a', 's', 'm'}) {
		t.Fatalf("wasm module = %d / %d bytes", wasmModule.StatusCode, len(wasmModule.Body))
	}
	stylesheet := studioRequest(t, handler, http.MethodGet, "/studio.css", nil, "")
	if stylesheet.StatusCode != http.StatusOK || !bytes.Contains(stylesheet.Body, []byte("STALE PREVIEW")) ||
		!bytes.Contains(stylesheet.Body, []byte(".inspection-mark.is-reading")) ||
		!bytes.Contains(stylesheet.Body, []byte(".inspection-mark.is-instance-repeated")) ||
		!bytes.Contains(stylesheet.Body, []byte(".inspection-mark.is-baseline")) ||
		!bytes.Contains(stylesheet.Body, []byte(".inspection-mark.is-table-cell")) ||
		!bytes.Contains(stylesheet.Body, []byte(".overlap-picker")) ||
		!bytes.Contains(stylesheet.Body, []byte(".region-state.is-repeated")) ||
		!bytes.Contains(stylesheet.Body, []byte(".rail-badge.is-change")) ||
		!bytes.Contains(stylesheet.Body, []byte(".syntax-keyword")) ||
		!bytes.Contains(stylesheet.Body, []byte("white-space: pre")) ||
		bytes.Contains(stylesheet.Body, []byte(".canvas-toolbar")) ||
		bytes.Contains(stylesheet.Body, []byte(".preview-authority")) ||
		bytes.Contains(stylesheet.Body, []byte(".status-dot")) ||
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
	webRender := studioRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/page/1.render?revision=%s", workspace.Revision), nil, "")
	if webRender.StatusCode != http.StatusOK || webRender.Header.Get("Content-Type") != "application/vnd.gopdfkit.display-render" {
		t.Fatalf("web render payload = %d %q %s", webRender.StatusCode, webRender.Header, webRender.Body)
	}
	artifact, err := layoutengine.RenderWebDisplayPayload(t.Context(), webRender.Body)
	if err != nil || artifact.Manifest().PlanHash != workspace.Revision || artifact.Manifest().Page != 1 || len(artifact.PNG()) == 0 {
		t.Fatalf("web render evidence = %+v / %d bytes / %v", artifact.Manifest(), len(artifact.PNG()), err)
	}
	highDPI := studioRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/page/1.render?revision=%s&dpi=288", workspace.Revision), nil, "")
	highArtifact, highErr := layoutengine.RenderWebDisplayPayload(t.Context(), highDPI.Body)
	if highDPI.StatusCode != http.StatusOK || highErr != nil || highArtifact.Manifest().Profile.DPI != 288 || highArtifact.Manifest().PixelWidth <= artifact.Manifest().PixelWidth {
		t.Fatalf("high-DPI web render = %d / %+v / %v", highDPI.StatusCode, highArtifact.Manifest(), highErr)
	}
	badDPI := studioRequest(t, handler, http.MethodGet, fmt.Sprintf("/api/page/1.render?revision=%s&dpi=999", workspace.Revision), nil, "")
	if badDPI.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad render DPI status = %d", badDPI.StatusCode)
	}

	hit := postStudioJSON(t, handler, "/api/hit", map[string]any{"revision": workspace.Revision, "page": 1, "x_fixed": 10 * 1024, "y_fixed": 10 * 1024})
	if hit.StatusCode != http.StatusOK || !strings.Contains(hit.Body, `"Page":1`) {
		t.Fatalf("hit = %d %s", hit.StatusCode, hit.Body)
	}
	var hitEvidence struct {
		Commands  []struct{ HasFragmentProvenance bool }
		Fragments []struct{ Key string }
	}
	if err := json.Unmarshal([]byte(hit.Body), &hitEvidence); err != nil || len(hitEvidence.Fragments) == 0 || hitEvidence.Fragments[0].Key != "@message" {
		t.Fatalf("visible pixel hit evidence = %s / %v", hit.Body, err)
	}
	explain := postStudioJSON(t, handler, "/api/explain", map[string]any{"revision": workspace.Revision, "selector": map[string]any{"key": hitEvidence.Fragments[0].Key}})
	if explain.StatusCode != http.StatusOK || !strings.Contains(explain.Body, `"key":"@message"`) ||
		!strings.Contains(explain.Body, `"fragments":[`) || !strings.Contains(explain.Body, `"commands":[`) || !strings.Contains(explain.Body, `"page":1`) {
		t.Fatalf("explain = %d %s", explain.StatusCode, explain.Body)
	}
	inspection := postStudioJSON(t, handler, "/api/inspect", map[string]any{"revision": workspace.Revision, "page": 1})
	if inspection.StatusCode != http.StatusOK || !strings.Contains(inspection.Body, `"plan_hash":"`+workspace.Revision+`"`) ||
		!strings.Contains(inspection.Body, `"selector":{"page":1,"max_results":128}`) ||
		!strings.Contains(inspection.Body, `"border_box"`) || !strings.Contains(inspection.Body, `"content_box"`) ||
		!strings.Contains(inspection.Body, `"reading_order"`) || !strings.Contains(inspection.Body, `"semantics"`) || !strings.Contains(inspection.Body, `"provenance":`) || !strings.Contains(inspection.Body, `"computed_styles"`) {
		t.Fatalf("inspection = %d %s", inspection.StatusCode, inspection.Body)
	}
	typed := studioRequest(t, handler, http.MethodGet, "/api/typed-experiments?revision="+workspace.Revision, nil, "")
	if typed.StatusCode != http.StatusOK || !bytes.Contains(typed.Body, []byte(`"projection"`)) || !bytes.Contains(typed.Body, []byte(`"fixtures"`)) || !bytes.Contains(typed.Body, []byte(`"break_ledger"`)) {
		t.Fatalf("typed experiments = %d %s", typed.StatusCode, typed.Body)
	}
	staleTyped := studioRequest(t, handler, http.MethodGet, "/api/typed-experiments?revision=wrong", nil, "")
	if staleTyped.StatusCode != http.StatusConflict {
		t.Fatalf("stale typed experiment status = %d", staleTyped.StatusCode)
	}
	delivery := studioRequest(t, handler, http.MethodGet, "/api/delivery?revision="+workspace.Revision, nil, "")
	if delivery.StatusCode != http.StatusOK || !bytes.Contains(delivery.Body, []byte(`"preflight":{"status":"ready"`)) || !bytes.Contains(delivery.Body, []byte(`"pdf_verification":{"status":"verified"`)) || !bytes.Contains(delivery.Body, []byte(`"export":{"status":"ready"`)) || !bytes.Contains(delivery.Body, []byte(`"publish":{"status":"separate_authorized_capability"`)) {
		t.Fatalf("delivery status = %d %s", delivery.StatusCode, delivery.Body)
	}
	export := studioRequest(t, handler, http.MethodGet, "/api/export.pdf?revision="+workspace.Revision, nil, "")
	if export.StatusCode != http.StatusOK || export.Header.Get("Content-Type") != "application/pdf" || !bytes.HasPrefix(export.Body, []byte("%PDF-")) {
		t.Fatalf("export = %d %q %d", export.StatusCode, export.Header, len(export.Body))
	}
	staleDelivery := studioRequest(t, handler, http.MethodGet, "/api/delivery?revision=wrong", nil, "")
	if staleDelivery.StatusCode != http.StatusConflict {
		t.Fatalf("stale delivery status = %d", staleDelivery.StatusCode)
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
	staleRender := studioRequest(t, handler, http.MethodGet, "/api/page/1.render?revision=wrong", nil, "")
	if staleRender.StatusCode != http.StatusConflict {
		t.Fatalf("stale web render status = %d", staleRender.StatusCode)
	}
}

func TestPaperStudioInspectionRetainsRepeatedFragmentEvidence(t *testing.T) {
	var source strings.Builder
	source.WriteString("document @report:\n  page @sheet:\n    width: 180pt\n    height: 96pt\n    margin: 6pt\n    body @body:\n      table @ledger:\n        repeat-header: true\n        split: \"rows\"\n        table-track @left:\n          width: 84pt\n        table-track @right:\n          width: 84pt\n        table-header @head:\n          table-row @head-row:\n            cell @head-cell:\n              colspan: 2\n              text: \"REPEATED HEADER\"\n")
	for index := 0; index < 10; index++ {
		fmt.Fprintf(&source, "        table-row @row-%d:\n          cell @label-%d:\n            text: \"Row %d\"\n          cell @value-%d:\n            text: \"Value %d\"\n", index, index, index, index, index)
	}
	file := filepath.Join(t.TempDir(), "repeated.paper")
	if err := os.WriteFile(file, []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace := fetchStudioWorkspace(t, studio.routes())
	if workspace.Pages < 2 {
		t.Fatalf("repeated fixture pages = %d", workspace.Pages)
	}
	inspection := postStudioJSON(t, studio.routes(), "/api/inspect", map[string]any{"revision": workspace.Revision, "page": 2})
	if inspection.StatusCode != http.StatusOK || !strings.Contains(inspection.Body, `"repeated":true`) ||
		!strings.Contains(inspection.Body, `"region":"body"`) || !strings.Contains(inspection.Body, `"instance":"@typed-table-r1-c1"`) ||
		!strings.Contains(inspection.Body, `"semantic_ownership":{"owner":`) || !strings.Contains(inspection.Body, `"table_header":true`) {
		t.Fatalf("repeated inspection = %d %s", inspection.StatusCode, inspection.Body)
	}
}

func TestPaperStudioInspectionShowsBindingAndTokenProvenance(t *testing.T) {
	source := "document @report:\n" +
		"  theme: \"@print\"\n" +
		"  theme @print:\n" +
		"    token @font:\n      type: \"string\"\n      value: \"Courier\"\n" +
		"    token @size:\n      type: \"length\"\n      value: 11pt\n" +
		"  schema @invoice:\n" +
		"    field @total:\n      type: \"number\"\n" +
		"  page:\n    width: 160pt\n    height: 80pt\n    margin: 8pt\n" +
		"    body:\n      paragraph @message:\n" +
		"        bind: \"@invoice.total\"\n        font-token: \"font\"\n        size-token: \"size\"\n        text: \"Visible\"\n"
	file := filepath.Join(t.TempDir(), "provenance.paper")
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace := fetchStudioWorkspace(t, studio.routes())
	inspection := postStudioJSON(t, studio.routes(), "/api/inspect", map[string]any{"revision": workspace.Revision, "page": 1})
	if inspection.StatusCode != http.StatusOK || !strings.Contains(inspection.Body, `"bindings":[`) || !strings.Contains(inspection.Body, `"path":"@invoice.total"`) || !strings.Contains(inspection.Body, `"style_tokens":[`) || !strings.Contains(inspection.Body, `"token":"font"`) {
		t.Fatalf("provenance inspection = %d %s", inspection.StatusCode, inspection.Body)
	}
}

func TestPaperStudioInspectionShowsBreakLedger(t *testing.T) {
	file := filepath.Join(t.TempDir(), "breaks.paper")
	source := "document @report:\n  page @sheet:\n    width: 120pt\n    height: 56pt\n    margin: 6pt\n    body @body:\n      paragraph @first:\n        line-height: 30pt\n        text: \"first\"\n      paragraph @second:\n        text: \"second\"\n"
	if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	workspace := fetchStudioWorkspace(t, studio.routes())
	inspection := postStudioJSON(t, studio.routes(), "/api/inspect", map[string]any{"revision": workspace.Revision, "page": 1})
	if inspection.StatusCode != http.StatusOK || !strings.Contains(inspection.Body, `"breaks":[`) || !strings.Contains(inspection.Body, `"from_page":1`) {
		t.Fatalf("break ledger inspection = %d %s", inspection.StatusCode, inspection.Body)
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

func BenchmarkPaperStudioWarmWorkspaceAndVisibleRenderPayload(b *testing.B) {
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
	target := fmt.Sprintf("/api/page/1.render?revision=%s", workspace.Revision)
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
