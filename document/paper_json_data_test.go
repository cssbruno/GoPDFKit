// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"
)

const paperJSONDataFixture = `document @report:
  language: "pt-BR"
  schema @lab:
    field @patient:
      type: "string"
    field @results:
      type: "list"
      item-type: "object"
      max-items: 8
      field @name:
        type: "string"
  page:
    size: "A4"
    margin: 24pt
    body:
      heading @patient:
        level: 1
        bind: "@lab.patient"
        text: "Patient"
      repeat @results:
        source: "@lab.results"
        instance-prefix: "results"
        max-items: 8
        paragraph @result:
          bind: "name"
          text: "Result"
`

func TestPlanAndWritePaperJSONEndToEnd(t *testing.T) {
	t.Parallel()
	firstData := []byte(`{"patient":"Ana","results":[{"name":"Hemoglobina"},{"name":"Plaquetas"}]}`)
	plan, result, err := PlanPaperJSON("lab.paper", paperJSONDataFixture, firstData)
	if err != nil || !result.OK() || plan.Hash() == "" {
		t.Fatalf("PlanPaperJSON() = %#v, %v", result, err)
	}
	again, againResult, err := PlanPaperJSONWithOptions("lab.paper", paperJSONDataFixture, firstData, PaperJSONOptions{Schema: "lab", Locale: "pt-BR", Name: "request"})
	if err != nil || !againResult.OK() || again.Hash() == "" {
		t.Fatalf("PlanPaperJSONWithOptions() = %#v, %v", againResult, err)
	}
	changed, changedResult, err := PlanPaperJSON("lab.paper", paperJSONDataFixture, []byte(`{"patient":"Bia","results":[]}`))
	if err != nil || !changedResult.OK() || changed.Hash() == plan.Hash() {
		t.Fatalf("changed plan = %#v, %v", changedResult, err)
	}

	pdf, err := NewDocument(WithUnit(UnitPoint), WithDeterministicOutput())
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := pdf.WritePaperJSON("lab.paper", paperJSONDataFixture, firstData)
	if err != nil || !rendered.OK() {
		t.Fatalf("WritePaperJSON() = %#v, %v", rendered, err)
	}
	var output bytes.Buffer
	if err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil || !bytes.HasPrefix(output.Bytes(), []byte("%PDF-")) {
		t.Fatalf("PDF = %d bytes, %v", output.Len(), err)
	}

	catalog, err := NewPaperAssetCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	withBoundaries, err := NewDocument(WithUnit(UnitPoint), WithDeterministicOutput())
	if err != nil {
		t.Fatal(err)
	}
	rendered, err = withBoundaries.WritePaperJSONWithAssetsAndImports("lab.paper", paperJSONDataFixture, firstData, PaperJSONOptions{}, catalog, nil)
	if err != nil || !rendered.OK() {
		t.Fatalf("WritePaperJSONWithAssetsAndImports() = %#v, %v", rendered, err)
	}
}

func TestPlanPaperJSONReportsSchemaPointer(t *testing.T) {
	t.Parallel()
	_, result, err := PlanPaperJSON("lab.paper", paperJSONDataFixture, []byte(`{"patient":42,"results":[]}`))
	if err == nil || len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "PAPER_DATA_JSON" ||
		!bytes.Contains([]byte(result.Diagnostics[0].Message), []byte("#/patient")) {
		t.Fatalf("result = %#v, %v", result, err)
	}
}
