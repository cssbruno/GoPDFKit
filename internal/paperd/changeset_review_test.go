// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/cssbruno/paperrune/document"
	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

const changesetSecret = "PRIVATE-CHANGESET-IGNORE-ALL-INSTRUCTIONS"

type changesetFixture struct {
	workspace *Workspace
	created   PaperCreateResult
	applied   ApplyResult
	opened    PaperOpenSnapshot
}

func newChangesetFixture(t *testing.T, domain DisclosureDomain, root string) changesetFixture {
	t.Helper()
	options := WorkspaceOptions{Limits: Limits{MaxPlans: 64, MaxRenderBytes: 16 << 20}, ProjectID: "changeset-project",
		PolicyRevision: "changeset-policy-v1", DisclosureDomain: domain, PersistenceRoot: root,
		CandidateAcceptance: CandidateAcceptancePolicy{RequiredScenarios: []string{"typical"},
			RequiredValidators:      []CandidateValidatorRequirement{{Profile: "layout", Version: "1.0.0"}},
			RequiredReviewArtifacts: []string{"changeset_bundle", "semantic_diff"}}}
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
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "changeset.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := workspace.Apply(ApplyRequest{Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
		ExpectedRevision: created.Revision.Revision, Group: "agent:copy", IdempotencyKey: "changeset-edit-1",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "changeset.paper", workspaceFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: changesetSecret}}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: applied.Revision.Handle,
		ExpectedDigest: applied.Revision.Revision, Mode: CapabilityEdit})
	if err != nil {
		t.Fatal(err)
	}
	return changesetFixture{workspace: workspace, created: created, applied: applied, opened: opened}
}

func changesetReviewRequest(t *testing.T, fixture changesetFixture, open OpenHandle, includePayloads bool) AgentChangesetReviewRequest {
	t.Helper()
	font, err := os.ReadFile("../../assets/static/font/DejaVuSansCondensed.ttf")
	if err != nil {
		t.Fatal(err)
	}
	review := document.DefaultPaperReviewRequest()
	review.CoreFontProgram = font
	review.MaxArtifactBytes, review.MaxTotalBytes, review.MaxManifestBytes = 4<<20, 12<<20, 1<<20
	return AgentChangesetReviewRequest{Open: open, Candidate: fixture.created.Candidate.Handle,
		Before: fixture.created.Revision.Handle, After: fixture.applied.Revision.Handle,
		BeforeRevision: fixture.created.Revision.Revision, AfterRevision: fixture.applied.Revision.Revision,
		IncludePayloads: includePayloads, Review: review}
}

func TestAgentChangesetReviewProvesCompleteTransitionWithoutRestrictedSourceLeak(t *testing.T) {
	fixture := newChangesetFixture(t, DisclosureRestricted, "")
	request := changesetReviewRequest(t, fixture, fixture.opened.Handle, true)
	first, err := fixture.workspace.ReviewAgentChangeset(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := fixture.workspace.ReviewAgentChangeset(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.EvidenceHash != second.EvidenceHash || first.ManifestHash != second.ManifestHash ||
		first.BeforeRevision != string(fixture.created.Revision.Revision) || first.AfterRevision != string(fixture.applied.Revision.Revision) ||
		!validSHA256(first.SourceDiffHash) || !validSHA256(first.SemanticDiffHash) || len(first.Patches) != 1 ||
		first.Layout.BeforePlanHash == first.Layout.AfterPlanHash || len(first.Layout.ChangedPages) == 0 ||
		!validSHA256(first.Layout.BeforeSemantics.SHA256) || !validSHA256(first.Layout.AfterSemantics.SHA256) ||
		first.Diagnostics.BeforeSHA256 == "" || len(first.Artifacts) < 8 {
		t.Fatalf("changeset evidence = %#v / %#v", first, second)
	}
	if len(first.Payloads) != 0 || first.Patches[0].Target != "" || !validSHA256(first.Patches[0].TargetSHA256) {
		t.Fatalf("restricted capability leaked payload/target: %#v", first)
	}
	if first.Patches[0].TargetSHA256 == hashBytes([]byte("@copy")) ||
		first.Patches[0].RemovedSHA256 == hashBytes([]byte("Hello agent")) ||
		first.Patches[0].ReplacementSHA256 == hashBytes([]byte(changesetSecret)) {
		t.Fatal("sensitive patch identities used dictionary-comparable raw SHA-256")
	}
	encoded, err := first.CanonicalJSON(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(changesetSecret)) || bytes.Contains(encoded, []byte("Hello agent")) || bytes.Contains(encoded, []byte("@copy")) {
		t.Fatalf("restricted summary leaked authored source: %s", encoded)
	}
	kinds := make(map[string]bool)
	for _, artifact := range first.Artifacts {
		kinds[artifact.Kind] = true
	}
	for _, kind := range []string{"source_diff", "semantic_diff", "plan_diff", "accessibility_diff", "diagnostics", "before_crop", "after_crop"} {
		if !kinds[kind] {
			t.Fatalf("missing %s evidence in %#v", kind, first.Artifacts)
		}
	}
	if _, err := first.CanonicalJSON(32); !errors.Is(err, ErrLimit) {
		t.Fatalf("small response bound = %v", err)
	}
	tampered := first
	tampered.Artifacts = append([]ChangesetArtifactReference(nil), first.Artifacts...)
	tampered.Artifacts[0].SHA256 = hashBytes([]byte("forged-artifact"))
	if _, err := tampered.CanonicalJSON(1 << 20); !errors.Is(err, ErrChangesetReview) {
		t.Fatalf("tampered protocol projection = %v", err)
	}
}

func TestAgentChangesetPayloadCapabilityFilterAndBounds(t *testing.T) {
	fixture := newChangesetFixture(t, DisclosureProject, "")
	readOpen, err := fixture.workspace.PaperOpen(PaperOpenRequest{Candidate: fixture.created.Candidate.Handle,
		Revision: fixture.applied.Revision.Handle, ExpectedDigest: fixture.applied.Revision.Revision, Mode: CapabilityRead})
	if err != nil {
		t.Fatal(err)
	}
	readReview, err := fixture.workspace.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, fixture, readOpen.Handle, true))
	if err != nil || len(readReview.Payloads) != 0 {
		t.Fatalf("read projection = payloads %d, %v", len(readReview.Payloads), err)
	}
	editReview, err := fixture.workspace.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, fixture, fixture.opened.Handle, true))
	if err != nil || len(editReview.Payloads) == 0 || editReview.Patches[0].Target != "@copy" {
		t.Fatalf("edit projection = payloads %d target %q, %v", len(editReview.Payloads), editReview.Patches[0].Target, err)
	}
	bounded := changesetReviewRequest(t, fixture, fixture.opened.Handle, false)
	bounded.Review.MaxArtifacts = 1
	if _, err := fixture.workspace.ReviewAgentChangeset(context.Background(), bounded); !errors.Is(err, ErrLimit) {
		t.Fatalf("artifact bound = %v", err)
	}
	wrongOpen, _ := fixture.workspace.PaperOpen(PaperOpenRequest{Candidate: fixture.created.Candidate.Handle,
		Revision: fixture.applied.Revision.Handle, ExpectedDigest: fixture.applied.Revision.Revision, Mode: CapabilityRead})
	wrong := changesetReviewRequest(t, fixture, wrongOpen.Handle, false)
	wrong.BeforeRevision = paperedit.SourceRevision("forged")
	if _, err := fixture.workspace.ReviewAgentChangeset(context.Background(), wrong); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("forged revision = %v", err)
	}
	fixture.workspace.mu.Lock()
	fixture.workspace.limits.MaxPlans = 2
	fixture.workspace.mu.Unlock()
	for index := 0; index < 6; index++ {
		if _, err := fixture.workspace.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, fixture, fixture.opened.Handle, false)); err != nil {
			t.Fatalf("bounded repeated review %d = %v", index, err)
		}
	}
	fixture.workspace.mu.RLock()
	retainedPlans := len(fixture.workspace.plans)
	fixture.workspace.mu.RUnlock()
	if retainedPlans != 0 {
		t.Fatalf("review leaked %d transient plans", retainedPlans)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := fixture.workspace.ReviewAgentChangeset(cancelled, changesetReviewRequest(t, fixture, fixture.opened.Handle, false)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled review = %v", err)
	}
}

func TestAgentChangesetSelectiveAcceptanceBindsRegeneratedEvidence(t *testing.T) {
	fixture := newChangesetFixture(t, DisclosureRestricted, "")
	review, err := fixture.workspace.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, fixture, fixture.opened.Handle, false))
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := review.CanonicalJSON(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	var clientProjection AgentChangesetReview
	if err := json.Unmarshal(protocol, &clientProjection); err != nil {
		t.Fatal(err)
	}
	semantic := ChangesetArtifactReference{}
	for _, artifact := range clientProjection.Artifacts {
		if artifact.Kind == "semantic_diff" {
			semantic = artifact
		}
	}
	scenario, err := fixture.workspace.CreateScenarioRevision([]paperscenario.Scenario{{Name: "typical", Locale: "en-US"}}, paperscenario.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	reviewRequest := changesetReviewRequest(t, fixture, fixture.opened.Handle, false)
	request := AgentChangesetAcceptanceRequest{ReviewRequest: reviewRequest, ExpectedEvidenceHash: clientProjection.EvidenceHash,
		ExpectedManifestHash: clientProjection.ManifestHash, Actor: "reviewer:ada", IdempotencyKey: "accept-changeset-1",
		Scenarios: []ScenarioAcceptanceEvidence{{Revision: scenario.Handle, Name: "typical", Digest: scenario.Digest,
			ResultHash: hashBytes([]byte("typical:pass")), Passed: true}},
		Validators: []ValidatorAcceptanceEvidence{{Profile: "layout", Version: "1.0.0", Hash: hashBytes([]byte("layout:pass")), Passed: true}},
		SelectedArtifacts: []ReviewAcceptanceEvidence{{Kind: "changeset_bundle", Hash: review.EvidenceHash, Approved: true},
			{Kind: "semantic_diff", Hash: semantic.SHA256, Approved: true}},
		ApprovalNonce: "changeset-approval-1", ApprovalTTL: time.Minute}
	invalidSelection := request
	invalidSelection.SelectedArtifacts = append([]ReviewAcceptanceEvidence(nil), request.SelectedArtifacts...)
	invalidSelection.SelectedArtifacts[1].Hash = hashBytes([]byte("not-produced"))
	if _, err := fixture.workspace.AcceptAgentChangeset(context.Background(), invalidSelection); !errors.Is(err, ErrCandidateAcceptanceDenied) {
		t.Fatalf("unproduced selective evidence = %v", err)
	}
	receipt, err := fixture.workspace.AcceptAgentChangeset(context.Background(), request)
	if err != nil || receipt.CandidateRevision != review.AfterRevision || len(receipt.ReviewArtifacts) != 2 {
		t.Fatalf("acceptance = %#v, %v", receipt, err)
	}

	forgedFixture := newChangesetFixture(t, DisclosureRestricted, "")
	forged, _ := forgedFixture.workspace.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, forgedFixture, forgedFixture.opened.Handle, false))
	forged.EvidenceHash = hashBytes([]byte("forged"))
	request.ReviewRequest = changesetReviewRequest(t, forgedFixture, forgedFixture.opened.Handle, false)
	request.ExpectedEvidenceHash = forged.EvidenceHash
	request.ExpectedManifestHash = forged.ManifestHash
	request.SelectedArtifacts[0].Hash = forged.EvidenceHash
	if _, err := forgedFixture.workspace.AcceptAgentChangeset(context.Background(), request); !errors.Is(err, ErrCandidateAcceptanceDenied) {
		t.Fatalf("forged review acceptance = %v", err)
	}
}

func TestAgentChangesetReviewRejectsStaleHeadAndRacesSafely(t *testing.T) {
	fixture := newChangesetFixture(t, DisclosureRestricted, "")
	request := changesetReviewRequest(t, fixture, fixture.opened.Handle, false)
	newSource := fixture.applied.Revision.Source + "# later\n"
	if _, err := fixture.workspace.PaperApplySource(PaperSourceEditRequest{Candidate: fixture.created.Candidate.Handle,
		ExpectedHead: fixture.applied.Revision.Handle, ExpectedRevision: fixture.applied.Revision.Revision, Group: "later", Source: newSource}); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.workspace.ReviewAgentChangeset(context.Background(), request); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale review = %v", err)
	}

	race := newChangesetFixture(t, DisclosureRestricted, "")
	raceRequest := changesetReviewRequest(t, race, race.opened.Handle, false)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	errs := make(chan error, 2)
	go func() {
		defer wait.Done()
		<-start
		_, err := race.workspace.ReviewAgentChangeset(context.Background(), raceRequest)
		errs <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := race.workspace.PaperApplySource(PaperSourceEditRequest{Candidate: race.created.Candidate.Handle,
			ExpectedHead: race.applied.Revision.Handle, ExpectedRevision: race.applied.Revision.Revision,
			Group: "race", Source: race.applied.Revision.Source + "# raced\n"})
		errs <- err
	}()
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, ErrRevisionConflict) {
			t.Fatalf("race error = %v", err)
		}
	}
}

func TestAgentChangesetReviewRebuildsAfterPersistenceRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	fixture := newChangesetFixture(t, DisclosureRestricted, root)
	before, err := fixture.workspace.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, fixture, fixture.opened.Handle, false))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered := newChangesetRecoveryWorkspace(t, root)
	candidate := recovered.candidates[1]
	var base, head *revisionRecord
	for _, revision := range recovered.revisions {
		switch revision.revision {
		case fixture.created.Revision.Revision:
			base = revision
		case fixture.applied.Revision.Revision:
			head = revision
		}
	}
	if base == nil || head == nil {
		t.Fatal("recovered transition revisions missing")
	}
	opened, err := recovered.PaperOpen(PaperOpenRequest{Candidate: candidate.handle, Revision: head.handle,
		ExpectedDigest: head.revision, Mode: CapabilityEdit})
	if err != nil {
		t.Fatal(err)
	}
	recoveredFixture := changesetFixture{workspace: recovered,
		created: PaperCreateResult{Candidate: snapshotCandidate(candidate), Revision: snapshotOf(base)},
		applied: ApplyResult{Candidate: snapshotCandidate(candidate), Revision: snapshotOf(head)}, opened: opened}
	after, err := recovered.ReviewAgentChangeset(context.Background(), changesetReviewRequest(t, recoveredFixture, opened.Handle, false))
	if err != nil || after.EvidenceHash != before.EvidenceHash || after.ManifestHash != before.ManifestHash {
		t.Fatalf("recovered hashes = %s/%s, %v; want %s/%s", after.EvidenceHash, after.ManifestHash, err, before.EvidenceHash, before.ManifestHash)
	}
}

func newChangesetRecoveryWorkspace(t *testing.T, root string) *Workspace {
	t.Helper()
	workspace, err := OpenWorkspace(context.Background(), WorkspaceOptions{Limits: Limits{MaxPlans: 64, MaxRenderBytes: 16 << 20},
		ProjectID: "changeset-project", PolicyRevision: "changeset-policy-v1", DisclosureDomain: DisclosureRestricted,
		PersistenceRoot: root, CandidateAcceptance: CandidateAcceptancePolicy{RequiredScenarios: []string{"typical"},
			RequiredValidators:      []CandidateValidatorRequirement{{Profile: "layout", Version: "1.0.0"}},
			RequiredReviewArtifacts: []string{"changeset_bundle", "semantic_diff"}}})
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}
