// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

type PaperInsertTemplateRequest struct {
	Guard      PaperMutationGuard `json:"guard"`
	Template   string             `json:"template"`
	ID         string             `json:"id"`
	Component  string             `json:"component,omitempty"`
	ImportPath string             `json:"import_path,omitempty"`
	Preset     string             `json:"preset,omitempty"`
	Path       string             `json:"path,omitempty"`
}

// PaperInsertTemplate inserts one closed, typed starter shape beneath an
// exact layout container. The journal renders one minimal CST insertion.
func (w *Workspace) PaperInsertTemplate(request PaperInsertTemplateRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	parent := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if parent == nil || (parent.Kind != paperlang.NodeDocument && parent.Kind != paperlang.NodePage && parent.Kind != paperlang.NodeBody && parent.Kind != paperlang.NodeHeader && parent.Kind != paperlang.NodeFooter && parent.Kind != paperlang.NodeRow && parent.Kind != paperlang.NodeColumn) {
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "template parent must be a document, page, region, body, row, or column", paperedit.ErrInvalidOperation)
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
			{Kind: paperlang.NodeField, ID: "@" + base + "-value", FieldType: paperlang.FieldString},
		}}
	case "schema-object":
		if parent.Kind != paperlang.NodeDocument {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "custom object template must target the document", paperedit.ErrInvalidOperation)
		}
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 110 {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_ID", "custom object ID is too long for its starter field ID", ErrInvalidQuery)
		}
		node = paperedit.NodeSpec{Kind: paperlang.NodeObjectType, ID: request.ID, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeField, ID: "@" + base + "-value", FieldType: paperlang.FieldString},
		}}
	case "page", "document-preset":
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
		if request.Template == "document-preset" {
			node, err = authoringDocumentPreset(request.Preset, request.ID)
			if err != nil {
				return PaperMutationResult{}, err
			}
		} else {
			node = paperedit.NodeSpec{Kind: paperlang.NodePage, ID: request.ID, Children: []paperedit.NodeSpec{
				{Kind: paperlang.NodeBody, ID: "@" + base + "-body", Children: []paperedit.NodeSpec{
					{Kind: paperlang.NodeParagraph, ID: "@" + base + "-copy", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("New content")}}},
				}},
			}}
		}
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
	case "image", "table", "canvas", "note-box", "metadata-grid", "signature-row", "qr-verification", "clause", "styled-container":
		if parent.Kind == paperlang.NodeDocument || parent.Kind == paperlang.NodePage {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "flow templates require a region, body, row, or column", paperedit.ErrInvalidOperation)
		}
		node, err = authoringFlowTemplate(request.Template, request.ID)
		if err != nil {
			return PaperMutationResult{}, err
		}
	case "repeat", "loop":
		if parent.Kind == paperlang.NodeDocument || parent.Kind == paperlang.NodePage {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "repeat and loop templates require a flow container", paperedit.ErrInvalidOperation)
		}
		node, err = authoringRepeaterTemplate(revision.parsed.AST, request.Template, request.ID, request.Path)
		if err != nil {
			return PaperMutationResult{}, err
		}
	case "header", "footer":
		if parent.Kind != paperlang.NodePage {
			return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE_PARENT", "header and footer templates must target a page", paperedit.ErrInvalidOperation)
		}
		kind := paperlang.NodeHeader
		if request.Template == "footer" {
			kind = paperlang.NodeFooter
		}
		for _, member := range parent.Members {
			if member.Node != nil && member.Node.Kind == kind {
				return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE", request.Template+" already exists on this page", paperedit.ErrInvalidOperation)
			}
		}
		base := strings.TrimPrefix(request.ID, "@")
		node = paperedit.NodeSpec{Kind: kind, ID: request.ID, Children: []paperedit.NodeSpec{{Kind: paperlang.NodeParagraph, ID: "@" + base + "-copy", Properties: []paperedit.PropertySpec{{Name: "size", Value: paperedit.UnitValue(8, "pt")}, {Name: "text", Value: paperedit.StringValue(strings.ToUpper(request.Template) + " | Document title | Page")}}}}}
	default:
		return PaperMutationResult{}, workspaceError("INVALID_TEMPLATE", "template is outside the closed authoring palette", ErrInvalidQuery)
	}
	return w.applyPaperMutation("insert_template", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNode{Parent: request.Guard.Target, Node: node}}, "INVALID_TEMPLATE_RESULT")
}

func authoringRepeaterTemplate(ast paperlang.AST, template, id, sourcePath string) (paperedit.NodeSpec, error) {
	base := strings.TrimPrefix(id, "@")
	paragraph := paperedit.NodeSpec{Kind: paperlang.NodeParagraph, ID: "@" + base + "-item", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("Repeated item")}}}
	if template == "loop" {
		return paperedit.NodeSpec{Kind: paperlang.NodeLoop, ID: id, Properties: []paperedit.PropertySpec{
			{Name: "from", Value: paperedit.NumberValue(1)}, {Name: "through", Value: paperedit.NumberValue(3)}, {Name: "step", Value: paperedit.NumberValue(1)}, {Name: "max-iterations", Value: paperedit.NumberValue(3)}, {Name: "instance-prefix", Value: paperedit.StringValue(base)},
		}, Children: []paperedit.NodeSpec{paragraph}}, nil
	}
	sourcePath = strings.TrimSpace(sourcePath)
	metadata := papercompile.ExtractSchemas(ast)
	if !metadata.OK() {
		return paperedit.NodeSpec{}, workspaceError("INVALID_TEMPLATE", "repeat requires valid compiler schema metadata", ErrInvalidSource)
	}
	var selected *papercompile.FieldDescriptor
	for schemaIndex := range metadata.Schemas {
		schema := &metadata.Schemas[schemaIndex]
		path := sourcePath
		if len(metadata.Schemas) > 1 {
			prefix := strings.TrimPrefix(schema.Name, "@") + "."
			if !strings.HasPrefix(sourcePath, prefix) {
				continue
			}
			path = strings.TrimPrefix(sourcePath, prefix)
		}
		parts := strings.Split(path, ".")
		fields := schema.Fields
		for index, part := range parts {
			selected = nil
			for fieldIndex := range fields {
				if fields[fieldIndex].Name == strings.TrimPrefix(part, "@") {
					selected = &fields[fieldIndex]
					break
				}
			}
			if selected == nil || index < len(parts)-1 && selected.Kind != papercompile.SchemaObject {
				selected = nil
				break
			}
			fields = selected.Fields
		}
		break
	}
	if selected == nil || selected.Kind != papercompile.SchemaList || selected.MaxItems == 0 {
		return paperedit.NodeSpec{}, workspaceError("INVALID_TEMPLATE", "repeat source must address one bounded schema list", ErrInvalidQuery)
	}
	if selected.ItemKind == papercompile.SchemaObject && len(selected.Fields) != 0 {
		paragraph.Properties = append([]paperedit.PropertySpec{{Name: "bind", Value: paperedit.StringValue(selected.Fields[0].Name)}}, paragraph.Properties...)
	}
	return paperedit.NodeSpec{Kind: paperlang.NodeRepeat, ID: id, Properties: []paperedit.PropertySpec{
		{Name: "source", Value: paperedit.StringValue(sourcePath)}, {Name: "instance-prefix", Value: paperedit.StringValue(base)}, {Name: "max-items", Value: paperedit.NumberValue(float64(selected.MaxItems))},
	}, Children: []paperedit.NodeSpec{paragraph}}, nil
}

const authoringPixelDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func authoringFlowTemplate(template, id string) (paperedit.NodeSpec, error) {
	base := strings.TrimPrefix(id, "@")
	childID := func(suffix string) string { return "@" + base + "-" + suffix }
	text := func(kind paperlang.NodeKind, suffix, value string) paperedit.NodeSpec {
		return paperedit.NodeSpec{Kind: kind, ID: childID(suffix), Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue(value)}}}
	}
	cell := func(suffix, value string, header bool) paperedit.NodeSpec {
		properties := []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue(value)}, {Name: "padding", Value: paperedit.UnitValue(6, "pt")}, {Name: "border-width", Value: paperedit.UnitValue(0.5, "pt")}, {Name: "border-color", Value: paperedit.StringValue("#CBD5E1")}}
		if header {
			properties = append(properties, paperedit.PropertySpec{Name: "bold", Value: paperedit.BoolValue(true)}, paperedit.PropertySpec{Name: "background", Value: paperedit.StringValue("#E8F1F5")})
		}
		return paperedit.NodeSpec{Kind: paperlang.NodeTableCell, ID: childID(suffix), Properties: properties}
	}
	switch template {
	case "image":
		return paperedit.NodeSpec{Kind: paperlang.NodeImage, ID: id, Properties: []paperedit.PropertySpec{
			{Name: "source", Value: paperedit.StringValue(authoringPixelDataURI)}, {Name: "width", Value: paperedit.UnitValue(140, "pt")}, {Name: "height", Value: paperedit.UnitValue(84, "pt")}, {Name: "fit", Value: paperedit.StringValue("contain")}, {Name: "alt", Value: paperedit.StringValue("Replace with a descriptive image")},
		}}, nil
	case "table", "metadata-grid", "signature-row":
		rows := []paperedit.NodeSpec{}
		switch template {
		case "table":
			rows = []paperedit.NodeSpec{
				{Kind: paperlang.NodeTableHeader, ID: childID("head"), Children: []paperedit.NodeSpec{{Kind: paperlang.NodeTableRow, ID: childID("head-row"), Children: []paperedit.NodeSpec{cell("head-one", "Item", true), cell("head-two", "Details", true)}}}},
				{Kind: paperlang.NodeTableRow, ID: childID("row-one"), Children: []paperedit.NodeSpec{cell("one-a", "First item", false), cell("one-b", "Add details", false)}},
				{Kind: paperlang.NodeTableRow, ID: childID("row-two"), Children: []paperedit.NodeSpec{cell("two-a", "Second item", false), cell("two-b", "Add details", false)}},
			}
		case "metadata-grid":
			rows = []paperedit.NodeSpec{
				{Kind: paperlang.NodeTableRow, ID: childID("row-one"), Children: []paperedit.NodeSpec{cell("label-one", "REFERENCE", true), cell("value-one", "DOC-0001", false)}},
				{Kind: paperlang.NodeTableRow, ID: childID("row-two"), Children: []paperedit.NodeSpec{cell("label-two", "ISSUED", true), cell("value-two", "Add date and owner", false)}},
			}
		case "signature-row":
			rows = []paperedit.NodeSpec{{Kind: paperlang.NodeTableRow, ID: childID("row"), Children: []paperedit.NodeSpec{cell("signer", "____________________________\nAuthorized signature", false), cell("date", "____________________________\nDate", false)}}}
		}
		return paperedit.NodeSpec{Kind: paperlang.NodeTable, ID: id, Properties: []paperedit.PropertySpec{{Name: "split", Value: paperedit.StringValue("rows")}}, Children: rows}, nil
	case "canvas":
		return paperedit.NodeSpec{Kind: paperlang.NodeCanvas, ID: id, Properties: []paperedit.PropertySpec{{Name: "width", Value: paperedit.UnitValue(160, "pt")}, {Name: "height", Value: paperedit.UnitValue(100, "pt")}}, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeAnchor, ID: childID("panel"), Properties: []paperedit.PropertySpec{{Name: "width", Value: paperedit.UnitValue(120, "pt")}, {Name: "height", Value: paperedit.UnitValue(64, "pt")}, {Name: "left", Value: paperedit.StringValue("canvas.left + 12pt")}, {Name: "top", Value: paperedit.StringValue("canvas.top + 12pt")}, {Name: "background", Value: paperedit.StringValue("#DCEAF7")}, {Name: "alt", Value: paperedit.StringValue("Positioned design panel")}}},
		}}, nil
	case "note-box", "styled-container":
		label := "NOTE\nAdd important supporting information here."
		if template == "styled-container" {
			label = "Styled content\nUse the inspector to adjust color, border, spacing, and typography."
		}
		return paperedit.NodeSpec{Kind: paperlang.NodeParagraph, ID: id, Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue(label)}, {Name: "padding", Value: paperedit.UnitValue(12, "pt")}, {Name: "background", Value: paperedit.StringValue("#F1F5F9")}, {Name: "border-left-width", Value: paperedit.UnitValue(3, "pt")}, {Name: "border-color", Value: paperedit.StringValue("#2C6E7F")}}}, nil
	case "qr-verification":
		return paperedit.NodeSpec{Kind: paperlang.NodeTable, ID: id, Children: []paperedit.NodeSpec{
			{Kind: paperlang.NodeTableRow, ID: childID("row"), Children: []paperedit.NodeSpec{
				{Kind: paperlang.NodeTableCell, ID: childID("qr"), Children: []paperedit.NodeSpec{{Kind: paperlang.NodeImage, ID: childID("image"), Properties: []paperedit.PropertySpec{{Name: "source", Value: paperedit.StringValue(authoringPixelDataURI)}, {Name: "width", Value: paperedit.UnitValue(56, "pt")}, {Name: "height", Value: paperedit.UnitValue(56, "pt")}, {Name: "alt", Value: paperedit.StringValue("Replace with verification QR code")}}}}},
				cell("copy", "VERIFY THIS DOCUMENT\nReplace the image with a generated QR resource and add the verification URL.", false),
			}},
		}}, nil
	case "clause":
		return paperedit.NodeSpec{Kind: paperlang.NodeColumn, ID: id, Properties: []paperedit.PropertySpec{{Name: "gap", Value: paperedit.UnitValue(4, "pt")}}, Children: []paperedit.NodeSpec{text(paperlang.NodeHeading, "title", "1. Clause title"), text(paperlang.NodeParagraph, "copy", "Write the complete clause in clear, reviewable language.")}}, nil
	default:
		return paperedit.NodeSpec{}, workspaceError("INVALID_TEMPLATE", "unknown flow template", ErrInvalidQuery)
	}
}

func authoringDocumentPreset(preset, id string) (paperedit.NodeSpec, error) {
	if preset == "" {
		preset = "blank"
	}
	base := strings.TrimPrefix(id, "@")
	flow, err := authoringFlowTemplate("metadata-grid", "@"+base+"-metadata")
	if err != nil {
		return paperedit.NodeSpec{}, err
	}
	title, subtitle := "Untitled document", "Start writing with a clean, production-ready structure."
	switch preset {
	case "blank":
	case "letter":
		title, subtitle = "Business Letter", "Recipient name\nOrganization\nAddress\n\nDear recipient,"
	case "prescription":
		title, subtitle = "Prescription", "Patient and prescriber details"
	case "medical-report":
		title, subtitle = "Clinical Report", "Patient | encounter | responsible clinician"
	case "invoice":
		title, subtitle = "Invoice", "Bill to | invoice number | issue date"
	case "contract":
		title, subtitle = "Agreement", "Parties | effective date | reference"
	case "certificate":
		title, subtitle = "Certificate", "This certifies that"
	case "table-report":
		title, subtitle = "Tabular Report", "Reporting period | owner | generated date"
	default:
		return paperedit.NodeSpec{}, workspaceError("INVALID_TEMPLATE", "unknown document preset", ErrInvalidQuery)
	}
	children := []paperedit.NodeSpec{
		{Kind: paperlang.NodeHeading, ID: "@" + base + "-title", Properties: []paperedit.PropertySpec{{Name: "level", Value: paperedit.NumberValue(1)}, {Name: "size", Value: paperedit.UnitValue(28, "pt")}, {Name: "color", Value: paperedit.StringValue("#163A46")}, {Name: "text", Value: paperedit.StringValue(title)}}},
		{Kind: paperlang.NodeParagraph, ID: "@" + base + "-subtitle", Properties: []paperedit.PropertySpec{{Name: "color", Value: paperedit.StringValue("#475569")}, {Name: "text", Value: paperedit.StringValue(subtitle)}}},
		flow,
	}
	switch preset {
	case "prescription":
		medications, _ := authoringFlowTemplate("table", "@"+base+"-medications")
		children = append(children, paperedit.NodeSpec{Kind: paperlang.NodeHeading, ID: "@" + base + "-medications-title", Properties: []paperedit.PropertySpec{{Name: "level", Value: paperedit.NumberValue(2)}, {Name: "text", Value: paperedit.StringValue("Medication plan")}}}, medications)
	case "invoice", "table-report":
		table, _ := authoringFlowTemplate("table", "@"+base+"-items")
		children = append(children, table)
	case "contract":
		clause, _ := authoringFlowTemplate("clause", "@"+base+"-clause")
		children = append(children, clause)
	default:
		children = append(children, paperedit.NodeSpec{Kind: paperlang.NodeParagraph, ID: "@" + base + "-content", Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("Add document content here.")}}})
	}
	return paperedit.NodeSpec{Kind: paperlang.NodePage, ID: id, Properties: []paperedit.PropertySpec{{Name: "width", Value: paperedit.UnitValue(595.275590551, "pt")}, {Name: "height", Value: paperedit.UnitValue(841.88976378, "pt")}, {Name: "margin", Value: paperedit.UnitValue(36, "pt")}}, Children: []paperedit.NodeSpec{
		{Kind: paperlang.NodeHeader, ID: "@" + base + "-header", Children: []paperedit.NodeSpec{{Kind: paperlang.NodeParagraph, ID: "@" + base + "-header-copy", Properties: []paperedit.PropertySpec{{Name: "size", Value: paperedit.UnitValue(8, "pt")}, {Name: "bold", Value: paperedit.BoolValue(true)}, {Name: "color", Value: paperedit.StringValue("#2C6E7F")}, {Name: "text", Value: paperedit.StringValue("YOUR ORGANIZATION  |  DOCUMENT")}}}}},
		{Kind: paperlang.NodeFooter, ID: "@" + base + "-footer", Children: []paperedit.NodeSpec{{Kind: paperlang.NodeParagraph, ID: "@" + base + "-footer-copy", Properties: []paperedit.PropertySpec{{Name: "size", Value: paperedit.UnitValue(8, "pt")}, {Name: "color", Value: paperedit.StringValue("#64748B")}, {Name: "text", Value: paperedit.StringValue("Confidential | Replace with document footer")}}}}},
		{Kind: paperlang.NodeBody, ID: "@" + base + "-body", Children: children},
	}}, nil
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
	if parent.Kind != paperlang.NodeSchema && parent.Kind != paperlang.NodeObjectType && parent.Kind != paperlang.NodeField {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "schema field parent must be a schema, custom object, or inline object field", paperedit.ErrInvalidOperation)
	}
	if parent.Kind == paperlang.NodeField && !schemaFieldCanContainChildren(parent) {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "nested fields require an object or object-list parent", paperedit.ErrInvalidOperation)
	}
	if sourceNodesByID(revision.parsed.AST.Root, request.ID) != nil {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "field ID already exists in the exact source revision", paperedit.ErrInvalidOperation)
	}
	fieldKind, ok := papercompile.SchemaKind(request.Type), false
	typeRef := ""
	switch fieldKind {
	case papercompile.SchemaString, papercompile.SchemaNumber, papercompile.SchemaBool, papercompile.SchemaObject, papercompile.SchemaList:
		ok = true
	default:
		if schemaObjectTypeExists(revision.parsed.AST.Root, request.Type) {
			ok = true
			typeRef = request.Type
		}
	}
	if !ok {
		return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "field type must be string, number, bool, object, list, or a declared custom object", ErrInvalidQuery)
	}
	properties := []paperedit.PropertySpec(nil)
	itemTypeRef := ""
	if fieldKind == papercompile.SchemaList {
		itemKind := papercompile.SchemaKind(request.ItemType)
		switch itemKind {
		case papercompile.SchemaString, papercompile.SchemaNumber, papercompile.SchemaBool, papercompile.SchemaObject:
		default:
			if schemaObjectTypeExists(revision.parsed.AST.Root, request.ItemType) {
				itemTypeRef = request.ItemType
			} else {
				return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "list item type must be string, number, bool, object, or a declared custom object", ErrInvalidQuery)
			}
		}
		maxItems := request.MaxItems
		if maxItems == 0 {
			maxItems = 16
		}
		if maxItems > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "list max-items exceeds the schema limit", ErrLimit)
		}
		properties = append(properties, paperedit.PropertySpec{Name: "max-items", Value: paperedit.NumberValue(float64(maxItems))})
	}
	field := paperedit.NodeSpec{Kind: paperlang.NodeField, ID: request.ID, FieldType: paperlang.SchemaFieldType(fieldKind), TypeRef: typeRef, ItemType: paperlang.SchemaFieldType(request.ItemType), ItemTypeRef: itemTypeRef, Properties: properties}
	if typeRef != "" {
		field.FieldType = ""
	}
	if itemTypeRef != "" {
		field.ItemType = ""
	}
	if fieldKind == papercompile.SchemaObject {
		base := strings.TrimPrefix(request.ID, "@")
		if len(base) > 110 {
			return PaperMutationResult{}, workspaceError("INVALID_SCHEMA_FIELD", "object field ID is too long for its starter child", ErrInvalidQuery)
		}
		field.Children = []paperedit.NodeSpec{{Kind: paperlang.NodeField, ID: "@" + base + "-value", FieldType: paperlang.FieldString}}
	} else if fieldKind == papercompile.SchemaList && request.ItemType == string(papercompile.SchemaObject) {
		field.Children = []paperedit.NodeSpec{{Kind: paperlang.NodeField, ID: "@value", FieldType: paperlang.FieldString}}
	}
	return w.applyPaperMutation("add_schema_field", request.Guard, opened, revision,
		[]string{request.Guard.Target}, []paperedit.Operation{paperedit.InsertNode{Parent: request.Guard.Target, Node: field}}, "INVALID_SCHEMA_FIELD")
}

func schemaObjectTypeExists(root *paperlang.Node, name string) bool {
	if root == nil || name == "" {
		return false
	}
	for _, member := range root.Members {
		if member.Node != nil && member.Node.Kind == paperlang.NodeObjectType && strings.TrimPrefix(member.Node.ID, "@") == name {
			return true
		}
	}
	return false
}

func schemaFieldCanContainChildren(node *paperlang.Node) bool {
	if node.FieldType == paperlang.FieldObject {
		return true
	}
	if node.FieldType != paperlang.FieldList {
		return false
	}
	return node.ItemType == paperlang.FieldObject
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
