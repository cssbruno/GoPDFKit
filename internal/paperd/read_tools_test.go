// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

func TestPaperCreatePublishesRevisionAndCandidateAtomically(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxRevisions: 2, MaxCandidates: 1})
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatalf("PaperCreate() error = %v", err)
	}
	if created.Candidate.Head != created.Revision.Handle || created.Revision.Revision != paperedit.SourceRevision(workspaceFixture) {
		t.Fatalf("PaperCreate() = %#v", created)
	}

	before := len(workspace.revisions)
	if _, err := workspace.PaperCreate(PaperCreateRequest{File: "second.paper", Source: workspaceFixture}); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperCreate(at capacity) error = %v", err)
	}
	if got := len(workspace.revisions); got != before {
		t.Fatalf("failed atomic create retained %d revisions, want %d", got, before)
	}
	opened, err := workspace.OpenRevision(created.Revision.Handle)
	if err != nil || opened.Source != workspaceFixture {
		t.Fatalf("OpenRevision(created) = %#v, %v", opened, err)
	}
}

func TestPaperOpenPinsExactCandidateHeadAndDigest(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxOpenDocuments: 1})
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	request := PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: CapabilityEdit,
	}
	opened, err := workspace.PaperOpen(request)
	if err != nil || opened.Digest != created.Revision.Revision || opened.Mode != CapabilityEdit {
		t.Fatalf("PaperOpen() = %#v, %v", opened, err)
	}
	encoded, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Handle", "Candidate", "Revision", "scope", "serial"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("opaque handle leaked in %s", encoded)
		}
	}

	bad := request
	bad.ExpectedDigest = paperedit.SourceRevision("different")
	if _, err := workspace.PaperOpen(bad); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperOpen(wrong digest) error = %v", err)
	}
	if _, err := workspace.PaperOpen(request); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperOpen(capacity) error = %v", err)
	}
	if err := workspace.ClosePaperOpen(opened.Handle); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.PaperOpen(request); err != nil {
		t.Fatalf("PaperOpen(after close) error = %v", err)
	}
}

func TestPaperContextIsDeterministicDetachedAndStrictlyBounded(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxContextBytes: 8192, MaxSearchResults: 8})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	opened, _ := workspace.PaperOpen(PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead,
	})
	request := PaperContextRequest{
		Open: opened.Handle, ExpectedRevision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		MaxBytes: 4096, MaxItems: 4, IncludeSource: true,
	}
	first, err := workspace.PaperContext(request)
	if err != nil {
		t.Fatalf("PaperContext() error = %v", err)
	}
	second, err := workspace.PaperContext(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("PaperContext() is not deterministic:\n%#v\n%#v\n%v", first, second, err)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != first.EncodedBytes || len(encoded) > request.MaxBytes {
		t.Fatalf("encoded context bytes = %d, field = %d, limit = %d", len(encoded), first.EncodedBytes, request.MaxBytes)
	}
	if first.Revision.Digest != created.Revision.Revision || first.Root.ID != "@report" || first.Title != "Agent report" {
		t.Fatalf("PaperContext() identity = %#v", first)
	}
	if len(first.Mappings)+len(first.Diagnostics) > request.MaxItems || !first.ItemsTruncated {
		t.Fatalf("PaperContext() item bounds = diagnostics %d mappings %d truncated %v", len(first.Diagnostics), len(first.Mappings), first.ItemsTruncated)
	}
	if len(first.Mappings) != 0 {
		first.Mappings[0].ID = "mutated"
	}
	again, _ := workspace.PaperContext(request)
	if len(again.Mappings) != 0 && again.Mappings[0].ID == "mutated" {
		t.Fatal("detached context mutated retained mappings")
	}

	tiny := request
	tiny.MaxBytes = 1
	if _, err := workspace.PaperContext(tiny); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperContext(tiny) error = %v", err)
	}
	wrongDigest := request
	wrongDigest.ExpectedDigest = paperedit.SourceRevision("stale")
	if _, err := workspace.PaperContext(wrongDigest); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperContext(wrong digest) error = %v", err)
	}
}

func TestPaperContextRejectsCandidateDriftAndCrossWorkspaceHandles(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	opened, _ := workspace.PaperOpen(PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: CapabilityEdit,
	})
	result, err := workspace.Apply(ApplyRequest{
		Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
		ExpectedRevision: created.Revision.Revision, IdempotencyKey: "advance",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "report.paper", workspaceFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "advanced"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := PaperContextRequest{
		Open: opened.Handle, ExpectedRevision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		MaxBytes: 4096, MaxItems: 4,
	}
	if _, err := workspace.PaperContext(request); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperContext(stale open) error = %v", err)
	}
	if result.Revision.Handle == created.Revision.Handle {
		t.Fatal("candidate did not advance")
	}

	other := mustWorkspace(t, Limits{})
	if _, err := other.PaperContext(request); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("PaperContext(cross workspace) error = %v", err)
	}
}
