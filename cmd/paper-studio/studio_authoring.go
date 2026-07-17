// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"errors"
	"net/http"
	"sort"
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

type studioAuthoringTarget struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type studioAuthoringResponse struct {
	FormatVersion   uint16                  `json:"format_version"`
	Revision        string                  `json:"revision"`
	SourceRevision  string                  `json:"source_revision"`
	PlanHash        string                  `json:"plan_hash"`
	Scenario        string                  `json:"scenario,omitempty"`
	DocumentTarget  string                  `json:"document_target,omitempty"`
	TemplateTargets []studioAuthoringTarget `json:"template_targets"`
	BindingTargets  []studioAuthoringTarget `json:"binding_targets"`
	Schemas         []studioSchemaChoice    `json:"schemas"`
	Scenarios       []string                `json:"scenarios"`
	StressPresets   []string                `json:"stress_presets"`
	Components      []string                `json:"components"`
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
		TemplateTargets: []studioAuthoringTarget{}, BindingTargets: []studioAuthoringTarget{}, Schemas: []studioSchemaChoice{}, Scenarios: []string{}, Components: []string{},
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
	scenarios := papercompile.ExtractScenarios(ast)
	if scenarios.OK() {
		if fixtures, err := paperscenario.Resolve(scenarios.Scenarios, paperscenario.Limits{}); err == nil {
			for _, fixture := range fixtures {
				response.Scenarios = append(response.Scenarios, "@"+strings.TrimPrefix(fixture.Name, "@"))
			}
		}
	}
	sort.Strings(response.Scenarios)
	sort.Strings(response.Components)
	return response
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
