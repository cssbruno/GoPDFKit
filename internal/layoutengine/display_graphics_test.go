// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"testing"
)

func graphFixed(value int64) Fixed { return Fixed(value * FixedScale) }

func graphicsDisplayPlan(t *testing.T) LayoutPlan {
	t.Helper()
	geometry, err := NewLayoutPlan(LayoutPlanInput{Pages: []PlannedPage{{Number: 1, Size: Size{Width: graphFixed(100), Height: graphFixed(100)}}}})
	if err != nil {
		t.Fatal(err)
	}
	paths := []PlannedPath{
		{Bounds: Rect{X: graphFixed(10), Y: graphFixed(10), Width: graphFixed(30), Height: graphFixed(20)}, Segments: []PathSegment{
			{Kind: PathMoveTo, Point: Point{X: graphFixed(10), Y: graphFixed(10)}},
			{Kind: PathLineTo, Point: Point{X: graphFixed(40), Y: graphFixed(10)}},
			{Kind: PathLineTo, Point: Point{X: graphFixed(40), Y: graphFixed(30)}},
			{Kind: PathLineTo, Point: Point{X: graphFixed(10), Y: graphFixed(30)}}, {Kind: PathClose},
		}},
		{Bounds: Rect{X: graphFixed(50), Width: graphFixed(30), Height: graphFixed(40)}, Segments: []PathSegment{
			{Kind: PathMoveTo, Point: Point{X: graphFixed(50), Y: graphFixed(10)}},
			{Kind: PathCubicTo, Control1: Point{X: graphFixed(60)}, Control2: Point{X: graphFixed(70), Y: graphFixed(40)}, Point: Point{X: graphFixed(80), Y: graphFixed(30)}},
		}},
	}
	plan, err := AttachDisplayList(geometry, DisplayListInput{
		Paths: paths, Transforms: []Transform{TranslationTransform(graphFixed(5), graphFixed(7))},
		Clips:   []PlannedClip{{Path: 0, Rule: FillEvenOdd}},
		Fills:   []PlannedFill{{Path: 0, Rule: FillNonZero, Color: CoreRGBColor{R: 17, G: 34, B: 51, Set: true}}},
		Strokes: []PlannedStroke{{Path: 1, Color: CoreRGBColor{R: 200, G: 100, B: 50, Set: true}, Width: graphFixed(2)}},
		Items: []DisplayItem{
			{Kind: CommandSaveState, Page: 1}, {Kind: CommandTransform, Page: 1}, {Kind: CommandClip, Page: 1},
			{Kind: CommandFillPath, Page: 1}, {Kind: CommandStrokePath, Page: 1}, {Kind: CommandRestoreState, Page: 1},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestDisplayGraphicsReplayIsDeterministicAndDetached(t *testing.T) {
	plan := graphicsDisplayPlan(t)
	recording, err := RecordDisplayPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	events := recording.Events()
	want := []DisplayPaintEventKind{DisplayPaintPageBegin, DisplayPaintSaveState, DisplayPaintTransform, DisplayPaintClip, DisplayPaintFill, DisplayPaintStroke, DisplayPaintRestoreState, DisplayPaintPageEnd}
	if len(events) != len(want) {
		t.Fatalf("events = %d, want %d", len(events), len(want))
	}
	for index := range want {
		if events[index].Kind != want[index] {
			t.Fatalf("event %d = %s, want %s", index, events[index].Kind, want[index])
		}
	}
	events[3].Path.Segments[0].Point.X = 99
	again := recording.Events()
	if again[3].Path.Segments[0].Point.X != graphFixed(10) {
		t.Fatal("recording exposed mutable path storage")
	}
}

func TestDisplayGraphicsStrictValidationAndWorkBounds(t *testing.T) {
	plan := graphicsDisplayPlan(t)
	projection := plan.Projection()
	cases := []func(*LayoutPlanProjection){
		func(value *LayoutPlanProjection) {
			value.Commands = value.Commands[:len(value.Commands)-1]
			value.Pages[0].Commands.Count--
		},
		func(value *LayoutPlanProjection) { value.Transforms[0] = Transform{A: Fixed(FixedScale)} },
		func(value *LayoutPlanProjection) { value.Paths[0].Bounds.Width++ },
		func(value *LayoutPlanProjection) {
			value.Commands[1], value.Commands[5] = value.Commands[5], value.Commands[1]
		},
	}
	for index, mutate := range cases {
		value := projection
		value.Paths = clonePlannedPaths(projection.Paths)
		value.Commands = cloneSlice(projection.Commands)
		value.Transforms = cloneSlice(projection.Transforms)
		mutate(&value)
		if _, err := NewLayoutPlan(layoutPlanInputFromStoredProjection(value)); err == nil {
			t.Fatalf("invalid graphics case %d validated", index)
		}
	}
	limits := DefaultDisplayPaintLimits()
	limits.MaxPathSegments = 4
	recording := &DisplayPaintRecording{}
	if err := PaintDisplayPlanWithLimits(plan, recording, limits); !errors.Is(err, ErrDisplayPaintLimit) {
		t.Fatalf("path segment bound = %v", err)
	}
	if len(recording.Events()) != 0 {
		t.Fatal("limit failure invoked sink callbacks")
	}
}

func TestDisplayGraphicsCanonicalStoreRoundTrip(t *testing.T) {
	plan := graphicsDisplayPlan(t)
	store, err := NewMemoryPlanStore(DefaultPlanStoreLimits())
	if err != nil {
		t.Fatal(err)
	}
	hash, err := store.Put(plan)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(hash)
	if err != nil {
		t.Fatal(err)
	}
	projection := loaded.Projection()
	if len(projection.Paths) != 2 || len(projection.Transforms) != 1 || len(projection.Clips) != 1 || len(projection.Fills) != 1 || len(projection.Strokes) != 1 {
		t.Fatalf("stored graphics projection = %+v", projection)
	}
	projection.Paths[0].Segments[0].Point.X++
	if loaded.Projection().Paths[0].Segments[0].Point.X != graphFixed(10) {
		t.Fatal("projection exposed stored path segments")
	}
	if got, _ := loaded.Hash(); got != hash {
		t.Fatal("graphics plan hash changed after detached projection mutation")
	}
}
