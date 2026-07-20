// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func paginationTestDocument(t *testing.T, height float64, options ...Option) *Document {
	t.Helper()
	base := []Option{WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 140, Ht: height}), WithNoCompression()}
	base = append(base, options...)
	doc := MustNew(base...)
	doc.SetMargins(10, 10, 10)
	doc.SetAutoPageBreak(true, 10)
	return doc
}

func paginationParagraph(text string, box layout.BoxStyle) layout.ParagraphBlock {
	return layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: text}},
		Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10},
		Box:      box,
	}
}

func TestTypedKeepTogetherAndKeepWithNextProduceConstraintBreakEvidence(t *testing.T) {
	tests := []struct {
		name string
		body []layout.Block
	}{
		{
			name: "container keep together",
			body: []layout.Block{
				paginationParagraph("fill-1\nfill-2\nfill-3\nfill-4", layout.BoxStyle{KeepTogether: true}),
				layout.SectionBlock{Box: layout.BoxStyle{KeepTogether: true}, Blocks: []layout.Block{
					paginationParagraph("group-a", layout.BoxStyle{}),
					paginationParagraph("group-b", layout.BoxStyle{}),
				}},
			},
		},
		{
			name: "keep with next chain",
			body: []layout.Block{
				paginationParagraph("fill-1\nfill-2\nfill-3\nfill-4", layout.BoxStyle{KeepTogether: true}),
				paginationParagraph("chain-a", layout.BoxStyle{KeepWithNext: true}),
				paginationParagraph("chain-b", layout.BoxStyle{KeepWithNext: true}),
				paginationParagraph("chain-c", layout.BoxStyle{}),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			planner := paginationTestDocument(t, 70) // 50pt body: filler leaves 10pt.
			plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: test.body})
			if err != nil {
				t.Fatal(err)
			}
			projection := plan.plan.Projection()
			if len(projection.Pages) != 2 || len(projection.Breaks) == 0 {
				t.Fatalf("pages/breaks = %d/%+v, want two pages and break evidence", len(projection.Pages), projection.Breaks)
			}
			decision := projection.Breaks[0]
			if decision.Reason != layoutengine.BreakPaginationConstraint || decision.Required <= decision.Available ||
				decision.Preceding == 0 || decision.Triggering == 0 {
				t.Fatalf("constraint break = %+v", decision)
			}
			if projection.Fragments[0].Page != 1 || projection.Fragments[1].Page != 2 {
				t.Fatalf("fragment pages = %+v", projection.Fragments)
			}
			repeated, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: test.body})
			if err != nil || repeated.Hash() != plan.Hash() {
				t.Fatalf("repeated plan hash = %q, %v; want %q", repeated.Hash(), err, plan.Hash())
			}
			painted := paginationTestDocument(t, 70)
			pages, err := painted.WriteLayoutDocumentPlan(plan)
			if err != nil || pages != 2 {
				t.Fatalf("paint = %d, %v", pages, err)
			}
			var pdf bytes.Buffer
			if err := painted.Output(&pdf); err != nil || pdf.Len() == 0 {
				t.Fatalf("PDF = %d bytes, %v", pdf.Len(), err)
			}
		})
	}
}

func TestTypedWidowsAndOrphansArePlannedAndRelaxedDeterministically(t *testing.T) {
	planner := paginationTestDocument(t, 60) // 40pt body; widows retain two lines on page two.
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		paginationParagraph("one\ntwo\nthree\nfour\nfive", layout.BoxStyle{Orphans: 2, Widows: 2}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	projection := plan.plan.Projection()
	if len(projection.Pages) != 2 || len(projection.Fragments) != 2 ||
		projection.Pages[0].Lines.Count != 3 || projection.Pages[1].Lines.Count != 2 {
		t.Fatalf("widow/orphan pages = %+v fragments=%+v", projection.Pages, projection.Fragments)
	}
	if len(projection.Breaks) != 1 || projection.Breaks[0].Reason != layoutengine.BreakPaginationConstraint {
		t.Fatalf("widow/orphan breaks = %+v", projection.Breaks)
	}

	relaxed, err := paginationTestDocument(t, 50).PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{
		paginationParagraph("one\ntwo\nthree\nfour\nfive", layout.BoxStyle{Orphans: 4, Widows: 4}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diagnostic := range relaxed.plan.Projection().Diagnostics {
		found = found || diagnostic.Code == layoutengine.DiagnosticParagraphConstraintRelaxed
	}
	if !found {
		t.Fatalf("relaxation diagnostics = %+v", relaxed.plan.Projection().Diagnostics)
	}
}

func TestTypedOversizedKeepTerminatesWithDiagnosticAndLimits(t *testing.T) {
	body := []layout.Block{layout.SectionBlock{Box: layout.BoxStyle{KeepTogether: true}, Blocks: []layout.Block{
		paginationParagraph("a1\na2", layout.BoxStyle{}),
		paginationParagraph("b1\nb2", layout.BoxStyle{}),
	}}}
	plan, err := paginationTestDocument(t, 50).PlanLayoutDocument(&layout.LayoutDocument{Body: body})
	if err != nil || plan.PageCount() != 2 {
		t.Fatalf("oversized keep = pages %d, %v", plan.PageCount(), err)
	}
	found := false
	for _, diagnostic := range plan.plan.Projection().Diagnostics {
		found = found || diagnostic.Code == layoutengine.DiagnosticKeepTooLarge
	}
	if !found {
		t.Fatalf("oversized keep diagnostics = %+v", plan.plan.Projection().Diagnostics)
	}

	limited := paginationTestDocument(t, 50, WithLimits(Limits{MaxPages: 1}))
	if _, err := limited.PlanLayoutDocument(&layout.LayoutDocument{Body: body}); !errors.Is(err, ErrPageLimitExceeded) {
		t.Fatalf("page-limited plan error = %v, want ErrPageLimitExceeded", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := paginationTestDocument(t, 50).PlanLayoutDocumentContext(canceled, &layout.LayoutDocument{Body: body}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled plan error = %v", err)
	}
}

func TestTypedContainerPaginationPoliciesLowerAcrossSupportedKinds(t *testing.T) {
	sharedPagination := layout.BoxStyle{KeepWithNext: true, Orphans: 2, Widows: 2}
	blocks := []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "shared policy"}},
			Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10}, BoxRef: &sharedPagination},
		layout.ListBlock{Box: layout.BoxStyle{KeepTogether: true, KeepWithNext: true}, Items: []layout.ListItem{
			{Blocks: []layout.Block{paginationParagraph("first", layout.BoxStyle{})}},
			{Blocks: []layout.Block{paginationParagraph("second", layout.BoxStyle{})}},
		}},
		layout.SectionBlock{Title: "Section", KeepTitleWithBody: true, Box: layout.BoxStyle{KeepTogether: true}, Blocks: []layout.Block{paginationParagraph("body", layout.BoxStyle{})}},
		layout.ClauseBlock{Number: "1.", Title: "Clause", KeepTogether: true, Blocks: []layout.Block{paginationParagraph("clause body", layout.BoxStyle{})}},
		layout.NoteBoxBlock{Title: "Note", Box: layout.BoxStyle{KeepWithNext: true, Orphans: 2, Widows: 2}, Body: []layout.Block{paginationParagraph("note one\nnote two", layout.BoxStyle{})}},
		layout.MetadataGridBlock{Columns: 1, Box: layout.BoxStyle{KeepTogether: true}, Fields: []layout.MetadataField{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}},
		layout.SignatureRowBlock{Box: layout.BoxStyle{KeepWithNext: true}, Columns: []layout.SignatureColumn{{Name: "Signer"}}},
		paginationParagraph("tail", layout.BoxStyle{}),
	}
	planner := paginationTestDocument(t, 240)
	plan, err := planner.PlanLayoutDocument(&layout.LayoutDocument{Body: blocks, Signature: &layout.SignatureBlock{
		KeepTogether: true, Rows: []layout.SignatureRowBlock{{Columns: []layout.SignatureColumn{{Name: "Final signer"}}}},
	}})
	if err != nil || plan.PageCount() == 0 {
		t.Fatalf("container policies = pages %d, %v", plan.PageCount(), err)
	}
}
