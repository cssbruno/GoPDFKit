// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"context"
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/papercompile"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

type paperMeasuredCanvasItem struct {
	node     layoutengine.NodeID
	key      layoutengine.NodeKey
	instance layoutengine.InstanceID
	source   layoutengine.SourceSpan
	role     layoutengine.SemanticRole
	alt      string
}

type paperMeasuredCanvas struct {
	projection layoutengine.LayoutPlanProjection
	height     layoutengine.Fixed
	items      []paperMeasuredCanvasItem
}

func paperCanvasMappingForBody(mapping papercompile.CompileMapping, bodyIndex int) papercompile.CompileMapping {
	result := papercompile.CompileMapping{ThemeProperties: append([]papercompile.ThemePropertyMapping(nil), mapping.ThemeProperties...)}
	for _, node := range mapping.Nodes {
		if node.Kind == paperlang.NodeDocument || node.BodyIndex == bodyIndex {
			if node.BodyIndex == bodyIndex {
				node.BodyIndex = 0
			}
			result.Nodes = append(result.Nodes, node)
		}
	}
	return result
}

func layoutFixedFromDocumentUnits(f *Document, value float64) layoutengine.Fixed {
	fixed, _ := layoutengine.FixedFromPoints(f.UnitToPointConvert(value))
	return fixed
}

func (f *Document) planPaperCanvas(ctx context.Context, doc *layout.LayoutDocument, mapping papercompile.CompileMapping, block layout.CanvasBlock) (layoutengine.LayoutPlan, error) {
	left, top, right, bottom := typedShadowMargins(f, doc.PageTemplate.Margins)
	pageSize, body, err := typedShadowFixedGeometry(f, left, top, f.w-left-right, f.h-top-bottom)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	width, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(block.Width))
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	height, err := layoutengine.FixedFromPoints(f.UnitToPointConvert(block.Height))
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	if width <= 0 || height <= 0 || width > body.Width || height > body.Height {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: canvas dimensions must be positive and fit the page body")
	}
	container, err := layoutengine.NewRect(body.X, body.Y, width, height)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	horizontal, ok := paperCanvasAnchor(block.DefaultHorizontal)
	if !ok {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: invalid canvas horizontal default %q", block.DefaultHorizontal)
	}
	vertical, ok := paperCanvasAnchor(block.DefaultVertical)
	if !ok {
		return layoutengine.LayoutPlan{}, fmt.Errorf("document: invalid canvas vertical default %q", block.DefaultVertical)
	}
	nodeIDs := make(map[string]layoutengine.NodeID, len(block.Items))
	for index, item := range block.Items {
		if item.ID == "" || nodeIDs[item.ID].Valid() {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: canvas item IDs must be non-empty and unique")
		}
		nodeIDs[item.ID] = layoutengine.NodeID(index + 1)
	}
	nodes := make([]layoutengine.CanvasNode, 0, len(block.Items))
	for index, item := range block.Items {
		identity := paperBlockIdentity(mapping, 0, index, -1, index)
		itemWidth, widthErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(item.Width))
		itemHeight, heightErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(item.Height))
		if widthErr != nil || heightErr != nil || itemWidth <= 0 || itemHeight <= 0 {
			return layoutengine.LayoutPlan{}, fmt.Errorf("document: canvas item %s has invalid dimensions", item.ID)
		}
		node := layoutengine.CanvasNode{Node: layoutengine.NodeID(index + 1), Key: identity.key,
			Instance: identity.instance, Source: identity.source, Size: layoutengine.Size{Width: itemWidth, Height: itemHeight}}
		for _, authored := range item.Constraints {
			anchor, anchorOK := paperCanvasAnchor(authored.Anchor)
			targetAnchor, targetOK := paperCanvasAnchor(authored.TargetAnchor)
			if !anchorOK || !targetOK {
				return layoutengine.LayoutPlan{}, fmt.Errorf("document: canvas item %s uses an invalid anchor", item.ID)
			}
			var target layoutengine.NodeID
			if authored.Target != "canvas" {
				target = nodeIDs[authored.Target]
				if !target.Valid() {
					return layoutengine.LayoutPlan{}, fmt.Errorf("document: canvas item %s targets missing sibling %s", item.ID, authored.Target)
				}
			}
			offset, offsetErr := layoutengine.FixedFromPoints(f.UnitToPointConvert(authored.Offset))
			if offsetErr != nil {
				return layoutengine.LayoutPlan{}, offsetErr
			}
			node.Constraints = append(node.Constraints, layoutengine.CanvasConstraint{Anchor: anchor, TargetNode: target, TargetAnchor: targetAnchor, Offset: offset})
		}
		nodes = append(nodes, node)
	}
	plan, err := layoutengine.PlanCanvas(ctx, layoutengine.CanvasPlanInput{PageSize: pageSize, Container: container,
		Defaults: layoutengine.CanvasDefaults{Horizontal: horizontal, Vertical: vertical}, Nodes: nodes}, layoutengine.DefaultCanvasPlanLimits())
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	decorations := make([]layoutengine.BoxDecoration, 0, len(block.Items))
	for index, item := range block.Items {
		measured, measureErr := f.paperMeasureBox(item.Box, fmt.Sprintf("body[0].items[%d].box", index))
		if measureErr != nil {
			return layoutengine.LayoutPlan{}, measureErr
		}
		decorations = append(decorations, layoutengine.BoxDecoration{Fragment: layoutengine.FragmentID(index + 1),
			Background: measured.background, Radius: measured.radius, Shadow: measured.shadow,
			Top:    layoutengine.BoxBorderSide{Width: measured.borders[0].width, Color: measured.borders[0].color},
			Right:  layoutengine.BoxBorderSide{Width: measured.borders[1].width, Color: measured.borders[1].color},
			Bottom: layoutengine.BoxBorderSide{Width: measured.borders[2].width, Color: measured.borders[2].color},
			Left:   layoutengine.BoxBorderSide{Width: measured.borders[3].width, Color: measured.borders[3].color}})
	}
	plan, err = layoutengine.AttachBoxDecorations(plan, decorations)
	if err != nil {
		return layoutengine.LayoutPlan{}, err
	}
	rootKey, rootInstance := layoutengine.NodeKey("@paper-document"), layoutengine.InstanceID("@paper-document")
	var rootSource layoutengine.SourceSpan
	for _, mapped := range mapping.Nodes {
		if mapped.Kind != paperlang.NodeDocument {
			continue
		}
		if mapped.ID != "" {
			rootKey, rootInstance = layoutengine.NodeKey(mapped.ID), layoutengine.InstanceID(mapped.ID)
		}
		rootSource = paperLayoutSourceSpan(mapped.Span)
		break
	}
	canvasIdentity := paperBlockIdentity(mapping, 0, -1, -1, 0)
	semanticNodes := []layoutengine.SemanticNode{
		{ID: 1, Role: layoutengine.SemanticRoleDocument, Key: rootKey, Instance: rootInstance, Source: rootSource,
			Attributes: layoutengine.SemanticAttributes{Language: strings.TrimSpace(doc.Language)}},
		{ID: 2, Parent: 1, Role: layoutengine.SemanticRoleSection, Key: canvasIdentity.key, Instance: canvasIdentity.instance, Source: canvasIdentity.source},
	}
	associations := make([]layoutengine.SemanticFragmentAssociation, 0, len(block.Items))
	reading := make([]layoutengine.ReadingOccurrence, 0, len(block.Items))
	for index, item := range block.Items {
		role := layoutengine.SemanticRoleArtifact
		attributes := layoutengine.SemanticAttributes{}
		if strings.TrimSpace(item.Alt) != "" {
			role = layoutengine.SemanticRoleFigure
			attributes.AlternateText = strings.TrimSpace(item.Alt)
		}
		fragment := layoutengine.FragmentID(index + 1)
		identity := nodes[index]
		semantic := layoutengine.SemanticNodeID(index + 3)
		semanticNodes = append(semanticNodes, layoutengine.SemanticNode{ID: semantic, Parent: 2, Role: role,
			Key: identity.Key, Instance: identity.Instance, Source: identity.Source, Attributes: attributes})
		associations = append(associations, layoutengine.SemanticFragmentAssociation{Semantic: semantic, Page: 1, Fragment: fragment})
		if role != layoutengine.SemanticRoleArtifact {
			reading = append(reading, layoutengine.ReadingOccurrence{Semantic: semantic, Page: 1, Fragment: fragment, ReadingIndex: uint32(len(reading))})
		}
	}
	return layoutengine.AttachSemantics(plan, semanticNodes, associations, reading)
}

func paperCanvasAnchor(value string) (layoutengine.CanvasAnchor, bool) {
	anchor := layoutengine.CanvasAnchor(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_"))
	switch anchor {
	case layoutengine.CanvasAnchorLeft, layoutengine.CanvasAnchorRight, layoutengine.CanvasAnchorCenterX,
		layoutengine.CanvasAnchorTop, layoutengine.CanvasAnchorBottom, layoutengine.CanvasAnchorCenterY, layoutengine.CanvasAnchorBaseline:
		return anchor, true
	default:
		return "", false
	}
}
