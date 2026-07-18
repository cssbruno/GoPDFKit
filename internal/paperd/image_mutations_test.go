// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
)

func TestPaperSetImageSourceUsesExplicitWorkspaceCatalog(t *testing.T) {
	data, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==")
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	assets := []papercompile.AssetResource{{Name: "old", MediaType: "image/png", Digest: hash, Data: data}, {Name: "new", MediaType: "image/png", Digest: hash, Data: data}}
	workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true, AssetResources: assets})
	source := "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n        source: \"asset:old\"\n        width: 20pt\n        height: 20pt\n        alt: \"Evidence\"\n"
	guard, _, opened := mutationGuard(t, workspace, source, "@hero", "replace-image", CapabilityEdit)
	guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:resource", []MutationOperation{MutationSetImageProperty}, []string{"@hero"}, nil)
	result, err := workspace.PaperSetImageProperty(PaperSetImagePropertyRequest{Guard: guard, Property: PaperImageSource, Text: "asset:new"})
	if err != nil || !strings.Contains(result.Revision.Source, `source: "asset:new"`) || len(result.Edit.Diff.Patches) != 1 {
		t.Fatalf("replacement=%#v err=%v", result, err)
	}
	missing := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true, AssetResources: assets[:1]})
	badGuard, _, badOpen := mutationGuard(t, missing, source, "@hero", "missing-image", CapabilityEdit)
	badGuard.Authority = grantMutationAuthority(t, missing, badOpen, "agent:resource", []MutationOperation{MutationSetImageProperty}, []string{"@hero"}, nil)
	if _, err := missing.PaperSetImageProperty(PaperSetImagePropertyRequest{Guard: badGuard, Property: PaperImageSource, Text: "asset:new"}); err == nil {
		t.Fatal("missing replacement resource accepted")
	}
}

const imageMutationFixture = "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n        source: \"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==\"\n        width: 40pt\n        height: 20pt\n        fit: \"contain\"\n        focus-x: 0.5\n        focus-y: 0.5\n        alt: \"Evidence\"\n"

const componentImageMutationFixture = "document @report:\n  component @card:\n    image @template-image:\n      source: \"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==\"\n      width: 20pt\n      height: 20pt\n      alt: \"Template evidence\"\n  page @sheet:\n    body @body:\n      use @instance:\n        component: \"@card\"\n"

func TestPaperSetImagePropertyWritesTypedMinimalPatches(t *testing.T) {
	tests := []struct {
		name    string
		request PaperSetImagePropertyRequest
		want    string
		patches int
	}{
		{"fit", PaperSetImagePropertyRequest{Property: PaperImageFit, Fit: "cover"}, `fit: "cover"`, 1},
		{"focus", PaperSetImagePropertyRequest{Property: PaperImageFocusX, Number: 0.25}, "focus-x: 0.25", 1},
		{"dimension", PaperSetImagePropertyRequest{Property: PaperImageWidth, Points: 48}, "width: 48pt", 1},
		{"responsive dimension", PaperSetImagePropertyRequest{Property: PaperImageWidth, Length: "50%"}, "width: 50%", 1},
		{"automatic height", PaperSetImagePropertyRequest{Property: PaperImageHeight, Length: "auto"}, `height: "auto"`, 1},
		{"alt", PaperSetImagePropertyRequest{Property: PaperImageAlt, Text: "Updated evidence"}, `alt: "Updated evidence"`, 1},
		{"decorative", PaperSetImagePropertyRequest{Property: PaperImageDecorative, Bool: true}, "decorative: true", 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := authorizationWorkspace(t, WorkspaceOptions{RequireMutationAuthority: true})
			guard, _, opened := mutationGuard(t, workspace, imageMutationFixture, "@hero", "image-"+test.name, CapabilityEdit)
			guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:image", []MutationOperation{MutationSetImageProperty}, []string{"@hero"}, nil)
			test.request.Guard = guard
			result, err := workspace.PaperSetImageProperty(test.request)
			if err != nil {
				t.Fatalf("PaperSetImageProperty() = %v", err)
			}
			if !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != test.patches || !strings.Contains(result.Revision.Source, test.want) {
				t.Fatalf("result = %#v", result)
			}
			if test.name == "decorative" && !strings.Contains(result.Revision.Source, `alt: ""`) {
				t.Fatalf("decorative transition retained alt: %s", result.Revision.Source)
			}
			if !result.Authorization.Explicit || result.Authorization.Operation != MutationSetImageProperty {
				t.Fatalf("authorization = %#v", result.Authorization)
			}
		})
	}
}

func TestPaperSetImagePropertyRejectsAdversarialValuesAtomically(t *testing.T) {
	tests := []PaperSetImagePropertyRequest{
		{Property: PaperImageProperty("fit\nowned"), Fit: "cover"},
		{Property: PaperImageFit, Fit: "fill"},
		{Property: PaperImageFocusX, Number: math.NaN()},
		{Property: PaperImageFocusY, Number: 2},
		{Property: PaperImageWidth, Points: -1},
		{Property: PaperImageWidth, Points: 1_000_001},
		{Property: PaperImageWidth, Length: "101%"},
		{Property: PaperImageHeight, Length: "50%"},
		{Property: PaperImageAlt, Text: strings.Repeat("x", 9<<20)},
	}
	for index, request := range tests {
		workspace := mustWorkspace(t, Limits{})
		guard, created, _ := mutationGuard(t, workspace, imageMutationFixture, "@hero", "image-invalid-"+string(rune('a'+index)), CapabilityEdit)
		request.Guard = guard
		if _, err := workspace.PaperSetImageProperty(request); err == nil {
			t.Fatalf("request %d unexpectedly succeeded", index)
		}
		candidate, _ := workspace.Candidate(created.Candidate.Handle)
		if candidate.Head != created.Revision.Handle {
			t.Fatalf("request %d advanced candidate", index)
		}
	}
}

func TestPaperSetImagePropertyAuthorizesComponentBlastRadius(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{ProtectedNodeIDs: []string{"@card"}})
	guard, _, opened := mutationGuard(t, workspace, componentImageMutationFixture, "@template-image", "image-component", CapabilityEdit)
	request := PaperSetImagePropertyRequest{Guard: guard, Property: PaperImageFit, Fit: "cover"}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:image", []MutationOperation{MutationSetImageProperty}, []string{"@card", "@instance"}, nil)
	if _, err := workspace.PaperSetImageProperty(request); errorCode(err) != "PROTECTED_NODE_DENIED" {
		t.Fatalf("missing protected grant = %v", err)
	}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:image", []MutationOperation{MutationSetImageProperty}, []string{"@card", "@instance"}, []string{"@card"})
	result, err := workspace.PaperSetImageProperty(request)
	if err != nil || !result.Authorization.Allowed || len(result.Authorization.Effects) < 2 {
		t.Fatalf("authorized component image = %#v, %v", result.Authorization, err)
	}
}

func TestPaperSetImagePropertyIdempotentRace(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, imageMutationFixture, "@hero", "image-race", CapabilityEdit)
	request := PaperSetImagePropertyRequest{Guard: guard, Property: PaperImageFocusY, Number: 0.75}
	const workers = 8
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	revisions := make(chan string, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := workspace.PaperSetImageProperty(request)
			errors <- err
			revisions <- string(result.Revision.Revision)
		}()
	}
	wait.Wait()
	close(errors)
	close(revisions)
	for err := range errors {
		if err != nil {
			t.Fatalf("race mutation = %v", err)
		}
	}
	var revision string
	for got := range revisions {
		if revision == "" {
			revision = got
		} else if got != revision {
			t.Fatalf("revision = %q, want %q", got, revision)
		}
	}
}
