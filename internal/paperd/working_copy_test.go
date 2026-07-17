// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

func TestWorkspaceSourceAndSemanticEditsShareJournalAndExactHistory(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	if err != nil {
		t.Fatal(err)
	}
	semantic, err := workspace.Apply(ApplyRequest{
		Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle, ExpectedRevision: created.Revision.Revision,
		Group: "agent-copy", TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "report.paper", workspaceFixture, "@copy")},
		Operations: []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "Reviewed"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err := workspace.PaperWorkingCopy(created.Candidate.Handle)
	if err != nil || view.Journal.UndoCount != 1 || len(view.Entries) != 1 || view.Entries[0].Kind != paperedit.JournalSemanticEdit {
		t.Fatalf("semantic journal = %+v, %v", view, err)
	}

	firstSource := strings.Replace(semantic.Revision.Source, "Agent report", "Agent report!", 1)
	first, err := workspace.PaperApplySource(PaperSourceEditRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: semantic.Revision.Handle, ExpectedRevision: semantic.Revision.Revision, Group: "typing:title", Source: firstSource})
	if err != nil {
		t.Fatal(err)
	}
	secondSource := strings.Replace(firstSource, "Agent report!", "Agent report!!", 1)
	second, err := workspace.PaperApplySource(PaperSourceEditRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: first.Revision.Handle, ExpectedRevision: first.Revision.Revision, Group: "typing:title", Source: secondSource})
	if err != nil {
		t.Fatal(err)
	}
	if second.Journal.UndoCount != 2 || second.Entry.BeforeRevision != semantic.Revision.Revision || second.Entry.AfterRevision != second.Revision.Revision {
		t.Fatalf("coalesced source journal = %+v", second)
	}
	if _, err := workspace.PaperUndo(PaperHistoryRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: first.Revision.Handle, ExpectedRevision: first.Revision.Revision}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale undo = %v", err)
	}
	unchanged, _ := workspace.Candidate(created.Candidate.Handle)
	if unchanged.Head != second.Revision.Handle {
		t.Fatal("stale undo moved candidate head")
	}
	undone, err := workspace.PaperUndo(PaperHistoryRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: second.Revision.Handle, ExpectedRevision: second.Revision.Revision})
	if err != nil || undone.Revision.Source != semantic.Revision.Source || undone.Journal.RedoCount != 1 {
		t.Fatalf("undo = %+v, %v", undone, err)
	}
	redone, err := workspace.PaperRedo(PaperHistoryRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: undone.Revision.Handle, ExpectedRevision: undone.Revision.Revision})
	if err != nil || redone.Revision.Source != secondSource || redone.Journal.RedoCount != 0 {
		t.Fatalf("redo = %+v, %v", redone, err)
	}
	if base, _ := workspace.OpenRevision(created.Revision.Handle); base.Source != workspaceFixture {
		t.Fatal("history changed an immutable base revision")
	}
}

func TestWorkspaceExternalReloadConflictPreservesCandidateHistory(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	localSource := strings.Replace(workspaceFixture, "Agent report", "Local report", 1)
	local, err := workspace.PaperApplySource(PaperSourceEditRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: created.Revision.Handle, ExpectedRevision: created.Revision.Revision, Group: "local", Source: localSource})
	if err != nil {
		t.Fatal(err)
	}
	externalSource := strings.Replace(workspaceFixture, "Agent report", "External report", 1)
	conflicted, err := workspace.PaperReloadExternal(PaperExternalReloadRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: created.Revision.Handle, ExpectedRevision: created.Revision.Revision, Source: externalSource})
	if !errors.Is(err, ErrRevisionConflict) || conflicted.Conflict == nil || conflicted.Conflict.CurrentRevision != local.Revision.Revision {
		t.Fatalf("stale reload = %+v, %v", conflicted, err)
	}
	afterConflict, _ := workspace.PaperWorkingCopy(created.Candidate.Handle)
	if afterConflict.Revision.Source != localSource || afterConflict.Journal != local.Journal || len(afterConflict.Entries) != 1 {
		t.Fatalf("reload conflict changed working copy = %+v", afterConflict)
	}
	accepted, err := workspace.PaperReloadExternal(PaperExternalReloadRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: local.Revision.Handle, ExpectedRevision: local.Revision.Revision, Source: externalSource})
	if err != nil || accepted.Conflict != nil || accepted.Entry.Kind != paperedit.JournalExternalReload || accepted.Journal.UndoCount != 2 {
		t.Fatalf("accepted reload = %+v, %v", accepted, err)
	}
	undone, err := workspace.PaperUndo(PaperHistoryRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: accepted.Revision.Handle, ExpectedRevision: accepted.Revision.Revision})
	if err != nil || undone.Revision.Source != localSource {
		t.Fatalf("undo reload = %+v, %v", undone, err)
	}
}

func TestWorkspaceWorkingCopyCASAllowsOneSemanticOrSourceWriter(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	precondition := exactTargetPrecondition(t, "report.paper", workspaceFixture, "@copy")
	editorSource := strings.Replace(workspaceFixture, "Agent report", "Editor report", 1)
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		_, err := workspace.Apply(ApplyRequest{Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
			ExpectedRevision: created.Revision.Revision, TargetPreconditions: []paperedit.TargetPrecondition{precondition},
			Operations: []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "Agent"}}})
		errorsSeen <- err
	}()
	go func() {
		defer wait.Done()
		<-start
		_, err := workspace.PaperApplySource(PaperSourceEditRequest{Candidate: created.Candidate.Handle,
			ExpectedHead: created.Revision.Handle, ExpectedRevision: created.Revision.Revision, Group: "editor", Source: editorSource})
		errorsSeen <- err
	}()
	close(start)
	wait.Wait()
	close(errorsSeen)
	success, conflict := 0, 0
	for err := range errorsSeen {
		if err == nil {
			success++
		} else if errors.Is(err, ErrRevisionConflict) {
			conflict++
		} else {
			t.Fatalf("writer error = %v", err)
		}
	}
	view, err := workspace.PaperWorkingCopy(created.Candidate.Handle)
	if success != 1 || conflict != 1 || err != nil || view.Journal.Sequence != 1 || view.Journal.UndoCount != 1 {
		t.Fatalf("CAS success/conflict/view = %d/%d/%+v, %v", success, conflict, view, err)
	}
}

func TestWorkspaceWorkingCopyJournalSurvivesRecoveryWithRedo(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	workspace, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	firstSource := strings.Replace(workspaceFixture, "Agent report", "Recovered report", 1)
	first, err := workspace.PaperApplySource(PaperSourceEditRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: created.Revision.Handle, ExpectedRevision: created.Revision.Revision, Group: "typing", Source: firstSource})
	if err != nil {
		t.Fatal(err)
	}
	undone, err := workspace.PaperUndo(PaperHistoryRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: first.Revision.Handle, ExpectedRevision: first.Revision.Revision})
	if err != nil || undone.Journal.RedoCount != 1 {
		t.Fatalf("undo before save = %+v, %v", undone, err)
	}
	if err := workspace.SaveSnapshot(context.Background()); err != nil {
		t.Fatal(err)
	}
	recovered, err := OpenWorkspace(context.Background(), persistenceOptions(root))
	if err != nil {
		t.Fatal(err)
	}
	candidate := recovered.candidates[1]
	view, err := recovered.PaperWorkingCopy(candidate.handle)
	if err != nil || view.Revision.Source != workspaceFixture || view.Journal.RedoCount != 1 || view.Journal.UndoCount != 0 {
		t.Fatalf("recovered journal = %+v, %v", view, err)
	}
	redone, err := recovered.PaperRedo(PaperHistoryRequest{Candidate: candidate.handle,
		ExpectedHead: view.Revision.Handle, ExpectedRevision: view.Revision.Revision})
	if err != nil || redone.Revision.Source != firstSource || redone.Journal.RedoCount != 0 {
		t.Fatalf("recovered redo = %+v, %v", redone, err)
	}
}

func TestWorkspaceRawHeadMoveInvalidatesSemanticAndAcceptanceReceipts(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	semanticRequest := ApplyRequest{Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
		ExpectedRevision: created.Revision.Revision, IdempotencyKey: "semantic-before-raw",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "report.paper", workspaceFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "Reviewed"}}}
	semantic, err := workspace.Apply(semanticRequest)
	if err != nil {
		t.Fatal(err)
	}
	workspace.mu.Lock()
	record := workspace.candidates[created.Candidate.Handle.value.serial]
	record.acceptance = &candidateAcceptanceRecord{}
	record.acceptanceIdempotency["accepted-old-head"] = candidateAcceptanceIdempotencyRecord{}
	workspace.mu.Unlock()
	rawSource := strings.Replace(semantic.Revision.Source, "Agent report", "Raw report", 1)
	raw, err := workspace.PaperApplySource(PaperSourceEditRequest{Candidate: created.Candidate.Handle,
		ExpectedHead: semantic.Revision.Handle, ExpectedRevision: semantic.Revision.Revision, Group: "raw", Source: rawSource})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Apply(semanticRequest); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("old semantic idempotency replay = %v", err)
	}
	workspace.mu.RLock()
	record = workspace.candidates[created.Candidate.Handle.value.serial]
	acceptance, acceptanceReceipts, semanticReceipts := record.acceptance, len(record.acceptanceIdempotency), len(record.idempotency)
	workspace.mu.RUnlock()
	current, _ := workspace.PaperWorkingCopy(created.Candidate.Handle)
	if current.Revision.Handle != raw.Revision.Handle || acceptance != nil || acceptanceReceipts != 0 || semanticReceipts != 0 {
		t.Fatalf("raw cache invalidation = head %v acceptance=%v/%d semantic=%d", current.Revision.Handle == raw.Revision.Handle, acceptance, acceptanceReceipts, semanticReceipts)
	}
}

func TestWorkspaceJournalDivergenceRestoresCompleteCheckpoint(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	created, _ := workspace.PaperCreate(PaperCreateRequest{File: "report.paper", Source: workspaceFixture})
	candidate := workspace.candidates[created.Candidate.Handle.value.serial]
	changed := strings.Replace(workspaceFixture, "Agent report", "Changed report", 1)
	_, changedSnapshot, err := candidate.journal.ApplySource(paperedit.SourceJournalRequest{ExpectedRevision: created.Revision.Revision, Group: "first", Source: changed})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := candidate.journal.Undo(changedSnapshot.Revision); err != nil {
		t.Fatal(err)
	}
	checkpoint := candidate.journal.ExportState()
	divergent := strings.Replace(workspaceFixture, "Agent report", "Divergent report", 1)
	_, _, err = candidate.journal.ApplySource(paperedit.SourceJournalRequest{ExpectedRevision: created.Revision.Revision, Group: "branch", Source: divergent})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := workspace.prepareRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	err = reconcilePreparedJournalLocked(candidate, checkpoint, paperedit.Result{Source: divergent, Revision: paperedit.SourceRevision(divergent)}, prepared)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("reconcile divergence = %v", err)
	}
	if restored := candidate.journal.ExportState(); !reflect.DeepEqual(restored, checkpoint) {
		t.Fatalf("checkpoint was not restored:\n got  %+v\n want %+v", restored, checkpoint)
	}
}
