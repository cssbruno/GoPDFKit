// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestAnchorSensitiveAuditSignsExactVerifiedRootWithSeparateAuthority(t *testing.T) {
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	seedAuthority := grantSensitiveForTest(t, workspace, opened, SensitiveExport)
	seedEvidence := completeSensitiveEvidence(string(created.Revision.Revision))
	seedApproval := grantApprovalForTest(t, workspace, created, seedAuthority, seedEvidence, "audit-seed-approval", time.Minute)
	if _, err := workspace.AuthorizeSensitiveOperation(SensitiveOperationRequest{Authority: seedAuthority.Handle, Approval: seedApproval.Handle, Operation: SensitiveExport, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: seedEvidence}); err != nil {
		t.Fatal(err)
	}

	inputHash, err := workspace.SensitiveAuditAnchorInputHash(8)
	if err != nil {
		t.Fatal(err)
	}
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveSign)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	evidence.OperationInputHash = inputHash
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "audit-anchor-approval", time.Minute)
	request := SensitiveAuditAnchorRequest{Limit: 8, Authorization: SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveSign, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}}
	var signed []byte
	anchor, err := workspace.AnchorSensitiveAudit(context.Background(), request, func(_ context.Context, payload []byte) (AuditRootSignature, error) {
		signed = append([]byte(nil), payload...)
		return AuditRootSignature{Scheme: "ed25519", KeyID: "release-key-7", AnchorURI: "transparency-log:42", ReceiptHash: evidenceHashForTest("anchor-receipt"), Signature: []byte("public-signature")}, nil
	})
	if err != nil || anchor.Statement.Count != 1 || anchor.Statement.RootHash == "" || anchor.StatementHash != digestBytes(signed) || anchor.SignatureHash != digestBytes([]byte("public-signature")) || anchor.ExecutionAuditHash == "" {
		t.Fatalf("anchor = %#v, %v", anchor, err)
	}
	anchor.Signature[0] ^= 0xff
	if bytes.Equal(anchor.Signature, []byte("public-signature")) {
		t.Fatal("anchor signature was not detached")
	}
	audit, _ := workspace.SensitiveOperationAudit(8)
	if len(audit) != 3 || !audit[0].Allowed || !audit[1].Allowed || !audit[2].Allowed || audit[2].Reason != "sensitive audit root externally anchored" {
		t.Fatalf("anchor audit = %#v", audit)
	}
}

func TestAnchorSensitiveAuditRejectsStaleRootBeforeSignerAndConsumesOnSignerFailure(t *testing.T) {
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	seedAuthority := grantSensitiveForTest(t, workspace, opened, SensitiveExport)
	seedEvidence := completeSensitiveEvidence(string(created.Revision.Revision))
	seedApproval := grantApprovalForTest(t, workspace, created, seedAuthority, seedEvidence, "audit-stale-seed", time.Minute)
	if _, err := workspace.AuthorizeSensitiveOperation(SensitiveOperationRequest{Authority: seedAuthority.Handle, Approval: seedApproval.Handle, Operation: SensitiveExport, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: seedEvidence}); err != nil {
		t.Fatal(err)
	}
	inputHash, _ := workspace.SensitiveAuditAnchorInputHash(8)
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveSign)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	evidence.OperationInputHash = inputHash
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "audit-stale-anchor", time.Minute)
	request := SensitiveAuditAnchorRequest{Limit: 8, Authorization: SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveSign, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}}
	workspace.auditSensitiveExecutionDenial(request.Authorization, "CONCURRENT_AUDIT_EVENT")
	calls := 0
	if _, err := workspace.AnchorSensitiveAudit(context.Background(), request, func(context.Context, []byte) (AuditRootSignature, error) { calls++; return AuditRootSignature{}, nil }); errorCode(err) != "AUDIT_ANCHOR_BINDING_MISMATCH" || calls != 0 {
		t.Fatalf("stale anchor = %v calls=%d", err, calls)
	}

	inputHash, _ = workspace.SensitiveAuditAnchorInputHash(8)
	evidence.OperationInputHash = inputHash
	approval = grantApprovalForTest(t, workspace, created, authority, evidence, "audit-failure-anchor", time.Minute)
	request.Authorization.Approval = approval.Handle
	request.Authorization.Evidence = evidence
	anchor, err := workspace.AnchorSensitiveAudit(context.Background(), request, func(context.Context, []byte) (AuditRootSignature, error) {
		calls++
		return AuditRootSignature{}, errors.New("log unavailable")
	})
	if !errors.Is(err, ErrAuditAnchor) || anchor.Authorization.AuditHash == "" || anchor.ExecutionAuditHash == "" || calls != 1 {
		t.Fatalf("failed anchor = %#v %v calls=%d", anchor, err, calls)
	}
	if _, err := workspace.AuthorizeSensitiveOperation(request.Authorization); !errors.Is(err, ErrApprovalReplay) || calls != 1 {
		t.Fatalf("anchor approval replay = %v calls=%d", err, calls)
	}
}
