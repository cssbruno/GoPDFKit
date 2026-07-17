// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

const paperTableSource = "document @report:\n" +
	"  language: \"en\"\n" +
	"  page @sheet:\n" +
	"    width: 200pt\n" +
	"    height: 120pt\n" +
	"    margin: 8pt\n" +
	"    body @body:\n" +
	"      table @ledger:\n" +
	"        caption: \"Ledger\"\n" +
	"        repeat-header: true\n" +
	"        split: \"rows\"\n" +
	"        table-track @name-track:\n" +
	"          width: 100pt\n" +
	"        table-track @value-track:\n" +
	"          width: 84pt\n" +
	"        table-header @head:\n" +
	"          table-row @head-row:\n" +
	"            cell @name-head:\n" +
	"              text: \"Name\"\n" +
	"            cell @value-head:\n" +
	"              text: \"Value\"\n" +
	"        table-row @body-row:\n" +
	"          cell @name:\n" +
	"            text: \"Alpha\"\n" +
	"          cell @value:\n" +
	"            paragraph:\n" +
	"              text: \"10\"\n"

func TestPaperTablePlansRendersCapturesAndRetainsTableSemantics(t *testing.T) {
	plan, result, err := PlanPaper("table.paper", paperTableSource)
	if err != nil || !result.OK() || result.Pages != 1 {
		t.Fatalf("PlanPaper(table) = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	roles := make(map[layoutengine.SemanticRole]int)
	for _, semantic := range projection.SemanticNodes {
		roles[semantic.Role]++
	}
	for _, role := range []layoutengine.SemanticRole{
		layoutengine.SemanticRoleTable,
		layoutengine.SemanticRoleRow,
		layoutengine.SemanticRoleCell,
	} {
		if roles[role] == 0 {
			t.Fatalf("semantic role %q missing from %#v", role, projection.SemanticNodes)
		}
	}
	if len(projection.ReadingOrder) < 4 || len(projection.Fragments) < 4 {
		t.Fatalf("table projection lost reading/fragment structure: %d/%d", len(projection.ReadingOrder), len(projection.Fragments))
	}
	query, err := plan.Query(PaperPlanSelector{Key: "@ledger", MaxResults: 32})
	if err != nil || !bytes.Contains(query.JSON(), []byte(`"key":"@ledger"`)) || !bytes.Contains(query.JSON(), []byte(`"file":"table.paper"`)) {
		t.Fatalf("table source identity = %s, %v", query.JSON(), err)
	}

	display, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || bytes.Count(display.SVG, []byte("<text ")) < len("LedgerNameValueAlpha10") {
		t.Fatalf("table display capture = %q, %v", display.SVG, err)
	}
	rasterRequest := DefaultPaperPlanRasterRequest()
	rasterRequest.CoreFontProgram = goregular.TTF
	raster, err := plan.CaptureRasterPages(context.Background(), rasterRequest)
	if err != nil || len(raster.Pages) != 1 || len(raster.Pages[0].PNG) == 0 {
		t.Fatalf("table raster = %#v, %v", raster, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	rendered, err := target.WritePaperPlan(plan)
	if err != nil || !rendered.OK() || target.PageCount() != 1 {
		t.Fatalf("WritePaperPlan(table) = %#v, %v", rendered, err)
	}
	var pdf bytes.Buffer
	if err := target.Output(&pdf); err != nil || pdf.Len() == 0 {
		t.Fatalf("table PDF = %d bytes, %v", pdf.Len(), err)
	}

	changed, changedResult, err := PlanPaper("table.paper", strings.Replace(paperTableSource, "Alpha", "Beta", 1))
	if err != nil || !changedResult.OK() || changed.Hash() == plan.Hash() {
		t.Fatalf("table content did not participate in plan identity: %q/%q, %v", plan.Hash(), changed.Hash(), err)
	}
}

func TestPaperTableMultiPageRepeatHeaderSplitSpansAndNestedContent(t *testing.T) {
	const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	source := "document @report:\n  page @sheet:\n    width: 180pt\n    height: 96pt\n    margin: 6pt\n    body @body:\n      table @ledger:\n        repeat-header: true\n        split: \"rows\"\n        table-track @left-track:\n          width: 84pt\n        table-track @right-track:\n          width: 84pt\n        table-header @head:\n          table-row @head-row:\n            cell @head-cell:\n              colspan: 2\n              text: \"REPEATED HEADER\"\n"
	for index := 0; index < 10; index++ {
		source += fmt.Sprintf("        table-row @row-%d:\n          cell @label-%d:\n            text: \"Row %d\"\n          cell @value-%d:\n            text: \"Value %d\"\n", index, index, index, index, index)
	}
	source += "        table-row @nested-row:\n          cell @nested-list:\n            list:\n              item:\n                text: \"Nested one\"\n              item:\n                text: \"Nested two\"\n          cell @nested-image:\n            image:\n              source: \"data:image/png;base64," + pixel + "\"\n              width: 12pt\n              height: 12pt\n              alt: \"Nested evidence\"\n"
	plan, result, err := PlanPaper("multi-table.paper", source)
	if err != nil || !result.OK() || result.Pages < 2 {
		t.Fatalf("PlanPaper = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	headerCells, figures, lists := 0, 0, 0
	headerIDs := map[layoutengine.SemanticNodeID]bool{}
	for _, semantic := range projection.SemanticNodes {
		if semantic.Attributes.TableHeader {
			headerCells++
			headerIDs[semantic.ID] = true
		}
		if semantic.Role == layoutengine.SemanticRoleFigure {
			figures++
		}
		if semantic.Role == layoutengine.SemanticRoleList {
			lists++
		}
	}
	if headerCells == 0 || figures == 0 || lists == 0 {
		t.Fatalf("semantics header=%d figure=%d list=%d", headerCells, figures, lists)
	}
	headerOwned := map[layoutengine.SemanticNodeID]bool{}
	for _, node := range projection.SemanticNodes {
		id := node.ID
		parent := node.Parent
		for parent.Valid() {
			if headerIDs[parent] {
				headerOwned[id] = true
				break
			}
			parent = projection.SemanticNodes[parent-1].Parent
		}
	}
	headerPages := map[uint32]bool{}
	for _, association := range projection.SemanticFragments {
		if headerIDs[association.Semantic] || headerOwned[association.Semantic] {
			headerPages[association.Page] = true
		}
	}
	if len(headerPages) != result.Pages {
		t.Fatalf("repeated header pages=%v want %d", headerPages, result.Pages)
	}
	for page := uint32(1); page <= uint32(result.Pages); page++ {
		capture, err := plan.CaptureDisplayPageSVG(context.Background(), page, nil)
		if err != nil || len(capture.SVG) == 0 {
			t.Fatalf("page %d header capture=%v", page, err)
		}
	}
	rasterRequest := DefaultPaperPlanRasterRequest()
	rasterRequest.CoreFontProgram = goregular.TTF
	raster, err := plan.CaptureRasterPages(context.Background(), rasterRequest)
	if err != nil || len(raster.Pages) != result.Pages {
		t.Fatalf("raster pages=%d/%d %v", len(raster.Pages), result.Pages, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	rendered, err := target.WritePaperPlan(plan)
	if err != nil || !rendered.OK() || rendered.Pages != result.Pages {
		t.Fatalf("paint=%#v %v", rendered, err)
	}
}

func TestPaperFixedTableTracksRemainValidWhenPageChangesToA4(t *testing.T) {
	source := "document @report:\n" +
		"  page @sheet:\n" +
		"    width: 595.275590551pt\n" +
		"    height: 841.88976378pt\n" +
		"    margin: 6pt\n" +
		"    body @body:\n" +
		"      table @ledger:\n" +
		"        table-track @left:\n" +
		"          width: 84pt\n" +
		"        table-track @right:\n" +
		"          width: 84pt\n" +
		"        table-row @row:\n" +
		"          cell @label:\n" +
		"            text: \"Label\"\n" +
		"          cell @value:\n" +
		"            text: \"Value\"\n"

	plan, result, err := PlanPaper("a4-fixed-table.paper", source)
	if err != nil || !result.OK() || result.Pages != 1 || plan.Hash() == "" {
		t.Fatalf("PlanPaper(A4 fixed table) = %#v, %v", result, err)
	}
}
