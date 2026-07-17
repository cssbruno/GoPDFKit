// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"bytes"
	"math"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompileLowersPaperASTToLayoutDocumentAndMapping(t *testing.T) {
	source := "document @invoice:\n" +
		"  title: \"Invoice\"\n" +
		"  language: \"en\"\n" +
		"  page @main:\n" +
		"    size: \"Letter\"\n" +
		"    margin: 12mm\n" +
		"    margin-left: 18mm\n" +
		"    body @content:\n" +
		"      heading @title:\n" +
		"        level: 2\n" +
		"        font: \"Helvetica\"\n" +
		"        size: 18pt\n" +
		"        line-height: 22pt\n" +
		"        align: \"center\"\n" +
		"        bold: true\n" +
		"        text @title_text: \"Hello\"\n" +
		"      paragraph @intro:\n" +
		"        font: \"Times\"\n" +
		"        size: 11pt\n" +
		"        italic: true\n" +
		"        text @first: \"First \"\n" +
		"        text @second: \"paragraph\"\n" +
		"      text @plain: \"Plain text\"\n"
	parsed := paperlang.Parse("invoice.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	before, _ := parsed.AST.CanonicalJSON()
	result := Compile(parsed.AST)
	after, _ := parsed.AST.CanonicalJSON()
	if !result.OK() || len(result.Diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("Compile mutated the paperlang AST")
	}
	if result.Document.Title != "Invoice" || result.Document.Language != "en" || result.Page != (PageSpec{Width: 612, Height: 792}) {
		t.Fatalf("document/page = %#v / %#v", result.Document, result.Page)
	}
	margin12 := 12.0 * 72 / 25.4
	margin18 := 18.0 * 72 / 25.4
	if got := result.Document.PageTemplate.Margins; got != (layout.Spacing{Top: margin12, Right: margin12, Bottom: margin12, Left: margin18}) {
		t.Fatalf("margins = %#v", got)
	}
	if len(result.Document.Body) != 3 {
		t.Fatalf("body blocks = %d", len(result.Document.Body))
	}
	heading := result.Document.Body[0].(layout.HeadingBlock)
	if heading.Level != 2 || heading.Style != (layout.TextStyle{FontFamily: "Helvetica", FontSize: 18, Bold: true, Align: "C", LineHeight: 22}) ||
		len(heading.Segments) != 1 || heading.Segments[0].Text != "Hello" {
		t.Fatalf("heading = %#v", heading)
	}
	paragraph := result.Document.Body[1].(layout.ParagraphBlock)
	if paragraph.Style.FontFamily != "Times" || paragraph.Style.FontSize != 11 || !paragraph.Style.Italic ||
		!reflect.DeepEqual(paragraph.Segments, []layout.TextSegment{{Text: "First "}, {Text: "paragraph"}}) {
		t.Fatalf("paragraph = %#v", paragraph)
	}
	plain := result.Document.Body[2].(layout.ParagraphBlock)
	if plain.Segments[0].Text != "Plain text" {
		t.Fatalf("plain text = %#v", plain)
	}
	wantMapping := []struct {
		id            string
		body, segment int
	}{
		{"@invoice", -1, -1}, {"@main", -1, -1}, {"@content", -1, -1},
		{"@title", 0, -1}, {"@title_text", 0, 0}, {"@intro", 1, -1},
		{"@first", 1, 0}, {"@second", 1, 1}, {"@plain", 2, 0},
	}
	if len(result.Mapping.Nodes) != len(wantMapping) {
		t.Fatalf("mapping = %#v", result.Mapping.Nodes)
	}
	for index, want := range wantMapping {
		got := result.Mapping.Nodes[index]
		if got.ID != want.id || got.BodyIndex != want.body || got.SegmentIndex != want.segment {
			t.Fatalf("mapping[%d] = %#v, want %#v", index, got, want)
		}
	}
}

func TestCompileSupportsNamedA3AndLegalPageSizes(t *testing.T) {
	for _, test := range []struct {
		name   string
		width  float64
		height float64
	}{{"A3", 841.88976378, 1190.551181102}, {"Legal", 612, 1008}} {
		parsed := paperlang.Parse("size.paper", "document @d:\n  page @p:\n    size: \""+test.name+"\"\n    body @b:\n")
		result := Compile(parsed.AST)
		if !result.OK() || math.Abs(result.Page.Width-test.width) > 1e-9 || math.Abs(result.Page.Height-test.height) > 1e-9 {
			t.Fatalf("%s page = %#v, diagnostics=%#v", test.name, result.Page, result.Diagnostics)
		}
	}
}

func TestCompileDiagnosticsAreDeterministicAndFirstDuplicateWins(t *testing.T) {
	source := "document:\n" +
		"  mystery: true\n" +
		"  page:\n" +
		"    width: 100pt\n" +
		"    width: 200pt\n" +
		"    height: 300pt\n" +
		"    body:\n" +
		"      paragraph:\n" +
		"        font: \"Comic Sans\"\n" +
		"        align: \"sideways\"\n" +
		"        custom: 1\n" +
		"        text: \"content\"\n"
	parsed := paperlang.Parse("bad-compile.paper", source)
	if !parsed.OK() {
		t.Fatalf("parser rejected compiler fixture: %#v", parsed.Diagnostics)
	}
	first := Compile(parsed.AST)
	second := Compile(parsed.AST)
	if first.OK() || !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) {
		t.Fatalf("diagnostics differ:\n%#v\n%#v", first.Diagnostics, second.Diagnostics)
	}
	codes := make([]string, len(first.Diagnostics))
	for index, diagnostic := range first.Diagnostics {
		codes[index] = diagnostic.Code
	}
	want := []string{
		"PAPER_COMPILE_UNSUPPORTED_PROPERTY", "PAPER_COMPILE_DUPLICATE_PROPERTY",
		"PAPER_COMPILE_UNSUPPORTED_PROPERTY", "PAPER_COMPILE_FONT", "PAPER_COMPILE_ALIGN",
	}
	if !reflect.DeepEqual(codes, want) {
		t.Fatalf("diagnostic codes = %#v, want %#v", codes, want)
	}
	if first.Page.Width != 100 {
		t.Fatalf("duplicate width selected %g, want first value 100", first.Page.Width)
	}
}

func TestCompileConvertsExplicitPhysicalPageAndMarginUnits(t *testing.T) {
	source := "document:\n  page:\n    width: 8.5in\n    height: 11in\n    margin-top: 1pc\n    margin-right: 96px\n    margin-bottom: 2.54cm\n    margin-left: 25.4mm\n    body:\n      text: \"ok\"\n"
	parsed := paperlang.Parse("units.paper", source)
	result := Compile(parsed.AST)
	if !result.OK() {
		t.Fatalf("Compile diagnostics = %#v", result.Diagnostics)
	}
	if result.Page != (PageSpec{Width: 612, Height: 792}) || result.Document.PageTemplate.Margins != (layout.Spacing{Top: 12, Right: 72, Bottom: 72, Left: 72}) {
		t.Fatalf("page/margins = %#v / %#v", result.Page, result.Document.PageTemplate.Margins)
	}
}

func TestCompileLowersOrderedAndUnorderedListsToSharedModel(t *testing.T) {
	source := "document @doc:\n" +
		"  page:\n" +
		"    body:\n" +
		"      list @ordered:\n" +
		"        marker: \"decimal\"\n" +
		"        font: \"Courier\"\n" +
		"        size: 10pt\n" +
		"        line-height: 12pt\n" +
		"        item @first:\n" +
		"          text @first_text: \"First\"\n" +
		"        item @second:\n" +
		"          paragraph @second_paragraph:\n" +
		"            bold: true\n" +
		"            text @second_text: \"Second\"\n" +
		"      list @unordered:\n" +
		"        ordered: false\n" +
		"        marker: \"asterisk\"\n" +
		"        item:\n" +
		"          text: \"Third\"\n"
	parsed := paperlang.Parse("lists.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	result := Compile(parsed.AST)
	if !result.OK() || len(result.Diagnostics) != 0 {
		t.Fatalf("Compile() diagnostics = %#v", result.Diagnostics)
	}
	if len(result.Document.Body) != 2 {
		t.Fatalf("body = %#v", result.Document.Body)
	}
	ordered := result.Document.Body[0].(layout.ListBlock)
	if !ordered.Ordered || ordered.MarkerStyle != "decimal" ||
		ordered.Style != (layout.TextStyle{FontFamily: "Courier", FontSize: 10, LineHeight: 12}) || len(ordered.Items) != 2 {
		t.Fatalf("ordered list = %#v", ordered)
	}
	first := ordered.Items[0].Blocks[0].(layout.ParagraphBlock)
	second := ordered.Items[1].Blocks[0].(layout.ParagraphBlock)
	if first.Segments[0].Text != "First" || first.Style != ordered.Style ||
		second.Segments[0].Text != "Second" || !second.Style.Bold || second.Style.FontFamily != "Courier" {
		t.Fatalf("list items = %#v / %#v", first, second)
	}
	unordered := result.Document.Body[1].(layout.ListBlock)
	if unordered.Ordered || unordered.MarkerStyle != "asterisk" || unordered.Items[0].Blocks[0].(layout.ParagraphBlock).Segments[0].Text != "Third" {
		t.Fatalf("unordered list = %#v", unordered)
	}

	wantMappings := map[string]struct{ body, segment int }{
		"@ordered": {0, -1}, "@first": {0, 0}, "@first_text": {0, 0},
		"@second": {0, 1}, "@second_paragraph": {0, 1}, "@second_text": {0, 1},
		"@unordered": {1, -1},
	}
	for _, mapping := range result.Mapping.Nodes {
		want, ok := wantMappings[mapping.ID]
		if !ok {
			continue
		}
		if mapping.BodyIndex != want.body || mapping.SegmentIndex != want.segment {
			t.Fatalf("mapping %s = %#v, want body %d segment %d", mapping.ID, mapping, want.body, want.segment)
		}
		delete(wantMappings, mapping.ID)
	}
	if len(wantMappings) != 0 {
		t.Fatalf("missing mappings = %#v", wantMappings)
	}
}

func TestCompileListMarkerInferenceAndConflictsAreDeterministic(t *testing.T) {
	valid := paperlang.Parse("inferred.paper", "document:\n  page:\n    body:\n      list:\n        marker: \"decimal\"\n        item:\n          text: \"One\"\n")
	result := Compile(valid.AST)
	if !result.OK() || !result.Document.Body[0].(layout.ListBlock).Ordered {
		t.Fatalf("inferred ordered list = %#v / %#v", result.Document, result.Diagnostics)
	}

	conflict := paperlang.Parse("conflict.paper", "document:\n  page:\n    body:\n      list:\n        ordered: true\n        marker: \"dash\"\n        item:\n          text: \"One\"\n")
	first, second := Compile(conflict.AST), Compile(conflict.AST)
	if first.OK() || !reflect.DeepEqual(first.Diagnostics, second.Diagnostics) || len(first.Diagnostics) != 1 ||
		first.Diagnostics[0].Code != "PAPER_COMPILE_LIST_MARKER_ORDER" {
		t.Fatalf("conflict diagnostics = %#v / %#v", first.Diagnostics, second.Diagnostics)
	}
}
