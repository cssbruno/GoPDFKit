// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperassets"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

type studioResourceCatalogRequest struct {
	SourceRevision string   `json:"source_revision"`
	PlanRevision   string   `json:"plan_revision"`
	Scenario       string   `json:"scenario,omitempty"`
	Operation      string   `json:"operation"`
	Name           string   `json:"name"`
	Path           string   `json:"path,omitempty"`
	MediaType      string   `json:"media_type,omitempty"`
	Family         string   `json:"family,omitempty"`
	Weight         uint16   `json:"weight,omitempty"`
	Style          string   `json:"style,omitempty"`
	License        string   `json:"license,omitempty"`
	Fallback       []string `json:"fallback,omitempty"`
	Replaces       string   `json:"replaces,omitempty"`
	FocusX         *float64 `json:"focus_x,omitempty"`
	FocusY         *float64 `json:"focus_y,omitempty"`
}

type studioResourceCatalogResponse struct {
	OK        bool                 `json:"ok"`
	Operation string               `json:"operation"`
	Name      string               `json:"name"`
	Inventory studioAssetInventory `json:"inventory"`
}

func (s *studioServer) handleResourceCatalogMutation(w http.ResponseWriter, r *http.Request) {
	var request studioResourceCatalogRequest
	if err := decodeStudioJSON(r, &request); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}
	if err := validateStudioResourceCatalogRequest(request); err != nil {
		writeStudioError(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	s.editMu.Lock()
	defer s.editMu.Unlock()

	manifestPath, assetRoot := s.projectManifestConfig()
	if manifestPath == "" {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: catalog editing requires an explicit -assets manifest"))
		return
	}
	snapshot, err := s.current(ctx, request.Scenario)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if request.SourceRevision != studioSourceRevision(snapshot.source) || request.PlanRevision != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: stale resource catalog revision"))
		return
	}
	if _, actual, err := readStudioSource(snapshot.file); err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	} else if actual != snapshot.sourceHash {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: source changed before catalog update"))
		return
	}
	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	if !parsed.OK() {
		writeStudioError(w, http.StatusUnprocessableEntity, errors.New("paper-studio: source AST is unavailable"))
		return
	}
	if request.Operation == "remove" && studioASTUsesResource(parsed.AST.Root, request.Name) {
		writeStudioError(w, http.StatusUnprocessableEntity, fmt.Errorf("paper-studio: resource %q is still referenced by the source", request.Name))
		return
	}

	var project []paperassets.ProjectResource
	switch request.Operation {
	case "add":
		project, err = paperassets.AddProjectResource(manifestPath, assetRoot, paperassets.ResourceSpec{
			Name: request.Name, MediaType: request.MediaType, Path: request.Path, Family: request.Family,
			Weight: request.Weight, Style: request.Style, License: request.License, Fallback: request.Fallback,
			Replaces: request.Replaces, FocusX: request.FocusX, FocusY: request.FocusY,
		})
	case "remove":
		project, err = paperassets.RemoveProjectResource(manifestPath, assetRoot, request.Name)
	}
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if err := s.setProjectResources(project); err != nil {
		writeStudioError(w, http.StatusInternalServerError, err)
		return
	}
	after, err := s.current(ctx, request.Scenario)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	parsedAfter := paperlang.Parse(after.file, after.source)
	resources, editable := s.resourceSnapshot()
	inventory, err := buildStudioResourceInventory(after.revision, after.plan.Hash(), after.scenario, parsedAfter.AST, resources)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	inventory.SourceRevision = studioSourceRevision(after.source)
	inventory.CatalogEditable = editable
	writeStudioJSON(w, http.StatusOK, studioResourceCatalogResponse{OK: true, Operation: request.Operation, Name: request.Name, Inventory: inventory})
}

func validateStudioResourceCatalogRequest(request studioResourceCatalogRequest) error {
	fields := []string{request.SourceRevision, request.PlanRevision, request.Scenario, request.Operation, request.Name, request.Path, request.MediaType, request.Family, request.Style, request.License, request.Replaces}
	for _, field := range fields {
		if len(field) > studioEditFieldLimit || !utf8.ValidString(field) {
			return errors.New("paper-studio: resource catalog field exceeds its bound")
		}
	}
	if request.SourceRevision == "" || request.PlanRevision == "" || request.Name == "" {
		return errors.New("paper-studio: exact resource revisions and name are required")
	}
	if request.Operation != "add" && request.Operation != "remove" {
		return errors.New("paper-studio: resource catalog operation must be add or remove")
	}
	if request.Operation == "add" && (request.Path == "" || request.MediaType == "") {
		return errors.New("paper-studio: adding a resource requires path and media type")
	}
	if request.Operation == "remove" && (request.Path != "" || request.MediaType != "" || request.Family != "" || request.Style != "" || request.License != "" || request.Replaces != "" || len(request.Fallback) != 0 || request.FocusX != nil || request.FocusY != nil || request.Weight != 0) {
		return errors.New("paper-studio: removing a resource accepts only its name and exact revisions")
	}
	for _, fallback := range request.Fallback {
		if len(fallback) > studioEditFieldLimit || !utf8.ValidString(fallback) {
			return errors.New("paper-studio: fallback name exceeds its bound")
		}
	}
	for _, value := range []*float64{request.FocusX, request.FocusY} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return errors.New("paper-studio: resource focus must be finite")
		}
	}
	return nil
}

func studioASTUsesResource(root *paperlang.Node, name string) bool {
	var used bool
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil || used {
			return
		}
		if node.Kind == paperlang.NodeImage {
			for _, member := range node.Members {
				if member.Property != nil && member.Property.Name == "source" && member.Property.Value.StringValue != nil && *member.Property.Value.StringValue == "asset:"+name {
					used = true
					return
				}
			}
		}
		for _, member := range node.Members {
			walk(member.Node)
		}
	}
	walk(root)
	return used
}
