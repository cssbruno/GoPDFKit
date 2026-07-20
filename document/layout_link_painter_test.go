// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
)

func TestPaintDisplayLayoutPlanPDFReplaysExactInternalAndExternalLinksWithoutLayout(t *testing.T) {
	plan := plannedLinkPDFPlan(t)
	target := MustNew(WithUnit(UnitMillimeter), WithNoCompression(), WithDeterministicOutput())
	if err := target.paintDisplayLayoutPlanPDF(plan, nil); err != nil {
		t.Fatal(err)
	}
	if target.PageCount() != 2 || len(target.pageLinks[1]) != 2 || len(target.pageLinks[2]) != 0 {
		t.Fatalf("pages/annotations = %d, %d/%d", target.PageCount(), len(target.pageLinks[1]), len(target.pageLinks[2]))
	}
	internal, external := target.pageLinks[1][0], target.pageLinks[1][1]
	if internal.link != 1 || internal.linkStr != "" || external.link != 0 || external.linkStr != "https://example.test/exact" {
		t.Fatalf("planned annotations = internal %+v external %+v", internal, external)
	}
	if len(target.links) != 2 || target.links[1].page != 2 ||
		target.links[1].x != target.PointConvert(25) || target.links[1].y != target.PointConvert(40) {
		t.Fatalf("planned destination = %+v", target.links)
	}
	for page := 1; page <= 2; page++ {
		content := target.pages[page].Bytes()
		if bytes.Contains(content, []byte("BT")) || bytes.Contains(content, []byte(" Td")) || bytes.Contains(content, []byte(" Tm")) {
			t.Fatalf("link replay performed text layout on page %d: %s", page, content)
		}
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte("/URI (https://example.test/exact)")) ||
		!bytes.Contains(output.Bytes(), []byte("/XYZ 25.00 110.00 null")) {
		t.Fatalf("PDF does not contain exact planned targets:\n%s", output.Bytes())
	}
}

func TestInvalidPlannedLinkTargetFailsBeforeDocumentMutation(t *testing.T) {
	geometry, source := plannedLinkGeometry(t)
	invalidDestination := layoutengine.PlannedDestination{ID: 1, Page: 3, Point: layoutengine.Point{}, Source: source}
	link := layoutengine.PlannedLink{Fragment: 1, Bounds: fixedRect(10, 10, 20, 10), Destination: 1, Source: source}
	if _, err := layoutengine.AttachLinks(geometry, []layoutengine.PlannedDestination{invalidDestination}, []layoutengine.PlannedLink{link}); err == nil {
		t.Fatal("invalid destination page unexpectedly produced a paintable plan")
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression())
	before := typedShadowSnapshotOf(target)
	beforeResources := target.resources
	if after := typedShadowSnapshotOf(target); after != before || target.PageCount() != 0 || target.resources != beforeResources || len(target.links) != 1 {
		t.Fatalf("invalid target preflight mutated document: before=%#v after=%#v pages=%d links=%d", before, after, target.PageCount(), len(target.links))
	}
}

func plannedLinkPDFPlan(t *testing.T) layoutengine.LayoutPlan {
	t.Helper()
	geometry, source := plannedLinkGeometry(t)
	destinationSource := layoutengine.SourceSpan{File: "links.paper",
		Start: layoutengine.SourcePosition{Offset: 2, Line: 2, Column: 1},
		End:   layoutengine.SourcePosition{Offset: 3, Line: 2, Column: 2}}
	destinations := []layoutengine.PlannedDestination{{ID: 1, Page: 2, Fragment: 2,
		Point: layoutengine.Point{X: fixed(25), Y: fixed(40)}, Source: destinationSource}}
	links := []layoutengine.PlannedLink{
		{Fragment: 1, Bounds: fixedRect(10, 10, 20, 10), Destination: 1, Source: source},
		{Fragment: 1, Bounds: fixedRect(35, 10, 30, 10), URI: "https://example.test/exact", Source: source},
	}
	plan, err := layoutengine.AttachLinks(geometry, destinations, links)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func plannedLinkGeometry(t *testing.T) (layoutengine.LayoutPlan, layoutengine.SourceSpan) {
	t.Helper()
	source := layoutengine.SourceSpan{File: "links.paper", Start: layoutengine.SourcePosition{Line: 1, Column: 1},
		End: layoutengine.SourcePosition{Offset: 1, Line: 1, Column: 2}}
	targetSource := layoutengine.SourceSpan{File: "links.paper", Start: layoutengine.SourcePosition{Offset: 2, Line: 2, Column: 1},
		End: layoutengine.SourcePosition{Offset: 3, Line: 2, Column: 2}}
	first, second := fixedRect(5, 5, 100, 40), fixedRect(20, 30, 100, 60)
	plan, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: []layoutengine.PlannedPage{
			{Number: 1, Size: layoutengine.Size{Width: fixed(200), Height: fixed(150)}, Fragments: layoutengine.IndexRange{Count: 1}},
			{Number: 2, Size: layoutengine.Size{Width: fixed(200), Height: fixed(150)}, Fragments: layoutengine.IndexRange{Start: 1, Count: 1}},
		},
		Fragments: []layoutengine.Fragment{
			{ID: 1, Node: 1, Key: "@link", Instance: "@link", Page: 1, Region: layoutengine.RegionBody,
				BorderBox: first, ContentBox: first, Source: source, Continuation: layoutengine.ContinuationWhole},
			{ID: 2, Node: 2, Key: "@target", Instance: "@target", Page: 2, Region: layoutengine.RegionBody,
				BorderBox: second, ContentBox: second, Source: targetSource, Continuation: layoutengine.ContinuationWhole},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan, source
}

func fixed(value int64) layoutengine.Fixed {
	return layoutengine.Fixed(value * layoutengine.FixedScale)
}

func fixedRect(x, y, width, height int64) layoutengine.Rect {
	return layoutengine.Rect{X: fixed(x), Y: fixed(y), Width: fixed(width), Height: fixed(height)}
}
