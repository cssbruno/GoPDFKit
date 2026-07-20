// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/document"
)

const authoringMutationFixture = `document @report:
  # schema note must survive
  schema invoice:
    number total
    object customer:
      string name
  page @sheet:
    body @body:
      # body note must survive
      paragraph @copy:
        text: "Invoice"
`

func TestPaperInsertTemplateUsesOneJournalPatchAndPreservesTrivia(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, authoringMutationFixture, "@body", "insert-template", CapabilityEdit)
	result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: "section", ID: "@summary"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Edit.Applied || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !result.Semantic.AfterCompileOK {
		t.Fatalf("result = %#v", result)
	}
	for _, want := range []string{"# schema note must survive", "# body note must survive", "column @summary:", "heading @summary-heading:", "paragraph @summary-body:"} {
		if !strings.Contains(result.Revision.Source, want) {
			t.Fatalf("source omitted %q:\n%s", want, result.Revision.Source)
		}
	}
}

func TestPaperInsertTemplatePaletteCoversTypedPrimitivesAndComponents(t *testing.T) {
	for _, template := range []string{"paragraph", "heading", "list", "row", "column", "page-break", "image", "table", "canvas", "note-box", "metadata-grid", "signature-row", "qr-verification", "clause", "styled-container"} {
		workspace := mustWorkspace(t, Limits{})
		guard, _, _ := mutationGuard(t, workspace, authoringMutationFixture, "@body", "palette-"+template, CapabilityEdit)
		result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: template, ID: "@new-" + template})
		if err != nil {
			t.Fatalf("template %s: %v result=%#v", template, err, result)
		}
		if !result.Edit.Applied || !result.Semantic.AfterCompileOK || len(result.Edit.Diff.Patches) != 1 {
			t.Fatalf("template %s result = %#v", template, result)
		}
		if plan, planned, planErr := document.PlanPaper("palette.paper", result.Revision.Source); planErr != nil || !planned.OK() || plan.PageCount() == 0 {
			t.Fatalf("template %s did not render: pages=%d result=%#v err=%v", template, plan.PageCount(), planned, planErr)
		}
	}

	source := "document @report:\n" +
		"  component @card:\n" +
		"    paragraph:\n" +
		"      text: \"Card\"\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        text: \"Body\"\n"
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, source, "@body", "palette-component", CapabilityEdit)
	result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: "component", Component: "@card", ID: "@card-instance"})
	if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, "use @card-instance:") || !strings.Contains(result.Revision.Source, "component: \"@card\"") {
		t.Fatalf("component template = %v result=%#v\nsource=%s", err, result, result.Revision.Source)
	}

	schemaWorkspace := mustWorkspace(t, Limits{})
	schemaGuard, _, _ := mutationGuard(t, schemaWorkspace, authoringMutationFixture, "@report", "palette-schema", CapabilityEdit)
	schemaResult, err := schemaWorkspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: schemaGuard, Template: "schema", ID: "@receipt"})
	if err != nil || !schemaResult.Semantic.AfterCompileOK || !strings.Contains(schemaResult.Revision.Source, "schema receipt:") || !strings.Contains(schemaResult.Revision.Source, "string receipt-value") {
		t.Fatalf("schema template = %v result=%#v\nsource=%s", err, schemaResult, schemaResult.Revision.Source)
	}
}

func TestPaperInsertTemplateCreatesRegionsAndCompleteDocumentPresets(t *testing.T) {
	for _, template := range []string{"header", "footer"} {
		source := "document @report:\n  page @sheet:\n    body @body:\n      paragraph @copy:\n        text: \"Body\"\n"
		workspace := mustWorkspace(t, Limits{})
		guard, _, _ := mutationGuard(t, workspace, source, "@sheet", "region-"+template, CapabilityEdit)
		result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: template, ID: "@running-" + template})
		if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, template+" @running-"+template+":") {
			t.Fatalf("region %s = %v result=%#v\n%s", template, err, result, result.Revision.Source)
		}
	}
	for _, preset := range []string{"blank", "letter", "prescription", "medical-report", "invoice", "contract", "certificate", "table-report"} {
		workspace := mustWorkspace(t, Limits{})
		bootstrap := "document @report:\n  component @placeholder:\n    paragraph:\n      text: \"Placeholder\"\n"
		guard, _, _ := mutationGuard(t, workspace, bootstrap, "@report", "preset-"+preset, CapabilityEdit)
		result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: "document-preset", Preset: preset, ID: "@sheet"})
		if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, "header @sheet-header:") || !strings.Contains(result.Revision.Source, "footer @sheet-footer:") || !strings.Contains(result.Revision.Source, "body @sheet-body:") {
			t.Fatalf("document preset %s = %v result=%#v\n%s", preset, err, result, result.Revision.Source)
		}
		if plan, planned, planErr := document.PlanPaper("preset.paper", result.Revision.Source); planErr != nil || !planned.OK() || plan.PageCount() == 0 {
			t.Fatalf("document preset %s did not render: pages=%d result=%#v err=%v", preset, plan.PageCount(), planned, planErr)
		}
	}
}

func TestPaperInsertTemplateCreatesBoundedRepeatAndLoop(t *testing.T) {
	source := `document @report:
  schema invoice:
    list object items:
      max-items: 3
      string name
  scenario @preview:
    keyed-list @items:
      object @first:
        value @name: "Alpha"
  page @sheet:
    body @body:
      paragraph @copy:
        text: "Body"
`
	for _, request := range []PaperInsertTemplateRequest{{Template: "repeat", ID: "@lines", Path: "items"}, {Template: "loop", ID: "@copies"}} {
		workspace := mustWorkspace(t, Limits{})
		guard, _, _ := mutationGuard(t, workspace, source, "@body", "repeater-"+request.Template, CapabilityEdit)
		request.Guard = guard
		result, err := workspace.PaperInsertTemplate(request)
		if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, request.Template+" "+request.ID+":") {
			t.Fatalf("%s template = %v result=%#v\n%s", request.Template, err, result, result.Revision.Source)
		}
		if plan, planned, planErr := document.PlanPaperScenario("repeat.paper", result.Revision.Source, "@preview"); planErr != nil || !planned.OK() || plan.PageCount() == 0 {
			t.Fatalf("%s template did not render: pages=%d result=%#v err=%v", request.Template, plan.PageCount(), planned, planErr)
		}
	}
}

func TestPaperCreateScenarioUsesCompilerSchemaAndStressPreset(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, authoringMutationFixture, "@report", "create-scenario", CapabilityEdit)
	result, err := workspace.PaperCreateScenario(PaperCreateScenarioRequest{Guard: guard, Name: "@stress", Schema: "@invoice", Preset: "stress"})
	if err != nil {
		t.Fatalf("%v result=%#v", err, result)
	}
	if !result.Edit.Applied || len(result.Edit.Diff.Patches) != 1 || !result.Semantic.AfterCompileOK {
		t.Fatalf("result = %#v", result)
	}
	for _, want := range []string{"scenario @stress:", "value @total: 999999.99", "object @customer:", "value @name: \"Wide value"} {
		if !strings.Contains(result.Revision.Source, want) {
			t.Fatalf("source omitted %q:\n%s", want, result.Revision.Source)
		}
	}

	badWorkspace := mustWorkspace(t, Limits{})
	badGuard, _, _ := mutationGuard(t, badWorkspace, authoringMutationFixture, "@report", "missing-schema", CapabilityEdit)
	if _, err := badWorkspace.PaperCreateScenario(PaperCreateScenarioRequest{Guard: badGuard, Name: "@bad", Schema: "@missing", Preset: "typical"}); err == nil {
		t.Fatal("missing exact schema unexpectedly accepted")
	}
}

func TestPaperAuthoringAddsNestedSchemaFieldsAndCreatesAtomicMatrix(t *testing.T) {
	source := `document @report:
  schema invoice:
    object customer:
      string name
  page @sheet:
    body @body:
      paragraph @copy:
        text: "Invoice"
`
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, source, "@customer", "add-nested-field", CapabilityEdit)
	result, err := workspace.PaperAddSchemaField(PaperAddSchemaFieldRequest{Guard: guard, ID: "@email", Type: "string"})
	if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, "string email") {
		t.Fatalf("nested schema field = %v result=%#v\nsource=%s", err, result, result.Revision.Source)
	}

	matrixWorkspace := mustWorkspace(t, Limits{})
	matrixGuard, _, _ := mutationGuard(t, matrixWorkspace, source, "@report", "matrix", CapabilityEdit)
	matrix, err := matrixWorkspace.PaperCreateScenarioMatrix(PaperCreateScenarioMatrixRequest{Guard: matrixGuard, Schema: "@invoice", Cases: []PaperScenarioMatrixCase{
		{Name: "@empty", Preset: "empty"}, {Name: "@typical", Preset: "typical"}, {Name: "@stress", Preset: "stress"},
	}})
	if err != nil || !matrix.Semantic.AfterCompileOK || len(matrix.Edit.Diff.Patches) != 1 {
		t.Fatalf("matrix = %v result=%#v", err, matrix)
	}
	for _, name := range []string{"@empty", "@typical", "@stress"} {
		if !strings.Contains(matrix.Revision.Source, "scenario "+name+":") {
			t.Fatalf("matrix omitted %s:\n%s", name, matrix.Revision.Source)
		}
	}
}

func TestPaperAuthoringCreatesAndUsesCustomObjects(t *testing.T) {
	base := `document @report:
  object Address:
    string street
    string city
  schema invoice:
    string number
  page @sheet:
    body @body:
      paragraph @copy:
        text: "Invoice"
`
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, base, "@invoice", "custom-field", CapabilityEdit)
	result, err := workspace.PaperAddSchemaField(PaperAddSchemaFieldRequest{Guard: guard, ID: "@billing", Type: "Address"})
	if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, "Address billing") {
		t.Fatalf("custom object field = %v result=%#v\nsource=%s", err, result, result.Revision.Source)
	}

	listWorkspace := mustWorkspace(t, Limits{})
	listGuard, _, _ := mutationGuard(t, listWorkspace, base, "@invoice", "custom-list", CapabilityEdit)
	list, err := listWorkspace.PaperAddSchemaField(PaperAddSchemaFieldRequest{Guard: listGuard, ID: "@history", Type: "list", ItemType: "Address", MaxItems: 5})
	if err != nil || !list.Semantic.AfterCompileOK || !strings.Contains(list.Revision.Source, "list Address history:") {
		t.Fatalf("custom object list = %v result=%#v\nsource=%s", err, list, list.Revision.Source)
	}

	templateWorkspace := mustWorkspace(t, Limits{})
	templateSource := "document @report:\n  page @sheet:\n    body @body:\n      text: \"x\"\n"
	templateGuard, _, _ := mutationGuard(t, templateWorkspace, templateSource, "@report", "custom-template", CapabilityEdit)
	template, err := templateWorkspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: templateGuard, Template: "schema-object", ID: "@Contact"})
	if err != nil || !template.Semantic.AfterCompileOK || !strings.Contains(template.Revision.Source, "object Contact:") || !strings.Contains(template.Revision.Source, "string Contact-value") {
		t.Fatalf("custom object template = %v result=%#v\nsource=%s", err, template, template.Revision.Source)
	}
}

func TestPaperAuthoringEditsScenarioFixtureValueByRelativePath(t *testing.T) {
	source := `document @report:
  schema invoice:
    object customer:
      string name
  page @sheet:
    body @body:
      paragraph @copy:
        text: "Invoice"
  scenario @review:
    object @customer:
      value @name: "Ada"
`
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, source, "@review", "fixture-value", CapabilityEdit)
	result, err := workspace.PaperSetScenarioFixtureValue(PaperSetScenarioFixtureValueRequest{Guard: guard, Path: "customer.name", Kind: "string", Text: "Grace"})
	if err != nil || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, `value @name: "Grace"`) || len(result.Edit.Diff.Patches) != 1 {
		t.Fatalf("fixture value = %v result=%#v\nsource=%s", err, result, result.Revision.Source)
	}
}

func TestPaperInsertImportTemplateAppendsOneResolvedDesignImport(t *testing.T) {
	workspace, err := NewWorkspaceWithOptions(WorkspaceOptions{ImportResolver: func(importerFile, importPath string) (string, string, error) {
		if importerFile != "mutation.paper" || importPath != "styles/design.paper" {
			t.Fatalf("resolver request = %s %s", importerFile, importPath)
		}
		return "styles/design.paper", "document:\n  style @base:\n    font: \"Helvetica\"\n", nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	source := "document @report:\n" +
		"  page @sheet:\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        style: \"@base\"\n" +
		"        text: \"Imported\"\n"
	guard, _, _ := mutationGuard(t, workspace, source, "@report", "insert-import", CapabilityEdit)
	result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: "import", ImportPath: "styles/design.paper"})
	if err != nil || !result.Semantic.AfterCompileOK || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, `import: "styles/design.paper"`) {
		t.Fatalf("import result = %v %#v\nsource=%s", err, result, result.Revision.Source)
	}
}

func TestPaperInsertPageTemplateBootstrapsDocumentWithoutPage(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	source := "document @report:\n  title: \"Bootstrap\"\n"
	guard, _, _ := mutationGuard(t, workspace, source, "@report", "insert-page-template", CapabilityEdit)
	result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: "page", ID: "@sheet"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Edit.Applied || !result.Semantic.AfterCompileOK || !strings.Contains(result.Revision.Source, "page @sheet:") || !strings.Contains(result.Revision.Source, "body @sheet-body:") {
		t.Fatalf("page bootstrap result = %#v\nsource=%s", result, result.Revision.Source)
	}
	if _, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: "page", ID: "@other"}); err == nil {
		t.Fatal("replayed stale page bootstrap unexpectedly succeeded")
	}
}

func TestPaperManageScenarioRenamesAndDeletesExactMatrixRows(t *testing.T) {
	source := `document @report:
  schema invoice:
    number total
  page @sheet:
    body @body:
      paragraph @copy:
        text: "Invoice"
  scenario @review:
    value @total: 10
`
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, source, "@review", "scenario-rename", CapabilityEdit)
	rename, err := workspace.PaperManageScenario(PaperManageScenarioRequest{Guard: guard, Action: "rename", NewName: "@approved"})
	if err != nil || !rename.Semantic.AfterCompileOK || !strings.Contains(rename.Revision.Source, "scenario @approved:") || strings.Contains(rename.Revision.Source, "scenario @review:") {
		t.Fatalf("rename = %v result=%#v\nsource=%s", err, rename, rename.Revision.Source)
	}

	deleteWorkspace := mustWorkspace(t, Limits{})
	deleteGuard, _, _ := mutationGuard(t, deleteWorkspace, source, "@review", "scenario-delete", CapabilityEdit)
	deleted, err := deleteWorkspace.PaperManageScenario(PaperManageScenarioRequest{Guard: deleteGuard, Action: "delete"})
	if err != nil || !deleted.Semantic.AfterCompileOK || strings.Contains(deleted.Revision.Source, "scenario @review:") {
		t.Fatalf("delete = %v result=%#v\nsource=%s", err, deleted, deleted.Revision.Source)
	}
}
