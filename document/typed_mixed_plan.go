// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/layout"
)

type typedMixedSegment struct {
	plan        layoutengine.LayoutPlan
	startPage   uint32
	startReason layoutengine.BreakReason
}

type typedMixedDecoratedBlock struct {
	block layout.Block
	box   layout.BoxStyle
	path  string
	text  string
}

func (typedMixedDecoratedBlock) DocumentBlockKind() layout.BlockKind { return layout.BlockKindSection }

// planTypedMixedBodies plans each top-level block from the cursor left by its
// predecessor. Each child planner receives a shortened first body and the full
// selected body on continuation pages. Composition only rebases already-final
// geometry and display payloads; it never measures or paginates again.
func (f *Document) planTypedMixedBodies(ctx context.Context, doc *layout.LayoutDocument, selectBody paperBodySelector) (layoutengine.LayoutPlan, error) {
	return f.planTypedMixedBodiesMapped(ctx, doc, papercompile.CompileMapping{}, selectBody)
}

// planTypedMixedBodiesMapped keeps the source mapping owned by the frontend
// while composing heterogeneous top-level blocks. Child planners still own
// all measurement and pagination; this layer only supplies the exact mapping
// to text/row-column children before rebasing their finalized plans.
func (f *Document) planTypedMixedBodiesMapped(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, selectBody paperBodySelector) (layoutengine.LayoutPlan, error) {
	blocks, err := typedMixedExpandContainers(doc.Body, "body")
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	if len(blocks) == 1 {
		if table, ok := blocks[0].(layout.TableBlock); ok {
			child := *doc
			child.Body = []layout.Block{table}
			child.PageTemplate = layout.PageTemplate{Margins: doc.PageTemplate.Margins}
			return f.planTypedTableBodiesMapped(ctx, &child, table, "body[0]", mapping, 0, selectBody)
		}
		if _, ok := blocks[0].(layout.RowColumnBlock); ok {
			child := *doc
			child.Body = blocks
			child.PageTemplate = layout.PageTemplate{Margins: doc.PageTemplate.Margins}
			return f.planPaperTextBlocksMappedBodiesContext(ctx, &child, mapping, selectBody)
		}
	}
	if len(blocks) < 2 {
		if _, decorated := blocks[0].(typedMixedDecoratedBlock); !decorated {
			return layoutengine.LayoutPlan{}, errors.New("document: mixed typed planning requires at least two blocks")
		}
	}
	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	pageSize, baseBody, err := typedShadowFixedGeometry(f, left, top, f.w-left-right, f.h-top-bottom)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	globalBody := func(page uint32) (layoutengine.Rect, error) {
		body := baseBody
		if selectBody != nil {
			selected, selectErr := selectBody(page, baseBody)
			if selectErr != nil {
				return layoutengine.Rect{}, selectErr
			}
			body = selected
		}
		baseBottom, _ := baseBody.Bottom()
		baseRight, _ := baseBody.Right()
		bodyBottom, bottomErr := body.Bottom()
		bodyRight, rightErr := body.Right()
		if bottomErr != nil || rightErr != nil || body.X < baseBody.X || bodyRight > baseRight || body.Width <= 0 || body.Height <= 0 || body.Y < baseBody.Y || bodyBottom > baseBottom {
			return layoutengine.Rect{}, fmt.Errorf("document: page %d selected body region is invalid or outside the page margins", page)
		}
		return body, nil
	}
	page := uint32(1)
	body, err := globalBody(page)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	cursor := body.Y
	segments := make([]typedMixedSegment, 0, len(blocks))
	var pendingReason layoutengine.BreakReason
	for index, block := range blocks {
		if err := layoutengine.ChargePlanningWork(ctx, "mixed typed block planning", 1); err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		if pageBreak, ok := block.(layout.PageBreakBlock); ok && (pageBreak.Before || pageBreak.After) {
			if len(segments) != 0 && index+1 < len(blocks) {
				page++
				if f.limits.MaxPages > 0 && int(page) > f.limits.MaxPages {
					return layoutengine.LayoutPlan{}, fmt.Errorf("%w: mixed typed flow requires page %d", layoutengine.ErrTablePageLimit, page)
				}
				body, err = globalBody(page)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				cursor, pendingReason = body.Y, layoutengine.BreakExplicitPageBreak
			}
			continue
		}
		startPage := page
		startCursor := cursor
		selector := func(localPage uint32, base layoutengine.Rect) (layoutengine.Rect, error) {
			selected, selectErr := globalBody(startPage + localPage - 1)
			if selectErr != nil {
				return layoutengine.Rect{}, selectErr
			}
			if localPage != 1 {
				return selected, nil
			}
			bottom, bottomErr := selected.Bottom()
			if bottomErr != nil || startCursor < selected.Y || startCursor >= bottom {
				return layoutengine.Rect{}, fmt.Errorf("document: mixed block %d has no remaining first-page body region", index)
			}
			height, heightErr := bottom.Sub(startCursor)
			if heightErr != nil {
				return layoutengine.Rect{}, heightErr
			}
			return layoutengine.NewRect(selected.X, startCursor, selected.Width, height)
		}
		child := *doc
		child.Body = []layout.Block{block}
		child.PageTemplate = layout.PageTemplate{Margins: doc.PageTemplate.Margins}
		// A mixed compositor gives each child a one-block document. Rebase the
		// frontend's body index for that child so source identity remains tied to
		// the authored node instead of silently falling back to a synthetic ID.
		childMapping := paperMappingForMixedBody(mapping, index)
		var planned layoutengine.LayoutPlan
		if decorated, ok := block.(typedMixedDecoratedBlock); ok {
			planned, err = f.planTypedMixedDecoratedBlock(ctx, &child, decorated, selector)
		} else if table, ok := block.(layout.TableBlock); ok {
			planned, err = f.planTypedTableBodiesMapped(ctx, &child, table, fmt.Sprintf("body[%d]", index), childMapping, 0, selector)
		} else {
			planned, err = f.planPaperTextBlocksMappedBodiesContext(ctx, &child, childMapping, selector)
		}
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		projection := planned.Projection()
		if len(projection.Pages) == 0 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: mixed block %d produced no pages", index)
		}
		endPage := startPage + uint32(len(projection.Pages)) - 1
		if f.limits.MaxPages > 0 && int(endPage) > f.limits.MaxPages {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%w: mixed typed flow requires page %d", layoutengine.ErrTablePageLimit, endPage)
		}
		lastLocal := uint32(len(projection.Pages))
		endCursor := layoutengine.Fixed(0)
		for _, fragment := range projection.Fragments {
			if fragment.Page != lastLocal || fragment.Region != layoutengine.RegionBody {
				continue
			}
			fragmentBottom, bottomErr := fragment.BorderBox.Bottom()
			if bottomErr != nil {
				return layoutengine.LayoutPlan{}, bottomErr
			}
			if fragmentBottom > endCursor {
				endCursor = fragmentBottom
			}
		}
		if endCursor == 0 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: mixed block %d produced no body fragments", index)
		}
		// A table-level keep-with-next can be decided exactly after the table's
		// finalized height is known. For the common text successor, measure that
		// successor through the same core planner; if both fit a fresh body but
		// not the shortened current body, replan the table once on the next page.
		if table, ok := block.(layout.TableBlock); ok && table.Box.KeepWithNext && index+1 < len(blocks) && endPage == startPage && startCursor > body.Y {
			if nextParagraph, ok := paperParagraphBlock(blocks[index+1]); ok {
				fullBody, bodyErr := globalBody(startPage)
				if bodyErr != nil {
					return layoutengine.LayoutPlan{}, bodyErr
				}
				nextMeasurement, measureErr := f.measurePaperRowColumnChild(ctx, doc, nextParagraph, f.PointConvert(fullBody.X.Points()), f.PointConvert(fullBody.Y.Points()), f.w-f.PointConvert(fullBody.X.Points()+fullBody.Width.Points()), f.h-f.PointConvert(fullBody.Y.Points()+fullBody.Height.Points()), fullBody.Width)
				if measureErr == nil {
					bottom, _ := fullBody.Bottom()
					remaining, _ := bottom.Sub(endCursor)
					if nextMeasurement.height > remaining {
						startPage++
						if f.limits.MaxPages > 0 && int(startPage) > f.limits.MaxPages {
							return layoutengine.LayoutPlan{}, fmt.Errorf("%w: mixed typed keep-with-next requires page %d", layoutengine.ErrTablePageLimit, startPage)
						}
						body, err = globalBody(startPage)
						if err != nil {
							return layoutengine.LayoutPlan{}, err
						}
						startCursor, pendingReason = body.Y, layoutengine.BreakPaginationConstraint
						child.Body = []layout.Block{table}
						planned, err = f.planTypedTableBodies(ctx, &child, table, fmt.Sprintf("body[%d]", index), selector)
						if err != nil {
							return layoutengine.LayoutPlan{}, err
						}
						projection = planned.Projection()
						endPage = startPage + uint32(len(projection.Pages)) - 1
						lastLocal, endCursor = uint32(len(projection.Pages)), 0
						for _, fragment := range projection.Fragments {
							if fragment.Page != lastLocal || fragment.Region != layoutengine.RegionBody {
								continue
							}
							fragmentBottom, bottomErr := fragment.BorderBox.Bottom()
							if bottomErr != nil {
								return layoutengine.LayoutPlan{}, bottomErr
							}
							if fragmentBottom > endCursor {
								endCursor = fragmentBottom
							}
						}
					}
				}
			}
		}
		segments = append(segments, typedMixedSegment{plan: planned, startPage: startPage, startReason: pendingReason})
		pendingReason = ""
		page, cursor = endPage, endCursor
		if index+1 < len(blocks) {
			body, err = globalBody(page)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			bodyBottom, _ := body.Bottom()
			if cursor >= bodyBottom {
				page++
				if f.limits.MaxPages > 0 && int(page) > f.limits.MaxPages {
					return layoutengine.LayoutPlan{}, fmt.Errorf("%w: mixed typed flow requires page %d", layoutengine.ErrTablePageLimit, page)
				}
				body, err = globalBody(page)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				cursor = body.Y
				pendingReason = layoutengine.BreakPaginationConstraint
			}
		}
	}
	return composeTypedMixedSegments(segments, pageSize, page)
}

// paperMappingForMixedBody narrows a full compile mapping to one top-level
// child and rebases its body index to the child document's local index zero.
// Root/theme mappings are retained because they describe the same compiled
// document rather than a particular body slot.
func paperMappingForMixedBody(mapping papercompile.CompileMapping, bodyIndex int) papercompile.CompileMapping {
	filtered := papercompile.CompileMapping{ThemeProperties: append([]papercompile.ThemePropertyMapping(nil), mapping.ThemeProperties...)}
	for _, node := range mapping.Nodes {
		if node.BodyIndex >= 0 && node.BodyIndex != bodyIndex {
			continue
		}
		if node.BodyIndex == bodyIndex {
			node.BodyIndex = 0
		}
		filtered.Nodes = append(filtered.Nodes, node)
	}
	return filtered
}

func typedBlocksContainTable(blocks []layout.Block) bool {
	for _, candidate := range layout.NormalizeBlocks(blocks) {
		switch block := candidate.(type) {
		case layout.TableBlock:
			return true
		case layout.SectionBlock:
			if typedBlocksContainTable(block.Blocks) {
				return true
			}
		case layout.ClauseBlock:
			if typedBlocksContainTable(block.Blocks) {
				return true
			}
		case layout.NoteBoxBlock:
			if typedBlocksContainTable(block.Body) {
				return true
			}
		case layout.ListBlock:
			for _, item := range block.Items {
				if typedBlocksContainTable(item.Blocks) {
					return true
				}
			}
		case layout.RowColumnBlock:
			for _, item := range block.Items {
				if item.Block != nil && typedBlocksContainTable([]layout.Block{item.Block}) {
					return true
				}
			}
		}
	}
	return false
}

func typedBlocksContainRowColumn(blocks []layout.Block) bool {
	for _, candidate := range layout.NormalizeBlocks(blocks) {
		switch block := candidate.(type) {
		case layout.RowColumnBlock:
			return true
		case layout.SectionBlock:
			if typedBlocksContainRowColumn(block.Blocks) {
				return true
			}
		case layout.ClauseBlock:
			if typedBlocksContainRowColumn(block.Blocks) {
				return true
			}
		case layout.NoteBoxBlock:
			if typedBlocksContainRowColumn(block.Body) {
				return true
			}
		case layout.ListBlock:
			for _, item := range block.Items {
				if typedBlocksContainRowColumn(item.Blocks) {
					return true
				}
			}
		}
	}
	return false
}

func typedBlocksNeedMixedBoxContainers(blocks []layout.Block) bool {
	for _, candidate := range layout.NormalizeBlocks(blocks) {
		switch block := candidate.(type) {
		case layout.SectionBlock:
			if htmlUnifiedVisualBox(block.EffectiveBox()) || typedBlocksNeedMixedBoxContainers(block.Blocks) {
				return true
			}
		case layout.ClauseBlock:
			if htmlUnifiedVisualBox(block.EffectiveBox()) || typedBlocksNeedMixedBoxContainers(block.Blocks) {
				return true
			}
		case layout.NoteBoxBlock:
			// Preserve the established top-level NoteBox rejection contract.
			// Decorated SectionBlock is the bounded mixed-box frontier; NoteBox
			// visual styling still belongs to its existing explicit unsupported path.
			if typedBlocksNeedMixedBoxContainers(block.Body) {
				return true
			}
		}
	}
	return false
}

// typedMixedExpandContainers retains the established exact container title
// lowering and source order while exposing nested tables to the sequential
// compositor. Row/column children retain their own exact bounded compositor.
func typedMixedExpandContainers(blocks []layout.Block, path string) ([]layout.Block, error) {
	expanded := make([]layout.Block, 0, len(blocks))
	for index, candidate := range layout.NormalizeBlocks(blocks) {
		blockPath := fmt.Sprintf("%s[%d]", path, index)
		switch block := candidate.(type) {
		case layout.SectionBlock:
			if htmlUnifiedVisualBox(block.EffectiveBox()) {
				policy, visual := paperContainerBoxPolicy(block.EffectiveBox())
				block.Box, block.BoxRef = layout.BoxStyle{KeepTogether: policy.keepTogether, KeepWithNext: policy.keepWithNext,
					Orphans: policy.orphans, Widows: policy.widows}, nil
				expanded = append(expanded, typedMixedDecoratedBlock{block: block, box: visual, path: blockPath, text: strings.TrimSpace(block.Title)})
				continue
			}
			policy, policyErr := paperBoxPaginationPolicy(block.EffectiveBox(), block.BoxRef, blockPath)
			if policyErr != nil {
				return nil, policyErr
			}
			if block.Title != "" {
				expanded = append(expanded, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: block.Title}},
					Box: layout.BoxStyle{KeepWithNext: block.KeepTitleWithBody || policy.keepTogether}})
			}
			children, childErr := typedMixedExpandContainers(block.Blocks, blockPath+".blocks")
			if childErr != nil {
				return nil, childErr
			}
			expanded = append(expanded, children...)
		case layout.ClauseBlock:
			if htmlUnifiedVisualBox(block.EffectiveBox()) {
				policy, visual := paperContainerBoxPolicy(block.EffectiveBox())
				block.Box, block.BoxRef = layout.BoxStyle{KeepTogether: policy.keepTogether, KeepWithNext: policy.keepWithNext,
					Orphans: policy.orphans, Widows: policy.widows}, nil
				expanded = append(expanded, typedMixedDecoratedBlock{block: block, box: visual, path: blockPath, text: strings.TrimSpace(block.Number + " " + block.Title)})
				continue
			}
			policy, policyErr := paperBoxPaginationPolicy(block.EffectiveBox(), block.BoxRef, blockPath)
			if policyErr != nil {
				return nil, policyErr
			}
			if block.BreakBefore {
				expanded = append(expanded, layout.PageBreakBlock{Before: true})
			}
			if title := strings.TrimSpace(block.Number + " " + block.Title); title != "" {
				expanded = append(expanded, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: title}},
					Box: layout.BoxStyle{KeepWithNext: block.KeepTogether || policy.keepTogether}})
			}
			children, childErr := typedMixedExpandContainers(block.Blocks, blockPath+".blocks")
			if childErr != nil {
				return nil, childErr
			}
			expanded = append(expanded, children...)
			if block.BreakAfter {
				expanded = append(expanded, layout.PageBreakBlock{After: true})
			}
		case layout.NoteBoxBlock:
			if htmlUnifiedVisualBox(block.EffectiveBox()) {
				if strings.TrimSpace(block.Title) == "" && len(layout.NormalizeBlocks(block.Body)) == 0 {
					return nil, fmt.Errorf("%s: visual box styling is unsupported for an empty container", blockPath)
				}
				policy, visual := paperContainerBoxPolicy(block.EffectiveBox())
				block.Box, block.BoxRef = layout.BoxStyle{KeepTogether: policy.keepTogether, KeepWithNext: policy.keepWithNext,
					Orphans: policy.orphans, Widows: policy.widows}, nil
				expanded = append(expanded, typedMixedDecoratedBlock{block: block, box: visual, path: blockPath, text: strings.TrimSpace(block.Title)})
				continue
			}
			policy, policyErr := paperBoxPaginationPolicy(block.EffectiveBox(), block.BoxRef, blockPath)
			if policyErr != nil {
				return nil, policyErr
			}
			if block.StyleRef != nil {
				return nil, fmt.Errorf("%s: note title style references are unsupported", blockPath)
			}
			if block.Title != "" {
				expanded = append(expanded, layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: block.Title}}, Style: block.Style,
					Box: layout.BoxStyle{KeepWithNext: policy.keepTogether}})
			}
			children, childErr := typedMixedExpandContainers(block.Body, blockPath+".body")
			if childErr != nil {
				return nil, childErr
			}
			expanded = append(expanded, children...)
		case layout.RowColumnBlock:
			expanded = append(expanded, block)
		default:
			expanded = append(expanded, block)
		}
	}
	return expanded, nil
}

func (f *Document) planTypedMixedDecoratedBlock(ctx context.Context, doc *layout.LayoutDocument, wrapper typedMixedDecoratedBlock, outer paperBodySelector) (layoutengine.LayoutPlan, error) {
	measured, err := f.paperMeasureBox(wrapper.box, wrapper.path)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	child := *doc
	child.Body = []layout.Block{wrapper.block}
	leftInset := measured.style.Margin.Left + measured.style.Border.Left + measured.style.Padding.Left
	rightInset := measured.style.Margin.Right + measured.style.Border.Right + measured.style.Padding.Right
	baseLeft, baseTop, baseRight, baseBottom := typedShadowMargins(f, child.PageTemplate.Margins)
	child.PageTemplate.Margins = layout.Spacing{Left: baseLeft + f.PointConvert(leftInset.Points()), Top: baseTop,
		Right: baseRight + f.PointConvert(rightInset.Points()), Bottom: baseBottom}
	planWithLastPage := func(last uint32) (layoutengine.LayoutPlan, error) {
		selector := func(page uint32, base layoutengine.Rect) (layoutengine.Rect, error) {
			body := base
			if outer != nil {
				var selectErr error
				body, selectErr = outer(page, base)
				if selectErr != nil {
					return layoutengine.Rect{}, selectErr
				}
			}
			left := leftInset
			top, bottom := layoutengine.Fixed(0), layoutengine.Fixed(0)
			if page == 1 {
				top = measured.style.Margin.Top + measured.style.Border.Top + measured.style.Padding.Top
			}
			if last != 0 && page == last {
				bottom = measured.style.Padding.Bottom + measured.style.Border.Bottom + measured.style.Margin.Bottom
			}
			x, addErr := body.X.Add(left)
			y, yErr := body.Y.Add(top)
			width, widthErr := measured.contentWidth(body.Width)
			height, heightErr := body.Height.Sub(top)
			if heightErr == nil {
				height, heightErr = height.Sub(bottom)
			}
			if addErr != nil || yErr != nil || widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
				return layoutengine.Rect{}, fmt.Errorf("%s: box edges leave no continuation content region", wrapper.path)
			}
			return layoutengine.NewRect(x, y, width, height)
		}
		if typedBlocksContainTable(child.Body) {
			return f.planTypedMixedBodies(ctx, &child, selector)
		}
		return f.planPaperTextBlocksMappedBodiesContext(ctx, &child, papercompile.CompileMapping{}, selector)
	}
	planned, err := planWithLastPage(0)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	last := uint32(len(planned.Projection().Pages))
	for attempt := 0; attempt < 3; attempt++ {
		planned, err = planWithLastPage(last)
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		next := uint32(len(planned.Projection().Pages))
		if next == last {
			break
		}
		last = next
		if attempt == 2 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%s: decorated continuation page count did not converge", wrapper.path)
		}
	}
	projection := planned.Projection()
	// A fixed/min/max height is a border-box constraint. The child planner
	// still receives the available flow region (so visible overflow and
	// pagination remain deterministic), while the immutable wrapper decoration
	// clamps the final border rectangle below. A constrained wrapper that
	// fragments over multiple pages cannot be represented as one CSS box by
	// this compositor yet; reject that combination instead of inventing
	// renderer-specific continuation geometry.
	heightConstrained := measured.height > 0 || measured.minHeight > 0 || measured.maxHeight > 0
	if heightConstrained && len(projection.Pages) > 1 {
		return layoutengine.LayoutPlan{}, fmt.Errorf("%s: explicit or bounded height cannot fragment across pages in the unified structural-box cohort", wrapper.path)
	}
	decorations := make([]layoutengine.BoxDecoration, 0, last)
	overflowClips := make(map[layoutengine.FragmentID]layoutengine.Rect)
	for page := uint32(1); page <= last; page++ {
		outerBody, bodyErr := outer(page, layoutengine.Rect{})
		if bodyErr != nil {
			return layoutengine.LayoutPlan{}, bodyErr
		}
		var owner layoutengine.FragmentID
		pageFragments := make([]layoutengine.FragmentID, 0)
		var contentBottom layoutengine.Fixed
		for _, fragment := range projection.Fragments {
			if fragment.Page != page || fragment.Region != layoutengine.RegionBody {
				continue
			}
			if !owner.Valid() {
				owner = fragment.ID
			}
			pageFragments = append(pageFragments, fragment.ID)
			bottom, _ := fragment.BorderBox.Bottom()
			if bottom > contentBottom {
				contentBottom = bottom
			}
		}
		if !owner.Valid() {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%s: continuation page %d has no body fragment", wrapper.path, page)
		}
		x, _ := outerBody.X.Add(measured.style.Margin.Left)
		width, widthErr := measured.borderWidth(outerBody.Width)
		y := outerBody.Y
		if page == 1 {
			y, _ = y.Add(measured.style.Margin.Top)
		}
		bottom, _ := outerBody.Bottom()
		if page == last {
			bottom, _ = contentBottom.Add(measured.style.Padding.Bottom)
			bottom, _ = bottom.Add(measured.style.Border.Bottom)
		}
		height, heightErr := bottom.Sub(y)
		if heightErr == nil {
			if measured.height > 0 {
				height = measured.height
			}
			if measured.minHeight > 0 && height < measured.minHeight {
				height = measured.minHeight
			}
			if measured.maxHeight > 0 && height > measured.maxHeight {
				height = measured.maxHeight
			}
		}
		if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("%s: continuation decoration is invalid", wrapper.path)
		}
		box, rectErr := layoutengine.NewRect(x, y, width, height)
		if rectErr != nil {
			return layoutengine.LayoutPlan{}, rectErr
		}
		decoration := layoutengine.BoxDecoration{Fragment: owner, BorderBox: &box, Background: measured.background,
			Right: layoutengine.BoxBorderSide{Width: measured.borders[1].width, Color: measured.borders[1].color},
			Left:  layoutengine.BoxBorderSide{Width: measured.borders[3].width, Color: measured.borders[3].color}}
		if page == 1 {
			decoration.Top = layoutengine.BoxBorderSide{Width: measured.borders[0].width, Color: measured.borders[0].color}
		}
		if page == last {
			decoration.Bottom = layoutengine.BoxBorderSide{Width: measured.borders[2].width, Color: measured.borders[2].color}
		}
		decorations = append(decorations, decoration)
		if measured.overflow == "hidden" {
			clipX, clipErr := box.X.Add(measured.style.Border.Left)
			clipY, clipYErr := box.Y.Add(measured.style.Border.Top)
			clipWidth, clipWidthErr := box.Width.Sub(measured.style.Border.Left)
			if clipWidthErr == nil {
				clipWidth, clipWidthErr = clipWidth.Sub(measured.style.Border.Right)
			}
			clipHeight, clipHeightErr := box.Height.Sub(measured.style.Border.Top)
			if clipHeightErr == nil {
				clipHeight, clipHeightErr = clipHeight.Sub(measured.style.Border.Bottom)
			}
			if clipErr != nil || clipYErr != nil || clipWidthErr != nil || clipHeightErr != nil || clipWidth <= 0 || clipHeight <= 0 {
				return layoutengine.LayoutPlan{}, fmt.Errorf("%s: overflow clip is invalid", wrapper.path)
			}
			clip, clipRectErr := layoutengine.NewRect(clipX, clipY, clipWidth, clipHeight)
			if clipRectErr != nil {
				return layoutengine.LayoutPlan{}, clipRectErr
			}
			for _, fragment := range pageFragments {
				overflowClips[fragment] = clip
			}
		}
	}
	if len(overflowClips) != 0 {
		planned, err = layoutengine.AttachFragmentClips(planned, overflowClips)
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
	}
	return layoutengine.AttachBoxDecorations(planned, decorations)
}

func composeTypedMixedSegments(segments []typedMixedSegment, pageSize layoutengine.Size, pages uint32) (layoutengine.LayoutPlan, error) {
	input := layoutengine.LayoutPlanInput{SemanticNodes: []layoutengine.SemanticNode{{ID: 1, Role: layoutengine.SemanticRoleDocument, Key: "@typed-document", Instance: "@typed-document"}}}
	items := make([]layoutengine.DisplayItem, 0)
	fonts := make([]layoutengine.CoreFontResource, 0)
	fontIDs := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	images := make([]layoutengine.ImageResource, 0)
	imageIDs := make(map[layoutengine.ImageContentDigest]layoutengine.ImageResourceID)
	pageFragmentCounts := make([]uint32, pages)
	pageLineCounts := make([]uint32, pages)
	pageReadingCounts := make([]uint32, pages)
	var nextNode layoutengine.NodeID
	var previousLast layoutengine.FragmentID
	var previousEndPage uint32
	for segmentIndex, segment := range segments {
		projection := segment.plan.Projection()
		prefix := fmt.Sprintf("@mixed-%d-", segmentIndex+1)
		fragmentMap := make(map[layoutengine.FragmentID]layoutengine.FragmentID, len(projection.Fragments))
		nodeMap := make(map[layoutengine.NodeID]layoutengine.NodeID)
		semanticMap := map[layoutengine.SemanticNodeID]layoutengine.SemanticNodeID{1: 1}
		semanticOwned := make(map[layoutengine.FragmentID]bool, len(projection.Fragments))
		readingOwned := make(map[layoutengine.FragmentID]bool, len(projection.Fragments))
		lineMap := make(map[uint32]uint32, len(projection.Lines))
		fontMap := make(map[layoutengine.FontResourceID]layoutengine.FontResourceID, len(projection.Fonts))
		for _, font := range projection.Fonts {
			oldID := font.ID
			key := paperFontIdentity(font)
			id := fontIDs[key]
			if !id.Valid() {
				id = layoutengine.FontResourceID(len(fonts) + 1)
				font.ID = id
				fonts = append(fonts, font)
				fontIDs[key] = id
			}
			fontMap[oldID] = id
		}
		for _, semantic := range projection.SemanticNodes {
			if semantic.ID == 1 && semantic.Role == layoutengine.SemanticRoleDocument {
				semanticMap[semantic.ID] = 1
				continue
			}
			oldID, oldParent := semantic.ID, semantic.Parent
			semantic.ID = layoutengine.SemanticNodeID(len(input.SemanticNodes) + 1)
			semantic.Parent = semanticMap[oldParent]
			if !semantic.Parent.Valid() {
				semantic.Parent = 1
			}
			semantic.Key = layoutengine.NodeKey(prefix + string(semantic.Key))
			semantic.Instance = layoutengine.InstanceID(prefix + string(semantic.Instance))
			semanticMap[oldID] = semantic.ID
			input.SemanticNodes = append(input.SemanticNodes, semantic)
		}
		for _, fragment := range projection.Fragments {
			oldID, oldNode := fragment.ID, fragment.Node
			if !nodeMap[oldNode].Valid() {
				nextNode++
				nodeMap[oldNode] = nextNode
			}
			fragment.ID = layoutengine.FragmentID(len(input.Fragments) + 1)
			fragment.Node = nodeMap[oldNode]
			fragment.Key = layoutengine.NodeKey(prefix + string(fragment.Key))
			fragment.Instance = layoutengine.InstanceID(prefix + string(fragment.Instance))
			fragment.Page += segment.startPage - 1
			fragmentMap[oldID] = fragment.ID
			input.Fragments = append(input.Fragments, fragment)
			pageFragmentCounts[fragment.Page-1]++
		}
		if len(projection.Fragments) != 0 && previousLast.Valid() && segment.startPage > previousEndPage {
			first := fragmentMap[projection.Fragments[0].ID]
			reason := segment.startReason
			if reason == "" {
				reason = layoutengine.BreakPaginationConstraint
			}
			required := projection.Fragments[0].BorderBox.Height
			if reason == layoutengine.BreakExplicitPageBreak {
				required = 0
			}
			input.Breaks = append(input.Breaks, layoutengine.BreakDecision{
				Reason: reason, FromPage: previousEndPage, ToPage: segment.startPage,
				Region: layoutengine.RegionBody, Preceding: previousLast, Triggering: first,
				Required: required, Available: 0,
			})
		}
		if len(projection.Fragments) != 0 {
			previousLast = fragmentMap[projection.Fragments[len(projection.Fragments)-1].ID]
			previousEndPage = segment.startPage + uint32(len(projection.Pages)) - 1
		}
		for oldIndex, line := range projection.Lines {
			line.Fragment = fragmentMap[line.Fragment]
			lineMap[uint32(oldIndex)] = uint32(len(input.Lines))
			input.Lines = append(input.Lines, line)
			pageLineCounts[input.Fragments[line.Fragment-1].Page-1]++
		}
		for _, association := range projection.SemanticFragments {
			association.Semantic = semanticMap[association.Semantic]
			association.Fragment = fragmentMap[association.Fragment]
			association.Page += segment.startPage - 1
			input.SemanticFragments = append(input.SemanticFragments, association)
			semanticOwned[association.Fragment] = true
		}
		for _, occurrence := range projection.ReadingOrder {
			occurrence.Semantic = semanticMap[occurrence.Semantic]
			occurrence.Fragment = fragmentMap[occurrence.Fragment]
			occurrence.Page += segment.startPage - 1
			occurrence.ReadingIndex = pageReadingCounts[occurrence.Page-1]
			pageReadingCounts[occurrence.Page-1]++
			input.ReadingOrder = append(input.ReadingOrder, occurrence)
			readingOwned[occurrence.Fragment] = true
		}
		for _, sourceFragment := range projection.Fragments {
			fragment := fragmentMap[sourceFragment.ID]
			if semanticOwned[fragment] {
				continue
			}
			owner := layoutengine.SemanticNodeID(len(input.SemanticNodes) + 1)
			composed := input.Fragments[fragment-1]
			input.SemanticNodes = append(input.SemanticNodes, layoutengine.SemanticNode{ID: owner, Parent: 1,
				Role: layoutengine.SemanticRoleParagraph, Key: composed.Key, Instance: composed.Instance, Source: composed.Source})
			input.SemanticFragments = append(input.SemanticFragments, layoutengine.SemanticFragmentAssociation{Semantic: owner, Page: composed.Page, Fragment: fragment})
			semanticOwned[fragment] = true
			if composed.Region == layoutengine.RegionBody && !readingOwned[fragment] {
				input.ReadingOrder = append(input.ReadingOrder, layoutengine.ReadingOccurrence{Semantic: owner, Page: composed.Page,
					Fragment: fragment, ReadingIndex: pageReadingCounts[composed.Page-1]})
				pageReadingCounts[composed.Page-1]++
			}
		}
		for _, decision := range projection.Breaks {
			decision.FromPage += segment.startPage - 1
			decision.ToPage += segment.startPage - 1
			decision.Preceding = fragmentMap[decision.Preceding]
			decision.Triggering = fragmentMap[decision.Triggering]
			input.Breaks = append(input.Breaks, decision)
		}
		for _, diagnostic := range projection.Diagnostics {
			if diagnostic.Location.Page != 0 {
				diagnostic.Location.Page += segment.startPage - 1
			}
			if diagnostic.Location.Fragment.Valid() {
				diagnostic.Location.Fragment = fragmentMap[diagnostic.Location.Fragment]
				fragment := input.Fragments[diagnostic.Location.Fragment-1]
				diagnostic.Location.Node = fragment.Node
				diagnostic.Location.Key = fragment.Key
				diagnostic.Location.Instance = fragment.Instance
				diagnostic.Location.Page = fragment.Page
				diagnostic.Location.Region = fragment.Region
			}
			input.Diagnostics = append(input.Diagnostics, diagnostic)
		}
		pathBase := uint32(len(input.Paths))
		transformBase := uint32(len(input.Transforms))
		clipBase := uint32(len(input.Clips))
		input.Paths = append(input.Paths, projection.Paths...)
		commandPages := make([]uint32, len(projection.Commands))
		for _, page := range projection.Pages {
			end := page.Commands.Start + page.Commands.Count
			for commandIndex := page.Commands.Start; commandIndex < end && int(commandIndex) < len(commandPages); commandIndex++ {
				commandPages[commandIndex] = segment.startPage + page.Number - 1
			}
		}
		input.Transforms = append(input.Transforms, projection.Transforms...)
		for _, clip := range projection.Clips {
			clip.Path += pathBase
			clip.Fragment = fragmentMap[clip.Fragment]
			input.Clips = append(input.Clips, clip)
		}
		for commandIndex, command := range projection.Commands {
			page := commandPages[commandIndex]
			switch command.Kind {
			case layoutengine.CommandSaveState, layoutengine.CommandRestoreState:
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Page: page})
			case layoutengine.CommandTransform:
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: transformBase + command.Payload, Page: page})
			case layoutengine.CommandClip:
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: clipBase + command.Payload, Page: page})
			case layoutengine.CommandGlyphRun:
				run := projection.GlyphRuns[command.Payload]
				run.Line, run.Font = lineMap[run.Line], fontMap[run.Font]
				run.Advances = append([]layoutengine.Fixed(nil), run.Advances...)
				input.GlyphRuns = append(input.GlyphRuns, run)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.GlyphRuns) - 1)})
			case layoutengine.CommandImage:
				image := projection.Images[command.Payload]
				resource := projection.ImageResources[image.Resource-1]
				id := imageIDs[resource.Digest]
				if !id.Valid() {
					id = layoutengine.ImageResourceID(len(images) + 1)
					resource.ID = id
					images = append(images, resource)
					imageIDs[resource.Digest] = id
				}
				image.Resource, image.Fragment = id, fragmentMap[image.Fragment]
				input.Images = append(input.Images, image)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.Images) - 1)})
			case layoutengine.CommandLink:
				link := projection.Links[command.Payload]
				if link.Destination.Valid() {
					return layoutengine.LayoutPlan{}, errors.New("document: internal destinations are not supported in initial mixed typed flow")
				}
				link.Fragment = fragmentMap[link.Fragment]
				input.Links = append(input.Links, link)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.Links) - 1)})
			case layoutengine.CommandFillPath:
				fill := projection.Fills[command.Payload]
				fill.Path += pathBase
				fill.Fragment = fragmentMap[fill.Fragment]
				input.Fills = append(input.Fills, fill)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.Fills) - 1)})
			case layoutengine.CommandStrokePath:
				stroke := projection.Strokes[command.Payload]
				stroke.Path += pathBase
				stroke.Fragment = fragmentMap[stroke.Fragment]
				input.Strokes = append(input.Strokes, stroke)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.Strokes) - 1)})
			default:
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: unsupported mixed typed command %q", command.Kind)
			}
		}
	}
	var fragmentStart, lineStart uint32
	for page := uint32(1); page <= pages; page++ {
		input.Pages = append(input.Pages, layoutengine.PlannedPage{Number: page, Size: pageSize,
			Fragments: layoutengine.IndexRange{Start: fragmentStart, Count: pageFragmentCounts[page-1]},
			Lines:     layoutengine.IndexRange{Start: lineStart, Count: pageLineCounts[page-1]}})
		fragmentStart += pageFragmentCounts[page-1]
		lineStart += pageLineCounts[page-1]
	}
	geometryInput := input
	geometryInput.Fonts, geometryInput.GlyphRuns = nil, nil
	geometryInput.ImageResources, geometryInput.Images = nil, nil
	geometryInput.Destinations, geometryInput.Links = nil, nil
	geometryInput.Paths, geometryInput.Transforms, geometryInput.Clips, geometryInput.Fills, geometryInput.Strokes = nil, nil, nil, nil, nil
	geometry, err := layoutengine.NewLayoutPlan(geometryInput)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: compose mixed typed geometry: %w", err)
	}
	return layoutengine.AttachDisplayList(geometry, layoutengine.DisplayListInput{Fonts: fonts, GlyphRuns: input.GlyphRuns,
		ImageResources: images, Images: input.Images, Links: input.Links, Paths: input.Paths,
		Transforms: input.Transforms, Clips: input.Clips, Fills: input.Fills, Strokes: input.Strokes, Items: items})
}
