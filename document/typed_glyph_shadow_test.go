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

func TestTypedCoreFontResourceCanonicalFacesAndDigests(t *testing.T) {
	tests := []struct {
		key  string
		face layoutengine.CoreFontFace
		name string
	}{
		{"courier", layoutengine.CoreFontCourier, "Courier"},
		{"courierB", layoutengine.CoreFontCourierBold, "Courier-Bold"},
		{"courierI", layoutengine.CoreFontCourierOblique, "Courier-Oblique"},
		{"courierBI", layoutengine.CoreFontCourierBoldOblique, "Courier-BoldOblique"},
		{"helvetica", layoutengine.CoreFontHelvetica, "Helvetica"},
		{"helveticaB", layoutengine.CoreFontHelveticaBold, "Helvetica-Bold"},
		{"helveticaI", layoutengine.CoreFontHelveticaOblique, "Helvetica-Oblique"},
		{"helveticaBI", layoutengine.CoreFontHelveticaBoldOblique, "Helvetica-BoldOblique"},
		{"times", layoutengine.CoreFontTimesRoman, "Times-Roman"},
		{"timesB", layoutengine.CoreFontTimesBold, "Times-Bold"},
		{"timesI", layoutengine.CoreFontTimesItalic, "Times-Italic"},
		{"timesBI", layoutengine.CoreFontTimesBoldItalic, "Times-BoldItalic"},
		{"symbol", layoutengine.CoreFontSymbol, "Symbol"},
		{"zapfdingbats", layoutengine.CoreFontZapfDingbats, "ZapfDingbats"},
	}
	for _, test := range tests {
		t.Run(test.key, func(t *testing.T) {
			font, err := loadCoreFontDef(test.key)
			if err != nil {
				t.Fatalf("loadCoreFontDef() = %v", err)
			}
			if font.Name != test.name {
				t.Fatalf("base font = %q, want %q", font.Name, test.name)
			}
			resource, err := typedCoreFontResource(font)
			if err != nil {
				t.Fatalf("typedCoreFontResource() = %v", err)
			}
			if resource.ID != 1 || resource.Face != test.face || len(resource.MetricsDigest) != 64 {
				t.Fatalf("resource = %+v", resource)
			}
		})
	}
	for _, golden := range []struct {
		key, digest string
	}{
		{"helvetica", "83e2f281391b2cb70e113f03880ff75b9aa9aa70304899d24701ed2a4259bd94"},
		{"symbol", "4f652487dcf79a1a89450997e82eff7cf29282bb37e3d287adb31593780e991a"},
	} {
		font, _ := loadCoreFontDef(golden.key)
		resource, _ := typedCoreFontResource(font)
		if got := string(resource.MetricsDigest); got != golden.digest {
			t.Fatalf("%s digest = %s, want %s", golden.key, got, golden.digest)
		}
	}
}

func TestTypedParagraphLineShadowLowersExactGlyphCommands(t *testing.T) {
	pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 72, Ht: 50}))
	pdf.SetMargins(10, 10, 10)
	pdf.SetAutoPageBreak(true, 10)
	pdf.SetFont("Courier", "", 10)
	pdf.SetWordSpacing(.375)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{
		Segments: []layout.TextSegment{{Text: "AA BB\n\nCC DD EE"}},
		Style: layout.TextStyle{
			FontFamily: "Courier", FontSize: 10, LineHeight: 10, Align: "R",
		},
	}}

	before := typedShadowSnapshotOf(pdf)
	first, err := pdf.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("planTypedParagraphLineShadow() = %v", err)
	}
	second, err := pdf.planTypedParagraphLineShadow(doc)
	if err != nil {
		t.Fatalf("second planTypedParagraphLineShadow() = %v", err)
	}
	if after := typedShadowSnapshotOf(pdf); after != before {
		t.Fatalf("glyph lowering mutated live document:\nbefore %#v\nafter  %#v", before, after)
	}
	firstJSON, _ := first.Plan.CanonicalJSON()
	secondJSON, _ := second.Plan.CanonicalJSON()
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatal("identical glyph lowering is not deterministic")
	}

	projection := first.Plan.Projection()
	if len(projection.Fonts) != 1 || projection.Fonts[0].Face != layoutengine.CoreFontCourier {
		t.Fatalf("font table = %+v", projection.Fonts)
	}
	nonempty := 0
	for lineIndex, wrapped := range first.Lines {
		codes := first.Text[wrapped.StartByte:wrapped.EndByte]
		if codes == "" {
			continue
		}
		if nonempty >= len(projection.GlyphRuns) {
			t.Fatalf("line %d has no glyph run", lineIndex)
		}
		run := projection.GlyphRuns[nonempty]
		line := projection.Lines[lineIndex]
		if run.Line != uint32(lineIndex) || run.Codes != codes ||
			run.Origin != (layoutengine.Point{X: line.Bounds.X, Y: line.Baseline}) {
			t.Fatalf("run %d = %+v, line = %+v, codes %q", nonempty, run, line, codes)
		}
		var width layoutengine.Fixed
		for _, advance := range run.Advances {
			width += advance
		}
		if width != line.Bounds.Width {
			t.Fatalf("run %d width = %d, want %d", nonempty, width, line.Bounds.Width)
		}
		command := projection.Commands[nonempty]
		if command.Kind != layoutengine.CommandGlyphRun || command.Payload != uint32(nonempty) ||
			command.Fragment != line.Fragment || command.Bounds != line.Bounds {
			t.Fatalf("command %d = %+v", nonempty, command)
		}
		nonempty++
	}
	if nonempty != len(projection.GlyphRuns) || nonempty != len(projection.Commands) {
		t.Fatalf("nonempty lines/runs/commands = %d/%d/%d", nonempty, len(projection.GlyphRuns), len(projection.Commands))
	}
}

func TestTypedParagraphGlyphShadowRejectsDeferredAliasesAndNegativeSpacing(t *testing.T) {
	makeDocument := func() (*Document, *layout.LayoutDocument) {
		pdf := MustNew(WithUnit(UnitPoint), WithCustomPageSize(Size{Wd: 100, Ht: 100}))
		pdf.SetMargins(10, 10, 10)
		pdf.SetAutoPageBreak(true, 10)
		pdf.SetFont("Courier", "", 10)
		doc := layout.NewLayoutDocument()
		doc.Body = []layout.Block{layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: "AA BB"}},
			Style:    layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 10},
		}}
		return pdf, doc
	}
	pdf, doc := makeDocument()
	pdf.RegisterAlias("{page}", "1")
	if _, err := pdf.planTypedParagraphLineShadow(doc); !errors.Is(err, errTypedShadowUnsupported) {
		t.Fatalf("alias error = %v", err)
	}
	pdf, doc = makeDocument()
	pdf.SetWordSpacing(-1)
	if _, err := pdf.planTypedParagraphLineShadow(doc); !errors.Is(err, errTypedShadowUnsupported) {
		t.Fatalf("negative word spacing error = %v", err)
	}

	for _, test := range []struct {
		name, family, key string
		bold              bool
	}{
		{name: "Arial alias override", family: "Arial", key: "arial"},
		{name: "styled Symbol override", family: "Symbol", key: "symbolB", bold: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pdf, doc := makeDocument()
			custom, err := loadCoreFontDef("courier")
			if err != nil {
				t.Fatalf("loadCoreFontDef() = %v", err)
			}
			pdf.resources.setFont(test.key, custom)
			block := doc.Body[0].(layout.ParagraphBlock)
			block.Style.FontFamily = test.family
			block.Style.Bold = test.bold
			doc.Body[0] = block
			if _, err := pdf.planTypedParagraphLineShadow(doc); !errors.Is(err, errTypedShadowUnsupported) {
				t.Fatalf("custom override error = %v", err)
			}
		})
	}
}
