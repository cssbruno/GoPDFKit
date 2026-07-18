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

// errTypedShadowUnsupported is retained as an internal capability diagnostic
// for the lowering adapter. It is not a renderer fallback signal: callers now
// receive the error and no automatic legacy engine is invoked.
var errTypedShadowUnsupported = errors.New("document: typed layout contract unsupported")

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
