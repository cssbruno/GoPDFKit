// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

const flowMutationFixture = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      paragraph @first:\n" +
	"        text: \"First\"\n" +
	"      row @grid:\n" +
	"        paragraph @inside:\n" +
	"          text: \"Inside\"\n" +
	"      paragraph @second:\n" +
	"        text: \"Second\"\n"

func TestPaperMoveNodeUsesExactDestinationGuardAndPreservesFlow(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, flowMutationFixture, "@second", "move-1", CapabilityEdit)
	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", flowMutationFixture, "@grid")}

	result, err := workspace.PaperMoveNode(PaperMoveNodeRequest{Guard: guard, NewParent: "@grid"})
	if err != nil {
		t.Fatalf("PaperMoveNode() error = %v", err)
	}
	if !result.Edit.Applied || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 {
		t.Fatalf("move edit = %#v", result.Edit)
	}
	if result.Semantic.Operation != "move_node" || len(result.Semantic.Targets) != 2 || !result.Semantic.AfterCompileOK {
		t.Fatalf("move semantic = %#v", result.Semantic)
	}
	if !strings.Contains(result.Revision.Source, "        paragraph @second:") || !strings.Contains(result.Revision.Source, "          text: \"Second\"") {
		t.Fatalf("moved node was not reindented beneath destination:\n%s", result.Revision.Source)
	}
	if strings.Index(result.Revision.Source, "paragraph @second") < strings.Index(result.Revision.Source, "paragraph @inside") {
		t.Fatalf("moved node did not land after existing destination children:\n%s", result.Revision.Source)
	}
}

func TestPaperMoveNodeRejectsMissingDestinationPreconditionAndDescendant(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, created, _ := mutationGuard(t, workspace, flowMutationFixture, "@second", "move-2", CapabilityEdit)
	if _, err := workspace.PaperMoveNode(PaperMoveNodeRequest{Guard: guard, NewParent: "@grid"}); errorCode(err) != "TRANSITIVE_PRECONDITION_REQUIRED" {
		t.Fatalf("missing destination precondition = %v", err)
	}
	if candidate, _ := workspace.Candidate(created.Candidate.Handle); candidate.Head != created.Revision.Handle {
		t.Fatal("rejected move advanced candidate")
	}

	guard.TargetPreconditions = []paperedit.TargetPrecondition{exactTargetPrecondition(t, "mutation.paper", flowMutationFixture, "@grid")}
	guard.Target = "@grid"
	guard.ExpectedFingerprint, _ = paperedit.FingerprintNode("mutation.paper", flowMutationFixture, "@grid")
	guard.ExpectedInstance, _ = paperedit.SourceInstance("mutation.paper", flowMutationFixture, "@grid")
	if _, err := workspace.PaperMoveNode(PaperMoveNodeRequest{Guard: guard, NewParent: "@inside"}); !errors.Is(err, paperedit.ErrInvalidOperation) {
		t.Fatalf("descendant move error = %v", err)
	}
}
