// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestTypedPaginationShadowMatchesLegacyAtomicParagraphPagesWithoutMutation(t *testing.T) {
	pdf := MustNew(
		WithUnit(UnitPoint),
		WithCustomPageSize(Size{Wd: 200, Ht: 160}),
	)
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	doc := layout.NewLayoutDocument()
	doc.PageTemplate.Margins = layout.Spacing{Top: 15, Right: 15, Bottom: 15, Left: 15}
	for index := range 5 {
		doc.Body = append(doc.Body, layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: "atomic paragraph"}},
			Style: layout.TextStyle{
				FontFamily: "Helvetica",
				FontSize:   11,
				LineHeight: 30,
				Bold:       index%2 == 0,
			},
			Box: layout.BoxStyle{KeepTogether: true},
		})
	}

	before := typedShadowSnapshotOf(pdf)
	shadow, err := pdf.planTypedDocumentPaginationShadow(doc)
	if err != nil {
		t.Fatalf("planTypedDocumentPaginationShadow() = %v", err)
	}
	if after := typedShadowSnapshotOf(pdf); after != before {
		t.Fatalf("shadow mutated live document:\nbefore %#v\nafter  %#v", before, after)
	}
	if got, want := shadow.BlockIndices, []int{0, 1, 2, 3, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("block indexes = %#v, want %#v", got, want)
	}
	projection := shadow.Plan.Projection()
	if len(projection.Pages) != 2 || len(projection.Fragments) != 5 {
		t.Fatalf("shadow projection = %#v, want two pages and five fragments", projection)
	}
	if projection.Pages[0].Fragments.Count != 4 || projection.Pages[1].Fragments.Count != 1 {
		t.Fatalf("shadow page fragment ranges = %#v/%#v", projection.Pages[0].Fragments, projection.Pages[1].Fragments)
	}
	if len(projection.Commands) != 0 {
		t.Fatalf("shadow commands = %#v, want allocation geometry only", projection.Commands)
	}

	pdf.writeDocumentLegacy(doc)
	if err := pdf.Error(); err != nil {
		t.Fatalf("WriteDocument() = %v", err)
	}
	if got, want := pdf.PageCount(), len(projection.Pages); got != want {
		t.Fatalf("legacy pages = %d, shadow pages = %d", got, want)
	}
}

func TestTypedPaginationShadowConvertsSupportedDocumentUnitsToFixedPoints(t *testing.T) {
	for _, unit := range []Unit{UnitPoint, UnitMillimeter, UnitCentimeter, UnitInch} {
		t.Run(unit.String(), func(t *testing.T) {
			pdf := MustNew(WithUnit(unit))
			margin := pdf.PointConvert(36)
			pdf.SetMargins(margin, margin, margin)
			pdf.SetAutoPageBreak(true, margin)
			doc := layout.NewLayoutDocument()
			doc.Body = []layout.Block{layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: "unit conversion"}},
				Style:    layout.TextStyle{LineHeight: pdf.PointConvert(18)},
				Box:      layout.BoxStyle{KeepTogether: true},
			}}

			shadow, err := pdf.planTypedDocumentPaginationShadow(doc)
			if err != nil {
				t.Fatalf("planTypedDocumentPaginationShadow() = %v", err)
			}
			projection := shadow.Plan.Projection()
			pageWidth, err := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(pdf.w))
			if err != nil {
				t.Fatalf("page width conversion = %v", err)
			}
			pageHeight, err := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(pdf.h))
			if err != nil {
				t.Fatalf("page height conversion = %v", err)
			}
			fixedMargin, err := layoutengine.FixedFromPoints(36)
			if err != nil {
				t.Fatalf("margin conversion = %v", err)
			}
			bodyWidth, err := layoutengine.FixedFromPoints(pdf.UnitToPointConvert(pdf.w - 2*margin))
			if err != nil {
				t.Fatalf("body width conversion = %v", err)
			}
			if got, want := projection.Pages[0].Size, (layoutengine.Size{Width: pageWidth, Height: pageHeight}); got != want {
				t.Fatalf("page size = %#v, want %#v", got, want)
			}
			fragment := projection.Fragments[0]
			if fragment.BorderBox.X != fixedMargin || fragment.BorderBox.Y != fixedMargin || fragment.BorderBox.Width != bodyWidth {
				t.Fatalf("fragment fixed geometry = %#v, margin %d width %d", fragment.BorderBox, fixedMargin, bodyWidth)
			}
		})
	}
}

func TestTypedPaginationShadowPreservesExactFitAndOneFixedUnitRemaining(t *testing.T) {
	tests := []struct {
		name            string
		lineHeights     []float64
		wantPages       int
		wantAvailable   layoutengine.Fixed
		wantBreakReason layoutengine.BreakReason
	}{
		{
			name:        "exact fit",
			lineHeights: []float64{78},
			wantPages:   1,
		},
		{
			name:            "one fixed unit remaining",
			lineHeights:     []float64{78 - 1.0/float64(layoutengine.FixedScale), 1},
			wantPages:       2,
			wantAvailable:   1,
			wantBreakReason: layoutengine.BreakInsufficientRemainingBodySpace,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}))
			pdf.SetMargins(10, 10, 10)
			pdf.SetAutoPageBreak(true, 10)
			doc := layout.NewLayoutDocument()
			for _, lineHeight := range test.lineHeights {
				doc.Body = append(doc.Body, layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "x"}},
					Style:    layout.TextStyle{LineHeight: lineHeight},
					Box:      layout.BoxStyle{KeepTogether: true},
				})
			}

			shadow, err := pdf.planTypedDocumentPaginationShadow(doc)
			if err != nil {
				t.Fatalf("planTypedDocumentPaginationShadow() = %v", err)
			}
			projection := shadow.Plan.Projection()
			if got := len(projection.Pages); got != test.wantPages {
				t.Fatalf("shadow pages = %d, want %d", got, test.wantPages)
			}
			if test.wantBreakReason == "" {
				if len(projection.Breaks) != 0 {
					t.Fatalf("exact-fit breaks = %#v, want none", projection.Breaks)
				}
			} else {
				if len(projection.Breaks) != 1 || projection.Breaks[0].Reason != test.wantBreakReason ||
					projection.Breaks[0].Available != test.wantAvailable {
					t.Fatalf("shadow break = %#v, want %q with available %d", projection.Breaks, test.wantBreakReason, test.wantAvailable)
				}
			}

			pdf.writeDocumentLegacy(doc)
			if err := pdf.Error(); err != nil {
				t.Fatalf("WriteDocument() = %v", err)
			}
			if got := pdf.PageCount(); got != test.wantPages {
				t.Fatalf("legacy pages = %d, want %d", got, test.wantPages)
			}
		})
	}
}

func TestTypedPaginationShadowRejectsUncharacterizedStateAndContent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Document, *layout.LayoutDocument)
		reason typedShadowUnsupportedReason
	}{
		{
			name:   "existing page",
			mutate: func(pdf *Document, _ *layout.LayoutDocument) { pdf.AddPage() },
			reason: typedShadowDocumentState,
		},
		{
			name:   "auto page break disabled",
			mutate: func(pdf *Document, _ *layout.LayoutDocument) { pdf.SetAutoPageBreak(false, 10) },
			reason: typedShadowDocumentPolicy,
		},
		{
			name:   "custom page break callback",
			mutate: func(pdf *Document, _ *layout.LayoutDocument) { pdf.SetAcceptPageBreakFunc(func() bool { return true }) },
			reason: typedShadowDocumentPolicy,
		},
		{
			name:   "header callback",
			mutate: func(pdf *Document, _ *layout.LayoutDocument) { pdf.SetHeaderFunc(func() {}) },
			reason: typedShadowDocumentPolicy,
		},
		{
			name: "page template header",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.PageTemplate.Header = &layout.HeaderBlock{}
			},
			reason: typedShadowPageTemplate,
		},
		{
			name: "document signature",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Signature = &layout.SignatureBlock{}
			},
			reason: typedShadowDocumentEnvelope,
		},
		{
			name: "table block",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.TableBlock{}}
			},
			reason: typedShadowBlockKind,
		},
		{
			name: "splittable paragraph",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "split"}}}}
			},
			reason: typedShadowParagraphContract,
		},
		{
			name: "segment link",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "link", Link: "https://example.test"}},
					Box:      layout.BoxStyle{KeepTogether: true},
				}}
			},
			reason: typedShadowParagraphContract,
		},
		{
			name: "carriage return text",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "first\rsecond"}},
					Box:      layout.BoxStyle{KeepTogether: true},
				}}
			},
			reason: typedShadowParagraphContract,
		},
		{
			name: "multiple trailing newlines",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "line\n\n"}},
					Box:      layout.BoxStyle{KeepTogether: true},
				}}
			},
			reason: typedShadowParagraphContract,
		},
		{
			name: "non ASCII core font text",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "café"}},
					Box:      layout.BoxStyle{KeepTogether: true},
				}}
			},
			reason: typedShadowParagraphContract,
		},
		{
			name: "tab whitespace",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "left\tright"}},
					Box:      layout.BoxStyle{KeepTogether: true},
				}}
			},
			reason: typedShadowParagraphContract,
		},
		{
			name: "custom font",
			mutate: func(_ *Document, doc *layout.LayoutDocument) {
				doc.Body = []layout.Block{layout.ParagraphBlock{
					Segments: []layout.TextSegment{{Text: "font"}},
					Style:    layout.TextStyle{FontFamily: "Unregistered"},
					Box:      layout.BoxStyle{KeepTogether: true},
				}}
			},
			reason: typedShadowParagraphContract,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 200, Ht: 200}))
			pdf.SetMargins(20, 20, 20)
			pdf.SetAutoPageBreak(true, 20)
			doc := layout.NewLayoutDocument()
			doc.Body = []layout.Block{layout.ParagraphBlock{
				Segments: []layout.TextSegment{{Text: "atomic"}},
				Box:      layout.BoxStyle{KeepTogether: true},
			}}
			test.mutate(pdf, doc)

			_, err := pdf.planTypedDocumentPaginationShadow(doc)
			if !errors.Is(err, errTypedShadowUnsupported) {
				t.Fatalf("shadow error = %v, want errTypedShadowUnsupported", err)
			}
			var unsupported *typedShadowUnsupportedError
			if !errors.As(err, &unsupported) || unsupported.Reason != test.reason {
				t.Fatalf("shadow error = %#v, want reason %q", err, test.reason)
			}
		})
	}
}

type typedShadowSnapshot struct {
	page         int
	state        documentState
	x            float64
	y            float64
	left         float64
	top          float64
	right        float64
	bottom       float64
	fontFamily   string
	fontStyle    string
	fontSizePt   float64
	fontSizeUnit float64
	fontCount    int
	errorText    string
}

func typedShadowSnapshotOf(pdf *Document) typedShadowSnapshot {
	left, top, right, bottom := pdf.GetMargins()
	errorText := ""
	if pdf.err != nil {
		errorText = pdf.err.Error()
	}
	return typedShadowSnapshot{
		page:         pdf.page,
		state:        pdf.state,
		x:            pdf.x,
		y:            pdf.y,
		left:         left,
		top:          top,
		right:        right,
		bottom:       bottom,
		fontFamily:   pdf.fontFamily,
		fontStyle:    pdf.fontStyle,
		fontSizePt:   pdf.fontSizePt,
		fontSizeUnit: pdf.fontSize,
		fontCount:    len(pdf.ensureResourceStore().fonts),
		errorText:    errorText,
	}
}
