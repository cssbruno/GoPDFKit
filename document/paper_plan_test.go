// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlanPaperIsReadOnlyReusableAndPreservesSourceIdentity(t *testing.T) {
	const source = "document @report:\n" +
		"  page @sheet:\n" +
		"    margin: 24pt\n" +
		"    body @body:\n" +
		"      paragraph @intro:\n" +
		"        text @copy: \"Hello plan\"\n"
	plan, result, err := PlanPaper("identity.paper", source)
	if err != nil || result.Pages != 1 || result.Hash == "" || plan.Hash() != result.Hash {
		t.Fatalf("PlanPaper() = %#v, %#v, %v", plan, result, err)
	}
	manifestResult, err := plan.DeterministicInputManifest()
	if err != nil || manifestResult.PlanHash != plan.Hash() {
		t.Fatalf("DeterministicInputManifest() = %s, %v", manifestResult.JSON(), err)
	}
	manifestJSON := string(manifestResult.JSON())
	for _, required := range []string{`"locale":"und"`, `"timezone":"UTC"`, `"cldr":"none"`, `"hyphenation":"none"`, `"page_profile"`, `"core-font-metrics"`} {
		if !strings.Contains(manifestJSON, required) {
			t.Fatalf("deterministic manifest missing %s: %s", required, manifestJSON)
		}
	}
	detachedManifest := manifestResult.JSON()
	detachedManifest[0] = 'x'
	againManifest, _ := plan.DeterministicInputManifest()
	if againManifest.JSON()[0] == 'x' {
		t.Fatal("deterministic manifest was not detached")
	}
	projection := plan.plan.Projection()
	if len(projection.SemanticNodes) != 2 || projection.SemanticNodes[0].Role != "document" ||
		projection.SemanticNodes[0].Key != "@report" || projection.SemanticNodes[0].Source.File != "identity.paper" ||
		projection.SemanticNodes[1].Role != "paragraph" || projection.SemanticNodes[1].Key != "@intro" ||
		len(projection.SemanticFragments) != 1 || len(projection.ReadingOrder) != 1 {
		t.Fatalf("authored plan semantics = nodes %#v, associations %#v, reading %#v",
			projection.SemanticNodes, projection.SemanticFragments, projection.ReadingOrder)
	}
	query, err := plan.Query(PaperPlanSelector{Key: "@intro", MaxResults: 8})
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(query.JSON())
	if !strings.Contains(encoded, `"key":"@intro"`) || !strings.Contains(encoded, `"file":"identity.paper"`) {
		t.Fatalf("query did not preserve readable identity/source span: %s", encoded)
	}
	hit, err := plan.HitTest(1, 25*1024, 25*1024)
	if err != nil || hit.PlanHash != plan.Hash() ||
		!strings.Contains(string(hit.JSON()), `"Key":"@intro"`) ||
		!strings.Contains(string(hit.JSON()), `"InsidePage":true`) {
		t.Fatalf("HitTest() = %s, %v", hit.JSON(), err)
	}
	detached := hit.JSON()
	detached[0] = 'x'
	if again, hitErr := plan.HitTest(1, 25*1024, 25*1024); hitErr != nil || again.JSON()[0] == 'x' {
		t.Fatal("hit-test result was not detached")
	}
	pixelHit, err := plan.HitTestPixel(PaperPlanPixelHitTestRequest{
		Page: 1, PixelX: 24, PixelY: 24, PixelWidth: 612, PixelHeight: 792,
		CaptureWidth: 612 * 1024, CaptureHeight: 792 * 1024,
	})
	if err != nil || !bytes.Contains(pixelHit.JSON(), []byte(`"Key":"@intro"`)) {
		t.Fatalf("HitTestPixel() = %s, %v", pixelHit.JSON(), err)
	}
	first, _ := NewDocument(WithUnit(UnitPoint), WithDeterministicOutput())
	second, _ := NewDocument(WithUnit(UnitPoint), WithDeterministicOutput())
	firstResult, firstErr := first.WritePaperPlan(plan)
	secondResult, secondErr := second.WritePaperPlan(plan)
	if firstErr != nil || secondErr != nil || firstResult.Pages != 1 || secondResult.Pages != 1 {
		t.Fatalf("WritePaperPlan() = %#v/%v, %#v/%v", firstResult, firstErr, secondResult, secondErr)
	}
	var firstPDF, secondPDF bytes.Buffer
	if err := first.OutputWithOptions(&firstPDF, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if err := second.OutputWithOptions(&secondPDF, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstPDF.Bytes(), secondPDF.Bytes()) {
		t.Fatal("reusing one immutable plan produced different deterministic PDFs")
	}
}

func TestPlanPaperFailureReturnsNoUsablePlan(t *testing.T) {
	plan, result, err := PlanPaper("broken.paper", "document:\n  page\n")
	if err == nil || plan.Hash() != "" || result.Pages != 0 || result.Hash != "" || len(result.Diagnostics) == 0 {
		t.Fatalf("PlanPaper(invalid) = %#v, %#v, %v", plan, result, err)
	}
	target, _ := NewDocument(WithUnit(UnitPoint))
	if painted, paintErr := target.WritePaperPlan(plan); paintErr == nil || painted.Pages != 0 || target.PageCount() != 0 {
		t.Fatalf("WritePaperPlan(zero) = %#v, %v, pages=%d", painted, paintErr, target.PageCount())
	}
}

func TestWritePaperPlanTaggedOutputUsesSemanticDisplayPainter(t *testing.T) {
	const source = "document @report:\n  page @sheet:\n    body @body:\n      paragraph @copy:\n        text: \"Tagged Paper\"\n"
	plan, result, err := PlanPaper("tagged.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	pdf := MustNew(WithUnit(UnitPoint), WithDeterministicOutput())
	pdf.EnableTaggedPDF()
	pdf.SetCompression(false)
	if rendered, err := pdf.WritePaperPlan(plan); err != nil || rendered.Pages != 1 {
		t.Fatalf("WritePaperPlan() = %#v, %v", rendered, err)
	}
	var output bytes.Buffer
	if err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"/MarkInfo", "/Marked true", "/StructTreeRoot", "/S /Document", "/S /P", "/MCID 0", "BDC"} {
		if !bytes.Contains(output.Bytes(), []byte(marker)) {
			t.Fatalf("tagged Paper output is missing %q", marker)
		}
	}
}

func TestPlanPaperListItemParagraphMappingsRemainUnique(t *testing.T) {
	const source = "document:\n" +
		"  page:\n" +
		"    margin: 20pt\n" +
		"    body:\n" +
		"      list @steps:\n" +
		"        item @one:\n" +
		"          paragraph @first:\n" +
		"            text: \"First paragraph\"\n" +
		"          paragraph @second:\n" +
		"            text: \"Second paragraph\"\n"
	plan, result, err := PlanPaper("list.paper", source)
	if err != nil || result.Pages != 1 {
		t.Fatalf("PlanPaper(list) = %#v, %v", result, err)
	}
	for _, key := range []string{"@first", "@second"} {
		query, queryErr := plan.Query(PaperPlanSelector{Key: key, MaxResults: 4})
		if queryErr != nil || !strings.Contains(string(query.JSON()), `"key":"`+key+`"`) {
			t.Fatalf("Query(%s) = %s, %v", key, query.JSON(), queryErr)
		}
	}
}
