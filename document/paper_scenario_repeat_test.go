// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

const paperScenarioRepeatFixture = `document @doc:
  schema @invoice:
    field @items:
      type: "list"
      item-type: "object"
      max-items: 4
      field @active:
        type: "bool"
      field @name:
        type: "string"
  scenario @preview:
    keyed-list @items:
      object @alpha:
        value @active: true
        value @name: "Alpha"
      object @beta:
        value @active: false
        value @name: "Beta"
      object @gamma:
        value @active: true
        value @name: "Gamma"
  page:
    body:
      repeat @visible:
        source: "@invoice.items"
        instance-prefix: "preview-lines"
        max-items: 2
        when: "active"
        paragraph @line:
          bind: "name"
          text: "Scenario line"
`

func TestPlanAndWritePaperScenarioRepeatEndToEnd(t *testing.T) {
	t.Parallel()

	plan, planned, err := PlanPaperScenario("scenario-repeat.paper", paperScenarioRepeatFixture, "@preview")
	if err != nil || !planned.OK() || plan.PageCount() != 1 || plan.Hash() == "" {
		t.Fatalf("plan = %#v, result = %#v, error = %v", plan, planned, err)
	}
	again, againResult, err := PlanPaperScenario("scenario-repeat.paper", paperScenarioRepeatFixture, "preview")
	if err != nil || !againResult.OK() || again.Hash() != plan.Hash() {
		t.Fatalf("deterministic plan = %#v / %#v, %v", planned, againResult, err)
	}
	capture, err := plan.Capture(PaperPlanCaptureRequest{
		Mode: "core_text_svg", IncludeContactSheet: true, ContactSheetColumns: 1,
		MaxPages: 1, MaxCrops: 4, MaxArtifactBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxManifestBytes: 1 << 20,
	})
	if err != nil || len(capture.Artifacts) != 1 {
		t.Fatalf("scenario capture does not contain evaluated keyed values: artifacts=%d error=%v", len(capture.Artifacts), err)
	}
	embedded, err := firstEmbeddedSVG(capture.Artifacts[0].SVG)
	if err != nil || !bytes.Contains(embedded, []byte(">A</text>")) || !bytes.Contains(embedded, []byte(">G</text>")) ||
		bytes.Contains(embedded, []byte("Scenario line")) {
		t.Fatalf("scenario capture does not contain evaluated keyed glyphs: %v\n%s", err, embedded)
	}

	document, err := NewDocument(WithUnit(UnitPoint), WithDeterministicOutput())
	if err != nil {
		t.Fatalf("new document: %v", err)
	}
	rendered, err := document.WritePaperScenario("scenario-repeat.paper", paperScenarioRepeatFixture, "preview")
	if err != nil || !rendered.OK() || rendered.Pages != 1 || document.PageCount() != 1 {
		t.Fatalf("render = %#v, pages = %d, error = %v", rendered, document.PageCount(), err)
	}
}

func TestPlanAndWritePaperScenarioBoundedLoopEndToEnd(t *testing.T) {
	t.Parallel()
	const source = `document:
  schema @settings:
    field @enabled:
      type: "bool"
  scenario @preview:
    value @enabled: true
  page:
    body:
      loop @copies:
        from: 1
        through: 3
        step: 1
        max-iterations: 3
        instance-prefix: "copies"
        when: "enabled && !loop.last"
        paragraph @copy:
          when: "loop.first || loop.index == 2"
          text: "Loop"
`
	plan, result, err := PlanPaperScenario("loop.paper", source, "preview")
	if err != nil || !result.OK() || plan.PageCount() != 1 {
		t.Fatalf("plan = %#v / %v", result, err)
	}
	again, againResult, err := PlanPaperScenario("loop.paper", source, "@preview")
	if err != nil || !againResult.OK() || again.Hash() != plan.Hash() {
		t.Fatalf("deterministic plan = %#v / %v", againResult, err)
	}
	query, err := plan.Query(PaperPlanSelector{Page: 1, MaxResults: 16})
	json := string(query.JSON())
	if err != nil || !strings.Contains(json, `"instance":"copies[1]/`) || !strings.Contains(json, `"instance":"copies[2]/`) || strings.Contains(json, `"instance":"copies[3]/`) {
		t.Fatalf("loop query = %s / %v", json, err)
	}
	document := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	rendered, err := document.WritePaperScenario("loop.paper", source, "preview")
	if err != nil || !rendered.OK() || rendered.Pages != 1 {
		t.Fatalf("render = %#v / %v", rendered, err)
	}
	if count := bytes.Count(document.pages[1].Bytes(), []byte("(L) Tj")); count != 2 {
		t.Fatalf("loop glyph count = %d\n%s", count, document.pages[1].Bytes())
	}
}

func firstEmbeddedSVG(contactSheet []byte) ([]byte, error) {
	const prefix = `href="data:image/svg+xml;base64,`
	start := strings.Index(string(contactSheet), prefix)
	if start < 0 {
		return nil, base64.CorruptInputError(0)
	}
	encoded := string(contactSheet[start+len(prefix):])
	end := strings.IndexByte(encoded, '"')
	if end < 0 {
		return nil, base64.CorruptInputError(len(encoded))
	}
	return base64.StdEncoding.DecodeString(encoded[:end])
}

func TestPlanPaperScenarioRequiresExplicitValidSelection(t *testing.T) {
	t.Parallel()

	for _, scenario := range []string{"", "missing"} {
		_, result, err := PlanPaperScenario("scenario-repeat.paper", paperScenarioRepeatFixture, scenario)
		if err == nil || result.OK() {
			t.Fatalf("scenario %q unexpectedly planned: %#v, %v", scenario, result, err)
		}
	}
}
