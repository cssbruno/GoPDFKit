// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "errors"

// AttachOverlayLines adds already-positioned lines to one page of a
// resource-free geometry plan. It exists for graphics formats whose text
// positions are part of the source rather than the document flow. The
// returned start is the first global line index assigned to the new lines.
func AttachOverlayLines(plan LayoutPlan, page uint32, lines []PlannedLine) (LayoutPlan, uint32, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, 0, err
	}
	projection := plan.Projection()
	if page == 0 || uint64(page) > uint64(len(projection.Pages)) {
		return LayoutPlan{}, 0, errors.New("layoutengine: overlay lines require an existing page")
	}
	if len(projection.Commands) != 0 || len(projection.Fonts) != 0 || len(projection.GlyphRuns) != 0 ||
		len(projection.ImageResources) != 0 || len(projection.Images) != 0 || len(projection.Paths) != 0 ||
		len(projection.Destinations) != 0 || len(projection.Links) != 0 || len(projection.Transforms) != 0 ||
		len(projection.Clips) != 0 || len(projection.Fills) != 0 || len(projection.Strokes) != 0 {
		return LayoutPlan{}, 0, errors.New("layoutengine: overlay lines require a resource-free geometry plan")
	}
	if len(lines) == 0 {
		return plan, projection.Pages[page-1].Lines.Start + projection.Pages[page-1].Lines.Count, nil
	}
	fragmentPages := make(map[FragmentID]uint32, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragmentPages[fragment.ID] = fragment.Page
	}
	for _, line := range lines {
		if fragmentPages[line.Fragment] != page {
			return LayoutPlan{}, 0, errors.New("layoutengine: overlay line fragment does not belong to its page")
		}
	}
	selected := projection.Pages[page-1].Lines
	insertAt := int(selected.Start + selected.Count)
	combined := make([]PlannedLine, 0, len(projection.Lines)+len(lines))
	combined = append(combined, projection.Lines[:insertAt]...)
	combined = append(combined, lines...)
	combined = append(combined, projection.Lines[insertAt:]...)
	projection.Pages[page-1].Lines.Count += uint32(len(lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	for index := int(page); index < len(projection.Pages); index++ {
		projection.Pages[index].Lines.Start += uint32(len(lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	}
	result, err := NewLayoutPlan(LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: combined,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments,
		ReadingOrder: projection.ReadingOrder,
	})
	if err != nil {
		return LayoutPlan{}, 0, err
	}
	result, err = rebindDeterministicResources(result, projection.DeterministicInputs)
	return result, uint32(insertAt), err // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
}
