// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrSensitiveAuthorityNotFound = errors.New("paperd: sensitive authority not found")
	ErrSensitiveApprovalNotFound  = errors.New("paperd: sensitive approval not found")
	ErrSensitiveOperationDenied   = errors.New("paperd: sensitive operation denied")
	ErrApprovalReplay             = errors.New("paperd: approval replay")
)

// SensitiveOperation is a closed vocabulary whose members intentionally map
// to distinct handle capabilities. Edit authority never implies any member.
type SensitiveOperation string

const (
	SensitiveExport            SensitiveOperation = "export"
	SensitivePublish           SensitiveOperation = "publish"
	SensitiveAttachment        SensitiveOperation = "attachment"
	SensitiveProductionCapture SensitiveOperation = "production_capture"
	SensitiveSign              SensitiveOperation = "sign"
	SensitiveAccept            SensitiveOperation = "accept_candidate"
)

func (operation SensitiveOperation) capability() (handleCapability, bool) {
	switch operation {
	case SensitiveExport:
		return capabilityExport, true
	case SensitivePublish:
		return capabilityPublish, true
	case SensitiveAttachment:
		return capabilityAttachment, true
	case SensitiveProductionCapture:
		return capabilityProductionCapture, true
	case SensitiveSign:
		return capabilitySign, true
	case SensitiveAccept:
		return capabilityAccept, true
	default:
		return 0, false
	}
}

type SensitiveAuthorityGrant struct {
	Open      OpenHandle
	Actor     string
	Operation SensitiveOperation
}

type SensitiveAuthoritySnapshot struct {
	Handle           SensitiveAuthorityHandle `json:"-"`
	Actor            string                   `json:"actor"`
	Operation        SensitiveOperation       `json:"operation"`
	DisclosureDomain DisclosureDomain         `json:"disclosure_domain"`
	ExpiresAt        time.Time                `json:"expires_at"`
}

type ValidatorEvidence struct {
	Profile string `json:"profile"`
	Version string `json:"version"`
	Hash    string `json:"hash"`
}

type ReviewArtifactEvidence struct {
	Kind string `json:"kind"`
	Hash string `json:"hash"`
}

// SensitiveEvidence contains hashes and stable identifiers only. Source,
// rendered production values, prompts, and review artifact bytes do not enter
// retained approval or audit state.
type SensitiveEvidence struct {
	CandidateRevision  string                   `json:"candidate_revision"`
	SourceDiffHash     string                   `json:"source_diff_hash"`
	SemanticDiffHash   string                   `json:"semantic_diff_hash"`
	OperationInputHash string                   `json:"operation_input_hash"`
	RequiredScenarios  []string                 `json:"required_scenarios"`
	Validators         []ValidatorEvidence      `json:"validators"`
	ReviewArtifacts    []ReviewArtifactEvidence `json:"review_artifacts"`
}

type SensitiveApprovalGrant struct {
	Authority      SensitiveAuthorityHandle
	ExpectedHead   RevisionHandle
	PolicyRevision string
	Evidence       SensitiveEvidence
	Nonce          string
	TTL            time.Duration
}

type SensitiveApprovalSnapshot struct {
	Handle         SensitiveApprovalHandle `json:"-"`
	Operation      SensitiveOperation      `json:"operation"`
	Actor          string                  `json:"actor"`
	PolicyRevision string                  `json:"policy_revision"`
	EvidenceHash   string                  `json:"evidence_hash"`
	ExpiresAt      time.Time               `json:"expires_at"`
}

type SensitiveOperationRequest struct {
	Authority      SensitiveAuthorityHandle
	Approval       SensitiveApprovalHandle
	Operation      SensitiveOperation
	ExpectedHead   RevisionHandle
	PolicyRevision string
	Evidence       SensitiveEvidence
}

type SensitiveOperationReceipt struct {
	Sequence       uint64             `json:"sequence"`
	Operation      SensitiveOperation `json:"operation"`
	Actor          string             `json:"actor"`
	PolicyRevision string             `json:"policy_revision"`
	EvidenceHash   string             `json:"evidence_hash"`
	AuditHash      string             `json:"audit_hash"`
}

// SensitiveAuditEntry is an append-only, hash-chained decision record. It
// deliberately excludes raw evidence, nonce values, and capability handles.
type SensitiveAuditEntry struct {
	Sequence        uint64             `json:"sequence"`
	At              time.Time          `json:"at"`
	Actor           string             `json:"actor"`
	Operation       SensitiveOperation `json:"operation"`
	CandidateSerial uint64             `json:"candidate_serial"`
	PolicyRevision  string             `json:"policy_revision"`
	EvidenceHash    string             `json:"evidence_hash"`
	Allowed         bool               `json:"allowed"`
	Reason          string             `json:"reason"`
	PreviousHash    string             `json:"previous_hash,omitempty"`
	EventHash       string             `json:"event_hash"`
}

func (w *Workspace) GrantSensitiveAuthority(request SensitiveAuthorityGrant) (SensitiveAuthoritySnapshot, error) {
	if w == nil {
		return SensitiveAuthoritySnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	capability, ok := request.Operation.capability()
	if !ok {
		return SensitiveAuthoritySnapshot{}, workspaceError("INVALID_SENSITIVE_OPERATION", "sensitive operation is unsupported", ErrInvalidQuery)
	}
	if !validSensitiveLabel(request.Actor, w.limits.MaxQueryBytes) {
		return SensitiveAuthoritySnapshot{}, workspaceError("INVALID_ACTOR", "actor identity must be bounded valid UTF-8", ErrInvalidQuery)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	opened, err := w.openLocked(request.Open)
	if err != nil {
		return SensitiveAuthoritySnapshot{}, err
	}
	if opened.candidate.value.serial == 0 {
		return SensitiveAuthoritySnapshot{}, workspaceError("CAPABILITY_DENIED", "sensitive authority requires an exact candidate open", ErrSensitiveOperationDenied)
	}
	if len(w.sensitiveAuthorities) >= w.limits.MaxMutationAuthorities {
		return SensitiveAuthoritySnapshot{}, workspaceError("AUTHORITY_LIMIT", "sensitive authority capacity is exhausted", ErrLimit)
	}
	w.nextSensitiveAuthority++
	handle := SensitiveAuthorityHandle{value: w.newHandle(handleSensitiveAuthority, capability, w.nextSensitiveAuthority)}
	record := &sensitiveAuthorityRecord{handle: handle, open: request.Open, candidate: opened.candidate, actor: request.Actor,
		operation: request.Operation, expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.sensitiveAuthorities[handle.value.serial] = record
	return sensitiveAuthoritySnapshot(record), nil
}

func (w *Workspace) OpenSensitiveAuthority(handle SensitiveAuthorityHandle) (SensitiveAuthoritySnapshot, error) {
	if w == nil {
		return SensitiveAuthoritySnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.sensitiveAuthorityLocked(handle)
	if err != nil {
		return SensitiveAuthoritySnapshot{}, err
	}
	return sensitiveAuthoritySnapshot(record), nil
}

func sensitiveAuthoritySnapshot(record *sensitiveAuthorityRecord) SensitiveAuthoritySnapshot {
	return SensitiveAuthoritySnapshot{Handle: record.handle, Actor: record.actor, Operation: record.operation, DisclosureDomain: record.disclosure, ExpiresAt: record.expires}
}

func (w *Workspace) sensitiveAuthorityLocked(handle SensitiveAuthorityHandle) (*sensitiveAuthorityRecord, error) {
	capability, ok := sensitiveCapability(handle.value.capability)
	if !ok || w.validateHandle(handle.value, handleSensitiveAuthority, handle.value.capability, false) != nil {
		return nil, workspaceError("INVALID_HANDLE", "handle is unavailable", ErrInvalidHandle)
	}
	record := w.sensitiveAuthorities[handle.value.serial]
	if record == nil || record.handle != handle || record.operation != capability || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrSensitiveAuthorityNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func sensitiveCapability(capability handleCapability) (SensitiveOperation, bool) {
	switch capability {
	case capabilityExport:
		return SensitiveExport, true
	case capabilityPublish:
		return SensitivePublish, true
	case capabilityAttachment:
		return SensitiveAttachment, true
	case capabilityProductionCapture:
		return SensitiveProductionCapture, true
	case capabilitySign:
		return SensitiveSign, true
	case capabilityAccept:
		return SensitiveAccept, true
	default:
		return "", false
	}
}

func (w *Workspace) GrantSensitiveApproval(request SensitiveApprovalGrant) (SensitiveApprovalSnapshot, error) {
	if w == nil {
		return SensitiveApprovalSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	canonical, evidenceHash, err := canonicalSensitiveEvidence(request.Evidence, w.limits.MaxQueryBytes)
	if err != nil {
		return SensitiveApprovalSnapshot{}, err
	}
	_ = canonical
	if request.PolicyRevision != w.policyRevision {
		return SensitiveApprovalSnapshot{}, workspaceError("APPROVAL_POLICY_MISMATCH", "approval policy revision does not match the workspace", ErrSensitiveOperationDenied)
	}
	if !validSensitiveLabel(request.Nonce, w.limits.MaxQueryBytes) || len(request.Nonce) < 16 {
		return SensitiveApprovalSnapshot{}, workspaceError("INVALID_APPROVAL_NONCE", "approval nonce must be unique and contain at least 16 bounded UTF-8 bytes", ErrInvalidQuery)
	}
	if request.TTL <= 0 || request.TTL > w.handleTTL || request.TTL > MaxHandleTTLHard {
		return SensitiveApprovalSnapshot{}, workspaceError("INVALID_APPROVAL_TTL", "approval TTL must be positive and no greater than the workspace handle TTL", ErrInvalidLimits)
	}
	nonceHash := sha256.Sum256([]byte("paperd/approval-nonce/v1\x00" + request.Nonce))
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	authority, err := w.sensitiveAuthorityLocked(request.Authority)
	if err != nil {
		return SensitiveApprovalSnapshot{}, err
	}
	candidate, err := w.candidateLocked(authority.candidate)
	if err != nil {
		return SensitiveApprovalSnapshot{}, err
	}
	if candidate.head != request.ExpectedHead {
		return SensitiveApprovalSnapshot{}, workspaceError("APPROVAL_HEAD_MISMATCH", "approval expected head is stale", ErrRevisionConflict)
	}
	revision, err := w.revisionLocked(request.ExpectedHead)
	if err != nil {
		return SensitiveApprovalSnapshot{}, err
	}
	if string(revision.revision) != request.Evidence.CandidateRevision {
		return SensitiveApprovalSnapshot{}, workspaceError("APPROVAL_EVIDENCE_MISMATCH", "candidate revision is not the reviewed evidence revision", ErrSensitiveOperationDenied)
	}
	if _, exists := w.sensitiveApprovalNonces[nonceHash]; exists {
		return SensitiveApprovalSnapshot{}, workspaceError("APPROVAL_NONCE_REPLAY", "approval nonce was already used", ErrApprovalReplay)
	}
	if len(w.sensitiveApprovals) >= w.limits.MaxMutationAuthorities || len(w.sensitiveApprovalNonces) >= w.limits.MaxAuthorizationAudit {
		return SensitiveApprovalSnapshot{}, workspaceError("APPROVAL_LIMIT", "approval capacity is exhausted", ErrLimit)
	}
	w.sensitiveApprovalNonces[nonceHash] = struct{}{}
	w.nextSensitiveApproval++
	handle := SensitiveApprovalHandle{value: w.newHandle(handleSensitiveApproval, capabilityApprove, w.nextSensitiveApproval)}
	record := &sensitiveApprovalRecord{handle: handle, authority: request.Authority, candidate: authority.candidate, expectedHead: request.ExpectedHead,
		operation: authority.operation, actor: authority.actor, policy: request.PolicyRevision, evidenceHash: evidenceHash,
		expires: w.now().Add(request.TTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.sensitiveApprovals[handle.value.serial] = record
	return sensitiveApprovalSnapshot(record), nil
}

func sensitiveApprovalSnapshot(record *sensitiveApprovalRecord) SensitiveApprovalSnapshot {
	return SensitiveApprovalSnapshot{Handle: record.handle, Operation: record.operation, Actor: record.actor, PolicyRevision: record.policy, EvidenceHash: record.evidenceHash, ExpiresAt: record.expires}
}

func (w *Workspace) AuthorizeSensitiveOperation(request SensitiveOperationRequest) (SensitiveOperationReceipt, error) {
	if w == nil {
		return SensitiveOperationReceipt{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	_, evidenceHash, evidenceErr := canonicalSensitiveEvidence(request.Evidence, w.limits.MaxQueryBytes)
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.authorizeSensitiveOperationLocked(request, evidenceHash, evidenceErr)
}

// authorizeSensitiveOperationLocked consumes a one-use approval while the
// caller holds w.mu. Keeping this commit primitive shared lets operations such
// as candidate acceptance bind authorization and their state transition in
// one compare-and-swap critical section.
func (w *Workspace) authorizeSensitiveOperationLocked(request SensitiveOperationRequest, evidenceHash string, evidenceErr error) (SensitiveOperationReceipt, error) {
	actor := "unavailable"
	candidateSerial := uint64(0)
	reason := "SENSITIVE_OPERATION_DENIED"
	deny := func(err error) (SensitiveOperationReceipt, error) {
		entry := w.appendSensitiveAuditLocked(actor, request.Operation, candidateSerial, request.PolicyRevision, evidenceHash, false, reason)
		_ = entry
		return SensitiveOperationReceipt{}, err
	}
	if evidenceErr != nil {
		reason = errorCodeForAudit(evidenceErr)
		return deny(evidenceErr)
	}
	authority, err := w.sensitiveAuthorityLocked(request.Authority)
	if err != nil {
		reason = errorCodeForAudit(err)
		return deny(workspaceError("SENSITIVE_AUTHORITY_REQUIRED", "an exact live sensitive authority is required", ErrSensitiveOperationDenied))
	}
	actor, candidateSerial = authority.actor, authority.candidate.value.serial
	if authority.operation != request.Operation {
		reason = "SENSITIVE_CAPABILITY_DENIED"
		return deny(workspaceError(reason, "sensitive capability does not permit this operation", ErrSensitiveOperationDenied))
	}
	approval := w.sensitiveApprovals[request.Approval.value.serial]
	if err := w.validateHandle(request.Approval.value, handleSensitiveApproval, capabilityApprove, false); err != nil || approval == nil || approval.handle != request.Approval || !w.ownsPartition(approval.partition) {
		reason = "SENSITIVE_APPROVAL_REQUIRED"
		return deny(workspaceError(reason, "an exact live approval is required", ErrSensitiveOperationDenied))
	}
	if approval.used {
		reason = "APPROVAL_REPLAY"
		return deny(workspaceError(reason, "approval has already been consumed", errors.Join(ErrSensitiveOperationDenied, ErrApprovalReplay)))
	}
	if !approval.expires.After(w.now()) {
		reason = "APPROVAL_EXPIRED"
		return deny(workspaceError(reason, "approval has expired", errors.Join(ErrSensitiveOperationDenied, ErrHandleExpired)))
	}
	if approval.authority != request.Authority || approval.operation != request.Operation || approval.expectedHead != request.ExpectedHead || approval.policy != request.PolicyRevision || approval.evidenceHash != evidenceHash {
		reason = "APPROVAL_BINDING_MISMATCH"
		return deny(workspaceError(reason, "approval does not bind the exact authority, head, policy, and evidence", ErrSensitiveOperationDenied))
	}
	candidate, err := w.candidateLocked(approval.candidate)
	if err != nil || candidate.head != request.ExpectedHead {
		reason = "APPROVAL_HEAD_CHANGED"
		return deny(workspaceError(reason, "candidate head changed after approval", errors.Join(ErrSensitiveOperationDenied, ErrRevisionConflict)))
	}
	approval.used = true
	entry := w.appendSensitiveAuditLocked(actor, request.Operation, candidateSerial, request.PolicyRevision, evidenceHash, true, "approved evidence consumed")
	return SensitiveOperationReceipt{Sequence: entry.Sequence, Operation: request.Operation, Actor: actor, PolicyRevision: request.PolicyRevision, EvidenceHash: evidenceHash, AuditHash: entry.EventHash}, nil
}

func (w *Workspace) appendSensitiveAuditLocked(actor string, operation SensitiveOperation, candidate uint64, policy, evidence string, allowed bool, reason string) SensitiveAuditEntry {
	w.nextSensitiveAudit++
	entry := SensitiveAuditEntry{Sequence: w.nextSensitiveAudit, At: w.now().UTC(), Actor: actor, Operation: operation, CandidateSerial: candidate,
		PolicyRevision: policy, EvidenceHash: evidence, Allowed: allowed, Reason: reason, PreviousHash: w.sensitiveAuditRoot}
	entry.EventHash = sensitiveAuditEventHash(entry)
	w.sensitiveAuditRoot = entry.EventHash
	for len(w.sensitiveAudit) >= w.limits.MaxAuthorizationAudit {
		w.sensitiveAudit = w.sensitiveAudit[1:]
	}
	w.sensitiveAudit = append(w.sensitiveAudit, entry)
	return entry
}

func sensitiveAuditEventHash(entry SensitiveAuditEntry) string {
	payload, err := json.Marshal(struct {
		Sequence  uint64             `json:"sequence"`
		At        int64              `json:"at"`
		Actor     string             `json:"actor"`
		Operation SensitiveOperation `json:"operation"`
		Candidate uint64             `json:"candidate"`
		Policy    string             `json:"policy"`
		Evidence  string             `json:"evidence"`
		Allowed   bool               `json:"allowed"`
		Reason    string             `json:"reason"`
		Previous  string             `json:"previous"`
	}{entry.Sequence, entry.At.UnixNano(), entry.Actor, entry.Operation, entry.CandidateSerial, entry.PolicyRevision, entry.EvidenceHash, entry.Allowed, entry.Reason, entry.PreviousHash})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(append([]byte("paperd/sensitive-audit/v1\x00"), payload...))
	return hex.EncodeToString(sum[:])
}

func (w *Workspace) SensitiveOperationAudit(limit int) ([]SensitiveAuditEntry, error) {
	if w == nil {
		return nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if limit <= 0 || limit > w.limits.MaxAuthorizationAudit {
		return nil, workspaceError("AUDIT_LIMIT", "audit limit is outside configured bounds", ErrLimit)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	start := len(w.sensitiveAudit) - limit
	if start < 0 {
		start = 0
	}
	return append([]SensitiveAuditEntry(nil), w.sensitiveAudit[start:]...), nil
}

func canonicalSensitiveEvidence(evidence SensitiveEvidence, limit int) (SensitiveEvidence, string, error) {
	if !validSHA256(evidence.CandidateRevision) || !validSHA256(evidence.SourceDiffHash) || !validSHA256(evidence.SemanticDiffHash) || !validSHA256(evidence.OperationInputHash) {
		return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "candidate, diff, and operation-input evidence must be lowercase SHA-256 values", ErrInvalidQuery)
	}
	if len(evidence.RequiredScenarios) == 0 || len(evidence.Validators) == 0 || len(evidence.ReviewArtifacts) == 0 || len(evidence.RequiredScenarios)+len(evidence.Validators)+len(evidence.ReviewArtifacts) > limit {
		return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "approval requires bounded scenarios, validators, and review artifacts", ErrInvalidQuery)
	}
	result := evidence
	result.RequiredScenarios = append([]string(nil), evidence.RequiredScenarios...)
	result.Validators = append([]ValidatorEvidence(nil), evidence.Validators...)
	result.ReviewArtifacts = append([]ReviewArtifactEvidence(nil), evidence.ReviewArtifacts...)
	for _, scenario := range result.RequiredScenarios {
		if !validSensitiveLabel(scenario, limit) {
			return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "scenario identity is invalid", ErrInvalidQuery)
		}
	}
	for _, validator := range result.Validators {
		if !validSensitiveLabel(validator.Profile, limit) || !validSensitiveLabel(validator.Version, limit) || !validSHA256(validator.Hash) {
			return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "validator profile, version, or hash is invalid", ErrInvalidQuery)
		}
	}
	for _, artifact := range result.ReviewArtifacts {
		if !validSensitiveLabel(artifact.Kind, limit) || !validSHA256(artifact.Hash) {
			return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "review artifact kind or hash is invalid", ErrInvalidQuery)
		}
	}
	sort.Strings(result.RequiredScenarios)
	sort.Slice(result.Validators, func(i, j int) bool { return fmt.Sprint(result.Validators[i]) < fmt.Sprint(result.Validators[j]) })
	sort.Slice(result.ReviewArtifacts, func(i, j int) bool {
		return fmt.Sprint(result.ReviewArtifacts[i]) < fmt.Sprint(result.ReviewArtifacts[j])
	})
	if hasDuplicateStrings(result.RequiredScenarios) || hasDuplicateJSON(result.Validators) || hasDuplicateJSON(result.ReviewArtifacts) {
		return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "approval evidence contains duplicate identities", ErrInvalidQuery)
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return SensitiveEvidence{}, "", workspaceError("INVALID_APPROVAL_EVIDENCE", "approval evidence cannot be canonicalized", err)
	}
	sum := sha256.Sum256(append([]byte("paperd/approval-evidence/v1\x00"), payload...))
	return result, hex.EncodeToString(sum[:]), nil
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validSensitiveLabel(value string, limit int) bool {
	return value != "" && len(value) <= limit && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func hasDuplicateStrings(values []string) bool {
	for i := 1; i < len(values); i++ {
		if values[i] == values[i-1] {
			return true
		}
	}
	return false
}

func hasDuplicateJSON[T any](values []T) bool {
	for i := 1; i < len(values); i++ {
		if fmt.Sprint(values[i]) == fmt.Sprint(values[i-1]) {
			return true
		}
	}
	return false
}
