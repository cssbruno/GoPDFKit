// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"testing"
)

type displayPaintCall struct {
	kind     string
	page     uint32
	fragment FragmentID
	font     FontResourceID
	image    ImageResourceID
}

type recordingDisplaySink struct {
	calls []displayPaintCall
	page  uint32
}

func (s *recordingDisplaySink) BeginPlannedPage(page PlannedPage) error {
	s.page = page.Number
	s.calls = append(s.calls, displayPaintCall{kind: "begin", page: page.Number})
	return nil
}

func (s *recordingDisplaySink) PaintCoreGlyphRun(font CoreFontResource, _ CoreGlyphRun, command DisplayCommand) error {
	s.calls = append(s.calls, displayPaintCall{kind: "glyph", page: s.page, fragment: command.Fragment, font: font.ID})
	return nil
}

func (s *recordingDisplaySink) PaintPlannedImage(resource ImageResource, _ PlannedImage, command DisplayCommand) error {
	s.calls = append(s.calls, displayPaintCall{kind: "image", page: s.page, fragment: command.Fragment, image: resource.ID})
	return nil
}

func (s *recordingDisplaySink) PaintPlannedLink(_ PlannedLink, _ PlannedDestination, command DisplayCommand) error {
	s.calls = append(s.calls, displayPaintCall{kind: "link", page: s.page, fragment: command.Fragment})
	return nil
}

func (s *recordingDisplaySink) EndPlannedPage(page PlannedPage) error {
	s.calls = append(s.calls, displayPaintCall{kind: "end", page: page.Number})
	s.page = 0
	return nil
}

func TestPaintDisplayPlanReplaysMixedCanonicalOrder(t *testing.T) {
	plan := composedDisplayPlan(t)
	sink := &recordingDisplaySink{}
	if err := PaintDisplayPlan(plan, sink); err != nil {
		t.Fatalf("PaintDisplayPlan() = %v", err)
	}
	want := []displayPaintCall{
		{kind: "begin", page: 1},
		{kind: "image", page: 1, fragment: 2, image: 1},
		{kind: "glyph", page: 1, fragment: 1, font: 1},
		{kind: "end", page: 1},
	}
	if len(sink.calls) != len(want) {
		t.Fatalf("calls = %+v, want %+v", sink.calls, want)
	}
	for index := range want {
		if sink.calls[index] != want[index] {
			t.Fatalf("call %d = %+v, want %+v", index, sink.calls[index], want[index])
		}
	}
}

func TestPaintDisplayPlanChecksLimitsBeforeCallbacks(t *testing.T) {
	plan := composedDisplayPlan(t)
	sink := &recordingDisplaySink{}
	limits := DefaultDisplayPaintLimits()
	limits.MaxImages = 0
	if err := PaintDisplayPlanWithLimits(plan, sink, limits); err == nil {
		t.Fatal("zero limit unexpectedly accepted")
	}
	if len(sink.calls) != 0 {
		t.Fatalf("limit failure invoked sink: %+v", sink.calls)
	}
	limits = DefaultDisplayPaintLimits()
	limits.MaxImages = 1
	limits.MaxCommands = 1
	if err := PaintDisplayPlanWithLimits(plan, sink, limits); !errors.Is(err, ErrDisplayPaintLimit) {
		t.Fatalf("command limit = %v, want ErrDisplayPaintLimit", err)
	}
	if len(sink.calls) != 0 {
		t.Fatalf("work-limit failure invoked sink: %+v", sink.calls)
	}
}

func TestRecordDisplayPlanRetainsCropPayload(t *testing.T) {
	plan := composedDisplayPlan(t)
	projection := plan.Projection()
	crop := &ImageCrop{
		Intrinsic: Size{Width: 300, Height: 400},
		Source:    Rect{X: 75, Width: 150, Height: 400},
		Clip:      projection.Images[0].Bounds,
	}
	projection.Images[0].Crop = crop
	plan, err := NewLayoutPlan(LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		Fonts: projection.Fonts, GlyphRuns: projection.GlyphRuns,
		ImageResources: projection.ImageResources, Images: projection.Images,
		Destinations: projection.Destinations, Links: projection.Links,
		Commands: projection.Commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan() = %v", err)
	}
	recording, err := RecordDisplayPlan(plan)
	if err != nil {
		t.Fatalf("RecordDisplayPlan() = %v", err)
	}
	events := recording.Events()
	var imageEvent *DisplayPaintEvent
	for index := range events {
		if events[index].Kind == DisplayPaintImage {
			imageEvent = &events[index]
		}
	}
	if imageEvent == nil || imageEvent.Image.Crop == nil || *imageEvent.Image.Crop != *crop {
		t.Fatalf("recorded image event = %+v", imageEvent)
	}
	imageEvent.Image.Crop.Source.X = 0
	fresh := recording.Events()
	for _, event := range fresh {
		if event.Kind == DisplayPaintImage && event.Image.Crop.Source.X != 75 {
			t.Fatalf("recording crop was not detached: %+v", event.Image.Crop)
		}
	}
}

func composedDisplayPlan(t *testing.T) LayoutPlan {
	t.Helper()
	glyphInput := coreGlyphPlanInput()
	imageInput := imagePlanInput()
	glyphInput.Pages[0].Commands = IndexRange{}
	imageFragment := imageInput.Fragments[0]
	imageFragment.ID = 2
	imageFragment.Node = 2
	imageFragment.Key = "@paint-image"
	imageFragment.Instance = "@paint-image"
	glyphInput.Fragments = append(glyphInput.Fragments, imageFragment)
	glyphInput.Pages[0].Fragments.Count = 2
	geometry, err := NewLayoutPlan(LayoutPlanInput{
		Pages: glyphInput.Pages, Fragments: glyphInput.Fragments, Lines: glyphInput.Lines,
	})
	if err != nil {
		t.Fatalf("NewLayoutPlan(geometry) = %v", err)
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
	return plan
}

func TestValidateDisplayPaintReadyAcceptsExactContiguousMixedRuns(t *testing.T) {
	input := coreGlyphPlanInput()
	input.Pages[0].Commands = IndexRange{}
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: input.Pages, Fragments: input.Fragments, Lines: input.Lines})
	if err != nil {
		t.Fatal(err)
	}
	first := input.GlyphRuns[0]
	first.Codes, first.Advances = "A", []Fixed{4}
	second := input.GlyphRuns[0]
	second.Codes, second.Advances = "B", []Fixed{6}
	second.Origin.X += 4
	plan, err := AttachDisplayList(geometry, DisplayListInput{Fonts: input.Fonts, GlyphRuns: []CoreGlyphRun{first, second},
		Items: []DisplayItem{{Kind: CommandGlyphRun, Payload: 0}, {Kind: CommandGlyphRun, Payload: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.ValidateDisplayPaintReady(); err != nil {
		t.Fatalf("contiguous mixed-run plan is not paint-ready: %v", err)
	}
}
