// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestTypedRowColumnInterleavesTextAndDecoratedImagesExactly(t *testing.T) {
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	red := layout.DocumentColor{R: 210, G: 30, B: 20, Set: true}
	blue := layout.DocumentColor{R: 20, G: 40, B: 190, Set: true}
	box := layout.BoxStyle{
		Padding:         layout.Spacing{Top: 2, Right: 3, Bottom: 4, Left: 5},
		BackgroundColor: red,
		Border: layout.BorderStyle{
			Top:    layout.BorderSide{Width: 1, Style: "solid", Color: blue},
			Right:  layout.BorderSide{Width: 1, Style: "solid", Color: blue},
			Bottom: layout.BorderSide{Width: 1, Style: "solid", Color: blue},
			Left:   layout.BorderSide{Width: 1, Style: "solid", Color: blue},
		},
	}
	doc := &layout.LayoutDocument{PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Left: 20, Top: 20, Right: 20, Bottom: 20}}, Body: []layout.Block{layout.RowColumnBlock{
		Direction: layout.RowDirection,
		Gap:       8,
		Items: []layout.RowColumnItem{
			{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 60}, Block: layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: "Evidence"}}, Style: layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12},
			}},
			{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 70}, Block: layout.ImageBlock{
				DataRef: &pixel, Format: "png", Alt: "Reviewed proof", Width: 30, Height: 20,
				Fit: layout.ImageFitContain, Align: "center", BoxRef: &box,
			}},
			{Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 54}, Block: layout.ImageBlock{
				Data: pixel, Format: "png", Width: 20, Height: 15, Align: "right",
			}},
		},
	}}}
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 100}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(20, 20, 20)
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 1 || plan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("PlanLayoutDocument(row images) = pages %d hash %q source pages %d, %v", plan.PageCount(), plan.Hash(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fragments) != 3 || len(projection.Lines) != 1 || len(projection.GlyphRuns) != 1 ||
		len(projection.ImageResources) != 1 || len(projection.Images) != 2 || len(projection.Paths) != 5 ||
		len(projection.Fills) != 1 || len(projection.Strokes) != 4 || len(projection.Commands) != 8 {
		t.Fatalf("row image plan cardinality fragments/lines/runs/resources/images/paths/fills/strokes/commands = %d/%d/%d/%d/%d/%d/%d/%d/%d",
			len(projection.Fragments), len(projection.Lines), len(projection.GlyphRuns), len(projection.ImageResources), len(projection.Images),
			len(projection.Paths), len(projection.Fills), len(projection.Strokes), len(projection.Commands))
	}
	if projection.Commands[0].Kind != layoutengine.CommandGlyphRun || projection.Commands[1].Kind != layoutengine.CommandFillPath ||
		projection.Commands[2].Kind != layoutengine.CommandImage || projection.Commands[7].Kind != layoutengine.CommandImage {
		t.Fatalf("mixed child paint order = %#v", projection.Commands)
	}
	firstImage := projection.Fragments[1]
	if firstImage.BorderBox.X.Points() != 103 || firstImage.BorderBox.Width.Points() != 40 || firstImage.BorderBox.Height.Points() != 28 ||
		firstImage.ContentBox.X.Points() != 109 || firstImage.ContentBox.Y.Points() != 23 ||
		firstImage.ContentBox.Width.Points() != 30 || firstImage.ContentBox.Height.Points() != 20 {
		t.Fatalf("decorated container image boxes = border %+v content %+v", firstImage.BorderBox, firstImage.ContentBox)
	}
	if got := projection.Fragments[2].BorderBox.X.Points(); got != 200 {
		t.Fatalf("right-aligned decorative image x = %v, want 200", got)
	}
	var figure, artifact bool
	for _, node := range projection.SemanticNodes {
		switch node.Role {
		case layoutengine.SemanticRoleFigure:
			figure = node.Attributes.AlternateText == "Reviewed proof"
		case layoutengine.SemanticRoleArtifact:
			artifact = true
		}
	}
	if !figure || !artifact || len(projection.SemanticFragments) != 3 || len(projection.ReadingOrder) != 2 {
		t.Fatalf("row image semantics nodes=%+v fragments=%+v reading=%+v", projection.SemanticNodes, projection.SemanticFragments, projection.ReadingOrder)
	}

	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`class="planned-image"`)) ||
		!bytes.Contains(capture.SVG(), []byte(`fill="#d21e14"`)) || bytes.Count(capture.SVG(), []byte("data:image/png;base64,")) != 2 {
		t.Fatalf("row image exact display capture = %v\n%s", err, capture.SVG())
	}
	// The bounded direct rasterizer deliberately rejects stroke commands. A
	// border-free snapshot proves the same mixed container/image/fill geometry
	// through raster capture; the decorated snapshot remains covered above and
	// by exact PDF replay below.
	box.Border = layout.BorderStyle{}
	rasterPlan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	raster, status, err := captureCharacterizationRaster(t.Context(), "row-column-images", rasterPlan, &characterizationRasterBudget{})
	rasterAgain, statusAgain, againErr := captureCharacterizationRaster(t.Context(), "row-column-images", rasterPlan, &characterizationRasterBudget{})
	if err != nil || againErr != nil || status != "captured" || statusAgain != "captured" || raster == nil || rasterAgain == nil ||
		len(raster.Pages) != 1 || len(rasterAgain.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" ||
		raster.Pages[0].PNGSHA256 != rasterAgain.Pages[0].PNGSHA256 || raster.Pages[0].PNGBytes == 0 {
		t.Fatalf("row image raster evidence = status %q/%q evidence %+v/%+v, %v/%v", status, statusAgain, raster, rasterAgain, err, againErr)
	}
	before := plan.Hash()
	pixel[0] ^= 0xff
	box.Padding.Left = 99
	if plan.Hash() != before || plan.plan.Projection().Fragments[1] != firstImage {
		t.Fatal("row image plan aliases DataRef or BoxRef")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != 1 || target.PageCount() != 1 {
		t.Fatalf("WriteLayoutDocumentPlan(row images) = pages %d target pages %d, %v", pages, target.PageCount(), err)
	}
	content := target.pages[1].Bytes()
	if !bytes.Contains(content, []byte("(E) Tj")) || !bytes.Contains(content, []byte("(e) Tj")) || bytes.Count(content, []byte("/I")) < 2 ||
		!bytes.Contains(content, []byte(" rg")) || bytes.Count(content, []byte(" S")) != 4 {
		t.Fatalf("row image PDF paint is incomplete:\n%s", content)
	}
}

func TestTypedRowColumnImagePlansCaptionAndCancellationAtomically(t *testing.T) {
	pixel, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 120, Ht: 140}))
	image := layout.ImageBlock{Data: pixel, Format: "png", Width: 20, Height: 20, Caption: []layout.TextSegment{{Text: "pending"}}}
	doc := &layout.LayoutDocument{Body: []layout.Block{layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{
		Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 40}, Block: image,
	}}}}}
	captionPlan, err := planner.PlanLayoutDocument(doc)
	if err != nil || captionPlan.Hash() == "" || planner.PageCount() != 0 {
		t.Fatalf("caption plan = %#v, %v, source pages %d", captionPlan, err, planner.PageCount())
	}
	var caption strings.Builder
	for _, run := range captionPlan.plan.Projection().GlyphRuns {
		caption.WriteString(run.Codes)
	}
	projection := captionPlan.plan.Projection()
	if caption.String() != "pending" || len(projection.Images) != 1 || len(projection.GlyphRuns) != 1 ||
		projection.GlyphRuns[0].Origin.X <= projection.Fragments[0].ContentBox.X ||
		projection.GlyphRuns[0].Origin.Y <= projection.Images[0].Bounds.Y+projection.Images[0].Bounds.Height {
		t.Fatalf("caption output = %q images=%d", caption.String(), len(captionPlan.plan.Projection().Images))
	}
	image.Caption = nil
	doc.Body = []layout.Block{layout.RowColumnBlock{Direction: layout.RowDirection, Items: []layout.RowColumnItem{{
		Track: layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 40}, Block: image,
	}}}}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := planner.PlanLayoutDocumentContext(canceled, doc); !errors.Is(err, context.Canceled) || planner.PageCount() != 0 {
		t.Fatalf("canceled row image plan = %v, pages %d", err, planner.PageCount())
	}
}
