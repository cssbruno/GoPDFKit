// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/cssbruno/paperrune/layout"
)

// WriteDocument lowers a shared document model to one immutable plan and
// paints that plan. Automatic layout no longer retries through a second
// renderer: unsupported receiver state or model contracts are reported on the
// receiver before any page bytes are opened.
func (f *Document) WriteDocument(doc *layout.LayoutDocument) {
	if f == nil || f.err != nil {
		return
	}
	if doc == nil {
		f.SetErrorf("document is nil")
		return
	}
	if !f.typedWriteDocumentFresh() {
		f.SetError(newTypedShadowUnsupported(typedShadowDocumentState,
			"WriteDocument requires a fresh receiver after legacy automatic layout deletion"))
		return
	}
	if len(layout.NormalizeBlocks(doc.Body)) == 0 && doc.QR == nil && (doc.Signature == nil || len(doc.Signature.Rows) == 0) {
		envelope, err := f.snapshotLayoutDocumentEnvelope(doc)
		if err != nil {
			f.SetError(err)
			return
		}
		f.installLayoutDocumentEnvelope(envelope)
		f.applyPageTemplateMargins(doc.PageTemplate)
		f.installTypedWriteDocumentCompatibilityAliases(doc)
		f.observeLayoutEngineRoute("WriteDocument", "unified", "")
		return
	}
	plan, err := f.PlanLayoutDocument(doc)
	if err != nil {
		f.SetError(err)
		return
	}
	if _, err := f.WriteLayoutDocumentPlan(plan); err != nil {
		f.SetError(err)
		return
	}
	// Preserve the documented post-call margin state even though the immutable
	// plan already consumed these values in points.
	f.applyPageTemplateMargins(doc.PageTemplate)
	f.installTypedWriteDocumentCompatibilityAliases(doc)
	f.observeLayoutEngineRoute("WriteDocument", "unified", "")
}

func (f *Document) typedWriteDocumentFresh() bool {
	return f != nil && f.err == nil && f.page == 0 && f.state == documentStateUnopened &&
		f.clipNest == 0 && f.transformNest == 0
}

func (f *Document) observeLayoutEngineRoute(entryPoint, engine, reason string) {
	if f != nil && f.hooks.OnLayoutEngineRoute != nil {
		f.hooks.OnLayoutEngineRoute(entryPoint, engine, reason)
	}
}

// installTypedWriteDocumentCompatibilityAliases keeps the historical QR image
// resource aliases available to callers while the actual image is planned and
// painted by the unified display-list path.
func (f *Document) installTypedWriteDocumentCompatibilityAliases(doc *layout.LayoutDocument) {
	if doc == nil {
		return
	}
	install := func(qr layout.QRBlock) {
		payload := strings.TrimSpace(qr.URL)
		if payload == "" {
			payload = strings.TrimSpace(qr.Value)
		}
		if payload == "" {
			return
		}
		data, err := QRCodePNG(payload, defaultQRCodeSizePx)
		if err != nil {
			return
		}
		digest := sha256.Sum256(data)
		f.ensureResourceStore().setImageAlias(QRCodeImageName(payload), "plan-image-"+hex.EncodeToString(digest[:]))
	}
	if doc.QR != nil {
		install(*doc.QR)
	}
	var visit func([]layout.Block)
	visit = func(blocks []layout.Block) {
		for _, raw := range blocks {
			block, ok := layout.NormalizeBlock(raw)
			if !ok {
				continue
			}
			switch value := block.(type) {
			case layout.QRVerificationBlock:
				install(value.QR)
			case layout.SectionBlock:
				visit(value.Blocks)
			case layout.ClauseBlock:
				visit(value.Blocks)
			case layout.NoteBoxBlock:
				visit(value.Body)
			case layout.ListBlock:
				for _, item := range value.Items {
					visit(item.Blocks)
				}
			case layout.TableBlock:
				for _, group := range [][]layout.TableRow{value.Header, value.Body, value.Footer} {
					for _, row := range group {
						for _, cell := range row.Cells {
							visit(cell.Blocks)
						}
					}
				}
			case layout.RowColumnBlock:
				for _, item := range value.Items {
					visit([]layout.Block{item.Block})
				}
			}
		}
	}
	visit(doc.Body)
	for _, header := range []*layout.HeaderBlock{doc.PageTemplate.Header, doc.PageTemplate.FirstPageHeader} {
		if header != nil {
			visit(header.Blocks)
		}
	}
	for _, footer := range []*layout.FooterBlock{doc.PageTemplate.Footer, doc.PageTemplate.FirstPageFooter, doc.PageTemplate.EvenPageFooter} {
		if footer != nil {
			visit(footer.Blocks)
		}
	}
}

func textAlign(align string) string {
	switch strings.ToUpper(align) {
	case "C", "CENTER":
		return "C"
	case "R", "RIGHT":
		return "R"
	default:
		return "L"
	}
}

func signatureColumnText(col layout.SignatureColumn) string {
	lines := make([]string, 0, 3+len(col.Metadata))
	if col.Label != "" {
		lines = append(lines, col.Label)
	}
	if col.Name != "" && col.Name != col.Label {
		lines = append(lines, col.Name)
	}
	if col.Role != "" && col.Role != col.Label {
		lines = append(lines, col.Role)
	}
	for _, field := range col.Metadata {
		if field.Label == "" && field.Value == "" {
			continue
		}
		switch {
		case field.Label == "":
			lines = append(lines, field.Value)
		case field.Value == "":
			lines = append(lines, field.Label)
		default:
			lines = append(lines, field.Label+": "+field.Value)
		}
	}
	return strings.Join(lines, "\n")
}

func metadataFieldText(field layout.MetadataField) string {
	if field.Value == "" {
		return field.Label
	}
	return field.Label + ": " + field.Value
}

func (f *Document) applyPageTemplateMargins(template layout.PageTemplate) {
	margins := template.Margins
	if margins.Top <= 0 && margins.Right <= 0 && margins.Bottom <= 0 && margins.Left <= 0 {
		return
	}
	left, top, right, bottom := f.GetMargins()
	if margins.Left > 0 {
		left = margins.Left
	}
	if margins.Top > 0 {
		top = margins.Top
	}
	if margins.Right > 0 {
		right = margins.Right
	}
	if margins.Bottom > 0 {
		bottom = margins.Bottom
	}
	f.SetMargins(left, top, right)
	autoPageBreak, _ := f.GetAutoPageBreak()
	f.SetAutoPageBreak(autoPageBreak, bottom)
	if f.page > 0 {
		if f.x < left {
			f.x = left
		}
		if f.y < top {
			f.y = top
		}
	}
}

func documentAttachments(blocks []layout.AttachmentBlock) []Attachment {
	attachments := make([]Attachment, 0, len(blocks))
	for _, block := range blocks {
		if block.Name == "" && len(block.Data) == 0 {
			continue
		}
		attachments = append(attachments, Attachment{
			Content: block.Data, Filename: block.Name, Description: block.Description, MIMEType: block.MIMEType,
		})
	}
	return attachments
}
