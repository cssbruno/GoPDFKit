// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
	"sort"
)

var ErrGlyphLinkContract = errors.New("layoutengine: invalid glyph-link contract")

// GlyphRunLinkSpan selects a non-empty consecutive glyph range in one exact
// positioned core glyph run. Run is a zero-based projection index. Start and
// Count are glyph indexes, not UTF-8 byte offsets. A frontend splits a logical
// link at planned line/run boundaries before calling AttachGlyphRunLinks.
type GlyphRunLinkSpan struct {
	Run         uint32
	Start       uint32
	Count       uint32
	URI         string
	Destination DestinationID
}

// AttachGlyphRunLinks derives exact annotation rectangles from final glyph
// advances and inserts link display commands immediately after their owning
// glyph command. It performs no shaping, wrapping, positioning, or pagination.
// The source plan must not already own links or destinations.
func AttachGlyphRunLinks(plan LayoutPlan, spans []GlyphRunLinkSpan) (LayoutPlan, error) {
	return AttachGlyphRunLinksWithDestinations(plan, nil, spans)
}

// AttachGlyphRunLinksWithDestinations is AttachGlyphRunLinks with an already
// resolved, consecutive destination catalog. Each span must select exactly one
// external URI or internal destination. Resolution remains a frontend concern;
// this function only derives final clickable bounds and command ownership.
func AttachGlyphRunLinksWithDestinations(plan LayoutPlan, destinations []PlannedDestination, spans []GlyphRunLinkSpan) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	projection := plan.Projection()
	if len(projection.Links) != 0 || len(projection.Destinations) != 0 {
		return LayoutPlan{}, fmt.Errorf("%w: plan already owns links or destinations", ErrGlyphLinkContract)
	}
	if len(spans) == 0 && len(destinations) == 0 {
		return plan, nil
	}
	if len(spans) > 1<<20 {
		return LayoutPlan{}, fmt.Errorf("%w: link span count exceeds the hard limit", ErrGlyphLinkContract)
	}
	ordered := append([]GlyphRunLinkSpan(nil), spans...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Run != ordered[j].Run {
			return ordered[i].Run < ordered[j].Run
		}
		return ordered[i].Start < ordered[j].Start
	})
	linksByRun := make(map[uint32][]uint32)
	links := make([]PlannedLink, 0, len(ordered))
	var previous GlyphRunLinkSpan
	var prefixRun = ^uint32(0)
	var advancePrefix []Fixed
	for index, span := range ordered {
		if uint64(span.Run) >= uint64(len(projection.GlyphRuns)) || span.Count == 0 {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] references an absent run or empty range", ErrGlyphLinkContract, index)
		}
		if (span.URI == "") == (!span.Destination.Valid()) {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] must select exactly one URI or destination", ErrGlyphLinkContract, index)
		}
		if span.Destination.Valid() && uint64(span.Destination) > uint64(len(destinations)) {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] references a missing destination", ErrGlyphLinkContract, index)
		}
		run := projection.GlyphRuns[span.Run]
		end := uint64(span.Start) + uint64(span.Count)
		if end > uint64(len(run.Advances)) || len(run.Advances) != projection.Fonts[run.Font-1].GlyphCount(run.Codes) {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] exceeds its glyph run", ErrGlyphLinkContract, index)
		}
		if index > 0 && previous.Run == span.Run && uint64(previous.Start)+uint64(previous.Count) > uint64(span.Start) {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] overlaps the preceding range", ErrGlyphLinkContract, index)
		}
		if uint64(run.Line) >= uint64(len(projection.Lines)) {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] run has no planned line", ErrGlyphLinkContract, index)
		}
		line := projection.Lines[run.Line]
		if prefixRun != span.Run {
			advancePrefix = make([]Fixed, len(run.Advances)+1)
			for advanceIndex, advance := range run.Advances {
				var err error
				advancePrefix[advanceIndex+1], err = advancePrefix[advanceIndex].Add(advance)
				if err != nil {
					return LayoutPlan{}, fmt.Errorf("%w: glyph run %d advance prefix overflows", ErrGlyphLinkContract, span.Run)
				}
			}
			prefixRun = span.Run
		}
		x, err := run.Origin.X.Add(advancePrefix[span.Start])
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] x origin overflows", ErrGlyphLinkContract, index)
		}
		width, err := advancePrefix[uint32(end)].Sub(advancePrefix[span.Start])
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] width overflows", ErrGlyphLinkContract, index)
		}
		if width <= 0 {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] has no clickable width", ErrGlyphLinkContract, index)
		}
		fragmentIndex := uint64(line.Fragment) - 1
		if !line.Fragment.Valid() || fragmentIndex >= uint64(len(projection.Fragments)) {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] line has no fragment", ErrGlyphLinkContract, index)
		}
		fragment := projection.Fragments[fragmentIndex]
		bounds, err := NewRect(x, line.Bounds.Y, width, line.Bounds.Height)
		if err != nil {
			return LayoutPlan{}, fmt.Errorf("%w: spans[%d] bounds: %w", ErrGlyphLinkContract, index, err)
		}
		links = append(links, PlannedLink{Fragment: line.Fragment, Bounds: bounds, URI: span.URI, Destination: span.Destination, Source: fragment.Source})
		linksByRun[span.Run] = append(linksByRun[span.Run], uint32(len(links)-1))
		previous = span
	}

	commands := make([]DisplayCommand, 0, len(projection.Commands)+len(links))
	pageCounts := make([]uint32, len(projection.Pages))
	seenRuns := make([]bool, len(projection.GlyphRuns))
	for index, command := range projection.Commands {
		commands = append(commands, command)
		fragmentIndex := uint64(command.Fragment) - 1
		if !command.Fragment.Valid() || fragmentIndex >= uint64(len(projection.Fragments)) {
			return LayoutPlan{}, fmt.Errorf("%w: commands[%d] has no fragment", ErrGlyphLinkContract, index)
		}
		page := projection.Fragments[fragmentIndex].Page
		pageCounts[page-1]++
		if command.Kind != CommandGlyphRun {
			continue
		}
		if uint64(command.Payload) >= uint64(len(seenRuns)) || seenRuns[command.Payload] {
			return LayoutPlan{}, fmt.Errorf("%w: glyph run command ownership is not one-to-one", ErrGlyphLinkContract)
		}
		seenRuns[command.Payload] = true
		for _, linkIndex := range linksByRun[command.Payload] {
			link := links[linkIndex]
			commands = append(commands, DisplayCommand{Kind: CommandLink, Fragment: link.Fragment, Bounds: link.Bounds, Payload: linkIndex})
			pageCounts[page-1]++
		}
	}
	for run, ownedLinks := range linksByRun {
		if len(ownedLinks) != 0 && !seenRuns[run] {
			return LayoutPlan{}, fmt.Errorf("%w: glyph run %d has no display command", ErrGlyphLinkContract, run)
		}
	}
	var start uint32
	for index := range projection.Pages {
		projection.Pages[index].Commands = IndexRange{Start: start, Count: pageCounts[index]}
		start += pageCounts[index]
	}
	return NewLayoutPlan(LayoutPlanInput{
		DeterministicInputs: projection.DeterministicInputs,
		Pages:               projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		ImageResources: projection.ImageResources, Images: projection.Images, Destinations: destinations,
		Paths: projection.Paths, Transforms: projection.Transforms, Clips: projection.Clips, Fills: projection.Fills, Strokes: projection.Strokes,
		Links: links, Commands: commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments,
		ReadingOrder: projection.ReadingOrder,
	})
}
