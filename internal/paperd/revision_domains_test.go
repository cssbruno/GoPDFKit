// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSemanticTemplateAndPolicyDomainsAreSeparateImmutableAndCASGuarded(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	semanticBase, err := workspace.CreateSemanticTemplateRevision("document @invoice\nsection @body")
	if err != nil {
		t.Fatal(err)
	}
	policyBase, err := workspace.CreatePolicyRevision("allow edit @body\ndeny publish")
	if err != nil {
		t.Fatal(err)
	}
	semanticCandidate, err := workspace.NewSemanticTemplateCandidate(semanticBase.Handle)
	if err != nil {
		t.Fatal(err)
	}
	policyCandidate, err := workspace.NewPolicyCandidate(policyBase.Handle)
	if err != nil {
		t.Fatal(err)
	}
	if semanticBase.Digest == policyBase.Digest || semanticCandidate.HeadDigest != semanticBase.Digest || policyCandidate.HeadDigest != policyBase.Digest {
		t.Fatalf("domain identities = semantic=%+v policy=%+v", semanticBase, policyBase)
	}
	semanticResult, err := workspace.ApplySemanticTemplate(SemanticTemplateApplyRequest{Candidate: semanticCandidate.Handle, ExpectedHead: semanticBase.Handle, ExpectedDigest: semanticBase.Digest, IdempotencyKey: "semantic-1", Content: "document @invoice\nsection @content"})
	if err != nil {
		t.Fatal(err)
	}
	policyResult, err := workspace.ApplyPolicy(PolicyApplyRequest{Candidate: policyCandidate.Handle, ExpectedHead: policyBase.Handle, ExpectedDigest: policyBase.Digest, IdempotencyKey: "policy-1", Content: "allow edit @content\ndeny publish"})
	if err != nil {
		t.Fatal(err)
	}
	if semanticResult.Revision.Content != "document @invoice\nsection @content" || policyResult.Revision.Content != "allow edit @content\ndeny publish" ||
		semanticResult.Candidate.Head != semanticResult.Revision.Handle || policyResult.Candidate.Head != policyResult.Revision.Handle {
		t.Fatalf("results = %+v / %+v", semanticResult, policyResult)
	}
	semanticReplay, err := workspace.ApplySemanticTemplate(SemanticTemplateApplyRequest{Candidate: semanticCandidate.Handle, ExpectedHead: semanticBase.Handle, ExpectedDigest: semanticBase.Digest, IdempotencyKey: "semantic-1", Content: "document @invoice\nsection @content"})
	if err != nil || semanticReplay.Revision.Handle != semanticResult.Revision.Handle {
		t.Fatalf("semantic replay = %+v, %v", semanticReplay, err)
	}
	if _, err := workspace.ApplySemanticTemplate(SemanticTemplateApplyRequest{Candidate: semanticCandidate.Handle, ExpectedHead: semanticBase.Handle, ExpectedDigest: semanticBase.Digest, IdempotencyKey: "semantic-1", Content: "different"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("semantic idempotency conflict = %v", err)
	}
	forgedHead := semanticBase.Handle
	forgedHead.value.nonce++
	if _, err := workspace.ApplySemanticTemplate(SemanticTemplateApplyRequest{Candidate: semanticCandidate.Handle, ExpectedHead: forgedHead, ExpectedDigest: semanticBase.Digest, IdempotencyKey: "semantic-1", Content: "document @invoice\nsection @content"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("forged cached head identity = %v", err)
	}
	if _, err := workspace.ApplyPolicy(PolicyApplyRequest{Candidate: policyCandidate.Handle, ExpectedHead: policyBase.Handle, ExpectedDigest: policyBase.Digest, IdempotencyKey: "policy-stale", Content: "allow all"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("policy stale CAS = %v", err)
	}

	wrongSemantic := SemanticTemplateRevisionHandle{value: policyBase.Handle.value}
	if _, err := workspace.OpenSemanticTemplateRevision(wrongSemantic); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("cross-domain revision handle = %v", err)
	}
	wrongPolicyCandidate := PolicyCandidateHandle{value: semanticCandidate.Handle.value}
	if _, err := workspace.PolicyCandidate(wrongPolicyCandidate); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("cross-domain candidate handle = %v", err)
	}
	openedSemantic, _ := workspace.OpenSemanticTemplateRevision(semanticResult.Revision.Handle)
	openedPolicy, _ := workspace.OpenPolicyRevision(policyResult.Revision.Handle)
	if openedSemantic != semanticResult.Revision || openedPolicy != policyResult.Revision {
		t.Fatal("immutable revisions did not reopen exactly")
	}
}

func TestRevisionDomainConcurrentCASAndIdempotency(t *testing.T) {
	t.Run("same semantic request is one commit", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		base, _ := workspace.CreateSemanticTemplateRevision("base")
		candidate, _ := workspace.NewSemanticTemplateCandidate(base.Handle)
		request := SemanticTemplateApplyRequest{Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "same", Content: "next"}
		start := make(chan struct{})
		results := make(chan SemanticTemplateApplyResult, 2)
		errs := make(chan error, 2)
		var wait sync.WaitGroup
		for range 2 {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				r, e := workspace.ApplySemanticTemplate(request)
				results <- r
				errs <- e
			}()
		}
		close(start)
		wait.Wait()
		close(results)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		var handle SemanticTemplateRevisionHandle
		for result := range results {
			if handle.value.serial == 0 {
				handle = result.Revision.Handle
			} else if handle != result.Revision.Handle {
				t.Fatal("idempotent commits differ")
			}
		}
		if len(workspace.semanticTemplateRevisions) != 2 {
			t.Fatalf("semantic revisions=%d", len(workspace.semanticTemplateRevisions))
		}
	})
	t.Run("different policy requests CAS", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		base, _ := workspace.CreatePolicyRevision("base")
		candidate, _ := workspace.NewPolicyCandidate(base.Handle)
		start := make(chan struct{})
		errs := make(chan error, 2)
		var wait sync.WaitGroup
		for i, content := range []string{"first", "second"} {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				_, err := workspace.ApplyPolicy(PolicyApplyRequest{Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: string(rune('a' + i)), Content: content})
				errs <- err
			}()
		}
		close(start)
		wait.Wait()
		close(errs)
		success, conflict := 0, 0
		for err := range errs {
			if err == nil {
				success++
			} else if errors.Is(err, ErrRevisionConflict) {
				conflict++
			} else {
				t.Fatal(err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success/conflict=%d/%d", success, conflict)
		}
	})
}

func TestRevisionDomainLifecycleLimitsAndPartitions(t *testing.T) {
	now := time.Unix(1_900_000_000, 0).UTC()
	workspace := lifecycleWorkspace(t, &now, Limits{MaxRevocations: 16, MaxSemanticTemplateRevisions: 1, MaxPolicyCandidates: 1})
	semantic, _ := workspace.CreateSemanticTemplateRevision("semantic")
	semanticCandidate, _ := workspace.NewSemanticTemplateCandidate(semantic.Handle)
	policy, _ := workspace.CreatePolicyRevision("policy")
	policyCandidate, _ := workspace.NewPolicyCandidate(policy.Handle)
	if _, err := workspace.CreateSemanticTemplateRevision("second"); !errors.Is(err, ErrLimit) {
		t.Fatalf("semantic revision limit=%v", err)
	}
	if _, err := workspace.NewPolicyCandidate(policy.Handle); !errors.Is(err, ErrLimit) {
		t.Fatalf("policy candidate limit=%v", err)
	}
	if semantic.Capability != CapabilityRead || semanticCandidate.Capability != CapabilityEdit || policy.Capability != CapabilityRead || policyCandidate.Capability != CapabilityEdit {
		t.Fatal("domain capability tags are wrong")
	}
	now = now.Add(time.Minute)
	if removed := workspace.PruneExpiredHandles(); removed != 4 {
		t.Fatalf("domain prune=%d", removed)
	}
	if _, err := workspace.OpenSemanticTemplateRevision(semantic.Handle); !errors.Is(err, ErrHandleExpired) {
		t.Fatalf("semantic expiry=%v", err)
	}
	if _, err := workspace.PolicyCandidate(policyCandidate.Handle); !errors.Is(err, ErrHandleExpired) {
		t.Fatalf("policy candidate expiry=%v", err)
	}

	now = now.Add(time.Second)
	workspace = lifecycleWorkspace(t, &now, Limits{MaxRevocations: 16})
	semantic, _ = workspace.CreateSemanticTemplateRevision("semantic")
	semanticCandidate, _ = workspace.NewSemanticTemplateCandidate(semantic.Handle)
	policy, _ = workspace.CreatePolicyRevision("policy")
	policyCandidate, _ = workspace.NewPolicyCandidate(policy.Handle)
	semanticRecord := workspace.semanticTemplateRevisions[semantic.Handle.value.serial]
	policyCandidateRecord := workspace.policyCandidates[policyCandidate.Handle.value.serial]
	if err := workspace.RevokeSemanticTemplateCandidate(semanticCandidate.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokeSemanticTemplateRevision(semantic.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokePolicyCandidate(policyCandidate.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokePolicyRevision(policy.Handle); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.OpenPolicyRevision(policy.Handle); !errors.Is(err, ErrHandleRevoked) || !errors.Is(err, ErrPolicyRevisionNotFound) {
		t.Fatalf("policy revocation=%v", err)
	}

	other, err := NewWorkspaceWithOptions(WorkspaceOptions{ProjectID: "other", PolicyRevision: "different", DisclosureDomain: DisclosureRestricted})
	if err != nil {
		t.Fatal(err)
	}
	other.scope = workspace.scope
	other.disclosureTag = workspace.disclosureTag
	other.semanticTemplateRevisions[semantic.Handle.value.serial] = semanticRecord
	other.policyCandidates[policyCandidate.Handle.value.serial] = policyCandidateRecord
	if _, err := other.OpenSemanticTemplateRevision(semantic.Handle); !errors.Is(err, ErrSemanticTemplateRevisionNotFound) {
		t.Fatalf("semantic partition collision=%v", err)
	}
	if _, err := other.PolicyCandidate(policyCandidate.Handle); !errors.Is(err, ErrPolicyCandidateNotFound) {
		t.Fatalf("policy partition collision=%v", err)
	}
}

func TestRevisionDomainValidationIsBounded(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxSourceBytes: 8, MaxQueryBytes: 4})
	if _, err := workspace.CreatePolicyRevision("123456789"); !errors.Is(err, ErrLimit) {
		t.Fatalf("content limit=%v", err)
	}
	if _, err := workspace.CreateSemanticTemplateRevision(""); !errors.Is(err, ErrLimit) {
		t.Fatalf("empty content=%v", err)
	}
	base, _ := workspace.CreatePolicyRevision("base")
	candidate, _ := workspace.NewPolicyCandidate(base.Handle)
	if _, err := workspace.ApplyPolicy(PolicyApplyRequest{Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "long-key", Content: "next"}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("key limit=%v", err)
	}
	if _, err := workspace.ApplyPolicy(PolicyApplyRequest{Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: "bad", IdempotencyKey: "key", Content: "next"}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("digest guard=%v", err)
	}
}
