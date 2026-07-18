// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

var ErrHeadlessWorkflow = errors.New("paperd: headless workflow failed")

// HeadlessLiteralRequest is the smallest complete document-creation
// transaction exposed by the transport-independent agent workflow. Source is
// accepted only at creation; every later mutation is an exact semantic patch.
type HeadlessLiteralRequest struct {
	File           string
	Source         string
	Target         string
	Literal        string
	Actor          string
	IdempotencyKey string
	ProtectedNodes []string
}

// HeadlessCandidate contains opaque capabilities and non-secret hashes only.
// In particular, it never serializes the authored source or replacement text.
type HeadlessCandidate struct {
	Candidate        CandidateHandle `json:"-"`
	BaseRevision     RevisionHandle  `json:"-"`
	HeadRevision     RevisionHandle  `json:"-"`
	BasePlan         PlanHandle      `json:"-"`
	HeadPlan         PlanHandle      `json:"-"`
	File             string          `json:"-"`
	Target           string          `json:"target"`
	BaseDigest       string          `json:"base_digest"`
	HeadDigest       string          `json:"head_digest"`
	BasePlanHash     string          `json:"base_plan_hash"`
	HeadPlanHash     string          `json:"head_plan_hash"`
	SourceDiffHash   string          `json:"source_diff_hash"`
	SemanticDiffHash string          `json:"semantic_diff_hash"`
	PatchCount       int             `json:"patch_count"`
	Applied          bool            `json:"applied"`
}

// BeginHeadlessLiteralWorkflow creates a candidate, opens it with an exact edit
// capability, grants one actor/target-scoped authority, and applies one minimal
// set_literal transaction. It retains immutable plans before and after the
// edit so review never reparses a caller-supplied source snapshot.
func (w *Workspace) BeginHeadlessLiteralWorkflow(ctx context.Context, request HeadlessLiteralRequest) (HeadlessCandidate, error) {
	if w == nil {
		return HeadlessCandidate{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if ctx == nil {
		return HeadlessCandidate{}, workspaceError("INVALID_CONTEXT", "context is nil", ErrInvalidQuery)
	}
	if err := ctx.Err(); err != nil {
		return HeadlessCandidate{}, err
	}
	if request.Target == "" || len(request.Target) > w.limits.MaxQueryBytes || !utf8.ValidString(request.Target) ||
		request.Actor == "" || len(request.Actor) > w.limits.MaxQueryBytes || !utf8.ValidString(request.Actor) {
		return HeadlessCandidate{}, workspaceError("INVALID_HEADLESS_REQUEST", "actor and target must be bounded valid UTF-8", ErrInvalidQuery)
	}
	created, err := w.PaperCreate(PaperCreateRequest{File: request.File, Source: request.Source})
	if err != nil {
		return HeadlessCandidate{}, err
	}
	partial := HeadlessCandidate{Candidate: created.Candidate.Handle, BaseRevision: created.Revision.Handle,
		HeadRevision: created.Revision.Handle, File: created.Revision.File, Target: request.Target,
		BaseDigest: string(created.Revision.Revision), HeadDigest: string(created.Revision.Revision)}
	if !created.Revision.ParseOK || !created.Revision.CompileOK {
		return partial, workspaceError("INVALID_SOURCE", "headless workflow source must parse and compile before planning", ErrInvalidSource)
	}
	if err := ctx.Err(); err != nil {
		return partial, err
	}
	opened, err := w.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: CapabilityEdit})
	if err != nil {
		return partial, err
	}
	authority, err := w.GrantMutationAuthority(MutationAuthorityGrant{Open: opened.Handle, Actor: request.Actor,
		Operations: []MutationOperation{MutationSetLiteral}, NodeScopes: []string{request.Target}, ProtectedNodes: append([]string(nil), request.ProtectedNodes...)})
	if err != nil {
		return partial, err
	}
	fingerprint, err := paperedit.FingerprintNode(request.File, request.Source, request.Target)
	if err != nil {
		return partial, workspaceError("INVALID_HEADLESS_TARGET", "literal target is absent or ambiguous", err)
	}
	instance, err := paperedit.SourceInstance(request.File, request.Source, request.Target)
	if err != nil {
		return partial, workspaceError("INVALID_HEADLESS_TARGET", "literal target is not an exact source instance", err)
	}
	basePlan, _, err := w.CreatePlan(created.Revision.Handle)
	if err != nil {
		return partial, err
	}
	partial.BasePlan, partial.BasePlanHash = basePlan.Handle, basePlan.Hash
	if err := ctx.Err(); err != nil {
		return partial, err
	}
	mutation, err := w.PaperSetLiteral(PaperSetLiteralRequest{Guard: PaperMutationGuard{
		Open: opened.Handle, Authority: authority.Handle, Candidate: created.Candidate.Handle,
		ExpectedHead: created.Revision.Handle, ExpectedDigest: created.Revision.Revision,
		Target: request.Target, ExpectedFingerprint: fingerprint, ExpectedInstance: instance,
		IdempotencyKey: request.IdempotencyKey,
	}, Text: request.Literal})
	if err != nil {
		return partial, err
	}
	partial.HeadRevision = mutation.Revision.Handle
	partial.HeadDigest = string(mutation.Revision.Revision)
	partial.Applied = mutation.Edit.Applied
	if mutation.Edit.Diff == nil || len(mutation.Edit.Diff.Patches) != 1 || mutation.Edit.Diff.Patches[0].Target == "" {
		return partial, workspaceError("HEADLESS_EDIT_NOT_MINIMAL", "headless literal workflow requires exactly one source patch", ErrHeadlessWorkflow)
	}
	partial.PatchCount = 1
	partial.SourceDiffHash = hashCanonical("paperd/headless-source-diff/v1", mutation.Edit.Diff)
	// Recovery uses the most specific exact source node changed by the
	// minimal patch, even when the initial convenience request selected its
	// paragraph/heading container.
	partial.Target = mutation.Edit.Diff.Patches[0].Target
	partial.SemanticDiffHash = headlessSemanticHash(partial.Target, mutation.Edit.Diff)
	if err := ctx.Err(); err != nil {
		return partial, err
	}
	headPlan, _, err := w.CreatePlan(mutation.Revision.Handle)
	if err != nil {
		return partial, err
	}
	partial.HeadPlan, partial.HeadPlanHash = headPlan.Handle, headPlan.Hash
	return partial, nil
}

type HeadlessRecoveryRequest struct {
	File             string `json:"file"`
	BaseDigest       string `json:"base_digest"`
	HeadDigest       string `json:"head_digest"`
	Target           string `json:"target"`
	SourceDiffHash   string `json:"source_diff_hash"`
	SemanticDiffHash string `json:"semantic_diff_hash"`
	PatchCount       int    `json:"patch_count"`
}

// RecoverHeadlessCandidate deterministically locates one persisted candidate
// by exact file/base/head digests and issues fresh transient plan handles.
// Ambiguous candidates are rejected rather than guessed.
func (w *Workspace) RecoverHeadlessCandidate(ctx context.Context, request HeadlessRecoveryRequest) (HeadlessCandidate, error) {
	if w == nil || ctx == nil {
		return HeadlessCandidate{}, workspaceError("INVALID_HEADLESS_RECOVERY", "workspace and context are required", ErrInvalidQuery)
	}
	if err := ctx.Err(); err != nil {
		return HeadlessCandidate{}, err
	}
	if request.File == "" || !validSHA256(request.BaseDigest) || !validSHA256(request.HeadDigest) ||
		!validSHA256(request.SourceDiffHash) || !validSHA256(request.SemanticDiffHash) || request.PatchCount < 1 {
		return HeadlessCandidate{}, workspaceError("INVALID_HEADLESS_RECOVERY", "recovery requires exact bounded workflow identities", ErrInvalidQuery)
	}
	w.mu.RLock()
	var base, head *revisionRecord
	var candidate *candidateRecord
	for _, record := range w.revisions {
		if record.file != request.File {
			continue
		}
		if string(record.revision) == request.BaseDigest {
			if base != nil {
				w.mu.RUnlock()
				return HeadlessCandidate{}, workspaceError("AMBIGUOUS_RECOVERY", "base revision identity is ambiguous", ErrRevisionConflict)
			}
			base = record
		}
		if string(record.revision) == request.HeadDigest {
			if head != nil {
				w.mu.RUnlock()
				return HeadlessCandidate{}, workspaceError("AMBIGUOUS_RECOVERY", "head revision identity is ambiguous", ErrRevisionConflict)
			}
			head = record
		}
	}
	if head != nil {
		for _, record := range w.candidates {
			if record.head == head.handle {
				if candidate != nil {
					w.mu.RUnlock()
					return HeadlessCandidate{}, workspaceError("AMBIGUOUS_RECOVERY", "candidate identity is ambiguous", ErrRevisionConflict)
				}
				candidate = record
			}
		}
	}
	if base == nil || head == nil || candidate == nil {
		w.mu.RUnlock()
		return HeadlessCandidate{}, workspaceError("RECOVERY_NOT_FOUND", "exact persisted workflow state is unavailable", ErrRevisionNotFound)
	}
	baseHandle, headHandle, candidateHandle := base.handle, head.handle, candidate.handle
	baseSource, headSource := base.source, head.source
	w.mu.RUnlock()
	// Reconstruct the semantic operation from authenticated persisted source
	// bytes. Caller-provided hashes, patch counts, and targets are assertions,
	// never recovery authority.
	headNode := findNodeByID(head.parsed.AST.Root, request.Target)
	literal, err := headlessLiteralText(headNode)
	if err != nil {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_TARGET", "recovered target is not one exact literal representation", err)
	}
	baseNode := findNodeByID(base.parsed.AST.Root, request.Target)
	operation, _, err := literalOperation(baseNode, request.Target, literal)
	if err != nil {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_TARGET", "recovered base target is not the same literal representation", err)
	}
	fingerprint, err := paperedit.FingerprintNode(request.File, baseSource, request.Target)
	if err != nil {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_TARGET", "recovered target fingerprint is unavailable", err)
	}
	instance, err := paperedit.SourceInstance(request.File, baseSource, request.Target)
	if err != nil {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_TARGET", "recovered source instance is unavailable", err)
	}
	replayed, err := paperedit.Apply(paperedit.Transaction{File: request.File, Source: baseSource,
		ExpectedRevision: paperedit.Revision(request.BaseDigest), RequireExactTargets: true,
		TargetPreconditions: []paperedit.TargetPrecondition{{Target: request.Target, ExpectedFingerprint: fingerprint, ExpectedInstance: instance}},
		Operations:          []paperedit.Operation{operation}})
	if err != nil || !replayed.Applied || replayed.Source != headSource || replayed.Diff == nil || len(replayed.Diff.Patches) != 1 || replayed.Diff.Patches[0].Target != request.Target {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_MISMATCH", "persisted head is not the exact minimal literal edit claimed by recovery", errors.Join(ErrRevisionConflict, err))
	}
	sourceHash := hashCanonical("paperd/headless-source-diff/v1", replayed.Diff)
	semanticHash := headlessSemanticHash(request.Target, replayed.Diff)
	if request.PatchCount != len(replayed.Diff.Patches) || request.SourceDiffHash != sourceHash || request.SemanticDiffHash != semanticHash {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_EVIDENCE", "recovery evidence does not match reconstructed persisted edit", ErrRevisionConflict)
	}
	basePlan, _, err := w.CreatePlan(baseHandle)
	if err != nil {
		return HeadlessCandidate{}, err
	}
	headPlan, _, err := w.CreatePlan(headHandle)
	if err != nil {
		return HeadlessCandidate{}, err
	}
	current, err := w.Candidate(candidateHandle)
	if err != nil || current.Head != headHandle {
		return HeadlessCandidate{}, workspaceError("HEADLESS_RECOVERY_RACE", "candidate head changed while recovery evidence was rebuilt", errors.Join(ErrRevisionConflict, err))
	}
	return HeadlessCandidate{Candidate: candidateHandle, BaseRevision: baseHandle, HeadRevision: headHandle,
		BasePlan: basePlan.Handle, HeadPlan: headPlan.Handle, File: request.File, Target: request.Target,
		BaseDigest: request.BaseDigest, HeadDigest: request.HeadDigest, BasePlanHash: basePlan.Hash,
		HeadPlanHash: headPlan.Hash, SourceDiffHash: sourceHash, SemanticDiffHash: semanticHash,
		PatchCount: len(replayed.Diff.Patches), Applied: true}, nil
}

type HeadlessReviewRequest struct {
	Candidate       HeadlessCandidate
	Selectors       []document.PaperPlanSelector
	MaxExplainBytes uint32
	Review          document.PaperReviewRequest
}

type HeadlessArtifactEvidence struct {
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type HeadlessReview struct {
	Candidate           HeadlessCandidate          `json:"candidate"`
	ExplainSHA256       string                     `json:"explain_sha256"`
	ExplainBytes        int                        `json:"explain_bytes"`
	ReviewManifestHash  string                     `json:"review_manifest_hash"`
	ReviewManifestBytes int                        `json:"review_manifest_bytes"`
	Artifacts           []HeadlessArtifactEvidence `json:"artifacts"`
	Bundle              PlanReviewResult           `json:"-"`
	selectors           []document.PaperPlanSelector
	maxExplainBytes     uint32
	reviewRequest       document.PaperReviewRequest
}

// ReviewHeadlessCandidate returns structural explanation plus a bounded visual
// review bundle. JSON protocol summaries contain hashes and sizes; artifact
// payloads remain on the explicitly disclosure-bound in-process result.
func (w *Workspace) ReviewHeadlessCandidate(ctx context.Context, request HeadlessReviewRequest) (HeadlessReview, error) {
	if ctx == nil {
		return HeadlessReview{}, workspaceError("INVALID_CONTEXT", "context is nil", ErrInvalidQuery)
	}
	if err := w.validateHeadlessCandidate(request.Candidate); err != nil {
		return HeadlessReview{}, err
	}
	if len(request.Selectors) == 0 {
		request.Selectors = []document.PaperPlanSelector{{Key: request.Candidate.Target, MaxResults: 16}}
	}
	if request.MaxExplainBytes == 0 {
		request.MaxExplainBytes = uint32(min(w.limits.MaxRenderBytes, 1<<20)) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	}
	explanation, err := w.ExplainPlan(PlanExplainRequest{Plan: request.Candidate.HeadPlan, Selectors: request.Selectors, MaxBytes: request.MaxExplainBytes})
	if err != nil {
		return HeadlessReview{}, err
	}
	if err := ctx.Err(); err != nil {
		return HeadlessReview{}, err
	}
	if request.Review.MaxPages == 0 {
		request.Review = document.DefaultPaperReviewRequest()
	}
	bundle, err := w.ReviewPlans(ctx, PlanReviewRequest{Before: request.Candidate.BasePlan, After: request.Candidate.HeadPlan, Review: request.Review})
	if err != nil {
		return HeadlessReview{}, err
	}
	result := HeadlessReview{Candidate: request.Candidate, ExplainSHA256: hashBytes(explanation.JSON), ExplainBytes: len(explanation.JSON),
		ReviewManifestHash: hashBytes(bundle.ManifestJSON), ReviewManifestBytes: len(bundle.ManifestJSON), Bundle: bundle,
		Artifacts: make([]HeadlessArtifactEvidence, len(bundle.Artifacts))}
	for index, artifact := range bundle.Artifacts {
		var metadata struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(artifact.MetadataJSON, &metadata); err != nil || metadata.Kind == "" {
			return HeadlessReview{}, workspaceError("INVALID_REVIEW_ARTIFACT", "review artifact metadata is invalid", ErrHeadlessWorkflow)
		}
		result.Artifacts[index] = HeadlessArtifactEvidence{Kind: metadata.Kind, SHA256: hashBytes(artifact.Bytes), Bytes: len(artifact.Bytes)}
	}
	result.selectors = append([]document.PaperPlanSelector(nil), request.Selectors...)
	result.maxExplainBytes = request.MaxExplainBytes
	result.reviewRequest = clonePaperReviewRequest(request.Review)
	return result, nil
}

type HeadlessExportRequest struct {
	Review        HeadlessReview
	Acceptance    CandidateAcceptanceReceipt
	Actor         string
	Target        string
	ApprovalNonce string
	ApprovalTTL   time.Duration
}

type HeadlessAcceptanceRequest struct {
	Review          HeadlessReview
	Actor           string
	IdempotencyKey  string
	Scenarios       []ScenarioAcceptanceEvidence
	Validators      []ValidatorAcceptanceEvidence
	ReviewArtifacts []ReviewAcceptanceEvidence
	ApprovalNonce   string
	ApprovalTTL     time.Duration
}

// AcceptHeadlessCandidate converts exact review/scenario/validator evidence
// into the workspace's configured acceptance gate. The outer caller still
// chooses which reviewed artifacts it approves; this adapter only proves that
// every supplied hash came from this exact bounded workflow result.
func (w *Workspace) AcceptHeadlessCandidate(ctx context.Context, request HeadlessAcceptanceRequest) (CandidateAcceptanceReceipt, error) {
	if ctx == nil {
		return CandidateAcceptanceReceipt{}, workspaceError("INVALID_CONTEXT", "context is nil", ErrInvalidQuery)
	}
	if err := w.validateHeadlessCandidate(request.Review.Candidate); err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	verified, err := w.ReviewHeadlessCandidate(ctx, HeadlessReviewRequest{Candidate: request.Review.Candidate,
		Selectors: append([]document.PaperPlanSelector(nil), request.Review.selectors...), MaxExplainBytes: request.Review.maxExplainBytes,
		Review: clonePaperReviewRequest(request.Review.reviewRequest)})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	if request.Review.ExplainSHA256 != verified.ExplainSHA256 || request.Review.ExplainBytes != verified.ExplainBytes ||
		request.Review.ReviewManifestHash != verified.ReviewManifestHash || request.Review.ReviewManifestBytes != verified.ReviewManifestBytes ||
		!equalHeadlessArtifacts(request.Review.Artifacts, verified.Artifacts) {
		return CandidateAcceptanceReceipt{}, workspaceError("REVIEW_EVIDENCE_MISMATCH", "acceptance review summary differs from regenerated retained-plan evidence", ErrCandidateAcceptanceDenied)
	}
	request.Review = verified
	available := make(map[string]map[string]struct{}, len(request.Review.Artifacts)+2)
	addAvailable := func(kind, hash string) {
		if available[kind] == nil {
			available[kind] = make(map[string]struct{})
		}
		available[kind][hash] = struct{}{}
	}
	addAvailable("review_manifest", request.Review.ReviewManifestHash)
	addAvailable("layout_explanation", request.Review.ExplainSHA256)
	for _, artifact := range request.Review.Artifacts {
		addAvailable(artifact.Kind, artifact.SHA256)
	}
	for _, artifact := range request.ReviewArtifacts {
		if _, ok := available[artifact.Kind][artifact.Hash]; !ok {
			return CandidateAcceptanceReceipt{}, workspaceError("REVIEW_EVIDENCE_MISMATCH", "acceptance review evidence was not produced by this workflow", ErrCandidateAcceptanceDenied)
		}
	}
	opened, err := w.PaperOpen(PaperOpenRequest{Candidate: request.Review.Candidate.Candidate,
		Revision: request.Review.Candidate.HeadRevision, ExpectedDigest: paperedit.Revision(request.Review.Candidate.HeadDigest), Mode: CapabilityEdit})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	authority, err := w.GrantSensitiveAuthority(SensitiveAuthorityGrant{Open: opened.Handle, Actor: request.Actor, Operation: SensitiveAccept})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	acceptance := CandidateAcceptanceRequest{Candidate: request.Review.Candidate.Candidate,
		ExpectedHead: request.Review.Candidate.HeadRevision, ExpectedRevision: paperedit.Revision(request.Review.Candidate.HeadDigest),
		IdempotencyKey: request.IdempotencyKey, Scenarios: append([]ScenarioAcceptanceEvidence(nil), request.Scenarios...),
		Validators: append([]ValidatorAcceptanceEvidence(nil), request.Validators...), ReviewArtifacts: append([]ReviewAcceptanceEvidence(nil), request.ReviewArtifacts...)}
	inputHash, err := w.CandidateAcceptanceInputHash(acceptance)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	requiredScenarios := make([]string, len(request.Scenarios))
	validators := make([]ValidatorEvidence, len(request.Validators))
	reviews := make([]ReviewArtifactEvidence, len(request.ReviewArtifacts))
	for index, scenario := range request.Scenarios {
		requiredScenarios[index] = scenario.Name
	}
	for index, validator := range request.Validators {
		validators[index] = ValidatorEvidence{Profile: validator.Profile, Version: validator.Version, Hash: validator.Hash}
	}
	for index, artifact := range request.ReviewArtifacts {
		reviews[index] = ReviewArtifactEvidence{Kind: artifact.Kind, Hash: artifact.Hash}
	}
	evidence := SensitiveEvidence{CandidateRevision: request.Review.Candidate.HeadDigest,
		SourceDiffHash: request.Review.Candidate.SourceDiffHash, SemanticDiffHash: request.Review.Candidate.SemanticDiffHash,
		OperationInputHash: inputHash, RequiredScenarios: requiredScenarios, Validators: validators, ReviewArtifacts: reviews}
	approval, err := w.GrantSensitiveApproval(SensitiveApprovalGrant{Authority: authority.Handle,
		ExpectedHead: request.Review.Candidate.HeadRevision, PolicyRevision: w.policyRevision, Evidence: evidence,
		Nonce: request.ApprovalNonce, TTL: request.ApprovalTTL})
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	acceptance.Authorization = SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle,
		Operation: SensitiveAccept, ExpectedHead: request.Review.Candidate.HeadRevision, PolicyRevision: w.policyRevision, Evidence: evidence}
	return w.AcceptCandidate(ctx, acceptance)
}

type HeadlessExport struct {
	CandidateDigest string                    `json:"candidate_digest"`
	PlanHash        string                    `json:"plan_hash"`
	PDFSHA256       string                    `json:"pdf_sha256"`
	PDFBytes        int                       `json:"pdf_bytes"`
	InputHash       string                    `json:"input_hash"`
	Approval        SensitiveApprovalSnapshot `json:"approval"`
	Authorization   SensitiveOperationRequest `json:"-"`
	Input           SensitiveOperationInput   `json:"-"`
}

// PrepareHeadlessExport renders the exact reviewed plan and creates a one-use
// export approval bound to candidate, source/semantic diffs, required
// scenarios, validators, review artifacts, policy revision, and output bytes.
func (w *Workspace) PrepareHeadlessExport(request HeadlessExportRequest) (HeadlessExport, error) {
	if err := w.validateHeadlessCandidate(request.Review.Candidate); err != nil {
		return HeadlessExport{}, err
	}
	accepted, err := w.CandidateAcceptance(request.Review.Candidate.Candidate)
	if err != nil {
		return HeadlessExport{}, err
	}
	if request.Acceptance.AcceptanceHash == "" || request.Acceptance.AcceptanceHash != accepted.AcceptanceHash ||
		accepted.CandidateRevision != request.Review.Candidate.HeadDigest {
		return HeadlessExport{}, workspaceError("ACCEPTANCE_EVIDENCE_MISMATCH", "export requires the committed acceptance for the exact candidate head", ErrCandidateAcceptanceDenied)
	}
	opened, err := w.PaperOpen(PaperOpenRequest{Candidate: request.Review.Candidate.Candidate,
		Revision: request.Review.Candidate.HeadRevision, ExpectedDigest: paperedit.Revision(request.Review.Candidate.HeadDigest), Mode: CapabilityEdit})
	if err != nil {
		return HeadlessExport{}, err
	}
	authority, err := w.GrantSensitiveAuthority(SensitiveAuthorityGrant{Open: opened.Handle, Actor: request.Actor, Operation: SensitiveExport})
	if err != nil {
		return HeadlessExport{}, err
	}
	rendered, err := w.RenderPlan(request.Review.Candidate.HeadPlan)
	if err != nil {
		return HeadlessExport{}, err
	}
	input := SensitiveOperationInput{Target: request.Target, MediaType: "application/pdf", Payload: append([]byte(nil), rendered.PDF...)}
	inputHash, err := SensitiveOperationInputHash(SensitiveExport, input)
	if err != nil {
		return HeadlessExport{}, err
	}
	requiredScenarios := make([]string, len(accepted.ScenarioResults))
	validators := make([]ValidatorEvidence, len(accepted.Validators))
	artifacts := make([]ReviewArtifactEvidence, 0, len(accepted.ReviewArtifacts)+1)
	artifacts = append(artifacts, ReviewArtifactEvidence{Kind: "candidate_acceptance", Hash: accepted.AcceptanceHash})
	for index, scenario := range accepted.ScenarioResults {
		if !scenario.Passed {
			return HeadlessExport{}, workspaceError("ACCEPTANCE_EVIDENCE_MISMATCH", "accepted scenario evidence is not passing", ErrCandidateAcceptanceDenied)
		}
		requiredScenarios[index] = scenario.Name
	}
	for index, validator := range accepted.Validators {
		if !validator.Passed {
			return HeadlessExport{}, workspaceError("ACCEPTANCE_EVIDENCE_MISMATCH", "accepted validator evidence is not passing", ErrCandidateAcceptanceDenied)
		}
		validators[index] = ValidatorEvidence{Profile: validator.Profile, Version: validator.Version, Hash: validator.Hash}
	}
	for _, artifact := range accepted.ReviewArtifacts {
		if !artifact.Approved {
			return HeadlessExport{}, workspaceError("ACCEPTANCE_EVIDENCE_MISMATCH", "accepted review evidence is not approved", ErrCandidateAcceptanceDenied)
		}
		artifacts = append(artifacts, ReviewArtifactEvidence{Kind: artifact.Kind, Hash: artifact.Hash})
	}
	evidence := SensitiveEvidence{CandidateRevision: request.Review.Candidate.HeadDigest,
		SourceDiffHash: request.Review.Candidate.SourceDiffHash, SemanticDiffHash: request.Review.Candidate.SemanticDiffHash,
		OperationInputHash: inputHash, RequiredScenarios: requiredScenarios,
		Validators: validators, ReviewArtifacts: artifacts}
	approval, err := w.GrantSensitiveApproval(SensitiveApprovalGrant{Authority: authority.Handle,
		ExpectedHead: request.Review.Candidate.HeadRevision, PolicyRevision: w.policyRevision, Evidence: evidence,
		Nonce: request.ApprovalNonce, TTL: request.ApprovalTTL})
	if err != nil {
		return HeadlessExport{}, err
	}
	authorization := SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: SensitiveExport,
		ExpectedHead: request.Review.Candidate.HeadRevision, PolicyRevision: w.policyRevision, Evidence: evidence}
	return HeadlessExport{CandidateDigest: request.Review.Candidate.HeadDigest, PlanHash: request.Review.Candidate.HeadPlanHash,
		PDFSHA256: hashBytes(rendered.PDF), PDFBytes: len(rendered.PDF), InputHash: inputHash,
		Approval: approval, Authorization: authorization, Input: input}, nil
}

func (w *Workspace) ExecuteHeadlessExport(ctx context.Context, prepared HeadlessExport, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	return w.ExecuteExport(ctx, prepared.Authorization, prepared.Input, executor)
}

func (w *Workspace) validateHeadlessCandidate(candidate HeadlessCandidate) error {
	if !validSHA256(candidate.BaseDigest) || !validSHA256(candidate.HeadDigest) || !validSHA256(candidate.SourceDiffHash) ||
		!validSHA256(candidate.SemanticDiffHash) || !validSHA256(candidate.BasePlanHash) || !validSHA256(candidate.HeadPlanHash) || candidate.PatchCount < 1 {
		return workspaceError("INVALID_HEADLESS_CANDIDATE", "candidate evidence is incomplete", ErrInvalidQuery)
	}
	head, err := w.Candidate(candidate.Candidate)
	if err != nil {
		return err
	}
	if head.Head != candidate.HeadRevision {
		return workspaceError("REVISION_CONFLICT", "headless candidate head changed", ErrRevisionConflict)
	}
	basePlan, err := w.OpenPlan(candidate.BasePlan)
	if err != nil {
		return err
	}
	headPlan, err := w.OpenPlan(candidate.HeadPlan)
	if err != nil {
		return err
	}
	if basePlan.Revision != candidate.BaseRevision || basePlan.Digest != candidate.BaseDigest || basePlan.Hash != candidate.BasePlanHash ||
		headPlan.Revision != candidate.HeadRevision || headPlan.Digest != candidate.HeadDigest || headPlan.Hash != candidate.HeadPlanHash {
		return workspaceError("HEADLESS_EVIDENCE_MISMATCH", "retained plans do not match candidate evidence", ErrRevisionConflict)
	}
	return nil
}

func hashCanonical(domain string, value any) string {
	payload, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(append(append([]byte(domain), 0), payload...))
	return hex.EncodeToString(digest[:])
}

func hashBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func headlessSemanticHash(target string, diff *paperedit.SourceDiff) string {
	return hashCanonical("paperd/headless-semantic-diff/v1", struct {
		Domain    string                `json:"domain"`
		Operation string                `json:"operation"`
		Target    string                `json:"target"`
		Diff      *paperedit.SourceDiff `json:"diff"`
	}{Domain: "source", Operation: "set_literal", Target: target, Diff: diff})
}

func headlessLiteralText(node *paperlang.Node) (string, error) {
	if node == nil {
		return "", paperedit.ErrInvalidOperation
	}
	if node.Kind == paperlang.NodeText {
		if node.Value == nil || node.Value.Kind != paperlang.ScalarString || node.Value.StringValue == nil {
			return "", paperedit.ErrInvalidOperation
		}
		return *node.Value.StringValue, nil
	}
	if node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading {
		return "", paperedit.ErrInvalidOperation
	}
	var values []string
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == "text" && member.Property.Value.Kind == paperlang.ScalarString && member.Property.Value.StringValue != nil {
			values = append(values, *member.Property.Value.StringValue)
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeText && member.Node.Value != nil && member.Node.Value.Kind == paperlang.ScalarString && member.Node.Value.StringValue != nil {
			values = append(values, *member.Node.Value.StringValue)
		}
	}
	if len(values) != 1 {
		return "", paperedit.ErrInvalidOperation
	}
	return values[0], nil
}

func equalHeadlessArtifacts(left, right []HeadlessArtifactEvidence) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func clonePaperReviewRequest(request document.PaperReviewRequest) document.PaperReviewRequest {
	request.SourceDiff = append([]byte(nil), request.SourceDiff...)
	request.CoreFontProgram = append([]byte(nil), request.CoreFontProgram...)
	request.BeforeFontPrograms = cloneByteMap(request.BeforeFontPrograms)
	request.AfterFontPrograms = cloneByteMap(request.AfterFontPrograms)
	request.BeforeImages = cloneByteMap(request.BeforeImages)
	request.AfterImages = cloneByteMap(request.AfterImages)
	return request
}

func cloneByteMap(source map[string][]byte) map[string][]byte {
	if source == nil {
		return nil
	}
	result := make(map[string][]byte, len(source))
	for key, value := range source {
		result[key] = append([]byte(nil), value...)
	}
	return result
}

// CanonicalJSON returns the bounded, handle-free protocol summary. Visual and
// PDF payloads must be requested through their separately authorized handles.
func (review HeadlessReview) CanonicalJSON(maxBytes int) ([]byte, error) {
	if maxBytes < 1 || maxBytes > MaxContextBytesHard {
		return nil, workspaceError("HEADLESS_RESPONSE_LIMIT", "response limit is outside hard bounds", ErrLimit)
	}
	if err := validateHeadlessProtocolSummary(review); err != nil {
		return nil, err
	}
	// Keep this projection explicit. A future exported field added to the
	// in-process review must not silently become a protocol capability or leak
	// artifact/source data merely because json.Marshal can see it.
	payload, err := json.Marshal(struct {
		SchemaVersion       uint16                     `json:"schema_version"`
		Candidate           HeadlessCandidate          `json:"candidate"`
		ExplainSHA256       string                     `json:"explain_sha256"`
		ExplainBytes        int                        `json:"explain_bytes"`
		ReviewManifestHash  string                     `json:"review_manifest_hash"`
		ReviewManifestBytes int                        `json:"review_manifest_bytes"`
		Artifacts           []HeadlessArtifactEvidence `json:"artifacts"`
	}{1, review.Candidate, review.ExplainSHA256, review.ExplainBytes, review.ReviewManifestHash, review.ReviewManifestBytes,
		append([]HeadlessArtifactEvidence(nil), review.Artifacts...)})
	if err != nil {
		return nil, fmt.Errorf("%w: encode headless review: %w", ErrHeadlessWorkflow, err)
	}
	if len(payload) > maxBytes {
		return nil, workspaceError("HEADLESS_RESPONSE_LIMIT", "headless review summary exceeds the caller bound", ErrLimit)
	}
	return payload, nil
}

func validateHeadlessProtocolSummary(review HeadlessReview) error {
	candidate := review.Candidate
	for _, digest := range []string{candidate.BaseDigest, candidate.HeadDigest, candidate.BasePlanHash, candidate.HeadPlanHash,
		candidate.SourceDiffHash, candidate.SemanticDiffHash, review.ExplainSHA256, review.ReviewManifestHash} {
		if !validSHA256(digest) {
			return workspaceError("INVALID_HEADLESS_SUMMARY", "headless protocol summary contains an invalid digest", ErrHeadlessWorkflow)
		}
	}
	if !safeProtocolIdentity(candidate.Target) || candidate.PatchCount != 1 || !candidate.Applied || review.ExplainBytes <= 0 || review.ReviewManifestBytes <= 0 {
		return workspaceError("INVALID_HEADLESS_SUMMARY", "headless protocol summary contains invalid identity, state, or sizes", ErrHeadlessWorkflow)
	}
	for _, artifact := range review.Artifacts {
		if !knownHeadlessArtifactKind(artifact.Kind) || !validSHA256(artifact.SHA256) || artifact.Bytes <= 0 {
			return workspaceError("INVALID_HEADLESS_SUMMARY", "headless protocol summary contains invalid artifact evidence", ErrHeadlessWorkflow)
		}
	}
	return nil
}

func knownHeadlessArtifactKind(kind string) bool {
	switch kind {
	case "clean_page", "overlay_page", "before_crop", "after_crop", "raster_diff", "contact_sheet", "source_diff",
		"semantic_diff", "plan_diff", "accessibility_diff", "diagnostics":
		return true
	default:
		return false
	}
}

// safeProtocolIdentity admits machine identifiers, not free-form prose. This
// prevents an authored label from becoming an instruction-bearing protocol
// string while retaining normal node/instance/path identities.
func safeProtocolIdentity(value string) bool {
	if value == "" || len(value) > 1024 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("@_./:#[]-", r) {
			continue
		}
		return false
	}
	return true
}
