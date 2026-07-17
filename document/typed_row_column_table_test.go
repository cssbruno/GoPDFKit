// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestTypedRowColumnTableStructuredCaptionStyleRefDestinationsCaptureRasterAndPDF(t *testing.T) {
	shared := layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12, Bold: true}
	side := layout.BorderSide{Width: 1, Style: "solid", Color: layout.DocumentColor{R: 20, G: 40, B: 180, Set: true}}
	table := layout.TableBlock{
		CaptionSegments: []layout.TextSegment{
			{Text: "CAP", Destination: "caption"},
			{Text: " jump", Link: "#caption"},
			{Text: " docs", Link: "https://example.test/table"},
		},
		Columns: []layout.TableColumn{{Width: 70}, {Width: 70}},
		Header: []layout.TableRow{{Cells: []layout.TableCell{
			{Blocks: []layout.Block{rowColumnTableParagraph("H1")}, Header: true},
			{Blocks: []layout.Block{rowColumnTableParagraph("H2")}, Header: true},
		}}},
		Body: []layout.TableRow{{Cells: []layout.TableCell{
			{Blocks: []layout.Block{rowColumnTableParagraph("VALUE")}, StyleRef: &shared,
				Box: layout.BoxStyle{BackgroundColor: layout.DocumentColor{R: 240, G: 245, B: 255, Set: true}, Border: layout.BorderStyle{Top: side, Right: side, Bottom: side, Left: side}}},
			{Blocks: []layout.Block{rowColumnTableParagraph("OTHER")}},
		}}},
	}
	container := layout.RowColumnBlock{Direction: layout.RowDirection, Gap: 10, Items: []layout.RowColumnItem{
		{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 140}, Block: table},
		{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 70}, Block: rowColumnTableParagraph("SIDE")},
	}}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 120}), WithNoCompression(), WithDeterministicOutput())
	doc := &layout.LayoutDocument{Language: "en-US", PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Left: 10, Top: 10, Right: 10, Bottom: 10}}, Body: []layout.Block{container}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 1 || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("row/column table plan = pages %d hash %q source pages %d, %v", plan.PageCount(), plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Destinations) != 1 || len(projection.Links) != 2 || projection.Links[0].Destination != 1 || projection.Links[1].URI != "https://example.test/table" {
		t.Fatalf("nested table destinations/links = %+v / %+v", projection.Destinations, projection.Links)
	}
	if len(projection.Fonts) != 3 { // Caption bold, StyleRef bold, and shared regular table/side text.
		t.Fatalf("nested table deduplicated fonts = %+v", projection.Fonts)
	}
	var tableRole, cellRole, artifact bool
	for _, node := range projection.SemanticNodes {
		switch node.Role {
		case layoutengine.SemanticRoleTable:
			tableRole = true
		case layoutengine.SemanticRoleCell:
			cellRole = true
		case layoutengine.SemanticRoleArtifact:
			artifact = true
		}
	}
	if !tableRole || !cellRole || !artifact || len(projection.SemanticFragments) != len(projection.Fragments) {
		t.Fatalf("nested table semantics nodes=%+v associations=%d fragments=%d", projection.SemanticNodes, len(projection.SemanticFragments), len(projection.Fragments))
	}
	if len(projection.Fills) != 1 || len(projection.Strokes) != 4 || len(projection.Paths) != 5 {
		t.Fatalf("nested table decorations fills/strokes/paths = %d/%d/%d", len(projection.Fills), len(projection.Strokes), len(projection.Paths))
	}
	display, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(display.SVG(), []byte(">S</text>")) || !bytes.Contains(display.SVG(), []byte(`fill="#f0f5ff"`)) {
		t.Fatalf("nested table display capture = %v\n%s", err, display.SVG())
	}
	before := plan.Hash()
	shared.FontSize = 44
	table.CaptionSegments[0].Text = "mutated"
	if plan.Hash() != before || plan.plan.Projection().Destinations[0] != projection.Destinations[0] {
		t.Fatal("nested table plan aliases StyleRef or structured caption segments")
	}

	plain := table
	plain.Body = cloneTypedTableRows(table.Body)
	plain.Body[0].Cells[0].Box = layout.BoxStyle{}
	plainContainer := container
	plainContainer.Items = append([]layout.RowColumnItem(nil), container.Items...)
	plainContainer.Items[0].Block = plain
	rasterPlan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: doc.PageTemplate, Body: []layout.Block{plainContainer}})
	if err != nil {
		t.Fatal(err)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "row-column-table", rasterPlan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("nested table raster = status %q evidence %+v, %v", status, raster, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 {
		t.Fatalf("nested table PDF replay = pages %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil ||
		!bytes.Contains(pdf.Bytes(), []byte("/Dest [")) || !bytes.Contains(pdf.Bytes(), []byte("/URI (https://example.test/table)")) {
		t.Fatalf("nested table PDF links = %d bytes, %v", pdf.Len(), err)
	}
}

func TestTypedRowColumnTablePaginationCaptionConflictsAndAtomicFailures(t *testing.T) {
	base := layout.TableBlock{Columns: []layout.TableColumn{{Width: 80}}, Body: []layout.TableRow{{Cells: []layout.TableCell{{Blocks: []layout.Block{rowColumnTableParagraph("row")}}}}}}
	container := func(table layout.TableBlock) layout.RowColumnBlock {
		return layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 80}, Block: table}}}
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 70}))
	doc := func(table layout.TableBlock) *layout.LayoutDocument {
		return &layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Left: 10, Top: 10, Right: 10, Bottom: 10}}, Body: []layout.Block{container(table)}}
	}
	conflict := base
	conflict.Caption = "plain"
	conflict.CaptionSegments = []layout.TextSegment{{Text: "structured"}}
	if plan, err := planner.PlanLayoutDocument(doc(conflict)); !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || !bytes.Contains([]byte(err.Error()), []byte("mutually exclusive")) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("caption conflict = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	tall := base
	for index := 0; index < 10; index++ {
		tall.Body = append(tall.Body, layout.TableRow{Cells: []layout.TableCell{{Blocks: []layout.Block{rowColumnTableParagraph("another row")}}}})
	}
	if plan, err := planner.PlanLayoutDocument(doc(tall)); err != nil || plan.Hash() == "" || plan.PageCount() < 2 || planner.PageCount() != 0 {
		t.Fatalf("nested pagination plan = hash %q plan pages %d source pages %d, %v", plan.Hash(), plan.PageCount(), planner.PageCount(), err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if plan, err := planner.PlanLayoutDocumentContext(canceled, doc(base)); !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("nested table cancellation = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	limited, err := WithPlanningWorkLimit(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if plan, err := planner.PlanLayoutDocumentContext(limited, doc(base)); !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("nested table work limit = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
}

func TestTypedRowColumnMultiPageTablePreservesTrackMastersBreaksResourcesAndSemantics(t *testing.T) {
	header := layout.TableRow{Cells: []layout.TableCell{{Header: true, Blocks: []layout.Block{rowColumnTableParagraph("HEADER")}}}}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{{Width: 78}}, Header: []layout.TableRow{header},
		Style: layout.TableStyle{RepeatHeader: true},
	}
	for index := 0; index < 14; index++ {
		segments := []layout.TextSegment{{Text: "row"}}
		if index == 0 {
			segments = []layout.TextSegment{{Text: "target", Destination: "nested-target"}, {Text: " docs", Link: "https://example.test/nested"}}
		}
		table.Body = append(table.Body, layout.TableRow{Cells: []layout.TableCell{{Blocks: []layout.Block{
			layout.ParagraphBlock{Segments: segments, Style: layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12}},
		}}}})
	}
	container := layout.RowColumnBlock{Direction: layout.RowDirection, Gap: 8, Items: []layout.RowColumnItem{
		{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 78}, Block: table},
		{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 24}, Block: rowColumnTableParagraph("SIDE")},
	}}
	doc := &layout.LayoutDocument{
		Language: "en-US",
		PageTemplate: layout.PageTemplate{
			Margins:         layout.Spacing{Left: 10, Top: 8, Right: 10, Bottom: 8},
			FirstPageHeader: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("FIRST MASTER"), pageShellParagraph("SECOND LINE")}},
			Header:          &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("NEXT")}},
		},
		Body: []layout.Block{container},
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 130, Ht: 82}), WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Pages) < 3 || len(projection.Breaks) != len(projection.Pages)-1 {
		t.Fatalf("nested table pages/breaks = %d/%d", len(projection.Pages), len(projection.Breaks))
	}
	var readingPage uint32
	var readingIndex uint32
	var lastPageOne layoutengine.ReadingOccurrence
	for position, occurrence := range projection.ReadingOrder {
		if occurrence.Page != readingPage {
			if occurrence.Page <= readingPage {
				t.Fatalf("reading order is not page-canonical at %d: %+v", position, projection.ReadingOrder)
			}
			readingPage, readingIndex = occurrence.Page, 0
		}
		if occurrence.ReadingIndex != readingIndex {
			t.Fatalf("reading order page %d index = %d, want %d", occurrence.Page, occurrence.ReadingIndex, readingIndex)
		}
		readingIndex++
		if occurrence.Page == 1 {
			lastPageOne = occurrence
		}
	}
	if !lastPageOne.Fragment.Valid() || projection.Fragments[lastPageOne.Fragment-1].Node != 2 {
		t.Fatalf("page-one authored reading order does not end with the following sibling: %+v", lastPageOne)
	}
	for pageIndex, decision := range projection.Breaks {
		if decision.FromPage != uint32(pageIndex+1) || decision.ToPage != uint32(pageIndex+2) ||
			decision.Reason != layoutengine.BreakPaginationConstraint || !decision.Preceding.Valid() || !decision.Triggering.Valid() {
			t.Fatalf("break[%d] = %+v", pageIndex, decision)
		}
	}
	var repeatedHeaders int
	var sidePages []uint32
	var tableOuter []layoutengine.Fragment
	for _, fragment := range projection.Fragments {
		isHeader := fragment.Key == "@typed-table-r1-c1" || bytes.HasSuffix([]byte(fragment.Key), []byte("/@typed-table-r1-c1"))
		if fragment.Repeated && isHeader {
			repeatedHeaders++
		}
		if isHeader {
			if fragment.Repeated && fragment.Page == 1 {
				t.Fatalf("first-page header marked repeated: %+v", fragment)
			}
		}
		if fragment.Node == 2 {
			sidePages = append(sidePages, fragment.Page)
		}
		if fragment.Node == 1 {
			tableOuter = append(tableOuter, fragment)
		}
	}
	if repeatedHeaders != len(projection.Pages)-1 {
		t.Fatalf("repeated headers = %d, want %d", repeatedHeaders, len(projection.Pages)-1)
	}
	if len(sidePages) != 1 || sidePages[0] != 1 {
		t.Fatalf("non-paginating sibling pages = %v, want [1]", sidePages)
	}
	if len(tableOuter) != len(projection.Pages) || tableOuter[0].Continuation != layoutengine.ContinuationStart || tableOuter[len(tableOuter)-1].Continuation != layoutengine.ContinuationEnd {
		t.Fatalf("outer table continuations = %+v", tableOuter)
	}
	if len(tableOuter) > 1 && tableOuter[0].BorderBox.Y == tableOuter[1].BorderBox.Y {
		t.Fatalf("page-master body offsets were not preserved: page 1 y=%d page 2 y=%d", tableOuter[0].BorderBox.Y, tableOuter[1].BorderBox.Y)
	}
	if len(projection.Destinations) != 1 || projection.Destinations[0].Page != 1 || len(projection.Links) < 1 || projection.Links[0].URI != "https://example.test/nested" {
		t.Fatalf("nested destinations/links = %+v / %+v", projection.Destinations, projection.Links)
	}
	for page := 1; page <= len(projection.Pages); page++ {
		capture, captureErr := plan.CaptureDisplayPage(uint32(page))
		if captureErr != nil || len(capture.SVG()) == 0 {
			t.Fatalf("capture page %d = %v", page, captureErr)
		}
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "row-column-table-multipage", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != len(projection.Pages) {
		t.Fatalf("multi-page raster = %q %+v, %v", status, raster, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, writeErr := target.WriteLayoutDocumentPlan(plan); writeErr != nil || pages != len(projection.Pages) {
		t.Fatalf("PDF replay = %d pages, %v", pages, writeErr)
	}
	var pdf bytes.Buffer
	if outputErr := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); outputErr != nil || bytes.Count(pdf.Bytes(), []byte("/Type /Page")) < len(projection.Pages) {
		t.Fatalf("PDF output = %d bytes, %v", pdf.Len(), outputErr)
	}
	before := plan.Hash()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if failed, cancelErr := planner.PlanLayoutDocumentContext(canceled, doc); !errors.Is(cancelErr, context.Canceled) || failed.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled multi-page nested table = hash %q source pages %d, %v", failed.Hash(), planner.PageCount(), cancelErr)
	}
	const concurrentPlans = 4
	hashes := make(chan string, concurrentPlans)
	errorsFound := make(chan error, concurrentPlans)
	var group sync.WaitGroup
	for index := 0; index < concurrentPlans; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			candidate, candidateErr := planner.PlanLayoutDocument(doc)
			if candidateErr != nil {
				errorsFound <- candidateErr
				return
			}
			hashes <- candidate.Hash()
		}()
	}
	group.Wait()
	close(hashes)
	close(errorsFound)
	for candidateErr := range errorsFound {
		t.Fatal(candidateErr)
	}
	for hash := range hashes {
		if hash != before {
			t.Fatalf("concurrent plan hash = %q, want %q", hash, before)
		}
	}
	table.Body[0].Cells[0].Blocks = []layout.Block{rowColumnTableParagraph("mutated")}
	if plan.Hash() != before {
		t.Fatal("multi-page nested table plan aliases authored rows")
	}
	limited := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 130, Ht: 82}), WithLimits(Limits{MaxPages: 1}))
	if failed, limitErr := limited.PlanLayoutDocument(doc); limitErr == nil || failed.Hash() != "" || limited.PageCount() != 0 {
		t.Fatalf("nested table page-limit failure = hash %q source pages %d, %v", failed.Hash(), limited.PageCount(), limitErr)
	}
	bad := *doc
	twoTables := table
	twoTables.Columns = []layout.TableColumn{{Width: 50}}
	badContainer := layout.RowColumnBlock{Direction: layout.RowDirection, Gap: 8, Items: []layout.RowColumnItem{
		{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 50}, Block: twoTables},
		{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 50}, Block: twoTables},
	}}
	bad.Body = []layout.Block{badContainer}
	if failed, badErr := planner.PlanLayoutDocument(&bad); !errors.Is(badErr, ErrLayoutDocumentPlanUnsupported) || failed.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("two independently paginating siblings = hash %q source pages %d, %v", failed.Hash(), planner.PageCount(), badErr)
	}
}

func rowColumnTableParagraph(text string) layout.ParagraphBlock {
	return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12}}
}
