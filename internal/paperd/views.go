// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cssbruno/gopdfkit/document"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type RevisionSnapshot struct {
	Handle               RevisionHandle
	File                 string
	Source               string
	Revision             paperedit.Revision
	Bytes                int
	SyntaxNodes          int
	ParseOK              bool
	CompileOK            bool
	ParseDiagnostics     []paperlang.Diagnostic
	CompileDiagnostics   []paperlang.Diagnostic
	DiagnosticsTruncated bool
	Capability           CapabilityMode   `json:"capability"`
	DisclosureDomain     DisclosureDomain `json:"disclosure_domain"`
	ExpiresAt            time.Time        `json:"expires_at"`
}

type CandidateSnapshot struct {
	Handle           CandidateHandle  `json:"-"`
	Head             RevisionHandle   `json:"-"`
	Capability       CapabilityMode   `json:"capability"`
	DisclosureDomain DisclosureDomain `json:"disclosure_domain"`
	ExpiresAt        time.Time        `json:"expires_at"`
}

func snapshotCandidate(record *candidateRecord) CandidateSnapshot {
	return CandidateSnapshot{
		Handle: record.handle, Head: record.head, Capability: CapabilityEdit,
		DisclosureDomain: record.disclosure, ExpiresAt: record.expires,
	}
}

func snapshotOf(record *revisionRecord) RevisionSnapshot {
	snapshot := RevisionSnapshot{
		Handle: record.handle, File: record.file, Source: record.source,
		Revision: record.revision, Bytes: len(record.source), SyntaxNodes: record.nodes,
		ParseOK: record.parsed.OK(), CompileOK: record.parsed.OK() && record.compiled.OK(),
		Capability: CapabilityRead, DisclosureDomain: record.disclosure, ExpiresAt: record.expires,
	}
	snapshot.ParseDiagnostics, snapshot.DiagnosticsTruncated = boundedDiagnostics(record.parsed.Diagnostics, MaxSearchResultsHard)
	compile, truncated := boundedDiagnostics(record.compiled.Diagnostics, MaxSearchResultsHard)
	snapshot.CompileDiagnostics = compile
	snapshot.DiagnosticsTruncated = snapshot.DiagnosticsTruncated || truncated
	return snapshot
}

func clonePaperDiagnostics(source []paperlang.Diagnostic) []paperlang.Diagnostic {
	if len(source) == 0 {
		return nil
	}
	return append([]paperlang.Diagnostic(nil), source...)
}

func boundedDiagnostics(source []paperlang.Diagnostic, limit int) ([]paperlang.Diagnostic, bool) {
	if len(source) > limit {
		return append([]paperlang.Diagnostic(nil), source[:limit]...), true
	}
	return clonePaperDiagnostics(source), false
}

// Context is a compact source/semantic orientation view suitable for deciding
// which ID to inspect or edit next.
type Context struct {
	Revision          RevisionSnapshot
	Root              NodeSummary
	Page              papercompile.PageSpec
	Title             string
	Language          string
	BodyBlocks        int
	Mappings          []papercompile.NodeMapping
	MappingsTruncated bool
}

type NodeSummary struct {
	Kind paperlang.NodeKind `json:"kind"`
	ID   string             `json:"id,omitempty"`
	Span paperlang.Span     `json:"span"`
}

func (w *Workspace) Context(handle RevisionHandle) (Context, error) {
	record, err := w.revision(handle)
	if err != nil {
		return Context{}, err
	}
	context := Context{Revision: snapshotOf(record)}
	if root := record.parsed.AST.Root; root != nil {
		context.Root = nodeSummary(root)
	}
	if record.parsed.OK() {
		context.Page = record.compiled.Page
		if record.compiled.Document != nil {
			context.Title = record.compiled.Document.Title
			context.Language = record.compiled.Document.Language
			context.BodyBlocks = len(record.compiled.Document.Body)
		}
		mappings := record.compiled.Mapping.Nodes
		limit := w.limits.MaxSearchResults
		if len(mappings) > limit {
			mappings = mappings[:limit]
			context.MappingsTruncated = true
		}
		context.Mappings = append([]papercompile.NodeMapping(nil), mappings...)
	}
	return context, nil
}

type PropertyView struct {
	Name string
	Kind paperlang.ScalarKind
	Raw  string
	Span paperlang.Span
}

type NodeView struct {
	Node             NodeSummary
	Value            *PropertyView
	Properties       []PropertyView
	Children         []NodeSummary
	MembersTruncated bool
}

// InspectID resolves one exact readable ID and returns a detached, bounded
// structural view. It does not return AST pointers.
func (w *Workspace) InspectID(handle RevisionHandle, id string) (NodeView, error) {
	if id == "" || len(id) > w.limits.MaxQueryBytes {
		return NodeView{}, workspaceError("INVALID_QUERY", "readable ID is empty or exceeds the query limit", ErrInvalidQuery)
	}
	record, err := w.revision(handle)
	if err != nil {
		return NodeView{}, err
	}
	node := findNodeByID(record.parsed.AST.Root, id)
	if node == nil {
		return NodeView{}, workspaceError("NODE_NOT_FOUND", "readable ID was not found", ErrRevisionNotFound)
	}
	view := NodeView{Node: nodeSummary(node)}
	if node.Value != nil {
		value := scalarView("", *node.Value)
		view.Value = &value
	}
	for _, member := range node.Members {
		if len(view.Properties)+len(view.Children) >= w.limits.MaxSearchResults {
			view.MembersTruncated = true
			break
		}
		if member.Property != nil {
			view.Properties = append(view.Properties, scalarView(member.Property.Name, member.Property.Value))
		}
		if member.Node != nil {
			view.Children = append(view.Children, nodeSummary(member.Node))
		}
	}
	return view, nil
}

func findNodeByID(root *paperlang.Node, id string) *paperlang.Node {
	if root == nil {
		return nil
	}
	if root.ID == id {
		return root
	}
	for _, member := range root.Members {
		if found := findNodeByID(member.Node, id); found != nil {
			return found
		}
	}
	return nil
}

func nodeSummary(node *paperlang.Node) NodeSummary {
	return NodeSummary{Kind: node.Kind, ID: node.ID, Span: node.Span}
}

func scalarView(name string, value paperlang.Scalar) PropertyView {
	return PropertyView{Name: name, Kind: value.Kind, Raw: value.Raw, Span: value.Span}
}

type SearchRequest struct {
	Revision RevisionHandle
	Query    string
	Limit    int
}

type SearchMatch struct {
	Node  NodeSummary
	Field string
	Value string
	Span  paperlang.Span
}

type SearchResult struct {
	Matches   []SearchMatch
	Total     int
	Truncated bool
}

// Search performs a deterministic, case-insensitive search over node IDs,
// kinds, inline values, property names, and property raw values.
func (w *Workspace) Search(request SearchRequest) (SearchResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" || len(query) > w.limits.MaxQueryBytes {
		return SearchResult{}, workspaceError("INVALID_QUERY", "search query is empty or exceeds the query limit", ErrInvalidQuery)
	}
	if request.Limit < 1 || request.Limit > w.limits.MaxSearchResults {
		return SearchResult{}, workspaceError("SEARCH_LIMIT", "search result limit is outside configured bounds", ErrLimit)
	}
	record, err := w.revision(request.Revision)
	if err != nil {
		return SearchResult{}, err
	}
	needle := strings.ToLower(query)
	result := SearchResult{}
	add := func(match SearchMatch) {
		result.Total++
		if len(result.Matches) < request.Limit {
			result.Matches = append(result.Matches, match)
		}
	}
	var walk func(*paperlang.Node)
	walk = func(node *paperlang.Node) {
		if node == nil {
			return
		}
		summary := nodeSummary(node)
		fields := []struct {
			name, value string
			span        paperlang.Span
		}{
			{"kind", string(node.Kind), node.HeaderSpan}, {"id", node.ID, node.HeaderSpan},
		}
		if node.Value != nil {
			fields = append(fields, struct {
				name, value string
				span        paperlang.Span
			}{"value", node.Value.Raw, node.Value.Span})
		}
		for _, field := range fields {
			if field.value != "" && strings.Contains(strings.ToLower(field.value), needle) {
				add(SearchMatch{Node: summary, Field: field.name, Value: field.value, Span: field.span})
			}
		}
		for _, member := range node.Members {
			if property := member.Property; property != nil {
				if strings.Contains(strings.ToLower(property.Name), needle) {
					add(SearchMatch{Node: summary, Field: "property_name", Value: property.Name, Span: property.Span})
				}
				if strings.Contains(strings.ToLower(property.Value.Raw), needle) {
					add(SearchMatch{Node: summary, Field: "property_value", Value: property.Value.Raw, Span: property.Value.Span})
				}
			}
			walk(member.Node)
		}
	}
	walk(record.parsed.AST.Root)
	result.Truncated = result.Total > len(result.Matches)
	return result, nil
}

type Compilation struct {
	Revision             RevisionHandle
	Page                 papercompile.PageSpec
	Title                string
	Language             string
	BodyBlocks           int
	Mappings             []papercompile.NodeMapping
	Diagnostics          []paperlang.Diagnostic
	MappingsTruncated    bool
	DiagnosticsTruncated bool
}

// Compile returns a detached semantic projection. It never exposes the
// mutable layout document retained inside the immutable revision record.
func (w *Workspace) Compile(handle RevisionHandle) (Compilation, error) {
	record, err := w.revision(handle)
	if err != nil {
		return Compilation{}, err
	}
	if !record.parsed.OK() {
		diagnostics, truncated := boundedDiagnostics(record.parsed.Diagnostics, w.limits.MaxSearchResults)
		return Compilation{Revision: handle, Diagnostics: diagnostics, DiagnosticsTruncated: truncated}, workspaceError("INVALID_SOURCE", "source has parse errors", ErrInvalidSource)
	}
	result := Compilation{Revision: handle, Page: record.compiled.Page}
	mappings := record.compiled.Mapping.Nodes
	if len(mappings) > w.limits.MaxSearchResults {
		mappings = mappings[:w.limits.MaxSearchResults]
		result.MappingsTruncated = true
	}
	result.Mappings = append([]papercompile.NodeMapping(nil), mappings...)
	result.Diagnostics, result.DiagnosticsTruncated = boundedDiagnostics(record.compiled.Diagnostics, w.limits.MaxSearchResults)
	if record.compiled.Document != nil {
		result.Title = record.compiled.Document.Title
		result.Language = record.compiled.Document.Language
		result.BodyBlocks = len(record.compiled.Document.Body)
	}
	if !record.compiled.OK() {
		return result, workspaceError("INVALID_SOURCE", "source has semantic compile errors", ErrInvalidSource)
	}
	return result, nil
}

type RenderResult struct {
	Revision RevisionHandle
	PDF      []byte
	Pipeline document.PaperRenderResult
}

// Render executes the production .paper boundary: document.WritePaper owns
// parsing, semantic compilation, planning, painter preflight, and painting.
// The workspace only bounds and captures the resulting PDF bytes.
func (w *Workspace) Render(handle RevisionHandle) (RenderResult, error) {
	record, err := w.revision(handle)
	if err != nil {
		return RenderResult{}, err
	}
	if !record.parsed.OK() || !record.compiled.OK() {
		return RenderResult{Revision: handle}, workspaceError("INVALID_SOURCE", "source is not renderable", ErrInvalidSource)
	}
	plan, _, err := document.PlanPaperWithImports(record.file, record.source, document.PaperImportResolver(w.importResolver))
	if err != nil {
		return RenderResult{Revision: handle}, err
	}
	pdf, err := document.NewDocument(document.WithUnit(document.UnitPoint), document.WithDeterministicOutput())
	if err != nil {
		return RenderResult{Revision: handle}, fmt.Errorf("paperd: create render document: %w", err)
	}
	pipeline, err := pdf.WritePaperPlan(plan)
	if err != nil {
		return RenderResult{Revision: handle, Pipeline: pipeline}, err
	}
	var output bytes.Buffer
	bounded := &limitWriter{writer: &output, remaining: int64(w.limits.MaxRenderBytes)}
	if err := pdf.OutputWithOptions(bounded, document.OutputOptions{Deterministic: true}); err != nil {
		if errorsIsLimit(err) || bounded.exceeded {
			return RenderResult{Revision: handle, Pipeline: pipeline}, workspaceError("RENDER_LIMIT", "rendered PDF exceeds the configured byte limit", ErrLimit)
		}
		return RenderResult{Revision: handle, Pipeline: pipeline}, fmt.Errorf("paperd: output PDF: %w", err)
	}
	return RenderResult{Revision: handle, PDF: append([]byte(nil), output.Bytes()...), Pipeline: pipeline}, nil
}

type limitWriter struct {
	writer    io.Writer
	remaining int64
	exceeded  bool
}

var errWriteLimit = errors.New("paperd: render byte limit exceeded")

func (w *limitWriter) Write(value []byte) (int, error) {
	if int64(len(value)) > w.remaining {
		w.exceeded = true
		return 0, errWriteLimit
	}
	n, err := w.writer.Write(value)
	w.remaining -= int64(n)
	return n, err
}

func errorsIsLimit(err error) bool {
	return errors.Is(err, errWriteLimit) || strings.Contains(err.Error(), errWriteLimit.Error())
}
