// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

func acceptancePolicyFixture() CandidateAcceptancePolicy {
	return CandidateAcceptancePolicy{
		RequiredScenarios:       []string{"extreme", "typical"},
		RequiredValidators:      []CandidateValidatorRequirement{{Profile: "layout", Version: "1.8.0"}, {Profile: "pdfua", Version: "2.4.1"}},
		RequiredReviewArtifacts: []string{"contact-sheet", "semantic-diff"},
	}
}

type acceptanceFixture struct {
	workspace *Workspace
	created   PaperCreateResult
	opened    PaperOpenSnapshot
	scenarios ScenarioRevisionSnapshot
	request   CandidateAcceptanceRequest
}

func newAcceptanceFixture(t *testing.T, mutateOptions func(*WorkspaceOptions)) acceptanceFixture {
	t.Helper()
	options := WorkspaceOptions{PolicyRevision: "policy-v7", HandleTTL: time.Hour, CandidateAcceptance: acceptancePolicyFixture()}
	if mutateOptions != nil {
		mutateOptions(&options)
	}
	workspace, created, opened := sensitiveFixture(t, options)
	scenarios, err := workspace.CreateScenarioRevision([]paperscenario.Scenario{{Name: "typical"}, {Name: "extreme"}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	request := CandidateAcceptanceRequest{
		Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle, ExpectedRevision: created.Revision.Revision,
		IdempotencyKey: "acceptance-request-0001",
		Scenarios: []ScenarioAcceptanceEvidence{
			{Revision: scenarios.Handle, Name: "typical", Digest: scenarios.Digest, ResultHash: evidenceHashForTest("scenario-typical"), Passed: true},
			{Revision: scenarios.Handle, Name: "extreme", Digest: scenarios.Digest, ResultHash: evidenceHashForTest("scenario-extreme"), Passed: true},
		},
		Validators: []ValidatorAcceptanceEvidence{
			{Profile: "pdfua", Version: "2.4.1", Hash: evidenceHashForTest("pdfua-result"), Passed: true},
			{Profile: "layout", Version: "1.8.0", Hash: evidenceHashForTest("layout-result"), Passed: true},
		},
		ReviewArtifacts: []ReviewAcceptanceEvidence{
			{Kind: "contact-sheet", Hash: evidenceHashForTest("contact-sheet"), Approved: true},
			{Kind: "semantic-diff", Hash: evidenceHashForTest("review-diff"), Approved: true},
		},
	}
	return acceptanceFixture{workspace: workspace, created: created, opened: opened, scenarios: scenarios, request: request}
}

func authorizeAcceptance(t *testing.T, fixture acceptanceFixture, request *CandidateAcceptanceRequest, nonce string) {
	t.Helper()
	inputHash, err := fixture.workspace.CandidateAcceptanceInputHash(*request)
	if err != nil {
		t.Fatal(err)
	}
	evidence := completeSensitiveEvidence(string(request.ExpectedRevision))
	evidence.OperationInputHash = inputHash
	evidence.RequiredScenarios = []string{"extreme", "typical"}
	evidence.Validators = []ValidatorEvidence{
		{Profile: "layout", Version: "1.8.0", Hash: evidenceHashForTest("layout-result")},
		{Profile: "pdfua", Version: "2.4.1", Hash: evidenceHashForTest("pdfua-result")},
	}
	evidence.ReviewArtifacts = []ReviewArtifactEvidence{
		{Kind: "contact-sheet", Hash: evidenceHashForTest("contact-sheet")},
		{Kind: "semantic-diff", Hash: evidenceHashForTest("review-diff")},
	}
	authority := grantSensitiveForTest(t, fixture.workspace, fixture.opened, SensitiveAccept)
	approval := grantApprovalForTest(t, fixture.workspace, fixture.created, authority, evidence, nonce, time.Minute)
	request.Authorization = SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveAccept,
		ExpectedHead: request.ExpectedHead, PolicyRevision: "policy-v7", Evidence: evidence}
}

func TestAcceptCandidateAtomicallyBindsConfiguredEvidenceAndIdempotentReplay(t *testing.T) {
	fixture := newAcceptanceFixture(t, nil)
	authorizeAcceptance(t, fixture, &fixture.request, "candidate-accept-approval-0001")
	receipt, err := fixture.workspace.AcceptCandidate(context.Background(), fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CandidateRevision != string(fixture.created.Revision.Revision) || receipt.PolicyHash == "" || receipt.EvidenceHash == "" || receipt.AcceptanceHash == "" || receipt.Audit.Operation != SensitiveAccept || receipt.Audit.AuditHash == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	receipt.ScenarioResults[0].Name = "mutated"
	reordered := fixture.request
	reordered.Authorization.Evidence.RequiredScenarios = []string{"typical", "extreme"}
	reordered.Authorization.Evidence.Validators = []ValidatorEvidence{reordered.Authorization.Evidence.Validators[1], reordered.Authorization.Evidence.Validators[0]}
	reordered.Authorization.Evidence.ReviewArtifacts = []ReviewArtifactEvidence{reordered.Authorization.Evidence.ReviewArtifacts[1], reordered.Authorization.Evidence.ReviewArtifacts[0]}
	replayed, err := fixture.workspace.AcceptCandidate(context.Background(), reordered)
	if err != nil || replayed.ScenarioResults[0].Name == "mutated" || replayed.AcceptanceHash == "" {
		t.Fatalf("replay = %#v, %v", replayed, err)
	}
	current, err := fixture.workspace.CandidateAcceptance(fixture.created.Candidate.Handle)
	if err != nil || current.AcceptanceHash != replayed.AcceptanceHash {
		t.Fatalf("CandidateAcceptance() = %#v, %v", current, err)
	}
	conflict := fixture.request
	conflict.Validators = append([]ValidatorAcceptanceEvidence(nil), conflict.Validators...)
	conflict.Validators[0].Hash = evidenceHashForTest("other-validator-result")
	if _, err := fixture.workspace.AcceptCandidate(context.Background(), conflict); errorCode(err) != "ACCEPTANCE_IDEMPOTENCY_CONFLICT" {
		t.Fatalf("idempotency conflict = %v", err)
	}
	audit, err := fixture.workspace.SensitiveOperationAudit(8)
	if err != nil || len(audit) != 1 || !audit[0].Allowed || audit[0].Operation != SensitiveAccept {
		t.Fatalf("audit = %#v, %v", audit, err)
	}
}

func TestAcceptCandidateRejectsMissingFailedStaleAndWrongCapabilityEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*acceptanceFixture)
		want   string
	}{
		{"missing scenario", func(f *acceptanceFixture) { f.request.Scenarios = f.request.Scenarios[:1] }, "ACCEPTANCE_EVIDENCE_INCOMPLETE"},
		{"failed scenario", func(f *acceptanceFixture) { f.request.Scenarios[0].Passed = false }, "SCENARIO_GATE_FAILED"},
		{"failed validator", func(f *acceptanceFixture) { f.request.Validators[0].Passed = false }, "VALIDATOR_GATE_FAILED"},
		{"unapproved review", func(f *acceptanceFixture) { f.request.ReviewArtifacts[0].Approved = false }, "REVIEW_GATE_FAILED"},
		{"stale scenario digest", func(f *acceptanceFixture) { f.request.Scenarios[0].Digest = evidenceHashForTest("stale") }, "SCENARIO_EVIDENCE_STALE"},
		{"wrong operation", func(f *acceptanceFixture) { f.request.Authorization.Operation = SensitivePublish }, "ACCEPTANCE_AUTHORIZATION_BINDING"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAcceptanceFixture(t, nil)
			authorizeAcceptance(t, fixture, &fixture.request, "adversarial-accept-"+test.name)
			test.mutate(&fixture)
			_, err := fixture.workspace.AcceptCandidate(context.Background(), fixture.request)
			if errorCode(err) != test.want || !errors.Is(err, ErrCandidateAcceptanceDenied) {
				t.Fatalf("AcceptCandidate() = %v, want %s", err, test.want)
			}
			candidate, candidateErr := fixture.workspace.Candidate(fixture.created.Candidate.Handle)
			if candidateErr != nil || candidate.Head != fixture.created.Revision.Handle {
				t.Fatalf("candidate changed after denial = %#v, %v", candidate, candidateErr)
			}
		})
	}
}

func TestAcceptCandidateCancellationAndConcurrentAcceptanceCAS(t *testing.T) {
	fixture := newAcceptanceFixture(t, nil)
	authorizeAcceptance(t, fixture, &fixture.request, "cancelled-acceptance-0001")
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.workspace.AcceptCandidate(cancelled, fixture.request); errorCode(err) != "ACCEPTANCE_CANCELLED" {
		t.Fatalf("cancelled acceptance = %v", err)
	}

	first := fixture.request
	first.IdempotencyKey = "concurrent-acceptance-first"
	authorizeAcceptance(t, fixture, &first, "concurrent-accept-first")
	second := fixture.request
	second.IdempotencyKey = "concurrent-acceptance-second"
	authorizeAcceptance(t, fixture, &second, "concurrent-accept-second")
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, request := range []CandidateAcceptanceRequest{first, second} {
		wait.Add(1)
		go func(request CandidateAcceptanceRequest) {
			defer wait.Done()
			<-start
			_, err := fixture.workspace.AcceptCandidate(context.Background(), request)
			errs <- err
		}(request)
	}
	close(start)
	wait.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errorCode(err) == "CANDIDATE_ALREADY_ACCEPTED" {
			conflict++
		} else {
			t.Fatalf("concurrent error = %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestAcceptedCandidateIsInvalidatedByExactHeadEdit(t *testing.T) {
	fixture := newAcceptanceFixture(t, nil)
	authorizeAcceptance(t, fixture, &fixture.request, "accepted-then-edit-0001")
	if _, err := fixture.workspace.AcceptCandidate(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.workspace.Apply(ApplyRequest{Candidate: fixture.created.Candidate.Handle, ExpectedHead: fixture.created.Revision.Handle,
		ExpectedRevision: fixture.created.Revision.Revision, IdempotencyKey: "edit-after-acceptance",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "sensitive.paper", workspaceFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "Changed after review"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.workspace.CandidateAcceptance(fixture.created.Candidate.Handle); !errors.Is(err, ErrCandidateAcceptanceNotFound) {
		t.Fatalf("acceptance followed edited head = %v", err)
	}
	stale := fixture.request
	stale.IdempotencyKey = "stale-head-after-edit"
	stale.ExpectedHead = result.Revision.Handle
	stale.ExpectedRevision = result.Revision.Revision
	// Old scenario/review evidence cannot be paired with an approval for a
	// different source revision; leaving the old approval proves exact binding.
	if _, err := fixture.workspace.AcceptCandidate(context.Background(), stale); errorCode(err) != "ACCEPTANCE_AUTHORIZATION_BINDING" {
		t.Fatalf("stale evidence after edit = %v", err)
	}
}

func TestAcceptCandidateRejectsAuthorityForDifferentCandidateAtSameRevision(t *testing.T) {
	fixture := newAcceptanceFixture(t, nil)
	other, err := fixture.workspace.NewCandidate(fixture.created.Revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	otherOpen, err := fixture.workspace.PaperOpen(PaperOpenRequest{Candidate: other.Handle, Revision: fixture.created.Revision.Handle, ExpectedDigest: fixture.created.Revision.Revision, Mode: CapabilityEdit})
	if err != nil {
		t.Fatal(err)
	}
	fixture.opened = otherOpen
	authorizeAcceptance(t, fixture, &fixture.request, "wrong-candidate-authority")
	if _, err := fixture.workspace.AcceptCandidate(context.Background(), fixture.request); errorCode(err) != "ACCEPTANCE_AUTHORIZATION_BINDING" {
		t.Fatalf("cross-candidate acceptance = %v", err)
	}
}

func TestAcceptCandidateRacesHeadEditWithoutStaleCommittedAcceptance(t *testing.T) {
	fixture := newAcceptanceFixture(t, nil)
	authorizeAcceptance(t, fixture, &fixture.request, "accept-edit-race-approval")
	start := make(chan struct{})
	precondition := exactTargetPrecondition(t, "sensitive.paper", workspaceFixture, "@copy")
	var wait sync.WaitGroup
	var acceptErr, editErr error
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, acceptErr = fixture.workspace.AcceptCandidate(context.Background(), fixture.request)
	}()
	go func() {
		defer wait.Done()
		<-start
		_, editErr = fixture.workspace.Apply(ApplyRequest{Candidate: fixture.created.Candidate.Handle, ExpectedHead: fixture.created.Revision.Handle,
			ExpectedRevision: fixture.created.Revision.Revision, IdempotencyKey: "accept-edit-race-edit",
			TargetPreconditions: []paperedit.TargetPrecondition{precondition},
			Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "Raced"}}})
	}()
	close(start)
	wait.Wait()
	if editErr != nil {
		t.Fatalf("edit race = %v", editErr)
	}
	if acceptErr != nil && errorCode(acceptErr) != "ACCEPTANCE_HEAD_CHANGED" {
		t.Fatalf("accept race = %v", acceptErr)
	}
	if _, err := fixture.workspace.CandidateAcceptance(fixture.created.Candidate.Handle); !errors.Is(err, ErrCandidateAcceptanceNotFound) {
		t.Fatalf("stale acceptance survived winning edit: %v", err)
	}
}
