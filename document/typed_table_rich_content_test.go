// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"encoding/base64"
	"sync"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestTypedTableStructuredCellListsSectionsClausesNotesAndImages(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}}
	}
	cell := layout.TableCell{Box: layout.BoxStyle{KeepTogether: true, KeepWithNext: true, Orphans: 2, Widows: 2}, Blocks: []layout.Block{
		layout.ListBlock{Ordered: true, Items: []layout.ListItem{{Blocks: []layout.Block{paragraph("first")}}, {Blocks: []layout.Block{paragraph("second")}}}},
		layout.SectionBlock{Title: "Section title", KeepTitleWithBody: true, Blocks: []layout.Block{paragraph("section body")}},
		layout.ClauseBlock{Number: "1.2", Title: "Clause title", KeepTogether: true, Blocks: []layout.Block{paragraph("clause body")}},
		layout.NoteBoxBlock{Title: "Important", Body: []layout.Block{paragraph("note body")}},
		layout.ImageBlock{Data: pixel, Format: "png", Width: 12, Height: 8, Alt: "Evidence mark", Caption: []layout.TextSegment{{Text: "Image caption"}}},
		layout.ImageBlock{Data: pixel, Format: "png", Width: 8, Height: 6},
	}}
	table := layout.TableBlock{
		CaptionSegments: []layout.TextSegment{{Text: "Rich table", Destination: "rich-table"}, {Text: " return", Link: "#rich-table"}},
		Columns:         []layout.TableColumn{{Width: 180}},
		Body:            []layout.TableRow{{KeepTogether: true, Cells: []layout.TableCell{cell}}},
		Style:           layout.TableStyle{KeepRows: true},
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 260}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	doc := &layout.LayoutDocument{Language: "en-US", PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Left: 10, Top: 10, Right: 10, Bottom: 10}}, Body: []layout.Block{table}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("structured table plan = hash %q source pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 2 || len(projection.Destinations) != 1 || len(projection.Links) != 1 {
		t.Fatalf("resources/images/destinations/links = %d/%d/%d/%d", len(projection.ImageResources), len(projection.Images), len(projection.Destinations), len(projection.Links))
	}
	text := ""
	for _, run := range projection.GlyphRuns {
		text += run.Codes + "\n"
	}
	for _, want := range []string{"1. first", "2. second", "Section title", "section body", "1.2 Clause title", "clause body", "Important", "note body", "Image caption"} {
		if !bytes.Contains([]byte(text), []byte(want)) {
			t.Fatalf("planned text lacks %q:\n%s", want, text)
		}
	}
	var figure, artifact, heading bool
	for _, node := range projection.SemanticNodes {
		switch node.Role {
		case layoutengine.SemanticRoleFigure:
			figure = figure || node.Attributes.AlternateText == "Evidence mark"
		case layoutengine.SemanticRoleArtifact:
			artifact = true
		case layoutengine.SemanticRoleHeading:
			heading = true
		}
	}
	if !figure || !artifact || !heading {
		t.Fatalf("structured semantic evidence figure=%t artifact=%t heading=%t nodes=%+v", figure, artifact, heading, projection.SemanticNodes)
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("data:image/png;base64,")) || !bytes.Contains(capture.SVG(), []byte(`font-family="helvetica_bold"`)) {
		t.Fatalf("structured display capture = %v\n%s", err, capture.SVG())
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != plan.PageCount() {
		t.Fatalf("structured table replay = pages %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 || !bytes.Contains(pdf.Bytes(), []byte("/Subtype /Image")) {
		t.Fatalf("structured table PDF = %d bytes, %v", pdf.Len(), err)
	}

	want := plan.Hash()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate, err := planner.PlanLayoutDocument(doc)
			if err == nil && candidate.Hash() != want {
				err = &typedTableHashMismatch{got: candidate.Hash(), want: want}
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTypedTableRepeatedHeaderRetainsRichImageResourceAndSemantics(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	text := func(value string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: value}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}}
	}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{{Width: 80}}, Style: layout.TableStyle{RepeatHeader: true, KeepRows: true},
		Header: []layout.TableRow{{KeepTogether: true, Cells: []layout.TableCell{{Header: true, Blocks: []layout.Block{
			layout.ImageBlock{Data: pixel, Format: "png", Width: 6, Height: 6, Alt: "Repeated mark"},
			text("Header"),
		}}}}},
	}
	for index := 0; index < 12; index++ {
		table.Body = append(table.Body, layout.TableRow{KeepTogether: true, Cells: []layout.TableCell{{Blocks: []layout.Block{
			layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{text("body")}}}},
		}}}})
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Language: "en-US", Body: []layout.Block{table}})
	if err != nil || plan.PageCount() < 2 || planner.PageCount() != 0 {
		t.Fatalf("repeated rich header = pages %d source pages %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != plan.PageCount() {
		t.Fatalf("repeated image catalog/placements = %d/%d, want 1/%d", len(projection.ImageResources), len(projection.Images), plan.PageCount())
	}
	for _, image := range projection.Images {
		owner := projection.Fragments[image.Fragment-1]
		if !owner.Repeated && owner.Page > 1 {
			t.Fatalf("continued header image owner = %+v", owner)
		}
	}
	figures := 0
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleFigure && node.Attributes.AlternateText == "Repeated mark" {
			figures++
		}
	}
	if figures != 1 {
		t.Fatalf("repeated logical figure nodes = %d, want 1", figures)
	}
	if _, err := plan.CaptureDisplayPage(uint32(plan.PageCount())); err != nil {
		t.Fatalf("capture repeated rich header: %v", err)
	}
}

type typedTableHashMismatch struct{ got, want string }

func (err *typedTableHashMismatch) Error() string {
	return "typed table concurrent plan hash mismatch: got " + err.got + ", want " + err.want
}
