// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
	"github.com/cssbruno/gopdfkit/layout"
)

const jsonDataPaperSource = `document @report:
  language: "pt-BR"
  schema @lab:
    field @patient:
      type: "object"
      field @name:
        type: "string"
      field @note:
        type: "string"
        required: false
    field @results:
      type: "list"
      item-type: "object"
      max-items: 4
      field @name:
        type: "string"
      field @value:
        type: "number"
      field @critical:
        type: "bool"
  page:
    size: "A4"
    margin: 20pt
    body:
      paragraph @patient-name:
        bind: "@lab.patient.name"
        text: "placeholder"
      repeat @result-lines:
        source: "@lab.results"
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
