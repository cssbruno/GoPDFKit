// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestParseDocumentFoundationPreservesTypesOrderAndInterpolation(t *testing.T) {
	source := strings.Join([]string{
		"document @invoice:",
		"  language: \"en\"",
		"  draft: false",
		"  copies: 2",
		"  page @first:",
		"    width: 210mm",
		"    body:",
		"      heading @title:",
		"        level: 1",
		"        text: \"Invoice {{ customer.name }}\"",
		"      paragraph @intro:",
		"        keep: true",
		"        text: \"Hello\"",
		"      text @footer: \"Page {{ page.number }}\"",
	}, "\n") + "\n"

	result := Parse("invoice.paper", source)
	if !result.OK() {
		t.Fatalf("Parse diagnostics = %#v", result.Diagnostics)
	}
	root := result.AST.Root
	if root.Kind != NodeDocument || root.ID != "@invoice" || len(root.Members) != 4 {
		t.Fatalf("root = %#v", root)
	}
	if got := []string{
		root.Members[0].Property.Name,
		root.Members[1].Property.Name,
		root.Members[2].Property.Name,
		string(root.Members[3].Node.Kind),
	}; !reflect.DeepEqual(got, []string{"language", "draft", "copies", "page"}) {
		t.Fatalf("root member order = %#v", got)
	}
	if got := *root.Members[0].Property.Value.StringValue; got != "en" {
		t.Fatalf("language = %q", got)
	}
	if got := *root.Members[1].Property.Value.BoolValue; got {
		t.Fatal("draft = true, want false")
	}
	if got := *root.Members[2].Property.Value.NumberValue; got != 2 {
		t.Fatalf("copies = %v", got)
	}
	page := root.Members[3].Node
	unit := page.Members[0].Property.Value
	if unit.Kind != ScalarUnit || unit.Raw != "210mm" || unit.UnitValue == nil || *unit.UnitValue != (UnitValue{Number: 210, Unit: "mm"}) {
		t.Fatalf("width unit = %#v", unit)
	}
	body := page.Members[1].Node
	headingText := body.Members[0].Node.Members[1].Node.Value
	if headingText.Raw != `"Invoice {{ customer.name }}"` || *headingText.StringValue != "Invoice {{ customer.name }}" {
		t.Fatalf("interpolated string = %#v", headingText)
	}
	footer := body.Members[2].Node
	if footer.ID != "@footer" || footer.Value == nil || *footer.Value.StringValue != "Page {{ page.number }}" {
		t.Fatalf("footer = %#v", footer)
	}
	if root.Span.Start.Offset != 0 || root.Span.End.Offset != uint64(len(source)-1) ||
		root.Span.End.Line != 14 || root.Span.End.Column != 45 {
		t.Fatalf("root span = %#v, source length %d", root.Span, len(source))
	}
}

func TestASTProjectionIsDetachedAndDeterministic(t *testing.T) {
	source := "document @doc:\n  page:\n    body:\n      text: \"Hello {{ name }}\"\n"
	first := Parse("stable.paper", source)
	second := Parse("stable.paper", source)
	if !first.OK() || !second.OK() {
		t.Fatalf("parse diagnostics = %#v / %#v", first.Diagnostics, second.Diagnostics)
	}
	firstJSON, err := first.AST.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	secondJSON, _ := second.AST.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) || !bytes.Contains(firstJSON, []byte(`"schema_version":1`)) ||
		!bytes.Contains(firstJSON, []byte(`"grammar_version":"paper/0.3"`)) {
		t.Fatalf("canonical JSON differs:\n%s\n%s", firstJSON, secondJSON)
	}
	var decoded ASTProjection
	if err := json.Unmarshal(firstJSON, &decoded); err != nil {
		t.Fatalf("Unmarshal() = %v", err)
	}
	roundTrip, err := json.Marshal(decoded)
	if err != nil || !bytes.Equal(roundTrip, firstJSON) {
		t.Fatalf("round trip = %s, %v; want %s", roundTrip, err, firstJSON)
	}

	projection := first.AST.Projection()
	projection.Root.ID = "@mutated"
	text := projection.Root.Members[0].Node.Members[0].Node.Members[0].Node.Value
	*text.StringValue = "mutated"
	if got := first.AST.Projection().Root; got.ID != "@doc" || *got.Members[0].Node.Members[0].Node.Members[0].Node.Value.StringValue != "Hello {{ name }}" {
		t.Fatal("projection exposed mutable AST storage")
	}
}

func TestParseScalarKinds(t *testing.T) {
	source := "document:\n  string_value: \"line\\n{{ raw }}\"\n  yes: true\n  no: false\n  integer: -12\n  decimal: +0.25\n  margin: 12.5pt\n  ratio: 80%\n  page:\n    body:\n      text: \"ok\"\n"
	result := Parse("values.paper", source)
	if !result.OK() {
		t.Fatalf("Parse diagnostics = %#v", result.Diagnostics)
	}
	members := result.AST.Root.Members
	if got := *members[0].Property.Value.StringValue; got != "line\n{{ raw }}" {
		t.Fatalf("decoded string = %q", got)
	}
	if got := members[0].Property.Value.Raw; got != `"line\n{{ raw }}"` {
		t.Fatalf("raw string = %q", got)
	}
	if !*members[1].Property.Value.BoolValue || *members[2].Property.Value.BoolValue {
		t.Fatal("boolean values were not typed")
	}
	if *members[3].Property.Value.NumberValue != -12 || *members[4].Property.Value.NumberValue != .25 {
		t.Fatal("number values were not typed")
	}
	if got := members[5].Property.Value.UnitValue; got == nil || *got != (UnitValue{Number: 12.5, Unit: "pt"}) {
		t.Fatalf("margin = %#v", got)
	}
	if got := members[6].Property.Value.UnitValue; got == nil || *got != (UnitValue{Number: 80, Unit: "%"}) {
		t.Fatalf("ratio = %#v", got)
	}
}

func TestParsePreservesDollarInterpolationAsAuthored(t *testing.T) {
	source := "document:\n  page:\n    body:\n      text @welcome: \"Hello ${customer.name}, total ${invoice.total}\"\n"
	result := Parse("interpolation.paper", source)
	if !result.OK() {
		t.Fatalf("Parse diagnostics = %#v", result.Diagnostics)
	}
	text := result.AST.Root.Members[0].Node.Members[0].Node.Members[0].Node.Value
	if text.Raw != `"Hello ${customer.name}, total ${invoice.total}"` ||
		*text.StringValue != "Hello ${customer.name}, total ${invoice.total}" {
		t.Fatalf("interpolation = %#v", text)
	}
	encoded, err := result.AST.CanonicalJSON()
	if err != nil || !bytes.Contains(encoded, []byte(`${customer.name}`)) || !bytes.Contains(encoded, []byte(`${invoice.total}`)) {
		t.Fatalf("CanonicalJSON() = %s, %v", encoded, err)
	}
}

func TestParseListAndItemsPreservesReadableStructureAndOrder(t *testing.T) {
	source := "document @doc:\n" +
		"  page:\n" +
		"    body:\n" +
		"      list @steps:\n" +
		"        ordered: true\n" +
		"        marker: \"decimal\"\n" +
		"        item @first:\n" +
		"          text: \"Measure twice\"\n" +
		"        item @second:\n" +
		"          paragraph:\n" +
		"            text: \"Cut once\"\n"
	result := Parse("list.paper", source)
	if !result.OK() {
		t.Fatalf("Parse() diagnostics = %#v", result.Diagnostics)
	}
	body := result.AST.Root.Members[0].Node.Members[0].Node
	list := body.Members[0].Node
	if list.Kind != NodeList || list.ID != "@steps" || len(list.Members) != 4 {
		t.Fatalf("list = %#v", list)
	}
	if list.Members[0].Property.Name != "ordered" || !*list.Members[0].Property.Value.BoolValue ||
		list.Members[1].Property.Name != "marker" || *list.Members[1].Property.Value.StringValue != "decimal" {
		t.Fatalf("list properties = %#v", list.Members[:2])
	}
	first, second := list.Members[2].Node, list.Members[3].Node
	if first.Kind != NodeItem || first.ID != "@first" || second.Kind != NodeItem || second.ID != "@second" ||
		first.Members[0].Node.Kind != NodeText || second.Members[0].Node.Kind != NodeParagraph {
		t.Fatalf("items = %#v / %#v", first, second)
	}
}

func TestParseRejectsListItemsOutsideListsAndNonItemsInsideLists(t *testing.T) {
	source := "document:\n  page:\n    body:\n      item:\n        text: \"orphan\"\n      list:\n        paragraph:\n          text: \"not an item\"\n"
	result := Parse("bad-list.paper", source)
	if result.OK() || !diagnosticCodes(result.Diagnostics)["PAPER_INVALID_CHILD"] {
		t.Fatalf("Parse() diagnostics = %#v", result.Diagnostics)
	}
}

func TestParseReportsHelpfulStructuralDiagnostics(t *testing.T) {
	source := strings.Join([]string{
		"document @same:",
		"  page @same:",
		"    title: bare",
		"    width: 10qu",
		"    paragraph:",
		"      text:",
		"        body:",
		"          text: \"nested\"",
	}, "\n") + "\n"
	result := Parse("broken.paper", source)
	repeated := Parse("broken.paper", source)
	if !reflect.DeepEqual(result.Diagnostics, repeated.Diagnostics) {
		t.Fatalf("diagnostics are not deterministic:\n%#v\n%#v", result.Diagnostics, repeated.Diagnostics)
	}
	if result.OK() {
		t.Fatal("broken source unexpectedly parsed without errors")
	}
	codes := diagnosticCodes(result.Diagnostics)
	for _, want := range []string{
		"PAPER_DUPLICATE_ID", "PAPER_EXPECTED_VALUE", "PAPER_INVALID_UNIT",
		"PAPER_INVALID_CHILD", "PAPER_TEXT_VALUE", "PAPER_TEXT_BLOCK",
	} {
		if !codes[want] {
			t.Fatalf("diagnostic codes = %#v, want %s; diagnostics=%#v", codes, want, result.Diagnostics)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity != SeverityError || diagnostic.Message == "" || diagnostic.Span.File != "broken.paper" {
			t.Fatalf("unhelpful diagnostic = %#v", diagnostic)
		}
	}
}

func TestParseRequiresSingleDocumentRoot(t *testing.T) {
	for _, test := range []struct {
		name   string
		source string
		code   string
	}{
		{"empty", "", "PAPER_EMPTY_DOCUMENT"},
		{"wrong root", "page:\n  body:\n    text: \"x\"\n", "PAPER_ROOT_DOCUMENT"},
		{"multiple roots", "document:\n  page:\n    body:\n      text: \"x\"\npage:\n  body:\n    text: \"y\"\n", "PAPER_MULTIPLE_ROOTS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := Parse("root.paper", test.source)
			if !diagnosticCodes(result.Diagnostics)[test.code] {
				t.Fatalf("diagnostics = %#v, want %s", result.Diagnostics, test.code)
			}
		})
	}
}
