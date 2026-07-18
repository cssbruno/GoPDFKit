// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"strings"

	"github.com/cssbruno/gopdfkit/layout"
)

// LongFormHTMLDocumentModel converts supported long-form HTML into a shared
// document model with extracted footer configuration.
func LongFormHTMLDocumentModel(title, htmlStr string) (*layout.LayoutDocument, []string) {
	bodyHTML, footer := ExtractHTMLFooterBlock(htmlStr)
	pdf := MustNew()
	html := pdf.HTMLNew()
	messages := html.ValidateHTML(bodyHTML)
	doc := layout.NewLayoutDocument()
	doc.Title = strings.TrimSpace(title)
	if doc.Title != "" {
		doc.Body = append(doc.Body, layout.HeadingBlock{Level: 1, Segments: []layout.TextSegment{{Text: doc.Title}}})
	}
	compiled, err := CompileHTML(bodyHTML)
	if err != nil {
		messages = append(messages, err.Error())
	} else {
		const defaultLineHeight = 5.0
		resolved, resolveErr := pdf.resolveCompiledHTMLUnifiedSnapshot(context.Background(), compiled, defaultLineHeight)
		if resolveErr != nil {
			messages = append(messages, resolveErr.Error())
		} else {
			availableWidth := pdf.w - pdf.lMargin - pdf.rMargin
			lowered, lowerErr := lowerCompiledHTMLTextCohortUnitsWidth(context.Background(), resolved, defaultLineHeight, pdf.PointConvert, availableWidth)
			if lowerErr != nil {
				messages = append(messages, lowerErr.Error())
			} else {
				doc.Body = append(doc.Body, lowered.Body...)
			}
		}
	}
	doc.PageTemplate.Footer = footer
	return doc, messages
}
