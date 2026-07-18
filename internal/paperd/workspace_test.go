// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

const workspaceFixture = "document @report:\n" +
	"  title: \"Agent report\"\n" +
	"  language: \"en\"\n" +
	"  page @sheet:\n" +
	"    size: \"A4\"\n" +
	"    margin: 24pt\n" +
	"    body @content:\n" +
	"      paragraph @intro:\n" +
	"        font: \"Helvetica\"\n" +
	"        size: 12pt\n" +
	"        text @copy: \"Hello agent\"\n"

func TestWorkspaceRevisionContextInspectSearchAndCompile(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxSearchResults: 8})
	revision, err := workspace.CreateRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatalf("CreateRevision() error = %v", err)
	}
	if !revision.ParseOK || !revision.CompileOK || revision.Revision != paperedit.SourceRevision(workspaceFixture) {
		t.Fatalf("revision = %#v", revision)
	}

	opened, err := workspace.OpenRevision(revision.Handle)
	if err != nil || opened.Source != workspaceFixture || opened.Handle != revision.Handle {
		t.Fatalf("OpenRevision() = %#v, %v", opened, err)
	}
	opened.ParseDiagnostics = append(opened.ParseDiagnostics, revision.ParseDiagnostics...)
	again, _ := workspace.OpenRevision(revision.Handle)
	if len(again.ParseDiagnostics) != 0 {
		t.Fatal("detached snapshot mutated retained diagnostics")
	}

	context, err := workspace.Context(revision.Handle)
	if err != nil || context.Title != "Agent report" || context.Language != "en" || context.BodyBlocks != 1 || context.Root.ID != "@report" {
		t.Fatalf("Context() = %#v, %v", context, err)
	}
	view, err := workspace.InspectID(revision.Handle, "@intro")
	if err != nil || view.Node.ID != "@intro" || len(view.Properties) != 2 || len(view.Children) != 1 {
		t.Fatalf("InspectID() = %#v, %v", view, err)
	}
	search, err := workspace.Search(SearchRequest{Revision: revision.Handle, Query: "hello", Limit: 2})
	if err != nil || search.Total != 1 || len(search.Matches) != 1 || search.Matches[0].Node.ID != "@copy" {
		t.Fatalf("Search() = %#v, %v", search, err)
	}
	compiled, err := workspace.Compile(revision.Handle)
	if err != nil || compiled.Title != "Agent report" || compiled.BodyBlocks != 1 || len(compiled.Mappings) != 5 {
		t.Fatalf("Compile() = %#v, %v", compiled, err)
	}
}

func TestWorkspaceHandlesAreScopedAndInvalidSourceIsInspectable(t *testing.T) {
	first := mustWorkspace(t, Limits{})
	second := mustWorkspace(t, Limits{})
	revision, err := first.CreateRevision("broken.paper", "document:\n  page\n")
	if err != nil || revision.ParseOK || len(revision.ParseDiagnostics) == 0 {
		t.Fatalf("invalid CreateRevision() = %#v, %v", revision, err)
	}
	if _, err := second.OpenRevision(revision.Handle); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("cross-workspace OpenRevision() error = %v", err)
	}
	if _, err := first.Compile(revision.Handle); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("Compile(invalid) error = %v", err)
	}
	if _, err := first.Render(revision.Handle); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("Render(invalid) error = %v", err)
	}
}

func TestWorkspaceApplyPublishesOneImmutableCandidateHead(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	base, err := workspace.CreateRevision("report.paper", workspaceFixture)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := workspace.NewCandidate(base.Handle)
	if err != nil {
		t.Fatal(err)
	}
	result, err := workspace.Apply(ApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedRevision: base.Revision,
		IdempotencyKey:      "workspace-edit-1",
		TargetPreconditions: []paperedit.TargetPrecondition{exactTargetPrecondition(t, "report.paper", workspaceFixture, "@copy")},
		Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "Edited"}},
	})
	if err != nil || !result.Edit.Applied || !strings.Contains(result.Revision.Source, `"Edited"`) {
		t.Fatalf("Apply() = %#v, %v", result, err)
	}
	if result.Revision.Handle == base.Handle || result.Candidate.Head != result.Revision.Handle {
		t.Fatalf("published handles = %#v", result)
	}
	if result.Edit.IdempotencyKey != "workspace-edit-1" || result.Edit.Diff == nil || result.Edit.Invalidation == nil {
		t.Fatalf("edit evidence was not propagated: %+v", result.Edit)
	}
	openedBase, _ := workspace.OpenRevision(base.Handle)
	if openedBase.Source != workspaceFixture {
		t.Fatal("base revision was mutated")
	}
	if _, err := workspace.Apply(ApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedRevision: base.Revision,
		Operations: []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "stale"}},
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale Apply() error = %v", err)
	}
	if _, err := workspace.Apply(ApplyRequest{
		Candidate: candidate.Handle, ExpectedHead: result.Revision.Handle, ExpectedRevision: base.Revision,
		Operations: []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: "bad digest"}},
	}); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("digest-conflict Apply() error = %v", err)
	}
}

func TestWorkspaceConcurrentApplyUsesHeadCompareAndSwap(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	base, _ := workspace.CreateRevision("report.paper", workspaceFixture)
	candidate, _ := workspace.NewCandidate(base.Handle)
	precondition := exactTargetPrecondition(t, "report.paper", workspaceFixture, "@copy")
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	errorsSeen := make(chan error, 2)
	for _, text := range []string{"first", "second"} {
		go func() {
			defer wait.Done()
			<-start
			_, err := workspace.Apply(ApplyRequest{
				Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedRevision: base.Revision,
				TargetPreconditions: []paperedit.TargetPrecondition{precondition},
				Operations:          []paperedit.Operation{paperedit.ReplaceText{Target: "@copy", Text: text}},
			})
			errorsSeen <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	succeeded, conflicted := 0, 0
	for err := range errorsSeen {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrRevisionConflict):
			conflicted++
		default:
			t.Fatalf("Apply() unexpected error = %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("success/conflict = %d/%d", succeeded, conflicted)
	}
}

func TestWorkspaceRenderUsesProductionPaperPipelineAndBoundsOutput(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	revision, _ := workspace.CreateRevision("report.paper", workspaceFixture)
	rendered, err := workspace.Render(revision.Handle)
	if err != nil || !rendered.Pipeline.OK() || !bytes.HasPrefix(rendered.PDF, []byte("%PDF-")) {
		t.Fatalf("Render() = pages %d, bytes %d, %v", rendered.Pipeline.Pages, len(rendered.PDF), err)
	}

	limited := mustWorkspace(t, Limits{MaxRenderBytes: 64})
	limitedRevision, _ := limited.CreateRevision("report.paper", workspaceFixture)
	if _, err := limited.Render(limitedRevision.Handle); !errors.Is(err, ErrLimit) {
		t.Fatalf("bounded Render() error = %v", err)
	}
}

func TestWorkspaceLimitsAreExplicitAndDeterministic(t *testing.T) {
	if _, err := NewWorkspace(Limits{MaxOperations: paperedit.MaxOperations + 1}); !errors.Is(err, ErrInvalidLimits) {
		t.Fatalf("NewWorkspace(invalid limits) error = %v", err)
	}
	workspace := mustWorkspace(t, Limits{MaxSourceBytes: 8, MaxRevisions: 1})
	if _, err := workspace.CreateRevision("x.paper", "more than eight bytes"); !errors.Is(err, ErrLimit) {
		t.Fatalf("CreateRevision(source limit) error = %v", err)
	}
	if _, err := workspace.Search(SearchRequest{Query: "x", Limit: 1}); !errors.Is(err, ErrInvalidHandle) {
		t.Fatalf("Search(empty handle) error = %v", err)
	}
}

func mustWorkspace(t *testing.T, limits Limits) *Workspace {
	t.Helper()
	workspace, err := NewWorkspace(limits)
	if err != nil {
		t.Fatalf("NewWorkspace() error = %v", err)
	}
	return workspace
}
