// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

const discoveryFixture = "document @report:\n" +
	"  title: \"Components\"\n" +
	"  component @card:\n" +
	"    slot @content:\n" +
	"      type: \"blocks\"\n" +
	"      required: false\n" +
	"      paragraph @default-copy:\n" +
	"        text: \"Default\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @card-instance:\n" +
	"        component: \"@card\"\n" +
	"        fill @content:\n" +
	"          paragraph @message:\n" +
	"            text @copy: \"Hello discovery\"\n"

func openDiscoveryFixture(t *testing.T, workspace *Workspace) (PaperCreateResult, PaperOpenSnapshot, PaperReadScope) {
	t.Helper()
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "discovery.paper", Source: discoveryFixture})
	if err != nil {
		t.Fatalf("PaperCreate() error = %v", err)
	}
	opened, err := workspace.PaperOpen(PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: CapabilityEdit,
	})
	if err != nil {
		t.Fatalf("PaperOpen() error = %v", err)
	}
	return created, opened, PaperReadScope{
		Open: opened.Handle, ExpectedRevision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		MaxResults: 16, MaxBytes: 16 << 10, MaxWork: 256,
	}
}

func TestPaperComponentsReturnsDeterministicDetachedContracts(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 32 << 10, MaxSearchResults: 32, MaxNodes: 512})
	_, _, scope := openDiscoveryFixture(t, workspace)
	request := PaperComponentsRequest{Scope: scope, Query: "card"}
	first, err := workspace.PaperComponents(request)
	if err != nil {
		t.Fatalf("PaperComponents() error = %v", err)
	}
	second, err := workspace.PaperComponents(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("PaperComponents() not deterministic:\n%#v\n%#v\n%v", first, second, err)
	}
	if first.Total != 1 || !first.TotalExact || len(first.Components) != 1 || first.Components[0].ID != "@card" {
		t.Fatalf("PaperComponents() = %#v", first)
	}
	component := first.Components[0]
	if len(component.Slots) != 1 || component.Slots[0].ID != "@content" || component.Slots[0].Type != "blocks" {
		t.Fatalf("component slots = %#v", component.Slots)
	}
	assertExactJSONSize(t, first.EncodedBytes, scope.MaxBytes, first)
	first.Components[0].Slots[0].Properties[0].Raw = "mutated"
	again, _ := workspace.PaperComponents(request)
	if again.Components[0].Slots[0].Properties[0].Raw == "mutated" {
		t.Fatal("component result aliases retained state")
	}
}

func TestPaperInspectReturnsBoundedSourceASTAndMappings(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 32 << 10, MaxSearchResults: 32, MaxNodes: 512})
	_, _, scope := openDiscoveryFixture(t, workspace)
	request := PaperInspectRequest{Scope: scope, Target: "@message", IncludeSource: true}
	first, err := workspace.PaperInspect(request)
	if err != nil {
		t.Fatalf("PaperInspect() error = %v", err)
	}
	second, err := workspace.PaperInspect(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("PaperInspect() not deterministic:\n%#v\n%#v\n%v", first, second, err)
	}
	if first.Node.Node.ID != "@message" || first.Node.Node.Kind != "paragraph" || first.Source == "" {
		t.Fatalf("PaperInspect() = %#v", first)
	}
	if len(first.Mappings) == 0 || first.Mappings[0].ID != "@message" {
		t.Fatalf("inspect mappings = %#v", first.Mappings)
	}
	assertExactJSONSize(t, first.EncodedBytes, scope.MaxBytes, first)
	first.Node.Children[0].ID = "mutated"
	again, _ := workspace.PaperInspect(request)
	if again.Node.Children[0].ID == "mutated" {
		t.Fatal("inspect result aliases retained AST")
	}
}

func TestPaperSearchReturnsASTAndCompilerDomainsDeterministically(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 32 << 10, MaxSearchResults: 32, MaxNodes: 512})
	_, _, scope := openDiscoveryFixture(t, workspace)
	request := PaperSearchRequest{Scope: scope, Query: "@message"}
	first, err := workspace.PaperSearch(request)
	if err != nil {
		t.Fatalf("PaperSearch() error = %v", err)
	}
	second, err := workspace.PaperSearch(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("PaperSearch() not deterministic:\n%#v\n%#v\n%v", first, second, err)
	}
	domains := map[string]bool{}
	for _, match := range first.Matches {
		domains[match.Domain] = true
	}
	if !first.TotalExact || first.Total < 2 || !domains["ast"] || !domains["mapping"] {
		t.Fatalf("PaperSearch() = %#v", first)
	}
	assertExactJSONSize(t, first.EncodedBytes, scope.MaxBytes, first)
	for i := range first.Matches {
		if first.Matches[i].Mapping != nil {
			first.Matches[i].Mapping.ID = "mutated"
			break
		}
	}
	again, _ := workspace.PaperSearch(request)
	for _, match := range again.Matches {
		if match.Mapping != nil && match.Mapping.ID == "mutated" {
			t.Fatal("search result aliases retained mapping")
		}
	}
}

func TestPaperDiscoveryRejectsStaleAndRevokedOpens(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 32 << 10, MaxSearchResults: 32, MaxNodes: 512})
	created, opened, scope := openDiscoveryFixture(t, workspace)
	_, err := workspace.Apply(ApplyRequest{
		Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
		ExpectedRevision: created.Revision.Revision, IdempotencyKey: "advance-discovery",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "discovery.paper", discoveryFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "advanced"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.PaperComponents(PaperComponentsRequest{Scope: scope}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperComponents(stale) error = %v", err)
	}
	if _, err := workspace.PaperInspect(PaperInspectRequest{Scope: scope, Target: "@message"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperInspect(stale) error = %v", err)
	}
	if _, err := workspace.PaperSearch(PaperSearchRequest{Scope: scope, Query: "message"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperSearch(stale) error = %v", err)
	}

	standalone, err := workspace.PaperOpen(PaperOpenRequest{
		Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	standaloneScope := scope
	standaloneScope.Open = standalone.Handle
	if err := workspace.ClosePaperOpen(standalone.Handle); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.PaperSearch(PaperSearchRequest{Scope: standaloneScope, Query: "message"}); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("PaperSearch(revoked) error = %v", err)
	}
	_ = opened
}

func TestPaperDiscoveryEnforcesIndependentLimits(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 32 << 10, MaxSearchResults: 8, MaxNodes: 128})
	_, _, scope := openDiscoveryFixture(t, workspace)
	scope.MaxResults = 8

	tinyBytes := scope
	tinyBytes.MaxBytes = 1
	if _, err := workspace.PaperComponents(PaperComponentsRequest{Scope: tinyBytes}); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperComponents(tiny bytes) error = %v", err)
	}
	badResults := scope
	badResults.MaxResults = 9
	if _, err := workspace.PaperInspect(PaperInspectRequest{Scope: badResults, Target: "@message"}); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperInspect(result limit) error = %v", err)
	}
	tinyWork := scope
	tinyWork.MaxWork = 1
	search, err := workspace.PaperSearch(PaperSearchRequest{Scope: tinyWork, Query: "message"})
	if err != nil {
		t.Fatalf("PaperSearch(tiny work) error = %v", err)
	}
	if search.TotalExact || !search.Truncated || search.WorkUsed != 1 {
		t.Fatalf("PaperSearch(tiny work) = %#v", search)
	}
	if _, err := workspace.PaperInspect(PaperInspectRequest{Scope: tinyWork, Target: "@message"}); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperInspect(tiny work) error = %v", err)
	}
}

func assertExactJSONSize(t *testing.T, reported, limit int, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != reported || len(encoded) > limit {
		t.Fatalf("JSON bytes = %d, reported = %d, limit = %d", len(encoded), reported, limit)
	}
}
