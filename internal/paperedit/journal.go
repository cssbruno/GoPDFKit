// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"errors"
	"sort"
	"sync"
	"unicode/utf8"
)

const (
	MaxJournalEntriesHard = 65536
	MaxJournalBytesHard   = 256 << 20
	MaxJournalGroupBytes  = 4096
	MaxJournalFileBytes   = 4096
)

var (
	ErrJournalConflict = errors.New("paperedit: journal revision conflict")
	ErrJournalLimit    = errors.New("paperedit: journal limit exceeded")
	ErrExternalReload  = errors.New("paperedit: external source conflicts with current journal head")
)

type JournalChangeKind string

const (
	JournalSemanticEdit   JournalChangeKind = "semantic_edit"
	JournalSourceEdit     JournalChangeKind = "source_edit"
	JournalExternalReload JournalChangeKind = "external_reload"
)

type JournalLimits struct {
	MaxEntries int
	MaxBytes   int
}

func DefaultJournalLimits() JournalLimits {
	return JournalLimits{MaxEntries: 1024, MaxBytes: 32 << 20}
}

// JournalEntry is detached review evidence for one committed transition.
// Source snapshots remain private to the journal; Diff contains the minimal
// authored patch and may be selectively presented to a reviewer.
type JournalEntry struct {
	Sequence       uint64            `json:"sequence"`
	Kind           JournalChangeKind `json:"kind"`
	Group          string            `json:"group,omitempty"`
	BeforeRevision Revision          `json:"before_revision"`
	AfterRevision  Revision          `json:"after_revision"`
	Diff           SourceDiff        `json:"diff"`
	before         string
	after          string
}

type JournalSnapshot struct {
	File        string   `json:"file"`
	Revision    Revision `json:"revision"`
	Sequence    uint64   `json:"sequence"`
	UndoCount   int      `json:"undo_count"`
	RedoCount   int      `json:"redo_count"`
	CanUndo     bool     `json:"can_undo"`
	CanRedo     bool     `json:"can_redo"`
	SourceBytes int      `json:"source_bytes"`
}

// JournalState is the trusted persistence representation of a working-copy
// journal. It intentionally contains source snapshots and must never be sent
// through an untrusted agent transport or review response.
type JournalState struct {
	File     string              `json:"file"`
	Source   string              `json:"source"`
	Revision Revision            `json:"revision"`
	Sequence uint64              `json:"sequence"`
	Limits   JournalLimits       `json:"limits"`
	Undo     []JournalStateEntry `json:"undo,omitempty"`
	Redo     []JournalStateEntry `json:"redo,omitempty"`
}

type JournalStateEntry struct {
	Sequence       uint64            `json:"sequence"`
	Kind           JournalChangeKind `json:"kind"`
	Group          string            `json:"group,omitempty"`
	BeforeRevision Revision          `json:"before_revision"`
	AfterRevision  Revision          `json:"after_revision"`
	BeforeSource   string            `json:"before_source"`
	AfterSource    string            `json:"after_source"`
	Diff           SourceDiff        `json:"diff"`
}

type SemanticJournalRequest struct {
	ExpectedRevision    Revision
	Group               string
	IdempotencyKey      string
	TargetPreconditions []TargetPrecondition
	RequireExactTargets bool
	Operations          []Operation
}

type SourceJournalRequest struct {
	ExpectedRevision Revision
	Group            string
	Source           string
}

type ExternalReloadConflict struct {
	ExpectedRevision Revision `json:"expected_revision"`
	CurrentRevision  Revision `json:"current_revision"`
	ExternalRevision Revision `json:"external_revision"`
}

// Journal serializes one working copy. Both source-editor and semantic edits
// advance the same exact revision head and therefore share undo/redo ordering.
type Journal struct {
	mu       sync.Mutex
	file     string
	source   string
	revision Revision
	sequence uint64
	limits   JournalLimits
	undo     []JournalEntry
	redo     []JournalEntry
}

func NewJournal(file, source string, limits JournalLimits) (*Journal, error) {
	if limits == (JournalLimits{}) {
		limits = DefaultJournalLimits()
	}
	if limits.MaxEntries <= 0 || limits.MaxEntries > MaxJournalEntriesHard || limits.MaxBytes <= 0 || limits.MaxBytes > MaxJournalBytesHard ||
		len(file) == 0 || len(file) > MaxJournalFileBytes || !utf8.ValidString(file) ||
		len(source) > MaxSourceBytes || len(file)+len(source) > limits.MaxBytes || !utf8.ValidString(source) {
		return nil, ErrJournalLimit
	}
	return &Journal{file: file, source: source, revision: SourceRevision(source), limits: limits}, nil
}

// ExportState returns a detached, persistence-only copy of the complete undo
// and redo graph. Review callers should use Entries, which withholds source
// snapshots.
func (journal *Journal) ExportState() JournalState {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journalStateLocked(journal)
}

// RestoreJournal validates every retained snapshot, digest, transition, and
// bound before admitting persisted working-copy history.
func RestoreJournal(state JournalState) (*Journal, error) {
	journal, err := NewJournal(state.File, state.Source, state.Limits)
	if err != nil || state.Revision != SourceRevision(state.Source) {
		return nil, ErrJournalLimit
	}
	journal.sequence = state.Sequence
	journal.undo, err = restoreJournalEntries(state.Undo, state.Sequence)
	if err != nil {
		return nil, err
	}
	journal.redo, err = restoreJournalEntries(state.Redo, state.Sequence)
	if err != nil {
		return nil, err
	}
	journal.revision = state.Revision
	if len(journal.undo) > 0 && journal.undo[len(journal.undo)-1].AfterRevision != journal.revision {
		return nil, ErrJournalConflict
	}
	if len(journal.redo) > 0 && journal.redo[len(journal.redo)-1].BeforeRevision != journal.revision {
		return nil, ErrJournalConflict
	}
	for index := 1; index < len(journal.undo); index++ {
		if journal.undo[index-1].AfterRevision != journal.undo[index].BeforeRevision {
			return nil, ErrJournalConflict
		}
	}
	for index := len(journal.redo) - 1; index > 0; index-- {
		if journal.redo[index].AfterRevision != journal.redo[index-1].BeforeRevision {
			return nil, ErrJournalConflict
		}
	}
	if len(journal.undo) > journal.limits.MaxEntries || len(journal.redo) > journal.limits.MaxEntries ||
		len(journal.file)+journalBytes(journal.source, journal.undo, journal.redo) > journal.limits.MaxBytes {
		return nil, ErrJournalLimit
	}
	return journal, nil
}

func journalStateLocked(journal *Journal) JournalState {
	state := JournalState{File: journal.file, Source: journal.source, Revision: journal.revision,
		Sequence: journal.sequence, Limits: journal.limits}
	state.Undo = exportJournalEntries(journal.undo)
	state.Redo = exportJournalEntries(journal.redo)
	return state
}

func exportJournalEntries(entries []JournalEntry) []JournalStateEntry {
	result := make([]JournalStateEntry, len(entries))
	for index, entry := range entries {
		result[index] = JournalStateEntry{Sequence: entry.Sequence, Kind: entry.Kind, Group: entry.Group,
			BeforeRevision: entry.BeforeRevision, AfterRevision: entry.AfterRevision,
			BeforeSource: entry.before, AfterSource: entry.after, Diff: cloneSourceDiff(entry.Diff)}
	}
	return result
}

func restoreJournalEntries(entries []JournalStateEntry, sequence uint64) ([]JournalEntry, error) {
	result := make([]JournalEntry, len(entries))
	for index, entry := range entries {
		if entry.Sequence == 0 || entry.Sequence > sequence || !validJournalKind(entry.Kind) || !validJournalGroup(entry.Group) ||
			!utf8.ValidString(entry.BeforeSource) || !utf8.ValidString(entry.AfterSource) ||
			len(entry.BeforeSource) > MaxSourceBytes || len(entry.AfterSource) > MaxSourceBytes ||
			entry.BeforeRevision != SourceRevision(entry.BeforeSource) || entry.AfterRevision != SourceRevision(entry.AfterSource) {
			return nil, ErrJournalConflict
		}
		diff := entry.Diff
		if len(diff.Patches) == 0 {
			diff = minimalSourceDiff(entry.BeforeSource, entry.AfterSource)
		} else if !validJournalStateDiff(entry.BeforeSource, entry.AfterSource, diff) {
			return nil, ErrJournalConflict
		}
		result[index] = JournalEntry{Sequence: entry.Sequence, Kind: entry.Kind, Group: entry.Group,
			BeforeRevision: entry.BeforeRevision, AfterRevision: entry.AfterRevision,
			Diff: cloneSourceDiff(diff), before: entry.BeforeSource, after: entry.AfterSource}
	}
	return result, nil
}

func validJournalStateDiff(before, after string, diff SourceDiff) bool {
	if diff.BeforeRevision != SourceRevision(before) || diff.AfterRevision != SourceRevision(after) {
		return false
	}
	patches := append([]SourcePatch(nil), diff.Patches...)
	sort.Slice(patches, func(i, j int) bool { return patches[i].Start > patches[j].Start })
	result := before
	previousStart := uint32(len(before) + 1)
	for _, patch := range patches {
		if patch.Start > patch.End || uint64(patch.End) > uint64(len(result)) || patch.End > previousStart ||
			!utf8.ValidString(patch.Target) || len(patch.Target) > MaxJournalGroupBytes ||
			result[patch.Start:patch.End] != patch.Removed {
			return false
		}
		result = result[:patch.Start] + patch.Replacement + result[patch.End:]
		previousStart = patch.Start
	}
	return result == after
}

func validJournalKind(kind JournalChangeKind) bool {
	return kind == JournalSemanticEdit || kind == JournalSourceEdit || kind == JournalExternalReload
}

func (journal *Journal) Snapshot() JournalSnapshot {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.snapshotLocked()
}

func (journal *Journal) Source() (string, Revision) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.source, journal.revision
}

func (journal *Journal) Entries() []JournalEntry {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	entries := make([]JournalEntry, len(journal.undo))
	for index, entry := range journal.undo {
		entries[index] = cloneJournalEntry(entry)
	}
	return entries
}

func (journal *Journal) ApplySemantic(request SemanticJournalRequest) (Result, JournalSnapshot, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if !validJournalGroup(request.Group) {
		return Result{File: journal.file, Source: journal.source, Revision: journal.revision}, journal.snapshotLocked(), ErrJournalLimit
	}
	if request.ExpectedRevision != journal.revision {
		return Result{File: journal.file, Source: journal.source, Revision: journal.revision}, journal.snapshotLocked(), ErrJournalConflict
	}
	result, err := Apply(Transaction{File: journal.file, Source: journal.source, ExpectedRevision: request.ExpectedRevision,
		IdempotencyKey: request.IdempotencyKey, TargetPreconditions: request.TargetPreconditions,
		RequireExactTargets: request.RequireExactTargets, Operations: request.Operations})
	if err != nil || !result.Applied || result.Diff == nil {
		return result, journal.snapshotLocked(), err
	}
	entry := JournalEntry{Kind: JournalSemanticEdit, Group: request.Group, BeforeRevision: journal.revision,
		AfterRevision: result.Revision, Diff: cloneSourceDiff(*result.Diff), before: journal.source, after: result.Source}
	if err := journal.commitLocked(entry, false); err != nil {
		return Result{File: journal.file, Source: journal.source, Revision: journal.revision}, journal.snapshotLocked(), err
	}
	return result, journal.snapshotLocked(), nil
}

func (journal *Journal) ApplySource(request SourceJournalRequest) (JournalEntry, JournalSnapshot, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	return journal.applySourceLocked(request, JournalSourceEdit, true)
}

// ReloadExternal advances only when the caller proves the exact journal head
// it observed. A conflict returns both opaque revisions and retains all undo,
// redo, and current source state unchanged.
func (journal *Journal) ReloadExternal(expected Revision, source string) (JournalEntry, *ExternalReloadConflict, JournalSnapshot, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	external := SourceRevision(source)
	if expected != journal.revision {
		conflict := &ExternalReloadConflict{ExpectedRevision: expected, CurrentRevision: journal.revision, ExternalRevision: external}
		return JournalEntry{}, conflict, journal.snapshotLocked(), ErrExternalReload
	}
	entry, snapshot, err := journal.applySourceLocked(SourceJournalRequest{ExpectedRevision: expected, Source: source}, JournalExternalReload, false)
	return entry, nil, snapshot, err
}

func (journal *Journal) Undo(expected Revision) (JournalEntry, JournalSnapshot, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if expected != journal.revision || len(journal.undo) == 0 {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalConflict
	}
	entry := journal.undo[len(journal.undo)-1]
	if entry.AfterRevision != journal.revision {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalConflict
	}
	journal.undo = journal.undo[:len(journal.undo)-1]
	journal.redo = append(journal.redo, entry)
	journal.source, journal.revision = entry.before, entry.BeforeRevision
	return cloneJournalEntry(entry), journal.snapshotLocked(), nil
}

func (journal *Journal) Redo(expected Revision) (JournalEntry, JournalSnapshot, error) {
	journal.mu.Lock()
	defer journal.mu.Unlock()
	if expected != journal.revision || len(journal.redo) == 0 {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalConflict
	}
	entry := journal.redo[len(journal.redo)-1]
	if entry.BeforeRevision != journal.revision {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalConflict
	}
	journal.redo = journal.redo[:len(journal.redo)-1]
	journal.undo = append(journal.undo, entry)
	journal.source, journal.revision = entry.after, entry.AfterRevision
	return cloneJournalEntry(entry), journal.snapshotLocked(), nil
}

func (journal *Journal) applySourceLocked(request SourceJournalRequest, kind JournalChangeKind, coalesce bool) (JournalEntry, JournalSnapshot, error) {
	if !validJournalGroup(request.Group) {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalLimit
	}
	if request.ExpectedRevision != journal.revision {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalConflict
	}
	if len(request.Source) > MaxSourceBytes || !utf8.ValidString(request.Source) {
		return JournalEntry{}, journal.snapshotLocked(), ErrJournalLimit
	}
	after := SourceRevision(request.Source)
	if after == journal.revision {
		return JournalEntry{}, journal.snapshotLocked(), nil
	}
	diff := minimalSourceDiff(journal.source, request.Source)
	entry := JournalEntry{Kind: kind, Group: request.Group, BeforeRevision: journal.revision, AfterRevision: after,
		Diff: diff, before: journal.source, after: request.Source}
	if err := journal.commitLocked(entry, coalesce); err != nil {
		return JournalEntry{}, journal.snapshotLocked(), err
	}
	return cloneJournalEntry(journal.undo[len(journal.undo)-1]), journal.snapshotLocked(), nil
}

func (journal *Journal) commitLocked(entry JournalEntry, coalesce bool) error {
	entry.Sequence = journal.sequence + 1
	undo := append([]JournalEntry(nil), journal.undo...)
	if coalesce && entry.Kind == JournalSourceEdit && entry.Group != "" && len(undo) > 0 {
		last := &undo[len(undo)-1]
		if last.Kind == JournalSourceEdit && last.Group == entry.Group && last.AfterRevision == entry.BeforeRevision {
			last.AfterRevision, last.after = entry.AfterRevision, entry.after
			last.Diff = minimalSourceDiff(last.before, last.after)
			last.Sequence = entry.Sequence
		} else {
			undo = append(undo, entry)
		}
	} else {
		undo = append(undo, entry)
	}
	for len(undo) > journal.limits.MaxEntries {
		undo = undo[1:]
	}
	for len(journal.file)+journalBytes(entry.after, undo, nil) > journal.limits.MaxBytes && len(undo) > 1 {
		undo = undo[1:]
	}
	if len(journal.file)+journalBytes(entry.after, undo, nil) > journal.limits.MaxBytes {
		return ErrJournalLimit
	}
	journal.undo, journal.redo = undo, nil
	journal.source, journal.revision = entry.after, entry.AfterRevision
	journal.sequence++
	return nil
}

func (journal *Journal) snapshotLocked() JournalSnapshot {
	return JournalSnapshot{File: journal.file, Revision: journal.revision, Sequence: journal.sequence,
		UndoCount: len(journal.undo), RedoCount: len(journal.redo), CanUndo: len(journal.undo) > 0,
		CanRedo: len(journal.redo) > 0, SourceBytes: len(journal.source)}
}

func journalBytes(head string, undo, redo []JournalEntry) int {
	total := len(head)
	for _, list := range [][]JournalEntry{undo, redo} {
		for _, entry := range list {
			total += len(entry.Group) + len(entry.before) + len(entry.after)
			for _, patch := range entry.Diff.Patches {
				total += len(patch.Removed) + len(patch.Replacement)
			}
		}
	}
	return total
}

func validJournalGroup(group string) bool {
	return len(group) <= MaxJournalGroupBytes && utf8.ValidString(group)
}

func minimalSourceDiff(before, after string) SourceDiff {
	prefixBefore, prefixAfter := 0, 0
	beforeRunes, afterRunes := []rune(before), []rune(after)
	common := 0
	for common < len(beforeRunes) && common < len(afterRunes) && beforeRunes[common] == afterRunes[common] {
		common++
	}
	for _, value := range beforeRunes[:common] {
		prefixBefore += utf8.RuneLen(value)
	}
	for _, value := range afterRunes[:common] {
		prefixAfter += utf8.RuneLen(value)
	}
	suffix := 0
	for suffix < len(beforeRunes)-common && suffix < len(afterRunes)-common && beforeRunes[len(beforeRunes)-1-suffix] == afterRunes[len(afterRunes)-1-suffix] {
		suffix++
	}
	endBefore, endAfter := len(before), len(after)
	for _, value := range beforeRunes[len(beforeRunes)-suffix:] {
		endBefore -= utf8.RuneLen(value)
	}
	for _, value := range afterRunes[len(afterRunes)-suffix:] {
		endAfter -= utf8.RuneLen(value)
	}
	return SourceDiff{BeforeRevision: SourceRevision(before), AfterRevision: SourceRevision(after), Patches: []SourcePatch{{
		Start: uint32(prefixBefore), End: uint32(endBefore), Removed: before[prefixBefore:endBefore], Replacement: after[prefixAfter:endAfter],
	}}}
}

func cloneJournalEntry(entry JournalEntry) JournalEntry {
	entry.Diff = cloneSourceDiff(entry.Diff)
	entry.before, entry.after = "", ""
	return entry
}

func cloneSourceDiff(diff SourceDiff) SourceDiff {
	diff.Patches = append([]SourcePatch(nil), diff.Patches...)
	return diff
}
