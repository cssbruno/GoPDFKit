// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

var ErrCorePaintLimit = errors.New("layoutengine: core paint work limit exceeded")

// CorePaintLimits bounds synchronous plan replay. Limits are checked before
// full plan validation and before the first sink callback.
type CorePaintLimits struct {
	MaxPages    uint64
	MaxCommands uint64
	MaxGlyphs   uint64
}

// DefaultCorePaintLimits returns the immutable default replay policy by value.
func DefaultCorePaintLimits() CorePaintLimits {
	return CorePaintLimits{MaxPages: 4096, MaxCommands: 1 << 20, MaxGlyphs: 1 << 24}
}

// ValidatePaintReady applies the stricter initial core-text painter contract.
// Geometry-only plans remain valid under Validate, but cannot be painted until
// every positive-width line has exactly one glyph run and every command is a
// resolved core glyph operation.
func (p LayoutPlan) ValidatePaintReady() error {
	if err := p.Validate(); err != nil {
		return err
	}
	lineRuns := make([]uint32, len(p.lines))
	for index, run := range p.glyphRuns {
		lineRuns[run.Line]++
		if lineRuns[run.Line] > 1 {
			return planError(fmt.Sprintf("glyph_runs[%d].line", index), "paint-ready core text permits one run per line")
		}
	}
	for index, line := range p.lines {
		if line.Bounds.Width > 0 && lineRuns[index] != 1 {
			return planError(fmt.Sprintf("lines[%d]", index), "positive-width line has no core glyph run")
		}
		if line.Bounds.Width == 0 && lineRuns[index] != 0 {
			return planError(fmt.Sprintf("lines[%d]", index), "zero-width line must not paint a core glyph run")
		}
	}
	for index, command := range p.commands {
		if command.Kind != CommandGlyphRun {
			return planError(fmt.Sprintf("commands[%d]", index), "initial core-text painter supports only glyph-run commands")
		}
		if command.Payload != uint32(index) {
			return planError(fmt.Sprintf("commands[%d]", index), "glyph commands are not in canonical run order")
		}
	}
	return nil
}

// CorePlanPaintSink receives already positioned operations. Implementations
// must not measure, shape, wrap, paginate, or consult a live Document.
type CorePlanPaintSink interface {
	BeginPlannedPage(PlannedPage) error
	PaintCoreGlyphRun(CoreFontResource, CoreGlyphRun, DisplayCommand) error
	EndPlannedPage(PlannedPage) error
}

// PaintCorePlan validates the complete plan before the first sink callback,
// then replays its page command ranges exactly once in canonical order.
func PaintCorePlan(plan LayoutPlan, sink CorePlanPaintSink) error {
	return PaintCorePlanWithLimits(plan, sink, DefaultCorePaintLimits())
}

// PaintCorePlanWithLimits is PaintCorePlan with an explicit bounded-work
// policy, useful for services and agent tools with smaller budgets.
func PaintCorePlanWithLimits(plan LayoutPlan, sink CorePlanPaintSink, limits CorePaintLimits) error {
	if sink == nil {
		return errors.New("layoutengine: core plan paint sink is nil")
	}
	if err := validateCorePaintLimits(plan, limits); err != nil {
		return err
	}
	if err := plan.ValidatePaintReady(); err != nil {
		return err
	}
	for _, page := range plan.pages {
		if err := sink.BeginPlannedPage(page); err != nil {
			return fmt.Errorf("layoutengine: begin planned page %d: %w", page.Number, err)
		}
		commandEnd, _ := page.Commands.end(len(plan.commands))
		for index := int(page.Commands.Start); index < commandEnd; index++ {
			command := plan.commands[index]
			run := plan.glyphRuns[command.Payload]
			font := plan.fonts[run.Font-1]
			if err := sink.PaintCoreGlyphRun(font, cloneCoreGlyphRun(run), command); err != nil {
				return fmt.Errorf("layoutengine: paint command %d on page %d: %w", index, page.Number, err)
			}
		}
		if err := sink.EndPlannedPage(page); err != nil {
			return fmt.Errorf("layoutengine: end planned page %d: %w", page.Number, err)
		}
	}
	return nil
}

func validateCorePaintLimits(plan LayoutPlan, limits CorePaintLimits) error {
	if limits.MaxPages == 0 || limits.MaxCommands == 0 || limits.MaxGlyphs == 0 {
		return errors.New("layoutengine: core paint limits must be positive")
	}
	if uint64(len(plan.pages)) > limits.MaxPages {
		return fmt.Errorf("%w: pages", ErrCorePaintLimit)
	}
	if uint64(len(plan.commands)) > limits.MaxCommands {
		return fmt.Errorf("%w: commands", ErrCorePaintLimit)
	}
	var glyphs uint64
	for _, run := range plan.glyphRuns {
		count := uint64(plan.fonts[run.Font-1].GlyphCount(run.Codes)) // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
		if count > limits.MaxGlyphs-glyphs {
			return fmt.Errorf("%w: glyphs", ErrCorePaintLimit)
		}
		glyphs += count
	}
	return nil
}

func cloneCoreGlyphRun(run CoreGlyphRun) CoreGlyphRun {
	run.Advances = cloneSlice(run.Advances)
	return run
}

// CorePaintEventKind identifies a deterministic recording-painter event.
type CorePaintEventKind string

const (
	PaintPageBegin CorePaintEventKind = "page_begin"
	PaintGlyphRun  CorePaintEventKind = "glyph_run"
	PaintPageEnd   CorePaintEventKind = "page_end"
)

// CorePaintEvent is a detached trace event suitable for tests and visualizers.
// Glyph events contain user-authored text and declare that explicitly.
type CorePaintEvent struct {
	Kind             CorePaintEventKind `json:"kind"`
	Page             uint32             `json:"page"`
	ContainsUserText bool               `json:"contains_user_text"`
	Size             Size               `json:"size"`
	Font             CoreFontResource   `json:"font"`
	Run              CoreGlyphRun       `json:"run"`
	Command          DisplayCommand     `json:"command"`
}

// CorePaintRecording is the built-in no-layout recording sink.
type CorePaintRecording struct {
	ContainsUserText bool
	events           []CorePaintEvent
	page             uint32
}

// RecordCorePlan validates and records a plan as deterministic paint events.
func RecordCorePlan(plan LayoutPlan) (CorePaintRecording, error) {
	recording := CorePaintRecording{ContainsUserText: true}
	if err := PaintCorePlan(plan, &recording); err != nil {
		return CorePaintRecording{}, err
	}
	return recording, nil
}

func (r *CorePaintRecording) BeginPlannedPage(page PlannedPage) error {
	r.page = page.Number
	r.events = append(r.events, CorePaintEvent{Kind: PaintPageBegin, Page: page.Number, Size: page.Size})
	return nil
}

func (r *CorePaintRecording) PaintCoreGlyphRun(font CoreFontResource, run CoreGlyphRun, command DisplayCommand) error {
	r.events = append(r.events, CorePaintEvent{
		Kind: PaintGlyphRun, Page: r.page, ContainsUserText: true,
		Font: font, Run: cloneCoreGlyphRun(run), Command: command,
	})
	return nil
}

func (r *CorePaintRecording) EndPlannedPage(page PlannedPage) error {
	r.events = append(r.events, CorePaintEvent{Kind: PaintPageEnd, Page: page.Number, Size: page.Size})
	r.page = 0
	return nil
}

// Events returns a detached event stream.
func (r CorePaintRecording) Events() []CorePaintEvent {
	events := append([]CorePaintEvent(nil), r.events...)
	for index := range events {
		events[index].Run = cloneCoreGlyphRun(events[index].Run)
	}
	return events
}
