// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

type studioBindingChoice struct {
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	Required   bool   `json:"required"`
	Collection bool   `json:"collection,omitempty"`
}

type studioSchemaChoice struct {
	Name   string                `json:"name"`
	Fields []studioBindingChoice `json:"fields"`
}

type studioSchemaFieldTarget struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Schema string `json:"schema"`
	Path   string `json:"path"`
}

type studioScenarioValue struct {
	Scenario string `json:"scenario"`
	Path     string `json:"path"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
}

type studioAuthoringTarget struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type studioAuthoringResponse struct {
	FormatVersion   uint16                    `json:"format_version"`
	Revision        string                    `json:"revision"`
	SourceRevision  string                    `json:"source_revision"`
	PlanHash        string                    `json:"plan_hash"`
	Scenario        string                    `json:"scenario,omitempty"`
	DocumentTarget  string                    `json:"document_target,omitempty"`
	TemplateTargets []studioAuthoringTarget   `json:"template_targets"`
	BindingTargets  []studioAuthoringTarget   `json:"binding_targets"`
	Schemas         []studioSchemaChoice      `json:"schemas"`
	SchemaFields    []studioSchemaFieldTarget `json:"schema_field_targets"`
	Scenarios       []string                  `json:"scenarios"`
	ScenarioValues  []studioScenarioValue     `json:"scenario_values"`
	StressPresets   []string                  `json:"stress_presets"`
	Components      []string                  `json:"components"`
}

func (s *studioServer) handleAuthoring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if revision := r.URL.Query().Get("revision"); revision == "" || revision != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: authoring metadata revision is stale"))
		return
	}
	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	if !parsed.OK() {
		writeStudioError(w, http.StatusUnprocessableEntity, errors.New("paper-studio: source cannot provide authoring metadata"))
		return
	}
	response := buildStudioAuthoringResponse(snapshot, parsed.AST)
	writeStudioJSON(w, http.StatusOK, response)
}

func buildStudioAuthoringResponse(snapshot *studioSnapshot, ast paperlang.AST) studioAuthoringResponse {
	response := studioAuthoringResponse{
		FormatVersion: 1, Revision: snapshot.revision, SourceRevision: studioSourceRevision(snapshot.source),
		PlanHash: snapshot.plan.Hash(), Scenario: snapshot.scenario, StressPresets: []string{"empty", "typical", "stress"},
		TemplateTargets: []studioAuthoringTarget{}, BindingTargets: []studioAuthoringTarget{}, Schemas: []studioSchemaChoice{}, SchemaFields: []studioSchemaFieldTarget{}, Scenarios: []string{}, ScenarioValues: []studioScenarioValue{}, Components: []string{},
	}
	hasPage := false
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil {
			return
		}
		if node.Kind == paperlang.NodeDocument && node.ID != "" {
			response.DocumentTarget = node.ID
		}
		if node.Kind == paperlang.NodePage {
			hasPage = true
		}
		if node.ID != "" && (node.Kind == paperlang.NodeBody || node.Kind == paperlang.NodeRow || node.Kind == paperlang.NodeColumn) {
			response.TemplateTargets = append(response.TemplateTargets, studioAuthoringTarget{ID: node.ID, Kind: string(node.Kind)})
		}
		if node.ID != "" && (node.Kind == paperlang.NodeParagraph || node.Kind == paperlang.NodeHeading || node.Kind == paperlang.NodeUse) {
			response.BindingTargets = append(response.BindingTargets, studioAuthoringTarget{ID: node.ID, Kind: string(node.Kind)})
		}
		if node.ID != "" && node.Kind == paperlang.NodeComponent {
			response.Components = append(response.Components, node.ID)
		}
		for _, member := range node.Members {
			walk(member.Node)
		}
	}
	walk(ast.Root)
	if response.DocumentTarget != "" && !hasPage {
		response.TemplateTargets = append(response.TemplateTargets, studioAuthoringTarget{ID: response.DocumentTarget, Kind: string(paperlang.NodeDocument)})
	}
	sort.Slice(response.TemplateTargets, func(i, j int) bool { return response.TemplateTargets[i].ID < response.TemplateTargets[j].ID })
	sort.Slice(response.BindingTargets, func(i, j int) bool { return response.BindingTargets[i].ID < response.BindingTargets[j].ID })
	schemas := papercompile.ExtractSchemas(ast)
	for _, schema := range schemas.Schemas {
		choice := studioSchemaChoice{Name: schema.Name, Fields: []studioBindingChoice{}}
		flattenStudioFields(&choice.Fields, schema.Name, schema.Fields, false)
		response.Schemas = append(response.Schemas, choice)
	}
	collectStudioSchemaFieldTargets(ast.Root, "", "", &response.SchemaFields)
	scenarios := papercompile.ExtractScenarios(ast)
	if scenarios.OK() {
		if fixtures, err := paperscenario.Resolve(scenarios.Scenarios, paperscenario.Limits{}); err == nil {
			for _, fixture := range fixtures {
				response.Scenarios = append(response.Scenarios, "@"+strings.TrimPrefix(fixture.Name, "@"))
			}
		}
	}
	collectStudioScenarioValues(ast.Root, &response.ScenarioValues)
	sort.Strings(response.Scenarios)
	sort.Slice(response.SchemaFields, func(i, j int) bool {
		if response.SchemaFields[i].Schema != response.SchemaFields[j].Schema {
			return response.SchemaFields[i].Schema < response.SchemaFields[j].Schema
		}
		return response.SchemaFields[i].Path < response.SchemaFields[j].Path
	})
	sort.Slice(response.ScenarioValues, func(i, j int) bool {
		if response.ScenarioValues[i].Scenario != response.ScenarioValues[j].Scenario {
			return response.ScenarioValues[i].Scenario < response.ScenarioValues[j].Scenario
		}
		return response.ScenarioValues[i].Path < response.ScenarioValues[j].Path
	})
	sort.Strings(response.Components)
	return response
}

func collectStudioSchemaFieldTargets(node *paperlang.Node, schema, prefix string, output *[]studioSchemaFieldTarget) {
	if node == nil {
		return
	}
	if node.Kind == paperlang.NodeSchema {
		schema = node.ID
		prefix = ""
		if node.ID != "" {
			*output = append(*output, studioSchemaFieldTarget{ID: node.ID, Kind: string(node.Kind), Schema: schema, Path: ""})
		}
	}
	if node.Kind == paperlang.NodeField && node.ID != "" && schema != "" {
		path := strings.TrimPrefix(prefix+"."+strings.TrimPrefix(node.ID, "@"), ".")
		if node.Kind == paperlang.NodeField && schemaFieldTargetCanContainChildren(node) {
			*output = append(*output, studioSchemaFieldTarget{ID: node.ID, Kind: string(node.Kind), Schema: schema, Path: path})
		}
		prefix = path
	}
	for _, member := range node.Members {
		collectStudioSchemaFieldTargets(member.Node, schema, prefix, output)
	}
}

func schemaFieldTargetCanContainChildren(node *paperlang.Node) bool {
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
	return typeName == "object" || typeName == "list" && itemType == "object"
}

func collectStudioScenarioValues(root *paperlang.Node, output *[]studioScenarioValue) {
	var walk func(*paperlang.Node, string, string)
	walk = func(node *paperlang.Node, scenario, prefix string) {
		if node == nil {
			return
		}
		if node.Kind == paperlang.NodeScenario {
			scenario = node.ID
			prefix = ""
		}
		if scenario == "" {
			for _, member := range node.Members {
				walk(member.Node, scenario, prefix)
			}
			return
		}
		if node.ID != "" && node.Kind != paperlang.NodeScenario {
			prefix = strings.TrimPrefix(prefix+"."+strings.TrimPrefix(node.ID, "@"), ".")
		}
		if node.Kind == paperlang.NodeValue && node.Value != nil && node.ID != "" {
			kind, value, ok := studioScenarioScalar(node.Value)
			if ok {
				*output = append(*output, studioScenarioValue{Scenario: scenario, Path: prefix, Kind: kind, Value: value})
			}
		}
		for _, member := range node.Members {
			walk(member.Node, scenario, prefix)
		}
	}
	walk(root, "", "")
}

func studioScenarioScalar(value *paperlang.Scalar) (string, string, bool) {
	switch value.Kind {
	case paperlang.ScalarString:
		if value.StringValue != nil {
			return "string", *value.StringValue, true
		}
	case paperlang.ScalarBool:
		if value.BoolValue != nil {
			return "bool", strconv.FormatBool(*value.BoolValue), true
		}
	case paperlang.ScalarNumber:
		if value.NumberValue != nil {
			return "number", strconv.FormatFloat(*value.NumberValue, 'g', -1, 64), true
		}
	}
	return "", "", false
}

func flattenStudioFields(output *[]studioBindingChoice, prefix string, fields []papercompile.FieldDescriptor, collection bool) {
	for _, field := range fields {
		path := prefix + "." + field.Name
		switch field.Kind {
		case papercompile.SchemaObject:
			flattenStudioFields(output, path, field.Fields, collection)
		case papercompile.SchemaList:
			*output = append(*output, studioBindingChoice{Path: path, Kind: string(field.Kind), Required: field.Required, Collection: true})
			if field.ItemKind == papercompile.SchemaObject {
				flattenStudioFields(output, path+"[]", field.Fields, true)
			}
		default:
			*output = append(*output, studioBindingChoice{Path: path, Kind: string(field.Kind), Required: field.Required, Collection: collection})
		}
	}
}
