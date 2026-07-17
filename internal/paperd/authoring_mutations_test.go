// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"strings"
	"testing"
)

const authoringMutationFixture = "document @report:\n" +
	"  # schema note must survive\n" +
	"  schema @invoice:\n" +
	"    field @total:\n" +
	"      type: \"number\"\n" +
	"    field @customer:\n" +
	"      type: \"object\"\n" +
	"      field @name:\n" +
	"        type: \"string\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      # body note must survive\n" +
	"      paragraph @copy:\n" +
	"        text: \"Invoice\"\n"

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
	for _, template := range []string{"paragraph", "heading", "list", "row", "column", "page-break"} {
		workspace := mustWorkspace(t, Limits{})
		guard, _, _ := mutationGuard(t, workspace, authoringMutationFixture, "@body", "palette-"+template, CapabilityEdit)
		result, err := workspace.PaperInsertTemplate(PaperInsertTemplateRequest{Guard: guard, Template: template, ID: "@new-" + template})
		if err != nil {
			t.Fatalf("template %s: %v", template, err)
		}
		if !result.Edit.Applied || !result.Semantic.AfterCompileOK || len(result.Edit.Diff.Patches) != 1 {
			t.Fatalf("template %s result = %#v", template, result)
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
