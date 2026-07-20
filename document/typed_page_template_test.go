// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
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

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func pageShellParagraph(text string) layout.ParagraphBlock {
	return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}},
		Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}}
}

func TestTypedPageTemplateComposesContentAddressedImagesAndExternalLinks(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	template := layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{
		layout.ImageBlock{Data: pixel, Format: "png", Alt: "Header mark", Width: 12, Height: 8, Fit: layout.ImageFitContain},
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Verify header", Link: "https://example.test/header"}}, Style: layout.TextStyle{LineHeight: 10}},
	}}}
	planner := paginationTestDocument(t, 100)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: template, Body: []layout.Block{
		pageShellParagraph("Page one"), layout.PageBreakBlock{After: true}, pageShellParagraph("Page two"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 2 || len(projection.Links) != 2 {
		t.Fatalf("shell resources/images/links = %d/%d/%d, want 1/2/2", len(projection.ImageResources), len(projection.Images), len(projection.Links))
	}
	for _, image := range projection.Images {
		fragment := projection.Fragments[image.Fragment-1]
		if fragment.Region != layoutengine.RegionHeader || image.Bounds.Width <= 0 || image.Bounds.Height <= 0 {
			t.Fatalf("shell image/fragment = %#v / %#v", image, fragment)
		}
	}
	for _, link := range projection.Links {
		fragment := projection.Fragments[link.Fragment-1]
		if fragment.Region != layoutengine.RegionHeader || link.URI != "https://example.test/header" || link.Bounds.Width <= 0 {
			t.Fatalf("shell link/fragment = %#v / %#v", link, fragment)
		}
	}
	capture, err := plan.CaptureDisplayPage(2)
	if err != nil || !bytes.Contains(capture.SVG(), []byte("data:image/png;base64,")) {
		t.Fatalf("shell image capture = %v, %s", err, capture.SVG())
	}
	target := paginationTestDocument(t, 100)
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if got := bytes.Count(pdf.Bytes(), []byte("/URI (https://example.test/header)")); got != 2 {
		t.Fatalf("shell PDF links = %d, want 2", got)
	}
	if got := bytes.Count(pdf.Bytes(), []byte("/Subtype /Image")); got == 0 {
		t.Fatal("shell PDF contains no image object")
	}
}

func TestTypedPageTemplateRepeatsStructuredTableLinksCountersAndArtifacts(t *testing.T) {
	cell := layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{
		Text: "Structured shell", Link: "https://example.test/structured-shell",
	}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 9}}}}
	headerTable := layout.TableBlock{Columns: []layout.TableColumn{{}}, Body: []layout.TableRow{{Cells: []layout.TableCell{cell}}}}
	footerCell := layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{
		Text: "Footer evidence", Link: "https://example.test/footer-evidence",
	}}, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 8, LineHeight: 9}}}}
	footerTable := layout.TableBlock{Columns: []layout.TableColumn{{}}, Body: []layout.TableRow{{Cells: []layout.TableCell{footerCell}}}}
	shellBox := layout.BoxStyle{Padding: layout.Spacing{Top: 1, Right: 2, Bottom: 1, Left: 2},
		BackgroundColor: layout.DocumentColor{R: 242, G: 246, B: 250, Set: true}}
	planner := paginationTestDocument(t, 92)
	planner.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Structured repeated regions", Lang: "en-US"})
	doc := &layout.LayoutDocument{Title: "Structured repeated regions", Language: "en-US", PageTemplate: layout.PageTemplate{
		Header:      &layout.HeaderBlock{Blocks: []layout.Block{headerTable}, Box: shellBox},
		Footer:      &layout.FooterBlock{Blocks: []layout.Block{footerTable}, Box: shellBox},
		PageNumbers: layout.PageNumberOptions{Enabled: true, Format: "Page %d / {total}", TotalPageAlias: "{total}"},
	}, Body: []layout.Block{
		pageShellParagraph("first body"), layout.PageBreakBlock{After: true},
		pageShellParagraph("second body"), layout.PageBreakBlock{After: true},
		pageShellParagraph("third body"),
	}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 3 {
		t.Fatalf("structured shell plan = pages %d, %v", plan.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Links) != 6 || len(projection.Breaks) != 2 {
		t.Fatalf("structured shell links/breaks = %d/%d", len(projection.Links), len(projection.Breaks))
	}
	pageText := make(map[uint32]string)
	for _, command := range projection.Commands {
		if command.Kind == layoutengine.CommandGlyphRun {
			pageText[projection.Fragments[command.Fragment-1].Page] += projection.GlyphRuns[command.Payload].Codes
		}
	}
	for page := uint32(1); page <= 3; page++ {
		want := fmt.Sprintf("Page %d / 3", page)
		if !strings.Contains(pageText[page], want) {
			t.Fatalf("page %d text = %q, want corrected counter %q", page, pageText[page], want)
		}
	}
	for _, link := range projection.Links {
		fragment := projection.Fragments[link.Fragment-1]
		if (fragment.Region == layoutengine.RegionHeader && link.URI != "https://example.test/structured-shell") ||
			(fragment.Region == layoutengine.RegionFooter && link.URI != "https://example.test/footer-evidence") {
			t.Fatalf("shell link = %+v fragment %+v", link, fragment)
		}
	}
	for index, decision := range projection.Breaks {
		if decision.Reason != layoutengine.BreakExplicitPageBreak || decision.ToPage != uint32(index+2) {
			t.Fatalf("break[%d] = %+v", index, decision)
		}
	}
	for _, association := range projection.SemanticFragments {
		fragment := projection.Fragments[association.Fragment-1]
		if fragment.Region != layoutengine.RegionBody && projection.SemanticNodes[association.Semantic-1].Role != layoutengine.SemanticRoleArtifact {
			t.Fatalf("shell table semantic = %+v", projection.SemanticNodes[association.Semantic-1])
		}
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "typed-structured-page-shell", plan, &characterizationRasterBudget{})
	if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 3 || raster.Pages[0].PNGSHA256 == "" {
		t.Fatalf("structured shell raster = %q %+v, %v", status, raster, err)
	}
	target := paginationTestDocument(t, 92)
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	uriCount := bytes.Count(pdf.Bytes(), []byte("/URI (https://example.test/structured-shell)"))
	footerURICount := bytes.Count(pdf.Bytes(), []byte("/URI (https://example.test/footer-evidence)"))
	if uriCount != 3 || footerURICount != 3 || !bytes.Contains(pdf.Bytes(), []byte("/StructTreeRoot")) {
		t.Fatalf("structured repeated-region PDF lacks links or tags: URI=%d StructTreeRoot=%t", uriCount, bytes.Contains(pdf.Bytes(), []byte("/StructTreeRoot")))
	}
}

func TestTypedPageTemplateRemapsInternalShellDestinationsPerPage(t *testing.T) {
	f := paginationTestDocument(t, 100)
	header := layout.ParagraphBlock{Segments: []layout.TextSegment{
		{Text: "Target", Destination: "shell-target"},
		{Text: " jump", Link: "#shell-target"},
	}, Style: layout.TextStyle{LineHeight: 10}}
	doc := &layout.LayoutDocument{
		PageTemplate: layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{header}}},
		Body: []layout.Block{
			pageShellParagraph("Page one"), layout.PageBreakBlock{After: true}, pageShellParagraph("Page two"),
		},
	}

	plan, err := f.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Destinations) != 2 || len(projection.Links) != 2 {
		t.Fatalf("shell destinations/links = %d/%d, want 2/2", len(projection.Destinations), len(projection.Links))
	}
	for index := range projection.Destinations {
		destination, link := projection.Destinations[index], projection.Links[index]
		wantPage := uint32(index + 1)
		if destination.ID != layoutengine.DestinationID(index+1) || destination.Page != wantPage ||
			!destination.Fragment.Valid() || projection.Fragments[destination.Fragment-1].Page != wantPage {
			t.Fatalf("destination[%d] = %+v", index, destination)
		}
		if link.Destination != destination.ID || projection.Fragments[link.Fragment-1].Page != wantPage || link.URI != "" {
			t.Fatalf("link[%d] = %+v for destination %+v", index, link, destination)
		}
	}

	out := paginationTestDocument(t, 100)
	if _, err := out.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := out.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(pdf.String(), "/Subtype /Link"); got != 2 {
		t.Fatalf("PDF link annotations = %d, want 2", got)
	}
	if got := strings.Count(pdf.String(), "/Dest ["); got != 2 {
		t.Fatalf("PDF internal destinations = %d, want 2", got)
	}
}

func TestTypedPageTemplateKeepsBodyAndShellDestinationDomainsDistinct(t *testing.T) {
	planner := paginationTestDocument(t, 100)
	doc := &layout.LayoutDocument{
		PageTemplate: layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{
				{Text: "Shell", Destination: "shell"}, {Text: " jump", Link: "#shell"},
			}, Style: layout.TextStyle{LineHeight: 10}},
		}}},
		Body: []layout.Block{
			layout.ParagraphBlock{Segments: []layout.TextSegment{
				{Text: "Body", Destination: "body"}, {Text: " jump", Link: "#body"},
			}, Style: layout.TextStyle{LineHeight: 10}},
			layout.PageBreakBlock{After: true}, pageShellParagraph("Page two"),
		},
	}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Destinations) != 3 || len(projection.Links) != 3 {
		t.Fatalf("destinations/links = %d/%d, want 3/3", len(projection.Destinations), len(projection.Links))
	}
	seen := map[layoutengine.RegionID]map[uint32]layoutengine.DestinationID{}
	for _, link := range projection.Links {
		fragment := projection.Fragments[link.Fragment-1]
		if seen[fragment.Region] == nil {
			seen[fragment.Region] = map[uint32]layoutengine.DestinationID{}
		}
		seen[fragment.Region][fragment.Page] = link.Destination
	}
	if seen[layoutengine.RegionBody][1] != 1 || seen[layoutengine.RegionHeader][1] != 2 || seen[layoutengine.RegionHeader][2] != 3 {
		t.Fatalf("destination domains = %#v", seen)
	}
}

func TestTypedPageTemplateComposesDecoratedShellBoxes(t *testing.T) {
	red := layout.DocumentColor{R: 240, G: 220, B: 210, Set: true}
	blue := layout.DocumentColor{R: 20, G: 60, B: 180, Set: true}
	box := layout.BoxStyle{
		Padding:         layout.Spacing{Top: 1, Right: 2, Bottom: 1, Left: 2},
		BackgroundColor: red,
		Border: layout.BorderStyle{
			Top:    layout.BorderSide{Width: 1, Style: "solid", Color: blue},
			Bottom: layout.BorderSide{Width: 1, Style: "solid", Color: blue},
		},
	}
	planner := paginationTestDocument(t, 100)
	doc := &layout.LayoutDocument{
		PageTemplate: layout.PageTemplate{
			Header: &layout.HeaderBlock{Box: box, Blocks: []layout.Block{pageShellParagraph("Header")}},
			Footer: &layout.FooterBlock{Box: box, Blocks: []layout.Block{pageShellParagraph("Footer")}},
		},
		Body: []layout.Block{pageShellParagraph("one"), layout.PageBreakBlock{After: true}, pageShellParagraph("two")},
	}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fills) != 12 || len(projection.Strokes) != 0 || len(projection.Paths) != 12 {
		t.Fatalf("decorated shell paths/fills/strokes = %d/%d/%d, want 12/12/0", len(projection.Paths), len(projection.Fills), len(projection.Strokes))
	}
	for _, fill := range projection.Fills {
		fragment := projection.Fragments[fill.Fragment-1]
		if fragment.Region != layoutengine.RegionHeader && fragment.Region != layoutengine.RegionFooter {
			t.Fatalf("shell fill owns non-shell fragment: %+v / %+v", fill, fragment)
		}
		bounds := projection.Paths[fill.Path].Bounds
		page := projection.Pages[fragment.Page-1]
		if bounds.Width <= 0 || bounds.Height <= 0 || bounds.X < 0 || bounds.Y < 0 ||
			bounds.X.Points()+bounds.Width.Points() > page.Size.Width.Points() ||
			bounds.Y.Points()+bounds.Height.Points() > page.Size.Height.Points() {
			t.Fatalf("shell fill escapes its page = %+v / %+v / %+v", fill, bounds, page.Size)
		}
	}
	first, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(first.SVG(), []byte("fill=\"#f0dcd2\"")) || !bytes.Contains(first.SVG(), []byte("fill=\"#143cb4\"")) {
		t.Fatalf("decorated shell capture = %v, %s", err, first.SVG())
	}
	target := paginationTestDocument(t, 100)
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var pdf bytes.Buffer
	if err := target.OutputWithOptions(&pdf, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf.Bytes(), []byte("0.9411764706 0.8627450980 0.8235294118 rg")) ||
		!bytes.Contains(pdf.Bytes(), []byte("0.0784313725 0.2352941176 0.7058823529 rg")) {
		t.Fatal("decorated shell colors missing from deterministic PDF")
	}
}

func TestTypedPageTemplateDecoratesMultiChildShellAsOneExactGroup(t *testing.T) {
	background := layout.DocumentColor{R: 230, G: 240, B: 250, Set: true}
	border := layout.DocumentColor{R: 25, G: 50, B: 75, Set: true}
	box := layout.BoxStyle{
		Margin:          layout.Spacing{Top: 1, Bottom: 2},
		Padding:         layout.Spacing{Top: 2, Right: 3, Bottom: 2, Left: 3},
		BackgroundColor: background,
		Border: layout.BorderStyle{
			Top: layout.BorderSide{Width: 1, Style: "solid", Color: border}, Right: layout.BorderSide{Width: 1, Style: "solid", Color: border},
			Bottom: layout.BorderSide{Width: 1, Style: "solid", Color: border}, Left: layout.BorderSide{Width: 1, Style: "solid", Color: border},
		},
	}
	planner := paginationTestDocument(t, 130)
	doc := &layout.LayoutDocument{
		PageTemplate: layout.PageTemplate{
			Header: &layout.HeaderBlock{Box: box, Blocks: []layout.Block{pageShellParagraph("Header first"), pageShellParagraph("Header second")}},
			Footer: &layout.FooterBlock{Box: box, Blocks: []layout.Block{pageShellParagraph("Footer first"), pageShellParagraph("Footer second")}},
		},
		Body: []layout.Block{pageShellParagraph("Body one"), layout.PageBreakBlock{After: true}, pageShellParagraph("Body two")},
	}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 2 || planner.PageCount() != 0 {
		t.Fatalf("plan pages/source/error = %d/%d/%v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	backgroundColor := coreGlyphColor(background)
	backgrounds := make([]layoutengine.Rect, 0, 4)
	for _, fill := range projection.Fills {
		if fill.Color == backgroundColor {
			backgrounds = append(backgrounds, projection.Paths[fill.Path].Bounds)
		}
	}
	if len(backgrounds) != 4 {
		t.Fatalf("multi-child shell backgrounds = %d, want 4", len(backgrounds))
	}
	for page := uint32(1); page <= 2; page++ {
		for _, region := range []layoutengine.RegionID{layoutengine.RegionHeader, layoutengine.RegionFooter} {
			children := make([]layoutengine.Rect, 0, 2)
			for _, fragment := range projection.Fragments {
				if fragment.Page == page && fragment.Region == region {
					children = append(children, fragment.BorderBox)
				}
			}
			if len(children) != 2 {
				t.Fatalf("page %d %s child fragments = %d, want 2", page, region, len(children))
			}
			found := false
			for _, candidate := range backgrounds {
				containsAll := true
				candidateRight, _ := candidate.Right()
				candidateBottom, _ := candidate.Bottom()
				for _, child := range children {
					childRight, _ := child.Right()
					childBottom, _ := child.Bottom()
					containsAll = containsAll && child.X >= candidate.X && child.Y >= candidate.Y && childRight <= candidateRight && childBottom <= candidateBottom
				}
				if containsAll {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("page %d %s children are not enclosed by one group background: children=%+v backgrounds=%+v", page, region, children, backgrounds)
			}
		}
	}
	for page := uint32(1); page <= 2; page++ {
		capture, captureErr := plan.CaptureDisplayPage(page)
		if captureErr != nil || len(capture.SVG()) == 0 {
			t.Fatalf("page %d multi-child capture = %v, %d bytes", page, captureErr, len(capture.SVG()))
		}
		var text strings.Builder
		pagePlan := projection.Pages[page-1]
		for commandIndex := pagePlan.Commands.Start; commandIndex < pagePlan.Commands.Start+pagePlan.Commands.Count; commandIndex++ {
			command := projection.Commands[commandIndex]
			if command.Kind == layoutengine.CommandGlyphRun {
				text.WriteString(projection.GlyphRuns[command.Payload].Codes)
				text.WriteByte('\n')
			}
		}
		for _, want := range []string{"Header first", "Header second", "Footer first", "Footer second"} {
			if !strings.Contains(text.String(), want) {
				t.Fatalf("page %d text %q does not contain %q", page, text.String(), want)
			}
		}
	}
	target := paginationTestDocument(t, 130)
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil || output.Len() == 0 {
		t.Fatalf("multi-child shell PDF = %d bytes, %v", output.Len(), err)
	}
}

func TestTypedPageTemplatePlansVariableFirstEvenRegionsCountersCaptureAndPDF(t *testing.T) {
	planner := paginationTestDocument(t, 80)
	template := layout.PageTemplate{
		Header:          &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("Default header")}},
		FirstPageHeader: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("First header A\nFirst header B")}},
		Footer:          &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Default footer")}},
		FirstPageFooter: &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("First footer")}},
		EvenPageFooter:  &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Even footer A\nEven footer B")}},
		PageNumbers:     layout.PageNumberOptions{Enabled: true, Format: "Page %d / {total}", TotalPageAlias: "{total}"},
	}
	doc := &layout.LayoutDocument{PageTemplate: template, Body: []layout.Block{
		pageShellParagraph("Body one"), layout.PageBreakBlock{After: true},
		pageShellParagraph("Body two"), layout.PageBreakBlock{After: true},
		pageShellParagraph("Body three"),
	}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 3 || planner.PageCount() != 0 {
		t.Fatalf("PlanLayoutDocument() = pages %d source pages %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	texts := make(map[uint32]map[layoutengine.RegionID]string)
	for _, command := range projection.Commands {
		if command.Kind != layoutengine.CommandGlyphRun {
			continue
		}
		fragment := projection.Fragments[command.Fragment-1]
		if texts[fragment.Page] == nil {
			texts[fragment.Page] = make(map[layoutengine.RegionID]string)
		}
		texts[fragment.Page][fragment.Region] += projection.GlyphRuns[command.Payload].Codes + "\n"
	}
	checks := []struct {
		page   uint32
		region layoutengine.RegionID
		want   string
	}{
		{1, layoutengine.RegionHeader, "First header A"}, {1, layoutengine.RegionHeader, "First header B"},
		{1, layoutengine.RegionFooter, "First footer"}, {1, layoutengine.RegionFooter, "Page 1 / 3"},
		{2, layoutengine.RegionHeader, "Default header"}, {2, layoutengine.RegionFooter, "Even footer A"},
		{2, layoutengine.RegionFooter, "Even footer B"}, {2, layoutengine.RegionFooter, "Page 2 / 3"},
		{3, layoutengine.RegionFooter, "Default footer"}, {3, layoutengine.RegionFooter, "Page 3 / 3"},
	}
	for _, check := range checks {
		if !strings.Contains(texts[check.page][check.region], check.want) {
			t.Fatalf("page %d %s text = %q, want %q", check.page, check.region, texts[check.page][check.region], check.want)
		}
	}
	var bodyY = make(map[uint32]layoutengine.Fixed)
	for _, fragment := range projection.Fragments {
		if fragment.Region == layoutengine.RegionBody && bodyY[fragment.Page] == 0 {
			bodyY[fragment.Page] = fragment.BorderBox.Y
		}
	}
	if bodyY[1] <= bodyY[2] || bodyY[2] != bodyY[3] {
		t.Fatalf("body origins = %+v; first page must reserve taller actual header", bodyY)
	}
	for _, association := range projection.SemanticFragments {
		fragment := projection.Fragments[association.Fragment-1]
		if fragment.Region != layoutengine.RegionBody && projection.SemanticNodes[association.Semantic-1].Role != layoutengine.SemanticRoleArtifact {
			t.Fatalf("shell fragment semantic = %+v / %+v", fragment, projection.SemanticNodes[association.Semantic-1])
		}
	}
	capture, err := plan.CaptureDisplayPage(2)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`data-format="display-plan-preview"`)) ||
		!bytes.Contains(capture.SVG(), []byte(">2</text>")) {
		t.Fatalf("capture = %q, %v", capture.SVG(), err)
	}
	target := paginationTestDocument(t, 80)
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != 3 {
		t.Fatalf("paint = %d, %v", pages, err)
	}
	var output bytes.Buffer
	if err := target.Output(&output); err != nil || output.Len() == 0 {
		t.Fatalf("PDF = %d bytes, %v", output.Len(), err)
	}
}

func TestTypedPageTemplateRejectsCircularFixedAndImpossibleRegionsAtomically(t *testing.T) {
	tests := []struct {
		name     string
		template layout.PageTemplate
		want     string
	}{
		{"circular alias", layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("Total {pages}")}}, PageNumbers: layout.PageNumberOptions{Enabled: true, TotalPageAlias: "{pages}"}}, "circular page-master dependency"},
		{"manual header height", layout.PageTemplate{Header: &layout.HeaderBlock{Height: 20, Blocks: []layout.Block{pageShellParagraph("Header")}}}, "manual height conflicts"},
		{"manual footer reserve", layout.PageTemplate{ReserveFooterHeight: 10, Footer: &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Footer")}}}, "manual footer reserve heights conflict"},
		{"explicit break", layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("A"), layout.PageBreakBlock{After: true}, pageShellParagraph("B")}}}, "subtree exceeds one page"},
		{"no body region", layout.PageTemplate{Header: &layout.HeaderBlock{Blocks: []layout.Block{pageShellParagraph("1\n2\n3\n4\n5\n6")}}}, "leave no body region"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planner := paginationTestDocument(t, 80)
			plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: test.template, Body: []layout.Block{pageShellParagraph("body")}})
			if err == nil || !strings.Contains(err.Error(), test.want) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("plan = pages %d hash %q error %v, want %q", plan.PageCount(), plan.Hash(), err, test.want)
			}
		})
	}
}

func TestTypedPageTemplateCancellationAndPageLimitsAreAtomic(t *testing.T) {
	template := layout.PageTemplate{Footer: &layout.FooterBlock{Blocks: []layout.Block{pageShellParagraph("Footer")}},
		PageNumbers: layout.PageNumberOptions{Enabled: true, Format: "Page %d"}}
	body := []layout.Block{pageShellParagraph("one"), layout.PageBreakBlock{After: true}, pageShellParagraph("two")}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := paginationTestDocument(t, 80)
	if _, err := planner.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{PageTemplate: template, Body: body}); !errors.Is(err, context.Canceled) || planner.PageCount() != 0 {
		t.Fatalf("canceled plan = pages %d, %v", planner.PageCount(), err)
	}
	limited := paginationTestDocument(t, 80, WithLimits(Limits{MaxPages: 1}))
	if _, err := limited.PlanLayoutDocument(&layout.LayoutDocument{PageTemplate: template, Body: body}); !errors.Is(err, ErrPageLimitExceeded) || limited.PageCount() != 0 {
		t.Fatalf("limited plan = pages %d, %v", limited.PageCount(), err)
	}
}
