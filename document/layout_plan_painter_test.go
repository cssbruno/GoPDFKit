// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestPaintCoreLayoutPlanPDFEmitsOnlyPlannedPagesAndPositionedText(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 72, Ht: 40}))
	source.SetMargins(8, 8, 8)
	source.SetAutoPageBreak(true, 8)
	source.SetFont("Courier", "", 10)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "AB\nCD\nEF\nGH"}},
		Style: layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 8,
			Color: layout.DocumentColor{R: 51, G: 102, B: 153, Set: true}},
	}}
	shadow, err := source.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("planTypedParagraphLineShadow() = %v", err)
	}
	projection := shadow.Plan.Projection()

	target := MustNew(WithUnit(UnitMillimeter), WithNoCompression())
	if err := target.paintCoreLayoutPlanPDF(shadow.Plan); err != nil {
		t.Fatalf("paintCoreLayoutPlanPDF() = %v", err)
	}
	if got, want := target.PageCount(), len(projection.Pages); got != want {
		t.Fatalf("painted pages = %d, want %d", got, want)
	}
	for pageIndex, page := range projection.Pages {
		width, height, _ := target.PageSize(pageIndex + 1)
		if got, want := target.UnitToPointConvert(width), page.Size.Width.Points(); got != want {
			t.Fatalf("page %d width = %gpt, want %gpt", pageIndex+1, got, want)
		}
		if got, want := target.UnitToPointConvert(height), page.Size.Height.Points(); got != want {
			t.Fatalf("page %d height = %gpt, want %gpt", pageIndex+1, got, want)
		}
		content := target.pages[pageIndex+1].Bytes()
		if bytes.Contains(content, []byte(" Tw")) || bytes.Contains(content, []byte(" Td")) {
			t.Fatalf("page %d contains stateful spacing/relative-position operators:\n%s", pageIndex+1, content)
		}
		if !bytes.Contains(content, []byte("0.2000000000 0.4000000000 0.6000000000 rg")) {
			t.Fatalf("page %d lacks planned RGB text color:\n%s", pageIndex+1, content)
		}
		if got, want := bytes.Count(content, []byte(" Tm (")), int(page.Commands.Count)*2; got != want {
			t.Fatalf("page %d positioned glyphs = %d, want %d", pageIndex+1, got, want)
		}
		if got, want := bytes.Count(content, []byte(") Tj")), int(page.Commands.Count)*2; got != want {
			t.Fatalf("page %d painted glyphs = %d, want %d", pageIndex+1, got, want)
		}
	}
	all := target.pages[1].String()
	if !strings.Contains(all, "(A) Tj") || !strings.Contains(all, "(B) Tj") || !strings.Contains(all, " Tf ") {
		t.Fatalf("first page lacks direct planned core text operators:\n%s", all)
	}
}

func TestPaintCoreLayoutPlanPDFPreflightFailureDoesNotMutateDocument(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 80, Ht: 80}))
	source.SetMargins(10, 10, 10)
	source.SetAutoPageBreak(true, 10)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "paint me"}},
		Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 10},
	}}
	shadow, err := source.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("planTypedParagraphLineShadow() = %v", err)
	}
	projection := shadow.Plan.Projection()
	projection.Fonts[0].MetricsDigest = layoutengine.CoreFontMetricsDigest(strings.Repeat("1", 64))
	wrongMetrics, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns, Commands: projection.Commands,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan(wrong metrics) = %v", err)
	}

	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	before := typedShadowSnapshotOf(target)
	err = target.paintCoreLayoutPlanPDF(wrongMetrics)
	if !errors.Is(err, errCoreLayoutPlanPaintUnsupported) {
		t.Fatalf("paint error = %v, want errCoreLayoutPlanPaintUnsupported", err)
	}
	if after := typedShadowSnapshotOf(target); after != before {
		t.Fatalf("failed painter preflight mutated document:\nbefore %#v\nafter  %#v", before, after)
	}
	if target.PageCount() != 0 || target.Error() != nil {
		t.Fatalf("failed painter opened document: pages %d, error %v", target.PageCount(), target.Error())
	}
}
