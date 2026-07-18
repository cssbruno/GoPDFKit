// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"encoding/json"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

// PaperReadScope pins every discovery request to one exact opened revision and
// declares independent result, serialization, and traversal bounds.
type PaperReadScope struct {
	Open             OpenHandle         `json:"-"`
	ExpectedRevision RevisionHandle     `json:"-"`
	ExpectedDigest   paperedit.Revision `json:"expected_digest"`
	MaxResults       int                `json:"max_results"`
	MaxBytes         int                `json:"max_bytes"`
	MaxWork          int                `json:"max_work"`
}

type readWork struct {
	used      int
	limit     int
	truncated bool
}

func (b *readWork) spend() bool {
	if b.used >= b.limit {
		b.truncated = true
		return false
	}
	b.used++
	return true
}

func (w *Workspace) validateReadScope(scope PaperReadScope) error {
	if scope.MaxResults < 1 || scope.MaxResults > w.limits.MaxSearchResults ||
		scope.MaxBytes < 1 || scope.MaxBytes > w.limits.MaxContextBytes ||
		scope.MaxWork < 1 || scope.MaxWork > w.limits.MaxNodes {
		return workspaceError("READ_LIMIT", "read result, byte, or work bounds are outside configured limits", ErrLimit)
	}
	return nil
}

type ComponentSlotSummary struct {
	ID         string         `json:"id"`
	Type       string         `json:"type,omitempty"`
	Required   bool           `json:"required,omitempty"`
	Span       paperlang.Span `json:"span"`
	Properties []PropertyView `json:"properties,omitempty"`
}

type ComponentSummary struct {
	ID               string                     `json:"id"`
	Span             paperlang.Span             `json:"span"`
	Properties       []PropertyView             `json:"properties,omitempty"`
	Slots            []ComponentSlotSummary     `json:"slots,omitempty"`
	Children         []NodeSummary              `json:"children,omitempty"`
	Mappings         []papercompile.NodeMapping `json:"mappings,omitempty"`
	MembersTruncated bool                       `json:"members_truncated,omitempty"`
}

type PaperComponentsRequest struct {
	Scope PaperReadScope `json:"scope"`
	Query string         `json:"query,omitempty"`
}

type PaperComponentsResult struct {
	Open         PaperOpenSnapshot  `json:"open"`
	Components   []ComponentSummary `json:"components,omitempty"`
	Total        int                `json:"total"`
	TotalExact   bool               `json:"total_exact"`
	Truncated    bool               `json:"truncated,omitempty"`
	WorkUsed     int                `json:"work_used"`
	EncodedBytes int                `json:"encoded_bytes"`
}

// PaperComponents discovers source component definitions and their direct
// slot/property contracts, plus compiler mappings originating inside each
// definition span.
func (w *Workspace) PaperComponents(request PaperComponentsRequest) (PaperComponentsResult, error) {
	if w == nil {
		return PaperComponentsResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := w.validateReadScope(request.Scope); err != nil {
		return PaperComponentsResult{}, err
	}
	query := strings.ToLower(strings.TrimSpace(request.Query))
	if len(query) > w.limits.MaxQueryBytes {
		return PaperComponentsResult{}, workspaceError("INVALID_QUERY", "component query exceeds the configured limit", ErrInvalidQuery)
	}
	opened, revision, err := w.exactOpenRevision(request.Scope.Open, request.Scope.ExpectedRevision, request.Scope.ExpectedDigest)
	if err != nil {
		return PaperComponentsResult{}, err
	}
	result := PaperComponentsResult{Open: snapshotOpen(opened, revision.file), TotalExact: true}
	work := readWork{limit: request.Scope.MaxWork}
	root := revision.parsed.AST.Root
	if root != nil {
		for _, member := range root.Members {
			if !work.spend() {
				break
			}
			node := member.Node
			if node == nil || node.Kind != paperlang.NodeComponent || (query != "" && !strings.Contains(strings.ToLower(node.ID), query)) {
				continue
			}
			result.Total++
			summary := summarizeComponent(node, revision.compiled.Mapping.Nodes, &work, request.Scope.MaxResults)
			if len(result.Components) >= request.Scope.MaxResults {
				result.Truncated = true
				continue
			}
			trial := result
			trial.Components = append(append([]ComponentSummary(nil), result.Components...), summary)
			trial.Truncated = true
			if stableJSONBytes(&trial) > request.Scope.MaxBytes {
				result.Truncated = true
				continue
			}
			result.Components = trial.Components
		}
	}
	result.WorkUsed = work.used
	result.TotalExact = !work.truncated
	result.Truncated = result.Truncated || work.truncated || len(result.Components) < result.Total
	result.EncodedBytes = stableJSONBytes(&result)
	if result.EncodedBytes > request.Scope.MaxBytes {
		return PaperComponentsResult{}, workspaceError("READ_LIMIT", "component byte budget is too small for required metadata", ErrLimit)
	}
	return cloneComponentsResult(result), nil
}

func summarizeComponent(node *paperlang.Node, mappings []papercompile.NodeMapping, work *readWork, memberLimit int) ComponentSummary {
	result := ComponentSummary{ID: node.ID, Span: node.Span}
	members := 0
	for _, member := range node.Members {
		if !work.spend() {
			result.MembersTruncated = true
			break
		}
		if members >= memberLimit {
			result.MembersTruncated = true
			break
		}
		if member.Property != nil {
			result.Properties = append(result.Properties, scalarView(member.Property.Name, member.Property.Value))
			members++
		}
		if member.Node != nil {
			result.Children = append(result.Children, nodeSummary(member.Node))
			members++
			if member.Node.Kind == paperlang.NodeSlot {
				slot, truncated := summarizeSlot(member.Node, work, memberLimit)
				result.Slots = append(result.Slots, slot)
				result.MembersTruncated = result.MembersTruncated || truncated
			}
		}
	}
	for _, mapping := range mappings {
		if !work.spend() {
			result.MembersTruncated = true
			break
		}
		if spanInside(mapping.DefinitionSpan, node.Span) {
			if len(result.Mappings) >= memberLimit {
				result.MembersTruncated = true
				break
			}
			result.Mappings = append(result.Mappings, mapping)
		}
	}
	return result
}

func summarizeSlot(node *paperlang.Node, work *readWork, limit int) (ComponentSlotSummary, bool) {
	result := ComponentSlotSummary{ID: node.ID, Span: node.Span}
	for _, member := range node.Members {
		if member.Property == nil {
			continue
		}
		if len(result.Properties) >= limit || !work.spend() {
			return result, true
		}
		view := scalarView(member.Property.Name, member.Property.Value)
		result.Properties = append(result.Properties, view)
		if member.Property.Name == "type" && member.Property.Value.StringValue != nil {
			result.Type = *member.Property.Value.StringValue
		}
		if member.Property.Name == "required" && member.Property.Value.BoolValue != nil {
			result.Required = *member.Property.Value.BoolValue
		}
	}
	return result, false
}

type PaperInspectRequest struct {
	Scope         PaperReadScope `json:"scope"`
	Target        string         `json:"target"`
	IncludeSource bool           `json:"include_source,omitempty"`
}

type PaperInspectResult struct {
	Open         PaperOpenSnapshot          `json:"open"`
	Node         NodeView                   `json:"node"`
	Source       string                     `json:"source,omitempty"`
	Mappings     []papercompile.NodeMapping `json:"mappings,omitempty"`
	Truncated    bool                       `json:"truncated,omitempty"`
	WorkUsed     int                        `json:"work_used"`
	EncodedBytes int                        `json:"encoded_bytes"`
}

// PaperInspect resolves one readable source ID without returning retained AST
// pointers. Source and compiler provenance are included only when they fit the
// caller's declared bounds.
func (w *Workspace) PaperInspect(request PaperInspectRequest) (PaperInspectResult, error) {
	if w == nil {
		return PaperInspectResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := w.validateReadScope(request.Scope); err != nil {
		return PaperInspectResult{}, err
	}
	if request.Target == "" || len(request.Target) > w.limits.MaxQueryBytes {
		return PaperInspectResult{}, workspaceError("INVALID_QUERY", "inspect target is empty or exceeds the configured limit", ErrInvalidQuery)
	}
	opened, revision, err := w.exactOpenRevision(request.Scope.Open, request.Scope.ExpectedRevision, request.Scope.ExpectedDigest)
	if err != nil {
		return PaperInspectResult{}, err
	}
	work := readWork{limit: request.Scope.MaxWork}
	node := findNodeByIDBounded(revision.parsed.AST.Root, request.Target, &work)
	if node == nil {
		if work.truncated {
			return PaperInspectResult{}, workspaceError("READ_WORK_LIMIT", "inspect traversal exhausted its work budget", ErrLimit)
		}
		return PaperInspectResult{}, workspaceError("NODE_NOT_FOUND", "readable ID was not found", ErrRevisionNotFound)
	}
	result := PaperInspectResult{Open: snapshotOpen(opened, revision.file)}
	result.Node = boundedNodeView(node, request.Scope.MaxResults, &result.Truncated)
	if request.IncludeSource {
		if source, ok := sourceForSpan(revision.source, node.Span); ok {
			trial := result
			trial.Source = source
			trial.Truncated = true
			if stableJSONBytes(&trial) <= request.Scope.MaxBytes {
				result.Source = source
			} else {
				result.Truncated = true
			}
		}
	}
	for _, mapping := range revision.compiled.Mapping.Nodes {
		if !work.spend() {
			result.Truncated = true
			break
		}
		if mapping.ID != request.Target {
			continue
		}
		if len(result.Mappings) >= request.Scope.MaxResults {
			result.Truncated = true
			break
		}
		trial := result
		trial.Mappings = append(append([]papercompile.NodeMapping(nil), result.Mappings...), mapping)
		trial.Truncated = true
		if stableJSONBytes(&trial) > request.Scope.MaxBytes {
			result.Truncated = true
			break
		}
		result.Mappings = trial.Mappings
	}
	result.WorkUsed = work.used
	result.Truncated = result.Truncated || work.truncated
	result.EncodedBytes = stableJSONBytes(&result)
	if result.EncodedBytes > request.Scope.MaxBytes {
		return PaperInspectResult{}, workspaceError("READ_LIMIT", "inspect byte budget is too small for required node metadata", ErrLimit)
	}
	return cloneInspectResult(result), nil
}

func findNodeByIDBounded(root *paperlang.Node, id string, work *readWork) *paperlang.Node {
	if root == nil || !work.spend() {
		return nil
	}
	if root.ID == id {
		return root
	}
	for _, member := range root.Members {
		if member.Node == nil {
			continue
		}
		if found := findNodeByIDBounded(member.Node, id, work); found != nil {
			return found
		}
		if work.truncated {
			return nil
		}
	}
	return nil
}

func boundedNodeView(node *paperlang.Node, limit int, truncated *bool) NodeView {
	view := NodeView{Node: nodeSummary(node)}
	if node.Value != nil {
		value := scalarView("", *node.Value)
		view.Value = &value
	}
	for _, member := range node.Members {
		if len(view.Properties)+len(view.Children) >= limit {
			view.MembersTruncated = true
			*truncated = true
			break
		}
		if member.Property != nil {
			view.Properties = append(view.Properties, scalarView(member.Property.Name, member.Property.Value))
		}
		if member.Node != nil {
			view.Children = append(view.Children, nodeSummary(member.Node))
		}
	}
	return view
}

type PaperSearchRequest struct {
	Scope PaperReadScope `json:"scope"`
	Query string         `json:"query"`
}

type PaperSearchMatch struct {
	Domain  string                    `json:"domain"`
	Node    NodeSummary               `json:"node"`
	Field   string                    `json:"field"`
	Value   string                    `json:"value"`
	Span    paperlang.Span            `json:"span"`
	Mapping *papercompile.NodeMapping `json:"mapping,omitempty"`
}

type PaperSearchResult struct {
	Open         PaperOpenSnapshot  `json:"open"`
	Matches      []PaperSearchMatch `json:"matches,omitempty"`
	Total        int                `json:"total"`
	TotalExact   bool               `json:"total_exact"`
	Truncated    bool               `json:"truncated,omitempty"`
	WorkUsed     int                `json:"work_used"`
	EncodedBytes int                `json:"encoded_bytes"`
}

// PaperSearch deterministically searches source AST fields first and compiler
// mappings second. It continues counting after the result cap while work
// remains, making Total exact whenever TotalExact is true.
func (w *Workspace) PaperSearch(request PaperSearchRequest) (PaperSearchResult, error) {
	if w == nil {
		return PaperSearchResult{}, workspaceError("INVALID_WORKSPACE", "workspace is nil", ErrInvalidHandle)
	}
	if err := w.validateReadScope(request.Scope); err != nil {
		return PaperSearchResult{}, err
	}
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > w.limits.MaxQueryBytes {
		return PaperSearchResult{}, workspaceError("INVALID_QUERY", "search query is empty or exceeds the configured limit", ErrInvalidQuery)
	}
	opened, revision, err := w.exactOpenRevision(request.Scope.Open, request.Scope.ExpectedRevision, request.Scope.ExpectedDigest)
	if err != nil {
		return PaperSearchResult{}, err
	}
	result := PaperSearchResult{Open: snapshotOpen(opened, revision.file), TotalExact: true}
	work := readWork{limit: request.Scope.MaxWork}
	needle := strings.ToLower(query)
	add := func(match PaperSearchMatch) {
		result.Total++
		if len(result.Matches) >= request.Scope.MaxResults {
			result.Truncated = true
			return
		}
		trial := result
		trial.Matches = append(append([]PaperSearchMatch(nil), result.Matches...), match)
		trial.Truncated = true
		if stableJSONBytes(&trial) > request.Scope.MaxBytes {
			result.Truncated = true
			return
		}
		result.Matches = trial.Matches
	}
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil || work.truncated {
			return
		}
		fields := []struct {
			name, value string
			span        paperlang.Span
		}{{"kind", string(node.Kind), node.HeaderSpan}, {"id", node.ID, node.HeaderSpan}}
		if node.Value != nil {
			fields = append(fields, struct {
				name, value string
				span        paperlang.Span
			}{"value", node.Value.Raw, node.Value.Span})
		}
		for _, field := range fields {
			if !work.spend() {
				return
			}
			if field.value != "" && strings.Contains(strings.ToLower(field.value), needle) {
				add(PaperSearchMatch{Domain: "ast", Node: nodeSummary(node), Field: field.name, Value: field.value, Span: field.span})
			}
		}
		for _, member := range node.Members {
			if property := member.Property; property != nil {
				for _, field := range []struct{ name, value string }{{"property_name", property.Name}, {"property_value", property.Value.Raw}} {
					if !work.spend() {
						return
					}
					if strings.Contains(strings.ToLower(field.value), needle) {
						add(PaperSearchMatch{Domain: "ast", Node: nodeSummary(node), Field: field.name, Value: field.value, Span: property.Span})
					}
				}
			}
			walk(member.Node)
		}
	}
	walk(revision.parsed.AST.Root)
	if !work.truncated {
		for i := range revision.compiled.Mapping.Nodes {
			mapping := revision.compiled.Mapping.Nodes[i]
			for _, field := range []struct{ name, value string }{
				{"id", mapping.ID}, {"instance_path", mapping.InstancePath}, {"binding_path", mapping.BindingPath},
			} {
				if !work.spend() {
					break
				}
				if field.value != "" && strings.Contains(strings.ToLower(field.value), needle) {
					copyMapping := mapping
					add(PaperSearchMatch{Domain: "mapping", Node: NodeSummary{Kind: mapping.Kind, ID: mapping.ID, Span: mapping.Span}, Field: field.name, Value: field.value, Span: mapping.Span, Mapping: &copyMapping})
				}
			}
			if work.truncated {
				break
			}
		}
	}
	result.WorkUsed = work.used
	result.TotalExact = !work.truncated
	result.Truncated = result.Truncated || work.truncated || len(result.Matches) < result.Total
	result.EncodedBytes = stableJSONBytes(&result)
	if result.EncodedBytes > request.Scope.MaxBytes {
		return PaperSearchResult{}, workspaceError("READ_LIMIT", "search byte budget is too small for required metadata", ErrLimit)
	}
	return cloneSearchResult(result), nil
}

func spanInside(inner, outer paperlang.Span) bool {
	return inner.File != "" && inner.File == outer.File && inner.Start.Offset >= outer.Start.Offset && inner.End.Offset <= outer.End.Offset
}

func sourceForSpan(source string, span paperlang.Span) (string, bool) {
	start, end := span.Start.Offset, span.End.Offset
	if start > end || end > uint64(len(source)) {
		return "", false
	}
	return source[start:end], true
}

func stableJSONBytes(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return int(^uint(0) >> 1)
	}
	// Callers keep EncodedBytes at zero until this function returns, so the
	// encoded form currently contains one digit for that field. Solve the
	// self-describing length to a fixed point without a second output buffer.
	base := len(encoded) - 1
	size := len(encoded)
	for {
		next := base + decimalDigits(size)
		if next == size {
			return size
		}
		size = next
	}
}

func decimalDigits(value int) int {
	digits := 1
	for value >= 10 {
		value /= 10
		digits++
	}
	return digits
}

func cloneComponentsResult(result PaperComponentsResult) PaperComponentsResult {
	result.Components = append([]ComponentSummary(nil), result.Components...)
	for i := range result.Components {
		component := &result.Components[i]
		component.Properties = append([]PropertyView(nil), component.Properties...)
		component.Children = append([]NodeSummary(nil), component.Children...)
		component.Mappings = append([]papercompile.NodeMapping(nil), component.Mappings...)
		component.Slots = append([]ComponentSlotSummary(nil), component.Slots...)
		for j := range component.Slots {
			component.Slots[j].Properties = append([]PropertyView(nil), component.Slots[j].Properties...)
		}
	}
	return result
}

func cloneInspectResult(result PaperInspectResult) PaperInspectResult {
	result.Node.Properties = append([]PropertyView(nil), result.Node.Properties...)
	result.Node.Children = append([]NodeSummary(nil), result.Node.Children...)
	if result.Node.Value != nil {
		value := *result.Node.Value
		result.Node.Value = &value
	}
	result.Mappings = append([]papercompile.NodeMapping(nil), result.Mappings...)
	return result
}

func cloneSearchResult(result PaperSearchResult) PaperSearchResult {
	result.Matches = append([]PaperSearchMatch(nil), result.Matches...)
	for i := range result.Matches {
		if result.Matches[i].Mapping != nil {
			mapping := *result.Matches[i].Mapping
			result.Matches[i].Mapping = &mapping
		}
	}
	return result
}
