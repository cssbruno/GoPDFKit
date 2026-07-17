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

// htmlStartFrame is the immutable compatibility boundary between an open PDF
// page and whole-fragment HTML planning. Values are captured in document units;
// Body is fixed-point page geometry consumed by the unified planner.
type htmlStartFrame struct {
	page, pageCount                int
	pageSize                       Size
	orientation                    string
	rotation                       int
	left, top, right, bottom       float64
	x, y                           float64
	fontFamily, fontStyle          string
	fontSizePoints                 float64
	underline, strikeout           bool
	autoPageBreak, customPageBreak bool
	body                           layoutengine.Rect
}

type htmlFinalFrame struct {
	page int
	x, y float64
}

type htmlFragmentPlan struct {
	plan               LayoutDocumentPlan
	start              htmlStartFrame
	final              htmlFinalFrame
	reuseCurrentPage   bool
	appendTrailingPage bool
}

func (f *Document) captureHTMLStartFrame() (htmlStartFrame, error) {
	if f == nil || f.err != nil || f.page <= 0 || f.state != documentStatePageOpen {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "requires an open, error-free current page")
	}
	if f.page != f.PageCount() {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "current page must be the final document page")
	}
	if f.clipNest != 0 || f.transformNest != 0 || f.inHeader || f.inFooter {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "active clipping, transforms, headers, or footers cannot cross the fragment boundary")
	}
	if f.headerFnc != nil || f.footerFnc != nil || f.footerFncLpi != nil || f.pageAddGuard != nil ||
		len(f.aliasMap) != 0 || f.aliasNbPagesStr != "" {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "custom or deferred page lifecycle behavior is not in the unified cohort")
	}
	if !f.autoPageBreak || f.acceptPageBreakSet {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "the unified cohort requires the standard automatic page-break policy")
	}
	if f.curRotation != 0 {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "rotated current pages are not in the unified cohort")
	}
	if !typedShadowCoreFont(f.coreFonts, f.fontFamily) || f.currentFont.Name == "" || f.fontSizePt <= 0 {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "current font context must be a selected core font")
	}
	_, body, err := typedShadowFixedGeometry(f, f.lMargin, f.tMargin,
		f.w-f.lMargin-f.rMargin, f.h-f.tMargin-f.bMargin)
	if err != nil {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, err.Error())
	}
	bodyBottom, _ := body.Bottom()
	cursor, err := fixedFromDocumentUnits(f, f.y)
	if err != nil || cursor < body.Y || cursor >= bodyBottom {
		return htmlStartFrame{}, htmlPlanUnsupported("frame", 0, "current cursor must be inside the active body region")
	}
	return htmlStartFrame{
		page: f.page, pageCount: f.PageCount(),
		pageSize:    f.curPageSize,
		orientation: f.curOrientation, rotation: f.curRotation,
		left: f.lMargin, top: f.tMargin, right: f.rMargin, bottom: f.bMargin,
		x: f.x, y: f.y, fontFamily: f.fontFamily, fontStyle: f.fontStyle,
		fontSizePoints: f.fontSizePt, underline: f.underline, strikeout: f.strikeout,
		autoPageBreak:   f.autoPageBreak,
		customPageBreak: f.acceptPageBreakSet, body: body,
	}, nil
}

func (html *HTML) planCompiledHTMLFragmentContext(ctx context.Context, lineHeight float64, compiled *CompiledHTML) (htmlFragmentPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return htmlFragmentPlan{}, err
	}
	if lineHeight <= 0 {
		return htmlFragmentPlan{}, errors.New("document: HTML unified plan line height must be positive")
	}
	if compiled == nil {
		return htmlFragmentPlan{}, errors.New("document: compiled HTML is nil")
	}
	if err := compiled.validate(); err != nil {
		return htmlFragmentPlan{}, err
	}
	if len(compiled.recovery) != 0 {
		return htmlFragmentPlan{}, htmlPlanUnsupported("malformed-recovery", 0, "recovered HTML is not accepted by the strict unified cohort")
	}
	frame, err := html.pdf.captureHTMLStartFrame()
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	if len(compiled.inlineSVGs) != 0 {
		if _, soleErr := htmlUnifiedInlineSVGMeta(compiled); soleErr == nil {
			return html.planCompiledInlineSVGFragmentContext(ctx, compiled, frame)
		}
	}
	hasImage := false
	for _, token := range compiled.tokens {
		if token.Cat == 'O' && token.Str == "img" {
			hasImage = true
			src := strings.TrimSpace(token.Attr["src"])
			if src != "" && !strings.HasPrefix(strings.ToLower(src), "data:") && !html.AllowLocalImages {
				return htmlFragmentPlan{}, fmt.Errorf("%w: local HTML images require HTML.AllowLocalImages", ErrSecurityPolicyDenied)
			}
		}
	}
	resolved, err := html.pdf.resolveCompiledHTMLUnifiedSnapshot(ctx, compiled, lineHeight)
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	if hasImage {
		resolved, err = html.pdf.resolveCompiledHTMLImageSources(ctx, resolved)
		if err != nil {
			return htmlFragmentPlan{}, err
		}
	}
	availableWidth := html.pdf.PointConvert(frame.body.Width.Points())
	availableHeight := html.pdf.PointConvert(frame.body.Height.Points())
	var model *layout.LayoutDocument
	var svgMetas []htmlUnifiedSVGMeta
	var svgPlaceholder layoutengine.ImageContentDigest
	if len(resolved.inlineSVGs) != 0 {
		svgMetas, err = htmlUnifiedMixedSVGMetas(resolved)
		if err != nil {
			return htmlFragmentPlan{}, err
		}
		model, svgPlaceholder, err = lowerCompiledHTMLMixedSVGUnitsBounds(ctx, resolved, lineHeight, html.pdf.PointConvert, availableWidth, availableHeight, svgMetas)
	} else {
		model, err = lowerCompiledHTMLTextCohortUnitsWidth(ctx, resolved, lineHeight, html.pdf.PointConvert, availableWidth)
	}
	if err != nil {
		return htmlFragmentPlan{}, err
	}
	if htmlModelHasOversizedTableRow(model, availableWidth, html.pdf.PointConvert(frame.body.Height.Points())) {
		return htmlFragmentPlan{}, fmt.Errorf("%w: structured HTML table row exceeds one page body", ErrHTMLLimitExceeded)
	}
	leadingBreak := false
	for len(model.Body) != 0 {
		pageBreak, ok := model.Body[0].(layout.PageBreakBlock)
		if !ok || (!pageBreak.Before && !pageBreak.After) {
			break
		}
		leadingBreak = true
		model.Body = model.Body[1:]
	}
	trailingBreak := false
	for len(model.Body) != 0 {
		pageBreak, ok := model.Body[len(model.Body)-1].(layout.PageBreakBlock)
		if !ok || (!pageBreak.Before && !pageBreak.After) {
			break
		}
		trailingBreak = true
		model.Body = model.Body[:len(model.Body)-1]
	}
	if len(model.Body) == 0 {
		return htmlFragmentPlan{}, htmlPlanUnsupported("frame", 0, "fragment contains only page-break boundaries")
	}
	ctx = withHTMLAuthoredWhitespace(ctx)
	model.PageTemplate.Margins = layout.Spacing{Left: frame.left, Top: frame.top, Right: frame.right, Bottom: frame.bottom}
	tree, err := papercompile.LowerLayoutDocumentTreeContext(ctx, model, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		return htmlFragmentPlan{}, fmt.Errorf("document: lower HTML canonical tree: %w", err)
	}
	cursor, _ := fixedFromDocumentUnits(html.pdf, frame.y)
	blocks := layout.NormalizeBlocks(model.Body)
	planAtFrame := func(startOnNewPage bool) (layoutengine.LayoutPlan, error) {
		selectBody := func(page uint32, base layoutengine.Rect) (layoutengine.Rect, error) {
			if page != 1 || startOnNewPage {
				return base, nil
			}
			bottom, bottomErr := base.Bottom()
			if bottomErr != nil {
				return layoutengine.Rect{}, bottomErr
			}
			height, heightErr := bottom.Sub(cursor)
			if heightErr != nil {
				return layoutengine.Rect{}, heightErr
			}
			return layoutengine.NewRect(base.X, cursor, base.Width, height)
		}
		if len(blocks) == 1 && !typedBlocksContainTable(blocks) && !typedBlocksNeedMixedBoxContainers(blocks) {
			return html.pdf.planPaperTextBlocksMappedBodiesContext(ctx, model, papercompile.CompileMapping{}, selectBody)
		}
		return html.pdf.planTypedMixedBodies(ctx, model, selectBody)
	}
	planned, err := planAtFrame(leadingBreak)
	if err != nil && !leadingBreak {
		// A keep-together block may be larger than the captured remainder while
		// still fitting an empty body page. Retry atomically at the next page
		// before classifying the fragment as unsupported.
		if retry, retryErr := planAtFrame(true); retryErr == nil {
			planned, err, leadingBreak = retry, nil, true
		}
	}
	if err != nil {
		if errors.Is(err, ErrLayoutDocumentPlanUnsupported) || errors.Is(err, errTypedShadowUnsupported) {
			return htmlFragmentPlan{}, fmt.Errorf("%w: frame planning: %v", ErrHTMLPlanUnsupported, err)
		}
		return htmlFragmentPlan{}, err
	}
	if !leadingBreak && htmlBlocksContainKeptTable(blocks) && cursor > frame.body.Y {
		fresh, freshErr := planAtFrame(true)
		if freshErr != nil {
			return htmlFragmentPlan{}, freshErr
		}
		bodyBottom, _ := frame.body.Bottom()
		remaining, _ := bodyBottom.Sub(cursor)
		if htmlFirstPageBodyExtent(fresh) > remaining {
			planned, leadingBreak = fresh, true
		}
	}
	projection := planned.Projection()
	if compiledHTMLTableRowCount(compiled) == 1 && len(projection.Pages) > 1 {
		return htmlFragmentPlan{}, fmt.Errorf("%w: structured HTML table row exceeds one page body", ErrHTMLLimitExceeded)
	}
	if !leadingBreak {
		bodyBottom, _ := frame.body.Bottom()
		overflowsCapturedBody := false
		for _, fragment := range projection.Fragments {
			if fragment.Page != 1 || fragment.Region != layoutengine.RegionBody {
				continue
			}
			bottom, bottomErr := fragment.BorderBox.Bottom()
			if bottomErr != nil {
				return htmlFragmentPlan{}, bottomErr
			}
			if bottom > bodyBottom {
				overflowsCapturedBody = true
				break
			}
		}
		if overflowsCapturedBody {
			leadingBreak = true
			planned, err = planAtFrame(true)
			if err != nil {
				return htmlFragmentPlan{}, err
			}
			projection = planned.Projection()
		}
	}
	if len(svgMetas) != 0 {
		planned, err = composeHTMLMixedSVGPlan(ctx, planned, svgMetas, svgPlaceholder)
		if err != nil {
			return htmlFragmentPlan{}, err
		}
		projection = planned.Projection()
	}
	planned, err = bindTypedDeterministicInputs(planned, tree, model)
	if err != nil {
		return htmlFragmentPlan{}, fmt.Errorf("document: bind HTML fragment deterministic inputs: %w", err)
	}
	projection = planned.Projection()
	hash, err := planned.Hash()
	if err != nil {
		return htmlFragmentPlan{}, fmt.Errorf("document: hash HTML fragment plan: %w", err)
	}
	if len(projection.Pages) == 0 {
		return htmlFragmentPlan{}, htmlPlanUnsupported("frame", 0, "fragment planning produced no pages")
	}
	reuseCurrent := !leadingBreak
	addedPages := len(projection.Pages)
	if reuseCurrent {
		addedPages--
	}
	if trailingBreak {
		addedPages++
	}
	if html.pdf.limits.MaxPages > 0 && frame.pageCount+addedPages > html.pdf.limits.MaxPages {
		return htmlFragmentPlan{}, fmt.Errorf("%w: %d > %d", ErrPageLimitExceeded, frame.pageCount+addedPages, html.pdf.limits.MaxPages)
	}
	generatedPages := addedPages + 1
	if generatedPages > html.maxGeneratedPages() {
		return htmlFragmentPlan{}, fmt.Errorf("%w: HTML rendering exceeded maximum generated pages: %d > %d", ErrHTMLLimitExceeded, generatedPages, html.maxGeneratedPages())
	}
	lastPage := uint32(len(projection.Pages))
	finalY := layoutengine.Fixed(0)
	fragmentPages := make(map[layoutengine.FragmentID]uint32, len(projection.Fragments))
	for _, fragment := range projection.Fragments {
		fragmentPages[fragment.ID] = fragment.Page
		if fragment.Page != lastPage || fragment.Region != layoutengine.RegionBody {
			continue
		}
		bottom, bottomErr := fragment.BorderBox.Bottom()
		if bottomErr != nil {
			return htmlFragmentPlan{}, bottomErr
		}
		if bottom > finalY {
			finalY = bottom
		}
	}
	for _, line := range projection.Lines {
		if fragmentPages[line.Fragment] != lastPage {
			continue
		}
		bottom, bottomErr := line.Bounds.Bottom()
		if bottomErr != nil {
			return htmlFragmentPlan{}, bottomErr
		}
		if bottom > finalY {
			finalY = bottom
		}
	}
	if finalY == 0 {
		return htmlFragmentPlan{}, htmlPlanUnsupported("frame", 0, "fragment has no final body cursor")
	}
	// A live cursor must never round to a coordinate fractionally before the
	// exact fixed-point content edge when converted back to document units.
	finalY++
	imageSources, err := typedLayoutImageSourcesContext(ctx, model, uint64(html.pdf.imageSourceLimit()))
	if err != nil {
		return htmlFragmentPlan{}, fmt.Errorf("document: build bounded HTML image resource catalog: %w", err)
	}
	imageSources = withoutHTMLSVGPlaceholder(imageSources, svgPlaceholder)
	finalPage := frame.page + addedPages
	finalYUnits := html.pdf.PointConvert(finalY.Points())
	if trailingBreak {
		finalYUnits = frame.top
	}
	plan := LayoutDocumentPlan{plan: planned, tree: tree, hash: hash.String(), pages: len(projection.Pages), imageSources: imageSources}
	return htmlFragmentPlan{plan: plan, start: frame, reuseCurrentPage: reuseCurrent, appendTrailingPage: trailingBreak,
		final: htmlFinalFrame{page: finalPage, x: frame.left, y: finalYUnits}}, nil
}

func htmlBlocksContainKeptTable(blocks []layout.Block) bool {
	for _, block := range blocks {
		if table, ok := block.(layout.TableBlock); ok && (table.Box.KeepTogether || table.Style.KeepRows) {
			return true
		}
	}
	return false
}

func htmlModelHasOversizedTableRow(model *layout.LayoutDocument, width, bodyHeight float64) bool {
	if model == nil || width <= 0 || bodyHeight <= 0 {
		return false
	}
	measure := layout.NewMeasureContext(width, layout.TextStyle{})
	for _, block := range layout.NormalizeBlocks(model.Body) {
		table, ok := block.(layout.TableBlock)
		if !ok {
			continue
		}
		rows := append(append(append([]layout.TableRow(nil), table.Header...), table.Body...), table.Footer...)
		for _, row := range rows {
			for _, cell := range row.Cells {
				height := cell.Box.Padding.Top + cell.Box.Padding.Bottom
				for _, child := range layout.NormalizeBlocks(cell.Blocks) {
					height += layout.MeasureBlock(measure, child).Height
				}
				if height > bodyHeight {
					return true
				}
			}
		}
	}
	return false
}

func htmlFirstPageBodyExtent(plan layoutengine.LayoutPlan) layoutengine.Fixed {
	projection := plan.Projection()
	var top, bottom layoutengine.Fixed
	set := false
	for _, fragment := range projection.Fragments {
		if fragment.Page != 1 || fragment.Region != layoutengine.RegionBody {
			continue
		}
		fragmentBottom, err := fragment.BorderBox.Bottom()
		if err != nil {
			continue
		}
		if !set || fragment.BorderBox.Y < top {
			top = fragment.BorderBox.Y
		}
		if !set || fragmentBottom > bottom {
			bottom = fragmentBottom
		}
		set = true
	}
	if !set || bottom <= top {
		return 0
	}
	return bottom - top
}

func compiledHTMLTableRowCount(compiled *CompiledHTML) int {
	if compiled == nil {
		return 0
	}
	count := 0
	for _, token := range compiled.tokens {
		if token.Cat == 'O' && token.Str == "tr" {
			count++
		}
	}
	return count
}

func (html *HTML) writeCompiledUnifiedFragmentContext(ctx context.Context, lineHeight float64, compiled *CompiledHTML) (bool, error) {
	fragment, err := html.planCompiledHTMLFragmentContext(ctx, lineHeight, compiled)
	if err != nil {
		if errors.Is(err, ErrHTMLPlanUnsupported) {
			return false, err
		}
		return true, err
	}
	projection := fragment.plan.plan.Projection()
	withDisplay := len(projection.ImageResources) != 0 || len(projection.Links) != 0 || len(projection.Destinations) != 0 ||
		len(projection.Paths) != 0 || len(projection.Fills) != 0 || len(projection.Strokes) != 0 || len(projection.Clips) != 0 || len(projection.Transforms) != 0
	withDisplay = withDisplay || layoutPlanHasMultipleGlyphRunsPerLine(projection)
	if withDisplay {
		prepared, preflightErr := html.pdf.preflightDisplayLayoutPlanPDFContextForTarget(ctx, fragment.plan.plan, fragment.plan.imageSources, true)
		if preflightErr != nil {
			return true, fmt.Errorf("document: preflight HTML fragment plan: %w", preflightErr)
		}
		pageOffset := fragment.start.page
		if fragment.reuseCurrentPage {
			pageOffset--
		}
		if paintErr := html.pdf.paintPreparedDisplayLayoutPlanPDFAtCurrentPage(prepared, fragment.reuseCurrentPage, pageOffset, true); paintErr != nil {
			return true, fmt.Errorf("document: paint HTML fragment plan: %w", paintErr)
		}
	} else {
		prepared, preflightErr := html.pdf.preflightCoreLayoutPlanPDFContextForTarget(ctx, fragment.plan.plan, true)
		if preflightErr != nil {
			return true, fmt.Errorf("document: preflight HTML fragment plan: %w", preflightErr)
		}
		if paintErr := html.pdf.paintPreparedCoreLayoutPlanPDFAtCurrentPage(prepared, fragment.reuseCurrentPage, true); paintErr != nil {
			return true, fmt.Errorf("document: paint HTML fragment plan: %w", paintErr)
		}
	}
	if fragment.appendTrailingPage {
		html.pdf.AddPageFormat(fragment.start.orientation, fragment.start.pageSize)
		if html.pdf.Error() != nil {
			return true, html.pdf.Error()
		}
	}
	if html.pdf.PageNo() != fragment.final.page {
		return true, fmt.Errorf("document: HTML fragment final page %d does not match planned page %d", html.pdf.PageNo(), fragment.final.page)
	}
	style := fragment.start.fontStyle
	if fragment.start.underline {
		style += "U"
	}
	if fragment.start.strikeout {
		style += "S"
	}
	html.pdf.SetFont(fragment.start.fontFamily, style, fragment.start.fontSizePoints)
	if html.pdf.Error() != nil {
		return true, html.pdf.Error()
	}
	html.pdf.SetXY(fragment.final.x, fragment.final.y)
	// Raw plan painting deliberately leaves the selected font context intact.
	// Assert the captured compatibility state instead of silently changing it.
	if !strings.EqualFold(html.pdf.fontFamily, fragment.start.fontFamily) || html.pdf.fontStyle != fragment.start.fontStyle ||
		html.pdf.fontSizePt != fragment.start.fontSizePoints || html.pdf.underline != fragment.start.underline || html.pdf.strikeout != fragment.start.strikeout {
		return true, errors.New("document: HTML fragment painter changed the captured font context")
	}
	return true, html.pdf.Error()
}

func htmlUnifiedFallbackCategory(compiled *CompiledHTML, err error) string {
	if compiled != nil && len(compiled.recovery) != 0 {
		return "malformed-recovery"
	}
	detail := ""
	if err != nil {
		detail = err.Error()
	}
	switch {
	case strings.Contains(detail, "<frame>"):
		return "frame-contract"
	case compiled != nil && len(compiled.inlineSVGs) != 0:
		return "svg-contract"
	case compiled != nil && len(compiled.cssRules) != 0:
		return "stylesheet-contract"
	case compiledHTMLContainsTable(compiled):
		return "table-contract"
	default:
		return "unsupported-layout-contract"
	}
}

func compiledHTMLContainsTable(compiled *CompiledHTML) bool {
	if compiled == nil {
		return false
	}
	for _, token := range compiled.tokens {
		if token.Cat == 'O' && token.Str == "table" {
			return true
		}
	}
	return false
}

func compiledHTMLContainsImageInTable(compiled *CompiledHTML) bool {
	if compiled == nil {
		return false
	}
	depth := 0
	for _, token := range compiled.tokens {
		switch {
		case token.Cat == 'O' && token.Str == "table":
			depth++
		case token.Cat == 'C' && token.Str == "table":
			if depth > 0 {
				depth--
			}
		case token.Cat == 'O' && token.Str == "img" && depth > 0:
			return true
		}
	}
	return false
}
