// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"strings"
	"testing"
)

func TestImagePlanOwnsContentAddressedPlacement(t *testing.T) {
	input := imagePlanInput()
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	projection := plan.Projection()
	if len(projection.ImageResources) != 1 || len(projection.Images) != 1 ||
		projection.Commands[0].Kind != CommandImage {
		t.Fatalf("image projection = %+v", projection)
	}
	wantHash, _ := plan.Hash()
	input.ImageResources[0].PixelWidth++
	input.Images[0].Bounds.Width++
	if got, _ := plan.Hash(); got != wantHash {
		t.Fatal("image input mutation changed immutable plan hash")
	}
	if err := plan.ValidatePaintReady(); err == nil {
		t.Fatal("core-text painter unexpectedly accepted an image command")
	}
}

func TestImagePlanValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*LayoutPlanInput)
	}{
		{"resource ID", func(input *LayoutPlanInput) { input.ImageResources[0].ID = 2 }},
		{"digest", func(input *LayoutPlanInput) {
			input.ImageResources[0].Digest = ImageContentDigest(strings.Repeat("0", 64))
		}},
		{"format", func(input *LayoutPlanInput) { input.ImageResources[0].Format = "gif" }},
		{"pixel width", func(input *LayoutPlanInput) { input.ImageResources[0].PixelWidth = 0 }},
		{"missing resource", func(input *LayoutPlanInput) { input.Images[0].Resource = 2 }},
		{"missing fragment", func(input *LayoutPlanInput) { input.Images[0].Fragment = 2 }},
		{"empty bounds", func(input *LayoutPlanInput) { input.Images[0].Bounds.Width = 0 }},
		{"missing command", func(input *LayoutPlanInput) { input.Commands = nil; input.Pages[0].Commands = IndexRange{} }},
		{"bad payload", func(input *LayoutPlanInput) { input.Commands[0].Payload = 1 }},
		{"command bounds", func(input *LayoutPlanInput) { input.Commands[0].Bounds.Width++ }},
		{"unused resource", func(input *LayoutPlanInput) {
			input.ImageResources = append(input.ImageResources, ImageResource{
				ID: 2, Digest: ImageContentDigest(strings.Repeat("2", 64)), Format: ImageJPEG, PixelWidth: 1, PixelHeight: 1,
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := imagePlanInput()
			test.mutate(&input)
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid image plan unexpectedly validated")
			}
		})
	}
}

func TestAttachImagesBuildsCanonicalMultiPageCommands(t *testing.T) {
	input := imagePlanInput()
	input.Pages = append(input.Pages, PlannedPage{
		Number: 2, Size: input.Pages[0].Size, Fragments: IndexRange{Start: 1, Count: 1},
	})
	input.Pages[0].Commands = IndexRange{}
	secondFragment := input.Fragments[0]
	secondFragment.ID = 2
	secondFragment.Node = 2
	secondFragment.Key = "@image-two"
	secondFragment.Instance = "@image-two"
	secondFragment.Page = 2
	input.Fragments = append(input.Fragments, secondFragment)
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: input.Pages, Fragments: input.Fragments})
	if err != nil {
		t.Fatalf("geometry NewLayoutPlan() = %v", err)
	}
	images := []PlannedImage{
		{Resource: 1, Fragment: 1, Bounds: input.Fragments[0].ContentBox},
		{Resource: 1, Fragment: 2, Bounds: input.Fragments[1].ContentBox},
	}
	plan, err := AttachImages(geometry, input.ImageResources, images)
	if err != nil {
		t.Fatalf("AttachImages() = %v", err)
	}
	projection := plan.Projection()
	if projection.Pages[0].Commands != (IndexRange{Count: 1}) ||
		projection.Pages[1].Commands != (IndexRange{Start: 1, Count: 1}) {
		t.Fatalf("page command ranges = %+v, %+v", projection.Pages[0].Commands, projection.Pages[1].Commands)
	}
	for index, command := range projection.Commands {
		if command.Kind != CommandImage || command.Payload != uint32(index) || command.Fragment != FragmentID(index+1) {
			t.Fatalf("command %d = %+v", index, command)
		}
	}
	images[0].Bounds.Width++
	if plan.Projection().Images[0].Bounds == images[0].Bounds {
		t.Fatal("image input mutation reached attached plan")
	}
}

func TestImageCropValidationAndOwnership(t *testing.T) {
	input := imagePlanInput()
	crop := &ImageCrop{
		Intrinsic: Size{Width: 300, Height: 400},
		Source:    Rect{X: 75, Width: 150, Height: 400},
		Clip:      input.Images[0].Bounds,
	}
	input.Images[0].Crop = crop
	plan, err := NewLayoutPlan(input)
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	projection := plan.Projection()
	if projection.Images[0].Crop == nil || *projection.Images[0].Crop != *crop {
		t.Fatalf("projected crop = %+v", projection.Images[0].Crop)
	}
	crop.Source.X = 0
	projection.Images[0].Crop.Source.Y = 99
	owned := plan.Projection().Images[0].Crop
	if owned == nil || owned.Source != (Rect{X: 75, Width: 150, Height: 400}) {
		t.Fatalf("crop mutation reached immutable plan: %+v", owned)
	}
}

func TestImageCropValidationRejectsInconsistentPayload(t *testing.T) {
	tests := []struct {
		name string
		crop ImageCrop
	}{
		{"empty", ImageCrop{}},
		{"source outside intrinsic", ImageCrop{Intrinsic: Size{Width: 300, Height: 400}, Source: Rect{X: 299, Width: 2, Height: 400}, Clip: Rect{X: 10, Y: 20, Width: 30, Height: 40}}},
		{"clip mismatch", ImageCrop{Intrinsic: Size{Width: 300, Height: 400}, Source: Rect{Width: 300, Height: 400}, Clip: Rect{X: 10, Y: 20, Width: 29, Height: 40}}},
		{"aspect mismatch", ImageCrop{Intrinsic: Size{Width: 400, Height: 400}, Source: Rect{Width: 400, Height: 400}, Clip: Rect{X: 10, Y: 20, Width: 30, Height: 40}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := imagePlanInput()
			input.Images[0].Crop = &test.crop
			if _, err := NewLayoutPlan(input); err == nil {
				t.Fatal("invalid image crop unexpectedly validated")
			}
		})
	}
}

func imagePlanInput() LayoutPlanInput {
	bounds := Rect{X: 10, Y: 20, Width: 30, Height: 40}
	return LayoutPlanInput{
		Pages: []PlannedPage{{
			Number: 1, Size: Size{Width: 100, Height: 100},
			Fragments: IndexRange{Count: 1}, Commands: IndexRange{Count: 1},
		}},
		Fragments: []Fragment{{
			ID: 1, Node: 1, Key: "@image", Instance: "@image", Page: 1, Region: RegionBody,
			BorderBox: bounds, ContentBox: bounds, Continuation: ContinuationWhole,
		}},
		ImageResources: []ImageResource{{
			ID: 1, Digest: ImageContentDigest(strings.Repeat("1", 64)), Format: ImagePNG,
			PixelWidth: 300, PixelHeight: 400,
		}},
		Images:   []PlannedImage{{Resource: 1, Fragment: 1, Bounds: bounds}},
		Commands: []DisplayCommand{{Kind: CommandImage, Fragment: 1, Bounds: bounds}},
	}
}
