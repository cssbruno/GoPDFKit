// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"time"

	"github.com/cssbruno/paperrune/internal/papercompile"
	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

// CapabilityMode declares what an opened document may subsequently be used
// for. Enforcement belongs to every operation that accepts the OpenHandle;
// the initial read tools only issue read or edit capabilities explicitly.
type CapabilityMode string

const (
	CapabilityRead   CapabilityMode = "read"
	CapabilityEdit   CapabilityMode = "edit"
	CapabilityRender CapabilityMode = "render"
)

// PaperCreateRequest is the transport-independent input for paper.create.
// Source may be syntactically invalid: the resulting candidate remains useful
// for diagnostic-driven repair.
type PaperCreateRequest struct {
	File   string `json:"file"`
	Source string `json:"source"`
}

// PaperCreateResult owns one immutable initial revision and one mutable
// candidate head. Publication of the pair is atomic.
type PaperCreateResult struct {
	Revision  RevisionSnapshot  `json:"revision"`
	Candidate CandidateSnapshot `json:"candidate"`
}

// PaperCreate atomically creates the initial source revision and candidate.
// It cannot leave an unreachable revision behind when either capacity is
// exhausted.
func (w *Workspace) PaperCreate(request PaperCreateRequest) (PaperCreateResult, error) {
	if w == nil {
		return PaperCreateResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	prepared, err := w.prepareRevision(request.File, request.Source)
	if err != nil {
		return PaperCreateResult{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	if len(w.revisions) >= w.limits.MaxRevisions {
		return PaperCreateResult{}, workspaceError("REVISION_LIMIT", "workspace revision capacity is exhausted", ErrLimit)
	}
	if len(w.candidates) >= w.limits.MaxCandidates {
		return PaperCreateResult{}, workspaceError("CANDIDATE_LIMIT", "workspace candidate capacity is exhausted", ErrLimit)
	}
	w.nextRevision++
	prepared.handle = RevisionHandle{value: w.newHandle(handleRevision, capabilityRead, w.nextRevision)}
	prepared.expires = w.expiresAt(w.handleTTL)
	prepared.disclosure = w.disclosureDomain
	prepared.partition = w.partition
	w.revisions[w.nextRevision] = prepared
	w.nextCandidate++
	candidate := &candidateRecord{
		handle: CandidateHandle{value: w.newHandle(handleCandidate, capabilityEdit, w.nextCandidate)},
		head:   prepared.handle, idempotency: make(map[string]sourceIdempotencyRecord), acceptanceIdempotency: make(map[string]candidateAcceptanceIdempotencyRecord), expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition,
	}
	candidate.journal, err = paperedit.NewJournal(prepared.file, prepared.source, w.journalLimits())
	if err != nil {
		return PaperCreateResult{}, workspaceError("JOURNAL_LIMIT", "candidate working-copy journal cannot retain its initial source", ErrLimit)
	}
	w.candidates[w.nextCandidate] = candidate
	return PaperCreateResult{
		Revision:  snapshotOf(prepared),
		Candidate: snapshotCandidate(candidate),
	}, nil
}

type PaperOpenRequest struct {
	Candidate        CandidateHandle    `json:"-"`
	Revision         RevisionHandle     `json:"-"`
	ExpectedDigest   paperedit.Revision `json:"expected_digest"`
	Mode             CapabilityMode     `json:"mode"`
	DisclosureDomain DisclosureDomain   `json:"disclosure_domain,omitempty"`
}

// PaperOpenSnapshot is a detached description of an immutable open
// capability. Handle fields are deliberately omitted from JSON transports.
type PaperOpenSnapshot struct {
	Handle           OpenHandle         `json:"-"`
	Candidate        CandidateHandle    `json:"-"`
	Revision         RevisionHandle     `json:"-"`
	Digest           paperedit.Revision `json:"digest"`
	Mode             CapabilityMode     `json:"mode"`
	File             string             `json:"file"`
	DisclosureDomain DisclosureDomain   `json:"disclosure_domain"`
	ExpiresAt        time.Time          `json:"expires_at"`
}

// PaperOpen pins an exact retained revision. When Candidate is supplied, its
// head must equal Revision at publication time; the handle never silently
// follows later candidate updates.
func (w *Workspace) PaperOpen(request PaperOpenRequest) (PaperOpenSnapshot, error) {
	if w == nil {
		return PaperOpenSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if request.Mode != CapabilityRead && request.Mode != CapabilityEdit {
		return PaperOpenSnapshot{}, workspaceError("INVALID_CAPABILITY_MODE", "capability mode must be read or edit", ErrInvalidQuery)
	}
	requestedDomain := request.DisclosureDomain
	if requestedDomain == "" {
		requestedDomain = w.disclosureDomain
	}
	if requestedDomain != w.disclosureDomain {
		w.recordDisclosureDenial("paper.open", requestedDomain, "domain_mismatch")
		return PaperOpenSnapshot{}, workspaceError("DISCLOSURE_DENIED", "requested disclosure domain is unavailable", ErrDisclosureDenied)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	revision, err := w.revisionLocked(request.Revision)
	if err != nil {
		return PaperOpenSnapshot{}, err
	}
	if revision.revision != request.ExpectedDigest {
		return PaperOpenSnapshot{}, workspaceError("REVISION_CONFLICT", "exact source digest does not match the requested revision", ErrRevisionConflict)
	}
	if request.Candidate.value.serial != 0 {
		candidate, err := w.candidateLocked(request.Candidate)
		if err != nil {
			return PaperOpenSnapshot{}, err
		}
		if candidate.head != request.Revision {
			return PaperOpenSnapshot{}, workspaceError("REVISION_CONFLICT", "candidate head does not match the requested revision", ErrRevisionConflict)
		}
	}
	if len(w.opens) >= w.limits.MaxOpenDocuments {
		return PaperOpenSnapshot{}, workspaceError("OPEN_LIMIT", "workspace open-document capacity is exhausted", ErrLimit)
	}
	w.nextOpen++
	capability := capabilityRead
	if request.Mode == CapabilityEdit {
		capability = capabilityEdit
	}
	record := &openRecord{
		handle: OpenHandle{value: w.newHandle(handleOpen, capability, w.nextOpen)}, candidate: request.Candidate,
		revision: request.Revision, digest: request.ExpectedDigest, mode: request.Mode, expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition,
	}
	w.opens[w.nextOpen] = record
	return snapshotOpen(record, revision.file), nil
}

// ClosePaperOpen explicitly revokes one open capability and releases its
// bounded workspace capacity. It does not remove its source revision.
func (w *Workspace) ClosePaperOpen(handle OpenHandle) error {
	if w == nil {
		return workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, err := w.openLocked(handle); err != nil {
		return err
	}
	delete(w.opens, handle.value.serial)
	w.recordRevocationLocked(handle.value, revokedExplicitly, w.now())
	return nil
}

func snapshotOpen(record *openRecord, file string) PaperOpenSnapshot {
	return PaperOpenSnapshot{
		Handle: record.handle, Candidate: record.candidate, Revision: record.revision,
		Digest: record.digest, Mode: record.mode, File: file,
		DisclosureDomain: record.disclosure, ExpiresAt: record.expires,
	}
}

type PaperContextRequest struct {
	Open             OpenHandle         `json:"-"`
	ExpectedRevision RevisionHandle     `json:"-"`
	ExpectedDigest   paperedit.Revision `json:"expected_digest"`
	MaxBytes         int                `json:"max_bytes"`
	MaxItems         int                `json:"max_items"`
	IncludeSource    bool               `json:"include_source,omitempty"`
}

type RevisionIdentity struct {
	File        string             `json:"file"`
	Digest      paperedit.Revision `json:"digest"`
	Bytes       int                `json:"bytes"`
	SyntaxNodes int                `json:"syntax_nodes"`
	ParseOK     bool               `json:"parse_ok"`
	CompileOK   bool               `json:"compile_ok"`
}

// PaperContextResult is a detached, deterministic, byte-budgeted task view.
// EncodedBytes is the exact length of json.Marshal(result).
type PaperContextResult struct {
	Open            PaperOpenSnapshot          `json:"open"`
	Revision        RevisionIdentity           `json:"revision"`
	Root            NodeSummary                `json:"root"`
	Page            papercompile.PageSpec      `json:"page"`
	Title           string                     `json:"title,omitempty"`
	Language        string                     `json:"language,omitempty"`
	BodyBlocks      int                        `json:"body_blocks"`
	Source          string                     `json:"source,omitempty"`
	Diagnostics     []paperlang.Diagnostic     `json:"diagnostics,omitempty"`
	Mappings        []papercompile.NodeMapping `json:"mappings,omitempty"`
	SourceTruncated bool                       `json:"source_truncated,omitempty"`
	ItemsTruncated  bool                       `json:"items_truncated,omitempty"`
	EncodedBytes    int                        `json:"encoded_bytes"`
}

// PaperContext returns context for the exact open revision only. A
// candidate-backed open becomes stale as soon as its candidate advances;
// callers must explicitly open the new head rather than observing drift.
func (w *Workspace) PaperContext(request PaperContextRequest) (PaperContextResult, error) {
	if w == nil {
		return PaperContextResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if request.MaxBytes < 1 || request.MaxBytes > w.limits.MaxContextBytes ||
		request.MaxItems < 1 || request.MaxItems > w.limits.MaxSearchResults {
		return PaperContextResult{}, workspaceError("CONTEXT_LIMIT", "context bounds are outside configured limits", ErrLimit)
	}
	opened, revision, err := w.exactOpenRevision(request.Open, request.ExpectedRevision, request.ExpectedDigest)
	if err != nil {
		return PaperContextResult{}, err
	}
	openSnapshot := snapshotOpen(opened, revision.file)

	result := PaperContextResult{
		Open: openSnapshot,
		Revision: RevisionIdentity{
			File: revision.file, Digest: revision.revision, Bytes: len(revision.source), SyntaxNodes: revision.nodes,
			ParseOK: revision.parsed.OK(), CompileOK: revision.parsed.OK() && revision.compiled.OK(),
		},
	}
	if revision.parsed.AST.Root != nil {
		result.Root = nodeSummary(revision.parsed.AST.Root)
	}
	if revision.parsed.OK() {
		result.Page = revision.compiled.Page
		if revision.compiled.Document != nil {
			result.Title = revision.compiled.Document.Title
			result.Language = revision.compiled.Document.Language
			result.BodyBlocks = len(revision.compiled.Document.Body)
		}
	}
	if request.IncludeSource {
		trial := result
		trial.Source = revision.source
		if contextEncodedBytes(&trial) <= request.MaxBytes {
			result.Source = revision.source
		} else {
			result.SourceTruncated = true
		}
	}

	diagnostics := append([]paperlang.Diagnostic(nil), revision.parsed.Diagnostics...)
	diagnostics = append(diagnostics, revision.compiled.Diagnostics...)
	items := 0
	for _, diagnostic := range diagnostics {
		if items == request.MaxItems {
			result.ItemsTruncated = true
			break
		}
		trial := result
		trial.Diagnostics = append(append([]paperlang.Diagnostic(nil), result.Diagnostics...), diagnostic)
		if contextEncodedBytes(&trial) > request.MaxBytes {
			result.ItemsTruncated = true
			break
		}
		result.Diagnostics = trial.Diagnostics
		items++
	}
	if len(result.Diagnostics) < len(diagnostics) {
		result.ItemsTruncated = true
	}
	for _, mapping := range revision.compiled.Mapping.Nodes {
		if items == request.MaxItems {
			result.ItemsTruncated = true
			break
		}
		trial := result
		trial.Mappings = append(append([]papercompile.NodeMapping(nil), result.Mappings...), mapping)
		if contextEncodedBytes(&trial) > request.MaxBytes {
			result.ItemsTruncated = true
			break
		}
		result.Mappings = trial.Mappings
		items++
	}
	if len(result.Mappings) < len(revision.compiled.Mapping.Nodes) {
		result.ItemsTruncated = true
	}
	result.EncodedBytes = contextEncodedBytes(&result)
	if result.EncodedBytes > request.MaxBytes {
		return PaperContextResult{}, workspaceError("CONTEXT_LIMIT", "context byte budget is too small for required identity metadata", ErrLimit)
	}
	return clonePaperContext(result), nil
}

// exactOpenRevision is the single revision-safety gate for read tools. The
// returned records are immutable after publication; maps and candidate heads
// are inspected while holding the workspace read lock.
func (w *Workspace) exactOpenRevision(handle OpenHandle, expected RevisionHandle, digest paperedit.Revision) (*openRecord, *revisionRecord, error) {
	if w == nil {
		return nil, nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	opened, err := w.openLocked(handle)
	if err != nil {
		return nil, nil, err
	}
	if opened.revision != expected || opened.digest != digest {
		return nil, nil, workspaceError("REVISION_CONFLICT", "read preconditions do not match the open revision", ErrRevisionConflict)
	}
	if opened.candidate.value.serial != 0 {
		candidate, err := w.candidateLocked(opened.candidate)
		if err != nil {
			return nil, nil, err
		}
		if candidate.head != opened.revision {
			return nil, nil, workspaceError("REVISION_CONFLICT", "opened candidate has advanced; reopen its exact head", ErrRevisionConflict)
		}
	}
	revision, err := w.revisionLocked(opened.revision)
	if err != nil {
		return nil, nil, err
	}
	return opened, revision, nil
}

func (w *Workspace) openLocked(handle OpenHandle) (*openRecord, error) {
	if err := w.validateHandle(handle.value, handleOpen, capabilityRead, true); err != nil {
		return nil, err
	}
	record := w.opens[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrRevisionNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func contextEncodedBytes(result *PaperContextResult) int {
	previous := -1
	for result.EncodedBytes != previous {
		previous = result.EncodedBytes
		encoded, _ := json.Marshal(result)
		result.EncodedBytes = len(encoded)
	}
	encoded, _ := json.Marshal(result)
	return len(encoded)
}

func clonePaperContext(result PaperContextResult) PaperContextResult {
	result.Diagnostics = append([]paperlang.Diagnostic(nil), result.Diagnostics...)
	result.Mappings = append([]papercompile.NodeMapping(nil), result.Mappings...)
	return result
}
