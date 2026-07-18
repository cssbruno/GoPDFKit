// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

var ErrDisplayPaintLimit = errors.New("layoutengine: display paint work limit exceeded")

// DisplayPaintLimits bounds replay of the mixed graphics/text/image display
// list. Validation and every limit check complete before the first callback.
type DisplayPaintLimits struct {
	MaxPages        uint64
	MaxCommands     uint64
	MaxGlyphs       uint64
	MaxImages       uint64
	MaxDestinations uint64
	MaxLinks        uint64
	MaxPaths        uint64
	MaxPathSegments uint64
	MaxStateDepth   uint64
}

func DefaultDisplayPaintLimits() DisplayPaintLimits {
	return DisplayPaintLimits{
		MaxPages: 4096, MaxCommands: 1 << 20, MaxGlyphs: 1 << 24, MaxImages: 1 << 20,
		MaxDestinations: 1 << 20, MaxLinks: 1 << 20, MaxPaths: 1 << 20,
		MaxPathSegments: 1 << 24, MaxStateDepth: 256,
	}
}

// DisplayPlanPaintSink consumes final positioned commands. Implementations
// must not measure, size, wrap, position, or paginate their payloads.
type DisplayPlanPaintSink interface {
	BeginPlannedPage(PlannedPage) error
	PaintCoreGlyphRun(CoreFontResource, CoreGlyphRun, DisplayCommand) error
	PaintPlannedImage(ImageResource, PlannedImage, DisplayCommand) error
	PaintPlannedLink(PlannedLink, PlannedDestination, DisplayCommand) error
	EndPlannedPage(PlannedPage) error
}

// DisplayGraphicsPaintSink extends the original text/image/link sink without
// breaking existing sinks. Plans containing graphics require this interface;
// support is checked during preflight before the first callback.
type DisplayGraphicsPaintSink interface {
	DisplayPlanPaintSink
	SaveGraphicsState(DisplayCommand) error
	RestoreGraphicsState(DisplayCommand) error
	ConcatTransform(Transform, DisplayCommand) error
	ClipPlannedPath(PlannedPath, PlannedClip, DisplayCommand) error
	FillPlannedPath(PlannedPath, PlannedFill, DisplayCommand) error
	StrokePlannedPath(PlannedPath, PlannedStroke, DisplayCommand) error
}

// ValidateDisplayPaintReady applies the production display-list contract:
// every command has a resolved immutable payload before replay begins.
func (p LayoutPlan) ValidateDisplayPaintReady() error {
	if err := p.Validate(); err != nil {
		return err
	}
	lineRuns := make([]uint32, len(p.lines))
	for _, run := range p.glyphRuns {
		lineRuns[run.Line]++
	}
	for index, line := range p.lines {
		if line.Bounds.Width > 0 && lineRuns[index] == 0 {
			return planError(fmt.Sprintf("lines[%d]", index), "positive-width line must have at least one core glyph run")
		}
		if line.Bounds.Width == 0 && lineRuns[index] != 0 {
			return planError(fmt.Sprintf("lines[%d]", index), "zero-width line must not paint a core glyph run")
		}
	}
	for index, command := range p.commands {
		if !command.Kind.valid() {
			return planError(fmt.Sprintf("commands[%d]", index), "display painter command is invalid")
		}
		switch command.Kind {
		case CommandTransform:
			if uint64(command.Payload) >= uint64(len(p.transforms)) {
				return planError(fmt.Sprintf("commands[%d]", index), "has no resolved transform")
			}
		case CommandClip:
			if uint64(command.Payload) >= uint64(len(p.clips)) {
				return planError(fmt.Sprintf("commands[%d]", index), "has no resolved clip")
			}
		case CommandFillPath:
			if uint64(command.Payload) >= uint64(len(p.fills)) {
				return planError(fmt.Sprintf("commands[%d]", index), "has no resolved fill")
			}
		case CommandStrokePath:
			if uint64(command.Payload) >= uint64(len(p.strokes)) {
				return planError(fmt.Sprintf("commands[%d]", index), "has no resolved stroke")
			}
		}
	}
	return nil
}

func PaintDisplayPlan(plan LayoutPlan, sink DisplayPlanPaintSink) error {
	return PaintDisplayPlanWithLimits(plan, sink, DefaultDisplayPaintLimits())
}

func PaintDisplayPlanWithLimits(plan LayoutPlan, sink DisplayPlanPaintSink, limits DisplayPaintLimits) error {
	if sink == nil {
		return errors.New("layoutengine: display plan paint sink is nil")
	}
	if err := ValidateDisplayPaintPlan(plan, limits); err != nil {
		return err
	}
	var graphics DisplayGraphicsPaintSink
	for _, command := range plan.commands {
		if command.Kind != CommandGlyphRun && command.Kind != CommandImage && command.Kind != CommandLink {
			var ok bool
			graphics, ok = sink.(DisplayGraphicsPaintSink)
			if !ok {
				return errors.New("layoutengine: display plan graphics sink is required")
			}
			break
		}
	}
	for _, page := range plan.pages {
		if err := sink.BeginPlannedPage(page); err != nil {
			return fmt.Errorf("layoutengine: begin planned page %d: %w", page.Number, err)
		}
		commandEnd, _ := page.Commands.end(len(plan.commands))
		for index := int(page.Commands.Start); index < commandEnd; index++ {
			command := plan.commands[index]
			switch command.Kind {
			case CommandSaveState:
				if err := graphics.SaveGraphicsState(command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandRestoreState:
				if err := graphics.RestoreGraphicsState(command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandTransform:
				if err := graphics.ConcatTransform(plan.transforms[command.Payload], command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandClip:
				clip := plan.clips[command.Payload]
				if err := graphics.ClipPlannedPath(clonePlannedPath(plan.paths[clip.Path]), clip, command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandFillPath:
				fill := plan.fills[command.Payload]
				if err := graphics.FillPlannedPath(clonePlannedPath(plan.paths[fill.Path]), fill, command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandStrokePath:
				stroke := plan.strokes[command.Payload]
				if err := graphics.StrokePlannedPath(clonePlannedPath(plan.paths[stroke.Path]), stroke, command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandGlyphRun:
				run := cloneCoreGlyphRun(plan.glyphRuns[command.Payload])
				font := plan.fonts[run.Font-1]
				if err := sink.PaintCoreGlyphRun(font, run, command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandImage:
				placement := clonePlannedImage(plan.images[command.Payload])
				resource := plan.imageResources[placement.Resource-1]
				if err := sink.PaintPlannedImage(resource, placement, command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			case CommandLink:
				link := plan.links[command.Payload]
				var destination PlannedDestination
				if link.Destination.Valid() {
					destination = plan.destinations[link.Destination-1]
				}
				if err := sink.PaintPlannedLink(link, destination, command); err != nil {
					return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
				}
			}
		}
		if err := sink.EndPlannedPage(page); err != nil {
			return fmt.Errorf("layoutengine: end planned page %d: %w", page.Number, err)
		}
	}
	return nil
}

// ValidateDisplayPaintPlan performs the complete bounded replay preflight
// without invoking a sink.
func ValidateDisplayPaintPlan(plan LayoutPlan, limits DisplayPaintLimits) error {
	if err := validateDisplayPaintLimits(plan, limits); err != nil {
		return err
	}
	return plan.ValidateDisplayPaintReady()
}

func validateDisplayPaintLimits(plan LayoutPlan, limits DisplayPaintLimits) error {
	if limits.MaxPages == 0 || limits.MaxCommands == 0 || limits.MaxGlyphs == 0 || limits.MaxImages == 0 ||
		limits.MaxDestinations == 0 || limits.MaxLinks == 0 || limits.MaxPaths == 0 || limits.MaxPathSegments == 0 || limits.MaxStateDepth == 0 {
		return errors.New("layoutengine: display paint limits must be positive")
	}
	if uint64(len(plan.pages)) > limits.MaxPages {
		return fmt.Errorf("%w: pages", ErrDisplayPaintLimit)
	}
	if uint64(len(plan.commands)) > limits.MaxCommands {
		return fmt.Errorf("%w: commands", ErrDisplayPaintLimit)
	}
	if uint64(len(plan.images)) > limits.MaxImages {
		return fmt.Errorf("%w: images", ErrDisplayPaintLimit)
	}
	if uint64(len(plan.destinations)) > limits.MaxDestinations {
		return fmt.Errorf("%w: destinations", ErrDisplayPaintLimit)
	}
	if uint64(len(plan.links)) > limits.MaxLinks {
		return fmt.Errorf("%w: links", ErrDisplayPaintLimit)
	}
	if uint64(len(plan.paths)) > limits.MaxPaths {
		return fmt.Errorf("%w: paths", ErrDisplayPaintLimit)
	}
	var segments uint64
	for _, path := range plan.paths {
		count := uint64(len(path.Segments))
		if count > limits.MaxPathSegments-segments {
			return fmt.Errorf("%w: path segments", ErrDisplayPaintLimit)
		}
		segments += count
	}
	var depth uint64
	for _, command := range plan.commands {
		if command.Kind == CommandSaveState {
			depth++
			if depth > limits.MaxStateDepth {
				return fmt.Errorf("%w: state depth", ErrDisplayPaintLimit)
			}
		}
		if command.Kind == CommandRestoreState {
			if depth == 0 {
				return errors.New("layoutengine: display state restore is unmatched")
			}
			depth--
		}
	}
	var glyphs uint64
	for _, run := range plan.glyphRuns {
		count := uint64(plan.fonts[run.Font-1].GlyphCount(run.Codes)) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		if count > limits.MaxGlyphs-glyphs {
			return fmt.Errorf("%w: glyphs", ErrDisplayPaintLimit)
		}
		glyphs += count
	}
	return nil
}

// DisplayPaintEventKind identifies a deterministic mixed-display recording
// event. Image events retain the complete crop payload.
type DisplayPaintEventKind string

const (
	DisplayPaintPageBegin    DisplayPaintEventKind = "page_begin"
	DisplayPaintGlyphRun     DisplayPaintEventKind = "glyph_run"
	DisplayPaintImage        DisplayPaintEventKind = "image"
	DisplayPaintLink         DisplayPaintEventKind = "link"
	DisplayPaintSaveState    DisplayPaintEventKind = "save_state"
	DisplayPaintRestoreState DisplayPaintEventKind = "restore_state"
	DisplayPaintTransform    DisplayPaintEventKind = "transform"
	DisplayPaintClip         DisplayPaintEventKind = "clip"
	DisplayPaintFill         DisplayPaintEventKind = "fill_path"
	DisplayPaintStroke       DisplayPaintEventKind = "stroke_path"
	DisplayPaintPageEnd      DisplayPaintEventKind = "page_end"
)

type DisplayPaintEvent struct {
	Kind        DisplayPaintEventKind `json:"kind"`
	Page        uint32                `json:"page"`
	Size        Size                  `json:"size"`
	Font        CoreFontResource      `json:"font"`
	Run         CoreGlyphRun          `json:"run"`
	Resource    ImageResource         `json:"resource"`
	Image       PlannedImage          `json:"image"`
	Link        PlannedLink           `json:"link"`
	Destination PlannedDestination    `json:"destination"`
	Command     DisplayCommand        `json:"command"`
	Path        PlannedPath           `json:"path"`
	Transform   Transform             `json:"transform"`
	Clip        PlannedClip           `json:"clip"`
	Fill        PlannedFill           `json:"fill"`
	Stroke      PlannedStroke         `json:"stroke"`
}

// DisplayPaintRecording is a detached no-layout trace for tests, tools, and
// adapters. Events returns defensive copies of glyph advances.
type DisplayPaintRecording struct {
	events []DisplayPaintEvent
	page   uint32
}

func RecordDisplayPlan(plan LayoutPlan) (DisplayPaintRecording, error) {
	recording := DisplayPaintRecording{}
	if err := PaintDisplayPlan(plan, &recording); err != nil {
		return DisplayPaintRecording{}, err
	}
	return recording, nil
}

func (recording *DisplayPaintRecording) BeginPlannedPage(page PlannedPage) error {
	recording.page = page.Number
	recording.events = append(recording.events, DisplayPaintEvent{
		Kind: DisplayPaintPageBegin, Page: page.Number, Size: page.Size,
	})
	return nil
}

func (recording *DisplayPaintRecording) PaintCoreGlyphRun(font CoreFontResource, run CoreGlyphRun, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{
		Kind: DisplayPaintGlyphRun, Page: recording.page, Font: font,
		Run: cloneCoreGlyphRun(run), Command: command,
	})
	return nil
}

func (recording *DisplayPaintRecording) PaintPlannedImage(resource ImageResource, image PlannedImage, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{
		Kind: DisplayPaintImage, Page: recording.page,
		Resource: resource, Image: image, Command: command,
	})
	return nil
}

func (recording *DisplayPaintRecording) PaintPlannedLink(link PlannedLink, destination PlannedDestination, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{
		Kind: DisplayPaintLink, Page: recording.page, Link: link, Destination: destination, Command: command,
	})
	return nil
}

func (recording *DisplayPaintRecording) SaveGraphicsState(command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintSaveState, Page: recording.page, Command: command})
	return nil
}
func (recording *DisplayPaintRecording) RestoreGraphicsState(command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintRestoreState, Page: recording.page, Command: command})
	return nil
}
func (recording *DisplayPaintRecording) ConcatTransform(transform Transform, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintTransform, Page: recording.page, Transform: transform, Command: command})
	return nil
}
func (recording *DisplayPaintRecording) ClipPlannedPath(path PlannedPath, clip PlannedClip, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintClip, Page: recording.page, Path: clonePlannedPath(path), Clip: clip, Command: command})
	return nil
}
func (recording *DisplayPaintRecording) FillPlannedPath(path PlannedPath, fill PlannedFill, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintFill, Page: recording.page, Path: clonePlannedPath(path), Fill: fill, Command: command})
	return nil
}
func (recording *DisplayPaintRecording) StrokePlannedPath(path PlannedPath, stroke PlannedStroke, command DisplayCommand) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintStroke, Page: recording.page, Path: clonePlannedPath(path), Stroke: stroke, Command: command})
	return nil
}

func (recording *DisplayPaintRecording) EndPlannedPage(page PlannedPage) error {
	recording.events = append(recording.events, DisplayPaintEvent{Kind: DisplayPaintPageEnd, Page: page.Number})
	recording.page = 0
	return nil
}

func (recording DisplayPaintRecording) Events() []DisplayPaintEvent {
	events := cloneSlice(recording.events)
	for index := range events {
		events[index].Run = cloneCoreGlyphRun(events[index].Run)
		events[index].Image = clonePlannedImage(events[index].Image)
		events[index].Path = clonePlannedPath(events[index].Path)
	}
	return events
}
