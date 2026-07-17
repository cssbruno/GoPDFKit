// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

var errTypedShadowUnsupported = errors.New("document: typed pagination shadow unsupported")

type typedShadowUnsupportedReason string

const (
	typedShadowDocumentState     typedShadowUnsupportedReason = "document_state"
	typedShadowDocumentPolicy    typedShadowUnsupportedReason = "document_policy"
	typedShadowPageTemplate      typedShadowUnsupportedReason = "page_template"
	typedShadowDocumentEnvelope  typedShadowUnsupportedReason = "document_envelope"
	typedShadowBlockKind         typedShadowUnsupportedReason = "block_kind"
	typedShadowParagraphContract typedShadowUnsupportedReason = "paragraph_contract"
	typedShadowFont              typedShadowUnsupportedReason = "font"
	typedShadowGeometry          typedShadowUnsupportedReason = "geometry"
)

type typedShadowUnsupportedError struct {
	Reason typedShadowUnsupportedReason
	Detail string
}

func (e *typedShadowUnsupportedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Detail == "" {
		return fmt.Sprintf("%v: %s", errTypedShadowUnsupported, e.Reason)
	}
	return fmt.Sprintf("%v: %s: %s", errTypedShadowUnsupported, e.Reason, e.Detail)
}

func (e *typedShadowUnsupportedError) Unwrap() error { return errTypedShadowUnsupported }

func newTypedShadowUnsupported(reason typedShadowUnsupportedReason, detail string) error {
	return &typedShadowUnsupportedError{Reason: reason, Detail: detail}
}

// typedShadowResult is an observational allocation plan. BlockIndices maps
// plan fragment order back to LayoutDocument.Body indexes. The generated node
// identities are revision-local comparison handles, not durable editing IDs.
type typedShadowResult struct {
	Plan         layoutengine.LayoutPlan
	BlockIndices []int
}

// planTypedDocumentPaginationShadow lowers a deliberately narrow typed-layout
// subset into the new private fixed-point planner. It never paints, adds a
// page, changes the live cursor, loads a live resource, or participates in
// WriteDocument output.
//
// The initial subset is a fresh, uniform page flow of plain, atomic paragraph
// blocks. The resulting fragments represent measured allocation footprints,
// not exact glyph, border, or painter geometry.
func (f *Document) planTypedDocumentPaginationShadow(doc *layout.LayoutDocument) (typedShadowResult, error) {
	if f == nil || f.err != nil || f.page != 0 || f.state != documentStateUnopened ||
		f.clipNest != 0 || f.transformNest != 0 {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentState, "requires a fresh error-free document")
	}
	if doc == nil {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "layout document is nil")
	}
	if !f.autoPageBreak || f.acceptPageBreakSet || f.headerFnc != nil ||
		f.footerFnc != nil || f.footerFncLpi != nil || f.pageAddGuard != nil {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentPolicy, "custom page lifecycle behavior is present")
	}
	if !typedShadowTemplateHasOnlyMargins(doc.PageTemplate) {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowPageTemplate, "only uniform margins are supported")
	}
	if doc.Signature != nil || doc.QR != nil || len(doc.Attachments) != 0 {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowDocumentEnvelope, "signature, QR, or attachments are present")
	}
	if !typedShadowCoreFont(f.coreFonts, f.fontFamily) {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowFont, "current font is not a core font")
	}

	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	contentWidth := f.w - left - right
	bodyHeight := f.h - top - bottom
	if contentWidth <= 0 || bodyHeight <= 0 {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, "page margins leave no body area")
	}
	pageSize, body, err := typedShadowFixedGeometry(f, left, top, contentWidth, bodyHeight)
	if err != nil {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, err.Error())
	}

	scratch := documentNew("P", f.unitStr, "", f.fontDirStr, Size{Wd: f.w, Ht: f.h})
	if scratch.err != nil {
		return typedShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, scratch.err.Error())
	}
	scratch.cMargin = f.cMargin
	scratch.ws = f.ws
	scratch.fontFamily = f.fontFamily
	scratch.fontStyle = f.fontStyle
	scratch.fontSizePt = f.fontSizePt
	scratch.fontSize = f.fontSizePt / scratch.k
	measureContext := newMeasureContext(scratch, contentWidth)

	flowBlocks := make([]layoutengine.VerticalFlowBlock, 0, len(doc.Body))
	blockIndices := make([]int, 0, len(doc.Body))
	for bodyIndex, candidate := range doc.Body {
		block, ok := layout.NormalizeBlock(candidate)
		if !ok {
			continue
		}
		paragraph, ok := block.(layout.ParagraphBlock)
		if !ok {
			return typedShadowResult{}, newTypedShadowUnsupported(typedShadowBlockKind, fmt.Sprintf("body[%d] is %s", bodyIndex, block.DocumentBlockKind()))
		}
		if detail := typedShadowParagraphUnsupported(paragraph, f.coreFonts); detail != "" {
			return typedShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, fmt.Sprintf("body[%d]: %s", bodyIndex, detail))
		}

		measurement := layout.MeasureBlock(measureContext, paragraph)
		if scratch.err != nil {
			return typedShadowResult{}, newTypedShadowUnsupported(typedShadowFont, scratch.err.Error())
		}
		height, err := fixedFromDocumentUnits(f, measurement.Height)
		if err != nil || height <= 0 {
			return typedShadowResult{}, newTypedShadowUnsupported(typedShadowGeometry, fmt.Sprintf("body[%d] has invalid measured height", bodyIndex))
		}
		if height > body.Height {
			return typedShadowResult{}, newTypedShadowUnsupported(typedShadowParagraphContract, fmt.Sprintf("body[%d] is taller than an empty body page", bodyIndex))
		}
		identity := fmt.Sprintf("@typed-shadow-%d", bodyIndex+1)
		flowBlocks = append(flowBlocks, layoutengine.VerticalFlowBlock{
			Node:     layoutengine.NodeID(bodyIndex + 1),
			Key:      layoutengine.NodeKey(identity),
			Instance: layoutengine.InstanceID(identity),
			Height:   height,
		})
		blockIndices = append(blockIndices, bodyIndex)
	}

	plan, err := layoutengine.PlanVerticalFlow(layoutengine.VerticalFlowInput{
		PageSize: pageSize,
		Body:     body,
		Blocks:   flowBlocks,
	})
	if err != nil {
		return typedShadowResult{}, fmt.Errorf("document: typed pagination shadow: %w", err)
	}
	return typedShadowResult{Plan: plan, BlockIndices: blockIndices}, nil
}

func typedShadowTemplateHasOnlyMargins(template layout.PageTemplate) bool {
	return template.Header == nil && template.Footer == nil &&
		template.FirstPageHeader == nil && template.FirstPageFooter == nil &&
		template.EvenPageFooter == nil && template.PageNumbers == (layout.PageNumberOptions{}) &&
		template.ReserveFooterHeight == 0 && template.EvenPageFooterHeight == 0
}

func typedShadowMargins(f *Document, override layout.Spacing) (left, top, right, bottom float64) {
	left, top, right, bottom = f.GetMargins()
	if override.Left > 0 {
		left = override.Left
	}
	if override.Top > 0 {
		top = override.Top
	}
	if override.Right > 0 {
		right = override.Right
	}
	if override.Bottom > 0 {
		bottom = override.Bottom
	}
	return left, top, right, bottom
}

func typedShadowFixedGeometry(f *Document, left, top, width, height float64) (layoutengine.Size, layoutengine.Rect, error) {
	toFixed := func(userUnits float64) (layoutengine.Fixed, error) {
		return fixedFromDocumentUnits(f, userUnits)
	}
	pageWidth, err := toFixed(f.w)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	pageHeight, err := toFixed(f.h)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	bodyX, err := toFixed(left)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	bodyY, err := toFixed(top)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	bodyWidth, err := toFixed(width)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	bodyHeight, err := toFixed(height)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	pageSize, err := layoutengine.NewSize(pageWidth, pageHeight)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	body, err := layoutengine.NewRect(bodyX, bodyY, bodyWidth, bodyHeight)
	if err != nil {
		return layoutengine.Size{}, layoutengine.Rect{}, err
	}
	return pageSize, body, nil
}

func typedShadowParagraphUnsupported(block layout.ParagraphBlock, coreFonts map[string]bool) string {
	if block.BoxRef != nil || block.Box != (layout.BoxStyle{KeepTogether: true}) {
		return "paragraph must be a plain keep-together box"
	}
	if block.StyleRef != nil || !typedShadowCoreFont(coreFonts, block.Style.FontFamily) {
		return "paragraph style reference or non-core font is unsupported"
	}
	for _, segment := range block.Segments {
		if segment.StyleRef != nil || segment.Style != (layout.TextStyle{}) || segment.Link != "" {
			return "segment styles, style references, and links are unsupported"
		}
	}
	plainText := layout.TextSegmentsPlainText(block.Segments)
	if strings.ContainsRune(plainText, '\r') || strings.HasSuffix(plainText, "\n\n") {
		return "carriage returns and multiple trailing newlines have uncharacterized legacy measurement"
	}
	if !isPlannerCoreText(plainText) {
		return "only printable ASCII and line feeds have characterized core-font measurement"
	}
	return ""
}

func typedShadowCoreFont(coreFonts map[string]bool, family string) bool {
	if strings.TrimSpace(family) == "" {
		return true
	}
	family = strings.ToLower(fontFamilyEscape(family))
	if family == "arial" {
		family = "helvetica"
	}
	return coreFonts[family]
}
