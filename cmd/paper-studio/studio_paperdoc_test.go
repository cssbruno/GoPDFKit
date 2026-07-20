// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperassets"
	"github.com/cssbruno/paperrune/internal/paperdoc"
)

func TestStudioPaperDocumentExportOpenAndEditRoundTrip(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "prescription.paper")
	source := "document @report:\n  page @sheet:\n    width: 180pt\n    height: 96pt\n    margin: 6pt\n    body @body:\n      image @header:\n        source: \"asset:prescription-header\"\n        width: 80pt\n        height: 32pt\n        alt: \"Clinic header\"\n      paragraph @medication:\n        text: \"Amoxicillin 500 mg\"\n"
	if err := os.WriteFile(sourceFile, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	imageBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "paper", "assets", "prescription-header.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	imageDigest := sha256.Sum256(imageBytes)
	server, err := newStudioServer(sourceFile, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := server.setProjectResources([]paperassets.ProjectResource{{Name: "prescription-header", MediaType: "image/jpeg", Digest: hex.EncodeToString(imageDigest[:]), Data: imageBytes}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := server.current(context.Background(), "")
	if err != nil || snapshot.pages != 1 {
		t.Fatalf("source snapshot = pages %d, %v", snapshot.pages, err)
	}
	exported := studioRequest(t, server.routes(), http.MethodGet, "/api/export.paperdoc?revision="+snapshot.revision, nil, "")
	if exported.StatusCode != http.StatusOK || exported.Header.Get("Content-Type") != paperdoc.MediaType || !strings.Contains(exported.Header.Get("Content-Disposition"), "prescription.paperdoc") {
		t.Fatalf("export = %d %q", exported.StatusCode, exported.Header)
	}
	document, err := paperdoc.Decode(context.Background(), exported.Body)
	if err != nil || document.Source != source || len(document.Resources) != 1 || !bytes.Equal(document.Resources[0].Data, imageBytes) {
		t.Fatalf("decoded export = resources %d, %v", len(document.Resources), err)
	}

	packageFile := filepath.Join(root, "prescription.paperdoc")
	if err := os.WriteFile(packageFile, exported.Body, 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := newStudioServer(packageFile, "")
	if err != nil {
		t.Fatal(err)
	}
	openedSnapshot, err := opened.current(context.Background(), "")
	if err != nil || openedSnapshot.pages != 1 {
		t.Fatalf("opened snapshot = pages %d, %v", openedSnapshot.pages, err)
	}
	updated := strings.Replace(source, "Amoxicillin 500 mg", "Amoxicillin 875 mg", 1)
	if err := writeStudioSourceCAS(packageFile, openedSnapshot.sourceHash, updated); err != nil {
		t.Fatal(err)
	}
	after, _, err := readStudioPaperDocument(packageFile)
	if err != nil || after.Source != updated || len(after.Resources) != 1 || !bytes.Equal(after.Resources[0].Data, imageBytes) {
		t.Fatalf("edited package = resources %d, %v", len(after.Resources), err)
	}
}

func TestStudioPaperDocumentExportEmbedsImportsAndRejectsStaleRevision(t *testing.T) {
	root := t.TempDir()
	sourceFile := filepath.Join(root, "document.paper")
	if err := os.WriteFile(sourceFile, []byte("document @report:\n  import: \"shared.paper\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shared := "document @shared:\n  style @body:\n    font-size: 10pt\n"
	if err := os.WriteFile(filepath.Join(root, "shared.paper"), []byte(shared), 0o600); err != nil {
		t.Fatal(err)
	}
	server, err := newStudioServer(sourceFile, "")
	if err != nil {
		t.Fatal(err)
	}
	stale := studioRequest(t, server.routes(), http.MethodGet, "/api/export.paperdoc?revision=stale", nil, "")
	if stale.StatusCode != http.StatusConflict {
		t.Fatalf("stale export = %d", stale.StatusCode)
	}
	exported := studioRequest(t, server.routes(), http.MethodGet, "/api/export.paperdoc", nil, "")
	document, err := paperdoc.Decode(context.Background(), exported.Body)
	if exported.StatusCode != http.StatusOK || err != nil || document.Imports["shared.paper"] != shared {
		t.Fatalf("import export = %d, imports %#v, %v", exported.StatusCode, document.Imports, err)
	}
	packageFile := filepath.Join(root, "imports.paperdoc")
	if err := os.WriteFile(packageFile, exported.Body, 0o600); err != nil {
		t.Fatal(err)
	}
	opened, err := newStudioServer(packageFile, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.current(context.Background(), ""); err != nil {
		t.Fatalf("open packaged imports: %v", err)
	}
}
