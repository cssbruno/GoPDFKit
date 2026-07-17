// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"encoding/json"
)

const (
	// GrammarVersion pins the accepted source-language contract independently
	// from the JSON projection schema used by internal tools.
	GrammarVersion          = "paper/0.1"
	ASTSchemaVersion uint16 = 1
)

// Position is a source location. Offset is a zero-based UTF-8 byte offset;
// Line and Column are one-based Unicode-code-point coordinates.
type Position struct {
	Offset uint64 `json:"offset"`
	Line   uint32 `json:"line"`
	Column uint32 `json:"column"`
}

// Span is a half-open source range [Start, End).
type Span struct {
	File  string   `json:"file"`
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// NodeKind identifies syntax-level document components.
type NodeKind string

const (
	NodeDocument    NodeKind = "document"
	NodePage        NodeKind = "page"
	NodeBody        NodeKind = "body"
	NodeHeader      NodeKind = "header"
	NodeFooter      NodeKind = "footer"
	NodeCanvas      NodeKind = "canvas"
	NodeAnchor      NodeKind = "anchor"
	NodeHeading     NodeKind = "heading"
	NodeText        NodeKind = "text"
	NodeParagraph   NodeKind = "paragraph"
	NodeList        NodeKind = "list"
	NodeItem        NodeKind = "item"
	NodePageBreak   NodeKind = "page-break"
	NodeRow         NodeKind = "row"
	NodeColumn      NodeKind = "column"
	NodeImage       NodeKind = "image"
	NodeTable       NodeKind = "table"
	NodeTableRow    NodeKind = "table-row"
	NodeTableCell   NodeKind = "cell"
	NodeTableHeader NodeKind = "table-header"
	NodeTableTrack  NodeKind = "table-track"
	NodeComponent   NodeKind = "component"
	NodeProp        NodeKind = "prop"
	NodeSlot        NodeKind = "slot"
	NodeUse         NodeKind = "use"
	NodeArg         NodeKind = "arg"
	NodeFill        NodeKind = "fill"
	NodeRepeat      NodeKind = "repeat"
	NodeLoop        NodeKind = "loop"
	NodeSchema      NodeKind = "schema"
	NodeField       NodeKind = "field"
	NodeScenario    NodeKind = "scenario"
	NodeValue       NodeKind = "value"
	NodeObject      NodeKind = "object"
	NodeKeyedList   NodeKind = "keyed-list"
	NodeTheme       NodeKind = "theme"
	NodeStyle       NodeKind = "style"
	NodeToken       NodeKind = "token"
	NodeScope       NodeKind = "scope"
)

func parseNodeKind(value string) (NodeKind, bool) {
	kind := NodeKind(value)
	switch kind {
	case NodeDocument, NodePage, NodeBody, NodeHeader, NodeFooter, NodeCanvas, NodeAnchor, NodeHeading, NodeText, NodeParagraph, NodeList, NodeItem, NodePageBreak, NodeRow, NodeColumn, NodeImage, NodeTable, NodeTableRow, NodeTableCell, NodeTableHeader, NodeTableTrack, NodeComponent, NodeProp, NodeSlot, NodeUse, NodeArg, NodeFill, NodeRepeat, NodeLoop, NodeSchema, NodeField, NodeScenario, NodeValue, NodeObject, NodeKeyedList, NodeTheme, NodeStyle, NodeToken, NodeScope:
		return kind, true
	default:
		return "", false
	}
}

// ScalarKind identifies the typed scalar encoded by a property or shorthand
// node value.
type ScalarKind string

const (
	ScalarString ScalarKind = "string"
	ScalarBool   ScalarKind = "bool"
	ScalarNumber ScalarKind = "number"
	ScalarUnit   ScalarKind = "unit"
	ScalarNull   ScalarKind = "null"
)

// UnitValue stores a numeric magnitude and canonical unit suffix.
type UnitValue struct {
	Number float64 `json:"number"`
	Unit   string  `json:"unit"`
}

// Scalar retains Raw exactly as authored. String values are decoded for
// consumers, but interpolation such as {{ customer.name }} remains ordinary
// source text and is not evaluated by this package.
type Scalar struct {
	Kind        ScalarKind `json:"kind"`
	Raw         string     `json:"raw"`
	StringValue *string    `json:"string_value,omitempty"`
	BoolValue   *bool      `json:"bool_value,omitempty"`
	NumberValue *float64   `json:"number_value,omitempty"`
	UnitValue   *UnitValue `json:"unit_value,omitempty"`
	Span        Span       `json:"span"`
}

// Property is a named typed value inside a node.
type Property struct {
	Name  string `json:"name"`
	Value Scalar `json:"value"`
	Span  Span   `json:"span"`
}

// Member preserves source order while holding exactly one node or property.
type Member struct {
	Node     *Node     `json:"node,omitempty"`
	Property *Property `json:"property,omitempty"`
}

// Node is one indentation-delimited .paper component. HeaderSpan covers only
// its declaration line; Span grows through its last descendant.
type Node struct {
	Kind       NodeKind `json:"kind"`
	ID         string   `json:"id,omitempty"`
	Value      *Scalar  `json:"value,omitempty"`
	Members    []Member `json:"members,omitempty"`
	HeaderSpan Span     `json:"header_span"`
	Span       Span     `json:"span"`
}

// AST is one parsed .paper source. A syntactically damaged input may have no
// Root while still returning useful diagnostics.
type AST struct {
	File string `json:"file"`
	Root *Node  `json:"root,omitempty"`
}

// ASTProjection is the versioned, map-free canonical syntax projection.
type ASTProjection struct {
	SchemaVersion  uint16 `json:"schema_version"`
	GrammarVersion string `json:"grammar_version"`
	File           string `json:"file"`
	Root           *Node  `json:"root,omitempty"`
}

// Projection returns a detached immutable-by-convention AST projection.
func (a AST) Projection() ASTProjection {
	return ASTProjection{
		SchemaVersion: ASTSchemaVersion, GrammarVersion: GrammarVersion,
		File: a.File, Root: cloneNode(a.Root),
	}
}

// CanonicalJSON serializes the deterministic map-free AST projection.
func (a AST) CanonicalJSON() ([]byte, error) {
	return json.Marshal(a.Projection())
}

func cloneNode(node *Node) *Node {
	if node == nil {
		return nil
	}
	cloned := *node
	cloned.Value = cloneScalar(node.Value)
	if len(node.Members) == 0 {
		cloned.Members = nil
		return &cloned
	}
	cloned.Members = make([]Member, len(node.Members))
	for index, member := range node.Members {
		cloned.Members[index].Node = cloneNode(member.Node)
		if member.Property != nil {
			property := *member.Property
			property.Value = *cloneScalar(&member.Property.Value)
			cloned.Members[index].Property = &property
		}
	}
	return &cloned
}

func cloneScalar(value *Scalar) *Scalar {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.StringValue != nil {
		copyValue := *value.StringValue
		cloned.StringValue = &copyValue
	}
	if value.BoolValue != nil {
		copyValue := *value.BoolValue
		cloned.BoolValue = &copyValue
	}
	if value.NumberValue != nil {
		copyValue := *value.NumberValue
		cloned.NumberValue = &copyValue
	}
	if value.UnitValue != nil {
		copyValue := *value.UnitValue
		cloned.UnitValue = &copyValue
	}
	return &cloned
}
