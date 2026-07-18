// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperd"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const studioEditFieldLimit = 256

// studioEditRequest is deliberately not paperd's wire representation. The
// browser supplies review facts and a closed semantic intent; all opaque edit,
// candidate, revision, and authority handles remain inside the server.
type studioEditRequest struct {
	SourceRevision string                     `json:"source_revision"`
	PlanRevision   string                     `json:"plan_revision"`
	Scenario       string                     `json:"scenario,omitempty"`
	Operation      string                     `json:"operation"`
	Target         string                     `json:"target"`
	Property       string                     `json:"property"`
	Points         *float64                   `json:"points,omitempty"`
	Number         *float64                   `json:"number,omitempty"`
	Width          *float64                   `json:"width_points,omitempty"`
	Height         *float64                   `json:"height_points,omitempty"`
	Color          string                     `json:"color,omitempty"`
	Kind           string                     `json:"kind,omitempty"`
	Weight         *uint32                    `json:"weight,omitempty"`
	Text           string                     `json:"text,omitempty"`
	Bool           *bool                      `json:"bool,omitempty"`
	Split          string                     `json:"split,omitempty"`
	Path           string                     `json:"path,omitempty"`
	Required       *bool                      `json:"required,omitempty"`
	Format         string                     `json:"format,omitempty"`
	FormatLocale   string                     `json:"format_locale,omitempty"`
	FormatCurrency string                     `json:"format_currency,omitempty"`
	MinFraction    *uint32                    `json:"format_min_fraction,omitempty"`
	MaxFraction    *uint32                    `json:"format_max_fraction,omitempty"`
	Template       string                     `json:"template,omitempty"`
	Component      string                     `json:"component,omitempty"`
	ImportPath     string                     `json:"import_path,omitempty"`
	ID             string                     `json:"id,omitempty"`
	NewParent      string                     `json:"new_parent,omitempty"`
	Schema         string                     `json:"schema,omitempty"`
	Preset         string                     `json:"preset,omitempty"`
	Cases          []studioScenarioMatrixCase `json:"cases,omitempty"`
	BreakPolicy    string                     `json:"break_policy,omitempty"`
}

type studioScenarioMatrixCase struct {
	Name   string `json:"name"`
	Preset string `json:"preset"`
}

type studioEditAuthorization struct {
	Actor   string                       `json:"actor"`
	Allowed bool                         `json:"allowed"`
	Effects []paperd.AuthorizationEffect `json:"effects"`
}

type studioEditResponse struct {
	OK                   bool                    `json:"ok"`
	Operation            string                  `json:"operation"`
	Target               string                  `json:"target"`
	Property             string                  `json:"property"`
	BeforeSourceRevision string                  `json:"before_source_revision"`
	SourceRevision       string                  `json:"source_revision"`
	BeforePlanRevision   string                  `json:"before_plan_revision"`
	PlanRevision         string                  `json:"plan_revision"`
	Applied              bool                    `json:"applied"`
	PatchCount           int                     `json:"patch_count"`
	Scenario             string                  `json:"scenario"`
	ReviewIntent         string                  `json:"review_intent,omitempty"`
	Authorization        studioEditAuthorization `json:"authorization"`
}

func studioSourceRevision(source string) string {
	return string(paperedit.SourceRevision(source))
}

func (s *studioServer) handleEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	var request studioEditRequest
	if err := decodeStudioJSON(r, &request); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()

	s.editMu.Lock()
	defer s.editMu.Unlock()
	result, err := s.applyStudioEdit(ctx, request)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, paperd.ErrRevisionConflict) || errors.Is(err, errStudioStaleEdit) {
			status = http.StatusConflict
		} else if errors.Is(err, errStudioInvalidEdit) || errors.Is(err, paperd.ErrInvalidQuery) {
			status = http.StatusBadRequest
		}
		writeStudioError(w, status, err)
		return
	}
	writeStudioJSON(w, http.StatusOK, result)
}

var (
	errStudioInvalidEdit = errors.New("paper-studio: invalid edit request")
	errStudioStaleEdit   = errors.New("paper-studio: stale edit revision")
)

func normalizeStudioScenario(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "@")
}

func (s *studioServer) applyStudioEdit(ctx context.Context, request studioEditRequest) (studioEditResponse, error) {
	if err := validateStudioEditRequest(request); err != nil {
		return studioEditResponse{}, err
	}
	snapshot, err := s.current(ctx, request.Scenario)
	if err != nil {
		return studioEditResponse{}, err
	}
	fontRepair := request.Operation == "text" && request.Property == "font"
	if request.SourceRevision != studioSourceRevision(snapshot.source) || request.PlanRevision != snapshot.revision || (snapshot.pages == 0 && request.Operation != "template" && request.Operation != "import" && request.Operation != "schema" && request.Operation != "schema-field" && request.Operation != "scenario" && request.Operation != "scenario-create" && request.Operation != "scenario-matrix" && request.Operation != "scenario-value" && !fontRepair) {
		return studioEditResponse{}, fmt.Errorf("%w: source or plan changed after selection", errStudioStaleEdit)
	}

	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	target, parent := studioSourceTarget(parsed.AST.Root, request.Target)
	if target == nil {
		return studioEditResponse{}, fmt.Errorf("%w: target must resolve to one exact authored node", errStudioInvalidEdit)
	}
	directTargets := []string{request.Target}
	operation := paperd.MutationSetBoxProperty
	switch request.Operation {
	case "text":
		operation = paperd.MutationSetTextProperty
	case "grid":
		operation = paperd.MutationSetGridTrack
		if parent == nil || parent.ID == "" {
			return studioEditResponse{}, fmt.Errorf("%w: grid target requires an addressed layout parent", errStudioInvalidEdit)
		}
		directTargets = append(directTargets, parent.ID)
	case "image":
		operation = paperd.MutationSetImageProperty
	case "table":
		operation = paperd.MutationSetTableProperty
		table := studioTableAncestor(parsed.AST.Root, request.Target)
		if table == nil || table.ID == "" {
			return studioEditResponse{}, fmt.Errorf("%w: table target requires an addressed governing table", errStudioInvalidEdit)
		}
		if table.ID != request.Target {
			directTargets = append(directTargets, table.ID)
		}
	case "page":
		operation = paperd.MutationSetPageMargin
	case "page-size":
		operation = paperd.MutationSetPageSize
	case "canvas":
		operation = paperd.MutationSetCanvasAnchor
		if parent == nil || parent.Kind != paperlang.NodeCanvas || parent.ID == "" {
			return studioEditResponse{}, fmt.Errorf("%w: canvas target requires an addressed governing canvas", errStudioInvalidEdit)
		}
		directTargets = append(directTargets, parent.ID)
	case "region":
		operation = paperd.MutationSetPageRegion
		if parent == nil || parent.Kind != paperlang.NodePage || parent.ID == "" || (target.Kind != paperlang.NodeHeader && target.Kind != paperlang.NodeFooter) {
			return studioEditResponse{}, fmt.Errorf("%w: region target requires an authored header/footer and governing page", errStudioInvalidEdit)
		}
		directTargets = append(directTargets, parent.ID)
	case "flow":
		operation = paperd.MutationMoveNode
		destination, _ := studioSourceTarget(parsed.AST.Root, request.NewParent)
		if destination == nil || !studioFlowParentKind(destination.Kind) || request.NewParent == request.Target || !studioFlowChildKind(destination.Kind, target.Kind) {
			return studioEditResponse{}, fmt.Errorf("%w: flow move requires an exact compatible body, row, or column destination", errStudioInvalidEdit)
		}
		directTargets = append(directTargets, request.NewParent)
	case "binding":
		operation = paperd.MutationSetBinding
	case "template":
		operation = paperd.MutationInsertTemplate
	case "import":
		operation = paperd.MutationInsertTemplate
	case "schema":
		operation = paperd.MutationInsertTemplate
	case "scenario-create":
		operation = paperd.MutationCreateScenario
	case "scenario-matrix":
		operation = paperd.MutationCreateScenarioMatrix
	case "schema-field":
		operation = paperd.MutationAddSchemaField
	case "scenario-value":
		operation = paperd.MutationSetScenarioValue
	case "scenario":
		operation = paperd.MutationManageScenario
	}

	s.mu.Lock()
	assetResources := make([]papercompile.AssetResource, len(s.assets))
	for i, asset := range s.assets {
		assetResources[i] = papercompile.AssetResource{Name: asset.Name, MediaType: asset.MediaType, Digest: asset.Digest, Data: append([]byte(nil), asset.Data...), Family: asset.Family, Style: asset.Style, Weight: asset.Weight, License: asset.License}
	}
	s.mu.Unlock()
	workspace, err := paperd.NewWorkspaceWithOptions(paperd.WorkspaceOptions{
		DisclosureDomain:         paperd.DisclosureRestricted,
		RequireMutationAuthority: true,
		AssetResources:           assetResources,
		ImportResolver:           papercompile.ImportResolver(studioFileImportResolver()),
	})
	if err != nil {
		return studioEditResponse{}, err
	}
	created, err := workspace.PaperCreate(paperd.PaperCreateRequest{File: snapshot.file, Source: snapshot.source})
	if err != nil {
		return studioEditResponse{}, err
	}
	opened, err := workspace.PaperOpen(paperd.PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: paperd.CapabilityEdit,
		DisclosureDomain: paperd.DisclosureRestricted,
	})
	if err != nil {
		return studioEditResponse{}, err
	}
	authority, err := workspace.GrantMutationAuthority(paperd.MutationAuthorityGrant{
		Open: opened.Handle, Actor: "studio:local-user", Operations: []paperd.MutationOperation{operation},
		NodeScopes: directTargets,
	})
	if err != nil {
		return studioEditResponse{}, err
	}
	fingerprint, instance, err := studioTargetPrecondition(snapshot.file, snapshot.source, request.Target)
	if err != nil {
		return studioEditResponse{}, fmt.Errorf("%w: target precondition: %v", errStudioInvalidEdit, err)
	}
	guard := paperd.PaperMutationGuard{
		Open: opened.Handle, Authority: authority.Handle, Candidate: created.Candidate.Handle,
		ExpectedHead: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		Target: request.Target, ExpectedFingerprint: fingerprint, ExpectedInstance: instance,
		IdempotencyKey: studioEditIdempotencyKey(request),
	}
	additionalTarget := ""
	if request.Operation == "grid" {
		additionalTarget = parent.ID
	} else if request.Operation == "canvas" {
		additionalTarget = parent.ID
	} else if request.Operation == "region" {
		additionalTarget = parent.ID
	} else if request.Operation == "flow" {
		additionalTarget = request.NewParent
	} else if request.Operation == "table" && len(directTargets) == 2 {
		additionalTarget = directTargets[1]
	}
	if additionalTarget != "" {
		parentFingerprint, parentInstance, preconditionErr := studioTargetPrecondition(snapshot.file, snapshot.source, additionalTarget)
		if preconditionErr != nil {
			return studioEditResponse{}, fmt.Errorf("%w: parent precondition: %v", errStudioInvalidEdit, preconditionErr)
		}
		guard.TargetPreconditions = []paperedit.TargetPrecondition{{
			Target: additionalTarget, ExpectedFingerprint: parentFingerprint, ExpectedInstance: parentInstance,
		}}
	}

	mutation, err := applyStudioSemanticMutation(workspace, guard, request)
	if err != nil {
		return studioEditResponse{}, err
	}
	if mutation.Edit.Diff == nil || len(mutation.Edit.Diff.Patches) == 0 || len(mutation.Edit.Diff.Patches) > 7 {
		return studioEditResponse{}, errors.New("paper-studio: semantic handle did not produce a bounded minimal source patch set")
	}
	if err := writeStudioSourceCAS(snapshot.file, snapshot.sourceHash, mutation.Edit.Source); err != nil {
		return studioEditResponse{}, err
	}
	afterScenario := request.Scenario
	if request.Operation == "scenario" && normalizeStudioScenario(request.Scenario) == normalizeStudioScenario(request.Target) {
		if request.Property == "rename" {
			afterScenario = request.ID
		} else if request.Property == "delete" {
			afterScenario = ""
		}
	}
	after, err := s.current(ctx, afterScenario)
	if err != nil {
		return studioEditResponse{}, err
	}
	return studioEditResponse{
		OK: true, Operation: request.Operation, Target: request.Target, Property: request.Property,
		BeforeSourceRevision: request.SourceRevision, SourceRevision: studioSourceRevision(after.source),
		BeforePlanRevision: request.PlanRevision, PlanRevision: after.revision,
		Scenario: afterScenario,
		Applied:  mutation.Edit.Applied, PatchCount: len(mutation.Edit.Diff.Patches),
		ReviewIntent: request.BreakPolicy,
		Authorization: studioEditAuthorization{Actor: mutation.Authorization.Actor, Allowed: mutation.Authorization.Allowed,
			Effects: append([]paperd.AuthorizationEffect(nil), mutation.Authorization.Effects...)},
	}, nil
}

func validateStudioEditRequest(request studioEditRequest) error {
	fields := []string{request.SourceRevision, request.PlanRevision, request.Scenario, request.Operation, request.Target, request.Property, request.Color, request.Kind, request.Text, request.Split, request.Path, request.Format, request.FormatLocale, request.FormatCurrency, request.Template, request.Component, request.ImportPath, request.ID, request.NewParent, request.Schema, request.Preset, request.BreakPolicy}
	for _, field := range fields {
		if len(field) > studioEditFieldLimit || !utf8.ValidString(field) {
			return fmt.Errorf("%w: edit field exceeds its bound", errStudioInvalidEdit)
		}
	}
	if request.SourceRevision == "" || request.PlanRevision == "" || request.Target == "" || request.Target[0] != '@' || strings.ContainsAny(request.Target, " \t\r\n") {
		return fmt.Errorf("%w: exact revisions and readable target are required", errStudioInvalidEdit)
	}
	if request.Points != nil && (math.IsNaN(*request.Points) || math.IsInf(*request.Points, 0)) {
		return fmt.Errorf("%w: points must be finite", errStudioInvalidEdit)
	}
	if request.Number != nil && (math.IsNaN(*request.Number) || math.IsInf(*request.Number, 0)) {
		return fmt.Errorf("%w: number must be finite", errStudioInvalidEdit)
	}
	if request.MinFraction != nil && *request.MinFraction > 18 || request.MaxFraction != nil && *request.MaxFraction > 18 {
		return fmt.Errorf("%w: binding fraction digits must be between 0 and 18", errStudioInvalidEdit)
	}
	if request.Width != nil && (math.IsNaN(*request.Width) || math.IsInf(*request.Width, 0)) || request.Height != nil && (math.IsNaN(*request.Height) || math.IsInf(*request.Height, 0)) {
		return fmt.Errorf("%w: page dimensions must be finite", errStudioInvalidEdit)
	}
	if request.Operation != "box" && request.Operation != "text" && request.Operation != "grid" && request.Operation != "image" && request.Operation != "table" && request.Operation != "page" && request.Operation != "page-size" && request.Operation != "canvas" && request.Operation != "region" && request.Operation != "binding" && request.Operation != "template" && request.Operation != "import" && request.Operation != "schema" && request.Operation != "schema-field" && request.Operation != "scenario-create" && request.Operation != "scenario-matrix" && request.Operation != "scenario-value" && request.Operation != "scenario" && request.Operation != "flow" {
		return fmt.Errorf("%w: operation is outside the closed Studio authoring vocabulary", errStudioInvalidEdit)
	}
	if request.Operation == "scenario" {
		if request.Property != "rename" && request.Property != "delete" {
			return fmt.Errorf("%w: scenario action must be rename or delete", errStudioInvalidEdit)
		}
		if request.Property == "rename" && request.ID == "" {
			return fmt.Errorf("%w: scenario rename requires a new readable @id", errStudioInvalidEdit)
		}
		if request.Property == "delete" && request.ID != "" {
			return fmt.Errorf("%w: scenario delete cannot carry a replacement @id", errStudioInvalidEdit)
		}
	}
	if request.Operation == "scenario-value" && request.Path == "" {
		return fmt.Errorf("%w: scenario value operation requires a fixture path", errStudioInvalidEdit)
	}
	if request.Operation == "scenario-matrix" && (len(request.Cases) == 0 || len(request.Cases) > 16) {
		return fmt.Errorf("%w: scenario matrix requires between one and sixteen cases", errStudioInvalidEdit)
	}
	if request.Operation == "flow" && (request.NewParent == "" || request.NewParent[0] != '@') {
		return fmt.Errorf("%w: flow operation requires a readable destination @id", errStudioInvalidEdit)
	}
	if request.BreakPolicy != "" && request.BreakPolicy != "hard" && request.BreakPolicy != "keep-with-next" && request.BreakPolicy != "avoid-orphan" {
		return fmt.Errorf("%w: break policy is outside the closed Studio vocabulary", errStudioInvalidEdit)
	}
	return nil
}

func applyStudioSemanticMutation(workspace *paperd.Workspace, guard paperd.PaperMutationGuard, request studioEditRequest) (paperd.PaperMutationResult, error) {
	if request.Operation == "binding" {
		return workspace.PaperSetBinding(paperd.PaperSetBindingRequest{
			Guard: guard, Path: request.Path, Required: request.Required,
			Format: request.Format, FormatLocale: request.FormatLocale, FormatCurrency: request.FormatCurrency,
			MinFractionDigits: request.MinFraction, MaxFractionDigits: request.MaxFraction,
		})
	}
	if request.Operation == "template" {
		return workspace.PaperInsertTemplate(paperd.PaperInsertTemplateRequest{Guard: guard, Template: request.Template, ID: request.ID, Component: request.Component})
	}
	if request.Operation == "import" {
		return workspace.PaperInsertTemplate(paperd.PaperInsertTemplateRequest{Guard: guard, Template: "import", ImportPath: request.ImportPath})
	}
	if request.Operation == "schema" {
		return workspace.PaperInsertTemplate(paperd.PaperInsertTemplateRequest{Guard: guard, Template: "schema", ID: request.ID})
	}
	if request.Operation == "scenario-create" {
		return workspace.PaperCreateScenario(paperd.PaperCreateScenarioRequest{Guard: guard, Name: request.ID, Schema: request.Schema, Preset: request.Preset})
	}
	if request.Operation == "scenario-matrix" {
		cases := make([]paperd.PaperScenarioMatrixCase, len(request.Cases))
		for index, matrixCase := range request.Cases {
			cases[index] = paperd.PaperScenarioMatrixCase{Name: matrixCase.Name, Preset: matrixCase.Preset}
		}
		return workspace.PaperCreateScenarioMatrix(paperd.PaperCreateScenarioMatrixRequest{Guard: guard, Schema: request.Schema, Cases: cases})
	}
	if request.Operation == "schema-field" {
		maxItems := uint32(0)
		if request.Weight != nil {
			maxItems = *request.Weight
		}
		return workspace.PaperAddSchemaField(paperd.PaperAddSchemaFieldRequest{Guard: guard, ID: request.ID, Type: request.Kind, ItemType: request.Text, MaxItems: maxItems})
	}
	if request.Operation == "scenario-value" {
		return workspace.PaperSetScenarioFixtureValue(paperd.PaperSetScenarioFixtureValueRequest{Guard: guard, Path: request.Path, Kind: request.Kind, Text: request.Text, Bool: request.Bool, Number: request.Number})
	}
	if request.Operation == "scenario" {
		return workspace.PaperManageScenario(paperd.PaperManageScenarioRequest{Guard: guard, Action: request.Property, NewName: request.ID})
	}
	if request.Operation == "flow" {
		return workspace.PaperMoveNode(paperd.PaperMoveNodeRequest{Guard: guard, NewParent: request.NewParent})
	}
	if request.Operation == "box" {
		points := 0.0
		if request.Points != nil {
			points = *request.Points
		}
		return workspace.PaperSetBoxProperty(paperd.PaperSetBoxPropertyRequest{
			Guard: guard, Property: paperd.PaperBoxProperty(request.Property), Points: points, Color: request.Color,
		})
	}
	if request.Operation == "text" {
		return workspace.PaperSetTextProperty(paperd.PaperSetTextPropertyRequest{
			Guard: guard, Property: paperd.PaperTextProperty(request.Property), Text: request.Text,
		})
	}
	if request.Operation == "image" {
		number, points, boolean := 0.0, 0.0, false
		if request.Number != nil {
			number = *request.Number
		}
		if request.Points != nil {
			points = *request.Points
		}
		if request.Bool != nil {
			boolean = *request.Bool
		}
		return workspace.PaperSetImageProperty(paperd.PaperSetImagePropertyRequest{
			Guard: guard, Property: paperd.PaperImageProperty(request.Property), Fit: request.Kind,
			Number: number, Points: points, Text: request.Text, Bool: boolean,
		})
	}
	if request.Operation == "table" {
		points, boolean := 0.0, false
		if request.Points != nil {
			points = *request.Points
		}
		if request.Bool != nil {
			boolean = *request.Bool
		}
		return workspace.PaperSetTableProperty(paperd.PaperSetTablePropertyRequest{
			Guard: guard, Property: paperd.PaperTableProperty(request.Property), Split: request.Split, Points: points, Bool: boolean,
		})
	}
	if request.Operation == "page" {
		points := 0.0
		if request.Points != nil {
			points = *request.Points
		}
		return workspace.PaperSetPageMargin(paperd.PaperSetPageMarginRequest{
			Guard: guard, Property: paperd.PaperPageMarginProperty(request.Property), Points: points,
		})
	}
	if request.Operation == "page-size" {
		if request.Width == nil || request.Height == nil {
			return paperd.PaperMutationResult{}, fmt.Errorf("%w: page size requires width and height", errStudioInvalidEdit)
		}
		return workspace.PaperSetPageSize(paperd.PaperSetPageSizeRequest{Guard: guard, WidthPoints: *request.Width, HeightPoints: *request.Height})
	}
	if request.Operation == "canvas" {
		offset := 0.0
		if request.Points != nil {
			offset = *request.Points
		}
		return workspace.PaperSetCanvasAnchor(paperd.PaperSetCanvasAnchorRequest{
			Guard: guard, Property: paperd.PaperCanvasAnchorProperty(request.Property), Reference: request.Text,
			TargetAnchor: paperd.PaperCanvasAnchorProperty(request.Kind), Offset: offset,
		})
	}
	if request.Operation == "region" {
		points, boolean := 0.0, false
		if request.Points != nil {
			points = *request.Points
		}
		if request.Bool != nil {
			boolean = *request.Bool
		}
		return workspace.PaperSetPageRegion(paperd.PaperSetPageRegionRequest{Guard: guard, Property: request.Property, Points: points, Color: request.Color, Bool: boolean})
	}
	points := 0.0
	if request.Points != nil {
		points = *request.Points
	}
	weight := uint32(0)
	if request.Weight != nil {
		weight = *request.Weight
	}
	return workspace.PaperSetGridTrack(paperd.PaperSetGridTrackRequest{
		Guard: guard, Property: paperd.PaperGridTrackProperty(request.Property), Kind: request.Kind, Points: points, Weight: weight,
	})
}

func studioFlowParentKind(kind paperlang.NodeKind) bool {
	return kind == paperlang.NodeBody || kind == paperlang.NodeRow || kind == paperlang.NodeColumn
}

func studioFlowChildKind(parent, child paperlang.NodeKind) bool {
	switch parent {
	case paperlang.NodeBody:
		return child == paperlang.NodeHeading || child == paperlang.NodeParagraph || child == paperlang.NodeList ||
			child == paperlang.NodePageBreak || child == paperlang.NodeText || child == paperlang.NodeRow ||
			child == paperlang.NodeColumn || child == paperlang.NodeImage || child == paperlang.NodeTable ||
			child == paperlang.NodeCanvas || child == paperlang.NodeUse || child == paperlang.NodeRepeat || child == paperlang.NodeLoop
	case paperlang.NodeRow, paperlang.NodeColumn:
		return child == paperlang.NodeHeading || child == paperlang.NodeParagraph || child == paperlang.NodeUse
	default:
		return false
	}
}

func studioTableAncestor(root *paperlang.Node, target string) *paperlang.Node {
	var found, table *paperlang.Node
	var matches int
	var walk func(*paperlang.Node, *paperlang.Node)
	walk = func(node, governing *paperlang.Node) {
		if node == nil {
			return
		}
		if node.Kind == paperlang.NodeTable {
			governing = node
		}
		if node.ID == target {
			matches++
			found, table = node, governing
		}
		for _, member := range node.Members {
			walk(member.Node, governing)
		}
	}
	walk(root, nil)
	if matches != 1 || found == nil {
		return nil
	}
	return table
}

func studioSourceTarget(root *paperlang.Node, id string) (found, parent *paperlang.Node) {
	var matches int
	var walk func(*paperlang.Node, *paperlang.Node)
	walk = func(node, owner *paperlang.Node) {
		if node == nil {
			return
		}
		if node.ID == id {
			matches++
			found, parent = node, owner
		}
		for _, member := range node.Members {
			walk(member.Node, node)
		}
	}
	walk(root, nil)
	if matches != 1 {
		return nil, nil
	}
	return found, parent
}

func studioTargetPrecondition(file, source, target string) (paperedit.NodeFingerprint, string, error) {
	fingerprint, err := paperedit.FingerprintNode(file, source, target)
	if err != nil {
		return "", "", err
	}
	instance, err := paperedit.SourceInstance(file, source, target)
	return fingerprint, instance, err
}

func studioEditIdempotencyKey(request studioEditRequest) string {
	encoded, _ := json.Marshal(request)
	digest := sha256.Sum256(encoded)
	return "studio-edit-" + hex.EncodeToString(digest[:16])
}

func writeStudioSourceCAS(file string, expected [32]byte, source string) error {
	_, actual, err := readStudioSource(file)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w: file changed before commit", errStudioStaleEdit)
	}
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	directory := filepath.Dir(file)
	temporary, err := os.CreateTemp(directory, ".paper-studio-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.WriteString(source); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	_, actual, err = readStudioSource(file)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("%w: file changed during commit", errStudioStaleEdit)
	}
	if err := os.Rename(temporaryName, file); err != nil {
		return err
	}
	if directoryHandle, openErr := os.Open(directory); openErr == nil {
		_ = directoryHandle.Sync()
		_ = directoryHandle.Close()
	}
	return nil
}
