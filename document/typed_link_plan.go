// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

type typedLinkedText struct {
	text    string
	cursor  int
	links   []typedLinkedRange
	anchors []typedDestinationRange
}

type typedLinkedRange struct {
	start       int
	end         int
	target      string
	destination layoutengine.DestinationID
}

type typedDestinationRange struct {
	start int
	end   int
	name  string
	id    layoutengine.DestinationID
}

func typedMeasuredSegmentLinks(blocks []paperMeasuredBlock) map[layoutengine.NodeID][]layout.TextSegment {
	result := make(map[layoutengine.NodeID][]layout.TextSegment)
	appendLinked := func(node layoutengine.NodeID, segments []layout.TextSegment) {
		for _, segment := range segments {
			if segment.Link != "" || segment.Destination != "" {
				result[node] = append([]layout.TextSegment(nil), segments...)
				return
			}
		}
	}
	for _, block := range blocks {
		if block.explicitBreak || block.image != nil {
			continue
		}
		if block.gridRow != nil {
			for _, cell := range block.gridRow.cells {
				if cell.image == nil && !cell.artifactOnly {
					appendLinked(cell.node, cell.segments)
				}
			}
			continue
		}
		appendLinked(block.node, block.segments)
	}
	return result
}

// attachTypedSegmentLinks maps authored segment byte ranges onto already
// finalized one-byte core glyph runs. Wrapping may omit boundary whitespace;
// monotonic source matching keeps those bytes authored but unpainted. Exact
// annotation geometry is then derived only from final glyph advances.
func attachTypedSegmentLinks(plan layoutengine.LayoutPlan, byNode map[layoutengine.NodeID][]layout.TextSegment) (layoutengine.LayoutPlan, error) {
	if len(byNode) == 0 {
		return plan, nil
	}
	states := make(map[layoutengine.NodeID]*typedLinkedText, len(byNode))
	nodes := make([]layoutengine.NodeID, 0, len(byNode))
	for node := range byNode {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	destinationIDs := make(map[string]layoutengine.DestinationID)
	for _, node := range nodes {
		segments := byNode[node]
		state := &typedLinkedText{}
		for _, segment := range segments {
			start := len(state.text)
			state.text += segment.Text
			if segment.Link != "" && len(segment.Text) != 0 {
				state.links = append(state.links, typedLinkedRange{start: start, end: len(state.text), target: segment.Link})
			}
			if segment.Destination != "" {
				if len(segment.Text) == 0 {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed destination %q has no text glyphs", segment.Destination)
				}
				if err := validateTypedDestinationName(segment.Destination); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				if destinationIDs[segment.Destination].Valid() {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: duplicate typed destination %q", segment.Destination)
				}
				id := layoutengine.DestinationID(len(destinationIDs) + 1)
				destinationIDs[segment.Destination] = id
				state.anchors = append(state.anchors, typedDestinationRange{start: start, end: len(state.text), name: segment.Destination, id: id})
			}
		}
		if len(state.links) != 0 || len(state.anchors) != 0 {
			states[node] = state
		}
	}
	if len(states) == 0 {
		return plan, nil
	}
	projection := plan.Projection()
	fragmentNodes := make(map[layoutengine.FragmentID]layoutengine.NodeID, len(projection.Fragments))
	repeatedFragments := make(map[layoutengine.FragmentID]bool, len(projection.Fragments))
	type pageKey struct {
		page uint32
		key  layoutengine.NodeKey
	}
	repeatedParents := make(map[pageKey]bool)
	for _, fragment := range projection.Fragments {
		if fragment.Repeated {
			repeatedParents[pageKey{fragment.Page, fragment.Key}] = true
		}
	}
	for _, fragment := range projection.Fragments {
		fragmentNodes[fragment.ID] = fragment.Node
		repeatedFragments[fragment.ID] = fragment.Repeated
		if split := strings.Index(string(fragment.Key), "/content-"); split > 0 {
			repeatedFragments[fragment.ID] = repeatedFragments[fragment.ID] || repeatedParents[pageKey{fragment.Page, layoutengine.NodeKey(string(fragment.Key)[:split])}]
		}
	}
	spans := make([]layoutengine.GlyphRunLinkSpan, 0)
	destinations := make([]layoutengine.PlannedDestination, len(destinationIDs))
	resolvedDestinations := make([]bool, len(destinationIDs))
	var previousFragment layoutengine.FragmentID
	for runIndex, run := range projection.GlyphRuns {
		if uint64(run.Line) >= uint64(len(projection.Lines)) {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed link glyph run %d has no line", runIndex)
		}
		fragment := projection.Lines[run.Line].Fragment
		node := fragmentNodes[fragment]
		state := states[node]
		if state == nil || run.Codes == "" {
			continue
		}
		if fragment != previousFragment && repeatedFragments[fragment] {
			state.cursor = 0
		}
		previousFragment = fragment
		relative := strings.Index(state.text[state.cursor:], run.Codes)
		if relative < 0 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed link text for node %d does not match finalized glyph run %d", node, runIndex)
		}
		runStart := state.cursor + relative
		runEnd := runStart + len(run.Codes)
		for _, anchor := range state.anchors {
			if resolvedDestinations[anchor.id-1] || runEnd <= anchor.start || runStart >= anchor.end {
				continue
			}
			glyphBytes := max(runStart, anchor.start) - runStart
			glyph := utf8.RuneCountInString(run.Codes[:glyphBytes])
			x := run.Origin.X
			for _, advance := range run.Advances[:glyph] {
				var addErr error
				x, addErr = x.Add(advance)
				if addErr != nil {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed destination %q x overflows", anchor.name)
				}
			}
			line := projection.Lines[run.Line]
			owner := projection.Fragments[uint64(line.Fragment)-1]
			destinations[anchor.id-1] = layoutengine.PlannedDestination{ID: anchor.id, Page: owner.Page, Fragment: owner.ID,
				Point: layoutengine.Point{X: x, Y: line.Bounds.Y}, Source: owner.Source}
			resolvedDestinations[anchor.id-1] = true
		}
		for _, linked := range state.links {
			start, end := max(runStart, linked.start), min(runEnd, linked.end)
			if start >= end {
				continue
			}
			startGlyph := utf8.RuneCountInString(run.Codes[:start-runStart])
			endGlyph := utf8.RuneCountInString(run.Codes[:end-runStart])
			span := layoutengine.GlyphRunLinkSpan{Run: uint32(runIndex), Start: uint32(startGlyph), Count: uint32(endGlyph - startGlyph)}
			if strings.HasPrefix(linked.target, "#") {
				name := strings.TrimPrefix(linked.target, "#")
				span.Destination = destinationIDs[name]
				if !span.Destination.Valid() {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed internal link %q references a missing destination", linked.target)
				}
			} else {
				span.URI = linked.target
			}
			spans = append(spans, span)
		}
		state.cursor = runEnd
	}
	for node, state := range states {
		for _, linked := range state.links {
			tail := max(state.cursor, linked.start)
			if tail < linked.end && strings.TrimSpace(state.text[tail:linked.end]) != "" {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed link text for node %d was not represented by finalized glyphs", node)
			}
		}
		for _, anchor := range state.anchors {
			if !resolvedDestinations[anchor.id-1] {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: typed destination %q on node %d was not represented by finalized glyphs", anchor.name, node)
			}
		}
	}
	linked, err := layoutengine.AttachGlyphRunLinksWithDestinations(plan, destinations, spans)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: attach typed segment links: %w", err)
	}
	return linked, nil
}

func validateTypedDestinationName(name string) error {
	if name == "" || len(name) > 256 || strings.TrimSpace(name) != name || !utf8.ValidString(name) {
		return fmt.Errorf("document: typed destination name %q is not canonical UTF-8", name)
	}
	for _, character := range name {
		if unicode.IsControl(character) || unicode.IsSpace(character) || character == '#' {
			return fmt.Errorf("document: typed destination name %q contains an unsupported character", name)
		}
	}
	return nil
}
