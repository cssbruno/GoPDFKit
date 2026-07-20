// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrSemanticTemplateRevisionNotFound  = workspaceError("SEMANTIC_TEMPLATE_REVISION_NOT_FOUND", "semantic-template revision handle is not retained", ErrRevisionNotFound)
	ErrSemanticTemplateCandidateNotFound = workspaceError("SEMANTIC_TEMPLATE_CANDIDATE_NOT_FOUND", "semantic-template candidate handle is not retained", ErrCandidateNotFound)
	ErrPolicyRevisionNotFound            = workspaceError("POLICY_REVISION_NOT_FOUND", "policy revision handle is not retained", ErrRevisionNotFound)
	ErrPolicyCandidateNotFound           = workspaceError("POLICY_CANDIDATE_NOT_FOUND", "policy candidate handle is not retained", ErrCandidateNotFound)
)

type SemanticTemplateRevisionSnapshot struct {
	Handle           SemanticTemplateRevisionHandle `json:"-"`
	Content          string                         `json:"content"`
	Digest           string                         `json:"digest"`
	Bytes            int                            `json:"bytes"`
	Capability       CapabilityMode                 `json:"capability"`
	DisclosureDomain DisclosureDomain               `json:"disclosure_domain"`
	ExpiresAt        time.Time                      `json:"expires_at"`
}

type SemanticTemplateCandidateSnapshot struct {
	Handle           SemanticTemplateCandidateHandle `json:"-"`
	Head             SemanticTemplateRevisionHandle  `json:"-"`
	HeadDigest       string                          `json:"head_digest"`
	Capability       CapabilityMode                  `json:"capability"`
	DisclosureDomain DisclosureDomain                `json:"disclosure_domain"`
	ExpiresAt        time.Time                       `json:"expires_at"`
}

type SemanticTemplateApplyRequest struct {
	Candidate      SemanticTemplateCandidateHandle
	ExpectedHead   SemanticTemplateRevisionHandle
	ExpectedDigest string
	IdempotencyKey string
	Content        string
}

type SemanticTemplateApplyResult struct {
	Candidate      SemanticTemplateCandidateSnapshot `json:"candidate"`
	Revision       SemanticTemplateRevisionSnapshot  `json:"revision"`
	IdempotencyKey string                            `json:"idempotency_key"`
}

type semanticTemplateIdempotencyRecord struct {
	fingerprint string
	result      SemanticTemplateApplyResult
}

type PolicyRevisionSnapshot struct {
	Handle           PolicyRevisionHandle `json:"-"`
	Content          string               `json:"content"`
	Digest           string               `json:"digest"`
	Bytes            int                  `json:"bytes"`
	Capability       CapabilityMode       `json:"capability"`
	DisclosureDomain DisclosureDomain     `json:"disclosure_domain"`
	ExpiresAt        time.Time            `json:"expires_at"`
}

type PolicyCandidateSnapshot struct {
	Handle           PolicyCandidateHandle `json:"-"`
	Head             PolicyRevisionHandle  `json:"-"`
	HeadDigest       string                `json:"head_digest"`
	Capability       CapabilityMode        `json:"capability"`
	DisclosureDomain DisclosureDomain      `json:"disclosure_domain"`
	ExpiresAt        time.Time             `json:"expires_at"`
}

type PolicyApplyRequest struct {
	Candidate      PolicyCandidateHandle
	ExpectedHead   PolicyRevisionHandle
	ExpectedDigest string
	IdempotencyKey string
	Content        string
}

type PolicyApplyResult struct {
	Candidate      PolicyCandidateSnapshot `json:"candidate"`
	Revision       PolicyRevisionSnapshot  `json:"revision"`
	IdempotencyKey string                  `json:"idempotency_key"`
}

type policyIdempotencyRecord struct {
	fingerprint string
	result      PolicyApplyResult
}

func validateRevisionDomainContent(domain, content string, limit int) (string, string, error) {
	if content == "" || len(content) > limit || !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
		return "", "", workspaceError("DOMAIN_CONTENT_LIMIT", domain+" content must be non-empty bounded UTF-8 without NUL", ErrLimit)
	}
	sum := sha256.Sum256([]byte("paperd/" + domain + "/v1\x00" + content))
	return content, hex.EncodeToString(sum[:]), nil
}

func validateDomainApply(idempotencyKey, expectedDigest string, maxKey int) error {
	if idempotencyKey == "" || len(idempotencyKey) > maxKey || !utf8.ValidString(idempotencyKey) {
		return workspaceError("INVALID_IDEMPOTENCY_KEY", "domain idempotency key is invalid or exceeds configured bounds", ErrInvalidQuery)
	}
	if len(expectedDigest) != sha256.Size*2 {
		return workspaceError("REVISION_CONFLICT", "expected domain digest is malformed", ErrRevisionConflict)
	}
	if _, err := hex.DecodeString(expectedDigest); err != nil {
		return workspaceError("REVISION_CONFLICT", "expected domain digest is malformed", ErrRevisionConflict)
	}
	return nil
}

func domainRequestFingerprint(domain string, candidate, head scopedHandle, digest, content string) string {
	contentSum := sha256.Sum256([]byte(content))
	encoded, err := json.Marshal(struct {
		Domain         string `json:"domain"`
		CandidateScope uint64 `json:"candidate_scope"`
		Candidate      uint64 `json:"candidate"`
		CandidateNonce uint64 `json:"candidate_nonce"`
		HeadScope      uint64 `json:"head_scope"`
		Head           uint64 `json:"head"`
		HeadNonce      uint64 `json:"head_nonce"`
		Digest         string `json:"digest"`
		ContentSHA256  string `json:"content_sha256"`
	}{domain, candidate.scope, candidate.serial, candidate.nonce, head.scope, head.serial, head.nonce, digest, hex.EncodeToString(contentSum[:])})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (w *Workspace) CreateSemanticTemplateRevision(content string) (SemanticTemplateRevisionSnapshot, error) {
	if w == nil {
		return SemanticTemplateRevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	content, digest, err := validateRevisionDomainContent("semantic-template", content, w.limits.MaxSourceBytes)
	if err != nil {
		return SemanticTemplateRevisionSnapshot{}, err
	}
	record := &semanticTemplateRevisionRecord{content: content, digest: digest}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if len(w.semanticTemplateRevisions) >= w.limits.MaxSemanticTemplateRevisions {
		return SemanticTemplateRevisionSnapshot{}, workspaceError("SEMANTIC_TEMPLATE_REVISION_LIMIT", "semantic-template revision capacity is exhausted", ErrLimit)
	}
	w.nextSemanticTemplateRevision++
	record.handle = SemanticTemplateRevisionHandle{value: w.newHandle(handleSemanticTemplateRevision, capabilityRead, w.nextSemanticTemplateRevision)}
	record.expires, record.disclosure, record.partition = w.expiresAt(w.handleTTL), w.disclosureDomain, w.partition
	w.semanticTemplateRevisions[w.nextSemanticTemplateRevision] = record
	return semanticTemplateRevisionSnapshot(record), nil
}

func (w *Workspace) OpenSemanticTemplateRevision(handle SemanticTemplateRevisionHandle) (SemanticTemplateRevisionSnapshot, error) {
	if w == nil {
		return SemanticTemplateRevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.semanticTemplateRevisionLocked(handle)
	if err != nil {
		return SemanticTemplateRevisionSnapshot{}, err
	}
	return semanticTemplateRevisionSnapshot(record), nil
}

func (w *Workspace) NewSemanticTemplateCandidate(base SemanticTemplateRevisionHandle) (SemanticTemplateCandidateSnapshot, error) {
	if w == nil {
		return SemanticTemplateCandidateSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	revision, err := w.semanticTemplateRevisionLocked(base)
	if err != nil {
		return SemanticTemplateCandidateSnapshot{}, err
	}
	if len(w.semanticTemplateCandidates) >= w.limits.MaxSemanticTemplateCandidates {
		return SemanticTemplateCandidateSnapshot{}, workspaceError("SEMANTIC_TEMPLATE_CANDIDATE_LIMIT", "semantic-template candidate capacity is exhausted", ErrLimit)
	}
	w.nextSemanticTemplateCandidate++
	handle := SemanticTemplateCandidateHandle{value: w.newHandle(handleSemanticTemplateCandidate, capabilityEdit, w.nextSemanticTemplateCandidate)}
	record := &semanticTemplateCandidateRecord{handle: handle, head: base, idempotency: make(map[string]semanticTemplateIdempotencyRecord), expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.semanticTemplateCandidates[w.nextSemanticTemplateCandidate] = record
	return semanticTemplateCandidateSnapshot(record, revision), nil
}

func (w *Workspace) SemanticTemplateCandidate(handle SemanticTemplateCandidateHandle) (SemanticTemplateCandidateSnapshot, error) {
	if w == nil {
		return SemanticTemplateCandidateSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	candidate, err := w.semanticTemplateCandidateLocked(handle)
	if err != nil {
		return SemanticTemplateCandidateSnapshot{}, err
	}
	revision, err := w.semanticTemplateRevisionLocked(candidate.head)
	if err != nil {
		return SemanticTemplateCandidateSnapshot{}, err
	}
	return semanticTemplateCandidateSnapshot(candidate, revision), nil
}

func (w *Workspace) ApplySemanticTemplate(request SemanticTemplateApplyRequest) (SemanticTemplateApplyResult, error) {
	if w == nil {
		return SemanticTemplateApplyResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := validateDomainApply(request.IdempotencyKey, request.ExpectedDigest, w.limits.MaxQueryBytes); err != nil {
		return SemanticTemplateApplyResult{}, err
	}
	content, digest, err := validateRevisionDomainContent("semantic-template", request.Content, w.limits.MaxSourceBytes)
	if err != nil {
		return SemanticTemplateApplyResult{}, err
	}
	fingerprint := domainRequestFingerprint("semantic-template", request.Candidate.value, request.ExpectedHead.value, request.ExpectedDigest, content)
	w.mu.RLock()
	candidate, err := w.semanticTemplateCandidateLocked(request.Candidate)
	if err != nil {
		w.mu.RUnlock()
		return SemanticTemplateApplyResult{}, err
	}
	if cached, ok := candidate.idempotency[request.IdempotencyKey]; ok {
		w.mu.RUnlock()
		return semanticTemplateCached(cached, fingerprint)
	}
	base, err := w.semanticTemplateRevisionLocked(request.ExpectedHead)
	if err != nil {
		w.mu.RUnlock()
		return SemanticTemplateApplyResult{}, err
	}
	if candidate.head != request.ExpectedHead || base.digest != request.ExpectedDigest {
		w.mu.RUnlock()
		return SemanticTemplateApplyResult{}, workspaceError("SEMANTIC_TEMPLATE_REVISION_CONFLICT", "semantic-template candidate head or digest changed", ErrRevisionConflict)
	}
	w.mu.RUnlock()
	prepared := &semanticTemplateRevisionRecord{content: content, digest: digest}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	candidate, err = w.semanticTemplateCandidateLocked(request.Candidate)
	if err != nil {
		return SemanticTemplateApplyResult{}, err
	}
	if cached, ok := candidate.idempotency[request.IdempotencyKey]; ok {
		return semanticTemplateCached(cached, fingerprint)
	}
	if candidate.head != request.ExpectedHead {
		return SemanticTemplateApplyResult{}, workspaceError("SEMANTIC_TEMPLATE_REVISION_CONFLICT", "semantic-template candidate head changed", ErrRevisionConflict)
	}
	if len(w.semanticTemplateRevisions) >= w.limits.MaxSemanticTemplateRevisions {
		return SemanticTemplateApplyResult{}, workspaceError("SEMANTIC_TEMPLATE_REVISION_LIMIT", "semantic-template revision capacity is exhausted", ErrLimit)
	}
	w.nextSemanticTemplateRevision++
	prepared.handle = SemanticTemplateRevisionHandle{value: w.newHandle(handleSemanticTemplateRevision, capabilityRead, w.nextSemanticTemplateRevision)}
	prepared.expires, prepared.disclosure, prepared.partition = w.expiresAt(w.handleTTL), w.disclosureDomain, w.partition
	w.semanticTemplateRevisions[w.nextSemanticTemplateRevision] = prepared
	candidate.head = prepared.handle
	result := SemanticTemplateApplyResult{Candidate: semanticTemplateCandidateSnapshot(candidate, prepared), Revision: semanticTemplateRevisionSnapshot(prepared), IdempotencyKey: request.IdempotencyKey}
	candidate.idempotency[request.IdempotencyKey] = semanticTemplateIdempotencyRecord{fingerprint: fingerprint, result: result}
	return result, nil
}

func (w *Workspace) semanticTemplateRevisionLocked(handle SemanticTemplateRevisionHandle) (*semanticTemplateRevisionRecord, error) {
	if err := w.validateHandle(handle.value, handleSemanticTemplateRevision, capabilityRead, false); err != nil {
		return nil, err
	}
	record := w.semanticTemplateRevisions[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrSemanticTemplateRevisionNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}
func (w *Workspace) semanticTemplateCandidateLocked(handle SemanticTemplateCandidateHandle) (*semanticTemplateCandidateRecord, error) {
	if err := w.validateHandle(handle.value, handleSemanticTemplateCandidate, capabilityEdit, false); err != nil {
		return nil, err
	}
	record := w.semanticTemplateCandidates[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrSemanticTemplateCandidateNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}
func semanticTemplateRevisionSnapshot(r *semanticTemplateRevisionRecord) SemanticTemplateRevisionSnapshot {
	return SemanticTemplateRevisionSnapshot{Handle: r.handle, Content: r.content, Digest: r.digest, Bytes: len(r.content), Capability: CapabilityRead, DisclosureDomain: r.disclosure, ExpiresAt: r.expires}
}
func semanticTemplateCandidateSnapshot(c *semanticTemplateCandidateRecord, r *semanticTemplateRevisionRecord) SemanticTemplateCandidateSnapshot {
	return SemanticTemplateCandidateSnapshot{Handle: c.handle, Head: c.head, HeadDigest: r.digest, Capability: CapabilityEdit, DisclosureDomain: c.disclosure, ExpiresAt: c.expires}
}
func semanticTemplateCached(c semanticTemplateIdempotencyRecord, f string) (SemanticTemplateApplyResult, error) {
	if c.fingerprint != f {
		return SemanticTemplateApplyResult{}, workspaceError("SEMANTIC_TEMPLATE_IDEMPOTENCY_CONFLICT", "idempotency key was already used for another semantic-template request", ErrRevisionConflict)
	}
	return c.result, nil
}

func (w *Workspace) CreatePolicyRevision(content string) (PolicyRevisionSnapshot, error) {
	if w == nil {
		return PolicyRevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	content, digest, err := validateRevisionDomainContent("policy", content, w.limits.MaxSourceBytes)
	if err != nil {
		return PolicyRevisionSnapshot{}, err
	}
	record := &policyRevisionRecord{content: content, digest: digest}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if len(w.policyRevisions) >= w.limits.MaxPolicyRevisions {
		return PolicyRevisionSnapshot{}, workspaceError("POLICY_REVISION_LIMIT", "policy revision capacity is exhausted", ErrLimit)
	}
	w.nextPolicyRevision++
	record.handle = PolicyRevisionHandle{value: w.newHandle(handlePolicyRevision, capabilityRead, w.nextPolicyRevision)}
	record.expires, record.disclosure, record.partition = w.expiresAt(w.handleTTL), w.disclosureDomain, w.partition
	w.policyRevisions[w.nextPolicyRevision] = record
	return policyRevisionSnapshot(record), nil
}
func (w *Workspace) OpenPolicyRevision(handle PolicyRevisionHandle) (PolicyRevisionSnapshot, error) {
	if w == nil {
		return PolicyRevisionSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.policyRevisionLocked(handle)
	if err != nil {
		return PolicyRevisionSnapshot{}, err
	}
	return policyRevisionSnapshot(record), nil
}
func (w *Workspace) NewPolicyCandidate(base PolicyRevisionHandle) (PolicyCandidateSnapshot, error) {
	if w == nil {
		return PolicyCandidateSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	revision, err := w.policyRevisionLocked(base)
	if err != nil {
		return PolicyCandidateSnapshot{}, err
	}
	if len(w.policyCandidates) >= w.limits.MaxPolicyCandidates {
		return PolicyCandidateSnapshot{}, workspaceError("POLICY_CANDIDATE_LIMIT", "policy candidate capacity is exhausted", ErrLimit)
	}
	w.nextPolicyCandidate++
	handle := PolicyCandidateHandle{value: w.newHandle(handlePolicyCandidate, capabilityEdit, w.nextPolicyCandidate)}
	record := &policyCandidateRecord{handle: handle, head: base, idempotency: make(map[string]policyIdempotencyRecord), expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.policyCandidates[w.nextPolicyCandidate] = record
	return policyCandidateSnapshot(record, revision), nil
}
func (w *Workspace) PolicyCandidate(handle PolicyCandidateHandle) (PolicyCandidateSnapshot, error) {
	if w == nil {
		return PolicyCandidateSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	candidate, err := w.policyCandidateLocked(handle)
	if err != nil {
		return PolicyCandidateSnapshot{}, err
	}
	revision, err := w.policyRevisionLocked(candidate.head)
	if err != nil {
		return PolicyCandidateSnapshot{}, err
	}
	return policyCandidateSnapshot(candidate, revision), nil
}
func (w *Workspace) ApplyPolicy(request PolicyApplyRequest) (PolicyApplyResult, error) {
	if w == nil {
		return PolicyApplyResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := validateDomainApply(request.IdempotencyKey, request.ExpectedDigest, w.limits.MaxQueryBytes); err != nil {
		return PolicyApplyResult{}, err
	}
	content, digest, err := validateRevisionDomainContent("policy", request.Content, w.limits.MaxSourceBytes)
	if err != nil {
		return PolicyApplyResult{}, err
	}
	fingerprint := domainRequestFingerprint("policy", request.Candidate.value, request.ExpectedHead.value, request.ExpectedDigest, content)
	w.mu.RLock()
	candidate, err := w.policyCandidateLocked(request.Candidate)
	if err != nil {
		w.mu.RUnlock()
		return PolicyApplyResult{}, err
	}
	if cached, ok := candidate.idempotency[request.IdempotencyKey]; ok {
		w.mu.RUnlock()
		return policyCached(cached, fingerprint)
	}
	base, err := w.policyRevisionLocked(request.ExpectedHead)
	if err != nil {
		w.mu.RUnlock()
		return PolicyApplyResult{}, err
	}
	if candidate.head != request.ExpectedHead || base.digest != request.ExpectedDigest {
		w.mu.RUnlock()
		return PolicyApplyResult{}, workspaceError("POLICY_REVISION_CONFLICT", "policy candidate head or digest changed", ErrRevisionConflict)
	}
	w.mu.RUnlock()
	prepared := &policyRevisionRecord{content: content, digest: digest}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	candidate, err = w.policyCandidateLocked(request.Candidate)
	if err != nil {
		return PolicyApplyResult{}, err
	}
	if cached, ok := candidate.idempotency[request.IdempotencyKey]; ok {
		return policyCached(cached, fingerprint)
	}
	if candidate.head != request.ExpectedHead {
		return PolicyApplyResult{}, workspaceError("POLICY_REVISION_CONFLICT", "policy candidate head changed", ErrRevisionConflict)
	}
	if len(w.policyRevisions) >= w.limits.MaxPolicyRevisions {
		return PolicyApplyResult{}, workspaceError("POLICY_REVISION_LIMIT", "policy revision capacity is exhausted", ErrLimit)
	}
	w.nextPolicyRevision++
	prepared.handle = PolicyRevisionHandle{value: w.newHandle(handlePolicyRevision, capabilityRead, w.nextPolicyRevision)}
	prepared.expires, prepared.disclosure, prepared.partition = w.expiresAt(w.handleTTL), w.disclosureDomain, w.partition
	w.policyRevisions[w.nextPolicyRevision] = prepared
	candidate.head = prepared.handle
	result := PolicyApplyResult{Candidate: policyCandidateSnapshot(candidate, prepared), Revision: policyRevisionSnapshot(prepared), IdempotencyKey: request.IdempotencyKey}
	candidate.idempotency[request.IdempotencyKey] = policyIdempotencyRecord{fingerprint: fingerprint, result: result}
	return result, nil
}
func (w *Workspace) policyRevisionLocked(handle PolicyRevisionHandle) (*policyRevisionRecord, error) {
	if err := w.validateHandle(handle.value, handlePolicyRevision, capabilityRead, false); err != nil {
		return nil, err
	}
	record := w.policyRevisions[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrPolicyRevisionNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}
func (w *Workspace) policyCandidateLocked(handle PolicyCandidateHandle) (*policyCandidateRecord, error) {
	if err := w.validateHandle(handle.value, handlePolicyCandidate, capabilityEdit, false); err != nil {
		return nil, err
	}
	record := w.policyCandidates[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrPolicyCandidateNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}
func policyRevisionSnapshot(r *policyRevisionRecord) PolicyRevisionSnapshot {
	return PolicyRevisionSnapshot{Handle: r.handle, Content: r.content, Digest: r.digest, Bytes: len(r.content), Capability: CapabilityRead, DisclosureDomain: r.disclosure, ExpiresAt: r.expires}
}
func policyCandidateSnapshot(c *policyCandidateRecord, r *policyRevisionRecord) PolicyCandidateSnapshot {
	return PolicyCandidateSnapshot{Handle: c.handle, Head: c.head, HeadDigest: r.digest, Capability: CapabilityEdit, DisclosureDomain: c.disclosure, ExpiresAt: c.expires}
}
func policyCached(c policyIdempotencyRecord, f string) (PolicyApplyResult, error) {
	if c.fingerprint != f {
		return PolicyApplyResult{}, workspaceError("POLICY_IDEMPOTENCY_CONFLICT", "idempotency key was already used for another policy request", ErrRevisionConflict)
	}
	return c.result, nil
}
