// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

var ErrChangesetReview = errors.New("paperd: changeset review failed")

type AgentChangesetReviewRequest struct {
	Open            OpenHandle         `json:"-"`
	Candidate       CandidateHandle    `json:"-"`
	Before          RevisionHandle     `json:"-"`
	After           RevisionHandle     `json:"-"`
	BeforeRevision  paperedit.Revision `json:"before_revision"`
	AfterRevision   paperedit.Revision `json:"after_revision"`
	IncludePayloads bool               `json:"include_payloads,omitempty"`
	Review          document.PaperReviewRequest
}

type ChangesetPatchEvidence struct {
	Index             uint32 `json:"index"`
	Target            string `json:"target,omitempty"`
	TargetSHA256      string `json:"target_sha256"`
	Start             uint32 `json:"start"`
	End               uint32 `json:"end"`
	RemovedBytes      int    `json:"removed_bytes"`
	ReplacementBytes  int    `json:"replacement_bytes"`
	RemovedSHA256     string `json:"removed_sha256"`
	ReplacementSHA256 string `json:"replacement_sha256"`
}

type ChangesetArtifactReference struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Page   uint32 `json:"page,omitempty"`
}

type ChangesetDiagnosticEvidence struct {
	BeforeCount  int    `json:"before_count"`
	AfterCount   int    `json:"after_count"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
}

type ChangesetLayoutEvidence struct {
	BeforePlanHash      string                                   `json:"before_plan_hash"`
	AfterPlanHash       string                                   `json:"after_plan_hash"`
	BeforePages         int                                      `json:"before_pages"`
	AfterPages          int                                      `json:"after_pages"`
	ChangedPages        []uint32                                 `json:"changed_pages"`
	BeforeSemantics     layoutengine.ReviewSemanticSnapshot      `json:"before_semantics"`
	AfterSemantics      layoutengine.ReviewSemanticSnapshot      `json:"after_semantics"`
	BeforeAccessibility layoutengine.ReviewAccessibilitySnapshot `json:"before_accessibility"`
	AfterAccessibility  layoutengine.ReviewAccessibilitySnapshot `json:"after_accessibility"`
	ReadingOrderChanged bool                                     `json:"reading_order_changed"`
	TagStructureChanged bool                                     `json:"tag_structure_changed"`
}

// AgentChangesetReview is a bounded hash-first projection. Payloads are never
// serialized and are populated only for edit-capable non-restricted opens.
type AgentChangesetReview struct {
	SchemaVersion    uint16                         `json:"schema_version"`
	DisclosureDomain DisclosureDomain               `json:"disclosure_domain"`
	PolicyRevision   string                         `json:"policy_revision"`
	BeforeRevision   string                         `json:"before_revision"`
	AfterRevision    string                         `json:"after_revision"`
	SourceDiffHash   string                         `json:"source_diff_hash"`
	SemanticDiffHash string                         `json:"semantic_diff_hash"`
	ManifestHash     string                         `json:"manifest_hash"`
	EvidenceHash     string                         `json:"evidence_hash"`
	Patches          []ChangesetPatchEvidence       `json:"patches"`
	Layout           ChangesetLayoutEvidence        `json:"layout"`
	Diagnostics      ChangesetDiagnosticEvidence    `json:"diagnostics"`
	Artifacts        []ChangesetArtifactReference   `json:"artifacts"`
	Candidate        CandidateHandle                `json:"-"`
	BeforeHandle     RevisionHandle                 `json:"-"`
	AfterHandle      RevisionHandle                 `json:"-"`
	Payloads         []document.PaperReviewArtifact `json:"-"`
}

type changesetSourceProjection struct {
	BeforeRevision string                   `json:"before_revision"`
	AfterRevision  string                   `json:"after_revision"`
	Patches        []ChangesetPatchEvidence `json:"patches"`
}

func (w *Workspace) ReviewAgentChangeset(ctx context.Context, request AgentChangesetReviewRequest) (AgentChangesetReview, error) {
	if w == nil || ctx == nil {
		return AgentChangesetReview{}, workspaceError("INVALID_CHANGESET_REVIEW", "workspace and context are required", ErrInvalidQuery)
	}
	if err := ctx.Err(); err != nil {
		return AgentChangesetReview{}, err
	}
	if request.Review.MaxPages == 0 {
		request.Review = document.DefaultPaperReviewRequest()
	}
	entry, before, after, opened, err := w.exactChangesetTransition(request)
	if err != nil {
		return AgentChangesetReview{}, err
	}
	patches := redactedPatchEvidence(entry.Diff.Patches, opened.disclosure != DisclosureRestricted)
	sourceProjection := changesetSourceProjection{BeforeRevision: string(entry.BeforeRevision), AfterRevision: string(entry.AfterRevision), Patches: patches}
	redactedSource, err := json.Marshal(sourceProjection)
	if err != nil {
		return AgentChangesetReview{}, workspaceError("CHANGESET_SOURCE_PROJECTION", "source-diff projection cannot be encoded", ErrChangesetReview)
	}
	reviewRequest := clonePaperReviewRequest(request.Review)
	reviewRequest.SourceDiff = redactedSource
	beforePlan, _, err := w.CreatePlan(before.handle)
	if err != nil {
		return AgentChangesetReview{}, err
	}
	defer func() { _ = w.ReleasePlan(beforePlan.Handle) }()
	afterPlan, _, err := w.CreatePlan(after.handle)
	if err != nil {
		return AgentChangesetReview{}, err
	}
	defer func() { _ = w.ReleasePlan(afterPlan.Handle) }()
	bundle, err := w.ReviewPlans(ctx, PlanReviewRequest{Before: beforePlan.Handle, After: afterPlan.Handle, Review: reviewRequest})
	if err != nil {
		if errors.Is(err, layoutengine.ErrReviewBundleLimit) || errors.Is(err, ErrLimit) {
			return AgentChangesetReview{}, workspaceError("CHANGESET_REVIEW_LIMIT", "changeset review exceeds configured bounds", ErrLimit)
		}
		return AgentChangesetReview{}, err
	}
	var manifest layoutengine.ReviewBundleManifest
	if err := json.Unmarshal(bundle.ManifestJSON, &manifest); err != nil {
		return AgentChangesetReview{}, workspaceError("CHANGESET_MANIFEST", "review manifest cannot be decoded", ErrChangesetReview)
	}
	artifacts, semanticHash, err := changesetArtifactReferences(bundle.Artifacts)
	if err != nil {
		return AgentChangesetReview{}, err
	}
	result := AgentChangesetReview{SchemaVersion: 1, DisclosureDomain: opened.disclosure, PolicyRevision: w.policyRevision,
		BeforeRevision: string(before.revision), AfterRevision: string(after.revision),
		SourceDiffHash: hashCanonical("paperd/changeset-source/v1", entry.Diff), SemanticDiffHash: semanticHash,
		ManifestHash: hashCanonical("paperd/changeset-manifest/v1", bundle.ManifestJSON), Patches: patches, Artifacts: artifacts,
		Candidate: request.Candidate, BeforeHandle: before.handle, AfterHandle: after.handle,
		Layout: ChangesetLayoutEvidence{BeforePlanHash: beforePlan.Hash, AfterPlanHash: afterPlan.Hash,
			BeforePages: beforePlan.Pages, AfterPages: afterPlan.Pages, ChangedPages: append([]uint32(nil), manifest.ChangedPages...),
			BeforeSemantics: manifest.BeforeSemantics, AfterSemantics: manifest.AfterSemantics,
			BeforeAccessibility: manifest.BeforeAccessibility, AfterAccessibility: manifest.AfterAccessibility,
			ReadingOrderChanged: manifest.BeforeSemantics.ReadingSHA256 != manifest.AfterSemantics.ReadingSHA256,
			TagStructureChanged: manifest.BeforeSemantics.SHA256 != manifest.AfterSemantics.SHA256},
		Diagnostics: changesetDiagnostics(before, after)}
	result.EvidenceHash = changesetEvidenceHash(result)
	if request.IncludePayloads && opened.mode == CapabilityEdit && opened.disclosure != DisclosureRestricted {
		result.Payloads = cloneReviewArtifacts(bundle.Artifacts)
	}
	request.Review = reviewRequest
	request.Review.SourceDiff = nil
	if err := w.revalidateChangesetHead(request.Candidate, request.After, request.AfterRevision); err != nil {
		return AgentChangesetReview{}, err
	}
	return result, nil
}

func (w *Workspace) exactChangesetTransition(request AgentChangesetReviewRequest) (paperedit.JournalEntry, *revisionRecord, *revisionRecord, *openRecord, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	opened, err := w.openLocked(request.Open)
	if err != nil {
		return paperedit.JournalEntry{}, nil, nil, nil, err
	}
	if opened.candidate != request.Candidate || opened.revision != request.After || opened.digest != request.AfterRevision {
		return paperedit.JournalEntry{}, nil, nil, nil, workspaceError("CHANGESET_CAPABILITY", "open capability does not bind the exact candidate transition head", ErrRevisionConflict)
	}
	candidate, err := w.candidateLocked(request.Candidate)
	if err != nil {
		return paperedit.JournalEntry{}, nil, nil, nil, err
	}
	if candidate.head != request.After {
		return paperedit.JournalEntry{}, nil, nil, nil, workspaceError("REVISION_CONFLICT", "candidate head changed before review", ErrRevisionConflict)
	}
	before, err := w.revisionLocked(request.Before)
	if err != nil {
		return paperedit.JournalEntry{}, nil, nil, nil, err
	}
	after, err := w.revisionLocked(request.After)
	if err != nil {
		return paperedit.JournalEntry{}, nil, nil, nil, err
	}
	if before.file != after.file || before.revision != request.BeforeRevision || after.revision != request.AfterRevision {
		return paperedit.JournalEntry{}, nil, nil, nil, workspaceError("CHANGESET_REVISION", "transition revisions do not match retained source snapshots", ErrRevisionConflict)
	}
	var found *paperedit.JournalEntry
	for _, entry := range candidate.journal.Entries() {
		if entry.BeforeRevision == request.BeforeRevision && entry.AfterRevision == request.AfterRevision {
			if found != nil {
				return paperedit.JournalEntry{}, nil, nil, nil, workspaceError("AMBIGUOUS_CHANGESET", "candidate journal contains an ambiguous transition", ErrRevisionConflict)
			}
			copy := entry
			found = &copy
		}
	}
	if found == nil {
		return paperedit.JournalEntry{}, nil, nil, nil, workspaceError("CHANGESET_NOT_FOUND", "exact transition is not retained in candidate history", ErrRevisionNotFound)
	}
	return *found, before, after, opened, nil
}

func redactedPatchEvidence(patches []paperedit.SourcePatch, includeTarget bool) []ChangesetPatchEvidence {
	result := make([]ChangesetPatchEvidence, len(patches))
	for index, patch := range patches {
		target := ""
		if includeTarget {
			target = patch.Target
		}
		result[index] = ChangesetPatchEvidence{Index: uint32(index), Target: target, TargetSHA256: hashCanonical("paperd/changeset-target/v1", patch.Target), Start: patch.Start, End: patch.End,
			RemovedBytes: len(patch.Removed), ReplacementBytes: len(patch.Replacement),
			RemovedSHA256: hashCanonical("paperd/changeset-removed/v1", patch.Removed), ReplacementSHA256: hashCanonical("paperd/changeset-replacement/v1", patch.Replacement)}
	}
	return result
}

func changesetArtifactReferences(artifacts []document.PaperReviewArtifact) ([]ChangesetArtifactReference, string, error) {
	result := make([]ChangesetArtifactReference, len(artifacts))
	semanticHash := ""
	for index, artifact := range artifacts {
		var metadata struct {
			Kind string `json:"kind"`
			Page uint32 `json:"page"`
		}
		if err := json.Unmarshal(artifact.MetadataJSON, &metadata); err != nil || !knownHeadlessArtifactKind(metadata.Kind) {
			return nil, "", workspaceError("CHANGESET_ARTIFACT", "review artifact metadata is invalid", ErrChangesetReview)
		}
		result[index] = ChangesetArtifactReference{Kind: metadata.Kind,
			SHA256: hashCanonical("paperd/changeset-artifact/v1", struct {
				Kind    string
				Payload []byte
			}{metadata.Kind, artifact.Bytes}),
			Bytes: len(artifact.Bytes), Page: metadata.Page}
		if metadata.Kind == "semantic_diff" {
			semanticHash = result[index].SHA256
		}
	}
	if semanticHash == "" {
		return nil, "", workspaceError("CHANGESET_ARTIFACT", "semantic-diff evidence is missing", ErrChangesetReview)
	}
	return result, semanticHash, nil
}

func changesetDiagnostics(before, after *revisionRecord) ChangesetDiagnosticEvidence {
	beforeDiagnostics := struct {
		Parse   any `json:"parse"`
		Compile any `json:"compile"`
	}{before.parsed.Diagnostics, before.compiled.Diagnostics}
	afterDiagnostics := struct {
		Parse   any `json:"parse"`
		Compile any `json:"compile"`
	}{after.parsed.Diagnostics, after.compiled.Diagnostics}
	return ChangesetDiagnosticEvidence{BeforeCount: len(before.parsed.Diagnostics) + len(before.compiled.Diagnostics),
		AfterCount:   len(after.parsed.Diagnostics) + len(after.compiled.Diagnostics),
		BeforeSHA256: hashCanonical("paperd/changeset-diagnostics/v1", beforeDiagnostics),
		AfterSHA256:  hashCanonical("paperd/changeset-diagnostics/v1", afterDiagnostics)}
}

func changesetEvidenceHash(review AgentChangesetReview) string {
	return hashCanonical("paperd/agent-changeset/v1", struct {
		Disclosure, Policy, Before, After, Source, Semantic, Manifest string
		Patches                                                       []ChangesetPatchEvidence
		Layout                                                        ChangesetLayoutEvidence
		Diagnostics                                                   ChangesetDiagnosticEvidence
		Artifacts                                                     []ChangesetArtifactReference
	}{string(review.DisclosureDomain), review.PolicyRevision, review.BeforeRevision, review.AfterRevision, review.SourceDiffHash, review.SemanticDiffHash, review.ManifestHash,
		review.Patches, review.Layout, review.Diagnostics, review.Artifacts})
}

func cloneReviewArtifacts(values []document.PaperReviewArtifact) []document.PaperReviewArtifact {
	result := make([]document.PaperReviewArtifact, len(values))
	for index, value := range values {
		result[index] = document.PaperReviewArtifact{MetadataJSON: append([]byte(nil), value.MetadataJSON...), Bytes: append([]byte(nil), value.Bytes...)}
	}
	return result
}

func (w *Workspace) revalidateChangesetHead(candidateHandle CandidateHandle, head RevisionHandle, digest paperedit.Revision) error {
	w.mu.RLock()
	defer w.mu.RUnlock()
	candidate, err := w.candidateLocked(candidateHandle)
	if err != nil {
		return err
	}
	if candidate.head != head {
		return workspaceError("REVISION_CONFLICT", "candidate head changed while review evidence was generated", ErrRevisionConflict)
	}
	revision, err := w.revisionLocked(head)
	if err != nil {
		return err
	}
	if revision.revision != digest {
		return workspaceError("REVISION_CONFLICT", "candidate digest changed while review evidence was generated", ErrRevisionConflict)
	}
	return nil
}

func (review AgentChangesetReview) CanonicalJSON(maxBytes int) ([]byte, error) {
	if maxBytes < 1 || maxBytes > MaxContextBytesHard || !validSHA256(review.EvidenceHash) || review.EvidenceHash != changesetEvidenceHash(review) {
		return nil, workspaceError("INVALID_CHANGESET_SUMMARY", "changeset projection or response bound is invalid", ErrChangesetReview)
	}
	payload, err := json.Marshal(struct {
		SchemaVersion    uint16                       `json:"schema_version"`
		DisclosureDomain DisclosureDomain             `json:"disclosure_domain"`
		PolicyRevision   string                       `json:"policy_revision"`
		BeforeRevision   string                       `json:"before_revision"`
		AfterRevision    string                       `json:"after_revision"`
		SourceDiffHash   string                       `json:"source_diff_hash"`
		SemanticDiffHash string                       `json:"semantic_diff_hash"`
		ManifestHash     string                       `json:"manifest_hash"`
		EvidenceHash     string                       `json:"evidence_hash"`
		Patches          []ChangesetPatchEvidence     `json:"patches"`
		Layout           ChangesetLayoutEvidence      `json:"layout"`
		Diagnostics      ChangesetDiagnosticEvidence  `json:"diagnostics"`
		Artifacts        []ChangesetArtifactReference `json:"artifacts"`
	}{review.SchemaVersion, review.DisclosureDomain, review.PolicyRevision, review.BeforeRevision, review.AfterRevision, review.SourceDiffHash,
		review.SemanticDiffHash, review.ManifestHash, review.EvidenceHash, review.Patches, review.Layout, review.Diagnostics, review.Artifacts})
	if err != nil || len(payload) > maxBytes {
		return nil, workspaceError("CHANGESET_RESPONSE_LIMIT", "changeset summary exceeds its response bound", ErrLimit)
	}
	return payload, nil
}

type AgentChangesetAcceptanceRequest struct {
	ReviewRequest        AgentChangesetReviewRequest
	ExpectedEvidenceHash string
	ExpectedManifestHash string
	Actor                string
	IdempotencyKey       string
	Scenarios            []ScenarioAcceptanceEvidence
	Validators           []ValidatorAcceptanceEvidence
	SelectedArtifacts    []ReviewAcceptanceEvidence
	ApprovalNonce        string
	ApprovalTTL          time.Duration
}

// AcceptAgentChangeset regenerates the exact transition bundle, verifies the
// caller's selected artifact subset, and delegates commit to the existing
// policy-bound one-use candidate acceptance path.
func (w *Workspace) AcceptAgentChangeset(ctx context.Context, request AgentChangesetAcceptanceRequest) (CandidateAcceptanceReceipt, error) {
	if ctx == nil {
		return CandidateAcceptanceReceipt{}, workspaceError("INVALID_CONTEXT", "acceptance context is nil", ErrInvalidQuery)
	}
	verified, err := w.ReviewAgentChangeset(ctx, request.ReviewRequest)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	if request.ExpectedEvidenceHash != verified.EvidenceHash || request.ExpectedManifestHash != verified.ManifestHash {
		return CandidateAcceptanceReceipt{}, workspaceError("CHANGESET_EVIDENCE_MISMATCH", "changeset approval does not match regenerated evidence", ErrCandidateAcceptanceDenied)
	}
	available := make(map[string]map[string]bool)
	add := func(kind, hash string) {
		if available[kind] == nil {
			available[kind] = make(map[string]bool)
		}
		available[kind][hash] = true
	}
	add("changeset_bundle", verified.EvidenceHash)
	for _, artifact := range verified.Artifacts {
		add(artifact.Kind, artifact.SHA256)
	}
	for _, selected := range request.SelectedArtifacts {
		if !selected.Approved || !available[selected.Kind][selected.Hash] {
			return CandidateAcceptanceReceipt{}, workspaceError("CHANGESET_SELECTION_MISMATCH", "selected review evidence is absent or unapproved", ErrCandidateAcceptanceDenied)
		}
	}
	opened, err := w.PaperOpen(PaperOpenRequest{Candidate: verified.Candidate, Revision: verified.AfterHandle,
		ExpectedDigest: paperedit.Revision(verified.AfterRevision), Mode: CapabilityEdit})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	authority, err := w.GrantSensitiveAuthority(SensitiveAuthorityGrant{Open: opened.Handle, Actor: request.Actor, Operation: SensitiveAccept})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	acceptance := CandidateAcceptanceRequest{Candidate: verified.Candidate, ExpectedHead: verified.AfterHandle,
		ExpectedRevision: paperedit.Revision(verified.AfterRevision), IdempotencyKey: request.IdempotencyKey,
		Scenarios: append([]ScenarioAcceptanceEvidence(nil), request.Scenarios...), Validators: append([]ValidatorAcceptanceEvidence(nil), request.Validators...),
		ReviewArtifacts: append([]ReviewAcceptanceEvidence(nil), request.SelectedArtifacts...)}
	inputHash, err := w.CandidateAcceptanceInputHash(acceptance)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	required := make([]string, len(request.Scenarios))
	validators := make([]ValidatorEvidence, len(request.Validators))
	reviews := make([]ReviewArtifactEvidence, len(request.SelectedArtifacts))
	for index, value := range request.Scenarios {
		required[index] = value.Name
	}
	for index, value := range request.Validators {
		validators[index] = ValidatorEvidence{Profile: value.Profile, Version: value.Version, Hash: value.Hash}
	}
	for index, value := range request.SelectedArtifacts {
		reviews[index] = ReviewArtifactEvidence{Kind: value.Kind, Hash: value.Hash}
	}
	sort.Strings(required)
	sort.Slice(validators, func(i, j int) bool {
		return validators[i].Profile+"\x00"+validators[i].Version < validators[j].Profile+"\x00"+validators[j].Version
	})
	sort.Slice(reviews, func(i, j int) bool { return reviews[i].Kind < reviews[j].Kind })
	evidence := SensitiveEvidence{CandidateRevision: verified.AfterRevision, SourceDiffHash: verified.SourceDiffHash,
		SemanticDiffHash: verified.SemanticDiffHash, OperationInputHash: inputHash, RequiredScenarios: required,
		Validators: validators, ReviewArtifacts: reviews}
	approval, err := w.GrantSensitiveApproval(SensitiveApprovalGrant{Authority: authority.Handle, ExpectedHead: verified.AfterHandle,
		PolicyRevision: w.policyRevision, Evidence: evidence, Nonce: request.ApprovalNonce, TTL: request.ApprovalTTL})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	acceptance.Authorization = SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle,
		Operation: SensitiveAccept, ExpectedHead: verified.AfterHandle, PolicyRevision: w.policyRevision, Evidence: evidence}
	return w.AcceptCandidate(ctx, acceptance)
}
