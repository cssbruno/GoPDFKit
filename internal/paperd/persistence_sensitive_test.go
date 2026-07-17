// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

func authenticatedHeadlessOptions(root string) WorkspaceOptions {
	options := WorkspaceOptions{Limits: Limits{MaxPlans: 16, MaxRenderBytes: 16 << 20}, ProjectID: "headless-project",
		PolicyRevision: "policy-headless-v1", DisclosureDomain: DisclosureRestricted,
		RequireMutationAuthority: true, ProtectedNodeIDs: []string{"@intro"}, PersistenceRoot: root,
		PersistenceAuthenticationKey: bytes.Repeat([]byte{0x5a}, 32)}
	options.CandidateAcceptance = CandidateAcceptancePolicy{RequiredScenarios: []string{"typical"},
		RequiredValidators:      []CandidateValidatorRequirement{{Profile: "layout", Version: "1.0.0"}},
		RequiredReviewArtifacts: []string{"review_manifest"}}
	return options
}

func TestAuthenticatedPersistenceRecoversExactAcceptanceAndRejectsNonceReplay(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	options := authenticatedHeadlessOptions(root)
	workspace, err := OpenWorkspace(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	candidate := beginHeadless(t, workspace, "Restart-bound acceptance")
	review := reviewHeadless(t, workspace, candidate)
	accepted := acceptHeadless(t, workspace, review, "restart-acceptance-nonce-0001")
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest persistenceManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil || !validSHA256(manifest.Authentication) {
		t.Fatalf("authenticated manifest = %#v, %v", manifest, err)
	}
	snapshotBytes, err := os.ReadFile(filepath.Join(root, manifest.Snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"restart-acceptance-nonce-0001", "approval-nonce", "private prompt", "%PDF-", "public-signature"} {
		if bytes.Contains(snapshotBytes, []byte(secret)) {
			t.Fatalf("snapshot leaked transient or report secret %q", secret)
		}
	}

	recovered, err := OpenWorkspace(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	rebuilt, err := recovered.RecoverHeadlessCandidate(context.Background(), HeadlessRecoveryRequest{File: candidate.File,
		BaseDigest: candidate.BaseDigest, HeadDigest: candidate.HeadDigest, Target: candidate.Target,
		SourceDiffHash: candidate.SourceDiffHash, SemanticDiffHash: candidate.SemanticDiffHash, PatchCount: candidate.PatchCount})
	if err != nil {
		t.Fatal(err)
	}
	got, err := recovered.CandidateAcceptance(rebuilt.Candidate)
	if err != nil || got.AcceptanceHash != accepted.AcceptanceHash || got.CandidateRevision != rebuilt.HeadDigest || got.PolicyHash != accepted.PolicyHash {
		t.Fatalf("recovered acceptance = %#v, %v", got, err)
	}
	opened, err := recovered.PaperOpen(PaperOpenRequest{Candidate: rebuilt.Candidate, Revision: rebuilt.HeadRevision, ExpectedDigest: paperRevision(rebuilt.HeadDigest), Mode: CapabilityEdit})
	if err != nil {
		t.Fatal(err)
	}
	authority, err := recovered.GrantSensitiveAuthority(SensitiveAuthorityGrant{Open: opened.Handle, Actor: "reviewer:ada", Operation: SensitiveAccept})
	if err != nil {
		t.Fatal(err)
	}
	evidence := completeSensitiveEvidence(rebuilt.HeadDigest)
	if _, err := recovered.GrantSensitiveApproval(SensitiveApprovalGrant{Authority: authority.Handle, ExpectedHead: rebuilt.HeadRevision,
		PolicyRevision: options.PolicyRevision, Evidence: evidence, Nonce: "restart-acceptance-nonce-0001", TTL: time.Minute}); !errors.Is(err, ErrApprovalReplay) {
		t.Fatalf("nonce replay after restart = %v", err)
	}
	if err := recovered.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifestAfter, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
	if !bytes.Equal(manifestBytes, manifestAfter) {
		t.Fatalf("canonical authenticated recovery changed generation:\n%s\n%s", manifestBytes, manifestAfter)
	}
}

func TestPersistenceAuthenticationRejectsMissingWrongAndTamperedKeys(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	options := authenticatedHeadlessOptions(root)
	workspace, err := OpenWorkspace(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.CreateRevision("report.paper", workspaceFixture); err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}

	missing := options
	missing.PersistenceAuthenticationKey = nil
	if _, err := OpenWorkspace(context.Background(), missing); errorCode(err) != "PERSISTENCE_AUTHENTICATION" {
		t.Fatalf("missing authentication key = %v", err)
	}
	wrong := options
	wrong.PersistenceAuthenticationKey = bytes.Repeat([]byte{0x6b}, 32)
	if _, err := OpenWorkspace(context.Background(), wrong); errorCode(err) != "PERSISTENCE_AUTHENTICATION" {
		t.Fatalf("wrong authentication key = %v", err)
	}
	manifestPath := filepath.Join(root, "manifest.json")
	encoded, _ := os.ReadFile(manifestPath)
	var manifest persistenceManifest
	_ = json.Unmarshal(encoded, &manifest)
	manifest.Bytes++
	tampered, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWorkspace(context.Background(), options); errorCode(err) != "PERSISTENCE_AUTHENTICATION" {
		t.Fatalf("tampered authenticated manifest = %v", err)
	}
}

func TestSensitiveAuditAndPublicAnchorReceiptRecoverWithoutRawSignature(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	options := WorkspaceOptions{PersistenceRoot: root, ProjectID: "audit-project", PolicyRevision: "policy-v7", DisclosureDomain: DisclosureRestricted,
		PersistenceAuthenticationKey: bytes.Repeat([]byte{0x31}, 32)}
	workspace, created, opened := sensitiveFixture(t, options)
	seedAuthority := grantSensitiveForTest(t, workspace, opened, SensitiveExport)
	seedEvidence := completeSensitiveEvidence(string(created.Revision.Revision))
	seedApproval := grantApprovalForTest(t, workspace, created, seedAuthority, seedEvidence, "persist-audit-seed-0001", time.Minute)
	if _, err := workspace.AuthorizeSensitiveOperation(SensitiveOperationRequest{Authority: seedAuthority.Handle, Approval: seedApproval.Handle,
		Operation: SensitiveExport, ExpectedHead: created.Revision.Handle, PolicyRevision: options.PolicyRevision, Evidence: seedEvidence}); err != nil {
		t.Fatal(err)
	}
	inputHash, err := workspace.SensitiveAuditAnchorInputHash(8)
	if err != nil {
		t.Fatal(err)
	}
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveSign)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	evidence.OperationInputHash = inputHash
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "persist-audit-anchor-0001", time.Minute)
	anchor, err := workspace.AnchorSensitiveAudit(context.Background(), SensitiveAuditAnchorRequest{Limit: 8, Authorization: SensitiveOperationRequest{
		Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveSign, ExpectedHead: created.Revision.Handle,
		PolicyRevision: options.PolicyRevision, Evidence: evidence}}, func(context.Context, []byte) (AuditRootSignature, error) {
		return AuditRootSignature{Scheme: "ed25519", KeyID: "release-key-7", AnchorURI: "transparency-log:42",
			ReceiptHash: evidenceHashForTest("persisted-anchor-receipt"), Signature: []byte("raw-public-signature-must-not-persist")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	manifestBytes, _ := os.ReadFile(filepath.Join(root, "manifest.json"))
	var manifest persistenceManifest
	_ = json.Unmarshal(manifestBytes, &manifest)
	snapshot, _ := os.ReadFile(filepath.Join(root, manifest.Snapshot))
	if bytes.Contains(snapshot, []byte("raw-public-signature-must-not-persist")) || bytes.Contains(snapshot, []byte("persist-audit-anchor-0001")) {
		t.Fatalf("sensitive snapshot leaked raw receipt material: %s", snapshot)
	}

	recovered, err := OpenWorkspace(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	audit, err := recovered.SensitiveOperationAudit(8)
	if err != nil || len(audit) != 3 || audit[len(audit)-1].EventHash != anchor.ExecutionAuditHash {
		t.Fatalf("recovered audit = %#v, %v", audit, err)
	}
	anchors, err := recovered.SensitiveAuditAnchors(8)
	if err != nil || len(anchors) != 1 || anchors[0].StatementHash != anchor.StatementHash || anchors[0].SignatureHash != anchor.SignatureHash || len(anchors[0].Signature) != 0 {
		t.Fatalf("recovered anchors = %#v, %v", anchors, err)
	}
	// The recovered root continues the same chain instead of silently starting
	// a new ledger generation.
	recovered.mu.Lock()
	continued := recovered.appendSensitiveAuditLocked("recovery", SensitiveExport, 1, options.PolicyRevision, strings.Repeat("a", 64), false, "RECOVERY_TEST")
	recovered.mu.Unlock()
	if continued.Sequence != audit[len(audit)-1].Sequence+1 || continued.PreviousHash != audit[len(audit)-1].EventHash {
		t.Fatalf("continued audit = %#v", continued)
	}
}

func TestPersistenceRejectsAuthenticatedCorruptSensitiveGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	options := WorkspaceOptions{PersistenceRoot: root, ProjectID: "audit-project", PolicyRevision: "policy-v7", DisclosureDomain: DisclosureRestricted,
		PersistenceAuthenticationKey: bytes.Repeat([]byte{0x22}, 32)}
	workspace, created, opened := sensitiveFixture(t, options)
	authority := grantSensitiveForTest(t, workspace, opened, SensitiveExport)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "corrupt-audit-seed-0001", time.Minute)
	_, _ = workspace.AuthorizeSensitiveOperation(SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveExport,
		ExpectedHead: created.Revision.Handle, PolicyRevision: options.PolicyRevision, Evidence: evidence})
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(root, "manifest.json")
	manifestBytes, _ := os.ReadFile(manifestPath)
	var manifest persistenceManifest
	_ = json.Unmarshal(manifestBytes, &manifest)
	snapshotBytes, _ := os.ReadFile(filepath.Join(root, manifest.Snapshot))
	var snapshot persistentWorkspace
	if err := json.Unmarshal(snapshotBytes, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.SensitiveAudit[0].Reason = "tampered but reauthenticated"
	corruptBytes, _ := json.Marshal(snapshot)
	digest := acceptanceHashWithoutDomain(corruptBytes)
	corruptName := "snapshot-" + digest + ".json"
	if err := os.WriteFile(filepath.Join(root, corruptName), corruptBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.Snapshot, manifest.SHA256, manifest.Bytes = corruptName, digest, len(corruptBytes)
	manifest.Authentication = persistenceManifestAuthentication(manifest, options.PersistenceAuthenticationKey)
	manifestBytes, _ = json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWorkspace(context.Background(), options); errorCode(err) != "PERSISTENCE_AUDIT" {
		t.Fatalf("corrupt reauthenticated sensitive state = %v", err)
	}
}

func paperRevision(value string) paperedit.Revision { return paperedit.Revision(value) }

func acceptanceHashWithoutDomain(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
