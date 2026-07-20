// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestAttachTypedSegmentLinksMapsWrappedAuthoredRangesToExactPDFAnnotations(t *testing.T) {
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 125, Ht: 150}), WithNoCompression())
	planner.SetMargins(15, 15, 15)
	planner.SetAutoPageBreak(true, 15)
	segments := []layout.TextSegment{
		{Text: "before "},
		{Text: "linked words that wrap across exact planned lines", Link: "https://example.test/exact"},
		{Text: " after"},
	}
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: segments, Style: layout.TextStyle{LineHeight: 12}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Links) < 2 {
		t.Fatalf("wrapped authored link produced %d annotations, want at least 2", len(projection.Links))
	}
	for index, link := range projection.Links {
		if link.URI != "https://example.test/exact" || link.Bounds.Width <= 0 || link.Bounds.Height <= 0 {
			t.Fatalf("links[%d] = %#v", index, link)
		}
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(pdf.Bytes(), []byte("/URI (https://example.test/exact)")); got != len(projection.Links) {
		t.Fatalf("PDF URI annotations = %d, want %d", got, len(projection.Links))
	}
}

func TestAttachTypedSegmentLinksRejectsUnsafeAndUnrepresentedAuthoredTargets(t *testing.T) {
	planner := MustNew(WithUnit(UnitPoint))
	unsafe, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "visible", Link: "javascript:alert(1)"}}},
	}})
	if err == nil || unsafe.Hash() != "" || planner.PageCount() != 0 || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsafe public link = plan %#v pages %d, %v", unsafe, planner.PageCount(), err)
	}

	planner = MustNew(WithUnit(UnitPoint))
	base, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "visible"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := attachTypedSegmentLinks(base.plan, map[layoutengine.NodeID][]layout.TextSegment{
		1: {{Text: "different", Link: "https://example.test"}},
	}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unrepresented link = %v", err)
	}
}

func TestTypedInternalDestinationsLinksHierarchyReadingOrderAndPDFReplay(t *testing.T) {
	style := layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}
	targetSegments := []layout.TextSegment{{Text: "Destination heading", Destination: "details"}}
	doc := &layout.LayoutDocument{Language: "en", Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Jump to details", Link: "#details"}}, Style: style},
		layout.SectionBlock{Title: "Details section", Blocks: []layout.Block{
			layout.HeadingBlock{Level: 2, Segments: targetSegments, Style: style},
			layout.NoteBoxBlock{Title: "Remember", Body: []layout.Block{
				layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{
					layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "First fact"}}, Style: style},
				}}}},
			}},
		}},
	}}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 180, Ht: 220}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(15, 15, 15)
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Destinations) != 1 || len(projection.Links) != 1 || projection.Links[0].Destination != 1 || projection.Links[0].URI != "" {
		t.Fatalf("internal destination/link = %#v / %#v", projection.Destinations, projection.Links)
	}
	destination := projection.Destinations[0]
	if destination.Page != 1 || !destination.Fragment.Valid() || destination.Point.X <= 0 || destination.Point.Y <= 0 {
		t.Fatalf("resolved destination = %#v", destination)
	}

	children := make(map[layoutengine.SemanticNodeID][]layoutengine.SemanticRole)
	for _, node := range projection.SemanticNodes {
		children[node.Parent] = append(children[node.Parent], node.Role)
	}
	containsRole := func(parent layoutengine.SemanticNodeID, role layoutengine.SemanticRole) bool {
		for _, current := range children[parent] {
			if current == role {
				return true
			}
		}
		return false
	}
	var root, outerSection, noteSection, list, item layoutengine.SemanticNodeID
	for _, node := range projection.SemanticNodes {
		switch {
		case node.Role == layoutengine.SemanticRoleDocument:
			root = node.ID
		case node.Role == layoutengine.SemanticRoleSection && node.Parent == root && outerSection == 0:
			outerSection = node.ID
		case node.Role == layoutengine.SemanticRoleSection && node.Parent == outerSection:
			noteSection = node.ID
		case node.Role == layoutengine.SemanticRoleList:
			list = node.ID
		case node.Role == layoutengine.SemanticRoleListItem:
			item = node.ID
		}
	}
	if !root.Valid() || !outerSection.Valid() || !noteSection.Valid() || !list.Valid() || !item.Valid() ||
		!containsRole(root, layoutengine.SemanticRoleParagraph) || !containsRole(outerSection, layoutengine.SemanticRoleHeading) ||
		!containsRole(noteSection, layoutengine.SemanticRoleList) || !containsRole(list, layoutengine.SemanticRoleListItem) ||
		!containsRole(item, layoutengine.SemanticRoleParagraph) {
		t.Fatalf("semantic hierarchy = %#v", projection.SemanticNodes)
	}
	if len(projection.ReadingOrder) != len(projection.Fragments) {
		t.Fatalf("reading occurrences=%d fragments=%d", len(projection.ReadingOrder), len(projection.Fragments))
	}
	for index, occurrence := range projection.ReadingOrder {
		if occurrence.Page != 1 || occurrence.ReadingIndex != uint32(index) || occurrence.Fragment != projection.Fragments[index].ID {
			t.Fatalf("reading[%d] = %#v fragment=%#v", index, occurrence, projection.Fragments[index])
		}
	}

	before := plan.Hash()
	targetSegments[0].Destination = "mutated"
	doc.Body = nil
	if plan.Hash() != before || plan.plan.Projection().Destinations[0] != destination {
		t.Fatal("internal destination plan aliases source model")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if bytes.Count(pdf.Bytes(), []byte("/Subtype /Link")) != 1 || !bytes.Contains(pdf.Bytes(), []byte("/Dest [")) || bytes.Contains(pdf.Bytes(), []byte("/URI")) {
		t.Fatalf("internal PDF link replay is incomplete:\n%s", pdf.Bytes())
	}
}

func TestTypedInternalDestinationFailuresAreAtomicAndDeterministic(t *testing.T) {
	style := layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}
	tests := []struct {
		name     string
		segments []layout.TextSegment
		want     string
	}{
		{"missing", []layout.TextSegment{{Text: "jump", Link: "#absent"}}, "missing destination"},
		{"duplicate", []layout.TextSegment{{Text: "one", Destination: "same"}, {Text: "two", Destination: "same"}}, "duplicate typed destination"},
		{"empty", []layout.TextSegment{{Destination: "empty"}}, "has no text glyphs"},
		{"invalid", []layout.TextSegment{{Text: "target", Destination: "bad name"}}, "unsupported character"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := &layout.LayoutDocument{Body: []layout.Block{layout.ParagraphBlock{Segments: test.segments, Style: style}}}
			firstPlanner := MustNew(WithUnit(UnitPoint))
			first, firstErr := firstPlanner.PlanLayoutDocument(model)
			second, secondErr := MustNew(WithUnit(UnitPoint)).PlanLayoutDocument(model)
			if firstErr == nil || secondErr == nil || first.Hash() != "" || second.Hash() != "" || firstPlanner.PageCount() != 0 ||
				firstErr.Error() != secondErr.Error() || !strings.Contains(firstErr.Error(), test.want) {
				t.Fatalf("atomic deterministic failure = %#v/%#v, %v / %v", first, second, firstErr, secondErr)
			}
		})
	}
}

func TestTypedRowColumnRejectsUnrepresentedDestinationSegmentsAtomically(t *testing.T) {
	model := &layout.LayoutDocument{Body: []layout.Block{layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{
		Block: layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "target", Destination: "inside"}}},
		Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: 1},
	}}}}}
	planner := MustNew(WithUnit(UnitPoint))
	plan, err := planner.PlanLayoutDocument(model)
	if !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || plan.Hash() != "" || planner.PageCount() != 0 || !strings.Contains(err.Error(), "not represented") {
		t.Fatalf("row/column destination rejection = plan %#v pages %d, %v", plan, planner.PageCount(), err)
	}
}

func TestTypedCharacterizationIncludesInternalLinkPDFEvidence(t *testing.T) {
	for _, fixture := range typedCharacterizationFixtures() {
		if fixture.inventory.Name != "internal-links-hierarchy" {
			continue
		}
		if _, err := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: fixture.pageHeight}), WithNoCompression()).PlanLayoutDocument(fixture.doc); err != nil {
			t.Fatalf("plan internal-link characterization fixture: %v", err)
		}
	}
	projection, err := RunTypedCharacterization(t.Context(), DefaultTypedCharacterizationLimits())
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range projection.Fixtures {
		if fixture.Name != "internal-links-hierarchy" {
			continue
		}
		if fixture.Outcome != "planned" || fixture.PDF == nil || fixture.PDF.Links != 1 || fixture.PDF.Destinations == 0 {
			t.Fatalf("internal-link characterization = %#v", fixture)
		}
		return
	}
	t.Fatal("internal-links-hierarchy characterization fixture is missing")
}
