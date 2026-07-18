// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func lifecycleWorkspace(t *testing.T, now *time.Time, limits Limits) *Workspace {
	t.Helper()
	workspace, err := NewWorkspaceWithOptions(WorkspaceOptions{
		Limits: limits, HandleTTL: time.Minute, PlanTTL: time.Minute,
		DisclosureDomain: DisclosureRestricted, Now: func() time.Time { return *now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func TestEveryHandleFamilyCarriesExplicitLifecycleMetadata(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	workspace := lifecycleWorkspace(t, &now, Limits{})
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "lifecycle.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.PaperOpen(PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		Mode: CapabilityEdit, DisclosureDomain: DisclosureRestricted,
	})
	if err != nil {
		t.Fatal(err)
	}
	scenarioRevision := mustScenarioRevision(t, workspace)
	scenarioCandidate, err := workspace.NewScenarioCandidate(scenarioRevision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	semanticRevision, err := workspace.CreateSemanticTemplateRevision("document @lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	semanticCandidate, err := workspace.NewSemanticTemplateCandidate(semanticRevision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	policyRevision, err := workspace.CreatePolicyRevision("deny publish")
	if err != nil {
		t.Fatal(err)
	}
	policyCandidate, err := workspace.NewPolicyCandidate(policyRevision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	plan, _, err := workspace.CreatePlan(created.Revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	wantExpiry := now.Add(time.Minute)
	checks := []struct {
		name       string
		capability CapabilityMode
		domain     DisclosureDomain
		expires    time.Time
	}{
		{"revision", created.Revision.Capability, created.Revision.DisclosureDomain, created.Revision.ExpiresAt},
		{"candidate", created.Candidate.Capability, created.Candidate.DisclosureDomain, created.Candidate.ExpiresAt},
		{"open", opened.Mode, opened.DisclosureDomain, opened.ExpiresAt},
		{"scenario revision", scenarioRevision.Capability, scenarioRevision.DisclosureDomain, scenarioRevision.ExpiresAt},
		{"scenario candidate", scenarioCandidate.Capability, scenarioCandidate.DisclosureDomain, scenarioCandidate.ExpiresAt},
		{"semantic-template revision", semanticRevision.Capability, semanticRevision.DisclosureDomain, semanticRevision.ExpiresAt},
		{"semantic-template candidate", semanticCandidate.Capability, semanticCandidate.DisclosureDomain, semanticCandidate.ExpiresAt},
		{"policy revision", policyRevision.Capability, policyRevision.DisclosureDomain, policyRevision.ExpiresAt},
		{"policy candidate", policyCandidate.Capability, policyCandidate.DisclosureDomain, policyCandidate.ExpiresAt},
		{"plan", plan.Capability, plan.DisclosureDomain, plan.ExpiresAt},
	}
	wantCapabilities := []CapabilityMode{CapabilityRead, CapabilityEdit, CapabilityEdit, CapabilityRead, CapabilityEdit, CapabilityRead, CapabilityEdit, CapabilityRead, CapabilityEdit, CapabilityRender}
	for i, check := range checks {
		if check.capability != wantCapabilities[i] || check.domain != DisclosureRestricted || !check.expires.Equal(wantExpiry) {
			t.Fatalf("%s lifecycle = capability %q domain %q expires %v", check.name, check.capability, check.domain, check.expires)
		}
	}
	values := []scopedHandle{
		created.Revision.Handle.value, created.Candidate.Handle.value, opened.Handle.value,
		scenarioRevision.Handle.value, scenarioCandidate.Handle.value, plan.Handle.value,
		semanticRevision.Handle.value, semanticCandidate.Handle.value, policyRevision.Handle.value, policyCandidate.Handle.value,
	}
	seenNonces := map[uint64]bool{}
	for _, value := range values {
		if value.nonce == 0 || seenNonces[value.nonce] || value.domain != workspace.disclosureTag {
			t.Fatalf("opaque capability identity = %#v", value)
		}
		seenNonces[value.nonce] = true
	}
}

func TestHandleExpiryUsesInjectedClockAcrossEveryFamily(t *testing.T) {
	now := time.Unix(1_800_000_100, 0).UTC()
	workspace := lifecycleWorkspace(t, &now, Limits{MaxRevocations: 16})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "lifecycle.paper", Source: workspaceFixture})
	opened, _ := workspace.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead})
	scenarioRevision := mustScenarioRevision(t, workspace)
	scenarioCandidate, _ := workspace.NewScenarioCandidate(scenarioRevision.Handle)
	semanticRevision, _ := workspace.CreateSemanticTemplateRevision("document @expiry")
	semanticCandidate, _ := workspace.NewSemanticTemplateCandidate(semanticRevision.Handle)
	policyRevision, _ := workspace.CreatePolicyRevision("deny publish")
	policyCandidate, _ := workspace.NewPolicyCandidate(policyRevision.Handle)
	plan, _, _ := workspace.CreatePlan(created.Revision.Handle)

	now = now.Add(time.Minute)
	if removed := workspace.PruneExpiredHandles(); removed != 10 {
		t.Fatalf("PruneExpiredHandles() = %d, want 10", removed)
	}
	assertExpired := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, ErrHandleExpired) {
			t.Fatalf("%s error = %v, want ErrHandleExpired", name, err)
		}
	}
	_, err := workspace.OpenRevision(created.Revision.Handle)
	assertExpired("revision", err)
	_, err = workspace.Candidate(created.Candidate.Handle)
	assertExpired("candidate", err)
	_, err = workspace.OpenScenarioRevision(scenarioRevision.Handle)
	assertExpired("scenario revision", err)
	_, err = workspace.ScenarioCandidate(scenarioCandidate.Handle)
	assertExpired("scenario candidate", err)
	_, err = workspace.OpenSemanticTemplateRevision(semanticRevision.Handle)
	assertExpired("semantic-template revision", err)
	_, err = workspace.SemanticTemplateCandidate(semanticCandidate.Handle)
	assertExpired("semantic-template candidate", err)
	_, err = workspace.OpenPolicyRevision(policyRevision.Handle)
	assertExpired("policy revision", err)
	_, err = workspace.PolicyCandidate(policyCandidate.Handle)
	assertExpired("policy candidate", err)
	_, err = workspace.PaperContext(PaperContextRequest{Open: opened.Handle, ExpectedRevision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, MaxBytes: 1024, MaxItems: 1})
	assertExpired("open", err)
	if _, err := workspace.OpenPlan(plan.Handle); !errors.Is(err, ErrHandleExpired) || !errors.Is(err, ErrPlanExpired) {
		t.Fatalf("plan expiry compatibility = %v", err)
	}
}

func TestExplicitRevocationCoversEveryPublicHandleFamily(t *testing.T) {
	now := time.Unix(1_800_000_200, 0).UTC()
	workspace := lifecycleWorkspace(t, &now, Limits{MaxRevocations: 16})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "lifecycle.paper", Source: workspaceFixture})
	opened, _ := workspace.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityRead})
	scenarioRevision := mustScenarioRevision(t, workspace)
	scenarioCandidate, _ := workspace.NewScenarioCandidate(scenarioRevision.Handle)
	semanticRevision, _ := workspace.CreateSemanticTemplateRevision("document @revoke")
	semanticCandidate, _ := workspace.NewSemanticTemplateCandidate(semanticRevision.Handle)
	policyRevision, _ := workspace.CreatePolicyRevision("deny publish")
	policyCandidate, _ := workspace.NewPolicyCandidate(policyRevision.Handle)
	plan, _, _ := workspace.CreatePlan(created.Revision.Handle)

	if err := workspace.RevokeCandidate(created.Candidate.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokeScenarioCandidate(scenarioCandidate.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokeSemanticTemplateCandidate(semanticCandidate.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokePolicyCandidate(policyCandidate.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.ClosePaperOpen(opened.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.ReleasePlan(plan.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokeScenarioRevision(scenarioRevision.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokeSemanticTemplateRevision(semanticRevision.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokePolicyRevision(policyRevision.Handle); err != nil {
		t.Fatal(err)
	}
	if err := workspace.RevokeRevision(created.Revision.Handle); err != nil {
		t.Fatal(err)
	}

	assertRevoked := func(name string, err error, legacy error) {
		t.Helper()
		if !errors.Is(err, ErrHandleRevoked) || !errors.Is(err, legacy) {
			t.Fatalf("%s revocation = %v", name, err)
		}
	}
	_, err := workspace.OpenRevision(created.Revision.Handle)
	assertRevoked("revision", err, ErrRevisionNotFound)
	_, err = workspace.Candidate(created.Candidate.Handle)
	assertRevoked("candidate", err, ErrCandidateNotFound)
	_, err = workspace.OpenScenarioRevision(scenarioRevision.Handle)
	assertRevoked("scenario revision", err, ErrScenarioRevisionNotFound)
	_, err = workspace.ScenarioCandidate(scenarioCandidate.Handle)
	assertRevoked("scenario candidate", err, ErrScenarioCandidateNotFound)
	_, err = workspace.OpenSemanticTemplateRevision(semanticRevision.Handle)
	assertRevoked("semantic-template revision", err, ErrSemanticTemplateRevisionNotFound)
	_, err = workspace.SemanticTemplateCandidate(semanticCandidate.Handle)
	assertRevoked("semantic-template candidate", err, ErrSemanticTemplateCandidateNotFound)
	_, err = workspace.OpenPolicyRevision(policyRevision.Handle)
	assertRevoked("policy revision", err, ErrPolicyRevisionNotFound)
	_, err = workspace.PolicyCandidate(policyCandidate.Handle)
	assertRevoked("policy candidate", err, ErrPolicyCandidateNotFound)
	_, err = workspace.PaperContext(PaperContextRequest{Open: opened.Handle, ExpectedRevision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, MaxBytes: 1024, MaxItems: 1})
	assertRevoked("open", err, ErrRevisionNotFound)
	_, err = workspace.OpenPlan(plan.Handle)
	assertRevoked("plan", err, ErrPlanNotFound)
}

func TestDisclosureAndCapabilityTagsDenyCrossDomainAndForgery(t *testing.T) {
	now := time.Unix(1_800_000_300, 0).UTC()
	restricted := lifecycleWorkspace(t, &now, Limits{})
	created, _ := restricted.PaperCreate(PaperCreateRequest{File: "lifecycle.paper", Source: workspaceFixture})
	if _, err := restricted.PaperOpen(PaperOpenRequest{
		Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		Mode: CapabilityRead, DisclosureDomain: DisclosurePublic,
	}); !errors.Is(err, ErrDisclosureDenied) {
		t.Fatalf("cross-disclosure open = %v", err)
	}
	public, err := NewWorkspaceWithOptions(WorkspaceOptions{DisclosureDomain: DisclosurePublic})
	if err != nil {
		t.Fatal(err)
	}
	_, wrongErr := public.OpenRevision(created.Revision.Handle)
	if !errors.Is(wrongErr, ErrWrongWorkspace) {
		t.Fatalf("cross-workspace disclosure = %v", wrongErr)
	}
	missing := created.Revision.Handle
	missing.value.serial++
	_, missingErr := restricted.OpenRevision(missing)
	var wrongTyped *Error
	var missingTyped *Error
	if !errors.As(wrongErr, &wrongTyped) || !errors.As(missingErr, &missingTyped) {
		t.Fatalf("handle failures did not retain typed errors: %v / %v", wrongErr, missingErr)
	}
	if wrongTyped.Message != missingTyped.Message || strings.Contains(wrongErr.Error(), "restricted") || strings.Contains(missingErr.Error(), "nonce") {
		t.Fatalf("handle failure leaked shape/domain: %v / %v", wrongErr, missingErr)
	}
	forged := created.Revision.Handle
	forged.value.capability = capabilityEdit
	if _, err := restricted.OpenRevision(forged); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("forged capability = %v", err)
	}
	forged = created.Revision.Handle
	forged.value.nonce++
	if _, err := restricted.OpenRevision(forged); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("forged nonce = %v", err)
	}
}

func TestRevocationTombstonesAreBounded(t *testing.T) {
	now := time.Unix(1_800_000_400, 0).UTC()
	workspace := lifecycleWorkspace(t, &now, Limits{MaxRevocations: 2})
	handles := make([]RevisionHandle, 3)
	for i := range handles {
		revision, err := workspace.CreateRevision("bounded.paper", workspaceFixture+strings.Repeat("\n", i))
		if err != nil {
			t.Fatal(err)
		}
		handles[i] = revision.Handle
		if err := workspace.RevokeRevision(revision.Handle); err != nil {
			t.Fatal(err)
		}
	}
	if len(workspace.revocations) != 2 || len(workspace.revocationOrder) != 2 {
		t.Fatalf("revocation storage = %d/%d", len(workspace.revocations), len(workspace.revocationOrder))
	}
	if _, err := workspace.OpenRevision(handles[0]); errors.Is(err, ErrHandleRevoked) || !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("evicted tombstone = %v", err)
	}
	for _, handle := range handles[1:] {
		if _, err := workspace.OpenRevision(handle); !errors.Is(err, ErrHandleRevoked) {
			t.Fatalf("retained tombstone = %v", err)
		}
	}
}

func TestConcurrentLookupAndRevocationIsRaceSafe(t *testing.T) {
	now := time.Unix(1_800_000_500, 0).UTC()
	workspace := lifecycleWorkspace(t, &now, Limits{})
	revision, _ := workspace.CreateRevision("race.paper", workspaceFixture)
	start := make(chan struct{})
	errorsSeen := make(chan error, 33)
	var wait sync.WaitGroup
	for range 32 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := workspace.OpenRevision(revision.Handle)
			errorsSeen <- err
		}()
	}
	wait.Add(1)
	go func() {
		defer wait.Done()
		<-start
		errorsSeen <- workspace.RevokeRevision(revision.Handle)
	}()
	close(start)
	wait.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil && !errors.Is(err, ErrHandleRevoked) && !errors.Is(err, ErrRevisionNotFound) {
			t.Fatalf("concurrent lifecycle error = %v", err)
		}
	}
	if _, err := workspace.OpenRevision(revision.Handle); !errors.Is(err, ErrHandleRevoked) {
		t.Fatalf("post-revocation lookup = %v", err)
	}
}

func TestWorkspaceRejectsInvalidLifecyclePolicy(t *testing.T) {
	if _, err := NewWorkspaceWithOptions(WorkspaceOptions{HandleTTL: -time.Second}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("negative HandleTTL = %v", err)
	}
	if _, err := NewWorkspaceWithOptions(WorkspaceOptions{HandleTTL: MaxHandleTTLHard + time.Second}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("oversized HandleTTL = %v", err)
	}
	if _, err := NewWorkspaceWithOptions(WorkspaceOptions{DisclosureDomain: " restricted "}); !errors.Is(err, ErrDisclosureDenied) {
		t.Fatalf("invalid disclosure domain = %v", err)
	}
	if _, err := NewWorkspace(Limits{MaxRevocations: MaxRevocationsHard + 1}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("invalid revocation bound = %v", err)
	}
}
