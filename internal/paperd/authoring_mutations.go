// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type PaperInsertTemplateRequest struct {
	Guard     PaperMutationGuard `json:"guard"`
	Template  string             `json:"template"`
	ID        string             `json:"id"`
	Component string             `json:"component,omitempty"`
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
	if !validAuthorityNodeID(request.ID) {
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "template requires a bounded readable @id", ErrInvalidQuery)
	}
	var node paperedit.NodeSpec
	switch request.Template {
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
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE", "template must be page, paragraph, heading, list, row, column, page-break, component, or section", ErrInvalidQuery)
	}
	return w.applyPaperMutation("insert_template", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNode{Parent: request.Guard.Target, Node: node}}, "INVALID_TEMPLATE_RESULT")
}

type PaperCreateScenarioRequest struct {
	Guard  PaperMutationGuard `json:"guard"`
	Name   string             `json:"name"`
	Schema string             `json:"schema"`
	Preset string             `json:"preset"`
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
			if preset == "typical" {
				items = 1
			} else if preset == "stress" {
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
