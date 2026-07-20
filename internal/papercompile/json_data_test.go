// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
	"github.com/cssbruno/paperrune/internal/paperscenario"
	"github.com/cssbruno/paperrune/layout"
)

const jsonDataPaperSource = `document @report:
  language: "pt-BR"
  schema lab:
    object patient:
      string name
      optional string note
    list object results:
      max-items: 4
      string name
      number value
      bool critical
  page:
    size: "A4"
    margin: 20pt
    body:
      paragraph @patient-name:
        bind: "patient.name"
        text: "placeholder"
      repeat @result-lines:
        source: "results"
        instance-prefix: "results"
        max-items: 4
        paragraph @result-name:
          bind: "name"
          text: "placeholder"
`

func TestFixtureFromJSONDataValidatesAndNormalizes(t *testing.T) {
	t.Parallel()
	parsed := paperlang.Parse("lab.paper", jsonDataPaperSource)
	schemas := ExtractSchemas(parsed.AST)
	fixture, err := FixtureFromJSONData([]byte(`{
  "patient":{"name":"Ana","note":null},
  "results":[
    {"name":"Hemoglobina","value":12.50,"critical":false},
    {"name":"Plaquetas","value":2e3,"critical":true}
  ]
}`), schemas.Schemas, JSONDataOptions{Name: "input", Locale: "pt-BR"})
	if err != nil {
		t.Fatal(err)
	}
	if fixture.Name != "input" || fixture.Locale != "pt-BR" || fixture.Digest == "" || len(fixture.Values) != 2 {
		t.Fatalf("fixture = %#v", fixture)
	}
	results := fixture.Values[1].Value.List
	if len(results) != 2 || results[0].Key == results[1].Key || !strings.HasPrefix(results[0].Key, "item-") {
		t.Fatalf("result keys = %#v", results)
	}
	value := results[0].Value.Object[2].Value
	if value.Kind != paperscenario.Number || value.Number != "12.5" {
		t.Fatalf("normalized number = %#v", value)
	}
}

func TestCompileJSONDataBindsAndExpandsRepeats(t *testing.T) {
	t.Parallel()
	parsed := paperlang.Parse("lab.paper", jsonDataPaperSource)
	compiled := CompileJSONData(parsed.AST, []byte(`{"patient":{"name":"Ana"},"results":[{"name":"A","value":1,"critical":false},{"name":"B","value":2,"critical":true}]}`), JSONDataOptions{})
	if !compiled.OK() {
		t.Fatalf("diagnostics = %#v", compiled.Diagnostics)
	}
	if compiled.Locale != "pt-BR" || compiled.ScenarioDigest == "" || len(compiled.Document.Body) != 3 {
		t.Fatalf("compiled = locale %q digest %q body %#v", compiled.Locale, compiled.ScenarioDigest, compiled.Document.Body)
	}
	want := []string{"Ana", "A", "B"}
	for index, block := range compiled.Document.Body {
		paragraph, ok := block.(layout.ParagraphBlock)
		if !ok || layout.TextSegmentsPlainText(paragraph.Segments) != want[index] {
			t.Fatalf("body[%d] = %#v", index, block)
		}
	}
}

func TestCompileJSONDataExpandsRepeatedTableRows(t *testing.T) {
	t.Parallel()
	const source = `document @report:
  schema lab:
    list object results:
      max-items: 4
      string name
  page:
    body:
      table @results-table:
        repeat-header: true
        split: "rows"
        table-track:
          width: 100%
        table-header:
          table-row:
            cell:
              text: "NAME"
        repeat @result-rows:
          source: "results"
          instance-prefix: "results"
          max-items: 4
          table-row @result-row:
            cell:
              paragraph @result-name:
                bind: "name"
                text: "placeholder"
`
	parsed := paperlang.Parse("table-repeat.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics = %#v", parsed.Diagnostics)
	}
	compiled := CompileJSONData(parsed.AST, []byte(`{"results":[{"name":"A"},{"name":"B"}]}`), JSONDataOptions{})
	if !compiled.OK() || len(compiled.Document.Body) != 1 {
		t.Fatalf("compiled = %#v", compiled.Diagnostics)
	}
	table, ok := compiled.Document.Body[0].(layout.TableBlock)
	if !ok || len(table.Header) != 1 || len(table.Body) != 2 {
		t.Fatalf("table = %#v", compiled.Document.Body[0])
	}
	for index, want := range []string{"A", "B"} {
		paragraph, ok := table.Body[index].Cells[0].Blocks[0].(layout.ParagraphBlock)
		if !ok || layout.TextSegmentsPlainText(paragraph.Segments) != want {
			t.Fatalf("row %d = %#v", index, table.Body[index])
		}
	}
}

func TestFixtureFromJSONDataRejectsStructuralErrors(t *testing.T) {
	t.Parallel()
	parsed := paperlang.Parse("lab.paper", jsonDataPaperSource)
	schemas := ExtractSchemas(parsed.AST).Schemas
	tests := []struct {
		name    string
		data    string
		pointer string
		problem string
	}{
		{name: "missing", data: `{"patient":{},"results":[]}`, pointer: "/patient/name", problem: "required field is missing"},
		{name: "unknown", data: `{"patient":{"name":"Ana","extra":1},"results":[]}`, pointer: "/patient/extra", problem: "not declared"},
		{name: "type", data: `{"patient":{"name":9},"results":[]}`, pointer: "/patient/name", problem: "expected string"},
		{name: "list bound", data: `{"patient":{"name":"Ana"},"results":[{"name":"1","value":1,"critical":false},{"name":"2","value":2,"critical":false},{"name":"3","value":3,"critical":false},{"name":"4","value":4,"critical":false},{"name":"5","value":5,"critical":false}]}`, pointer: "/results", problem: "at most 4"},
		{name: "duplicate", data: `{"patient":{"name":"Ana","name":"Bia"},"results":[]}`, pointer: "/patient/name", problem: "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FixtureFromJSONData([]byte(test.data), schemas, JSONDataOptions{})
			var structured *JSONDataError
			if !errors.As(err, &structured) || structured.Pointer != test.pointer || !strings.Contains(structured.Problem, test.problem) {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestFixtureFromJSONDataRequiresExplicitSchemaWhenAmbiguous(t *testing.T) {
	t.Parallel()
	descriptor := SchemaDescriptor{Name: "@one", Kind: SchemaObject, Fields: []FieldDescriptor{{Name: "value", Kind: SchemaString, Required: true}}}
	_, err := FixtureFromJSONData([]byte(`{"value":"ok"}`), []SchemaDescriptor{descriptor, {Name: "@two", Kind: SchemaObject, Fields: descriptor.Fields}}, JSONDataOptions{})
	if err == nil || !strings.Contains(err.Error(), "multiple schemas") {
		t.Fatalf("error = %v", err)
	}
	fixture, err := FixtureFromJSONData([]byte(`{"value":"ok"}`), []SchemaDescriptor{descriptor, {Name: "@two", Kind: SchemaObject, Fields: descriptor.Fields}}, JSONDataOptions{Schema: "one"})
	if err != nil || fixture.Digest == "" {
		t.Fatalf("fixture = %#v, %v", fixture, err)
	}
}

func FuzzFixtureFromJSONDataDeterministic(f *testing.F) {
	f.Add([]byte(`{"value":"ok"}`))
	f.Add([]byte(`{"value":null}`))
	f.Add([]byte(`{"value":"first","value":"duplicate"}`))
	descriptor := SchemaDescriptor{Name: "@input", Kind: SchemaObject, Fields: []FieldDescriptor{{Name: "value", Kind: SchemaString, Required: true}}}
	f.Fuzz(func(t *testing.T, data []byte) {
		first, firstErr := FixtureFromJSONData(data, []SchemaDescriptor{descriptor}, JSONDataOptions{})
		second, secondErr := FixtureFromJSONData(data, []SchemaDescriptor{descriptor}, JSONDataOptions{})
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("nondeterministic error state: %v / %v", firstErr, secondErr)
		}
		if firstErr != nil {
			if firstErr.Error() != secondErr.Error() {
				t.Fatalf("nondeterministic errors: %q / %q", firstErr, secondErr)
			}
			return
		}
		if first.Digest == "" || first.Digest != second.Digest {
			t.Fatalf("nondeterministic fixture digest: %q / %q", first.Digest, second.Digest)
		}
	})
}

func TestCompileJSONDataTreatsNilAsAnExplicitEmptyDocument(t *testing.T) {
	t.Parallel()
	parsed := paperlang.Parse("lab.paper", jsonDataPaperSource)
	compiled := CompileJSONData(parsed.AST, nil, JSONDataOptions{})
	if compiled.OK() || !hasCompileDiagnostic(compiled.Diagnostics, "PAPER_DATA_JSON") {
		t.Fatalf("diagnostics = %#v", compiled.Diagnostics)
	}
}
