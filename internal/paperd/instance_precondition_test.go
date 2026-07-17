// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"reflect"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

func TestSemanticMutationRequiresExactSourceInstanceWithoutAdvancingCandidate(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, created, _ := mutationGuard(t, workspace, workspaceFixture, "@intro", "instance-required", CapabilityEdit)

	missing := guard
	missing.ExpectedInstance = ""
	if _, err := workspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: missing, Text: "must not apply"}); !errors.Is(err, paperedit.ErrInvalidOperation) || errorCode(err) != "INSTANCE_PRECONDITION_REQUIRED" {
		t.Fatalf("missing instance error = %v", err)
	}
	assertCandidateHead(t, workspace, created.Candidate.Handle, created.Revision.Handle)

	wrong := guard
	wrong.ExpectedInstance = "/@report/@sheet/@body/@other"
	_, firstErr := workspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: wrong, Text: "must not apply"})
	_, secondErr := workspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: wrong, Text: "must not apply"})
	if !errors.Is(firstErr, ErrRevisionConflict) || errorCode(firstErr) != "INSTANCE_CONFLICT" || errorCode(secondErr) != "INSTANCE_CONFLICT" {
		t.Fatalf("wrong instance errors = %v / %v", firstErr, secondErr)
	}
	first, ok := firstErr.(*Error)
	second, secondOK := secondErr.(*Error)
	if !ok || !secondOK || len(first.Candidates) != 1 || len(first.Candidates) > paperedit.MaxDiagnosticCandidates || !reflect.DeepEqual(first.Candidates, second.Candidates) {
		t.Fatalf("instance candidates are unstable or unbounded: %+v / %+v", firstErr, secondErr)
	}
	if first.Candidates[0].Target != "@intro" || first.Candidates[0].Instance != guard.ExpectedInstance {
		t.Fatalf("instance candidate = %+v", first.Candidates[0])
	}
	assertCandidateHead(t, workspace, created.Candidate.Handle, created.Revision.Handle)
}

func TestWorkspaceApplyRejectsUnguardedTargetAtomically(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	base, err := workspace.CreateRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := workspace.NewCandidate(base.Handle)
	if err != nil {
		t.Fatal(err)
	}
	request := ApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedRevision: base.Revision,
		IdempotencyKey: "no-fuzzy-raw", Operations: []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "must not apply"}},
	}
	first, firstErr := workspace.Apply(request)
	second, secondErr := workspace.Apply(request)
	if !errors.Is(firstErr, paperedit.ErrInvalidOperation) || errorCode(firstErr) != "EDIT_REJECTED" ||
		!errors.Is(secondErr, paperedit.ErrInvalidOperation) || !reflect.DeepEqual(first.Edit, second.Edit) {
		t.Fatalf("unguarded replay = %+v/%v then %+v/%v", first, firstErr, second, secondErr)
	}
	if len(first.Edit.Diagnostics) != 1 || first.Edit.Diagnostics[0].Code != "PAPER_EDIT_PRECONDITION_REQUIRED" ||
		len(first.Edit.Diagnostics[0].Candidates) > paperedit.MaxDiagnosticCandidates {
		t.Fatalf("unguarded diagnostics = %+v", first.Edit.Diagnostics)
	}
	assertCandidateHead(t, workspace, candidate.Handle, base.Handle)
}

func assertCandidateHead(t *testing.T, workspace *Workspace, candidateHandle CandidateHandle, wantHead RevisionHandle) {
	t.Helper()
	candidate, err := workspace.Candidate(candidateHandle)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Head != wantHead {
		t.Fatalf("candidate head = %v, want %v", candidate.Head, wantHead)
	}
}
