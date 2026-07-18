package paperd

import (
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

const tableMutationFixture = "document @report:\n  page @sheet:\n    body @body:\n      table @ledger:\n        repeat-header: true\n        split: \"rows\"\n        table-track @name-track:\n          width: 60pt\n        table-row @body-row:\n          cell @name:\n            text: \"Alpha\"\n"

const componentTableMutationFixture = "document @report:\n  component @table-card:\n    table @template-table:\n      split: \"rows\"\n      table-track @template-track:\n        width: 60pt\n      table-row @template-row:\n        cell @template-cell:\n          text: \"Alpha\"\n  page @sheet:\n    body @body:\n      use @instance:\n        component: \"@table-card\"\n"

func TestPaperSetTablePropertyUsesExactTableGuardAndMinimalPatch(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, tableMutationFixture, "@name-track", "table-track", CapabilityEdit)
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", tableMutationFixture, "@ledger")}
	result, err := workspace.PaperSetTableProperty(PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Points: 72})
	if err != nil || !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, "width: 72pt") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestPaperSetTablePropertyAcceptsContainerRelativeTrackWidth(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, tableMutationFixture, "@name-track", "table-responsive-track", CapabilityEdit)
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", tableMutationFixture, "@ledger")}
	result, err := workspace.PaperSetTableProperty(PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Length: "50%"})
	if err != nil || !result.Revision.CompileOK || !strings.Contains(result.Revision.Source, "width: 50%") {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestPaperSetTablePropertyRejectsMissingStaleAndAdversarialGuards(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, created, _ := mutationGuard(t, workspace, tableMutationFixture, "@name-track", "table-invalid", CapabilityEdit)
	if _, err := workspace.PaperSetTableProperty(PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Points: 72}); errorCode(err) != "TRANSITIVE_PRECONDITION_REQUIRED" {
		t.Fatalf("missing guard=%v", err)
	}
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", tableMutationFixture, "@ledger")}
	guard.TargetPreconditions[0].ExpectedFingerprint = paperedit.NodeFingerprint(strings.Repeat("0", 64))
	if _, err := workspace.PaperSetTableProperty(PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Points: 72}); errorCode(err) != "TRANSITIVE_PRECONDITION_CONFLICT" {
		t.Fatalf("stale guard=%v", err)
	}
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", tableMutationFixture, "@ledger")}
	if _, err := workspace.PaperSetTableProperty(PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Points: math.NaN()}); err == nil {
		t.Fatal("NaN accepted")
	}
	if candidate, _ := workspace.Candidate(created.Candidate.Handle); candidate.Head != created.Revision.Handle {
		t.Fatal("invalid mutation advanced head")
	}
}

func TestPaperSetTablePropertyIdempotentRace(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, tableMutationFixture, "@name-track", "table-race", CapabilityEdit)
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", tableMutationFixture, "@ledger")}
	request := PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Points: 72}
	var wait sync.WaitGroup
	errors := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wait.Add(1)
		go func() { defer wait.Done(); _, err := workspace.PaperSetTableProperty(request); errors <- err }()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("race=%v", err)
		}
	}
}

func TestPaperSetTablePropertyAuthorizesProtectedComponentBlastRadius(t *testing.T) {
	workspace := authorizationWorkspace(t, WorkspaceOptions{ProtectedNodeIDs: []string{"@table-card"}})
	guard, _, opened := mutationGuard(t, workspace, componentTableMutationFixture, "@template-track", "table-protected", CapabilityEdit)
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", componentTableMutationFixture, "@template-table")}
	request := PaperSetTablePropertyRequest{Guard: guard, Property: PaperTableTrackWidth, Points: 72}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:table", []MutationOperation{MutationSetTableProperty}, []string{"@table-card", "@instance"}, nil)
	if _, err := workspace.PaperSetTableProperty(request); errorCode(err) != "PROTECTED_NODE_DENIED" {
		t.Fatalf("missing protected grant=%v", err)
	}
	request.Guard.Authority = grantMutationAuthority(t, workspace, opened, "agent:table", []MutationOperation{MutationSetTableProperty}, []string{"@table-card", "@instance"}, []string{"@table-card"})
	result, err := workspace.PaperSetTableProperty(request)
	if err != nil || !result.Authorization.Allowed || len(result.Authorization.Effects) < 2 {
		t.Fatalf("authorized=%#v %v", result.Authorization, err)
	}
}
