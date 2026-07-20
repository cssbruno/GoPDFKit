// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/cssbruno/paperrune/internal/paperscenario"
)

var ErrScenarioCandidateNotFound = workspaceError("SCENARIO_CANDIDATE_NOT_FOUND", "scenario candidate handle is not retained", ErrCandidateNotFound)

type ScenarioOperationKind string

const (
	ScenarioOperationSetValue        ScenarioOperationKind = "set_value"
	ScenarioOperationDeleteValue     ScenarioOperationKind = "delete_value"
	ScenarioOperationReplaceListItem ScenarioOperationKind = "replace_list_item"
)

// ScenarioOperation edits one fully resolved fixture. Paths are dotted object
// field names. A keyed-list replacement changes only the value belonging to an
// existing stable key; it cannot create, delete, reorder, or rename list items.
type ScenarioOperation struct {
	Kind     ScenarioOperationKind `json:"kind"`
	Scenario string                `json:"scenario"`
	Path     string                `json:"path"`
	Key      string                `json:"key,omitempty"`
	Value    paperscenario.Value   `json:"value,omitempty"`
}

// ScenarioApplyRequest binds an edit to one exact candidate head and content
// digest. Idempotency keys are scoped to the candidate.
type ScenarioApplyRequest struct {
	Candidate      ScenarioCandidateHandle
	ExpectedHead   ScenarioRevisionHandle
	ExpectedDigest string
	IdempotencyKey string
	Operations     []ScenarioOperation
}

// ScenarioCandidateSnapshot is deliberately redacted. The head handle and
// digest identify exact state without disclosing retained fixture values.
type ScenarioCandidateSnapshot struct {
	Handle           ScenarioCandidateHandle `json:"-"`
	Head             ScenarioRevisionHandle  `json:"-"`
	HeadDigest       string                  `json:"head_digest"`
	FixtureCount     int                     `json:"fixture_count"`
	Capability       CapabilityMode          `json:"capability"`
	DisclosureDomain DisclosureDomain        `json:"disclosure_domain"`
	ExpiresAt        time.Time               `json:"expires_at"`
}

// ScenarioApplyResult contains only detached, redacted snapshots. In
// particular it never echoes operations or authored fixture values.
type ScenarioApplyResult struct {
	Candidate      ScenarioCandidateSnapshot `json:"candidate"`
	Revision       ScenarioRevisionSnapshot  `json:"revision"`
	Applied        int                       `json:"applied"`
	IdempotencyKey string                    `json:"idempotency_key"`
}

type scenarioIdempotencyRecord struct {
	fingerprint string
	result      ScenarioApplyResult
}

type scenarioEditBudget struct {
	used  uint64
	limit uint64
}

func (b *scenarioEditBudget) spend(amount uint64) error {
	if amount > b.limit-b.used {
		return workspaceError("SCENARIO_WORK_LIMIT", "scenario edit exceeds the configured work limit", ErrLimit)
	}
	b.used += amount
	return nil
}

// NewScenarioCandidate creates a mutable scenario head without exposing or
// copying values into its public snapshot.
func (w *Workspace) NewScenarioCandidate(base ScenarioRevisionHandle) (ScenarioCandidateSnapshot, error) {
	if w == nil {
		return ScenarioCandidateSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	revision, err := w.scenarioRevisionLocked(base)
	if err != nil {
		return ScenarioCandidateSnapshot{}, err
	}
	if len(w.scenarioCandidates) >= w.limits.MaxScenarioCandidates {
		return ScenarioCandidateSnapshot{}, workspaceError("SCENARIO_CANDIDATE_LIMIT", "workspace scenario candidate capacity is exhausted", ErrLimit)
	}
	w.nextScenarioCandidate++
	handle := ScenarioCandidateHandle{value: w.newHandle(handleScenarioCandidate, capabilityEdit, w.nextScenarioCandidate)}
	record := &scenarioCandidateRecord{
		handle:      handle,
		head:        base,
		idempotency: make(map[string]scenarioIdempotencyRecord),
		expires:     w.expiresAt(w.handleTTL),
		disclosure:  w.disclosureDomain,
		partition:   w.partition,
	}
	w.scenarioCandidates[w.nextScenarioCandidate] = record
	return scenarioCandidateSnapshot(record, revision), nil
}

func (w *Workspace) ScenarioCandidate(handle ScenarioCandidateHandle) (ScenarioCandidateSnapshot, error) {
	if w == nil {
		return ScenarioCandidateSnapshot{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	candidate, err := w.scenarioCandidateLocked(handle)
	if err != nil {
		return ScenarioCandidateSnapshot{}, err
	}
	revision, err := w.scenarioRevisionLocked(candidate.head)
	if err != nil {
		return ScenarioCandidateSnapshot{}, err
	}
	return scenarioCandidateSnapshot(candidate, revision), nil
}

// ApplyScenario prepares an immutable resolved scenario revision outside the
// workspace lock, then publishes it with a candidate-head compare-and-swap.
func (w *Workspace) ApplyScenario(request ScenarioApplyRequest) (ScenarioApplyResult, error) {
	if w == nil {
		return ScenarioApplyResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	operations, budget, err := w.validateScenarioApplyRequest(request)
	if err != nil {
		return ScenarioApplyResult{}, err
	}
	fingerprint := scenarioRequestFingerprint(request, operations)

	w.mu.RLock()
	candidate, err := w.scenarioCandidateLocked(request.Candidate)
	if err != nil {
		w.mu.RUnlock()
		return ScenarioApplyResult{}, err
	}
	if cached, exists := candidate.idempotency[request.IdempotencyKey]; exists {
		w.mu.RUnlock()
		return scenarioCachedResult(cached, fingerprint)
	}
	base, err := w.scenarioRevisionLocked(request.ExpectedHead)
	if err != nil {
		w.mu.RUnlock()
		return ScenarioApplyResult{}, err
	}
	if candidate.head != request.ExpectedHead {
		w.mu.RUnlock()
		return ScenarioApplyResult{}, scenarioRevisionConflict("scenario candidate head changed")
	}
	if base.digest != request.ExpectedDigest {
		w.mu.RUnlock()
		return ScenarioApplyResult{}, scenarioRevisionConflict("exact scenario digest does not match the candidate head")
	}
	fixtures := cloneScenarioFixtures(base.fixtures)
	w.mu.RUnlock()

	for i := range operations {
		if err := applyScenarioOperation(fixtures, operations[i], budget); err != nil {
			return ScenarioApplyResult{}, err
		}
	}
	prepared, err := w.prepareScenarioEdit(fixtures, budget)
	if err != nil {
		return ScenarioApplyResult{}, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.pruneExpiredHandlesLocked(w.now())
	candidate, err = w.scenarioCandidateLocked(request.Candidate)
	if err != nil {
		return ScenarioApplyResult{}, err
	}
	if cached, exists := candidate.idempotency[request.IdempotencyKey]; exists {
		return scenarioCachedResult(cached, fingerprint)
	}
	if candidate.head != request.ExpectedHead {
		return ScenarioApplyResult{}, scenarioRevisionConflict("scenario candidate head changed")
	}
	if len(w.scenarioRevisions) >= w.limits.MaxScenarioRevisions {
		return ScenarioApplyResult{}, workspaceError("SCENARIO_REVISION_LIMIT", "workspace scenario revision capacity is exhausted", ErrLimit)
	}
	w.nextScenarioRevision++
	prepared.handle = ScenarioRevisionHandle{value: w.newHandle(handleScenarioRevision, capabilityRead, w.nextScenarioRevision)}
	prepared.expires = w.expiresAt(w.handleTTL)
	prepared.disclosure = w.disclosureDomain
	prepared.partition = w.partition
	w.scenarioRevisions[w.nextScenarioRevision] = prepared
	candidate.head = prepared.handle
	result := ScenarioApplyResult{
		Candidate:      scenarioCandidateSnapshot(candidate, prepared),
		Revision:       scenarioSnapshot(prepared),
		Applied:        len(operations),
		IdempotencyKey: request.IdempotencyKey,
	}
	candidate.idempotency[request.IdempotencyKey] = scenarioIdempotencyRecord{fingerprint: fingerprint, result: cloneScenarioApplyResult(result)}
	return cloneScenarioApplyResult(result), nil
}

func (w *Workspace) validateScenarioApplyRequest(request ScenarioApplyRequest) ([]ScenarioOperation, *scenarioEditBudget, error) {
	if request.IdempotencyKey == "" || !utf8.ValidString(request.IdempotencyKey) || len(request.IdempotencyKey) > w.limits.MaxQueryBytes {
		return nil, nil, workspaceError("INVALID_IDEMPOTENCY_KEY", "idempotency key must be valid UTF-8 and within the configured byte limit", ErrInvalidQuery)
	}
	if len(request.Operations) == 0 || len(request.Operations) > w.limits.MaxScenarioOperations {
		return nil, nil, workspaceError("SCENARIO_OPERATION_LIMIT", "scenario operation count is outside the configured bounds", ErrLimit)
	}
	if len(request.ExpectedDigest) != sha256.Size*2 {
		return nil, nil, scenarioRevisionConflict("expected scenario digest is malformed")
	}
	if _, err := hex.DecodeString(request.ExpectedDigest); err != nil {
		return nil, nil, scenarioRevisionConflict("expected scenario digest is malformed")
	}

	budget := &scenarioEditBudget{limit: uint64(w.limits.MaxScenarioWork)} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	operations := make([]ScenarioOperation, len(request.Operations))
	valueNodes := 0
	for i, operation := range request.Operations {
		if !validScenarioName(operation.Scenario) {
			return nil, nil, workspaceError("INVALID_SCENARIO_OPERATION", "operation has an invalid scenario name", paperscenario.ErrInvalid)
		}
		parts, err := validateScenarioPath(operation.Path, w.limits.MaxScenarioPathBytes)
		if err != nil {
			return nil, nil, err
		}
		if err := budget.spend(1 + uint64(len(operation.Scenario)+len(operation.Path)+len(operation.Key)+len(parts))); err != nil {
			return nil, nil, err
		}
		switch operation.Kind {
		case ScenarioOperationSetValue:
			if operation.Key != "" {
				return nil, nil, workspaceError("INVALID_SCENARIO_OPERATION", "set-value operation cannot include a list key", paperscenario.ErrInvalid)
			}
		case ScenarioOperationDeleteValue:
			if operation.Key != "" || !emptyScenarioValue(operation.Value) {
				return nil, nil, workspaceError("INVALID_SCENARIO_OPERATION", "delete-value operation cannot include a key or value", paperscenario.ErrInvalid)
			}
		case ScenarioOperationReplaceListItem:
			if !validScenarioName(operation.Key) {
				return nil, nil, workspaceError("INVALID_SCENARIO_OPERATION", "replace-list-item operation requires a valid stable key", paperscenario.ErrInvalid)
			}
		default:
			return nil, nil, workspaceError("INVALID_SCENARIO_OPERATION", "operation kind is unsupported", paperscenario.ErrInvalid)
		}
		if operation.Kind != ScenarioOperationDeleteValue {
			if err := validateScenarioValue(operation.Value, 1, &valueNodes, w.limits.MaxScenarioValueNodes, budget); err != nil {
				return nil, nil, err
			}
		}
		operations[i] = ScenarioOperation{
			Kind: operation.Kind, Scenario: operation.Scenario, Path: operation.Path,
			Key: operation.Key, Value: cloneScenarioValue(operation.Value),
		}
	}
	return operations, budget, nil
}

func validateScenarioValue(value paperscenario.Value, depth int, nodes *int, maxNodes int, budget *scenarioEditBudget) error {
	if depth > 64 {
		return workspaceError("SCENARIO_VALUE_LIMIT", "scenario value exceeds the maximum depth", ErrLimit)
	}
	*nodes++
	if *nodes > maxNodes {
		return workspaceError("SCENARIO_VALUE_LIMIT", "scenario values exceed the configured node limit", ErrLimit)
	}
	if err := budget.spend(1 + uint64(len(value.String)+len(value.Number))); err != nil {
		return err
	}
	for i := range value.Object {
		if err := budget.spend(1 + uint64(len(value.Object[i].Name))); err != nil {
			return err
		}
		if err := validateScenarioValue(value.Object[i].Value, depth+1, nodes, maxNodes, budget); err != nil {
			return err
		}
	}
	for i := range value.List {
		if err := budget.spend(1 + uint64(len(value.List[i].Key))); err != nil {
			return err
		}
		if err := validateScenarioValue(value.List[i].Value, depth+1, nodes, maxNodes, budget); err != nil {
			return err
		}
	}
	return nil
}

func validateScenarioPath(path string, maxBytes int) ([]string, error) {
	if path == "" || len(path) > maxBytes {
		return nil, workspaceError("SCENARIO_PATH_LIMIT", "scenario path is empty or exceeds the configured byte limit", ErrLimit)
	}
	parts := splitScenarioPath(path)
	for _, part := range parts {
		if !validScenarioName(part) {
			return nil, workspaceError("INVALID_SCENARIO_PATH", "scenario path contains an invalid field name", paperscenario.ErrInvalid)
		}
	}
	return parts, nil
}

func splitScenarioPath(path string) []string {
	parts := make([]string, 0, 4)
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			parts = append(parts, path[start:i])
			start = i + 1
		}
	}
	return append(parts, path[start:])
}

func validScenarioName(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for i, r := range value {
		if r != '_' && r != '-' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func emptyScenarioValue(value paperscenario.Value) bool {
	return value.Kind == "" && value.String == "" && value.Number == "" && !value.Bool && len(value.Object) == 0 && len(value.List) == 0
}

func applyScenarioOperation(fixtures []paperscenario.Fixture, operation ScenarioOperation, budget *scenarioEditBudget) error {
	fixtureIndex := -1
	for i := range fixtures {
		if err := budget.spend(1); err != nil {
			return err
		}
		if fixtures[i].Name == operation.Scenario {
			fixtureIndex = i
			break
		}
	}
	if fixtureIndex < 0 {
		return workspaceError("SCENARIO_EDIT_REJECTED", "scenario operation targets a missing fixture", paperscenario.ErrInvalid)
	}
	parts := splitScenarioPath(operation.Path)
	switch operation.Kind {
	case ScenarioOperationSetValue:
		return setScenarioValue(&fixtures[fixtureIndex].Values, parts, operation.Value, budget)
	case ScenarioOperationDeleteValue:
		return deleteScenarioValue(&fixtures[fixtureIndex].Values, parts, budget)
	case ScenarioOperationReplaceListItem:
		return replaceScenarioListItem(&fixtures[fixtureIndex].Values, parts, operation.Key, operation.Value, budget)
	default:
		return workspaceError("INVALID_SCENARIO_OPERATION", "operation kind is unsupported", paperscenario.ErrInvalid)
	}
}

func setScenarioValue(fields *[]paperscenario.Field, parts []string, value paperscenario.Value, budget *scenarioEditBudget) error {
	parent, err := scenarioParentFields(fields, parts, budget)
	if err != nil {
		return err
	}
	name := parts[len(parts)-1]
	index, err := scenarioFieldIndex(*parent, name, budget)
	if err != nil {
		return err
	}
	if index >= 0 {
		(*parent)[index].Value = cloneScenarioValue(value)
		return nil
	}
	*parent = append(*parent, paperscenario.Field{Name: name, Value: cloneScenarioValue(value)})
	sort.Slice(*parent, func(i, j int) bool { return (*parent)[i].Name < (*parent)[j].Name })
	return nil
}

func deleteScenarioValue(fields *[]paperscenario.Field, parts []string, budget *scenarioEditBudget) error {
	parent, err := scenarioParentFields(fields, parts, budget)
	if err != nil {
		return err
	}
	index, err := scenarioFieldIndex(*parent, parts[len(parts)-1], budget)
	if err != nil {
		return err
	}
	if index < 0 {
		return workspaceError("SCENARIO_EDIT_REJECTED", "delete operation targets a missing value", paperscenario.ErrInvalid)
	}
	copy((*parent)[index:], (*parent)[index+1:])
	*parent = (*parent)[:len(*parent)-1]
	return nil
}

func replaceScenarioListItem(fields *[]paperscenario.Field, parts []string, key string, value paperscenario.Value, budget *scenarioEditBudget) error {
	target, err := scenarioValueAt(fields, parts, budget)
	if err != nil {
		return err
	}
	if target.Kind != paperscenario.List {
		return workspaceError("SCENARIO_EDIT_REJECTED", "replace-list-item operation targets a non-list value", paperscenario.ErrInvalid)
	}
	for i := range target.List {
		if err := budget.spend(1); err != nil {
			return err
		}
		if target.List[i].Key == key {
			target.List[i].Value = cloneScenarioValue(value)
			return nil
		}
	}
	return workspaceError("SCENARIO_EDIT_REJECTED", "replace-list-item operation targets a missing stable key", paperscenario.ErrInvalid)
}

func scenarioParentFields(fields *[]paperscenario.Field, parts []string, budget *scenarioEditBudget) (*[]paperscenario.Field, error) {
	current := fields
	for _, part := range parts[:len(parts)-1] {
		index, err := scenarioFieldIndex(*current, part, budget)
		if err != nil {
			return nil, err
		}
		if index < 0 || (*current)[index].Value.Kind != paperscenario.Object {
			return nil, workspaceError("SCENARIO_EDIT_REJECTED", "scenario path parent is missing or is not an object", paperscenario.ErrInvalid)
		}
		current = &(*current)[index].Value.Object
	}
	return current, nil
}

func scenarioValueAt(fields *[]paperscenario.Field, parts []string, budget *scenarioEditBudget) (*paperscenario.Value, error) {
	parent, err := scenarioParentFields(fields, parts, budget)
	if err != nil {
		return nil, err
	}
	index, err := scenarioFieldIndex(*parent, parts[len(parts)-1], budget)
	if err != nil {
		return nil, err
	}
	if index < 0 {
		return nil, workspaceError("SCENARIO_EDIT_REJECTED", "scenario path targets a missing value", paperscenario.ErrInvalid)
	}
	return &(*parent)[index].Value, nil
}

func scenarioFieldIndex(fields []paperscenario.Field, name string, budget *scenarioEditBudget) (int, error) {
	for i := range fields {
		if err := budget.spend(1); err != nil {
			return -1, err
		}
		if fields[i].Name == name {
			return i, nil
		}
	}
	return -1, nil
}

func (w *Workspace) prepareScenarioEdit(fixtures []paperscenario.Fixture, budget *scenarioEditBudget) (*scenarioRevisionRecord, error) {
	remainingWork := budget.limit - budget.used
	if remainingWork == 0 {
		return nil, workspaceError("SCENARIO_WORK_LIMIT", "scenario edit exceeds the configured work limit", ErrLimit)
	}
	input := make([]paperscenario.Scenario, len(fixtures))
	for i := range fixtures {
		input[i] = paperscenario.Scenario{Name: fixtures[i].Name, Locale: fixtures[i].Locale, Values: cloneScenarioFields(fixtures[i].Values)}
	}
	limits := paperscenario.DefaultLimits()
	limits.MaxNodes = uint32(w.limits.MaxScenarioValueNodes)    // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	limits.MaxPathBytes = uint32(w.limits.MaxScenarioPathBytes) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	limits.MaxWork = remainingWork
	resolved, err := paperscenario.Resolve(input, limits)
	if err != nil {
		if errors.Is(err, paperscenario.ErrLimit) {
			return nil, workspaceError("SCENARIO_EDIT_LIMIT", "resolved scenario revision exceeds a configured limit", ErrLimit)
		}
		return nil, workspaceError("INVALID_SCENARIO_EDIT", "scenario edit would produce an invalid fixture revision", err)
	}
	return &scenarioRevisionRecord{fixtures: resolved, digest: scenarioDigest(resolved)}, nil
}

func (w *Workspace) scenarioCandidateLocked(handle ScenarioCandidateHandle) (*scenarioCandidateRecord, error) {
	if err := w.validateHandle(handle.value, handleScenarioCandidate, capabilityEdit, false); err != nil {
		return nil, err
	}
	record := w.scenarioCandidates[handle.value.serial]
	if record == nil || record.handle != handle || !w.ownsPartition(record.partition) {
		return nil, w.unavailableHandle(handle.value, ErrScenarioCandidateNotFound)
	}
	if err := w.ensureLive(handle.value, record.expires); err != nil {
		return nil, err
	}
	return record, nil
}

func scenarioCandidateSnapshot(candidate *scenarioCandidateRecord, revision *scenarioRevisionRecord) ScenarioCandidateSnapshot {
	return ScenarioCandidateSnapshot{
		Handle: candidate.handle, Head: candidate.head, HeadDigest: revision.digest, FixtureCount: len(revision.fixtures),
		Capability: CapabilityEdit, DisclosureDomain: candidate.disclosure, ExpiresAt: candidate.expires,
	}
}

func scenarioRevisionConflict(message string) error {
	return workspaceError("SCENARIO_REVISION_CONFLICT", message, ErrRevisionConflict)
}

func scenarioCachedResult(cached scenarioIdempotencyRecord, fingerprint string) (ScenarioApplyResult, error) {
	if cached.fingerprint != fingerprint {
		return ScenarioApplyResult{}, workspaceError("SCENARIO_IDEMPOTENCY_CONFLICT", "idempotency key was already used for another scenario request", ErrRevisionConflict)
	}
	return cloneScenarioApplyResult(cached.result), nil
}

func scenarioRequestFingerprint(request ScenarioApplyRequest, operations []ScenarioOperation) string {
	encoded, err := json.Marshal(struct {
		CandidateScope uint64              `json:"candidate_scope"`
		Candidate      uint64              `json:"candidate"`
		HeadScope      uint64              `json:"head_scope"`
		Head           uint64              `json:"head"`
		Digest         string              `json:"digest"`
		Operations     []ScenarioOperation `json:"operations"`
	}{
		CandidateScope: request.Candidate.value.scope,
		Candidate:      request.Candidate.value.serial,
		HeadScope:      request.ExpectedHead.value.scope,
		Head:           request.ExpectedHead.value.serial,
		Digest:         request.ExpectedDigest,
		Operations:     operations,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func cloneScenarioApplyResult(result ScenarioApplyResult) ScenarioApplyResult {
	result.Revision = cloneScenarioSnapshot(result.Revision)
	return result
}

func cloneScenarioFixtures(input []paperscenario.Fixture) []paperscenario.Fixture {
	output := append([]paperscenario.Fixture(nil), input...)
	for i := range output {
		output[i].Values = cloneScenarioFields(output[i].Values)
	}
	return output
}

func cloneScenarioFields(input []paperscenario.Field) []paperscenario.Field {
	output := make([]paperscenario.Field, len(input))
	for i := range input {
		output[i] = paperscenario.Field{Name: input[i].Name, Value: cloneScenarioValue(input[i].Value)}
	}
	return output
}

func cloneScenarioValue(value paperscenario.Value) paperscenario.Value {
	value.Object = cloneScenarioFields(value.Object)
	value.List = append([]paperscenario.Item(nil), value.List...)
	for i := range value.List {
		value.List[i].Value = cloneScenarioValue(value.List[i].Value)
	}
	return value
}
