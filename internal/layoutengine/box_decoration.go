// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package layoutengine

import (
	"errors"
	"fmt"
)

// BoxBorderSide is one exact, solid, inside-edge border band. A zero-width
// side must have an unset color; a visible side requires a set color.
type BoxBorderSide struct {
	Width Fixed
	Color CoreRGBColor
}

// BoxShadow is one exact outer, solid-color shadow. Offset and spread are
// fixed geometry; blur and inset effects deliberately do not enter the shared
// display contract because they cannot be replayed identically by all sinks.
type BoxShadow struct {
	OffsetX Fixed
	OffsetY Fixed
	Spread  Fixed
	Color   CoreRGBColor
}

// BoxDecoration describes paint attached to one already-positioned fragment.
// The background covers BorderBox. Border bands are painted over it in
// top/right/bottom/left order, followed by the fragment's existing content.
type BoxDecoration struct {
	Fragment FragmentID
	// BorderBox optionally overrides the owning fragment's border box. It is
	// intended for a planned semantic group whose paint is associated with one
	// of its already-positioned descendant fragments. The rectangle must remain
	// inside that fragment's page and does not mutate fragment geometry.
	BorderBox  *Rect
	Background CoreRGBColor
	Radius     Fixed
	Shadow     BoxShadow
	Top        BoxBorderSide
	Right      BoxBorderSide
	Bottom     BoxBorderSide
	Left       BoxBorderSide
}

// AttachBoxDecorations adds exact solid backgrounds and per-side border bands
// to an immutable display plan without measuring or changing geometry. The
// operation validates and builds the complete replacement plan atomically.
func AttachBoxDecorations(plan LayoutPlan, decorations []BoxDecoration) (LayoutPlan, error) {
	if err := plan.Validate(); err != nil {
		return LayoutPlan{}, err
	}
	if len(decorations) == 0 {
		return plan, nil
	}
	projection := plan.Projection()
	if len(decorations) > maxDisplayGraphicResources {
		return LayoutPlan{}, errors.New("layoutengine: box decoration resource limit exceeded")
	}
	fragments := make(map[FragmentID]Fragment, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragments[fragment.ID] = fragment
	}
	seen := make(map[FragmentID]bool, len(decorations))
	byFragment := make(map[FragmentID][]DisplayCommand, len(decorations))
	byPage := make([][]FragmentID, len(projection.Pages))
	paths := projection.Paths
	fills := projection.Fills
	for index, decoration := range decorations {
		fragment, exists := fragments[decoration.Fragment]
		if !exists || seen[decoration.Fragment] {
			return LayoutPlan{}, planError(fmt.Sprintf("box_decorations[%d].fragment", index), "must reference one unique fragment")
		}
		seen[decoration.Fragment] = true
		if fragment.Page == 0 || uint64(fragment.Page) > uint64(len(byPage)) {
			return LayoutPlan{}, planError(fmt.Sprintf("box_decorations[%d].fragment", index), "has no owning page")
		}
		box := fragment.BorderBox
		if decoration.BorderBox != nil {
			box = *decoration.BorderBox
			if err := box.Validate(); err != nil || box.Width <= 0 || box.Height <= 0 {
				return LayoutPlan{}, planError(fmt.Sprintf("box_decorations[%d].border_box", index), "must be a non-empty valid rectangle")
			}
			page := projection.Pages[fragment.Page-1]
			if !rectContainsRect(Rect{Width: page.Size.Width, Height: page.Size.Height}, box) {
				return LayoutPlan{}, planError(fmt.Sprintf("box_decorations[%d].border_box", index), "must remain inside the owning page")
			}
		}
		if err := validateBoxDecoration(decoration, box, index); err != nil {
			return LayoutPlan{}, err
		}
		commands := make([]DisplayCommand, 0, 6)
		appendFill := func(box Rect, radius Fixed, color CoreRGBColor, rule FillRule) error {
			if !color.Set || box.Width == 0 || box.Height == 0 {
				return nil
			}
			if len(paths) >= maxDisplayGraphicResources || len(fills) >= maxDisplayGraphicResources {
				return errors.New("layoutengine: box decoration resource limit exceeded")
			}
			path, err := boxDecorationRoundedPath(box, radius, false)
			if err != nil {
				return err
			}
			paths = append(paths, path)
			fills = append(fills, PlannedFill{Path: uint32(len(paths) - 1), Rule: rule, Color: color, Fragment: fragment.ID})
			commands = append(commands, DisplayCommand{Kind: CommandFillPath, Fragment: fragment.ID, Bounds: box, Payload: uint32(len(fills) - 1)})
			return nil
		}
		if decoration.Shadow.Color.Set {
			shadowBox, shadowErr := boxDecorationShadowBox(box, decoration.Shadow)
			if shadowErr != nil {
				return LayoutPlan{}, planError(fmt.Sprintf("box_decorations[%d].shadow", index), shadowErr.Error())
			}
			shadowRadius, radiusErr := decoration.Radius.Add(decoration.Shadow.Spread)
			if radiusErr != nil {
				return LayoutPlan{}, radiusErr
			}
			if shadowRadius < 0 {
				shadowRadius = 0
			}
			if err := appendFill(shadowBox, shadowRadius, decoration.Shadow.Color, FillNonZero); err != nil {
				return LayoutPlan{}, err
			}
		}
		if decoration.Radius > 0 && decoration.Top.Width > 0 {
			inner, innerErr := insetDecorationRect(box, decoration.Top.Width)
			if innerErr != nil {
				return LayoutPlan{}, planError(fmt.Sprintf("box_decorations[%d].border", index), innerErr.Error())
			}
			innerRadius := decoration.Radius - decoration.Top.Width
			if innerRadius < 0 {
				innerRadius = 0
			}
			if err := appendFill(box, decoration.Radius, decoration.Top.Color, FillNonZero); err != nil {
				return LayoutPlan{}, err
			}
			if err := appendFill(inner, innerRadius, decoration.Background, FillNonZero); err != nil {
				return LayoutPlan{}, err
			}
			byFragment[fragment.ID] = commands
			byPage[fragment.Page-1] = append(byPage[fragment.Page-1], fragment.ID)
			continue
		}
		if err := appendFill(box, decoration.Radius, decoration.Background, FillNonZero); err != nil {
			return LayoutPlan{}, err
		}
		right, _ := box.Right()
		bottom, _ := box.Bottom()
		bands := []struct {
			side BoxBorderSide
			box  Rect
		}{
			{decoration.Top, Rect{X: box.X, Y: box.Y, Width: box.Width, Height: decoration.Top.Width}},
			{decoration.Right, Rect{X: right - decoration.Right.Width, Y: box.Y, Width: decoration.Right.Width, Height: box.Height}},
			{decoration.Bottom, Rect{X: box.X, Y: bottom - decoration.Bottom.Width, Width: box.Width, Height: decoration.Bottom.Width}},
			{decoration.Left, Rect{X: box.X, Y: box.Y, Width: decoration.Left.Width, Height: box.Height}},
		}
		for _, band := range bands {
			if err := appendFill(band.box, 0, band.side.Color, FillNonZero); err != nil {
				return LayoutPlan{}, err
			}
		}
		byFragment[fragment.ID] = commands
		byPage[fragment.Page-1] = append(byPage[fragment.Page-1], fragment.ID)
	}

	addedCommands := 0
	for _, decorationCommands := range byFragment {
		addedCommands += len(decorationCommands)
	}
	if len(projection.Commands) > maxDisplayGraphicResources-addedCommands {
		return LayoutPlan{}, errors.New("layoutengine: box decoration command limit exceeded")
	}
	commands := make([]DisplayCommand, 0, len(projection.Commands)+len(decorations)*5)
	for pageIndex := range projection.Pages {
		page := &projection.Pages[pageIndex]
		start := uint32(len(commands))
		for _, fragment := range byPage[pageIndex] {
			commands = append(commands, byFragment[fragment]...)
		}
		end, ok := page.Commands.end(len(projection.Commands))
		if !ok {
			return LayoutPlan{}, planError(fmt.Sprintf("pages[%d].commands", pageIndex), "range exceeds commands")
		}
		for commandIndex := int(page.Commands.Start); commandIndex < end; commandIndex++ {
			commands = append(commands, projection.Commands[commandIndex])
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
		Paths: paths, Transforms: projection.Transforms, Clips: projection.Clips, Fills: fills, Strokes: projection.Strokes,
		Commands: commands, Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
		SemanticNodes: projection.SemanticNodes, SemanticFragments: projection.SemanticFragments, ReadingOrder: projection.ReadingOrder,
	})
}

func validateBoxDecoration(decoration BoxDecoration, box Rect, index int) error {
	if !decoration.Background.Set && decoration.Background != (CoreRGBColor{}) {
		return planError(fmt.Sprintf("box_decorations[%d].background", index), "unset color must be zero")
	}
	if decoration.Radius < 0 {
		return planError(fmt.Sprintf("box_decorations[%d].radius", index), "must be non-negative")
	}
	if !decoration.Shadow.Color.Set && decoration.Shadow != (BoxShadow{}) {
		return planError(fmt.Sprintf("box_decorations[%d].shadow", index), "geometry requires a set color")
	}
	sides := []struct {
		name  string
		side  BoxBorderSide
		limit Fixed
	}{{"top", decoration.Top, box.Height}, {"right", decoration.Right, box.Width}, {"bottom", decoration.Bottom, box.Height}, {"left", decoration.Left, box.Width}}
	for _, entry := range sides {
		path := fmt.Sprintf("box_decorations[%d].%s", index, entry.name)
		if entry.side.Width < 0 || entry.side.Width > entry.limit {
			return planError(path, "width must be non-negative and no larger than the border box")
		}
		if entry.side.Width == 0 && entry.side.Color != (CoreRGBColor{}) {
			return planError(path, "zero-width side must have a zero unset color")
		}
		if entry.side.Width > 0 && !entry.side.Color.Set {
			return planError(path, "visible side requires a color")
		}
	}
	if decoration.Radius > 0 {
		first := decoration.Top
		if decoration.Right != first || decoration.Bottom != first || decoration.Left != first {
			return planError(fmt.Sprintf("box_decorations[%d].border", index), "rounded borders require equal sides")
		}
		if first.Width > box.Width/2 || first.Width > box.Height/2 {
			return planError(fmt.Sprintf("box_decorations[%d].border", index), "rounded border is too wide for its box")
		}
		if first.Width > 0 && !decoration.Background.Set {
			return planError(fmt.Sprintf("box_decorations[%d].background", index), "rounded borders require an opaque background")
		}
	}
	return nil
}

func boxDecorationRectPath(box Rect) (PlannedPath, error) {
	return boxDecorationRoundedPath(box, 0, false)
}

func boxDecorationRoundedPath(box Rect, radius Fixed, reverse bool) (PlannedPath, error) {
	right, err := box.Right()
	if err != nil {
		return PlannedPath{}, err
	}
	bottom, err := box.Bottom()
	if err != nil {
		return PlannedPath{}, err
	}
	if radius > box.Width/2 {
		radius = box.Width / 2
	}
	if radius > box.Height/2 {
		radius = box.Height / 2
	}
	if radius <= 0 {
		segments := []PathSegment{{Kind: PathMoveTo, Point: Point{X: box.X, Y: box.Y}}, {Kind: PathLineTo, Point: Point{X: right, Y: box.Y}}, {Kind: PathLineTo, Point: Point{X: right, Y: bottom}}, {Kind: PathLineTo, Point: Point{X: box.X, Y: bottom}}, {Kind: PathClose}}
		if reverse {
			segments = []PathSegment{{Kind: PathMoveTo, Point: Point{X: box.X, Y: box.Y}}, {Kind: PathLineTo, Point: Point{X: box.X, Y: bottom}}, {Kind: PathLineTo, Point: Point{X: right, Y: bottom}}, {Kind: PathLineTo, Point: Point{X: right, Y: box.Y}}, {Kind: PathClose}}
		}
		return PlannedPath{Bounds: box, Segments: segments}, nil
	}
	// Four cubic quarter-circles. The control distance is fixed once here, so
	// PDF, SVG, and raster all consume the identical immutable approximation.
	kappa, err := FixedFromPoints(radius.Points() * 0.5522847498307936)
	if err != nil {
		return PlannedPath{}, err
	}
	x0, y0, x1, y1 := box.X, box.Y, right, bottom
	if !reverse {
		return PlannedPath{Bounds: box, Segments: []PathSegment{
			{Kind: PathMoveTo, Point: Point{X: x0 + radius, Y: y0}},
			{Kind: PathLineTo, Point: Point{X: x1 - radius, Y: y0}},
			{Kind: PathCubicTo, Control1: Point{X: x1 - radius + kappa, Y: y0}, Control2: Point{X: x1, Y: y0 + radius - kappa}, Point: Point{X: x1, Y: y0 + radius}},
			{Kind: PathLineTo, Point: Point{X: x1, Y: y1 - radius}},
			{Kind: PathCubicTo, Control1: Point{X: x1, Y: y1 - radius + kappa}, Control2: Point{X: x1 - radius + kappa, Y: y1}, Point: Point{X: x1 - radius, Y: y1}},
			{Kind: PathLineTo, Point: Point{X: x0 + radius, Y: y1}},
			{Kind: PathCubicTo, Control1: Point{X: x0 + radius - kappa, Y: y1}, Control2: Point{X: x0, Y: y1 - radius + kappa}, Point: Point{X: x0, Y: y1 - radius}},
			{Kind: PathLineTo, Point: Point{X: x0, Y: y0 + radius}},
			{Kind: PathCubicTo, Control1: Point{X: x0, Y: y0 + radius - kappa}, Control2: Point{X: x0 + radius - kappa, Y: y0}, Point: Point{X: x0 + radius, Y: y0}},
			{Kind: PathClose},
		}}, nil
	}
	return PlannedPath{Bounds: box, Segments: []PathSegment{
		{Kind: PathMoveTo, Point: Point{X: x0 + radius, Y: y0}},
		{Kind: PathCubicTo, Control1: Point{X: x0 + radius - kappa, Y: y0}, Control2: Point{X: x0, Y: y0 + radius - kappa}, Point: Point{X: x0, Y: y0 + radius}},
		{Kind: PathLineTo, Point: Point{X: x0, Y: y1 - radius}},
		{Kind: PathCubicTo, Control1: Point{X: x0, Y: y1 - radius + kappa}, Control2: Point{X: x0 + radius - kappa, Y: y1}, Point: Point{X: x0 + radius, Y: y1}},
		{Kind: PathLineTo, Point: Point{X: x1 - radius, Y: y1}},
		{Kind: PathCubicTo, Control1: Point{X: x1 - radius + kappa, Y: y1}, Control2: Point{X: x1, Y: y1 - radius + kappa}, Point: Point{X: x1, Y: y1 - radius}},
		{Kind: PathLineTo, Point: Point{X: x1, Y: y0 + radius}},
		{Kind: PathCubicTo, Control1: Point{X: x1, Y: y0 + radius - kappa}, Control2: Point{X: x1 - radius + kappa, Y: y0}, Point: Point{X: x1 - radius, Y: y0}},
		{Kind: PathLineTo, Point: Point{X: x0 + radius, Y: y0}},
		{Kind: PathClose},
	}}, nil
}

func insetDecorationRect(box Rect, inset Fixed) (Rect, error) {
	width, err := box.Width.Sub(2 * inset)
	if err != nil {
		return Rect{}, err
	}
	height, err := box.Height.Sub(2 * inset)
	if err != nil || width <= 0 || height <= 0 {
		return Rect{}, errors.New("border leaves no rounded inner area")
	}
	return NewRect(box.X+inset, box.Y+inset, width, height)
}

func boxDecorationShadowBox(box Rect, shadow BoxShadow) (Rect, error) {
	x, err := box.X.Add(shadow.OffsetX)
	if err == nil {
		x, err = x.Sub(shadow.Spread)
	}
	y, yErr := box.Y.Add(shadow.OffsetY)
	if yErr == nil {
		y, yErr = y.Sub(shadow.Spread)
	}
	width, widthErr := box.Width.Add(2 * shadow.Spread)
	height, heightErr := box.Height.Add(2 * shadow.Spread)
	if err != nil || yErr != nil || widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return Rect{}, errors.New("spread leaves no shadow area")
	}
	return NewRect(x, y, width, height)
}
