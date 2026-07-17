// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"golang.org/x/image/font/gofont/goregular"
)

const paperPageRegionSource = "document @report:\n" +
	"  language: \"en\"\n" +
	"  page @master:\n" +
	"    width: 200pt\n" +
	"    height: 140pt\n" +
	"    margin: 12pt\n" +
	"    header @running-head:\n" +
	"      paragraph @header-copy:\n" +
	"        size: 8pt\n" +
	"        text: \"RUNNING HEADER\"\n" +
	"    footer @running-foot:\n" +
	"      paragraph @footer-copy:\n" +
	"        size: 8pt\n" +
	"        text: \"RUNNING FOOTER\"\n" +
	"    body @body:\n" +
	"      paragraph @copy:\n" +
	"        text: \"Body content\"\n"

func TestPaperPageRegionsPlanCaptureRasterPDFAndSemantics(t *testing.T) {
	plan, result, err := PlanPaper("regions.paper", paperPageRegionSource)
	if err != nil || !result.OK() || result.Pages != 1 {
		t.Fatalf("PlanPaper = %#v, %v", result, err)
	}
	projection := plan.plan.Projection()
	if len(projection.PageRegions) != 3 || projection.PageRegions[0].Region != layoutengine.RegionHeader ||
		projection.PageRegions[1].Region != layoutengine.RegionBody || projection.PageRegions[2].Region != layoutengine.RegionFooter {
		t.Fatalf("retained page regions = %+v", projection.PageRegions)
	}
	headerBottom, _ := projection.PageRegions[0].Bounds.Bottom()
	bodyBottom, _ := projection.PageRegions[1].Bounds.Bottom()
	if headerBottom > projection.PageRegions[1].Bounds.Y || bodyBottom > projection.PageRegions[2].Bounds.Y {
		t.Fatalf("retained page regions overlap = %+v", projection.PageRegions)
	}
	regions := map[layoutengine.RegionID]bool{}
	fragmentRegions := map[layoutengine.FragmentID]layoutengine.RegionID{}
	for _, fragment := range projection.Fragments {
		regions[fragment.Region] = true
		fragmentRegions[fragment.ID] = fragment.Region
	}
	if !regions[layoutengine.RegionHeader] || !regions[layoutengine.RegionBody] || !regions[layoutengine.RegionFooter] {
		t.Fatalf("regions = %#v", regions)
	}
	capture, err := plan.CaptureDisplayPageSVG(context.Background(), 1, nil)
	if err != nil || bytes.Count(capture.SVG, []byte("<text ")) < len("RUNNING HEADERRUNNING FOOTERBody content") {
		t.Fatalf("SVG = %s, %v", capture.SVG, err)
	}
	regionText := map[layoutengine.RegionID]*strings.Builder{}
	for _, run := range projection.GlyphRuns {
		region := fragmentRegions[projection.Lines[run.Line].Fragment]
		if regionText[region] == nil {
			regionText[region] = &strings.Builder{}
		}
		regionText[region].WriteString(run.Codes)
	}
	if regionText[layoutengine.RegionHeader].String() != "RUNNING HEADER" ||
		regionText[layoutengine.RegionBody].String() != "Body content" ||
		regionText[layoutengine.RegionFooter].String() != "RUNNING FOOTER" {
		t.Fatalf("region text = %#v", regionText)
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
}

func TestPaperPageRegionsPlanMixedRowColumnBody(t *testing.T) {
	const source = "document @report:\n" +
		"  page @master:\n" +
		"    width: 200pt\n" +
		"    height: 140pt\n" +
		"    margin: 12pt\n" +
		"    header @running-head:\n" +
		"      paragraph @header-copy:\n" +
		"        size: 8pt\n" +
		"        text: \"RUNNING HEADER\"\n" +
		"    body @body:\n" +
		"      paragraph @copy:\n" +
		"        text: \"Body content\"\n" +
		"      row @summary:\n" +
		"        paragraph @label:\n" +
		"          text: \"Summary\"\n"
	plan, result, err := PlanPaper("regions-mixed.paper", source)
	if err != nil || !result.OK() || result.Pages != 1 {
		t.Fatalf("PlanPaper mixed body = %#v, %v", result, err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) < 3 {
		t.Fatalf("mixed body fragments = %+v", fragments)
	}
}
