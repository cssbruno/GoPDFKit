// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperlang"
)

func TestApplyCommitsTypedOperationsAtomicallyBackToFront(t *testing.T) {
	source := editableFixture()
	transaction := Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{
			SetProperty{Target: "@intro", Name: "keep", Value: BoolValue(true)},
			ReplaceText{Target: "@copy", Text: "Hello agent"},
			DeleteNode{Target: "@spare"},
			SetProperty{Target: "@page", Name: "width", Value: UnitValue(210, "mm")},
		},
	}
	first, err := Apply(transaction)
	if err != nil {
		t.Fatalf("Apply() = %v, diagnostics %+v", err, first.Diagnostics)
	}
	second, err := Apply(transaction)
	if err != nil {
		t.Fatalf("second Apply() = %v", err)
	}
	want := "document @doc:\n" +
		"  language: \"en\"\n" +
		"  page @page:\n" +
		"    body @body:\n" +
		"      paragraph @intro:\n" +
		"        keep: true\n" +
		"        text @copy: \"Hello agent\"\n" +
		"    width: 210mm\n"
	if !first.Applied || first.AppliedOperations != 4 || first.Source != want ||
		first.Revision != SourceRevision(want) || len(first.Diagnostics) != 0 {
		t.Fatalf("committed result = %+v\nsource:\n%s\nwant:\n%s", first, first.Source, want)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated results differ:\n%+v\n%+v", first, second)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatalf("CanonicalJSON() = %v", err)
	}
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical results differ:\n%s\n%s", firstJSON, secondJSON)
	}
	if parsed := paperlang.Parse(transaction.File, first.Source); !parsed.OK() {
		t.Fatalf("committed candidate diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyInsertsTypedNodeAtReadableID(t *testing.T) {
	source := editableFixture()
	text := StringValue("Inserted title")
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{InsertNode{Parent: "@body", Node: NodeSpec{
			Kind: paperlang.NodeHeading, ID: "@inserted",
			Properties: []PropertySpec{{Name: "level", Value: NumberValue(2)}},
			Children:   []NodeSpec{{Kind: paperlang.NodeText, ID: "@inserted-text", Value: &text}},
		}}},
	})
	if err != nil {
		t.Fatalf("Apply(insert) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	wantBlock := "      heading @inserted:\n        level: 2\n        text @inserted-text: \"Inserted title\"\n"
	if !result.Applied || !bytes.Contains([]byte(result.Source), []byte(wantBlock)) {
		t.Fatalf("inserted source:\n%s", result.Source)
	}
	parsed := paperlang.Parse("invoice.paper", result.Source)
	if !parsed.OK() {
		t.Fatalf("inserted candidate diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyInsertsTypedListAndItems(t *testing.T) {
	source := editableFixture()
	first, second := StringValue("First"), StringValue("Second")
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{InsertNode{Parent: "@body", Node: NodeSpec{
			Kind: paperlang.NodeList, ID: "@steps",
			Properties: []PropertySpec{
				{Name: "ordered", Value: BoolValue(true)},
				{Name: "marker", Value: StringValue("decimal")},
			},
			Children: []NodeSpec{
				{Kind: paperlang.NodeItem, ID: "@first", Children: []NodeSpec{{Kind: paperlang.NodeText, Value: &first}}},
				{Kind: paperlang.NodeItem, ID: "@second", Children: []NodeSpec{{Kind: paperlang.NodeText, Value: &second}}},
			},
		}}},
	})
	if err != nil {
		t.Fatalf("Apply(list insert) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	want := "      list @steps:\n" +
		"        ordered: true\n" +
		"        marker: \"decimal\"\n" +
		"        item @first:\n" +
		"          text: \"First\"\n" +
		"        item @second:\n" +
		"          text: \"Second\"\n"
	if !result.Applied || !strings.Contains(result.Source, want) {
		t.Fatalf("inserted list source:\n%s\nwant block:\n%s", result.Source, want)
	}
	if parsed := paperlang.Parse("invoice.paper", result.Source); !parsed.OK() {
		t.Fatalf("inserted list diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyInsertsCustomObjectAndReferences(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{InsertNodes{Parent: "@doc", Nodes: []NodeSpec{
			{
				Kind: paperlang.NodeObjectType, ID: "@Address",
				Children: []NodeSpec{
					{Kind: paperlang.NodeField, ID: "@street", FieldType: paperlang.FieldString},
					{Kind: paperlang.NodeField, ID: "@city", FieldType: paperlang.FieldString},
				},
			},
			{
				Kind: paperlang.NodeSchema, ID: "@data",
				Children: []NodeSpec{
					{Kind: paperlang.NodeField, ID: "@billing", TypeRef: "Address"},
					{
						Kind: paperlang.NodeField, ID: "@previous", FieldType: paperlang.FieldList, ItemTypeRef: "Address",
						Properties: []PropertySpec{{Name: "max-items", Value: NumberValue(5)}},
					},
				},
			},
		}}},
	})
	if err != nil {
		t.Fatalf("Apply(custom objects) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	want := "  object Address:\n" +
		"    string street\n" +
		"    string city\n" +
		"  schema data:\n" +
		"    Address billing\n" +
		"    list Address previous:\n" +
		"      max-items: 5\n"
	if !strings.Contains(result.Source, want) {
		t.Fatalf("custom object insertion:\n%s\nwant block:\n%s", result.Source, want)
	}
	if parsed := paperlang.Parse("invoice.paper", result.Source); !parsed.OK() {
		t.Fatalf("custom object candidate diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyRejectsStaleRevisionWithoutPartialResult(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source + "changed"),
		Operations: []Operation{ReplaceText{Target: "@copy", Text: "must not apply"}},
	})
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("Apply(stale) error = %v, want ErrRevisionConflict", err)
	}
	if result.Applied || result.Source != source || result.Revision != SourceRevision(source) ||
		len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "PAPER_EDIT_REVISION_CONFLICT" {
		t.Fatalf("stale result = %+v", result)
	}
}

func TestApplyRollsBackAllOperationsWhenCandidateIsInvalid(t *testing.T) {
	source := editableFixture()
	badText := StringValue("nested")
	transaction := Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{
			ReplaceText{Target: "@copy", Text: "valid but rolled back"},
			InsertNode{Parent: "@body", Node: NodeSpec{
				Kind: paperlang.NodePage, ID: "@bad-page",
				Children: []NodeSpec{{Kind: paperlang.NodeBody, ID: "@bad-body", Children: []NodeSpec{{
					Kind: paperlang.NodeText, ID: "@bad-text", Value: &badText,
				}}}},
			}},
		},
	}
	first, err := Apply(transaction)
	if !errors.Is(err, ErrCandidateInvalid) {
		t.Fatalf("Apply(invalid candidate) = %v, diagnostics %+v", err, first.Diagnostics)
	}
	second, secondErr := Apply(transaction)
	if !errors.Is(secondErr, ErrCandidateInvalid) || !reflect.DeepEqual(first, second) {
		t.Fatalf("rollback is nondeterministic:\n%+v/%v\n%+v/%v", first, err, second, secondErr)
	}
	if first.Applied || first.Source != source || first.Revision != SourceRevision(source) {
		t.Fatalf("invalid candidate leaked a partial edit: %+v", first)
	}
	foundInvalidChild := false
	for _, diagnostic := range first.Diagnostics {
		foundInvalidChild = foundInvalidChild || diagnostic.Code == "PAPER_INVALID_CHILD"
	}
	if !foundInvalidChild {
		t.Fatalf("rollback diagnostics = %+v, want PAPER_INVALID_CHILD", first.Diagnostics)
	}
}

func TestApplyRejectsOverlappingOperationsWithoutPartialResult(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{
			ReplaceText{Target: "@copy", Text: "first"},
			ReplaceText{Target: "@copy", Text: "second"},
		},
	})
	if !errors.Is(err, ErrPatchConflict) {
		t.Fatalf("Apply(overlap) = %v, want ErrPatchConflict", err)
	}
	if result.Applied || result.Source != source || result.Revision != SourceRevision(source) ||
		len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "PAPER_EDIT_PATCH_CONFLICT" {
		t.Fatalf("overlap result = %+v", result)
	}
}

func TestApplyRejectsDuplicateInsertedIDAndRollsBack(t *testing.T) {
	source := editableFixture()
	text := StringValue("duplicate")
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{InsertNode{Parent: "@body", Node: NodeSpec{
			Kind: paperlang.NodeText, ID: "@copy", Value: &text,
		}}},
	})
	if !errors.Is(err, ErrCandidateInvalid) || result.Source != source || result.Applied {
		t.Fatalf("duplicate insertion = %+v, %v", result, err)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "PAPER_DUPLICATE_ID"
	}
	if !found {
		t.Fatalf("duplicate insertion diagnostics = %+v", result.Diagnostics)
	}
}

func TestApplyRenameIDRewritesOnlyTheDeclaration(t *testing.T) {
	source := strings.Replace(editableFixture(), `"Delete me"`, `"Reference @copy stays text"`, 1)
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{RenameID{Target: "@copy", NewID: "@renamed-copy"}},
	})
	if err != nil {
		t.Fatalf("Apply(rename) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	if !strings.Contains(result.Source, `text @renamed-copy: "Hello"`) ||
		!strings.Contains(result.Source, `"Reference @copy stays text"`) ||
		strings.Contains(result.Source, `text @copy: "Hello"`) {
		t.Fatalf("renamed source:\n%s", result.Source)
	}
	if parsed := paperlang.Parse("invoice.paper", result.Source); !parsed.OK() {
		t.Fatalf("renamed source diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyRenameIDDuplicateRollsBack(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{RenameID{Target: "@copy", NewID: "@spare-text"}},
	})
	if !errors.Is(err, ErrCandidateInvalid) || result.Applied || result.Source != source || result.Revision != SourceRevision(source) {
		t.Fatalf("duplicate rename = %+v, %v", result, err)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		found = found || diagnostic.Code == "PAPER_DUPLICATE_ID"
	}
	if !found {
		t.Fatalf("duplicate rename diagnostics = %+v", result.Diagnostics)
	}
}

func TestApplyMoveNodePreservesSourceAndAdjustsIndentation(t *testing.T) {
	source := strings.Replace(editableFixture(), `text @copy: "Hello"`, `text @copy: "Hello" # preserve exactly`, 1)
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{MoveNode{Target: "@copy", NewParent: "@body"}},
	})
	if err != nil {
		t.Fatalf("Apply(move) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	wantTail := "        text @spare-text: \"Delete me\"\n" +
		"      text @copy: \"Hello\" # preserve exactly\n"
	if !result.Applied || !strings.Contains(result.Source, wantTail) ||
		strings.Contains(result.Source, `        text @copy: "Hello"`) {
		t.Fatalf("moved source:\n%s", result.Source)
	}
	if parsed := paperlang.Parse("invoice.paper", result.Source); !parsed.OK() {
		t.Fatalf("moved source diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyMoveNodeIntoDescendantIsAtomicFailure(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{MoveNode{Target: "@intro", NewParent: "@copy"}},
	})
	if !errors.Is(err, ErrInvalidOperation) || result.Applied || result.Source != source || result.Revision != SourceRevision(source) {
		t.Fatalf("descendant move = %+v, %v", result, err)
	}
}

func TestApplyWrapNodePreservesTargetBlockAndCRLF(t *testing.T) {
	source := "document @doc:\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      text @loose: \"Hello\" # preserve exactly\r\n" +
		"      # following trivia remains outside\r\n"
	result, err := Apply(Transaction{
		File: "wrap.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{WrapNode{Target: "@loose", Wrapper: NodeSpec{
			Kind: paperlang.NodeParagraph, ID: "@wrapped",
			Properties: []PropertySpec{{Name: "keep", Value: BoolValue(true)}},
		}}},
	})
	if err != nil {
		t.Fatalf("Apply(wrap) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	want := "document @doc:\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      paragraph @wrapped:\r\n" +
		"        keep: true\r\n" +
		"        text @loose: \"Hello\" # preserve exactly\r\n" +
		"      # following trivia remains outside\r\n"
	if !result.Applied || result.Source != want || result.Revision != SourceRevision(want) {
		t.Fatalf("wrapped result = %+v\nsource:\n%q\nwant:\n%q", result, result.Source, want)
	}
	if parsed := paperlang.Parse("wrap.paper", result.Source); !parsed.OK() {
		t.Fatalf("wrapped candidate diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyUnwrapNodePromotesExactChildTriviaAndCRLF(t *testing.T) {
	source := "document @doc:\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      paragraph @wrapped: # remove wrapper comment\r\n" +
		"        text @first: \"One\" # exact one\r\n" +
		"        # child comment\r\n" +
		"        text @second: \"Two\"\r\n"
	result, err := Apply(Transaction{
		File: "unwrap.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{UnwrapNode{Target: "@wrapped"}},
	})
	if err != nil {
		t.Fatalf("Apply(unwrap) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	want := "document @doc:\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      text @first: \"One\" # exact one\r\n" +
		"      # child comment\r\n" +
		"      text @second: \"Two\"\r\n"
	if !result.Applied || result.Source != want {
		t.Fatalf("unwrapped source:\n%q\nwant:\n%q", result.Source, want)
	}
	if parsed := paperlang.Parse("unwrap.paper", result.Source); !parsed.OK() {
		t.Fatalf("unwrapped candidate diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyUnwrapNodeWithPropertiesIsAtomicFailure(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{UnwrapNode{Target: "@intro"}},
	})
	if !errors.Is(err, ErrInvalidOperation) || result.Applied || result.Source != source || result.Revision != SourceRevision(source) {
		t.Fatalf("property unwrap = %+v, %v", result, err)
	}
}

func TestApplyReplaceComponentPreservesSurroundingTriviaAndCRLF(t *testing.T) {
	source := "document @doc:\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      text @head: \"Head\"\r\n" +
		"      # before remains\r\n" +
		"      paragraph @old: # replaced\r\n" +
		"        text @old-text: \"Old\"\r\n" +
		"      # after remains\r\n" +
		"      text @tail: \"Tail\"\r\n"
	newText := StringValue("New")
	result, err := Apply(Transaction{
		File: "replace.paper", Source: source, ExpectedRevision: SourceRevision(source),
		Operations: []Operation{ReplaceComponent{Target: "@old", Node: NodeSpec{
			Kind: paperlang.NodeHeading, ID: "@new",
			Properties: []PropertySpec{{Name: "level", Value: NumberValue(2)}},
			Children:   []NodeSpec{{Kind: paperlang.NodeText, ID: "@new-text", Value: &newText}},
		}}},
	})
	if err != nil {
		t.Fatalf("Apply(replace) = %v, diagnostics %+v", err, result.Diagnostics)
	}
	want := "document @doc:\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      text @head: \"Head\"\r\n" +
		"      # before remains\r\n" +
		"      heading @new:\r\n" +
		"        level: 2\r\n" +
		"        text @new-text: \"New\"\r\n" +
		"      # after remains\r\n" +
		"      text @tail: \"Tail\"\r\n"
	if !result.Applied || result.Source != want {
		t.Fatalf("replaced source:\n%q\nwant:\n%q", result.Source, want)
	}
	if parsed := paperlang.Parse("replace.paper", result.Source); !parsed.OK() {
		t.Fatalf("replacement candidate diagnostics = %+v", parsed.Diagnostics)
	}
}

func TestApplyStructuralOperationsRejectSelfConflictAtomically(t *testing.T) {
	source := "document @doc:\n" +
		"  page @page:\n" +
		"    body @body:\n" +
		"      text @loose: \"Hello\"\n"
	tests := []struct {
		name       string
		operations []Operation
		want       error
	}{
		{
			name: "wrapper reuses target id",
			operations: []Operation{WrapNode{Target: "@loose", Wrapper: NodeSpec{
				Kind: paperlang.NodeParagraph, ID: "@loose",
			}}},
			want: ErrInvalidOperation,
		},
		{
			name: "overlapping wrap and replacement",
			operations: []Operation{
				WrapNode{Target: "@loose", Wrapper: NodeSpec{Kind: paperlang.NodeParagraph, ID: "@wrapper"}},
				ReplaceNode{Target: "@loose", Node: NodeSpec{Kind: paperlang.NodeText, ID: "@replacement", Value: valuePointer(StringValue("New"))}},
			},
			want: ErrPatchConflict,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Apply(Transaction{
				File: "conflict.paper", Source: source, ExpectedRevision: SourceRevision(source), Operations: test.operations,
			})
			if !errors.Is(err, test.want) || result.Applied || result.Source != source || result.Revision != SourceRevision(source) {
				t.Fatalf("structural conflict = %+v, %v; want %v", result, err, test.want)
			}
		})
	}
}

func TestMixedSemanticEditSequencePreservesTriviaAndMinimalPatches(t *testing.T) {
	source := "document @doc:\r\n" +
		"  # page-leading trivia\r\n" +
		"  page @page:\r\n" +
		"    body @body:\r\n" +
		"      # loose-leading trivia\r\n" +
		"      text @loose: \"Hello\" # loose-inline trivia\r\n" +
		"      paragraph @move:\r\n" +
		"        text @copy: \"World\" # copy-inline trivia\r\n" +
		"        text @stay: \"Remain\" # stay-inline trivia\r\n" +
		"      # body-trailing trivia\r\n"
	steps := [][]Operation{
		{WrapNode{Target: "@loose", Wrapper: NodeSpec{Kind: paperlang.NodeParagraph, ID: "@wrapped"}}},
		{SetProperty{Target: "@wrapped", Name: "keep", Value: BoolValue(true)}},
		{MoveNode{Target: "@copy", NewParent: "@body"}},
		{ReplaceText{Target: "@loose", Text: "Hello edited"}},
	}
	wantPatchCounts := []int{1, 1, 2, 1}
	for index, operations := range steps {
		before := source
		result, err := Apply(Transaction{File: "sequence.paper", Source: before,
			ExpectedRevision: SourceRevision(before), Operations: operations})
		if err != nil {
			t.Fatalf("step %d: Apply() = %v, diagnostics %+v", index+1, err, result.Diagnostics)
		}
		if !result.Applied || result.Diff == nil || len(result.Diff.Patches) != wantPatchCounts[index] {
			t.Fatalf("step %d: result = %+v, want %d minimal patches", index+1, result, wantPatchCounts[index])
		}
		if rebuilt := applyExportedPatches(t, before, result.Diff.Patches); rebuilt != result.Source {
			t.Fatalf("step %d: exported patches rebuilt %q, want %q", index+1, rebuilt, result.Source)
		}
		if parsed := paperlang.Parse("sequence.paper", result.Source); !parsed.OK() {
			t.Fatalf("step %d: diagnostics = %+v", index+1, parsed.Diagnostics)
		}
		source = result.Source
	}
	for _, exact := range []string{
		"  # page-leading trivia\r\n",
		"      # loose-leading trivia\r\n",
		`        text @loose: "Hello edited" # loose-inline trivia` + "\r\n",
		`      text @copy: "World" # copy-inline trivia` + "\r\n",
		`        text @stay: "Remain" # stay-inline trivia` + "\r\n",
		"      # body-trailing trivia\r\n",
	} {
		if !strings.Contains(source, exact) {
			t.Fatalf("final source lost or rewrote trivia %q:\n%q", exact, source)
		}
	}
	if strings.Contains(strings.ReplaceAll(source, "\r\n", ""), "\n") {
		t.Fatalf("mixed edit sequence changed CRLF policy: %q", source)
	}
}

func valuePointer(value Value) *Value { return &value }

func editableFixture() string {
	return "document @doc:\n" +
		"  language: \"en\"\n" +
		"  page @page:\n" +
		"    body @body:\n" +
		"      paragraph @intro:\n" +
		"        keep: false\n" +
		"        text @copy: \"Hello\"\n" +
		"      paragraph @spare:\n" +
		"        keep: true\n" +
		"        text @spare-text: \"Delete me\"\n"
}
