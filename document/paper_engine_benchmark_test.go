// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
)

const paperEngineBenchmarkParagraphs = 48

var (
	paperEngineBenchmarkPlanSink  LayoutDocumentPlan
	paperEngineBenchmarkBytesSink []byte
	paperEngineBenchmarkPagesSink int
)

// BenchmarkPaperEnginePlannerTyped measures only typed lowering, measurement,
// wrapping, pagination, display-list construction, and plan hashing. The model
// is immutable and is shared by every iteration.
func BenchmarkPaperEnginePlannerTyped(b *testing.B) {
	model := paperEngineBenchmarkTypedFixture()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		planner := paperEngineBenchmarkDocument()
		plan, err := planner.PlanLayoutDocument(model)
		if err != nil {
			b.Fatal(err)
		}
		paperEngineBenchmarkPlanSink = plan
	}
}

// BenchmarkPaperEnginePainterTyped starts from one immutable retained plan. It
// deliberately excludes planning and PDF serialization, while including the
// production painter's read-only preflight and positioned-command replay into
// a fresh document.
func BenchmarkPaperEnginePainterTyped(b *testing.B) {
	plan := paperEngineBenchmarkTypedPlan(b)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		target := paperEngineBenchmarkDocument()
		pages, err := target.WriteLayoutDocumentPlan(plan)
		if err != nil {
			b.Fatal(err)
		}
		paperEngineBenchmarkPagesSink = pages
	}
}

// BenchmarkPaperEngineEndToEndTyped includes planning, painting, and
// deterministic PDF serialization from the typed frontend.
func BenchmarkPaperEngineEndToEndTyped(b *testing.B) {
	model := paperEngineBenchmarkTypedFixture()
	benchmarkPaperEngineEndToEnd(b, func() ([]byte, int, error) {
		planner := paperEngineBenchmarkDocument()
		plan, err := planner.PlanLayoutDocument(model)
		if err != nil {
			return nil, 0, err
		}
		target := paperEngineBenchmarkDocument()
		pages, err := target.WriteLayoutDocumentPlan(plan)
		if err != nil {
			return nil, 0, err
		}
		output, err := paperEngineBenchmarkOutput(target)
		return output, pages, err
	})
}

// BenchmarkPaperEngineEndToEndCompiledHTML includes whole-fragment HTML
// adaptation, unified planning, painting, and deterministic serialization. HTML
// tokenization is excluded because the fixture is a reusable CompiledHTML.
func BenchmarkPaperEngineEndToEndCompiledHTML(b *testing.B) {
	compiled, err := CompileHTML(paperEngineBenchmarkHTMLFixture())
	if err != nil {
		b.Fatal(err)
	}
	benchmarkPaperEngineEndToEnd(b, func() ([]byte, int, error) {
		planner := paperEngineBenchmarkDocument()
		plan, err := planner.PlanCompiledHTML(12, compiled)
		if err != nil {
			return nil, 0, err
		}
		target := paperEngineBenchmarkDocument()
		pages, err := target.WriteLayoutDocumentPlan(plan)
		if err != nil {
			return nil, 0, err
		}
		output, err := paperEngineBenchmarkOutput(target)
		return output, pages, err
	})
}

// BenchmarkPaperEngineEndToEndPaper includes parsing, semantic compilation,
// unified planning, painting, and deterministic serialization.
func BenchmarkPaperEngineEndToEndPaper(b *testing.B) {
	source := paperEngineBenchmarkPaperFixture()
	benchmarkPaperEngineEndToEnd(b, func() ([]byte, int, error) {
		plan, result, err := PlanPaper("benchmark.paper", source)
		if err != nil {
			return nil, 0, fmt.Errorf("plan .paper: %w (%v)", err, result.Diagnostics)
		}
		target := paperEngineBenchmarkDocument()
		painted, err := target.WritePaperPlan(plan)
		if err != nil {
			return nil, 0, fmt.Errorf("paint .paper: %w (%v)", err, painted.Diagnostics)
		}
		output, err := paperEngineBenchmarkOutput(target)
		return output, painted.Pages, err
	})
}

// BenchmarkPaperEngineWarmCompiledPaper measures the Stage 6 warm path: the
// source has already been parsed, compiled, measured, and planned into an
// immutable PaperPlan. Each operation paints that retained plan into a fresh
// document and serializes deterministic PDF bytes. No source parsing,
// semantic compilation, measurement, wrapping, or pagination occurs here.
func BenchmarkPaperEngineWarmCompiledPaper(b *testing.B) {
	plan, result, err := PlanPaper("benchmark.paper", paperEngineBenchmarkPaperFixture())
	if err != nil {
		b.Fatalf("compile warm .paper fixture: %v (%v)", err, result.Diagnostics)
	}
	benchmarkPaperEngineEndToEnd(b, func() ([]byte, int, error) {
		target := paperEngineBenchmarkDocument()
		painted, err := target.WritePaperPlan(plan)
		if err != nil {
			return nil, 0, fmt.Errorf("paint warm .paper: %w (%v)", err, painted.Diagnostics)
		}
		output, err := paperEngineBenchmarkOutput(target)
		return output, painted.Pages, err
	})
}

// BenchmarkPaperEngineConcurrentPlanWrite16 measures immutable retained-plan
// reuse by exactly sixteen persistent workers. One benchmark operation is one
// synchronized batch of sixteen independent document writes.
func BenchmarkPaperEngineConcurrentPlanWrite16(b *testing.B) {
	const workers = 16
	plan := paperEngineBenchmarkTypedPlan(b)
	jobs := make(chan struct{}, workers)
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for range workers {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for range jobs {
				target := paperEngineBenchmarkDocument()
				_, err := target.WriteLayoutDocumentPlan(plan)
				results <- err
			}
		}()
	}
	b.Cleanup(func() {
		close(jobs)
		wait.Wait()
	})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for range workers {
			jobs <- struct{}{}
		}
		for range workers {
			if err := <-results; err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkPaperEngineTableLarge128(b *testing.B) {
	benchmarkPaperEngineTablePlan(b, 128, 4)
}

func BenchmarkPaperEngineTableWide32(b *testing.B) {
	benchmarkPaperEngineTablePlan(b, 8, 32)
}

func benchmarkPaperEngineTablePlan(b *testing.B, rows, columns int) {
	b.Helper()
	model := paperEngineBenchmarkTableFixture(rows, columns)
	planner := paperEngineBenchmarkTableDocument(columns)
	if plan, err := planner.PlanLayoutDocument(model); err != nil || plan.PageCount() == 0 {
		b.Fatalf("validate %dx%d table benchmark = pages %d, %v", rows, columns, plan.PageCount(), err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		plan, err := paperEngineBenchmarkTableDocument(columns).PlanLayoutDocument(model)
		if err != nil {
			b.Fatal(err)
		}
		paperEngineBenchmarkPlanSink = plan
	}
}

func benchmarkPaperEngineEndToEnd(b *testing.B, render func() ([]byte, int, error)) {
	b.Helper()
	// Validate the fixture and retain one result before the clock starts. This
	// catches accidental zero-work benchmarks without charging validation to the
	// measured pipeline.
	output, pages, err := render()
	if err != nil {
		b.Fatal(err)
	}
	if len(output) == 0 || pages == 0 {
		b.Fatalf("fixture produced %d bytes and %d pages", len(output), pages)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		output, pages, err = render()
		if err != nil {
			b.Fatal(err)
		}
		paperEngineBenchmarkBytesSink = output
		paperEngineBenchmarkPagesSink = pages
	}
}

func paperEngineBenchmarkTypedPlan(tb testing.TB) LayoutDocumentPlan {
	tb.Helper()
	plan, err := paperEngineBenchmarkDocument().PlanLayoutDocument(paperEngineBenchmarkTypedFixture())
	if err != nil {
		tb.Fatal(err)
	}
	return plan
}

func paperEngineBenchmarkDocument() *Document {
	return MustNew(
		WithUnit(UnitPoint),
		WithCustomPageSize(Size{Wd: 220, Ht: 300}),
		WithNoCompression(),
		WithDeterministicOutput(),
	)
}

func paperEngineBenchmarkTableDocument(columns int) *Document {
	if columns <= 16 {
		return paperEngineBenchmarkDocument()
	}
	return MustNew(
		WithUnit(UnitPoint),
		WithCustomPageSize(Size{Wd: 640, Ht: 300}),
		WithNoCompression(),
		WithDeterministicOutput(),
	)
}

func paperEngineBenchmarkOutput(pdf *Document) ([]byte, error) {
	var output bytes.Buffer
	err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true})
	return output.Bytes(), err
}

func paperEngineBenchmarkTypedFixture() *layout.LayoutDocument {
	body := make([]layout.Block, 0, paperEngineBenchmarkParagraphs+1)
	body = append(body, layout.HeadingBlock{
		Level: 1, Segments: []layout.TextSegment{{Text: "Quarterly operating report"}},
		Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 14, LineHeight: 16, Bold: true},
	})
	for index := 0; index < paperEngineBenchmarkParagraphs; index++ {
		body = append(body, paperEngineBenchmarkParagraph(index))
	}
	return &layout.LayoutDocument{
		Title:        "Paper Engine benchmark",
		Language:     "en",
		PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: 14, Right: 14, Bottom: 14, Left: 14}},
		Body:         body,
	}
}

func paperEngineBenchmarkTableFixture(rows, columns int) *layout.LayoutDocument {
	style := layout.TextStyle{FontFamily: "Helvetica", FontSize: 6, LineHeight: 7}
	margin := 14.0
	contentWidth := 192.0
	if columns > 16 {
		contentWidth = 612
	}
	tracks := make([]layout.TableColumn, columns)
	headerCells := make([]layout.TableCell, columns)
	for column := range columns {
		tracks[column] = layout.TableColumn{Width: contentWidth / float64(columns)}
		header := fmt.Sprintf("H%d", column)
		headerCells[column] = layout.TableCell{Header: true, Blocks: []layout.Block{layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: header}}, Style: style,
		}}}
	}
	body := make([]layout.TableRow, rows)
	for row := range rows {
		cells := make([]layout.TableCell, columns)
		for column := range columns {
			text := fmt.Sprintf("r%05dc%02d", row, column)
			if columns > 16 {
				// The wide-table benchmark measures track and cell scaling, not
				// rejection of an unbreakable word wider than its six-point track.
				// Keep three independently breakable glyph tokens in every cell so
				// the fixture remains valid under exact intrinsic minimums.
				text = fmt.Sprintf("r%03d c%02d", row, column)
			}
			cells[column] = layout.TableCell{Blocks: []layout.Block{layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: text}}, Style: style,
			}}}
		}
		body[row] = layout.TableRow{Cells: cells}
	}
	return &layout.LayoutDocument{
		PageTemplate: layout.PageTemplate{Margins: layout.Spacing{Top: margin, Right: margin, Bottom: margin, Left: margin}},
		Body: []layout.Block{layout.TableBlock{
			Columns: tracks, Header: []layout.TableRow{{Cells: headerCells}}, Body: body,
			Style: layout.TableStyle{RepeatHeader: true},
		}},
	}
}

func paperEngineBenchmarkParagraph(index int) layout.ParagraphBlock {
	return layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: fmt.Sprintf(
			"Section %02d has deterministic operational text that wraps across the shared content width.", index,
		)}},
		Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10, LineHeight: 12},
	}
}

func paperEngineBenchmarkHTMLFixture() string {
	var source strings.Builder
	source.WriteString("<h1>Quarterly operating report</h1>")
	for index := 0; index < paperEngineBenchmarkParagraphs; index++ {
		fmt.Fprintf(&source, "<p>Section %02d has deterministic operational text that wraps across the shared content width.</p>", index)
	}
	return source.String()
}

func paperEngineBenchmarkPaperFixture() string {
	var source strings.Builder
	source.WriteString("document @benchmark:\n")
	source.WriteString("  title: \"Paper Engine benchmark\"\n")
	source.WriteString("  language: \"en\"\n")
	source.WriteString("  page @sheet:\n")
	source.WriteString("    width: 220pt\n")
	source.WriteString("    height: 300pt\n")
	source.WriteString("    margin: 14pt\n")
	source.WriteString("    body @content:\n")
	source.WriteString("      heading @title:\n")
	source.WriteString("        level: 1\n")
	source.WriteString("        font: \"Helvetica\"\n")
	source.WriteString("        size: 14pt\n")
	source.WriteString("        line-height: 16pt\n")
	source.WriteString("        bold: true\n")
	source.WriteString("        text: \"Quarterly operating report\"\n")
	for index := 0; index < paperEngineBenchmarkParagraphs; index++ {
		fmt.Fprintf(&source, "      paragraph @section_%02d:\n", index)
		source.WriteString("        font: \"Helvetica\"\n")
		source.WriteString("        size: 10pt\n")
		source.WriteString("        line-height: 12pt\n")
		fmt.Fprintf(&source, "        text: \"Section %02d has deterministic operational text that wraps across the shared content width.\"\n", index)
	}
	return source.String()
}

func TestPaperEngineBenchmarkFixturesAreDeterministic(t *testing.T) {
	for _, fixture := range []struct{ rows, columns int }{{128, 4}, {8, 32}} {
		plan, err := paperEngineBenchmarkTableDocument(fixture.columns).PlanLayoutDocument(paperEngineBenchmarkTableFixture(fixture.rows, fixture.columns))
		if err != nil || plan.PageCount() == 0 {
			t.Fatalf("table benchmark fixture %dx%d = pages %d, %v", fixture.rows, fixture.columns, plan.PageCount(), err)
		}
	}
	typedPlan := paperEngineBenchmarkTypedPlan(t)
	secondTypedPlan := paperEngineBenchmarkTypedPlan(t)
	if typedPlan.Hash() != secondTypedPlan.Hash() || typedPlan.PageCount() != secondTypedPlan.PageCount() {
		t.Fatalf("typed plans differ: (%s, %d) != (%s, %d)", typedPlan.Hash(), typedPlan.PageCount(), secondTypedPlan.Hash(), secondTypedPlan.PageCount())
	}

	compiled, err := CompileHTML(paperEngineBenchmarkHTMLFixture())
	if err != nil {
		t.Fatal(err)
	}
	htmlPlan, err := paperEngineBenchmarkDocument().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	if htmlPlan.PageCount() == 0 || htmlPlan.Hash() == "" {
		t.Fatalf("HTML plan is empty: hash %q pages %d", htmlPlan.Hash(), htmlPlan.PageCount())
	}
	secondHTMLPlan, err := paperEngineBenchmarkDocument().PlanCompiledHTML(12, compiled)
	if err != nil {
		t.Fatal(err)
	}
	if htmlPlan.Hash() != secondHTMLPlan.Hash() || htmlPlan.PageCount() != secondHTMLPlan.PageCount() {
		t.Fatalf("HTML plans differ: (%s, %d) != (%s, %d)", htmlPlan.Hash(), htmlPlan.PageCount(), secondHTMLPlan.Hash(), secondHTMLPlan.PageCount())
	}

	paperPlan, result, err := PlanPaper("benchmark.paper", paperEngineBenchmarkPaperFixture())
	if err != nil {
		t.Fatalf("PlanPaper() = %v (%v)", err, result.Diagnostics)
	}
	if paperPlan.PageCount() == 0 || paperPlan.Hash() == "" {
		t.Fatalf(".paper plan is empty: hash %q pages %d", paperPlan.Hash(), paperPlan.PageCount())
	}
	secondPaperPlan, secondResult, err := PlanPaper("benchmark.paper", paperEngineBenchmarkPaperFixture())
	if err != nil {
		t.Fatalf("second PlanPaper() = %v (%v)", err, secondResult.Diagnostics)
	}
	if paperPlan.Hash() != secondPaperPlan.Hash() || paperPlan.PageCount() != secondPaperPlan.PageCount() {
		t.Fatalf(".paper plans differ: (%s, %d) != (%s, %d)", paperPlan.Hash(), paperPlan.PageCount(), secondPaperPlan.Hash(), secondPaperPlan.PageCount())
	}

	assertPaperEngineLayoutPlanOutputsEqual(t, "typed", typedPlan, secondTypedPlan)
	assertPaperEngineLayoutPlanOutputsEqual(t, "HTML", htmlPlan, secondHTMLPlan)
	firstPaper := paperEngineBenchmarkRenderPaperPlan(t, paperPlan)
	secondPaper := paperEngineBenchmarkRenderPaperPlan(t, secondPaperPlan)
	if !bytes.Equal(firstPaper, secondPaper) {
		t.Fatalf("deterministic .paper outputs differ: %d and %d bytes", len(firstPaper), len(secondPaper))
	}
}

func assertPaperEngineLayoutPlanOutputsEqual(t *testing.T, frontend string, firstPlan, secondPlan LayoutDocumentPlan) {
	t.Helper()
	first := paperEngineBenchmarkRenderLayoutPlan(t, firstPlan)
	second := paperEngineBenchmarkRenderLayoutPlan(t, secondPlan)
	if !bytes.Equal(first, second) {
		t.Fatalf("deterministic %s outputs differ: %d and %d bytes", frontend, len(first), len(second))
	}
}

func paperEngineBenchmarkRenderLayoutPlan(t *testing.T, plan LayoutDocumentPlan) []byte {
	t.Helper()
	target := paperEngineBenchmarkDocument()
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	output, err := paperEngineBenchmarkOutput(target)
	if err != nil {
		t.Fatal(err)
	}
	return output
}

func paperEngineBenchmarkRenderPaperPlan(t *testing.T, plan PaperPlan) []byte {
	t.Helper()
	target := paperEngineBenchmarkDocument()
	if result, err := target.WritePaperPlan(plan); err != nil {
		t.Fatalf("WritePaperPlan() = %v (%v)", err, result.Diagnostics)
	}
	output, err := paperEngineBenchmarkOutput(target)
	if err != nil {
		t.Fatal(err)
	}
	return output
}
