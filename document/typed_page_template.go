// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/layout"
)

const typedPageCounterCorrectionLimit = 8

var errTypedPageCounterUnstable = errors.New("document: typed page counter correction did not converge")

type typedPageShell struct {
	header       layoutengine.LayoutPlan
	headerHeight layoutengine.Fixed
	headerOrigin layoutengine.Fixed
	footer       layoutengine.LayoutPlan
	footerHeight layoutengine.Fixed
	footerOrigin layoutengine.Fixed
}

type typedPageShellKey struct {
	page  uint32
	total uint32
}

func paperMappingForRegion(mapping papercompile.CompileMapping, region layoutengine.RegionID) papercompile.CompileMapping {
	filtered := papercompile.CompileMapping{ThemeProperties: append([]papercompile.ThemePropertyMapping(nil), mapping.ThemeProperties...)}
	for _, node := range mapping.Nodes {
		if region == layoutengine.RegionBody {
			if node.Region != "" && node.Region != layoutengine.RegionBody {
				continue
			}
		} else if node.Region != region {
			continue
		}
		node.Region = ""
		filtered.Nodes = append(filtered.Nodes, node)
	}
	return filtered
}

func (f *Document) planTypedPageTemplate(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping) (layoutengine.LayoutPlan, error) {
	if err := validateTypedPageTemplateContract(doc.PageTemplate); err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	blocks := layout.NormalizeBlocks(doc.Body)
	var soleTable *layout.TableBlock
	hasMixedTable := typedBlocksContainTable(blocks)
	hasMixedRowColumn := len(blocks) > 1 && typedBlocksContainRowColumn(blocks)
	if len(blocks) == 1 {
		if table, ok := blocks[0].(layout.TableBlock); ok {
			copy := table
			soleTable = &copy
		}
	}

	cache := make(map[typedPageShellKey]typedPageShell)
	load := func(page, total uint32) (typedPageShell, error) {
		key := typedPageShellKey{page: page, total: total}
		if shell, ok := cache[key]; ok {
			return shell, nil
		}
		if err := layoutengine.ChargePlanningWork(ctx, "typed page shell planning", 1); err != nil {
			return typedPageShell{}, err
		}
		shell, err := f.planTypedPageShell(ctx, doc, mapping, page, total)
		if err != nil {
			return typedPageShell{}, err
		}
		cache[key] = shell
		return shell, nil
	}

	totalGuess := uint32(1)
	var bodyPlan layoutengine.LayoutPlan
	for correction := 0; correction < typedPageCounterCorrectionLimit; correction++ {
		if err := layoutengine.ChargePlanningWork(ctx, "typed page counter correction", 1); err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		selector := func(page uint32, base layoutengine.Rect) (layoutengine.Rect, error) {
			shell, err := load(page, totalGuess)
			if err != nil {
				return layoutengine.Rect{}, err
			}
			y, err := base.Y.Add(shell.headerHeight)
			if err != nil {
				return layoutengine.Rect{}, err
			}
			height, err := base.Height.Sub(shell.headerHeight)
			if err == nil {
				height, err = height.Sub(shell.footerHeight)
			}
			if err != nil || height <= 0 {
				return layoutengine.Rect{}, fmt.Errorf("document: page %d header/footer subtrees leave no body region", page)
			}
			return layoutengine.NewRect(base.X, y, base.Width, height)
		}
		var err error
		if soleTable != nil {
			bodyPlan, err = f.planTypedTableBodies(ctx, doc, *soleTable, "body[0]", selector)
		} else if hasMixedTable || hasMixedRowColumn {
			bodyPlan, err = f.planTypedMixedBodiesMapped(ctx, doc, paperMappingForRegion(mapping, layoutengine.RegionBody), selector)
		} else {
			bodyPlan, err = f.planPaperTextBlocksMappedBodiesContext(ctx, doc, paperMappingForRegion(mapping, layoutengine.RegionBody), selector)
		}
		if err != nil {
			return layoutengine.LayoutPlan{}, err
		}
		pages := uint32(len(bodyPlan.Projection().Pages))
		if pages == 0 {
			return layoutengine.LayoutPlan{}, errors.New("document: typed page template body produced no pages")
		}
		if pages == totalGuess {
			for page := uint32(1); page <= pages; page++ {
				if _, err := load(page, pages); err != nil {
					return layoutengine.LayoutPlan{}, err
				}
			}
			_, _, _, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
			bottomInset, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(bottom))
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			return composeTypedPageShells(bodyPlan, cache, pages, bottomInset)
		}
		totalGuess = pages
	}
	return layoutengine.LayoutPlan{}, errTypedPageCounterUnstable
}

func validateTypedPageTemplateContract(template layout.PageTemplate) error {
	if !finiteNumbers(template.ReserveFooterHeight, template.EvenPageFooterHeight) ||
		template.ReserveFooterHeight < 0 || template.EvenPageFooterHeight < 0 {
		return newTypedShadowUnsupported(typedShadowPageTemplate, "footer reserve heights must be finite and non-negative")
	}
	if template.ReserveFooterHeight != 0 || template.EvenPageFooterHeight != 0 {
		return newTypedShadowUnsupported(typedShadowPageTemplate,
			"manual footer reserve heights conflict with actual planned subtree heights")
	}
	for name, header := range map[string]*layout.HeaderBlock{
		"header": template.Header, "first-page-header": template.FirstPageHeader,
	} {
		if header == nil {
			continue
		}
		if !finiteNumbers(header.Height) || header.Height < 0 {
			return newTypedShadowUnsupported(typedShadowPageTemplate, name+" height must be finite and non-negative")
		}
		if header.Height != 0 {
			return newTypedShadowUnsupported(typedShadowPageTemplate, name+" manual height conflicts with actual planned subtree height")
		}
		box := header.EffectiveBox()
		if htmlUnifiedVisualBox(box) {
			children := layout.NormalizeBlocks(header.Blocks)
			if len(children) == 0 {
				return newTypedShadowUnsupported(typedShadowPageTemplate, name+": a visual shell box requires content")
			}
			if box.Orphans != 0 || box.Widows != 0 {
				return newTypedShadowUnsupported(typedShadowPageTemplate, name+": widow/orphan policy does not apply to a page shell")
			}
		} else if _, err := paperBoxPaginationPolicy(box, header.BoxRef, name); err != nil {
			return newTypedShadowUnsupported(typedShadowPageTemplate, err.Error())
		}
	}
	for name, footer := range map[string]*layout.FooterBlock{
		"footer": template.Footer, "first-page-footer": template.FirstPageFooter, "even-page-footer": template.EvenPageFooter,
	} {
		if footer == nil {
			continue
		}
		if !finiteNumbers(footer.Height) || footer.Height < 0 {
			return newTypedShadowUnsupported(typedShadowPageTemplate, name+" height must be finite and non-negative")
		}
		if footer.Height != 0 {
			return newTypedShadowUnsupported(typedShadowPageTemplate, name+" manual height conflicts with actual planned subtree height")
		}
		box := footer.EffectiveBox()
		if htmlUnifiedVisualBox(box) {
			children := layout.NormalizeBlocks(footer.Blocks)
			if len(children) == 0 {
				return newTypedShadowUnsupported(typedShadowPageTemplate, name+": a visual shell box requires content")
			}
			if box.Orphans != 0 || box.Widows != 0 {
				return newTypedShadowUnsupported(typedShadowPageTemplate, name+": widow/orphan policy does not apply to a page shell")
			}
		} else if _, err := paperBoxPaginationPolicy(box, footer.BoxRef, name); err != nil {
			return newTypedShadowUnsupported(typedShadowPageTemplate, err.Error())
		}
	}
	alias := template.PageTotalAlias()
	if alias != "" {
		for _, blocks := range typedPageTemplateBlockSets(template) {
			if typedBlocksContainText(blocks, alias) {
				return newTypedShadowUnsupported(typedShadowPageTemplate,
					"total-page alias inside a header/footer subtree is a circular page-master dependency")
			}
		}
	}
	return nil
}

func typedPageTemplateBlockSets(template layout.PageTemplate) [][]layout.Block {
	sets := make([][]layout.Block, 0, 5)
	for _, header := range []*layout.HeaderBlock{template.Header, template.FirstPageHeader} {
		if header != nil {
			sets = append(sets, header.Blocks)
		}
	}
	for _, footer := range []*layout.FooterBlock{template.Footer, template.FirstPageFooter, template.EvenPageFooter} {
		if footer != nil {
			sets = append(sets, footer.Blocks)
		}
	}
	return sets
}

func typedBlocksContainText(blocks []layout.Block, text string) bool {
	for _, block := range layout.NormalizeBlocks(blocks) {
		switch block := block.(type) {
		case layout.ParagraphBlock:
			if strings.Contains(layout.TextSegmentsPlainText(block.Segments), text) {
				return true
			}
		case layout.HeadingBlock:
			if strings.Contains(layout.TextSegmentsPlainText(block.Segments), text) {
				return true
			}
		case layout.SectionBlock:
			if strings.Contains(block.Title, text) || typedBlocksContainText(block.Blocks, text) {
				return true
			}
		case layout.ClauseBlock:
			if strings.Contains(block.Title, text) || typedBlocksContainText(block.Blocks, text) {
				return true
			}
		case layout.NoteBoxBlock:
			if strings.Contains(block.Title, text) || typedBlocksContainText(block.Body, text) {
				return true
			}
		case layout.ListBlock:
			for _, item := range block.Items {
				if typedBlocksContainText(item.Blocks, text) {
					return true
				}
			}
		}
	}
	return false
}

func (f *Document) planTypedPageShell(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, page, total uint32) (typedPageShell, error) {
	template := doc.PageTemplate
	header := template.HeaderForPage(int(page))
	footer := template.FooterForPage(int(page))
	headerBlocks := []layout.Block(nil)
	footerBlocks := []layout.Block(nil)
	var headerBox, footerBox layout.BoxStyle
	if header != nil && len(layout.NormalizeBlocks(header.Blocks)) > 0 {
		headerBlocks = append(headerBlocks, header.Blocks...)
		headerBox = header.EffectiveBox()
	}
	if footer != nil && len(layout.NormalizeBlocks(footer.Blocks)) > 0 {
		footerBlocks = append(footerBlocks, footer.Blocks...)
		footerBox = footer.EffectiveBox()
	}
	if text, err := typedPageNumberText(template, page, total); err != nil {
		return typedPageShell{}, err
	} else if text != "" {
		footerBlocks = append(footerBlocks, layout.ParagraphBlock{
			Segments: []layout.TextSegment{{Text: text}},
			Style:    layout.TextStyle{FontFamily: "Helvetica", FontSize: 9, LineHeight: 10, Align: "C"},
			Box:      layout.BoxStyle{KeepTogether: true},
		})
	}
	headerPlan, headerHeight, headerOrigin, err := f.planTypedPageRegion(ctx, doc, headerBlocks, headerBox, "header", layoutengine.RegionHeader, paperMappingForRegion(mapping, layoutengine.RegionHeader), page)
	if err != nil {
		return typedPageShell{}, err
	}
	footerPlan, footerHeight, footerOrigin, err := f.planTypedPageRegion(ctx, doc, footerBlocks, footerBox, "footer", layoutengine.RegionFooter, paperMappingForRegion(mapping, layoutengine.RegionFooter), page)
	if err != nil {
		return typedPageShell{}, err
	}
	return typedPageShell{header: headerPlan, headerHeight: headerHeight, headerOrigin: headerOrigin,
		footer: footerPlan, footerHeight: footerHeight, footerOrigin: footerOrigin}, nil
}

func typedPageNumberText(template layout.PageTemplate, page, total uint32) (string, error) {
	text := template.PageNumberText(int(page))
	if text == "" {
		return "", nil
	}
	if alias := template.PageTotalAlias(); alias != "" {
		text = strings.ReplaceAll(text, alias, strconv.FormatUint(uint64(total), 10))
	}
	if strings.Contains(text, "%!") || strings.ContainsRune(text, '\x00') {
		return "", newTypedShadowUnsupported(typedShadowPageTemplate, "page-number format is invalid")
	}
	return text, nil
}

func (f *Document) planTypedPageRegion(ctx context.Context, doc *layout.LayoutDocument, blocks []layout.Block, box layout.BoxStyle, boxPath string, region layoutengine.RegionID, mapping papercompile.CompileMapping, page uint32) (layoutengine.LayoutPlan, layoutengine.Fixed, layoutengine.Fixed, error) {
	if len(layout.NormalizeBlocks(blocks)) == 0 {
		return layoutengine.LayoutPlan{}, 0, 0, nil
	}
	left, top, right, bottomMargin := typedShadowMargins(f, doc.PageTemplate.Margins)
	_, base, err := typedShadowFixedGeometry(f, left, top, f.w-left-right, f.h-top-bottomMargin)
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, err
	}
	measuredBox, err := f.paperMeasureBox(box, boxPath)
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, newTypedShadowUnsupported(typedShadowPageTemplate, err.Error())
	}
	addInsets := func(first, second layoutengine.Insets) (layoutengine.Insets, error) {
		result := first
		var err error
		if result.Top, err = result.Top.Add(second.Top); err != nil {
			return layoutengine.Insets{}, err
		}
		if result.Right, err = result.Right.Add(second.Right); err != nil {
			return layoutengine.Insets{}, err
		}
		if result.Bottom, err = result.Bottom.Add(second.Bottom); err != nil {
			return layoutengine.Insets{}, err
		}
		if result.Left, err = result.Left.Add(second.Left); err != nil {
			return layoutengine.Insets{}, err
		}
		return result, nil
	}
	totalInsets, err := addInsets(measuredBox.style.Margin, measuredBox.style.Border)
	if err == nil {
		totalInsets, err = addInsets(totalInsets, measuredBox.style.Padding)
	}
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: %s shell insets overflow", region)
	}
	var selector paperBodySelector
	if measuredBox.visual {
		selector = func(_ uint32, candidate layoutengine.Rect) (layoutengine.Rect, error) {
			// Horizontal shell insets are represented in the region document's
			// base margins so strict table planners see one stable x/width on every
			// selected page. Only vertical insets remain selector-relative.
			content, insetErr := candidate.Inset(layoutengine.Insets{Top: totalInsets.Top, Bottom: totalInsets.Bottom})
			if insetErr != nil || content.Width <= 0 || content.Height <= 0 {
				return layoutengine.Rect{}, fmt.Errorf("document: %s shell box leaves no content region", region)
			}
			return content, nil
		}
	}
	regionDoc := *doc
	regionDoc.Body = append([]layout.Block(nil), blocks...)
	regionDoc.PageTemplate = layout.PageTemplate{Margins: doc.PageTemplate.Margins}
	if measuredBox.visual {
		regionDoc.PageTemplate.Margins = layout.Spacing{
			Left: left + f.PointConvert(totalInsets.Left.Points()), Top: top,
			Right: right + f.PointConvert(totalInsets.Right.Points()), Bottom: bottomMargin,
		}
	}
	regionDoc.Signature, regionDoc.QR, regionDoc.Attachments = nil, nil, nil
	normalized := layout.NormalizeBlocks(regionDoc.Body)
	containsTable := typedBlocksContainTable(normalized)
	var plan layoutengine.LayoutPlan
	if len(normalized) == 1 {
		if table, ok := normalized[0].(layout.TableBlock); ok {
			plan, err = f.planTypedTableBodies(ctx, &regionDoc, table, boxPath+".blocks[0]", selector)
		} else {
			plan, err = f.planPaperTextBlocksMappedBodiesContext(ctx, &regionDoc, mapping, selector)
		}
	} else if containsTable || (len(normalized) > 1 && typedBlocksContainRowColumn(normalized)) {
		plan, err = f.planTypedMixedBodiesMapped(ctx, &regionDoc, paperMappingForRegion(mapping, region), selector)
	} else {
		plan, err = f.planPaperTextBlocksMappedBodiesContext(ctx, &regionDoc, mapping, selector)
	}
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s subtree: %w", page, region, err)
	}
	projection := plan.Projection()
	if len(projection.Pages) != 1 {
		return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s subtree exceeds one page", page, region)
	}
	topFixed, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(top))
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, err
	}
	bottom := topFixed
	for _, fragment := range projection.Fragments {
		candidate, bottomErr := fragment.BorderBox.Bottom()
		if bottomErr != nil {
			return layoutengine.LayoutPlan{}, 0, 0, bottomErr
		}
		if candidate > bottom {
			bottom = candidate
		}
	}
	if !measuredBox.visual {
		height, heightErr := bottom.Sub(topFixed)
		if heightErr != nil || height <= 0 {
			return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s subtree has invalid height", page, region)
		}
		return plan, height, topFixed, nil
	}
	contentTop, err := base.Y.Add(totalInsets.Top)
	if err != nil || bottom <= contentTop {
		return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s shell content has invalid height", page, region)
	}
	contentHeight, _ := bottom.Sub(contentTop)
	borderX, err := base.X.Add(measuredBox.style.Margin.Left)
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, err
	}
	borderY, err := base.Y.Add(measuredBox.style.Margin.Top)
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, err
	}
	borderWidth, err := base.Width.Sub(measuredBox.style.Margin.Left)
	if err == nil {
		borderWidth, err = borderWidth.Sub(measuredBox.style.Margin.Right)
	}
	borderHeight := contentHeight
	for _, inset := range []layoutengine.Fixed{measuredBox.style.Border.Top, measuredBox.style.Padding.Top,
		measuredBox.style.Padding.Bottom, measuredBox.style.Border.Bottom} {
		if err == nil {
			borderHeight, err = borderHeight.Add(inset)
		}
	}
	if err != nil || borderWidth <= 0 || borderHeight <= 0 {
		return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s shell border box is invalid", page, region)
	}
	borderBox, err := layoutengine.NewRect(borderX, borderY, borderWidth, borderHeight)
	if err != nil {
		return layoutengine.LayoutPlan{}, 0, 0, err
	}
	height := borderHeight
	if height, err = height.Add(measuredBox.style.Margin.Top); err == nil {
		height, err = height.Add(measuredBox.style.Margin.Bottom)
	}
	if err != nil || height <= 0 {
		return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s shell outer height is invalid", page, region)
	}
	if measuredBox.background.Set || measuredBox.borders[0].width > 0 || measuredBox.borders[1].width > 0 || measuredBox.borders[2].width > 0 || measuredBox.borders[3].width > 0 {
		plan, err = layoutengine.AttachBoxDecorations(plan, []layoutengine.BoxDecoration{{
			Fragment: projection.Fragments[0].ID, BorderBox: &borderBox, Background: measuredBox.background,
			Top:    layoutengine.BoxBorderSide{Width: measuredBox.borders[0].width, Color: measuredBox.borders[0].color},
			Right:  layoutengine.BoxBorderSide{Width: measuredBox.borders[1].width, Color: measuredBox.borders[1].color},
			Bottom: layoutengine.BoxBorderSide{Width: measuredBox.borders[2].width, Color: measuredBox.borders[2].color},
			Left:   layoutengine.BoxBorderSide{Width: measuredBox.borders[3].width, Color: measuredBox.borders[3].color},
		}})
		if err != nil {
			return layoutengine.LayoutPlan{}, 0, 0, fmt.Errorf("document: page %d %s shell decoration: %w", page, region, err)
		}
	}
	return plan, height, base.Y, nil
}

func translateTypedRect(rect layoutengine.Rect, dx, dy layoutengine.Fixed) (layoutengine.Rect, error) {
	x, err := rect.X.Add(dx)
	if err != nil {
		return layoutengine.Rect{}, err
	}
	y, err := rect.Y.Add(dy)
	if err != nil {
		return layoutengine.Rect{}, err
	}
	return layoutengine.NewRect(x, y, rect.Width, rect.Height)
}

func translateTypedPoint(point layoutengine.Point, dx, dy layoutengine.Fixed) (layoutengine.Point, error) {
	x, err := point.X.Add(dx)
	if err != nil {
		return layoutengine.Point{}, err
	}
	y, err := point.Y.Add(dy)
	if err != nil {
		return layoutengine.Point{}, err
	}
	return layoutengine.Point{X: x, Y: y}, nil
}

type typedComposedProjection struct {
	projection layoutengine.LayoutPlanProjection
	page       uint32
	region     layoutengine.RegionID
	dx         layoutengine.Fixed
	dy         layoutengine.Fixed
	artifact   bool
}

func composeTypedPageShells(bodyPlan layoutengine.LayoutPlan, cache map[typedPageShellKey]typedPageShell, pages uint32, bottomInset layoutengine.Fixed) (layoutengine.LayoutPlan, error) {
	body := bodyPlan.Projection()
	input := layoutengine.LayoutPlanInput{}
	fonts := make([]layoutengine.CoreFontResource, 0, len(body.Fonts))
	fontIDs := make(map[paperCoreFontIdentity]layoutengine.FontResourceID)
	fontID := func(font layoutengine.CoreFontResource) layoutengine.FontResourceID {
		key := paperFontIdentity(font)
		if id := fontIDs[key]; id != 0 {
			return id
		}
		id := layoutengine.FontResourceID(len(fonts) + 1)
		font.ID = id
		fonts = append(fonts, font)
		fontIDs[key] = id
		return id
	}

	input.SemanticNodes = append(input.SemanticNodes, body.SemanticNodes...)
	if len(input.SemanticNodes) == 0 {
		input.SemanticNodes = append(input.SemanticNodes, layoutengine.SemanticNode{
			ID: 1, Role: layoutengine.SemanticRoleDocument, Key: "@typed-document", Instance: "@typed-document",
		})
	}
	regions := make([]typedComposedProjection, 0, int(pages)*3)
	for page := uint32(1); page <= pages; page++ {
		shell := cache[typedPageShellKey{page: page, total: pages}]
		pageSize := body.Pages[page-1].Size
		if shell.headerHeight > 0 {
			projection := shell.header.Projection()
			regions = append(regions, typedComposedProjection{projection: projection, page: page,
				region: layoutengine.RegionHeader, artifact: true})
		}
		regions = append(regions, typedComposedProjection{projection: body, page: page, region: layoutengine.RegionBody})
		if shell.footerHeight > 0 {
			projection := shell.footer.Projection()
			origin := shell.footerOrigin
			target, err := pageSize.Height.Sub(bottomInset)
			if err == nil {
				target, err = target.Sub(shell.footerHeight)
			}
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			dy, err := target.Sub(origin)
			if err != nil {
				return layoutengine.LayoutPlan{}, err
			}
			regions = append(regions, typedComposedProjection{projection: projection, page: page,
				region: layoutengine.RegionFooter, dy: dy, artifact: true})
		}
	}

	fragmentMaps := make([]map[layoutengine.FragmentID]layoutengine.FragmentID, len(regions))
	lineMaps := make([]map[uint32]uint32, len(regions))
	bodyFragmentMap := make(map[layoutengine.FragmentID]layoutengine.FragmentID)
	var nextNode layoutengine.NodeID
	for _, fragment := range body.Fragments {
		if fragment.Node > nextNode {
			nextNode = fragment.Node
		}
	}
	for page := uint32(1); page <= pages; page++ {
		pagePlan := layoutengine.PlannedPage{Number: page, Size: body.Pages[page-1].Size,
			Fragments: layoutengine.IndexRange{Start: uint32(len(input.Fragments))},
			Lines:     layoutengine.IndexRange{Start: uint32(len(input.Lines))}}
		for regionIndex, region := range regions {
			if region.page != page {
				continue
			}
			fragmentMap := make(map[layoutengine.FragmentID]layoutengine.FragmentID)
			lineMap := make(map[uint32]uint32)
			shellNodes := make(map[layoutengine.NodeID]layoutengine.NodeID)
			shellOwners := make(map[layoutengine.NodeID]layoutengine.SemanticNodeID)
			fragmentMaps[regionIndex], lineMaps[regionIndex] = fragmentMap, lineMap
			for _, fragment := range region.projection.Fragments {
				if fragment.Page != 1 && region.region != layoutengine.RegionBody || region.region == layoutengine.RegionBody && fragment.Page != page {
					continue
				}
				oldID := fragment.ID
				var shellOwner layoutengine.SemanticNodeID
				if region.region != layoutengine.RegionBody {
					oldNode := fragment.Node
					if shellNodes[oldNode] == 0 {
						nextNode++
						shellNodes[oldNode] = nextNode
					}
					fragment.Node = shellNodes[oldNode]
					shellOwner = shellOwners[oldNode]
					if !shellOwner.Valid() {
						shellOwner = layoutengine.SemanticNodeID(len(input.SemanticNodes) + 1)
						identity := layoutengine.NodeKey(fmt.Sprintf("@page-%d-%s-node-%d", page, region.region, oldNode))
						input.SemanticNodes = append(input.SemanticNodes, layoutengine.SemanticNode{ID: shellOwner, Parent: 1,
							Role: layoutengine.SemanticRoleArtifact, Key: identity, Instance: layoutengine.InstanceID(identity), Source: fragment.Source})
						shellOwners[oldNode] = shellOwner
					}
					owner := input.SemanticNodes[shellOwner-1]
					fragment.Key, fragment.Instance = owner.Key, owner.Instance
				}
				fragment.ID = layoutengine.FragmentID(len(input.Fragments) + 1)
				fragment.Page, fragment.Region = page, region.region
				var err error
				fragment.BorderBox, err = translateTypedRect(fragment.BorderBox, region.dx, region.dy)
				if err == nil {
					fragment.ContentBox, err = translateTypedRect(fragment.ContentBox, region.dx, region.dy)
				}
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				input.Fragments = append(input.Fragments, fragment)
				fragmentMap[oldID] = fragment.ID
				if region.region == layoutengine.RegionBody {
					bodyFragmentMap[oldID] = fragment.ID
				}
				if shellOwner.Valid() {
					input.SemanticFragments = append(input.SemanticFragments, layoutengine.SemanticFragmentAssociation{Semantic: shellOwner, Page: page, Fragment: fragment.ID})
				}
			}
			for oldIndex, line := range region.projection.Lines {
				newFragment := fragmentMap[line.Fragment]
				if !newFragment.Valid() {
					continue
				}
				line.Fragment = newFragment
				var err error
				line.Bounds, err = translateTypedRect(line.Bounds, region.dx, region.dy)
				if err == nil {
					line.Baseline, err = line.Baseline.Add(region.dy)
				}
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				lineMap[uint32(oldIndex)] = uint32(len(input.Lines))
				input.Lines = append(input.Lines, line)
			}
		}
		pagePlan.Fragments.Count = uint32(len(input.Fragments)) - pagePlan.Fragments.Start
		pagePlan.Lines.Count = uint32(len(input.Lines)) - pagePlan.Lines.Start
		input.Pages = append(input.Pages, pagePlan)
	}

	for _, association := range body.SemanticFragments {
		if fragment := bodyFragmentMap[association.Fragment]; fragment.Valid() {
			association.Fragment, association.Page = fragment, input.Fragments[fragment-1].Page
			input.SemanticFragments = append(input.SemanticFragments, association)
		}
	}
	for _, occurrence := range body.ReadingOrder {
		if fragment := bodyFragmentMap[occurrence.Fragment]; fragment.Valid() {
			occurrence.Fragment, occurrence.Page = fragment, input.Fragments[fragment-1].Page
			input.ReadingOrder = append(input.ReadingOrder, occurrence)
		}
	}

	items := make([]layoutengine.DisplayItem, 0)
	paths := make([]layoutengine.PlannedPath, 0)
	fills := make([]layoutengine.PlannedFill, 0)
	strokes := make([]layoutengine.PlannedStroke, 0)
	imageResources := make([]layoutengine.ImageResource, 0, len(body.ImageResources))
	imageResourceIDs := make(map[layoutengine.ImageContentDigest]layoutengine.ImageResourceID)
	imageID := func(resource layoutengine.ImageResource) (layoutengine.ImageResourceID, error) {
		if id := imageResourceIDs[resource.Digest]; id.Valid() {
			existing := imageResources[id-1]
			if existing.Format != resource.Format || existing.PixelWidth != resource.PixelWidth || existing.PixelHeight != resource.PixelHeight {
				return 0, errors.New("document: page-shell image digest has inconsistent intrinsic metadata")
			}
			return id, nil
		}
		id := layoutengine.ImageResourceID(len(imageResources) + 1)
		resource.ID = id
		imageResources = append(imageResources, resource)
		imageResourceIDs[resource.Digest] = id
		return id, nil
	}
	destinations := append([]layoutengine.PlannedDestination(nil), body.Destinations...)
	links := make([]layoutengine.PlannedLink, 0, len(body.Links))
	for index := range destinations {
		if destinations[index].Fragment.Valid() {
			destinations[index].Fragment = bodyFragmentMap[destinations[index].Fragment]
		}
	}
	for regionIndex, region := range regions {
		fragmentMap, lineMap := fragmentMaps[regionIndex], lineMaps[regionIndex]
		if fragmentMap == nil {
			continue
		}
		destinationMap := make(map[layoutengine.DestinationID]layoutengine.DestinationID, len(region.projection.Destinations))
		if region.region == layoutengine.RegionBody {
			for _, destination := range region.projection.Destinations {
				destinationMap[destination.ID] = destination.ID
			}
		} else {
			for _, destination := range region.projection.Destinations {
				if destination.Page != 1 {
					return layoutengine.LayoutPlan{}, errors.New("document: page-shell destination references a non-local page")
				}
				oldID := destination.ID
				destination.ID = layoutengine.DestinationID(len(destinations) + 1)
				destination.Page = region.page
				if destination.Fragment.Valid() {
					destination.Fragment = fragmentMap[destination.Fragment]
					if !destination.Fragment.Valid() {
						return layoutengine.LayoutPlan{}, errors.New("document: page-shell destination references a missing fragment")
					}
				}
				point, err := translateTypedPoint(destination.Point, region.dx, region.dy)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				destination.Point = point
				destinationMap[oldID] = destination.ID
				destinations = append(destinations, destination)
			}
		}
		pathMap := make(map[uint32]uint32)
		mapPath := func(old uint32) (uint32, error) {
			if mapped, ok := pathMap[old]; ok {
				return mapped, nil
			}
			if uint64(old) >= uint64(len(region.projection.Paths)) {
				return 0, errors.New("document: page-shell graphic references a missing path")
			}
			path, err := translatePaperNestedPath(region.projection.Paths[old], region.dx, region.dy)
			if err != nil {
				return 0, err
			}
			mapped := uint32(len(paths))
			paths = append(paths, path)
			pathMap[old] = mapped
			return mapped, nil
		}
		for _, command := range region.projection.Commands {
			if !fragmentMap[command.Fragment].Valid() {
				continue
			}
			switch command.Kind {
			case layoutengine.CommandGlyphRun:
				run := region.projection.GlyphRuns[command.Payload]
				run.Line = lineMap[run.Line]
				run.Font = fontID(region.projection.Fonts[run.Font-1])
				origin, err := translateTypedPoint(run.Origin, region.dx, region.dy)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				run.Origin = origin
				run.Advances = append([]layoutengine.Fixed(nil), run.Advances...)
				input.GlyphRuns = append(input.GlyphRuns, run)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.GlyphRuns) - 1)})
			case layoutengine.CommandImage:
				image := region.projection.Images[command.Payload]
				image.Fragment = fragmentMap[image.Fragment]
				resource, err := imageID(region.projection.ImageResources[image.Resource-1])
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				image.Resource = resource
				image.Bounds, err = translateTypedRect(image.Bounds, region.dx, region.dy)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				if image.Crop != nil {
					crop := *image.Crop
					crop.Clip, err = translateTypedRect(crop.Clip, region.dx, region.dy)
					if err != nil {
						return layoutengine.LayoutPlan{}, err
					}
					image.Crop = &crop
				}
				input.Images = append(input.Images, image)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(input.Images) - 1)})
			case layoutengine.CommandLink:
				link := region.projection.Links[command.Payload]
				link.Fragment = fragmentMap[link.Fragment]
				if link.Destination.Valid() {
					link.Destination = destinationMap[link.Destination]
					if !link.Destination.Valid() {
						return layoutengine.LayoutPlan{}, errors.New("document: page-shell link references a missing destination")
					}
				}
				var err error
				link.Bounds, err = translateTypedRect(link.Bounds, region.dx, region.dy)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				links = append(links, link)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(links) - 1)})
			case layoutengine.CommandFillPath:
				fill := region.projection.Fills[command.Payload]
				path, err := mapPath(fill.Path)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				fill.Path, fill.Fragment = path, fragmentMap[fill.Fragment]
				fills = append(fills, fill)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(fills) - 1)})
			case layoutengine.CommandStrokePath:
				stroke := region.projection.Strokes[command.Payload]
				path, err := mapPath(stroke.Path)
				if err != nil {
					return layoutengine.LayoutPlan{}, err
				}
				stroke.Path, stroke.Fragment = path, fragmentMap[stroke.Fragment]
				strokes = append(strokes, stroke)
				items = append(items, layoutengine.DisplayItem{Kind: command.Kind, Payload: uint32(len(strokes) - 1)})
			default:
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: unsupported page-shell command %q", command.Kind)
			}
		}
	}
	for index := range links {
		if links[index].Destination.Valid() && uint64(links[index].Destination) > uint64(len(destinations)) {
			return layoutengine.LayoutPlan{}, errors.New("document: composed page-shell link destination is missing")
		}
	}

	for _, decision := range body.Breaks {
		decision.Preceding = bodyFragmentMap[decision.Preceding]
		decision.Triggering = bodyFragmentMap[decision.Triggering]
		input.Breaks = append(input.Breaks, decision)
	}
	for _, diagnostic := range body.Diagnostics {
		if diagnostic.Location.Fragment.Valid() {
			diagnostic.Location.Fragment = bodyFragmentMap[diagnostic.Location.Fragment]
		}
		input.Diagnostics = append(input.Diagnostics, diagnostic)
	}
	geometryInput := input
	geometryInput.GlyphRuns = nil
	geometryInput.Images = nil
	geometryInput.Paths, geometryInput.Fills, geometryInput.Strokes = nil, nil, nil
	geometry, err := layoutengine.NewLayoutPlan(geometryInput)
	if err != nil {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: compose typed page-shell geometry: %w", err)
	}
	return layoutengine.AttachDisplayList(geometry, layoutengine.DisplayListInput{
		Fonts: fonts, GlyphRuns: input.GlyphRuns, ImageResources: imageResources, Images: input.Images,
		Destinations: destinations, Links: links, Paths: paths, Fills: fills, Strokes: strokes, Items: items,
	})
}
