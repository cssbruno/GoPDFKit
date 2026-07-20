// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ExplainLayoutSchemaVersion         uint16 = 2
	ExplainLayoutMaxSelectors          uint32 = 32
	ExplainLayoutMaxCanonicalBytes     uint32 = 8 << 20
	ExplainLayoutDefaultCanonicalBytes uint32 = 1 << 20
)

var (
	ErrExplainLayoutNoSelectors   = errors.New("layoutengine: explain layout has no selectors")
	ErrExplainLayoutInvalidLimits = errors.New("layoutengine: explain layout limits are invalid")
	ErrExplainLayoutTooLarge      = errors.New("layoutengine: explain layout canonical JSON exceeds its byte limit")
)

// ExplainLayoutLimits bounds the total request independently from each
// StructuralQuery.MaxResults per-category bound.
type ExplainLayoutLimits struct {
	MaxSelectors      uint32 `json:"max_selectors"`
	MaxCanonicalBytes uint32 `json:"max_canonical_bytes"`
}

// DefaultExplainLayoutLimits returns conservative agent-inspection limits.
func DefaultExplainLayoutLimits() ExplainLayoutLimits {
	return ExplainLayoutLimits{
		MaxSelectors:      8,
		MaxCanonicalBytes: ExplainLayoutDefaultCanonicalBytes,
	}
}

type ExplainLayoutSelector struct {
	DiagnosticCode DiagnosticCode `json:"diagnostic_code,omitempty"`
	Node           NodeID         `json:"node,omitempty"`
	Key            NodeKey        `json:"key,omitempty"`
	Instance       InstanceID     `json:"instance,omitempty"`
	Fragment       FragmentID     `json:"fragment,omitempty"`
	Page           uint32         `json:"page,omitempty"`
	MaxResults     uint32         `json:"max_results"`
}

type ExplainLayoutCount struct {
	Matches   uint64 `json:"matches"`
	Returned  uint32 `json:"returned"`
	Truncated bool   `json:"truncated"`
}

type ExplainSelectionSummary struct {
	Pages        uint32             `json:"pages"`
	Fragments    ExplainLayoutCount `json:"fragments"`
	Lines        ExplainLayoutCount `json:"lines"`
	PageRegions  ExplainLayoutCount `json:"page_regions"`
	GridTracks   ExplainLayoutCount `json:"grid_tracks"`
	Commands     ExplainLayoutCount `json:"commands"`
	Breaks       ExplainLayoutCount `json:"breaks"`
	Diagnostics  ExplainLayoutCount `json:"diagnostics"`
	Semantics    ExplainLayoutCount `json:"semantics"`
	ReadingOrder ExplainLayoutCount `json:"reading_order"`
}

// ExplainEvidenceSummary describes the chain-expanded evidence returned by an
// explanation. Glyphs and images are derived from returned commands, while
// continuation matches are exact for the bounded selected-fragment seed set;
// break and diagnostic matches are the exact union of selector and chain
// evidence.
type ExplainEvidenceSummary struct {
	ContinuationFragments ExplainLayoutCount `json:"continuation_fragments"`
	Glyphs                ExplainLayoutCount `json:"glyphs"`
	Images                ExplainLayoutCount `json:"images"`
	Breaks                ExplainLayoutCount `json:"breaks"`
	Diagnostics           ExplainLayoutCount `json:"diagnostics"`
}

type ExplainSourceIdentity struct {
	Node     NodeID     `json:"node"`
	Key      NodeKey    `json:"key"`
	Instance InstanceID `json:"instance"`
	Source   SourceSpan `json:"source"`
}

// ExplainSemanticOwnership is the bounded semantic-owner path for one visual
// fragment. Roles are ordered from the directly associated owner toward the
// document root. Cell identifies an exact table-cell ancestor when present.
type ExplainSemanticOwnership struct {
	Owner       SemanticNodeID `json:"owner"`
	Roles       []SemanticRole `json:"roles"`
	Cell        SemanticNodeID `json:"cell,omitempty"`
	TableHeader bool           `json:"table_header,omitempty"`
}

// ExplainFragment is a flattened, detached fragment record including its
// owning page geometry.
type ExplainFragment struct {
	Index        uint64                    `json:"index"`
	PageIndex    uint32                    `json:"page_index"`
	PageSize     Size                      `json:"page_size"`
	ID           FragmentID                `json:"id"`
	Source       ExplainSourceIdentity     `json:"source_identity"`
	Semantic     *ExplainSemanticOwnership `json:"semantic_ownership,omitempty"`
	Page         uint32                    `json:"page"`
	Region       RegionID                  `json:"region"`
	Repeated     bool                      `json:"repeated,omitempty"`
	MarginBox    Rect                      `json:"margin_box"`
	BorderBox    Rect                      `json:"border_box"`
	PaddingBox   Rect                      `json:"padding_box"`
	ContentBox   Rect                      `json:"content_box"`
	Continuation FragmentContinuation      `json:"continuation"`
}

type ExplainLine struct {
	Index     uint64                `json:"index"`
	Page      uint32                `json:"page"`
	PageIndex uint32                `json:"page_index"`
	Source    ExplainSourceIdentity `json:"source_identity"`
	Region    RegionID              `json:"region"`
	Line      PlannedLine           `json:"line"`
}

type ExplainGridTrack struct {
	Index uint64           `json:"index"`
	Track PlannedGridTrack `json:"track"`
}

type ExplainPageRegion struct {
	Index  uint64            `json:"index"`
	Region PlannedPageRegion `json:"region"`
}

type ExplainCommand struct {
	Index                 uint64                `json:"index"`
	Page                  uint32                `json:"page"`
	PageIndex             uint32                `json:"page_index"`
	HasFragmentProvenance bool                  `json:"has_fragment_provenance"`
	Source                ExplainSourceIdentity `json:"source_identity"`
	Region                RegionID              `json:"region,omitempty"`
	Command               DisplayCommand        `json:"command"`
}

type ExplainGlyph struct {
	CommandIndex uint64           `json:"command_index"`
	RunIndex     uint32           `json:"run_index"`
	Run          CoreGlyphRun     `json:"run"`
	Font         CoreFontResource `json:"font"`
}

type ExplainImage struct {
	CommandIndex uint64        `json:"command_index"`
	ImageIndex   uint32        `json:"image_index"`
	Image        PlannedImage  `json:"image"`
	Resource     ImageResource `json:"resource"`
}

type ExplainBreak struct {
	Index    uint64        `json:"index"`
	Decision BreakDecision `json:"decision"`
}

type ExplainDiagnostic struct {
	Index      uint64     `json:"index"`
	Diagnostic Diagnostic `json:"diagnostic"`
}

// ExplainLayoutTarget is the evidence for one selector. Fragments, lines, and
// commands are direct StructuralQuery results. ContinuationFragments, Breaks,
// and Diagnostics add causal evidence for the returned fragment seed set.
type ExplainLayoutTarget struct {
	Selector              ExplainLayoutSelector         `json:"selector"`
	Selection             ExplainSelectionSummary       `json:"selection"`
	Evidence              ExplainEvidenceSummary        `json:"evidence"`
	Fragments             []ExplainFragment             `json:"fragments,omitempty"`
	ContinuationFragments []ExplainFragment             `json:"continuation_fragments,omitempty"`
	Lines                 []ExplainLine                 `json:"lines,omitempty"`
	PageRegions           []ExplainPageRegion           `json:"page_regions,omitempty"`
	GridTracks            []ExplainGridTrack            `json:"grid_tracks,omitempty"`
	Commands              []ExplainCommand              `json:"commands,omitempty"`
	Glyphs                []ExplainGlyph                `json:"glyphs,omitempty"`
	Images                []ExplainImage                `json:"images,omitempty"`
	Breaks                []ExplainBreak                `json:"breaks,omitempty"`
	Diagnostics           []ExplainDiagnostic           `json:"diagnostics,omitempty"`
	Semantics             []StructuralSemantic          `json:"semantics,omitempty"`
	ReadingOrder          []StructuralReadingOccurrence `json:"reading_order,omitempty"`
}

type explainChainIdentity struct {
	node     NodeID
	instance InstanceID
}

type explainChainContext struct {
	key   NodeKey
	pages map[uint32]struct{}
}

// LayoutExplanation is a deterministic, map-free explanation of exact plan
// evidence. It contains no raster interpretation or generated prose.
type LayoutExplanation struct {
	SchemaVersion uint16                `json:"schema_version"`
	PlanHash      string                `json:"plan_hash"`
	Limits        ExplainLayoutLimits   `json:"limits"`
	Targets       []ExplainLayoutTarget `json:"targets"`
}

// CanonicalJSON returns the stable map-free encoding and enforces the byte
// ceiling recorded by ExplainLayout.
func (e LayoutExplanation) CanonicalJSON() ([]byte, error) {
	if err := validateExplainLayoutLimits(e.Limits); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("layoutengine: encode layout explanation: %w", err)
	}
	if uint64(len(encoded)) > uint64(e.Limits.MaxCanonicalBytes) {
		return nil, fmt.Errorf("%w: encoded=%d limit=%d", ErrExplainLayoutTooLarge, len(encoded), e.Limits.MaxCanonicalBytes)
	}
	return encoded, nil
}

// ExplainLayout resolves selectors in supplied order and returns bounded,
// detached causal evidence. Each selector is validated by QueryStructure.
func (p LayoutPlan) ExplainLayout(selectors []StructuralQuery, limits ExplainLayoutLimits) (LayoutExplanation, error) {
	return p.ExplainLayoutContext(context.Background(), selectors, limits, ^uint64(0))
}

// ExplainLayoutContext is the cancellation-aware, work-bounded form used by
// untrusted agent read tools. MaxWork bounds conservative canonical-table
// visits rather than wall-clock time.
func (p LayoutPlan) ExplainLayoutContext(ctx context.Context, selectors []StructuralQuery, limits ExplainLayoutLimits, maxWork uint64) (LayoutExplanation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return LayoutExplanation{}, err
	}
	if len(selectors) == 0 {
		return LayoutExplanation{}, ErrExplainLayoutNoSelectors
	}
	if err := validateExplainLayoutLimits(limits); err != nil {
		return LayoutExplanation{}, err
	}
	if uint64(len(selectors)) > uint64(limits.MaxSelectors) {
		return LayoutExplanation{}, fmt.Errorf("%w: selectors=%d limit=%d", ErrExplainLayoutInvalidLimits, len(selectors), limits.MaxSelectors)
	}
	state := uint64(len(p.pages)) + uint64(len(p.fragments)) + uint64(len(p.lines)) + uint64(len(p.pageRegions)) + uint64(len(p.gridTracks)) + uint64(len(p.commands)) +
		uint64(len(p.breaks)) + uint64(len(p.diagnostics)) + uint64(len(p.semanticNodes)) + uint64(len(p.readingOrder)) + 1
	requiredWork := state * uint64(len(selectors)) * 8
	if maxWork == 0 || requiredWork > maxWork {
		return LayoutExplanation{}, fmt.Errorf("%w: required_work=%d max_work=%d", ErrExplainLayoutInvalidLimits, requiredWork, maxWork)
	}
	hash, err := p.Hash()
	if err != nil {
		return LayoutExplanation{}, fmt.Errorf("layoutengine: hash explained plan: %w", err)
	}
	explanation := LayoutExplanation{
		SchemaVersion: ExplainLayoutSchemaVersion,
		PlanHash:      hash.String(),
		Limits:        limits,
		Targets:       make([]ExplainLayoutTarget, 0, len(selectors)),
	}
	for index, selector := range selectors {
		if err := ctx.Err(); err != nil {
			return LayoutExplanation{}, err
		}
		query, queryErr := p.QueryStructure(selector)
		if queryErr != nil {
			return LayoutExplanation{}, fmt.Errorf("layoutengine: explain selector %d: %w", index, queryErr)
		}
		explanation.Targets = append(explanation.Targets, p.explainLayoutTarget(selector, query))
	}
	if err := ctx.Err(); err != nil {
		return LayoutExplanation{}, err
	}
	if _, err := explanation.CanonicalJSON(); err != nil {
		return LayoutExplanation{}, err
	}
	return explanation, nil
}

func validateExplainLayoutLimits(limits ExplainLayoutLimits) error {
	if limits.MaxSelectors == 0 || limits.MaxSelectors > ExplainLayoutMaxSelectors {
		return fmt.Errorf("%w: max selectors must be between 1 and %d", ErrExplainLayoutInvalidLimits, ExplainLayoutMaxSelectors)
	}
	if limits.MaxCanonicalBytes == 0 || limits.MaxCanonicalBytes > ExplainLayoutMaxCanonicalBytes {
		return fmt.Errorf("%w: max canonical bytes must be between 1 and %d", ErrExplainLayoutInvalidLimits, ExplainLayoutMaxCanonicalBytes)
	}
	return nil
}

func (p LayoutPlan) explainLayoutTarget(selector StructuralQuery, query StructuralQueryResult) ExplainLayoutTarget {
	target := ExplainLayoutTarget{
		Selector:  explainSelector(selector),
		Selection: explainSelectionSummary(query.Summary),
	}
	target.Semantics = append(target.Semantics, query.Semantics...)
	target.ReadingOrder = append(target.ReadingOrder, query.ReadingOrder...)
	pageSizes := make(map[uint32]Size, len(p.pages))
	fragmentsByID := make(map[FragmentID]Fragment, len(p.fragments))
	semanticByFragment := make(map[FragmentID]SemanticNodeID, len(p.semanticFragments))
	nodeKeys := make(map[NodeID]NodeKey)
	keyNodes := make(map[NodeKey]NodeID)
	fragmentIndexes := make(map[FragmentID]struct {
		index     uint64
		pageIndex uint32
	}, len(p.fragments))
	for _, page := range p.pages {
		pageSizes[page.Number] = page.Size
		end, _ := page.Fragments.end(len(p.fragments))
		for index := int(page.Fragments.Start); index < end; index++ {
			fragmentIndexes[p.fragments[index].ID] = struct {
				index     uint64
				pageIndex uint32
			}{uint64(index), uint32(index - int(page.Fragments.Start))} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		}
	}
	for _, fragment := range p.fragments {
		fragmentsByID[fragment.ID] = fragment
		nodeKeys[fragment.Node] = fragment.Key
		keyNodes[fragment.Key] = fragment.Node
	}
	for _, association := range p.semanticFragments {
		semanticByFragment[association.Fragment] = association.Semantic
	}
	for _, fragment := range query.Fragments {
		target.Fragments = append(target.Fragments, explainFragment(fragment.Index, fragment.PageIndex, pageSizes[fragment.Fragment.Page], fragment.Fragment,
			explainSemanticOwnership(fragment.Fragment.ID, semanticByFragment, p.semanticNodes)))
	}
	for _, line := range query.Lines {
		fragment := fragmentsByID[line.Line.Fragment]
		target.Lines = append(target.Lines, ExplainLine{
			Index: line.Index, Page: line.Page, PageIndex: line.PageIndex,
			Source: ExplainSourceIdentity{Node: line.Node, Key: line.Key, Instance: line.Instance, Source: fragment.Source},
			Region: line.Region, Line: line.Line,
		})
	}
	if selector.Page != 0 {
		for index, region := range p.pageRegions {
			if region.Page != selector.Page {
				continue
			}
			target.Selection.PageRegions.Matches++
			if uint32(len(target.PageRegions)) < selector.MaxResults { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				target.PageRegions = append(target.PageRegions, ExplainPageRegion{Index: uint64(index), Region: region})
			}
		}
		finalizeExplainCount(&target.Selection.PageRegions, len(target.PageRegions))
		for index, track := range p.gridTracks {
			if track.Page != selector.Page {
				continue
			}
			target.Selection.GridTracks.Matches++
			if uint32(len(target.GridTracks)) < selector.MaxResults { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				target.GridTracks = append(target.GridTracks, ExplainGridTrack{Index: uint64(index), Track: track})
			}
		}
		finalizeExplainCount(&target.Selection.GridTracks, len(target.GridTracks))
	}
	for _, command := range query.Commands {
		item := ExplainCommand{
			Index: command.Index, Page: command.Page, PageIndex: command.PageIndex,
			HasFragmentProvenance: command.HasFragmentProvenance,
			Source:                ExplainSourceIdentity{Node: command.Node, Key: command.Key, Instance: command.Instance},
			Region:                command.Region, Command: command.Command,
		}
		if command.HasFragmentProvenance {
			item.Source.Source = fragmentsByID[command.Command.Fragment].Source
		}
		target.Commands = append(target.Commands, item)
		switch command.Command.Kind {
		case CommandGlyphRun:
			run := p.glyphRuns[command.Command.Payload]
			run.Advances = cloneSlice(run.Advances)
			target.Glyphs = append(target.Glyphs, ExplainGlyph{
				CommandIndex: command.Index, RunIndex: command.Command.Payload,
				Run: run, Font: p.fonts[run.Font-1],
			})
		case CommandImage:
			image := p.images[command.Command.Payload]
			target.Images = append(target.Images, ExplainImage{
				CommandIndex: command.Index, ImageIndex: command.Command.Payload,
				Image: image, Resource: p.imageResources[image.Resource-1],
			})
		}
	}

	chainIdentities := make(map[explainChainIdentity]explainChainContext, len(query.Fragments))
	for _, selected := range query.Fragments {
		identity := explainChainIdentity{selected.Fragment.Node, selected.Fragment.Instance}
		if _, found := chainIdentities[identity]; !found {
			chainIdentities[identity] = explainChainContext{key: selected.Fragment.Key, pages: make(map[uint32]struct{})}
		}
	}
	chainIDs := make(map[FragmentID]struct{})
	for _, fragment := range p.fragments {
		identity := explainChainIdentity{fragment.Node, fragment.Instance}
		context, selected := chainIdentities[identity]
		if !selected {
			continue
		}
		context.pages[fragment.Page] = struct{}{}
		chainIdentities[identity] = context
		target.Evidence.ContinuationFragments.Matches++
		chainIDs[fragment.ID] = struct{}{}
		if uint32(len(target.ContinuationFragments)) < selector.MaxResults { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			position := fragmentIndexes[fragment.ID]
			target.ContinuationFragments = append(target.ContinuationFragments,
				explainFragment(position.index, position.pageIndex, pageSizes[fragment.Page], fragment,
					explainSemanticOwnership(fragment.ID, semanticByFragment, p.semanticNodes)))
		}
	}
	finalizeExplainCount(&target.Evidence.ContinuationFragments, len(target.ContinuationFragments))
	target.Evidence.Glyphs = exactExplainCount(len(target.Glyphs))
	target.Evidence.Images = exactExplainCount(len(target.Images))

	matchesSelector := func(fragment Fragment) bool {
		return (!selector.Node.Valid() || fragment.Node == selector.Node) &&
			(selector.Key == "" || fragment.Key == selector.Key) &&
			(!selector.Instance.Valid() || fragment.Instance == selector.Instance) &&
			(!selector.Fragment.Valid() || fragment.ID == selector.Fragment) &&
			(selector.Page == 0 || fragment.Page == selector.Page)
	}
	for index, decision := range p.breaks {
		_, preceding := chainIDs[decision.Preceding]
		_, triggering := chainIDs[decision.Triggering]
		direct := matchesSelector(fragmentsByID[decision.Preceding]) || matchesSelector(fragmentsByID[decision.Triggering])
		if !direct && !preceding && !triggering {
			continue
		}
		target.Evidence.Breaks.Matches++
		if uint32(len(target.Breaks)) < selector.MaxResults { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			target.Breaks = append(target.Breaks, ExplainBreak{Index: uint64(index), Decision: decision})
		}
	}
	finalizeExplainCount(&target.Evidence.Breaks, len(target.Breaks))
	selectedFragment := fragmentsByID[selector.Fragment]
	for index, diagnostic := range p.diagnostics {
		direct := diagnosticMatchesStructuralQuery(diagnostic, selector, selectedFragment, fragmentsByID, nodeKeys, keyNodes)
		if !direct && !diagnosticMatchesExplainChain(diagnostic, chainIDs, chainIdentities) {
			continue
		}
		target.Evidence.Diagnostics.Matches++
		if uint32(len(target.Diagnostics)) < selector.MaxResults { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			target.Diagnostics = append(target.Diagnostics, ExplainDiagnostic{
				Index: uint64(index), Diagnostic: cloneDiagnostic(diagnostic),
			})
		}
	}
	finalizeExplainCount(&target.Evidence.Diagnostics, len(target.Diagnostics))
	return target
}

func explainSelector(query StructuralQuery) ExplainLayoutSelector {
	return ExplainLayoutSelector{
		DiagnosticCode: query.DiagnosticCode,
		Node:           query.Node, Key: query.Key, Instance: query.Instance, Fragment: query.Fragment,
		Page: query.Page, MaxResults: query.MaxResults,
	}
}

func explainSelectionSummary(summary StructuralQuerySummary) ExplainSelectionSummary {
	return ExplainSelectionSummary{
		Pages: summary.Pages, Fragments: explainCount(summary.Fragments), Lines: explainCount(summary.Lines),
		Commands: explainCount(summary.Commands), Breaks: explainCount(summary.Breaks),
		Diagnostics: explainCount(summary.Diagnostics), Semantics: explainCount(summary.Semantics),
		ReadingOrder: explainCount(summary.ReadingOrder),
	}
}

func explainCount(count StructuralQueryCount) ExplainLayoutCount {
	return ExplainLayoutCount(count)
}

func exactExplainCount(count int) ExplainLayoutCount {
	return ExplainLayoutCount{Matches: uint64(count), Returned: uint32(count)} // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
}

func finalizeExplainCount(count *ExplainLayoutCount, returned int) {
	count.Returned = uint32(returned)                  // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
	count.Truncated = uint64(returned) < count.Matches // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
}

func explainFragment(index uint64, pageIndex uint32, pageSize Size, fragment Fragment, semantic *ExplainSemanticOwnership) ExplainFragment {
	return ExplainFragment{
		Index: index, PageIndex: pageIndex, PageSize: pageSize, ID: fragment.ID,
		Source:   ExplainSourceIdentity{Node: fragment.Node, Key: fragment.Key, Instance: fragment.Instance, Source: fragment.Source},
		Semantic: semantic,
		Page:     fragment.Page, Region: fragment.Region, Repeated: fragment.Repeated,
		MarginBox: fragment.MarginBox, BorderBox: fragment.BorderBox, PaddingBox: fragment.PaddingBox,
		ContentBox: fragment.ContentBox, Continuation: fragment.Continuation,
	}
}

func explainSemanticOwnership(fragment FragmentID, owners map[FragmentID]SemanticNodeID, nodes []SemanticNode) *ExplainSemanticOwnership {
	owner := owners[fragment]
	if !owner.Valid() {
		return nil
	}
	result := &ExplainSemanticOwnership{Owner: owner, Roles: make([]SemanticRole, 0, 4)}
	for id, depth := owner, uint32(0); id.Valid() && depth <= SemanticMaxDepth; depth++ {
		if uint64(id) > uint64(len(nodes)) {
			return nil
		}
		node := nodes[id-1]
		result.Roles = append(result.Roles, node.Role)
		if node.Role == SemanticRoleCell && !result.Cell.Valid() {
			result.Cell = node.ID
			result.TableHeader = node.Attributes.TableHeader
		}
		id = node.Parent
	}
	return result
}

func diagnosticMatchesExplainChain(diagnostic Diagnostic, chainIDs map[FragmentID]struct{}, identities map[explainChainIdentity]explainChainContext) bool {
	location := diagnostic.Location
	if location.Fragment.Valid() {
		_, found := chainIDs[location.Fragment]
		return found
	}
	for identity, context := range identities {
		if location.Node.Valid() && location.Node != identity.node ||
			location.Key != "" && location.Key != context.key ||
			location.Instance.Valid() && location.Instance != identity.instance {
			continue
		}
		if location.Page != 0 {
			if _, found := context.pages[location.Page]; !found {
				continue
			}
		}
		if location.Node.Valid() || location.Key != "" || location.Instance.Valid() || location.Page != 0 {
			return true
		}
	}
	return false
}
