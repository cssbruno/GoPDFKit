// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperassets"
)

func TestStudioResourceCatalogMutationAddsAndRemovesManifestEntry(t *testing.T) {
	dir := t.TempDir()
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	if err := os.WriteFile(filepath.Join(dir, "hero.png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "project.json")
	if err := os.WriteFile(manifest, []byte(`{"assets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	source := "document @report:\n  page @sheet:\n    body @body:\n      paragraph @copy:\n        text: \"Catalog editing\"\n"
	file := filepath.Join(dir, "report.paper")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	project, err := paperassets.LoadProjectManifest(manifest, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := studio.setProjectManifest(manifest, dir, project); err != nil {
		t.Fatal(err)
	}
	workspace := fetchStudioWorkspace(t, studio.routes())
	added := postStudioJSON(t, studio.routes(), "/api/resources", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision, "operation": "add",
		"name": "hero", "path": "hero.png", "media_type": "image/png", "focus_x": 0.5, "focus_y": 0.25,
	})
	if added.StatusCode != http.StatusOK {
		t.Fatalf("add status=%d body=%s", added.StatusCode, added.Body)
	}
	var mutation studioResourceCatalogResponse
	if err := json.Unmarshal([]byte(added.Body), &mutation); err != nil || !mutation.OK || !mutation.Inventory.CatalogEditable || len(mutation.Inventory.Items) != 1 || mutation.Inventory.Items[0].Name != "hero" {
		t.Fatalf("add response=%s err=%v", added.Body, err)
	}
	manifestBytes, err := os.ReadFile(manifest)
	if err != nil || !bytes.Contains(manifestBytes, []byte(`"name": "hero"`)) {
		t.Fatalf("manifest after add=%s err=%v", manifestBytes, err)
	}

	updated := fetchStudioWorkspace(t, studio.routes())
	removed := postStudioJSON(t, studio.routes(), "/api/resources", map[string]any{
		"source_revision": updated.SourceRevision, "plan_revision": updated.Revision, "operation": "remove", "name": "hero",
	})
	if removed.StatusCode != http.StatusOK {
		t.Fatalf("remove status=%d body=%s", removed.StatusCode, removed.Body)
	}
	project, err = paperassets.LoadProjectManifest(manifest, dir)
	if err != nil || len(project) != 0 {
		t.Fatalf("project after remove=%#v err=%v", project, err)
	}
}

func TestStudioResourceCatalogMutationRejectsReferencedRemovalAndStaleRevision(t *testing.T) {
	dir := t.TempDir()
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	digest := sha256.Sum256(image)
	if err := os.WriteFile(filepath.Join(dir, "hero.png"), image, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "project.json")
	body := []byte(`{"assets":[{"name":"hero","media_type":"image/png","sha256":"` + hex.EncodeToString(digest[:]) + `","path":"hero.png"}]}`)
	if err := os.WriteFile(manifest, body, 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "report.paper")
	source := "document @report:\n  page @sheet:\n    body @body:\n      image @hero-node:\n        source: \"asset:hero\"\n        alt: \"Hero\"\n"
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	studio, err := newStudioServer(file, "")
	if err != nil {
		t.Fatal(err)
	}
	project, err := paperassets.LoadProjectManifest(manifest, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := studio.setProjectManifest(manifest, dir, project); err != nil {
		t.Fatal(err)
	}
	workspace := fetchStudioWorkspace(t, studio.routes())
	remove := postStudioJSON(t, studio.routes(), "/api/resources", map[string]any{
		"source_revision": workspace.SourceRevision, "plan_revision": workspace.Revision, "operation": "remove", "name": "hero",
	})
	if remove.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("referenced remove status=%d body=%s", remove.StatusCode, remove.Body)
	}
	unchanged, _ := os.ReadFile(manifest)
	if !bytes.Equal(unchanged, body) {
		t.Fatalf("referenced remove changed manifest: %s", unchanged)
	}
	stale := postStudioJSON(t, studio.routes(), "/api/resources", map[string]any{
		"source_revision": "stale", "plan_revision": workspace.Revision, "operation": "remove", "name": "hero",
	})
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale status=%d body=%s", stale.StatusCode, stale.Body)
	}
}
