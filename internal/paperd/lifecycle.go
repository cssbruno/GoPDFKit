// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

type DisclosureDomain string

const (
	DisclosureProject    DisclosureDomain = "project"
	DisclosureRestricted DisclosureDomain = "restricted"
	DisclosurePublic     DisclosureDomain = "public"
)

type handleKind uint8

const (
	handleRevision handleKind = iota + 1
	handleCandidate
	handleScenarioRevision
	handleScenarioCandidate
	handleSemanticTemplateRevision
	handleSemanticTemplateCandidate
	handlePolicyRevision
	handlePolicyCandidate
	handleMutationAuthority
	handlePlan
	handleOpen
	handleSensitiveAuthority
	handleSensitiveApproval
)

type handleCapability uint8

const (
	capabilityRead handleCapability = iota + 1
	capabilityEdit
	capabilityRender
	capabilityAuthorize
	capabilityExport
	capabilityPublish
	capabilityAttachment
	capabilityProductionCapture
	capabilitySign
	capabilityApprove
	capabilityAccept
)

type revocationReason uint8

const (
	revokedExplicitly revocationReason = iota + 1
	revokedExpired
)

type revocationRecord struct {
	reason revocationReason
	at     time.Time
}

var nextHandleNonce atomic.Uint64

func normalizeDisclosureDomain(domain DisclosureDomain) (DisclosureDomain, uint64, error) {
	if domain == "" {
		domain = DisclosureProject
	}
	if !utf8.ValidString(string(domain)) || len(domain) > MaxQueryBytesHard || strings.TrimSpace(string(domain)) != string(domain) {
		return "", 0, workspaceError("INVALID_DISCLOSURE_DOMAIN", "disclosure domain must be bounded valid UTF-8 without surrounding whitespace", ErrDisclosureDenied)
	}
	sum := sha256.Sum256([]byte("paperd/disclosure/v1\x00" + string(domain)))
	return domain, binary.BigEndian.Uint64(sum[:8]), nil
}

func (w *Workspace) newHandle(kind handleKind, capability handleCapability, serial uint64) scopedHandle {
	return scopedHandle{
		scope: w.scope, serial: serial, nonce: nextHandleNonce.Add(1),
		domain: w.disclosureTag, kind: kind, capability: capability,
	}
}

func (w *Workspace) expiresAt(ttl time.Duration) time.Time { return w.now().Add(ttl) }

func (w *Workspace) validateHandle(value scopedHandle, kind handleKind, capability handleCapability, allowEitherOpenMode bool) error {
	if value.serial == 0 || value.nonce == 0 {
		return workspaceError("INVALID_HANDLE", "handle is unavailable", ErrInvalidHandle)
	}
	if value.scope != w.scope || value.domain != w.disclosureTag {
		return workspaceError("WRONG_WORKSPACE", "handle is unavailable", ErrWrongWorkspace)
	}
	if value.kind != kind || (!allowEitherOpenMode && value.capability != capability) ||
		(allowEitherOpenMode && value.capability != capabilityRead && value.capability != capabilityEdit) {
		return workspaceError("INVALID_HANDLE", "handle is unavailable", ErrInvalidHandle)
	}
	return nil
}

func (w *Workspace) unavailableHandle(value scopedHandle, notFound error) error {
	if tombstone, ok := w.revocations[value]; ok {
		if tombstone.reason == revokedExpired {
			compatibility := notFound
			if value.kind == handlePlan {
				return workspaceError("HANDLE_EXPIRED", "handle is unavailable", errors.Join(ErrHandleExpired, ErrPlanExpired, ErrPlanNotFound))
			}
			return workspaceError("HANDLE_EXPIRED", "handle is unavailable", errors.Join(ErrHandleExpired, compatibility))
		}
		return workspaceError("HANDLE_REVOKED", "handle is unavailable", errors.Join(ErrHandleRevoked, notFound))
	}
	return workspaceError("HANDLE_NOT_FOUND", "handle is unavailable", notFound)
}

func (w *Workspace) ensureLive(value scopedHandle, expires time.Time) error {
	if !expires.After(w.now()) {
		return workspaceError("HANDLE_EXPIRED", "handle is unavailable", ErrHandleExpired)
	}
	return nil
}

func (w *Workspace) recordRevocationLocked(value scopedHandle, reason revocationReason, now time.Time) {
	if _, exists := w.revocations[value]; exists {
		return
	}
	for len(w.revocationOrder) >= w.limits.MaxRevocations {
		oldest := w.revocationOrder[0]
		w.revocationOrder = w.revocationOrder[1:]
		delete(w.revocations, oldest)
	}
	w.revocations[value] = revocationRecord{reason: reason, at: now}
	w.revocationOrder = append(w.revocationOrder, value)
}

// PruneExpiredHandles reclaims every expired retained capability family and
// records bounded expiry tombstones. Operations that already acquired an
// immutable record may finish; later lookups fail deterministically.
func (w *Workspace) PruneExpiredHandles() int {
	if w == nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.pruneExpiredHandlesLocked(w.now())
}

func (w *Workspace) pruneExpiredHandlesLocked(now time.Time) int {
	removed := 0
	prune := func(value scopedHandle, expires time.Time, remove func()) {
		if expires.After(now) {
			return
		}
		remove()
		w.recordRevocationLocked(value, revokedExpired, now)
		removed++
	}
	for serial, record := range w.revisions {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.revisions, serial) })
	}
	for serial, record := range w.candidates {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.candidates, serial) })
	}
	for serial, record := range w.scenarioRevisions {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.scenarioRevisions, serial) })
	}
	for serial, record := range w.scenarioCandidates {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.scenarioCandidates, serial) })
	}
	for serial, record := range w.semanticTemplateRevisions {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.semanticTemplateRevisions, serial) })
	}
	for serial, record := range w.semanticTemplateCandidates {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.semanticTemplateCandidates, serial) })
	}
	for serial, record := range w.policyRevisions {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.policyRevisions, serial) })
	}
	for serial, record := range w.policyCandidates {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.policyCandidates, serial) })
	}
	for serial, record := range w.mutationAuthorities {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.mutationAuthorities, serial) })
	}
	for serial, record := range w.sensitiveAuthorities {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.sensitiveAuthorities, serial) })
	}
	for serial, record := range w.sensitiveApprovals {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.sensitiveApprovals, serial) })
	}
	for serial, record := range w.plans {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.plans, serial) })
	}
	for serial, record := range w.opens {
		serial, record := serial, record
		prune(record.handle.value, record.expires, func() { delete(w.opens, serial) })
	}
	return removed
}

func (w *Workspace) RevokeRevision(handle RevisionHandle) error {
	return w.revoke(handle.value, handleRevision, capabilityRead, func() { delete(w.revisions, handle.value.serial) })
}

func (w *Workspace) RevokeCandidate(handle CandidateHandle) error {
	return w.revoke(handle.value, handleCandidate, capabilityEdit, func() { delete(w.candidates, handle.value.serial) })
}

func (w *Workspace) RevokeScenarioRevision(handle ScenarioRevisionHandle) error {
	return w.revoke(handle.value, handleScenarioRevision, capabilityRead, func() { delete(w.scenarioRevisions, handle.value.serial) })
}

func (w *Workspace) RevokeScenarioCandidate(handle ScenarioCandidateHandle) error {
	return w.revoke(handle.value, handleScenarioCandidate, capabilityEdit, func() { delete(w.scenarioCandidates, handle.value.serial) })
}

func (w *Workspace) RevokeSemanticTemplateRevision(handle SemanticTemplateRevisionHandle) error {
	return w.revoke(handle.value, handleSemanticTemplateRevision, capabilityRead, func() { delete(w.semanticTemplateRevisions, handle.value.serial) })
}

func (w *Workspace) RevokeSemanticTemplateCandidate(handle SemanticTemplateCandidateHandle) error {
	return w.revoke(handle.value, handleSemanticTemplateCandidate, capabilityEdit, func() { delete(w.semanticTemplateCandidates, handle.value.serial) })
}

func (w *Workspace) RevokePolicyRevision(handle PolicyRevisionHandle) error {
	return w.revoke(handle.value, handlePolicyRevision, capabilityRead, func() { delete(w.policyRevisions, handle.value.serial) })
}

func (w *Workspace) RevokePolicyCandidate(handle PolicyCandidateHandle) error {
	return w.revoke(handle.value, handlePolicyCandidate, capabilityEdit, func() { delete(w.policyCandidates, handle.value.serial) })
}

func (w *Workspace) RevokeMutationAuthority(handle MutationAuthorityHandle) error {
	return w.revoke(handle.value, handleMutationAuthority, capabilityAuthorize, func() { delete(w.mutationAuthorities, handle.value.serial) })
}

func (w *Workspace) RevokeSensitiveAuthority(handle SensitiveAuthorityHandle) error {
	return w.revoke(handle.value, handleSensitiveAuthority, handle.value.capability, func() { delete(w.sensitiveAuthorities, handle.value.serial) })
}

func (w *Workspace) RevokeSensitiveApproval(handle SensitiveApprovalHandle) error {
	return w.revoke(handle.value, handleSensitiveApproval, capabilityApprove, func() { delete(w.sensitiveApprovals, handle.value.serial) })
}

func (w *Workspace) revoke(value scopedHandle, kind handleKind, capability handleCapability, remove func()) error {
	if w == nil {
		return workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if err := w.validateHandle(value, kind, capability, false); err != nil {
		return err
	}
	if _, revoked := w.revocations[value]; revoked {
		return w.unavailableHandle(value, ErrInvalidHandle)
	}
	// Callers validate record existence before invoking this helper through the
	// family-specific public method's central lookup.
	switch kind {
	case handleRevision:
		if record := w.revisions[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrRevisionNotFound)
		}
	case handleCandidate:
		if record := w.candidates[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrCandidateNotFound)
		}
	case handleScenarioRevision:
		if record := w.scenarioRevisions[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrRevisionNotFound)
		}
	case handleScenarioCandidate:
		if record := w.scenarioCandidates[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrCandidateNotFound)
		}
	case handleSemanticTemplateRevision:
		if record := w.semanticTemplateRevisions[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrSemanticTemplateRevisionNotFound)
		}
	case handleSemanticTemplateCandidate:
		if record := w.semanticTemplateCandidates[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrSemanticTemplateCandidateNotFound)
		}
	case handlePolicyRevision:
		if record := w.policyRevisions[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrPolicyRevisionNotFound)
		}
	case handlePolicyCandidate:
		if record := w.policyCandidates[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrPolicyCandidateNotFound)
		}
	case handleMutationAuthority:
		if record := w.mutationAuthorities[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrMutationAuthorityNotFound)
		}
	case handleSensitiveAuthority:
		if record := w.sensitiveAuthorities[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrSensitiveAuthorityNotFound)
		}
	case handleSensitiveApproval:
		if record := w.sensitiveApprovals[value.serial]; record == nil || record.handle.value != value || !w.ownsPartition(record.partition) {
			return w.unavailableHandle(value, ErrSensitiveApprovalNotFound)
		}
	}
	remove()
	w.recordRevocationLocked(value, revokedExplicitly, w.now())
	return nil
}
