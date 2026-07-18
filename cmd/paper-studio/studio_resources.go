// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/paperassets"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

func (s *studioServer) handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleResourceCatalogMutation(w, r)
		return
	}
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet+", "+http.MethodPost)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), studioAPITimeout)
	defer cancel()
	_ = ctx
	snapshot, err := s.current(ctx, r.URL.Query().Get("scenario"))
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	if r.URL.Query().Get("revision") != snapshot.revision {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: stale resource inventory revision"))
		return
	}
	if r.URL.Query().Get("source_revision") != studioSourceRevision(snapshot.source) {
		writeStudioError(w, http.StatusConflict, errors.New("paper-studio: stale resource inventory source revision"))
		return
	}
	parsed := paperlang.Parse(snapshot.file, snapshot.source)
	if !parsed.OK() {
		writeStudioError(w, http.StatusUnprocessableEntity, errors.New("paper-studio: source AST is unavailable"))
		return
	}
	resources, editable := s.resourceSnapshot()
	inventory, err := buildStudioResourceInventory(snapshot.revision, snapshot.plan.Hash(), snapshot.scenario, parsed.AST, resources)
	if err != nil {
		writeStudioError(w, http.StatusUnprocessableEntity, err)
		return
	}
	inventory.SourceRevision = studioSourceRevision(snapshot.source)
	inventory.CatalogEditable = editable
	writeStudioJSON(w, http.StatusOK, inventory)
}

const studioMaxAssetUsages = 4096

func (s *studioServer) setAssetResources(resources []document.PaperAssetResource) error {
	project := make([]paperassets.ProjectResource, len(resources))
	for i, r := range resources {
		project[i] = paperassets.ProjectResource{Name: r.Name, MediaType: r.MediaType, Digest: r.Digest, Data: r.Data}
	}
	if err := s.setProjectResources(project); err != nil {
		return err
	}
	s.mu.Lock()
	s.resourceManifest, s.resourceRoot = "", ""
	s.mu.Unlock()
	return nil
}

func (s *studioServer) setProjectManifest(manifestPath, assetRoot string, project []paperassets.ProjectResource) error {
	manifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return err
	}
	if assetRoot == "" {
		assetRoot = filepath.Dir(manifestPath)
	}
	assetRoot, err = filepath.Abs(assetRoot)
	if err != nil {
		return err
	}
	if err := s.setProjectResources(project); err != nil {
		return err
	}
	s.mu.Lock()
	s.resourceManifest, s.resourceRoot = manifestPath, assetRoot
	s.mu.Unlock()
	return nil
}

func (s *studioServer) setProjectResources(project []paperassets.ProjectResource) error {
	resources := make([]document.PaperAssetResource, 0, len(project))
	for _, r := range project {
		if r.MediaType == "image/png" || r.MediaType == "image/jpeg" || r.MediaType == "font/ttf" || r.MediaType == "font/otf" {
			resources = append(resources, document.PaperAssetResource{Name: r.Name, MediaType: r.MediaType, Digest: r.Digest, Data: r.Data, Family: r.Family, Style: r.Style, Weight: r.Weight, License: r.License})
		}
	}
	catalog, err := document.NewPaperAssetCatalog(resources)
	if err != nil {
		return err
	}
	detached := make([]document.PaperAssetResource, len(resources))
	for i, resource := range resources {
		detached[i] = document.PaperAssetResource{Name: resource.Name, MediaType: resource.MediaType, Digest: resource.Digest, Data: append([]byte(nil), resource.Data...), Family: resource.Family, Style: resource.Style, Weight: resource.Weight, License: resource.License}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets, s.assetCatalog = detached, catalog
	s.projectResources = make([]paperassets.ProjectResource, len(project))
	for i, r := range project {
		s.projectResources[i] = r
		s.projectResources[i].Data = append([]byte(nil), r.Data...)
		s.projectResources[i].Fallback = append([]string(nil), r.Fallback...)
		s.projectResources[i].FocusX = cloneStudioFloat(r.FocusX)
		s.projectResources[i].FocusY = cloneStudioFloat(r.FocusY)
	}
	clear(s.snapshots)
	s.hasSourceHash = false
	return nil
}

func (s *studioServer) resourceSnapshot() ([]paperassets.ProjectResource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resources := append([]paperassets.ProjectResource(nil), s.projectResources...)
	for i := range resources {
		resources[i].Data = append([]byte(nil), resources[i].Data...)
		resources[i].Fallback = append([]string(nil), resources[i].Fallback...)
		resources[i].FocusX = cloneStudioFloat(resources[i].FocusX)
		resources[i].FocusY = cloneStudioFloat(resources[i].FocusY)
	}
	if len(resources) == 0 {
		for _, asset := range s.assets {
			resources = append(resources, paperassets.ProjectResource{Name: asset.Name, MediaType: asset.MediaType, Digest: asset.Digest, Data: append([]byte(nil), asset.Data...)})
		}
	}
	return resources, s.resourceManifest != ""
}

func (s *studioServer) projectManifestConfig() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resourceManifest, s.resourceRoot
}

func cloneStudioFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type studioAssetUsage struct {
	Node       string   `json:"node"`
	Scenario   string   `json:"scenario"`
	Alt        string   `json:"alt,omitempty"`
	Decorative bool     `json:"decorative"`
	FocusX     *float64 `json:"focus_x,omitempty"`
	FocusY     *float64 `json:"focus_y,omitempty"`
}

type studioAssetInventoryItem struct {
	Name          string             `json:"name"`
	MediaType     string             `json:"media_type"`
	Digest        string             `json:"digest"`
	Bytes         uint64             `json:"bytes"`
	Width         uint32             `json:"width_px"`
	Height        uint32             `json:"height_px"`
	Usages        []studioAssetUsage `json:"usages,omitempty"`
	Kind          string             `json:"kind"`
	Family        string             `json:"family,omitempty"`
	Weight        uint16             `json:"weight,omitempty"`
	Style         string             `json:"style,omitempty"`
	License       string             `json:"license,omitempty"`
	Fallback      []string           `json:"fallback,omitempty"`
	Replaces      string             `json:"replaces,omitempty"`
	DefaultFocusX *float64           `json:"default_focus_x,omitempty"`
	DefaultFocusY *float64           `json:"default_focus_y,omitempty"`
}

type studioAssetInventory struct {
	FormatVersion   uint16                     `json:"format_version"`
	Revision        string                     `json:"revision"`
	SourceRevision  string                     `json:"source_revision"`
	PlanHash        string                     `json:"plan_hash"`
	Scenario        string                     `json:"scenario"`
	CatalogEditable bool                       `json:"catalog_editable"`
	Items           []studioAssetInventoryItem `json:"items"`
}

func buildStudioAssetInventory(revision, planHash, scenario string, ast paperlang.AST, resources []document.PaperAssetResource) (studioAssetInventory, error) {
	project := make([]paperassets.ProjectResource, len(resources))
	for i, r := range resources {
		project[i] = paperassets.ProjectResource{Name: r.Name, MediaType: r.MediaType, Digest: r.Digest, Data: r.Data}
	}
	return buildStudioResourceInventory(revision, planHash, scenario, ast, project)
}

func buildStudioResourceInventory(revision, planHash, scenario string, ast paperlang.AST, resources []paperassets.ProjectResource) (studioAssetInventory, error) {
	if revision == "" || planHash == "" || ast.Root == nil {
		return studioAssetInventory{}, errors.New("paper-studio: asset inventory requires exact revision, plan, and AST")
	}
	items := make([]studioAssetInventoryItem, len(resources))
	byName := make(map[string]int, len(resources))
	for i, resource := range resources {
		item := studioAssetInventoryItem{Name: resource.Name, MediaType: resource.MediaType, Digest: resource.Digest, Bytes: uint64(len(resource.Data)), Family: resource.Family, Weight: resource.Weight, Style: resource.Style, License: resource.License, Fallback: append([]string(nil), resource.Fallback...), Replaces: resource.Replaces, DefaultFocusX: resource.FocusX, DefaultFocusY: resource.FocusY}
		if strings.HasPrefix(resource.MediaType, "image/") {
			config, _, err := image.DecodeConfig(bytes.NewReader(resource.Data))
			if err != nil || config.Width <= 0 || config.Height <= 0 {
				return studioAssetInventory{}, errors.New("paper-studio: asset dimensions are invalid")
			}
			item.Kind = "image"
			item.Width = uint32(config.Width)
			item.Height = uint32(config.Height)
		} else if strings.HasPrefix(resource.MediaType, "font/") {
			item.Kind = "font"
		} else {
			return studioAssetInventory{}, errors.New("paper-studio: unsupported resource media type")
		}
		items[i] = item
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	for i := range items {
		byName[items[i].Name] = i
	}
	usageCount := 0
	var walk func(*paperlang.Node) error
	walk = func(node *paperlang.Node) error {
		if node == nil {
			return nil
		}
		if node.Kind == paperlang.NodeImage {
			source, alt := "", ""
			decorative := false
			var focusX, focusY *float64
			for _, member := range node.Members {
				p := member.Property
				if p == nil {
					continue
				}
				switch p.Name {
				case "source":
					if p.Value.StringValue != nil {
						source = *p.Value.StringValue
					}
				case "alt":
					if p.Value.StringValue != nil {
						alt = *p.Value.StringValue
					}
				case "decorative":
					if p.Value.BoolValue != nil {
						decorative = *p.Value.BoolValue
					}
				case "focus-x":
					if p.Value.NumberValue != nil {
						v := *p.Value.NumberValue
						focusX = &v
					}
				case "focus-y":
					if p.Value.NumberValue != nil {
						v := *p.Value.NumberValue
						focusY = &v
					}
				}
			}
			if strings.HasPrefix(source, "asset:") {
				name := strings.TrimPrefix(source, "asset:")
				index, ok := byName[name]
				if !ok {
					return errors.New("paper-studio: authored asset reference is absent from catalog")
				}
				usageCount++
				if usageCount > studioMaxAssetUsages {
					return errors.New("paper-studio: asset usage limit exceeded")
				}
				items[index].Usages = append(items[index].Usages, studioAssetUsage{Node: node.ID, Scenario: scenario, Alt: alt, Decorative: decorative, FocusX: focusX, FocusY: focusY})
			}
		}
		for _, member := range node.Members {
			if err := walk(member.Node); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(ast.Root); err != nil {
		return studioAssetInventory{}, err
	}
	for i := range items {
		sort.Slice(items[i].Usages, func(a, b int) bool {
			left, right := items[i].Usages[a], items[i].Usages[b]
			if left.Node != right.Node {
				return left.Node < right.Node
			}
			return left.Scenario < right.Scenario
		})
	}
	return studioAssetInventory{FormatVersion: 1, Revision: revision, PlanHash: planHash, Scenario: scenario, Items: items}, nil
}
