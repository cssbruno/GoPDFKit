// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// DefaultFormatMaxBytes bounds ordinary in-memory formatter output.
	DefaultFormatMaxBytes = 8 << 20
	// DefaultFormatMaxDepth prevents malformed programmatic ASTs from using
	// unbounded recursion.
	DefaultFormatMaxDepth = 256
)

var (
	ErrFormatInvalidAST  = errors.New("paperlang: AST cannot be formatted")
	ErrFormatOutputLimit = errors.New("paperlang: formatted output exceeds limit")
	ErrFormatDepthLimit  = errors.New("paperlang: AST nesting exceeds limit")
)

// FormatOptions controls bounded whole-file formatting. Zero values select
// the package defaults; negative values are invalid.
type FormatOptions struct {
	MaxBytes int
	MaxDepth int
}

// FormatError locates an unformattable semantic AST state. Path is stable and
// index-based so callers can identify programmatically built invalid nodes.
type FormatError struct {
	Path    string
	Problem string
	Cause   error
}

func (e *FormatError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("paperlang: cannot format %s: %s", e.Path, e.Problem)
}

func (e *FormatError) Unwrap() error { return e.Cause }

// Format writes one canonical .paper file from semantic AST values. Source
// spans and scalar Raw spelling are intentionally ignored: output uses two
// spaces, properties sort by name, child node order is stable, strings are
// quoted, and the file ends in one newline.
func Format(ast AST) ([]byte, error) {
	return FormatWithOptions(ast, FormatOptions{})
}

// FormatWithOptions is Format with explicit output and depth limits.
func FormatWithOptions(ast AST, options FormatOptions) ([]byte, error) {
	if options.MaxBytes < 0 || options.MaxDepth < 0 {
		return nil, &FormatError{Path: "options", Problem: "limits cannot be negative", Cause: ErrFormatInvalidAST}
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultFormatMaxBytes
	}
	if options.MaxDepth == 0 {
		options.MaxDepth = DefaultFormatMaxDepth
	}
	formatter := astFormatter{
		maxBytes: options.MaxBytes,
		maxDepth: options.MaxDepth,
		ids:      make(map[string]string),
	}
	if ast.Root == nil {
		return nil, formatter.invalid("root", "document root is nil")
	}
	if ast.Root.Kind != NodeDocument {
		return nil, formatter.invalid("root", "root kind must be document")
	}
	if err := formatter.writeNode(ast.Root, "root", 0); err != nil {
		return nil, err
	}
	return append([]byte(nil), formatter.output...), nil
}

type astFormatter struct {
	output   []byte
	maxBytes int
	maxDepth int
	ids      map[string]string
}

func (f *astFormatter) writeNode(node *Node, path string, depth int) error {
	if node == nil {
		return f.invalid(path, "node is nil")
	}
	if depth > f.maxDepth {
		return &FormatError{Path: path, Problem: fmt.Sprintf("nesting exceeds %d levels", f.maxDepth), Cause: ErrFormatDepthLimit}
	}
	if _, valid := parseNodeKind(string(node.Kind)); !valid && node.Kind != NodeField && node.Kind != NodeObjectType {
		return f.invalid(path+".kind", fmt.Sprintf("%q is not a document component", node.Kind))
	}
	if node.ID != "" {
		if !validReadableID(node.ID) {
			return f.invalid(path+".id", fmt.Sprintf("%q is not a readable @id", node.ID))
		}
		if !scopedReadableID(node.Kind) {
			if firstPath, duplicate := f.ids[node.ID]; duplicate {
				return f.invalid(path+".id", fmt.Sprintf("%s duplicates %s", node.ID, firstPath))
			}
			f.ids[node.ID] = path + ".id"
		}
	}
	if node.Kind != NodeField && (node.FieldType != "" || node.TypeRef != "" || node.ItemType != "" || node.ItemTypeRef != "" || node.Optional) {
		return f.invalid(path, "only typed schema fields can carry type or optional state")
	}

	properties := make([]*Property, 0, len(node.Members))
	children := make([]struct {
		node  *Node
		index int
	}, 0, len(node.Members))
	propertyNames := make(map[string]int)
	for index, member := range node.Members {
		memberPath := fmt.Sprintf("%s.members[%d]", path, index)
		if (member.Node == nil) == (member.Property == nil) {
			return f.invalid(memberPath, "member must contain exactly one node or property")
		}
		if member.Property != nil {
			property := member.Property
			if !validPropertyName(property.Name) && (node.Kind != NodeUse || property.Name != string(NodeComponent)) && (node.Kind != NodeFill || property.Name != string(NodeScenario)) {
				return f.invalid(memberPath+".property.name", fmt.Sprintf("%q is not a canonical property name", property.Name))
			}
			if firstIndex, duplicate := propertyNames[property.Name]; duplicate {
				return f.invalid(memberPath+".property.name", fmt.Sprintf("%q duplicates members[%d]", property.Name, firstIndex))
			}
			propertyNames[property.Name] = index
			properties = append(properties, property)
			continue
		}
		if !allowedChild(node.Kind, member.Node.Kind) {
			return f.invalid(memberPath+".node", fmt.Sprintf("%s cannot contain %s", node.Kind, member.Node.Kind))
		}
		children = append(children, struct {
			node  *Node
			index int
		}{member.Node, index})
	}

	switch node.Kind {
	case NodeText, NodeValue, NodeArg:
		if node.Value == nil {
			return f.invalid(path+".value", fmt.Sprintf("%s requires an inline scalar value", node.Kind))
		}
		if len(node.Members) != 0 {
			return f.invalid(path+".members", fmt.Sprintf("%s cannot contain members", node.Kind))
		}
	case NodePageBreak:
		if node.Value != nil {
			return f.invalid(path+".value", "page-break cannot have an inline scalar value")
		}
		if len(node.Members) != 0 {
			return f.invalid(path+".members", "page-break cannot contain members")
		}
	case NodeField:
		if node.Value != nil {
			return f.invalid(path+".value", "typed schema field cannot have an inline scalar value")
		}
		if node.TypeRef != "" {
			if node.FieldType != "" || !validSchemaTypeName(node.TypeRef) {
				return f.invalid(path+".type_ref", fmt.Sprintf("%q is not a custom object reference", node.TypeRef))
			}
		} else if _, valid := parseSchemaFieldType(string(node.FieldType)); !valid {
			return f.invalid(path+".field_type", fmt.Sprintf("%q is not a schema field type", node.FieldType))
		}
		if node.FieldType == FieldList {
			if node.ItemTypeRef != "" {
				if node.ItemType != "" || !validSchemaTypeName(node.ItemTypeRef) {
					return f.invalid(path+".item_type_ref", fmt.Sprintf("%q is not a custom object reference", node.ItemTypeRef))
				}
			} else if _, valid := parseSchemaFieldType(string(node.ItemType)); !valid || node.ItemType == FieldList {
				return f.invalid(path+".item_type", fmt.Sprintf("%q is not a supported list item type", node.ItemType))
			}
		} else if node.ItemType != "" || node.ItemTypeRef != "" {
			return f.invalid(path+".item_type", "only list fields can carry an item type")
		}
		block := node.FieldType == FieldObject || node.FieldType == FieldList
		if block && len(node.Members) == 0 {
			return f.invalid(path+".members", fmt.Sprintf("%s schema field requires indented content", node.FieldType))
		}
		if !block && len(node.Members) != 0 {
			return f.invalid(path+".members", fmt.Sprintf("%s schema field cannot contain members", node.FieldType))
		}
	default:
		if node.Value != nil {
			return f.invalid(path+".value", fmt.Sprintf("%s cannot have an inline scalar value", node.Kind))
		}
		if len(node.Members) == 0 && node.Kind != NodeScenario && node.Kind != NodeObject && node.Kind != NodeKeyedList && node.Kind != NodeTheme && node.Kind != NodeStyle && node.Kind != NodeScope {
			return f.invalid(path+".members", fmt.Sprintf("%s requires indented content", node.Kind))
		}
	}
	if node.Kind == NodeObjectType && node.ID == "" {
		return f.invalid(path+".id", "custom object requires a name")
	}
	if (node.Kind == NodeScenario || isFixtureNodeKind(node.Kind)) && node.ID == "" {
		return f.invalid(path+".id", fmt.Sprintf("%s requires a readable @name", node.Kind))
	}
	if (node.Kind == NodeTheme || node.Kind == NodeToken || node.Kind == NodeScope) && node.ID == "" {
		return f.invalid(path+".id", fmt.Sprintf("%s requires a readable @name", node.Kind))
	}
	if node.Kind == NodeStyle && node.ID == "" {
		return f.invalid(path+".id", "style requires a readable @name")
	}
	if node.Kind == NodeRepeat && node.ID == "" {
		return f.invalid(path+".id", "repeat requires a readable @name")
	}
	if node.Kind == NodeLoop && node.ID == "" {
		return f.invalid(path+".id", "loop requires a readable @name")
	}
	if (node.Kind == NodeProp || node.Kind == NodeArg) && node.ID == "" {
		return f.invalid(path+".id", fmt.Sprintf("%s requires a readable @name", node.Kind))
	}

	if err := f.writeIndent(depth); err != nil {
		return err
	}
	if node.Kind == NodeField {
		if node.Optional {
			if err := f.write("optional "); err != nil {
				return err
			}
		}
		declaration := string(node.FieldType)
		if node.TypeRef != "" {
			declaration = node.TypeRef
		}
		if node.FieldType == FieldList {
			itemDeclaration := string(node.ItemType)
			if node.ItemTypeRef != "" {
				itemDeclaration = node.ItemTypeRef
			}
			declaration += " " + itemDeclaration
		}
		if err := f.write(declaration + " " + strings.TrimPrefix(node.ID, "@")); err != nil {
			return err
		}
		if node.FieldType == FieldObject || node.FieldType == FieldList {
			if err := f.write(":"); err != nil {
				return err
			}
		}
	} else if node.Kind == NodeObjectType {
		if err := f.write("object " + strings.TrimPrefix(node.ID, "@") + ":"); err != nil {
			return err
		}
	} else {
		if err := f.write(string(node.Kind)); err != nil {
			return err
		}
		if node.ID != "" {
			id := node.ID
			if node.Kind == NodeSchema {
				id = strings.TrimPrefix(id, "@")
			}
			if err := f.write(" " + id); err != nil {
				return err
			}
		}
		if err := f.write(":"); err != nil {
			return err
		}
	}
	if node.Value != nil {
		formatted, err := formatScalar(*node.Value, path+".value")
		if err != nil {
			return err
		}
		if err := f.write(" " + formatted); err != nil {
			return err
		}
	}
	if err := f.write("\n"); err != nil {
		return err
	}

	sort.SliceStable(properties, func(left, right int) bool {
		return properties[left].Name < properties[right].Name
	})
	for _, property := range properties {
		originalIndex := propertyNames[property.Name]
		propertyPath := fmt.Sprintf("%s.members[%d].property", path, originalIndex)
		formatted, err := formatScalar(property.Value, propertyPath+".value")
		if err != nil {
			return err
		}
		if err := f.writeIndent(depth + 1); err != nil {
			return err
		}
		if err := f.write(property.Name + ": " + formatted + "\n"); err != nil {
			return err
		}
	}
	for _, child := range children {
		if err := f.writeNode(child.node, fmt.Sprintf("%s.members[%d].node", path, child.index), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func formatScalar(value Scalar, path string) (string, error) {
	nonmatching := func(problem string) (string, error) {
		return "", &FormatError{Path: path, Problem: problem, Cause: ErrFormatInvalidAST}
	}
	switch value.Kind {
	case ScalarString:
		if value.StringValue == nil || value.BoolValue != nil || value.NumberValue != nil || value.UnitValue != nil {
			return nonmatching("string scalar has inconsistent typed fields")
		}
		if !utf8.ValidString(*value.StringValue) {
			return nonmatching("string scalar is not valid UTF-8")
		}
		return strconv.Quote(*value.StringValue), nil
	case ScalarBool:
		if value.StringValue != nil || value.BoolValue == nil || value.NumberValue != nil || value.UnitValue != nil {
			return nonmatching("boolean scalar has inconsistent typed fields")
		}
		return strconv.FormatBool(*value.BoolValue), nil
	case ScalarNumber:
		if value.StringValue != nil || value.BoolValue != nil || value.NumberValue == nil || value.UnitValue != nil {
			return nonmatching("number scalar has inconsistent typed fields")
		}
		if math.IsNaN(*value.NumberValue) || math.IsInf(*value.NumberValue, 0) {
			return nonmatching("number scalar must be finite")
		}
		return strconv.FormatFloat(*value.NumberValue, 'f', -1, 64), nil
	case ScalarUnit:
		if value.StringValue != nil || value.BoolValue != nil || value.NumberValue != nil || value.UnitValue == nil {
			return nonmatching("unit scalar has inconsistent typed fields")
		}
		if math.IsNaN(value.UnitValue.Number) || math.IsInf(value.UnitValue.Number, 0) || !validUnit(value.UnitValue.Unit) {
			return nonmatching("unit scalar has a non-finite number or unsupported unit")
		}
		return strconv.FormatFloat(value.UnitValue.Number, 'f', -1, 64) + value.UnitValue.Unit, nil
	case ScalarNull:
		if value.StringValue != nil || value.BoolValue != nil || value.NumberValue != nil || value.UnitValue != nil {
			return nonmatching("null scalar has inconsistent typed fields")
		}
		return "null", nil
	default:
		return nonmatching(fmt.Sprintf("unknown scalar kind %q", value.Kind))
	}
}

func validPropertyName(name string) bool {
	if name == "" || !isIdentifierStart(name[0]) {
		return false
	}
	for index := 1; index < len(name); index++ {
		if !isIdentifierContinue(name[index]) {
			return false
		}
	}
	kind, reserved := parseNodeKind(name)
	return !reserved || isContextualNodeKind(kind)
}

func (f *astFormatter) writeIndent(depth int) error {
	if depth < 0 || depth > (f.maxBytes/2)+1 {
		return &FormatError{Path: "output", Problem: "indentation exceeds output limit", Cause: ErrFormatOutputLimit}
	}
	return f.write(strings.Repeat("  ", depth))
}

func (f *astFormatter) write(value string) error {
	if len(value) > f.maxBytes-len(f.output) {
		return &FormatError{Path: "output", Problem: fmt.Sprintf("formatted file exceeds %d bytes", f.maxBytes), Cause: ErrFormatOutputLimit}
	}
	f.output = append(f.output, value...)
	return nil
}

func (f *astFormatter) invalid(path, problem string) error {
	return &FormatError{Path: path, Problem: problem, Cause: ErrFormatInvalidAST}
}
