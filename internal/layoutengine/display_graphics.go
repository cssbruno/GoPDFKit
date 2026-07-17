// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

const (
	maxDisplayGraphicResources = 1 << 20
	maxDisplayPathSegments     = 1 << 24
)

type PathSegmentKind string

const (
	PathMoveTo  PathSegmentKind = "move_to"
	PathLineTo  PathSegmentKind = "line_to"
	PathCubicTo PathSegmentKind = "cubic_to"
	PathClose   PathSegmentKind = "close"
)

type PathSegment struct {
	Kind     PathSegmentKind `json:"kind"`
	Point    Point           `json:"point"`
	Control1 Point           `json:"control_1"`
	Control2 Point           `json:"control_2"`
}

// PlannedPath is immutable page-space geometry. Bounds are the exact bounding
// box of all authored endpoints and cubic controls, making command bounds
// deterministic without recomputing Bézier extrema in a painter.
type PlannedPath struct {
	Segments []PathSegment `json:"segments"`
	Bounds   Rect          `json:"bounds"`
}

type FillRule string

const (
	FillNonZero FillRule = "nonzero"
	FillEvenOdd FillRule = "even_odd"
)

func (rule FillRule) valid() bool { return rule == FillNonZero || rule == FillEvenOdd }

type PlannedClip struct {
	Path     uint32     `json:"path"`
	Rule     FillRule   `json:"rule"`
	Fragment FragmentID `json:"fragment,omitempty"`
}

type PlannedFill struct {
	Path     uint32       `json:"path"`
	Rule     FillRule     `json:"rule"`
	Color    CoreRGBColor `json:"color"`
	Opacity  Fixed        `json:"opacity,omitempty"`
	Fragment FragmentID   `json:"fragment,omitempty"`
}

type PlannedStroke struct {
	Path       uint32         `json:"path"`
	Color      CoreRGBColor   `json:"color"`
	Width      Fixed          `json:"width"`
	LineCap    StrokeLineCap  `json:"line_cap,omitempty"`
	LineJoin   StrokeLineJoin `json:"line_join,omitempty"`
	Dash       []Fixed        `json:"dash,omitempty"`
	DashOffset Fixed          `json:"dash_offset,omitempty"`
	Opacity    Fixed          `json:"opacity,omitempty"`
	Fragment   FragmentID     `json:"fragment,omitempty"`
}

type StrokeLineCap string

const (
	StrokeCapButt   StrokeLineCap = "butt"
	StrokeCapRound  StrokeLineCap = "round"
	StrokeCapSquare StrokeLineCap = "square"
)

func (cap StrokeLineCap) valid() bool {
	return cap == "" || cap == StrokeCapButt || cap == StrokeCapRound || cap == StrokeCapSquare
}

type StrokeLineJoin string

const (
	StrokeJoinMiter StrokeLineJoin = "miter"
	StrokeJoinRound StrokeLineJoin = "round"
	StrokeJoinBevel StrokeLineJoin = "bevel"
)

func (join StrokeLineJoin) valid() bool {
	return join == "" || join == StrokeJoinMiter || join == StrokeJoinRound || join == StrokeJoinBevel
}

func clonePlannedPaths(paths []PlannedPath) []PlannedPath {
	result := cloneSlice(paths)
	for index := range result {
		result[index].Segments = cloneSlice(result[index].Segments)
	}
	return result
}

func clonePlannedPath(path PlannedPath) PlannedPath {
	path.Segments = cloneSlice(path.Segments)
	return path
}

func clonePlannedStrokes(strokes []PlannedStroke) []PlannedStroke {
	result := cloneSlice(strokes)
	for index := range result {
		result[index].Dash = cloneSlice(result[index].Dash)
	}
	return result
}

func validateDisplayGraphics(paths []PlannedPath, transforms []Transform, clips []PlannedClip, fills []PlannedFill, strokes []PlannedStroke) error {
	if len(paths) > maxDisplayGraphicResources || len(transforms) > maxDisplayGraphicResources || len(clips) > maxDisplayGraphicResources ||
		len(fills) > maxDisplayGraphicResources || len(strokes) > maxDisplayGraphicResources {
		return planError("graphics", "resource count exceeds the hard limit")
	}
	segments := 0
	for index, path := range paths {
		if len(path.Segments) > maxDisplayPathSegments-segments {
			return planError("paths", "segment count exceeds the hard limit")
		}
		segments += len(path.Segments)
		if err := validatePlannedPath(path); err != nil {
			return planError(fmt.Sprintf("paths[%d]", index), err.Error())
		}
	}
	for index, transform := range transforms {
		if err := transform.Validate(); err != nil {
			return planError(fmt.Sprintf("transforms[%d]", index), err.Error())
		}
	}
	for index, clip := range clips {
		if uint64(clip.Path) >= uint64(len(paths)) || !clip.Rule.valid() {
			return planError(fmt.Sprintf("clips[%d]", index), "has an invalid path or fill rule")
		}
	}
	for index, fill := range fills {
		if uint64(fill.Path) >= uint64(len(paths)) || !fill.Rule.valid() || !fill.Color.Set || fill.Opacity < 0 || fill.Opacity > Fixed(FixedScale) {
			return planError(fmt.Sprintf("fills[%d]", index), "has an invalid path, fill rule, or color")
		}
	}
	for index, stroke := range strokes {
		if uint64(stroke.Path) >= uint64(len(paths)) || !stroke.Color.Set || stroke.Width <= 0 || !stroke.LineCap.valid() || !stroke.LineJoin.valid() || len(stroke.Dash) > 256 || stroke.Opacity < 0 || stroke.Opacity > Fixed(FixedScale) {
			return planError(fmt.Sprintf("strokes[%d]", index), "has an invalid path, color, width, cap, join, or dash offset")
		}
		positive := false
		for _, dash := range stroke.Dash {
			if dash < 0 {
				return planError(fmt.Sprintf("strokes[%d]", index), "has a negative dash length")
			}
			positive = positive || dash > 0
		}
		if len(stroke.Dash) != 0 && !positive {
			return planError(fmt.Sprintf("strokes[%d]", index), "has an all-zero dash pattern")
		}
	}
	return nil
}

func validatePlannedPath(path PlannedPath) error {
	if len(path.Segments) < 2 || path.Segments[0].Kind != PathMoveTo {
		return errors.New("must begin with move_to and contain drawing geometry")
	}
	points := make([]Point, 0, len(path.Segments)*3)
	active, drew := false, false
	for index, segment := range path.Segments {
		zeroControls := segment.Control1 == (Point{}) && segment.Control2 == (Point{})
		switch segment.Kind {
		case PathMoveTo:
			if !zeroControls {
				return fmt.Errorf("segments[%d] move_to has controls", index)
			}
			points = append(points, segment.Point)
			active = true
		case PathLineTo:
			if !active || !zeroControls {
				return fmt.Errorf("segments[%d] line_to is invalid", index)
			}
			points = append(points, segment.Point)
			drew = true
		case PathCubicTo:
			if !active {
				return fmt.Errorf("segments[%d] cubic_to has no active subpath", index)
			}
			points = append(points, segment.Control1, segment.Control2, segment.Point)
			drew = true
		case PathClose:
			if !active || segment.Point != (Point{}) || !zeroControls {
				return fmt.Errorf("segments[%d] close is invalid", index)
			}
			active = false
		default:
			return fmt.Errorf("segments[%d] has an invalid kind", index)
		}
	}
	if !drew {
		return errors.New("contains no drawing segment")
	}
	minimum, maximum := points[0], points[0]
	for _, point := range points[1:] {
		if point.X < minimum.X {
			minimum.X = point.X
		}
		if point.Y < minimum.Y {
			minimum.Y = point.Y
		}
		if point.X > maximum.X {
			maximum.X = point.X
		}
		if point.Y > maximum.Y {
			maximum.Y = point.Y
		}
	}
	bounds, err := RectFromPoints(minimum, maximum)
	if err != nil || bounds != path.Bounds {
		return errors.New("bounds do not match endpoint/control geometry")
	}
	return nil
}
