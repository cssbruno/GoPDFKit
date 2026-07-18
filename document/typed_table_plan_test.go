// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestLayoutDocumentPlanTableSpansHeadersPaginationCaptureAndPDF(t *testing.T) {
	planner := typedTableTestPlanner()
	table := typedTableTestBlock(true)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Language: "en-US", Body: []layout.Block{table}})
	if err != nil || plan.PageCount() != 2 || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("PlanLayoutDocument(table) = pages %d hash %q planner pages %d, %v", plan.PageCount(), plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Breaks) != 1 || projection.Breaks[0].Reason != layoutengine.BreakPaginationConstraint {
		t.Fatalf("table breaks = %#v", projection.Breaks)
	}
	var headerRuns int
	for _, run := range projection.GlyphRuns {
		if run.Codes == "H1" {
			headerRuns++
		}
	}
	if headerRuns != 2 {
		t.Fatalf("repeated header glyph runs = %d, want 2", headerRuns)
	}
	var rowspan, colspan layoutengine.Fragment
	for _, fragment := range projection.Fragments {
		switch fragment.Key {
		case "@typed-table-r2-c1":
			rowspan = fragment
		case "@typed-table-r4-c1":
			colspan = fragment
		}
	}
	if !rowspan.ID.Valid() || rowspan.BorderBox.Height.Points() != 20 ||
		!colspan.ID.Valid() || colspan.BorderBox.Width.Points() != 180 {
		t.Fatalf("rowspan/colspan geometry = %#v / %#v", rowspan.BorderBox, colspan.BorderBox)
	}
	var tableNodes, rowNodes, headerCells int
	for _, node := range projection.SemanticNodes {
		switch node.Role {
		case layoutengine.SemanticRoleTable:
			tableNodes++
		case layoutengine.SemanticRoleRow:
			rowNodes++
		case layoutengine.SemanticRoleCell:
			if node.Attributes.TableHeader {
				headerCells++
			}
		}
	}
	semanticRoles := make(map[layoutengine.SemanticNodeID]layoutengine.SemanticRole, len(projection.SemanticNodes))
	readFragments := make(map[layoutengine.FragmentID]bool, len(projection.ReadingOrder))
	for _, node := range projection.SemanticNodes {
		semanticRoles[node.ID] = node.Role
	}
	for _, occurrence := range projection.ReadingOrder {
		if semanticRoles[occurrence.Semantic] == layoutengine.SemanticRoleArtifact {
			t.Fatalf("artifact appears in table reading order: %+v", occurrence)
		}
		readFragments[occurrence.Fragment] = true
	}
	if tableNodes != 1 || rowNodes != 9 || headerCells != 2 || len(projection.SemanticFragments) != len(projection.Fragments) {
		t.Fatalf("table semantics = tables %d rows %d headers %d reading %d fragments %d", tableNodes, rowNodes, headerCells, len(projection.ReadingOrder), len(projection.Fragments))
	}
	for _, association := range projection.SemanticFragments {
		if (semanticRoles[association.Semantic] == layoutengine.SemanticRoleArtifact) == readFragments[association.Fragment] {
			t.Fatalf("table fragment reading ownership mismatch: association=%+v role=%s read=%t", association, semanticRoles[association.Semantic], readFragments[association.Fragment])
		}
	}

	capture, err := plan.Capture(PaperPlanCaptureRequest{
		Mode: "core_text_svg", IncludeContactSheet: true, ContactSheetColumns: 2,
		MaxPages: 2, MaxCrops: 8, MaxArtifactBytes: 1 << 20, MaxTotalBytes: 4 << 20, MaxManifestBytes: 1 << 20,
	})
	if err != nil || capture.PlanHash != plan.Hash() || len(capture.Artifacts) != 1 {
		t.Fatalf("table capture = artifacts %d hash %q, %v", len(capture.Artifacts), capture.PlanHash, err)
	}
	embedded, err := firstEmbeddedSVG(capture.Artifacts[0].SVG)
	if err != nil || !bytes.Contains(embedded, []byte(">H</text>")) || !bytes.Contains(embedded, []byte(">E</text>")) {
		t.Fatalf("table capture lacks planned header/body text: %v\n%s", err, embedded)
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != 2 || target.PageCount() != 2 {
		t.Fatalf("WriteLayoutDocumentPlan(table) = %d target pages %d, %v", pages, target.PageCount(), err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 {
		t.Fatalf("table PDF = %d bytes, %v", pdf.Len(), err)
	}
}

func TestTypedTableMaterializesSparseTrailingCells(t *testing.T) {
	planner := typedTableTestPlanner()
	paragraph := layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "value"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12}}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{{Width: 60}, {Width: 60}, {Width: 60}},
		Body: []layout.TableRow{
			{Cells: []layout.TableCell{{RowSpan: 2, Blocks: []layout.Block{paragraph}}, {Blocks: []layout.Block{paragraph}}}},
			{Cells: []layout.TableCell{{Blocks: []layout.Block{paragraph}}}},
		},
	}
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
	if err != nil || plan.PageCount() != 1 {
		t.Fatalf("sparse table plan = pages %d, %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var cells int
	for _, node := range projection.SemanticNodes {
		if node.Role == layoutengine.SemanticRoleCell {
			cells++
		}
	}
	if cells != 5 {
		t.Fatalf("sparse table semantic cells = %d, want 5 including two deterministic empty cells", cells)
	}
}

func TestTypedTableComposesVariablePageTemplateCountersCaptureAndPDF(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	headerMedia := func(label string) []layout.Block {
		return []layout.Block{
			layout.ImageBlock{Data: pixel, Format: "png", Alt: "Table shell mark", Width: 12, Height: 8, Fit: layout.ImageFitContain},
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Verify table shell", Link: "https://example.test/table-shell"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}},
			pageShellParagraph(label),
		}
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 120}), WithNoCompression())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	template := layout.PageTemplate{
		Header:          &layout.HeaderBlock{Blocks: headerMedia("Table header")},
		FirstPageHeader: &layout.HeaderBlock{Blocks: headerMedia("First A\nFirst B")},
		Footer:          &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Table footer")}},
		EvenPageFooter:  &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Even A\nEven B")}},
		PageNumbers:     layout.PageNumberOptions{Enabled: true, Format: "Table page %d / {total}", TotalPageAlias: "{total}"},
	}
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Language: "en-US", PageTemplate: template, Body: []layout.Block{typedTableTestBlock(true)}})
	if err != nil || plan.PageCount() < 2 || planner.PageCount() != 0 {
		t.Fatalf("table shell plan = pages %d source %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != plan.PageCount() || len(projection.Links) != plan.PageCount() {
		t.Fatalf("table shell resources/images/links = %d/%d/%d, want 1/%d/%d", len(projection.ImageResources), len(projection.Images), len(projection.Links), plan.PageCount(), plan.PageCount())
	}
	for _, image := range projection.Images {
		if fragment := projection.Fragments[image.Fragment-1]; fragment.Region != layoutengine.RegionHeader {
			t.Fatalf("table shell image region = %s", fragment.Region)
		}
	}
	for _, link := range projection.Links {
		if fragment := projection.Fragments[link.Fragment-1]; fragment.Region != layoutengine.RegionHeader || link.URI != "https://example.test/table-shell" {
			t.Fatalf("table shell link = %#v region %s", link, fragment.Region)
		}
	}
	headerRuns := 0
	for _, run := range projection.GlyphRuns {
		if run.Codes == "H1" {
			headerRuns++
		}
	}
	if headerRuns != plan.PageCount() {
		t.Fatalf("repeated table header runs = %d, want %d", headerRuns, plan.PageCount())
	}
	bodyY := make(map[uint32]layoutengine.Fixed)
	counters := make(map[uint32]string)
	for _, command := range projection.Commands {
		fragment := projection.Fragments[command.Fragment-1]
		if fragment.Region == layoutengine.RegionBody && bodyY[fragment.Page] == 0 {
			bodyY[fragment.Page] = fragment.BorderBox.Y
		}
		if command.Kind == layoutengine.CommandGlyphRun && fragment.Region == layoutengine.RegionFooter {
			counters[fragment.Page] += projection.GlyphRuns[command.Payload].Codes
		}
	}
	if bodyY[1] <= bodyY[2] {
		t.Fatalf("table body origins = %+v, want taller first header reservation", bodyY)
	}
	for page := uint32(1); page <= uint32(plan.PageCount()); page++ {
		want := fmt.Sprintf("Table page %d / %d", page, plan.PageCount())
		if !strings.Contains(counters[page], want) {
			t.Fatalf("page %d footer = %q, want %q", page, counters[page], want)
		}
	}
	if len(projection.Breaks) != plan.PageCount()-1 {
		t.Fatalf("table shell breaks = %+v, want %d", projection.Breaks, plan.PageCount()-1)
	}
	for _, decision := range projection.Breaks {
		if decision.Reason != layoutengine.BreakPaginationConstraint || !decision.Preceding.Valid() || !decision.Triggering.Valid() {
			t.Fatalf("table shell break = %+v", decision)
		}
	}
	capture, err := plan.CaptureDisplayPage(2)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`data-format="display-plan-preview"`)) || !bytes.Contains(capture.SVG(), []byte("data:image/png;base64,")) {
		t.Fatalf("capture = %d, %v", len(capture.SVG()), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != plan.PageCount() {
		t.Fatalf("table shell paint = %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 {
		t.Fatalf("PDF = %d, %v", pdf.Len(), err)
	}
	if got := bytes.Count(pdf.Bytes(), []byte("/URI (https://example.test/table-shell)")); got != plan.PageCount() {
		t.Fatalf("table shell PDF links = %d, want %d", got, plan.PageCount())
	}
}

func TestTypedMixedParagraphTableParagraphFlowsThroughVariablePageShells(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 120}), WithNoCompression())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	template := layout.PageTemplate{
		Header: &layout.HeaderBlock{Blocks: []layout.Block{
			layout.ImageBlock{Data: pixel, Format: "png", Alt: "Mixed mark", Width: 8, Height: 6, Fit: layout.ImageFitContain},
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Mixed shell", Link: "https://example.test/mixed"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 9}},
		}},
		FirstPageHeader: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("First mixed A\nFirst mixed B")}},
		Footer:          &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Mixed footer")}},
		EvenPageFooter:  &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Even mixed A\nEven mixed B")}},
		PageNumbers:     layout.PageNumberOptions{Enabled: true, Format: "Mixed %d/{total}", TotalPageAlias: "{total}"},
	}
	prefix := layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "PREFIX", Link: "https://example.test/prefix"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}}
	suffix := pageShellParagraph("SUFFIX")
	doc := &layout.LayoutDocument{Language: "en-US", PageTemplate: template, Body: []layout.Block{prefix, typedTableTestBlock(true), suffix}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() < 2 || planner.PageCount() != 0 {
		t.Fatalf("mixed typed plan = pages %d source %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var text strings.Builder
	headerRuns := 0
	for _, command := range projection.Commands {
		if command.Kind != layoutengine.CommandGlyphRun {
			continue
		}
		codes := projection.GlyphRuns[command.Payload].Codes
		text.WriteString(codes)
		text.WriteByte('|')
		if codes == "H1" {
			headerRuns++
		}
	}
	allText := text.String()
	if prefixAt, headerAt, suffixAt := strings.Index(allText, "PREFIX"), strings.Index(allText, "H1"), strings.LastIndex(allText, "SUFFIX"); prefixAt < 0 || headerAt <= prefixAt || suffixAt <= headerAt {
		t.Fatalf("mixed paint order = %q", allText)
	}
	if headerRuns < 2 {
		t.Fatalf("mixed repeated table headers = %d, want at least 2", headerRuns)
	}
	if len(projection.ImageResources) != 1 || len(projection.Images) != plan.PageCount()-1 {
		// FirstPageHeader overrides the default image-bearing header.
		t.Fatalf("mixed shell resources/images = %d/%d, want 1/%d", len(projection.ImageResources), len(projection.Images), plan.PageCount()-1)
	}
	if len(projection.Breaks) == 0 {
		t.Fatal("mixed typed flow has no causal pagination decisions")
	}
	for _, decision := range projection.Breaks {
		if !decision.Preceding.Valid() || !decision.Triggering.Valid() || decision.FromPage >= decision.ToPage {
			t.Fatalf("mixed break = %#v", decision)
		}
	}
	if got := len(projection.Links); got != plan.PageCount() { // one prefix link plus default headers on pages 2..N
		t.Fatalf("mixed links = %d, want %d", got, plan.PageCount())
	}
	capture, err := plan.CaptureDisplayPage(2)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("data:image/png;base64,")) {
		t.Fatalf("mixed capture = %v, %s", err, capture.SVG())
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != plan.PageCount() {
		t.Fatalf("mixed paint = pages %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 {
		t.Fatalf("mixed PDF = %d, %v", pdf.Len(), err)
	}
}

func TestTypedMixedTableFlowCancellationAndGlobalPageLimitAreAtomic(t *testing.T) {
	body := []layout.Block{pageShellParagraph("before"), typedTableTestBlock(true), pageShellParagraph("after")}
	plain := typedTableTestPlanner()
	plainPlan, err := plain.PlanLayoutDocument(&layout.LayoutDocument{Body: body})
	if err != nil || plainPlan.PageCount() < 2 || plain.PageCount() != 0 {
		t.Fatalf("plain mixed table = pages %d source %d, %v", plainPlan.PageCount(), plain.PageCount(), err)
	}
	template := layout.PageTemplate{
		Header:      &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("header")}},
		Footer:      &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("footer")}},
		PageNumbers: layout.PageNumberOptions{Enabled: true, Format: "%d/{total}", TotalPageAlias: "{total}"},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := typedTableTestPlanner()
	plan, err := planner.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{PageTemplate: template, Body: body})
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled mixed table = hash %q pages %d, %v", plan.Hash(), planner.PageCount(), err)
	}
	limited := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 80}), WithLimits(Limits{MaxPages: 1}))
	limited.SetMargins(10, 10, 10)
	limited.SetAutoPageBreak(true, 10)
	plan, err = limited.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: template, Body: body})
	if !errors.Is(err, layoutengine.ErrTablePageLimit) || plan.Hash() != "" || limited.PageCount() != 0 {
		t.Fatalf("limited mixed table = hash %q pages %d, %v", plan.Hash(), limited.PageCount(), err)
	}
	work, err := WithPlanningWorkLimit(context.Background(), 4)
	if err != nil {
		t.Fatal(err)
	}
	bounded := typedTableTestPlanner()
	plan, err = bounded.PlanLayoutDocumentContext(work, &layout.LayoutDocument{PageTemplate: template, Body: body})
	if !errors.Is(err, layoutengine.ErrPlanningBudgetExhausted) || plan.Hash() != "" || bounded.PageCount() != 0 {
		t.Fatalf("work-limited mixed table = hash %q pages %d, %v", plan.Hash(), bounded.PageCount(), err)
	}
}

func TestTypedNestedContainersAndRowColumnSiblingComposeWithTable(t *testing.T) {
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 140}), WithNoCompression())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	rowColumn := layout.RowColumnBlock{Direction: layout.RowDirection, Gap: 20, Items: []layout.RowColumnItem{
		{Block: pageShellParagraph("ROW LEFT"), Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 80}},
		{Block: pageShellParagraph("ROW RIGHT"), Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 80}},
	}}
	nested := layout.SectionBlock{Title: "SECTION", KeepTitleWithBody: true, Blocks: []layout.Block{
		layout.ClauseBlock{Number: "1", Title: "CLAUSE", KeepTogether: true, BreakAfter: true, Blocks: []layout.Block{
			layout.NoteBoxBlock{Title: "NOTE", Box: layout.BoxStyle{KeepTogether: true}, Body: []layout.Block{
				pageShellParagraph("NESTED BEFORE"), typedTableTestBlock(true), pageShellParagraph("NESTED AFTER"),
			}},
		}},
	}}
	template := layout.PageTemplate{
		Header:          &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("Nested header")}},
		FirstPageHeader: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("First nested A\nFirst nested B")}},
		Footer:          &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Nested footer")}},
		EvenPageFooter:  &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Even nested A\nEven nested B")}},
		PageNumbers:     layout.PageNumberOptions{Enabled: true, Format: "Nested %d/{total}", TotalPageAlias: "{total}"},
	}
	source := &layout.LayoutDocument{Language: "en-US", PageTemplate: template, Body: []layout.Block{nested, rowColumn}}
	plan, err := planner.PlanLayoutDocument(source)
	if err != nil || plan.PageCount() < 2 || planner.PageCount() != 0 {
		t.Fatalf("nested mixed plan = pages %d source %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	var text strings.Builder
	for _, command := range projection.Commands {
		if command.Kind == layoutengine.CommandGlyphRun {
			text.WriteString(projection.GlyphRuns[command.Payload].Codes)
			text.WriteByte('|')
		}
	}
	painted := text.String()
	ordered := []string{"SECTION", "1 CLAUSE", "NOTE", "NESTED BEFORE", "H1", "NESTED AFTER", "ROW LEFT", "ROW RIGHT"}
	last := -1
	for _, token := range ordered {
		at := strings.Index(painted, token)
		if at <= last {
			t.Fatalf("nested source order token %q at %d after %d: %q", token, at, last, painted)
		}
		last = at
	}
	if len(projection.SemanticNodes) < len(ordered) || len(projection.SemanticFragments) == 0 || len(projection.ReadingOrder) == 0 {
		t.Fatalf("nested semantics = nodes %d fragments %d reading %d", len(projection.SemanticNodes), len(projection.SemanticFragments), len(projection.ReadingOrder))
	}
	explicit := false
	for _, decision := range projection.Breaks {
		if !decision.Preceding.Valid() || !decision.Triggering.Valid() || decision.FromPage >= decision.ToPage {
			t.Fatalf("nested break = %#v", decision)
		}
		explicit = explicit || decision.Reason == layoutengine.BreakExplicitPageBreak
	}
	if !explicit {
		t.Fatalf("nested breaks = %#v, want explicit clause break", projection.Breaks)
	}
	capture, err := plan.CaptureDisplayPage(uint32(plan.PageCount()))
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`data-format="display-plan-preview"`)) {
		t.Fatalf("nested capture = %v, %s", err, capture.SVG())
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != plan.PageCount() {
		t.Fatalf("nested paint = pages %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 {
		t.Fatalf("nested PDF = %d, %v", pdf.Len(), err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	atomic := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 140}))
	atomic.SetMargins(10, 10, 10)
	if zero, err := atomic.PlanLayoutDocumentContext(canceled, source); !errors.Is(err, context.Canceled) || zero.Hash() != "" || atomic.PageCount() != 0 {
		t.Fatalf("nested cancellation = hash %q pages %d, %v", zero.Hash(), atomic.PageCount(), err)
	}
	insideTrack := rowColumn
	insideTrack.Items[0].Block = typedTableTestBlock(true)
	unsupported := typedTableTestPlanner()
	zero, err := unsupported.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{insideTrack, pageShellParagraph("tail")}})
	if !errors.Is(err, layoutengine.ErrTableTrackOverflow) || zero.Hash() != "" || unsupported.PageCount() != 0 {
		t.Fatalf("nested row table = hash %q pages %d, %v", zero.Hash(), unsupported.PageCount(), err)
	}
}

func TestTypedTablePageTemplateLimitsCancellationAndImpossibleRegionsAreAtomic(t *testing.T) {
	template := layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("Header")}},
		Footer: &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Footer")}}, PageNumbers: layout.PageNumberOptions{Enabled: true}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := typedTableTestPlanner()
	if _, err := planner.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{PageTemplate: template, Body: []layout.Block{typedTableTestBlock(true)}}); !errors.Is(err, context.Canceled) || planner.PageCount() != 0 {
		t.Fatalf("canceled table shell = %v pages %d", err, planner.PageCount())
	}
	limited := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 100}), WithLimits(Limits{MaxPages: 1}))
	limited.SetMargins(10, 10, 10)
	limited.SetAutoPageBreak(true, 10)
	if _, err := limited.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: template, Body: []layout.Block{typedTableTestBlock(true)}}); !errors.Is(err, layoutengine.ErrTablePageLimit) || limited.PageCount() != 0 {
		t.Fatalf("limited table shell = %v pages %d", err, limited.PageCount())
	}
	impossible := typedTableTestPlanner()
	tall := layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("1\n2\n3\n4\n5")}}, Footer: &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("1\n2")}}}
	plan, err := impossible.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: tall, Body: []layout.Block{typedTableTestBlock(true)}})
	if err == nil || !strings.Contains(err.Error(), "leave no body region") || plan.Hash() != "" || impossible.PageCount() != 0 {
		t.Fatalf("impossible table shell = hash %q pages %d err %v", plan.Hash(), impossible.PageCount(), err)
	}
}

func TestLayoutDocumentPlanTableRepeatHeaderIsExplicit(t *testing.T) {
	planner := typedTableTestPlanner()
	table := typedTableTestBlock(false)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
	if err != nil || plan.PageCount() != 2 {
		t.Fatalf("nonrepeating table = pages %d, %v", plan.PageCount(), err)
	}
	count := 0
	for _, run := range plan.plan.Projection().GlyphRuns {
		if run.Codes == "H1" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("nonrepeating header occurrences = %d, want 1", count)
	}
}

func TestLayoutDocumentPlanTableLimitsCancellationAndFailuresAreAtomic(t *testing.T) {
	table := typedTableTestBlock(true)
	limited := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 80}), WithLimits(Limits{MaxPages: 1}))
	limited.SetMargins(10, 10, 10)
	limited.SetAutoPageBreak(true, 10)
	plan, err := limited.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
	if err == nil || plan.Hash() != "" || limited.PageCount() != 0 || !errors.Is(err, layoutengine.ErrTablePageLimit) {
		t.Fatalf("table page limit = plan %#v pages %d, %v", plan, limited.PageCount(), err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := typedTableTestPlanner()
	plan, err = planner.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{Body: []layout.Block{table}})
	if !errors.Is(err, context.Canceled) || plan.Hash() != "" || planner.PageCount() != 0 {
		t.Fatalf("canceled table = plan %#v pages %d, %v", plan, planner.PageCount(), err)
	}

	target := MustNew(WithUnit(UnitPoint))
	pages, paintErr := target.WriteLayoutDocumentPlan(plan)
	if paintErr == nil || pages != 0 || target.PageCount() != 0 {
		t.Fatalf("zero table plan paint = %d pages %d, %v", pages, target.PageCount(), paintErr)
	}
}

func TestTypedTableIntrinsicMinMaxMixedTracksSpansPaginationCaptureRasterAndPDF(t *testing.T) {
	style := layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12}
	paragraph := func(text string) []layout.Block {
		return []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: style}}
	}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{
			{Width: 40, MinWidth: 30, MaxWidth: 40},
			{MinWidth: 20, MaxWidth: 60},
			{MinWidth: 10, MaxWidth: 100},
		},
		Header: []layout.TableRow{{Cells: []layout.TableCell{
			{Blocks: paragraph("ID"), Header: true},
			{Blocks: paragraph("alpha beta gamma delta epsilon"), Header: true},
			{Blocks: paragraph("Z"), Header: true},
		}}},
		Style: layout.TableStyle{RepeatHeader: true},
	}
	for index := 0; index < 8; index++ {
		if index == 3 {
			table.Body = append(table.Body, layout.TableRow{Cells: []layout.TableCell{
				{Blocks: paragraph("S")},
				{Blocks: paragraph("joined narrative evidence"), ColSpan: 2},
			}})
			continue
		}
		table.Body = append(table.Body, layout.TableRow{Cells: []layout.TableCell{
			{Blocks: paragraph(fmt.Sprintf("%d", index))},
			{Blocks: paragraph("short words")},
			{Blocks: paragraph("value")},
		}})
	}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 220, Ht: 140}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	doc := &layout.LayoutDocument{Language: "en-US", PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Left: 10, Top: 10, Right: 10, Bottom: 10}}, Body: []layout.Block{table}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() < 2 || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("intrinsic table plan = pages %d hash %q source pages %d, %v", plan.PageCount(), plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	widths := make(map[string]float64)
	var colspan layoutengine.Fragment
	for _, fragment := range projection.Fragments {
		if fragment.Page == 1 {
			switch fragment.Key {
			case "@typed-table-r1-c1", "@typed-table-r1-c2", "@typed-table-r1-c3":
				widths[string(fragment.Key)] = fragment.BorderBox.Width.Points()
			}
		}
		if fragment.Key == "@typed-table-r5-c2" {
			colspan = fragment
		}
	}
	if widths["@typed-table-r1-c1"] != 40 || widths["@typed-table-r1-c2"] != 60 || widths["@typed-table-r1-c3"] != 100 {
		t.Fatalf("resolved intrinsic tracks = %+v, want 40/60/100", widths)
	}
	if !colspan.ID.Valid() || colspan.BorderBox.Width.Points() != 160 {
		t.Fatalf("intrinsic colspan = %+v, want 160pt", colspan)
	}
	if len(projection.Breaks) == 0 || projection.Breaks[0].Reason != layoutengine.BreakPaginationConstraint {
		t.Fatalf("intrinsic pagination breaks = %+v", projection.Breaks)
	}
	headerOccurrences := 0
	for _, fragment := range projection.Fragments {
		if fragment.Key == "@typed-table-r1-c2" {
			headerOccurrences++
		}
	}
	if headerOccurrences != plan.PageCount() {
		t.Fatalf("repeated intrinsic header occurrences = %d, want %d", headerOccurrences, plan.PageCount())
	}
	display, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(display.SVG(), []byte(`data-format="display-plan-preview"`)) || !bytes.Contains(display.SVG(), []byte(">a</text>")) {
		t.Fatalf("intrinsic table display capture = %v\n%s", err, display.SVG())
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "typed-table-intrinsic", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != plan.PageCount() || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("intrinsic table raster = status %q evidence %+v, %v", status, raster, err)
	}
	again, err := planner.PlanLayoutDocument(doc)
	if err != nil || again.Hash() != plan.Hash() {
		t.Fatalf("intrinsic table determinism = %q / %q, %v", plan.Hash(), again.Hash(), err)
	}
	table.Columns[1].MaxWidth = 21
	if plan.plan.Projection().Fragments[1].BorderBox.Width.Points() != 60 {
		t.Fatal("intrinsic table plan aliases authored column constraints")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != plan.PageCount() {
		t.Fatalf("intrinsic table PDF replay = pages %d, %v", pages, err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || pdf.Len() == 0 || !bytes.Contains(pdf.Bytes(), []byte("%PDF")) {
		t.Fatalf("intrinsic table PDF = %d bytes, %v", pdf.Len(), err)
	}
}

func TestTypedTableIntrinsicBoundsAndSpanningMinimumFailuresAreAtomic(t *testing.T) {
	cell := func(text string, span int) layout.TableCell {
		return layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12}}}, ColSpan: span}
	}
	tests := []struct {
		name    string
		columns []layout.TableColumn
		text    string
		want    error
	}{
		{"maximum underfill", []layout.TableColumn{{MaxWidth: 20}, {MaxWidth: 20}}, "x", layoutengine.ErrTableTrackUnresolved},
		{"spanning minimum overflow", []layout.TableColumn{{MaxWidth: 20}, {MaxWidth: 20}}, "unbreakable-spanning-minimum", layoutengine.ErrTableTrackOverflow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}))
			planner.SetMargins(10, 10, 10)
			table := layout.TableBlock{Columns: test.columns, Body: []layout.TableRow{{Cells: []layout.TableCell{cell(test.text, 2)}}}}
			plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Left: 10, Top: 10, Right: 10, Bottom: 10}}, Body: []layout.Block{table}})
			if !errors.Is(err, test.want) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("bounded intrinsic failure = hash %q pages %d, %v; want %v", plan.Hash(), planner.PageCount(), err, test.want)
			}
		})
	}
}

func TestLayoutDocumentPlanTableRejectsUnrepresentedContractsWithPaths(t *testing.T) {
	base := typedTableTestBlock(false)
	tests := []struct {
		name string
		edit func(*layout.TableBlock)
		want string
	}{
		{"inverted column bounds", func(table *layout.TableBlock) { table.Columns[0].MinWidth, table.Columns[0].MaxWidth = 30, 20 }, "body[0].columns[0]"},
		{"fixed width outside maximum", func(table *layout.TableBlock) { table.Columns[0].MaxWidth = table.Columns[0].Width - 1 }, "body[0].columns[0]"},
		{"table padding", func(table *layout.TableBlock) { table.Box.Padding.Top = 1 }, "body[0]"},
		{"margin", func(table *layout.TableBlock) { table.Body[0].Cells[0].Box.Margin.Top = 1 }, "body[0].rows[1].cells[0]"},
		{"dashed border", func(table *layout.TableBlock) {
			table.Body[0].Cells[0].Box.Border.Top = layout.BorderSide{Width: 1, Style: "dashed"}
		}, "only solid borders"},
		{"justify", func(table *layout.TableBlock) { table.Body[0].Cells[0].Align = "justify" }, "body[0].rows[1].cells[0].align"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			table := base
			table.Columns = append([]layout.TableColumn(nil), base.Columns...)
			table.Header = cloneTypedTableRows(base.Header)
			table.Body = cloneTypedTableRows(base.Body)
			table.Footer = cloneTypedTableRows(base.Footer)
			test.edit(&table)
			planner := typedTableTestPlanner()
			_, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
			if !errors.Is(err, ErrLayoutDocumentPlanUnsupported) || !strings.Contains(err.Error(), test.want) || planner.PageCount() != 0 {
				t.Fatalf("unsupported table = pages %d, %v, want %q", planner.PageCount(), err, test.want)
			}
		})
	}
}

func TestTypedTableDecorationsPaddingAlignmentAndCommandOrder(t *testing.T) {
	table := typedTableTestBlock(true)
	table.Box.BackgroundColor = layout.DocumentColor{R: 240, G: 240, B: 240, Set: true}
	cell := &table.Header[0].Cells[0]
	cell.Box.Padding = layout.Spacing{Top: 2, Right: 3, Bottom: 4, Left: 5}
	cell.Box.BackgroundColor = layout.DocumentColor{R: 10, G: 20, B: 30, Set: true}
	side := layout.BorderSide{Width: 1, Style: "solid", Color: layout.DocumentColor{R: 40, G: 50, B: 60, Set: true}}
	cell.Box.Border = layout.BorderStyle{Top: side, Right: side, Bottom: side, Left: side}
	cell.Align = "right"
	cell.VerticalAlign = "bottom"
	plan, err := typedTableTestPlanner().PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
	if err != nil {
		t.Fatal(err)
	}
	p := plan.plan.Projection()
	if len(p.Fills) == 0 || len(p.Strokes) < 4 {
		t.Fatalf("decorations = fills %d strokes %d", len(p.Fills), len(p.Strokes))
	}
	for _, page := range p.Pages {
		seenGlyph := map[layoutengine.FragmentID]bool{}
		for _, command := range p.Commands[page.Commands.Start : page.Commands.Start+page.Commands.Count] {
			if command.Kind == layoutengine.CommandGlyphRun {
				seenGlyph[command.Fragment] = true
			}
			if seenGlyph[command.Fragment] && (command.Kind == layoutengine.CommandFillPath || command.Kind == layoutengine.CommandStrokePath) {
				t.Fatalf("fragment decoration painted after its text on page %d", page.Number)
			}
		}
	}
	var decoratedHeaders int
	var headerFills, headerStrokes int
	for _, fragment := range p.Fragments {
		if fragment.Key == "@typed-table-r1-c1" {
			decoratedHeaders++
		}
	}
	for _, command := range p.Commands {
		if !command.Fragment.Valid() || p.Fragments[command.Fragment-1].Key != "@typed-table-r1-c1" {
			continue
		}
		switch command.Kind {
		case layoutengine.CommandFillPath:
			headerFills++
		case layoutengine.CommandStrokePath:
			headerStrokes++
		}
	}
	if decoratedHeaders != plan.PageCount() {
		t.Fatalf("decorated repeated headers=%d pages=%d", decoratedHeaders, plan.PageCount())
	}
	if headerFills != plan.PageCount() || headerStrokes != 4*plan.PageCount() {
		t.Fatalf("repeated header decorations fills=%d strokes=%d pages=%d", headerFills, headerStrokes, plan.PageCount())
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("<path")) {
		t.Fatalf("display capture=%v", err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil || !bytes.Contains(pdf.Bytes(), []byte(" rg ")) || !bytes.Contains(pdf.Bytes(), []byte(" RG ")) {
		t.Fatalf("decorated PDF bytes=%d err=%v", pdf.Len(), err)
	}
}

func TestTypedTableCollapsedBordersResolveSpansAndRepeatedHeadersOnce(t *testing.T) {
	table := typedTableTestBlock(true)
	table.Style.BorderCollapse = true
	black := layout.DocumentColor{Set: true}
	base := layout.BorderSide{Width: 1, Style: "solid", Color: black}
	decorate := func(rows []layout.TableRow) {
		for r := range rows {
			for c := range rows[r].Cells {
				rows[r].Cells[c].Box.Border = layout.BorderStyle{Top: base, Right: base, Bottom: base, Left: base}
			}
		}
	}
	decorate(table.Header)
	decorate(table.Body)
	decorate(table.Footer)
	table.Header[0].Cells[0].Box.Border.Right = layout.BorderSide{Width: 3, Style: "solid", Color: layout.DocumentColor{R: 255, Set: true}}
	table.Header[0].Cells[1].Box.Border.Left = layout.BorderSide{Width: 2, Style: "solid", Color: layout.DocumentColor{B: 255, Set: true}}
	plan, err := typedTableTestPlanner().PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{table}})
	if err != nil {
		t.Fatal(err)
	}
	p := plan.plan.Projection()
	seen := map[string]bool{}
	wide := 0
	for _, stroke := range p.Strokes {
		fragment := p.Fragments[stroke.Fragment-1]
		path := p.Paths[stroke.Path]
		key := fmt.Sprintf("%d:%v:%v", fragment.Page, path.Segments[0].Point, path.Segments[1].Point)
		if seen[key] {
			t.Fatalf("collapsed edge emitted twice: %s", key)
		}
		seen[key] = true
		if stroke.Width.Points() == 3 {
			wide++
			if stroke.Color.R != 255 {
				t.Fatalf("winning edge color=%#v", stroke.Color)
			}
		}
	}
	if wide != plan.PageCount() {
		t.Fatalf("wide shared header winners=%d pages=%d", wide, plan.PageCount())
	}
	if len(p.Strokes) >= 4*len(p.Fragments) {
		t.Fatalf("collapse retained naive duplicate edges: strokes=%d fragments=%d", len(p.Strokes), len(p.Fragments))
	}
}

func TestTypedTableCollapsedBorderResolutionCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := typedTableCollapsedBorders(ctx, layoutengine.LayoutPlanProjection{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("collapsed cancellation = %v", err)
	}
}

func typedTableTestPlanner() *Document {
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 80}), WithNoCompression())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	return planner
}

func TestTypedTableIntrinsicMeasurementReusesScratchDocumentByResolvedStyle(t *testing.T) {
	planner := typedTableTestPlanner()
	doc := &layout.LayoutDocument{}
	style := layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}
	placement := func(column uint32, text string) typedTablePlacement {
		return typedTablePlacement{row: 0, column: column, rowSpan: 1, columnSpan: 1,
			path: fmt.Sprintf("body[0].rows[0].cells[%d]", column),
			cell: layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: text}}, Style: style,
			}}}}
	}
	cache := make(map[layout.TextStyle]*Document)
	firstMin, firstPreferred, err := planner.measureTypedTableCellIntrinsic(t.Context(), doc, placement(0, "same style one"), cache)
	if err != nil {
		t.Fatal(err)
	}
	secondMin, secondPreferred, err := planner.measureTypedTableCellIntrinsic(t.Context(), doc, placement(1, "same style two"), cache)
	if err != nil {
		t.Fatal(err)
	}
	if len(cache) != 1 || firstMin <= 0 || firstPreferred < firstMin || secondMin <= 0 || secondPreferred < secondMin {
		t.Fatalf("intrinsic cache/widths = %d, %v/%v and %v/%v", len(cache), firstMin, firstPreferred, secondMin, secondPreferred)
	}
}

func typedTableTestBlock(repeat bool) layout.TableBlock {
	cell := func(text string) layout.TableCell {
		return layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: text}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10},
		}}}
	}
	table := layout.TableBlock{
		Columns: []layout.TableColumn{{Width: 90}, {Width: 90}},
		Header: []layout.TableRow{{Cells: []layout.TableCell{
			{Blocks: []layout.Block{layout.HeadingBlock{Level: 3, Segments: []layout.TextSegment{{Text: "H1"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}}}},
			{Blocks: []layout.Block{layout.HeadingBlock{Level: 3, Segments: []layout.TextSegment{{Text: "H2"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}}}},
		}}},
		Style: layout.TableStyle{RepeatHeader: repeat, KeepRows: true},
	}
	a := cell("A")
	a.RowSpan = 2
	table.Body = append(table.Body,
		layout.TableRow{Cells: []layout.TableCell{a, cell("B1")}, KeepTogether: true},
		layout.TableRow{Cells: []layout.TableCell{cell("B2")}},
	)
	c := cell("C span")
	c.ColSpan = 2
	table.Body = append(table.Body, layout.TableRow{Cells: []layout.TableCell{c}})
	for _, prefix := range []string{"D", "E", "F", "G"} {
		table.Body = append(table.Body, layout.TableRow{Cells: []layout.TableCell{cell(prefix + "1"), cell(prefix + "2")}})
	}
	total := cell("Total")
	total.Blocks = append(total.Blocks, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Rows"}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 10}})
	table.Footer = []layout.TableRow{{Cells: []layout.TableCell{total, cell("8")}}}
	return table
}

func cloneTypedTableRows(rows []layout.TableRow) []layout.TableRow {
	result := make([]layout.TableRow, len(rows))
	for index, row := range rows {
		result[index] = row
		result[index].Cells = append([]layout.TableCell(nil), row.Cells...)
		for cell := range result[index].Cells {
			result[index].Cells[cell].Blocks = append([]layout.Block(nil), row.Cells[cell].Blocks...)
		}
	}
	return result
}
