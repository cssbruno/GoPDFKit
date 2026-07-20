// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/cssbruno/paperrune/internal/paperedit"
	"github.com/cssbruno/paperrune/internal/paperlang"
)

// DiagnosticRemedyCode is a closed protocol allowlist. It is deliberately not
// a free-form patch name or compiler diagnostic code.
type DiagnosticRemedyCode string

const (
	RemedySetBindingPath        DiagnosticRemedyCode = "set_binding_path"
	RemedyAllowNullableBinding  DiagnosticRemedyCode = "allow_nullable_binding"
	RemedyFillRequiredSlot      DiagnosticRemedyCode = "fill_required_slot"
	RemedySetComponentReference DiagnosticRemedyCode = "set_component_reference"
)

// PaperDiagnosticFixPayload is a typed union whose permitted fields are fixed
// by Remedy. Unused fields are rejected rather than silently ignored.
type PaperDiagnosticFixPayload struct {
	Path      string               `json:"path,omitempty"`
	Slot      string               `json:"slot,omitempty"`
	Component string               `json:"component,omitempty"`
	Content   []paperedit.NodeSpec `json:"content,omitempty"`
}

type PaperApplyDiagnosticFixRequest struct {
	Guard                 PaperMutationGuard        `json:"guard"`
	DiagnosticFingerprint string                    `json:"diagnostic_fingerprint"`
	Remedy                DiagnosticRemedyCode      `json:"remedy"`
	Payload               PaperDiagnosticFixPayload `json:"payload"`
}

// PaperDiagnosticFingerprint binds the complete deterministic diagnostic
// identity to one exact source digest. Callers can compute it from diagnostic
// evidence returned by context/compile without access to workspace internals.
func PaperDiagnosticFingerprint(revision paperedit.Revision, diagnostic paperlang.Diagnostic) string {
	payload := struct {
		Revision   paperedit.Revision   `json:"revision"`
		Diagnostic paperlang.Diagnostic `json:"diagnostic"`
	}{revision, diagnostic}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// PaperApplyDiagnosticFix verifies one exact compiler diagnostic and lowers a
// stable typed remedy to minimal paperedit operations. No arbitrary operation
// list or source replacement is accepted at this boundary.
func (w *Workspace) PaperApplyDiagnosticFix(request PaperApplyDiagnosticFixRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	if !validLowerSHA256(request.DiagnosticFingerprint) {
		return PaperMutationResult{}, workspaceError("INVALID_DIAGNOSTIC_FINGERPRINT", "diagnostic fingerprint must be a lowercase SHA-256 digest", ErrInvalidQuery)
	}
	nodes := sourceNodesByID(revision.parsed.AST.Root, request.Guard.Target)
	if len(nodes) == 0 {
		for _, mapping := range revision.compiled.Mapping.Nodes {
			if mapping.ID == request.Guard.Target && mapping.InstancePath != "" {
				return PaperMutationResult{}, workspaceError("INSTANCE_TARGET", "diagnostic fixes must target source definitions or invocations, not expanded instances", paperedit.ErrInvalidOperation)
			}
		}
		return PaperMutationResult{}, workspaceError("NODE_NOT_FOUND", "diagnostic fix target was not found in source", ErrRevisionNotFound)
	}
	if len(nodes) != 1 {
		return PaperMutationResult{}, workspaceError("AMBIGUOUS_TARGET", "diagnostic fix target ID resolves to more than one source node", paperedit.ErrInvalidOperation)
	}
	node := nodes[0]
	diagnostic, err := exactDiagnosticForFix(revision, request.DiagnosticFingerprint)
	if err != nil {
		return PaperMutationResult{}, err
	}
	if diagnosticOwner(revision.parsed.AST.Root, diagnostic.Span) != node {
		return PaperMutationResult{}, workspaceError("DIAGNOSTIC_TARGET_CONFLICT", "diagnostic does not belong to the exact source target", ErrRevisionConflict)
	}

	var operations []paperedit.Operation
	targets := []string{request.Guard.Target}
	switch request.Remedy {
	case RemedySetBindingPath:
		if diagnostic.Code != "PAPER_BIND_PATH" {
			return PaperMutationResult{}, remedyMismatch()
		}
		if request.Payload.Path == "" || request.Payload.Slot != "" || request.Payload.Component != "" || len(request.Payload.Content) != 0 ||
			len(request.Payload.Path) > w.limits.MaxQueryBytes || !validBindingPath(request.Payload.Path) {
			return PaperMutationResult{}, invalidRemedyPayload("set_binding_path requires only one bounded binding path")
		}
		if node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading && node.Kind != paperlang.NodeUse {
			return PaperMutationResult{}, invalidRemedyPayload("binding remedy target must be a paragraph, heading, or component use")
		}
		operations = []paperedit.Operation{paperedit.SetProperty{Target: request.Guard.Target, Name: "bind", Value: paperedit.StringValue(request.Payload.Path)}}

	case RemedyAllowNullableBinding:
		if diagnostic.Code != "PAPER_BIND_NULLABLE" {
			return PaperMutationResult{}, remedyMismatch()
		}
		if !emptyDiagnosticFixPayload(request.Payload) {
			return PaperMutationResult{}, invalidRemedyPayload("allow_nullable_binding accepts no payload")
		}
		if node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading && node.Kind != paperlang.NodeUse {
			return PaperMutationResult{}, invalidRemedyPayload("nullable binding remedy target is unsupported")
		}
		operations = []paperedit.Operation{paperedit.SetProperty{Target: request.Guard.Target, Name: "bind-required", Value: paperedit.BoolValue(false)}}

	case RemedyFillRequiredSlot:
		if diagnostic.Code != "PAPER_SLOT_MISSING" {
			return PaperMutationResult{}, remedyMismatch()
		}
		if request.Payload.Path != "" || request.Payload.Component != "" {
			return PaperMutationResult{}, invalidRemedyPayload("fill_required_slot accepts only slot and typed content")
		}
		operation, fillTargets, err := w.prepareSlotFill(revision, request.Guard.Target, request.Payload.Slot, request.Payload.Content)
		if err != nil {
			return PaperMutationResult{}, err
		}
		operations = []paperedit.Operation{operation}
		targets = fillTargets

	case RemedySetComponentReference:
		if diagnostic.Code != "PAPER_COMPONENT_UNKNOWN" {
			return PaperMutationResult{}, remedyMismatch()
		}
		if request.Payload.Component == "" || request.Payload.Path != "" || request.Payload.Slot != "" || len(request.Payload.Content) != 0 ||
			len(request.Payload.Component) > w.limits.MaxQueryBytes || request.Payload.Component[0] != '@' {
			return PaperMutationResult{}, invalidRemedyPayload("set_component_reference requires only one bounded component @id")
		}
		if node.Kind != paperlang.NodeUse {
			return PaperMutationResult{}, invalidRemedyPayload("component reference remedy target must be a use")
		}
		if _, err := uniqueComponentDefinition(revision.parsed.AST.Root, request.Payload.Component); err != nil {
			return PaperMutationResult{}, err
		}
		operations = []paperedit.Operation{paperedit.SetProperty{Target: request.Guard.Target, Name: "component", Value: paperedit.StringValue(request.Payload.Component)}}

	default:
		return PaperMutationResult{}, workspaceError("REMEDY_NOT_ALLOWED", "diagnostic remedy code is not in the stable allowlist", paperedit.ErrInvalidOperation)
	}

	return w.applyPaperMutation("apply_fix:"+string(request.Remedy), request.Guard, opened, revision, targets, operations, "DIAGNOSTIC_FIX_REJECTED")
}

func exactDiagnosticForFix(revision *revisionRecord, fingerprint string) (paperlang.Diagnostic, error) {
	var found paperlang.Diagnostic
	matches := 0
	for _, diagnostic := range revision.compiled.Diagnostics {
		if PaperDiagnosticFingerprint(revision.revision, diagnostic) != fingerprint {
			continue
		}
		found = diagnostic
		matches++
	}
	switch matches {
	case 0:
		return paperlang.Diagnostic{}, workspaceError("DIAGNOSTIC_CONFLICT", "diagnostic fingerprint is absent from the exact source revision", ErrRevisionConflict)
	case 1:
		return found, nil
	default:
		return paperlang.Diagnostic{}, workspaceError("AMBIGUOUS_DIAGNOSTIC", "diagnostic fingerprint resolves more than once", paperedit.ErrInvalidOperation)
	}
}

func validLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func spanInsideOrEqual(inner, outer paperlang.Span) bool {
	return inner.File != "" && inner.File == outer.File && inner.Start.Offset >= outer.Start.Offset && inner.End.Offset <= outer.End.Offset
}

func sourceNodesByID(root *paperlang.Node, id string) []*paperlang.Node {
	var result []*paperlang.Node
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil {
			return
		}
		if node.ID == id {
			result = append(result, node)
		}
		for _, member := range node.Members {
			walk(member.Node)
		}
	}
	walk(root)
	return result
}

// diagnosticOwner returns the deepest source node containing the diagnostic
// span. This prevents a fuzzy ancestor ID from claiming a descendant's fix.
func diagnosticOwner(root *paperlang.Node, span paperlang.Span) *paperlang.Node {
	var owner *paperlang.Node
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil || !spanInsideOrEqual(span, node.Span) {
			return
		}
		owner = node
		for _, member := range node.Members {
			walk(member.Node)
		}
	}
	walk(root)
	return owner
}

func emptyDiagnosticFixPayload(payload PaperDiagnosticFixPayload) bool {
	return payload.Path == "" && payload.Slot == "" && payload.Component == "" && len(payload.Content) == 0
}

func remedyMismatch() error {
	return workspaceError("REMEDY_MISMATCH", "remedy code does not match the exact diagnostic code", paperedit.ErrInvalidOperation)
}

func invalidRemedyPayload(message string) error {
	return workspaceError("INVALID_REMEDY_PAYLOAD", message, paperedit.ErrInvalidOperation)
}
