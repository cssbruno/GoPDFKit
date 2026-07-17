// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperedit

import (
	"errors"
	"reflect"
	"testing"
)

func exactPrecondition(t *testing.T, file, source, target string) TargetPrecondition {
	t.Helper()
	fingerprint, err := FingerprintNode(file, source, target)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := SourceInstance(file, source, target)
	if err != nil {
		t.Fatal(err)
	}
	return TargetPrecondition{Target: target, ExpectedFingerprint: fingerprint, ExpectedInstance: instance}
}

func TestApplyExactSourceInstancePreconditionsAreAtomicAndDeterministic(t *testing.T) {
	source := editableFixture()
	precondition := exactPrecondition(t, "invoice.paper", source, "@intro")
	transaction := Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source), RequireExactTargets: true,
		IdempotencyKey: "exact-instance", TargetPreconditions: []TargetPrecondition{precondition},
		Operations: []Operation{ReplaceText{Target: "@copy", Text: "exact"}},
	}
	first, err := Apply(transaction)
	if err != nil || !first.Applied {
		t.Fatalf("Apply(exact ancestor) = %+v, %v", first, err)
	}
	second, err := Apply(transaction)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("exact replay changed = %+v, %v; want %+v", second, err, first)
	}

	missing := transaction
	missing.TargetPreconditions = append([]TargetPrecondition(nil), transaction.TargetPreconditions...)
	missing.TargetPreconditions[0].ExpectedInstance = ""
	result, err := Apply(missing)
	assertAtomicInstanceFailure(t, source, result, err, ErrInvalidOperation, "PAPER_EDIT_INSTANCE_PRECONDITION_REQUIRED")

	wrong := transaction
	wrong.TargetPreconditions = append([]TargetPrecondition(nil), transaction.TargetPreconditions...)
	wrong.TargetPreconditions[0].ExpectedInstance = "/@wrong/@intro"
	result, err = Apply(wrong)
	assertAtomicInstanceFailure(t, source, result, err, ErrTargetConflict, "PAPER_EDIT_INSTANCE_CONFLICT")
	if len(result.Diagnostics[0].Candidates) != 1 || result.Diagnostics[0].Candidates[0].Instance != precondition.ExpectedInstance {
		t.Fatalf("instance candidates = %+v", result.Diagnostics[0].Candidates)
	}
}

func TestApplyMoveRequiresExactSourceAndDestinationInstances(t *testing.T) {
	source := editableFixture()
	copyGuard := exactPrecondition(t, "invoice.paper", source, "@copy")
	transaction := Transaction{
		File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source), RequireExactTargets: true,
		TargetPreconditions: []TargetPrecondition{copyGuard},
		Operations:          []Operation{MoveNode{Target: "@copy", NewParent: "@body"}},
	}
	result, err := Apply(transaction)
	assertAtomicInstanceFailure(t, source, result, err, ErrInvalidOperation, "PAPER_EDIT_PRECONDITION_REQUIRED")
	if got := result.Diagnostics[0].Target; got != "@body" {
		t.Fatalf("unguarded destination target = %q", got)
	}

	transaction.TargetPreconditions = append(transaction.TargetPreconditions, exactPrecondition(t, "invoice.paper", source, "@body"))
	result, err = Apply(transaction)
	if err != nil || !result.Applied {
		t.Fatalf("Apply(exact move) = %+v, %v", result, err)
	}
}

func TestApplyNeverSelectsOrRebasesAcrossInvalidInstances(t *testing.T) {
	source := editableFixture()
	valid := exactPrecondition(t, "invoice.paper", source, "@copy")
	for _, instance := range []string{"/@copy", valid.ExpectedInstance + "/@copy", "/@doc/@page/@body/@spare/@copy"} {
		transaction := Transaction{
			File: "invoice.paper", Source: source, ExpectedRevision: SourceRevision(source), RequireExactTargets: true,
			TargetPreconditions: []TargetPrecondition{{Target: valid.Target, ExpectedFingerprint: valid.ExpectedFingerprint, ExpectedInstance: instance}},
			Operations:          []Operation{ReplaceText{Target: valid.Target, Text: "must not apply"}},
		}
		first, firstErr := Apply(transaction)
		second, secondErr := Apply(transaction)
		assertAtomicInstanceFailure(t, source, first, firstErr, ErrTargetConflict, "PAPER_EDIT_INSTANCE_CONFLICT")
		if !errors.Is(secondErr, ErrTargetConflict) || !reflect.DeepEqual(first, second) || len(first.Diagnostics[0].Candidates) > MaxDiagnosticCandidates {
			t.Fatalf("invalid instance %q was unstable or unbounded: first=%+v/%v second=%+v/%v", instance, first, firstErr, second, secondErr)
		}
	}
}

func assertAtomicInstanceFailure(t *testing.T, source string, result Result, err, wantErr error, wantCode string) {
	t.Helper()
	if !errors.Is(err, wantErr) || result.Applied || result.Source != source || result.Diff != nil || result.Invalidation != nil {
		t.Fatalf("atomic failure = %+v, %v; want %v", result, err, wantErr)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != wantCode {
		t.Fatalf("diagnostics = %+v; want %s", result.Diagnostics, wantCode)
	}
}
