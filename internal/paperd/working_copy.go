// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperedit"
)

// WorkingCopySnapshot is a detached view of a candidate's single revision
// journal. Entries contain minimal patches but never the journal's private
// before/after source snapshots.
type WorkingCopySnapshot struct {
	Candidate CandidateSnapshot         `json:"candidate"`
	Revision  RevisionSnapshot          `json:"revision"`
	Journal   paperedit.JournalSnapshot `json:"journal"`
	Entries   []paperedit.JournalEntry  `json:"entries,omitempty"`
}

type WorkingCopyResult struct {
	Candidate CandidateSnapshot         `json:"candidate"`
	Revision  RevisionSnapshot          `json:"revision"`
	Journal   paperedit.JournalSnapshot `json:"journal"`
	Entry     paperedit.JournalEntry    `json:"entry"`
}

type PaperSourceEditRequest struct {
	Candidate        CandidateHandle    `json:"-"`
	ExpectedHead     RevisionHandle     `json:"-"`
	ExpectedRevision paperedit.Revision `json:"expected_revision"`
	Group            string             `json:"group,omitempty"`
	Source           string             `json:"source"`
}

type PaperHistoryRequest struct {
	Candidate        CandidateHandle    `json:"-"`
	ExpectedHead     RevisionHandle     `json:"-"`
	ExpectedRevision paperedit.Revision `json:"expected_revision"`
}

type PaperExternalReloadRequest struct {
	Candidate        CandidateHandle    `json:"-"`
	ExpectedHead     RevisionHandle     `json:"-"`
	ExpectedRevision paperedit.Revision `json:"expected_revision"`
	Source           string             `json:"source"`
}

type PaperExternalReloadResult struct {
	WorkingCopyResult
	Conflict *paperedit.ExternalReloadConflict `json:"conflict,omitempty"`
}

// PaperWorkingCopy returns the exact candidate head and its shared journal.
func (w *Workspace) PaperWorkingCopy(handle CandidateHandle) (WorkingCopySnapshot, error) {
	if w == nil {
		return WorkingCopySnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	candidate, err := w.candidateLocked(handle)
	if err != nil {
		return WorkingCopySnapshot{}, err
	}
	revision, err := w.revisionLocked(candidate.head)
	if err != nil {
		return WorkingCopySnapshot{}, err
	}
	return WorkingCopySnapshot{Candidate: snapshotCandidate(candidate), Revision: snapshotOf(revision),
		Journal: candidate.journal.Snapshot(), Entries: candidate.journal.Entries()}, nil
}

// PaperApplySource commits an editor buffer to the same journal used by
// semantic operations. Consecutive non-empty equal groups coalesce into one
// undo transition while every publication still receives an immutable handle.
func (w *Workspace) PaperApplySource(request PaperSourceEditRequest) (WorkingCopyResult, error) {
	if w == nil {
		return WorkingCopyResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := w.validateWorkingCopyInput(request.Source, request.Group); err != nil {
		return WorkingCopyResult{}, err
	}
	prepared, err := w.prepareWorkingCopyRevision(request.Candidate, request.Source)
	if err != nil {
		return WorkingCopyResult{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	candidate, current, err := w.exactWorkingCopyLocked(request.Candidate, request.ExpectedHead, request.ExpectedRevision)
	if err != nil {
		return WorkingCopyResult{}, err
	}
	if prepared.revision == current.revision {
		return workingCopyResult(candidate, current, candidate.journal.Snapshot(), paperedit.JournalEntry{}), nil
	}
	if len(w.revisions) >= w.limits.MaxRevisions {
		return WorkingCopyResult{}, workspaceError("REVISION_LIMIT", "workspace revision capacity is exhausted", ErrLimit)
	}
	entry, journal, err := candidate.journal.ApplySource(paperedit.SourceJournalRequest{
		ExpectedRevision: request.ExpectedRevision, Group: request.Group, Source: request.Source,
	})
	if err != nil {
		return WorkingCopyResult{}, wrapJournalError(err)
	}
	return w.publishWorkingCopyLocked(candidate, prepared, entry, journal), nil
}

func (w *Workspace) PaperUndo(request PaperHistoryRequest) (WorkingCopyResult, error) {
	return w.applyWorkingCopyHistory(request, true)
}

func (w *Workspace) PaperRedo(request PaperHistoryRequest) (WorkingCopyResult, error) {
	return w.applyWorkingCopyHistory(request, false)
}

func (w *Workspace) applyWorkingCopyHistory(request PaperHistoryRequest, undo bool) (WorkingCopyResult, error) {
	if w == nil {
		return WorkingCopyResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	candidate, _, err := w.exactWorkingCopyLocked(request.Candidate, request.ExpectedHead, request.ExpectedRevision)
	if err != nil {
		return WorkingCopyResult{}, err
	}
	if len(w.revisions) >= w.limits.MaxRevisions {
		return WorkingCopyResult{}, workspaceError("REVISION_LIMIT", "workspace revision capacity is exhausted", ErrLimit)
	}
	var entry paperedit.JournalEntry
	var journal paperedit.JournalSnapshot
	if undo {
		entry, journal, err = candidate.journal.Undo(request.ExpectedRevision)
	} else {
		entry, journal, err = candidate.journal.Redo(request.ExpectedRevision)
	}
	if err != nil {
		return WorkingCopyResult{}, wrapJournalError(err)
	}
	source, revision := candidate.journal.Source()
	prepared, prepareErr := w.prepareRevisionForCandidateLocked(candidate, source)
	if prepareErr != nil || prepared.revision != revision {
		// Every history source was validated when first admitted. Roll back the
		// journal if retained state is unexpectedly inconsistent.
		if undo {
			_, _, _ = candidate.journal.Redo(revision)
		} else {
			_, _, _ = candidate.journal.Undo(revision)
		}
		if prepareErr != nil {
			return WorkingCopyResult{}, prepareErr
		}
		return WorkingCopyResult{}, workspaceError("JOURNAL_DIVERGENCE", "working-copy history digest is inconsistent", ErrPersistenceCorrupt)
	}
	return w.publishWorkingCopyLocked(candidate, prepared, entry, journal), nil
}

// PaperReloadExternal applies a filesystem/editor reload only against the
// exact observed candidate head. A stale reload reports opaque revisions and
// leaves source, undo, and redo state untouched.
func (w *Workspace) PaperReloadExternal(request PaperExternalReloadRequest) (PaperExternalReloadResult, error) {
	if w == nil {
		return PaperExternalReloadResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := w.validateWorkingCopyInput(request.Source, ""); err != nil {
		return PaperExternalReloadResult{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	candidate, err := w.candidateLocked(request.Candidate)
	if err != nil {
		return PaperExternalReloadResult{}, err
	}
	current, err := w.revisionLocked(candidate.head)
	if err != nil {
		return PaperExternalReloadResult{}, err
	}
	if candidate.head != request.ExpectedHead || current.revision != request.ExpectedRevision {
		conflict := &paperedit.ExternalReloadConflict{ExpectedRevision: request.ExpectedRevision,
			CurrentRevision: current.revision, ExternalRevision: paperedit.SourceRevision(request.Source)}
		return PaperExternalReloadResult{WorkingCopyResult: workingCopyResult(candidate, current, candidate.journal.Snapshot(), paperedit.JournalEntry{}), Conflict: conflict},
			workspaceError("EXTERNAL_RELOAD_CONFLICT", "external source conflicts with the current working-copy head", ErrRevisionConflict)
	}
	prepared, err := w.prepareRevision(current.file, request.Source)
	if err != nil {
		return PaperExternalReloadResult{}, err
	}
	if prepared.revision == current.revision {
		return PaperExternalReloadResult{WorkingCopyResult: workingCopyResult(candidate, current, candidate.journal.Snapshot(), paperedit.JournalEntry{})}, nil
	}
	if len(w.revisions) >= w.limits.MaxRevisions {
		return PaperExternalReloadResult{}, workspaceError("REVISION_LIMIT", "workspace revision capacity is exhausted", ErrLimit)
	}
	entry, conflict, journal, err := candidate.journal.ReloadExternal(request.ExpectedRevision, request.Source)
	if err != nil {
		return PaperExternalReloadResult{Conflict: conflict}, wrapJournalError(err)
	}
	return PaperExternalReloadResult{WorkingCopyResult: w.publishWorkingCopyLocked(candidate, prepared, entry, journal)}, nil
}

func (w *Workspace) validateWorkingCopyInput(source, group string) error {
	if len(source) > w.limits.MaxSourceBytes || !utf8.ValidString(source) {
		return workspaceError("SOURCE_LIMIT", "working-copy source must be valid UTF-8 within the configured source limit", ErrLimit)
	}
	if len(group) > w.limits.MaxQueryBytes || !utf8.ValidString(group) {
		return workspaceError("GROUP_LIMIT", "working-copy edit group exceeds the configured query limit", ErrLimit)
	}
	return nil
}

func (w *Workspace) prepareWorkingCopyRevision(candidateHandle CandidateHandle, source string) (*revisionRecord, error) {
	w.mu.RLock()
	candidate, err := w.candidateLocked(candidateHandle)
	if err != nil {
		w.mu.RUnlock()
		return nil, err
	}
	current, err := w.revisionLocked(candidate.head)
	if err != nil {
		w.mu.RUnlock()
		return nil, err
	}
	file := current.file
	w.mu.RUnlock()
	return w.prepareRevision(file, source)
}

func (w *Workspace) exactWorkingCopyLocked(handle CandidateHandle, expectedHead RevisionHandle, expectedRevision paperedit.Revision) (*candidateRecord, *revisionRecord, error) {
	candidate, err := w.candidateLocked(handle)
	if err != nil {
		return nil, nil, err
	}
	if candidate.head != expectedHead {
		return nil, nil, workspaceError("REVISION_CONFLICT", "candidate head changed", ErrRevisionConflict)
	}
	revision, err := w.revisionLocked(candidate.head)
	if err != nil {
		return nil, nil, err
	}
	if revision.revision != expectedRevision {
		return nil, nil, workspaceError("REVISION_CONFLICT", "exact source revision does not match the candidate head", ErrRevisionConflict)
	}
	return candidate, revision, nil
}

func (w *Workspace) prepareRevisionForCandidateLocked(candidate *candidateRecord, source string) (*revisionRecord, error) {
	current, err := w.revisionLocked(candidate.head)
	if err != nil {
		return nil, err
	}
	return w.prepareRevision(current.file, source)
}

func (w *Workspace) publishWorkingCopyLocked(candidate *candidateRecord, prepared *revisionRecord, entry paperedit.JournalEntry, journal paperedit.JournalSnapshot) WorkingCopyResult {
	w.nextRevision++
	prepared.handle = RevisionHandle{value: w.newHandle(handleRevision, capabilityRead, w.nextRevision)}
	prepared.expires = w.expiresAt(w.handleTTL)
	prepared.disclosure = w.disclosureDomain
	prepared.partition = w.partition
	w.revisions[w.nextRevision] = prepared
	candidate.head = prepared.handle
	candidate.clearHeadCachesLocked()
	return workingCopyResult(candidate, prepared, journal, entry)
}

func workingCopyResult(candidate *candidateRecord, revision *revisionRecord, journal paperedit.JournalSnapshot, entry paperedit.JournalEntry) WorkingCopyResult {
	return WorkingCopyResult{Candidate: snapshotCandidate(candidate), Revision: snapshotOf(revision), Journal: journal, Entry: entry}
}
