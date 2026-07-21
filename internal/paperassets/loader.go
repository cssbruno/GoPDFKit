// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperassets

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cssbruno/paperrune/internal/papercompile"
)

const MaxManifestBytes = 1 << 20

type manifest struct {
	Assets []entry `json:"assets"`
}
type entry struct {
	Name      string   `json:"name"`
	MediaType string   `json:"media_type"`
	Digest    string   `json:"sha256"`
	Path      string   `json:"path"`
	Family    string   `json:"family,omitempty"`
	Weight    uint16   `json:"weight,omitempty"`
	Style     string   `json:"style,omitempty"`
	License   string   `json:"license,omitempty"`
	Fallback  []string `json:"fallback,omitempty"`
	Replaces  string   `json:"replaces,omitempty"`
	FocusX    *float64 `json:"focus_x,omitempty"`
	FocusY    *float64 `json:"focus_y,omitempty"`
}

// ProjectResource retains private verified bytes with human-readable lifecycle
// metadata. Consumers must project metadata explicitly and never serialize Data.
type ProjectResource struct {
	Name, MediaType, Digest          string
	Data                             []byte
	Family, Style, License, Replaces string
	Weight                           uint16
	Fallback                         []string
	FocusX, FocusY                   *float64
}

// LoadManifest reads an explicitly named manifest and only regular,
// non-symlink project-root-relative files named by it.
func LoadManifest(manifestPath, assetRoot string) ([]papercompile.AssetResource, error) {
	project, err := LoadProjectManifest(manifestPath, assetRoot)
	if err != nil {
		return nil, err
	}
	resources := make([]papercompile.AssetResource, 0, len(project))
	for _, item := range project {
		if item.MediaType == "image/png" || item.MediaType == "image/jpeg" {
			resources = append(resources, papercompile.AssetResource{Name: item.Name, MediaType: item.MediaType, Digest: item.Digest, Data: append([]byte(nil), item.Data...)})
		}
	}
	return resources, nil
}

// LoadProjectManifest validates the complete explicit resource catalog,
// including non-layout font metadata and replacement/fallback relationships.
func LoadProjectManifest(manifestPath, assetRoot string) ([]ProjectResource, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return nil, errors.New("paperassets: manifest path is required")
	}
	manifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, err
	}
	manifestBytes, err := readRegularBounded(manifestPath, MaxManifestBytes)
	if err != nil {
		return nil, fmt.Errorf("paperassets: manifest: %w", err)
	}
	var decoded manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("paperassets: decode manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("paperassets: manifest has trailing JSON")
	}
	if assetRoot == "" {
		assetRoot = filepath.Dir(manifestPath)
	}
	assetRoot, err = filepath.Abs(assetRoot)
	if err != nil {
		return nil, err
	}
	rootInfo, err := os.Lstat(assetRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("paperassets: asset root must be a real directory")
	}
	if len(decoded.Assets) > papercompile.MaxAssetCatalogResources {
		return nil, errors.New("paperassets: asset count exceeds limit")
	}
	resources := make([]ProjectResource, 0, len(decoded.Assets))
	var total int
	for index, item := range decoded.Assets {
		relative, err := safeRelative(item.Path)
		if err != nil {
			return nil, fmt.Errorf("paperassets: assets[%d].path: %w", index, err)
		}
		full := filepath.Join(assetRoot, relative)
		if err := rejectSymlinkPath(assetRoot, relative); err != nil {
			return nil, fmt.Errorf("paperassets: assets[%d].path: %w", index, err)
		}
		data, err := readRegularBounded(full, papercompile.MaxAssetResourceBytes)
		if err != nil {
			return nil, fmt.Errorf("paperassets: assets[%d]: %w", index, err)
		}
		total += len(data)
		if total > papercompile.MaxAssetCatalogBytes {
			return nil, errors.New("paperassets: cumulative bytes exceed limit")
		}
		resources = append(resources, ProjectResource{Name: item.Name, MediaType: item.MediaType, Digest: item.Digest, Data: data,
			Family: item.Family, Weight: item.Weight, Style: item.Style, License: item.License, Fallback: append([]string(nil), item.Fallback...), Replaces: item.Replaces, FocusX: item.FocusX, FocusY: item.FocusY})
	}
	if err := validateProjectResources(resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func validateProjectResources(resources []ProjectResource) error {
	seen := make(map[string]ProjectResource, len(resources))
	images := make([]papercompile.AssetResource, 0, len(resources))
	for index := range resources {
		item := &resources[index]
		if !portableName(item.Name) {
			return fmt.Errorf("paperassets: resources[%d].name is not a portable identifier", index)
		}
		if _, duplicate := seen[item.Name]; duplicate {
			return fmt.Errorf("paperassets: duplicate resource name %q", item.Name)
		}
		digest := fmt.Sprintf("%x", sha256Sum(item.Data))
		if item.Digest != digest {
			return fmt.Errorf("paperassets: resources[%d].digest does not match its bytes", index)
		}
		switch item.MediaType {
		case "image/png", "image/jpeg":
			if item.Family != "" || item.Weight != 0 || item.Style != "" || len(item.Fallback) != 0 {
				return fmt.Errorf("paperassets: image %q has font-only metadata", item.Name)
			}
			if !focusOK(item.FocusX) || !focusOK(item.FocusY) {
				return fmt.Errorf("paperassets: image %q focus must be between zero and one", item.Name)
			}
			images = append(images, papercompile.AssetResource{Name: item.Name, MediaType: item.MediaType, Digest: item.Digest, Data: item.Data})
		case "font/ttf", "font/otf", "font/woff2":
			if item.Family == "" || len(item.Family) > 128 || item.License == "" || len(item.License) > 256 || item.FocusX != nil || item.FocusY != nil {
				return fmt.Errorf("paperassets: font %q requires bounded family/license and no image focus", item.Name)
			}
			if item.Weight == 0 {
				item.Weight = 400
			}
			if item.Weight > 1000 {
				return fmt.Errorf("paperassets: font %q weight is invalid", item.Name)
			}
			if item.Style == "" {
				item.Style = "normal"
			}
			if item.Style != "normal" && item.Style != "italic" && item.Style != "oblique" {
				return fmt.Errorf("paperassets: font %q style is invalid", item.Name)
			}
			if !fontSignature(item.MediaType, item.Data) {
				return fmt.Errorf("paperassets: font %q signature does not match media type", item.Name)
			}
		default:
			return fmt.Errorf("paperassets: resource %q has unsupported media type", item.Name)
		}
		seen[item.Name] = *item
	}
	if _, err := papercompile.NewAssetCatalog(images); err != nil {
		return err
	}
	for _, item := range resources {
		if item.Replaces != "" {
			target, ok := seen[item.Replaces]
			if !ok || resourceKind(target.MediaType) != resourceKind(item.MediaType) || item.Replaces == item.Name {
				return fmt.Errorf("paperassets: resource %q has invalid replacement target", item.Name)
			}
		}
		if strings.HasPrefix(item.MediaType, "font/") {
			for _, fallback := range item.Fallback {
				target, ok := seen[fallback]
				if !ok || !strings.HasPrefix(target.MediaType, "font/") || fallback == item.Name {
					return fmt.Errorf("paperassets: font %q has invalid fallback", item.Name)
				}
			}
		} else if len(item.Fallback) != 0 {
			return fmt.Errorf("paperassets: non-font %q has fallback metadata", item.Name)
		}
	}
	if relationshipCycle(resources, func(item ProjectResource) []string {
		if item.Replaces == "" {
			return nil
		}
		return []string{item.Replaces}
	}) || relationshipCycle(resources, func(item ProjectResource) []string { return item.Fallback }) {
		return errors.New("paperassets: resource lifecycle contains a cycle")
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Name < resources[j].Name })
	return nil
}

func resourceKind(media string) string {
	if strings.HasPrefix(media, "image/") {
		return "image"
	}
	if strings.HasPrefix(media, "font/") {
		return "font"
	}
	return ""
}

func portableName(name string) bool {
	if len(name) == 0 || len(name) > 128 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			continue
		}
		return false
	}
	return true
}
func focusOK(value *float64) bool { return value == nil || (*value >= 0 && *value <= 1) }
func fontSignature(media string, data []byte) bool {
	if len(data) < 4 {
		return false
	}
	magic := string(data[:4])
	switch media {
	case "font/ttf":
		return magic == "\x00\x01\x00\x00" || magic == "true"
	case "font/otf":
		return magic == "OTTO"
	case "font/woff2":
		return magic == "wOF2"
	}
	return false
}
func relationshipCycle(resources []ProjectResource, edges func(ProjectResource) []string) bool {
	by := make(map[string]ProjectResource, len(resources))
	for _, r := range resources {
		by[r.Name] = r
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(name string) bool {
		if state[name] == 1 {
			return true
		}
		if state[name] == 2 {
			return false
		}
		state[name] = 1
		for _, next := range edges(by[name]) {
			if visit(next) {
				return true
			}
		}
		state[name] = 2
		return false
	}
	for name := range by {
		if visit(name) {
			return true
		}
	}
	return false
}
func sha256Sum(data []byte) [32]byte { return sha256.Sum256(data) }

func safeRelative(value string) (string, error) {
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return "", errors.New("path must be non-empty relative slash syntax")
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.ToSlash(clean) != value {
		return "", errors.New("path is non-canonical or traverses the root")
	}
	return clean, nil
}
func rejectSymlinkPath(root, relative string) error {
	current := root
	for _, part := range strings.Split(filepath.ToSlash(relative), "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symlink components are forbidden")
		}
	}
	return nil
}
func readRegularBounded(path string, limit int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("file must be regular and not a symlink")
	}
	if info.Size() < 0 || info.Size() > int64(limit) {
		return nil, errors.New("file exceeds limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, errors.New("file identity changed while opening")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, errors.New("file exceeds limit")
	}
	return data, nil
}
