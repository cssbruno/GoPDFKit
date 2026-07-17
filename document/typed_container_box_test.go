// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func typedContainerTestBox() layout.BoxStyle {
	background := layout.DocumentColor{R: 224, G: 236, B: 255, Set: true}
	borderColor := layout.DocumentColor{R: 160, G: 24, B: 24, Set: true}
	border := layout.BorderSide{Width: 1, Style: "solid", Color: borderColor}
	return layout.BoxStyle{
		Margin:          layout.Spacing{Top: 2, Right: 3, Bottom: 4, Left: 5},
		Padding:         layout.Spacing{Top: 6, Right: 7, Bottom: 8, Left: 9},
		Border:          layout.BorderStyle{Top: border, Right: border, Bottom: border, Left: border},
		BackgroundColor: background,
	}
}

func TestTypedDecoratedMultiChildContainersPlanCapturePDFAndSemantics(t *testing.T) {
	planner := paginationTestDocument(t, 420)
	planner.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Decorated containers"})
	box := typedContainerTestBox()
	doc := &layout.LayoutDocument{Language: "en-US", Body: []layout.Block{
		layout.SectionBlock{Title: "Section", Box: box, Blocks: []layout.Block{
			paginationParagraph("first", layout.BoxStyle{}), paginationParagraph("second", layout.BoxStyle{}),
		}},
		layout.ClauseBlock{Number: "1", Title: "Clause", Box: box, Blocks: []layout.Block{
			paginationParagraph("clause one", layout.BoxStyle{}), paginationParagraph("clause two", layout.BoxStyle{}),
		}},
		layout.NoteBoxBlock{Title: "Note", Box: box, Body: []layout.Block{
			paginationParagraph("note one", layout.BoxStyle{}), paginationParagraph("note two", layout.BoxStyle{}),
		}},
		layout.MetadataGridBlock{Columns: 1, Box: box, Fields: []layout.MetadataField{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}},
		layout.SignatureRowBlock{Box: box, Columns: []layout.SignatureColumn{{Name: "Signer"}}},
	}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || planner.PageCount() != 0 || plan.Hash() == "" {
		t.Fatalf("plan = hash %q source pages %d error %v", plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fills) < 20 || len(projection.Paths) < len(projection.Fills) {
		t.Fatalf("decorated display resources = paths %d fills %d", len(projection.Paths), len(projection.Fills))
	}
	var sections, cells int
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleSection {
			sections++
		}
		if node.Role == layoutengine.SemanticRoleCell {
			cells++
		}
	}
	if sections < 5 || cells < 3 {
		t.Fatalf("semantic section/cell counts = %d/%d", sections, cells)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || len(capture.SVG()) == 0 {
		t.Fatalf("capture = %d bytes, %v", len(capture.SVG()), err)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "typed-container-boxes", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != plan.PageCount() || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("raster = status %q evidence %+v, %v", status, raster, err)
	}
	target := paginationTestDocument(t, 420)
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.Output(&pdf); err != nil || pdf.Len() == 0 {
		t.Fatalf("PDF = %d bytes, %v", pdf.Len(), err)
	}
	if !bytes.Contains(pdf.Bytes(), []byte("/StructTreeRoot")) || !bytes.Contains(pdf.Bytes(), []byte("/S /Sect")) {
		t.Fatal("PDF/UA output lacks the decorated container structure hierarchy")
	}
}

func TestTypedDecoratedContainerInvalidCancellationConcurrencyAndAtomicity(t *testing.T) {
	invalid := typedContainerTestBox()
	invalid.Border.Left.Width = -1
	planner := paginationTestDocument(t, 100)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.SectionBlock{Box: invalid, Blocks: []layout.Block{paginationParagraph("body", layout.BoxStyle{})}},
	}})
	if err == nil || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("invalid plan = hash %q pages %d error %v", plan.Hash(), planner.PageCount(), err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planner.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{Body: []layout.Block{
		layout.NoteBoxBlock{Box: typedContainerTestBox(), Body: []layout.Block{paginationParagraph("body", layout.BoxStyle{})}},
	}}); err != context.Canceled {
		t.Fatalf("canceled error = %v", err)
	}

	doc := &layout.LayoutDocument{Body: []layout.Block{layout.MetadataGridBlock{Columns: 1, Box: typedContainerTestBox(), Fields: []layout.MetadataField{{Label: "A", Value: "1"}}}}}
	const workers = 8
	hashes := make(chan string, workers)
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			local := paginationTestDocument(t, 100)
			planned, planErr := local.PlanLayoutDocument(doc)
			if planErr != nil {
				errs <- planErr
				return
			}
			hashes <- planned.Hash()
		}()
	}
	group.Wait()
	close(hashes)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var want string
	for hash := range hashes {
		if want == "" {
			want = hash
		} else if hash != want {
			t.Fatalf("concurrent hash = %q, want %q", hash, want)
		}
	}
}

func TestTypedDecoratedContainerComposesMixedTableAndPageShell(t *testing.T) {
	planner := paginationTestDocument(t, 95)
	cell := func(text string) layout.TableCell {
		return layout.TableCell{Blocks: []layout.Block{paginationParagraph(text, layout.BoxStyle{})}}
	}
	table := layout.TableBlock{Columns: []layout.TableColumn{{}}, Style: layout.TableStyle{RepeatHeader: true},
		Header: []layout.TableRow{{Cells: []layout.TableCell{cell("Header")}}}}
	for _, text := range []string{"one", "two", "three", "four", "five", "six"} {
		table.Body = append(table.Body, layout.TableRow{Cells: []layout.TableCell{cell(text)}})
	}
	doc := &layout.LayoutDocument{PageTemplate: layout.PageTemplate{
		Header: &layout.HeaderBlock{Blocks: []layout.Block{paginationParagraph("shell", layout.BoxStyle{})}},
	}, Body: []layout.Block{layout.SectionBlock{Title: "Decorated table", Box: typedContainerTestBox(), Blocks: []layout.Block{
		paginationParagraph("before", layout.BoxStyle{}), table, paginationParagraph("after", layout.BoxStyle{}),
	}}}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() < 2 {
		t.Fatalf("mixed decorated table = pages %d error %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Breaks) == 0 {
		t.Fatal("mixed decorated table has no causal break ledger")
	}
	for index, decision := range projection.Breaks {
		if decision.Reason == "" || decision.ToPage <= decision.FromPage {
			t.Fatalf("break[%d] = %+v", index, decision)
		}
	}
	pagesWithBackground := make(map[uint32]bool)
	for _, fill := range projection.Fills {
		if fill.Color.R == 224 && fill.Color.G == 236 && fill.Color.B == 255 {
			pagesWithBackground[projection.Fragments[fill.Fragment-1].Page] = true
		}
	}
	if len(pagesWithBackground) != plan.PageCount() {
		t.Fatalf("decorated continuation pages = %v, want %d pages", pagesWithBackground, plan.PageCount())
	}
}

func TestTypedDecoratedListsAndQRVariantsPreserveLinksAndReadingOrder(t *testing.T) {
	for _, align := range []string{"left", "center", "right"} {
		t.Run(align, func(t *testing.T) {
			planner := paginationTestDocument(t, 180)
			doc := &layout.LayoutDocument{Body: []layout.Block{
				layout.ListBlock{Ordered: true, Box: typedContainerTestBox(), Items: []layout.ListItem{
					{Blocks: []layout.Block{paginationParagraph("first item", layout.BoxStyle{})}},
					{Blocks: []layout.Block{paginationParagraph("second item", layout.BoxStyle{})}},
				}},
				layout.QRVerificationBlock{Box: typedContainerTestBox(), QR: layout.QRBlock{
					URL: "https://example.com/verify", Label: "Verify", Size: 20, Align: align,
				}},
			}}
			plan, err := planner.PlanLayoutDocument(doc)
			if err != nil {
				t.Fatal(err)
			}
			projection := plan.plan.Projection()
			if len(projection.Images) != 1 || len(projection.Links) < 1 || len(projection.ReadingOrder) < 4 {
				t.Fatalf("images/links/reading = %d/%d/%d", len(projection.Images), len(projection.Links), len(projection.ReadingOrder))
			}
			var list, items, figure bool
			for _, node := range projection.SemanticNodes {
				list = list || node.Role == layoutengine.SemanticRoleList
				items = items || node.Role == layoutengine.SemanticRoleListItem
				figure = figure || node.Role == layoutengine.SemanticRoleFigure
			}
			if !list || !items || !figure {
				t.Fatalf("list/item/figure semantics = %t/%t/%t", list, items, figure)
			}
			var output bytes.Buffer
			target := paginationTestDocument(t, 180)
			if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
				t.Fatal(err)
			}
			if err := target.Output(&output); err != nil || output.Len() == 0 {
				t.Fatalf("PDF = %d bytes, %v", output.Len(), err)
			}
		})
	}
}

func TestTypedNestedListInsideDecoratedListRetainsHierarchyAndMarkers(t *testing.T) {
	planner := paginationTestDocument(t, 130)
	nested := layout.ListBlock{Ordered: true, Items: []layout.ListItem{
		{Blocks: []layout.Block{paginationParagraph("nested one", layout.BoxStyle{})}},
		{Blocks: []layout.Block{paginationParagraph("nested two", layout.BoxStyle{})}},
	}}
	outer := layout.ListBlock{Box: typedContainerTestBox(), Items: []layout.ListItem{{Blocks: []layout.Block{
		paginationParagraph("parent", layout.BoxStyle{}), nested,
	}}}}
	doc := &layout.LayoutDocument{Body: []layout.Block{outer}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	var lists, items int
	var text bytes.Buffer
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleList {
			lists++
		}
		if node.Role == layoutengine.SemanticRoleListItem {
			items++
		}
	}
	for _, run := range projection.GlyphRuns {
		text.WriteString(run.Codes)
		text.WriteByte(' ')
	}
	if lists != 2 || items != 3 || !bytes.Contains(text.Bytes(), []byte("1. nested one")) || !bytes.Contains(text.Bytes(), []byte("2. nested two")) {
		t.Fatalf("nested list semantics/text = lists %d items %d text %q", lists, items, text.String())
	}
}
