// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
)

// layoutPlanHasMultipleGlyphRunsPerLine identifies plans whose finalized
// display list switches core fonts or visual styles within a line. The compact
// core painter intentionally has a one-run-per-line contract; these plans are
// still fully positioned and simply use the general display-list painter.
func layoutPlanHasMultipleGlyphRunsPerLine(projection layoutengine.LayoutPlanProjection) bool {
	counts := make(map[uint32]uint32)
	for _, run := range projection.GlyphRuns {
		counts[run.Line]++
		if counts[run.Line] > 1 {
			return true
		}
	}
	return false
}

var errCoreLayoutPlanPaintUnsupported = errors.New("document: core layout plan paint unsupported")

type preparedCorePlanFont struct {
	resource layoutengine.CoreFontResource
	key      string
	font     fontDefinition
}

type preparedCorePlanPage struct {
	page   layoutengine.PlannedPage
	events []layoutengine.CorePaintEvent
}

type preparedCorePlanPDF struct {
	fonts     map[layoutengine.FontResourceID]preparedCorePlanFont
	fontOrder []layoutengine.FontResourceID
	pages     []preparedCorePlanPage
}

// paintCoreLayoutPlanPDF is the initial production no-layout painter bridge.
// It accepts only a fresh Document and a paint-ready core-font LayoutPlan.
// Complete plan, policy, page, and font preflight happens before the Document
// is opened, a resource is installed, or a page is added.
func (f *Document) paintCoreLayoutPlanPDF(plan layoutengine.LayoutPlan) error {
	prepared, err := f.preflightCoreLayoutPlanPDF(plan)
	if err != nil {
		return err
	}
	return f.paintPreparedCoreLayoutPlanPDF(prepared)
}

func (f *Document) paintPreparedCoreLayoutPlanPDF(prepared preparedCorePlanPDF) error {
	return f.paintPreparedCoreLayoutPlanPDFAtCurrentPage(prepared, false, false)
}

// paintPreparedCoreLayoutPlanPDFAtCurrentPage replays a preflighted fragment
// into the active page when reuseCurrent is true. The fragment planner has
// already positioned its first page in the captured body region, so this sink
// performs no fitting or cursor-dependent layout.
func (f *Document) paintPreparedCoreLayoutPlanPDFAtCurrentPage(prepared preparedCorePlanPDF, reuseCurrent, compactNativeRuns bool) error {
	for _, id := range prepared.fontOrder {
		font := prepared.fonts[id]
		f.ensureResourceStore().setFont(font.key, font.font)
	}
	for index, plannedPage := range prepared.pages {
		size := Size{
			Wd: f.PointConvert(plannedPage.page.Size.Width.Points()),
			Ht: f.PointConvert(plannedPage.page.Size.Height.Points()),
		}
		if !reuseCurrent || index != 0 {
			f.AddPageFormat("P", size)
			if f.err != nil {
				return f.err
			}
		}
		var previousRun layoutengine.CoreGlyphRun
		previousRunSet := false
		for _, event := range plannedPage.events {
			font := prepared.fonts[event.Run.Font]
			if compactNativeRuns {
				run := event.Run
				if !run.LeadingSpace && previousRunSet {
					run.LeadingSpace = previousRun.TrailingSpace
				}
				f.outbytes(appendPlannedCoreGlyphRunExactTJ(nil, font.font, plannedPage.page.Size.Height, run))
			} else {
				f.outbytes(appendPlannedCoreGlyphRun(nil, font.font, plannedPage.page.Size.Height, event.Run))
			}
			previousRun, previousRunSet = event.Run, true
		}
	}
	return f.err
}

func (f *Document) preflightCoreLayoutPlanPDF(plan layoutengine.LayoutPlan) (preparedCorePlanPDF, error) {
	return f.preflightCoreLayoutPlanPDFContext(context.Background(), plan)
}

func (f *Document) preflightCoreLayoutPlanPDFContext(ctx context.Context, plan layoutengine.LayoutPlan) (preparedCorePlanPDF, error) {
	return f.preflightCoreLayoutPlanPDFContextForTarget(ctx, plan, false)
}

func (f *Document) preflightCoreLayoutPlanPDFContextForTarget(ctx context.Context, plan layoutengine.LayoutPlan, allowActivePage bool) (preparedCorePlanPDF, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return preparedCorePlanPDF{}, err
	}
	if f == nil || f.err != nil || (!allowActivePage && (f.page != 0 || f.state != documentStateUnopened)) ||
		(allowActivePage && (f.page <= 0 || f.state != documentStatePageOpen)) ||
		f.k <= 0 || !isFiniteFloat(f.k) || f.clipNest != 0 || f.transformNest != 0 {
		return preparedCorePlanPDF{}, fmt.Errorf("%w: requires a fresh error-free document", errCoreLayoutPlanPaintUnsupported)
	}
	if f.headerFnc != nil || f.footerFnc != nil || f.footerFncLpi != nil || f.pageAddGuard != nil {
		return preparedCorePlanPDF{}, fmt.Errorf("%w: custom page lifecycle behavior is present", errCoreLayoutPlanPaintUnsupported)
	}
	if len(f.aliasMap) != 0 || f.aliasNbPagesStr != "" {
		return preparedCorePlanPDF{}, fmt.Errorf("%w: deferred text aliases are present", errCoreLayoutPlanPaintUnsupported)
	}

	recording, err := layoutengine.RecordCorePlan(plan)
	if err != nil {
		return preparedCorePlanPDF{}, fmt.Errorf("document: preflight core layout plan: %w", err)
	}
	events := recording.Events()
	pageCount := 0
	for _, event := range events {
		if event.Kind == layoutengine.PaintPageBegin {
			pageCount++
		}
	}
	if f.limits.MaxPages > 0 && pageCount > f.limits.MaxPages {
		return preparedCorePlanPDF{}, fmt.Errorf("%w: %d > %d", ErrPageLimitExceeded, pageCount, f.limits.MaxPages)
	}

	resources := plan.Projection().Fonts
	prepared := preparedCorePlanPDF{
		fonts:     make(map[layoutengine.FontResourceID]preparedCorePlanFont),
		fontOrder: make([]layoutengine.FontResourceID, 0, len(resources)),
		pages:     make([]preparedCorePlanPage, 0, pageCount),
	}
	for index, resource := range resources {
		if index&31 == 0 {
			if err := ctx.Err(); err != nil {
				return preparedCorePlanPDF{}, err
			}
		}
		font, err := f.preflightCorePlanFontContext(ctx, resource)
		if err != nil {
			return preparedCorePlanPDF{}, err
		}
		prepared.fonts[resource.ID] = font
		prepared.fontOrder = append(prepared.fontOrder, resource.ID)
	}
	var active *preparedCorePlanPage
	for index, event := range events {
		if index&255 == 0 {
			if err := ctx.Err(); err != nil {
				return preparedCorePlanPDF{}, err
			}
		}
		switch event.Kind {
		case layoutengine.PaintPageBegin:
			prepared.pages = append(prepared.pages, preparedCorePlanPage{page: layoutengine.PlannedPage{
				Number: event.Page, Size: event.Size,
			}})
			active = &prepared.pages[len(prepared.pages)-1]
		case layoutengine.PaintGlyphRun:
			if active == nil || active.page.Number != event.Page {
				return preparedCorePlanPDF{}, errors.New("document: core layout plan recording has a glyph outside a page")
			}
			font, ok := prepared.fonts[event.Font.ID]
			if !ok || font.resource != event.Font {
				return preparedCorePlanPDF{}, errors.New("document: core layout plan recording references an unprepared font")
			}
			active.events = append(active.events, event)
		case layoutengine.PaintPageEnd:
			if active == nil || active.page.Number != event.Page {
				return preparedCorePlanPDF{}, errors.New("document: core layout plan recording has mismatched page boundaries")
			}
			active = nil
		default:
			return preparedCorePlanPDF{}, errors.New("document: core layout plan recording contains an unsupported event")
		}
	}
	if active != nil {
		return preparedCorePlanPDF{}, errors.New("document: core layout plan recording ends inside a page")
	}
	return prepared, nil
}

func (f *Document) preflightCorePlanFont(resource layoutengine.CoreFontResource) (preparedCorePlanFont, error) {
	return f.preflightCorePlanFontContext(context.Background(), resource)
}

func (f *Document) preflightCorePlanFontContext(ctx context.Context, resource layoutengine.CoreFontResource) (preparedCorePlanFont, error) {
	return f.preflightPlanFontContext(ctx, resource, nil)
}

func (f *Document) preflightPlanFontContext(ctx context.Context, resource layoutengine.CoreFontResource, sources plannedFontSources) (preparedCorePlanFont, error) {
	if err := ctx.Err(); err != nil {
		return preparedCorePlanFont{}, err
	}
	if resource.EmbeddedUTF8 != nil {
		data := sources[resource.EmbeddedUTF8.Digest]
		if len(data) == 0 || uint32(len(data)) != resource.EmbeddedUTF8.ByteLength {
			return preparedCorePlanFont{}, fmt.Errorf("%w: embedded UTF-8 font bytes are unavailable or over budget", errCoreLayoutPlanPaintUnsupported)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != string(resource.EmbeddedUTF8.Digest) {
			return preparedCorePlanFont{}, fmt.Errorf("%w: embedded UTF-8 font digest mismatch", errCoreLayoutPlanPaintUnsupported)
		}
		font, err := utf8FontDefinition(resource.EmbeddedUTF8.Name, "", data)
		if err != nil {
			return preparedCorePlanFont{}, fmt.Errorf("%w: parse embedded UTF-8 font: %v", errCoreLayoutPlanPaintUnsupported, err)
		}
		canonical, _, err := typedEmbeddedUTF8FontResource(font)
		if err != nil || canonical.MetricsDigest != resource.MetricsDigest || canonical.EmbeddedUTF8.Digest != resource.EmbeddedUTF8.Digest {
			return preparedCorePlanFont{}, fmt.Errorf("%w: embedded UTF-8 font metrics do not match the plan", errCoreLayoutPlanPaintUnsupported)
		}
		font.usedRunes = defaultUTF8UsedRunes("")
		return preparedCorePlanFont{resource: resource, key: resource.EmbeddedUTF8.Name, font: font}, nil
	}
	key, ok := coreFontKeyForPlanFace(resource.Face)
	if !ok {
		return preparedCorePlanFont{}, fmt.Errorf("%w: unsupported core font face %q", errCoreLayoutPlanPaintUnsupported, resource.Face)
	}
	font, err := loadCoreFontDef(key)
	if err != nil {
		return preparedCorePlanFont{}, fmt.Errorf("document: load planned core font %q: %w", key, err)
	}
	if err := ctx.Err(); err != nil {
		return preparedCorePlanFont{}, err
	}
	canonical, err := typedCoreFontResource(font)
	if err != nil {
		return preparedCorePlanFont{}, fmt.Errorf("document: validate planned core font %q: %w", key, err)
	}
	if canonical.Face != resource.Face || canonical.MetricsDigest != resource.MetricsDigest {
		return preparedCorePlanFont{}, fmt.Errorf("%w: core font metrics do not match face %q", errCoreLayoutPlanPaintUnsupported, resource.Face)
	}
	if f.resources != nil {
		if existing, exists := f.resources.font(key); exists {
			existingResource, existingErr := typedCoreFontResource(existing)
			if existingErr != nil || existingResource.Face != resource.Face ||
				existingResource.MetricsDigest != resource.MetricsDigest {
				return preparedCorePlanFont{}, fmt.Errorf("%w: document font %q shadows the planned core font", errCoreLayoutPlanPaintUnsupported, key)
			}
			font = existing
		}
	}
	return preparedCorePlanFont{resource: resource, key: key, font: font}, nil
}

func coreFontKeyForPlanFace(face layoutengine.CoreFontFace) (string, bool) {
	switch face {
	case layoutengine.CoreFontCourier:
		return "courier", true
	case layoutengine.CoreFontCourierBold:
		return "courierB", true
	case layoutengine.CoreFontCourierOblique:
		return "courierI", true
	case layoutengine.CoreFontCourierBoldOblique:
		return "courierBI", true
	case layoutengine.CoreFontHelvetica:
		return "helvetica", true
	case layoutengine.CoreFontHelveticaBold:
		return "helveticaB", true
	case layoutengine.CoreFontHelveticaOblique:
		return "helveticaI", true
	case layoutengine.CoreFontHelveticaBoldOblique:
		return "helveticaBI", true
	case layoutengine.CoreFontTimesRoman:
		return "times", true
	case layoutengine.CoreFontTimesBold:
		return "timesB", true
	case layoutengine.CoreFontTimesItalic:
		return "timesI", true
	case layoutengine.CoreFontTimesBoldItalic:
		return "timesBI", true
	case layoutengine.CoreFontSymbol:
		return "symbol", true
	case layoutengine.CoreFontZapfDingbats:
		return "zapfdingbats", true
	default:
		return "", false
	}
}

// appendPlannedCoreGlyphRun emits one absolute text matrix per planned code.
// Native font advances therefore cannot reshape, rewrap, align, or paginate
// the run; every following code starts at the cumulative planned advance.
func appendPlannedCoreGlyphRun(dst []byte, font fontDefinition, pageHeight layoutengine.Fixed, run layoutengine.CoreGlyphRun) []byte {
	// Emit black for an unset run as well: PDF non-stroking color is persistent
	// graphics state, so omitting it could leak a preceding themed run's color.
	dst = appendPDFNumberSpace(dst, float64(run.Color.R)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.G)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.B)/255, 10)
	dst = append(dst, "rg "...)
	dst = append(dst, "BT /F"...)
	dst = append(dst, font.i...)
	dst = append(dst, ' ')
	dst = appendPDFNumberSpace(dst, run.FontSize.Points(), 10)
	dst = append(dst, "Tf "...)
	x := run.Origin.X
	for index, code := range []byte(run.Codes) {
		dst = append(dst, "1 0 0 1 "...)
		dst = appendPDFNumberSpace(dst, x.Points(), 10)
		dst = appendPDFNumberSpace(dst, pageHeight.Points()-run.Origin.Y.Points(), 10)
		dst = append(dst, "Tm ("...)
		dst = appendEscapedPDFLiteralByte(dst, code)
		dst = append(dst, ") Tj "...)
		x += run.Advances[index]
	}
	return append(dst, "ET"...)
}

func appendPlannedUTF8GlyphRun(dst []byte, font fontDefinition, pageHeight layoutengine.Fixed, run layoutengine.CoreGlyphRun) []byte {
	dst = appendPDFNumberSpace(dst, float64(run.Color.R)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.G)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.B)/255, 10)
	dst = append(dst, "rg BT /F"...)
	dst = append(dst, font.i...)
	dst = append(dst, ' ')
	dst = appendPDFNumberSpace(dst, run.FontSize.Points(), 10)
	dst = append(dst, "Tf "...)
	x := run.Origin.X
	advanceIndex := 0
	for _, character := range run.Codes {
		dst = append(dst, "1 0 0 1 "...)
		dst = appendPDFNumberSpace(dst, x.Points(), 10)
		dst = appendPDFNumberSpace(dst, pageHeight.Points()-run.Origin.Y.Points(), 10)
		dst = append(dst, "Tm ("...)
		dst = appendEscapedUTF16BECodeUnit(dst, character)
		dst = append(dst, ") Tj "...)
		font.usedRunes[int(character)] = int(character)
		x += run.Advances[advanceIndex]
		advanceIndex++
	}
	return append(dst, "ET"...)
}

func appendPlannedUTF8GlyphRunActualText(dst []byte, font fontDefinition, pageHeight layoutengine.Fixed, run layoutengine.CoreGlyphRun) []byte {
	dst = append(dst, "/Span << /ActualText (\376\377"...)
	if run.LeadingSpace {
		dst = appendEscapedUTF16BECodeUnit(dst, ' ')
	}
	for _, character := range run.Codes {
		dst = appendEscapedUTF16BECodeUnit(dst, character)
	}
	dst = append(dst, ") >> BDC "...)
	dst = appendPlannedUTF8GlyphRun(dst, font, pageHeight, run)
	return append(dst, " EMC"...)
}

func appendPlannedCoreGlyphRunCompact(dst []byte, font fontDefinition, pageHeight layoutengine.Fixed, run layoutengine.CoreGlyphRun) []byte {
	if !plannedCoreRunUsesNativeAdvances(font, run) {
		return appendPlannedCoreGlyphRun(dst, font, pageHeight, run)
	}
	dst = appendPDFNumberSpace(dst, float64(run.Color.R)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.G)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.B)/255, 10)
	dst = append(dst, "rg BT /F"...)
	dst = append(dst, font.i...)
	dst = append(dst, ' ')
	dst = appendPDFNumberSpace(dst, run.FontSize.Points(), 10)
	dst = append(dst, "Tf 1 0 0 1 "...)
	dst = appendPDFNumberSpace(dst, run.Origin.X.Points(), 10)
	dst = appendPDFNumberSpace(dst, pageHeight.Points()-run.Origin.Y.Points(), 10)
	dst = append(dst, "Tm ("...)
	for _, code := range []byte(run.Codes) {
		dst = appendEscapedPDFLiteralByte(dst, code)
	}
	return append(dst, ") Tj ET"...)
}

// appendPlannedCoreGlyphRunExactTJ retains one authored PDF text sequence while
// correcting each native font advance to the immutable planned advance. This
// preserves spaces for extraction without surrendering planned positioning.
func appendPlannedCoreGlyphRunExactTJ(dst []byte, font fontDefinition, pageHeight layoutengine.Fixed, run layoutengine.CoreGlyphRun) []byte {
	if len(run.Codes) == 0 || len(run.Codes) != len(run.Advances) || len(font.Cw) < 256 || run.FontSize <= 0 {
		return appendPlannedCoreGlyphRun(dst, font, pageHeight, run)
	}
	dst = append(dst, "/Span << /ActualText ("...)
	if run.LeadingSpace {
		dst = append(dst, ' ')
	}
	for _, code := range []byte(run.Codes) {
		dst = appendEscapedPDFLiteralByte(dst, code)
	}
	dst = append(dst, ") >> BDC "...)
	dst = appendPDFNumberSpace(dst, float64(run.Color.R)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.G)/255, 10)
	dst = appendPDFNumberSpace(dst, float64(run.Color.B)/255, 10)
	dst = append(dst, "rg BT /F"...)
	dst = append(dst, font.i...)
	dst = append(dst, ' ')
	dst = appendPDFNumberSpace(dst, run.FontSize.Points(), 10)
	dst = append(dst, "Tf 1 0 0 1 "...)
	dst = appendPDFNumberSpace(dst, run.Origin.X.Points(), 10)
	dst = appendPDFNumberSpace(dst, pageHeight.Points()-run.Origin.Y.Points(), 10)
	dst = append(dst, "Tm ["...)
	for index, code := range []byte(run.Codes) {
		dst = append(dst, '(')
		dst = appendEscapedPDFLiteralByte(dst, code)
		dst = append(dst, ')')
		if index+1 == len(run.Advances) {
			continue
		}
		native := float64(font.Cw[int(code)]) * run.FontSize.Points() / 1000
		adjustment := (native - run.Advances[index].Points()) * 1000 / run.FontSize.Points()
		dst = append(dst, ' ')
		dst = appendPDFNumber(dst, adjustment, 10)
		dst = append(dst, ' ')
	}
	return append(dst, "] TJ ET EMC"...)
}

func plannedCoreRunUsesNativeAdvances(font fontDefinition, run layoutengine.CoreGlyphRun) bool {
	codes := []byte(run.Codes)
	if len(codes) == 0 || len(codes) != len(run.Advances) || len(font.Cw) < 256 {
		return false
	}
	var planned layoutengine.Fixed
	var nativeWidth int
	for index, code := range codes {
		planned += run.Advances[index]
		nativeWidth += font.Cw[int(code)]
		expected, err := layoutengine.FixedFromPoints(float64(nativeWidth) * run.FontSize.Points() / 1000)
		if err != nil || expected != planned {
			return false
		}
	}
	return true
}
