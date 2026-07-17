// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

type paperMeasuredBorder struct {
	width layoutengine.Fixed
	color layoutengine.CoreRGBColor
}

type paperMeasuredBox struct {
	style      layoutengine.BoxFlowStyle
	background layoutengine.CoreRGBColor
	borders    [4]paperMeasuredBorder // top, right, bottom, left
	width      layoutengine.Fixed
	height     layoutengine.Fixed
	minWidth   layoutengine.Fixed
	minHeight  layoutengine.Fixed
	maxWidth   layoutengine.Fixed
	maxHeight  layoutengine.Fixed
	overflow   string
	radius     layoutengine.Fixed
	shadow     layoutengine.BoxShadow
	visual     bool
}

func paperParagraphBoxPolicy(box layout.BoxStyle, ref *layout.BoxStyle, path string) (paperPaginationPolicy, layout.BoxStyle, error) {
	_ = ref // EffectiveBox has already snapshotted the referenced value.
	policy := paperPaginationPolicy{keepTogether: box.KeepTogether, keepWithNext: box.KeepWithNext,
		orphans: box.Orphans, widows: box.Widows, applyOrphans: box.Orphans != 0, applyWidows: box.Widows != 0}
	box.KeepTogether, box.KeepWithNext, box.Orphans, box.Widows = false, false, 0, 0
	if policy.orphans == 0 {
		policy.orphans = 1
	}
	if policy.widows == 0 {
		policy.widows = 1
	}
	return policy, box, nil
}

// paperContainerBoxPolicy separates flow policy from visual group styling.
// Visual styling is distributed over the group's finalized child fragments:
// horizontal edges repeat, while vertical outer edges occur only at the
// beginning and end of the authored group.
func paperContainerBoxPolicy(box layout.BoxStyle) (paperPaginationPolicy, layout.BoxStyle) {
	policy := paperPaginationPolicy{keepTogether: box.KeepTogether, keepWithNext: box.KeepWithNext,
		orphans: box.Orphans, widows: box.Widows, applyOrphans: box.Orphans != 0, applyWidows: box.Widows != 0}
	box.KeepTogether, box.KeepWithNext, box.Orphans, box.Widows = false, false, 0, 0
	if policy.orphans == 0 {
		policy.orphans = 1
	}
	if policy.widows == 0 {
		policy.widows = 1
	}
	return policy, box
}

func paperApplyContainerBox(parts *[]paperPlanningBlock, start int, box layout.BoxStyle, path string) error {
	if !htmlUnifiedVisualBox(box) {
		return nil
	}
	indices := make([]int, 0, len(*parts)-start)
	for index := start; index < len(*parts); index++ {
		if !(*parts)[index].explicitBreak {
			indices = append(indices, index)
		}
	}
	if len(indices) == 0 {
		return fmt.Errorf("%s: visual box styling is unsupported for an empty container", path)
	}
	if len(indices) != 1 && (box.BorderRadius != 0 || box.Shadow != (layout.BoxShadowStyle{})) {
		return fmt.Errorf("%s: rounded or shadowed containers require one plannable child", path)
	}
	for position, index := range indices {
		if htmlUnifiedVisualBox((*parts)[index].box) {
			return fmt.Errorf("%s: nested visual child boxes are outside the exact container cohort", path)
		}
		part := box
		if position != 0 {
			part.Margin.Top = 0
			part.Padding.Top = 0
			part.Border.Top = layout.BorderSide{}
		}
		if position != len(indices)-1 {
			part.Margin.Bottom = 0
			part.Padding.Bottom = 0
			part.Border.Bottom = layout.BorderSide{}
		}
		(*parts)[index].box = part
	}
	return nil
}

func paperMergeOuterBox(inner, outer layout.BoxStyle, path string) (layout.BoxStyle, error) {
	if inner.BackgroundColor.Set && outer.BackgroundColor.Set {
		return layout.BoxStyle{}, fmt.Errorf("%s: nested box backgrounds require an explicit stacking contract", path)
	}
	if !inner.BackgroundColor.Set {
		inner.BackgroundColor = outer.BackgroundColor
	}
	if inner.BorderRadius != 0 && outer.BorderRadius != 0 || inner.Shadow != (layout.BoxShadowStyle{}) && outer.Shadow != (layout.BoxShadowStyle{}) {
		return layout.BoxStyle{}, fmt.Errorf("%s: nested radius or shadow requires an explicit stacking contract", path)
	}
	if inner.BorderRadius == 0 {
		inner.BorderRadius = outer.BorderRadius
	}
	if inner.Shadow == (layout.BoxShadowStyle{}) {
		inner.Shadow = outer.Shadow
	}
	inner.Margin.Top += outer.Margin.Top
	inner.Margin.Right += outer.Margin.Right
	inner.Margin.Bottom += outer.Margin.Bottom
	inner.Margin.Left += outer.Margin.Left
	inner.Padding.Top += outer.Padding.Top
	inner.Padding.Right += outer.Padding.Right
	inner.Padding.Bottom += outer.Padding.Bottom
	inner.Padding.Left += outer.Padding.Left
	innerSides := []*layout.BorderSide{&inner.Border.Top, &inner.Border.Right, &inner.Border.Bottom, &inner.Border.Left}
	outerSides := []layout.BorderSide{outer.Border.Top, outer.Border.Right, outer.Border.Bottom, outer.Border.Left}
	for index, side := range outerSides {
		if side == (layout.BorderSide{}) {
			continue
		}
		if *innerSides[index] != (layout.BorderSide{}) {
			return layout.BoxStyle{}, fmt.Errorf("%s: nested borders on the same edge require an explicit stacking contract", path)
		}
		*innerSides[index] = side
	}
	return inner, nil
}

func paperApplySingleChildWrapperBox(candidate layout.Block, wrapper layout.BoxStyle, path string) (layout.Block, error) {
	paragraph, ok := paperParagraphBlock(candidate)
	if !ok {
		return nil, fmt.Errorf("%s: a visual section box requires one paragraph or heading child", path)
	}
	childBox := paragraph.EffectiveBox()
	childPolicy, childVisual, err := paperParagraphBoxPolicy(childBox, paragraph.BoxRef, path+".blocks[0]")
	if err != nil {
		return nil, err
	}
	if htmlUnifiedVisualBox(childVisual) {
		return nil, fmt.Errorf("%s: nested visual wrapper and child boxes are outside the exact cohort", path)
	}
	wrapper.KeepTogether = true
	wrapper.KeepWithNext = wrapper.KeepWithNext || childPolicy.keepWithNext
	wrapper.Orphans, wrapper.Widows = childPolicy.orphans, childPolicy.widows
	switch block := candidate.(type) {
	case layout.ParagraphBlock:
		block.Box, block.BoxRef = wrapper, nil
		return block, nil
	case layout.HeadingBlock:
		block.Box, block.BoxRef = wrapper, nil
		return block, nil
	default:
		return nil, fmt.Errorf("%s: a visual section box requires one paragraph or heading child", path)
	}
}

func (f *Document) paperMeasureBox(box layout.BoxStyle, path string) (paperMeasuredBox, error) {
	toFixed := func(value float64, field string) (layoutengine.Fixed, error) {
		fixed, err := fixedFromDocumentUnits(f, value)
		if err != nil || fixed < 0 {
			return 0, fmt.Errorf("%s.%s must be a finite non-negative fixed length", path, field)
		}
		return fixed, nil
	}
	spacing := func(value layout.Spacing, field string) (layoutengine.Insets, error) {
		var result layoutengine.Insets
		var err error
		if result.Top, err = toFixed(value.Top, field+".top"); err != nil {
			return layoutengine.Insets{}, err
		}
		if result.Right, err = toFixed(value.Right, field+".right"); err != nil {
			return layoutengine.Insets{}, err
		}
		if result.Bottom, err = toFixed(value.Bottom, field+".bottom"); err != nil {
			return layoutengine.Insets{}, err
		}
		if result.Left, err = toFixed(value.Left, field+".left"); err != nil {
			return layoutengine.Insets{}, err
		}
		return result, nil
	}
	margin, err := spacing(box.Margin, "box.margin")
	if err != nil {
		return paperMeasuredBox{}, err
	}
	padding, err := spacing(box.Padding, "box.padding")
	if err != nil {
		return paperMeasuredBox{}, err
	}
	measured := paperMeasuredBox{style: layoutengine.BoxFlowStyle{Margin: margin, Padding: padding}}
	for _, dimension := range []struct {
		name   string
		value  float64
		target *layoutengine.Fixed
	}{
		{"width", box.Width, &measured.width}, {"height", box.Height, &measured.height},
		{"min-width", box.MinWidth, &measured.minWidth}, {"min-height", box.MinHeight, &measured.minHeight},
		{"max-width", box.MaxWidth, &measured.maxWidth}, {"max-height", box.MaxHeight, &measured.maxHeight},
	} {
		if dimension.value == 0 {
			continue
		}
		fixed, dimensionErr := toFixed(dimension.value, "box."+dimension.name)
		if dimensionErr != nil || fixed <= 0 {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.%s must be a finite positive resolved length", path, dimension.name)
		}
		*dimension.target = fixed
		measured.visual = true
	}
	if measured.maxWidth > 0 && measured.maxWidth < measured.minWidth || measured.maxHeight > 0 && measured.maxHeight < measured.minHeight ||
		measured.width > 0 && (measured.width < measured.minWidth || measured.maxWidth > 0 && measured.width > measured.maxWidth) ||
		measured.height > 0 && (measured.height < measured.minHeight || measured.maxHeight > 0 && measured.height > measured.maxHeight) {
		return paperMeasuredBox{}, fmt.Errorf("%s.box resolved size constraints are inconsistent", path)
	}
	switch measured.overflow = strings.ToLower(strings.TrimSpace(box.Overflow)); measured.overflow {
	case "", "visible":
		measured.overflow = "visible"
	case "hidden", "clip":
		measured.overflow = "hidden"
		measured.visual = true
	default:
		return paperMeasuredBox{}, fmt.Errorf("%s.box.overflow supports visible or hidden", path)
	}
	if box.BackgroundColor.Set {
		if !validCoreGlyphColor(box.BackgroundColor) {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.background has invalid RGB components", path)
		}
		measured.background = coreGlyphColor(box.BackgroundColor)
		measured.visual = true
	}
	if box.BorderRadius != 0 {
		if measured.radius, err = toFixed(box.BorderRadius, "box.border-radius"); err != nil || measured.radius <= 0 {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.border-radius must be a finite positive fixed length", path)
		}
		measured.visual = true
	}
	if box.Shadow != (layout.BoxShadowStyle{}) {
		if !box.Shadow.Color.Set || !validCoreGlyphColor(box.Shadow.Color) {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.shadow requires an opaque RGB color", path)
		}
		if measured.shadow.OffsetX, err = fixedFromDocumentUnits(f, box.Shadow.OffsetX); err != nil {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.shadow.offset-x must be finite", path)
		}
		if measured.shadow.OffsetY, err = fixedFromDocumentUnits(f, box.Shadow.OffsetY); err != nil {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.shadow.offset-y must be finite", path)
		}
		if measured.shadow.Spread, err = fixedFromDocumentUnits(f, box.Shadow.Spread); err != nil {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.shadow.spread must be finite", path)
		}
		measured.shadow.Color = coreGlyphColor(box.Shadow.Color)
		measured.visual = true
	}
	sides := []struct {
		name   string
		source layout.BorderSide
		target *layoutengine.Fixed
	}{
		{"top", box.Border.Top, &measured.style.Border.Top},
		{"right", box.Border.Right, &measured.style.Border.Right},
		{"bottom", box.Border.Bottom, &measured.style.Border.Bottom},
		{"left", box.Border.Left, &measured.style.Border.Left},
	}
	for index, side := range sides {
		if side.source == (layout.BorderSide{}) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(side.source.Style), "solid") || side.source.Width <= 0 || !validCoreGlyphColor(side.source.Color) || !side.source.Color.Set {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.border.%s requires a positive solid RGB border", path, side.name)
		}
		width, widthErr := toFixed(side.source.Width, "box.border."+side.name+".width")
		if widthErr != nil || width <= 0 {
			return paperMeasuredBox{}, fmt.Errorf("%s.box.border.%s width is invalid", path, side.name)
		}
		*side.target = width
		measured.borders[index] = paperMeasuredBorder{width: width, color: coreGlyphColor(side.source.Color)}
		measured.visual = true
	}
	if measured.radius > 0 {
		first := measured.borders[0]
		for index := 1; index < len(measured.borders); index++ {
			if measured.borders[index] != first {
				return paperMeasuredBox{}, fmt.Errorf("%s.box rounded borders require equal solid width and color on all four sides", path)
			}
		}
	}
	if margin != (layoutengine.Insets{}) || padding != (layoutengine.Insets{}) {
		measured.visual = true
	}
	return measured, nil
}

func (box paperMeasuredBox) horizontalPoints() (left, right float64) {
	return box.style.Margin.Left.Points() + box.style.Border.Left.Points() + box.style.Padding.Left.Points(),
		box.style.Margin.Right.Points() + box.style.Border.Right.Points() + box.style.Padding.Right.Points()
}

func (box paperMeasuredBox) borderWidth(available layoutengine.Fixed) (layoutengine.Fixed, error) {
	width, err := available.Sub(box.style.Margin.Left)
	if err == nil {
		width, err = width.Sub(box.style.Margin.Right)
	}
	if box.width > 0 {
		width = box.width
	}
	if width < box.minWidth {
		width = box.minWidth
	}
	if box.maxWidth > 0 && width > box.maxWidth {
		width = box.maxWidth
	}
	if err != nil || width <= 0 || width > available {
		return 0, fmt.Errorf("resolved border width does not fit its containing block")
	}
	return width, nil
}

func (box paperMeasuredBox) contentWidth(available layoutengine.Fixed) (layoutengine.Fixed, error) {
	width, err := box.borderWidth(available)
	for _, inset := range []layoutengine.Fixed{box.style.Border.Left, box.style.Padding.Left, box.style.Padding.Right, box.style.Border.Right} {
		if err == nil {
			width, err = width.Sub(inset)
		}
	}
	if err != nil || width <= 0 {
		return 0, fmt.Errorf("resolved box edges leave no content width")
	}
	return width, nil
}

func (box paperMeasuredBox) contentHeight(lines []layoutengine.ParagraphLineInput) (layoutengine.Fixed, error) {
	var height layoutengine.Fixed
	for _, line := range lines {
		var err error
		height, err = height.Add(line.Height)
		if err != nil {
			return 0, err
		}
	}
	return height, nil
}

func (box paperMeasuredBox) outerHeight(content layoutengine.Fixed) (layoutengine.Fixed, error) {
	borderValues := []layoutengine.Fixed{box.style.Border.Top, box.style.Padding.Top, content, box.style.Padding.Bottom, box.style.Border.Bottom}
	var borderHeight layoutengine.Fixed
	for _, value := range borderValues {
		var err error
		borderHeight, err = borderHeight.Add(value)
		if err != nil {
			return 0, err
		}
	}
	if box.height > 0 {
		borderHeight = box.height
	}
	if borderHeight < box.minHeight {
		borderHeight = box.minHeight
	}
	if box.maxHeight > 0 && borderHeight > box.maxHeight {
		borderHeight = box.maxHeight
	}
	values := []layoutengine.Fixed{box.style.Margin.Top, borderHeight, box.style.Margin.Bottom}
	var result layoutengine.Fixed
	for _, value := range values {
		var err error
		result, err = result.Add(value)
		if err != nil {
			return 0, err
		}
	}
	return result, nil
}

func (box paperMeasuredBox) contentBoxHeight(natural layoutengine.Fixed) (layoutengine.Fixed, error) {
	outer, err := box.outerHeight(natural)
	if err != nil {
		return 0, err
	}
	height, err := outer.Sub(box.style.Margin.Top)
	for _, inset := range []layoutengine.Fixed{box.style.Border.Top, box.style.Padding.Top, box.style.Padding.Bottom, box.style.Border.Bottom, box.style.Margin.Bottom} {
		if err == nil {
			height, err = height.Sub(inset)
		}
	}
	if err != nil || height < 0 {
		return 0, fmt.Errorf("resolved box leaves an invalid content height")
	}
	return height, nil
}

func paperAttachBoxDecorations(plan layoutengine.LayoutPlan, measured []paperMeasuredBlock, explicit map[layoutengine.NodeID]layoutengine.Rect) (layoutengine.LayoutPlan, error) {
	byNode := make(map[layoutengine.NodeID]paperMeasuredBox)
	for _, block := range measured {
		if block.box.background.Set || block.box.radius > 0 || block.box.shadow.Color.Set || block.box.borders[0].width > 0 || block.box.borders[1].width > 0 || block.box.borders[2].width > 0 || block.box.borders[3].width > 0 {
			byNode[block.node] = block.box
		}
	}
	if len(byNode) == 0 {
		return plan, nil
	}
	projection := plan.Projection()
	fragmentCounts := make(map[layoutengine.NodeID]int, len(byNode))
	for _, fragment := range projection.Fragments {
		if _, exists := byNode[fragment.Node]; exists {
			fragmentCounts[fragment.Node]++
		}
	}
	decorations := make([]layoutengine.BoxDecoration, 0, len(byNode))
	for _, fragment := range projection.Fragments {
		box, exists := byNode[fragment.Node]
		if !exists {
			continue
		}
		if fragmentCounts[fragment.Node] != 1 && (box.radius > 0 || box.shadow.Color.Set) {
			return layoutengine.LayoutPlan{}, fmt.Errorf("rounded or shadowed box for node %d must fit one fragment", fragment.Node)
		}
		var borderBox *layoutengine.Rect
		if value, ok := explicit[fragment.Node]; ok {
			copy := value
			borderBox = &copy
		}
		decorations = append(decorations, layoutengine.BoxDecoration{
			Fragment: fragment.ID, Background: box.background,
			BorderBox: borderBox, Radius: box.radius, Shadow: box.shadow,
			Top:    layoutengine.BoxBorderSide{Width: box.borders[0].width, Color: box.borders[0].color},
			Right:  layoutengine.BoxBorderSide{Width: box.borders[1].width, Color: box.borders[1].color},
			Bottom: layoutengine.BoxBorderSide{Width: box.borders[2].width, Color: box.borders[2].color},
			Left:   layoutengine.BoxBorderSide{Width: box.borders[3].width, Color: box.borders[3].color},
		})
	}
	return layoutengine.AttachBoxDecorations(plan, decorations)
}

func paperAttachBoxOverflowClips(plan layoutengine.LayoutPlan, measured []paperMeasuredBlock) (layoutengine.LayoutPlan, error) {
	byNode := make(map[layoutengine.NodeID]paperMeasuredBox)
	for _, block := range measured {
		if block.box.overflow == "hidden" {
			byNode[block.node] = block.box
		}
	}
	if len(byNode) == 0 {
		return plan, nil
	}
	clips := make(map[layoutengine.FragmentID]layoutengine.Rect)
	for _, fragment := range plan.Projection().Fragments {
		box, exists := byNode[fragment.Node]
		if !exists {
			continue
		}
		x, err := fragment.BorderBox.X.Add(box.style.Border.Left)
		y, yErr := fragment.BorderBox.Y.Add(box.style.Border.Top)
		width, widthErr := fragment.BorderBox.Width.Sub(box.style.Border.Left)
		if widthErr == nil {
			width, widthErr = width.Sub(box.style.Border.Right)
		}
		height, heightErr := fragment.BorderBox.Height.Sub(box.style.Border.Top)
		if heightErr == nil {
			height, heightErr = height.Sub(box.style.Border.Bottom)
		}
		if err != nil || yErr != nil || widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("box overflow clip is invalid for fragment %d", fragment.ID)
		}
		clip, rectErr := layoutengine.NewRect(x, y, width, height)
		if rectErr != nil {
			return layoutengine.LayoutPlan{}, rectErr
		}
		clips[fragment.ID] = clip
	}
	return layoutengine.AttachFragmentClips(plan, clips)
}
