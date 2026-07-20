// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

// Package paperdoc defines the self-contained Paper Document interchange
// format. A Paper Document is a deterministic ZIP container with one editable
// .paper source and content-addressed image and font resources.
package paperdoc

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperassets"
	"github.com/cssbruno/paperrune/internal/paperpkg"
)

const (
	MediaType          = "application/vnd.paperrune.paperdoc+zip"
	Format             = "org.paperrune.paperdoc"
	SchemaVersion      = 1
	MaxSourceBytes     = 8 << 20
	MaxManifestBytes   = 1 << 20
	mimetypePath       = "mimetype"
	manifestPath       = "manifest.json"
	documentSourcePath = "document.paper"
)

var (
	ErrInvalid   = errors.New("paperdoc: invalid document")
	ErrIntegrity = errors.New("paperdoc: integrity check failed")
)

// Document is the complete editable state carried by a .paperdoc file.
type Document struct {
	Source    string
	Imports   map[string]string
	Resources []paperassets.ProjectResource
}

type manifest struct {
	Format       string             `json:"format"`
	Version      uint16             `json:"version"`
	Source       string             `json:"source"`
	SourceSHA256 string             `json:"source_sha256"`
	Imports      []manifestImport   `json:"imports,omitempty"`
	Resources    []manifestResource `json:"resources"`
}

type manifestImport struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type manifestResource struct {
	Name      string   `json:"name"`
	MediaType string   `json:"media_type"`
	SHA256    string   `json:"sha256"`
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

// Encode creates byte-for-byte deterministic package bytes for a document.
func Encode(document Document) ([]byte, error) {
	if err := validateSource(document.Source); err != nil {
		return nil, err
	}
	resources, err := paperassets.ValidateProjectResources(document.Resources)
	if err != nil {
		return nil, fmt.Errorf("%w: resources: %w", ErrInvalid, err)
	}
	sourceDigest := sha256.Sum256([]byte(document.Source))
	metadata := manifest{Format: Format, Version: SchemaVersion, Source: documentSourcePath,
		SourceSHA256: hex.EncodeToString(sourceDigest[:]), Resources: make([]manifestResource, len(resources))}
	files := map[string][]byte{mimetypePath: []byte(MediaType), documentSourcePath: []byte(document.Source)}
	importPaths := make([]string, 0, len(document.Imports))
	for importPath := range document.Imports {
		importPaths = append(importPaths, importPath)
	}
	sort.Strings(importPaths)
	metadata.Imports = make([]manifestImport, len(importPaths))
	for index, importPath := range importPaths {
		if canonicalImportPath(importPath) != importPath {
			return nil, fmt.Errorf("%w: import path %q is not canonical", ErrInvalid, importPath)
		}
		importSource := document.Imports[importPath]
		if err := validateSource(importSource); err != nil {
			return nil, fmt.Errorf("%w: import %q: %w", ErrInvalid, importPath, err)
		}
		archivePath := importArchivePath(importPath)
		files[archivePath] = []byte(importSource)
		metadata.Imports[index] = manifestImport{Path: importPath, SHA256: digest([]byte(importSource))}
	}
	for index, resource := range resources {
		path, pathErr := resourcePath(resource)
		if pathErr != nil {
			return nil, pathErr
		}
		if existing, exists := files[path]; exists && !bytes.Equal(existing, resource.Data) {
			return nil, fmt.Errorf("%w: resource path collision", ErrIntegrity)
		}
		files[path] = append([]byte(nil), resource.Data...)
		metadata.Resources[index] = manifestResource{Name: resource.Name, MediaType: resource.MediaType,
			SHA256: resource.Digest, Path: path, Family: resource.Family, Weight: resource.Weight,
			Style: resource.Style, License: resource.License, Fallback: append([]string(nil), resource.Fallback...),
			Replaces: resource.Replaces, FocusX: cloneFloat(resource.FocusX), FocusY: cloneFloat(resource.FocusY)}
	}
	encodedManifest, err := json.Marshal(metadata)
	if err != nil || len(encodedManifest) > MaxManifestBytes {
		return nil, fmt.Errorf("%w: manifest exceeds its bound", ErrInvalid)
	}
	files[manifestPath] = encodedManifest
	return encodeArchive(files)
}

// Decode validates and detaches a complete Paper Document.
func Decode(ctx context.Context, encoded []byte) (Document, error) {
	if ctx == nil {
		return Document{}, fmt.Errorf("%w: context is required", ErrInvalid)
	}
	plan, err := paperpkg.ValidateArchive(ctx, encoded, paperpkg.DefaultArchiveLimits())
	if err != nil {
		return Document{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	files := make(map[string][]byte, len(plan.Entries))
	for _, entry := range plan.Entries {
		files[entry.Path] = entry.Bytes
	}
	if string(files[mimetypePath]) != MediaType {
		return Document{}, fmt.Errorf("%w: unsupported media type", ErrInvalid)
	}
	manifestBytes := files[manifestPath]
	if len(manifestBytes) == 0 || len(manifestBytes) > MaxManifestBytes {
		return Document{}, fmt.Errorf("%w: missing or oversized manifest", ErrInvalid)
	}
	var metadata manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return Document{}, fmt.Errorf("%w: manifest: %w", ErrInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Document{}, fmt.Errorf("%w: manifest has trailing JSON", ErrInvalid)
	}
	canonical, err := json.Marshal(metadata)
	if err != nil || !bytes.Equal(canonical, manifestBytes) {
		return Document{}, fmt.Errorf("%w: manifest is not canonical", ErrInvalid)
	}
	if metadata.Format != Format || metadata.Version != SchemaVersion || metadata.Source != documentSourcePath {
		return Document{}, fmt.Errorf("%w: unsupported format or version", ErrInvalid)
	}
	sourceBytes, ok := files[metadata.Source]
	if !ok || validateSource(string(sourceBytes)) != nil || digest(sourceBytes) != metadata.SourceSHA256 {
		return Document{}, fmt.Errorf("%w: source", ErrIntegrity)
	}
	resources := make([]paperassets.ProjectResource, len(metadata.Resources))
	expectedFiles := map[string]bool{mimetypePath: true, manifestPath: true, documentSourcePath: true}
	imports := make(map[string]string, len(metadata.Imports))
	previousImport := ""
	for index, item := range metadata.Imports {
		if canonicalImportPath(item.Path) != item.Path || index > 0 && item.Path <= previousImport {
			return Document{}, fmt.Errorf("%w: imports are not canonical, sorted, and unique", ErrInvalid)
		}
		previousImport = item.Path
		archivePath := importArchivePath(item.Path)
		importBytes, exists := files[archivePath]
		if !exists || validateSource(string(importBytes)) != nil || digest(importBytes) != item.SHA256 {
			return Document{}, fmt.Errorf("%w: import %q", ErrIntegrity, item.Path)
		}
		expectedFiles[archivePath] = true
		imports[item.Path] = string(importBytes)
	}
	previousName := ""
	for index, item := range metadata.Resources {
		if index > 0 && item.Name <= previousName {
			return Document{}, fmt.Errorf("%w: resources are not sorted and unique", ErrInvalid)
		}
		previousName = item.Name
		data, exists := files[item.Path]
		if !exists || digest(data) != item.SHA256 {
			return Document{}, fmt.Errorf("%w: resource %q", ErrIntegrity, item.Name)
		}
		expectedPath, pathErr := resourcePath(paperassets.ProjectResource{MediaType: item.MediaType, Digest: item.SHA256})
		if pathErr != nil || item.Path != expectedPath {
			return Document{}, fmt.Errorf("%w: resource %q has a non-canonical path", ErrInvalid, item.Name)
		}
		expectedFiles[item.Path] = true
		resources[index] = paperassets.ProjectResource{Name: item.Name, MediaType: item.MediaType, Digest: item.SHA256,
			Path: item.Path, Data: append([]byte(nil), data...), Family: item.Family, Weight: item.Weight,
			Style: item.Style, License: item.License, Fallback: append([]string(nil), item.Fallback...),
			Replaces: item.Replaces, FocusX: cloneFloat(item.FocusX), FocusY: cloneFloat(item.FocusY)}
	}
	if len(files) != len(expectedFiles) {
		return Document{}, fmt.Errorf("%w: undeclared archive entry", ErrInvalid)
	}
	resources, err = paperassets.ValidateProjectResources(resources)
	if err != nil {
		return Document{}, fmt.Errorf("%w: resources: %w", ErrInvalid, err)
	}
	return Document{Source: string(sourceBytes), Imports: imports, Resources: resources}, nil
}

func validateSource(source string) error {
	if len(source) == 0 || len(source) > MaxSourceBytes || !utf8.ValidString(source) {
		return fmt.Errorf("%w: source must be bounded non-empty UTF-8", ErrInvalid)
	}
	return nil
}

func resourcePath(resource paperassets.ProjectResource) (string, error) {
	extension := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "font/ttf": ".ttf", "font/otf": ".otf", "font/woff2": ".woff2"}[resource.MediaType]
	if extension == "" || len(resource.Digest) != 64 {
		return "", fmt.Errorf("%w: unsupported resource identity", ErrInvalid)
	}
	prefix := "assets/"
	if strings.HasPrefix(resource.MediaType, "font/") {
		prefix = "fonts/"
	}
	return prefix + resource.Digest + extension, nil
}

func canonicalImportPath(value string) string {
	if value == "" || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') || strings.HasPrefix(value, "/") {
		return ""
	}
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.Ext(clean) != ".paper" {
		return ""
	}
	return clean
}

func importArchivePath(importPath string) string { return "sources/" + importPath }

func encodeArchive(files map[string][]byte) ([]byte, error) {
	paths := make([]string, 0, len(files)-1)
	for path := range files {
		if path != mimetypePath {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	paths = append([]string{mimetypePath}, paths...)
	var destination bytes.Buffer
	writer := zip.NewWriter(&destination)
	for _, path := range paths {
		header := &zip.FileHeader{Name: path, Method: zip.Store}
		header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		header.SetMode(0o600)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := entry.Write(files[path]); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return destination.Bytes(), nil
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

// BaseName returns the portable download name for a source or package path.
func BaseName(path string) string {
	base := filepath.Base(path)
	extension := filepath.Ext(base)
	return strings.TrimSuffix(base, extension) + ".paperdoc"
}
