// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type PaperInsertTemplateRequest struct {
	Guard      PaperMutationGuard `json:"guard"`
	Template   string             `json:"template"`
	ID         string             `json:"id"`
	Component  string             `json:"component,omitempty"`
	ImportPath string             `json:"import_path,omitempty"`
}

// PaperInsertTemplate inserts one closed, typed starter shape beneath an
// exact layout container. The journal renders one minimal CST insertion.
func (w *Workspace) PaperInsertTemplate(request PaperInsertTemplateRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	parent := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if parent == nil || (parent.Kind != paperlang.NodeDocument && parent.Kind != paperlang.NodeBody && parent.Kind != paperlang.NodeRow && parent.Kind != paperlang.NodeColumn) {
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "template parent must be a document, body, row, or column", paperedit.ErrInvalidOperation)
	}
	if request.Template == "import" {
		if parent.Kind != paperlang.NodeDocument || !safeAuthoringImportPath(request.ImportPath) {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE", "import template requires a safe relative .paper path on the document", ErrInvalidQuery)
		}
		return w.applyPaperMutation("insert_template", request.Guard, opened, revision,
			[]string{request.Guard.Target}, []paperedit.Operation{paperedit.AppendProperty{Target: request.Guard.Target, Name: "import", Value: paperedit.StringValue(request.ImportPath)}}, "INVALID_TEMPLATE_RESULT")
	}
	if !validAuthorityNodeID(request.ID) {
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "template requires a bounded readable @id", ErrInvalidQuery)
	}
	var node paperedit.NodeSpec
	switch request.Template {
	case "schema":
		if parent.Kind != paperlang.NodeDocument {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "schema template must target the document", paperedit.ErrInvalidOperation)
		}
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 110 {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "schema ID is too long for its starter field ID", ErrInvalidQuery)
		}
		node = paperedit.NodeSpec{Kind: paperlang.NodeSchema, ID: request.ID, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeField, ID: "@" + base + "-value", Properties: []paperedit.PropertySpec{{Name: "type", Value: paperedit.StringValue("string")}}},
		}}
	case "page":
		if parent.Kind != paperlang.NodeDocument {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "page template must target the document", paperedit.ErrInvalidOperation)
		}
		for _, member := range parent.Members {
			if member.Node != nil && member.Node.Kind == paperlang.NodePage {
				return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE", "page template is only available before the document has a page", paperedit.ErrInvalidOperation)
			}
		}
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 220 {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "template ID is too long for derived readable child IDs", ErrInvalidQuery)
		}
		node = paperedit.NodeSpec{Kind: paperlang.NodePage, ID: request.ID, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeBody, ID: "@" + base + "-body", Children: []paperedit.NodeSpec{
				{Kind: paperlang.NodeParagraph, ID: "@" + base + "-copy", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("New content")}}},
			}},
		}}
	case "paragraph":
		node = paperedit.NodeSpec{Kind: paperlang.NodeParagraph, ID: request.ID, Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("New content")}}}
	case "heading":
		node = paperedit.NodeSpec{Kind: paperlang.NodeHeading, ID: request.ID, Properties: []paperedit.PropertySpec{
			{Name: "level", Value: paperedit.NumberValue(2)},
			{Name: "text", Value: paperedit.StringValue("New heading")},
		}}
	case "list":
		value := paperedit.StringValue("New item")
		node = paperedit.NodeSpec{Kind: paperlang.NodeList, ID: request.ID, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeItem, Children: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: &value}}},
		}}
	case "row", "column":
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 220 {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "template ID is too long for derived readable child IDs", ErrInvalidQuery)
		}
		kind := paperlang.NodeRow
		if request.Template == "column" {
			kind = paperlang.NodeColumn
		}
		node = paperedit.NodeSpec{Kind: kind, ID: request.ID, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeParagraph, ID: "@" + base + "-copy", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("New content")}}},
		}}
	case "page-break":
		node = paperedit.NodeSpec{Kind: paperlang.NodePageBreak, ID: request.ID}
	case "component":
		if !validAuthorityNodeID(request.Component) {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_COMPONENT", "component template requires a readable component reference", ErrInvalidQuery)
		}
		if _, err := uniqueComponentDefinition(revision.parsed.AST.Root, request.Component); err != nil {
			return PaperMutationResult{}, err
		}
		node = paperedit.NodeSpec{Kind: paperlang.NodeUse, ID: request.ID, Properties: []paperedit.PropertySpec{{Name: "component", Value: paperedit.StringValue(request.Component)}}}
	case "section":
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 220 {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "template ID is too long for derived readable child IDs", ErrInvalidQuery)
		}
		node = paperedit.NodeSpec{Kind: paperlang.NodeColumn, ID: request.ID, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeHeading, ID: "@" + base + "-heading", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("Section heading")}}},
			{Kind: paperlang.NodeParagraph, ID: "@" + base + "-body", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("New content")}}},
		}}
	default:
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE", "template must be schema, page, paragraph, heading, list, row, column, page-break, component, section, or import", ErrInvalidQuery)
	}
	return w.applyPaperMutation("insert_template", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNode{Parent: request.Guard.Target, Node: node}}, "INVALID_TEMPLATE_RESULT")
}

func safeAuthoringImportPath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "://") || strings.HasPrefix(value, "~") || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") || (len(value) > 1 && value[1] == ':') {
		return false
	}
	return path.Clean(strings.ReplaceAll(value, "\\", "/")) != "."
}

type PaperCreateScenarioRequest struct {
	Guard  PaperMutationGuard `json:"guard"`
	Name   string             `json:"name"`
	Schema string             `json:"schema"`
	Preset string             `json:"preset"`
}

type PaperScenarioMatrixCase struct {
	Name   string `json:"name"`
	Preset string `json:"preset"`
}

type PaperCreateScenarioMatrixRequest struct {
	Guard  PaperMutationGuard        `json:"guard"`
	Schema string                    `json:"schema"`
	Cases  []PaperScenarioMatrixCase `json:"cases"`
}

// PaperCreateScenarioMatrix inserts several bounded schema-shaped fixtures in
// one source patch. Matrix creation is intentionally explicit: callers name
// every case and choose its compiler-owned preset.
func (w *Workspace) PaperCreateScenarioMatrix(request PaperCreateScenarioMatrixRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	parent := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if parent == nil || parent.Kind != paperlang.NodeDocument {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_PARENT", "scenario matrix parent must be the addressed document", paperedit.ErrInvalidOperation)
	}
	if !validAuthorityNodeID(request.Schema) || len(request.Cases) == 0 || len(request.Cases) > 16 {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_MATRIX", "matrix requires one schema and between one and sixteen cases", ErrInvalidQuery)
	}
	metadata := papercompile.ExtractSchemas(revision.parsed.AST)
	if !metadata.OK() {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_SCHEMA", "schema metadata contains compiler diagnostics", ErrInvalidSource)
	}
	var schema *papercompile.SchemaDescriptor
	for index := range metadata.Schemas {
		if metadata.Schemas[index].Name == request.Schema {
			schema = &metadata.Schemas[index]
			break
		}
	}
	if schema == nil {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_SCHEMA", "selected schema does not exist in the exact source revision", ErrInvalidQuery)
	}
	seen := make(map[string]struct{}, len(request.Cases))
	nodes := make([]paperedit.NodeSpec, 0, len(request.Cases))
	for _, matrixCase := range request.Cases {
		if !validAuthorityNodeID(matrixCase.Name) || requestCasePreset(matrixCase.Preset) == "" {
			return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_MATRIX", "matrix cases require readable IDs and empty, typical, or stress presets", ErrInvalidQuery)
		}
		if _, exists := seen[matrixCase.Name]; exists || findNodeByID(revision.parsed.AST.Root, matrixCase.Name) != nil {
			return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_MATRIX", "matrix case IDs must be unique and absent from the exact source revision", paperedit.ErrInvalidOperation)
		}
		seen[matrixCase.Name] = struct{}{}
		nodes = append(nodes, paperedit.NodeSpec{Kind: paperlang.NodeScenario, ID: matrixCase.Name, Children: scenarioFieldSpecs(schema.Fields, matrixCase.Preset, 0)})
	}
	return w.applyPaperMutation("create_scenario_matrix", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNodes{Parent: request.Guard.Target, Nodes: nodes}}, "INVALID_SCENARIO_RESULT")
}

func requestCasePreset(value string) string {
	switch value {
	case "empty", "typical", "stress":
		return value
	default:
		return ""
	}
}

type PaperAddSchemaFieldRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	ItemType string             `json:"item_type,omitempty"`
	MaxItems uint32             `json:"max_items,omitempty"`
}

// PaperAddSchemaField adds one compiler-shaped field below a schema or an
// object/object-list field. Object starters receive one valid nested string
// field so the edit remains compileable in a single transaction.
func (w *Workspace) PaperAddSchemaField(request PaperAddSchemaFieldRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	parent := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if parent == nil || !validAuthorityNodeID(request.ID) {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "schema field requires an exact schema/field parent and readable @id", ErrInvalidQuery)
	}
	if parent.Kind != paperlang.NodeSchema && parent.Kind != paperlang.NodeField {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "schema field parent must be a schema or object field", paperedit.ErrInvalidOperation)
	}
	if parent.Kind == paperlang.NodeField && !schemaFieldCanContainChildren(parent) {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "nested fields require an object or object-list parent", paperedit.ErrInvalidOperation)
	}
	if sourceNodesByID(revision.parsed.AST.Root, request.ID) != nil {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "field ID already exists in the exact source revision", paperedit.ErrInvalidOperation)
	}
	fieldKind, ok := papercompile.SchemaKind(request.Type), false
	switch fieldKind {
	case papercompile.SchemaString, papercompile.SchemaNumber, papercompile.SchemaBool, papercompile.SchemaObject, papercompile.SchemaList:
		ok = true
	}
	if !ok {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "field type must be string, number, bool, object, or list", ErrInvalidQuery)
	}
	properties := []paperedit.PropertySpec{{Name: "type", Value: paperedit.StringValue(string(fieldKind))}}
	if fieldKind == papercompile.SchemaList {
		itemKind := papercompile.SchemaKind(request.ItemType)
		switch itemKind {
		case papercompile.SchemaString, papercompile.SchemaNumber, papercompile.SchemaBool, papercompile.SchemaObject:
		default:
			return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "list item type must be string, number, bool, or object", ErrInvalidQuery)
		}
		maxItems := request.MaxItems
		if maxItems == 0 {
			maxItems = 16
		}
		if maxItems > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "list max-items exceeds the schema limit", ErrLimit)
		}
		properties = append(properties,
			paperedit.PropertySpec{Name: "item-type", Value: paperedit.StringValue(string(itemKind))},
			paperedit.PropertySpec{Name: "max-items", Value: paperedit.NumberValue(float64(maxItems))})
	}
	field := paperedit.NodeSpec{Kind: paperlang.NodeField, ID: request.ID, Properties: properties}
	if fieldKind == papercompile.SchemaObject || fieldKind == papercompile.SchemaList && request.ItemType == string(papercompile.SchemaObject) {
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 110 {
			return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "object field ID is too long for its starter child", ErrInvalidQuery)
		}
		field.Children = []paperedit.NodeSpec{{Kind: paperlang.NodeField, ID: "@" + base + "-value", Properties: []paperedit.PropertySpec{{Name: "type", Value: paperedit.StringValue("string")}}}}
	}
	return w.applyPaperMutation("add_schema_field", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNode{Parent: request.Guard.Target, Node: field}}, "INVALID_SCHEMA_FIELD")
}

func schemaFieldCanContainChildren(node *paperlang.Node) bool {
	typeName, itemType := "", ""
	for _, member := range node.Members {
		if member.Property == nil || member.Property.Value.StringValue == nil {
			continue
		}
		switch member.Property.Name {
		case "type":
			typeName = *member.Property.Value.StringValue
		case "item-type":
			itemType = *member.Property.Value.StringValue
		}
	}
	return typeName == string(papercompile.SchemaObject) || typeName == string(papercompile.SchemaList) && itemType == string(papercompile.SchemaObject)
}

type PaperSetScenarioFixtureValueRequest struct {
	Guard  PaperMutationGuard `json:"guard"`
	Path   string             `json:"path"`
	Kind   string             `json:"kind,omitempty"`
	Text   string             `json:"text,omitempty"`
	Bool   *bool              `json:"bool,omitempty"`
	Number *float64           `json:"number,omitempty"`
}

// PaperSetScenarioValue edits one scalar fixture in place using a path local
// to the exact scenario root. The existing scalar kind is the default type;
// callers cannot silently change a fixture's schema contract.
func (w *Workspace) PaperSetScenarioFixtureValue(request PaperSetScenarioFixtureValueRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	scenario := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if scenario == nil || scenario.Kind != paperlang.NodeScenario || !validBindingPath(request.Path) {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_VALUE", "fixture value requires an authored scenario and relative dotted path", paperedit.ErrInvalidOperation)
	}
	node, err := scenarioRelativeNode(scenario, request.Path)
	if err != nil || node.Kind != paperlang.NodeValue || node.Value == nil {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_VALUE", "fixture path must resolve to one scalar value", paperedit.ErrInvalidOperation)
	}
	value, err := scenarioPaperValue(node.Value.Kind, request)
	if err != nil {
		return PaperMutationResult{}, err
	}
	return w.applyPaperMutation("set_scenario_value", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.SetNodeValue{Root: request.Guard.Target, Path: request.Path, Value: value}}, "INVALID_SCENARIO_VALUE")
}

func scenarioPaperValue(kind paperlang.ScalarKind, request PaperSetScenarioFixtureValueRequest) (paperedit.Value, error) {
	want := request.Kind
	if want == "" {
		want = string(kind)
	}
	if want != string(kind) {
		return paperedit.Value{}, workspaceError("INVALID_SCENARIO_VALUE", "fixture value type must match its declared scalar kind", paperedit.ErrInvalidOperation)
	}
	switch kind {
	case paperlang.ScalarString:
		return paperedit.StringValue(request.Text), nil
	case paperlang.ScalarBool:
		if request.Bool != nil {
			return paperedit.BoolValue(*request.Bool), nil
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(request.Text))
		if err != nil {
			return paperedit.Value{}, workspaceError("INVALID_SCENARIO_VALUE", "boolean fixture values must be true or false", err)
		}
		return paperedit.BoolValue(parsed), nil
	case paperlang.ScalarNumber:
		var number float64
		if request.Number != nil {
			number = *request.Number
		} else {
			parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(request.Text), 64)
			if parseErr != nil {
				return paperedit.Value{}, workspaceError("INVALID_SCENARIO_VALUE", "number fixture values must be finite decimals", parseErr)
			}
			number = parsed
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return paperedit.Value{}, workspaceError("INVALID_SCENARIO_VALUE", "number fixture values must be finite", paperedit.ErrInvalidOperation)
		}
		return paperedit.NumberValue(number), nil
	default:
		return paperedit.Value{}, workspaceError("INVALID_SCENARIO_VALUE", "null and unit fixture values are not editable by this matrix control", paperedit.ErrInvalidOperation)
	}
}

func scenarioRelativeNode(root *paperlang.Node, path string) (*paperlang.Node, error) {
	current := root
	for _, segment := range strings.Split(path, ".") {
		segment = strings.TrimSuffix(segment, "[]")
		if segment == "" {
			return nil, fmt.Errorf("empty fixture path segment")
		}
		if segment[0] != '@' {
			segment = "@" + segment
		}
		var found *paperlang.Node
		for _, member := range current.Members {
			if member.Node != nil && member.Node.ID == segment {
				if found != nil {
					return nil, fmt.Errorf("fixture path is ambiguous")
				}
				found = member.Node
			}
		}
		if found == nil {
			return nil, fmt.Errorf("fixture path segment %s is absent", segment)
		}
		current = found
	}
	return current, nil
}

type PaperManageScenarioRequest struct {
	Guard   PaperMutationGuard `json:"guard"`
	Action  string             `json:"action"`
	NewName string             `json:"new_name,omitempty"`
}

// PaperManageScenario provides the bounded lifecycle operations needed by a
// scenario matrix after creation. Rename and delete remain source edits, so
// they preserve comments and participate in the same exact-revision journal.
func (w *Workspace) PaperManageScenario(request PaperManageScenarioRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node, parent := sourceNodeAndParent(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || parent == nil || node.Kind != paperlang.NodeScenario || parent.Kind != paperlang.NodeDocument {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_TARGET", "scenario lifecycle target must be an authored scenario directly beneath the document", paperedit.ErrInvalidOperation)
	}
	var operation paperedit.Operation
	switch request.Action {
	case "rename":
		if !validAuthorityNodeID(request.NewName) || request.NewName == request.Guard.Target {
			return PaperMutationResult{}, workspaceError("INVALID_SCENARIO", "scenario rename requires a distinct readable @id", ErrInvalidQuery)
		}
		if findNodeByID(revision.parsed.AST.Root, request.NewName) != nil {
			return PaperMutationResult{}, workspaceError("INVALID_SCENARIO", "scenario ID already exists in the exact source revision", paperedit.ErrInvalidOperation)
		}
		operation = paperedit.RenameID{Target: request.Guard.Target, NewID: request.NewName}
	case "delete":
		if request.NewName != "" {
			return PaperMutationResult{}, workspaceError("INVALID_SCENARIO", "scenario delete does not accept a replacement ID", ErrInvalidQuery)
		}
		operation = paperedit.DeleteNode{Target: request.Guard.Target}
	default:
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO", "scenario action must be rename or delete", ErrInvalidQuery)
	}
	return w.applyPaperMutation("manage_scenario", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{operation}, "INVALID_SCENARIO_RESULT")
}

// PaperCreateScenario creates one schema-shaped fixture from compiler-owned
// descriptors. It does not infer schema syntax or accept arbitrary CST.
func (w *Workspace) PaperCreateScenario(request PaperCreateScenarioRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	parent := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if parent == nil || parent.Kind != paperlang.NodeDocument {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_PARENT", "scenario parent must be the addressed document", paperedit.ErrInvalidOperation)
	}
	if !validAuthorityNodeID(request.Name) || !validAuthorityNodeID(request.Schema) {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO", "scenario and schema require bounded readable @ids", ErrInvalidQuery)
	}
	if request.Preset != "empty" && request.Preset != "typical" && request.Preset != "stress" {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO", "preset must be empty, typical, or stress", ErrInvalidQuery)
	}
	metadata := papercompile.ExtractSchemas(revision.parsed.AST)
	if !metadata.OK() {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_SCHEMA", "schema metadata contains compiler diagnostics", ErrInvalidSource)
	}
	var schema *papercompile.SchemaDescriptor
	for index := range metadata.Schemas {
		if metadata.Schemas[index].Name == request.Schema {
			schema = &metadata.Schemas[index]
			break
		}
	}
	if schema == nil {
		return PaperMutationResult{}, workspaceError("INVALID_SCENARIO_SCHEMA", "selected schema does not exist in the exact source revision", ErrInvalidQuery)
	}
	node := paperedit.NodeSpec{Kind: paperlang.NodeScenario, ID: request.Name, Children: scenarioFieldSpecs(schema.Fields, request.Preset, 0)}
	return w.applyPaperMutation("create_scenario", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNode{Parent: request.Guard.Target, Node: node}}, "INVALID_SCENARIO_RESULT")
}

func scenarioFieldSpecs(fields []papercompile.FieldDescriptor, preset string, depth int) []paperedit.NodeSpec {
	result := make([]paperedit.NodeSpec, 0, len(fields))
	for _, field := range fields {
		id := "@" + field.Name
		switch field.Kind {
		case papercompile.SchemaObject:
			result = append(result, paperedit.NodeSpec{Kind: paperlang.NodeObject, ID: id, Children: scenarioFieldSpecs(field.Fields, preset, depth+1)})
		case papercompile.SchemaList:
			items := 0
			switch preset {
			case "typical":
				items = 1
			case "stress":
				items = 3
			}
			if field.MaxItems > 0 && items > int(field.MaxItems) {
				items = int(field.MaxItems)
			}
			children := make([]paperedit.NodeSpec, 0, items)
			for index := 0; index < items; index++ {
				itemID := fmt.Sprintf("@item-%d", index+1)
				if field.ItemKind == papercompile.SchemaObject {
					children = append(children, paperedit.NodeSpec{Kind: paperlang.NodeObject, ID: itemID, Children: scenarioFieldSpecs(field.Fields, preset, depth+1)})
				} else {
					value := scenarioScalar(field.ItemKind, preset, depth+1)
					children = append(children, paperedit.NodeSpec{Kind: paperlang.NodeValue, ID: itemID, Value: &value})
				}
			}
			result = append(result, paperedit.NodeSpec{Kind: paperlang.NodeKeyedList, ID: id, Children: children})
		default:
			value := scenarioScalar(field.Kind, preset, depth)
			result = append(result, paperedit.NodeSpec{Kind: paperlang.NodeValue, ID: id, Value: &value})
		}
	}
	return result
}

func scenarioScalar(kind papercompile.SchemaKind, preset string, depth int) paperedit.Value {
	switch kind {
	case papercompile.SchemaNumber:
		if preset == "stress" {
			return paperedit.NumberValue(999999.99)
		}
		if preset == "typical" {
			return paperedit.NumberValue(123.45)
		}
		return paperedit.NumberValue(0)
	case papercompile.SchemaBool:
		return paperedit.BoolValue(preset == "typical")
	default:
		if preset == "stress" {
			return paperedit.StringValue(strings.Repeat("Wide value ", 8))
		}
		if preset == "typical" {
			return paperedit.StringValue("Sample value")
		}
		return paperedit.StringValue("")
	}
}
