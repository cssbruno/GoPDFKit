// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	// LayoutPlanSchemaVersion pins the canonical JSON shape.
	LayoutPlanSchemaVersion uint16 = 16
	// PlannerVersion pins automatic measurement, fragmentation, and pagination
	// semantics independently from the storage shape.
	PlannerVersion = "layoutengine/0.1"
	// PainterContractVersion pins the meaning of exact display commands. A
	// painter may have its own implementation version, but it must implement
	// this contract to consume the plan without performing layout.
	PainterContractVersion = "display-list/0.3"
)

// IndexRange selects a contiguous range in a plan-owned slice.
type IndexRange struct {
	Start uint32 `json:"start"`
	Count uint32 `json:"count"`
}

func (r IndexRange) end(limit int) (int, bool) {
	end := uint64(r.Start) + uint64(r.Count)
	if end > uint64(limit) { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		return 0, false
	}
	return int(end), true // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
}

// PlannedPage describes one finalized page and its slices in the plan.
type PlannedPage struct {
	Number    uint32     `json:"number"`
	Size      Size       `json:"size"`
	Fragments IndexRange `json:"fragments"`
	Lines     IndexRange `json:"lines"`
	Commands  IndexRange `json:"commands"`
}

// FragmentContinuation describes which portion of a fragmented instance was
// placed on a page.
type FragmentContinuation string

const (
	ContinuationWhole  FragmentContinuation = "whole"
	ContinuationStart  FragmentContinuation = "start"
	ContinuationMiddle FragmentContinuation = "middle"
	ContinuationEnd    FragmentContinuation = "end"
)

func (c FragmentContinuation) valid() bool {
	return c == ContinuationWhole || c == ContinuationStart ||
		c == ContinuationMiddle || c == ContinuationEnd
}

// Fragment is one positioned, plan-local projection of an expanded node.
type Fragment struct {
	ID           FragmentID           `json:"id"`
	Node         NodeID               `json:"node"`
	Key          NodeKey              `json:"key"`
	Instance     InstanceID           `json:"instance"`
	Page         uint32               `json:"page"`
	Region       RegionID             `json:"region"`
	MarginBox    Rect                 `json:"margin_box"`
	BorderBox    Rect                 `json:"border_box"`
	PaddingBox   Rect                 `json:"padding_box"`
	ContentBox   Rect                 `json:"content_box"`
	Source       SourceSpan           `json:"source"`
	Continuation FragmentContinuation `json:"continuation"`
	Repeated     bool                 `json:"repeated,omitempty"`
}

// PlannedLine is one exact, positioned line allocated to a fragment. Index is
// zero-based within the owning paragraph instance and remains stable across
// page fragmentation. Bounds and Baseline are absolute page coordinates;
// glyph payloads will be attached through display-command tables rather than
// causing the painter to reshape or rewrap this line.
type PlannedLine struct {
	Fragment FragmentID `json:"fragment"`
	Index    uint32     `json:"index"`
	Bounds   Rect       `json:"bounds"`
	Baseline Fixed      `json:"baseline"`
	Source   SourceSpan `json:"source"`
}

// PlannedPageRegion is one exact enabled page-master band. Master is optional
// for planners that receive only a concrete body rectangle.
type PlannedPageRegion struct {
	Page   uint32       `json:"page"`
	Region RegionID     `json:"region"`
	Bounds Rect         `json:"bounds"`
	Master PageMasterID `json:"master,omitempty"`
}

// GridTrackAxis identifies one resolved column or row track.
type GridTrackAxis string

const (
	GridTrackColumn GridTrackAxis = "column"
	GridTrackRow    GridTrackAxis = "row"
)

func (axis GridTrackAxis) valid() bool { return axis == GridTrackColumn || axis == GridTrackRow }

// PlannedGridTrack is exact, page-positioned track geometry retained after
// sizing. Group is a one-based plan-local grid occurrence; Index is zero-based
// within one group and axis. GapAfter is the resolved gap to the next track.
type PlannedGridTrack struct {
	Group    uint32        `json:"group"`
	Page     uint32        `json:"page"`
	Region   RegionID      `json:"region"`
	Axis     GridTrackAxis `json:"axis"`
	Index    uint32        `json:"index"`
	Bounds   Rect          `json:"bounds"`
	GapAfter Fixed         `json:"gap_after,omitempty"`
}

// DisplayCommandKind identifies a no-layout painter operation. Payload is an
// index into a future command-specific, plan-owned table.
type DisplayCommandKind string

const (
	CommandSaveState    DisplayCommandKind = "save_state"
	CommandRestoreState DisplayCommandKind = "restore_state"
	CommandTransform    DisplayCommandKind = "transform"
	CommandClip         DisplayCommandKind = "clip"
	CommandFillPath     DisplayCommandKind = "fill_path"
	CommandStrokePath   DisplayCommandKind = "stroke_path"
	CommandGlyphRun     DisplayCommandKind = "glyph_run"
	CommandImage        DisplayCommandKind = "image"
	CommandLink         DisplayCommandKind = "link"
)

func (k DisplayCommandKind) valid() bool {
	switch k {
	case CommandSaveState, CommandRestoreState, CommandTransform, CommandClip, CommandFillPath,
		CommandStrokePath, CommandGlyphRun, CommandImage, CommandLink:
		return true
	default:
		return false
	}
}

// DisplayCommand is an exact positioned painter instruction. Fragment may be
// zero for page-level state or decoration commands.
type DisplayCommand struct {
	Kind     DisplayCommandKind `json:"kind"`
	Fragment FragmentID         `json:"fragment,omitempty"`
	Bounds   Rect               `json:"bounds"`
	Payload  uint32             `json:"payload"`
}

// BreakReason is a stable, machine-readable explanation for a page
// transition. It is intentionally narrower than a human diagnostic message.
type BreakReason string

const (
	BreakInsufficientRemainingBodySpace BreakReason = "insufficient_remaining_body_space"
	BreakPreviousFragmentOverflow       BreakReason = "previous_fragment_overflow"
	BreakPaginationConstraint           BreakReason = "pagination_constraint"
	BreakExplicitPageBreak              BreakReason = "explicit_page_break"
)

func (r BreakReason) valid() bool {
	return r == BreakInsufficientRemainingBodySpace || r == BreakPreviousFragmentOverflow ||
		r == BreakPaginationConstraint || r == BreakExplicitPageBreak
}

// BreakDecision records the causal evidence for one transition between
// consecutive pages in a region. Required and Available describe the
// triggering fragment for an ordinary space break. For a pagination-policy
// break, Required is the total withheld paragraph continuation height, which
// truthfully explains why the policy selected an earlier break even when its
// next individual line could fit. After an oversized predecessor, Available
// is zero because no further content may be placed on that page. Explicit
// page breaks use zero for both heights because space did not cause them.
type BreakDecision struct {
	Reason     BreakReason `json:"reason"`
	FromPage   uint32      `json:"from_page"`
	ToPage     uint32      `json:"to_page"`
	Region     RegionID    `json:"region"`
	Preceding  FragmentID  `json:"preceding_fragment"`
	Triggering FragmentID  `json:"triggering_fragment"`
	Required   Fixed       `json:"required_height"`
	Available  Fixed       `json:"available_height"`
}

// LayoutPlanInput is copied by NewLayoutPlan. Mutating it afterward cannot
// mutate the plan.
type LayoutPlanInput struct {
	DeterministicInputs *DeterministicInputManifest
	Pages               []PlannedPage
	Fragments           []Fragment
	Lines               []PlannedLine
	PageRegions         []PlannedPageRegion
	GridTracks          []PlannedGridTrack
	Fonts               []CoreFontResource
	GlyphRuns           []CoreGlyphRun
	ImageResources      []ImageResource
	Images              []PlannedImage
	Destinations        []PlannedDestination
	Links               []PlannedLink
	Paths               []PlannedPath
	Transforms          []Transform
	Clips               []PlannedClip
	Fills               []PlannedFill
	Strokes             []PlannedStroke
	Commands            []DisplayCommand
	Breaks              []BreakDecision
	Diagnostics         []Diagnostic
	SemanticNodes       []SemanticNode
	SemanticFragments   []SemanticFragmentAssociation
	ReadingOrder        []ReadingOccurrence
}

// LayoutPlan is immutable by convention: construction validates and copies
// all input, while every projection returned to callers is another copy.
type LayoutPlan struct {
	deterministicInputs    DeterministicInputManifest
	hasDeterministicInputs bool
	pages                  []PlannedPage
	fragments              []Fragment
	lines                  []PlannedLine
	pageRegions            []PlannedPageRegion
	gridTracks             []PlannedGridTrack
	fonts                  []CoreFontResource
	glyphRuns              []CoreGlyphRun
	imageResources         []ImageResource
	images                 []PlannedImage
	destinations           []PlannedDestination
	links                  []PlannedLink
	paths                  []PlannedPath
	transforms             []Transform
	clips                  []PlannedClip
	fills                  []PlannedFill
	strokes                []PlannedStroke
	commands               []DisplayCommand
	breaks                 []BreakDecision
	diagnostics            []Diagnostic
	semanticNodes          []SemanticNode
	semanticFragments      []SemanticFragmentAssociation
	readingOrder           []ReadingOccurrence
	provenance             []ProvenanceEntry
	fragmentProvenance     []ProvenanceID
	lineProvenance         []ProvenanceID
}

// LayoutPlanProjection is the versioned, map-free canonical representation
// used for persistence tests and hashing.
type LayoutPlanProjection struct {
	SchemaVersion          uint16                        `json:"schema_version"`
	PlannerVersion         string                        `json:"planner_version"`
	PainterContractVersion string                        `json:"painter_contract_version"`
	DeterministicInputs    *DeterministicInputManifest   `json:"deterministic_inputs,omitempty"`
	Pages                  []PlannedPage                 `json:"pages,omitempty"`
	Fragments              []Fragment                    `json:"fragments,omitempty"`
	Lines                  []PlannedLine                 `json:"lines,omitempty"`
	PageRegions            []PlannedPageRegion           `json:"page_regions,omitempty"`
	GridTracks             []PlannedGridTrack            `json:"grid_tracks,omitempty"`
	Fonts                  []CoreFontResource            `json:"fonts,omitempty"`
	GlyphRuns              []CoreGlyphRun                `json:"glyph_runs,omitempty"`
	ImageResources         []ImageResource               `json:"image_resources,omitempty"`
	Images                 []PlannedImage                `json:"images,omitempty"`
	Destinations           []PlannedDestination          `json:"destinations,omitempty"`
	Links                  []PlannedLink                 `json:"links,omitempty"`
	Paths                  []PlannedPath                 `json:"paths,omitempty"`
	Transforms             []Transform                   `json:"transforms,omitempty"`
	Clips                  []PlannedClip                 `json:"clips,omitempty"`
	Fills                  []PlannedFill                 `json:"fills,omitempty"`
	Strokes                []PlannedStroke               `json:"strokes,omitempty"`
	Commands               []DisplayCommand              `json:"commands,omitempty"`
	Breaks                 []BreakDecision               `json:"breaks,omitempty"`
	Diagnostics            []Diagnostic                  `json:"diagnostics,omitempty"`
	SemanticNodes          []SemanticNode                `json:"semantic_nodes,omitempty"`
	SemanticFragments      []SemanticFragmentAssociation `json:"semantic_fragments,omitempty"`
	ReadingOrder           []ReadingOccurrence           `json:"reading_order,omitempty"`
	Provenance             []ProvenanceEntry             `json:"provenance,omitempty"`
	FragmentProvenance     []ProvenanceID                `json:"fragment_provenance,omitempty"`
	LineProvenance         []ProvenanceID                `json:"line_provenance,omitempty"`
}

// PlanHash is the SHA-256 digest of a canonical plan projection.
type PlanHash [sha256.Size]byte

func (h PlanHash) String() string { return hex.EncodeToString(h[:]) }

// NewLayoutPlan validates a synthetic or planned projection and takes
// ownership by copying it.
func NewLayoutPlan(input LayoutPlanInput) (LayoutPlan, error) {
	plan := LayoutPlan{
		pages:             cloneSlice(input.Pages),
		fragments:         cloneSlice(input.Fragments),
		lines:             cloneSlice(input.Lines),
		pageRegions:       cloneSlice(input.PageRegions),
		gridTracks:        cloneSlice(input.GridTracks),
		fonts:             cloneFontResources(input.Fonts),
		glyphRuns:         cloneCoreGlyphRuns(input.GlyphRuns),
		imageResources:    cloneSlice(input.ImageResources),
		images:            clonePlannedImages(input.Images),
		destinations:      cloneSlice(input.Destinations),
		links:             cloneSlice(input.Links),
		paths:             clonePlannedPaths(input.Paths),
		transforms:        cloneSlice(input.Transforms),
		clips:             cloneSlice(input.Clips),
		fills:             cloneSlice(input.Fills),
		strokes:           clonePlannedStrokes(input.Strokes),
		commands:          cloneSlice(input.Commands),
		breaks:            cloneSlice(input.Breaks),
		semanticNodes:     cloneSlice(input.SemanticNodes),
		semanticFragments: cloneSlice(input.SemanticFragments),
		readingOrder:      cloneSlice(input.ReadingOrder),
	}
	for index := range plan.fragments {
		if plan.fragments[index].MarginBox == (Rect{}) {
			plan.fragments[index].MarginBox = plan.fragments[index].BorderBox
		}
		if plan.fragments[index].PaddingBox == (Rect{}) {
			plan.fragments[index].PaddingBox = plan.fragments[index].ContentBox
		}
	}
	if input.DeterministicInputs != nil {
		plan.deterministicInputs = cloneDeterministicInputs(*input.DeterministicInputs)
		plan.hasDeterministicInputs = true
	}
	if len(input.Diagnostics) != 0 {
		plan.diagnostics = make([]Diagnostic, len(input.Diagnostics))
		for i, diagnostic := range input.Diagnostics {
			plan.diagnostics[i] = cloneDiagnostic(diagnostic)
		}
	}
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	plan.provenance, plan.fragmentProvenance, plan.lineProvenance = buildCompactProvenance(plan.fragments, plan.lines)
	return plan, nil
}

func cloneSlice[T any](values []T) []T {
	if len(values) == 0 {
		return nil
	}
	return append([]T(nil), values...)
}

// Validate checks referential integrity, page ownership, geometry, identity
// collisions, and contiguous page projections.
func (p LayoutPlan) Validate() error {
	if p.hasDeterministicInputs {
		if err := p.deterministicInputs.validate(); err != nil {
			return err
		}
		catalog, err := resourceCatalogFromProjection(p.fonts, p.imageResources)
		if err != nil || catalog.ID != p.deterministicInputs.ResourceCatalog.ID {
			return errors.New("layoutengine: deterministic resource catalog does not match exact plan resources")
		}
		if len(p.pages) == 0 {
			return errors.New("layoutengine: deterministic page profile requires at least one exact plan page")
		}
		for _, page := range p.pages {
			if page.Size.Width != p.deterministicInputs.PageProfile.Width || page.Size.Height != p.deterministicInputs.PageProfile.Height {
				return errors.New("layoutengine: deterministic page profile does not match exact plan page dimensions")
			}
		}
	}
	if err := validateCoreFonts(p.fonts); err != nil {
		return err
	}
	if err := validateImageResources(p.imageResources); err != nil {
		return err
	}
	if err := validateDisplayGraphics(p.paths, p.transforms, p.clips, p.fills, p.strokes); err != nil {
		return err
	}
	if err := validatePlannedGridTracks(p.gridTracks, len(p.pages)); err != nil {
		return err
	}
	fragmentIDs := make(map[FragmentID]Fragment, len(p.fragments))
	fragmentLineCounts := make(map[FragmentID]uint32, len(p.fragments))
	nodeKeys := make(map[NodeID]NodeKey)
	keyNodes := make(map[NodeKey]NodeID)
	seenFragmentInstances := make(map[struct {
		node     NodeID
		instance InstanceID
	}]bool)
	for i, fragment := range p.fragments {
		path := fmt.Sprintf("fragments[%d]", i)
		if !fragment.ID.Valid() || !fragment.Node.Valid() || !fragment.Instance.Valid() {
			return planError(path, "has an absent identity")
		}
		if err := validateTextIdentity("fragment node key", string(fragment.Key)); err != nil {
			return planError(path, err.Error())
		}
		if err := validateTextIdentity("fragment instance ID", string(fragment.Instance)); err != nil {
			return planError(path, err.Error())
		}
		if _, exists := fragmentIDs[fragment.ID]; exists {
			return planError(path, fmt.Sprintf("duplicates fragment ID %d", fragment.ID))
		}
		if key, exists := nodeKeys[fragment.Node]; exists && key != fragment.Key {
			return planError(path, "reuses a node ID with another key")
		}
		if node, exists := keyNodes[fragment.Key]; exists && node != fragment.Node {
			return planError(path, "reuses a node key with another node ID")
		}
		if fragment.Page == 0 || uint64(fragment.Page) > uint64(len(p.pages)) {
			return planError(path, "references an invalid page")
		}
		if !fragment.Region.Valid() {
			return planError(path, "references an invalid region")
		}
		if err := fragment.BorderBox.Validate(); err != nil {
			return planError(path+".border_box", err.Error())
		}
		if err := fragment.MarginBox.Validate(); err != nil {
			return planError(path+".margin_box", err.Error())
		}
		if err := fragment.PaddingBox.Validate(); err != nil {
			return planError(path+".padding_box", err.Error())
		}
		if err := fragment.ContentBox.Validate(); err != nil {
			return planError(path+".content_box", err.Error())
		}
		if !fragmentBoxContains(fragment.MarginBox, fragment.BorderBox) || !fragmentBoxContains(fragment.BorderBox, fragment.PaddingBox) || !fragmentBoxContains(fragment.PaddingBox, fragment.ContentBox) {
			return planError(path, "box-model rectangles are not nested margin/border/padding/content")
		}
		if err := fragment.Source.Validate(); err != nil {
			return planError(path+".source", err.Error())
		}
		if !fragment.Continuation.valid() {
			return planError(path, "has an invalid continuation")
		}
		identity := struct {
			node     NodeID
			instance InstanceID
		}{fragment.Node, fragment.Instance}
		if fragment.Repeated && !seenFragmentInstances[identity] {
			return planError(path, "repeated fragment has no earlier original")
		}
		seenFragmentInstances[identity] = true
		fragmentIDs[fragment.ID] = fragment
		nodeKeys[fragment.Node] = fragment.Key
		keyNodes[fragment.Key] = fragment.Node
	}

	lineIndexes := make(map[struct {
		node     NodeID
		instance InstanceID
	}]uint32)
	lineIndexSeen := make(map[struct {
		node     NodeID
		instance InstanceID
	}]bool)
	closedLineFragments := make(map[FragmentID]bool)
	fragmentCursor, lineCursor, commandCursor := 0, 0, 0
	commandPages := make([]uint32, len(p.commands))
	for i, page := range p.pages {
		path := fmt.Sprintf("pages[%d]", i)
		if page.Number != uint32(i+1) {
			return planError(path, "page numbers are not consecutive and one-based")
		}
		if err := page.Size.Validate(); err != nil || page.Size.IsEmpty() {
			return planError(path+".size", "page size must have positive extents")
		}
		fragmentEnd, ok := page.Fragments.end(len(p.fragments))
		if !ok || int(page.Fragments.Start) != fragmentCursor {
			return planError(path+".fragments", "range is invalid or non-contiguous")
		}
		for j := fragmentCursor; j < fragmentEnd; j++ {
			if p.fragments[j].Page != page.Number {
				return planError(path+".fragments", "contains a fragment owned by another page")
			}
		}
		fragmentCursor = fragmentEnd

		lineEnd, ok := page.Lines.end(len(p.lines))
		if !ok || int(page.Lines.Start) != lineCursor {
			return planError(path+".lines", "range is invalid or non-contiguous")
		}
		var activeLineFragment FragmentID
		for j := lineCursor; j < lineEnd; j++ {
			line := p.lines[j]
			linePath := fmt.Sprintf("lines[%d]", j)
			fragment, exists := fragmentIDs[line.Fragment]
			if !exists || fragment.Page != page.Number {
				return planError(linePath, "references a missing or cross-page fragment")
			}
			if activeLineFragment != line.Fragment {
				if closedLineFragments[line.Fragment] {
					return planError(linePath, "returns to a non-contiguous fragment line group")
				}
				if activeLineFragment.Valid() {
					closedLineFragments[activeLineFragment] = true
				}
				activeLineFragment = line.Fragment
			}
			if err := line.Bounds.Validate(); err != nil {
				return planError(linePath+".bounds", err.Error())
			}
			bottom, err := line.Bounds.Bottom()
			if err != nil || line.Baseline < line.Bounds.Y || line.Baseline > bottom {
				return planError(linePath, "baseline lies outside line bounds")
			}
			if err := line.Source.Validate(); err != nil {
				return planError(linePath+".source", err.Error())
			}
			identity := struct {
				node     NodeID
				instance InstanceID
			}{fragment.Node, fragment.Instance}
			if fragment.Repeated {
				want := fragmentLineCounts[line.Fragment]
				if line.Index != want {
					return planError(linePath, fmt.Sprintf("repeated fragment line index is %d, want %d", line.Index, want))
				}
			} else if lineIndexSeen[identity] {
				want := lineIndexes[identity] + 1
				if line.Index != want {
					return planError(linePath, fmt.Sprintf("paragraph line index is %d, want %d", line.Index, want))
				}
			} else if line.Index != 0 {
				return planError(linePath, "first paragraph line index is not zero")
			}
			lineIndexes[identity] = line.Index
			lineIndexSeen[identity] = true
			fragmentLineCounts[line.Fragment]++
		}
		if activeLineFragment.Valid() {
			closedLineFragments[activeLineFragment] = true
		}
		lineCursor = lineEnd

		commandEnd, ok := page.Commands.end(len(p.commands))
		if !ok || int(page.Commands.Start) != commandCursor {
			return planError(path+".commands", "range is invalid or non-contiguous")
		}
		for j := commandCursor; j < commandEnd; j++ {
			command := p.commands[j]
			commandPages[j] = page.Number
			if !command.Kind.valid() {
				return planError(fmt.Sprintf("commands[%d]", j), "has an invalid kind")
			}
			if err := command.Bounds.Validate(); err != nil {
				return planError(fmt.Sprintf("commands[%d].bounds", j), err.Error())
			}
			if command.Fragment.Valid() {
				fragment, exists := fragmentIDs[command.Fragment]
				if !exists || fragment.Page != page.Number {
					return planError(fmt.Sprintf("commands[%d]", j), "references a missing or cross-page fragment")
				}
			}
		}
		commandCursor = commandEnd
	}
	if fragmentCursor != len(p.fragments) || lineCursor != len(p.lines) || commandCursor != len(p.commands) {
		return planError("pages", "page ranges do not cover the plan")
	}
	if err := validatePlannedPageRegions(p.pageRegions, p.pages); err != nil {
		return err
	}
	if err := validatePlannedLinks(p.pages, fragmentIDs, p.destinations, p.links); err != nil {
		return err
	}
	if err := validateSemantics(p.semanticNodes, p.semanticFragments, p.readingOrder, p.pages, fragmentIDs, p.destinations, p.links); err != nil {
		return err
	}
	fontCount := uint64(len(p.fonts))
	fontReferences := make([]uint32, len(p.fonts))
	var activeLine uint32
	var activeLineWidth Fixed
	lineActive := false
	for index, run := range p.glyphRuns {
		path := fmt.Sprintf("glyph_runs[%d]", index)
		if index > 0 && run.Line < p.glyphRuns[index-1].Line {
			return planError(path+".line", "glyph runs are not in global line order")
		}
		if uint64(run.Line) >= uint64(len(p.lines)) {
			return planError(path+".line", "references a missing planned line")
		}
		if !run.Font.Valid() || uint64(run.Font) > fontCount {
			return planError(path+".font", "references a missing font resource")
		}
		fontReferences[run.Font-1]++
		if run.FontSize <= 0 {
			return planError(path+".font_size", "must be positive")
		}
		if !run.Color.Set && (run.Color.R != 0 || run.Color.G != 0 || run.Color.B != 0) {
			return planError(path+".color", "unset color must have zero RGB components")
		}
		if run.Opacity < 0 || run.Opacity > Fixed(FixedScale) {
			return planError(path+".opacity", "must be between zero and one")
		}
		line := p.lines[run.Line]
		if !lineActive || run.Line != activeLine {
			if lineActive && activeLineWidth != p.lines[activeLine].Bounds.Width {
				return planError(path+".line", "preceding line glyph-run advances do not cover its exact width")
			}
			activeLine, activeLineWidth, lineActive = run.Line, 0, true
		}
		expectedX, err := line.Bounds.X.Add(activeLineWidth)
		if err != nil || run.Origin != (Point{X: expectedX, Y: line.Baseline}) {
			return planError(path+".origin", fmt.Sprintf("does not continue the owning line at its exact baseline (codes %q got %d,%d want %d,%d)", run.Codes, run.Origin.X, run.Origin.Y, expectedX, line.Baseline))
		}
		if run.Source != line.Source {
			return planError(path+".source", "does not match the owning line provenance")
		}
		if err := run.Source.Validate(); err != nil {
			return planError(path+".source", err.Error())
		}
		resource := p.fonts[run.Font-1]
		glyphCount := resource.GlyphCount(run.Codes)
		if len(run.Codes) == 0 || !utf8.ValidString(run.Codes) || len(run.Advances) != glyphCount {
			return planError(path, "codes and advances must be non-empty and have equal lengths")
		}
		var width Fixed
		codeIndex := 0
		for _, code := range run.Codes {
			if resource.IsEmbeddedUTF8() {
				if code < 0x20 || code == utf8.RuneError || code > 0xffff {
					return planError(path+".codes", "contains an unsupported embedded-font Unicode scalar")
				}
			} else if code < 0x20 || code > 0x7e {
				return planError(path+".codes", "contains a non-printable core-font code")
			}
			advance := run.Advances[codeIndex]
			if advance < 0 {
				return planError(fmt.Sprintf("%s.advances[%d]", path, codeIndex), "must be non-negative")
			}
			var err error
			width, err = width.Add(advance)
			if err != nil {
				return planError(path+".advances", "sum overflows fixed-point geometry")
			}
			codeIndex++
		}
		activeLineWidth, err = activeLineWidth.Add(width)
		if err != nil || activeLineWidth > line.Bounds.Width {
			return planError(path+".advances", "cumulative runs exceed the owning line width")
		}
	}
	if lineActive && activeLineWidth != p.lines[activeLine].Bounds.Width {
		return planError("glyph_runs", "final line glyph-run advances do not cover its exact width")
	}
	for index, count := range fontReferences {
		if count == 0 {
			return planError(fmt.Sprintf("fonts[%d]", index), "is not referenced by a glyph run")
		}
	}
	imageResourceReferences := make([]uint32, len(p.imageResources))
	for index, image := range p.images {
		path := fmt.Sprintf("images[%d]", index)
		if !image.Resource.Valid() || uint64(image.Resource) > uint64(len(p.imageResources)) {
			return planError(path+".resource", "references a missing image resource")
		}
		fragment, exists := fragmentIDs[image.Fragment]
		if !exists {
			return planError(path+".fragment", "references a missing fragment")
		}
		if err := image.Bounds.Validate(); err != nil || image.Bounds.Width == 0 || image.Bounds.Height == 0 {
			return planError(path+".bounds", "must have positive valid extents")
		}
		if image.Opacity < 0 || image.Opacity > Fixed(FixedScale) {
			return planError(path+".opacity", "must be between zero and one")
		}
		if err := validateImageCrop(path, image, p.imageResources[image.Resource-1]); err != nil {
			return err
		}
		if err := image.Source.Validate(); err != nil {
			return planError(path+".source", err.Error())
		}
		if image.Source != fragment.Source {
			return planError(path+".source", "does not match the owning fragment provenance")
		}
		imageResourceReferences[image.Resource-1]++
	}
	runReferences := make([]uint32, len(p.glyphRuns))
	imageReferences := make([]uint32, len(p.images))
	linkReferences := make([]uint32, len(p.links))
	pathReferences := make([]uint32, len(p.paths))
	transformReferences := make([]uint32, len(p.transforms))
	clipReferences := make([]uint32, len(p.clips))
	fillReferences := make([]uint32, len(p.fills))
	strokeReferences := make([]uint32, len(p.strokes))
	graphicsMode := len(p.paths) != 0 || len(p.transforms) != 0 || len(p.clips) != 0 || len(p.fills) != 0 || len(p.strokes) != 0
	stateDepth := 0
	var statePage uint32
	for index, command := range p.commands {
		if commandPages[index] != statePage {
			if stateDepth != 0 {
				return planError(fmt.Sprintf("commands[%d]", index), "previous page has unbalanced saved state")
			}
			statePage = commandPages[index]
		}
		switch command.Kind {
		case CommandSaveState:
			if !graphicsMode {
				continue
			}
			if command.Payload != 0 || command.Fragment.Valid() || command.Bounds != (Rect{}) {
				return planError(fmt.Sprintf("commands[%d]", index), "save_state must not carry payload or geometry")
			}
			stateDepth++
		case CommandRestoreState:
			if !graphicsMode {
				continue
			}
			if command.Payload != 0 || command.Fragment.Valid() || command.Bounds != (Rect{}) || stateDepth == 0 {
				return planError(fmt.Sprintf("commands[%d]", index), "restore_state is unmatched or carries payload/geometry")
			}
			stateDepth--
		case CommandTransform:
			if uint64(command.Payload) >= uint64(len(p.transforms)) || command.Fragment.Valid() || command.Bounds != (Rect{}) || stateDepth == 0 {
				return planError(fmt.Sprintf("commands[%d]", index), "transform requires saved state and one valid transform payload")
			}
			transformReferences[command.Payload]++
		case CommandClip:
			if !graphicsMode {
				continue
			}
			if uint64(command.Payload) >= uint64(len(p.clips)) || stateDepth == 0 {
				return planError(fmt.Sprintf("commands[%d]", index), "clip requires saved state and one valid clip payload")
			}
			clip := p.clips[command.Payload]
			if command.Fragment != clip.Fragment || command.Bounds != p.paths[clip.Path].Bounds {
				return planError(fmt.Sprintf("commands[%d]", index), "does not match its clip path and fragment")
			}
			clipReferences[command.Payload]++
			pathReferences[clip.Path]++
		case CommandFillPath:
			if !graphicsMode {
				continue
			}
			if uint64(command.Payload) >= uint64(len(p.fills)) {
				return planError(fmt.Sprintf("commands[%d]", index), "references a missing fill")
			}
			fill := p.fills[command.Payload]
			if command.Fragment != fill.Fragment || command.Bounds != p.paths[fill.Path].Bounds {
				return planError(fmt.Sprintf("commands[%d]", index), "does not match its fill path and fragment")
			}
			fillReferences[command.Payload]++
			pathReferences[fill.Path]++
		case CommandStrokePath:
			if !graphicsMode {
				continue
			}
			if uint64(command.Payload) >= uint64(len(p.strokes)) {
				return planError(fmt.Sprintf("commands[%d]", index), "references a missing stroke")
			}
			stroke := p.strokes[command.Payload]
			if command.Fragment != stroke.Fragment || command.Bounds != p.paths[stroke.Path].Bounds {
				return planError(fmt.Sprintf("commands[%d]", index), "does not match its stroke path and fragment")
			}
			strokeReferences[command.Payload]++
			pathReferences[stroke.Path]++
		case CommandGlyphRun:
			if uint64(command.Payload) >= uint64(len(p.glyphRuns)) {
				return planError(fmt.Sprintf("commands[%d].payload", index), "references a missing glyph run")
			}
			run := p.glyphRuns[command.Payload]
			line := p.lines[run.Line]
			fragment := fragmentIDs[line.Fragment]
			if command.Fragment != line.Fragment || command.Bounds != line.Bounds || commandPages[index] != fragment.Page {
				return planError(fmt.Sprintf("commands[%d]", index), "does not match its glyph run line, fragment, and page")
			}
			runReferences[command.Payload]++
		case CommandImage:
			if uint64(command.Payload) >= uint64(len(p.images)) {
				return planError(fmt.Sprintf("commands[%d].payload", index), "references a missing planned image")
			}
			image := p.images[command.Payload]
			fragment := fragmentIDs[image.Fragment]
			if command.Fragment != image.Fragment || command.Bounds != image.Bounds || commandPages[index] != fragment.Page {
				return planError(fmt.Sprintf("commands[%d]", index), "does not match its planned image fragment, bounds, and page")
			}
			imageReferences[command.Payload]++
		case CommandLink:
			if uint64(command.Payload) >= uint64(len(p.links)) {
				return planError(fmt.Sprintf("commands[%d].payload", index), "references a missing planned link")
			}
			link := p.links[command.Payload]
			fragment := fragmentIDs[link.Fragment]
			if command.Fragment != link.Fragment || command.Bounds != link.Bounds || commandPages[index] != fragment.Page {
				return planError(fmt.Sprintf("commands[%d]", index), "does not match its planned link fragment, bounds, and page")
			}
			linkReferences[command.Payload]++
		}
	}
	if graphicsMode && stateDepth != 0 {
		return planError("commands", "last page has unbalanced saved state")
	}
	for index, count := range transformReferences {
		if count != 1 {
			return planError(fmt.Sprintf("transforms[%d]", index), "must be referenced exactly once")
		}
	}
	for index, count := range clipReferences {
		if count != 1 {
			return planError(fmt.Sprintf("clips[%d]", index), "must be referenced exactly once")
		}
	}
	for index, count := range fillReferences {
		if count != 1 {
			return planError(fmt.Sprintf("fills[%d]", index), "must be referenced exactly once")
		}
	}
	for index, count := range strokeReferences {
		if count != 1 {
			return planError(fmt.Sprintf("strokes[%d]", index), "must be referenced exactly once")
		}
	}
	for index, count := range pathReferences {
		if count == 0 {
			return planError(fmt.Sprintf("paths[%d]", index), "must be referenced by a clip, fill, or stroke")
		}
	}
	for index, count := range runReferences {
		if count != 1 {
			return planError(fmt.Sprintf("glyph_runs[%d]", index), "must be referenced by exactly one glyph command")
		}
	}
	for index, count := range imageReferences {
		if count != 1 {
			return planError(fmt.Sprintf("images[%d]", index), "must be referenced by exactly one image command")
		}
	}
	for index, count := range linkReferences {
		if count != 1 {
			return planError(fmt.Sprintf("links[%d]", index), "must be referenced by exactly one link command")
		}
	}
	for index, count := range imageResourceReferences {
		if count == 0 {
			return planError(fmt.Sprintf("image_resources[%d]", index), "is not referenced by a planned image")
		}
	}
	paragraphStates := make(map[struct {
		node     NodeID
		instance InstanceID
	}]FragmentContinuation)
	paragraphIdentities := make(map[struct {
		node     NodeID
		instance InstanceID
	}]bool)
	paragraphSources := make(map[struct {
		node     NodeID
		instance InstanceID
	}]SourceSpan)
	paragraphOrder := make([]struct {
		node     NodeID
		instance InstanceID
	}, 0)
	for _, fragment := range p.fragments {
		if fragmentLineCounts[fragment.ID] > 0 {
			identity := struct {
				node     NodeID
				instance InstanceID
			}{fragment.Node, fragment.Instance}
			paragraphIdentities[identity] = true
		}
	}
	for i, fragment := range p.fragments {
		identity := struct {
			node     NodeID
			instance InstanceID
		}{fragment.Node, fragment.Instance}
		if !paragraphIdentities[identity] {
			continue
		}
		if fragmentLineCounts[fragment.ID] == 0 {
			return planError(fmt.Sprintf("fragments[%d]", i), "paragraph fragment owns no planned lines")
		}
		if fragment.Repeated {
			if fragment.Continuation != ContinuationWhole {
				return planError(fmt.Sprintf("fragments[%d]", i), "repeated fragments must be whole")
			}
			state := paragraphStates[identity]
			if state != ContinuationWhole && state != ContinuationEnd {
				return planError(fmt.Sprintf("fragments[%d]", i), "repeated fragment has no completed original")
			}
			if fragment.Source != paragraphSources[identity] {
				return planError(fmt.Sprintf("fragments[%d]", i), "repeated fragment changes source provenance")
			}
			continue
		}
		state := paragraphStates[identity]
		if state == "" {
			paragraphOrder = append(paragraphOrder, identity)
			paragraphSources[identity] = fragment.Source
		} else if fragment.Source != paragraphSources[identity] {
			return planError(fmt.Sprintf("fragments[%d]", i), "paragraph continuation changes source provenance")
		}
		switch state {
		case "":
			if fragment.Continuation != ContinuationWhole && fragment.Continuation != ContinuationStart {
				return planError(fmt.Sprintf("fragments[%d]", i), "paragraph continuation must begin with whole or start")
			}
		case ContinuationStart, ContinuationMiddle:
			if fragment.Continuation != ContinuationMiddle && fragment.Continuation != ContinuationEnd {
				return planError(fmt.Sprintf("fragments[%d]", i), "paragraph continuation must continue with middle or end")
			}
		case ContinuationWhole, ContinuationEnd:
			return planError(fmt.Sprintf("fragments[%d]", i), "paragraph has lines after its terminal fragment")
		}
		paragraphStates[identity] = fragment.Continuation
	}
	for _, identity := range paragraphOrder {
		state := paragraphStates[identity]
		if state == ContinuationStart || state == ContinuationMiddle {
			return planError("fragments", fmt.Sprintf("paragraph node %d instance %q has no end fragment", identity.node, identity.instance))
		}
	}
	for i, decision := range p.breaks {
		path := fmt.Sprintf("breaks[%d]", i)
		if !decision.Reason.valid() {
			return planError(path, "has an invalid reason")
		}
		fromPage, toPage := uint64(decision.FromPage), uint64(decision.ToPage)
		if fromPage == 0 || fromPage > uint64(len(p.pages)) ||
			toPage > uint64(len(p.pages)) || toPage != fromPage+1 {
			return planError(path, "must transition between consecutive plan pages")
		}
		if !decision.Region.Valid() {
			return planError(path, "has an invalid region")
		}
		if !decision.Preceding.Valid() || !decision.Triggering.Valid() ||
			decision.Preceding == decision.Triggering {
			return planError(path, "must reference distinct preceding and triggering fragments")
		}
		preceding, precedingExists := fragmentIDs[decision.Preceding]
		triggering, triggeringExists := fragmentIDs[decision.Triggering]
		if !precedingExists || preceding.Page != decision.FromPage || preceding.Region != decision.Region {
			return planError(path, "preceding fragment does not match the source page and region")
		}
		if !triggeringExists || triggering.Page != decision.ToPage || triggering.Region != decision.Region {
			return planError(path, "triggering fragment does not match the destination page and region")
		}
		if decision.Required < 0 || decision.Available < 0 {
			return planError(path, "has a negative height")
		}
		switch decision.Reason {
		case BreakInsufficientRemainingBodySpace, BreakPaginationConstraint:
			if decision.Required <= decision.Available {
				return planError(path, "space or policy evidence must require more than is available")
			}
		case BreakPreviousFragmentOverflow:
			if decision.Required <= 0 || decision.Available != 0 {
				return planError(path, "post-overflow evidence must have positive required height and zero available height")
			}
		case BreakExplicitPageBreak:
			if decision.Required != 0 || decision.Available != 0 {
				return planError(path, "explicit page-break evidence must have zero required and available height")
			}
		}
	}
	for i, diagnostic := range p.diagnostics {
		if err := diagnostic.Validate(); err != nil {
			return planError(fmt.Sprintf("diagnostics[%d]", i), err.Error())
		}
		// A retained layout diagnostic is positioned evidence, not a global log
		// message. Authored inputs carry a source span; generated typed inputs
		// carry their stable logical node key instead.
		if diagnostic.Stage == StageLayout {
			if diagnostic.Location.Page == 0 {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "layout diagnostic has no page evidence")
			}
			if diagnostic.Location.Source.IsZero() && diagnostic.Location.Key == "" && !diagnostic.Location.Node.Valid() {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "layout diagnostic has no source evidence")
			}
		}
		if uint64(diagnostic.Location.Page) > uint64(len(p.pages)) {
			return planError(fmt.Sprintf("diagnostics[%d]", i), "references an invalid page")
		}
		if diagnostic.Location.Fragment.Valid() {
			fragment, exists := fragmentIDs[diagnostic.Location.Fragment]
			if !exists {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "references a missing fragment")
			}
			if diagnostic.Location.Page != 0 && diagnostic.Location.Page != fragment.Page {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "references a fragment on another page")
			}
			if diagnostic.Location.Region != "" && diagnostic.Location.Region != fragment.Region {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "references a fragment in another region")
			}
			if diagnostic.Location.Node.Valid() && diagnostic.Location.Node != fragment.Node {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "references a fragment owned by another node")
			}
			if diagnostic.Location.Key != "" && diagnostic.Location.Key != fragment.Key {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "references a fragment owned by another node key")
			}
			if diagnostic.Location.Instance.Valid() && diagnostic.Location.Instance != fragment.Instance {
				return planError(fmt.Sprintf("diagnostics[%d]", i), "references a fragment owned by another instance")
			}
		}
	}
	return nil
}

func planError(path, problem string) error {
	return fmt.Errorf("layoutengine: invalid layout plan at %s: %s", path, problem)
}

func fragmentBoxContains(outer, inner Rect) bool {
	outerRight, outerRightErr := outer.Right()
	outerBottom, outerBottomErr := outer.Bottom()
	innerRight, innerRightErr := inner.Right()
	innerBottom, innerBottomErr := inner.Bottom()
	return outerRightErr == nil && outerBottomErr == nil && innerRightErr == nil && innerBottomErr == nil &&
		inner.X >= outer.X && inner.Y >= outer.Y && innerRight <= outerRight && innerBottom <= outerBottom
}

func validatePlannedGridTracks(tracks []PlannedGridTrack, pages int) error {
	var previousGroup uint32
	var groupPage uint32
	var previousAxis GridTrackAxis
	var nextIndex uint32
	for index, track := range tracks {
		path := fmt.Sprintf("grid_tracks[%d]", index)
		if track.Group == 0 || track.Page == 0 || uint64(track.Page) > uint64(pages) { // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			return planError(path, "has an absent group or invalid page")
		}
		if !track.Region.Valid() || !track.Axis.valid() {
			return planError(path, "has an invalid region or axis")
		}
		if err := track.Bounds.Validate(); err != nil || track.Bounds.IsEmpty() {
			return planError(path+".bounds", "track bounds must have positive extents")
		}
		if track.GapAfter < 0 {
			return planError(path+".gap_after", "gap must be non-negative")
		}
		if track.Group != previousGroup {
			if track.Group != previousGroup+1 || track.Axis != GridTrackColumn || track.Index != 0 {
				return planError(path, "groups must be consecutive and begin with column zero")
			}
			previousGroup, groupPage, previousAxis, nextIndex = track.Group, track.Page, track.Axis, 1
			continue
		}
		if track.Page != groupPage {
			return planError(path, "one grid group spans multiple pages")
		}
		if track.Axis != previousAxis {
			if previousAxis != GridTrackColumn || track.Axis != GridTrackRow || track.Index != 0 {
				return planError(path, "row tracks must follow all column tracks")
			}
			previousAxis, nextIndex = track.Axis, 1
			continue
		}
		if track.Index != nextIndex {
			return planError(path, "track indexes are not consecutive")
		}
		nextIndex++
	}
	return nil
}

func validatePlannedPageRegions(regions []PlannedPageRegion, pages []PlannedPage) error {
	var previousPage uint32
	previousOrder := -1
	for index, region := range regions {
		path := fmt.Sprintf("page_regions[%d]", index)
		if region.Page == 0 || uint64(region.Page) > uint64(len(pages)) || !region.Region.Valid() {
			return planError(path, "has an invalid page or region")
		}
		if err := region.Bounds.Validate(); err != nil || region.Bounds.IsEmpty() {
			return planError(path+".bounds", "region bounds must have positive extents")
		}
		if region.Master != "" && !region.Master.valid() {
			return planError(path+".master", "master ID is invalid")
		}
		right, rightErr := region.Bounds.Right()
		bottom, bottomErr := region.Bounds.Bottom()
		page := pages[region.Page-1]
		if rightErr != nil || bottomErr != nil || region.Bounds.X < 0 || region.Bounds.Y < 0 || right > page.Size.Width || bottom > page.Size.Height {
			return planError(path+".bounds", "region lies outside its page")
		}
		order := regionOrder(region.Region)
		if region.Page < previousPage || region.Page == previousPage && order <= previousOrder {
			return planError(path, "regions must be unique and ordered by page, header, body, footer")
		}
		if region.Page != previousPage {
			previousPage, previousOrder = region.Page, order
		} else {
			previousOrder = order
		}
	}
	return nil
}

// Projection returns a detached canonical projection.
func (p LayoutPlan) Projection() LayoutPlanProjection {
	projection := LayoutPlanProjection{
		SchemaVersion:          LayoutPlanSchemaVersion,
		PlannerVersion:         PlannerVersion,
		PainterContractVersion: PainterContractVersion,
		Pages:                  cloneSlice(p.pages),
		Fragments:              cloneSlice(p.fragments),
		Lines:                  cloneSlice(p.lines),
		PageRegions:            cloneSlice(p.pageRegions),
		GridTracks:             cloneSlice(p.gridTracks),
		Fonts:                  cloneFontResources(p.fonts),
		GlyphRuns:              cloneCoreGlyphRuns(p.glyphRuns),
		ImageResources:         cloneSlice(p.imageResources),
		Images:                 clonePlannedImages(p.images),
		Destinations:           cloneSlice(p.destinations),
		Links:                  cloneSlice(p.links),
		Paths:                  clonePlannedPaths(p.paths),
		Transforms:             cloneSlice(p.transforms),
		Clips:                  cloneSlice(p.clips),
		Fills:                  cloneSlice(p.fills),
		Strokes:                clonePlannedStrokes(p.strokes),
		Commands:               cloneSlice(p.commands),
		Breaks:                 cloneSlice(p.breaks),
		SemanticNodes:          cloneSlice(p.semanticNodes),
		SemanticFragments:      cloneSlice(p.semanticFragments),
		ReadingOrder:           cloneSlice(p.readingOrder),
		Provenance:             cloneSlice(p.provenance),
		FragmentProvenance:     cloneSlice(p.fragmentProvenance),
		LineProvenance:         cloneSlice(p.lineProvenance),
	}
	if p.hasDeterministicInputs {
		manifest := cloneDeterministicInputs(p.deterministicInputs)
		projection.DeterministicInputs = &manifest
	}
	if len(p.diagnostics) != 0 {
		projection.Diagnostics = make([]Diagnostic, len(p.diagnostics))
		for i, diagnostic := range p.diagnostics {
			projection.Diagnostics[i] = cloneDiagnostic(diagnostic)
		}
	}
	return projection
}

// CanonicalJSON serializes the map-free versioned projection.
func (p LayoutPlan) CanonicalJSON() ([]byte, error) {
	return json.Marshal(p.Projection())
}

// Hash returns the deterministic digest of CanonicalJSON.
func (p LayoutPlan) Hash() (PlanHash, error) {
	encoded, err := p.CanonicalJSON()
	if err != nil {
		return PlanHash{}, err
	}
	return sha256.Sum256(encoded), nil
}
