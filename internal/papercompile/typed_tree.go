// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/layout"
)

const (
	typedTreeKeepTogether uint32 = 1 << iota
	typedTreeKeepWithNext
	typedTreeBreakBefore
	typedTreeBreakAfter
	typedTreeRepeated
)

// LowerLayoutDocumentTreeContext lowers the supported typed document surface
// into the same dense, interned canonical tree used by .paper compilation.
// Typed models do not carry source files, so their stable revision-scoped paths
// are retained as keys and semantic labels while SourceSpan remains empty.
func LowerLayoutDocumentTreeContext(ctx context.Context, doc *layout.LayoutDocument, limits layoutengine.CanonicalTreeLimits) (layoutengine.CanonicalTree, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if doc == nil {
		return layoutengine.CanonicalTree{}, fmt.Errorf("typed canonical tree: document is nil")
	}
	maxNodes := uint32(1 << 20)
	if limits.MaxNodes != 0 && limits.MaxNodes < maxNodes {
		maxNodes = limits.MaxNodes
	}
	b := typedTreeBuilder{ctx: ctx, maxNodes: maxNodes}
	root, err := b.add(-1, "@typed", "document", doc.Title, nil, nil, nil, layoutengine.SemanticRoleDocument, 0)
	if err != nil {
		return layoutengine.CanonicalTree{}, err
	}
	page, err := b.add(root, "@typed/page", "page", "", nil, nil, nil, layoutengine.SemanticRoleArtifact, 0)
	if err != nil {
		return layoutengine.CanonicalTree{}, err
	}
	if err := b.pageTemplate(page, doc.PageTemplate); err != nil {
		return layoutengine.CanonicalTree{}, err
	}
	body, err := b.add(page, "@typed/page/body", "region-body", "", typedTreeBoxStyle(layout.BoxStyle{Margin: doc.PageTemplate.Margins}, layout.TextStyle{}), nil, nil, layoutengine.SemanticRoleArtifact, 0)
	if err != nil {
		return layoutengine.CanonicalTree{}, err
	}
	for index, block := range layout.NormalizeBlocks(doc.Body) {
		if err := b.block(body, fmt.Sprintf("@typed/body/%d", index), block); err != nil {
			return layoutengine.CanonicalTree{}, err
		}
	}
	if doc.Signature != nil {
		if err := b.signature(root, doc.Signature); err != nil {
			return layoutengine.CanonicalTree{}, err
		}
	}
	if doc.QR != nil {
		if err := b.qr(root, "@typed/qr", *doc.QR); err != nil {
			return layoutengine.CanonicalTree{}, err
		}
	}
	for index, attachment := range doc.Attachments {
		digest := sha256.Sum256(attachment.Data)
		resource := &layoutengine.TreeResourceInput{Kind: "attachment", Key: attachment.Name, Digest: hex.EncodeToString(digest[:])}
		if _, err := b.add(root, fmt.Sprintf("@typed/attachment/%d", index), "attachment", attachment.Description, nil, nil, resource, layoutengine.SemanticRoleArtifact, 0); err != nil {
			return layoutengine.CanonicalTree{}, err
		}
	}
	return layoutengine.NewCanonicalTree(ctx, layoutengine.CanonicalTreeInput{Nodes: b.nodes}, limits)
}

type typedTreeBuilder struct {
	ctx      context.Context
	maxNodes uint32
	nodes    []layoutengine.TreeNodeInput
}

func (b *typedTreeBuilder) add(parent int, key, kind, text string, style *layoutengine.TreeStyleInput, track *layoutengine.TreeTrackInput, resource *layoutengine.TreeResourceInput, role layoutengine.SemanticRole, flags uint32) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if uint32(len(b.nodes)) >= b.maxNodes {
		return 0, layoutengine.ErrCanonicalTreeLimit
	}
	if err := layoutengine.ChargePlanningWork(b.ctx, "typed canonical tree lowering", 1); err != nil {
		return 0, err
	}
	index := len(b.nodes)
	b.nodes = append(b.nodes, layoutengine.TreeNodeInput{ID: layoutengine.NodeID(index + 1), Key: layoutengine.NodeKey(key),
		Kind: kind, Parent: int64(parent), Text: typedTreeText(text), Style: style, Track: track, Resource: resource,
		Semantic: &layoutengine.TreeSemanticInput{Role: role, Label: key}, Flags: flags})
	return index, nil
}

func (b *typedTreeBuilder) pageTemplate(page int, template layout.PageTemplate) error {
	type region struct {
		key, kind string
		blocks    []layout.Block
		box       layout.BoxStyle
	}
	regions := []region{}
	if template.Header != nil {
		regions = append(regions, region{"header", "region-header", template.Header.Blocks, template.Header.EffectiveBox()})
	}
	if template.FirstPageHeader != nil {
		regions = append(regions, region{"header-first", "region-header-first", template.FirstPageHeader.Blocks, template.FirstPageHeader.EffectiveBox()})
	}
	if template.Footer != nil {
		regions = append(regions, region{"footer", "region-footer", template.Footer.Blocks, template.Footer.EffectiveBox()})
	}
	if template.FirstPageFooter != nil {
		regions = append(regions, region{"footer-first", "region-footer-first", template.FirstPageFooter.Blocks, template.FirstPageFooter.EffectiveBox()})
	}
	if template.EvenPageFooter != nil {
		regions = append(regions, region{"footer-even", "region-footer-even", template.EvenPageFooter.Blocks, template.EvenPageFooter.EffectiveBox()})
	}
	for _, region := range regions {
		key := "@typed/page/" + region.key
		parent, err := b.add(page, key, region.kind, "", typedTreeBoxStyle(region.box, layout.TextStyle{}), nil, nil, layoutengine.SemanticRoleArtifact, typedTreeBoxFlags(region.box))
		if err != nil {
			return err
		}
		for index, block := range layout.NormalizeBlocks(region.blocks) {
			if err := b.block(parent, fmt.Sprintf("%s/%d", key, index), block); err != nil {
				return err
			}
		}
	}
	if template.PageNumbers.Enabled {
		_, err := b.add(page, "@typed/page/counter", "page-counter", template.PageNumbers.Format, nil, nil, nil, layoutengine.SemanticRoleArtifact, typedTreeRepeated)
		return err
	}
	return nil
}

func (b *typedTreeBuilder) block(parent int, key string, candidate layout.Block) error {
	block, ok := layout.NormalizeBlock(candidate)
	if !ok {
		return nil
	}
	switch block := block.(type) {
	case layout.ParagraphBlock:
		_, err := b.add(parent, key, "paragraph", layout.TextSegmentsPlainText(block.Segments), typedTreeBoxStyle(block.EffectiveBox(), block.EffectiveStyle()), nil, nil, layoutengine.SemanticRoleParagraph, typedTreeBoxFlags(block.EffectiveBox()))
		return err
	case layout.HeadingBlock:
		_, err := b.add(parent, key, "heading", layout.TextSegmentsPlainText(block.Segments), typedTreeBoxStyle(block.EffectiveBox(), block.EffectiveStyle()), nil, nil, layoutengine.SemanticRoleHeading, typedTreeBoxFlags(block.EffectiveBox())|uint32(block.Level)<<16)
		return err
	case layout.ListBlock:
		list, err := b.add(parent, key, "list", block.MarkerStyle, typedTreeBoxStyle(block.EffectiveBox(), block.EffectiveStyle()), nil, nil, layoutengine.SemanticRoleList, typedTreeBoxFlags(block.EffectiveBox()))
		if err != nil {
			return err
		}
		for itemIndex, item := range block.Items {
			itemKey := fmt.Sprintf("%s/item/%d", key, itemIndex)
			itemNode, addErr := b.add(list, itemKey, "list-item", strconv.Itoa(itemIndex+1), nil, nil, nil, layoutengine.SemanticRoleListItem, 0)
			if addErr != nil {
				return addErr
			}
			for childIndex, child := range layout.NormalizeBlocks(item.Blocks) {
				if err := b.block(itemNode, fmt.Sprintf("%s/%d", itemKey, childIndex), child); err != nil {
					return err
				}
			}
		}
		return nil
	case layout.SectionBlock:
		return b.container(parent, key, "section", block.Title, block.Blocks, block.EffectiveBox(), layoutengine.SemanticRoleSection, boolFlag(block.KeepTitleWithBody, typedTreeKeepWithNext))
	case layout.ClauseBlock:
		flags := boolFlag(block.BreakBefore, typedTreeBreakBefore) | boolFlag(block.BreakAfter, typedTreeBreakAfter) | boolFlag(block.KeepTogether, typedTreeKeepTogether)
		return b.container(parent, key, "clause", strings.TrimSpace(block.Number+" "+block.Title), block.Blocks, block.EffectiveBox(), layoutengine.SemanticRoleSection, flags)
	case layout.NoteBoxBlock:
		return b.container(parent, key, "note", block.Title, block.Body, block.EffectiveBox(), layoutengine.SemanticRoleSection, 0)
	case layout.ImageBlock:
		data := block.ImageData()
		digest := sha256.Sum256(data)
		resource := &layoutengine.TreeResourceInput{Kind: "image/" + strings.ToLower(block.Format), Key: key, Digest: hex.EncodeToString(digest[:])}
		role := imageSemanticRole(block.Alt)
		if block.Decorative {
			role = layoutengine.SemanticRoleArtifact
		}
		_, err := b.add(parent, key, "image", block.Alt, typedTreeBoxStyle(block.EffectiveBox(), layout.TextStyle{}), nil, resource, role, typedTreeBoxFlags(block.EffectiveBox()))
		return err
	case layout.QRVerificationBlock:
		node, err := b.add(parent, key, "qr-verification", layout.TextSegmentsPlainText(block.Text), typedTreeBoxStyle(block.EffectiveBox(), block.EffectiveStyle()), nil, nil, layoutengine.SemanticRoleFigure, typedTreeBoxFlags(block.EffectiveBox()))
		if err != nil {
			return err
		}
		return b.qr(node, key+"/qr", block.QR)
	case layout.TableBlock:
		return b.table(parent, key, block)
	case layout.MetadataGridBlock:
		return b.metadataGrid(parent, key, block)
	case layout.SignatureRowBlock:
		return b.signatureRow(parent, key, block)
	case layout.RowColumnBlock:
		kind := "row"
		if block.Direction == layout.ColumnDirection {
			kind = "column"
		}
		node, err := b.add(parent, key, kind, "", nil, nil, nil, layoutengine.SemanticRoleSection, 0)
		if err != nil {
			return err
		}
		for index, item := range block.Items {
			itemKey := fmt.Sprintf("%s/item/%d", key, index)
			track := typedTreeRowColumnTrack(item.Track, itemKey)
			itemNode, addErr := b.add(node, itemKey, "track", "", nil, &track, nil, layoutengine.SemanticRoleSection, 0)
			if addErr != nil {
				return addErr
			}
			if err := b.block(itemNode, itemKey+"/block", item.Block); err != nil {
				return err
			}
		}
		return nil
	case layout.PageBreakBlock:
		flags := boolFlag(block.Before, typedTreeBreakBefore) | boolFlag(block.After, typedTreeBreakAfter)
		_, err := b.add(parent, key, "page-break", "", nil, nil, nil, layoutengine.SemanticRoleArtifact, flags)
		return err
	default:
		_, err := b.add(parent, key, string(block.DocumentBlockKind()), "", nil, nil, nil, layoutengine.SemanticRoleArtifact, 0)
		return err
	}
}

func (b *typedTreeBuilder) container(parent int, key, kind, title string, blocks []layout.Block, box layout.BoxStyle, role layoutengine.SemanticRole, flags uint32) error {
	node, err := b.add(parent, key, kind, title, typedTreeBoxStyle(box, layout.TextStyle{}), nil, nil, role, flags|typedTreeBoxFlags(box))
	if err != nil {
		return err
	}
	for index, child := range layout.NormalizeBlocks(blocks) {
		if err := b.block(node, fmt.Sprintf("%s/%d", key, index), child); err != nil {
			return err
		}
	}
	return nil
}

func (b *typedTreeBuilder) table(parent int, key string, table layout.TableBlock) error {
	node, err := b.add(parent, key, "table", table.Caption, typedTreeBoxStyle(table.EffectiveBox(), layout.TextStyle{}), nil, nil, layoutengine.SemanticRoleTable, typedTreeBoxFlags(table.EffectiveBox())|boolFlag(table.Style.RepeatHeader, typedTreeRepeated))
	if err != nil {
		return err
	}
	for index, column := range table.Columns {
		track := layoutengine.TreeTrackInput{Name: fmt.Sprintf("column-%d", index), Min: typedTreeLength(column.MinWidth), Max: typedTreeLength(column.Width)}
		if column.Width == 0 {
			track.Max = layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}
		}
		if _, err := b.add(node, fmt.Sprintf("%s/column/%d", key, index), "column-track", "", nil, &track, nil, layoutengine.SemanticRoleArtifact, 0); err != nil {
			return err
		}
	}
	groups := []struct {
		name   string
		rows   []layout.TableRow
		header bool
	}{{"header", table.Header, true}, {"body", table.Body, false}, {"footer", table.Footer, false}}
	rowIndex := 0
	for _, group := range groups {
		for _, row := range group.rows {
			rowKey := fmt.Sprintf("%s/%s/%d", key, group.name, rowIndex)
			rowNode, addErr := b.add(node, rowKey, "table-row", "", nil, nil, nil, layoutengine.SemanticRoleRow, boolFlag(row.KeepTogether, typedTreeKeepTogether)|boolFlag(group.header && table.Style.RepeatHeader, typedTreeRepeated))
			if addErr != nil {
				return addErr
			}
			for cellIndex, cell := range row.Cells {
				cellKey := fmt.Sprintf("%s/cell/%d", rowKey, cellIndex)
				role := layoutengine.SemanticRoleCell
				flags := uint32(cell.ColSpan&0xff)<<16 | uint32(cell.RowSpan&0xff)<<24
				cellNode, cellErr := b.add(rowNode, cellKey, "table-cell", "", typedTreeBoxStyle(cell.EffectiveBox(), cell.EffectiveStyle()), nil, nil, role, flags)
				if cellErr != nil {
					return cellErr
				}
				for childIndex, child := range layout.NormalizeBlocks(cell.Blocks) {
					if err := b.block(cellNode, fmt.Sprintf("%s/%d", cellKey, childIndex), child); err != nil {
						return err
					}
				}
			}
			rowIndex++
		}
	}
	return nil
}

func (b *typedTreeBuilder) metadataGrid(parent int, key string, grid layout.MetadataGridBlock) error {
	track := layoutengine.TreeTrackInput{Name: "grid-columns", Min: layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}, Max: layoutengine.TreeLength{Kind: layoutengine.TreeLengthFraction, Value: layoutengine.Fixed(maxInt(grid.Columns, 1) * 1024)}}
	node, err := b.add(parent, key, "grid", "", typedTreeBoxStyle(grid.EffectiveBox(), grid.EffectiveStyle()), &track, nil, layoutengine.SemanticRoleSection, typedTreeBoxFlags(grid.EffectiveBox()))
	if err != nil {
		return err
	}
	for index, field := range grid.Fields {
		if _, err := b.add(node, fmt.Sprintf("%s/cell/%d", key, index), "grid-cell", field.Label+": "+field.Value, nil, nil, nil, layoutengine.SemanticRoleParagraph, 0); err != nil {
			return err
		}
	}
	return nil
}

func (b *typedTreeBuilder) signature(parent int, signature *layout.SignatureBlock) error {
	flags := boolFlag(signature.KeepTogether, typedTreeKeepTogether)
	node, err := b.add(parent, "@typed/signature", "signature", signature.PAdESFieldName(), nil, nil, nil, layoutengine.SemanticRoleSection, flags)
	if err != nil {
		return err
	}
	for index, row := range signature.Rows {
		if err := b.signatureRow(node, fmt.Sprintf("@typed/signature/row/%d", index), row); err != nil {
			return err
		}
	}
	return nil
}

func (b *typedTreeBuilder) signatureRow(parent int, key string, row layout.SignatureRowBlock) error {
	node, err := b.add(parent, key, "signature-row", "", typedTreeBoxStyle(row.EffectiveBox(), layout.TextStyle{}), nil, nil, layoutengine.SemanticRoleRow, typedTreeBoxFlags(row.EffectiveBox())|boolFlag(row.KeepTogether, typedTreeKeepTogether))
	if err != nil {
		return err
	}
	for index, column := range row.Columns {
		track := layoutengine.TreeTrackInput{Name: fmt.Sprintf("signature-%d", index), Min: layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}, Max: typedTreeLength(column.Width)}
		if column.Width == 0 {
			track.Max = layoutengine.TreeLength{Kind: layoutengine.TreeLengthFraction, Value: 1024}
		}
		text := strings.TrimSpace(column.Label + " " + column.Name + " " + column.Role)
		if _, err := b.add(node, fmt.Sprintf("%s/cell/%d", key, index), "signature-cell", text, nil, &track, nil, layoutengine.SemanticRoleCell, 0); err != nil {
			return err
		}
	}
	return nil
}

func (b *typedTreeBuilder) qr(parent int, key string, qr layout.QRBlock) error {
	digest := sha256.Sum256([]byte(qr.Value))
	resource := &layoutengine.TreeResourceInput{Kind: "qr", Key: key, Digest: hex.EncodeToString(digest[:])}
	_, err := b.add(parent, key, "qr", qr.Label, nil, nil, resource, layoutengine.SemanticRoleFigure, boolFlag(qr.KeepTogether, typedTreeKeepTogether))
	return err
}

func typedTreeBoxStyle(box layout.BoxStyle, text layout.TextStyle) *layoutengine.TreeStyleInput {
	fontSize, _ := layoutengine.FixedFromPoints(text.FontSize)
	lineHeight, _ := layoutengine.FixedFromPoints(text.LineHeight)
	if fontSize < 0 {
		fontSize = 0
	}
	if lineHeight < 0 {
		lineHeight = 0
	}
	return &layoutengine.TreeStyleInput{FontFamily: strings.TrimSpace(text.FontFamily), Align: strings.TrimSpace(text.Align), FontSize: fontSize, LineHeight: lineHeight,
		Margin: [4]layoutengine.TreeLength{typedTreeLength(box.Margin.Top), typedTreeLength(box.Margin.Right), typedTreeLength(box.Margin.Bottom), typedTreeLength(box.Margin.Left)}}
}

func typedTreeLength(points float64) layoutengine.TreeLength {
	if points <= 0 {
		return layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}
	}
	fixed, err := layoutengine.FixedFromPoints(points)
	if err != nil {
		return layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}
	}
	return layoutengine.TreeLength{Kind: layoutengine.TreeLengthFixed, Value: fixed}
}

func typedTreeRowColumnTrack(track layout.RowColumnTrack, name string) layoutengine.TreeTrackInput {
	result := layoutengine.TreeTrackInput{Name: name, Min: typedTreeLength(track.Min), Max: typedTreeLength(track.Size)}
	switch track.Kind {
	case layout.RowColumnTrackAuto:
		result.Max = layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}
	case layout.RowColumnTrackFraction:
		result.Max = layoutengine.TreeLength{Kind: layoutengine.TreeLengthFraction, Value: layoutengine.Fixed(maxUint32(track.Weight, 1) * 1024)}
	}
	return result
}

func typedTreeBoxFlags(box layout.BoxStyle) uint32 {
	return boolFlag(box.KeepTogether, typedTreeKeepTogether) | boolFlag(box.KeepWithNext, typedTreeKeepWithNext)
}
func boolFlag(value bool, flag uint32) uint32 {
	if value {
		return flag
	}
	return 0
}
func imageSemanticRole(alt string) layoutengine.SemanticRole {
	if strings.TrimSpace(alt) == "" {
		return layoutengine.SemanticRoleArtifact
	}
	return layoutengine.SemanticRoleFigure
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func maxUint32(a, b uint32) uint32 {
	if a > b {
		return a
	}
	return b
}

func typedTreeText(value string) string {
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, value)
	return strings.TrimSpace(clean)
}
