// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import "testing"

func TestAttachDisplayListComposesTextAndImagesInPaintOrder(t *testing.T) {
	glyphInput := coreGlyphPlanInput()
	imageInput := imagePlanInput()
	glyphInput.Pages[0].Commands = IndexRange{}
	imageFragment := imageInput.Fragments[0]
	imageFragment.ID = 2
	imageFragment.Node = 2
	imageFragment.Key = "@composed-image"
	imageFragment.Instance = "@composed-image"
	glyphInput.Fragments = append(glyphInput.Fragments, imageFragment)
	glyphInput.Pages[0].Fragments.Count = 2
	geometry, err := NewLayoutPlan(LayoutPlanInput{
		Pages: glyphInput.Pages, Fragments: glyphInput.Fragments, Lines: glyphInput.Lines,
	})
	if err != nil {
		t.Fatalf("geometry NewLayoutPlan() = %v", err)
	}
	image := imageInput.Images[0]
	image.Fragment = 2
	plan, err := AttachDisplayList(geometry, DisplayListInput{
		Fonts: glyphInput.Fonts, GlyphRuns: glyphInput.GlyphRuns,
		ImageResources: imageInput.ImageResources, Images: []PlannedImage{image},
		Items: []DisplayItem{{Kind: CommandImage}, {Kind: CommandGlyphRun}},
	})
	if err != nil {
		t.Fatalf("AttachDisplayList() = %v", err)
	}
	projection := plan.Projection()
	if projection.Pages[0].Commands != (IndexRange{Count: 2}) || len(projection.Commands) != 2 {
		t.Fatalf("commands = %+v / %+v", projection.Pages[0].Commands, projection.Commands)
	}
	if projection.Commands[0].Kind != CommandImage || projection.Commands[0].Fragment != 2 ||
		projection.Commands[1].Kind != CommandGlyphRun || projection.Commands[1].Fragment != 1 {
		t.Fatalf("paint order = %+v", projection.Commands)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("composed plan Validate() = %v", err)
	}
}

func TestAttachDisplayListRejectsMissingAndRepeatedPayloads(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Pages[0].Commands = IndexRange{}
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: input.Pages, Fragments: input.Fragments, Lines: input.Lines})
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	for _, items := range [][]DisplayItem{
		{{Kind: CommandGlyphRun, Payload: 1}},
		{{Kind: CommandFillPath}},
		{{Kind: CommandGlyphRun}, {Kind: CommandGlyphRun}},
	} {
		if _, err := AttachDisplayList(geometry, DisplayListInput{
			Fonts: input.Fonts, GlyphRuns: input.GlyphRuns, Items: items,
		}); err == nil {
			t.Fatalf("invalid display items unexpectedly attached: %+v", items)
		}
	}
}
