// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
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

func TestPlanPaperRowResolvesPercentageTracksAgainstContainingWidth(t *testing.T) {
	const source = "document:\n  page:\n    width: 200pt\n    height: 100pt\n    margin: 10pt\n    body:\n      row:\n        gap: 10pt\n        paragraph @left:\n          track-size: 50%\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"LEFT\"\n        paragraph @right:\n          track-size: 50%\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"RIGHT\"\n"
	plan, result, err := PlanPaper("responsive-row.paper", source)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].BorderBox.Width.Points() != 85 || fragments[0].BorderBox.X.Points() != 10 ||
		fragments[1].BorderBox.Width.Points() != 85 || fragments[1].BorderBox.X.Points() != 105 {
		t.Fatalf("percentage fragments = %+v", fragments)
	}
}

func TestPlanPaperRowSupportsResponsiveImageChildren(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	source := "document:\n  page:\n    width: 200pt\n    height: 100pt\n    margin: 10pt\n    body:\n      row @media:\n        image @hero:\n          track-size: 40%\n          source: \"data:image/png;base64," + pixel + "\"\n          width: 100%\n          height: 20pt\n          alt: \"Evidence\"\n        paragraph @copy:\n          track-size: 60%\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"Caption\"\n"
	plan, result, err := PlanPaper("row-image.paper", source)
	if err != nil || !result.OK() || plan.Hash() == "" || plan.PageCount() != 1 {
		t.Fatalf("PlanPaper() = plan=%#v result=%#v err=%v", plan, result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.Images) != 1 || len(projection.ImageResources) != 1 {
		t.Fatalf("planned images = %#v resources=%#v", projection.Images, projection.ImageResources)
	}
}

func TestPlanPaperRowSupportsResponsiveTableChildren(t *testing.T) {
	const source = "document:\n  page:\n    width: 240pt\n    height: 120pt\n    margin: 10pt\n    body:\n      row @summary:\n        table @facts:\n          track-size: 70%\n          table-track:\n            width: 50%\n          table-track:\n            width: 50%\n          table-row:\n            cell:\n              text: \"Name\"\n            cell:\n              text: \"Value\"\n        paragraph @aside:\n          track-size: 30%\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"Aside\"\n"
	plan, result, err := PlanPaper("row-table.paper", source)
	if err != nil || !result.OK() || plan.Hash() == "" || plan.PageCount() != 1 {
		t.Fatalf("PlanPaper() = plan=%#v result=%#v err=%v", plan, result, err)
	}
	projection := plan.plan.Projection()
	var tableNodes int
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleTable {
			tableNodes++
		}
	}
	if tableNodes != 1 {
		t.Fatalf("table semantic nodes = %d", tableNodes)
	}
}

func TestPlanPaperRowSupportsWrappedFlexLayoutProperties(t *testing.T) {
	const source = "document:\n  page:\n    width: 180pt\n    height: 140pt\n    margin: 10pt\n    body:\n      row @wrapped:\n        cross-gap: 4pt\n        cross-size: 80pt\n        wrap: \"wrap\"\n        main-align: \"space-between\"\n        align-content: \"space-around\"\n        paragraph @one:\n          track-size: 70pt\n          track-grow: 1\n          track-shrink: 1\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"One\"\n        paragraph @two:\n          track-size: 70pt\n          track-grow: 1\n          track-shrink: 1\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"Two\"\n        paragraph @three:\n          track-size: 70pt\n          track-grow: 1\n          track-shrink: 1\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"Three\"\n"
	plan, result, err := PlanPaper("wrapped-flex.paper", source)
	if err != nil || !result.OK() || plan.Hash() == "" || plan.PageCount() != 1 {
		t.Fatalf("PlanPaper() = plan=%#v result=%#v err=%v", plan, result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 3 || fragments[2].BorderBox.Y <= fragments[0].BorderBox.Y {
		t.Fatalf("wrapped fragments = %+v", fragments)
	}
}

func TestPlanPaperSupportsOneReadableNestedRowColumnLevel(t *testing.T) {
	const source = "document:\n  page:\n    width: 220pt\n    height: 140pt\n    margin: 10pt\n    body:\n      row @outer:\n        column @details:\n          track-size: 70%\n          gap: 2pt\n          paragraph @first:\n            track-size: 14pt\n            font: \"Courier\"\n            size: 10pt\n            line-height: 12pt\n            text: \"First\"\n          paragraph @second:\n            track-size: 14pt\n            font: \"Courier\"\n            size: 10pt\n            line-height: 12pt\n            text: \"Second\"\n        paragraph @aside:\n          track-size: 30%\n          font: \"Courier\"\n          size: 10pt\n          line-height: 12pt\n          text: \"Aside\"\n"
	plan, result, err := PlanPaper("nested-layout.paper", source)
	if err != nil || !result.OK() || plan.Hash() == "" || plan.PageCount() != 1 {
		t.Fatalf("PlanPaper() = plan=%#v result=%#v err=%v", plan, result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) < 4 || len(projection.GlyphRuns) != 3 {
		t.Fatalf("nested projection = fragments=%d runs=%d", len(projection.Fragments), len(projection.GlyphRuns))
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
