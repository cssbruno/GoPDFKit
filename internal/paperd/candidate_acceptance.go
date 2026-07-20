// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/cssbruno/paperrune/internal/paperedit"
)

var ErrCandidateAcceptanceDenied = errors.New("paperd: candidate acceptance denied")
var ErrCandidateAcceptanceNotFound = errors.New("paperd: candidate acceptance not found")

// CandidateAcceptancePolicy is configured at the workspace trust boundary.
// It names the complete evidence set required before a candidate may be
// accepted; callers cannot weaken it in an individual request.
type CandidateAcceptancePolicy struct {
	RequiredScenarios       []string                        `json:"required_scenarios"`
	RequiredValidators      []CandidateValidatorRequirement `json:"required_validators"`
	RequiredReviewArtifacts []string                        `json:"required_review_artifacts"`
}

type CandidateValidatorRequirement struct {
	Profile string `json:"profile"`
	Version string `json:"version"`
}

// ScenarioAcceptanceEvidence identifies a passed result for one exact
// retained scenario revision. ResultHash is a detached report hash; fixture
// values and report bytes are never retained in acceptance state or audit.
type ScenarioAcceptanceEvidence struct {
	Revision   ScenarioRevisionHandle `json:"-"`
	Name       string                 `json:"name"`
	Digest     string                 `json:"scenario_revision"`
	ResultHash string                 `json:"result_hash"`
	Passed     bool                   `json:"passed"`
}

type ValidatorAcceptanceEvidence struct {
	Profile string `json:"profile"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
	Passed  bool   `json:"passed"`
}

type ReviewAcceptanceEvidence struct {
	Kind     string `json:"kind"`
	Hash     string `json:"hash"`
	Approved bool   `json:"approved"`
}

type ScenarioAcceptanceReceipt struct {
	Name       string `json:"name"`
	Digest     string `json:"scenario_revision"`
	ResultHash string `json:"result_hash"`
	Passed     bool   `json:"passed"`
}

type CandidateAcceptanceRequest struct {
	Candidate        CandidateHandle
	ExpectedHead     RevisionHandle
	ExpectedRevision paperedit.Revision
	IdempotencyKey   string
	Scenarios        []ScenarioAcceptanceEvidence
	Validators       []ValidatorAcceptanceEvidence
	ReviewArtifacts  []ReviewAcceptanceEvidence
	Authorization    SensitiveOperationRequest
}

// CandidateAcceptanceReceipt is an immutable, secret-free proof projection.
// It contains only stable identifiers and hashes and is safe to detach.
type CandidateAcceptanceReceipt struct {
	CandidateRevision string                        `json:"candidate_revision"`
	PolicyRevision    string                        `json:"policy_revision"`
	PolicyHash        string                        `json:"policy_hash"`
	EvidenceHash      string                        `json:"evidence_hash"`
	AcceptanceHash    string                        `json:"acceptance_hash"`
	Actor             string                        `json:"actor"`
	AcceptedAt        time.Time                     `json:"accepted_at"`
	ScenarioResults   []ScenarioAcceptanceReceipt   `json:"scenario_results"`
	Validators        []ValidatorAcceptanceEvidence `json:"validators"`
	ReviewArtifacts   []ReviewAcceptanceEvidence    `json:"review_artifacts"`
	Audit             SensitiveOperationReceipt     `json:"audit"`
}

type candidateAcceptanceRecord struct{ receipt CandidateAcceptanceReceipt }
type candidateAcceptanceIdempotencyRecord struct {
	fingerprint string
	receipt     CandidateAcceptanceReceipt
}

func normalizeCandidateAcceptancePolicy(policy CandidateAcceptancePolicy, limit int) (CandidateAcceptancePolicy, string, error) {
	result := CandidateAcceptancePolicy{
		RequiredScenarios:       append([]string(nil), policy.RequiredScenarios...),
		RequiredValidators:      append([]CandidateValidatorRequirement(nil), policy.RequiredValidators...),
		RequiredReviewArtifacts: append([]string(nil), policy.RequiredReviewArtifacts...),
	}
	total := len(result.RequiredScenarios) + len(result.RequiredValidators) + len(result.RequiredReviewArtifacts)
	if total == 0 {
		return result, "", nil
	}
	if len(result.RequiredScenarios) == 0 || len(result.RequiredValidators) == 0 || len(result.RequiredReviewArtifacts) == 0 || total > limit {
		return CandidateAcceptancePolicy{}, "", workspaceError("INVALID_ACCEPTANCE_POLICY", "acceptance policy requires bounded scenario, validator, and review requirements", ErrInvalidLimits)
	}
	for _, value := range append(append([]string(nil), result.RequiredScenarios...), result.RequiredReviewArtifacts...) {
		if !validSensitiveLabel(value, limit) {
			return CandidateAcceptancePolicy{}, "", workspaceError("INVALID_ACCEPTANCE_POLICY", "acceptance policy contains an invalid identity", ErrInvalidLimits)
		}
	}
	for _, requirement := range result.RequiredValidators {
		if !validSensitiveLabel(requirement.Profile, limit) || !validSensitiveLabel(requirement.Version, limit) {
			return CandidateAcceptancePolicy{}, "", workspaceError("INVALID_ACCEPTANCE_POLICY", "acceptance policy contains an invalid validator identity", ErrInvalidLimits)
		}
	}
	sort.Strings(result.RequiredScenarios)
	sort.Strings(result.RequiredReviewArtifacts)
	sort.Slice(result.RequiredValidators, func(i, j int) bool {
		return result.RequiredValidators[i].Profile+"\x00"+result.RequiredValidators[i].Version < result.RequiredValidators[j].Profile+"\x00"+result.RequiredValidators[j].Version
	})
	if hasDuplicateStrings(result.RequiredScenarios) || hasDuplicateStrings(result.RequiredReviewArtifacts) || hasDuplicateJSON(result.RequiredValidators) {
		return CandidateAcceptancePolicy{}, "", workspaceError("INVALID_ACCEPTANCE_POLICY", "acceptance policy contains duplicate requirements", ErrInvalidLimits)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return CandidateAcceptancePolicy{}, "", workspaceError("INVALID_ACCEPTANCE_POLICY", "acceptance policy cannot be encoded", ErrInvalidLimits)
	}
	return result, acceptanceHash("paperd/candidate-acceptance-policy/v1", payload), nil
}

func (w *Workspace) CandidateAcceptancePolicy() (CandidateAcceptancePolicy, string) {
	if w == nil {
		return CandidateAcceptancePolicy{}, ""
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return cloneCandidateAcceptancePolicy(w.acceptancePolicy), w.acceptancePolicyHash
}

// CandidateAcceptanceInputHash returns the exact operation-input digest that
// an outer reviewer must place in SensitiveEvidence before granting a one-use
// acceptance approval. It exposes no opaque handle internals or evidence bytes.
func (w *Workspace) CandidateAcceptanceInputHash(request CandidateAcceptanceRequest) (string, error) {
	if w == nil {
		return "", workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	_, _, inputHash, err := w.canonicalCandidateAcceptanceRequest(request)
	return inputHash, err
}

func cloneCandidateAcceptancePolicy(policy CandidateAcceptancePolicy) CandidateAcceptancePolicy {
	policy.RequiredScenarios = append([]string(nil), policy.RequiredScenarios...)
	policy.RequiredValidators = append([]CandidateValidatorRequirement(nil), policy.RequiredValidators...)
	policy.RequiredReviewArtifacts = append([]string(nil), policy.RequiredReviewArtifacts...)
	return policy
}

// AcceptCandidate validates every configured gate and consumes the exact
// acceptance approval in the same lock acquisition that commits acceptance.
// A concurrent edit or acceptance therefore wins by CAS; no partial accepted
// state can escape cancellation, stale evidence, or authorization failure.
func (w *Workspace) AcceptCandidate(ctx context.Context, request CandidateAcceptanceRequest) (CandidateAcceptanceReceipt, error) {
	if w == nil {
		return CandidateAcceptanceReceipt{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if ctx == nil {
		return CandidateAcceptanceReceipt{}, workspaceError("INVALID_CONTEXT", "acceptance context is nil", ErrCandidateAcceptanceDenied)
	}
	if err := ctx.Err(); err != nil {
		return CandidateAcceptanceReceipt{}, workspaceError("ACCEPTANCE_CANCELLED", "candidate acceptance was cancelled", errors.Join(ErrCandidateAcceptanceDenied, err))
	}
	canonical, fingerprint, inputHash, err := w.canonicalCandidateAcceptanceRequest(request)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	candidate, err := w.candidateLocked(request.Candidate)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	if cached, ok := candidate.acceptanceIdempotency[request.IdempotencyKey]; ok {
		if cached.fingerprint != fingerprint {
			return CandidateAcceptanceReceipt{}, workspaceError("ACCEPTANCE_IDEMPOTENCY_CONFLICT", "idempotency key was already used for another acceptance request", ErrRevisionConflict)
		}
		return cloneCandidateAcceptanceReceipt(cached.receipt), nil
	}
	deny := func(code, message string, cause error) (CandidateAcceptanceReceipt, error) {
		actor := "unavailable"
		if authority, lookupErr := w.sensitiveAuthorityLocked(request.Authorization.Authority); lookupErr == nil {
			actor = authority.actor
		}
		_, evidenceHash, _ := canonicalSensitiveEvidence(request.Authorization.Evidence, w.limits.MaxQueryBytes)
		w.appendSensitiveAuditLocked(actor, SensitiveAccept, request.Candidate.value.serial, request.Authorization.PolicyRevision, evidenceHash, false, code)
		return CandidateAcceptanceReceipt{}, workspaceError(code, message, errors.Join(ErrCandidateAcceptanceDenied, cause))
	}
	if err := ctx.Err(); err != nil {
		return deny("ACCEPTANCE_CANCELLED", "candidate acceptance was cancelled", err)
	}
	if w.acceptancePolicyHash == "" {
		return deny("ACCEPTANCE_POLICY_REQUIRED", "workspace has no configured candidate acceptance policy", ErrSensitiveOperationDenied)
	}
	if candidate.head != request.ExpectedHead {
		return deny("ACCEPTANCE_HEAD_CHANGED", "candidate head changed before acceptance", ErrRevisionConflict)
	}
	revision, err := w.revisionLocked(request.ExpectedHead)
	if err != nil {
		return deny("ACCEPTANCE_REVISION_UNAVAILABLE", "candidate revision is unavailable", err)
	}
	if revision.revision != request.ExpectedRevision {
		return deny("ACCEPTANCE_REVISION_MISMATCH", "candidate revision does not match the exact expected digest", ErrRevisionConflict)
	}
	if candidate.acceptance != nil {
		return deny("CANDIDATE_ALREADY_ACCEPTED", "candidate head already has a committed acceptance", ErrRevisionConflict)
	}
	if err := w.validateAcceptanceEvidenceLocked(canonical, revision); err != nil {
		return deny(errorCodeForAudit(err), "candidate evidence does not satisfy the configured acceptance policy", err)
	}
	if request.Authorization.Operation != SensitiveAccept || request.Authorization.ExpectedHead != request.ExpectedHead || request.Authorization.PolicyRevision != w.policyRevision || request.Authorization.Evidence.OperationInputHash != inputHash {
		return deny("ACCEPTANCE_AUTHORIZATION_BINDING", "acceptance requires exact candidate, policy, operation, and input authority", ErrSensitiveOperationDenied)
	}
	authority, authorityErr := w.sensitiveAuthorityLocked(request.Authorization.Authority)
	if authorityErr != nil || authority.candidate != request.Candidate {
		return deny("ACCEPTANCE_AUTHORIZATION_BINDING", "acceptance authority does not bind the exact candidate", errors.Join(ErrSensitiveOperationDenied, authorityErr))
	}
	if request.Authorization.Evidence.CandidateRevision != string(revision.revision) || !equalStrings(request.Authorization.Evidence.RequiredScenarios, w.acceptancePolicy.RequiredScenarios) || !acceptanceValidatorsMatchSensitive(canonical.Validators, request.Authorization.Evidence.Validators) || !acceptanceReviewsMatchSensitive(canonical.ReviewArtifacts, request.Authorization.Evidence.ReviewArtifacts) {
		return deny("ACCEPTANCE_EVIDENCE_BINDING", "approval evidence does not bind the complete acceptance result set", ErrSensitiveOperationDenied)
	}
	_, evidenceHash, evidenceErr := canonicalSensitiveEvidence(request.Authorization.Evidence, w.limits.MaxQueryBytes)
	audit, err := w.authorizeSensitiveOperationLocked(request.Authorization, evidenceHash, evidenceErr)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	receipt := CandidateAcceptanceReceipt{CandidateRevision: string(revision.revision), PolicyRevision: w.policyRevision, PolicyHash: w.acceptancePolicyHash,
		EvidenceHash: evidenceHash, Actor: audit.Actor, AcceptedAt: w.now().UTC(), ScenarioResults: scenarioAcceptanceReceipts(canonical.Scenarios),
		Validators: canonical.Validators, ReviewArtifacts: canonical.ReviewArtifacts, Audit: audit}
	receipt.AcceptanceHash = candidateAcceptanceReceiptHash(receipt)
	candidate.acceptance = &candidateAcceptanceRecord{receipt: cloneCandidateAcceptanceReceipt(receipt)}
	candidate.acceptanceIdempotency[request.IdempotencyKey] = candidateAcceptanceIdempotencyRecord{fingerprint: fingerprint, receipt: cloneCandidateAcceptanceReceipt(receipt)}
	return cloneCandidateAcceptanceReceipt(receipt), nil
}

func (w *Workspace) canonicalCandidateAcceptanceRequest(request CandidateAcceptanceRequest) (CandidateAcceptanceRequest, string, string, error) {
	if !validSensitiveLabel(request.IdempotencyKey, w.limits.MaxQueryBytes) || !validSHA256(string(request.ExpectedRevision)) {
		return CandidateAcceptanceRequest{}, "", "", workspaceError("INVALID_ACCEPTANCE_REQUEST", "acceptance requires a bounded idempotency key and exact lowercase SHA-256 revision", ErrInvalidQuery)
	}
	result := request
	result.Scenarios = append([]ScenarioAcceptanceEvidence(nil), request.Scenarios...)
	result.Validators = append([]ValidatorAcceptanceEvidence(nil), request.Validators...)
	result.ReviewArtifacts = append([]ReviewAcceptanceEvidence(nil), request.ReviewArtifacts...)
	if len(result.Scenarios)+len(result.Validators)+len(result.ReviewArtifacts) > w.limits.MaxQueryBytes {
		return CandidateAcceptanceRequest{}, "", "", workspaceError("ACCEPTANCE_EVIDENCE_LIMIT", "acceptance evidence exceeds configured bounds", ErrLimit)
	}
	for _, value := range result.Scenarios {
		if !validSensitiveLabel(value.Name, w.limits.MaxQueryBytes) || !validSHA256(value.Digest) || !validSHA256(value.ResultHash) {
			return CandidateAcceptanceRequest{}, "", "", workspaceError("INVALID_SCENARIO_EVIDENCE", "scenario acceptance evidence is invalid", ErrInvalidQuery)
		}
	}
	for _, value := range result.Validators {
		if !validSensitiveLabel(value.Profile, w.limits.MaxQueryBytes) || !validSensitiveLabel(value.Version, w.limits.MaxQueryBytes) || !validSHA256(value.Hash) {
			return CandidateAcceptanceRequest{}, "", "", workspaceError("INVALID_VALIDATOR_EVIDENCE", "validator acceptance evidence is invalid", ErrInvalidQuery)
		}
	}
	for _, value := range result.ReviewArtifacts {
		if !validSensitiveLabel(value.Kind, w.limits.MaxQueryBytes) || !validSHA256(value.Hash) {
			return CandidateAcceptanceRequest{}, "", "", workspaceError("INVALID_REVIEW_EVIDENCE", "review acceptance evidence is invalid", ErrInvalidQuery)
		}
	}
	sort.Slice(result.Scenarios, func(i, j int) bool { return result.Scenarios[i].Name < result.Scenarios[j].Name })
	sort.Slice(result.Validators, func(i, j int) bool { return fmt.Sprint(result.Validators[i]) < fmt.Sprint(result.Validators[j]) })
	sort.Slice(result.ReviewArtifacts, func(i, j int) bool { return result.ReviewArtifacts[i].Kind < result.ReviewArtifacts[j].Kind })
	if duplicateScenarioEvidence(result.Scenarios) || duplicateValidatorAcceptance(result.Validators) || duplicateReviewAcceptance(result.ReviewArtifacts) {
		return CandidateAcceptanceRequest{}, "", "", workspaceError("DUPLICATE_ACCEPTANCE_EVIDENCE", "acceptance evidence contains duplicate identities", ErrInvalidQuery)
	}
	payload := acceptanceInputProjection{CandidateRevision: string(request.ExpectedRevision), PolicyRevision: w.policyRevision, PolicyHash: w.acceptancePolicyHash,
		Scenarios: result.Scenarios, Validators: result.Validators, ReviewArtifacts: result.ReviewArtifacts}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return CandidateAcceptanceRequest{}, "", "", workspaceError("INVALID_ACCEPTANCE_EVIDENCE", "acceptance evidence cannot be encoded", ErrInvalidQuery)
	}
	inputHash := acceptanceHash("paperd/candidate-acceptance-input/v1", encoded)
	_, authorizationEvidenceHash, _ := canonicalSensitiveEvidence(request.Authorization.Evidence, w.limits.MaxQueryBytes)
	fingerprintBytes, err := json.Marshal(struct {
		Input           string `json:"input"`
		HeadSerial      uint64 `json:"head_serial"`
		HeadNonce       uint64 `json:"head_nonce"`
		AuthoritySerial uint64 `json:"authority_serial"`
		ApprovalSerial  uint64 `json:"approval_serial"`
		EvidenceHash    string `json:"evidence_hash"`
	}{inputHash, request.ExpectedHead.value.serial, request.ExpectedHead.value.nonce, request.Authorization.Authority.value.serial, request.Authorization.Approval.value.serial, authorizationEvidenceHash})
	if err != nil {
		return CandidateAcceptanceRequest{}, "", "", workspaceError("INVALID_ACCEPTANCE_EVIDENCE", "acceptance fingerprint cannot be encoded", ErrInvalidQuery)
	}
	return result, acceptanceHash("paperd/candidate-acceptance-request/v1", fingerprintBytes), inputHash, nil
}

type acceptanceInputProjection struct {
	CandidateRevision string                        `json:"candidate_revision"`
	PolicyRevision    string                        `json:"policy_revision"`
	PolicyHash        string                        `json:"policy_hash"`
	Scenarios         []ScenarioAcceptanceEvidence  `json:"scenarios"`
	Validators        []ValidatorAcceptanceEvidence `json:"validators"`
	ReviewArtifacts   []ReviewAcceptanceEvidence    `json:"review_artifacts"`
}

func (w *Workspace) validateAcceptanceEvidenceLocked(request CandidateAcceptanceRequest, revision *revisionRecord) error {
	if len(request.Scenarios) != len(w.acceptancePolicy.RequiredScenarios) || len(request.Validators) != len(w.acceptancePolicy.RequiredValidators) || len(request.ReviewArtifacts) != len(w.acceptancePolicy.RequiredReviewArtifacts) {
		return workspaceError("ACCEPTANCE_EVIDENCE_INCOMPLETE", "acceptance evidence does not exactly cover configured requirements", ErrCandidateAcceptanceDenied)
	}
	for i, required := range w.acceptancePolicy.RequiredScenarios {
		value := request.Scenarios[i]
		if value.Name != required || !value.Passed {
			return workspaceError("SCENARIO_GATE_FAILED", "a required scenario did not pass", ErrCandidateAcceptanceDenied)
		}
		record, err := w.scenarioRevisionLocked(value.Revision)
		if err != nil || record.digest != value.Digest || !scenarioRecordContains(record, value.Name) {
			return workspaceError("SCENARIO_EVIDENCE_STALE", "scenario evidence does not bind a live exact fixture revision", errors.Join(ErrCandidateAcceptanceDenied, err))
		}
	}
	for i, required := range w.acceptancePolicy.RequiredValidators {
		value := request.Validators[i]
		if value.Profile != required.Profile || value.Version != required.Version || !value.Passed {
			return workspaceError("VALIDATOR_GATE_FAILED", "a required validator did not pass at the configured version", ErrCandidateAcceptanceDenied)
		}
	}
	for i, required := range w.acceptancePolicy.RequiredReviewArtifacts {
		value := request.ReviewArtifacts[i]
		if value.Kind != required || !value.Approved {
			return workspaceError("REVIEW_GATE_FAILED", "a required review artifact was not approved", ErrCandidateAcceptanceDenied)
		}
	}
	_ = revision
	return nil
}

func scenarioRecordContains(record *scenarioRevisionRecord, name string) bool {
	for _, fixture := range record.fixtures {
		if fixture.Name == name || "@"+fixture.Name == name || fixture.Name == "@"+name {
			return true
		}
	}
	return false
}

func acceptanceValidatorsMatchSensitive(values []ValidatorAcceptanceEvidence, sensitive []ValidatorEvidence) bool {
	if len(values) != len(sensitive) {
		return false
	}
	want := append([]ValidatorEvidence(nil), sensitive...)
	sort.Slice(want, func(i, j int) bool { return fmt.Sprint(want[i]) < fmt.Sprint(want[j]) })
	for i := range values {
		if values[i].Profile != want[i].Profile || values[i].Version != want[i].Version || values[i].Hash != want[i].Hash || !values[i].Passed {
			return false
		}
	}
	return true
}

func acceptanceReviewsMatchSensitive(values []ReviewAcceptanceEvidence, sensitive []ReviewArtifactEvidence) bool {
	if len(values) != len(sensitive) {
		return false
	}
	want := append([]ReviewArtifactEvidence(nil), sensitive...)
	sort.Slice(want, func(i, j int) bool { return want[i].Kind < want[j].Kind })
	for i := range values {
		if values[i].Kind != want[i].Kind || values[i].Hash != want[i].Hash || !values[i].Approved {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	l := append([]string(nil), left...)
	r := append([]string(nil), right...)
	sort.Strings(l)
	sort.Strings(r)
	for i := range l {
		if l[i] != r[i] {
			return false
		}
	}
	return true
}

func duplicateScenarioEvidence(values []ScenarioAcceptanceEvidence) bool {
	for i := 1; i < len(values); i++ {
		if values[i].Name == values[i-1].Name {
			return true
		}
	}
	return false
}
func duplicateValidatorAcceptance(values []ValidatorAcceptanceEvidence) bool {
	for i := 1; i < len(values); i++ {
		if values[i].Profile == values[i-1].Profile && values[i].Version == values[i-1].Version {
			return true
		}
	}
	return false
}
func duplicateReviewAcceptance(values []ReviewAcceptanceEvidence) bool {
	for i := 1; i < len(values); i++ {
		if values[i].Kind == values[i-1].Kind {
			return true
		}
	}
	return false
}

func candidateAcceptanceReceiptHash(receipt CandidateAcceptanceReceipt) string {
	copy := cloneCandidateAcceptanceReceipt(receipt)
	copy.AcceptanceHash = ""
	encoded, err := json.Marshal(copy)
	if err != nil {
		return ""
	}
	return acceptanceHash("paperd/candidate-acceptance-receipt/v1", encoded)
}

func cloneCandidateAcceptanceReceipt(receipt CandidateAcceptanceReceipt) CandidateAcceptanceReceipt {
	receipt.ScenarioResults = append([]ScenarioAcceptanceReceipt(nil), receipt.ScenarioResults...)
	receipt.Validators = append([]ValidatorAcceptanceEvidence(nil), receipt.Validators...)
	receipt.ReviewArtifacts = append([]ReviewAcceptanceEvidence(nil), receipt.ReviewArtifacts...)
	return receipt
}

func scenarioAcceptanceReceipts(values []ScenarioAcceptanceEvidence) []ScenarioAcceptanceReceipt {
	result := make([]ScenarioAcceptanceReceipt, len(values))
	for i, value := range values {
		result[i] = ScenarioAcceptanceReceipt{Name: value.Name, Digest: value.Digest, ResultHash: value.ResultHash, Passed: value.Passed}
	}
	return result
}

// CandidateAcceptance returns the currently accepted exact head. An edit
// clears this projection atomically, so stale acceptance cannot follow a
// mutable candidate pointer.
func (w *Workspace) CandidateAcceptance(candidate CandidateHandle) (CandidateAcceptanceReceipt, error) {
	if w == nil {
		return CandidateAcceptanceReceipt{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.candidateLocked(candidate)
	if err != nil {
		return CandidateAcceptanceReceipt{}, err
	}
	if record.acceptance == nil {
		return CandidateAcceptanceReceipt{}, workspaceError("CANDIDATE_ACCEPTANCE_NOT_FOUND", "candidate head has no committed acceptance", ErrCandidateAcceptanceNotFound)
	}
	return cloneCandidateAcceptanceReceipt(record.acceptance.receipt), nil
}

func acceptanceHash(domain string, payload []byte) string {
	sum := sha256.Sum256(append(append([]byte(nil), []byte(domain+"\x00")...), payload...))
	return hex.EncodeToString(sum[:])
}
