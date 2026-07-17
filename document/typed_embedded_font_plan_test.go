// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestLayoutDocumentPlanEmbedsImmutableUTF8FontForPDFA(t *testing.T) {
	fontBytes := readUTF8FontFixture(t)
	source := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 240, Ht: 160}), WithNoCompression(), WithDeterministicOutput())
	if err := source.AddUTF8FontFromBytesError("PlanSans", "", fontBytes); err != nil {
		t.Fatal(err)
	}
	source.SetComplianceMetadata(ComplianceMetadata{PDFA: PDFAMode4, Identifier: "urn:gopdfkit:typed-font-plan"})
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
