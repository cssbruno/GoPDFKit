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
	"sync"
	"testing"
)

const studioGridFixture = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      row @grid:\n" +
	"        gap: 4pt\n" +
	"        paragraph @left:\n" +
	"          track: \"fixed\"\n" +
	"          track-size: 40pt\n" +
	"          text @left-copy: \"Left\"\n" +
	"        paragraph @right:\n" +
	"          track: \"fraction\"\n" +
	"          track-weight: 1\n" +
	"          text @right-copy: \"Right\"\n"

const studioBoxFixture = "document @report:\n  page @sheet:\n    width: 160pt\n    height: 100pt\n    margin: 10pt\n    body @content:\n      paragraph @message:\n        text: \"Box\"\n"

const studioInvalidFontFixture = "document @report:\n  page @sheet:\n    width: 160pt\n    height: 100pt\n    margin: 10pt\n    body @content:\n      paragraph @message:\n        font: \"Unavailable Sans\"\n        text: \"Strict font\"\n"

func TestPaperStudioAppliesExactPageSize(t *testing.T) {
	file := filepath.Join(t.TempDir(), "page-size.paper")
	if err := os.WriteFile(file, []byte(studioBoxFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "page-size", "target": "@sheet", "property": "size",
		"width_points": 612, "height_points": 792,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page size edit = %d %s", response.StatusCode, response.Body)
	}
	after := fetchStudioWorkspace(t, handler)
	if !strings.Contains(after.Source, "width: 612pt") || !strings.Contains(after.Source, "height: 792pt") || after.Revision == before.Revision {
		t.Fatalf("page size source = %s", after.Source)
	}
}

func TestPaperStudioOffersExplicitFontRepairAgainstUnavailablePlan(t *testing.T) {
	file := filepath.Join(t.TempDir(), "invalid-font.paper")
	if err := os.WriteFile(file, []byte(studioInvalidFontFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	if before.Pages != 0 || len(before.Diagnostics) != 1 || before.Diagnostics[0].Code != "PAPER_COMPILE_FONT" {
		t.Fatalf("strict unavailable-font workspace = %+v", before)
	}
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "text", "target": "@message", "property": "font", "text": "Helvetica",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("font replacement = %d %s", response.StatusCode, response.Body)
	}
	after := fetchStudioWorkspace(t, handler)
	written, _ := os.ReadFile(file)
	if after.Pages != 1 || len(after.Diagnostics) != 0 || !strings.Contains(string(written), `font: "Helvetica"`) {
		t.Fatalf("repaired workspace = %+v / %s", after, written)
	}
}

const studioImageFixture = "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n" +
	"        source: \"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==\"\n" +
	"        width: 40pt\n        height: 24pt\n        fit: \"cover\"\n        alt: \"Pixel\"\n"

const studioTableFixture = "document @report:\n  language: \"en\"\n  page @sheet:\n    width: 200pt\n    height: 120pt\n    margin: 8pt\n    body @body:\n      table @ledger:\n" +
	"        repeat-header: true\n        split: \"rows\"\n        table-track @name-track:\n          width: 100pt\n" +
	"        table-track @value-track:\n          width: 84pt\n" +
	"        table-header @head:\n          table-row @head-row:\n            cell @name-head:\n              text: \"Name\"\n            cell @value-head:\n              text: \"Value\"\n" +
	"        table-row @body-row:\n          cell @name:\n            text: \"Alpha\"\n          cell @value:\n            text: \"10\"\n"

const studioCanvasFixture = "document @report:\n  language: \"en\"\n  page @sheet:\n    width: 200pt\n    height: 120pt\n    margin: 12pt\n    body @body:\n      canvas @diagram:\n        width: 160pt\n        height: 80pt\n        anchor @base:\n          width: 40pt\n          height: 20pt\n          left: \"canvas.left + 8pt\"\n          top: \"canvas.top + 8pt\"\n          background: \"#336699\"\n        anchor @badge:\n          width: 24pt\n          height: 12pt\n          left: \"@base.right + 6pt\"\n          top: \"@base.top\"\n          background: \"#cc3300\"\n"

func TestPaperStudioCanvasEditGuardsGoverningCanvasAndChangesCapture(t *testing.T) {
	file := filepath.Join(t.TempDir(), "canvas.paper")
	if err := os.WriteFile(file, []byte(studioCanvasFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	beforeCapture := studioPageCapture(t, handler, before.Revision)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "canvas", "target": "@badge", "property": "left",
		"text": "@base", "kind": "right", "points": 12,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("canvas edit = %d %s", response.StatusCode, response.Body)
	}
	var result studioEditResponse
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	effects := map[string]bool{}
	for _, effect := range result.Authorization.Effects {
		effects[effect.Node] = true
	}
	if !result.Authorization.Allowed || !effects["@badge"] || !effects["@diagram"] || result.PatchCount != 1 {
		t.Fatalf("canvas authorization = %+v", result)
	}
	if afterCapture := studioPageCapture(t, handler, result.PlanRevision); bytes.Equal(beforeCapture, afterCapture) {
		t.Fatal("canvas edit did not change the exact page SVG capture")
	}
	written, _ := os.ReadFile(file)
	if !strings.Contains(string(written), `left: "@base.right + 12pt"`) {
		t.Fatalf("canvas source = %s", written)
	}
}

const studioRegionFixture = "document @report:\n  language: \"en\"\n  page @sheet:\n    width: 200pt\n    height: 120pt\n    margin: 12pt\n    header @head:\n      paragraph @head-copy:\n        text: \"Header\"\n    body @body:\n      paragraph @copy:\n        text: \"Body\"\n"

func TestPaperStudioRegionEditGuardsPageAndChangesCapture(t *testing.T) {
	file := filepath.Join(t.TempDir(), "region.paper")
	if err := os.WriteFile(file, []byte(studioRegionFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	beforeCapture := studioPageCapture(t, handler, before.Revision)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "region", "target": "@head", "property": "background", "color": "#aabbcc",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("region edit = %d %s", response.StatusCode, response.Body)
	}
	var result studioEditResponse
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	effects := map[string]bool{}
	for _, effect := range result.Authorization.Effects {
		effects[effect.Node] = true
	}
	if !effects["@head"] || !effects["@sheet"] || result.PatchCount != 1 {
		t.Fatalf("region result = %+v", result)
	}
	if afterCapture := studioPageCapture(t, handler, result.PlanRevision); bytes.Equal(beforeCapture, afterCapture) {
		t.Fatal("region edit did not change the exact page SVG capture")
	}
}

func TestPaperStudioFlowEditMovesNodeThroughExactDropDestination(t *testing.T) {
	file := filepath.Join(t.TempDir(), "flow.paper")
	source := "document @report:\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      row @grid:\n" +
		"        paragraph @left:\n" +
		"          text: \"Left\"\n" +
		"      paragraph @right:\n" +
		"        text: \"Right\"\n"
	if err := os.WriteFile(file, []byte(source), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "flow", "target": "@right", "property": "destination", "new_parent": "@grid",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("flow edit = %d %s", response.StatusCode, response.Body)
	}
	var result studioEditResponse
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Authorization.Allowed || result.PatchCount != 1 {
		t.Fatalf("flow result = %+v", result)
	}
	after := fetchStudioWorkspace(t, handler)
	if !strings.Contains(after.Source, "        paragraph @right:") || !strings.Contains(after.Source, "          text: \"Right\"") {
		t.Fatalf("flow source =\n%s", after.Source)
	}
}

func TestPaperStudioPageTemplateBootstrapsParseableDocument(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bootstrap.paper")
	if err := os.WriteFile(file, []byte("document @report:\n  title: \"Bootstrap\"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "template", "target": "@report", "template": "page", "id": "@sheet",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap edit = %d %s", response.StatusCode, response.Body)
	}
	after := fetchStudioWorkspace(t, handler)
	if after.Pages != 1 || !strings.Contains(after.Source, "page @sheet:") || !strings.Contains(after.Source, "body @sheet-body:") {
		t.Fatalf("bootstrap workspace = %+v", after)
	}
}

const studioAuthoringFixture = "document @report:\n" +
	"  # preserve author note\n" +
	"  schema @invoice:\n" +
	"    field @total:\n" +
	"      type: \"number\"\n" +
	"    field @customer:\n" +
	"      type: \"string\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      paragraph @copy:\n" +
	"        text: \"Invoice\"\n"

func TestPaperStudioAuthoringMetadataAndJournaledCreateToConnectFlow(t *testing.T) {
	file := filepath.Join(t.TempDir(), "authoring.paper")
	if err := os.WriteFile(file, []byte(studioAuthoringFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	metadataResponse := studioRequest(t, handler, http.MethodGet, "/api/authoring?revision="+before.Revision, nil, "")
	if metadataResponse.StatusCode != http.StatusOK {
		t.Fatalf("metadata = %d %s", metadataResponse.StatusCode, metadataResponse.Body)
	}
	var metadata studioAuthoringResponse
	if err := json.Unmarshal(metadataResponse.Body, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Revision != before.Revision || metadata.SourceRevision != before.SourceRevision || metadata.DocumentTarget != "@report" || len(metadata.Schemas) != 1 || len(metadata.Schemas[0].Fields) != 2 || len(metadata.StressPresets) != 3 {
		t.Fatalf("metadata = %+v", metadata)
	}

	template := map[string]any{"source_revision": before.SourceRevision, "plan_revision": before.Revision, "operation": "template", "target": "@body", "property": "", "template": "paragraph", "id": "@summary"}
	created := postStudioJSON(t, handler, "/api/edit", template)
	if created.StatusCode != http.StatusOK {
		t.Fatalf("template = %d %s", created.StatusCode, created.Body)
	}
	if stale := postStudioJSON(t, handler, "/api/edit", template); stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale template = %d %s", stale.StatusCode, stale.Body)
	}
	afterTemplate := fetchStudioWorkspace(t, handler)
	if !strings.Contains(afterTemplate.Source, "# preserve author note") || !strings.Contains(afterTemplate.Source, "paragraph @summary:") {
		t.Fatalf("template source:\n%s", afterTemplate.Source)
	}
	schema := map[string]any{"source_revision": afterTemplate.SourceRevision, "plan_revision": afterTemplate.Revision, "operation": "schema", "target": "@report", "property": "", "id": "@receipt"}
	createdSchema := postStudioJSON(t, handler, "/api/edit", schema)
	if createdSchema.StatusCode != http.StatusOK {
		t.Fatalf("schema = %d %s", createdSchema.StatusCode, createdSchema.Body)
	}
	afterSchema := fetchStudioWorkspace(t, handler)
	if !strings.Contains(afterSchema.Source, "schema @receipt:") || !strings.Contains(afterSchema.Source, "field @receipt-value:") {
		t.Fatalf("schema source:\n%s", afterSchema.Source)
	}

	binding := map[string]any{"source_revision": afterSchema.SourceRevision, "plan_revision": afterSchema.Revision, "operation": "binding", "target": "@copy", "property": "", "path": "@invoice.total", "required": true, "format": "decimal", "format_locale": "pt-BR", "format_min_fraction": 2, "format_max_fraction": 2}
	bound := postStudioJSON(t, handler, "/api/edit", binding)
	if bound.StatusCode != http.StatusOK {
		t.Fatalf("binding = %d %s", bound.StatusCode, bound.Body)
	}
	afterBinding := fetchStudioWorkspace(t, handler)
	if !strings.Contains(afterBinding.Source, "bind: \"@invoice.total\"") || !strings.Contains(afterBinding.Source, "format: \"decimal\"") || !strings.Contains(afterBinding.Source, "format-locale: \"pt-BR\"") {
		t.Fatalf("binding source:\n%s", afterBinding.Source)
	}

	scenario := map[string]any{"source_revision": afterBinding.SourceRevision, "plan_revision": afterBinding.Revision, "operation": "scenario-create", "target": "@report", "property": "", "id": "@stress", "schema": "@invoice", "preset": "stress"}
	stressed := postStudioJSON(t, handler, "/api/edit", scenario)
	if stressed.StatusCode != http.StatusOK {
		t.Fatalf("scenario = %d %s", stressed.StatusCode, stressed.Body)
	}
	afterScenario := fetchStudioWorkspace(t, handler)
	if !strings.Contains(afterScenario.Source, "scenario @stress:") || !strings.Contains(afterScenario.Source, "value @total: 999999.99") {
		t.Fatalf("scenario source:\n%s", afterScenario.Source)
	}
	fetchScenario := func(name string) studioWorkspaceResponse {
		response := studioRequest(t, handler, http.MethodGet, "/api/workspace?scenario="+name, nil, "")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("scenario workspace %s = %d %s", name, response.StatusCode, response.Body)
		}
		var workspace studioWorkspaceResponse
		if err := json.Unmarshal(response.Body, &workspace); err != nil {
			t.Fatal(err)
		}
		return workspace
	}
	selectedScenario := fetchScenario("%40stress")
	renameScenario := map[string]any{"source_revision": selectedScenario.SourceRevision, "plan_revision": selectedScenario.Revision, "scenario": "@stress", "operation": "scenario", "target": "@stress", "property": "rename", "id": "@reviewed"}
	renamedResponse := postStudioJSON(t, handler, "/api/edit", renameScenario)
	if renamedResponse.StatusCode != http.StatusOK || !strings.Contains(string(renamedResponse.Body), `"scenario":"@reviewed"`) {
		t.Fatalf("scenario rename = %d %s", renamedResponse.StatusCode, renamedResponse.Body)
	}
	afterRename := fetchScenario("%40reviewed")
	if !strings.Contains(afterRename.Source, "scenario @reviewed:") || strings.Contains(afterRename.Source, "scenario @stress:") {
		t.Fatalf("renamed scenario source:\n%s", afterRename.Source)
	}
	deleteScenario := map[string]any{"source_revision": afterRename.SourceRevision, "plan_revision": afterRename.Revision, "scenario": "@reviewed", "operation": "scenario", "target": "@reviewed", "property": "delete"}
	deletedResponse := postStudioJSON(t, handler, "/api/edit", deleteScenario)
	if deletedResponse.StatusCode != http.StatusOK || !strings.Contains(string(deletedResponse.Body), `"scenario":""`) {
		t.Fatalf("scenario delete = %d %s", deletedResponse.StatusCode, deletedResponse.Body)
	}
	afterLifecycle := fetchStudioWorkspace(t, handler)
	if strings.Contains(afterLifecycle.Source, "scenario @reviewed:") {
		t.Fatalf("deleted scenario source:\n%s", afterLifecycle.Source)
	}
	delivery := studioRequest(t, handler, http.MethodGet, "/api/delivery?revision="+afterLifecycle.Revision, nil, "")
	if delivery.StatusCode != http.StatusOK || !strings.Contains(string(delivery.Body), `"status":"verified"`) || !strings.Contains(string(delivery.Body), `"publish":{"status":"separate_authorized_capability"`) {
		t.Fatalf("create-to-deliver status = %d %s", delivery.StatusCode, delivery.Body)
	}
	export := studioRequest(t, handler, http.MethodGet, "/api/export.pdf?revision="+afterLifecycle.Revision, nil, "")
	if export.StatusCode != http.StatusOK || export.Header.Get("Content-Type") != "application/pdf" || len(export.Body) < 64 || !bytes.HasPrefix(export.Body, []byte("%PDF")) {
		t.Fatalf("create-to-deliver export = %d %q %d", export.StatusCode, export.Header, len(export.Body))
	}
	oldMetadata := studioRequest(t, handler, http.MethodGet, "/api/authoring?revision="+before.Revision, nil, "")
	if oldMetadata.StatusCode != http.StatusConflict {
		t.Fatalf("stale metadata = %d %s", oldMetadata.StatusCode, oldMetadata.Body)
	}
}

func TestPaperStudioSchemaFieldMatrixAndFixtureValueControls(t *testing.T) {
	file := filepath.Join(t.TempDir(), "matrix-authoring.paper")
	source := "document @report:\n" +
		"  schema @invoice:\n" +
		"    field @customer:\n" +
		"      type: \"object\"\n" +
		"      field @name:\n" +
		"        type: \"string\"\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        text: \"Invoice\"\n" +
		"  scenario @review:\n" +
		"    object @fixture-customer:\n" +
		"      value @name: \"Ada\"\n"
	if err := os.WriteFile(file, []byte(source), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	metadataResponse := studioRequest(t, handler, http.MethodGet, "/api/authoring?revision="+before.Revision, nil, "")
	var metadata studioAuthoringResponse
	if err := json.Unmarshal(metadataResponse.Body, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.SchemaFields) == 0 || len(metadata.ScenarioValues) != 1 || metadata.ScenarioValues[0].Path != "fixture-customer.name" {
		t.Fatalf("extended authoring metadata = %+v", metadata)
	}
	field := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "schema-field", "target": "@customer", "property": "", "id": "@email", "kind": "string",
	})
	if field.StatusCode != http.StatusOK {
		t.Fatalf("schema field = %d %s", field.StatusCode, field.Body)
	}
	afterField := fetchStudioWorkspace(t, handler)
	matrix := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": afterField.SourceRevision, "plan_revision": afterField.Revision,
		"operation": "scenario-matrix", "target": "@report", "property": "", "schema": "@invoice",
		"cases": []map[string]any{{"name": "@empty", "preset": "empty"}, {"name": "@stress", "preset": "stress"}},
	})
	if matrix.StatusCode != http.StatusOK {
		t.Fatalf("scenario matrix = %d %s", matrix.StatusCode, matrix.Body)
	}
	selectedResponse := studioRequest(t, handler, http.MethodGet, "/api/workspace?scenario=%40review", nil, "")
	var selected studioWorkspaceResponse
	if err := json.Unmarshal(selectedResponse.Body, &selected); err != nil {
		t.Fatal(err)
	}
	value := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": selected.SourceRevision, "plan_revision": selected.Revision, "scenario": "@review",
		"operation": "scenario-value", "target": "@review", "property": "", "path": "fixture-customer.name", "kind": "string", "text": "Grace",
	})
	if value.StatusCode != http.StatusOK {
		t.Fatalf("scenario value = %d %s", value.StatusCode, value.Body)
	}
	after := fetchStudioWorkspace(t, handler)
	for _, want := range []string{"field @email:", "scenario @empty:", "scenario @stress:", `value @name: "Grace"`} {
		if !strings.Contains(after.Source, want) {
			t.Fatalf("extended authoring source omitted %q:\n%s", want, after.Source)
		}
	}
}

func TestPaperStudioTypedPaletteInsertsPrimitiveAndComponentInstances(t *testing.T) {
	file := filepath.Join(t.TempDir(), "palette.paper")
	source := "document @report:\n" +
		"  component @card:\n" +
		"    paragraph:\n" +
		"      text: \"Card\"\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        text: \"Body\"\n"
	if err := os.WriteFile(file, []byte(source), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	metadataResponse := studioRequest(t, handler, http.MethodGet, "/api/authoring?revision="+before.Revision, nil, "")
	if metadataResponse.StatusCode != http.StatusOK || !strings.Contains(string(metadataResponse.Body), `"@card"`) {
		t.Fatalf("component palette metadata = %d %s", metadataResponse.StatusCode, metadataResponse.Body)
	}
	previewPath := "/api/component-preview.svg?preview_format=2&revision=" + before.Revision + "&source_revision=" + before.SourceRevision + "&component=%40card"
	preview := studioRequest(t, handler, http.MethodGet, previewPath, nil, "")
	if preview.StatusCode != http.StatusOK || !strings.Contains(preview.Header.Get("Content-Type"), "image/svg+xml") ||
		!bytes.Contains(preview.Body, []byte(`data-format="display-plan-preview"`)) || !bytes.Contains(preview.Body, []byte(">C</text>")) ||
		preview.Header.Get("ETag") == "" {
		t.Fatalf("component current-theme preview = %d %q %s", preview.StatusCode, preview.Header, preview.Body)
	}
	unchanged, err := os.ReadFile(file)
	if err != nil || string(unchanged) != source {
		t.Fatalf("component preview mutated source = %v\n%s", err, unchanged)
	}
	stale := studioRequest(t, handler, http.MethodGet, "/api/component-preview.svg?preview_format=2&revision=stale&source_revision="+before.SourceRevision+"&component=%40card", nil, "")
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale component preview = %d %s", stale.StatusCode, stale.Body)
	}
	request := map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "template", "target": "@body", "template": "component", "component": "@card", "id": "@card-instance",
	}
	response := postStudioJSON(t, handler, "/api/edit", request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("component palette edit = %d %s", response.StatusCode, response.Body)
	}
	after := fetchStudioWorkspace(t, handler)
	if !strings.Contains(after.Source, "use @card-instance:") || !strings.Contains(after.Source, "component: \"@card\"") {
		t.Fatalf("component palette source =\n%s", after.Source)
	}
}

func TestPaperStudioImportTemplateFlowsThroughPreviewAndExport(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "import.paper")
	if err := os.MkdirAll(filepath.Join(dir, "styles"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "styles", "design.paper"), []byte("document:\n  style @base:\n    font: \"Helvetica\"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	source := "document @report:\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        style: \"@base\"\n" +
		"        text: \"Imported\"\n"
	if err := os.WriteFile(file, []byte(source), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "import", "target": "@report", "property": "", "import_path": "styles/design.paper",
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("import edit = %d %s", response.StatusCode, response.Body)
	}
	after := fetchStudioWorkspace(t, handler)
	if after.Pages != 1 || !strings.Contains(after.Source, `import: "styles/design.paper"`) || len(after.Diagnostics) != 0 {
		t.Fatalf("import workspace = %+v\nsource=%s", after, after.Source)
	}
	export := studioRequest(t, handler, http.MethodGet, "/api/export.pdf?revision="+after.Revision, nil, "")
	if export.StatusCode != http.StatusOK || !bytes.HasPrefix(export.Body, []byte("%PDF")) {
		t.Fatalf("import export = %d bytes=%d", export.StatusCode, len(export.Body))
	}
}

func TestPaperStudioBoxEditBindsExactRevisionsAndKeepsCapabilitiesServerSide(t *testing.T) {
	file := filepath.Join(t.TempDir(), "box.paper")
	if err := os.WriteFile(file, []byte(studioBoxFixture), 0o640); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	beforeCapture := studioPageCapture(t, handler, before.Revision)
	request := map[string]any{
		"source_revision": before.SourceRevision,
		"plan_revision":   before.Revision,
		"operation":       "box",
		"target":          "@message",
		"property":        "background",
		"color":           "#aabbcc",
	}
	response := postStudioJSON(t, handler, "/api/edit", request)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("box edit = %d %s", response.StatusCode, response.Body)
	}
	var result studioEditResponse
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK || !result.Applied || result.PatchCount != 1 || !result.Authorization.Allowed ||
		result.Authorization.Actor != "studio:local-user" || result.SourceRevision == before.SourceRevision ||
		result.PlanRevision == before.Revision || len(result.Authorization.Effects) == 0 {
		t.Fatalf("box edit result = %+v", result)
	}
	afterWorkspace := fetchStudioWorkspace(t, handler)
	if afterWorkspace.Pages == 0 || afterWorkspace.Revision != result.PlanRevision {
		t.Fatalf("box edit plan = %+v; result=%+v", afterWorkspace, result)
	}
	if afterCapture := studioPageCapture(t, handler, result.PlanRevision); bytes.Equal(beforeCapture, afterCapture) {
		t.Fatal("box edit did not change the exact page SVG capture")
	}
	for _, forbidden := range []string{"capability", "candidate", "expected_head", "authority", "expires_at"} {
		if strings.Contains(response.Body, `"`+forbidden+`"`) {
			t.Fatalf("browser response exposed %q: %s", forbidden, response.Body)
		}
	}
	written, err := os.ReadFile(file)
	if err != nil || !strings.Contains(string(written), `background: "#aabbcc"`) {
		t.Fatalf("written source = %q, %v", written, err)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v, %v", info.Mode().Perm(), err)
	}

	stale := postStudioJSON(t, handler, "/api/edit", request)
	if stale.StatusCode != http.StatusConflict || !strings.Contains(stale.Body, "stale edit revision") {
		t.Fatalf("stale box edit = %d %s", stale.StatusCode, stale.Body)
	}
	afterStale, err := os.ReadFile(file)
	if err != nil || string(afterStale) != string(written) {
		t.Fatal("stale edit changed the source")
	}
}

func TestPaperStudioGridEditAuthorizesAndGuardsTransitiveParent(t *testing.T) {
	file := filepath.Join(t.TempDir(), "grid.paper")
	if err := os.WriteFile(file, []byte(studioGridFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	beforeCapture := studioPageCapture(t, handler, before.Revision)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision,
		"plan_revision":   before.Revision,
		"operation":       "grid",
		"target":          "@left",
		"property":        "track-size",
		"points":          48,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("grid edit = %d %s", response.StatusCode, response.Body)
	}
	var result studioEditResponse
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	effects := make(map[string]bool)
	for _, effect := range result.Authorization.Effects {
		effects[effect.Node] = true
	}
	if !result.Authorization.Allowed || !effects["@left"] || !effects["@grid"] {
		t.Fatalf("transitive authorization = %+v", result.Authorization)
	}
	if afterCapture := studioPageCapture(t, handler, result.PlanRevision); bytes.Equal(beforeCapture, afterCapture) {
		t.Fatal("grid edit did not change the exact page SVG capture")
	}
	written, _ := os.ReadFile(file)
	if !strings.Contains(string(written), "track-size: 48pt") {
		t.Fatalf("grid source = %s", written)
	}
}

func TestPaperStudioEditRejectsUnboundedUnknownAndConcurrentStaleIntents(t *testing.T) {
	file := filepath.Join(t.TempDir(), "box.paper")
	if err := os.WriteFile(file, []byte(studioFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	base := map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "box", "target": "@message", "property": "padding", "points": 7,
	}
	unknown := make(map[string]any, len(base)+1)
	for key, value := range base {
		unknown[key] = value
	}
	unknown["open_handle"] = "forged"
	if response := postStudioJSON(t, handler, "/api/edit", unknown); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown capability field = %d %s", response.StatusCode, response.Body)
	}
	unbounded := make(map[string]any, len(base))
	for key, value := range base {
		unbounded[key] = value
	}
	unbounded["property"] = strings.Repeat("x", studioEditFieldLimit+1)
	if response := postStudioJSON(t, handler, "/api/edit", unbounded); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("unbounded field = %d %s", response.StatusCode, response.Body)
	}

	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			statuses <- postStudioJSON(t, handler, "/api/edit", base).StatusCode
		}()
	}
	wait.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent edit statuses = %#v", counts)
	}
}

func TestStudioEditIdempotencyKeyDependsOnValuesNotPointerIdentity(t *testing.T) {
	firstPoints, secondPoints := 12.5, 12.5
	first := studioEditRequest{SourceRevision: "source", PlanRevision: "plan", Operation: "box", Target: "@box", Property: "padding", Points: &firstPoints}
	second := first
	second.Points = &secondPoints
	if studioEditIdempotencyKey(first) != studioEditIdempotencyKey(second) {
		t.Fatal("equal semantic intents produced different idempotency keys")
	}
	secondPoints = 13
	if studioEditIdempotencyKey(first) == studioEditIdempotencyKey(second) {
		t.Fatal("different semantic intents produced the same idempotency key")
	}
}

func TestPaperStudioImageAndTableEditsUseClosedPrivateAuthorities(t *testing.T) {
	tests := []struct {
		name       string
		source     string
		request    map[string]any
		wantSource string
		wantEffect string
		visual     bool
	}{
		{"image width", studioImageFixture, map[string]any{"operation": "image", "target": "@hero", "property": "width", "points": 48}, "width: 48pt", "@hero", true},
		{"table track", studioTableFixture, map[string]any{"operation": "table", "target": "@name-track", "property": "min-width", "points": 40}, "min-width: 40pt", "@ledger", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "fixture.paper")
			if err := os.WriteFile(file, []byte(test.source), 0o600); err != nil {
				t.Fatal(err)
			}
			studio, err := newStudioServer(file, "")
			if err != nil {
				t.Fatal(err)
			}
			handler := studio.routes()
			before := fetchStudioWorkspace(t, handler)
			beforeCapture := studioPageCapture(t, handler, before.Revision)
			request := map[string]any{"source_revision": before.SourceRevision, "plan_revision": before.Revision}
			for key, value := range test.request {
				request[key] = value
			}
			response := postStudioJSON(t, handler, "/api/edit", request)
			if response.StatusCode != http.StatusOK {
				t.Fatalf("edit = %d %s; before=%+v", response.StatusCode, response.Body, before)
			}
			var result studioEditResponse
			if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
				t.Fatal(err)
			}
			effects := map[string]bool{}
			for _, effect := range result.Authorization.Effects {
				effects[effect.Node] = true
			}
			if !result.Authorization.Allowed || !effects[test.wantEffect] || result.PatchCount != 1 {
				t.Fatalf("authorization = %+v", result.Authorization)
			}
			written, _ := os.ReadFile(file)
			if !strings.Contains(string(written), test.wantSource) {
				t.Fatalf("source = %s", written)
			}
			if test.visual {
				afterCapture := studioPageCapture(t, handler, result.PlanRevision)
				if bytes.Equal(beforeCapture, afterCapture) {
					t.Fatal("visual mutation did not change the exact page SVG capture")
				}
			}
		})
	}
}

func TestPaperStudioPageMarginEditsGoverningPageMaster(t *testing.T) {
	file := filepath.Join(t.TempDir(), "page.paper")
	if err := os.WriteFile(file, []byte(studioBoxFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	handler := studio.routes()
	before := fetchStudioWorkspace(t, handler)
	beforeCapture := studioPageCapture(t, handler, before.Revision)
	response := postStudioJSON(t, handler, "/api/edit", map[string]any{
		"source_revision": before.SourceRevision, "plan_revision": before.Revision,
		"operation": "page", "target": "@sheet", "property": "margin-left", "points": 20,
	})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("page edit = %d %s", response.StatusCode, response.Body)
	}
	var result studioEditResponse
	if err := json.Unmarshal([]byte(response.Body), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Authorization.Allowed || len(result.Authorization.Effects) != 1 || result.Authorization.Effects[0].Node != "@sheet" {
		t.Fatalf("page authorization = %+v", result.Authorization)
	}
	if bytes.Equal(beforeCapture, studioPageCapture(t, handler, result.PlanRevision)) {
		t.Fatal("page margin did not change exact SVG capture")
	}
	written, _ := os.ReadFile(file)
	if !strings.Contains(string(written), "margin-left: 20pt") {
		t.Fatalf("page source = %s", written)
	}
}

func studioPageCapture(t *testing.T, handler http.Handler, revision string) []byte {
	t.Helper()
	response := studioRequest(t, handler, http.MethodGet, "/api/page/1.svg?revision="+revision, nil, "")
	if response.StatusCode != http.StatusOK || !bytes.Contains(response.Body, []byte("<svg")) {
		t.Fatalf("page capture = %d %s", response.StatusCode, response.Body)
	}
	return response.Body
}
