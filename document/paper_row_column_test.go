// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlanAndWritePaperRowUsesFixedPointContainerGeometry(t *testing.T) {
	const source = "document:\n" +
		"  page:\n" +
		"    width: 200pt\n" +
		"    height: 100pt\n" +
		"    margin: 10pt\n" +
		"    body:\n" +
		"      row @summary:\n" +
		"        gap: 10pt\n" +
		"        cross-align: \"start\"\n" +
		"        heading @label:\n" +
		"          track: \"fixed\"\n" +
		"          track-size: 60pt\n" +
		"          font: \"Courier\"\n" +
		"          size: 10pt\n" +
		"          line-height: 12pt\n" +
		"          text: \"L\\nL\"\n" +
		"        paragraph @value:\n" +
		"          track: \"fraction\"\n" +
		"          track-weight: 1\n" +
		"          cross-align: \"end\"\n" +
		"          font: \"Courier\"\n" +
		"          size: 10pt\n" +
		"          line-height: 12pt\n" +
		"          text: \"VALUE\"\n"
	plan, result, err := PlanPaper("row.paper", source)
	if err != nil || result.Pages != 1 {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 2 || len(projection.Lines) != 3 || len(projection.Commands) != 3 {
		t.Fatalf("projection cardinality = %d fragments, %d lines, %d commands", len(projection.Fragments), len(projection.Lines), len(projection.Commands))
	}
	first, second := projection.Fragments[0], projection.Fragments[1]
	if first.Key != "@label" || first.BorderBox.X.Points() != 10 || first.BorderBox.Width.Points() != 60 || first.BorderBox.Height.Points() != 24 {
		t.Fatalf("first fragment = %+v", first)
	}
	if second.Key != "@value" || second.BorderBox.X.Points() != 80 || second.BorderBox.Width.Points() != 110 ||
		second.BorderBox.Y.Points() != 22 || second.BorderBox.Height.Points() != 12 {
		t.Fatalf("second fragment = %+v", second)
	}
	if first.Source.File != "row.paper" || second.Source.File != "row.paper" {
		t.Fatalf("authored sources = %+v / %+v", first.Source, second.Source)
	}
	query, err := plan.Query(PaperPlanSelector{Key: "@value", MaxResults: 4})
	if err != nil || !strings.Contains(string(query.JSON()), `"key":"@value"`) {
		t.Fatalf("Query(@value) = %s, %v", query.JSON(), err)
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	painted, err := target.WritePaperPlan(plan)
	if err != nil || painted.Pages != 1 {
		t.Fatalf("WritePaperPlan() = %#v, %v", painted, err)
	}
	content := target.pages[1].Bytes()
	firstL := bytes.Index(content, []byte("(L) Tj"))
	secondRelative := -1
	if firstL >= 0 {
		secondRelative = bytes.Index(content[firstL+1:], []byte("(L) Tj"))
	}
	value := bytes.Index(content, []byte("(V) Tj"))
	if firstL < 0 || secondRelative < 0 || value <= firstL+1+secondRelative {
		t.Fatalf("planned row text is absent or out of order:\n%s", content)
	}
	if bytes.Contains(content, []byte(" Tw")) || bytes.Contains(content, []byte(" Td")) {
		t.Fatalf("row paint used live layout operators:\n%s", content)
	}
}

func TestPlanPaperColumnUsesFixedAndFractionalHeightTracks(t *testing.T) {
	const source = "document:\n  page:\n    width: 100pt\n    height: 100pt\n    margin: 10pt\n    body:\n      column @stack:\n        gap: 5pt\n        paragraph @top:\n          track: \"fixed\"\n          track-size: 20pt\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"TOP\"\n        paragraph @bottom:\n          track: \"fraction\"\n          track-weight: 1\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"BOTTOM\"\n"
	plan, result, err := PlanPaper("column.paper", source)
	if err != nil || result.Pages != 1 {
		t.Fatalf("PlanPaper(column) = %#v, %v", result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].BorderBox.Height.Points() != 20 ||
		fragments[1].BorderBox.Y.Points() != 35 || fragments[1].BorderBox.Height.Points() != 55 {
		t.Fatalf("column fragments = %+v", fragments)
	}
}

func TestPlanPaperPlansMixedTopLevelRowColumnAtomically(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      paragraph @before:\n        text: \"before\"\n      row @stack:\n        paragraph @inside:\n          text: \"inside\"\n"
	plan, result, err := PlanPaper("mixed.paper", source)
	if err != nil || plan.Hash() == "" || result.Pages != 1 {
		t.Fatalf("PlanPaper(mixed) = %#v, %#v, %v", plan, result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].Key != "@mixed-1-@before" || !strings.HasSuffix(string(fragments[1].Key), "@inside") {
		t.Fatalf("mixed row/column fragments = %+v", fragments)
	}
}
