// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/layout"
)

type paperRowColumnMeasurement struct {
	identity     paperSourceIdentity
	plan         layoutengine.LayoutPlanProjection
	body         layoutengine.Rect
	height       layoutengine.Fixed
	role         layoutengine.SemanticRole
	heading      uint8
	image        *paperMeasuredImage
	alt          string
	table        *layoutengine.LayoutPlanProjection
	tableBodies  []layoutengine.Rect
	tableHeights []layoutengine.Fixed
}

type paperNestedTableSemantics struct {
	projection layoutengine.LayoutPlanProjection
	fragments  map[layoutengine.FragmentID]layoutengine.FragmentID
}

const paperRowColumnMaxNesting = 64

// planPaperRowColumnMapped is the initial authored-container compositor. It
// uses the same exact core-font line shadow and immutable display-plan painter
// as ordinary .paper text; only geometry comes from PlanRowColumn.
func (f *Document) planPaperRowColumnMapped(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, bodyIndex int, container layout.RowColumnBlock, selectBody paperBodySelector) (layoutengine.LayoutPlan, error) {
	return f.planPaperRowColumnMappedDepth(ctx, doc, mapping, bodyIndex, container, selectBody, 0)
}

func (f *Document) planPaperRowColumnMappedDepth(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, bodyIndex int, container layout.RowColumnBlock, selectBody paperBodySelector, depth uint32) (layoutengine.LayoutPlan, error) {
	if depth > paperRowColumnMaxNesting {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column nesting exceeds the deterministic depth limit")
	}
	if len(container.Items) == 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, "row/column container has no children")
	}
	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	contentWidth, bodyHeight := f.w-left-right, f.h-top-bottom
	if contentWidth <= 0 || bodyHeight <= 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "page margins leave no body area")
	}
	pageSize, body, err := typedShadowFixedGeometry(f, left, top, contentWidth, bodyHeight)
	if err != nil {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, err.Error())
	}
	pageBodyBase := body
	if selectBody != nil {
		body, err = selectBody(1, body)
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
	}
	direction, err := paperRowColumnDirection(container.Direction)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	gap, err := layoutengine.FixedFromPoints(container.Gap)
	if err != nil || gap < 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column gap is invalid")
	}
	wrapped := container.Wrap == "wrap" || container.Wrap == "wrap-reverse"
	if container.Wrap != "" && container.Wrap != "nowrap" && !wrapped {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column wrap mode is invalid")
	}
	crossGap, err := layoutengine.FixedFromPoints(container.CrossGap)
	if err != nil || crossGap < 0 {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column cross gap is invalid")
	}
	var definiteCross layoutengine.Fixed
	if container.CrossSize != 0 {
		definiteCross, err = layoutengine.FixedFromPoints(container.CrossSize)
		if err != nil || definiteCross <= 0 {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column definite cross size is invalid")
		}
	}
	if wrapped && definiteCross <= 0 && direction == layoutengine.RowDirection {
		return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "wrapped row/column requires a definite cross size")
	}

	children := make([]layoutengine.RowColumnChild, len(container.Items))
	explicitCross := make([]layoutengine.Fixed, len(container.Items))
	for index, item := range container.Items {
		identity := paperBlockIdentity(mapping, bodyIndex, index, -1, index)
		track, trackErr := paperRowColumnTrack(item.Track)
		if trackErr != nil {
			return layoutengine.LayoutPlan{}, fmt.Errorf("body[%d] row/column child %d: %w", bodyIndex, index, trackErr)
		}
		children[index] = layoutengine.RowColumnChild{
			Node: layoutengine.NodeID(index + 1), Key: identity.key, Instance: identity.instance,
			Source: identity.source, Track: track, Align: layoutengine.CrossAlignment(item.CrossAlign),
		}
		children[index].CrossMin, trackErr = layoutengine.FixedFromPoints(item.CrossMin)
		if trackErr != nil {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("row/column child %d cross minimum is invalid", index))
		}
		children[index].CrossMax, trackErr = layoutengine.FixedFromPoints(item.CrossMax)
		if trackErr != nil {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("row/column child %d cross maximum is invalid", index))
		}
		children[index].CrossMinPercent, children[index].CrossMaxPercent = item.CrossMinPercent, item.CrossMaxPercent
		if item.CrossSize != 0 {
			explicitCross[index], trackErr = layoutengine.FixedFromPoints(item.CrossSize)
			if trackErr != nil || explicitCross[index] <= 0 {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("row/column child %d cross size is invalid", index))
			}
		}
	}
	mainAvailable := body.Width
	if direction == layoutengine.ColumnDirection {
		mainAvailable = body.Height
	}
	// Content/auto flex bases are measured before the geometry probe. The
	// exact preferred size becomes the flex base while MinMain carries the
	// min-content floor; the fixed-point planner remains the sole distributor.
	for index, item := range container.Items {
		if children[index].Track.Kind != layoutengine.RowColumnTrackFlex || children[index].Track.BasisKind != layoutengine.RowColumnFlexBasisContent {
			continue
		}
		paragraph, ok := paperParagraphBlock(item.Block)
		if !ok {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, fmt.Sprintf("row/column child %d content basis requires measurable structured support", index))
		}
		if direction == layoutengine.RowDirection {
			minimum, preferred, measureErr := f.measurePaperRowColumnTextIntrinsic(ctx, paragraph)
			if measureErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d intrinsic sizing: %w", index, measureErr)
			}
			children[index].MinMain, children[index].ContentMain = minimum, preferred
		} else {
			measurement, measureErr := f.measurePaperRowColumnChild(ctx, doc, paragraph, left, top, right, bottom, body.Width)
			if measureErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d intrinsic sizing: %w", index, measureErr)
			}
			children[index].MinMain, children[index].ContentMain = measurement.height, measurement.height
		}
	}
	if !wrapped {
		requiredMain, minimumErr := paperRowColumnAuthoredMainMinimum(children, gap)
		if minimumErr != nil {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column authored main-axis sizing is invalid")
		}
		if requiredMain > mainAvailable {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(
				typedShadowGeometry,
				fmt.Sprintf("resolve %s tracks: authored fixed/minimum tracks plus gaps require %d, available %d", direction, requiredMain, mainAvailable),
			)
		}
	}

	measureWidths := make([]layoutengine.Fixed, len(children))
	if direction == layoutengine.RowDirection {
		probeRegion := body
		if definiteCross > 0 {
			probeRegion.Height = definiteCross
		}
		probe, probeErr := layoutengine.PlanRowColumn(ctx, layoutengine.RowColumnPlanInput{
			PageSize: pageSize, Region: probeRegion, Direction: direction, Gap: gap,
			Align: layoutengine.CrossStart, Wrap: layoutengine.RowColumnWrap(container.Wrap), CrossGap: crossGap,
			AlignContent: layoutengine.ContentAlignment(container.AlignContent), ReverseMain: container.ReverseMain, Children: children,
		}, layoutengine.RowColumnPlanLimits{})
		if probeErr != nil {
			if wrapped {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "plan wrapped row/column: "+probeErr.Error())
			}
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: resolve row tracks: %w", probeErr)
		}
		measureWidths = probe.MainSizes()
	} else {
		for index := range measureWidths {
			measureWidths[index] = body.Width
			if explicitCross[index] > 0 {
				measureWidths[index] = explicitCross[index]
			}
			if wrapped && explicitCross[index] <= 0 {
				paragraph, measurable := paperParagraphBlock(container.Items[index].Block)
				if !measurable {
					if _, nested := container.Items[index].Block.(layout.RowColumnBlock); !nested {
						return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry,
							fmt.Sprintf("intrinsic wrapped column child %d is not measurably bounded", index))
					}
				} else {
					_, preferred, intrinsicErr := f.measurePaperRowColumnTextIntrinsic(ctx, paragraph)
					if intrinsicErr != nil {
						return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d intrinsic cross sizing: %w", index, intrinsicErr)
					}
					if preferred < body.Width {
						measureWidths[index] = preferred
					}
				}
			}
		}
	}

	measurements := make([]paperRowColumnMeasurement, len(container.Items))
	var maxHeight layoutengine.Fixed
	for index, item := range container.Items {
		var measurement paperRowColumnMeasurement
		paragraph, isText := paperParagraphBlock(item.Block)
		if isText {
			for _, segment := range paragraph.Segments {
				if segment.Link != "" || segment.Destination != "" {
					return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind,
						fmt.Sprintf("row/column child %d link and destination segments are not represented", index))
				}
			}
			var measureErr error
			measurement, measureErr = f.measurePaperRowColumnChild(ctx, doc, paragraph, left, top, right, bottom, measureWidths[index])
			if measureErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d: %w", index, measureErr)
			}
		} else if imageBlock, isImage := item.Block.(layout.ImageBlock); isImage {
			path := fmt.Sprintf("body[%d].items[%d].image", bodyIndex, index)
			if len(imageBlock.Caption) != 0 {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, fmt.Sprintf("row/column child %d image captions are not represented", index))
			}
			if err := validateTypedPlanningImage(imageBlock, path); err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			measured, measureErr := f.measureTypedPlanningImageContext(ctx, imageBlock, f.PointConvert(measureWidths[index].Points()))
			if measureErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d: %w", index, measureErr)
			}
			measurement.height = measured.height
			measurement.image = &measured
			measurement.alt = imageBlock.Alt
			measurement.role = layoutengine.SemanticRoleArtifact
			if imageBlock.Alt != "" {
				measurement.role = layoutengine.SemanticRoleFigure
			}
		} else if tableBlock, isTable := item.Block.(layout.TableBlock); isTable {
			measured, measureErr := f.measurePaperRowColumnTable(ctx, doc, tableBlock,
				fmt.Sprintf("body[%d].items[%d].table", bodyIndex, index), pageBodyBase, measureWidths[index], selectBody)
			if measureErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d: %w", index, measureErr)
			}
			measurement = measured
			measurement.role = layoutengine.SemanticRoleArtifact
		} else if nested, isNested := item.Block.(layout.RowColumnBlock); isNested {
			measured, measureErr := f.measurePaperNestedRowColumn(ctx, doc, mapping, bodyIndex, nested, body, measureWidths[index], depth+1)
			if measureErr != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("row/column child %d: %w", index, measureErr)
			}
			measurement = measured
			measurement.role = layoutengine.SemanticRoleSection
		} else {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, fmt.Sprintf("row/column child %d is %s", index, item.Block.DocumentBlockKind()))
		}
		measurement.identity = paperBlockIdentity(mapping, bodyIndex, index, -1, index)
		if isText {
			measurement.role = layoutengine.SemanticRoleParagraph
			if heading, isHeading := item.Block.(layout.HeadingBlock); isHeading {
				measurement.role = layoutengine.SemanticRoleHeading
				if heading.Level > 0 && heading.Level <= 255 {
					measurement.heading = uint8(heading.Level)
				}
			}
		}
		measurements[index] = measurement
		if measurement.height > maxHeight {
			maxHeight = measurement.height
		}
		if direction == layoutengine.ColumnDirection {
			children[index].MinMain = measurement.height
			children[index].CrossSize = body.Width
			if wrapped {
				children[index].CrossSize = measureWidths[index]
			}
			if measurement.image != nil {
				children[index].CrossSize = measurement.image.width
			} else if measurement.table != nil {
				children[index].CrossSize = measureWidths[index]
			}
			if explicitCross[index] > 0 {
				if measurement.image != nil && explicitCross[index] < measurement.image.width {
					return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("row/column child %d cross size is smaller than its measured image", index))
				}
				children[index].CrossSize = explicitCross[index]
			}
		} else {
			children[index].CrossSize = measurement.height
			if explicitCross[index] > 0 {
				if explicitCross[index] < measurement.height {
					return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("row/column child %d cross size is smaller than its measured content", index))
				}
				children[index].CrossSize = explicitCross[index]
			}
			if children[index].CrossSize > maxHeight {
				maxHeight = children[index].CrossSize
			}
		}
	}

	region := body
	if definiteCross > 0 {
		if direction == layoutengine.RowDirection {
			if definiteCross > body.Height {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column definite cross size exceeds the body")
			}
			region.Height = definiteCross
		} else {
			if definiteCross > body.Width {
				return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column definite cross size exceeds the body")
			}
			region.Width = definiteCross
		}
	} else if direction == layoutengine.RowDirection {
		if maxHeight <= 0 || maxHeight > body.Height {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "row children must fit one body page")
		}
		region.Height = maxHeight
	} else if wrapped {
		// A wrapped column without an authored width resolves to the smallest
		// definite cross extent that contains its greedily formed lines. The
		// probe has no align-content free-space distribution, so this remains a
		// bounded intrinsic measurement rather than a second layout policy.
		probe, probeErr := layoutengine.PlanRowColumn(ctx, layoutengine.RowColumnPlanInput{
			PageSize: pageSize, Region: body, Direction: direction, Gap: gap,
			Align: layoutengine.CrossAlignment(container.CrossAlign), Justify: layoutengine.MainAlignment(container.MainAlign),
			Wrap: layoutengine.RowColumnWrap(container.Wrap), CrossGap: crossGap,
			AlignContent: layoutengine.ContentStart, ReverseMain: container.ReverseMain, Children: children,
		}, layoutengine.RowColumnPlanLimits{})
		if probeErr != nil {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "resolve intrinsic wrapped column: "+probeErr.Error())
		}
		var intrinsicCross layoutengine.Fixed
		for lineIndex, line := range probe.Lines() {
			if lineIndex > 0 {
				intrinsicCross, err = intrinsicCross.Add(crossGap)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			intrinsicCross, err = intrinsicCross.Add(line.CrossSize)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
		}
		if intrinsicCross <= 0 || intrinsicCross > body.Width {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "intrinsic wrapped column width exceeds its bounded body")
		}
		region.Width = intrinsicCross
	}
	planned, err := layoutengine.PlanRowColumn(ctx, layoutengine.RowColumnPlanInput{
		PageSize: pageSize, Region: region, Direction: direction, Gap: gap,
		Align: layoutengine.CrossAlignment(container.CrossAlign), Justify: layoutengine.MainAlignment(container.MainAlign),
		Wrap: layoutengine.RowColumnWrap(container.Wrap), CrossGap: crossGap,
		AlignContent: layoutengine.ContentAlignment(container.AlignContent), ReverseMain: container.ReverseMain, Children: children,
	}, layoutengine.RowColumnPlanLimits{})
	if err != nil {
		if wrapped {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "plan wrapped row/column: "+err.Error())
		}
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: plan row/column: %w", err)
	}
	return composePaperRowColumnPlan(planned.Plan, measurements)
}

func (f *Document) measurePaperNestedRowColumn(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, bodyIndex int, nested layout.RowColumnBlock, body layoutengine.Rect, width layoutengine.Fixed, depth uint32) (paperRowColumnMeasurement, error) {
	if width <= 0 || width > body.Width {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested row/column width is outside its parent track")
	}
	childBody := body
	childBody.Width = width
	child := *doc
	child.Body = []layout.Block{nested}
	planned, err := f.planPaperRowColumnMappedDepth(ctx, &child, mapping, bodyIndex, nested, func(page uint32, _ layoutengine.Rect) (layoutengine.Rect, error) {
		if page != 1 {
			return layoutengine.Rect{}, newTypedShadowUnsupported(typedShadowGeometry, "nested row/column must fit one parent-page track")
		}
		return childBody, nil
	}, depth)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	projection := planned.Projection()
	if len(projection.Pages) != 1 {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested row/column must produce exactly one page")
	}
	var contentBottom layoutengine.Fixed
	for _, fragment := range projection.Fragments {
		candidate, bottomErr := fragment.BorderBox.Bottom()
		if bottomErr != nil {
			return paperRowColumnMeasurement{}, bottomErr
		}
		if candidate > contentBottom {
			contentBottom = candidate
		}
	}
	height, err := contentBottom.Sub(childBody.Y)
	if err != nil || height <= 0 || height > childBody.Height {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested row/column height is outside its parent body")
	}
	return paperRowColumnMeasurement{
		plan: projection, body: childBody, height: height, role: layoutengine.SemanticRoleSection,
		table: &projection, tableBodies: []layoutengine.Rect{childBody}, tableHeights: []layoutengine.Fixed{height},
	}, nil
}

func (f *Document) measurePaperRowColumnTable(ctx context.Context, doc *layout.LayoutDocument, table layout.TableBlock, path string, body layoutengine.Rect, width layoutengine.Fixed, selectBody paperBodySelector) (paperRowColumnMeasurement, error) {
	widthUser := f.PointConvert(width.Points())
	left := f.PointConvert(body.X.Points())
	top := f.PointConvert(body.Y.Points())
	bodyBottom, err := body.Bottom()
	if err != nil || widthUser <= 0 {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested table track geometry is invalid")
	}
	bottom := f.h - f.PointConvert(bodyBottom.Points())
	right := f.w - left - widthUser
	if right < 0 {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested table track exceeds the page body")
	}
	child := *doc
	child.Body = []layout.Block{table}
	child.PageTemplate = layout.PageTemplate{Margins: layout.Spacing{Left: left, Top: top, Right: right, Bottom: bottom}}
	selectedBodies := make([]layoutengine.Rect, 0, 1)
	nestedBody := func(page uint32, base layoutengine.Rect) (layoutengine.Rect, error) {
		selected := body
		if selectBody != nil {
			var selectErr error
			selected, selectErr = selectBody(page, body)
			if selectErr != nil {
				return layoutengine.Rect{}, selectErr
			}
		}
		selected.Width = width
		for uint32(len(selectedBodies)) < page { // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			selectedBodies = append(selectedBodies, layoutengine.Rect{})
		}
		selectedBodies[page-1] = selected
		return selected, nil
	}
	planned, err := f.planTypedTableBodies(ctx, &child, table, path, nestedBody)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	projection := planned.Projection()
	if len(projection.Pages) == 0 || len(selectedBodies) != len(projection.Pages) {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested table page-body selection is inconsistent")
	}
	heights := make([]layoutengine.Fixed, len(projection.Pages))
	for pageIndex, page := range projection.Pages {
		bodyForPage := selectedBodies[pageIndex]
		var tableBottom layoutengine.Fixed
		end := int(page.Fragments.Start + page.Fragments.Count)
		for _, fragment := range projection.Fragments[page.Fragments.Start:end] {
			if fragment.Region != layoutengine.RegionBody {
				continue
			}
			candidate, bottomErr := fragment.BorderBox.Bottom()
			if bottomErr != nil {
				return paperRowColumnMeasurement{}, bottomErr
			}
			if candidate > tableBottom {
				tableBottom = candidate
			}
		}
		height, heightErr := tableBottom.Sub(bodyForPage.Y)
		if heightErr != nil || height <= 0 || height > bodyForPage.Height {
			return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "nested table page height is invalid or exceeds its page-master body")
		}
		heights[pageIndex] = height
	}
	return paperRowColumnMeasurement{plan: projection, body: selectedBodies[0], height: heights[0], table: &projection, tableBodies: selectedBodies, tableHeights: heights}, nil
}

// paperRowColumnAuthoredMainMinimum rejects geometry the authored constraints
// cannot represent before measurement or planner work begins. Intrinsic child
// minima are intentionally excluded here and remain the solver's concern.
func paperRowColumnAuthoredMainMinimum(children []layoutengine.RowColumnChild, gap layoutengine.Fixed) (layoutengine.Fixed, error) {
	required := layoutengine.Fixed(0)
	for _, child := range children {
		minimum := child.Track.Min
		if child.Track.Kind == layoutengine.RowColumnTrackFixed {
			minimum = child.Track.Size
		} else if child.Track.Kind == layoutengine.RowColumnTrackFlex && child.Track.BasisKind == layoutengine.RowColumnFlexBasisFixed && child.Track.Shrink == 0 {
			minimum = child.Track.Basis
			if child.Track.Max > 0 && minimum > child.Track.Max {
				minimum = child.Track.Max
			}
			if child.Track.Min > minimum {
				minimum = child.Track.Min
			}
		}
		var err error
		required, err = required.Add(minimum)
		if err != nil {
			return 0, err
		}
	}
	if len(children) <= 1 {
		return required, nil
	}
	gaps, err := gap.MulInt(int64(len(children) - 1))
	if err != nil {
		return 0, err
	}
	return required.Add(gaps)
}

func (f *Document) measurePaperRowColumnChild(ctx context.Context, doc *layout.LayoutDocument, paragraph layout.ParagraphBlock, left, top, right, bottom float64, width layoutengine.Fixed) (paperRowColumnMeasurement, error) {
	widthUser := f.PointConvert(width.Points())
	maximumWidth := f.w - left - right
	if widthUser > maximumWidth && widthUser-maximumWidth <= f.PointConvert(1.0/1024.0) {
		widthUser = maximumWidth
	}
	if widthUser <= 0 || widthUser > maximumWidth {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowGeometry, "resolved child width is empty or outside the body")
	}
	single := *doc
	single.Body = []layout.Block{paragraph}
	single.PageTemplate = layout.PageTemplate{Margins: layout.Spacing{Left: left, Top: top, Right: f.w - left - widthUser, Bottom: bottom}}
	shadow, err := f.planTypedParagraphLineShadowContext(ctx, &single)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	projection := shadow.Plan.Projection()
	if len(projection.Pages) != 1 {
		return paperRowColumnMeasurement{}, newTypedShadowUnsupported(typedShadowParagraphContract, "row/column children cannot paginate internally")
	}
	_, measureBody, err := typedShadowFixedGeometry(f, left, top, widthUser, f.h-top-bottom)
	if err != nil {
		return paperRowColumnMeasurement{}, err
	}
	var height layoutengine.Fixed
	for _, line := range projection.Lines {
		height, err = height.Add(line.Bounds.Height)
		if err != nil {
			return paperRowColumnMeasurement{}, err
		}
	}
	return paperRowColumnMeasurement{plan: projection, body: measureBody, height: height}, nil
}

func (f *Document) measurePaperRowColumnTextIntrinsic(ctx context.Context, paragraph layout.ParagraphBlock) (layoutengine.Fixed, layoutengine.Fixed, error) {
	if err := layoutengine.ChargePlanningWork(ctx, "row or column intrinsic text measurement", 1); err != nil {
		return 0, 0, err
	}
	scratch := documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
	scratch.cMargin, scratch.ws = f.cMargin, f.ws
	scratch.fontFamily, scratch.fontStyle = f.fontFamily, f.fontStyle
	scratch.fontSizePt, scratch.fontSize = f.fontSizePt, f.fontSizePt/scratch.k
	style := layout.MergedTextStyle(plannerDefaultTextStyle(scratch), paragraph.EffectiveStyle())
	applyPlannerTextStyle(scratch, style)
	if scratch.err != nil || scratch.isCurrentUTF8 {
		return 0, 0, newTypedShadowUnsupported(typedShadowGeometry, "core font metrics could not be resolved for intrinsic flex sizing")
	}
	box := layout.ParagraphBox(paragraph.EffectiveBox())
	base := box.Margin.Left + box.Margin.Right + box.Padding.Left + box.Padding.Right + box.Border.Left.Width + box.Border.Right.Width
	minimum, preferred := base, base
	text := normalizeCoreMultiCellText(layout.TextSegmentsPlainText(paragraph.Segments))
	for _, line := range strings.Split(text, "\n") {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		if width := scratch.GetStringWidth(line) + base; width > preferred {
			preferred = width
		}
		for _, word := range strings.Fields(line) {
			if width := scratch.GetStringWidth(word) + base; width > minimum {
				minimum = width
			}
		}
	}
	minimumFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(minimum))
	if err != nil || minimumFixed <= 0 {
		return 0, 0, newTypedShadowUnsupported(typedShadowGeometry, "minimum intrinsic flex width is not representable")
	}
	preferredFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(preferred))
	if err != nil || preferredFixed < minimumFixed {
		return 0, 0, newTypedShadowUnsupported(typedShadowGeometry, "preferred intrinsic flex width is not representable")
	}
	return minimumFixed, preferredFixed, nil
}

func composePaperRowColumnPlan(base layoutengine.LayoutPlan, measurements []paperRowColumnMeasurement) (layoutengine.LayoutPlan, error) {
	projection := base.Projection()
	if len(projection.Pages) != 1 || len(projection.Fragments) != len(measurements) {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: row/column geometry has inconsistent cardinality")
	}
	paginatedTable := -1
	for index, measurement := range measurements {
		if measurement.table == nil || len(measurement.table.Pages) <= 1 {
			continue
		}
		if paginatedTable >= 0 {
			return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowGeometry, "only one independently paginated nested table is supported within a row/column container")
		}
		paginatedTable = index
		if len(measurement.tableBodies) != len(measurement.table.Pages) || len(measurement.tableHeights) != len(measurement.table.Pages) {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested table pagination metadata is inconsistent")
		}
		outer := &projection.Fragments[index]
		outer.Continuation = layoutengine.ContinuationStart
		var err error
		outer.BorderBox, err = layoutengine.NewRect(outer.BorderBox.X, outer.BorderBox.Y, outer.BorderBox.Width, measurement.tableHeights[0])
		if err == nil {
			outer.ContentBox = outer.BorderBox
		}
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
	}
	fonts := make([]layoutengine.CoreFontResource, 0)
	fontIndex := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	runs := make([]layoutengine.CoreGlyphRun, 0)
	imageResources := make([]layoutengine.ImageResource, 0)
	imageIndex := make(map[layoutengine.ImageContentDigest]layoutengine.ImageResourceID)
	images := make([]layoutengine.PlannedImage, 0)
	paths := make([]layoutengine.PlannedPath, 0)
	fills := make([]layoutengine.PlannedFill, 0)
	strokes := make([]layoutengine.PlannedStroke, 0)
	destinations := make([]layoutengine.PlannedDestination, 0)
	links := make([]layoutengine.PlannedLink, 0)
	items := make([]layoutengine.DisplayItem, 0)
	nestedSemantics := make(map[int]paperNestedTableSemantics)
	nestedOuterFragments := make(map[int][]layoutengine.FragmentID)
	nextNestedNode := layoutengine.NodeID(len(measurements)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
	compositionOrder := make([]int, 0, len(measurements))
	for index := range measurements {
		if index != paginatedTable {
			compositionOrder = append(compositionOrder, index)
		}
	}
	if paginatedTable >= 0 {
		compositionOrder = append(compositionOrder, paginatedTable)
	}
	for _, childIndex := range compositionOrder {
		measurement := measurements[childIndex]
		fragment := projection.Fragments[childIndex]
		if measurement.table != nil {
			nested := *measurement.table
			var err error
			fragmentMap := make(map[layoutengine.FragmentID]layoutengine.FragmentID, len(nested.Fragments))
			fragmentPages := make(map[layoutengine.FragmentID]uint32, len(nested.Fragments))
			nodeMap := make(map[layoutengine.NodeID]layoutengine.NodeID)
			outerFragments := []layoutengine.FragmentID{fragment.ID}
			currentPage := uint32(1)
			for _, childFragment := range nested.Fragments {
				if childFragment.Page == 0 || int(childFragment.Page) > len(measurement.tableBodies) {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested table fragment has an invalid page")
				}
				for currentPage < childFragment.Page {
					currentPage++
					outerBody := measurement.tableBodies[currentPage-1]
					xOffset, offsetErr := fragment.BorderBox.X.Sub(measurement.tableBodies[0].X)
					if offsetErr != nil {
						return layoutengine.LayoutPlan{}, offsetErr
					}
					yOffset, offsetErr := fragment.BorderBox.Y.Sub(measurement.tableBodies[0].Y)
					if offsetErr != nil {
						return layoutengine.LayoutPlan{}, offsetErr
					}
					x, offsetErr := outerBody.X.Add(xOffset)
					if offsetErr != nil {
						return layoutengine.LayoutPlan{}, offsetErr
					}
					y, offsetErr := outerBody.Y.Add(yOffset)
					if offsetErr != nil {
						return layoutengine.LayoutPlan{}, offsetErr
					}
					outerBox, boxErr := layoutengine.NewRect(x, y, fragment.BorderBox.Width, measurement.tableHeights[currentPage-1])
					if boxErr != nil {
						return layoutengine.LayoutPlan{}, boxErr
					}
					continuation := layoutengine.ContinuationMiddle
					if int(currentPage) == len(nested.Pages) {
						continuation = layoutengine.ContinuationEnd
					}
					outer := fragment
					outer.ID = layoutengine.FragmentID(len(projection.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					outer.Page = currentPage
					outer.MarginBox, outer.BorderBox, outer.PaddingBox, outer.ContentBox = outerBox, outerBox, outerBox, outerBox
					outer.Continuation = continuation
					projection.Fragments = append(projection.Fragments, outer)
					outerFragments = append(outerFragments, outer.ID)
				}
				sourceBody := measurement.tableBodies[childFragment.Page-1]
				targetOuter := projection.Fragments[int(outerFragments[childFragment.Page-1])-1]
				dx, err := targetOuter.BorderBox.X.Sub(sourceBody.X)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				dy, err := targetOuter.BorderBox.Y.Sub(sourceBody.Y)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				oldID, oldNode := childFragment.ID, childFragment.Node
				if !nodeMap[oldNode].Valid() {
					nextNestedNode++
					nodeMap[oldNode] = nextNestedNode
				}
				childFragment.ID = layoutengine.FragmentID(len(projection.Fragments) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				childFragment.Node = nodeMap[oldNode]
				childFragment.Key = layoutengine.NodeKey(string(measurement.identity.key) + "/" + string(childFragment.Key))
				childFragment.Instance = layoutengine.InstanceID(string(measurement.identity.instance) + "/" + string(childFragment.Instance))
				childFragment.Region = layoutengine.RegionBody
				childFragment.MarginBox, err = translateTypedRect(childFragment.MarginBox, dx, dy)
				if err == nil {
					childFragment.BorderBox, err = translateTypedRect(childFragment.BorderBox, dx, dy)
				}
				if err == nil {
					childFragment.PaddingBox, err = translateTypedRect(childFragment.PaddingBox, dx, dy)
				}
				if err == nil {
					childFragment.ContentBox, err = translateTypedRect(childFragment.ContentBox, dx, dy)
				}
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				projection.Fragments = append(projection.Fragments, childFragment)
				fragmentMap[oldID] = childFragment.ID
				fragmentPages[oldID] = childFragment.Page
			}
			nestedOuterFragments[childIndex] = outerFragments
			offsetForPage := func(page uint32) (layoutengine.Fixed, layoutengine.Fixed, error) {
				if page == 0 || int(page) > len(outerFragments) {
					return 0, 0, fmt.Errorf("document: nested table payload has an invalid page")
				}
				target := projection.Fragments[int(outerFragments[page-1])-1].BorderBox
				source := measurement.tableBodies[page-1]
				dx, offsetErr := target.X.Sub(source.X)
				if offsetErr != nil {
					return 0, 0, offsetErr
				}
				dy, offsetErr := target.Y.Sub(source.Y)
				return dx, dy, offsetErr
			}
			localFonts := make(map[layoutengine.FontResourceID]layoutengine.FontResourceID)
			for _, font := range nested.Fonts {
				localID := font.ID
				identity := paperFontIdentity(font)
				globalID, exists := fontIndex[identity]
				if !exists {
					globalID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
					font.ID = globalID
					fonts = append(fonts, font)
					fontIndex[identity] = globalID
				}
				localFonts[localID] = globalID
			}
			lineMap := make(map[uint32]uint32, len(nested.Lines))
			for oldIndex, line := range nested.Lines {
				page := fragmentPages[line.Fragment]
				dx, dy, offsetErr := offsetForPage(page)
				if offsetErr != nil {
					return layoutengine.LayoutPlan{}, offsetErr
				}
				line.Fragment = fragmentMap[line.Fragment]
				line.Bounds, err = translateTypedRect(line.Bounds, dx, dy)
				if err == nil {
					line.Baseline, err = line.Baseline.Add(dy)
				}
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				lineMap[uint32(oldIndex)] = uint32(len(projection.Lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				projection.Lines = append(projection.Lines, line)
			}
			pathPages := make(map[uint32]uint32)
			for _, fill := range nested.Fills {
				pathPages[fill.Path] = fragmentPages[fill.Fragment]
			}
			for _, stroke := range nested.Strokes {
				pathPages[stroke.Path] = fragmentPages[stroke.Fragment]
			}
			pathBase := uint32(len(paths)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			for pathIndex, path := range nested.Paths {
				dx, dy, offsetErr := offsetForPage(pathPages[uint32(pathIndex)])
				if offsetErr != nil {
					return layoutengine.LayoutPlan{}, offsetErr
				}
				translated, translateErr := translatePaperNestedPath(path, dx, dy)
				if translateErr != nil {
					return layoutengine.LayoutPlan{}, translateErr
				}
				paths = append(paths, translated)
			}
			destinationMap := make(map[layoutengine.DestinationID]layoutengine.DestinationID, len(nested.Destinations))
			for _, destination := range nested.Destinations {
				dx, dy, offsetErr := offsetForPage(destination.Page)
				if offsetErr != nil {
					return layoutengine.LayoutPlan{}, offsetErr
				}
				oldID := destination.ID
				destination.ID = layoutengine.DestinationID(len(destinations) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				if destination.Fragment.Valid() {
					destination.Fragment = fragmentMap[destination.Fragment]
				}
				destination.Point, err = translateTypedPoint(destination.Point, dx, dy)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				destinationMap[oldID] = destination.ID
				destinations = append(destinations, destination)
			}
			for _, command := range nested.Commands {
				page := fragmentPages[command.Fragment]
				if command.Kind == layoutengine.CommandGlyphRun && uint64(command.Payload) < uint64(len(nested.GlyphRuns)) {
					page = fragmentPages[nested.Lines[nested.GlyphRuns[command.Payload].Line].Fragment]
				}
				dx, dy, offsetErr := offsetForPage(page)
				if offsetErr != nil {
					return layoutengine.LayoutPlan{}, offsetErr
				}
				switch command.Kind {
				case layoutengine.CommandGlyphRun:
					run := nested.GlyphRuns[command.Payload]
					run.Line, run.Font = lineMap[run.Line], localFonts[run.Font]
					run.Origin, err = translateTypedPoint(run.Origin, dx, dy)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					runs = append(runs, run)
					items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(runs) - 1), Page: page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				case layoutengine.CommandFillPath:
					fill := nested.Fills[command.Payload]
					fill.Path += pathBase
					fill.Fragment = fragmentMap[fill.Fragment]
					fills = append(fills, fill)
					items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(fills) - 1), Page: page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				case layoutengine.CommandStrokePath:
					stroke := nested.Strokes[command.Payload]
					stroke.Path += pathBase
					stroke.Fragment = fragmentMap[stroke.Fragment]
					strokes = append(strokes, stroke)
					items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(strokes) - 1), Page: page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				case layoutengine.CommandLink:
					link := nested.Links[command.Payload]
					if link.Destination.Valid() {
						link.Destination = destinationMap[link.Destination]
						if !link.Destination.Valid() {
							return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested table link destination is missing")
						}
					}
					link.Fragment = fragmentMap[link.Fragment]
					link.Bounds, err = translateTypedRect(link.Bounds, dx, dy)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					links = append(links, link)
					items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(links) - 1), Page: page}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				default:
					return layoutengine.LayoutPlan{}, newTypedShadowUnsupported(typedShadowBlockKind, fmt.Sprintf("nested table command %q is unsupported", command.Kind))
				}
			}
			for _, decision := range nested.Breaks {
				decision.Preceding = fragmentMap[decision.Preceding]
				decision.Triggering = fragmentMap[decision.Triggering]
				if !decision.Preceding.Valid() || !decision.Triggering.Valid() {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested table break references a missing fragment")
				}
				projection.Breaks = append(projection.Breaks, decision)
			}
			nestedSemantics[childIndex] = paperNestedTableSemantics{projection: nested, fragments: fragmentMap}
			continue
		}
		if measurement.image != nil {
			image := *measurement.image
			x, err := image.targetX(fragment.BorderBox)
			if err != nil {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: row/column image %d: %w", childIndex, err)
			}
			outer, err := layoutengine.NewRect(x, fragment.BorderBox.Y, image.width, image.height)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			content, err := image.contentBox(outer)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			projection.Fragments[childIndex].MarginBox = outer
			projection.Fragments[childIndex].BorderBox = outer
			projection.Fragments[childIndex].PaddingBox = content
			projection.Fragments[childIndex].ContentBox = content
			fragment = projection.Fragments[childIndex]
			if image.background.Set {
				paths = append(paths, typedTableRectPath(outer))
				fills = append(fills, layoutengine.PlannedFill{Path: uint32(len(paths) - 1), Rule: layoutengine.FillNonZero, Color: image.background, Fragment: fragment.ID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandFillPath, Payload: uint32(len(fills) - 1)})                                          // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			}
			resourceID, exists := imageIndex[image.resource.Digest]
			if !exists {
				resourceID = layoutengine.ImageResourceID(len(imageResources) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				resource := image.resource
				resource.ID = resourceID
				imageResources = append(imageResources, resource)
				imageIndex[resource.Digest] = resourceID
			}
			image.resource.ID = resourceID
			placement, err := image.placement(fragment.ID, outer)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			placement.Source = measurement.identity.source
			images = append(images, placement)
			items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandImage, Payload: uint32(len(images) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			for side, border := range image.borders {
				if border.width <= 0 {
					continue
				}
				path, err := typedTableBorderPath(outer, side)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				paths = append(paths, path)
				strokes = append(strokes, layoutengine.PlannedStroke{Path: uint32(len(paths) - 1), Color: border.color, Width: border.width, Fragment: fragment.ID}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandStrokePath, Payload: uint32(len(strokes) - 1)})                             // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			}
			continue
		}
		localFonts := make(map[layoutengine.FontResourceID]layoutengine.FontResourceID)
		for _, font := range measurement.plan.Fonts {
			localID := font.ID
			identity := paperFontIdentity(font)
			globalID, exists := fontIndex[identity]
			if !exists {
				globalID = layoutengine.FontResourceID(len(fonts) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				font.ID = globalID
				fonts = append(fonts, font)
				fontIndex[identity] = globalID
			}
			localFonts[localID] = globalID
		}
		lineMap := make(map[uint32]uint32, len(measurement.plan.Lines))
		for localIndex, line := range measurement.plan.Lines {
			xOffset, err := line.Bounds.X.Sub(measurement.body.X)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			yOffset, err := line.Bounds.Y.Sub(measurement.body.Y)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			x, err := fragment.BorderBox.X.Add(xOffset)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			y, err := fragment.BorderBox.Y.Add(yOffset)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			bounds, err := layoutengine.NewRect(x, y, line.Bounds.Width, line.Bounds.Height)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			baselineOffset, err := line.Baseline.Sub(line.Bounds.Y)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			baseline, err := y.Add(baselineOffset)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			globalLine := uint32(len(projection.Lines)) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
			projection.Lines = append(projection.Lines, layoutengine.PlannedLine{
				Fragment: fragment.ID, Index: uint32(localIndex), Bounds: bounds,
				Baseline: baseline, Source: measurement.identity.source,
			})
			lineMap[uint32(localIndex)] = globalLine
		}
		for _, run := range measurement.plan.GlyphRuns {
			globalLine := lineMap[run.Line]
			line := projection.Lines[globalLine]
			run.Line, run.Font = globalLine, localFonts[run.Font]
			run.Origin = layoutengine.Point{X: line.Bounds.X, Y: line.Baseline}
			run.Source = measurement.identity.source
			runs = append(runs, run)
			items = append(items, layoutengine.DisplayItem{Kind: layoutengine.CommandGlyphRun, Payload: uint32(len(runs) - 1)}) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		}
	}
	pageCount := 1
	if paginatedTable >= 0 {
		pageCount = len(measurements[paginatedTable].table.Pages)
	}
	pageSize := projection.Pages[0].Size
	projection.Pages = make([]layoutengine.PlannedPage, pageCount)
	fragmentCursor, lineCursor := 0, 0
	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		pageNumber := uint32(pageIndex + 1)
		fragmentStart := fragmentCursor
		for fragmentCursor < len(projection.Fragments) && projection.Fragments[fragmentCursor].Page == pageNumber {
			fragmentCursor++
		}
		lineStart := lineCursor
		for lineCursor < len(projection.Lines) {
			fragmentID := projection.Lines[lineCursor].Fragment
			if !fragmentID.Valid() || int(fragmentID) > len(projection.Fragments) || projection.Fragments[int(fragmentID)-1].Page != pageNumber {
				break
			}
			lineCursor++
		}
		projection.Pages[pageIndex] = layoutengine.PlannedPage{
			Number: pageNumber, Size: pageSize,
			Fragments: layoutengine.IndexRange{Start: uint32(fragmentStart), Count: uint32(fragmentCursor - fragmentStart)},
			Lines:     layoutengine.IndexRange{Start: uint32(lineStart), Count: uint32(lineCursor - lineStart)},
		}
	}
	if fragmentCursor != len(projection.Fragments) || lineCursor != len(projection.Lines) {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: nested row/column table output is not in page order")
	}
	geometry, err := layoutengine.NewLayoutPlan(layoutengine.LayoutPlanInput{
		Pages: projection.Pages, Fragments: projection.Fragments, Lines: projection.Lines,
		PageRegions: projection.PageRegions, GridTracks: projection.GridTracks,
		Breaks: projection.Breaks, Diagnostics: projection.Diagnostics,
	})
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	painted, err := layoutengine.AttachDisplayList(geometry, layoutengine.DisplayListInput{
		Fonts: fonts, GlyphRuns: runs, ImageResources: imageResources, Images: images,
		Paths: paths, Fills: fills, Strokes: strokes, Destinations: destinations, Links: links, Items: items,
	})
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	rootKey := layoutengine.NodeKey("@row-column")
	nodes := []layoutengine.SemanticNode{{ID: 1, Role: layoutengine.SemanticRoleDocument, Key: rootKey, Instance: layoutengine.InstanceID(rootKey)}}
	associations := make([]layoutengine.SemanticFragmentAssociation, 0, len(measurements))
	reading := make([]layoutengine.ReadingOccurrence, 0, len(measurements))
	readingByPage := make(map[uint32]uint32)
	for index, measurement := range measurements {
		semantic := layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
		fragment := projection.Fragments[index]
		nodes = append(nodes, layoutengine.SemanticNode{ID: semantic, Parent: 1, Role: measurement.role,
			Key: measurement.identity.key, Instance: measurement.identity.instance, Source: measurement.identity.source,
			Attributes: layoutengine.SemanticAttributes{HeadingLevel: measurement.heading, AlternateText: measurement.alt}})
		outerFragments := nestedOuterFragments[index]
		if len(outerFragments) == 0 {
			outerFragments = []layoutengine.FragmentID{fragment.ID}
		}
		for _, outerFragment := range outerFragments {
			page := projection.Fragments[int(outerFragment)-1].Page
			associations = append(associations, layoutengine.SemanticFragmentAssociation{Semantic: semantic, Page: page, Fragment: outerFragment})
		}
		if nested, exists := nestedSemantics[index]; exists {
			semanticMap := map[layoutengine.SemanticNodeID]layoutengine.SemanticNodeID{1: semantic}
			prefix := string(measurement.identity.key) + "/"
			for _, childNode := range nested.projection.SemanticNodes {
				if childNode.ID == 1 && childNode.Role == layoutengine.SemanticRoleDocument {
					semanticMap[childNode.ID] = semantic
					continue
				}
				oldID, oldParent := childNode.ID, childNode.Parent
				childNode.ID = layoutengine.SemanticNodeID(len(nodes) + 1) // #nosec G115 -- collection length is bounded by the surrounding limit or container invariant
				childNode.Parent = semanticMap[oldParent]
				if !childNode.Parent.Valid() {
					childNode.Parent = 1
				}
				childNode.Key = layoutengine.NodeKey(prefix + string(childNode.Key))
				childNode.Instance = layoutengine.InstanceID(prefix + string(childNode.Instance))
				semanticMap[oldID] = childNode.ID
				nodes = append(nodes, childNode)
			}
			for _, association := range nested.projection.SemanticFragments {
				association.Semantic = semanticMap[association.Semantic]
				association.Fragment = nested.fragments[association.Fragment]
				association.Page = projection.Fragments[int(association.Fragment)-1].Page
				associations = append(associations, association)
			}
			if measurement.role != layoutengine.SemanticRoleArtifact {
				for _, outerFragment := range outerFragments {
					page := projection.Fragments[int(outerFragment)-1].Page
					reading = append(reading, layoutengine.ReadingOccurrence{Semantic: semantic, Page: page, Fragment: outerFragment, ReadingIndex: readingByPage[page]})
					readingByPage[page]++
				}
			}
			for _, occurrence := range nested.projection.ReadingOrder {
				occurrence.Semantic = semanticMap[occurrence.Semantic]
				occurrence.Fragment = nested.fragments[occurrence.Fragment]
				occurrence.Page = projection.Fragments[int(occurrence.Fragment)-1].Page
				occurrence.ReadingIndex = readingByPage[occurrence.Page]
				readingByPage[occurrence.Page]++
				reading = append(reading, occurrence)
			}
			continue
		}
		if measurement.role != layoutengine.SemanticRoleArtifact {
			reading = append(reading, layoutengine.ReadingOccurrence{Semantic: semantic, Page: 1, Fragment: fragment.ID, ReadingIndex: readingByPage[1]})
			readingByPage[1]++
		}
	}
	sort.SliceStable(reading, func(i, j int) bool {
		if reading[i].Page != reading[j].Page {
			return reading[i].Page < reading[j].Page
		}
		return reading[i].ReadingIndex < reading[j].ReadingIndex
	})
	return layoutengine.AttachSemantics(painted, nodes, associations, reading)
}

func translatePaperNestedPath(path layoutengine.PlannedPath, dx, dy layoutengine.Fixed) (layoutengine.PlannedPath, error) {
	var err error
	path.Bounds, err = translateTypedRect(path.Bounds, dx, dy)
	if err != nil {
		return layoutengine.PlannedPath{}, err
	}
	path.Segments = append([]layoutengine.PathSegment(nil), path.Segments...)
	for index := range path.Segments {
		segment := &path.Segments[index]
		switch segment.Kind {
		case layoutengine.PathMoveTo, layoutengine.PathLineTo:
			segment.Point, err = translateTypedPoint(segment.Point, dx, dy)
		case layoutengine.PathCubicTo:
			segment.Point, err = translateTypedPoint(segment.Point, dx, dy)
			if err == nil {
				segment.Control1, err = translateTypedPoint(segment.Control1, dx, dy)
			}
			if err == nil {
				segment.Control2, err = translateTypedPoint(segment.Control2, dx, dy)
			}
		case layoutengine.PathClose:
			continue
		default:
			return layoutengine.PlannedPath{}, fmt.Errorf("document: nested table path kind %q is unsupported", segment.Kind)
		}
		if err != nil {
			return layoutengine.PlannedPath{}, err
		}
	}
	return path, nil
}

func paperRowColumnDirection(direction layout.RowColumnDirection) (layoutengine.RowColumnDirection, error) {
	switch direction {
	case layout.RowDirection:
		return layoutengine.RowDirection, nil
	case layout.ColumnDirection:
		return layoutengine.ColumnDirection, nil
	default:
		return "", newTypedShadowUnsupported(typedShadowBlockKind, "row/column direction is invalid")
	}
}

func paperRowColumnTrack(track layout.RowColumnTrack) (layoutengine.RowColumnTrack, error) {
	size, err := layoutengine.FixedFromPoints(track.Size)
	if err != nil {
		return layoutengine.RowColumnTrack{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column track size is invalid")
	}
	minimum, err := layoutengine.FixedFromPoints(track.Min)
	if err != nil {
		return layoutengine.RowColumnTrack{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column track minimum is invalid")
	}
	maximum, err := layoutengine.FixedFromPoints(track.Max)
	if err != nil {
		return layoutengine.RowColumnTrack{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column track maximum is invalid")
	}
	basis, err := layoutengine.FixedFromPoints(track.Basis)
	if err != nil {
		return layoutengine.RowColumnTrack{}, newTypedShadowUnsupported(typedShadowGeometry, "row/column track flex basis is invalid")
	}
	result := layoutengine.RowColumnTrack{Size: size, Min: minimum, Max: maximum, Weight: track.Weight, Basis: basis,
		BasisPercent: track.BasisPercent, Grow: track.Grow, Shrink: track.Shrink, GrowFactor: track.GrowFactor, ShrinkFactor: track.ShrinkFactor,
		MinPercent: track.MinPercent, MaxPercent: track.MaxPercent}
	switch track.Kind {
	case layout.RowColumnTrackFixed:
		result.Kind = layoutengine.RowColumnTrackFixed
	case layout.RowColumnTrackAuto:
		result.Kind = layoutengine.RowColumnTrackAuto
	case layout.RowColumnTrackFraction:
		result.Kind = layoutengine.RowColumnTrackFraction
	case layout.RowColumnTrackFlex:
		result.Kind = layoutengine.RowColumnTrackFlex
		switch track.BasisKind {
		case layout.RowColumnFlexBasisFixed:
			result.BasisKind = layoutengine.RowColumnFlexBasisFixed
		case layout.RowColumnFlexBasisPercent:
			result.BasisKind = layoutengine.RowColumnFlexBasisPercent
		case layout.RowColumnFlexBasisContent:
			result.BasisKind = layoutengine.RowColumnFlexBasisContent
		default:
			return layoutengine.RowColumnTrack{}, newTypedShadowUnsupported(typedShadowBlockKind, "row/column flex basis kind is invalid")
		}
	default:
		return layoutengine.RowColumnTrack{}, newTypedShadowUnsupported(typedShadowBlockKind, "row/column track kind is invalid")
	}
	return result, nil
}
