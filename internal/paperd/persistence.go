// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperscenario"
)

const persistenceVersion = 1

type persistentSourceRevision struct {
	File   string `json:"file"`
	Source string `json:"source"`
}

type persistentCandidate struct {
	Head       int                         `json:"head"`
	Acceptance *CandidateAcceptanceReceipt `json:"acceptance,omitempty"`
	Journal    *paperedit.JournalState     `json:"journal,omitempty"`
}

type persistentScenarioRevision struct {
	Fixtures []paperscenario.Fixture `json:"fixtures"`
}

type persistentDomainRevision struct {
	Content string `json:"content"`
}

type persistentWorkspace struct {
	Version                    int                          `json:"version"`
	SourceRevisions            []persistentSourceRevision   `json:"source_revisions"`
	Candidates                 []persistentCandidate        `json:"candidates"`
	ScenarioRevisions          []persistentScenarioRevision `json:"scenario_revisions"`
	ScenarioCandidates         []persistentCandidate        `json:"scenario_candidates"`
	SemanticTemplateRevisions  []persistentDomainRevision   `json:"semantic_template_revisions,omitempty"`
	SemanticTemplateCandidates []persistentCandidate        `json:"semantic_template_candidates,omitempty"`
	PolicyRevisions            []persistentDomainRevision   `json:"policy_revisions,omitempty"`
	PolicyCandidates           []persistentCandidate        `json:"policy_candidates,omitempty"`
	// ReplayHashes are one-way approval-nonce hashes. SensitiveAudit and
	// SensitiveAuditAnchors contain identifiers and hashes only, never raw
	// capability, approval, source, fixture, report, or signature payloads.
	ReplayHashes          []string               `json:"replay_hashes,omitempty"`
	SensitiveAudit        []SensitiveAuditEntry  `json:"sensitive_audit,omitempty"`
	SensitiveAuditNext    uint64                 `json:"sensitive_audit_next,omitempty"`
	SensitiveAuditRoot    string                 `json:"sensitive_audit_root,omitempty"`
	SensitiveAuditAnchors []SensitiveAuditAnchor `json:"sensitive_audit_anchors,omitempty"`
	DisclosureAudit       []DisclosureAuditEntry `json:"disclosure_audit,omitempty"`
	DisclosureAuditNext   uint64                 `json:"disclosure_audit_next,omitempty"`
}

type persistenceManifest struct {
	Version          int              `json:"version"`
	Generation       uint64           `json:"generation,omitempty"`
	Project          string           `json:"project"`
	PolicyRevision   string           `json:"policy_revision"`
	DisclosureDomain DisclosureDomain `json:"disclosure_domain"`
	Snapshot         string           `json:"snapshot"`
	SHA256           string           `json:"sha256"`
	Bytes            int              `json:"bytes"`
	Authentication   string           `json:"authentication,omitempty"`
}

// persistenceMu avoids overlapping goroutines in this process. Every recovery
// and save also holds the root's advisory filesystem lock, which coordinates
// independent paperd processes without an unbounded path-to-lock registry.
var persistenceMu sync.Mutex

// OpenWorkspace recovers the latest manifest-selected canonical snapshot. A
// missing manifest is an empty workspace. A referenced truncated or corrupt
// generation fails closed; unreferenced generations from interrupted writes
// are ignored. Recovered capabilities always receive a fresh scope and nonce.
func OpenWorkspace(ctx context.Context, options WorkspaceOptions) (*Workspace, error) {
	if ctx == nil {
		return nil, workspaceError("INVALID_CONTEXT", "context is nil", ErrPersistence)
	}
	workspace, err := NewWorkspaceWithOptions(options)
	if err != nil {
		return nil, err
	}
	if options.PersistenceRoot == "" {
		return nil, workspaceError("PERSISTENCE_ROOT_REQUIRED", "an explicit persistence root is required", ErrPersistence)
	}
	if err := workspace.recoverSnapshot(ctx); err != nil {
		return nil, err
	}
	return workspace, nil
}

// SaveSnapshot writes immutable content first and atomically replaces the
// manifest last. The manifest is the commit record, so a crash cannot select a
// partially written generation. Raw handles, nonces, scopes, and idempotency
// cache entries are intentionally absent from the schema.
func (w *Workspace) SaveSnapshot(ctx context.Context) error {
	if w == nil {
		return workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrPersistence)
	}
	if ctx == nil {
		return workspaceError("INVALID_CONTEXT", "context is nil", ErrPersistence)
	}
	root, err := securePersistenceRoot(w.persistenceRoot, true)
	if err != nil {
		return err
	}
	persistenceMu.Lock()
	defer persistenceMu.Unlock()
	unlock, err := lockPersistenceRoot(ctx, root)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return workspaceError("PERSISTENCE_CANCELLED", "snapshot was cancelled", err)
	}
	currentManifest, currentExists, err := readCommittedManifest(root, w.limits.MaxPersistenceBytes)
	if err != nil {
		return err
	}
	current := ""
	if currentExists {
		current = currentManifest.Snapshot
		if currentManifest.Project != w.projectID || currentManifest.PolicyRevision != w.policyRevision || currentManifest.DisclosureDomain != w.disclosureDomain {
			w.recordDisclosureDenial("persistence.save", currentManifest.DisclosureDomain, "partition_mismatch")
			return workspaceError("PERSISTENCE_PARTITION", "persisted workspace belongs to another cache partition", ErrDisclosureDenied)
		}
		if !verifyPersistenceManifestAuthentication(currentManifest, w.persistenceAuthenticationKey) {
			return workspaceError("PERSISTENCE_AUTHENTICATION", "persisted manifest authentication failed or does not match workspace authentication mode", ErrPersistenceCorrupt)
		}
		if currentManifest.Generation == ^uint64(0) {
			return workspaceError("PERSISTENCE_GENERATION", "persisted workspace generation is exhausted", ErrPersistenceCorrupt)
		}
	}
	if err := pruneOrphanSnapshots(root, current); err != nil {
		return err
	}

	w.mu.Lock()
	w.pruneExpiredHandlesLocked(w.now())
	snapshot, err := w.persistentStateLocked()
	w.mu.Unlock()
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return workspaceError("PERSISTENCE_ENCODE", "snapshot encoding failed", ErrPersistence)
	}
	if len(encoded) > w.limits.MaxPersistenceBytes {
		return workspaceError("PERSISTENCE_QUOTA", "snapshot exceeds the configured persistence quota", ErrLimit)
	}
	if err := ctx.Err(); err != nil {
		return workspaceError("PERSISTENCE_CANCELLED", "snapshot was cancelled", err)
	}
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	name := "snapshot-" + digest + ".json"
	if currentExists && currentManifest.SHA256 == digest && currentManifest.Bytes == len(encoded) {
		w.persistenceGeneration = currentManifest.Generation
		return pruneOrphanSnapshots(root, currentManifest.Snapshot)
	}
	if currentManifest.Generation != w.persistenceGeneration {
		return workspaceError("PERSISTENCE_GENERATION_CONFLICT", "persisted workspace changed since it was opened", ErrPersistenceConflict)
	}
	persistenceFault("before_snapshot_write")
	if err := writeImmutable(root, name, encoded); err != nil {
		return err
	}
	manifest := persistenceManifest{Version: persistenceVersion, Generation: currentManifest.Generation + 1, Project: w.projectID, PolicyRevision: w.policyRevision,
		DisclosureDomain: w.disclosureDomain, Snapshot: name, SHA256: digest, Bytes: len(encoded)}
	manifest.Authentication = persistenceManifestAuthentication(manifest, w.persistenceAuthenticationKey)
	manifestBytes, _ := json.Marshal(manifest)
	if err := ctx.Err(); err != nil {
		return workspaceError("PERSISTENCE_CANCELLED", "snapshot was cancelled before commit", err)
	}
	persistenceFault("before_manifest_replace")
	if err := replaceManifest(root, manifestBytes); err != nil {
		return err
	}
	w.persistenceGeneration = manifest.Generation
	return pruneOrphanSnapshots(root, name)
}

func (w *Workspace) persistentStateLocked() (persistentWorkspace, error) {
	state := persistentWorkspace{Version: persistenceVersion}
	revisionSerials := sortedRecordKeys(w.revisions)
	revisionIndexes := make(map[RevisionHandle]int, len(revisionSerials))
	for _, serial := range revisionSerials {
		record := w.revisions[serial]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_PARTITION", "retained state has an invalid cache partition", ErrPersistenceCorrupt)
		}
		revisionIndexes[record.handle] = len(state.SourceRevisions)
		state.SourceRevisions = append(state.SourceRevisions, persistentSourceRevision{File: record.file, Source: record.source})
	}
	for _, serial := range sortedRecordKeys(w.candidates) {
		record := w.candidates[serial]
		head, ok := revisionIndexes[record.head]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_HEAD", "candidate head is not retained in its partition", ErrPersistenceCorrupt)
		}
		if !ok {
			continue
		}
		persisted := persistentCandidate{Head: head}
		journal := record.journal.ExportState()
		persisted.Journal = &journal
		if record.acceptance != nil {
			receipt := cloneCandidateAcceptanceReceipt(record.acceptance.receipt)
			persisted.Acceptance = &receipt
		}
		state.Candidates = append(state.Candidates, persisted)
	}
	scenarioSerials := sortedRecordKeys(w.scenarioRevisions)
	scenarioIndexes := make(map[ScenarioRevisionHandle]int, len(scenarioSerials))
	for _, serial := range scenarioSerials {
		record := w.scenarioRevisions[serial]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_PARTITION", "retained state has an invalid cache partition", ErrPersistenceCorrupt)
		}
		scenarioIndexes[record.handle] = len(state.ScenarioRevisions)
		state.ScenarioRevisions = append(state.ScenarioRevisions, persistentScenarioRevision{Fixtures: cloneScenarioFixtures(record.fixtures)})
	}
	for _, serial := range sortedRecordKeys(w.scenarioCandidates) {
		record := w.scenarioCandidates[serial]
		head, ok := scenarioIndexes[record.head]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_HEAD", "scenario candidate head is not retained in its partition", ErrPersistenceCorrupt)
		}
		if !ok {
			continue
		}
		state.ScenarioCandidates = append(state.ScenarioCandidates, persistentCandidate{Head: head})
	}
	semanticSerials := sortedRecordKeys(w.semanticTemplateRevisions)
	semanticIndexes := make(map[SemanticTemplateRevisionHandle]int, len(semanticSerials))
	for _, serial := range semanticSerials {
		record := w.semanticTemplateRevisions[serial]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_PARTITION", "retained semantic-template state has an invalid cache partition", ErrPersistenceCorrupt)
		}
		semanticIndexes[record.handle] = len(state.SemanticTemplateRevisions)
		state.SemanticTemplateRevisions = append(state.SemanticTemplateRevisions, persistentDomainRevision{Content: record.content})
	}
	for _, serial := range sortedRecordKeys(w.semanticTemplateCandidates) {
		record := w.semanticTemplateCandidates[serial]
		head, ok := semanticIndexes[record.head]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_HEAD", "semantic-template candidate partition is invalid", ErrPersistenceCorrupt)
		}
		if ok {
			state.SemanticTemplateCandidates = append(state.SemanticTemplateCandidates, persistentCandidate{Head: head})
		}
	}
	policySerials := sortedRecordKeys(w.policyRevisions)
	policyIndexes := make(map[PolicyRevisionHandle]int, len(policySerials))
	for _, serial := range policySerials {
		record := w.policyRevisions[serial]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_PARTITION", "retained policy state has an invalid cache partition", ErrPersistenceCorrupt)
		}
		policyIndexes[record.handle] = len(state.PolicyRevisions)
		state.PolicyRevisions = append(state.PolicyRevisions, persistentDomainRevision{Content: record.content})
	}
	for _, serial := range sortedRecordKeys(w.policyCandidates) {
		record := w.policyCandidates[serial]
		head, ok := policyIndexes[record.head]
		if !w.ownsPartition(record.partition) {
			return persistentWorkspace{}, workspaceError("PERSISTENCE_HEAD", "policy candidate partition is invalid", ErrPersistenceCorrupt)
		}
		if ok {
			state.PolicyCandidates = append(state.PolicyCandidates, persistentCandidate{Head: head})
		}
	}
	for nonceHash := range w.sensitiveApprovalNonces {
		state.ReplayHashes = append(state.ReplayHashes, hex.EncodeToString(nonceHash[:]))
	}
	sort.Strings(state.ReplayHashes)
	state.SensitiveAudit = append([]SensitiveAuditEntry(nil), w.sensitiveAudit...)
	state.SensitiveAuditNext = w.nextSensitiveAudit
	state.SensitiveAuditRoot = w.sensitiveAuditRoot
	state.SensitiveAuditAnchors = clonePersistentAuditAnchors(w.sensitiveAuditAnchors)
	state.DisclosureAudit = append([]DisclosureAuditEntry(nil), w.disclosureAudit...)
	state.DisclosureAuditNext = w.nextDisclosureAudit
	return state, nil
}

func (w *Workspace) restoreCandidateJournal(head *revisionRecord, persisted *paperedit.JournalState) (*paperedit.Journal, error) {
	if persisted == nil {
		journal, err := paperedit.NewJournal(head.file, head.source, w.journalLimits())
		if err != nil {
			return nil, workspaceError("PERSISTENCE_JOURNAL", "persisted candidate head cannot initialize its working-copy journal", ErrPersistenceCorrupt)
		}
		return journal, nil
	}
	if persisted.File != head.file || persisted.Source != head.source || persisted.Revision != head.revision || persisted.Limits != w.journalLimits() {
		return nil, workspaceError("PERSISTENCE_JOURNAL", "persisted working-copy journal does not match its candidate head", ErrPersistenceCorrupt)
	}
	allSources := make([]string, 0, 1+2*len(persisted.Undo)+2*len(persisted.Redo))
	allSources = append(allSources, persisted.Source)
	for _, entries := range [][]paperedit.JournalStateEntry{persisted.Undo, persisted.Redo} {
		for _, entry := range entries {
			allSources = append(allSources, entry.BeforeSource, entry.AfterSource)
		}
	}
	for _, source := range allSources {
		if _, err := w.prepareRevision(head.file, source); err != nil {
			return nil, workspaceError("PERSISTENCE_JOURNAL", "persisted working-copy history contains an invalid source snapshot", ErrPersistenceCorrupt)
		}
	}
	journal, err := paperedit.RestoreJournal(*persisted)
	if err != nil {
		return nil, workspaceError("PERSISTENCE_JOURNAL", "persisted working-copy journal is inconsistent", ErrPersistenceCorrupt)
	}
	return journal, nil
}

func persistenceManifestAuthentication(manifest persistenceManifest, key []byte) string {
	if len(key) == 0 {
		return ""
	}
	manifest.Authentication = ""
	encoded, _ := json.Marshal(manifest)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(append([]byte("paperd/persistence-manifest/v1\x00"), encoded...))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyPersistenceManifestAuthentication(manifest persistenceManifest, key []byte) bool {
	if len(key) == 0 {
		return manifest.Authentication == ""
	}
	if !validSHA256(manifest.Authentication) {
		return false
	}
	want, err := hex.DecodeString(manifest.Authentication)
	if err != nil {
		return false
	}
	got, err := hex.DecodeString(persistenceManifestAuthentication(manifest, key))
	return err == nil && hmac.Equal(want, got)
}

func sortedRecordKeys[T any](records map[uint64]*T) []uint64 {
	keys := make([]uint64, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func (w *Workspace) recoverSnapshot(ctx context.Context) error {
	root, err := securePersistenceRoot(w.persistenceRoot, true)
	if err != nil {
		return err
	}
	persistenceMu.Lock()
	defer persistenceMu.Unlock()
	unlock, err := lockPersistenceRoot(ctx, root)
	if err != nil {
		return err
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return workspaceError("PERSISTENCE_CANCELLED", "recovery was cancelled", err)
	}
	manifestBytes, exists, err := readSecureFile(root, "manifest.json", w.limits.MaxPersistenceBytes)
	if err != nil || !exists {
		return err
	}
	var manifest persistenceManifest
	if err := decodeStrict(manifestBytes, &manifest); err != nil || manifest.Version != persistenceVersion {
		return workspaceError("PERSISTENCE_MANIFEST", "manifest is invalid or unsupported", ErrPersistenceCorrupt)
	}
	if manifest.Project != w.projectID || manifest.PolicyRevision != w.policyRevision || manifest.DisclosureDomain != w.disclosureDomain {
		w.recordDisclosureDenial("persistence.open", manifest.DisclosureDomain, "partition_mismatch")
		return workspaceError("PERSISTENCE_PARTITION", "persisted workspace belongs to another cache partition", ErrDisclosureDenied)
	}
	if !verifyPersistenceManifestAuthentication(manifest, w.persistenceAuthenticationKey) {
		return workspaceError("PERSISTENCE_AUTHENTICATION", "persisted manifest authentication failed or does not match workspace authentication mode", ErrPersistenceCorrupt)
	}
	if !validSnapshotName(manifest.Snapshot, manifest.SHA256) || manifest.Bytes < 0 || manifest.Bytes > w.limits.MaxPersistenceBytes {
		return workspaceError("PERSISTENCE_MANIFEST", "manifest snapshot metadata is invalid", ErrPersistenceCorrupt)
	}
	encoded, exists, err := readSecureFile(root, manifest.Snapshot, w.limits.MaxPersistenceBytes)
	if err != nil {
		return err
	}
	if !exists || len(encoded) != manifest.Bytes {
		return workspaceError("PERSISTENCE_TRUNCATED", "committed snapshot is missing or truncated", ErrPersistenceCorrupt)
	}
	sum := sha256.Sum256(encoded)
	if hex.EncodeToString(sum[:]) != manifest.SHA256 {
		return workspaceError("PERSISTENCE_DIGEST", "committed snapshot digest does not match", ErrPersistenceCorrupt)
	}
	var snapshot persistentWorkspace
	if err := decodeStrict(encoded, &snapshot); err != nil || snapshot.Version != persistenceVersion {
		return workspaceError("PERSISTENCE_SNAPSHOT", "snapshot is invalid or unsupported", ErrPersistenceCorrupt)
	}
	if err := ctx.Err(); err != nil {
		return workspaceError("PERSISTENCE_CANCELLED", "recovery was cancelled", err)
	}
	if err := w.restoreSnapshot(snapshot); err != nil {
		return err
	}
	w.persistenceGeneration = manifest.Generation
	return nil
}

func (w *Workspace) restoreSnapshot(snapshot persistentWorkspace) error {
	if len(snapshot.SourceRevisions) > w.limits.MaxRevisions || len(snapshot.Candidates) > w.limits.MaxCandidates ||
		len(snapshot.ScenarioRevisions) > w.limits.MaxScenarioRevisions || len(snapshot.ScenarioCandidates) > w.limits.MaxScenarioCandidates ||
		len(snapshot.SemanticTemplateRevisions) > w.limits.MaxSemanticTemplateRevisions || len(snapshot.SemanticTemplateCandidates) > w.limits.MaxSemanticTemplateCandidates ||
		len(snapshot.PolicyRevisions) > w.limits.MaxPolicyRevisions || len(snapshot.PolicyCandidates) > w.limits.MaxPolicyCandidates {
		return workspaceError("PERSISTENCE_QUOTA", "persisted record count exceeds configured limits", ErrLimit)
	}
	if len(snapshot.ReplayHashes) > w.limits.MaxAuthorizationAudit || len(snapshot.SensitiveAudit) > w.limits.MaxAuthorizationAudit || len(snapshot.SensitiveAuditAnchors) > w.limits.MaxAuthorizationAudit {
		return workspaceError("PERSISTENCE_QUOTA", "persisted sensitive state exceeds configured limits", ErrLimit)
	}
	now := w.now()
	sourceHandles := make([]RevisionHandle, len(snapshot.SourceRevisions))
	for index, persisted := range snapshot.SourceRevisions {
		record, err := w.prepareRevision(persisted.File, persisted.Source)
		if err != nil {
			return workspaceError("PERSISTENCE_SOURCE", "persisted source revision is invalid", ErrPersistenceCorrupt)
		}
		w.nextRevision++
		record.handle = RevisionHandle{value: w.newHandle(handleRevision, capabilityRead, w.nextRevision)}
		record.expires, record.disclosure, record.partition = now.Add(w.handleTTL), w.disclosureDomain, w.partition
		w.revisions[w.nextRevision], sourceHandles[index] = record, record.handle
	}
	for _, persisted := range snapshot.Candidates {
		if persisted.Head < 0 || persisted.Head >= len(sourceHandles) {
			return workspaceError("PERSISTENCE_HEAD", "persisted candidate head is invalid", ErrPersistenceCorrupt)
		}
		w.nextCandidate++
		handle := CandidateHandle{value: w.newHandle(handleCandidate, capabilityEdit, w.nextCandidate)}
		headRevision := w.revisions[sourceHandles[persisted.Head].value.serial]
		journal, err := w.restoreCandidateJournal(headRevision, persisted.Journal)
		if err != nil {
			return err
		}
		record := &candidateRecord{handle: handle, head: sourceHandles[persisted.Head], journal: journal, idempotency: make(map[string]sourceIdempotencyRecord), acceptanceIdempotency: make(map[string]candidateAcceptanceIdempotencyRecord),
			expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
		if persisted.Acceptance != nil {
			receipt := cloneCandidateAcceptanceReceipt(*persisted.Acceptance)
			revision := w.revisions[sourceHandles[persisted.Head].value.serial]
			if err := w.validatePersistedAcceptance(receipt, revision); err != nil {
				return err
			}
			record.acceptance = &candidateAcceptanceRecord{receipt: receipt}
		}
		w.candidates[w.nextCandidate] = record
	}
	scenarioHandles := make([]ScenarioRevisionHandle, len(snapshot.ScenarioRevisions))
	for index, persisted := range snapshot.ScenarioRevisions {
		fixtures, err := w.validatePersistedFixtures(persisted.Fixtures)
		if err != nil {
			return workspaceError("PERSISTENCE_SCENARIO", "persisted scenario revision is invalid", ErrPersistenceCorrupt)
		}
		w.nextScenarioRevision++
		handle := ScenarioRevisionHandle{value: w.newHandle(handleScenarioRevision, capabilityRead, w.nextScenarioRevision)}
		w.scenarioRevisions[w.nextScenarioRevision] = &scenarioRevisionRecord{handle: handle, fixtures: fixtures, digest: scenarioDigest(fixtures),
			expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
		scenarioHandles[index] = handle
	}
	for _, persisted := range snapshot.ScenarioCandidates {
		if persisted.Head < 0 || persisted.Head >= len(scenarioHandles) {
			return workspaceError("PERSISTENCE_HEAD", "persisted scenario candidate head is invalid", ErrPersistenceCorrupt)
		}
		w.nextScenarioCandidate++
		handle := ScenarioCandidateHandle{value: w.newHandle(handleScenarioCandidate, capabilityEdit, w.nextScenarioCandidate)}
		w.scenarioCandidates[w.nextScenarioCandidate] = &scenarioCandidateRecord{handle: handle, head: scenarioHandles[persisted.Head], idempotency: make(map[string]scenarioIdempotencyRecord),
			expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	}
	semanticHandles := make([]SemanticTemplateRevisionHandle, len(snapshot.SemanticTemplateRevisions))
	for index, persisted := range snapshot.SemanticTemplateRevisions {
		content, digest, err := validateRevisionDomainContent("semantic-template", persisted.Content, w.limits.MaxSourceBytes)
		if err != nil {
			return workspaceError("PERSISTENCE_SEMANTIC_TEMPLATE", "persisted semantic-template revision is invalid", ErrPersistenceCorrupt)
		}
		w.nextSemanticTemplateRevision++
		handle := SemanticTemplateRevisionHandle{value: w.newHandle(handleSemanticTemplateRevision, capabilityRead, w.nextSemanticTemplateRevision)}
		w.semanticTemplateRevisions[w.nextSemanticTemplateRevision] = &semanticTemplateRevisionRecord{handle: handle, content: content, digest: digest, expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
		semanticHandles[index] = handle
	}
	for _, persisted := range snapshot.SemanticTemplateCandidates {
		if persisted.Head < 0 || persisted.Head >= len(semanticHandles) {
			return workspaceError("PERSISTENCE_HEAD", "persisted semantic-template candidate head is invalid", ErrPersistenceCorrupt)
		}
		w.nextSemanticTemplateCandidate++
		handle := SemanticTemplateCandidateHandle{value: w.newHandle(handleSemanticTemplateCandidate, capabilityEdit, w.nextSemanticTemplateCandidate)}
		w.semanticTemplateCandidates[w.nextSemanticTemplateCandidate] = &semanticTemplateCandidateRecord{handle: handle, head: semanticHandles[persisted.Head], idempotency: make(map[string]semanticTemplateIdempotencyRecord), expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	}
	policyHandles := make([]PolicyRevisionHandle, len(snapshot.PolicyRevisions))
	for index, persisted := range snapshot.PolicyRevisions {
		content, digest, err := validateRevisionDomainContent("policy", persisted.Content, w.limits.MaxSourceBytes)
		if err != nil {
			return workspaceError("PERSISTENCE_POLICY", "persisted policy revision is invalid", ErrPersistenceCorrupt)
		}
		w.nextPolicyRevision++
		handle := PolicyRevisionHandle{value: w.newHandle(handlePolicyRevision, capabilityRead, w.nextPolicyRevision)}
		w.policyRevisions[w.nextPolicyRevision] = &policyRevisionRecord{handle: handle, content: content, digest: digest, expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
		policyHandles[index] = handle
	}
	for _, persisted := range snapshot.PolicyCandidates {
		if persisted.Head < 0 || persisted.Head >= len(policyHandles) {
			return workspaceError("PERSISTENCE_HEAD", "persisted policy candidate head is invalid", ErrPersistenceCorrupt)
		}
		w.nextPolicyCandidate++
		handle := PolicyCandidateHandle{value: w.newHandle(handlePolicyCandidate, capabilityEdit, w.nextPolicyCandidate)}
		w.policyCandidates[w.nextPolicyCandidate] = &policyCandidateRecord{handle: handle, head: policyHandles[persisted.Head], idempotency: make(map[string]policyIdempotencyRecord), expires: now.Add(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	}
	if err := w.restoreSensitivePersistence(snapshot); err != nil {
		return err
	}
	if err := w.restoreDisclosureAudit(snapshot.DisclosureAudit, snapshot.DisclosureAuditNext); err != nil {
		return err
	}
	return nil
}

func (w *Workspace) validatePersistedAcceptance(receipt CandidateAcceptanceReceipt, revision *revisionRecord) error {
	if revision == nil || receipt.CandidateRevision != string(revision.revision) || receipt.PolicyRevision != w.policyRevision || receipt.PolicyHash != w.acceptancePolicyHash ||
		!validSHA256(receipt.EvidenceHash) || !validSHA256(receipt.AcceptanceHash) || receipt.AcceptanceHash != candidateAcceptanceReceiptHash(receipt) || receipt.AcceptedAt.IsZero() || receipt.AcceptedAt.Location() != time.UTC {
		return workspaceError("PERSISTENCE_ACCEPTANCE", "persisted candidate acceptance is not bound to the exact head and policy", ErrPersistenceCorrupt)
	}
	if receipt.Audit.Operation != SensitiveAccept || receipt.Audit.PolicyRevision != receipt.PolicyRevision || receipt.Audit.EvidenceHash != receipt.EvidenceHash || !validSHA256(receipt.Audit.AuditHash) {
		return workspaceError("PERSISTENCE_ACCEPTANCE", "persisted candidate acceptance audit binding is invalid", ErrPersistenceCorrupt)
	}
	if len(receipt.ScenarioResults) != len(w.acceptancePolicy.RequiredScenarios) || len(receipt.Validators) != len(w.acceptancePolicy.RequiredValidators) || len(receipt.ReviewArtifacts) != len(w.acceptancePolicy.RequiredReviewArtifacts) {
		return workspaceError("PERSISTENCE_ACCEPTANCE", "persisted candidate acceptance does not cover the current exact policy", ErrPersistenceCorrupt)
	}
	for index, required := range w.acceptancePolicy.RequiredScenarios {
		value := receipt.ScenarioResults[index]
		if value.Name != required || !value.Passed || !validSHA256(value.Digest) || !validSHA256(value.ResultHash) {
			return workspaceError("PERSISTENCE_ACCEPTANCE", "persisted scenario acceptance evidence is invalid", ErrPersistenceCorrupt)
		}
	}
	for index, required := range w.acceptancePolicy.RequiredValidators {
		value := receipt.Validators[index]
		if value.Profile != required.Profile || value.Version != required.Version || !value.Passed || !validSHA256(value.Hash) {
			return workspaceError("PERSISTENCE_ACCEPTANCE", "persisted validator acceptance evidence is invalid", ErrPersistenceCorrupt)
		}
	}
	for index, required := range w.acceptancePolicy.RequiredReviewArtifacts {
		value := receipt.ReviewArtifacts[index]
		if value.Kind != required || !value.Approved || !validSHA256(value.Hash) {
			return workspaceError("PERSISTENCE_ACCEPTANCE", "persisted review acceptance evidence is invalid", ErrPersistenceCorrupt)
		}
	}
	return nil
}

func (w *Workspace) restoreSensitivePersistence(snapshot persistentWorkspace) error {
	for _, encoded := range snapshot.ReplayHashes {
		decoded, err := hex.DecodeString(encoded)
		if err != nil || len(decoded) != sha256.Size {
			return workspaceError("PERSISTENCE_REPLAY", "persisted replay protection hash is invalid", ErrPersistenceCorrupt)
		}
		var value [sha256.Size]byte
		copy(value[:], decoded)
		if _, duplicate := w.sensitiveApprovalNonces[value]; duplicate {
			return workspaceError("PERSISTENCE_REPLAY", "persisted replay protection contains a duplicate", ErrPersistenceCorrupt)
		}
		w.sensitiveApprovalNonces[value] = struct{}{}
	}
	if err := validatePersistentAuditChain(snapshot.SensitiveAudit, snapshot.SensitiveAuditNext, snapshot.SensitiveAuditRoot, w.limits.MaxQueryBytes); err != nil {
		return err
	}
	w.sensitiveAudit = append([]SensitiveAuditEntry(nil), snapshot.SensitiveAudit...)
	w.nextSensitiveAudit = snapshot.SensitiveAuditNext
	w.sensitiveAuditRoot = snapshot.SensitiveAuditRoot
	anchors := clonePersistentAuditAnchors(snapshot.SensitiveAuditAnchors)
	for _, anchor := range anchors {
		if err := validatePersistentAuditAnchor(anchor, w.limits.MaxQueryBytes); err != nil {
			return err
		}
	}
	w.sensitiveAuditAnchors = anchors
	return nil
}

func validatePersistentAuditChain(entries []SensitiveAuditEntry, next uint64, root string, limit int) error {
	if len(entries) == 0 {
		if next != 0 || root != "" {
			return workspaceError("PERSISTENCE_AUDIT", "empty sensitive audit has non-empty chain state", ErrPersistenceCorrupt)
		}
		return nil
	}
	for index, entry := range entries {
		operationOK := validSensitiveLabel(string(entry.Operation), limit)
		identityOK := validSensitiveLabel(entry.Actor, limit) && (entry.PolicyRevision == "" || validSensitiveLabel(entry.PolicyRevision, limit)) && validSensitiveLabel(entry.Reason, limit)
		hashOK := (entry.EvidenceHash == "" || validSHA256(entry.EvidenceHash)) && (entry.PreviousHash == "" || validSHA256(entry.PreviousHash)) && validSHA256(entry.EventHash)
		if entry.Sequence == 0 || entry.At.IsZero() || entry.At.Location() != time.UTC || !operationOK || !identityOK || !hashOK || entry.EventHash != sensitiveAuditEventHash(entry) || (index > 0 && (entry.Sequence != entries[index-1].Sequence+1 || entry.PreviousHash != entries[index-1].EventHash)) {
			return workspaceError("PERSISTENCE_AUDIT", "persisted sensitive audit chain is invalid", ErrPersistenceCorrupt)
		}
	}
	last := entries[len(entries)-1]
	if next != last.Sequence || root != last.EventHash {
		return workspaceError("PERSISTENCE_AUDIT", "persisted sensitive audit root does not match the retained chain", ErrPersistenceCorrupt)
	}
	return nil
}

func (w *Workspace) validatePersistedFixtures(fixtures []paperscenario.Fixture) ([]paperscenario.Fixture, error) {
	input := make([]paperscenario.Scenario, len(fixtures))
	for index, fixture := range fixtures {
		input[index] = paperscenario.Scenario{Name: fixture.Name, Locale: fixture.Locale, Values: cloneScenarioFields(fixture.Values)}
	}
	limits := paperscenario.DefaultLimits()
	limits.MaxNodes = uint32(w.limits.MaxScenarioValueNodes)
	limits.MaxPathBytes = uint32(w.limits.MaxScenarioPathBytes)
	limits.MaxWork = uint64(w.limits.MaxScenarioWork)
	resolved, err := paperscenario.Resolve(input, limits)
	if err != nil {
		return nil, err
	}
	want, _ := json.Marshal(fixtures)
	got, _ := json.Marshal(resolved)
	if !bytes.Equal(want, got) {
		return nil, ErrPersistenceCorrupt
	}
	return resolved, nil
}

func decodeStrict(encoded []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}

func securePersistenceRoot(root string, create bool) (string, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return "", workspaceError("PERSISTENCE_ROOT", "persistence root must be an explicit clean absolute path", ErrPersistence)
	}
	if info, err := os.Lstat(root); err != nil {
		if !os.IsNotExist(err) || !create {
			return "", workspaceError("PERSISTENCE_ROOT", "persistence root is unavailable", ErrPersistence)
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			return "", workspaceError("PERSISTENCE_ROOT", "persistence root cannot be created", ErrPersistence)
		}
	} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", workspaceError("PERSISTENCE_ROOT", "persistence root must be a real directory", ErrPersistence)
	}
	info, err := os.Lstat(root)
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		return "", workspaceError("PERSISTENCE_PERMISSIONS", "persistence root must not grant group or other permissions", ErrPersistence)
	}
	return root, nil
}

func readSecureFile(root, name string, limit int) ([]byte, bool, error) {
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > int64(limit) {
		return nil, false, workspaceError("PERSISTENCE_FILE", "persisted file type, permissions, or size is invalid", ErrPersistenceCorrupt)
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, false, workspaceError("PERSISTENCE_READ", "persisted file cannot be read", ErrPersistence)
	}
	return encoded, true, nil
}

func validSnapshotName(name, digest string) bool {
	return len(digest) == sha256.Size*2 && name == "snapshot-"+digest+".json" && !strings.ContainsAny(name, `/\\`)
}

func readCommittedManifest(root string, limit int) (persistenceManifest, bool, error) {
	encoded, exists, err := readSecureFile(root, "manifest.json", limit)
	if err != nil || !exists {
		return persistenceManifest{}, exists, err
	}
	var manifest persistenceManifest
	if err := decodeStrict(encoded, &manifest); err != nil || manifest.Version != persistenceVersion || !validSnapshotName(manifest.Snapshot, manifest.SHA256) {
		return persistenceManifest{}, true, workspaceError("PERSISTENCE_MANIFEST", "existing manifest is invalid", ErrPersistenceCorrupt)
	}
	return manifest, true, nil
}

func pruneOrphanSnapshots(root, keep string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return workspaceError("PERSISTENCE_READ", "persistence root cannot be enumerated", ErrPersistence)
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == keep || !strings.HasPrefix(name, "snapshot-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return workspaceError("PERSISTENCE_FILE", "orphan snapshot path is unsafe", ErrPersistenceCorrupt)
		}
		if err := os.Remove(filepath.Join(root, name)); err != nil {
			return workspaceError("PERSISTENCE_CLEANUP", "orphan snapshot cannot be removed", ErrPersistence)
		}
	}
	return nil
}

func writeImmutable(root, name string, encoded []byte) error {
	path := filepath.Join(root, name)
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return workspaceError("PERSISTENCE_FILE", "snapshot path is unsafe", ErrPersistence)
		}
		existing, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Equal(existing, encoded) {
			return workspaceError("PERSISTENCE_COLLISION", "immutable snapshot content does not match its digest name", ErrPersistenceCorrupt)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return workspaceError("PERSISTENCE_FILE", "snapshot path is unavailable", ErrPersistence)
	}
	return atomicWrite(root, name, encoded, true)
}

func replaceManifest(root string, encoded []byte) error {
	if info, err := os.Lstat(filepath.Join(root, "manifest.json")); err == nil &&
		(info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0) {
		return workspaceError("PERSISTENCE_FILE", "manifest path is unsafe", ErrPersistence)
	} else if err != nil && !os.IsNotExist(err) {
		return workspaceError("PERSISTENCE_FILE", "manifest path is unavailable", ErrPersistence)
	}
	return atomicWrite(root, "manifest.json", encoded, false)
}

func atomicWrite(root, name string, encoded []byte, noReplace bool) error {
	kind := "manifest"
	if noReplace {
		kind = "snapshot"
	}
	temporary, err := os.CreateTemp(root, ".paperd-*")
	if err != nil {
		return workspaceError("PERSISTENCE_WRITE", "temporary snapshot cannot be created", ErrPersistence)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return workspaceError("PERSISTENCE_PERMISSIONS", "snapshot permissions cannot be restricted", ErrPersistence)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return workspaceError("PERSISTENCE_WRITE", "snapshot cannot be durably written", ErrPersistence)
	}
	persistenceFault("after_" + kind + "_write")
	persistenceFault("after_file_write")
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return workspaceError("PERSISTENCE_WRITE", "snapshot cannot be durably written", ErrPersistence)
	}
	persistenceFault("after_" + kind + "_fsync")
	persistenceFault("after_file_fsync")
	if err := temporary.Close(); err != nil {
		return workspaceError("PERSISTENCE_WRITE", "snapshot cannot be durably written", ErrPersistence)
	}
	target := filepath.Join(root, name)
	if noReplace {
		if _, err := os.Lstat(target); err == nil {
			return nil
		}
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return workspaceError("PERSISTENCE_COMMIT", "snapshot cannot be atomically committed", ErrPersistence)
	}
	persistenceFault("after_" + kind + "_replace")
	directory, err := os.Open(root)
	if err == nil {
		err = directory.Sync()
		_ = directory.Close()
	}
	if err != nil {
		return workspaceError("PERSISTENCE_SYNC", "snapshot directory cannot be synchronized", ErrPersistence)
	}
	persistenceFault("after_" + kind + "_directory_fsync")
	return nil
}

// persistenceFault is an unexported crash-injection seam. Production leaves
// it nil; subprocess tests replace it before invoking SaveSnapshot.
var persistenceFaultHook func(string)

func persistenceFault(point string) {
	if hook := persistenceFaultHook; hook != nil {
		hook(point)
	}
}
