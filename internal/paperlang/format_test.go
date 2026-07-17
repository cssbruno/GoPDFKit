// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"bytes"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestFormatCanonicalWholeFileAndIdempotence(t *testing.T) {
	source := "document @invoice:\n    zebra: 2\n    alpha: true\n    page @first:\n        body:\n            paragraph @intro:\n                text: \"Hello ${ customer.name }\"\n            heading @title:\n                text: \"Invoice\"\n"
	parsed := Parse("messy.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse diagnostics = %#v", parsed.Diagnostics)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	want := "document @invoice:\n  alpha: true\n  zebra: 2\n  page @first:\n    body:\n      paragraph @intro:\n        text: \"Hello ${ customer.name }\"\n      heading @title:\n        text: \"Invoice\"\n"
	if string(formatted) != want {
		t.Fatalf("formatted =\n%s\nwant:\n%s", formatted, want)
	}
	reparsed := Parse("formatted.paper", string(formatted))
	if !reparsed.OK() {
		t.Fatalf("formatted Parse diagnostics = %#v", reparsed.Diagnostics)
	}
	formattedAgain, err := Format(reparsed.AST)
	if err != nil || !bytes.Equal(formattedAgain, formatted) {
		t.Fatalf("second format = %s, %v; want %s", formattedAgain, err, formatted)
	}
	if !semanticASTEqual(parsed.AST, reparsed.AST) {
		t.Fatalf("semantic AST changed across formatting:\n%#v\n%#v", parsed.AST, reparsed.AST)
	}
}

func TestFormatUnicodeEscapesAndInterpolationRoundTrip(t *testing.T) {
	value := "Olá, 世界 👋\n${customer.name} {{ invoice.total }} \"quoted\" \\ path"
	ast := minimalFormatAST(value)
	formatted, err := Format(ast)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	if !bytes.Contains(formatted, []byte("Olá, 世界 👋")) ||
		!bytes.Contains(formatted, []byte("${customer.name}")) ||
		!bytes.Contains(formatted, []byte("{{ invoice.total }}")) ||
		bytes.Contains(formatted, []byte{'\n', '$'}) {
		t.Fatalf("formatted Unicode/interpolation = %q", formatted)
	}
	parsed := Parse("unicode.paper", string(formatted))
	if !parsed.OK() {
		t.Fatalf("Parse diagnostics = %#v; source=%s", parsed.Diagnostics, formatted)
	}
	text := parsed.AST.Root.Members[0].Node.Members[0].Node.Members[0].Node.Value
	if text == nil || text.StringValue == nil || *text.StringValue != value {
		t.Fatalf("round-trip text = %#v, want %q", text, value)
	}
	second, _ := Format(parsed.AST)
	if !bytes.Equal(second, formatted) {
		t.Fatalf("format is not idempotent:\n%s\n%s", formatted, second)
	}
}

func TestFormatCanonicalNumbersAndUnitsRemainParseable(t *testing.T) {
	ast := minimalFormatAST("ok")
	root := ast.Root
	root.Members = append([]Member{
		formatProperty("tiny", Scalar{Kind: ScalarNumber, NumberValue: floatPointer(1e-20)}),
		formatProperty("huge", Scalar{Kind: ScalarNumber, NumberValue: floatPointer(1e20)}),
		formatProperty("margin", Scalar{Kind: ScalarUnit, UnitValue: &UnitValue{Number: -12.5, Unit: "mm"}}),
	}, root.Members...)
	formatted, err := Format(ast)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	if !bytes.Contains(formatted, []byte("tiny: 0.00000000000000000001")) ||
		!bytes.Contains(formatted, []byte("huge: 100000000000000000000")) || !bytes.Contains(formatted, []byte("margin: -12.5mm")) {
		t.Fatalf("canonical numeric output = %s", formatted)
	}
	parsed := Parse("numbers.paper", string(formatted))
	if !parsed.OK() || !semanticASTEqual(ast, parsed.AST) {
		t.Fatalf("number round trip = diagnostics %#v, AST %#v", parsed.Diagnostics, parsed.AST)
	}
}

func TestFormatCanonicalListSyntaxIsIdempotent(t *testing.T) {
	source := "document:\n    page:\n        body:\n            list @steps:\n                size: 10pt\n                ordered: false\n                marker: \"asterisk\"\n                item @one:\n                    text: \"One\"\n                item @two:\n                    paragraph:\n                        text: \"Two\"\n"
	parsed := Parse("list.paper", source)
	if !parsed.OK() {
		t.Fatalf("Parse() diagnostics = %#v", parsed.Diagnostics)
	}
	formatted, err := Format(parsed.AST)
	if err != nil {
		t.Fatalf("Format() = %v", err)
	}
	want := "document:\n  page:\n    body:\n      list @steps:\n        marker: \"asterisk\"\n        ordered: false\n        size: 10pt\n        item @one:\n          text: \"One\"\n        item @two:\n          paragraph:\n            text: \"Two\"\n"
	if string(formatted) != want {
		t.Fatalf("formatted =\n%s\nwant:\n%s", formatted, want)
	}
	reparsed := Parse("formatted-list.paper", string(formatted))
	if !reparsed.OK() || !semanticASTEqual(parsed.AST, reparsed.AST) {
		t.Fatalf("round trip diagnostics/AST = %#v / %#v", reparsed.Diagnostics, reparsed.AST)
	}
	second, err := Format(reparsed.AST)
	if err != nil || !bytes.Equal(second, formatted) {
		t.Fatalf("second Format() = %s, %v", second, err)
	}
}

func TestFormatRejectsUnformattableASTStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AST)
	}{
		{"nil root", func(ast *AST) { ast.Root = nil }},
		{"wrong root", func(ast *AST) { ast.Root.Kind = NodePage }},
		{"bad ID", func(ast *AST) { ast.Root.ID = "not-an-id" }},
		{"duplicate ID", func(ast *AST) { ast.Root.Members[0].Node.ID = ast.Root.ID }},
		{"empty member", func(ast *AST) { ast.Root.Members = append(ast.Root.Members, Member{}) }},
		{"both member values", func(ast *AST) {
			ast.Root.Members[0].Property = &Property{Name: "bad", Value: stringScalar("x")}
		}},
		{"bad property", func(ast *AST) {
			ast.Root.Members = append([]Member{formatProperty("bad name", stringScalar("x"))}, ast.Root.Members...)
		}},
		{"reserved property", func(ast *AST) {
			ast.Root.Members = append([]Member{formatProperty("page", stringScalar("x"))}, ast.Root.Members...)
		}},
		{"duplicate property", func(ast *AST) {
			ast.Root.Members = append([]Member{formatProperty("title", stringScalar("one")), formatProperty("title", stringScalar("two"))}, ast.Root.Members...)
		}},
		{"invalid child", func(ast *AST) { ast.Root.Members[0].Node.Kind = NodeText }},
		{"text without value", func(ast *AST) {
			ast.Root.Members[0].Node.Members[0].Node.Members[0].Node.Value = nil
		}},
		{"text with member", func(ast *AST) {
			text := ast.Root.Members[0].Node.Members[0].Node.Members[0].Node
			text.Members = []Member{formatProperty("x", stringScalar("y"))}
		}},
		{"non-text value", func(ast *AST) { ast.Root.Value = scalarPointer(stringScalar("x")) }},
		{"empty block", func(ast *AST) { ast.Root.Members = nil }},
		{"scalar mismatch", func(ast *AST) {
			text := ast.Root.Members[0].Node.Members[0].Node.Members[0].Node
			text.Value.BoolValue = boolPointer(true)
		}},
		{"invalid UTF-8", func(ast *AST) {
			text := ast.Root.Members[0].Node.Members[0].Node.Members[0].Node
			invalid := string([]byte{0xff})
			text.Value.StringValue = &invalid
		}},
		{"non-finite number", func(ast *AST) {
			text := ast.Root.Members[0].Node.Members[0].Node.Members[0].Node
			text.Value = &Scalar{Kind: ScalarNumber, NumberValue: floatPointer(math.Inf(1))}
		}},
		{"invalid unit", func(ast *AST) {
			text := ast.Root.Members[0].Node.Members[0].Node.Members[0].Node
			text.Value = &Scalar{Kind: ScalarUnit, UnitValue: &UnitValue{Number: 1, Unit: "qu"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ast := minimalFormatAST("hello")
			test.mutate(&ast)
			if _, err := Format(ast); !errors.Is(err, ErrFormatInvalidAST) {
				t.Fatalf("Format() error = %v, want ErrFormatInvalidAST", err)
			} else {
				var formatError *FormatError
				if !errors.As(err, &formatError) || formatError.Path == "" || formatError.Problem == "" {
					t.Fatalf("Format() error is not actionable: %#v", err)
				}
			}
		})
	}
}

func TestFormatEnforcesOutputAndDepthBounds(t *testing.T) {
	ast := minimalFormatAST(strings.Repeat("x", 100))
	if output, err := FormatWithOptions(ast, FormatOptions{MaxBytes: 32}); !errors.Is(err, ErrFormatOutputLimit) || output != nil {
		t.Fatalf("output limit = (%q, %v), want nil ErrFormatOutputLimit", output, err)
	}
	if output, err := FormatWithOptions(ast, FormatOptions{MaxDepth: 2}); !errors.Is(err, ErrFormatDepthLimit) || output != nil {
		t.Fatalf("depth limit = (%q, %v), want nil ErrFormatDepthLimit", output, err)
	}
	if _, err := FormatWithOptions(ast, FormatOptions{MaxBytes: -1}); !errors.Is(err, ErrFormatInvalidAST) {
		t.Fatalf("negative limit error = %v", err)
	}
}

func minimalFormatAST(text string) AST {
	return AST{File: "ignored.paper", Root: &Node{
		Kind: NodeDocument, ID: "@doc", Members: []Member{{Node: &Node{
			Kind: NodePage, Members: []Member{{Node: &Node{
				Kind: NodeBody, Members: []Member{{Node: &Node{
					Kind: NodeText, Value: scalarPointer(stringScalar(text)),
				}}},
			}}},
		}}},
	}}
}

func formatProperty(name string, value Scalar) Member {
	return Member{Property: &Property{Name: name, Value: value}}
}

func stringScalar(value string) Scalar {
	return Scalar{Kind: ScalarString, StringValue: &value}
}

func scalarPointer(value Scalar) *Scalar  { return &value }
func floatPointer(value float64) *float64 { return &value }
func boolPointer(value bool) *bool        { return &value }

func semanticASTEqual(left, right AST) bool {
	return reflect.DeepEqual(semanticNode(left.Root), semanticNode(right.Root))
}

type semanticNodeProjection struct {
	Kind       NodeKind
	ID         string
	Value      any
	Properties []semanticPropertyProjection
	Children   []semanticNodeProjection
}

type semanticPropertyProjection struct {
	Name  string
	Value any
}

func semanticNode(node *Node) semanticNodeProjection {
	if node == nil {
		return semanticNodeProjection{}
	}
	projection := semanticNodeProjection{Kind: node.Kind, ID: node.ID, Value: semanticScalar(node.Value)}
	for _, member := range node.Members {
		if member.Property != nil {
			projection.Properties = append(projection.Properties, semanticPropertyProjection{
				Name: member.Property.Name, Value: semanticScalar(&member.Property.Value),
			})
		} else if member.Node != nil {
			projection.Children = append(projection.Children, semanticNode(member.Node))
		}
	}
	sortSemanticProperties(projection.Properties)
	return projection
}

func semanticScalar(value *Scalar) any {
	if value == nil {
		return nil
	}
	switch value.Kind {
	case ScalarString:
		return struct {
			Kind ScalarKind
			Text string
		}{value.Kind, *value.StringValue}
	case ScalarBool:
		return struct {
			Kind ScalarKind
			Bool bool
		}{value.Kind, *value.BoolValue}
	case ScalarNumber:
		return struct {
			Kind   ScalarKind
			Number float64
		}{value.Kind, *value.NumberValue}
	case ScalarUnit:
		return struct {
			Kind ScalarKind
			Unit UnitValue
		}{value.Kind, *value.UnitValue}
	default:
		return value.Kind
	}
}

func sortSemanticProperties(properties []semanticPropertyProjection) {
	for index := 1; index < len(properties); index++ {
		for cursor := index; cursor > 0 && properties[cursor].Name < properties[cursor-1].Name; cursor-- {
			properties[cursor], properties[cursor-1] = properties[cursor-1], properties[cursor]
		}
	}
}
