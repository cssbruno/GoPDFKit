// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"math"
	"strings"

	"github.com/cssbruno/paperrune/internal/paperlang"
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
	descriptors       []SchemaDescriptor
	byName            map[string]*SchemaDescriptor
	objectTypes       map[string]*paperlang.Node
	objectTypeOrder   []string
	resolvedTypes     map[string][]FieldDescriptor
	resolvedTypeDepth map[string]uint32
	resolvingTypes    map[string]bool
	diagnostics       []paperlang.Diagnostic
	limits            SchemaLimits
	fields            uint32
	expandedFields    uint32
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
	analysis := schemaAnalysis{
		byName:            make(map[string]*SchemaDescriptor),
		objectTypes:       make(map[string]*paperlang.Node),
		resolvedTypes:     make(map[string][]FieldDescriptor),
		resolvedTypeDepth: make(map[string]uint32),
		resolvingTypes:    make(map[string]bool),
		limits:            limits,
	}
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
		if member.Node == nil || member.Node.Kind != paperlang.NodeObjectType {
			continue
		}
		node := member.Node
		name := strings.TrimPrefix(node.ID, "@")
		if name == "" {
			analysis.add("PAPER_SCHEMA_OBJECT_NAME", "custom object requires a name", "write object Address:", node.HeaderSpan)
			continue
		}
		if _, duplicate := analysis.objectTypes[name]; duplicate {
			analysis.add("PAPER_SCHEMA_OBJECT_DUPLICATE", fmt.Sprintf("custom object %s is declared more than once", name), "keep one declaration per custom object", node.HeaderSpan)
			continue
		}
		if uint32(len(analysis.objectTypes)) >= analysis.limits.MaxSchemas { // #nosec G115 -- collection length is bounded by the configured schema limit.
			analysis.add("PAPER_SCHEMA_OBJECT_LIMIT", "custom object count exceeds the configured schema limit", "reduce custom object declarations or raise the bounded limit", node.HeaderSpan)
			continue
		}
		analysis.objectTypes[name] = node
		analysis.objectTypeOrder = append(analysis.objectTypeOrder, name)
	}
	for _, name := range analysis.objectTypeOrder {
		analysis.ensureObjectType(name, analysis.objectTypes[name].HeaderSpan)
	}
	schemaCount := 0
	for _, member := range ast.Root.Members {
		if member.Node != nil && member.Node.Kind == paperlang.NodeSchema {
			schemaCount++
		}
	}
	for _, member := range ast.Root.Members {
		if member.Node == nil || member.Node.Kind != paperlang.NodeSchema {
			continue
		}
		node := member.Node
		name := node.ID
		if name == "" {
			if schemaCount != 1 {
				analysis.add("PAPER_SCHEMA_NAME", "anonymous schema is only valid when it is the document's sole schema", "give every schema a bare name, for example schema invoice:", node.HeaderSpan)
				continue
			}
			name = "@root"
		}
		if _, duplicate := analysis.byName[name]; duplicate {
			analysis.add("PAPER_SCHEMA_DUPLICATE", fmt.Sprintf("schema %s is declared more than once", strings.TrimPrefix(name, "@")), "keep one declaration per schema", node.HeaderSpan)
			continue
		}
		if uint32(len(analysis.descriptors)) >= analysis.limits.MaxSchemas {
			analysis.add("PAPER_SCHEMA_LIMIT", "schema count exceeds the configured limit", "split declarations or raise the bounded limit", node.HeaderSpan)
			continue
		}
		descriptor := SchemaDescriptor{Name: name, Kind: SchemaObject, Source: node.Span}
		descriptor.Fields = analysis.fieldsFor(node, 1)
		analysis.descriptors = append(analysis.descriptors, descriptor)
		analysis.byName[name] = &analysis.descriptors[len(analysis.descriptors)-1]
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
			a.add("PAPER_FIELD_NAME", "typed schema field requires a name", "write string fieldName", node.HeaderSpan)
			continue
		}
		name := strings.TrimPrefix(node.ID, "@")
		if seen[name] {
			a.add("PAPER_FIELD_DUPLICATE", fmt.Sprintf("field %s is declared more than once", name), "keep one field per object scope", node.HeaderSpan)
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
	field := FieldDescriptor{Name: name, Required: !node.Optional, Source: node.Span}
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
	field.Kind = schemaKind(node.FieldType)
	if node.TypeRef != "" {
		field.Kind = SchemaObject
	}
	children := componentChildNodes(node)
	switch field.Kind {
	case SchemaObject:
		if node.TypeRef != "" {
			field.Fields = a.objectTypeFields(node.TypeRef, depth+1, node.HeaderSpan)
		} else {
			field.Fields = a.fieldsFor(node, depth+1)
		}
		if len(field.Fields) == 0 {
			hint := "add nested typed declarations"
			if node.TypeRef != "" {
				hint = "declare a non-empty custom object before using it"
			}
			a.add("PAPER_FIELD_OBJECT_EMPTY", fmt.Sprintf("object field %s has no fields", name), hint, node.HeaderSpan)
		}
	case SchemaList:
		field.ItemRequired = true
		field.ItemKind = schemaKind(node.ItemType)
		if node.ItemTypeRef != "" {
			field.ItemKind = SchemaObject
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
		if field.ItemKind == SchemaList || field.ItemKind == "" {
			a.add("PAPER_FIELD_LIST_ITEM", "list item must be string, number, bool, or object", "write the item type after list", node.HeaderSpan)
		} else if field.ItemKind == SchemaObject {
			if node.ItemTypeRef != "" {
				field.Fields = a.objectTypeFields(node.ItemTypeRef, depth+1, node.HeaderSpan)
				if len(children) != 0 {
					a.add("PAPER_FIELD_CUSTOM_CHILD", "custom object list cannot redefine item fields", "remove nested fields or use list object for an inline item shape", node.HeaderSpan)
				}
			} else {
				field.Fields = a.fieldsFor(node, depth+1)
			}
			if len(field.Fields) == 0 {
				a.add("PAPER_FIELD_OBJECT_EMPTY", fmt.Sprintf("object-list field %s has no item fields", name), "add typed fields below the list declaration", node.HeaderSpan)
			}
		} else if len(children) != 0 {
			a.add("PAPER_FIELD_PRIMITIVE_CHILD", "primitive list item cannot have nested fields", "remove the nested fields or use list object", node.HeaderSpan)
		}
	default:
		if len(children) != 0 {
			a.add("PAPER_FIELD_PRIMITIVE_CHILD", fmt.Sprintf("%s field cannot have nested fields", field.Kind), "use an object or list field", node.HeaderSpan)
		}
	}
	for name, property := range properties {
		if field.Kind != SchemaList || name != "max-items" {
			a.add("PAPER_SCHEMA_EXPRESSION_UNSUPPORTED", fmt.Sprintf("field property %q is unsupported", name), "expressions, defaults, and computed fields are not implemented", property.Span)
		}
	}
	return field
}

func (a *schemaAnalysis) ensureObjectType(name string, span paperlang.Span) []FieldDescriptor {
	if fields, resolved := a.resolvedTypes[name]; resolved {
		return fields
	}
	node := a.objectTypes[name]
	if node == nil {
		a.add("PAPER_SCHEMA_OBJECT_UNKNOWN", fmt.Sprintf("custom object %s is not declared", name), "declare object "+name+": at document scope", span)
		return nil
	}
	if a.resolvingTypes[name] {
		a.add("PAPER_SCHEMA_OBJECT_CYCLE", fmt.Sprintf("custom object cycle reaches %s", name), "remove the recursive custom object reference", span)
		return nil
	}
	a.resolvingTypes[name] = true
	fields := a.fieldsFor(node, 1)
	delete(a.resolvingTypes, name)
	a.resolvedTypes[name] = cloneFieldDescriptors(fields)
	a.resolvedTypeDepth[name] = fieldDescriptorDepth(fields)
	if len(fields) == 0 {
		a.add("PAPER_SCHEMA_OBJECT_EMPTY", fmt.Sprintf("custom object %s has no fields", name), "add at least one typed field", node.HeaderSpan)
	}
	return a.resolvedTypes[name]
}

func (a *schemaAnalysis) objectTypeFields(name string, depth uint32, span paperlang.Span) []FieldDescriptor {
	fields := a.ensureObjectType(name, span)
	if len(fields) == 0 {
		return nil
	}
	relativeDepth := a.resolvedTypeDepth[name]
	if relativeDepth != 0 && depth > a.limits.MaxDepth-relativeDepth+1 {
		a.add("PAPER_SCHEMA_DEPTH", fmt.Sprintf("custom object %s exceeds the configured depth at this field", name), "flatten the custom object graph or raise the bounded depth", span)
		return nil
	}
	count := fieldDescriptorCount(fields)
	if count > a.limits.MaxFields-a.expandedFields {
		a.add("PAPER_SCHEMA_FIELD_LIMIT", fmt.Sprintf("expanding custom object %s exceeds the configured field limit", name), "reduce custom object uses or raise the bounded limit", span)
		return nil
	}
	a.expandedFields += count
	return cloneFieldDescriptors(fields)
}

func fieldDescriptorDepth(fields []FieldDescriptor) uint32 {
	var result uint32
	for _, field := range fields {
		depth := uint32(1)
		if nested := fieldDescriptorDepth(field.Fields); nested != 0 {
			depth += nested
		}
		if depth > result {
			result = depth
		}
	}
	return result
}

func fieldDescriptorCount(fields []FieldDescriptor) uint32 {
	var result uint32
	for _, field := range fields {
		if result == ^uint32(0) {
			return result
		}
		result++
		nested := fieldDescriptorCount(field.Fields)
		if nested > ^uint32(0)-result {
			return ^uint32(0)
		}
		result += nested
	}
	return result
}

func schemaKind(fieldType paperlang.SchemaFieldType) SchemaKind {
	return SchemaKind(fieldType)
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
				analysis.add("PAPER_BIND_PATH", "bind requires a quoted path", "use field.path with one schema, schema.field with several schemas, or a component-relative path", bind.Value.Span)
			} else if strings.HasPrefix(strings.TrimSpace(*bind.Value.StringValue), "@") {
				analysis.add("PAPER_BIND_PATH", "bind paths no longer use @schema prefixes", "use a root-relative field path, or schema.field when several schemas are declared", bind.Value.Span)
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
				analysis.add("PAPER_BIND_PATH", err.Error(), "use field.path with one schema, schema.field with several schemas, and [] for list item traversal", span)
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
	if len(path) == 0 || uint64(len(path)) > uint64(limits.MaxPathBytes) || strings.ContainsAny(path, " {}()$+*/") {
		return "", false, "", false, fmt.Errorf("binding %q is not a supported path", path)
	}
	if !strings.HasPrefix(path, "@") {
		var err error
		path, err = qualifySchemaPath(path, schemas)
		if err != nil {
			return "", false, "", false, err
		}
	}
	if uint64(len(path)) > uint64(limits.MaxPathBytes) {
		return "", false, "", false, fmt.Errorf("binding path exceeds the byte limit")
	}
	parts := strings.Split(path, ".")
	if len(parts) == 0 || uint64(len(parts)-1) > uint64(limits.MaxPathSegments) {
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

func qualifySchemaPath(path string, schemas schemaAnalysis) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "@") {
		return path, nil
	}
	switch len(schemas.descriptors) {
	case 0:
		return "", fmt.Errorf("path %q requires one declared schema", path)
	case 1:
		return schemas.descriptors[0].Name + "." + path, nil
	default:
		parts := strings.SplitN(path, ".", 2)
		if len(parts) != 2 || schemas.byName["@"+parts[0]] == nil {
			return "", fmt.Errorf("path %q is ambiguous across %d schemas", path, len(schemas.descriptors))
		}
		return "@" + path, nil
	}
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
	if name == "" || !((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z')) {
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
