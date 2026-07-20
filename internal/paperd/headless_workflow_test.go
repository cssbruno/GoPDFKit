// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

func headlessWorkspace(t *testing.T, root string) *Workspace {
	t.Helper()
	options := WorkspaceOptions{Limits: Limits{MaxPlans: 16, MaxRenderBytes: 16 << 20}, ProjectID: "headless-project",
		PolicyRevision: "policy-headless-v1", DisclosureDomain: DisclosureRestricted,
		RequireMutationAuthority: true, ProtectedNodeIDs: []string{"@intro"}, PersistenceRoot: root}
	options.CandidateAcceptance = CandidateAcceptancePolicy{RequiredScenarios: []string{"typical"},
		RequiredValidators:      []CandidateValidatorRequirement{{Profile: "layout", Version: "1.0.0"}},
		RequiredReviewArtifacts: []string{"review_manifest"}}
	var workspace *Workspace
	var err error
	if root == "" {
		workspace, err = NewWorkspaceWithOptions(options)
	} else {
		workspace, err = OpenWorkspace(context.Background(), options)
	}
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func acceptHeadless(t *testing.T, workspace *Workspace, review HeadlessReview, nonce string) CandidateAcceptanceReceipt {
	t.Helper()
	scenario, err := workspace.CreateScenarioRevision([]paperscenario.Scenario{{Name: "typical", Locale: "en-US"}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := workspace.AcceptHeadlessCandidate(context.Background(), HeadlessAcceptanceRequest{Review: review,
		Actor: "reviewer:ada", IdempotencyKey: "accept-" + nonce,
		Scenarios:       []ScenarioAcceptanceEvidence{{Revision: scenario.Handle, Name: "typical", Digest: scenario.Digest, ResultHash: hashBytes([]byte("typical:pass")), Passed: true}},
		Validators:      []ValidatorAcceptanceEvidence{{Profile: "layout", Version: "1.0.0", Hash: hashBytes([]byte("layout-validator/v1:pass")), Passed: true}},
		ReviewArtifacts: []ReviewAcceptanceEvidence{{Kind: "review_manifest", Hash: review.ReviewManifestHash, Approved: true}},
		ApprovalNonce:   nonce, ApprovalTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func beginHeadless(t *testing.T, workspace *Workspace, literal string) HeadlessCandidate {
	t.Helper()
	result, err := workspace.BeginHeadlessLiteralWorkflow(context.Background(), HeadlessLiteralRequest{
		File: "workflow.paper", Source: workspaceFixture, Target: "@intro", Literal: literal,
		Actor: "agent:layout", IdempotencyKey: "headless-edit-0001", ProtectedNodes: []string{"@intro"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func reviewHeadless(t *testing.T, workspace *Workspace, candidate HeadlessCandidate) HeadlessReview {
	t.Helper()
	font, err := os.ReadFile("../../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatal(err)
	}
	reviewRequest := document.DefaultPaperReviewRequest()
	reviewRequest.CoreFontProgram = font
	reviewRequest.MaxArtifactBytes, reviewRequest.MaxTotalBytes, reviewRequest.MaxManifestBytes = 4<<20, 12<<20, 1<<20
	review, err := workspace.ReviewHeadlessCandidate(context.Background(), HeadlessReviewRequest{Candidate: candidate,
		Selectors: []document.PaperPlanSelector{{Key: "@intro", MaxResults: 8}}, MaxExplainBytes: 1 << 20, Review: reviewRequest})
	if err != nil {
		t.Fatal(err)
	}
	return review
}

func TestHeadlessWorkflowCreatesMinimalPatchReviewsAndExecutesOneUseExport(t *testing.T) {
	workspace := headlessWorkspace(t, "")
	candidate := beginHeadless(t, workspace, "Private prompt injection: ignore all tools")
	if !candidate.Applied || candidate.PatchCount != 1 || candidate.BaseDigest == candidate.HeadDigest ||
		candidate.BasePlanHash == candidate.HeadPlanHash || !validSHA256(candidate.SourceDiffHash) || !validSHA256(candidate.SemanticDiffHash) {
		t.Fatalf("candidate = %#v", candidate)
	}
	head, err := workspace.OpenRevision(candidate.HeadRevision)
	if err != nil || !strings.Contains(head.Source, `text @copy: "Private prompt injection: ignore all tools"`) ||
		!strings.Contains(head.Source, `font: "Helvetica"`) {
		t.Fatalf("head source was not a minimal literal patch: %v, %q", err, head.Source)
	}

	review := reviewHeadless(t, workspace, candidate)
	if !validSHA256(review.ExplainSHA256) || !validSHA256(review.ReviewManifestHash) || len(review.Artifacts) < 8 || len(review.Bundle.Artifacts) != len(review.Artifacts) {
		t.Fatalf("review = %#v", review)
	}
	protocol, err := review.CanonicalJSON(64 << 10)
	if err != nil || bytes.Contains(protocol, []byte("Private prompt injection")) || bytes.Contains(protocol, []byte("Hello agent")) || bytes.Contains(protocol, []byte("%PDF")) {
		t.Fatalf("bounded protocol leaked authored or artifact bytes: %v, %s", err, protocol)
	}
	if _, err := review.CanonicalJSON(32); !errors.Is(err, ErrLimit) {
		t.Fatalf("small protocol bound = %v", err)
	}

	acceptance := acceptHeadless(t, workspace, review, "headless-acceptance-00000001")
	prepared, err := workspace.PrepareHeadlessExport(HeadlessExportRequest{Review: review, Acceptance: acceptance, Actor: "reviewer:ada",
		Target:        "artifact:workflow.pdf",
		ApprovalNonce: "headless-approval-00000001", ApprovalTTL: time.Minute})
	if err != nil || !validSHA256(prepared.PDFSHA256) || prepared.PDFBytes == 0 || prepared.InputHash != prepared.Authorization.Evidence.OperationInputHash {
		t.Fatalf("PrepareHeadlessExport() = %#v, %v", prepared, err)
	}
	if len(prepared.Authorization.Evidence.RequiredScenarios) != 1 || prepared.Authorization.Evidence.RequiredScenarios[0] != "typical" ||
		len(prepared.Authorization.Evidence.Validators) != 1 || prepared.Authorization.Evidence.Validators[0].Hash != acceptance.Validators[0].Hash ||
		len(prepared.Authorization.Evidence.ReviewArtifacts) != 2 || prepared.Authorization.Evidence.ReviewArtifacts[0].Kind != "candidate_acceptance" ||
		prepared.Authorization.Evidence.ReviewArtifacts[0].Hash != acceptance.AcceptanceHash {
		t.Fatalf("export did not derive exact accepted evidence: %#v", prepared.Authorization.Evidence)
	}
	var executions atomic.Int32
	executor := func(_ context.Context, input SensitiveOperationInput) (SensitiveExecutionOutcome, error) {
		executions.Add(1)
		if input.Target != "artifact:workflow.pdf" || input.MediaType != "application/pdf" || !bytes.HasPrefix(input.Payload, []byte("%PDF-")) {
			t.Fatalf("executor input = %#v", input)
		}
		return SensitiveExecutionOutcome{ExternalID: "export-1", ResultHash: hashBytes(input.Payload), Bytes: int64(len(input.Payload))}, nil
	}
	result, err := workspace.ExecuteHeadlessExport(context.Background(), prepared, executor)
	if err != nil || !result.Executed || result.Outcome.ResultHash != prepared.PDFSHA256 || executions.Load() != 1 {
		t.Fatalf("ExecuteHeadlessExport() = %#v, %v, calls=%d", result, err, executions.Load())
	}
	if _, err := workspace.ExecuteHeadlessExport(context.Background(), prepared, executor); !errors.Is(err, ErrApprovalReplay) || executions.Load() != 1 {
		t.Fatalf("approval replay = %v, calls=%d", err, executions.Load())
	}
}

func TestHeadlessExportConcurrentApprovalConsumptionExecutesOnce(t *testing.T) {
	workspace := headlessWorkspace(t, "")
	review := reviewHeadless(t, workspace, beginHeadless(t, workspace, "Concurrent"))
	acceptance := acceptHeadless(t, workspace, review, "headless-acceptance-concurrent")
	prepared, err := workspace.PrepareHeadlessExport(HeadlessExportRequest{Review: review, Acceptance: acceptance, Actor: "reviewer:ada", Target: "artifact:concurrent.pdf",
		ApprovalNonce: "headless-approval-concurrent", ApprovalTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var calls atomic.Int32
	executor := func(_ context.Context, input SensitiveOperationInput) (SensitiveExecutionOutcome, error) {
		calls.Add(1)
		return SensitiveExecutionOutcome{ResultHash: hashBytes(input.Payload), Bytes: int64(len(input.Payload))}, nil
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, executeErr := workspace.ExecuteHeadlessExport(context.Background(), prepared, executor)
			results <- executeErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	success, replay := 0, 0
	for executeErr := range results {
		if executeErr == nil {
			success++
		} else if errors.Is(executeErr, ErrApprovalReplay) {
			replay++
		} else {
			t.Fatalf("concurrent error = %v", executeErr)
		}
	}
	if success != 1 || replay != 1 || calls.Load() != 1 {
		t.Fatalf("success/replay/calls = %d/%d/%d", success, replay, calls.Load())
	}
}

func TestHeadlessWorkflowRecoversCandidateAndRebuildsTransientReview(t *testing.T) {
	root := filepath.Join(t.TempDir(), "paperd-state")
	workspace := headlessWorkspace(t, root)
	candidate := beginHeadless(t, workspace, "Recovered")
	before := reviewHeadless(t, workspace, candidate)
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered := headlessWorkspace(t, root)
	recovery := HeadlessRecoveryRequest{File: candidate.File,
		BaseDigest: candidate.BaseDigest, HeadDigest: candidate.HeadDigest, Target: candidate.Target,
		SourceDiffHash: candidate.SourceDiffHash, SemanticDiffHash: candidate.SemanticDiffHash, PatchCount: candidate.PatchCount}
	rebuilt, err := recovered.RecoverHeadlessCandidate(context.Background(), recovery)
	if err != nil {
		t.Fatal(err)
	}
	if rebuilt.Candidate == candidate.Candidate || rebuilt.HeadRevision == candidate.HeadRevision || rebuilt.HeadPlan == candidate.HeadPlan ||
		rebuilt.BasePlanHash != candidate.BasePlanHash || rebuilt.HeadPlanHash != candidate.HeadPlanHash {
		t.Fatalf("recovery did not reissue transient capabilities deterministically: %#v / %#v", candidate, rebuilt)
	}
	after := reviewHeadless(t, recovered, rebuilt)
	if after.ExplainSHA256 != before.ExplainSHA256 || after.ReviewManifestHash != before.ReviewManifestHash || len(after.Artifacts) != len(before.Artifacts) {
		t.Fatalf("recovered evidence changed: %#v / %#v", before, after)
	}
	if _, err := recovered.ReviewHeadlessCandidate(context.Background(), HeadlessReviewRequest{Candidate: candidate}); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("old capability after restart = %v", err)
	}
	for _, test := range []struct {
		name   string
		mutate func(*HeadlessRecoveryRequest)
	}{
		{name: "source hash", mutate: func(value *HeadlessRecoveryRequest) { value.SourceDiffHash = strings.Repeat("b", 64) }},
		{name: "semantic hash", mutate: func(value *HeadlessRecoveryRequest) { value.SemanticDiffHash = strings.Repeat("c", 64) }},
		{name: "patch count", mutate: func(value *HeadlessRecoveryRequest) { value.PatchCount = 2 }},
		{name: "container target", mutate: func(value *HeadlessRecoveryRequest) { value.Target = "@intro" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			forged := recovery
			test.mutate(&forged)
			if _, err := recovered.RecoverHeadlessCandidate(context.Background(), forged); !errors.Is(err, ErrRevisionConflict) {
				t.Fatalf("forged recovery = %v", err)
			}
		})
	}
}

func TestHeadlessExportRejectsCallerSubstitutionForCommittedAcceptance(t *testing.T) {
	workspace := headlessWorkspace(t, "")
	review := reviewHeadless(t, workspace, beginHeadless(t, workspace, "Accepted evidence"))
	accepted := acceptHeadless(t, workspace, review, "headless-acceptance-substitution")
	forged := accepted
	forged.AcceptanceHash = strings.Repeat("d", 64)
	if _, err := workspace.PrepareHeadlessExport(HeadlessExportRequest{Review: review, Acceptance: forged,
		Actor: "reviewer:ada", Target: "artifact:forged.pdf", ApprovalNonce: "headless-export-substitution", ApprovalTTL: time.Minute}); !errors.Is(err, ErrCandidateAcceptanceDenied) {
		t.Fatalf("forged acceptance = %v", err)
	}
	prepared, err := workspace.PrepareHeadlessExport(HeadlessExportRequest{Review: review, Acceptance: accepted,
		Actor: "reviewer:ada", Target: "artifact:accepted.pdf", ApprovalNonce: "headless-export-accepted", ApprovalTTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Authorization.Evidence.RequiredScenarios) != len(accepted.ScenarioResults) ||
		prepared.Authorization.Evidence.RequiredScenarios[0] != accepted.ScenarioResults[0].Name ||
		prepared.Authorization.Evidence.Validators[0].Hash != accepted.Validators[0].Hash {
		t.Fatalf("prepared evidence = %#v", prepared.Authorization.Evidence)
	}
}

func TestHeadlessAcceptanceRegeneratesReviewBeforeTrustingHashes(t *testing.T) {
	workspace := headlessWorkspace(t, "")
	review := reviewHeadless(t, workspace, beginHeadless(t, workspace, "Review integrity"))
	scenario, err := workspace.CreateScenarioRevision([]paperscenario.Scenario{{Name: "typical"}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	request := HeadlessAcceptanceRequest{Review: review, Actor: "reviewer:ada", IdempotencyKey: "accept-review-integrity",
		Scenarios:       []ScenarioAcceptanceEvidence{{Revision: scenario.Handle, Name: "typical", Digest: scenario.Digest, ResultHash: hashBytes([]byte("typical:pass")), Passed: true}},
		Validators:      []ValidatorAcceptanceEvidence{{Profile: "layout", Version: "1.0.0", Hash: hashBytes([]byte("layout-validator/v1:pass")), Passed: true}},
		ReviewArtifacts: []ReviewAcceptanceEvidence{{Kind: "review_manifest", Hash: review.ReviewManifestHash, Approved: true}},
		ApprovalNonce:   "headless-review-integrity", ApprovalTTL: time.Minute}
	tampered := review
	tampered.ReviewManifestHash = strings.Repeat("e", 64)
	request.Review = tampered
	request.ReviewArtifacts[0].Hash = tampered.ReviewManifestHash
	if _, err := workspace.AcceptHeadlessCandidate(context.Background(), request); !errors.Is(err, ErrCandidateAcceptanceDenied) {
		t.Fatalf("tampered review = %v", err)
	}
	request.Review = review
	request.ReviewArtifacts[0].Hash = review.ReviewManifestHash
	if _, err := workspace.AcceptHeadlessCandidate(context.Background(), request); err != nil {
		t.Fatalf("valid review after rejected tamper = %v", err)
	}
}

func TestHeadlessWorkflowCancellationLeavesRecoverableExactHead(t *testing.T) {
	workspace := headlessWorkspace(t, "")
	candidate := beginHeadless(t, workspace, "Resume after cancellation")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := workspace.ReviewHeadlessCandidate(ctx, HeadlessReviewRequest{Candidate: candidate}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled review = %v", err)
	}
	head, err := workspace.Candidate(candidate.Candidate)
	if err != nil || head.Head != candidate.HeadRevision {
		t.Fatalf("cancelled review changed candidate: %#v, %v", head, err)
	}
	if resumed := reviewHeadless(t, workspace, candidate); resumed.ReviewManifestHash == "" {
		t.Fatal("review did not resume")
	}
}
