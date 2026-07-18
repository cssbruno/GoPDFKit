// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/papertheme"
)

const LayoutIssueExplanationSchemaVersion uint16 = 1

type LayoutIssueSelector struct {
	DiagnosticCode string `json:"diagnostic_code,omitempty"`
	Node           uint32 `json:"node,omitempty"`
	Key            string `json:"key,omitempty"`
	Instance       string `json:"instance,omitempty"`
	Fragment       uint32 `json:"fragment,omitempty"`
	Page           uint32 `json:"page,omitempty"`
}

type LayoutIssueExplainRequest struct {
	Open             OpenHandle
	Plan             PlanHandle
	ExpectedRevision RevisionHandle
	ExpectedDigest   paperedit.Revision
	Selector         LayoutIssueSelector
	MaxItems         uint32
	MaxBytes         uint32
	MaxWork          uint64
}

type LayoutIssueResolution string

const (
	LayoutIssueExact     LayoutIssueResolution = "exact"
	LayoutIssueAmbiguous LayoutIssueResolution = "ambiguous"
	LayoutIssueNotFound  LayoutIssueResolution = "not_found"
)

// LayoutIssueCandidate identifies an exact expanded instance without
// returning source text or bound data values.
type LayoutIssueCandidate struct {
	Node      layoutengine.NodeID       `json:"node"`
	Key       layoutengine.NodeKey      `json:"key"`
	Instance  layoutengine.InstanceID   `json:"instance"`
	Source    layoutengine.SourceSpan   `json:"source"`
	Pages     []uint32                  `json:"pages,omitempty"`
	Fragments []layoutengine.FragmentID `json:"fragments,omitempty"`
}

type LayoutSourceCause struct {
	ID                string             `json:"id"`
	Kind              paperlang.NodeKind `json:"kind"`
	Span              paperlang.Span     `json:"span"`
	DefinitionSpan    paperlang.Span     `json:"definition_span"`
	InvocationSpan    paperlang.Span     `json:"invocation_span"`
	InstancePath      string             `json:"instance_path,omitempty"`
	BindingPath       string             `json:"binding_path,omitempty"`
	BindingSpan       paperlang.Span     `json:"binding_span"`
	BindingNullable   bool               `json:"binding_nullable,omitempty"`
	BindingCollection bool               `json:"binding_collection,omitempty"`
}

// LayoutStyleCause is the typed token/provenance chain for a computed
// property. Values are style-domain values, never document or fixture data.
type LayoutStyleCause struct {
	Property     string                `json:"property"`
	ConsumerSpan paperlang.Span        `json:"consumer_span"`
	Theme        string                `json:"theme"`
	Token        string                `json:"token"`
	Value        papertheme.Value      `json:"value"`
	Provenance   papertheme.Provenance `json:"provenance"`
}

type LayoutIssueExplainResult struct {
	SchemaVersion  uint16                            `json:"schema_version"`
	Plan           PlanSnapshot                      `json:"plan"`
	Revision       RevisionIdentity                  `json:"revision"`
	CandidateBound bool                              `json:"candidate_bound"`
	Selector       LayoutIssueSelector               `json:"selector"`
	Resolution     LayoutIssueResolution             `json:"resolution"`
	Candidates     []LayoutIssueCandidate            `json:"candidates,omitempty"`
	Source         []LayoutSourceCause               `json:"source,omitempty"`
	Styles         []LayoutStyleCause                `json:"styles,omitempty"`
	Layout         *layoutengine.ExplainLayoutTarget `json:"layout,omitempty"`
	EncodedBytes   int                               `json:"encoded_bytes"`
	CanonicalHash  string                            `json:"canonical_hash"`
}

// ExplainLayoutIssue returns a deterministic source-to-display causal chain
// for one exact retained plan and exact open revision. Candidate-backed opens
// are rejected after their head advances. Ambiguous expanded instances return
// bounded exact candidates and no guessed causal chain.
func (w *Workspace) ExplainLayoutIssue(ctx context.Context, request LayoutIssueExplainRequest) (LayoutIssueExplainResult, error) {
	if w == nil {
		return LayoutIssueExplainResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LayoutIssueExplainResult{}, err
	}
	if request.MaxItems == 0 || int(request.MaxItems) > w.limits.MaxSearchResults ||
		request.MaxBytes == 0 || int(request.MaxBytes) > w.limits.MaxContextBytes ||
		request.MaxWork == 0 || request.MaxWork > uint64(w.limits.MaxScenarioWork) {
		return LayoutIssueExplainResult{}, workspaceError("LAYOUT_EXPLAIN_LIMIT", "layout explanation bounds are outside configured limits", ErrLimit)
	}
	selector := request.Selector
	if selector.DiagnosticCode == "" && selector.Node == 0 && selector.Key == "" && selector.Instance == "" && selector.Fragment == 0 && selector.Page == 0 {
		return LayoutIssueExplainResult{}, workspaceError("LAYOUT_EXPLAIN_SELECTOR", "an issue, node, instance, fragment, or page selector is required", ErrInvalidQuery)
	}

	opened, revision, plan, err := w.exactOpenRevisionPlan(request)
	if err != nil {
		return LayoutIssueExplainResult{}, err
	}
	compileWork := uint64(len(revision.compiled.Mapping.Nodes)+len(revision.compiled.Mapping.ThemeProperties)) + 1
	if compileWork >= request.MaxWork {
		return LayoutIssueExplainResult{}, workspaceError("LAYOUT_EXPLAIN_LIMIT", "layout explanation work budget is too small", ErrLimit)
	}
	planSelector := document.PaperPlanSelector{
		DiagnosticCode: selector.DiagnosticCode, Node: selector.Node, Key: selector.Key, Instance: selector.Instance,
		Fragment: selector.Fragment, Page: selector.Page, MaxResults: request.MaxItems,
	}
	raw, err := plan.plan.ExplainContext(ctx, []document.PaperPlanSelector{planSelector}, 1, request.MaxBytes, request.MaxWork-compileWork)
	if err != nil {
		return LayoutIssueExplainResult{}, workspaceError("INVALID_LAYOUT_EXPLAIN", "structural layout explanation was rejected", err)
	}
	var explanation layoutengine.LayoutExplanation
	if err := json.Unmarshal(raw.JSON(), &explanation); err != nil || len(explanation.Targets) != 1 {
		return LayoutIssueExplainResult{}, workspaceError("INVALID_LAYOUT_EXPLAIN", "structural layout explanation was not canonical", ErrInvalidQuery)
	}
	target := explanation.Targets[0]
	sanitizeLayoutTarget(&target)
	candidates := layoutCandidates(target)
	resolution := LayoutIssueExact
	if len(candidates) == 0 && target.Selection.Diagnostics.Matches == 0 {
		resolution = LayoutIssueNotFound
	} else if len(candidates) > 1 {
		resolution = LayoutIssueAmbiguous
	}

	result := LayoutIssueExplainResult{
		SchemaVersion: LayoutIssueExplanationSchemaVersion, Plan: snapshotPlan(plan),
		Revision: RevisionIdentity{File: revision.file, Digest: revision.revision, Bytes: len(revision.source), SyntaxNodes: revision.nodes,
			ParseOK: revision.parsed.OK(), CompileOK: revision.parsed.OK() && revision.compiled.OK()},
		CandidateBound: opened.candidate.value.serial != 0, Selector: selector, Resolution: resolution, Candidates: candidates,
	}
	if resolution == LayoutIssueExact {
		result.Layout = &target
		keys := selectedLayoutKeys(selector, candidates, target)
		result.Source = sourceCauses(revision.compiled.Mapping, keys, request.MaxItems)
		result.Styles = styleCauses(revision.compiled.Mapping, keys, request.MaxItems)
	}
	sanitizeLayoutIssueProtocol(&result)
	if err := ctx.Err(); err != nil {
		return LayoutIssueExplainResult{}, err
	}
	if err := finalizeLayoutIssueResult(&result, int(request.MaxBytes)); err != nil {
		return LayoutIssueExplainResult{}, err
	}
	return result, nil
}

func (w *Workspace) exactOpenRevisionPlan(request LayoutIssueExplainRequest) (*openRecord, *revisionRecord, *planRecord, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	opened, err := w.openLocked(request.Open)
	if err != nil {
		return nil, nil, nil, err
	}
	if opened.revision != request.ExpectedRevision || opened.digest != request.ExpectedDigest {
		return nil, nil, nil, workspaceError("REVISION_CONFLICT", "layout explanation preconditions do not match the open revision", ErrRevisionConflict)
	}
	if opened.candidate.value.serial != 0 {
		candidate, candidateErr := w.candidateLocked(opened.candidate)
		if candidateErr != nil {
			return nil, nil, nil, candidateErr
		}
		if candidate.head != opened.revision {
			return nil, nil, nil, workspaceError("REVISION_CONFLICT", "opened candidate has advanced; reopen its exact head", ErrRevisionConflict)
		}
	}
	revision, err := w.revisionLocked(opened.revision)
	if err != nil {
		return nil, nil, nil, err
	}
	plan, err := w.planLocked(request.Plan)
	if err != nil {
		return nil, nil, nil, err
	}
	if plan.revision != opened.revision || plan.digest != opened.digest || plan.partition != opened.partition || plan.disclosure != opened.disclosure {
		return nil, nil, nil, workspaceError("REVISION_CONFLICT", "retained plan does not belong to the exact open revision", ErrRevisionConflict)
	}
	return opened, revision, plan, nil
}

func sanitizeLayoutTarget(target *layoutengine.ExplainLayoutTarget) {
	target.Selector.Key = layoutengine.NodeKey(protocolIdentityOrDigest(string(target.Selector.Key)))
	target.Selector.Instance = layoutengine.InstanceID(protocolIdentityOrDigest(string(target.Selector.Instance)))
	target.Selector.DiagnosticCode = layoutengine.DiagnosticCode(protocolIdentityOrDigest(string(target.Selector.DiagnosticCode)))
	sanitizeSourceIdentity := func(identity *layoutengine.ExplainSourceIdentity) {
		identity.Source.File = redactedOptional(identity.Source.File)
	}
	for index := range target.Fragments {
		sanitizeSourceIdentity(&target.Fragments[index].Source)
	}
	for index := range target.ContinuationFragments {
		sanitizeSourceIdentity(&target.ContinuationFragments[index].Source)
	}
	for index := range target.Lines {
		sanitizeSourceIdentity(&target.Lines[index].Source)
	}
	for index := range target.Commands {
		sanitizeSourceIdentity(&target.Commands[index].Source)
	}
	for index := range target.Glyphs {
		// Core-font codes are the original text bytes; geometry and advances are
		// sufficient to explain layout without disclosing content.
		target.Glyphs[index].Run.Codes = ""
	}
	for index := range target.Diagnostics {
		diagnostic := &target.Diagnostics[index].Diagnostic
		diagnostic.Location.Source.File = redactedOptional(diagnostic.Location.Source.File)
		diagnostic.Location.Scenario = redactedOptional(diagnostic.Location.Scenario)
		diagnostic.Message = "layout issue " + string(diagnostic.Code)
		for evidenceIndex := range diagnostic.Evidence {
			diagnostic.Evidence[evidenceIndex].Value = redactedDigest(diagnostic.Evidence[evidenceIndex].Value)
		}
		for fixIndex := range diagnostic.Fixes {
			if diagnostic.Fixes[fixIndex].Value != "" {
				diagnostic.Fixes[fixIndex].Value = redactedDigest(diagnostic.Fixes[fixIndex].Value)
			}
		}
		for relatedIndex := range diagnostic.Related {
			diagnostic.Related[relatedIndex].Location.Source.File = redactedOptional(diagnostic.Related[relatedIndex].Location.Source.File)
			diagnostic.Related[relatedIndex].Location.Scenario = redactedOptional(diagnostic.Related[relatedIndex].Location.Scenario)
		}
	}
	for index := range target.Semantics {
		target.Semantics[index].Node.Source.File = redactedOptional(target.Semantics[index].Node.Source.File)
		target.Semantics[index].Node.Attributes.ActualText = ""
		target.Semantics[index].Node.Attributes.AlternateText = ""
	}
}

func sanitizeLayoutIssueProtocol(result *LayoutIssueExplainResult) {
	result.Revision.File = redactedOptional(result.Revision.File)
	result.Selector.Key = protocolIdentityOrDigest(result.Selector.Key)
	result.Selector.Instance = protocolIdentityOrDigest(result.Selector.Instance)
	result.Selector.DiagnosticCode = protocolIdentityOrDigest(result.Selector.DiagnosticCode)
	for index := range result.Source {
		result.Source[index].ID = protocolIdentityOrDigest(result.Source[index].ID)
		result.Source[index].InstancePath = protocolIdentityOrDigest(result.Source[index].InstancePath)
		result.Source[index].BindingPath = protocolIdentityOrDigest(result.Source[index].BindingPath)
	}
	for index := range result.Styles {
		style := &result.Styles[index]
		style.Property = protocolIdentityOrDigest(style.Property)
		style.Theme = protocolIdentityOrDigest(style.Theme)
		style.Token = protocolIdentityOrDigest(style.Token)
		style.Value.String = redactedOptional(style.Value.String)
		style.Provenance.Property.File = redactedOptional(style.Provenance.Property.File)
		for step := range style.Provenance.Chain {
			style.Provenance.Chain[step].Theme = protocolIdentityOrDigest(style.Provenance.Chain[step].Theme)
			style.Provenance.Chain[step].Token = protocolIdentityOrDigest(style.Provenance.Chain[step].Token)
			style.Provenance.Chain[step].Source.File = redactedOptional(style.Provenance.Chain[step].Source.File)
			for scope := range style.Provenance.Chain[step].Scope {
				style.Provenance.Chain[step].Scope[scope] = protocolIdentityOrDigest(style.Provenance.Chain[step].Scope[scope])
			}
		}
	}
}

func protocolIdentityOrDigest(value string) string {
	if value == "" || safeProtocolIdentity(value) {
		return value
	}
	return redactedDigest(value)
}

func redactedOptional(value string) string {
	if value == "" {
		return ""
	}
	return redactedDigest(value)
}

func redactedDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func layoutCandidates(target layoutengine.ExplainLayoutTarget) []LayoutIssueCandidate {
	type identity struct {
		node     layoutengine.NodeID
		key      layoutengine.NodeKey
		instance layoutengine.InstanceID
	}
	byIdentity := make(map[identity]*LayoutIssueCandidate)
	add := func(fragment layoutengine.ExplainFragment) {
		id := identity{fragment.Source.Node, fragment.Source.Key, fragment.Source.Instance}
		candidate := byIdentity[id]
		if candidate == nil {
			candidate = &LayoutIssueCandidate{Node: id.node, Key: id.key, Instance: id.instance, Source: fragment.Source.Source}
			byIdentity[id] = candidate
		}
		if len(candidate.Pages) == 0 || candidate.Pages[len(candidate.Pages)-1] != fragment.Page {
			candidate.Pages = append(candidate.Pages, fragment.Page)
		}
		candidate.Fragments = append(candidate.Fragments, fragment.ID)
	}
	for _, fragment := range target.ContinuationFragments {
		add(fragment)
	}
	if len(target.ContinuationFragments) == 0 {
		for _, fragment := range target.Fragments {
			add(fragment)
		}
	}
	result := make([]LayoutIssueCandidate, 0, len(byIdentity))
	for _, candidate := range byIdentity {
		result = append(result, *candidate)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Node != result[j].Node {
			return result[i].Node < result[j].Node
		}
		if result[i].Key != result[j].Key {
			return result[i].Key < result[j].Key
		}
		return result[i].Instance < result[j].Instance
	})
	return result
}

func selectedLayoutKeys(selector LayoutIssueSelector, candidates []LayoutIssueCandidate, target layoutengine.ExplainLayoutTarget) map[string]struct{} {
	keys := make(map[string]struct{})
	if selector.Key != "" {
		keys[strings.TrimPrefix(selector.Key, "@")] = struct{}{}
	}
	for _, candidate := range candidates {
		if candidate.Key != "" {
			keys[strings.TrimPrefix(string(candidate.Key), "@")] = struct{}{}
		}
	}
	for _, diagnostic := range target.Diagnostics {
		if diagnostic.Diagnostic.Location.Key != "" {
			keys[strings.TrimPrefix(string(diagnostic.Diagnostic.Location.Key), "@")] = struct{}{}
		}
	}
	return keys
}

func sourceCauses(mapping papercompile.CompileMapping, keys map[string]struct{}, max uint32) []LayoutSourceCause {
	result := make([]LayoutSourceCause, 0)
	for _, node := range mapping.Nodes {
		if _, found := keys[strings.TrimPrefix(node.ID, "@")]; !found {
			continue
		}
		result = append(result, LayoutSourceCause{ID: node.ID, Kind: node.Kind, Span: node.Span,
			DefinitionSpan: node.DefinitionSpan, InvocationSpan: node.InvocationSpan, InstancePath: node.InstancePath,
			BindingPath: node.BindingPath, BindingSpan: node.BindingSpan, BindingNullable: node.BindingNullable,
			BindingCollection: node.BindingCollection})
		if uint32(len(result)) == max {
			break
		}
	}
	return result
}

func styleCauses(mapping papercompile.CompileMapping, keys map[string]struct{}, max uint32) []LayoutStyleCause {
	result := make([]LayoutStyleCause, 0)
	for _, style := range mapping.ThemeProperties {
		if _, found := keys[strings.TrimPrefix(style.NodeID, "@")]; !found {
			continue
		}
		result = append(result, LayoutStyleCause{Property: style.Property, ConsumerSpan: style.ConsumerSpan, Theme: style.Theme,
			Token: style.Token, Value: style.Value, Provenance: style.Provenance})
		if uint32(len(result)) == max {
			break
		}
	}
	return result
}

func finalizeLayoutIssueResult(result *LayoutIssueExplainResult, maxBytes int) error {
	result.CanonicalHash = ""
	result.EncodedBytes = 0
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(encoded)
	result.CanonicalHash = hex.EncodeToString(digest[:])
	previous := -1
	for previous != result.EncodedBytes {
		previous = result.EncodedBytes
		encoded, err = json.Marshal(result)
		if err != nil {
			return err
		}
		result.EncodedBytes = len(encoded)
	}
	encoded, err = json.Marshal(result)
	if err != nil {
		return err
	}
	if len(encoded) > maxBytes {
		return workspaceError("LAYOUT_EXPLAIN_LIMIT", "layout explanation exceeds its response byte budget", ErrLimit)
	}
	return nil
}
