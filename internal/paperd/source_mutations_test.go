// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
)

const richMutationFixture = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      paragraph @rich:\n" +
	"        text @first: \"Hello\"\n" +
	"        text @second: \" world\"\n"

const bindingMutationFixture = "document @report:\n" +
	"  schema @invoice:\n" +
	"    field @total:\n" +
	"      type: \"number\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      paragraph @amount:\n" +
	"        text: \"Amount\"\n"

func mutationGuard(t *testing.T, workspace *Workspace, source, target, key string, mode CapabilityMode) (PaperMutationGuard, PaperCreateResult, PaperOpenSnapshot) {
	t.Helper()
	created, err := workspace.PaperCreate(PaperCreateRequest{File: "mutation.paper", Source: source})
	if err != nil {
		t.Fatalf("PaperCreate() error = %v", err)
	}
	opened, err := workspace.PaperOpen(PaperOpenRequest{
		Candidate: created.Candidate.Handle, Revision: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Mode: mode,
	})
	if err != nil {
		t.Fatalf("PaperOpen() error = %v", err)
	}
	fingerprint, err := paperedit.FingerprintNode("mutation.paper", source, target)
	if err != nil {
		t.Fatalf("FingerprintNode() error = %v", err)
	}
	instance, err := paperedit.SourceInstance("mutation.paper", source, target)
	if err != nil {
		t.Fatalf("SourceInstance() error = %v", err)
	}
	return PaperMutationGuard{
		Open: opened.Handle, Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
		ExpectedDigest: created.Revision.Revision, Target: target, ExpectedFingerprint: fingerprint, ExpectedInstance: instance, IdempotencyKey: key,
	}, created, opened
}

func exactTargetPrecondition(t *testing.T, file, source, target string) paperedit.TargetPrecondition {
	t.Helper()
	fingerprint, err := paperedit.FingerprintNode(file, source, target)
	if err != nil {
		t.Fatalf("FingerprintNode(%s) error = %v", target, err)
	}
	instance, err := paperedit.SourceInstance(file, source, target)
	if err != nil {
		t.Fatalf("SourceInstance(%s) error = %v", target, err)
	}
	return paperedit.TargetPrecondition{
		Target: target, ExpectedFingerprint: fingerprint, ExpectedInstance: instance,
	}
}

func TestPaperSetLiteralUsesMinimalPatchAndSemanticEvidence(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, workspaceFixture, "@intro", "literal-1", CapabilityEdit)
	result, err := workspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: guard, Text: "Edited literal"})
	if err != nil {
		t.Fatalf("PaperSetLiteral() error = %v", err)
	}
	if !result.Edit.Applied || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 {
		t.Fatalf("PaperSetLiteral() edit = %#v", result.Edit)
	}
	patch := result.Edit.Diff.Patches[0]
	if patch.Target != "@copy" || patch.Removed != `"Hello agent"` || patch.Replacement != `"Edited literal"` {
		t.Fatalf("literal patch = %#v", patch)
	}
	if result.Semantic.Domain != "source" || result.Semantic.Operation != "set_literal" || !result.Semantic.WholeDocument || !result.Semantic.AfterCompileOK {
		t.Fatalf("literal semantic diff = %#v", result.Semantic)
	}
	if !strings.Contains(result.Revision.Source, `text @copy: "Edited literal"`) || result.Candidate.Head != result.Revision.Handle {
		t.Fatalf("literal revision = %#v", result.Revision)
	}
}

func TestPaperSetRichTextCanonicalizesRunsToSourceOrder(t *testing.T) {
	apply := func(t *testing.T, runs []PaperRichTextRun) PaperMutationResult {
		t.Helper()
		workspace := mustWorkspace(t, Limits{})
		guard, _, _ := mutationGuard(t, workspace, richMutationFixture, "@rich", "rich-order", CapabilityEdit)
		result, err := workspace.PaperSetRichText(PaperSetRichTextRequest{Guard: guard, Runs: runs})
		if err != nil {
			t.Fatalf("PaperSetRichText() error = %v", err)
		}
		return result
	}
	forward := apply(t, []PaperRichTextRun{{Target: "@first", Text: "Goodbye"}, {Target: "@second", Text: " moon"}})
	reverse := apply(t, []PaperRichTextRun{{Target: "@second", Text: " moon"}, {Target: "@first", Text: "Goodbye"}})
	if forward.Revision.Source != reverse.Revision.Source || !reflect.DeepEqual(forward.Edit.Diff, reverse.Edit.Diff) || !reflect.DeepEqual(forward.Semantic, reverse.Semantic) {
		t.Fatalf("rich text depends on request order:\n%#v\n%#v", forward, reverse)
	}
	if len(forward.Edit.Diff.Patches) != 2 || forward.Edit.Diff.Patches[0].Target != "@first" || forward.Edit.Diff.Patches[1].Target != "@second" {
		t.Fatalf("rich patches = %#v", forward.Edit.Diff.Patches)
	}
}

func TestPaperSetBindingCompilesBeforePublishing(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, created, _ := mutationGuard(t, workspace, bindingMutationFixture, "@amount", "binding-1", CapabilityEdit)
	minFraction, maxFraction, required := uint32(2), uint32(2), true
	result, err := workspace.PaperSetBinding(PaperSetBindingRequest{
		Guard: guard, Path: "@invoice.total", Required: &required,
		Format: "decimal", FormatLocale: "pt-BR", MinFractionDigits: &minFraction, MaxFractionDigits: &maxFraction,
	})
	if err != nil {
		t.Fatalf("PaperSetBinding() error = %v", err)
	}
	if !result.Revision.CompileOK || !strings.Contains(result.Revision.Source, `bind: "@invoice.total"`) || !strings.Contains(result.Revision.Source, `format: "decimal"`) || !strings.Contains(result.Revision.Source, `format-locale: "pt-BR"`) || !strings.Contains(result.Revision.Source, "format-min-fraction: 2") || result.Edit.Diff == nil {
		t.Fatalf("PaperSetBinding() = %#v", result)
	}
	if result.Edit.Diff.Patches[0].Start != result.Edit.Diff.Patches[0].End {
		t.Fatalf("new binding should be one insertion patch: %#v", result.Edit.Diff.Patches)
	}

	secondWorkspace := mustWorkspace(t, Limits{})
	invalidGuard, invalidCreated, _ := mutationGuard(t, secondWorkspace, bindingMutationFixture, "@amount", "binding-invalid", CapabilityEdit)
	if _, err := secondWorkspace.PaperSetBinding(PaperSetBindingRequest{Guard: invalidGuard, Path: "@invoice.missing"}); !errors.Is(err, ErrInvalidSource) {
		t.Fatalf("PaperSetBinding(invalid path) error = %v", err)
	}
	candidate, _ := secondWorkspace.Candidate(invalidCreated.Candidate.Handle)
	if candidate.Head != invalidCreated.Revision.Handle {
		t.Fatal("invalid binding published a candidate revision")
	}
	_ = created
}

func TestPaperSourceMutationsEnforceModeRevocationAndAmbiguity(t *testing.T) {
	readWorkspace := mustWorkspace(t, Limits{})
	readGuard, _, _ := mutationGuard(t, readWorkspace, workspaceFixture, "@intro", "read-denied", CapabilityRead)
	if _, err := readWorkspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: readGuard, Text: "denied"}); err == nil {
		t.Fatal("PaperSetLiteral(read capability) succeeded")
	} else if typed, ok := err.(*Error); !ok || typed.Code != "CAPABILITY_DENIED" {
		t.Fatalf("PaperSetLiteral(read capability) error = %v", err)
	}

	revokedWorkspace := mustWorkspace(t, Limits{})
	revokedGuard, _, opened := mutationGuard(t, revokedWorkspace, bindingMutationFixture, "@amount", "revoked", CapabilityEdit)
	if err := revokedWorkspace.ClosePaperOpen(opened.Handle); err != nil {
		t.Fatal(err)
	}
	if _, err := revokedWorkspace.PaperSetBinding(PaperSetBindingRequest{Guard: revokedGuard, Path: "@invoice.total"}); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("PaperSetBinding(revoked) error = %v", err)
	}

	ambiguousWorkspace := mustWorkspace(t, Limits{})
	ambiguousGuard, _, _ := mutationGuard(t, ambiguousWorkspace, richMutationFixture, "@rich", "ambiguous", CapabilityEdit)
	if _, err := ambiguousWorkspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: ambiguousGuard, Text: "which run?"}); err == nil {
		t.Fatal("PaperSetLiteral(ambiguous) succeeded")
	} else if typed, ok := err.(*Error); !ok || typed.Code != "AMBIGUOUS_TARGET" {
		t.Fatalf("PaperSetLiteral(ambiguous) error = %v", err)
	}
	if _, err := ambiguousWorkspace.PaperSetRichText(PaperSetRichTextRequest{
		Guard: ambiguousGuard, Runs: []PaperRichTextRun{{Target: "@outside", Text: "x"}},
	}); err == nil {
		t.Fatal("PaperSetRichText(non-child) succeeded")
	} else if typed, ok := err.(*Error); !ok || typed.Code != "AMBIGUOUS_TARGET" {
		t.Fatalf("PaperSetRichText(non-child) error = %v", err)
	}
}

func TestPaperSourceMutationIdempotencyAndStaleHead(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, created, _ := mutationGuard(t, workspace, workspaceFixture, "@intro", "literal-replay", CapabilityEdit)
	request := PaperSetLiteralRequest{Guard: guard, Text: "Replay me"}
	first, err := workspace.PaperSetLiteral(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspace.PaperSetLiteral(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("idempotent replay =\n%#v\n%#v\n%v", first, second, err)
	}
	conflicting := request
	conflicting.Text = "Different payload"
	if _, err := workspace.PaperSetLiteral(conflicting); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("idempotency payload conflict error = %v", err)
	}
	stale := request
	stale.Guard.IdempotencyKey = "new-operation-on-stale-head"
	if _, err := workspace.PaperSetLiteral(stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale head error = %v", err)
	}
	candidate, _ := workspace.Candidate(created.Candidate.Handle)
	if candidate.Head != first.Revision.Handle {
		t.Fatalf("candidate head changed after replays = %#v", candidate)
	}
}

func TestPaperSourceMutationPayloadLimitsAreFailureAtomic(t *testing.T) {
	workspace := mustWorkspace(t, Limits{MaxSourceBytes: len(workspaceFixture) + 16})
	guard, created, _ := mutationGuard(t, workspace, workspaceFixture, "@intro", "oversized", CapabilityEdit)
	if _, err := workspace.PaperSetLiteral(PaperSetLiteralRequest{Guard: guard, Text: strings.Repeat("x", len(workspaceFixture)+17)}); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperSetLiteral(oversized) error = %v", err)
	}
	candidate, _ := workspace.Candidate(created.Candidate.Handle)
	if candidate.Head != created.Revision.Handle {
		t.Fatal("oversized mutation advanced candidate")
	}
}
