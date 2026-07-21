// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "fmt"

// AttachFragmentClips wraps each visual command owned by a selected fragment
// in an exact rectangular clip. A clip resource is intentionally emitted per
// command because the immutable IR requires every clip resource to be
// referenced exactly once; this also keeps save/clip/paint/restore scopes
// independent when a fragment has multiple glyph runs. It performs no layout
// and leaves annotations outside the graphics-state scope.
func AttachFragmentClips(plan LayoutPlan, bounds map[FragmentID]Rect) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	if len(bounds) == 0 {
		return plan, nil
	}
	projection := plan.Projection()
	fragmentPages := make(map[FragmentID]uint32, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragmentPages[fragment.ID] = fragment.Page
	}
	for fragment, box := range bounds {
		if fragmentPages[fragment] == 0 || box.Validate() != nil || box.Width <= 0 || box.Height <= 0 {
			return LayoutPlan{}, fmt.Errorf("layoutengine: fragment clip %d is invalid", fragment)
		}
	}
	commands := make([]DisplayCommand, 0, len(projection.Commands)+len(bounds)*3)
	for pageIndex := range projection.Pages {
		page := &projection.Pages[pageIndex]
		end, _ := page.Commands.end(len(projection.Commands))
		start := uint32(len(commands))
		for index := int(page.Commands.Start); index < end; index++ {
			command := projection.Commands[index]
			box, selected := bounds[command.Fragment]
			visual := command.Kind == CommandFillPath || command.Kind == CommandStrokePath || command.Kind == CommandGlyphRun || command.Kind == CommandImage
			if selected && visual {
				path, err := boxDecorationRectPath(box)
				if err != nil {
					return LayoutPlan{}, err
				}
				projection.Paths = append(projection.Paths, path)
				clip := uint32(len(projection.Clips))
				projection.Clips = append(projection.Clips, PlannedClip{Path: uint32(len(projection.Paths) - 1), Rule: FillNonZero, Fragment: command.Fragment})
				commands = append(commands, DisplayCommand{Kind: CommandSaveState}, DisplayCommand{Kind: CommandClip, Fragment: command.Fragment, Bounds: bounds[command.Fragment], Payload: clip})
			}
			commands = append(commands, command)
			if selected && visual {
				commands = append(commands, DisplayCommand{Kind: CommandRestoreState})
			}
		}
		page.Commands = IndexRange{Start: start, Count: uint32(len(commands)) - start}
	}
	return NewLayoutPlan(LayoutPlanInput{
		DeterministicInputs: projection.DeterministicInputs,
		Pages:               projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		ImageResources: projection.ImageResources, Images: projection.Images,
		Destinations: projection.Destinations, Links: projection.Links,
		Paths: projection.Paths, Transforms: projection.Transforms, Clips: projection.Clips,
		Fills: projection.Fills, Strokes: projection.Strokes, Commands: commands,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments, ReadingOrder: projection.ReadingOrder,
	})
}
