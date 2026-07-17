// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"errors"
	"fmt"
)

// StructuralQueryMaxResults is the hard per-category result ceiling for agent
// plan inspection. Match counts still report omitted results.
const StructuralQueryMaxResults uint32 = 256

var (
	ErrStructuralQueryNoSelector       = errors.New("layoutengine: structural query has no selector")
	ErrStructuralQueryInvalidLimit     = errors.New("layoutengine: structural query limit is invalid")
	ErrStructuralQueryInvalidSelector  = errors.New("layoutengine: structural query selector is invalid")
	ErrStructuralQueryPageNotFound     = errors.New("layoutengine: structural query page was not found")
	ErrStructuralQueryFragmentNotFound = errors.New("layoutengine: structural query fragment was not found")
	ErrStructuralQuerySelectorConflict = errors.New("layoutengine: structural query selectors conflict")
)

// StructuralQuery combines selectors with AND semantics. Zero values are
// absent. MaxResults is required and bounds each returned category
// independently; summary match counts remain exact.
type StructuralQuery struct {
	DiagnosticCode  DiagnosticCode
	Semantic        SemanticNodeID
	Role            SemanticRole
	Language        string
	HeadingLevel    uint8
	LinkDestination DestinationID
	Node            NodeID
	Key             NodeKey
	Instance        InstanceID
	Fragment        FragmentID
	Page            uint32
	MaxResults      uint32
}

// StructuralQueryCount reports exact matches and bounded returned results.
type StructuralQueryCount struct {
	Matches   uint64
	Returned  uint32
	Truncated bool
}

// StructuralQuerySummary contains stable totals in canonical plan order.
type StructuralQuerySummary struct {
	Pages        uint32
	Fragments    StructuralQueryCount
	Lines        StructuralQueryCount
	Commands     StructuralQueryCount
	Breaks       StructuralQueryCount
	Diagnostics  StructuralQueryCount
	Semantics    StructuralQueryCount
	ReadingOrder StructuralQueryCount
}

// StructuralFragment is a detached fragment projection with canonical indexes.
type StructuralFragment struct {
	Index      uint64
	PageIndex  uint32
	Provenance ProvenanceID
	Fragment   Fragment
}

// StructuralLine is a detached line projection with its owning semantic and
// page context copied from the fragment table.
type StructuralLine struct {
	Index      uint64
	Page       uint32
	PageIndex  uint32
	Node       NodeID
	Key        NodeKey
	Instance   InstanceID
	Region     RegionID
	Provenance ProvenanceID
	Line       PlannedLine
}

// StructuralCommand is a detached command projection. Fragment provenance is
// absent for page-level commands.
type StructuralCommand struct {
	Index                 uint64
	Page                  uint32
	PageIndex             uint32
	HasFragmentProvenance bool
	Node                  NodeID
	Key                   NodeKey
	Instance              InstanceID
	Region                RegionID
	Provenance            ProvenanceID
	Command               DisplayCommand
}

// StructuralBreak is a detached break-decision projection.
type StructuralBreak struct {
	Index    uint64
	Decision BreakDecision
}

type StructuralBreakDetailResult struct {
	Selector StructuralQuery
	Count    StructuralQueryCount
	Details  []BreakDetail
}

// StructuralDiagnostic is a detached diagnostic projection. Diagnostic owns
// copied nested evidence, related-reference, and fix slices.
type StructuralDiagnostic struct {
	Index      uint64
	Diagnostic Diagnostic
}

type StructuralSemantic struct {
	Index uint64
	Node  SemanticNode
}

type StructuralReadingOccurrence struct {
	Index      uint64
	Occurrence ReadingOccurrence
}

// StructuralQueryResult is a bounded, detached structural plan view.
type StructuralQueryResult struct {
	Selector     StructuralQuery
	Summary      StructuralQuerySummary
	Fragments    []StructuralFragment
	Lines        []StructuralLine
	Commands     []StructuralCommand
	Breaks       []StructuralBreak
	Diagnostics  []StructuralDiagnostic
	Semantics    []StructuralSemantic
	ReadingOrder []StructuralReadingOccurrence
}

// QueryStructure returns canonical-order plan records matching all supplied
// selectors. It never returns plan-owned slices or nested diagnostic storage.
func (p LayoutPlan) QueryStructure(query StructuralQuery) (StructuralQueryResult, error) {
	if query.DiagnosticCode == "" && !query.Semantic.Valid() && query.Role == "" && query.Language == "" && query.HeadingLevel == 0 &&
		!query.LinkDestination.Valid() && !query.Node.Valid() && query.Key == "" && !query.Instance.Valid() &&
		!query.Fragment.Valid() && query.Page == 0 {
		return StructuralQueryResult{}, ErrStructuralQueryNoSelector
	}
	if query.MaxResults == 0 || query.MaxResults > StructuralQueryMaxResults {
		return StructuralQueryResult{}, fmt.Errorf("%w: must be between 1 and %d", ErrStructuralQueryInvalidLimit, StructuralQueryMaxResults)
	}
	if query.Key != "" {
		if err := validateTextIdentity("structural query node key", string(query.Key)); err != nil {
			return StructuralQueryResult{}, fmt.Errorf("%w: %v", ErrStructuralQueryInvalidSelector, err)
		}
	}
	if query.DiagnosticCode != "" {
		if err := query.DiagnosticCode.validate(); err != nil {
			return StructuralQueryResult{}, fmt.Errorf("%w: %v", ErrStructuralQueryInvalidSelector, err)
		}
	}
	if query.Instance != "" {
		if err := validateTextIdentity("structural query instance ID", string(query.Instance)); err != nil {
			return StructuralQueryResult{}, fmt.Errorf("%w: %v", ErrStructuralQueryInvalidSelector, err)
		}
	}
	if query.Role != "" && !query.Role.valid() {
		return StructuralQueryResult{}, fmt.Errorf("%w: unsupported semantic role", ErrStructuralQueryInvalidSelector)
	}
	if query.Language != "" {
		if err := validateSemanticLanguage(query.Language); err != nil {
			return StructuralQueryResult{}, fmt.Errorf("%w: %v", ErrStructuralQueryInvalidSelector, err)
		}
	}
	if query.HeadingLevel > 6 {
		return StructuralQueryResult{}, fmt.Errorf("%w: heading level must be between 1 and 6", ErrStructuralQueryInvalidSelector)
	}
	if query.LinkDestination.Valid() && uint64(query.LinkDestination) > uint64(len(p.destinations)) {
		return StructuralQueryResult{}, fmt.Errorf("%w: missing link destination", ErrStructuralQueryInvalidSelector)
	}
	if query.Page > 0 && uint64(query.Page) > uint64(len(p.pages)) {
		return StructuralQueryResult{}, fmt.Errorf("%w: %d", ErrStructuralQueryPageNotFound, query.Page)
	}

	fragmentByID := make(map[FragmentID]Fragment, len(p.fragments))
	provenanceByFragment := make(map[FragmentID]ProvenanceID, len(p.fragments))
	semanticByID := make(map[SemanticNodeID]SemanticNode, len(p.semanticNodes))
	semanticByFragment := make(map[FragmentID]SemanticNodeID, len(p.semanticFragments))
	for _, node := range p.semanticNodes {
		semanticByID[node.ID] = node
	}
	for _, association := range p.semanticFragments {
		semanticByFragment[association.Fragment] = association.Semantic
	}
	semanticMatches := func(node SemanticNode) bool {
		return (!query.Semantic.Valid() || node.ID == query.Semantic) &&
			(query.Role == "" || node.Role == query.Role) &&
			(query.Language == "" || node.Attributes.Language == query.Language) &&
			(query.HeadingLevel == 0 || node.Attributes.HeadingLevel == query.HeadingLevel) &&
			(!query.LinkDestination.Valid() || node.Attributes.LinkDestination == query.LinkDestination) &&
			(query.Key == "" || node.Key == query.Key) &&
			(!query.Instance.Valid() || node.Instance == query.Instance)
	}
	semanticFilterPresent := query.Semantic.Valid() || query.Role != "" || query.Language != "" ||
		query.HeadingLevel != 0 || query.LinkDestination.Valid()
	if query.Semantic.Valid() {
		node, exists := semanticByID[query.Semantic]
		if !exists {
			return StructuralQueryResult{}, fmt.Errorf("%w: semantic %d", ErrStructuralQueryInvalidSelector, query.Semantic)
		}
		if !semanticMatches(node) {
			return StructuralQueryResult{}, ErrStructuralQuerySelectorConflict
		}
	}
	nodeKeys := make(map[NodeID]NodeKey)
	keyNodes := make(map[NodeKey]NodeID)
	for index, fragment := range p.fragments {
		fragmentByID[fragment.ID] = fragment
		provenanceByFragment[fragment.ID] = p.fragmentProvenance[index]
		nodeKeys[fragment.Node] = fragment.Key
		keyNodes[fragment.Key] = fragment.Node
	}
	issueFragments := make(map[FragmentID]struct{})
	if query.DiagnosticCode != "" {
		for _, diagnostic := range p.diagnostics {
			if diagnostic.Code != query.DiagnosticCode {
				continue
			}
			location := diagnostic.Location
			for _, fragment := range p.fragments {
				if location.Fragment.Valid() && fragment.ID != location.Fragment ||
					location.Node.Valid() && fragment.Node != location.Node ||
					location.Key != "" && fragment.Key != location.Key ||
					location.Instance.Valid() && fragment.Instance != location.Instance ||
					location.Page != 0 && fragment.Page != location.Page {
					continue
				}
				if location.Fragment.Valid() || location.Node.Valid() || location.Key != "" || location.Instance.Valid() || location.Page != 0 {
					issueFragments[fragment.ID] = struct{}{}
				}
			}
		}
	}
	if query.Fragment.Valid() {
		selected, found := fragmentByID[query.Fragment]
		if !found {
			return StructuralQueryResult{}, fmt.Errorf("%w: %d", ErrStructuralQueryFragmentNotFound, query.Fragment)
		}
		if query.Node.Valid() && query.Node != selected.Node || query.Key != "" && query.Key != selected.Key ||
			query.Instance.Valid() && query.Instance != selected.Instance || query.Page != 0 && query.Page != selected.Page ||
			semanticFilterPresent && !semanticMatches(semanticByID[semanticByFragment[selected.ID]]) {
			return StructuralQueryResult{}, ErrStructuralQuerySelectorConflict
		}
	}

	linePages := make([]uint32, len(p.lines))
	linePageIndexes := make([]uint32, len(p.lines))
	commandPages := make([]uint32, len(p.commands))
	commandPageIndexes := make([]uint32, len(p.commands))
	fragmentPageIndexes := make(map[FragmentID]uint32, len(p.fragments))
	for _, page := range p.pages {
		fragmentEnd, _ := page.Fragments.end(len(p.fragments))
		for index := int(page.Fragments.Start); index < fragmentEnd; index++ {
			fragmentPageIndexes[p.fragments[index].ID] = uint32(index - int(page.Fragments.Start))
		}
		lineEnd, _ := page.Lines.end(len(p.lines))
		for index := int(page.Lines.Start); index < lineEnd; index++ {
			linePages[index] = page.Number
			linePageIndexes[index] = uint32(index - int(page.Lines.Start))
		}
		commandEnd, _ := page.Commands.end(len(p.commands))
		for index := int(page.Commands.Start); index < commandEnd; index++ {
			commandPages[index] = page.Number
			commandPageIndexes[index] = uint32(index - int(page.Commands.Start))
		}
	}

	matchesFragment := func(fragment Fragment) bool {
		if query.DiagnosticCode != "" {
			if _, found := issueFragments[fragment.ID]; !found {
				return false
			}
		}
		semantic := semanticByID[semanticByFragment[fragment.ID]]
		return (!semanticFilterPresent || semanticMatches(semantic)) &&
			(!query.Node.Valid() || fragment.Node == query.Node) &&
			(query.Key == "" || fragment.Key == query.Key) &&
			(!query.Instance.Valid() || fragment.Instance == query.Instance) &&
			(!query.Fragment.Valid() || fragment.ID == query.Fragment) &&
			(query.Page == 0 || fragment.Page == query.Page)
	}

	result := StructuralQueryResult{Selector: query}
	matchedPages := make(map[uint32]struct{})
	if query.Page != 0 {
		matchedPages[query.Page] = struct{}{}
	}
	for index, fragment := range p.fragments {
		if !matchesFragment(fragment) {
			continue
		}
		matchedPages[fragment.Page] = struct{}{}
		result.Summary.Fragments.Matches++
		if uint32(len(result.Fragments)) < query.MaxResults {
			result.Fragments = append(result.Fragments, StructuralFragment{
				Index: uint64(index), PageIndex: fragmentPageIndexes[fragment.ID], Provenance: p.fragmentProvenance[index], Fragment: fragment,
			})
		}
	}
	for index, line := range p.lines {
		fragment := fragmentByID[line.Fragment]
		if !matchesFragment(fragment) {
			continue
		}
		result.Summary.Lines.Matches++
		if uint32(len(result.Lines)) < query.MaxResults {
			result.Lines = append(result.Lines, StructuralLine{
				Index: uint64(index), Page: linePages[index], PageIndex: linePageIndexes[index],
				Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Region: fragment.Region, Provenance: p.lineProvenance[index], Line: line,
			})
		}
	}
	for index, command := range p.commands {
		page := commandPages[index]
		var fragment Fragment
		matches := false
		if command.Fragment.Valid() {
			fragment = fragmentByID[command.Fragment]
			matches = matchesFragment(fragment)
		} else if query.Page != 0 && query.Page == page && !semanticFilterPresent && !query.Node.Valid() && query.Key == "" &&
			!query.Instance.Valid() && !query.Fragment.Valid() {
			matches = true
		}
		if !matches {
			continue
		}
		result.Summary.Commands.Matches++
		if uint32(len(result.Commands)) < query.MaxResults {
			projection := StructuralCommand{
				Index: uint64(index), Page: page, PageIndex: commandPageIndexes[index], Command: command,
			}
			if command.Fragment.Valid() {
				projection.HasFragmentProvenance = true
				projection.Node = fragment.Node
				projection.Key = fragment.Key
				projection.Instance = fragment.Instance
				projection.Region = fragment.Region
				projection.Provenance = provenanceByFragment[fragment.ID]
			}
			result.Commands = append(result.Commands, projection)
		}
	}
	for index, decision := range p.breaks {
		preceding := fragmentByID[decision.Preceding]
		triggering := fragmentByID[decision.Triggering]
		if !matchesFragment(preceding) && !matchesFragment(triggering) {
			continue
		}
		result.Summary.Breaks.Matches++
		if uint32(len(result.Breaks)) < query.MaxResults {
			result.Breaks = append(result.Breaks, StructuralBreak{Index: uint64(index), Decision: decision})
		}
	}
	selectedFragment := Fragment{}
	if query.Fragment.Valid() {
		selectedFragment = fragmentByID[query.Fragment]
	}
	for index, diagnostic := range p.diagnostics {
		if query.DiagnosticCode != "" && diagnostic.Code != query.DiagnosticCode {
			continue
		}
		if semanticFilterPresent {
			fragment, exists := fragmentByID[diagnostic.Location.Fragment]
			if !exists || !matchesFragment(fragment) {
				continue
			}
		}
		issueOnly := query.DiagnosticCode != "" && !query.Semantic.Valid() && query.Role == "" && query.Language == "" &&
			query.HeadingLevel == 0 && !query.LinkDestination.Valid() && !query.Node.Valid() && query.Key == "" &&
			!query.Instance.Valid() && !query.Fragment.Valid() && query.Page == 0
		if !issueOnly && !diagnosticMatchesStructuralQuery(diagnostic, query, selectedFragment, fragmentByID, nodeKeys, keyNodes) {
			continue
		}
		result.Summary.Diagnostics.Matches++
		if uint32(len(result.Diagnostics)) < query.MaxResults {
			result.Diagnostics = append(result.Diagnostics, StructuralDiagnostic{
				Index: uint64(index), Diagnostic: cloneDiagnostic(diagnostic),
			})
		}
	}
	semanticHasMatchingFragment := make(map[SemanticNodeID]bool)
	for _, fragment := range p.fragments {
		if matchesFragment(fragment) {
			semanticHasMatchingFragment[semanticByFragment[fragment.ID]] = true
		}
	}
	for index, node := range p.semanticNodes {
		matchesIdentity := semanticMatches(node)
		needsFragment := query.Node.Valid() || query.Fragment.Valid() || query.Page != 0
		if !matchesIdentity || needsFragment && !semanticHasMatchingFragment[node.ID] {
			continue
		}
		result.Summary.Semantics.Matches++
		if uint32(len(result.Semantics)) < query.MaxResults {
			result.Semantics = append(result.Semantics, StructuralSemantic{Index: uint64(index), Node: node})
		}
	}
	for index, occurrence := range p.readingOrder {
		fragment := fragmentByID[occurrence.Fragment]
		if !matchesFragment(fragment) {
			continue
		}
		result.Summary.ReadingOrder.Matches++
		if uint32(len(result.ReadingOrder)) < query.MaxResults {
			result.ReadingOrder = append(result.ReadingOrder, StructuralReadingOccurrence{Index: uint64(index), Occurrence: occurrence})
		}
	}

	result.Summary.Pages = uint32(len(matchedPages))
	finalizeStructuralCount(&result.Summary.Fragments, len(result.Fragments))
	finalizeStructuralCount(&result.Summary.Lines, len(result.Lines))
	finalizeStructuralCount(&result.Summary.Commands, len(result.Commands))
	finalizeStructuralCount(&result.Summary.Breaks, len(result.Breaks))
	finalizeStructuralCount(&result.Summary.Diagnostics, len(result.Diagnostics))
	finalizeStructuralCount(&result.Summary.Semantics, len(result.Semantics))
	finalizeStructuralCount(&result.Summary.ReadingOrder, len(result.ReadingOrder))
	return result, nil
}

// QueryBreakDetailsContext is the explicit bounded detailed-break query. The
// ordinary structural query always returns concise BreakDecision records only.
func (p LayoutPlan) QueryBreakDetailsContext(ctx context.Context, query StructuralQuery, limits BreakDetailLimits) (StructuralBreakDetailResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limits, err := normalizeBreakDetailLimits(limits)
	if err != nil {
		return StructuralBreakDetailResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return StructuralBreakDetailResult{}, err
	}
	// QueryStructure builds indexes and then scans the canonical tables. Charge
	// conservatively for those repeated passes so MaxWork remains a hard bound
	// even if its internal indexing strategy changes slightly.
	scanWork := 4 * (uint64(len(p.pages)) + uint64(len(p.fragments)) + uint64(len(p.lines)) + uint64(len(p.commands)) +
		uint64(len(p.breaks)) + uint64(len(p.diagnostics)) + uint64(len(p.semanticNodes)) +
		uint64(len(p.semanticFragments)) + uint64(len(p.readingOrder)))
	if scanWork > limits.MaxWork {
		return StructuralBreakDetailResult{}, ErrBreakDetailLimit
	}
	concise, err := p.QueryStructure(query)
	if err != nil {
		return StructuralBreakDetailResult{}, err
	}
	if uint64(len(concise.Breaks)) > uint64(limits.MaxBreaks) {
		return StructuralBreakDetailResult{}, ErrBreakDetailLimit
	}
	result := StructuralBreakDetailResult{Selector: query, Count: concise.Summary.Breaks,
		Details: make([]BreakDetail, 0, len(concise.Breaks))}
	work := scanWork
	for _, selected := range concise.Breaks {
		if err := ctx.Err(); err != nil {
			return StructuralBreakDetailResult{}, err
		}
		work += 4
		if work > limits.MaxWork {
			return StructuralBreakDetailResult{}, ErrBreakDetailLimit
		}
		result.Details = append(result.Details, detailForBreak(uint32(selected.Index), selected.Decision))
	}
	return result, nil
}

func finalizeStructuralCount(count *StructuralQueryCount, returned int) {
	count.Returned = uint32(returned)
	count.Truncated = uint64(returned) < count.Matches
}

func diagnosticMatchesStructuralQuery(diagnostic Diagnostic, query StructuralQuery, selected Fragment,
	fragments map[FragmentID]Fragment, nodeKeys map[NodeID]NodeKey, keyNodes map[NodeKey]NodeID,
) bool {
	location := diagnostic.Location
	candidate := Fragment{}
	if location.Fragment.Valid() {
		candidate = fragments[location.Fragment]
	} else {
		candidate.Node = location.Node
		candidate.Key = location.Key
		candidate.Instance = location.Instance
		candidate.Page = location.Page
		if !candidate.Node.Valid() && candidate.Key != "" {
			candidate.Node = keyNodes[candidate.Key]
		}
		if candidate.Key == "" && candidate.Node.Valid() {
			candidate.Key = nodeKeys[candidate.Node]
		}
	}
	if query.Fragment.Valid() {
		if location.Fragment.Valid() {
			return location.Fragment == query.Fragment
		}
		if !candidate.Node.Valid() && candidate.Key == "" && !candidate.Instance.Valid() {
			return false
		}
		return (!candidate.Node.Valid() || candidate.Node == selected.Node) &&
			(candidate.Key == "" || candidate.Key == selected.Key) &&
			(!candidate.Instance.Valid() || candidate.Instance == selected.Instance) &&
			(candidate.Page == 0 || candidate.Page == selected.Page)
	}
	if query.Node.Valid() && candidate.Node != query.Node {
		return false
	}
	if query.Key != "" && candidate.Key != query.Key {
		return false
	}
	if query.Instance.Valid() && candidate.Instance != query.Instance {
		return false
	}
	if query.Page != 0 && candidate.Page != query.Page {
		return false
	}
	return candidate.Node.Valid() || candidate.Key != "" || candidate.Instance.Valid() || candidate.Page != 0
}
