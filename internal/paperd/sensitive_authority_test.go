// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func sensitiveFixture(t *testing.T, options WorkspaceOptions) (*Workspace, PaperCreateResult, PaperOpenSnapshot) {
	t.Helper()
	workspace, err := NewWorkspaceWithOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "sensitive.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityEdit})
	if err != nil {
		t.Fatal(err)
	}
	return workspace, created, opened
}

func evidenceHashForTest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func completeSensitiveEvidence(revision string) SensitiveEvidence {
	return SensitiveEvidence{
		CandidateRevision: revision,
		SourceDiffHash:    evidenceHashForTest("source-diff"), SemanticDiffHash: evidenceHashForTest("semantic-diff"),
		OperationInputHash: evidenceHashForTest("operation-input"),
		RequiredScenarios:  []string{"extreme", "typical"},
		Validators: []ValidatorEvidence{
			{Profile: "pdfua", Version: "2.4.1", Hash: evidenceHashForTest("pdfua-result")},
			{Profile: "layout", Version: "1.8.0", Hash: evidenceHashForTest("layout-result")},
		},
		ReviewArtifacts: []ReviewArtifactEvidence{
			{Kind: "contact-sheet", Hash: evidenceHashForTest("contact-sheet")},
			{Kind: "semantic-diff", Hash: evidenceHashForTest("review-diff")},
		},
	}
}

func grantSensitiveForTest(t *testing.T, workspace *Workspace, opened PaperOpenSnapshot, operation SensitiveOperation) SensitiveAuthoritySnapshot {
	t.Helper()
	authority, err := workspace.GrantSensitiveAuthority(SensitiveAuthorityGrant{Open: opened.Handle, Actor: "agent:release", Operation: operation})
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func grantApprovalForTest(t *testing.T, workspace *Workspace, created PaperCreateResult, authority SensitiveAuthoritySnapshot, evidence SensitiveEvidence, nonce string, ttl time.Duration) SensitiveApprovalSnapshot {
	t.Helper()
	approval, err := workspace.GrantSensitiveApproval(SensitiveApprovalGrant{
		Authority: authority.Handle, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7",
		Evidence: evidence, Nonce: nonce, TTL: ttl,
	})
	if err != nil {
		t.Fatal(err)
	}
	return approval
}

func TestSensitiveCapabilitiesAreSeparateAndNonInterchangeable(t *testing.T) {
	workspace, _, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	operations := []SensitiveOperation{SensitiveExport, SensitivePublish, SensitiveAttachment, SensitiveProductionCapture, SensitiveSign, SensitiveAccept}
	handles := make(map[handleCapability]struct{})
	for _, operation := range operations {
		authority := grantSensitiveForTest(t, workspace, opened, operation)
		if authority.Operation != operation || authority.Actor != "agent:release" {
			t.Fatalf("authority(%s) = %#v", operation, authority)
		}
		handles[authority.Handle.value.capability] = struct{}{}
	}
	if len(handles) != len(operations) {
		t.Fatalf("sensitive operations shared capabilities: %#v", handles)
	}

	exportAuthority := grantSensitiveForTest(t, workspace, opened, SensitiveExport)
	evidence := completeSensitiveEvidence(string(opened.Digest))
	approval := grantApprovalForTest(t, workspace, PaperCreateResult{Revision: RevisionSnapshot{Handle: opened.Revision}}, exportAuthority, evidence, "separate-capability-0001", time.Minute)
	_, err := workspace.AuthorizeSensitiveOperation(SensitiveOperationRequest{
		Authority: exportAuthority.Handle, Approval: approval.Handle, Operation: SensitivePublish,
		ExpectedHead: opened.Revision, PolicyRevision: "policy-v7", Evidence: evidence,
	})
	if errorCode(err) != "SENSITIVE_CAPABILITY_DENIED" {
		t.Fatalf("export authority used for publish = %v", err)
	}
	if openedAuthority, err := workspace.OpenSensitiveAuthority(exportAuthority.Handle); err != nil || openedAuthority.Operation != SensitiveExport {
		t.Fatalf("OpenSensitiveAuthority = %#v, %v", openedAuthority, err)
	}
}

func TestSensitiveApprovalBindsEvidenceAndIsConsumedOnce(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7", HandleTTL: time.Hour, Now: func() time.Time { return now }})
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveSign)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	reordered := evidence
	reordered.RequiredScenarios = []string{"typical", "extreme"}
	reordered.Validators = []ValidatorEvidence{evidence.Validators[1], evidence.Validators[0]}
	reordered.ReviewArtifacts = []ReviewArtifactEvidence{evidence.ReviewArtifacts[1], evidence.ReviewArtifacts[0]}
	_, firstHash, _ := canonicalSensitiveEvidence(evidence, 1024)
	_, secondHash, _ := canonicalSensitiveEvidence(reordered, 1024)
	if firstHash != secondHash {
		t.Fatal("canonical evidence hash depends on caller ordering")
	}
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "single-use-approval-0001", 10*time.Minute)
	request := SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveSign, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: reordered}
	receipt, err := workspace.AuthorizeSensitiveOperation(request)
	if err != nil || receipt.EvidenceHash != approval.EvidenceHash || receipt.AuditHash == "" {
		t.Fatalf("AuthorizeSensitiveOperation = %#v, %v", receipt, err)
	}
	if _, err := workspace.AuthorizeSensitiveOperation(request); !errors.Is(err, ErrApprovalReplay) || errorCode(err) != "APPROVAL_REPLAY" {
		t.Fatalf("approval replay = %v", err)
	}
	audit, err := workspace.SensitiveOperationAudit(8)
	if err != nil || len(audit) != 2 || !audit[0].Allowed || audit[1].Allowed || audit[1].PreviousHash != audit[0].EventHash {
		t.Fatalf("sensitive audit chain = %#v, %v", audit, err)
	}
	if audit[0].EvidenceHash != approval.EvidenceHash || audit[0].EventHash != receipt.AuditHash {
		t.Fatalf("audit/receipt evidence mismatch: %#v %#v", audit[0], receipt)
	}
}

func TestSensitiveApprovalRejectsPolicyEvidenceNonceExpiryAndRevocation(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7", HandleTTL: time.Hour, Now: func() time.Time { return now }})
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveProductionCapture)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	grant := SensitiveApprovalGrant{Authority: authority.Handle, ExpectedHead: created.Revision.Handle, PolicyRevision: "wrong-policy", Evidence: evidence, Nonce: "binding-check-000001", TTL: time.Minute}
	if _, err := workspace.GrantSensitiveApproval(grant); errorCode(err) != "APPROVAL_POLICY_MISMATCH" {
		t.Fatalf("wrong policy = %v", err)
	}
	grant.PolicyRevision = "policy-v7"
	grant.Evidence.CandidateRevision = evidenceHashForTest("different-revision")
	if _, err := workspace.GrantSensitiveApproval(grant); errorCode(err) != "APPROVAL_EVIDENCE_MISMATCH" {
		t.Fatalf("wrong candidate evidence = %v", err)
	}
	grant.Evidence = evidence
	approval, err := workspace.GrantSensitiveApproval(grant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.GrantSensitiveApproval(grant); errorCode(err) != "APPROVAL_NONCE_REPLAY" || !errors.Is(err, ErrApprovalReplay) {
		t.Fatalf("nonce replay = %v", err)
	}
	now = now.Add(time.Minute)
	request := SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveProductionCapture, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}
	if _, err := workspace.AuthorizeSensitiveOperation(request); errorCode(err) != "APPROVAL_EXPIRED" {
		t.Fatalf("expired approval = %v", err)
	}

	now = now.Add(time.Second)
	freshAuthority := grantSensitiveForTest(t, workspace, opened, SensitiveExport)
	freshApproval := grantApprovalForTest(t, workspace, created, freshAuthority, evidence, "revoked-approval-0001", time.Minute)
	if err := workspace.RevokeSensitiveApproval(freshApproval.Handle); err != nil {
		t.Fatal(err)
	}
	request = SensitiveOperationRequest{Authority: freshAuthority.Handle, Approval: freshApproval.Handle, Operation: SensitiveExport, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}
	if _, err := workspace.AuthorizeSensitiveOperation(request); errorCode(err) != "SENSITIVE_APPROVAL_REQUIRED" {
		t.Fatalf("revoked approval = %v", err)
	}
	audit, _ := workspace.SensitiveOperationAudit(8)
	if len(audit) != 2 || audit[0].Reason != "APPROVAL_EXPIRED" || audit[1].Reason != "SENSITIVE_APPROVAL_REQUIRED" {
		t.Fatalf("denied sensitive audit = %#v", audit)
	}
}

func TestSensitiveApprovalConcurrentConsumptionAllowsExactlyOne(t *testing.T) {
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	authority := grantSensitiveForTest(t, workspace, opened, SensitivePublish)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "concurrent-approval-001", time.Minute)
	request := SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitivePublish, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}
	results := make([]SensitiveOperationReceipt, 2)
	errs := make([]error, 2)
	var wait sync.WaitGroup
	for index := range results {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			results[index], errs[index] = workspace.AuthorizeSensitiveOperation(request)
		}(index)
	}
	wait.Wait()
	allowed := 0
	for index := range results {
		if errs[index] == nil {
			allowed++
		} else if !errors.Is(errs[index], ErrApprovalReplay) {
			t.Fatalf("concurrent error[%d] = %v", index, errs[index])
		}
	}
	if allowed != 1 {
		t.Fatalf("concurrent approvals allowed=%d results=%#v errors=%v", allowed, results, errs)
	}
	audit, _ := workspace.SensitiveOperationAudit(2)
	if len(audit) != 2 || reflect.DeepEqual(audit[0].Allowed, audit[1].Allowed) || audit[1].PreviousHash != audit[0].EventHash {
		t.Fatalf("concurrent audit = %#v", audit)
	}
}
