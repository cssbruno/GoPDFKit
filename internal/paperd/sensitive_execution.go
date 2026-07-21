// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

var ErrSensitiveExecutor = errors.New("paperd: sensitive executor failed")

// SensitiveOperationInput is the complete caller-controlled input to a
// sensitive side effect. ApprovalEvidence binds its canonical hash, while the
// raw target and payload are never retained in workspace or audit state.
type SensitiveOperationInput struct {
	Target    string `json:"target"`
	MediaType string `json:"media_type"`
	Payload   []byte `json:"payload"`
}

// SensitiveExecutionOutcome contains only bounded, non-secret evidence
// returned by an external executor. ResultHash identifies the delivered or
// produced artifact without retaining it in paperd.
type SensitiveExecutionOutcome struct {
	ExternalID string `json:"external_id,omitempty"`
	ResultHash string `json:"result_hash"`
	Bytes      int64  `json:"bytes"`
}

// SensitiveExecutionResult always returns the authorization receipt after an
// approval has been consumed, including when the executor subsequently fails.
// Callers must create a fresh reviewed approval before retrying.
type SensitiveExecutionResult struct {
	Authorization      SensitiveOperationReceipt `json:"authorization"`
	Outcome            SensitiveExecutionOutcome `json:"outcome"`
	Executed           bool                      `json:"executed"`
	ExecutionAuditHash string                    `json:"execution_audit_hash,omitempty"`
}

// SensitiveExecutor is injected by the authenticated transport/application
// boundary. paperd never performs ambient network, filesystem, key, or signing
// operations itself.
type SensitiveExecutor func(context.Context, SensitiveOperationInput) (SensitiveExecutionOutcome, error)

func SensitiveOperationInputHash(operation SensitiveOperation, input SensitiveOperationInput) (string, error) {
	canonical, err := canonicalSensitiveOperationInput(operation, input, MaxRenderBytesHard, MaxQueryBytesHard)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(struct {
		Operation SensitiveOperation      `json:"operation"`
		Input     SensitiveOperationInput `json:"input"`
	}{Operation: operation, Input: canonical})
	sum := sha256.Sum256(append([]byte("paperd/sensitive-operation-input/v1\x00"), payload...))
	return hex.EncodeToString(sum[:]), nil
}

func canonicalSensitiveOperationInput(operation SensitiveOperation, input SensitiveOperationInput, maxPayload, maxLabel int) (SensitiveOperationInput, error) {
	if _, ok := operation.capability(); !ok {
		return SensitiveOperationInput{}, workspaceError("INVALID_SENSITIVE_OPERATION", "sensitive operation is unsupported", ErrInvalidQuery)
	}
	if input.Target == "" || len(input.Target) > maxLabel || !utf8.ValidString(input.Target) || strings.TrimSpace(input.Target) != input.Target {
		return SensitiveOperationInput{}, workspaceError("INVALID_SENSITIVE_INPUT", "sensitive target must be bounded valid UTF-8 without surrounding whitespace", ErrInvalidQuery)
	}
	if input.MediaType == "" || len(input.MediaType) > maxLabel || !utf8.ValidString(input.MediaType) || strings.TrimSpace(input.MediaType) != input.MediaType || strings.ContainsAny(input.MediaType, "\r\n") {
		return SensitiveOperationInput{}, workspaceError("INVALID_SENSITIVE_INPUT", "sensitive media type must be bounded canonical UTF-8", ErrInvalidQuery)
	}
	if len(input.Payload) == 0 || len(input.Payload) > maxPayload {
		return SensitiveOperationInput{}, workspaceError("SENSITIVE_INPUT_LIMIT", "sensitive payload is empty or exceeds the configured render bound", ErrLimit)
	}
	canonical := input
	canonical.Payload = append([]byte(nil), input.Payload...)
	return canonical, nil
}

func (w *Workspace) ExecuteExport(ctx context.Context, authorization SensitiveOperationRequest, input SensitiveOperationInput, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	return w.executeSensitive(ctx, SensitiveExport, authorization, input, executor)
}

func (w *Workspace) ExecutePublish(ctx context.Context, authorization SensitiveOperationRequest, input SensitiveOperationInput, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	return w.executeSensitive(ctx, SensitivePublish, authorization, input, executor)
}

func (w *Workspace) ExecuteAttachment(ctx context.Context, authorization SensitiveOperationRequest, input SensitiveOperationInput, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	return w.executeSensitive(ctx, SensitiveAttachment, authorization, input, executor)
}

func (w *Workspace) ExecuteProductionCapture(ctx context.Context, authorization SensitiveOperationRequest, input SensitiveOperationInput, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	return w.executeSensitive(ctx, SensitiveProductionCapture, authorization, input, executor)
}

func (w *Workspace) ExecuteSign(ctx context.Context, authorization SensitiveOperationRequest, input SensitiveOperationInput, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	return w.executeSensitive(ctx, SensitiveSign, authorization, input, executor)
}

func (w *Workspace) executeSensitive(ctx context.Context, operation SensitiveOperation, authorization SensitiveOperationRequest, input SensitiveOperationInput, executor SensitiveExecutor) (SensitiveExecutionResult, error) {
	if w == nil {
		return SensitiveExecutionResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if ctx == nil {
		return SensitiveExecutionResult{}, workspaceError("INVALID_CONTEXT", "context is nil", ErrInvalidQuery)
	}
	if err := ctx.Err(); err != nil {
		return SensitiveExecutionResult{}, err
	}
	if executor == nil {
		return SensitiveExecutionResult{}, workspaceError("SENSITIVE_EXECUTOR_REQUIRED", "a sensitive executor is required", ErrSensitiveExecutor)
	}
	canonical, err := canonicalSensitiveOperationInput(operation, input, w.limits.MaxRenderBytes, w.limits.MaxQueryBytes)
	if err != nil {
		return SensitiveExecutionResult{}, err
	}
	inputHash, err := SensitiveOperationInputHash(operation, canonical)
	if err != nil {
		return SensitiveExecutionResult{}, err
	}
	if authorization.Operation != operation {
		w.auditSensitiveExecutionDenial(authorization, "SENSITIVE_CAPABILITY_DENIED")
		return SensitiveExecutionResult{}, workspaceError("SENSITIVE_CAPABILITY_DENIED", "execution entry point does not match the requested sensitive operation", ErrSensitiveOperationDenied)
	}
	if authorization.Evidence.OperationInputHash != inputHash {
		w.auditSensitiveExecutionDenial(authorization, "SENSITIVE_INPUT_BINDING_MISMATCH")
		return SensitiveExecutionResult{}, workspaceError("SENSITIVE_INPUT_BINDING_MISMATCH", "execution input does not match approved evidence", ErrSensitiveOperationDenied)
	}
	receipt, err := w.AuthorizeSensitiveOperation(authorization)
	if err != nil {
		return SensitiveExecutionResult{}, err
	}
	result := SensitiveExecutionResult{Authorization: receipt}
	if err := ctx.Err(); err != nil {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(authorization, receipt, false, "SENSITIVE_EXECUTION_CANCELLED")
		return result, err
	}
	outcome, executeErr := callSensitiveExecutor(ctx, executor, canonical)
	result.Executed = true
	if executeErr != nil {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(authorization, receipt, false, "SENSITIVE_EXECUTOR_FAILED")
		return result, errors.Join(ErrSensitiveExecutor, executeErr)
	}
	if outcome.Bytes < 0 || outcome.Bytes > int64(w.limits.MaxRenderBytes) || !validSHA256(outcome.ResultHash) || len(outcome.ExternalID) > w.limits.MaxQueryBytes || !utf8.ValidString(outcome.ExternalID) {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(authorization, receipt, false, "INVALID_SENSITIVE_OUTCOME")
		return result, workspaceError("INVALID_SENSITIVE_OUTCOME", "executor returned invalid or unbounded non-secret evidence", ErrSensitiveExecutor)
	}
	result.Outcome = outcome
	result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(authorization, receipt, true, "execution completed")
	return result, nil
}

func callSensitiveExecutor(ctx context.Context, executor SensitiveExecutor, input SensitiveOperationInput) (outcome SensitiveExecutionOutcome, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("sensitive executor panic: %v", recovered)
		}
	}()
	return executor(ctx, input)
}

func (w *Workspace) auditSensitiveExecutionDenial(request SensitiveOperationRequest, reason string) {
	_, evidenceHash, _ := canonicalSensitiveEvidence(request.Evidence, w.limits.MaxQueryBytes)
	w.mu.Lock()
	defer w.mu.Unlock()
	actor := "unavailable"
	candidate := uint64(0)
	if authority, err := w.sensitiveAuthorityLocked(request.Authority); err == nil {
		actor = authority.actor
		candidate = authority.candidate.value.serial
	}
	w.appendSensitiveAuditLocked(actor, request.Operation, candidate, request.PolicyRevision, evidenceHash, false, reason)
}

func (w *Workspace) auditSensitiveExecutionOutcome(request SensitiveOperationRequest, receipt SensitiveOperationReceipt, allowed bool, reason string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	candidate := uint64(0)
	if authority := w.sensitiveAuthorities[request.Authority.value.serial]; authority != nil && authority.handle == request.Authority {
		candidate = authority.candidate.value.serial
	}
	entry := w.appendSensitiveAuditLocked(receipt.Actor, receipt.Operation, candidate, receipt.PolicyRevision, receipt.EvidenceHash, allowed, reason)
	return entry.EventHash
}
