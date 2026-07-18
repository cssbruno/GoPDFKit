// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/inspect"
	"github.com/cssbruno/gopdfkit/layout"
)

// TestTypedCanonicalPlanPreservesSimpleCompatibilityBaselines is deliberately
// an output-level compatibility test. It compares the public lowering adapter
// with the canonical plan/painter path after both have produced real PDFs; a
// plan projection alone cannot prove that PDF text extraction remains usable.
func TestTypedCanonicalPlanPreservesSimpleCompatibilityBaselines(t *testing.T) {
	paragraph := func(text string) layout.ParagraphBlock {
		return layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: text}},
			Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 11, LineHeight: 14},
		}
	}
	heading := func(text string) layout.HeadingBlock {
		return layout.HeadingBlock{
			Level:    2,
			Segments: []layout.TextSegment{{Text: text}},
			Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 13, LineHeight: 16},
		}
	}

	fixtures := []struct {
		name       string
		document   *layout.LayoutDocument
		wantPages  int
		wantText   []string
		wantByPage [][]string
	}{
		{
			name: "single-page-flow",
			document: &layout.LayoutDocument{
				PageTemplate: compatibilityPageTemplate(),
				Body: []layout.Block{
					heading("ALPHA"),
					paragraph("BRAVO"),
					paragraph("CHARLIE"),
				},
			},
			wantPages:  1,
			wantText:   []string{"ALPHA", "BRAVO", "CHARLIE"},
			wantByPage: [][]string{{"ALPHA", "BRAVO", "CHARLIE"}},
		},
		{
			name: "explicit-page-break",
			document: &layout.LayoutDocument{
				PageTemplate: compatibilityPageTemplate(),
				Body: []layout.Block{
					paragraph("DELTA"),
					layout.PageBreakBlock{After: true},
					paragraph("ECHO"),
				},
			},
			wantPages:  2,
			wantText:   []string{"DELTA", "ECHO"},
			wantByPage: [][]string{{"DELTA"}, {"ECHO"}},
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			adapted := renderAdaptedTypedCompatibilityPDF(t, fixture.document)
			planned, planPages := renderPlannedTypedCompatibilityPDF(t, fixture.document)

			adaptedPages := compatibilityPDFPageCount(t, adapted)
			plannedPages := compatibilityPDFPageCount(t, planned)
			if adaptedPages != fixture.wantPages || plannedPages != fixture.wantPages || planPages != fixture.wantPages {
				t.Fatalf("page counts = adapted %d plan %d painted %d, want %d",
					adaptedPages, planPages, plannedPages, fixture.wantPages)
			}

			adaptedOrder := compatibilityExtractedTokenOrder(t, adapted, fixture.wantText)
			plannedOrder := compatibilityExtractedTokenOrder(t, planned, fixture.wantText)
			wantOrder := strings.Join(fixture.wantText, "|")
			if adaptedOrder != wantOrder || plannedOrder != wantOrder || plannedOrder != adaptedOrder {
				t.Fatalf("extracted order = adapted %q planned %q, want %q", adaptedOrder, plannedOrder, wantOrder)
			}
			compatibilityAssertPageText(t, adapted, fixture.wantByPage)
			compatibilityAssertPageText(t, planned, fixture.wantByPage)
		})
	}
}

func compatibilityPageTemplate() layout.PageTemplate {
	return layout.PageTemplate{Margins: layout.Spacing{Top: 12, Right: 12, Bottom: 12, Left: 12}}
}

func newTypedCompatibilityPDF() *Document {
	return MustNew(
		WithUnit(UnitPoint),
		WithCustomPageSize(Size{Wd: 220, Ht: 160}),
		WithNoCompression(),
		WithDeterministicOutput(),
	)
}

func renderAdaptedTypedCompatibilityPDF(t *testing.T, model *layout.LayoutDocument) []byte {
	t.Helper()
	pdf := newTypedCompatibilityPDF()
	pdf.WriteDocument(model)
	if err := pdf.Error(); err != nil {
		t.Fatalf("WriteDocument: %v", err)
	}
	return outputTypedCompatibilityPDF(t, pdf)
}

func renderPlannedTypedCompatibilityPDF(t *testing.T, model *layout.LayoutDocument) ([]byte, int) {
	t.Helper()
	planner := newTypedCompatibilityPDF()
	plan, err := planner.PlanLayoutDocument(model)
	if err != nil {
		t.Fatalf("PlanLayoutDocument: %v", err)
	}
	if planner.PageCount() != 0 {
		t.Fatalf("planning mutated source: page count %d", planner.PageCount())
	}
	target := newTypedCompatibilityPDF()
	written, err := target.WriteLayoutDocumentPlan(plan)
	if err != nil {
		t.Fatalf("WriteLayoutDocumentPlan: %v", err)
	}
	if written != plan.PageCount() {
		t.Fatalf("written pages = %d, plan pages = %d", written, plan.PageCount())
	}
	return outputTypedCompatibilityPDF(t, target), plan.PageCount()
}

func outputTypedCompatibilityPDF(t *testing.T, pdf *Document) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatalf("output PDF: %v", err)
	}
	return append([]byte(nil), output.Bytes()...)
}

func compatibilityPDFPageCount(t *testing.T, pdf []byte) int {
	t.Helper()
	pages, err := inspect.PageCount(pdf)
	if err != nil {
		t.Fatalf("inspect page count: %v", err)
	}
	return pages
}

func compatibilityExtractedTokenOrder(t *testing.T, pdf []byte, tokens []string) string {
	t.Helper()
	pages := compatibilityPDFPageCount(t, pdf)
	var extracted strings.Builder
	for page := 1; page <= pages; page++ {
		text, err := inspect.PageText(pdf, page)
		if err != nil {
			t.Fatalf("extract page %d: %v", page, err)
		}
		extracted.WriteString(text)
		extracted.WriteByte('\n')
	}
	content := extracted.String()
	position := 0
	order := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if count := strings.Count(content, token); count != 1 {
			t.Fatalf("extracted PDF text contains %q %d times, want exactly once:\n%q", token, count, content)
		}
		index := strings.Index(content[position:], token)
		if index < 0 {
			t.Fatalf("extracted PDF text lacks %q after byte %d:\n%q", token, position, content)
		}
		position += index + len(token)
		order = append(order, token)
	}
	return strings.Join(order, "|")
}

func compatibilityAssertPageText(t *testing.T, pdf []byte, wantByPage [][]string) {
	t.Helper()
	for pageIndex, tokens := range wantByPage {
		text, err := inspect.PageText(pdf, pageIndex+1)
		if err != nil {
			t.Fatalf("extract page %d: %v", pageIndex+1, err)
		}
		position := 0
		for _, token := range tokens {
			index := strings.Index(text[position:], token)
			if index < 0 {
				t.Fatalf("page %d text lacks %q in order: %q", pageIndex+1, token, text)
			}
			position += index + len(token)
		}
	}
}
