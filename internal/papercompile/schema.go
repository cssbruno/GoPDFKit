// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"math"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type SchemaKind string

const (
	SchemaString SchemaKind = "string"
	SchemaNumber SchemaKind = "number"
	SchemaBool   SchemaKind = "bool"
	SchemaObject SchemaKind = "object"
	SchemaList   SchemaKind = "list"
)

// FieldDescriptor is deterministic compile-time schema IR. Lists use ItemKind,
// ItemRequired, and MaxItems; object fields and object-list items use Fields.
type FieldDescriptor struct {
	Name         string
	Kind         SchemaKind
	Required     bool
	ItemKind     SchemaKind
	ItemRequired bool
	MaxItems     uint32
	Fields       []FieldDescriptor
	Source       paperlang.Span
}

type SchemaDescriptor struct {
	Name         string
	Kind         SchemaKind
	ItemKind     SchemaKind
	ItemRequired bool
	MaxItems     uint32
	Fields       []FieldDescriptor
	Source       paperlang.Span
}

// SchemaSourceResult is the deterministic schema projection used by editors
// and other read-only tooling. It reuses the compiler's schema analysis and
// never reparses or reinterprets schema syntax.
type SchemaSourceResult struct {
	Schemas     []SchemaDescriptor
	Diagnostics []paperlang.Diagnostic
}

func (result SchemaSourceResult) OK() bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Severity == paperlang.SeverityError {
			return false
		}
	}
	return true
}

// ExtractSchemas exposes immutable compiler metadata from an already parsed
// AST. Returned slices are detached from the compiler analysis.
func ExtractSchemas(ast paperlang.AST) SchemaSourceResult {
	analysis := analyzeSchemas(ast, SchemaLimits{})
	return SchemaSourceResult{
		Schemas:     cloneSchemaDescriptors(analysis.descriptors),
		Diagnostics: append([]paperlang.Diagnostic(nil), analysis.diagnostics...),
	}
}

func cloneSchemaDescriptors(input []SchemaDescriptor) []SchemaDescriptor {
	result := make([]SchemaDescriptor, len(input))
	for index, schema := range input {
		result[index] = schema
		result[index].Fields = cloneFieldDescriptors(schema.Fields)
	}
	return result
}

func cloneFieldDescriptors(input []FieldDescriptor) []FieldDescriptor {
	result := make([]FieldDescriptor, len(input))
	for index, field := range input {
		result[index] = field
		result[index].Fields = cloneFieldDescriptors(field.Fields)
	}
	return result
}

type SchemaLimits struct {
	MaxSchemas      uint32
	MaxFields       uint32
	MaxDepth        uint32
	MaxPathSegments uint32
	MaxPathBytes    uint32
	MaxListItems    uint32
}

func DefaultSchemaLimits() SchemaLimits {
	return SchemaLimits{MaxSchemas: 1024, MaxFields: 100_000, MaxDepth: 32, MaxPathSegments: 64, MaxPathBytes: 4096, MaxListItems: 1_000_000}
}

type schemaAnalysis struct {
	descriptors []SchemaDescriptor
	byName      map[string]*SchemaDescriptor
	diagnostics []paperlang.Diagnostic
	limits      SchemaLimits
	fields      uint32
}

type bindingMetadata struct {
	path       string
	span       paperlang.Span
	nullable   bool
	collection bool
	required   bool
	kind       SchemaKind
}

type bindingAnalysis struct {
	metadata    map[*paperlang.Node]bindingMetadata
	diagnostics []paperlang.Diagnostic
}

func analyzeSchemas(ast paperlang.AST, limits SchemaLimits) schemaAnalysis {
	analysis := schemaAnalysis{byName: make(map[string]*SchemaDescriptor), limits: limits}
	if limits == (SchemaLimits{}) {
		analysis.limits = DefaultSchemaLimits()
	} else if !validSchemaLimits(limits) {
		analysis.diagnostics = append(analysis.diagnostics, schemaDiagnostic("PAPER_SCHEMA_LIMITS", "schema limits are incomplete or exceed hard caps", "use positive bounded schema limits", rootSpan(ast)))
		analysis.limits = DefaultSchemaLimits()
	}
	if ast.Root == nil || ast.Root.Kind != paperlang.NodeDocument {
		return analysis
	}
	for _, member := range ast.Root.Members {
		if member.Node == nil || member.Node.Kind != paperlang.NodeSchema {
			continue
		}
		node := member.Node
		if node.ID == "" {
			analysis.add("PAPER_SCHEMA_NAME", "schema requires a readable @name", "write schema @name:", node.HeaderSpan)
			continue
		}
		if _, duplicate := analysis.byName[node.ID]; duplicate {
			analysis.add("PAPER_SCHEMA_DUPLICATE", fmt.Sprintf("schema %s is declared more than once", node.ID), "keep one declaration per schema", node.HeaderSpan)
			continue
		}
		if uint32(len(analysis.descriptors)) >= analysis.limits.MaxSchemas { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			analysis.add("PAPER_SCHEMA_LIMIT", "schema count exceeds the configured limit", "split declarations or raise the bounded limit", node.HeaderSpan)
			continue
		}
		descriptor := SchemaDescriptor{Name: node.ID, Kind: SchemaObject, Source: node.Span}
		descriptor.Fields = analysis.fieldsFor(node, 1)
		analysis.descriptors = append(analysis.descriptors, descriptor)
		analysis.byName[node.ID] = &analysis.descriptors[len(analysis.descriptors)-1]
	}
	// Slice growth can move descriptor values; rebuild stable pointers.
	analysis.byName = make(map[string]*SchemaDescriptor, len(analysis.descriptors))
	for index := range analysis.descriptors {
		analysis.byName[analysis.descriptors[index].Name] = &analysis.descriptors[index]
	}
	return analysis
}

func (a *schemaAnalysis) fieldsFor(parent *paperlang.Node, depth uint32) []FieldDescriptor {
	if depth > a.limits.MaxDepth {
		a.add("PAPER_SCHEMA_DEPTH", "schema nesting exceeds the configured depth", "flatten the schema or raise the bounded depth", parent.HeaderSpan)
		return nil
	}
	seen := make(map[string]bool)
	fields := make([]FieldDescriptor, 0)
	for _, member := range parent.Members {
		if member.Property != nil {
			if parent.Kind == paperlang.NodeSchema {
				a.add("PAPER_SCHEMA_PROPERTY", fmt.Sprintf("property %q is not supported on %s", member.Property.Name, parent.Kind), "put type properties on field declarations", member.Property.Span)
			}
			continue
		}
		if member.Node == nil || member.Node.Kind != paperlang.NodeField {
			continue
		}
		node := member.Node
		if node.ID == "" {
			a.add("PAPER_FIELD_NAME", "field requires a readable @name", "write field @name:", node.HeaderSpan)
			continue
		}
		name := strings.TrimPrefix(node.ID, "@")
		if seen[name] {
			a.add("PAPER_FIELD_DUPLICATE", fmt.Sprintf("field %s is declared more than once", node.ID), "keep one field per object scope", node.HeaderSpan)
			continue
		}
		seen[name] = true
		if a.fields >= a.limits.MaxFields {
			a.add("PAPER_SCHEMA_FIELD_LIMIT", "schema field count exceeds the configured limit", "reduce fields or raise the bounded limit", node.HeaderSpan)
			continue
		}
		a.fields++
		fields = append(fields, a.field(node, name, depth))
	}
	return fields
}

func (a *schemaAnalysis) field(node *paperlang.Node, name string, depth uint32) FieldDescriptor {
	field := FieldDescriptor{Name: name, Required: true, Source: node.Span}
	properties := make(map[string]*paperlang.Property)
	for _, member := range node.Members {
		if member.Property == nil {
			continue
		}
		property := member.Property
		if properties[property.Name] != nil {
			a.add("PAPER_FIELD_PROPERTY_DUPLICATE", fmt.Sprintf("field property %q is repeated", property.Name), "remove the duplicate", property.Span)
			continue
		}
		properties[property.Name] = property
	}
	field.Kind = a.kindProperty(properties["type"], "type", node.HeaderSpan)
	if property := properties["required"]; property != nil {
		if property.Value.Kind != paperlang.ScalarBool || property.Value.BoolValue == nil {
			a.add("PAPER_FIELD_REQUIRED", "required must be boolean", "use required: true or false", property.Value.Span)
		} else {
			field.Required = *property.Value.BoolValue
		}
	}
	children := componentChildNodes(node)
	switch field.Kind {
	case SchemaObject:
		field.Fields = a.fieldsFor(node, depth+1)
		if len(field.Fields) == 0 {
			a.add("PAPER_FIELD_OBJECT_EMPTY", fmt.Sprintf("object field @%s has no fields", name), "add nested field declarations", node.HeaderSpan)
		}
	case SchemaList:
		field.ItemRequired = true
		field.ItemKind = a.kindProperty(properties["item-type"], "item-type", node.HeaderSpan)
		if field.ItemKind == SchemaList || field.ItemKind == "" {
			a.add("PAPER_FIELD_LIST_ITEM", "list item-type must be string, number, bool, or object", "use a supported non-list item type", node.HeaderSpan)
		}
		if property := properties["max-items"]; property == nil || property.Value.Kind != paperlang.ScalarNumber || property.Value.NumberValue == nil ||
			*property.Value.NumberValue <= 0 || *property.Value.NumberValue > float64(a.limits.MaxListItems) || math.Trunc(*property.Value.NumberValue) != *property.Value.NumberValue {
			span := node.HeaderSpan
			if property != nil {
				span = property.Value.Span
			}
			a.add("PAPER_FIELD_LIST_BOUND", "list requires a positive bounded integer max-items", "add max-items within the configured limit", span)
		} else {
			field.MaxItems = uint32(*property.Value.NumberValue)
		}
		if field.ItemKind == SchemaObject {
			field.Fields = a.fieldsFor(node, depth+1)
			if len(field.Fields) == 0 {
				a.add("PAPER_FIELD_OBJECT_EMPTY", fmt.Sprintf("object-list field @%s has no item fields", name), "add nested field declarations", node.HeaderSpan)
			}
		} else if len(children) != 0 {
			a.add("PAPER_FIELD_PRIMITIVE_CHILD", "primitive list items cannot have nested fields", "remove nested fields or use item-type: \"object\"", node.HeaderSpan)
		}
	default:
		if len(children) != 0 {
			a.add("PAPER_FIELD_PRIMITIVE_CHILD", fmt.Sprintf("%s field cannot have nested fields", field.Kind), "use type: \"object\"", node.HeaderSpan)
		}
	}
	for name, property := range properties {
		if name != "type" && name != "required" && name != "item-type" && name != "max-items" {
			a.add("PAPER_SCHEMA_EXPRESSION_UNSUPPORTED", fmt.Sprintf("field property %q is unsupported", name), "expressions, defaults, and computed fields are not implemented", property.Span)
		}
	}
	return field
}

func (a *schemaAnalysis) kindProperty(property *paperlang.Property, name string, fallback paperlang.Span) SchemaKind {
	if property == nil || property.Value.Kind != paperlang.ScalarString || property.Value.StringValue == nil {
		a.add("PAPER_FIELD_TYPE", fmt.Sprintf("field requires quoted %s", name), "use string, number, bool, object, or list", fallback)
		return ""
	}
	kind := SchemaKind(strings.ToLower(strings.TrimSpace(*property.Value.StringValue)))
	if kind != SchemaString && kind != SchemaNumber && kind != SchemaBool && kind != SchemaObject && kind != SchemaList {
		a.add("PAPER_FIELD_TYPE", fmt.Sprintf("unsupported %s %q", name, kind), "use string, number, bool, object, or list", property.Value.Span)
		return ""
	}
	return kind
}

func validateBindings(ast paperlang.AST, provenance map[*paperlang.Node]expansionProvenance, schemas schemaAnalysis, limits SchemaLimits) bindingAnalysis {
	analysis := bindingAnalysis{metadata: make(map[*paperlang.Node]bindingMetadata)}
	limits = schemas.limits
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil {
			return
		}
		var bind *paperlang.Property
		required := true
		if inherited := provenance[node]; inherited.bindingBase != "" {
			required = inherited.bindingRequired
		}
		for _, member := range node.Members {
			if member.Property == nil {
				continue
			}
			switch member.Property.Name {
			case "bind":
				bind = member.Property
			case "bind-required":
				if member.Property.Value.Kind != paperlang.ScalarBool || member.Property.Value.BoolValue == nil {
					analysis.add("PAPER_BIND_REQUIRED", "bind-required must be boolean", "use true or false", member.Property.Value.Span)
				} else {
					required = *member.Property.Value.BoolValue
				}
			}
		}
		base := provenance[node].bindingBase
		path, span := "", provenance[node].bindingSpan
		if bind != nil {
			if node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading {
				analysis.add("PAPER_BIND_TARGET", fmt.Sprintf("bind is unsupported on %s", node.Kind), "bind paragraphs/headings or component use instances", bind.Span)
			} else if bind.Value.Kind != paperlang.ScalarString || bind.Value.StringValue == nil {
				analysis.add("PAPER_BIND_PATH", "bind requires a quoted path", "use @schema.field or a relative component path", bind.Value.Span)
			} else {
				path = combineBindingPath(base, *bind.Value.StringValue)
				span = bind.Value.Span
			}
		}
		if path != "" {
			canonical, nullable, kind, collection, err := resolveBindingPath(path, schemas, limits)
			if provenance[node].repeatItem {
				collection = false
			}
			if err != nil {
				analysis.add("PAPER_BIND_PATH", err.Error(), "use a declared path and [] for list item traversal", span)
			} else if kind == SchemaObject || kind == SchemaList {
				analysis.add("PAPER_BIND_TARGET_TYPE", fmt.Sprintf("text binding %s terminates at %s", canonical, kind), "bind to a primitive string, number, or bool field", span)
			} else if collection {
				analysis.add("PAPER_BIND_COLLECTION", fmt.Sprintf("text binding %s is collection-valued", canonical), "collection evaluation requires a future bounded repeat construct", span)
			} else if nullable && required {
				analysis.add("PAPER_BIND_NULLABLE", fmt.Sprintf("nullable path %s is used as required", canonical), "set bind-required: false or make every traversed field required", span)
			} else {
				analysis.metadata[node] = bindingMetadata{path: canonical, span: span, nullable: nullable, collection: collection, required: required, kind: kind}
			}
		} else if bind == nil {
			for _, member := range node.Members {
				if member.Property == nil {
					continue
				}
				if member.Property.Name == "bind-required" {
					analysis.add("PAPER_BIND_REQUIRED", "bind-required has no bind path", "add bind or remove bind-required", member.Property.Span)
				} else if strings.HasPrefix(member.Property.Name, "format") {
					analysis.add("PAPER_BIND_FORMAT", member.Property.Name+" has no bind path", "add bind or remove the formatting property", member.Property.Span)
				}
			}
		}
		for _, member := range node.Members {
			walk(member.Node)
		}
	}
	walk(ast.Root)
	return analysis
}

type bindingSegment struct {
	name string
	list bool
}

func resolveBindingPath(path string, schemas schemaAnalysis, limits SchemaLimits) (string, bool, SchemaKind, bool, error) {
	path = strings.TrimSpace(path)
	if len(path) == 0 || uint64(len(path)) > uint64(limits.MaxPathBytes) || !strings.HasPrefix(path, "@") || strings.ContainsAny(path, " {}()$+*/") {
		return "", false, "", false, fmt.Errorf("binding %q is not a supported path", path)
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 || uint64(len(parts)-1) > uint64(limits.MaxPathSegments) { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		return "", false, "", false, fmt.Errorf("binding path exceeds the segment limit")
	}
	schema := schemas.byName[parts[0]]
	if schema == nil {
		return "", false, "", false, fmt.Errorf("schema %s is not declared", parts[0])
	}
	fields := schema.Fields
	current := SchemaObject
	nullable := false
	collection := false
	for index, raw := range parts[1:] {
		segment := bindingSegment{name: raw}
		if strings.HasSuffix(segment.name, "[]") {
			segment.list = true
			segment.name = strings.TrimSuffix(segment.name, "[]")
		}
		if !validBindingName(segment.name) || current != SchemaObject {
			return "", false, "", false, fmt.Errorf("segment %q cannot be traversed from %s", raw, current)
		}
		field := findSchemaField(fields, segment.name)
		if field == nil {
			return "", false, "", false, fmt.Errorf("field %q is not declared", segment.name)
		}
		if !field.Required {
			nullable = true
		}
		if segment.list {
			if field.Kind != SchemaList {
				return "", false, "", false, fmt.Errorf("segment %q uses [] on a non-list field", raw)
			}
			collection = true
			current, fields = field.ItemKind, field.Fields
		} else {
			current, fields = field.Kind, field.Fields
			if field.Kind == SchemaList && index+1 < len(parts)-1 {
				return "", false, "", false, fmt.Errorf("list field %q requires [] before item traversal", field.Name)
			}
		}
	}
	return path, nullable, current, collection, nil
}

func findSchemaField(fields []FieldDescriptor, name string) *FieldDescriptor {
	for index := range fields {
		if fields[index].Name == name {
			return &fields[index]
		}
	}
	return nil
}

func validBindingName(name string) bool {
	if name == "" || (name[0] < 'A' || name[0] > 'Z') && (name[0] < 'a' || name[0] > 'z') {
		return false
	}
	for index := 1; index < len(name); index++ {
		c := name[index]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func validSchemaLimits(limits SchemaLimits) bool {
	return limits.MaxSchemas > 0 && limits.MaxSchemas <= 4096 && limits.MaxFields > 0 && limits.MaxFields <= 1_000_000 &&
		limits.MaxDepth > 0 && limits.MaxDepth <= 128 && limits.MaxPathSegments > 0 && limits.MaxPathSegments <= 256 &&
		limits.MaxPathBytes > 0 && limits.MaxPathBytes <= 16_384 && limits.MaxListItems > 0 && limits.MaxListItems <= 10_000_000
}

func (a *schemaAnalysis) add(code, message, hint string, span paperlang.Span) {
	a.diagnostics = append(a.diagnostics, schemaDiagnostic(code, message, hint, span))
}

func (a *bindingAnalysis) add(code, message, hint string, span paperlang.Span) {
	a.diagnostics = append(a.diagnostics, schemaDiagnostic(code, message, hint, span))
}

func schemaDiagnostic(code, message, hint string, span paperlang.Span) paperlang.Diagnostic {
	return paperlang.Diagnostic{Code: code, Severity: paperlang.SeverityError, Message: message, Hint: hint, Span: span}
}
