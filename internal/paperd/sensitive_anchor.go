// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrAuditAnchor = errors.New("paperd: sensitive audit anchor failed")

type SensitiveAuditRootStatement struct {
	Version       uint16 `json:"version"`
	PartitionHash string `json:"partition_hash"`
	FirstSequence uint64 `json:"first_sequence"`
	LastSequence  uint64 `json:"last_sequence"`
	Count         uint32 `json:"count"`
	PreviousHash  string `json:"previous_hash,omitempty"`
	RootHash      string `json:"root_hash"`
	EntriesHash   string `json:"entries_hash"`
}

type AuditRootSignature struct {
	Scheme      string
	KeyID       string
	AnchorURI   string
	ReceiptHash string
	Signature   []byte
}

type AuditRootSigner func(context.Context, []byte) (AuditRootSignature, error)

type SensitiveAuditAnchorRequest struct {
	Authorization SensitiveOperationRequest
	Limit         int
}

type SensitiveAuditAnchor struct {
	Authorization      SensitiveOperationReceipt   `json:"authorization"`
	ExecutionAuditHash string                      `json:"execution_audit_hash"`
	Statement          SensitiveAuditRootStatement `json:"statement"`
	StatementHash      string                      `json:"statement_hash"`
	Scheme             string                      `json:"scheme"`
	KeyID              string                      `json:"key_id"`
	AnchorURI          string                      `json:"anchor_uri"`
	ReceiptHash        string                      `json:"receipt_hash"`
	SignatureHash      string                      `json:"signature_hash"`
	Signature          []byte                      `json:"-"`
}

func (w *Workspace) SensitiveAuditAnchorInputHash(limit int) (string, error) {
	statement, payload, err := w.sensitiveAuditRootStatement(limit)
	if err != nil {
		return "", err
	}
	return SensitiveOperationInputHash(SensitiveSign, SensitiveOperationInput{Target: "paperd:sensitive-audit-root:" + statement.RootHash, MediaType: "application/vnd.gopdfkit.sensitive-audit-root+json", Payload: payload})
}

func (w *Workspace) AnchorSensitiveAudit(ctx context.Context, request SensitiveAuditAnchorRequest, signer AuditRootSigner) (SensitiveAuditAnchor, error) {
	if w == nil {
		return SensitiveAuditAnchor{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if ctx == nil || signer == nil {
		return SensitiveAuditAnchor{}, workspaceError("INVALID_AUDIT_ANCHOR", "context and signer are required", ErrInvalidQuery)
	}
	if err := ctx.Err(); err != nil {
		return SensitiveAuditAnchor{}, err
	}
	statement, payload, err := w.sensitiveAuditRootStatement(request.Limit)
	if err != nil {
		return SensitiveAuditAnchor{}, err
	}
	inputHash, err := SensitiveOperationInputHash(SensitiveSign, SensitiveOperationInput{Target: "paperd:sensitive-audit-root:" + statement.RootHash, MediaType: "application/vnd.gopdfkit.sensitive-audit-root+json", Payload: payload})
	if err != nil {
		return SensitiveAuditAnchor{}, err
	}
	if request.Authorization.Operation != SensitiveSign || request.Authorization.Evidence.OperationInputHash != inputHash {
		w.auditSensitiveExecutionDenial(request.Authorization, "AUDIT_ANCHOR_BINDING_MISMATCH")
		return SensitiveAuditAnchor{}, workspaceError("AUDIT_ANCHOR_BINDING_MISMATCH", "audit anchor is not bound to the current exact root statement", ErrSensitiveOperationDenied)
	}
	receipt, err := w.AuthorizeSensitiveOperation(request.Authorization)
	if err != nil {
		return SensitiveAuditAnchor{}, err
	}
	result := SensitiveAuditAnchor{Authorization: receipt, Statement: statement, StatementHash: digestBytes(payload)}
	signature, signErr := callAuditRootSigner(ctx, signer, append([]byte(nil), payload...))
	if signErr != nil {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, receipt, false, "AUDIT_ANCHOR_SIGNER_FAILED")
		return result, errors.Join(ErrAuditAnchor, signErr)
	}
	if !validSensitiveLabel(signature.Scheme, w.limits.MaxQueryBytes) || !validSensitiveLabel(signature.KeyID, w.limits.MaxQueryBytes) || !validSensitiveLabel(signature.AnchorURI, w.limits.MaxQueryBytes) || !validSHA256(signature.ReceiptHash) || len(signature.Signature) == 0 || len(signature.Signature) > w.limits.MaxContextBytes {
		result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, receipt, false, "INVALID_AUDIT_ANCHOR_RECEIPT")
		return result, workspaceError("INVALID_AUDIT_ANCHOR_RECEIPT", "external anchor returned invalid or unbounded evidence", ErrAuditAnchor)
	}
	result.Scheme, result.KeyID, result.AnchorURI, result.ReceiptHash = signature.Scheme, signature.KeyID, signature.AnchorURI, signature.ReceiptHash
	result.Signature = append([]byte(nil), signature.Signature...)
	result.SignatureHash = digestBytes(signature.Signature)
	result.ExecutionAuditHash = w.auditSensitiveExecutionOutcome(request.Authorization, receipt, true, "sensitive audit root externally anchored")
	w.retainSensitiveAuditAnchor(result)
	return result, nil
}

// SensitiveAuditAnchors returns retained public anchor receipts. Signature
// bytes are intentionally excluded: persistence and protocol summaries keep
// only their digest alongside the external receipt identity.
func (w *Workspace) SensitiveAuditAnchors(limit int) ([]SensitiveAuditAnchor, error) {
	if w == nil {
		return nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if limit <= 0 || limit > w.limits.MaxAuthorizationAudit {
		return nil, workspaceError("AUDIT_LIMIT", "audit anchor limit is outside configured bounds", ErrLimit)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	start := len(w.sensitiveAuditAnchors) - limit
	if start < 0 {
		start = 0
	}
	return clonePersistentAuditAnchors(w.sensitiveAuditAnchors[start:]), nil
}

func (w *Workspace) retainSensitiveAuditAnchor(anchor SensitiveAuditAnchor) {
	anchor.Signature = nil
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(w.sensitiveAuditAnchors) >= w.limits.MaxAuthorizationAudit {
		w.sensitiveAuditAnchors = w.sensitiveAuditAnchors[1:]
	}
	w.sensitiveAuditAnchors = append(w.sensitiveAuditAnchors, anchor)
}

func clonePersistentAuditAnchors(values []SensitiveAuditAnchor) []SensitiveAuditAnchor {
	result := append([]SensitiveAuditAnchor(nil), values...)
	for index := range result {
		result[index].Signature = nil
	}
	return result
}

func validatePersistentAuditAnchor(anchor SensitiveAuditAnchor, limit int) error {
	payload, err := json.Marshal(anchor.Statement)
	if err != nil || anchor.Statement.Version != 1 || anchor.Statement.Count == 0 || anchor.Statement.FirstSequence == 0 || anchor.Statement.LastSequence < anchor.Statement.FirstSequence ||
		uint64(anchor.Statement.Count) != anchor.Statement.LastSequence-anchor.Statement.FirstSequence+1 || anchor.StatementHash != digestBytes(payload) || !validSHA256(anchor.Statement.RootHash) || !validSHA256(anchor.Statement.EntriesHash) ||
		!validSHA256(anchor.Statement.PartitionHash) || (anchor.Statement.PreviousHash != "" && !validSHA256(anchor.Statement.PreviousHash)) ||
		anchor.Authorization.Operation != SensitiveSign || !validSensitiveLabel(anchor.Authorization.Actor, limit) || !validSensitiveLabel(anchor.Authorization.PolicyRevision, limit) || !validSHA256(anchor.Authorization.EvidenceHash) || !validSHA256(anchor.Authorization.AuditHash) || !validSHA256(anchor.ExecutionAuditHash) ||
		!validSensitiveLabel(anchor.Scheme, limit) || !validSensitiveLabel(anchor.KeyID, limit) || !validSensitiveLabel(anchor.AnchorURI, limit) || !validSHA256(anchor.ReceiptHash) || !validSHA256(anchor.SignatureHash) || len(anchor.Signature) != 0 {
		return workspaceError("PERSISTENCE_ANCHOR", "persisted sensitive audit anchor receipt is invalid", ErrPersistenceCorrupt)
	}
	return nil
}

func (w *Workspace) sensitiveAuditRootStatement(limit int) (SensitiveAuditRootStatement, []byte, error) {
	if w == nil {
		return SensitiveAuditRootStatement{}, nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if limit <= 0 || limit > w.limits.MaxAuthorizationAudit {
		return SensitiveAuditRootStatement{}, nil, workspaceError("AUDIT_LIMIT", "audit anchor limit is outside configured bounds", ErrLimit)
	}
	w.mu.RLock()
	start := len(w.sensitiveAudit) - limit
	if start < 0 {
		start = 0
	}
	entries := append([]SensitiveAuditEntry(nil), w.sensitiveAudit[start:]...)
	projectID, policyRevision, disclosure := w.projectID, w.policyRevision, w.disclosureDomain
	w.mu.RUnlock()
	if len(entries) == 0 {
		return SensitiveAuditRootStatement{}, nil, workspaceError("EMPTY_AUDIT_ROOT", "no sensitive audit entries are available to anchor", ErrAuditAnchor)
	}
	for index, entry := range entries {
		if entry.EventHash != sensitiveAuditEventHash(entry) || (index > 0 && entry.PreviousHash != entries[index-1].EventHash) {
			return SensitiveAuditRootStatement{}, nil, workspaceError("CORRUPT_AUDIT_CHAIN", "sensitive audit chain verification failed", ErrAuditAnchor)
		}
	}
	encodedEntries, err := json.Marshal(entries)
	if err != nil {
		return SensitiveAuditRootStatement{}, nil, err
	}
	partition := sha256.Sum256([]byte(projectID + "\x00" + policyRevision + "\x00" + string(disclosure)))
	statement := SensitiveAuditRootStatement{Version: 1, PartitionHash: hex.EncodeToString(partition[:]), FirstSequence: entries[0].Sequence, LastSequence: entries[len(entries)-1].Sequence,
		Count: uint32(len(entries)), PreviousHash: entries[0].PreviousHash, RootHash: entries[len(entries)-1].EventHash, EntriesHash: digestBytes(encodedEntries)}
	payload, err := json.Marshal(statement)
	if err != nil || uint64(len(payload)) > uint64(w.limits.MaxContextBytes) {
		return SensitiveAuditRootStatement{}, nil, workspaceError("AUDIT_ANCHOR_LIMIT", "audit root statement exceeds configured bounds", ErrLimit)
	}
	return statement, payload, nil
}

func callAuditRootSigner(ctx context.Context, signer AuditRootSigner, payload []byte) (result AuditRootSignature, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("audit root signer panic: %v", recovered)
		}
	}()
	return signer(ctx, payload)
}
