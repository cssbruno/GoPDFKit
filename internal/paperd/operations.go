// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import "sort"

// OperationalCapacity is a privacy-safe retained-state gauge. It exposes no
// source, identity, handle, policy, disclosure, hash, or protected-node data.
type OperationalCapacity struct {
	Current int `json:"current"`
	Limit   int `json:"limit"`
}

// OperationalSnapshot is suitable for pull-based health monitoring. The
// snapshot is detached and intentionally contains only bounded aggregate
// counts. Saturated is sorted and identifies capacities that cannot retain one
// more item without pruning or rejecting work.
type OperationalSnapshot struct {
	Revisions             OperationalCapacity `json:"revisions"`
	Candidates            OperationalCapacity `json:"candidates"`
	Plans                 OperationalCapacity `json:"plans"`
	OpenDocuments         OperationalCapacity `json:"open_documents"`
	MutationAuthorities   OperationalCapacity `json:"mutation_authorities"`
	SensitiveAuthorities  OperationalCapacity `json:"sensitive_authorities"`
	SensitiveApprovals    OperationalCapacity `json:"sensitive_approvals"`
	Revocations           OperationalCapacity `json:"revocations"`
	AuthorizationAudit    OperationalCapacity `json:"authorization_audit"`
	DisclosureAudit       OperationalCapacity `json:"disclosure_audit"`
	SensitiveAudit        OperationalCapacity `json:"sensitive_audit"`
	AuditAnchors          OperationalCapacity `json:"audit_anchors"`
	PersistenceGeneration uint64              `json:"persistence_generation"`
	Saturated             []string            `json:"saturated,omitempty"`
}

// OperationalSnapshot returns one internally consistent aggregate view after
// pruning expired capabilities. It performs no I/O and is safe for concurrent
// use with workspace operations.
func (w *Workspace) OperationalSnapshot() OperationalSnapshot {
	if w == nil {
		return OperationalSnapshot{}
	}
	w.mu.Lock()
	w.pruneExpiredHandlesLocked(w.now())
	result := OperationalSnapshot{
		Revisions:             OperationalCapacity{Current: len(w.revisions), Limit: w.limits.MaxRevisions},
		Candidates:            OperationalCapacity{Current: len(w.candidates), Limit: w.limits.MaxCandidates},
		Plans:                 OperationalCapacity{Current: len(w.plans), Limit: w.limits.MaxPlans},
		OpenDocuments:         OperationalCapacity{Current: len(w.opens), Limit: w.limits.MaxOpenDocuments},
		MutationAuthorities:   OperationalCapacity{Current: len(w.mutationAuthorities), Limit: w.limits.MaxMutationAuthorities},
		SensitiveAuthorities:  OperationalCapacity{Current: len(w.sensitiveAuthorities), Limit: w.limits.MaxMutationAuthorities},
		SensitiveApprovals:    OperationalCapacity{Current: len(w.sensitiveApprovals), Limit: w.limits.MaxMutationAuthorities},
		Revocations:           OperationalCapacity{Current: len(w.revocations), Limit: w.limits.MaxRevocations},
		AuthorizationAudit:    OperationalCapacity{Current: len(w.authorizationAudit), Limit: w.limits.MaxAuthorizationAudit},
		DisclosureAudit:       OperationalCapacity{Current: len(w.disclosureAudit), Limit: w.limits.MaxAuthorizationAudit},
		SensitiveAudit:        OperationalCapacity{Current: len(w.sensitiveAudit), Limit: w.limits.MaxAuthorizationAudit},
		AuditAnchors:          OperationalCapacity{Current: len(w.sensitiveAuditAnchors), Limit: w.limits.MaxAuthorizationAudit},
		PersistenceGeneration: w.persistenceGeneration,
	}
	w.mu.Unlock()

	capacities := []struct {
		name  string
		value OperationalCapacity
	}{
		{"audit_anchors", result.AuditAnchors},
		{"authorization_audit", result.AuthorizationAudit},
		{"candidates", result.Candidates},
		{"disclosure_audit", result.DisclosureAudit},
		{"mutation_authorities", result.MutationAuthorities},
		{"open_documents", result.OpenDocuments},
		{"plans", result.Plans},
		{"revisions", result.Revisions},
		{"revocations", result.Revocations},
		{"sensitive_approvals", result.SensitiveApprovals},
		{"sensitive_audit", result.SensitiveAudit},
		{"sensitive_authorities", result.SensitiveAuthorities},
	}
	for _, capacity := range capacities {
		if capacity.value.Limit > 0 && capacity.value.Current >= capacity.value.Limit {
			result.Saturated = append(result.Saturated, capacity.name)
		}
	}
	sort.Strings(result.Saturated)
	return result
}
