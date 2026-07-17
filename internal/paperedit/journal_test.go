// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestJournalSharesOneHeadAcrossSemanticAndSourceEdits(t *testing.T) {
	source := strings.Replace(editableFixture(), `text @copy: "Hello"`, "# keep this comment\n        text @copy: \"Hello\"", 1)
	journal, err := NewJournal("invoice.paper", source, JournalLimits{})
	if err != nil {
		t.Fatal(err)
	}

	semantic, snapshot, err := journal.ApplySemantic(SemanticJournalRequest{
		ExpectedRevision: SourceRevision(source),
		Group:            "agent-copy-edit",
		Operations:       []Operation{ReplaceText{Target: "@copy", Text: "Reviewed"}},
	})
	if err != nil || !semantic.Applied {
		t.Fatalf("ApplySemantic() = %+v, %v", semantic, err)
	}
	if !strings.Contains(semantic.Source, "# keep this comment") {
		t.Fatalf("semantic edit lost authored comment:\n%s", semantic.Source)
	}
	if snapshot.Revision != semantic.Revision || snapshot.UndoCount != 1 {
		t.Fatalf("semantic snapshot = %+v", snapshot)
	}

	afterSource := strings.Replace(semantic.Source, "# keep this comment", "# keep this reviewed comment", 1)
	entry, snapshot, err := journal.ApplySource(SourceJournalRequest{
		ExpectedRevision: semantic.Revision,
		Group:            "manual-comment-edit",
		Source:           afterSource,
	})
	if err != nil {
		t.Fatalf("ApplySource() = %v", err)
	}
	if entry.Kind != JournalSourceEdit || entry.BeforeRevision != semantic.Revision ||
		entry.AfterRevision != SourceRevision(afterSource) || snapshot.UndoCount != 2 {
		t.Fatalf("source entry/snapshot = %+v / %+v", entry, snapshot)
	}
	entries := journal.Entries()
	if len(entries) != 2 || entries[0].Kind != JournalSemanticEdit || entries[1].Kind != JournalSourceEdit {
		t.Fatalf("shared journal entries = %+v", entries)
	}
	if entries[0].before != "" || entries[0].after != "" {
		t.Fatal("Entries exposed private source snapshots")
	}
}

func TestJournalGroupsUnicodeTextEditsAndUndoRedoUsesExactHead(t *testing.T) {
	const source = "prefix 😀 suffix\n"
	journal, err := NewJournal("draft.paper", source, JournalLimits{})
	if err != nil {
		t.Fatal(err)
	}
	first := "prefix 😀1 suffix\n"
	_, firstSnapshot, err := journal.ApplySource(SourceJournalRequest{
		ExpectedRevision: SourceRevision(source), Group: "typing:copy", Source: first,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := "prefix 😀12 suffix\n"
	entry, secondSnapshot, err := journal.ApplySource(SourceJournalRequest{
		ExpectedRevision: firstSnapshot.Revision, Group: "typing:copy", Source: second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondSnapshot.UndoCount != 1 || len(journal.Entries()) != 1 {
		t.Fatalf("grouped edit did not coalesce: %+v / %+v", secondSnapshot, journal.Entries())
	}
	if entry.BeforeRevision != SourceRevision(source) || entry.AfterRevision != SourceRevision(second) ||
		len(entry.Diff.Patches) != 1 {
		t.Fatalf("coalesced entry = %+v", entry)
	}
	patch := entry.Diff.Patches[0]
	if !utf8.ValidString(patch.Removed) || !utf8.ValidString(patch.Replacement) ||
		applyExportedPatches(t, source, entry.Diff.Patches) != second {
		t.Fatalf("Unicode-aware diff = %+v", entry.Diff)
	}

	if _, _, err := journal.Undo(firstSnapshot.Revision); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("stale Undo() = %v, want ErrJournalConflict", err)
	}
	current, revision := journal.Source()
	if current != second || revision != secondSnapshot.Revision {
		t.Fatalf("stale undo mutated journal: %q / %s", current, revision)
	}
	_, undone, err := journal.Undo(secondSnapshot.Revision)
	if err != nil || undone.Revision != SourceRevision(source) || undone.RedoCount != 1 {
		t.Fatalf("Undo() = %+v, %v", undone, err)
	}
	_, redone, err := journal.Redo(undone.Revision)
	if err != nil || redone.Revision != SourceRevision(second) || redone.UndoCount != 1 {
		t.Fatalf("Redo() = %+v, %v", redone, err)
	}
}

func TestJournalExternalReloadConflictPreservesCandidateAndHistory(t *testing.T) {
	const source = "document @doc:\n"
	journal, err := NewJournal("draft.paper", source, JournalLimits{})
	if err != nil {
		t.Fatal(err)
	}
	local := source + "  language: \"en\"\n"
	_, localSnapshot, err := journal.ApplySource(SourceJournalRequest{
		ExpectedRevision: SourceRevision(source), Group: "local", Source: local,
	})
	if err != nil {
		t.Fatal(err)
	}
	external := source + "  language: \"pt-BR\"\n"
	_, conflict, conflictSnapshot, err := journal.ReloadExternal(SourceRevision(source), external)
	if !errors.Is(err, ErrExternalReload) || conflict == nil {
		t.Fatalf("ReloadExternal(stale) = %+v, %v", conflict, err)
	}
	if conflict.CurrentRevision != localSnapshot.Revision || conflict.ExternalRevision != SourceRevision(external) ||
		conflictSnapshot != localSnapshot {
		t.Fatalf("reload conflict/snapshot = %+v / %+v, want %+v", conflict, conflictSnapshot, localSnapshot)
	}
	if current, _ := journal.Source(); current != local || len(journal.Entries()) != 1 {
		t.Fatalf("conflicting reload discarded candidate/history: %q / %+v", current, journal.Entries())
	}
	entry, conflict, accepted, err := journal.ReloadExternal(localSnapshot.Revision, external)
	if err != nil || conflict != nil || entry.Kind != JournalExternalReload || accepted.UndoCount != 2 {
		t.Fatalf("ReloadExternal(current) = %+v, %+v, %v", entry, accepted, err)
	}
	_, undone, err := journal.Undo(accepted.Revision)
	if err != nil || undone.Revision != localSnapshot.Revision {
		t.Fatalf("undo external reload = %+v, %v", undone, err)
	}
}

func TestJournalEnforcesEntryAndByteBounds(t *testing.T) {
	journal, err := NewJournal("draft.paper", "a", JournalLimits{MaxEntries: 2, MaxBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	revision := SourceRevision("a")
	for index, source := range []string{"ab", "abc", "abcd"} {
		_, snapshot, applyErr := journal.ApplySource(SourceJournalRequest{
			ExpectedRevision: revision, Group: fmt.Sprintf("edit-%d", index), Source: source,
		})
		if applyErr != nil {
			t.Fatalf("edit %d = %v", index, applyErr)
		}
		revision = snapshot.Revision
	}
	if snapshot := journal.Snapshot(); snapshot.UndoCount != 2 {
		t.Fatalf("bounded snapshot = %+v", snapshot)
	}
	before, beforeRevision := journal.Source()
	_, _, err = journal.ApplySource(SourceJournalRequest{
		ExpectedRevision: beforeRevision, Source: strings.Repeat("x", 65),
	})
	if !errors.Is(err, ErrJournalLimit) {
		t.Fatalf("oversized edit = %v, want ErrJournalLimit", err)
	}
	if after, afterRevision := journal.Source(); after != before || afterRevision != beforeRevision {
		t.Fatalf("oversized edit mutated journal: %q/%s", after, afterRevision)
	}
}

func TestJournalConcurrentWritersAllowExactlyOneExpectedRevision(t *testing.T) {
	journal, err := NewJournal("draft.paper", "base", JournalLimits{})
	if err != nil {
		t.Fatal(err)
	}
	const writers = 16
	start := make(chan struct{})
	errorsByWriter := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, _, applyErr := journal.ApplySource(SourceJournalRequest{
				ExpectedRevision: SourceRevision("base"),
				Group:            fmt.Sprintf("writer-%d", index),
				Source:           fmt.Sprintf("writer %d", index),
			})
			errorsByWriter <- applyErr
		}(index)
	}
	close(start)
	wait.Wait()
	close(errorsByWriter)
	succeeded, conflicted := 0, 0
	for applyErr := range errorsByWriter {
		switch {
		case applyErr == nil:
			succeeded++
		case errors.Is(applyErr, ErrJournalConflict):
			conflicted++
		default:
			t.Fatalf("unexpected writer error: %v", applyErr)
		}
	}
	if succeeded != 1 || conflicted != writers-1 {
		t.Fatalf("writers succeeded/conflicted = %d/%d", succeeded, conflicted)
	}
	if snapshot := journal.Snapshot(); snapshot.UndoCount != 1 || snapshot.Sequence != 1 {
		t.Fatalf("concurrent snapshot = %+v", snapshot)
	}
}

func TestJournalRejectsUnboundedOrCorruptGroupsWithoutMutation(t *testing.T) {
	journal, err := NewJournal("draft.paper", "base", JournalLimits{})
	if err != nil {
		t.Fatal(err)
	}
	initial := journal.Snapshot()
	for _, group := range []string{strings.Repeat("g", MaxJournalGroupBytes+1), string([]byte{0xff})} {
		if _, snapshot, err := journal.ApplySource(SourceJournalRequest{ExpectedRevision: initial.Revision, Group: group, Source: "changed"}); !errors.Is(err, ErrJournalLimit) || snapshot != initial {
			t.Fatalf("ApplySource(group) = %+v, %v", snapshot, err)
		}
		if _, snapshot, err := journal.ApplySemantic(SemanticJournalRequest{ExpectedRevision: initial.Revision, Group: group,
			Operations: []Operation{ReplaceText{Target: "@missing", Text: "x"}}}); !errors.Is(err, ErrJournalLimit) || snapshot != initial {
			t.Fatalf("ApplySemantic(group) = %+v, %v", snapshot, err)
		}
	}
	state := journal.ExportState()
	state.Sequence = 1
	state.Undo = []JournalStateEntry{{Sequence: 1, Kind: JournalSourceEdit, Group: strings.Repeat("g", MaxJournalGroupBytes+1),
		BeforeRevision: SourceRevision("base"), AfterRevision: SourceRevision("changed"), BeforeSource: "base", AfterSource: "changed"}}
	state.Source, state.Revision = "changed", SourceRevision("changed")
	if _, err := RestoreJournal(state); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("RestoreJournal(oversized group) = %v", err)
	}
}

func TestRestoreJournalRejectsTamperedExactPersistedDiff(t *testing.T) {
	journal, err := NewJournal("draft.paper", "base", JournalLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := journal.ApplySource(SourceJournalRequest{ExpectedRevision: SourceRevision("base"), Group: "editor", Source: "changed"}); err != nil {
		t.Fatal(err)
	}
	state := journal.ExportState()
	if len(state.Undo) != 1 || len(state.Undo[0].Diff.Patches) != 1 {
		t.Fatalf("persisted exact diff = %+v", state.Undo)
	}
	state.Undo[0].Diff.Patches[0].Removed = "forged"
	if _, err := RestoreJournal(state); !errors.Is(err, ErrJournalConflict) {
		t.Fatalf("RestoreJournal(tampered diff) = %v", err)
	}
}
