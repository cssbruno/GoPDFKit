// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

const invalidBindingFixFixture = `document @report:
  schema invoice:
    number subtotal
    number total
  page @sheet:
    body @body:
      paragraph @amount:
        bind: "missing"
        text: "Amount"
`

const nullableBindingFixFixture = `document @report:
  schema invoice:
    optional object customer:
      string name
  page @sheet:
    body @body:
      paragraph @customer-name:
        bind: "customer.name"
        text: "Name"
`

const componentReferenceFixFixture = "document @report:\n" +
	"  component @known:\n" +
	"    paragraph:\n" +
	"      text: \"Known\"\n" +
	"  component @alternate:\n" +
	"    paragraph:\n" +
	"      text: \"Alternate\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @instance:\n" +
	"        component: \"@missing\"\n"

const expandedInstanceFixFixture = "document @report:\n" +
	"  component @card:\n" +
	"    paragraph @inside:\n" +
	"      text: \"Inside\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @instance:\n" +
	"        component: \"@card\"\n"

func diagnosticFixRequest(t *testing.T, workspace *Workspace, source, target, key, diagnosticCode string, remedy DiagnosticRemedyCode, payload PaperDiagnosticFixPayload, mode CapabilityMode) (PaperApplyDiagnosticFixRequest, PaperCreateResult, PaperOpenSnapshot) {
	t.Helper()
	guard, created, opened := mutationGuard(t, workspace, source, target, key, mode)
	diagnostic := diagnosticByCode(t, created.Revision.CompileDiagnostics, diagnosticCode)
	return PaperApplyDiagnosticFixRequest{
		Guard: guard, DiagnosticFingerprint: PaperDiagnosticFingerprint(created.Revision.Revision, diagnostic),
		Remedy: remedy, Payload: payload,
	}, created, opened
}

func diagnosticByCode(t *testing.T, diagnostics []paperlang.Diagnostic, code string) paperlang.Diagnostic {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return diagnostic
		}
	}
	t.Fatalf("diagnostic %s not found in %#v", code, diagnostics)
	return paperlang.Diagnostic{}
}

func TestPaperApplyDiagnosticFixAllowlistedRemediesCompileAtomically(t *testing.T) {
	t.Run("binding path", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		request, _, _ := diagnosticFixRequest(t, workspace, invalidBindingFixFixture, "@amount", "fix-binding", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityEdit)
		result, err := workspace.PaperApplyDiagnosticFix(request)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || result.Edit.Diff.Patches[0].Removed != `"missing"` || result.Edit.Diff.Patches[0].Replacement != `"total"` {
			t.Fatalf("binding fix = %#v", result)
		}
		if result.Semantic.Operation != "apply_fix:set_binding_path" || result.Semantic.Domain != "source" {
			t.Fatalf("binding semantic evidence = %#v", result.Semantic)
		}
	})

	t.Run("nullable binding", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		request, _, _ := diagnosticFixRequest(t, workspace, nullableBindingFixFixture, "@customer-name", "fix-nullable", "PAPER_BIND_NULLABLE", RemedyAllowNullableBinding, PaperDiagnosticFixPayload{}, CapabilityEdit)
		result, err := workspace.PaperApplyDiagnosticFix(request)
		if err != nil || !result.Revision.CompileOK || !strings.Contains(result.Revision.Source, "bind-required: false") {
			t.Fatalf("nullable fix = %#v, %v", result, err)
		}
	})

	t.Run("required slot", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		payload := PaperDiagnosticFixPayload{Slot: "@content", Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, ID: "@fixed-copy", Value: valuePointer(paperedit.StringValue("fixed"))}}}
		request, _, _ := diagnosticFixRequest(t, workspace, slotMutationFixture, "@instance", "fix-slot", "PAPER_SLOT_MISSING", RemedyFillRequiredSlot, payload, CapabilityEdit)
		result, err := workspace.PaperApplyDiagnosticFix(request)
		if err != nil || !result.Revision.CompileOK || result.Edit.Diff == nil || len(result.Edit.Diff.Patches) != 1 || !strings.Contains(result.Revision.Source, "fill @content:") {
			t.Fatalf("slot fix = %#v, %v", result, err)
		}
	})

	t.Run("component reference", func(t *testing.T) {
		workspace := mustWorkspace(t, Limits{})
		request, _, _ := diagnosticFixRequest(t, workspace, componentReferenceFixFixture, "@instance", "fix-component", "PAPER_COMPONENT_UNKNOWN", RemedySetComponentReference, PaperDiagnosticFixPayload{Component: "@known"}, CapabilityEdit)
		result, err := workspace.PaperApplyDiagnosticFix(request)
		if err != nil || !result.Revision.CompileOK || !strings.Contains(result.Revision.Source, `component: "@known"`) {
			t.Fatalf("component fix = %#v, %v", result, err)
		}
	})
}

func TestPaperApplyDiagnosticFixIsDeterministicAndIdempotent(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	request, created, _ := diagnosticFixRequest(t, workspace, invalidBindingFixFixture, "@amount", "fix-replay", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityEdit)
	first, err := workspace.PaperApplyDiagnosticFix(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspace.PaperApplyDiagnosticFix(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("fix replay =\n%#v\n%#v\n%v", first, second, err)
	}
	changed := request
	changed.Payload.Path = "subtotal"
	if _, err := workspace.PaperApplyDiagnosticFix(changed); !errors.Is(err, ErrRevisionConflict) || errorCode(err) != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("fix idempotency conflict = %v", err)
	}
	stale := request
	stale.Guard.IdempotencyKey = "new-fix-on-stale-head"
	if _, err := workspace.PaperApplyDiagnosticFix(stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale fix error = %v", err)
	}
	candidate, _ := workspace.Candidate(created.Candidate.Handle)
	if candidate.Head != first.Revision.Handle {
		t.Fatalf("candidate after replay/conflicts = %#v", candidate)
	}

	diagnostic := diagnosticByCode(t, created.Revision.CompileDiagnostics, "PAPER_BIND_PATH")
	if a, b := PaperDiagnosticFingerprint(created.Revision.Revision, diagnostic), PaperDiagnosticFingerprint(created.Revision.Revision, diagnostic); a != b || !validLowerSHA256(a) {
		t.Fatalf("diagnostic fingerprint is unstable: %q %q", a, b)
	}
}

func TestPaperApplyDiagnosticFixRejectsFuzzyMismatchedAndCrossTargetRequests(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	request, created, _ := diagnosticFixRequest(t, workspace, invalidBindingFixFixture, "@amount", "reject-fuzzy", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityEdit)

	fuzzy := request
	fuzzy.DiagnosticFingerprint = strings.ToUpper(fuzzy.DiagnosticFingerprint)
	if _, err := workspace.PaperApplyDiagnosticFix(fuzzy); err == nil || errorCode(err) != "INVALID_DIAGNOSTIC_FINGERPRINT" {
		t.Fatalf("uppercase fingerprint error = %v", err)
	}
	missing := request
	missing.DiagnosticFingerprint = strings.Repeat("0", 64)
	if _, err := workspace.PaperApplyDiagnosticFix(missing); !errors.Is(err, ErrRevisionConflict) || errorCode(err) != "DIAGNOSTIC_CONFLICT" {
		t.Fatalf("absent fingerprint error = %v", err)
	}
	mismatch := request
	mismatch.Remedy = RemedyFillRequiredSlot
	mismatch.Payload = PaperDiagnosticFixPayload{Slot: "@x", Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("x"))}}}
	if _, err := workspace.PaperApplyDiagnosticFix(mismatch); err == nil || errorCode(err) != "REMEDY_MISMATCH" {
		t.Fatalf("remedy mismatch error = %v", err)
	}
	unknown := request
	unknown.Remedy = "arbitrary_patch"
	if _, err := workspace.PaperApplyDiagnosticFix(unknown); err == nil || errorCode(err) != "REMEDY_NOT_ALLOWED" {
		t.Fatalf("unknown remedy error = %v", err)
	}
	extraPayload := request
	extraPayload.Payload.Component = "@anything"
	if _, err := workspace.PaperApplyDiagnosticFix(extraPayload); err == nil || errorCode(err) != "INVALID_REMEDY_PAYLOAD" {
		t.Fatalf("extra payload error = %v", err)
	}

	otherFingerprint, err := paperedit.FingerprintNode("mutation.paper", invalidBindingFixFixture, "@body")
	if err != nil {
		t.Fatal(err)
	}
	crossTarget := request
	crossTarget.Guard.Target = "@body"
	crossTarget.Guard.ExpectedFingerprint = otherFingerprint
	crossTarget.Guard.ExpectedInstance, err = paperedit.SourceInstance("mutation.paper", invalidBindingFixFixture, "@body")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.PaperApplyDiagnosticFix(crossTarget); !errors.Is(err, ErrRevisionConflict) || errorCode(err) != "DIAGNOSTIC_TARGET_CONFLICT" {
		t.Fatalf("cross-target diagnostic error = %v", err)
	}
	candidate, _ := workspace.Candidate(created.Candidate.Handle)
	if candidate.Head != created.Revision.Handle {
		t.Fatal("rejected diagnostic request advanced candidate")
	}
}

func TestPaperApplyDiagnosticFixRejectsInstanceAndAmbiguousDiagnostics(t *testing.T) {
	instanceWorkspace := mustWorkspace(t, Limits{})
	created, err := instanceWorkspace.PaperCreate(PaperCreateRequest{File: "mutation.paper", Source: expandedInstanceFixFixture})
	if err != nil {
		t.Fatal(err)
	}
	opened, _ := instanceWorkspace.PaperOpen(PaperOpenRequest{Candidate: created.Candidate.Handle, Revision: created.Revision.Handle, ExpectedDigest: created.Revision.Revision, Mode: CapabilityEdit})
	compiled, err := instanceWorkspace.Compile(created.Revision.Handle)
	if err != nil {
		t.Fatal(err)
	}
	instanceID := ""
	for _, mapping := range compiled.Mappings {
		if mapping.InstancePath != "" && findNodeByID(instanceWorkspace.revisions[created.Revision.Handle.value.serial].parsed.AST.Root, mapping.ID) == nil {
			instanceID = mapping.ID
			break
		}
	}
	if instanceID == "" {
		t.Fatal("fixture produced no expanded instance mapping")
	}
	instanceRequest := PaperApplyDiagnosticFixRequest{
		Guard: PaperMutationGuard{
			Open: opened.Handle, Candidate: created.Candidate.Handle, ExpectedHead: created.Revision.Handle,
			ExpectedDigest: created.Revision.Revision, Target: instanceID, ExpectedFingerprint: paperedit.NodeFingerprint(strings.Repeat("0", 64)), IdempotencyKey: "instance-fix",
		},
		DiagnosticFingerprint: strings.Repeat("0", 64), Remedy: RemedyAllowNullableBinding,
	}
	if _, err := instanceWorkspace.PaperApplyDiagnosticFix(instanceRequest); err == nil || errorCode(err) != "INSTANCE_TARGET" {
		t.Fatalf("instance target error = %v", err)
	}

	ambiguousWorkspace := mustWorkspace(t, Limits{})
	request, createdAmbiguous, _ := diagnosticFixRequest(t, ambiguousWorkspace, invalidBindingFixFixture, "@amount", "ambiguous-diagnostic", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityEdit)
	ambiguousWorkspace.mu.Lock()
	record := ambiguousWorkspace.revisions[createdAmbiguous.Revision.Handle.value.serial]
	diagnostic := diagnosticByCode(t, record.compiled.Diagnostics, "PAPER_BIND_PATH")
	record.compiled.Diagnostics = append(record.compiled.Diagnostics, diagnostic)
	ambiguousWorkspace.mu.Unlock()
	if _, err := ambiguousWorkspace.PaperApplyDiagnosticFix(request); err == nil || errorCode(err) != "AMBIGUOUS_DIAGNOSTIC" {
		t.Fatalf("ambiguous diagnostic error = %v", err)
	}
}

func TestPaperApplyDiagnosticFixEnforcesCapabilityRevocationAndBounds(t *testing.T) {
	readWorkspace := mustWorkspace(t, Limits{})
	readRequest, _, _ := diagnosticFixRequest(t, readWorkspace, invalidBindingFixFixture, "@amount", "read-fix", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityRead)
	if _, err := readWorkspace.PaperApplyDiagnosticFix(readRequest); err == nil || errorCode(err) != "CAPABILITY_DENIED" {
		t.Fatalf("read fix error = %v", err)
	}

	revokedWorkspace := mustWorkspace(t, Limits{})
	revokedRequest, _, opened := diagnosticFixRequest(t, revokedWorkspace, invalidBindingFixFixture, "@amount", "revoked-fix", "PAPER_BIND_PATH", RemedySetBindingPath, PaperDiagnosticFixPayload{Path: "total"}, CapabilityEdit)
	if err := revokedWorkspace.ClosePaperOpen(opened.Handle); err != nil {
		t.Fatal(err)
	}
	if _, err := revokedWorkspace.PaperApplyDiagnosticFix(revokedRequest); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("revoked fix error = %v", err)
	}

	limitedWorkspace := mustWorkspace(t, Limits{MaxSourceBytes: len(slotMutationFixture) + 256})
	limitedRequest, limitedCreated, _ := diagnosticFixRequest(t, limitedWorkspace, slotMutationFixture, "@instance", "bounded-fix", "PAPER_SLOT_MISSING", RemedyFillRequiredSlot, PaperDiagnosticFixPayload{
		Slot: "@content", Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue(strings.Repeat("x", len(slotMutationFixture)+257)))}},
	}, CapabilityEdit)
	if _, err := limitedWorkspace.PaperApplyDiagnosticFix(limitedRequest); !errors.Is(err, ErrLimit) {
		t.Fatalf("bounded fix error = %v", err)
	}
	candidate, _ := limitedWorkspace.Candidate(limitedCreated.Candidate.Handle)
	if candidate.Head != limitedCreated.Revision.Handle {
		t.Fatal("oversized fix advanced candidate")
	}
}
