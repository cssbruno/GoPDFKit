// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestFingerprintNodeCoversExactSubtreeButNotSibling(t *testing.T) {
	source := editableFixture()
	intro, err := FingerprintNode("invoice.paper", source, "@intro")
	if err != nil {
		t.Fatalf("FingerprintNode(intro) = %v", err)
	}
	repeated, err := FingerprintNode("invoice.paper", source, "@intro")
	if err != nil || repeated != intro {
		t.Fatalf("repeated fingerprint = %q, %v; want %q", repeated, err, intro)
	}
	if len(intro) != 64 {
		t.Fatalf("fingerprint = %q, want lowercase SHA-256", intro)
	}

	changedChild := strings.Replace(source, `text @copy: "Hello"`, `text @copy: "Changed"`, 1)
	childFingerprint, err := FingerprintNode("invoice.paper", changedChild, "@intro")
	if err != nil || childFingerprint == intro {
		t.Fatalf("child change fingerprint = %q, %v; want change from %q", childFingerprint, err, intro)
	}
	changedSibling := strings.Replace(source, `text @spare-text: "Delete me"`, `text @spare-text: "Changed"`, 1)
	siblingFingerprint, err := FingerprintNode("invoice.paper", changedSibling, "@intro")
	if err != nil || siblingFingerprint != intro {
		t.Fatalf("sibling change fingerprint = %q, %v; want %q", siblingFingerprint, err, intro)
	}
}

func TestApplyEchoesIdempotencyAndReturnsExactDeterministicDiff(t *testing.T) {
	source := editableFixture()
	intro, err := FingerprintNode("invoice.paper", source, "@intro")
	if err != nil {
		t.Fatal(err)
	}
	page, err := FingerprintNode("invoice.paper", source, "@page")
	if err != nil {
		t.Fatal(err)
	}
	transaction := Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		IdempotencyKey: "agent-turn-0042",
		TargetPreconditions: []TargetPrecondition{
			{Target: "@page", ExpectedFingerprint: page},
			{Target: "@intro", ExpectedFingerprint: intro},
		},
		Operations: []Operation{
			ReplaceText{Target: "@copy", Text: "Hello deterministic agent"},
			SetProperty{Target: "@page", Name: "width", Value: UnitValue(210, "mm")},
		},
	}
	first, err := Apply(transaction)
	if err != nil {
		t.Fatalf("Apply() = %v, diagnostics %+v", err, first.Diagnostics)
	}
	second, err := Apply(transaction)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("idempotent replay differs:\n%+v / %v\n%+v", second, err, first)
	}
	firstJSON, err := first.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, _ := second.CanonicalJSON()
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("canonical replay differs:\n%s\n%s", firstJSON, secondJSON)
	}
	if first.SchemaVersion != 2 || first.IdempotencyKey != transaction.IdempotencyKey || first.Diff == nil {
		t.Fatalf("result contract = %+v", first)
	}
	if first.Diff.BeforeRevision != SourceRevision(source) || first.Diff.AfterRevision != first.Revision {
		t.Fatalf("diff revisions = %+v", first.Diff)
	}
	if got := applyExportedPatches(t, source, first.Diff.Patches); got != first.Source {
		t.Fatalf("reconstructed source:\n%s\nwant:\n%s", got, first.Source)
	}
	for index, patch := range first.Diff.Patches {
		if source[patch.Start:patch.End] != patch.Removed {
			t.Fatalf("patch %d removed %q, exact source is %q", index, patch.Removed, source[patch.Start:patch.End])
		}
		if index > 0 && first.Diff.Patches[index-1].Start > patch.Start {
			t.Fatalf("patches are not ordered by original offset: %+v", first.Diff.Patches)
		}
	}
	wantInvalidation := &InvalidationScope{
		WholeDocument: true,
		NodeIDs:       []string{"@body", "@copy", "@doc", "@intro", "@page"},
	}
	if !reflect.DeepEqual(first.Invalidation, wantInvalidation) {
		t.Fatalf("invalidation = %+v, want %+v", first.Invalidation, wantInvalidation)
	}

	reversed := transaction
	reversed.TargetPreconditions = []TargetPrecondition{
		{Target: "@intro", ExpectedFingerprint: intro},
		{Target: "@page", ExpectedFingerprint: page},
	}
	third, err := Apply(reversed)
	if err != nil || !reflect.DeepEqual(first, third) {
		t.Fatalf("precondition order changed result:\n%+v / %v\n%+v", third, err, first)
	}
}

func TestApplyRejectsChangedTargetEvenWithCurrentSourceRevision(t *testing.T) {
	original := editableFixture()
	fingerprint, err := FingerprintNode("invoice.paper", original, "@copy")
	if err != nil {
		t.Fatal(err)
	}
	current := strings.Replace(original, `text @copy: "Hello"`, `text @copy: "Someone else edited"`, 1)
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: current, ExpectedRevision: SourceRevision(current),
		IdempotencyKey: "agent-retry",
		TargetPreconditions: []TargetPrecondition{{
			Target: "@copy", ExpectedFingerprint: fingerprint,
		}},
		Operations: []Operation{ReplaceText{Target: "@copy", Text: "must not apply"}},
	})
	if !errors.Is(err, ErrTargetConflict) {
		t.Fatalf("Apply(target conflict) = %v, want ErrTargetConflict", err)
	}
	if result.Applied || result.Source != current || result.Revision != SourceRevision(current) ||
		result.Diff != nil || result.Invalidation != nil || result.IdempotencyKey != "agent-retry" {
		t.Fatalf("target conflict leaked change metadata: %+v", result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "PAPER_EDIT_TARGET_CONFLICT" ||
		result.Diagnostics[0].Target != "@copy" {
		t.Fatalf("target conflict diagnostics = %+v", result.Diagnostics)
	}
}

func TestApplyRejectsMalformedAndDuplicateTargetPreconditionsDeterministically(t *testing.T) {
	source := editableFixture()
	fingerprint, err := FingerprintNode("invoice.paper", source, "@copy")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name          string
		preconditions []TargetPrecondition
		wantCode      string
	}{
		{
			name: "malformed fingerprint",
			preconditions: []TargetPrecondition{{
				Target: "@copy", ExpectedFingerprint: "ABC",
			}},
			wantCode: "PAPER_EDIT_INVALID_FINGERPRINT",
		},
		{
			name: "duplicate target",
			preconditions: []TargetPrecondition{
				{Target: "@copy", ExpectedFingerprint: fingerprint},
				{Target: "@copy", ExpectedFingerprint: fingerprint},
			},
			wantCode: "PAPER_EDIT_DUPLICATE_PRECONDITION",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, applyErr := Apply(Transaction{
				File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
				TargetPreconditions: test.preconditions,
				Operations:          []Operation{ReplaceText{Target: "@copy", Text: "new"}},
			})
			if !errors.Is(applyErr, ErrInvalidOperation) || result.Applied || result.Source != source {
				t.Fatalf("Apply() = %+v, %v", result, applyErr)
			}
			if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != test.wantCode {
				t.Fatalf("diagnostics = %+v, want %s", result.Diagnostics, test.wantCode)
			}
		})
	}
}

func TestApplyRejectsOversizedIdempotencyKeyAtomically(t *testing.T) {
	source := editableFixture()
	result, err := Apply(Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source),
		IdempotencyKey: strings.Repeat("x", MaxIdempotencyKeyBytes+1),
		Operations:     []Operation{ReplaceText{Target: "@copy", Text: "new"}},
	})
	if !errors.Is(err, ErrLimit) || result.Applied || result.Source != source || result.Diff != nil {
		t.Fatalf("oversized idempotency result = %+v, %v", result, err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "PAPER_EDIT_IDEMPOTENCY_KEY_LIMIT" {
		t.Fatalf("diagnostics = %+v", result.Diagnostics)
	}
}

func applyExportedPatches(t *testing.T, source string, patches []SourcePatch) string {
	t.Helper()
	result := source
	for index := len(patches) - 1; index >= 0; index-- {
		patch := patches[index]
		if int(patch.End) > len(result) || patch.Start > patch.End {
			t.Fatalf("patch %d outside source: %+v", index, patch)
		}
		result = result[:patch.Start] + patch.Replacement + result[patch.End:]
	}
	return result
}
