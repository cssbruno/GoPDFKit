// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/internal/paperscenario"
)

// PaperScenarioMutationGuard belongs exclusively to the scenario revision
// domain. Open authorizes the operation but contributes no source revision to
// the scenario compare-and-swap.
type PaperScenarioMutationGuard struct {
	Open           OpenHandle              `json:"-"`
	Candidate      ScenarioCandidateHandle `json:"-"`
	ExpectedHead   ScenarioRevisionHandle  `json:"-"`
	ExpectedDigest string                  `json:"expected_digest"`
	IdempotencyKey string                  `json:"idempotency_key"`
}

type PaperSetScenarioValueRequest struct {
	Guard    PaperScenarioMutationGuard `json:"guard"`
	Scenario string                     `json:"scenario"`
	Path     string                     `json:"path"`
	// Key selects one existing stable-keyed list item. Empty Key addresses an
	// ordinary object field path and never implies a positional list index.
	Key   string              `json:"key,omitempty"`
	Value paperscenario.Value `json:"value"`
}

type PaperScenarioSemanticDiff struct {
	Domain       string `json:"domain"`
	Operation    string `json:"operation"`
	Scenario     string `json:"scenario"`
	Path         string `json:"path"`
	StableKey    string `json:"stable_key,omitempty"`
	BeforeDigest string `json:"before_digest"`
	AfterDigest  string `json:"after_digest"`
}

type PaperScenarioMutationResult struct {
	Candidate ScenarioCandidateSnapshot `json:"candidate"`
	Revision  ScenarioRevisionSnapshot  `json:"revision"`
	Applied   int                       `json:"applied"`
	Semantic  PaperScenarioSemanticDiff `json:"semantic_diff"`
}

// PaperSetScenarioValue updates one object field or one existing stable-keyed
// list item. It cannot publish or alter a source revision.
func (w *Workspace) PaperSetScenarioValue(request PaperSetScenarioValueRequest) (PaperScenarioMutationResult, error) {
	if err := w.requireEditCapability(request.Guard.Open); err != nil {
		return PaperScenarioMutationResult{}, err
	}
	if request.Guard.IdempotencyKey == "" || len(request.Guard.IdempotencyKey) > w.limits.MaxQueryBytes || !utf8.ValidString(request.Guard.IdempotencyKey) {
		return PaperScenarioMutationResult{}, workspaceError("INVALID_IDEMPOTENCY_KEY", "scenario mutation idempotency key is invalid or exceeds configured bounds", ErrInvalidQuery)
	}
	if request.Scenario == "" || len(request.Scenario) > w.limits.MaxQueryBytes ||
		request.Path == "" || len(request.Path) > w.limits.MaxScenarioPathBytes ||
		len(request.Key) > w.limits.MaxQueryBytes {
		return PaperScenarioMutationResult{}, workspaceError("INVALID_SCENARIO_OPERATION", "scenario name, path, or stable key exceeds configured bounds", ErrInvalidQuery)
	}
	kind := ScenarioOperationSetValue
	if request.Key != "" {
		kind = ScenarioOperationReplaceListItem
	}
	result, err := w.ApplyScenario(ScenarioApplyRequest{
		Candidate: request.Guard.Candidate, ExpectedHead: request.Guard.ExpectedHead,
		ExpectedDigest: request.Guard.ExpectedDigest, IdempotencyKey: request.Guard.IdempotencyKey,
		Operations: []ScenarioOperation{{Kind: kind, Scenario: request.Scenario, Path: request.Path, Key: request.Key, Value: request.Value}},
	})
	if err != nil {
		return PaperScenarioMutationResult{}, err
	}
	return PaperScenarioMutationResult{
		Candidate: result.Candidate, Revision: result.Revision, Applied: result.Applied,
		Semantic: PaperScenarioSemanticDiff{
			Domain: "scenario", Operation: "set_scenario_value", Scenario: request.Scenario,
			Path: request.Path, StableKey: request.Key, BeforeDigest: request.Guard.ExpectedDigest, AfterDigest: result.Revision.Digest,
		},
	}, nil
}

type PaperFillSlotRequest struct {
	Guard   PaperMutationGuard   `json:"guard"`
	Slot    string               `json:"slot"`
	Content []paperedit.NodeSpec `json:"content"`
}

// PaperFillSlot adds the sole fill for one declared component slot. It
// resolves the use's referenced component, validates slot type/cardinality,
// and semantically compiles the complete candidate before publication.
func (w *Workspace) PaperFillSlot(request PaperFillSlotRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	operation, targets, err := w.prepareSlotFill(revision, request.Guard.Target, request.Slot, request.Content)
	if err != nil {
		return PaperMutationResult{}, err
	}
	return w.applyPaperMutation("fill_slot", request.Guard, opened, revision, targets, []paperedit.Operation{operation}, "INVALID_SLOT_FILL")
}

func (w *Workspace) prepareSlotFill(revision *revisionRecord, target, slotID string, content []paperedit.NodeSpec) (paperedit.Operation, []string, error) {
	if slotID == "" || len(slotID) > w.limits.MaxQueryBytes || slotID[0] != '@' {
		return nil, nil, workspaceError("INVALID_SLOT_FILL", "slot must be a bounded readable @id", paperedit.ErrInvalidOperation)
	}
	if len(content) == 0 || len(content) > w.limits.MaxOperations {
		return nil, nil, workspaceError("SLOT_FILL_LIMIT", "slot fill content count is outside configured bounds", ErrLimit)
	}
	encoded, err := json.Marshal(content)
	if err != nil || len(encoded) > w.maxMutationPayloadBytes() {
		return nil, nil, workspaceError("SLOT_FILL_LIMIT", "slot fill payload exceeds configured bounds", ErrLimit)
	}
	nodes := 0
	for _, node := range content {
		if !countNodeSpecBounded(node, &nodes, w.limits.MaxNodes) {
			return nil, nil, workspaceError("SLOT_FILL_LIMIT", "slot fill node count exceeds configured bounds", ErrLimit)
		}
	}
	use := findNodeByID(revision.parsed.AST.Root, target)
	if use == nil || use.Kind != paperlang.NodeUse {
		return nil, nil, workspaceError("INVALID_SLOT_FILL", "slot fill target must be a component use", paperedit.ErrInvalidOperation)
	}
	componentName, err := componentReference(use)
	if err != nil {
		return nil, nil, err
	}
	definition, err := uniqueComponentDefinition(revision.parsed.AST.Root, componentName)
	if err != nil {
		return nil, nil, err
	}
	slot, err := uniqueComponentSlot(definition, slotID)
	if err != nil {
		return nil, nil, err
	}
	for _, member := range use.Members {
		if member.Node != nil && member.Node.Kind == paperlang.NodeFill && member.Node.ID == slotID {
			return nil, nil, workspaceError("SLOT_CARDINALITY", "component use already has a fill for this slot", paperedit.ErrInvalidOperation)
		}
	}
	slotType, err := slotTypeForMutation(slot)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range content {
		if !slotAcceptsMutation(slotType, node.Kind) {
			return nil, nil, workspaceError("SLOT_TYPE", "slot content does not satisfy the declared slot type", paperedit.ErrInvalidOperation)
		}
	}
	fill := paperedit.NodeSpec{Kind: paperlang.NodeFill, ID: slotID, Children: cloneNodeSpecsForMutation(content)}
	return paperedit.InsertNode{Parent: target, Node: fill}, []string{target, target + "/" + slotID}, nil
}

func (w *Workspace) requireEditCapability(handle OpenHandle) error {
	if w == nil {
		return workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	opened, err := w.openLocked(handle)
	if err != nil {
		return err
	}
	if opened.mode != CapabilityEdit {
		return workspaceError("CAPABILITY_DENIED", "mutation requires an edit-capable open handle", ErrInvalidHandle)
	}
	return nil
}

func countNodeSpecBounded(node paperedit.NodeSpec, count *int, limit int) bool {
	(*count)++
	if *count > limit {
		return false
	}
	for _, child := range node.Children {
		if !countNodeSpecBounded(child, count, limit) {
			return false
		}
	}
	return true
}

func componentReference(use *paperlang.Node) (string, error) {
	value := ""
	for _, member := range use.Members {
		if member.Property == nil || member.Property.Name != "component" {
			continue
		}
		if value != "" || member.Property.Value.StringValue == nil {
			return "", workspaceError("AMBIGUOUS_COMPONENT", "component use has an ambiguous component reference", paperedit.ErrInvalidOperation)
		}
		value = *member.Property.Value.StringValue
	}
	if value == "" {
		return "", workspaceError("INVALID_SLOT_FILL", "component use has no readable component reference", paperedit.ErrInvalidOperation)
	}
	return value, nil
}

func uniqueComponentDefinition(root *paperlang.Node, id string) (*paperlang.Node, error) {
	var found *paperlang.Node
	if root != nil {
		for _, member := range root.Members {
			if member.Node == nil || member.Node.Kind != paperlang.NodeComponent || member.Node.ID != id {
				continue
			}
			if found != nil {
				return nil, workspaceError("AMBIGUOUS_COMPONENT", "component reference resolves to more than one definition", paperedit.ErrInvalidOperation)
			}
			found = member.Node
		}
	}
	if found == nil {
		return nil, workspaceError("INVALID_SLOT_FILL", "referenced component definition was not found", paperedit.ErrInvalidOperation)
	}
	return found, nil
}

func uniqueComponentSlot(component *paperlang.Node, id string) (*paperlang.Node, error) {
	var found *paperlang.Node
	for _, member := range component.Members {
		if member.Node == nil || member.Node.Kind != paperlang.NodeSlot || member.Node.ID != id {
			continue
		}
		if found != nil {
			return nil, workspaceError("AMBIGUOUS_SLOT", "component declares the slot more than once", paperedit.ErrInvalidOperation)
		}
		found = member.Node
	}
	if found == nil {
		return nil, workspaceError("INVALID_SLOT_FILL", "component does not declare the requested slot", paperedit.ErrInvalidOperation)
	}
	return found, nil
}

func slotTypeForMutation(slot *paperlang.Node) (string, error) {
	slotType := "blocks"
	seen := false
	for _, member := range slot.Members {
		if member.Property == nil || member.Property.Name != "type" {
			continue
		}
		if seen || member.Property.Value.StringValue == nil {
			return "", workspaceError("AMBIGUOUS_SLOT", "slot has an ambiguous type contract", paperedit.ErrInvalidOperation)
		}
		seen = true
		slotType = strings.ToLower(strings.TrimSpace(*member.Property.Value.StringValue))
	}
	if slotType != "blocks" && slotType != "text" && slotType != "list" && slotType != "row-column" {
		return "", workspaceError("SLOT_TYPE", "slot declares an unsupported type", paperedit.ErrInvalidOperation)
	}
	return slotType, nil
}

func slotAcceptsMutation(slotType string, kind paperlang.NodeKind) bool {
	switch slotType {
	case "blocks":
		return kind == paperlang.NodeText || kind == paperlang.NodeParagraph || kind == paperlang.NodeHeading ||
			kind == paperlang.NodeList || kind == paperlang.NodePageBreak || kind == paperlang.NodeRow ||
			kind == paperlang.NodeColumn || kind == paperlang.NodeUse
	case "text":
		return kind == paperlang.NodeText || kind == paperlang.NodeParagraph || kind == paperlang.NodeHeading
	case "list":
		return kind == paperlang.NodeList
	case "row-column":
		return kind == paperlang.NodeRow || kind == paperlang.NodeColumn
	default:
		return false
	}
}

func cloneNodeSpecsForMutation(source []paperedit.NodeSpec) []paperedit.NodeSpec {
	result := make([]paperedit.NodeSpec, len(source))
	for i, node := range source {
		result[i] = cloneNodeSpec(node)
	}
	return result
}
