// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func approvedExecution(t *testing.T, operation SensitiveOperation, input SensitiveOperationInput) (*Workspace, SensitiveOperationRequest) {
	t.Helper()
	workspace, created, opened := sensitiveFixture(t, WorkspaceOptions{PolicyRevision: "policy-v7"})
	authority := grantSensitiveForTest(t, workspace, opened, operation)
	evidence := completeSensitiveEvidence(string(created.Revision.Revision))
	hash, err := SensitiveOperationInputHash(operation, input)
	if err != nil {
		t.Fatal(err)
	}
	evidence.OperationInputHash = hash
	approval := grantApprovalForTest(t, workspace, created, authority, evidence, "execution-approval-"+string(operation), time.Minute)
	return workspace, SensitiveOperationRequest{Authority: authority.Handle, Approval: approval.Handle, Operation: operation, ExpectedHead: created.Revision.Handle, PolicyRevision: "policy-v7", Evidence: evidence}
}

func TestSensitiveExecutionEntryPointsBindOperationAndInput(t *testing.T) {
	input := SensitiveOperationInput{Target: "artifact:release", MediaType: "application/pdf", Payload: []byte("%PDF-approved")}
	operations := []SensitiveOperation{SensitiveExport, SensitivePublish, SensitiveAttachment, SensitiveProductionCapture, SensitiveSign}
	for _, operation := range operations {
		t.Run(string(operation), func(t *testing.T) {
			workspace, authorization := approvedExecution(t, operation, input)
			var calls atomic.Int32
			executor := func(_ context.Context, got SensitiveOperationInput) (SensitiveExecutionOutcome, error) {
				calls.Add(1)
				got.Payload[0] = 'X'
				return SensitiveExecutionOutcome{ExternalID: "delivery:1", ResultHash: evidenceHashForTest("delivered"), Bytes: int64(len(got.Payload))}, nil
			}
			var result SensitiveExecutionResult
			var err error
			switch operation {
			case SensitiveExport:
				result, err = workspace.ExecuteExport(context.Background(), authorization, input, executor)
			case SensitivePublish:
				result, err = workspace.ExecutePublish(context.Background(), authorization, input, executor)
			case SensitiveAttachment:
				result, err = workspace.ExecuteAttachment(context.Background(), authorization, input, executor)
			case SensitiveProductionCapture:
				result, err = workspace.ExecuteProductionCapture(context.Background(), authorization, input, executor)
			case SensitiveSign:
				result, err = workspace.ExecuteSign(context.Background(), authorization, input, executor)
			}
			if err != nil || !result.Executed || result.Authorization.Operation != operation || result.ExecutionAuditHash == "" || calls.Load() != 1 {
				t.Fatalf("execute %s = %#v, %v, calls=%d", operation, result, err, calls.Load())
			}
			if string(input.Payload) != "%PDF-approved" {
				t.Fatalf("executor mutated caller input: %q", input.Payload)
			}
		})
	}
}

func TestSensitiveExecutionRejectsBeforeSideEffectAndPreservesApproval(t *testing.T) {
	input := SensitiveOperationInput{Target: "artifact:release", MediaType: "application/pdf", Payload: []byte("approved")}
	workspace, authorization := approvedExecution(t, SensitiveExport, input)
	var calls atomic.Int32
	executor := func(context.Context, SensitiveOperationInput) (SensitiveExecutionOutcome, error) {
		calls.Add(1)
		return SensitiveExecutionOutcome{ResultHash: evidenceHashForTest("ok"), Bytes: 8}, nil
	}
	changed := input
	changed.Payload = []byte("different")
	if _, err := workspace.ExecuteExport(context.Background(), authorization, changed, executor); errorCode(err) != "SENSITIVE_INPUT_BINDING_MISMATCH" {
		t.Fatalf("changed input error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("executor called before binding passed: %d", calls.Load())
	}
	audit, err := workspace.SensitiveOperationAudit(8)
	if err != nil || len(audit) != 1 || audit[0].Reason != "SENSITIVE_INPUT_BINDING_MISMATCH" || audit[0].Allowed {
		t.Fatalf("binding denial audit = %#v, %v", audit, err)
	}
	if _, err := workspace.ExecutePublish(context.Background(), authorization, input, executor); errorCode(err) != "SENSITIVE_CAPABILITY_DENIED" {
		t.Fatalf("wrong entrypoint error = %v", err)
	}
	result, err := workspace.ExecuteExport(context.Background(), authorization, input, executor)
	if err != nil || !result.Executed || calls.Load() != 1 {
		t.Fatalf("valid retry = %#v, %v, calls=%d", result, err, calls.Load())
	}
}

func TestSensitiveExecutionCancellationAndFailureAreExplicit(t *testing.T) {
	input := SensitiveOperationInput{Target: "signer:key-7", MediaType: "application/pdf", Payload: []byte("approved")}
	workspace, authorization := approvedExecution(t, SensitiveSign, input)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32
	executor := func(context.Context, SensitiveOperationInput) (SensitiveExecutionOutcome, error) {
		calls.Add(1)
		return SensitiveExecutionOutcome{}, errors.New("HSM unavailable")
	}
	if _, err := workspace.ExecuteSign(ctx, authorization, input, executor); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("cancelled execution = %v, calls=%d", err, calls.Load())
	}
	result, err := workspace.ExecuteSign(context.Background(), authorization, input, executor)
	if !errors.Is(err, ErrSensitiveExecutor) || !result.Executed || result.Authorization.AuditHash == "" || result.ExecutionAuditHash == "" || calls.Load() != 1 {
		t.Fatalf("failed executor = %#v, %v, calls=%d", result, err, calls.Load())
	}
	if _, err := workspace.ExecuteSign(context.Background(), authorization, input, executor); !errors.Is(err, ErrApprovalReplay) || calls.Load() != 1 {
		t.Fatalf("failed-operation replay = %v, calls=%d", err, calls.Load())
	}
}
