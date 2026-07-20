// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestTypedTableListItemsOwnDistinctContentFragmentsAndPDFStructure(t *testing.T) {
	text := func(value string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: value}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 11}}
	}
	list := layout.ListBlock{Ordered: true, Items: []layout.ListItem{
		{Blocks: []layout.Block{text("alpha"), layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{text("nested")}}}}}},
		{Blocks: []layout.Block{text("beta")}},
	}}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{{Width: 160}},
		Body:    []layout.TableRow{{Cells: []layout.TableCell{{Blocks: []layout.Block{list}}}}},
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	planner.EnableTaggedPDF()
	planner.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Typed list table", Lang: "en-US"})
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Title: "Typed list table", Language: "en-US", Body: []layout.Block{table}})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	byID := make(map[layoutengine.SemanticNodeID]layoutengine.SemanticNode, len(projection.SemanticNodes))
	owner := make(map[layoutengine.FragmentID]layoutengine.SemanticNodeID, len(projection.SemanticFragments))
	for _, node := range projection.SemanticNodes {
		byID[node.ID] = node
	}
	for _, association := range projection.SemanticFragments {
		owner[association.Fragment] = association.Semantic
	}
	items, ownedParagraphs := 0, 0
	distinctItems := make(map[layoutengine.SemanticNodeID]bool)
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleListItem {
			items++
			if byID[node.Parent].Role != layoutengine.SemanticRoleList {
				t.Fatalf("list item parent = %+v", byID[node.Parent])
			}
		}
	}
	for _, fragment := range projection.Fragments {
		semantic := byID[owner[fragment.ID]]
		if semantic.Role != layoutengine.SemanticRoleParagraph {
			continue
		}
		parent := byID[semantic.Parent]
		if parent.Role == layoutengine.SemanticRoleListItem {
			ownedParagraphs++
			distinctItems[parent.ID] = true
		}
	}
	if items != 3 || ownedParagraphs != 3 || len(distinctItems) != 3 || len(projection.SemanticFragments) != len(projection.Fragments) {
		t.Fatalf("list semantic ownership items=%d paragraphs=%d distinct=%d nodes=%+v associations=%+v", items, ownedParagraphs, len(distinctItems), projection.SemanticNodes, projection.SemanticFragments)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"/S /Table", "/S /TR", "/S /TD", "/S /L", "/S /LI", "/S /P"} {
		if !bytes.Contains(output.Bytes(), []byte(token)) {
			t.Fatalf("tagged typed table PDF lacks %q", token)
		}
	}
}

func TestTypedTableCaptionPreservesMixedRunsAndExactLinks(t *testing.T) {
	table := layout.TableBlock{
		CaptionSegments: []layout.TextSegment{
			{Text: "Alpha", Destination: "caption", Style: layout.TextStyle{Color: layout.DocumentColor{R: 220, G: 10, B: 20, Set: true}}},
			{Text: " Beta", Link: "#caption", Style: layout.TextStyle{Italic: true, Color: layout.DocumentColor{R: 10, G: 30, B: 220, Set: true}}},
			{Text: " URI", Link: "https://example.test/caption", Style: layout.TextStyle{Color: layout.DocumentColor{R: 10, G: 150, B: 40, Set: true}}},
		},
		Columns: []layout.TableColumn{{Width: 160}},
		Body:    []layout.TableRow{{Cells: []layout.TableCell{{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "body"}}}}}}}},
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 120}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Language: "en-US", Body: []layout.Block{table}})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Destinations) != 1 || len(projection.Links) != 2 {
		t.Fatalf("caption destinations/links = %+v / %+v", projection.Destinations, projection.Links)
	}
	internal, external := false, false
	for _, link := range projection.Links {
		internal = internal || link.Destination == 1
		external = external || link.URI == "https://example.test/caption"
	}
	faces := make(map[layoutengine.FontResourceID]layoutengine.CoreFontFace)
	for _, font := range projection.Fonts {
		faces[font.ID] = font.Face
	}
	colors := make(map[string]layoutengine.CoreRGBColor)
	fontFaces := make(map[string]layoutengine.CoreFontFace)
	for _, run := range projection.GlyphRuns {
		for _, token := range []string{"Alpha", "Beta", "URI"} {
			if strings.Contains(run.Codes, token) {
				colors[token], fontFaces[token] = run.Color, faces[run.Font]
			}
		}
	}
	if !internal || !external || colors["Alpha"] != (layoutengine.CoreRGBColor{R: 220, G: 10, B: 20, Set: true}) || colors["Beta"] != (layoutengine.CoreRGBColor{R: 10, G: 30, B: 220, Set: true}) || colors["URI"] != (layoutengine.CoreRGBColor{R: 10, G: 150, B: 40, Set: true}) || fontFaces["Alpha"] != layoutengine.CoreFontHelveticaBold || fontFaces["Beta"] != layoutengine.CoreFontHelveticaBoldOblique {
		t.Fatalf("caption run evidence internal=%t external=%t colors=%+v faces=%+v", internal, external, colors, fontFaces)
	}
}

func TestTypedTablePoliciesReachPlanBreaksRelaxationAndCancellation(t *testing.T) {
	paragraph := func(value string) []layout.Block {
		return []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: value}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 30}}}
	}
	table := layout.TableBlock{Columns: []layout.TableColumn{{Width: 80}}, Box: layout.BoxStyle{Orphans: 2, Widows: 2}}
	for index := 0; index < 5; index++ {
		table.Body = append(table.Body, layout.TableRow{KeepTogether: true, Cells: []layout.TableCell{{Box: layout.BoxStyle{KeepTogether: true}, Blocks: paragraph(string(rune('A' + index)))}}})
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
	if err != nil || plan.PageCount() != 3 {
		t.Fatalf("policy plan pages=%d, %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Breaks) != 2 || projection.Pages[0].Fragments.Count != 4 || projection.Pages[1].Fragments.Count != 2 || projection.Pages[2].Fragments.Count != 4 {
		t.Fatalf("policy projection pages=%+v breaks=%+v", projection.Pages, projection.Breaks)
	}
	keep := table
	keep.Box = layout.BoxStyle{KeepTogether: true}
	kept, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{keep}})
	if err != nil || kept.PageCount() != 1 {
		t.Fatalf("oversize keep pages=%d, %v", kept.PageCount(), err)
	}
	diagnostics := kept.plan.Projection().Diagnostics
	if len(diagnostics) < 2 || diagnostics[len(diagnostics)-1].Code != layoutengine.DiagnosticKeepTooLarge {
		t.Fatalf("oversize keep diagnostics=%+v", diagnostics)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	atomic := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}))
	atomic.SetMargins(10, 10, 10)
	zero, err := atomic.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{Body: []layout.Block{table}})
	if !errors.Is(err, context.Canceled) || zero.Hash() != "" || atomic.PageCount() != 0 {
		t.Fatalf("canceled policy plan hash=%q pages=%d err=%v", zero.Hash(), atomic.PageCount(), err)
	}
}

func TestTypedTableKeepWithNextMovesExactTableAndTextChain(t *testing.T) {
	paragraph := func(value string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: value}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}}
	}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{{Width: 80}}, Box: layout.BoxStyle{KeepWithNext: true},
		Body: []layout.TableRow{{KeepTogether: true, Cells: []layout.TableCell{{Blocks: []layout.Block{paragraph("table\nrow")}}}}},
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{paragraph("before-1\nbefore-2\nbefore-3\nbefore-4\nbefore-5"), table, paragraph("after-1\nafter-2")}})
	if err != nil {
		t.Fatalf("keep-with-next chain pages=%d, %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var beforePage, tablePage, afterPage uint32
	for _, fragment := range projection.Fragments {
		switch {
		case strings.Contains(string(fragment.Key), "@typed-table-r1-c1") && !strings.Contains(string(fragment.Key), "/content-"):
			tablePage = fragment.Page
		case beforePage == 0 && fragment.Page == 1:
			beforePage = fragment.Page
		case fragment.Page == 2 && !strings.Contains(string(fragment.Key), "@typed-table"):
			afterPage = fragment.Page
		}
	}
	if plan.PageCount() != 2 || beforePage != 1 || tablePage != 2 || afterPage != 2 || len(projection.Breaks) == 0 || projection.Breaks[0].Reason != layoutengine.BreakPaginationConstraint || projection.Breaks[0].Required <= projection.Breaks[0].Available {
		t.Fatalf("keep-with-next placement before/table/after=%d/%d/%d breaks=%+v fragments=%+v", beforePage, tablePage, afterPage, projection.Breaks, projection.Fragments)
	}
}
