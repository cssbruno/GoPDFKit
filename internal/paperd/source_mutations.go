// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type sourceIdempotencyRecord struct {
	fingerprint string
	result      ApplyResult
}

// PaperMutationGuard binds one mutation to a single source revision domain,
// exact opened candidate head, source digest, and target fingerprint.
type PaperMutationGuard struct {
	Open                OpenHandle                `json:"-"`
	Authority           MutationAuthorityHandle   `json:"-"`
	Candidate           CandidateHandle           `json:"-"`
	ExpectedHead        RevisionHandle            `json:"-"`
	ExpectedDigest      paperedit.Revision        `json:"expected_digest"`
	Target              string                    `json:"target"`
	ExpectedFingerprint paperedit.NodeFingerprint `json:"expected_fingerprint"`
	ExpectedInstance    string                    `json:"expected_instance"`
	// TargetPreconditions must cover every additional target addressed by a
	// multi-target semantic operation. They are never inferred or fuzzy-rebased.
	TargetPreconditions []paperedit.TargetPrecondition `json:"target_preconditions,omitempty"`
	IdempotencyKey      string                         `json:"idempotency_key"`
}

type PaperSemanticDiff struct {
	Domain             string   `json:"domain"`
	Operation          string   `json:"operation"`
	Targets            []string `json:"targets"`
	InvalidatedNodeIDs []string `json:"invalidated_node_ids,omitempty"`
	WholeDocument      bool     `json:"whole_document"`
	BeforeCompileOK    bool     `json:"before_compile_ok"`
	AfterCompileOK     bool     `json:"after_compile_ok"`
}

type PaperMutationResult struct {
	Candidate     CandidateSnapshot             `json:"candidate"`
	Revision      RevisionSnapshot              `json:"revision"`
	Edit          paperedit.Result              `json:"edit"`
	Semantic      PaperSemanticDiff             `json:"semantic_diff"`
	Authorization MutationAuthorizationEvidence `json:"authorization"`
}

type PaperSetLiteralRequest struct {
	Guard PaperMutationGuard `json:"guard"`
	Text  string             `json:"text"`
}

// PaperSetLiteral changes one unambiguous literal representation while
// retaining surrounding source trivia and layout properties.
func (w *Workspace) PaperSetLiteral(request PaperSetLiteralRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	if !utf8.ValidString(request.Text) || len(request.Text) > w.maxMutationPayloadBytes() {
		return PaperMutationResult{}, workspaceError("MUTATION_PAYLOAD_LIMIT", "literal text must be valid UTF-8 within the configured source limit", ErrLimit)
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	operation, addressed, err := literalOperation(node, request.Guard.Target, request.Text)
	if err != nil {
		return PaperMutationResult{}, err
	}
	return w.applyPaperMutation("set_literal", request.Guard, opened, revision, addressed, []paperedit.Operation{operation}, "")
}

// PaperRichTextRun addresses one styled text node. Styling remains authored
// on that node; this operation changes only its inline literal bytes.
type PaperRichTextRun struct {
	Target string `json:"target"`
	Text   string `json:"text"`
}

type PaperSetRichTextRequest struct {
	Guard PaperMutationGuard `json:"guard"`
	Runs  []PaperRichTextRun `json:"runs"`
}

// PaperSetRichText updates one or more explicitly identified direct text runs
// under a paragraph or heading. Operations are canonicalized to source order,
// so semantically identical request order produces identical patches.
func (w *Workspace) PaperSetRichText(request PaperSetRichTextRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	if len(request.Runs) == 0 || len(request.Runs) > w.limits.MaxOperations {
		return PaperMutationResult{}, workspaceError("MUTATION_PAYLOAD_LIMIT", "rich-text run count is outside configured bounds", ErrLimit)
	}
	total := 0
	requested := make(map[string]string, len(request.Runs))
	for _, run := range request.Runs {
		if run.Target == "" || len(run.Target) > w.limits.MaxQueryBytes || !utf8.ValidString(run.Text) {
			return PaperMutationResult{}, workspaceError("INVALID_RICH_TEXT", "rich-text runs require bounded readable targets and valid UTF-8", ErrInvalidQuery)
		}
		if _, duplicate := requested[run.Target]; duplicate {
			return PaperMutationResult{}, workspaceError("AMBIGUOUS_TARGET", "rich-text run target is declared more than once", paperedit.ErrInvalidOperation)
		}
		total += len(run.Text)
		if total > w.maxMutationPayloadBytes() {
			return PaperMutationResult{}, workspaceError("MUTATION_PAYLOAD_LIMIT", "rich-text payload exceeds the configured source limit", ErrLimit)
		}
		requested[run.Target] = run.Text
	}
	container := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if container == nil || (container.Kind != paperlang.NodeParagraph && container.Kind != paperlang.NodeHeading) {
		return PaperMutationResult{}, workspaceError("INVALID_RICH_TEXT", "rich text target must be a paragraph or heading", paperedit.ErrInvalidOperation)
	}
	for _, member := range container.Members {
		if member.Property != nil && member.Property.Name == "text" {
			return PaperMutationResult{}, workspaceError("AMBIGUOUS_TARGET", "rich text target mixes a text property with addressed runs", paperedit.ErrInvalidOperation)
		}
	}
	operations := make([]paperedit.Operation, 0, len(requested))
	addressed := make([]string, 0, len(requested)+1)
	addressed = append(addressed, request.Guard.Target)
	for _, member := range container.Members {
		child := member.Node
		if child == nil || child.Kind != paperlang.NodeText {
			continue
		}
		text, selected := requested[child.ID]
		if !selected {
			continue
		}
		if child.ID == "" || child.Value == nil {
			return PaperMutationResult{}, workspaceError("AMBIGUOUS_TARGET", "rich-text runs must be readable inline text nodes", paperedit.ErrInvalidOperation)
		}
		operations = append(operations, paperedit.ReplaceText{Target: child.ID, Text: text})
		addressed = append(addressed, child.ID)
		delete(requested, child.ID)
	}
	if len(requested) != 0 {
		return PaperMutationResult{}, workspaceError("AMBIGUOUS_TARGET", "a rich-text run is not a direct text child of the target", paperedit.ErrInvalidOperation)
	}
	return w.applyPaperMutation("set_rich_text", request.Guard, opened, revision, addressed, operations, "")
}

type PaperSetBindingRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	Path     string             `json:"path"`
	Required *bool              `json:"required,omitempty"`
}

// PaperSetBinding authors a typed binding property on a supported semantic
// node. The edited source is semantically compiled before publication.
func (w *Workspace) PaperSetBinding(request PaperSetBindingRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	if len(request.Path) == 0 || len(request.Path) > w.limits.MaxQueryBytes || !validBindingPath(request.Path) {
		return PaperMutationResult{}, workspaceError("INVALID_BINDING", "binding path must be a bounded absolute or relative dotted path", paperedit.ErrInvalidOperation)
	}
	node := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || (node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading && node.Kind != paperlang.NodeUse) {
		return PaperMutationResult{}, workspaceError("INVALID_BINDING", "binding target must be a paragraph, heading, or component use", paperedit.ErrInvalidOperation)
	}
	bindCount := 0
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == "bind" {
			bindCount++
		}
	}
	if bindCount > 1 {
		return PaperMutationResult{}, workspaceError("AMBIGUOUS_TARGET", "binding target has more than one bind property", paperedit.ErrInvalidOperation)
	}
	operations := []paperedit.Operation{paperedit.SetProperty{Target: request.Guard.Target, Name: "bind", Value: paperedit.StringValue(request.Path)}}
	if request.Required != nil {
		operations = append(operations, paperedit.SetProperty{Target: request.Guard.Target, Name: "bind-required", Value: paperedit.BoolValue(*request.Required)})
	}
	return w.applyPaperMutation("set_binding", request.Guard, opened, revision, []string{request.Guard.Target}, operations, "INVALID_BINDING")
}

func (w *Workspace) mutationRevision(guard PaperMutationGuard) (*openRecord, *revisionRecord, error) {
	if w == nil {
		return nil, nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if guard.IdempotencyKey == "" || len(guard.IdempotencyKey) > paperedit.MaxIdempotencyKeyBytes || !utf8.ValidString(guard.IdempotencyKey) {
		return nil, nil, workspaceError("INVALID_IDEMPOTENCY_KEY", "source mutation idempotency key must be valid UTF-8 within the edit limit", ErrInvalidQuery)
	}
	if guard.Target == "" || len(guard.Target) > w.limits.MaxQueryBytes {
		return nil, nil, workspaceError("INVALID_QUERY", "mutation target is empty or exceeds the configured limit", ErrInvalidQuery)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	opened, err := w.openLocked(guard.Open)
	if err != nil {
		return nil, nil, err
	}
	if opened.mode != CapabilityEdit {
		return nil, nil, workspaceError("CAPABILITY_DENIED", "mutation requires an edit-capable open handle", ErrInvalidHandle)
	}
	if opened.candidate.value.serial == 0 || opened.candidate != guard.Candidate {
		return nil, nil, workspaceError("REVISION_CONFLICT", "mutation candidate does not match the opened source domain", ErrRevisionConflict)
	}
	if opened.revision != guard.ExpectedHead || opened.digest != guard.ExpectedDigest {
		return nil, nil, workspaceError("REVISION_CONFLICT", "mutation preconditions do not match the opened revision", ErrRevisionConflict)
	}
	revision, err := w.revisionLocked(opened.revision)
	if err != nil {
		return nil, nil, err
	}
	nodes := sourceNodesByID(revision.parsed.AST.Root, guard.Target)
	if len(nodes) == 0 {
		for _, mapping := range revision.compiled.Mapping.Nodes {
			if mapping.ID == guard.Target && mapping.InstancePath != "" {
				return nil, nil, w.instanceTargetError(revision, guard.Target, "INSTANCE_TARGET", "expanded instances cannot be edited; target the exact source definition or invocation", paperedit.ErrInvalidOperation)
			}
		}
		return nil, nil, w.instanceTargetError(revision, guard.Target, "NODE_NOT_FOUND", "mutation target was not found in the exact source revision", ErrRevisionNotFound)
	}
	if len(nodes) != 1 {
		return nil, nil, w.instanceTargetError(revision, guard.Target, "AMBIGUOUS_TARGET", "mutation target does not resolve to exactly one source node", paperedit.ErrInvalidOperation)
	}
	actualInstance, err := paperedit.SourceInstance(revision.file, revision.source, guard.Target)
	if err != nil {
		return nil, nil, workspaceError("INSTANCE_TARGET", "mutation target is not an exact source definition or invocation", paperedit.ErrInvalidOperation)
	}
	if guard.ExpectedInstance == "" {
		return nil, nil, w.instanceTargetError(revision, guard.Target, "INSTANCE_PRECONDITION_REQUIRED", "mutation requires an exact source-instance precondition", paperedit.ErrInvalidOperation)
	}
	if guard.ExpectedInstance != actualInstance {
		return nil, nil, w.instanceTargetError(revision, guard.Target, "INSTANCE_CONFLICT", "mutation source instance changed after review", ErrRevisionConflict)
	}
	return opened, revision, nil
}

func (w *Workspace) instanceTargetError(revision *revisionRecord, target, code, message string, cause error) error {
	candidates := make([]paperedit.TargetCandidate, 0, paperedit.MaxDiagnosticCandidates)
	if revision != nil {
		if instance, err := paperedit.SourceInstance(revision.file, revision.source, target); err == nil {
			if nodes := sourceNodesByID(revision.parsed.AST.Root, target); len(nodes) == 1 {
				candidates = append(candidates, paperedit.TargetCandidate{Target: target, Instance: instance, Kind: nodes[0].Kind, Span: nodes[0].Span})
			}
		}
		seen := make(map[string]bool)
		for _, candidate := range candidates {
			seen[candidate.Instance] = true
		}
		mappings := append([]papercompile.NodeMapping(nil), revision.compiled.Mapping.Nodes...)
		sort.Slice(mappings, func(i, j int) bool {
			if mappings[i].InstancePath != mappings[j].InstancePath {
				return mappings[i].InstancePath < mappings[j].InstancePath
			}
			return mappings[i].Span.Start.Offset < mappings[j].Span.Start.Offset
		})
		for _, mapping := range mappings {
			if mapping.ID != target || mapping.InstancePath == "" || seen[mapping.InstancePath] || len(candidates) >= paperedit.MaxDiagnosticCandidates {
				continue
			}
			seen[mapping.InstancePath] = true
			span := mapping.InvocationSpan
			if span.File == "" {
				span = mapping.Span
			}
			candidates = append(candidates, paperedit.TargetCandidate{Target: target, Instance: mapping.InstancePath, Kind: mapping.Kind, Span: span})
		}
	}
	return &Error{Code: code, Message: message, Candidates: candidates, cause: cause}
}

func (w *Workspace) applyPaperMutation(kind string, guard PaperMutationGuard, opened *openRecord, before *revisionRecord, targets []string, operations []paperedit.Operation, semanticErrorCode string) (PaperMutationResult, error) {
	authorizationOperation := MutationOperation(kind)
	if strings.HasPrefix(kind, "apply_fix:") {
		authorizationOperation = MutationApplyFix
	}
	authorization, err := w.authorizeMutation(guard, authorizationOperation, before, targets)
	if err != nil {
		return PaperMutationResult{Authorization: authorization}, err
	}
	preconditions := append([]paperedit.TargetPrecondition(nil), guard.TargetPreconditions...)
	preconditions = append(preconditions, paperedit.TargetPrecondition{Target: guard.Target, ExpectedFingerprint: guard.ExpectedFingerprint, ExpectedInstance: guard.ExpectedInstance})
	if semanticErrorCode != "" {
		preview, err := paperedit.Apply(paperedit.Transaction{
			File: before.file, Source: before.source, ExpectedRevision: before.revision,
			IdempotencyKey: guard.IdempotencyKey, TargetPreconditions: preconditions, RequireExactTargets: true, Operations: cloneOperations(operations),
		})
		if err != nil {
			return PaperMutationResult{Edit: cloneEditResult(preview)}, wrapEditError(err)
		}
		prepared, err := w.prepareRevision(before.file, preview.Source)
		if err != nil {
			return PaperMutationResult{Edit: cloneEditResult(preview)}, err
		}
		if !prepared.parsed.OK() || !prepared.compiled.OK() {
			return PaperMutationResult{Edit: cloneEditResult(preview)}, workspaceError(semanticErrorCode, "typed mutation does not compile against the current source contracts", ErrInvalidSource)
		}
	}
	apply, err := w.Apply(ApplyRequest{
		Candidate: opened.candidate, ExpectedHead: guard.ExpectedHead, ExpectedRevision: guard.ExpectedDigest,
		IdempotencyKey: guard.IdempotencyKey, TargetPreconditions: preconditions, Operations: operations,
	})
	if err != nil {
		return PaperMutationResult{Edit: cloneEditResult(apply.Edit)}, err
	}
	semantic := PaperSemanticDiff{
		Domain: "source", Operation: kind, Targets: append([]string(nil), targets...),
		BeforeCompileOK: before.parsed.OK() && before.compiled.OK(), AfterCompileOK: apply.Revision.ParseOK && apply.Revision.CompileOK,
	}
	if apply.Edit.Invalidation != nil {
		semantic.WholeDocument = apply.Edit.Invalidation.WholeDocument
		semantic.InvalidatedNodeIDs = append([]string(nil), apply.Edit.Invalidation.NodeIDs...)
	}
	return PaperMutationResult{Candidate: apply.Candidate, Revision: apply.Revision, Edit: cloneEditResult(apply.Edit), Semantic: semantic, Authorization: authorization}, nil
}

func literalOperation(node *paperlang.Node, target, text string) (paperedit.Operation, []string, error) {
	if node == nil {
		return nil, nil, workspaceError("NODE_NOT_FOUND", "literal target was not found", ErrRevisionNotFound)
	}
	if node.Kind == paperlang.NodeText {
		if node.Value == nil {
			return nil, nil, workspaceError("AMBIGUOUS_TARGET", "text target has no inline literal", paperedit.ErrInvalidOperation)
		}
		return paperedit.ReplaceText{Target: target, Text: text}, []string{target}, nil
	}
	if node.Kind != paperlang.NodeParagraph && node.Kind != paperlang.NodeHeading {
		return nil, nil, workspaceError("INVALID_LITERAL", "literal target must be text, paragraph, or heading", paperedit.ErrInvalidOperation)
	}
	properties := 0
	var child *paperlang.Node
	children := 0
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == "text" {
			properties++
		}
		if member.Node != nil && member.Node.Kind == paperlang.NodeText {
			children++
			child = member.Node
		}
	}
	if properties > 1 || children > 1 || properties == 1 && children == 1 {
		return nil, nil, workspaceError("AMBIGUOUS_TARGET", "literal target has more than one text representation", paperedit.ErrInvalidOperation)
	}
	if children == 1 {
		if child.ID == "" || child.Value == nil {
			return nil, nil, workspaceError("AMBIGUOUS_TARGET", "literal child must have a readable ID and inline value", paperedit.ErrInvalidOperation)
		}
		return paperedit.ReplaceText{Target: child.ID, Text: text}, []string{target, child.ID}, nil
	}
	return paperedit.SetProperty{Target: target, Name: "text", Value: paperedit.StringValue(text)}, []string{target}, nil
}

func validBindingPath(path string) bool {
	if !utf8.ValidString(path) || strings.ContainsAny(path, " \t\r\n") {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(path, "@"), ".")
	if len(parts) == 0 || parts[0] == "" {
		return false
	}
	for _, part := range parts {
		part = strings.TrimSuffix(part, "[]")
		if part == "" || !identifierStartASCII(part[0]) {
			return false
		}
		for i := 1; i < len(part); i++ {
			if !identifierContinueASCII(part[i]) {
				return false
			}
		}
	}
	return true
}

func identifierStartASCII(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value == '_'
}

func identifierContinueASCII(value byte) bool {
	return identifierStartASCII(value) || value >= '0' && value <= '9' || value == '-'
}

func (w *Workspace) maxMutationPayloadBytes() int {
	limit := w.limits.MaxSourceBytes
	if limit > paperedit.MaxReplacementBytes {
		limit = paperedit.MaxReplacementBytes
	}
	return limit
}

func sourceApplyFingerprint(request ApplyRequest, operations []paperedit.Operation, preconditions []paperedit.TargetPrecondition) (string, error) {
	if request.IdempotencyKey == "" {
		return "", nil
	}
	ordered := append([]paperedit.TargetPrecondition(nil), preconditions...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Target < ordered[j].Target })
	type encodedOperation struct {
		Type  string `json:"type"`
		Value any    `json:"value"`
	}
	encodedOperations := make([]encodedOperation, len(operations))
	for i, operation := range operations {
		encodedOperations[i] = encodedOperation{Type: fmt.Sprintf("%T", operation), Value: operation}
	}
	payload := struct {
		HeadSerial    uint64                         `json:"head_serial"`
		Revision      paperedit.Revision             `json:"revision"`
		Group         string                         `json:"group,omitempty"`
		Preconditions []paperedit.TargetPrecondition `json:"preconditions"`
		Operations    []encodedOperation             `json:"operations"`
	}{request.ExpectedHead.value.serial, request.ExpectedRevision, request.Group, ordered, encodedOperations}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", workspaceError("INVALID_OPERATION", "source mutation cannot be fingerprinted", paperedit.ErrInvalidOperation)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func sourceCachedResult(cached sourceIdempotencyRecord, fingerprint string) (ApplyResult, error) {
	if cached.fingerprint != fingerprint {
		return ApplyResult{}, workspaceError("IDEMPOTENCY_CONFLICT", "idempotency key was already used for a different source mutation", ErrRevisionConflict)
	}
	return cloneApplyResult(cached.result), nil
}

func cloneApplyResult(result ApplyResult) ApplyResult {
	result.Revision.ParseDiagnostics = clonePaperDiagnostics(result.Revision.ParseDiagnostics)
	result.Revision.CompileDiagnostics = clonePaperDiagnostics(result.Revision.CompileDiagnostics)
	result.Edit = cloneEditResult(result.Edit)
	return result
}
