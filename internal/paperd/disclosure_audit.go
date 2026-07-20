// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// DisclosureAuditEntry is a bounded hash-only record of a denied disclosure
// attempt. Requested and Expected are domain-separated one-way hashes rather
// than the possibly sensitive raw disclosure labels.
type DisclosureAuditEntry struct {
	Sequence      uint64    `json:"sequence"`
	At            time.Time `json:"at"`
	Action        string    `json:"action"`
	RequestedHash string    `json:"requested_hash"`
	ExpectedHash  string    `json:"expected_hash,omitempty"`
	Reason        string    `json:"reason"`
}

func disclosureIdentityHash(value string) string {
	sum := sha256.Sum256([]byte("paperd/disclosure-identity/v1\x00" + value))
	return hex.EncodeToString(sum[:])
}

func (w *Workspace) recordDisclosureDenial(action string, requested DisclosureDomain, reason string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.nextDisclosureAudit++
	entry := DisclosureAuditEntry{Sequence: w.nextDisclosureAudit, At: w.now().UTC(), Action: action, RequestedHash: disclosureIdentityHash(string(requested)), ExpectedHash: disclosureIdentityHash(string(w.disclosureDomain)), Reason: reason}
	for len(w.disclosureAudit) >= w.limits.MaxAuthorizationAudit {
		copy(w.disclosureAudit, w.disclosureAudit[1:])
		w.disclosureAudit = w.disclosureAudit[:len(w.disclosureAudit)-1]
	}
	w.disclosureAudit = append(w.disclosureAudit, entry)
	sink := w.disclosureAuditSink
	w.mu.Unlock()
	emitDisclosureAudit(sink, entry)
}

func emitDisclosureAudit(sink func(DisclosureAuditEntry), entry DisclosureAuditEntry) {
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink(entry)
}

func (w *Workspace) DisclosureAudit(limit int) ([]DisclosureAuditEntry, error) {
	if w == nil || limit <= 0 || limit > w.limits.MaxAuthorizationAudit {
		return nil, workspaceError("DISCLOSURE_AUDIT_LIMIT", "disclosure audit limit is invalid", ErrLimit)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	start := len(w.disclosureAudit) - limit
	if start < 0 {
		start = 0
	}
	return append([]DisclosureAuditEntry(nil), w.disclosureAudit[start:]...), nil
}

func (w *Workspace) restoreDisclosureAudit(entries []DisclosureAuditEntry, next uint64) error {
	if len(entries) > w.limits.MaxAuthorizationAudit {
		return workspaceError("PERSISTENCE_DISCLOSURE_AUDIT", "persisted disclosure audit exceeds configured limits", ErrLimit)
	}
	var previous uint64
	for _, entry := range entries {
		if entry.Sequence == 0 || entry.Sequence <= previous || entry.At.IsZero() || entry.At.Location() != time.UTC || !validProtocolLabel(entry.Action) || !validProtocolLabel(entry.Reason) || !validSHA256(entry.RequestedHash) || !validSHA256(entry.ExpectedHash) {
			return workspaceError("PERSISTENCE_DISCLOSURE_AUDIT", "persisted disclosure audit is invalid", ErrPersistenceCorrupt)
		}
		previous = entry.Sequence
	}
	if (len(entries) == 0 && next != 0) || (len(entries) != 0 && next != entries[len(entries)-1].Sequence) {
		return workspaceError("PERSISTENCE_DISCLOSURE_AUDIT", "persisted disclosure audit sequence is invalid", ErrPersistenceCorrupt)
	}
	w.disclosureAudit = append([]DisclosureAuditEntry(nil), entries...)
	w.nextDisclosureAudit = next
	return nil
}
