package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/paperassets"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

func TestStudioAssetInventoryIsBoundedDeterministicRevisionBoundAndBytePrivate(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	resources := []document.PaperAssetResource{{Name: "hero", MediaType: "image/png", Digest: hex.EncodeToString(digest[:]), Data: data}}
	source := "document @report:\n  page @sheet:\n    body @body:\n      image @hero-node:\n        source: \"asset:hero\"\n        alt: \"Evidence\"\n        focus-x: 0.25\n        focus-y: 0.75\n"
	parsed := paperlang.Parse("asset.paper", source)
	if !parsed.OK() {
		t.Fatal(parsed.Diagnostics)
	}
	first, err := buildStudioAssetInventory("plan-1", "plan-1", "stress", parsed.AST, resources)
	if err != nil {
		t.Fatal(err)
	}
	second, err := buildStudioAssetInventory("plan-1", "plan-1", "stress", parsed.AST, resources)
	if err != nil {
		t.Fatal(err)
	}
	left, _ := json.Marshal(first)
	right, _ := json.Marshal(second)
	if string(left) != string(right) {
		t.Fatalf("nondeterministic inventory")
	}
	if len(first.Items) != 1 || first.Items[0].Width != 1 || first.Items[0].Height != 1 || len(first.Items[0].Usages) != 1 || first.Items[0].Usages[0].Node != "@hero-node" {
		t.Fatalf("inventory=%#v", first)
	}
	if json.Valid(data) && string(left) == string(data) {
		t.Fatal("raw bytes leaked")
	}
	var decoded map[string]any
	if err := json.Unmarshal(left, &decoded); err != nil {
		t.Fatal(err)
	}
	if string(left) == "" || containsJSONKey(decoded, "data") {
		t.Fatalf("raw data field leaked: %s", left)
	}
}

func TestStudioResourceInventoryProjectsFontLifecycleAndImageDefaultsWithoutBytes(t *testing.T) {
	imageData, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	fontData := []byte{0, 1, 0, 0, 0, 0, 0, 1}
	ix, iy := 0.2, 0.8
	id, fd := sha256.Sum256(imageData), sha256.Sum256(fontData)
	resources := []paperassets.ProjectResource{
		{Name: "body-font", MediaType: "font/ttf", Digest: hex.EncodeToString(fd[:]), Data: fontData, Family: "Readable Sans", Weight: 500, Style: "italic", License: "OFL-1.1", Fallback: []string{"fallback-font"}},
		{Name: "fallback-font", MediaType: "font/ttf", Digest: hex.EncodeToString(fd[:]), Data: fontData, Family: "Fallback Sans", Weight: 400, Style: "normal", License: "OFL-1.1"},
		{Name: "hero", MediaType: "image/png", Digest: hex.EncodeToString(id[:]), Data: imageData, FocusX: &ix, FocusY: &iy, Replaces: "old-hero"},
	}
	parsed := paperlang.Parse("resources.paper", "document @report:\n  page @page:\n    body @body:\n      image @picture:\n        source: \"asset:hero\"\n        alt: \"Evidence\"\n")
	inventory, err := buildStudioResourceInventory("revision-1", "plan-1", "", parsed.AST, resources)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(inventory)
	if bytes.Contains(encoded, fontData) || bytes.Contains(encoded, imageData) || bytes.Contains(encoded, []byte("path")) {
		t.Fatalf("private resource data leaked: %s", encoded)
	}
	if len(inventory.Items) != 3 || inventory.Items[0].Name != "body-font" || inventory.Items[0].Kind != "font" || inventory.Items[0].Family != "Readable Sans" || inventory.Items[2].Kind != "image" || inventory.Items[2].DefaultFocusX == nil || inventory.Items[2].Replaces != "old-hero" {
		t.Fatalf("inventory=%+v", inventory.Items)
	}
}

func TestStudioResourceReplacementCommitsExactCatalogReference(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	file := filepath.Join(t.TempDir(), "replace.paper")
	source := "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n        source: \"asset:old-hero\"\n        width: 20pt\n        height: 20pt\n        alt: \"Evidence\"\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := studio.setProjectResources([]paperassets.ProjectResource{{Name: "old-hero", MediaType: "image/png", Digest: hash, Data: data}, {Name: "new-hero", MediaType: "image/png", Digest: hash, Data: data, Replaces: "old-hero"}}); err != nil {
		t.Fatal(err)
	}
	workspace := fetchStudioWorkspace(t, studio.routes())
	response := postStudioJSON(t, studio.routes(), "/api/edit", map[string]any{"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision, "operation": "image", "target": "@hero", "property": "source", "text": "asset:new-hero"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("replacement=%d %s", response.StatusCode, response.Body)
	}
	written, _ := os.ReadFile(file)
	if !bytes.Contains(written, []byte(`source: "asset:new-hero"`)) {
		t.Fatalf("source=%s", written)
	}
}

func containsJSONKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for name, child := range typed {
			if name == key || containsJSONKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsJSONKey(child, key) {
				return true
			}
		}
	}
	return false
}

func TestStudioAssetInventoryRejectsStaleAndMissingInputs(t *testing.T) {
	if _, err := buildStudioAssetInventory("", "plan", "", paperlang.AST{}, nil); err == nil {
		t.Fatal("missing binding accepted")
	}
}

func TestStudioResourcesEndpointBindsExactPlanWithoutReturningBytes(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	digest := sha256.Sum256(data)
	resource := document.PaperAssetResource{Name: "hero", MediaType: "image/png", Digest: hex.EncodeToString(digest[:]), Data: data}
	dir := t.TempDir()
	file := filepath.Join(dir, "asset.paper")
	source := "document @report:\n  page @sheet:\n    width: 100pt\n    height: 80pt\n    margin: 8pt\n    body @body:\n      image @hero-node:\n        source: \"asset:hero\"\n        width: 20pt\n        height: 20pt\n        alt: \"Evidence\"\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := studio.setAssetResources([]document.PaperAssetResource{resource}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := studio.current(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/resources?revision="+snapshot.revision+"&source_revision="+studioSourceRevision(snapshot.source), nil)
	response := httptest.NewRecorder()
	studio.routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), data) || bytes.Contains(response.Body.Bytes(), []byte("Data")) {
		t.Fatalf("raw bytes leaked: %s", response.Body.String())
	}
	var inventory studioAssetInventory
	if err := json.Unmarshal(response.Body.Bytes(), &inventory); err != nil || len(inventory.Items) != 1 || len(inventory.Items[0].Usages) != 1 {
		t.Fatalf("inventory=%#v %v", inventory, err)
	}
	stale := httptest.NewRequest(http.MethodGet, "/api/resources?revision=stale&source_revision="+studioSourceRevision(snapshot.source), nil)
	staleResponse := httptest.NewRecorder()
	studio.routes().ServeHTTP(staleResponse, stale)
	if staleResponse.Code != http.StatusConflict {
		t.Fatalf("stale status=%d", staleResponse.Code)
	}
}
