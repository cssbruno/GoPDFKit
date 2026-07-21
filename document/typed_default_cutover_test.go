// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/layout"
)

type typedRouteObservation struct {
	entryPoint string
	engine     string
	reason     string
}

func TestWriteDocumentDefaultsToExactPlanAndReportsBoundedRoute(t *testing.T) {
	model := &layout.LayoutDocument{
		Title: "Unified default",
		Body: []layout.Block{
			layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: "Exact heading"}}},
			layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Exact body"}}},
		},
	}
	var observed []typedRouteObservation
	direct := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput(), WithHooks(Hooks{
		OnLayoutEngineRoute: func(entryPoint, engine, reason string) {
			observed = append(observed, typedRouteObservation{entryPoint, engine, reason})
		},
	}))
	direct.WriteDocument(model)
	if err := direct.Error(); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0] != (typedRouteObservation{"WriteDocument", "unified", ""}) {
		t.Fatalf("route observations = %#v", observed)
	}

	planner := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	plan, err := planner.PlanLayoutDocument(model)
	if err != nil {
		t.Fatal(err)
	}
	explicit := MustNew(WithUnit(UnitPoint), WithNoCompression(), WithDeterministicOutput())
	if _, err := explicit.WriteLayoutDocumentPlan(plan); err != nil {
		t.Fatal(err)
	}
	if got, want := deterministicDocumentBytes(t, direct), deterministicDocumentBytes(t, explicit); !bytes.Equal(got, want) {
		t.Fatalf("WriteDocument output differs from explicit immutable plan: %x / %x", planDigest(got), planDigest(want))
	}
}

func TestWriteDocumentFallbackIsWholeDocumentObservableAndPrivate(t *testing.T) {
	const authoredSecret = "customer-secret-must-not-enter-telemetry"
	model := &layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: authoredSecret}}},
	}}
	var observed []typedRouteObservation
	pdf := MustNew(WithNoCompression(), WithHooks(Hooks{
		OnLayoutEngineRoute: func(entryPoint, engine, reason string) {
			observed = append(observed, typedRouteObservation{entryPoint, engine, reason})
		},
	}))
	// A custom lifecycle callback is intentionally outside immutable planning,
	// but remains a supported whole-document legacy rollback contract.
	pdf.SetHeaderFunc(func() {})
	pdf.WriteDocument(model)
	if err := pdf.Error(); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0].entryPoint != "WriteDocument" || observed[0].engine != "legacy" || observed[0].reason != "document-policy" {
		t.Fatalf("fallback observation = %#v", observed)
	}
	if strings.Contains(observed[0].reason, authoredSecret) || len(observed[0].reason) > 64 {
		t.Fatalf("fallback reason leaked authored content: %q", observed[0].reason)
	}
	crafted := &layoutDocumentUnsupportedError{detail: "body[0]: " + authoredSecret, reason: typedShadowBlockKind}
	if category := typedWriteDocumentFallbackCategory(crafted); category != "unsupported-layout-contract" || strings.Contains(category, authoredSecret) {
		t.Fatalf("authored unsupported detail leaked into category %q", category)
	}
	if got := extractedDocumentText(t, deterministicDocumentBytes(t, pdf)); !strings.Contains(got, authoredSecret) {
		t.Fatalf("whole-document fallback lost body text: %q", got)
	}
}

func TestWriteDocumentNonFreshReceiverUsesObservableWholeDocumentRollback(t *testing.T) {
	var observed []typedRouteObservation
	pdf := MustNew(WithNoCompression(), WithHooks(Hooks{OnLayoutEngineRoute: func(entryPoint, engine, reason string) {
		observed = append(observed, typedRouteObservation{entryPoint, engine, reason})
	}}))
	pdf.AddPage()
	pdf.WriteDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "existing-page rollback"}}},
	}})
	if err := pdf.Error(); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 || observed[0] != (typedRouteObservation{"WriteDocument", "legacy", "non-fresh-receiver"}) {
		t.Fatalf("non-fresh route = %#v", observed)
	}
	if got := extractedDocumentText(t, deterministicDocumentBytes(t, pdf)); !strings.Contains(got, "existing-page rollback") {
		t.Fatalf("non-fresh whole-document route lost text: %q", got)
	}
}

func TestWriteDocumentInvalidUnifiedInputFailsAtomicallyWithoutFallback(t *testing.T) {
	var observed []typedRouteObservation
	pdf := MustNew(WithHooks(Hooks{OnLayoutEngineRoute: func(entryPoint, engine, reason string) {
		observed = append(observed, typedRouteObservation{entryPoint, engine, reason})
	}}))
	pdf.WriteDocument(&layout.LayoutDocument{Body: []layout.Block{
		layout.TableBlock{
			Columns: []layout.TableColumn{{Width: 400}, {Width: 400}},
			Body: []layout.TableRow{{Cells: []layout.TableCell{
				{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "left"}}}}},
				{Blocks: []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "right"}}}}},
			}}},
		},
	}})
	if pdf.Error() == nil || pdf.PageCount() != 0 || pdf.state != documentStateUnopened {
		t.Fatalf("invalid unified input = pages %d state %d error %v", pdf.PageCount(), pdf.state, pdf.Error())
	}
	if len(observed) != 0 {
		t.Fatalf("invalid input silently changed engines: %#v", observed)
	}
}

func TestWriteDocumentMatchesExactPlanAcrossCharacterizationCorpus(t *testing.T) {
	for _, fixture := range typedCharacterizationFixtures() {
		if fixture.mode == "cancel" || fixture.mode == "limit" || fixture.mode == "malformed" {
			continue
		}
		t.Run(fixture.inventory.Name, func(t *testing.T) {
			options := []Option{WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: fixture.pageHeight}), WithNoCompression(), WithDeterministicOutput()}
			planner := MustNew(options...)
			plan, err := planner.PlanLayoutDocument(fixture.doc)
			if err != nil {
				t.Skipf("characterized non-planned fixture: %v", err)
			}
			explicit := MustNew(options...)
			if _, err := explicit.WriteLayoutDocumentPlan(plan); err != nil {
				t.Fatal(err)
			}
			var observed []typedRouteObservation
			directOptions := append(append([]Option(nil), options...), WithHooks(Hooks{OnLayoutEngineRoute: func(entryPoint, engine, reason string) {
				observed = append(observed, typedRouteObservation{entryPoint, engine, reason})
			}}))
			direct := MustNew(directOptions...)
			direct.WriteDocument(fixture.doc)
			if err := direct.Error(); err != nil {
				t.Fatal(err)
			}
			if len(observed) != 1 || observed[0].engine != "unified" || observed[0].reason != "" {
				t.Fatalf("route = %#v", observed)
			}
			if got, want := deterministicDocumentBytes(t, direct), deterministicDocumentBytes(t, explicit); !bytes.Equal(got, want) {
				t.Fatalf("default/explicit plan outputs differ: %x / %x", planDigest(got), planDigest(want))
			}
		})
	}
}

func deterministicDocumentBytes(t *testing.T, pdf *Document) []byte {
	t.Helper()
	var output bytes.Buffer
	if err := pdf.OutputWithOptions(&output, OutputOptions{Deterministic: true}); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func planDigest(value []byte) [sha256.Size]byte { return sha256.Sum256(value) }

func BenchmarkPaperEngineProductionDefault(b *testing.B) {
	model := paperEngineBenchmarkTypedFixture()
	b.ReportAllocs()
	for b.Loop() {
		pdf := paperEngineBenchmarkDocument()
		pdf.WriteDocument(model)
		if err := pdf.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
