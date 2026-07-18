// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/inspect"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

const htmlUnifiedFlexRowFixture = `<style>
.row { display:flex; flex-direction:row; flex-wrap:nowrap; gap:6pt; align-items:center; justify-content:flex-start }
.fixed { flex:0 0 48pt; line-height:10pt }
.weighted { flex:2 1 0; line-height:20pt }
</style><div class="row"><p class="fixed">Fixed item</p><h2 class="weighted">Weighted item</h2></div>`

func TestHTMLUnifiedFlexResolvedRowColumnTracksAndSelectorFreeLowering(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedFlexRowFixture)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.cssRules) != 0 {
		t.Fatalf("selector rules crossed resolved boundary: %d", len(resolved.cssRules))
	}
	for _, token := range resolved.tokens {
		if token.Cat == 'O' && (token.Attr["class"] != "" || token.Attr["style"] != "" || token.Attr["id"] != "") {
			t.Fatalf("selector attributes crossed resolved boundary: %#v", token)
		}
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Body) != 1 {
		t.Fatalf("lowered body = %d, want one flex container", len(model.Body))
	}
	row, ok := model.Body[0].(layout.RowColumnBlock)
	if !ok || row.Direction != layout.RowDirection || row.Gap != 6 || row.CrossAlign != "center" || len(row.Items) != 2 {
		t.Fatalf("lowered row = %#v", model.Body[0])
	}
	if row.Items[0].Track != (layout.RowColumnTrack{Kind: layout.RowColumnTrackFixed, Size: 48}) ||
		row.Items[1].Track != (layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Weight: 2}) {
		t.Fatalf("lowered tracks = %#v", row.Items)
	}
	if _, ok := row.Items[0].Block.(layout.ParagraphBlock); !ok {
		t.Fatalf("first item = %#v", row.Items[0].Block)
	}
	if heading, ok := row.Items[1].Block.(layout.HeadingBlock); !ok || heading.Level != 2 {
		t.Fatalf("second item = %#v", row.Items[1].Block)
	}
}

func TestHTMLUnifiedFlexRowAndColumnPlansAreDeterministic(t *testing.T) {
	tests := []struct {
		name      string
		source    string
		direction layout.RowColumnDirection
	}{
		{name: "row", source: htmlUnifiedFlexRowFixture, direction: layout.RowDirection},
		{name: "column", source: `<style>
			.stack{display:flex;flex-direction:column;gap:4pt;align-items:stretch}
			.top{flex:0 0 24pt}.rest{flex:1 1 0}
		</style><section class="stack"><p class="top">Top</p><p class="rest">Remainder</p></section>`, direction: layout.ColumnDirection},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(test.source)
			if err != nil {
				t.Fatal(err)
			}
			planner := htmlUnifiedFlexTestPlanner()
			first, err := planner.PlanCompiledHTML(12, compiled)
			if err != nil {
				t.Fatal(err)
			}
			second, err := planner.PlanCompiledHTML(12, compiled)
			if err != nil {
				t.Fatal(err)
			}
			if first.Hash() == "" || first.Hash() != second.Hash() || first.PageCount() != 1 || planner.PageCount() != 0 {
				t.Fatalf("plans = hash %q/%q pages=%d source-pages=%d", first.Hash(), second.Hash(), first.PageCount(), planner.PageCount())
			}
			fragments := first.plan.Projection().Fragments
			if len(fragments) != 2 {
				t.Fatalf("fragments = %#v", fragments)
			}
			if test.direction == layout.RowDirection {
				if fragments[0].BorderBox.Width.Points() != 48 || fragments[1].BorderBox.X.Points() != fragments[0].BorderBox.X.Points()+54 {
					t.Fatalf("row geometry = %#v", fragments)
				}
			} else if fragments[0].BorderBox.Height.Points() != 24 || fragments[1].BorderBox.Y.Points() != fragments[0].BorderBox.Y.Points()+28 {
				t.Fatalf("column geometry = %#v", fragments)
			}
		})
	}
}

func TestHTMLUnifiedFlexResolvesDirectionalAndTwoValueGaps(t *testing.T) {
	tests := []struct {
		name, source string
		wantGap      float64
	}{
		{
			name:    "row uses column component and override",
			source:  `<div style="display:flex;gap:3pt 7pt;column-gap:11pt"><p style="flex:0 0 20pt">A</p><p style="flex:0 0 20pt">B</p></div>`,
			wantGap: 11,
		},
		{
			name:    "column uses row component and override",
			source:  `<div style="display:flex;flex-direction:column;gap:3pt 7pt;row-gap:5pt"><p style="flex:0 0 20pt">A</p><p style="flex:0 0 20pt">B</p></div>`,
			wantGap: 5,
		},
		{
			name:    "normal computes to zero for flex",
			source:  `<div style="display:flex;gap:normal"><p style="flex:0 0 20pt">A</p><p style="flex:0 0 20pt">B</p></div>`,
			wantGap: 0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(test.source)
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := htmlUnifiedFlexTestPlanner().resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
			if err != nil {
				t.Fatal(err)
			}
			model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
			if err != nil {
				t.Fatal(err)
			}
			container, ok := model.Body[0].(layout.RowColumnBlock)
			if !ok || container.Gap != test.wantGap {
				t.Fatalf("container = %#v, want gap %.2f", model.Body[0], test.wantGap)
			}
		})
	}
}

func TestHTMLUnifiedFlexJustificationHasExactPlanCursorRasterAndSemantics(t *testing.T) {
	requireDarwinRasterBaseline(t)
	tests := []struct {
		justify    string
		wantFirst  float64
		wantSecond float64
	}{
		{"flex-start", 20, 70},
		{"center", 75, 125},
		{"flex-end", 130, 180},
		{"space-between", 20, 180},
		{"space-around", 47.5, 152.5},
		{"space-evenly", 56.6669921875, 143.333984375},
	}
	for _, test := range tests {
		t.Run(test.justify, func(t *testing.T) {
			source := `<div style="display:flex;justify-content:` + test.justify + `;gap:10pt">` +
				`<p style="flex:0 0 40pt">One</p><h2 style="flex:0 0 40pt">Two</h2></div>`
			compiled, err := CompileHTML(source)
			if err != nil {
				t.Fatal(err)
			}
			planner := htmlUnifiedFlexTestPlanner()
			plan, err := planner.PlanCompiledHTML(12, compiled)
			if err != nil {
				t.Fatal(err)
			}
			projection := plan.plan.Projection()
			if len(projection.Fragments) != 2 || projection.Fragments[0].BorderBox.X.Points() != test.wantFirst ||
				projection.Fragments[1].BorderBox.X.Points() != test.wantSecond {
				t.Fatalf("%s x=%.12g,%.12g want %.12g,%.12g", test.justify,
					projection.Fragments[0].BorderBox.X.Points(), projection.Fragments[1].BorderBox.X.Points(), test.wantFirst, test.wantSecond)
			}
			if len(projection.ReadingOrder) != 2 || len(projection.SemanticFragments) != 2 {
				t.Fatalf("%s semantic projection=%+v/%+v", test.justify, projection.SemanticFragments, projection.ReadingOrder)
			}
			if test.justify == "space-evenly" {
				raster, status, err := captureCharacterizationRaster(t.Context(), "html-flex-space-evenly", plan, &characterizationRasterBudget{})
				if err != nil || status != "captured" || raster == nil || len(raster.Pages) != 1 || raster.Pages[0].PNGSHA256 == "" {
					t.Fatalf("space-evenly raster=%+v status=%q err=%v", raster, status, err)
				}
				if got := raster.Pages[0].PNGSHA256; got != "7c91853c3d84f8e3e551f65f5a638e16ad3aa2ec610272f16e8ea5dc62e258ea" {
					t.Fatalf("space-evenly raster drift: %s", got)
				}
				pdf := newHTMLFrameTestDocument(t, 160)
				pdf.SetXY(16, 42)
				html := pdf.HTMLNew()
				if err := html.WriteContext(context.Background(), 12, source); err != nil || pdf.GetY() <= 42 {
					t.Fatalf("space-evenly cursor=%.4f err=%v", pdf.GetY(), err)
				}
			}
		})
	}
}

func TestHTMLUnifiedFlexMainAndCrossAxisDimensions(t *testing.T) {
	compiled, err := CompileHTML(`<div style="display:flex;gap:4pt;align-items:center">
		<p style="width:30pt;min-width:50pt;height:20pt">Sized</p><p style="flex:1 1 0;min-width:25pt">Flexible</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	planner := htmlUnifiedFlexTestPlanner()
	resolved, err := planner.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, 12)
	if err != nil {
		t.Fatal(err)
	}
	model, err := lowerCompiledHTMLTextCohort(context.Background(), resolved, 12)
	if err != nil {
		t.Fatal(err)
	}
	row := model.Body[0].(layout.RowColumnBlock)
	if row.Items[0].Track != (layout.RowColumnTrack{Kind: layout.RowColumnTrackFlex, BasisKind: layout.RowColumnFlexBasisFixed, Basis: 30, Min: 50, Shrink: 1}) || row.Items[0].CrossSize != 20 ||
		row.Items[1].Track != (layout.RowColumnTrack{Kind: layout.RowColumnTrackFraction, Min: 25, Weight: 1}) {
		t.Fatalf("dimension lowering=%+v", row.Items)
	}

	columnSource := `<div style="display:flex;flex-direction:column;justify-content:center;gap:4pt;align-items:stretch">` +
		`<p style="height:20pt;width:60pt;align-self:center">One</p><p style="height:20pt;width:40pt;align-self:flex-end">Two</p></div>`
	column, err := CompileHTML(columnSource)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planner.PlanCompiledHTML(12, column)
	if err != nil {
		t.Fatal(err)
	}
	fragments := plan.plan.Projection().Fragments
	if len(fragments) != 2 || fragments[0].BorderBox != (layoutengine.Rect{X: 90 * 1024, Y: 58 * 1024, Width: 60 * 1024, Height: 20 * 1024}) ||
		fragments[1].BorderBox != (layoutengine.Rect{X: 180 * 1024, Y: 82 * 1024, Width: 40 * 1024, Height: 20 * 1024}) {
		t.Fatalf("column sizing/alignment=%+v", fragments)
	}
}

func TestHTMLUnifiedFlexCapabilityScanRejectsWholeFragment(t *testing.T) {
	tests := []struct {
		name, source, diagnostic string
	}{
		{"wrap-missing-cross-size", `<div style="display:flex;flex-wrap:wrap"><p>A</p></div>`, "definite positive height"},
		{"wrap-value", `<div style="display:flex;flex-wrap:balance"><p>A</p></div>`, "flex-wrap"},
		{"justify-unsupported", `<div style="display:flex;justify-content:stretch"><p>A</p></div>`, "justify-content"},
		{"baseline", `<div style="display:flex;align-items:baseline"><p>A</p></div>`, "align-items"},
		{"direct-text", `<div style="display:flex">outside<p>A</p></div>`, "direct content"},
		{"orphan-item", `<p style="flex:1">A</p>`, "direct supported flex child"},
		{"late-unsupported", `<div style="display:flex"><p style="flex:1">A</p><p style="flex:1;order:2">B</p></div>`, `property "order"`},
		{"invalid-gap-components", `<div style="display:flex;gap:1pt 2pt 3pt"><p>A</p></div>`, "one or two"},
		{"percentage-column-gap", `<div style="display:flex;column-gap:10%"><p>A</p></div>`, "column-gap"},
		{"negative-gap", `<div style="display:flex;gap:-1pt"><p>A</p></div>`, "non-negative"},
		{"unrepresentable-shrink", `<div style="display:flex;gap:10pt"><p style="flex:0 0 120pt">A</p><p style="flex:0 0 120pt">B</p></div>`, "resolve row tracks"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compiled, err := CompileHTML(test.source)
			if err != nil {
				t.Fatal(err)
			}
			planner := htmlUnifiedFlexTestPlanner()
			plan, err := planner.PlanCompiledHTML(12, compiled)
			if !errors.Is(err, ErrHTMLPlanUnsupported) || !strings.Contains(err.Error(), test.diagnostic) || plan.Hash() != "" || planner.PageCount() != 0 {
				t.Fatalf("plan=%#v pages=%d err=%v, want diagnostic %q", plan, planner.PageCount(), err, test.diagnostic)
			}
		})
	}
}

func TestHTMLUnifiedFlexFragmentCursorAndDeterministicPDF(t *testing.T) {
	render := func() ([]byte, float64) {
		pdf := newHTMLFrameTestDocument(t, 160)
		pdf.SetXY(16, 42)
		html := pdf.HTMLNew()
		if err := html.WriteContext(context.Background(), 12, htmlUnifiedFlexRowFixture); err != nil {
			t.Fatal(err)
		}
		if pdf.PageNo() != 1 || pdf.GetX() != 16 || pdf.GetY() <= 42 {
			t.Fatalf("flex exit cursor = page %d %.4f,%.4f", pdf.PageNo(), pdf.GetX(), pdf.GetY())
		}
		var out bytes.Buffer
		if err := pdf.OutputWithOptions(&out, OutputOptions{Deterministic: true}); err != nil {
			t.Fatal(err)
		}
		return out.Bytes(), pdf.GetY()
	}
	first, firstY := render()
	second, secondY := render()
	if firstY != secondY || !bytes.Equal(first, second) {
		t.Fatalf("deterministic flex render differs: y %.4f/%.4f bytes %d/%d", firstY, secondY, len(first), len(second))
	}
	text, err := inspect.PageTextContext(context.Background(), first, 1)
	if err != nil {
		t.Fatal(err)
	}
	text = strings.ReplaceAll(text, "\x00", "")
	for _, want := range []string{"Fixed item", "Weighted item"} {
		if !strings.Contains(text, want) {
			t.Fatalf("PDF text lacks %q: %q", want, text)
		}
	}
}

func TestHTMLUnifiedFlexCompiledFragmentConcurrentReuse(t *testing.T) {
	compiled, err := CompileHTML(htmlUnifiedFlexRowFixture)
	if err != nil {
		t.Fatal(err)
	}
	const workers = 16
	hashes := make(chan string, workers)
	errorsFound := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			plan, planErr := htmlUnifiedFlexTestPlanner().PlanCompiledHTMLContext(context.Background(), 12, compiled)
			if planErr != nil {
				errorsFound <- planErr
				return
			}
			hashes <- plan.Hash()
		}()
	}
	wait.Wait()
	close(hashes)
	close(errorsFound)
	for found := range errorsFound {
		t.Fatal(found)
	}
	var expected string
	for hash := range hashes {
		if hash == "" {
			t.Fatal("concurrent plan hash is empty")
		}
		if expected == "" {
			expected = hash
		} else if hash != expected {
			t.Fatalf("concurrent plan hash = %q, want %q", hash, expected)
		}
	}
}

func htmlUnifiedFlexTestPlanner() *Document {
	planner := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	planner.SetMargins(20, 20, 20)
	planner.SetAutoPageBreak(true, 20)
	return planner
}
