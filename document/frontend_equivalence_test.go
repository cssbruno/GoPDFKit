// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestPaperTypedAndHTMLEquivalentParagraphProduceEquivalentPlans(t *testing.T) {
	const text = "Equivalent frontend plan"
	paperSource := "document @equivalent:\n" +
		"  page @page:\n" +
		"    width: 200pt\n" +
		"    height: 120pt\n" +
		"    margin: 10pt\n" +
		"    body @body:\n" +
		"      paragraph @paragraph:\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 12pt\n" +
		"        line-height: 12pt\n" +
		"        text: \"" + text + "\"\n"
	assertEquivalentFrontendPlans(t, paperSource, []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}},
			Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 12, LineHeight: 12}},
	}, "<p>"+text+"</p>", 12)
}

func TestPaperTypedAndHTMLEquivalentHeadingProduceEquivalentPlans(t *testing.T) {
	const text = "Equivalent heading"
	paperSource := "document @equivalent:\n" +
		"  page @page:\n" +
		"    width: 200pt\n" +
		"    height: 120pt\n" +
		"    margin: 10pt\n" +
		"    body @body:\n" +
		"      heading @heading:\n" +
		"        level: 1\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 12pt\n" +
		"        line-height: 12pt\n" +
		"        text: \"" + text + "\"\n"
	assertEquivalentFrontendPlans(t, paperSource, []layout.Block{
		layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: text}},
			Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 12, LineHeight: 12}},
	}, `<h1 style="font-family:Helvetica;font-size:12pt;line-height:12pt;font-weight:normal;margin:0">`+text+"</h1>", 12)
}

func TestPaperTypedAndHTMLEquivalentListProduceEquivalentPlans(t *testing.T) {
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}},
			Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 12, LineHeight: 12}}
	}
	paperSource := "document @equivalent:\n" +
		"  page @page:\n" +
		"    width: 200pt\n" +
		"    height: 120pt\n" +
		"    margin: 10pt\n" +
		"    body @body:\n" +
		"      list @list:\n" +
		"        marker: \"dash\"\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 12pt\n" +
		"        line-height: 12pt\n" +
		"        item @first:\n" +
		"          text: \"First\"\n" +
		"        item @second:\n" +
		"          text: \"Second\"\n"
	assertEquivalentFrontendPlans(t, paperSource, []layout.Block{
		layout.ListBlock{MarkerStyle: "dash", Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 12, LineHeight: 12},
			Items: []layout.ListItem{{Blocks: []layout.Block{paragraph("First")}}, {Blocks: []layout.Block{paragraph("Second")}}}},
	}, "<ul><li>First</li><li>Second</li></ul>", 12)
}

func TestPaperTypedAndHTMLEquivalentExplicitBreakProduceEquivalentPlans(t *testing.T) {
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: text}},
			Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 12, LineHeight: 12}}
	}
	paperSource := "document @equivalent:\n" +
		"  page @page:\n" +
		"    width: 200pt\n" +
		"    height: 120pt\n" +
		"    margin: 10pt\n" +
		"    body @body:\n" +
		"      paragraph @first:\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 12pt\n" +
		"        line-height: 12pt\n" +
		"        text: \"First\"\n" +
		"      page-break @break:\n" +
		"      paragraph @second:\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 12pt\n" +
		"        line-height: 12pt\n" +
		"        text: \"Second\"\n"
	assertEquivalentFrontendPlans(t, paperSource, []layout.Block{
		paragraph("First"), layout.PageBreakBlock{After: true}, paragraph("Second"),
	}, `<p style="page-break-after: always">First</p><p>Second</p>`, 12)
}

func assertEquivalentFrontendPlans(t *testing.T, paperSource string, typedBlocks []layout.Block, htmlSource string, lineHeight float64) {
	t.Helper()
	paperPlan, result, err := PlanPaper("equivalent.paper", paperSource)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", result, err)
	}

	typedPlanner := equivalentFrontendPlanner()
	typedPlan, err := typedPlanner.PlanLayoutDocument(&layout.LayoutDocument{Body: typedBlocks})
	if err != nil {
		t.Fatalf("typed plan = %v", err)
	}

	compiled, err := CompileHTML(htmlSource)
	if err != nil {
		t.Fatal(err)
	}
	htmlPlanner := equivalentFrontendPlanner()
	htmlPlan, err := htmlPlanner.PlanCompiledHTML(lineHeight, compiled)
	if err != nil {
		t.Fatalf("HTML plan = %v", err)
	}

	want := normalizedFrontendPlan(paperPlan.plan)
	for name, candidate := range map[string]layoutengine.LayoutPlan{"typed": typedPlan.plan, "html": htmlPlan.plan} {
		got := normalizedFrontendPlan(candidate)
		if !reflect.DeepEqual(got, want) {
			wantJSON, err := json.MarshalIndent(want, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gotJSON, err := json.MarshalIndent(got, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			t.Fatalf("%s plan differs from .paper:\nwant %s\ngot  %s", name, wantJSON, gotJSON)
		}
	}
}

func equivalentFrontendPlanner() *Document {
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 120}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(10, 10, 10)
	planner.SetAutoPageBreak(true, 10)
	return planner
}

type frontendPlanEquivalence struct {
	Pages          []layoutengine.PlannedPage
	Fragments      []layoutengine.Fragment
	Lines          []layoutengine.PlannedLine
	Fonts          []layoutengine.CoreFontResource
	GlyphRuns      []layoutengine.CoreGlyphRun
	ImageResources []layoutengine.ImageResource
	Images         []layoutengine.PlannedImage
	Destinations   []layoutengine.PlannedDestination
	Links          []layoutengine.PlannedLink
	Paths          []layoutengine.PlannedPath
	Transforms     []layoutengine.Transform
	Clips          []layoutengine.PlannedClip
	Fills          []layoutengine.PlannedFill
	Strokes        []layoutengine.PlannedStroke
	Commands       []layoutengine.DisplayCommand
	Breaks         []layoutengine.BreakDecision
	SemanticRoles  []layoutengine.SemanticRole
	ReadingRoles   []layoutengine.SemanticRole
}

func normalizedFrontendPlan(plan layoutengine.LayoutPlan) frontendPlanEquivalence {
	projection := plan.Projection()
	fragmentIDs := make(map[layoutengine.FragmentID]layoutengine.FragmentID, len(projection.Fragments))
	for index := range projection.Fragments {
		fragmentIDs[projection.Fragments[index].ID] = layoutengine.FragmentID(index + 1)
		projection.Fragments[index].ID = layoutengine.FragmentID(index + 1)
		projection.Fragments[index].Node = 0
		projection.Fragments[index].Key = ""
		projection.Fragments[index].Instance = ""
		projection.Fragments[index].Source = layoutengine.SourceSpan{}
	}
	for index := range projection.Lines {
		projection.Lines[index].Fragment = fragmentIDs[projection.Lines[index].Fragment]
		projection.Lines[index].Source = layoutengine.SourceSpan{}
	}
	for index := range projection.GlyphRuns {
		projection.GlyphRuns[index].Source = layoutengine.SourceSpan{}
	}
	for index := range projection.Commands {
		projection.Commands[index].Fragment = fragmentIDs[projection.Commands[index].Fragment]
	}
	for index := range projection.Images {
		projection.Images[index].Fragment = fragmentIDs[projection.Images[index].Fragment]
		projection.Images[index].Source = layoutengine.SourceSpan{}
	}
	for index := range projection.Links {
		projection.Links[index].Fragment = fragmentIDs[projection.Links[index].Fragment]
		projection.Links[index].Source = layoutengine.SourceSpan{}
	}
	for index := range projection.Fills {
		projection.Fills[index].Fragment = fragmentIDs[projection.Fills[index].Fragment]
	}
	for index := range projection.Strokes {
		projection.Strokes[index].Fragment = fragmentIDs[projection.Strokes[index].Fragment]
	}
	for index := range projection.Clips {
		projection.Clips[index].Fragment = fragmentIDs[projection.Clips[index].Fragment]
	}
	for index := range projection.Breaks {
		projection.Breaks[index].Preceding = fragmentIDs[projection.Breaks[index].Preceding]
		projection.Breaks[index].Triggering = fragmentIDs[projection.Breaks[index].Triggering]
	}
	semanticRoles := make([]layoutengine.SemanticRole, len(projection.SemanticNodes))
	roleByID := make(map[layoutengine.SemanticNodeID]layoutengine.SemanticRole, len(projection.SemanticNodes))
	for index, semantic := range projection.SemanticNodes {
		semanticRoles[index] = semantic.Role
		roleByID[semantic.ID] = semantic.Role
	}
	readingRoles := make([]layoutengine.SemanticRole, len(projection.ReadingOrder))
	for index, occurrence := range projection.ReadingOrder {
		readingRoles[index] = roleByID[occurrence.Semantic]
	}
	return frontendPlanEquivalence{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns, ImageResources: projection.ImageResources,
		Images: projection.Images, Destinations: projection.Destinations, Links: projection.Links,
		Paths: projection.Paths, Transforms: projection.Transforms, Clips: projection.Clips,
		Fills: projection.Fills, Strokes: projection.Strokes, Commands: projection.Commands,
		Breaks: projection.Breaks, SemanticRoles: semanticRoles, ReadingRoles: readingRoles,
	}
}
