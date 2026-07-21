// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperlang"
)

var (
	ErrMutationAuthorityNotFound = workspaceError("MUTATION_AUTHORITY_NOT_FOUND", "mutation authority handle is not retained", ErrInvalidHandle)
	ErrMutationAuthorityDenied   = errors.New("paperd: mutation authority denied")
)

// MutationOperation is a closed capability vocabulary for the currently
// implemented typed source mutation tools.
type MutationOperation string

const (
	MutationSetLiteral       MutationOperation = "set_literal"
	MutationSetRichText      MutationOperation = "set_rich_text"
	MutationSetBinding       MutationOperation = "set_binding"
	MutationFillSlot         MutationOperation = "fill_slot"
	MutationApplyFix         MutationOperation = "apply_fix"
	MutationSetBoxProperty   MutationOperation = "set_box_property"
	MutationSetTextProperty  MutationOperation = "set_text_property"
	MutationSetGridTrack     MutationOperation = "set_grid_track"
	MutationSetImageProperty MutationOperation = "set_image_property"
	MutationSetTableProperty MutationOperation = "set_table_property"
	MutationSetPageMargin    MutationOperation = "set_page_margin"
	MutationSetPageSize      MutationOperation = "set_page_size"
	MutationSetCanvasAnchor  MutationOperation = "set_canvas_anchor"
	MutationSetPageRegion    MutationOperation = "set_page_region"
	MutationMoveNode         MutationOperation = "move_node"
	MutationInsertTemplate   MutationOperation = "insert_template"
	MutationCreateScenario   MutationOperation = "create_scenario"
)

func (operation MutationOperation) valid() bool {
	switch operation {
	case MutationSetLiteral, MutationSetRichText, MutationSetBinding, MutationFillSlot, MutationApplyFix,
		MutationSetBoxProperty, MutationSetTextProperty, MutationSetGridTrack, MutationSetImageProperty, MutationSetTableProperty, MutationSetPageMargin, MutationSetPageSize, MutationSetCanvasAnchor, MutationSetPageRegion:
		return true
	case MutationMoveNode, MutationInsertTemplate, MutationCreateScenario:
		return true
	default:
		return false
	}
}

type MutationAuthorityGrant struct {
	Open           OpenHandle
	Actor          string
	Operations     []MutationOperation
	NodeScopes     []string
	ProtectedNodes []string
}

type MutationAuthoritySnapshot struct {
	Handle           MutationAuthorityHandle `json:"-"`
	Actor            string                  `json:"actor"`
	Operations       []MutationOperation     `json:"operations"`
	NodeScopes       []string                `json:"node_scopes,omitempty"`
	ProtectedNodes   []string                `json:"protected_nodes,omitempty"`
	Capability       string                  `json:"capability"`
	DisclosureDomain DisclosureDomain        `json:"disclosure_domain"`
	ExpiresAt        time.Time               `json:"expires_at"`
}

type AuthorizationEffect struct {
	Node   string `json:"node"`
	Reason string `json:"reason"`
}

type MutationAuthorizationEvidence struct {
	Explicit         bool                  `json:"explicit"`
	Actor            string                `json:"actor"`
	Operation        MutationOperation     `json:"operation"`
	DirectTargets    []string              `json:"direct_targets"`
	Effects          []AuthorizationEffect `json:"effects"`
	ProtectedEffects []string              `json:"protected_effects,omitempty"`
	Allowed          bool                  `json:"allowed"`
	Reason           string                `json:"reason"`
}

// AuthorizationAuditEntry is a bounded in-memory decision record. It is not
// the chained/signed audit ledger required by the later authorization stage.
type AuthorizationAuditEntry struct {
	Sequence         uint64                `json:"sequence"`
	At               time.Time             `json:"at"`
	Actor            string                `json:"actor"`
	Operation        MutationOperation     `json:"operation"`
	CandidateSerial  uint64                `json:"candidate_serial"`
	SourceRevision   string                `json:"source_revision"`
	Allowed          bool                  `json:"allowed"`
	Reason           string                `json:"reason"`
	Effects          []AuthorizationEffect `json:"effects"`
	ProtectedEffects []string              `json:"protected_effects,omitempty"`
}

func normalizeProtectedNodeIDs(values []string, limit int) (map[string]struct{}, error) {
	if len(values) > limit {
		return nil, workspaceError("AUTHORIZATION_EFFECT_LIMIT", "protected-node count exceeds configured bounds", ErrLimit)
	}
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validAuthorityNodeID(value) {
			return nil, workspaceError("INVALID_PROTECTED_NODE", "protected nodes must be bounded readable @ids", ErrInvalidQuery)
		}
		result[value] = struct{}{}
	}
	return result, nil
}

func validAuthorityNodeID(value string) bool {
	return value != "" && len(value) <= MaxQueryBytesHard && value[0] == '@' && utf8.ValidString(value) && !strings.ContainsAny(value, " \t\r\n")
}

// GrantMutationAuthority is the trust-boundary hook used after an outer actor
// has been authenticated and authorized. The resulting capability is bound to
// one exact edit open/candidate and cannot be used for another workspace.
func (w *Workspace) GrantMutationAuthority(request MutationAuthorityGrant) (MutationAuthoritySnapshot, error) {
	if w == nil {
		return MutationAuthoritySnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if request.Actor == "" || len(request.Actor) > w.limits.MaxQueryBytes || !utf8.ValidString(request.Actor) || strings.TrimSpace(request.Actor) != request.Actor {
		return MutationAuthoritySnapshot{}, workspaceError("INVALID_ACTOR", "actor identity must be bounded valid UTF-8", ErrInvalidQuery)
	}
	operations := make(map[MutationOperation]struct{}, len(request.Operations))
	for _, operation := range request.Operations {
		if !operation.valid() {
			return MutationAuthoritySnapshot{}, workspaceError("INVALID_AUTHORITY_OPERATION", "authority contains an unsupported operation", ErrInvalidQuery)
		}
		operations[operation] = struct{}{}
	}
	if len(operations) == 0 {
		return MutationAuthoritySnapshot{}, workspaceError("INVALID_AUTHORITY_OPERATION", "authority requires at least one operation", ErrInvalidQuery)
	}
	nodeScopes, err := normalizeProtectedNodeIDs(request.NodeScopes, w.limits.MaxAuthorizationEffects)
	if err != nil {
		return MutationAuthoritySnapshot{}, err
	}
	protected, err := normalizeProtectedNodeIDs(request.ProtectedNodes, w.limits.MaxAuthorizationEffects)
	if err != nil {
		return MutationAuthoritySnapshot{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	opened, err := w.openLocked(request.Open)
	if err != nil {
		return MutationAuthoritySnapshot{}, err
	}
	if opened.mode != CapabilityEdit || opened.candidate.value.serial == 0 {
		return MutationAuthoritySnapshot{}, workspaceError("CAPABILITY_DENIED", "authority requires an edit-capable candidate open", ErrMutationAuthorityDenied)
	}
	revision, err := w.revisionLocked(opened.revision)
	if err != nil {
		return MutationAuthoritySnapshot{}, err
	}
	for node := range nodeScopes {
		if len(sourceNodesByID(revision.parsed.AST.Root, node)) != 1 {
			return MutationAuthoritySnapshot{}, workspaceError("INVALID_AUTHORITY_SCOPE", "authority node scope is absent or ambiguous", ErrInvalidQuery)
		}
	}
	for node := range protected {
		if _, configured := w.protectedNodeIDs[node]; !configured {
			return MutationAuthoritySnapshot{}, workspaceError("INVALID_PROTECTED_GRANT", "authority cannot grant an unconfigured protected node", ErrMutationAuthorityDenied)
		}
	}
	if len(w.mutationAuthorities) >= w.limits.MaxMutationAuthorities {
		return MutationAuthoritySnapshot{}, workspaceError("AUTHORITY_LIMIT", "mutation authority capacity is exhausted", ErrLimit)
	}
	w.nextMutationAuthority++
	handle := MutationAuthorityHandle{value: w.newHandle(handleMutationAuthority, capabilityAuthorize, w.nextMutationAuthority)}
	record := &mutationAuthorityRecord{handle: handle, open: request.Open, candidate: opened.candidate, actor: request.Actor,
		operations: operations, nodeScopes: nodeScopes, protectedNodes: protected,
		expires: w.expiresAt(w.handleTTL), disclosure: w.disclosureDomain, partition: w.partition}
	w.mutationAuthorities[w.nextMutationAuthority] = record
	return mutationAuthoritySnapshot(record), nil
}

func (w *Workspace) OpenMutationAuthority(handle MutationAuthorityHandle) (MutationAuthoritySnapshot, error) {
	if w == nil {
		return MutationAuthoritySnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	record, err := w.mutationAuthorityLocked(handle)
	if err != nil {
		return MutationAuthoritySnapshot{}, err
	}
	return mutationAuthoritySnapshot(record), nil
}

func (w *Workspace) mutationAuthorityLocked(handle MutationAuthorityHandle) (*mutationAuthorityRecord, error) {
	if err := w.validateHandle(handle.value, handleMutationAuthority, capabilityAuthorize, false); err != nil {
		return nil, err
	}
	record := w.mutationAuthorities[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrMutationAuthorityNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func mutationAuthoritySnapshot(record *mutationAuthorityRecord) MutationAuthoritySnapshot {
	result := MutationAuthoritySnapshot{Handle: record.handle, Actor: record.actor, Capability: "authorize_mutation", DisclosureDomain: record.disclosure, ExpiresAt: record.expires}
	for operation := range record.operations {
		result.Operations = append(result.Operations, operation)
	}
	for node := range record.nodeScopes {
		result.NodeScopes = append(result.NodeScopes, node)
	}
	for node := range record.protectedNodes {
		result.ProtectedNodes = append(result.ProtectedNodes, node)
	}
	sort.Slice(result.Operations, func(i, j int) bool { return result.Operations[i] < result.Operations[j] })
	sort.Strings(result.NodeScopes)
	sort.Strings(result.ProtectedNodes)
	return result
}

func (w *Workspace) authorizeMutation(guard PaperMutationGuard, operation MutationOperation, revision *revisionRecord, direct []string) (MutationAuthorizationEvidence, error) {
	if !operation.valid() {
		return MutationAuthorizationEvidence{}, workspaceError("INVALID_AUTHORITY_OPERATION", "mutation operation is outside the closed authorization vocabulary", ErrInvalidQuery)
	}
	effects, err := computeAuthorizationEffects(revision.parsed.AST.Root, direct, w.limits.MaxAuthorizationEffects)
	if err != nil {
		return MutationAuthorizationEvidence{}, err
	}
	protected := protectedEffects(revision.parsed.AST.Root, effects, w.protectedNodeIDs)
	evidence := MutationAuthorizationEvidence{Operation: operation, DirectTargets: canonicalStrings(direct), Effects: effects, ProtectedEffects: protected}
	if guard.Authority.value.serial == 0 && !w.requireMutationAuthority {
		evidence.Actor, evidence.Allowed, evidence.Reason = "compatibility-open", true, "legacy workspace policy permits edit-open authority"
		w.recordAuthorizationAudit(evidence, guard.Candidate, string(revision.revision))
		return evidence, nil
	}
	w.mu.RLock()
	record, lookupErr := w.mutationAuthorityLocked(guard.Authority)
	if lookupErr == nil && (record.open != guard.Open || record.candidate != guard.Candidate) {
		lookupErr = workspaceError("AUTHORITY_BINDING", "mutation authority does not bind the exact open and candidate", ErrMutationAuthorityDenied)
	}
	if lookupErr == nil {
		evidence.Explicit, evidence.Actor = true, record.actor
		if _, allowed := record.operations[operation]; !allowed {
			lookupErr = workspaceError("AUTHORITY_OPERATION_DENIED", "mutation operation is outside the granted capability", ErrMutationAuthorityDenied)
		}
		if lookupErr == nil && len(record.nodeScopes) != 0 {
			for _, effect := range effects {
				if !effectWithinScopes(revision.parsed.AST.Root, effect.Node, record.nodeScopes) {
					lookupErr = workspaceError("AUTHORITY_SCOPE_DENIED", "transitive semantic effects exceed granted node scopes", ErrMutationAuthorityDenied)
					break
				}
			}
		}
		if lookupErr == nil {
			for _, node := range protected {
				if _, allowed := record.protectedNodes[node]; !allowed {
					lookupErr = workspaceError("PROTECTED_NODE_DENIED", "mutation affects a protected node without an explicit grant", ErrMutationAuthorityDenied)
					break
				}
			}
		}
	}
	w.mu.RUnlock()
	if lookupErr != nil {
		if evidence.Actor == "" {
			evidence.Actor = "unavailable"
		}
		evidence.Allowed, evidence.Reason = false, errorCodeForAudit(lookupErr)
		w.recordAuthorizationAudit(evidence, guard.Candidate, string(revision.revision))
		if errors.Is(lookupErr, ErrMutationAuthorityNotFound) || errors.Is(lookupErr, ErrInvalidHandle) {
			return evidence, workspaceError("AUTHORITY_REQUIRED", "an explicit live mutation authority is required", ErrMutationAuthorityDenied)
		}
		return evidence, lookupErr
	}
	evidence.Allowed, evidence.Reason = true, "operation and complete effect set are authorized"
	w.recordAuthorizationAudit(evidence, guard.Candidate, string(revision.revision))
	return evidence, nil
}

func (w *Workspace) recordAuthorizationAudit(evidence MutationAuthorizationEvidence, candidate CandidateHandle, revision string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextAuthorizationAudit++
	entry := AuthorizationAuditEntry{Sequence: w.nextAuthorizationAudit, At: w.now(), Actor: evidence.Actor, Operation: evidence.Operation, CandidateSerial: candidate.value.serial, SourceRevision: revision, Allowed: evidence.Allowed, Reason: evidence.Reason, Effects: append([]AuthorizationEffect(nil), evidence.Effects...), ProtectedEffects: append([]string(nil), evidence.ProtectedEffects...)}
	for len(w.authorizationAudit) >= w.limits.MaxAuthorizationAudit {
		w.authorizationAudit = w.authorizationAudit[1:]
	}
	w.authorizationAudit = append(w.authorizationAudit, entry)
}

func (w *Workspace) AuthorizationAudit(limit int) ([]AuthorizationAuditEntry, error) {
	if w == nil {
		return nil, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if limit <= 0 || limit > w.limits.MaxAuthorizationAudit {
		return nil, workspaceError("AUDIT_LIMIT", "audit limit is outside configured bounds", ErrLimit)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	start := 0
	if len(w.authorizationAudit) > limit {
		start = len(w.authorizationAudit) - limit
	}
	result := make([]AuthorizationAuditEntry, len(w.authorizationAudit)-start)
	for i, entry := range w.authorizationAudit[start:] {
		entry.Effects = append([]AuthorizationEffect(nil), entry.Effects...)
		entry.ProtectedEffects = append([]string(nil), entry.ProtectedEffects...)
		result[i] = entry
	}
	return result, nil
}

func computeAuthorizationEffects(root *paperlang.Node, direct []string, limit int) ([]AuthorizationEffect, error) {
	byID := make(map[string]*paperlang.Node)
	var all []*paperlang.Node
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil {
			return
		}
		all = append(all, node)
		if node.ID != "" {
			byID[node.ID] = node
		}
		for _, member := range node.Members {
			walk(member.Node)
		}
	}
	walk(root)
	reasons := make(map[string]string)
	add := func(node, reason string) error {
		if node == "" {
			return nil
		}
		if _, exists := reasons[node]; !exists {
			if len(reasons) >= limit {
				return workspaceError("AUTHORIZATION_EFFECT_LIMIT", "transitive effect count exceeds configured bounds", ErrLimit)
			}
			reasons[node] = reason
		}
		return nil
	}
	var addSubtree func(*paperlang.Node, string) error
	addSubtree = func(node *paperlang.Node, reason string) error {
		if node == nil {
			return nil
		}
		if err := add(node.ID, reason); err != nil {
			return err
		}
		for _, member := range node.Members {
			if err := addSubtree(member.Node, reason); err != nil {
				return err
			}
		}
		return nil
	}
	for _, target := range canonicalStrings(direct) {
		node := byID[target]
		if node == nil {
			if err := add(target, "prospective_descendant"); err != nil {
				return nil, err
			}
			continue
		}
		if err := add(target, "direct"); err != nil {
			return nil, err
		}
		if node.Kind == paperlang.NodeComponent || node.Kind == paperlang.NodeSlot || node.Kind == paperlang.NodeTheme || node.Kind == paperlang.NodeScope {
			if err := addSubtree(node, "owned_descendant"); err != nil {
				return nil, err
			}
		}
		component := ancestorComponent(root, node.ID)
		if node.Kind == paperlang.NodeComponent {
			component = node.ID
		}
		if component != "" {
			if component != target {
				if err := add(component, "governing_component"); err != nil {
					return nil, err
				}
			}
			for _, candidate := range all {
				if candidate.Kind == paperlang.NodeUse && nodeStringProperty(candidate, "component") == component {
					if err := add(candidate.ID, "component_instance"); err != nil {
						return nil, err
					}
				}
			}
		}
	}
	result := make([]AuthorizationEffect, 0, len(reasons))
	for node, reason := range reasons {
		result = append(result, AuthorizationEffect{Node: node, Reason: reason})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Node != result[j].Node {
			return result[i].Node < result[j].Node
		}
		return result[i].Reason < result[j].Reason
	})
	return result, nil
}

func canonicalStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func nodeStringProperty(node *paperlang.Node, name string) string {
	for _, member := range node.Members {
		if member.Property != nil && member.Property.Name == name && member.Property.Value.StringValue != nil {
			return *member.Property.Value.StringValue
		}
	}
	return ""
}
func ancestorComponent(root *paperlang.Node, target string) string {
	var result string
	var visit func(*paperlang.Node, string)
	visit = func(node *paperlang.Node, component string) {
		if node == nil || result != "" {
			return
		}
		if node.Kind == paperlang.NodeComponent {
			component = node.ID
		}
		if node.ID == target {
			result = component
			return
		}
		for _, member := range node.Members {
			visit(member.Node, component)
		}
	}
	visit(root, "")
	return result
}
func protectedEffects(root *paperlang.Node, effects []AuthorizationEffect, protected map[string]struct{}) []string {
	found := make(map[string]struct{})
	targets := make(map[string]struct{}, len(effects))
	for _, effect := range effects {
		targets[effect.Node] = struct{}{}
	}
	var visit func(*paperlang.Node, []string)
	visit = func(node *paperlang.Node, ancestors []string) {
		if node == nil {
			return
		}
		next := ancestors
		if _, ok := protected[node.ID]; ok {
			next = append(append([]string(nil), ancestors...), node.ID)
		}
		if _, ok := targets[node.ID]; ok {
			for _, value := range next {
				found[value] = struct{}{}
			}
		}
		for _, member := range node.Members {
			visit(member.Node, next)
		}
	}
	visit(root, nil)
	result := make([]string, 0, len(found))
	for value := range found {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
func effectWithinScopes(root *paperlang.Node, target string, scopes map[string]struct{}) bool {
	for scope := range scopes {
		if strings.HasPrefix(target, scope+"/") {
			return true
		}
	}
	allowed := false
	var visit func(*paperlang.Node, bool)
	visit = func(node *paperlang.Node, inside bool) {
		if node == nil || allowed {
			return
		}
		if _, ok := scopes[node.ID]; ok {
			inside = true
		}
		if node.ID == target {
			allowed = inside
			return
		}
		for _, member := range node.Members {
			visit(member.Node, inside)
		}
	}
	visit(root, false)
	return allowed
}
func errorCodeForAudit(err error) string {
	var typed *Error
	if errors.As(err, &typed) {
		return typed.Code
	}
	return "AUTHORITY_DENIED"
}
