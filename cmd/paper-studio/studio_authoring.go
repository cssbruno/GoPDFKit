// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
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

const studioComponentPreviewFormat = "2"

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

type studioComponentPreviewFragment struct {
	Instance string `json:"instance"`
	Page     uint32 `json:"page"`
	Margin   struct {
		X      int64 `json:"x"`
		Y      int64 `json:"y"`
		Width  int64 `json:"width"`
		Height int64 `json:"height"`
	} `json:"margin_box"`
}

// handleComponentPreview plans one ephemeral component instance inside the
// current document. It returns the exact display-list SVG with a viewBox
// cropped to finalized fragment geometry; browser layout only scales that
// immutable geometry into the authoring rail.
func (s *studioServer) handleComponentPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	if r.URL.Query().Get("preview_format") != studioComponentPreviewFormat {
		writeStudioError(w, http.StatusBadRequest, errors.New("paper-studio: unsupported component preview format"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if r.URL.Query().Get("revision") != snapshot.revision || r.URL.Query().Get("source_revision") != studioSourceRevision(snapshot.source) {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: component preview revision is stale"))
		return
	}
	component := r.URL.Query().Get("component")
	preview, planHash, err := s.componentPreview(ctx, snapshot, component)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+planHash+`-component-v`+studioComponentPreviewFormat+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(preview) // #nosec G705 -- preview is emitted only by the bounded display-list SVG serializer, which XML-escapes authored text and attributes.
}

func (s *studioServer) componentPreview(ctx context.Context, snapshot *studioSnapshot, component string) ([]byte, string, error) {
	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	if !parsed.OK() {
		return nil, "", errors.New("paper-studio: source cannot provide a component preview")
	}
	metadata := buildStudioAuthoringResponse(snapshot, parsed.AST)
	componentFound, body := false, ""
	for _, candidate := range metadata.Components {
		componentFound = componentFound || candidate == component
	}
	for _, target := range metadata.TemplateTargets {
		if target.Kind == string(paperlang.NodeBody) && body == "" {
			body = target.ID
		}
	}
	if !componentFound || body == "" {
		return nil, "", errors.New("paper-studio: component preview requires one exact component and body")
	}
	previewID := "@studio-gallery-preview"
	for suffix := 2; studioSourceTargetExists(parsed.AST.Root, previewID); suffix++ {
		previewID = "@studio-gallery-preview-" + strconv.Itoa(suffix)
	}
	value := paperedit.StringValue(component)
	fingerprint, instance, err := studioTargetPrecondition(snapshot.file, snapshot.source, body)
	if err != nil {
		return nil, "", err
	}
	edit, err := paperedit.Apply(paperedit.Transaction{File: snapshot.file, Source: snapshot.source,
		ExpectedRevision: paperedit.SourceRevision(snapshot.source), RequireExactTargets: true,
		TargetPreconditions: []paperedit.TargetPrecondition{{Target: body, ExpectedFingerprint: fingerprint, ExpectedInstance: instance}},
		Operations: []paperedit.Operation{paperedit.InsertNode{Parent: body, Node: paperedit.NodeSpec{Kind: paperlang.NodeUse, ID: previewID,
			Properties: []paperedit.PropertySpec{{Name: "component", Value: value}}}}}})
	if err != nil || !edit.Applied {
		return nil, "", fmt.Errorf("paper-studio: create ephemeral component preview: %w (%+v)", err, edit.Diagnostics)
	}
	s.mu.Lock()
	catalog := s.assetCatalog
	s.mu.Unlock()
	resolver := studioFileImportResolver()
	var plan document.PaperPlan
	var planned document.PaperPlanResult
	if snapshot.scenario == "" {
		plan, planned, err = document.PlanPaperWithAssetsAndImportsContext(ctx, snapshot.file, edit.Source, catalog, resolver)
	} else {
		plan, planned, err = document.PlanPaperScenarioWithAssetsAndImportsContext(ctx, snapshot.file, edit.Source, snapshot.scenario, catalog, resolver)
	}
	if err != nil || !planned.OK() {
		return nil, "", fmt.Errorf("paper-studio: plan component preview: %w", err)
	}
	page, crop, err := componentPreviewBounds(plan, previewID)
	if err != nil {
		return nil, "", err
	}
	capture, err := plan.CaptureDisplayPageSVG(ctx, page, nil)
	if err != nil {
		return nil, "", err
	}
	return cropComponentPreviewSVG(capture.SVG, crop), plan.Hash(), nil
}

func studioSourceTargetExists(root *paperlang.Node, target string) bool {
	node, _ := studioSourceTarget(root, target)
	return node != nil
}

type studioComponentPreviewBounds struct{ X, Y, Width, Height int64 }

func componentPreviewBounds(plan document.PaperPlan, previewID string) (uint32, studioComponentPreviewBounds, error) {
	var selectedPage uint32
	var bounds, lineBounds studioComponentPreviewBounds
	for page := uint32(1); page <= uint32(plan.PageCount()); page++ { // #nosec G115 -- page count is bounded by the planner
		query, err := plan.Query(document.PaperPlanSelector{Page: page, MaxResults: 256})
		if err != nil {
			return 0, studioComponentPreviewBounds{}, err
		}
		var decoded struct {
			Summary struct {
				Fragments struct{ Truncated bool }
				Lines     struct{ Truncated bool }
			}
			Fragments []struct {
				Fragment studioComponentPreviewFragment
			}
			Lines []struct {
				Instance string
				Line     struct {
					Bounds studioComponentPreviewBounds `json:"bounds"`
				}
			}
		}
		if err := json.Unmarshal(query.JSON(), &decoded); err != nil {
			return 0, studioComponentPreviewBounds{}, err
		}
		if decoded.Summary.Fragments.Truncated || decoded.Summary.Lines.Truncated {
			return 0, studioComponentPreviewBounds{}, errors.New("paper-studio: component preview exceeds the structural query bound")
		}
		for _, record := range decoded.Fragments {
			fragment := record.Fragment
			if fragment.Instance != previewID && !strings.HasPrefix(fragment.Instance, previewID+"/") || fragment.Margin.Width <= 0 || fragment.Margin.Height <= 0 {
				continue
			}
			if selectedPage == 0 {
				selectedPage = page
			}
			if page != selectedPage {
				continue
			}
			bounds = unionStudioComponentBounds(bounds, studioComponentPreviewBounds(fragment.Margin))
		}
		if page == selectedPage {
			for _, line := range decoded.Lines {
				if line.Instance == previewID || strings.HasPrefix(line.Instance, previewID+"/") {
					lineBounds = unionStudioComponentBounds(lineBounds, line.Line.Bounds)
				}
			}
		}
	}
	if selectedPage == 0 || bounds.Width <= 0 || bounds.Height <= 0 {
		return 0, studioComponentPreviewBounds{}, errors.New("paper-studio: component produced no previewable finalized geometry")
	}
	if lineBounds.Width > 0 && lineBounds.Height > 0 {
		bounds = lineBounds
	}
	const padding = int64(4 * 1024)
	bounds.X -= padding
	bounds.Y -= padding
	bounds.Width += 2 * padding
	bounds.Height += 2 * padding
	if bounds.X < 0 {
		bounds.Width += bounds.X
		bounds.X = 0
	}
	if bounds.Y < 0 {
		bounds.Height += bounds.Y
		bounds.Y = 0
	}
	// The inspector rail is narrow. Keep a left-anchored exact crop rather than
	// shrinking a full-width paragraph fragment until its themed text is
	// illegible; this changes only the visible crop, never planner geometry.
	if maxPreviewWidth := bounds.Height * 4; bounds.Width > maxPreviewWidth {
		bounds.Width = maxPreviewWidth
	}
	return selectedPage, bounds, nil
}

func unionStudioComponentBounds(left, right studioComponentPreviewBounds) studioComponentPreviewBounds {
	if left.Width <= 0 || left.Height <= 0 {
		return right
	}
	x2, y2 := max(left.X+left.Width, right.X+right.Width), max(left.Y+left.Height, right.Y+right.Height)
	left.X, left.Y = min(left.X, right.X), min(left.Y, right.Y)
	left.Width, left.Height = x2-left.X, y2-left.Y
	return left
}

func cropComponentPreviewSVG(source []byte, crop studioComponentPreviewBounds) []byte {
	prefix := `viewBox="`
	start := strings.Index(string(source), prefix)
	if start < 0 {
		return append([]byte(nil), source...)
	}
	start += len(prefix)
	end := strings.IndexByte(string(source[start:]), '"')
	if end < 0 {
		return append([]byte(nil), source...)
	}
	viewBox := fmt.Sprintf("%d %d %d %d", crop.X, crop.Y, crop.Width, crop.Height)
	result := make([]byte, 0, len(source)+len(viewBox))
	result = append(result, source[:start]...)
	result = append(result, viewBox...)
	result = append(result, source[start+end:]...)
	return result
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
