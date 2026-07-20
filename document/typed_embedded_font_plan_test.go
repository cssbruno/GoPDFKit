// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/paperrune/internal/layoutengine"
	"github.com/cssbruno/paperrune/layout"
)

func TestLayoutDocumentPlanEmbedsImmutableUTF8FontForPDFA(t *testing.T) {
	fontBytes := readUTF8FontFixture(t)
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	if err := source.AddUTF8FontFromBytesError("PlanSans", "", fontBytes); err != nil {
		t.Fatal(err)
	}
	source.SetComplianceMetadata(ComplianceMetadata{PDFA: PDFAMode4, Identifier: "urn:paperrune:typed-font-plan"})
	if err := source.SetOutputIntent([]byte("deterministic-test-icc"), "sRGB IEC61966-2.1"); err != nil {
		t.Fatal(err)
	}
	doc := &layout.LayoutDocument{Title: "Unicode plan", Body: []layout.Block{layout.ParagraphBlock{
		Style:    layout.TextStyle{FontFamily: "PlanSans", FontSize: 12},
		Segments: []layout.TextSegment{{Text: "Olá, ação e café — UTF-8 planejado", Link: "https://example.test/unicode"}},
	}}}
	first, err := source.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatalf("PlanLayoutDocument() = %v", err)
	}
	second, err := source.PlanLayoutDocument(doc)
	if err != nil || first.Hash() != second.Hash() {
		t.Fatalf("deterministic plans = %q, %q, %v", first.Hash(), second.Hash(), err)
	}
	projection := first.plan.Projection()
	if len(projection.Fonts) != 1 || projection.Fonts[0].EmbeddedUTF8 == nil || projection.Fonts[0].Face != "" ||
		len(first.fontSources) != 1 || !strings.Contains(string(mustCanonicalPlanJSON(t, first)), `"embedded_utf8"`) {
		t.Fatalf("embedded font projection/sources = %#v / %d", projection.Fonts, len(first.fontSources))
	}
	for index := range fontBytes {
		fontBytes[index] ^= 0xff
	}

	render := func() []byte {
		t.Helper()
		target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
		if pages, err := target.WriteLayoutDocumentPlan(first); err != nil || pages != 1 {
			t.Fatalf("WriteLayoutDocumentPlan() = %d, %v", pages, err)
		}
		var output bytes.Buffer
		if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
			t.Fatalf("positive PDF/A output = %v", err)
		}
		for _, marker := range []string{"/Subtype /Type0", "/Encoding /Identity-H", "/ToUnicode ", "/FontFile2 ", "/URI (https://example.test/unicode)", "<pdfaid:part>4</pdfaid:part>"} {
			if !strings.Contains(output.String(), marker) {
				t.Fatalf("PDF/A output lacks %q", marker)
			}
		}
		return output.Bytes()
	}
	want := render()
	const workers = 4
	outputs := make([][]byte, workers)
	var wg sync.WaitGroup
	for index := range outputs {
		wg.Add(1)
		go func(index int) { defer wg.Done(); outputs[index] = render() }(index)
	}
	wg.Wait()
	for index, output := range outputs {
		if !bytes.Equal(output, want) {
			t.Fatalf("concurrent deterministic output %d differs", index)
		}
	}
}

func TestLayoutDocumentPlanSupportsMixedCoreAndEmbeddedUTF8MetricRuns(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	if err := source.AddUTF8FontFromBytesError("PlanSans", "", readUTF8FontFixture(t)); err != nil {
		t.Fatal(err)
	}
	doc := &layout.LayoutDocument{Body: []layout.Block{layout.ParagraphBlock{
		Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 12},
		Segments: []layout.TextSegment{
			{Text: "antes "},
			{Text: "ação 日本語 é", Style: layout.TextStyle{FontFamily: "PlanSans", FontSize: 18, Underline: true}},
			{Text: " depois"},
		},
	}}}
	plan, err := source.PlanLayoutDocument(doc)
	if err != nil {
		t.Fatalf("PlanLayoutDocument() = %v", err)
	}
	projection := plan.plan.Projection()
	if len(projection.Fonts) != 2 || projection.Fonts[0].Face == "" || projection.Fonts[1].EmbeddedUTF8 == nil || len(projection.GlyphRuns) < 3 || len(projection.Strokes) != 1 {
		t.Fatalf("mixed embedded projection = fonts %#v, glyph runs %d, strokes %d", projection.Fonts, len(projection.GlyphRuns), len(projection.Strokes))
	}
	fontSizes := make(map[layoutengine.Fixed]bool)
	for _, run := range projection.GlyphRuns {
		fontSizes[run.FontSize] = true
	}
	size12, _ := layoutengine.FixedFromPoints(12)
	size18, _ := layoutengine.FixedFromPoints(18)
	if !fontSizes[size12] || !fontSizes[size18] {
		t.Fatalf("mixed embedded run sizes = %#v, want 12pt and 18pt", fontSizes)
	}
	capture, err := plan.CaptureDisplayPage(1)
	capturedSVG := string(capture.SVG())
	if err != nil || !strings.Contains(capturedSVG, ">日<") || !strings.Contains(capturedSVG, ">́<") {
		t.Fatalf("mixed embedded display capture = %v, svg=%q", err, string(capture.SVG()))
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if pages, err := target.WriteLayoutDocumentPlan(plan); err != nil || pages != plan.PageCount() {
		t.Fatalf("WriteLayoutDocumentPlan() = pages %d, want %d, err %v", pages, plan.PageCount(), err)
	}
}

func TestLayoutDocumentPlanEmitsTypedPDFUASemanticStructure(t *testing.T) {
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 220}), WithNoCompression(), WithDeterministicOutput())
	source.SetMargins(10, 10, 10)
	if err := source.AddUTF8FontFromBytesError("PlanSans", "", readUTF8FontFixture(t)); err != nil {
		t.Fatal(err)
	}
	pixel, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	source.SetComplianceMetadata(ComplianceMetadata{PDFUA2: true, Title: "Typed PDF/UA fixture", Lang: "en-US"})
	model := &layout.LayoutDocument{
		Title: "Typed PDF/UA fixture", Language: "en-US",
		Body: []layout.Block{
			layout.HeadingBlock{Level: 1, Style: layout.TextStyle{FontFamily: "PlanSans", FontSize: 16}, Segments: []layout.TextSegment{{Text: "Overview"}}},
			layout.ParagraphBlock{Style: layout.TextStyle{FontFamily: "PlanSans", FontSize: 11}, Segments: []layout.TextSegment{{Text: "Accessible paragraph with a link", Link: "https://example.test/typed"}}},
			layout.ListBlock{Items: []layout.ListItem{{Blocks: []layout.Block{layout.ParagraphBlock{Style: layout.TextStyle{FontFamily: "PlanSans", FontSize: 11}, Segments: []layout.TextSegment{{Text: "First item"}}}}}}},
			layout.TableBlock{
				Columns: []layout.TableColumn{{Width: 110}, {Width: 110}},
				Header: []layout.TableRow{{Cells: []layout.TableCell{
					{Header: true, Blocks: []layout.Block{layout.ParagraphBlock{Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10}, Segments: []layout.TextSegment{{Text: "Name"}}}}},
					{Header: true, Blocks: []layout.Block{layout.ParagraphBlock{Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10}, Segments: []layout.TextSegment{{Text: "Value"}}}}},
				}}},
				Body: []layout.TableRow{{Cells: []layout.TableCell{
					{Blocks: []layout.Block{layout.ParagraphBlock{Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10}, Segments: []layout.TextSegment{{Text: "status"}}}}},
					{Blocks: []layout.Block{layout.ParagraphBlock{Style: layout.TextStyle{FontFamily: "Helvetica", FontSize: 10}, Segments: []layout.TextSegment{{Text: "ready"}}}}},
				}}},
			},
			layout.ImageBlock{Data: pixel, Format: "png", Alt: "A one-pixel status mark", Width: 12, Height: 12, Fit: layout.ImageFitContain},
		},
	}
	plan, err := source.PlanLayoutDocument(model)
	if err != nil || plan.PageCount() != 1 {
		t.Fatalf("typed PDF/UA plan = pages %d, %v", plan.PageCount(), err)
	}
	target := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := target.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := target.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"/StructTreeRoot", "/MarkInfo << /Marked true >>", "/Lang ", "/ActualText ", "/S /H1", "/S /P", "/S /L", "/S /LI", "/S /Table", "/S /TR", "/S /TH", "/S /TD", "/S /Figure", "/Alt ", "/Subtype /Link"} {
		if !bytes.Contains(output.Bytes(), []byte(marker)) {
			t.Fatalf("typed PDF/UA output lacks %q", marker)
		}
	}
}

func TestLayoutDocumentEmbeddedFontPreflightIsBoundedCancelableAndAtomic(t *testing.T) {
	digest := layoutengine.CoreFontMetricsDigest(strings.Repeat("a", 64))
	seen := make(map[layoutengine.CoreFontMetricsDigest]bool)
	var used uint64
	if err := chargePlannedFontSourceBytes(seen, digest, 6, 5, &used); !errors.Is(err, errCoreLayoutPlanPaintUnsupported) || used != 0 {
		t.Fatalf("font byte limit = used %d, %v", used, err)
	}
	if err := chargePlannedFontSourceBytes(seen, digest, 5, 5, &used); err != nil {
		t.Fatal(err)
	}
	if err := chargePlannedFontSourceBytes(seen, digest, 5, 5, &used); err != nil || used != 5 {
		t.Fatalf("deduplicated font byte charge = used %d, %v", used, err)
	}

	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}))
	if err := source.AddUTF8FontFromBytesError("PlanSans", "", readUTF8FontFixture(t)); err != nil {
		t.Fatal(err)
	}
	plan, err := source.PlanLayoutDocument(&layout.LayoutDocument{Body: []layout.Block{layout.ParagraphBlock{
		Style: layout.TextStyle{FontFamily: "PlanSans"}, Segments: []layout.TextSegment{{Text: "conteúdo"}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MustNew(WithUnit(UnitPoint)).WriteLayoutDocumentPlanContext(canceled, plan); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled write = %v", err)
	}
	tampered := plan
	tampered.fontSources = make(plannedFontSources, len(plan.fontSources))
	for digest, data := range plan.fontSources {
		tampered.fontSources[digest] = append([]byte(nil), data...)
		tampered.fontSources[digest][0] ^= 0xff
	}
	target := MustNew(WithUnit(UnitPoint))
	if _, err := target.WriteLayoutDocumentPlan(tampered); err == nil || target.PageCount() != 0 || target.state != documentStateUnopened {
		t.Fatalf("tampered font write = pages %d state %d error %v", target.PageCount(), target.state, err)
	}
}

func mustCanonicalPlanJSON(t *testing.T, plan LayoutDocumentPlan) []byte {
	t.Helper()
	encoded, err := plan.plan.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
