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

const slotMutationFixture = "document @report:\n" +
	"  component @card:\n" +
	"    slot @content:\n" +
	"      type: \"text\"\n" +
	"      required: true\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @instance:\n" +
	"        component: \"@card\"\n"

const filledSlotMutationFixture = "document @report:\n" +
	"  component @card:\n" +
	"    slot @content:\n" +
	"      type: \"text\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @instance:\n" +
	"        component: \"@card\"\n" +
	"        fill @content:\n" +
	"          text: \"existing\"\n"

const ambiguousComponentMutationFixture = "document @report:\n" +
	"  component @card:\n" +
	"    slot @content:\n" +
	"      type: \"text\"\n" +
	"  page @sheet:\n" +
	"    body @body:\n" +
	"      use @instance:\n" +
	"        component: \"@card\"\n" +
	"        component: \"@card\"\n"

func TestPaperSetScenarioValueUsesOnlyScenarioDomainAndStableKeys(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	_, sourceCreated, opened := mutationGuard(t, workspace, workspaceFixture, "@intro", "unused-source-key", CapabilityEdit)
	base := mustScenarioRevision(t, workspace)
	candidate, err := workspace.NewScenarioCandidate(base.Handle)
	if err != nil {
		t.Fatal(err)
	}
	guard := PaperScenarioMutationGuard{
		Open: opened.Handle, Candidate: candidate.Handle, ExpectedHead: base.Handle,
		ExpectedDigest: base.Digest, IdempotencyKey: "scenario-value-1",
	}
	request := PaperSetScenarioValueRequest{
		Guard: guard, Scenario: "typical", Path: "lines", Key: "line-a", Value: scenarioString("stable replacement"),
	}
	first, err := workspace.PaperSetScenarioValue(request)
	if err != nil {
		t.Fatalf("PaperSetScenarioValue() error = %v", err)
	}
	second, err := workspace.PaperSetScenarioValue(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("scenario idempotent replay =\n%#v\n%#v\n%v", first, second, err)
	}
	if first.Semantic.Domain != "scenario" || first.Semantic.StableKey != "line-a" || first.Semantic.BeforeDigest != base.Digest || first.Semantic.AfterDigest == base.Digest {
		t.Fatalf("scenario semantic diff = %#v", first.Semantic)
	}
	edited := retainedScenarioFixture(t, workspace, first.Revision.Handle)
	lines := scenarioFieldForTest(edited.Values, "lines")
	if len(lines.List) != 2 || lines.List[0].Key != "line-a" || lines.List[0].Value.String != "stable replacement" || lines.List[1].Key != "line-b" {
		t.Fatalf("stable keyed scenario value = %#v", lines.List)
	}
	sourceCandidate, _ := workspace.Candidate(sourceCreated.Candidate.Handle)
	if sourceCandidate.Head != sourceCreated.Revision.Handle {
		t.Fatal("scenario mutation changed the source revision domain")
	}

	conflict := request
	conflict.Value = scenarioString("different")
	if _, err := workspace.PaperSetScenarioValue(conflict); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("scenario idempotency conflict error = %v", err)
	}
}

func TestPaperSetScenarioValueEnforcesModeDomainStalenessAndLimits(t *testing.T) {
	readWorkspace := mustWorkspace(t, Limits{})
	_, _, readOpen := mutationGuard(t, readWorkspace, workspaceFixture, "@intro", "unused", CapabilityRead)
	base := mustScenarioRevision(t, readWorkspace)
	candidate, _ := readWorkspace.NewScenarioCandidate(base.Handle)
	request := PaperSetScenarioValueRequest{
		Guard:    PaperScenarioMutationGuard{Open: readOpen.Handle, Candidate: candidate.Handle, ExpectedHead: base.Handle, ExpectedDigest: base.Digest, IdempotencyKey: "read-denied"},
		Scenario: "typical", Path: "customer.name", Value: scenarioString("denied"),
	}
	if _, err := readWorkspace.PaperSetScenarioValue(request); err == nil || errorCode(err) != "CAPABILITY_DENIED" {
		t.Fatalf("PaperSetScenarioValue(read) error = %v", err)
	}

	workspace := mustWorkspace(t, Limits{MaxScenarioPathBytes: 16})
	_, _, opened := mutationGuard(t, workspace, workspaceFixture, "@intro", "unused", CapabilityEdit)
	scenario := mustScenarioRevision(t, workspace)
	scenarioCandidate, _ := workspace.NewScenarioCandidate(scenario.Handle)
	valid := PaperSetScenarioValueRequest{
		Guard:    PaperScenarioMutationGuard{Open: opened.Handle, Candidate: scenarioCandidate.Handle, ExpectedHead: scenario.Handle, ExpectedDigest: scenario.Digest, IdempotencyKey: "first"},
		Scenario: "typical", Path: "customer.name", Value: scenarioString("first"),
	}
	if _, err := workspace.PaperSetScenarioValue(valid); err != nil {
		t.Fatal(err)
	}
	stale := valid
	stale.Guard.IdempotencyKey = "stale"
	if _, err := workspace.PaperSetScenarioValue(stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperSetScenarioValue(stale) error = %v", err)
	}
	oversized := valid
	oversized.Guard.IdempotencyKey = "oversized"
	oversized.Path = strings.Repeat("x", 17)
	if _, err := workspace.PaperSetScenarioValue(oversized); !errors.Is(err, ErrLimit) && !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("PaperSetScenarioValue(path limit) error = %v", err)
	}

	other := mustWorkspace(t, Limits{})
	foreign := mustScenarioRevision(t, other)
	foreignCandidate, _ := other.NewScenarioCandidate(foreign.Handle)
	crossDomain := valid
	crossDomain.Guard.Candidate = foreignCandidate.Handle
	crossDomain.Guard.ExpectedHead = foreign.Handle
	crossDomain.Guard.ExpectedDigest = foreign.Digest
	crossDomain.Guard.IdempotencyKey = "foreign"
	if _, err := workspace.PaperSetScenarioValue(crossDomain); !errors.Is(err, ErrWrongWorkspace) {
		t.Fatalf("PaperSetScenarioValue(foreign domain) error = %v", err)
	}
}

func TestPaperFillSlotValidatesContractAndPublishesMinimalSourcePatch(t *testing.T) {
	workspace := mustWorkspace(t, Limits{})
	guard, _, _ := mutationGuard(t, workspace, slotMutationFixture, "@instance", "fill-slot-1", CapabilityEdit)
	request := PaperFillSlotRequest{
		Guard: guard, Slot: "@content",
		Content: []paperedit.NodeSpec{{
			Kind: paperlang.NodeParagraph, ID: "@filled-copy",
			Properties: []paperedit.PropertySpec{{Name: "text", Value: paperedit.StringValue("Filled safely")}},
		}},
	}
	first, err := workspace.PaperFillSlot(request)
	if err != nil {
		t.Fatalf("PaperFillSlot() error = %v", err)
	}
	second, err := workspace.PaperFillSlot(request)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("slot idempotent replay =\n%#v\n%#v\n%v", first, second, err)
	}
	if !first.Revision.CompileOK || first.Edit.Diff == nil || len(first.Edit.Diff.Patches) != 1 || first.Edit.Diff.Patches[0].Start != first.Edit.Diff.Patches[0].End {
		t.Fatalf("PaperFillSlot() = %#v", first)
	}
	if !strings.Contains(first.Revision.Source, "fill @content:\n          paragraph @filled-copy:") ||
		!strings.Contains(first.Revision.Source, `text: "Filled safely"`) {
		t.Fatalf("filled source =\n%s", first.Revision.Source)
	}
	if first.Semantic.Domain != "source" || first.Semantic.Operation != "fill_slot" || first.Semantic.BeforeCompileOK || !first.Semantic.AfterCompileOK {
		t.Fatalf("slot semantic evidence = %#v", first.Semantic)
	}

	stale := request
	stale.Guard.IdempotencyKey = "new-fill-on-stale-head"
	if _, err := workspace.PaperFillSlot(stale); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("PaperFillSlot(stale) error = %v", err)
	}
}

func TestPaperFillSlotRejectsTypeCardinalityAmbiguityAndLimits(t *testing.T) {
	typeWorkspace := mustWorkspace(t, Limits{})
	typeGuard, typeCreated, _ := mutationGuard(t, typeWorkspace, slotMutationFixture, "@instance", "wrong-type", CapabilityEdit)
	wrongType := PaperFillSlotRequest{
		Guard: typeGuard, Slot: "@content",
		Content: []paperedit.NodeSpec{{Kind: paperlang.NodeList, Children: []paperedit.NodeSpec{{Kind: paperlang.NodeItem, Children: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("x"))}}}}}},
	}
	if _, err := typeWorkspace.PaperFillSlot(wrongType); err == nil || errorCode(err) != "SLOT_TYPE" {
		t.Fatalf("PaperFillSlot(type) error = %v", err)
	}
	typeCandidate, _ := typeWorkspace.Candidate(typeCreated.Candidate.Handle)
	if typeCandidate.Head != typeCreated.Revision.Handle {
		t.Fatal("wrong slot type advanced candidate")
	}

	filledWorkspace := mustWorkspace(t, Limits{})
	filledGuard, _, _ := mutationGuard(t, filledWorkspace, filledSlotMutationFixture, "@instance", "duplicate-fill", CapabilityEdit)
	duplicate := PaperFillSlotRequest{Guard: filledGuard, Slot: "@content", Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("again"))}}}
	if _, err := filledWorkspace.PaperFillSlot(duplicate); err == nil || errorCode(err) != "SLOT_CARDINALITY" {
		t.Fatalf("PaperFillSlot(cardinality) error = %v", err)
	}

	ambiguous := duplicate
	ambiguous.Guard.IdempotencyKey = "unknown-slot"
	ambiguous.Slot = "@missing"
	if _, err := filledWorkspace.PaperFillSlot(ambiguous); err == nil || errorCode(err) != "INVALID_SLOT_FILL" {
		t.Fatalf("PaperFillSlot(unknown slot) error = %v", err)
	}

	ambiguousWorkspace := mustWorkspace(t, Limits{})
	ambiguousGuard, _, _ := mutationGuard(t, ambiguousWorkspace, ambiguousComponentMutationFixture, "@instance", "ambiguous-component", CapabilityEdit)
	ambiguousReference := PaperFillSlotRequest{Guard: ambiguousGuard, Slot: "@content", Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("x"))}}}
	if _, err := ambiguousWorkspace.PaperFillSlot(ambiguousReference); err == nil || errorCode(err) != "AMBIGUOUS_COMPONENT" {
		t.Fatalf("PaperFillSlot(ambiguous component) error = %v", err)
	}

	limitedWorkspace := mustWorkspace(t, Limits{MaxOperations: 1})
	limitedGuard, _, _ := mutationGuard(t, limitedWorkspace, slotMutationFixture, "@instance", "too-many", CapabilityEdit)
	tooMany := PaperFillSlotRequest{Guard: limitedGuard, Slot: "@content", Content: []paperedit.NodeSpec{
		{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("one"))},
		{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("two"))},
	}}
	if _, err := limitedWorkspace.PaperFillSlot(tooMany); !errors.Is(err, ErrLimit) {
		t.Fatalf("PaperFillSlot(limit) error = %v", err)
	}
}

func TestPaperFillSlotRequiresEditCapabilityAndLiveOpen(t *testing.T) {
	readWorkspace := mustWorkspace(t, Limits{})
	readGuard, _, _ := mutationGuard(t, readWorkspace, slotMutationFixture, "@instance", "read-slot", CapabilityRead)
	request := PaperFillSlotRequest{Guard: readGuard, Slot: "@content", Content: []paperedit.NodeSpec{{Kind: paperlang.NodeText, Value: valuePointer(paperedit.StringValue("x"))}}}
	if _, err := readWorkspace.PaperFillSlot(request); err == nil || errorCode(err) != "CAPABILITY_DENIED" {
		t.Fatalf("PaperFillSlot(read) error = %v", err)
	}

	revokedWorkspace := mustWorkspace(t, Limits{})
	revokedGuard, _, opened := mutationGuard(t, revokedWorkspace, slotMutationFixture, "@instance", "revoked-slot", CapabilityEdit)
	if err := revokedWorkspace.ClosePaperOpen(opened.Handle); err != nil {
		t.Fatal(err)
	}
	request.Guard = revokedGuard
	if _, err := revokedWorkspace.PaperFillSlot(request); !errors.Is(err, ErrRevisionNotFound) {
		t.Fatalf("PaperFillSlot(revoked) error = %v", err)
	}
}

func valuePointer(value paperedit.Value) *paperedit.Value { return &value }
