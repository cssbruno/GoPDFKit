package document

import (
	"bytes"
	"context"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

const paperCanvasSource = "document @d:\n  language: \"en\"\n  page @p:\n    width: 200pt\n    height: 120pt\n    margin: 12pt\n    body @b:\n      canvas @diagram:\n        width: 160pt\n        height: 80pt\n        anchor @base:\n          width: 40pt\n          height: 20pt\n          left: \"canvas.left + 8pt\"\n          top: \"canvas.top + 8pt\"\n          background: \"#336699\"\n          alt: \"Base panel\"\n        anchor @badge:\n          width: 24pt\n          height: 12pt\n          left: \"@base.right + 6pt\"\n          top: \"@base.top\"\n          background: \"#cc3300\"\n          alt: \"Status badge\"\n"

func TestPaperCanvasUsesUnifiedAnchorPlanAndExactOutputs(t *testing.T) {
	plan, result, err := PlanPaper("canvas.paper", paperCanvasSource)
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaper = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 2 || projection.Fragments[0].BorderBox.X.Points() != 20 ||
		projection.Fragments[1].BorderBox.X.Points() != 66 || projection.Fragments[1].BorderBox.Y.Points() != 20 {
		t.Fatalf("fragments = %#v", projection.Fragments)
	}
	if len(projection.SemanticNodes) != 4 || projection.SemanticNodes[1].Key != "@diagram" || projection.SemanticNodes[3].Attributes.AlternateText != "Status badge" {
		t.Fatalf("semantics = %#v", projection.SemanticNodes)
	}
	query, err := plan.Query(PaperPlanSelector{Key: "@diagram", MaxResults: 8})
	if err != nil || !bytes.Contains(query.JSON(), []byte(`"key":"@diagram"`)) || !bytes.Contains(query.JSON(), []byte(`"file":"canvas.paper"`)) {
		t.Fatalf("canvas query = %s, %v", query.JSON(), err)
	}
	explain, err := plan.Explain([]PaperPlanSelector{{Key: "@base", MaxResults: 8}}, 4, 64<<10)
	if err != nil || !bytes.Contains(explain.JSON(), []byte(`"key":"@base"`)) || !bytes.Contains(explain.JSON(), []byte(`"file":"canvas.paper"`)) {
		t.Fatalf("canvas explain = %s, %v", explain.JSON(), err)
	}
	svg, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || bytes.Count(svg.SVG, []byte("<path ")) < 2 {
		t.Fatalf("SVG = %s, %v", svg.SVG, err)
	}
	rasterRequest := DefaultPaperPlanRasterRequest()
	rasterRequest.CoreFontProgram = goregular.TTF
	raster, err := plan.CaptureRasterPages(context.Background(), rasterRequest)
	if err != nil || len(raster.Pages) != 1 || len(raster.Pages[0].PNG) == 0 {
		t.Fatalf("raster = %#v, %v", raster, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if rendered, err := target.WritePaperPlan(plan); err != nil || !rendered.OK() {
		t.Fatalf("WritePaperPlan = %#v, %v", rendered, err)
	}
	var pdf bytes.Buffer
	if err := target.Output(&pdf); err != nil || pdf.Len() == 0 {
		t.Fatalf("PDF bytes=%d, %v", pdf.Len(), err)
	}
	for _, diagnostic := range projection.Diagnostics {
		if diagnostic.Code == layoutengine.DiagnosticCanvasNodeOverflow {
			t.Fatalf("unexpected canvas overflow: %#v", diagnostic)
		}
	}
}

func TestPaperCanvasComposesWithFlowAndPageShell(t *testing.T) {
	source := "document @d:\n  language: \"en\"\n  page @p:\n    width: 200pt\n    height: 180pt\n    margin: 12pt\n    header @head:\n      paragraph @head-copy:\n        size: 8pt\n        text: \"HEADER\"\n    body @b:\n      paragraph @before:\n        size: 8pt\n        text: \"Before\"\n      canvas @diagram:\n        width: 160pt\n        height: 80pt\n        anchor @base:\n          width: 40pt\n          height: 20pt\n          left: \"canvas.left + 8pt\"\n          top: \"canvas.top + 8pt\"\n          background: \"#336699\"\n          alt: \"Base panel\"\n        anchor @badge:\n          width: 24pt\n          height: 12pt\n          left: \"@base.right + 6pt\"\n          top: \"@base.top\"\n          background: \"#cc3300\"\n      paragraph @after:\n        size: 8pt\n        text: \"After\"\n"
	plan, result, err := PlanPaper("mixed-canvas.paper", source)
	if err != nil || !result.OK() || result.Pages != 1 {
		t.Fatalf("PlanPaper = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	keys, regions := map[layoutengine.NodeKey]layoutengine.Fragment{}, map[layoutengine.RegionID]bool{}
	for _, fragment := range projection.Fragments {
		keys[fragment.Key], regions[fragment.Region] = fragment, true
	}
	if keys["@before"].ID == 0 || keys["@base"].ID == 0 || keys["@badge"].ID == 0 || keys["@after"].ID == 0 ||
		!regions[layoutengine.RegionHeader] || !regions[layoutengine.RegionBody] || keys["@base"].BorderBox.Y <= keys["@before"].BorderBox.Y ||
		keys["@after"].BorderBox.Y <= keys["@base"].BorderBox.Y {
		t.Fatalf("mixed fragments = %#v", projection.Fragments)
	}
	query, err := plan.Query(PaperPlanSelector{Key: "@diagram", MaxResults: 8})
	if err != nil || !bytes.Contains(query.JSON(), []byte(`"key":"@diagram"`)) {
		t.Fatalf("canvas container query = %s, %v", query.JSON(), err)
	}
	svg, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || bytes.Count(svg.SVG, []byte("<path ")) < 2 || bytes.Count(svg.SVG, []byte("<text ")) < len("HEADERBeforeAfter") {
		t.Fatalf("mixed SVG = %s, %v", svg.SVG, err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	if rendered, err := target.WritePaperPlan(plan); err != nil || !rendered.OK() {
		t.Fatalf("WritePaperPlan = %#v, %v", rendered, err)
	}
}

func TestPaperCanvasConsumesExplicitFlowBreakAsOneBlock(t *testing.T) {
	source := "document @d:\n  page @p:\n    width: 160pt\n    height: 120pt\n    margin: 10pt\n    body @b:\n      paragraph @before:\n        text: \"Before\"\n      page-break @break:\n      canvas @diagram:\n        width: 100pt\n        height: 50pt\n        anchor @box:\n          width: 20pt\n          height: 20pt\n          left: \"canvas.left\"\n          top: \"canvas.top\"\n          background: \"#336699\"\n      paragraph @after:\n        text: \"After\"\n"
	plan, result, err := PlanPaper("canvas-break.paper", source)
	if err != nil || !result.OK() || result.Pages != 2 {
		t.Fatalf("PlanPaper = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	pages := map[layoutengine.NodeKey]uint32{}
	for _, fragment := range projection.Fragments {
		pages[fragment.Key] = fragment.Page
	}
	if pages["@before"] != 1 || pages["@box"] != 2 || pages["@after"] != 2 || len(projection.Breaks) != 1 || projection.Breaks[0].Reason != layoutengine.BreakExplicitPageBreak {
		t.Fatalf("pages=%#v breaks=%#v", pages, projection.Breaks)
	}
}
