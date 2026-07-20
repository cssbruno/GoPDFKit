// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

// HitTestResultLimit is the maximum number of command hits and fragment hits
// returned independently. Match counts and truncation flags preserve evidence
// that more geometry overlapped the query without producing an unbounded tool
// response.
const HitTestResultLimit uint32 = 64

var (
	// ErrHitTestInvalidPage reports an absent one-based page selector.
	ErrHitTestInvalidPage = errors.New("layoutengine: hit-test page number is zero")
	// ErrHitTestPageNotFound reports a page selector outside a plan.
	ErrHitTestPageNotFound = errors.New("layoutengine: hit-test page was not found")
)

// FragmentHitArea identifies which recorded fragment box contains the query
// point. Content wins when both boxes contain the point.
type FragmentHitArea string

const (
	HitFragmentBorder  FragmentHitArea = "border"
	HitFragmentContent FragmentHitArea = "content"
)

// FragmentHit is a detached editor/tool projection of one positioned
// fragment. It carries stable node and instance provenance without exposing
// mutable LayoutPlan storage.
type FragmentHit struct {
	Index        uint64
	PageIndex    uint32
	Fragment     FragmentID
	Node         NodeID
	Key          NodeKey
	Instance     InstanceID
	Region       RegionID
	Continuation FragmentContinuation
	Source       SourceSpan
	BorderBox    Rect
	ContentBox   Rect
	Area         FragmentHitArea
}

// CommandHit identifies one display command whose recorded bounds contain the
// query point. Index is the zero-based index in the canonical plan command
// sequence. Command payloads are intentionally excluded from generic tool
// responses.
type CommandHit struct {
	Index                 uint64
	PageIndex             uint32
	Kind                  DisplayCommandKind
	Fragment              FragmentID
	Bounds                Rect
	BoundsOnly            bool
	HasFragmentProvenance bool
	Node                  NodeID
	Key                   NodeKey
	Instance              InstanceID
	Region                RegionID
	Source                SourceSpan
}

// PageHitTest is a deterministic page-coordinate query result. Commands are
// ordered in reverse plan order (last painted first). Fragments are also in
// reverse page-fragment order (later planned first), which gives overlapping
// and nested geometry a stable selection order without inventing z-index or
// tree relationships that LayoutPlan does not yet record.
//
// Points may lie outside PageBounds so tools can inspect planned overflow.
type PageHitTest struct {
	Page       uint32
	Point      Point
	PageBounds Rect
	InsidePage bool
	Commands   []CommandHit
	Fragments  []FragmentHit

	CommandMatchCount  uint32
	FragmentMatchCount uint32
	CommandsTruncated  bool
	FragmentsTruncated bool
}

// HitTestPage queries recorded plan geometry using raw Fixed page
// coordinates. Rectangles use the canonical half-open rule: left/top edges
// hit, while right/bottom edges do not.
func (p LayoutPlan) HitTestPage(pageNumber uint32, point Point) (PageHitTest, error) {
	if pageNumber == 0 {
		return PageHitTest{}, ErrHitTestInvalidPage
	}
	if uint64(pageNumber) > uint64(len(p.pages)) {
		return PageHitTest{}, fmt.Errorf("%w: %d", ErrHitTestPageNotFound, pageNumber)
	}

	page := p.pages[int(pageNumber-1)]
	fragmentEnd, fragmentsOK := page.Fragments.end(len(p.fragments))
	commandEnd, commandsOK := page.Commands.end(len(p.commands))
	if !fragmentsOK || !commandsOK {
		return PageHitTest{}, errors.New("layoutengine: hit-test found an invalid plan page range")
	}
	pageBounds := Rect{Width: page.Size.Width, Height: page.Size.Height}
	insidePage, err := pageBounds.ContainsPoint(point)
	if err != nil {
		return PageHitTest{}, fmt.Errorf("layoutengine: hit-test page bounds: %w", err)
	}

	result := PageHitTest{
		Page:       page.Number,
		Point:      point,
		PageBounds: pageBounds,
		InsidePage: insidePage,
	}
	for index := commandEnd; index > int(page.Commands.Start); {
		index--
		command := p.commands[index]
		if !hitTestIncludesCommand(command.Kind) {
			continue
		}
		contains, err := command.Bounds.ContainsPoint(point)
		if err != nil {
			return PageHitTest{}, fmt.Errorf("layoutengine: hit-test command %d: %w", index, err)
		}
		if !contains {
			continue
		}
		result.CommandMatchCount++
		if uint32(len(result.Commands)) == HitTestResultLimit { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			result.CommandsTruncated = true
			continue
		}
		hit := CommandHit{
			Index:      uint64(index),                                       // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			PageIndex:  uint32(uint64(index) - uint64(page.Commands.Start)), // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			Kind:       command.Kind,
			Fragment:   command.Fragment,
			Bounds:     command.Bounds,
			BoundsOnly: true,
		}
		if command.Fragment.Valid() {
			fragment, ok := layoutPlanFragmentInRange(p.fragments, page.Fragments, command.Fragment)
			if !ok {
				return PageHitTest{}, fmt.Errorf("layoutengine: hit-test command %d has no page-local fragment", index)
			}
			hit.HasFragmentProvenance = true
			hit.Node = fragment.Node
			hit.Key = fragment.Key
			hit.Instance = fragment.Instance
			hit.Region = fragment.Region
			hit.Source = fragment.Source
		}
		result.Commands = append(result.Commands, hit)
	}

	for index := fragmentEnd; index > int(page.Fragments.Start); {
		index--
		fragment := p.fragments[index]
		contentHit, err := fragment.ContentBox.ContainsPoint(point)
		if err != nil {
			return PageHitTest{}, fmt.Errorf("layoutengine: hit-test fragment %d content: %w", fragment.ID, err)
		}
		borderHit, err := fragment.BorderBox.ContainsPoint(point)
		if err != nil {
			return PageHitTest{}, fmt.Errorf("layoutengine: hit-test fragment %d border: %w", fragment.ID, err)
		}
		if !borderHit {
			continue
		}
		result.FragmentMatchCount++
		if uint32(len(result.Fragments)) == HitTestResultLimit { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			result.FragmentsTruncated = true
			continue
		}
		area := HitFragmentBorder
		if contentHit {
			area = HitFragmentContent
		}
		result.Fragments = append(result.Fragments, FragmentHit{
			Index:        uint64(index),                                        // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			PageIndex:    uint32(uint64(index) - uint64(page.Fragments.Start)), // #nosec G115 -- fixed-width conversion is bounded by the surrounding parser, planner, or resource invariant
			Fragment:     fragment.ID,
			Node:         fragment.Node,
			Key:          fragment.Key,
			Instance:     fragment.Instance,
			Region:       fragment.Region,
			Continuation: fragment.Continuation,
			Source:       fragment.Source,
			BorderBox:    fragment.BorderBox,
			ContentBox:   fragment.ContentBox,
			Area:         area,
		})
	}
	return result, nil
}

func layoutPlanFragmentInRange(fragments []Fragment, selected IndexRange, id FragmentID) (Fragment, bool) {
	end, ok := selected.end(len(fragments))
	if !ok {
		return Fragment{}, false
	}
	for index := int(selected.Start); index < end; index++ {
		if fragments[index].ID == id {
			return fragments[index], true
		}
	}
	return Fragment{}, false
}

// State-stack and clip commands are structural. Their rectangular bounds do
// not mean they painted or exposed a selectable visual at the query point.
func hitTestIncludesCommand(kind DisplayCommandKind) bool {
	switch kind {
	case CommandFillPath, CommandStrokePath, CommandGlyphRun, CommandImage, CommandLink:
		return true
	default:
		return false
	}
}
