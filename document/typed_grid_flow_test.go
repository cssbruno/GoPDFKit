// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestTypedMetadataAndSignatureMultiColumnGeometryMixedPlanCaptureAndPDF(t *testing.T) {
	planner := paginationTestDocument(t, 150)
	doc := &layout.LayoutDocument{Body: []layout.Block{
		paginationParagraph("before", layout.BoxStyle{}),
		layout.MetadataGridBlock{Columns: 3, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}, Fields: []layout.MetadataField{
			{Label: "A", Value: "one"}, {Label: "B", Value: "two lines\nsecond"}, {Label: "C", Value: "three"},
			{Label: "D", Value: "four"}, {Label: "E", Value: "five"},
		}},
		layout.SignatureRowBlock{Columns: []layout.SignatureColumn{
			{Label: "Left", Name: "Alice", Width: 24},
			{Label: "Flexible", Name: "Bob"},
			{Label: "Right", Name: "Carol", Width: 36},
		}},
		paginationParagraph("after", layout.BoxStyle{}),
	}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil || plan.PageCount() != 1 || planner.PageCount() != 0 {
		t.Fatalf("plan = pages %d source pages %d, %v", plan.PageCount(), planner.PageCount(), err)
	}
	projection := plan.plan.Projection()
	if got := len(projection.GridTracks); got != 12 {
		t.Fatalf("retained grid tracks = %d, want 12", got)
	}
	for group := uint32(1); group <= 3; group++ {
		base := int((group - 1) * 4)
		for column := uint32(0); column < 3; column++ {
			track := projection.GridTracks[base+int(column)]
			if track.Group != group || track.Page != 1 || track.Region != layoutengine.RegionBody || track.Axis != layoutengine.GridTrackColumn || track.Index != column {
				t.Fatalf("grid track group %d column %d = %+v", group, column, track)
			}
			if column < 2 && track.GapAfter != layoutengine.Fixed(8*1024) {
				t.Fatalf("grid track group %d column %d gap = %d", group, column, track.GapAfter)
			}
		}
		rowTrack := projection.GridTracks[base+3]
		if rowTrack.Group != group || rowTrack.Axis != layoutengine.GridTrackRow || rowTrack.Index != 0 || rowTrack.GapAfter != 0 {
			t.Fatalf("grid track group %d row = %+v", group, rowTrack)
		}
	}
	// The incomplete second metadata row still retains all three authored
	// columns, including the empty trailing track needed by inspection tools.
	if projection.GridTracks[6].Bounds.X != projection.GridTracks[2].Bounds.X || projection.GridTracks[6].Bounds.Width != projection.GridTracks[2].Bounds.Width {
		t.Fatalf("incomplete metadata row lost trailing column: first=%+v second=%+v", projection.GridTracks[2], projection.GridTracks[6])
	}
	fragmentsByText := make(map[string]layoutengine.Fragment)
	for _, association := range projection.SemanticFragments {
		node := projection.SemanticNodes[association.Semantic-1]
		if node.Role == layoutengine.SemanticRoleCell {
			fragmentsByText[node.Attributes.ActualText] = projection.Fragments[association.Fragment-1]
		}
	}
	a, b, c := fragmentsByText["A: one"], fragmentsByText["B: two lines second"], fragmentsByText["C: three"]
	d, e := fragmentsByText["D: four"], fragmentsByText["E: five"]
	if !a.ID.Valid() || !b.ID.Valid() || !c.ID.Valid() || !d.ID.Valid() || !e.ID.Valid() {
		t.Fatalf("metadata semantic fragments = %+v", fragmentsByText)
	}
	gap8, _ := layoutengine.FixedFromPoints(8)
	if a.BorderBox.Y != b.BorderBox.Y || b.BorderBox.Y != c.BorderBox.Y ||
		a.BorderBox.Height != b.BorderBox.Height || b.BorderBox.Height != c.BorderBox.Height ||
		b.BorderBox.Height <= layoutengine.Fixed(10*1024) {
		t.Fatalf("first metadata row geometry = A %+v B %+v C %+v", a.BorderBox, b.BorderBox, c.BorderBox)
	}
	aRight, _ := a.BorderBox.Right()
	bRight, _ := b.BorderBox.Right()
	if b.BorderBox.X-aRight != gap8 || c.BorderBox.X-bRight != gap8 || d.BorderBox.X != a.BorderBox.X || e.BorderBox.X != b.BorderBox.X || d.BorderBox.Width != a.BorderBox.Width {
		t.Fatalf("metadata columns/gaps = A %+v B %+v C %+v D %+v E %+v", a.BorderBox, b.BorderBox, c.BorderBox, d.BorderBox, e.BorderBox)
	}
	left, flex, right := fragmentsByText["Left; Alice"], fragmentsByText["Flexible; Bob"], fragmentsByText["Right; Carol"]
	wantLeft, _ := layoutengine.FixedFromPoints(24)
	wantRight, _ := layoutengine.FixedFromPoints(36)
	gap8Signature, _ := layoutengine.FixedFromPoints(8)
	leftRight, _ := left.BorderBox.Right()
	flexRight, _ := flex.BorderBox.Right()
	if left.BorderBox.Width != wantLeft || right.BorderBox.Width != wantRight || flex.BorderBox.Width <= 0 ||
		flex.BorderBox.X-leftRight != gap8Signature || right.BorderBox.X-flexRight != gap8Signature ||
		left.BorderBox.Y != flex.BorderBox.Y || flex.BorderBox.Y != right.BorderBox.Y {
		t.Fatalf("signature geometry = left %+v flex %+v right %+v", left.BorderBox, flex.BorderBox, right.BorderBox)
	}
	for page, readingIndex := uint32(1), uint32(0); readingIndex < uint32(len(projection.ReadingOrder)); readingIndex++ {
		occurrence := projection.ReadingOrder[readingIndex]
		if occurrence.Page != page || occurrence.ReadingIndex != readingIndex {
			t.Fatalf("reading order[%d] = %+v", readingIndex, occurrence)
		}
	}
	capture, err := plan.CaptureDisplayPage(1)
	if err != nil || !bytes.Contains(capture.SVG(), []byte(`data-format="display-plan-preview"`)) {
		t.Fatalf("capture = %d bytes, %v", len(capture.SVG()), err)
	}
	target := paginationTestDocument(t, 150)
	pages, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil || pages != 1 {
		t.Fatalf("paint = %d, %v", pages, err)
	}
	var output bytes.Buffer
	if err := target.Output(&output); err != nil || output.Len() == 0 {
		t.Fatalf("PDF = %d, %v", output.Len(), err)
	}
}

func TestTypedMetadataKeepTogetherMovesWholeMultiRowGrid(t *testing.T) {
	planner := paginationTestDocument(t, 70) // 50pt body.
	doc := &layout.LayoutDocument{Body: []layout.Block{
		paginationParagraph("fill-1\nfill-2\nfill-3\nfill-4", layout.BoxStyle{KeepTogether: true}),
		layout.MetadataGridBlock{Columns: 2, Box: layout.BoxStyle{KeepTogether: true},
			Style:  layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10},
			Fields: []layout.MetadataField{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}, {Label: "C", Value: "3"}, {Label: "D", Value: "4"}}},
	}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Pages) != 2 || len(projection.Breaks) == 0 || projection.Breaks[0].Reason != layoutengine.BreakPaginationConstraint {
		t.Fatalf("pages/breaks = %d/%+v", len(projection.Pages), projection.Breaks)
	}
	for _, association := range projection.SemanticFragments {
		node := projection.SemanticNodes[association.Semantic-1]
		if node.Role == layoutengine.SemanticRoleCell && association.Page != 2 {
			t.Fatalf("cell %q stayed on page %d", node.Attributes.ActualText, association.Page)
		}
	}
}

func TestTypedMetadataAndSignatureCustomColumnGapsUseDocumentUnits(t *testing.T) {
	planner := paginationTestDocument(t, 150)
	doc := &layout.LayoutDocument{Body: []layout.Block{
		layout.MetadataGridBlock{Columns: 2, Gap: 3.5, Fields: []layout.MetadataField{
			{Label: "A", Value: "one"}, {Label: "B", Value: "two"},
		}},
		layout.SignatureRowBlock{Gap: 5.25, Columns: []layout.SignatureColumn{
			{Name: "Left"}, {Name: "Right"},
		}},
	}}
	plan, err := planner.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	fragments := make(map[string]layoutengine.Fragment)
	for _, association := range projection.SemanticFragments {
		node := projection.SemanticNodes[association.Semantic-1]
		if node.Role == layoutengine.SemanticRoleCell {
			fragments[node.Attributes.ActualText] = projection.Fragments[association.Fragment-1]
		}
	}
	assertGap := func(left, right string, wantPoints float64) {
		t.Helper()
		leftBox, rightBox := fragments[left].BorderBox, fragments[right].BorderBox
		leftEdge, edgeErr := leftBox.Right()
		want, fixedErr := layoutengine.FixedFromPoints(wantPoints)
		if edgeErr != nil || fixedErr != nil || rightBox.X-leftEdge != want {
			t.Fatalf("gap %q -> %q = %v, want %v (left=%+v right=%+v)", left, right,
				(rightBox.X - leftEdge), want, leftBox, rightBox)
		}
	}
	assertGap("A: one", "B: two", 3.5)
	assertGap("Left", "Right", 5.25)

	defaultPlanner := paginationTestDocument(t, 150)
	defaultDoc := *doc
	defaultDoc.Body = []layout.Block{
		layout.MetadataGridBlock{Columns: 2, Fields: []layout.MetadataField{{Label: "A", Value: "one"}, {Label: "B", Value: "two"}}},
		layout.SignatureRowBlock{Columns: []layout.SignatureColumn{{Name: "Left"}, {Name: "Right"}}},
	}
	defaultPlan, err := defaultPlanner.PlanLayoutDocument(&defaultDoc)
	if err != nil || defaultPlan.Hash() == plan.Hash() {
		t.Fatalf("default/custom plan hashes = %q/%q, error %v", defaultPlan.Hash(), plan.Hash(), err)
	}
}

func TestTypedMultiColumnGeometryRejectsInvalidAndHonorsCancellation(t *testing.T) {
	tests := []struct {
		name  string
		block layout.Block
		want  string
	}{
		{"metadata columns", layout.MetadataGridBlock{Columns: 65, Fields: []layout.MetadataField{{Label: "A"}}}, "exceeds the exact planner limit"},
		{"metadata negative gap", layout.MetadataGridBlock{Columns: 2, Gap: -1, Fields: []layout.MetadataField{{Label: "A"}}}, "gap must be finite and non-negative"},
		{"metadata nan gap", layout.MetadataGridBlock{Columns: 2, Gap: math.NaN(), Fields: []layout.MetadataField{{Label: "A"}}}, "gap must be finite and non-negative"},
		{"metadata consuming gap", layout.MetadataGridBlock{Columns: 2, Gap: 200, Fields: []layout.MetadataField{{Label: "A"}, {Label: "B"}}}, "gaps leave no column width"},
		{"signature columns", layout.SignatureRowBlock{Columns: make([]layout.SignatureColumn, 65)}, "exceeds the exact planner limit"},
		{"signature negative gap", layout.SignatureRowBlock{Gap: -1, Columns: []layout.SignatureColumn{{Name: "A"}}}, "gap must be finite and non-negative"},
		{"signature nan gap", layout.SignatureRowBlock{Gap: math.NaN(), Columns: []layout.SignatureColumn{{Name: "A"}}}, "gap must be finite and non-negative"},
		{"signature consuming gap", layout.SignatureRowBlock{Gap: 200, Columns: []layout.SignatureColumn{{Name: "A"}, {Name: "B"}}}, "gaps leave no column width"},
		{"negative width", layout.SignatureRowBlock{Columns: []layout.SignatureColumn{{Name: "A", Width: -1}}}, "finite and non-negative"},
		{"nan width", layout.SignatureRowBlock{Columns: []layout.SignatureColumn{{Name: "A", Width: math.NaN()}}}, "finite and non-negative"},
		{"width overflow", layout.SignatureRowBlock{Columns: []layout.SignatureColumn{{Name: "A", Width: 100}, {Name: "B", Width: 100}}}, "exceed the body width"},
		{"oversized row", layout.MetadataGridBlock{Columns: 2, Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}, Fields: []layout.MetadataField{{Label: "A", Value: strings.Repeat("line\n", 8)}, {Label: "B", Value: "x"}}}, "cannot paginate internally"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planner := paginationTestDocument(t, 70)
			plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{test.block}})
			if err == nil || !strings.Contains(err.Error(), test.want) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("plan = pages %d hash %q error %v, want %q", plan.PageCount(), plan.Hash(), err, test.want)
			}
		})
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	planner := paginationTestDocument(t, 100)
	fields := make([]layout.MetadataField, 10_000)
	if _, err := planner.PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{Body: []layout.Block{layout.MetadataGridBlock{Columns: 4, Fields: fields}}}); err != context.Canceled {
		t.Fatalf("canceled error = %v", err)
	}
}

func TestTypedSignatureRowsPaintLegacyLinesAndPermitBlankPlaceholder(t *testing.T) {
	planner := paginationTestDocument(t, 100)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.SignatureRowBlock{},
		layout.SignatureRowBlock{Columns: []layout.SignatureColumn{{Label: "Signer", Name: "Alice"}, {Name: "Bob"}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Strokes) != 3 || len(projection.Paths) != 3 {
		t.Fatalf("signature paths/strokes = %d/%d, want 3/3", len(projection.Paths), len(projection.Strokes))
	}
	for index, stroke := range projection.Strokes {
		if stroke.Fragment == 0 || stroke.Width <= 0 || !stroke.Color.Set || len(projection.Paths[stroke.Path].Segments) != 2 {
			t.Fatalf("signature stroke[%d] = %+v path=%+v", index, stroke, projection.Paths[stroke.Path])
		}
	}
	var artifact, alice, bob bool
	for _, node := range projection.SemanticNodes {
		artifact = artifact || node.Role == layoutengine.SemanticRoleArtifact
		alice = alice || node.Attributes.ActualText == "Signer; Alice"
		bob = bob || node.Attributes.ActualText == "Bob"
	}
	if !artifact || !alice || !bob {
		t.Fatalf("signature semantics artifact/alice/bob = %t/%t/%t", artifact, alice, bob)
	}
}
