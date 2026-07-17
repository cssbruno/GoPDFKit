// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

func lowerCompileMappingTree(mapping CompileMapping) (layoutengine.CanonicalTree, error) {
	return LowerCompileMappingTreeContext(context.Background(), mapping, layoutengine.CanonicalTreeLimits{})
}

// LowerCompileMappingTreeContext is the bounded canonical-tree boundary used
// by the real .paper compiler after semantic lowering. It consumes the exact
// mapping order that document planning already uses for provenance.
func LowerCompileMappingTreeContext(ctx context.Context, mapping CompileMapping, limits layoutengine.CanonicalTreeLimits) (layoutengine.CanonicalTree, error) {
	nodes := make([]layoutengine.TreeNodeInput, len(mapping.Nodes))
	for index, mapped := range mapping.Nodes {
		parent := int64(-1)
		if index > 0 {
			parent = 0
		}
		role := compileTreeSemanticRole(mapped.Kind)
		nodes[index] = layoutengine.TreeNodeInput{
			ID: layoutengine.NodeID(index + 1), Key: layoutengine.NodeKey(mapped.ID), Kind: string(mapped.Kind), Parent: parent,
			Source: compileTreeSourceSpan(mapped.Span), Semantic: &layoutengine.TreeSemanticInput{Role: role, Label: mapped.ID},
		}
		if mapped.ResourceKind != "" && mapped.ResourceDigest != "" {
			nodes[index].Resource = &layoutengine.TreeResourceInput{Kind: mapped.ResourceKind, Key: mapped.ID, Digest: mapped.ResourceDigest}
		}
		if role == layoutengine.SemanticRoleParagraph || role == layoutengine.SemanticRoleHeading {
			nodes[index].Style = &layoutengine.TreeStyleInput{FontFamily: "default", Align: "start",
				Margin: [4]layoutengine.TreeLength{{Kind: layoutengine.TreeLengthAuto}, {Kind: layoutengine.TreeLengthAuto}, {Kind: layoutengine.TreeLengthAuto}, {Kind: layoutengine.TreeLengthAuto}}}
		}
		if mapped.Kind == paperlang.NodeRow || mapped.Kind == paperlang.NodeColumn {
			nodes[index].Track = &layoutengine.TreeTrackInput{Name: mapped.ID,
				Min: layoutengine.TreeLength{Kind: layoutengine.TreeLengthAuto}, Max: layoutengine.TreeLength{Kind: layoutengine.TreeLengthFraction, Value: 1024}}
		}
	}
	return layoutengine.NewCanonicalTree(ctx, layoutengine.CanonicalTreeInput{Nodes: nodes}, limits)
}

func compileTreeSemanticRole(kind paperlang.NodeKind) layoutengine.SemanticRole {
	switch kind {
	case paperlang.NodeParagraph:
		return layoutengine.SemanticRoleParagraph
	case paperlang.NodeHeading:
		return layoutengine.SemanticRoleHeading
	case paperlang.NodeList:
		return layoutengine.SemanticRoleList
	case paperlang.NodeItem:
		return layoutengine.SemanticRoleListItem
	case paperlang.NodeRow:
		return layoutengine.SemanticRoleRow
	case paperlang.NodeImage:
		return layoutengine.SemanticRoleFigure
	case paperlang.NodeTable:
		return layoutengine.SemanticRoleTable
	default:
		return layoutengine.SemanticRoleArtifact
	}
}

func compileTreeSourceSpan(span paperlang.Span) layoutengine.SourceSpan {
	return layoutengine.SourceSpan{File: span.File,
		Start: layoutengine.SourcePosition{Offset: span.Start.Offset, Line: span.Start.Line, Column: span.Start.Column},
		End:   layoutengine.SourcePosition{Offset: span.End.Offset, Line: span.End.Line, Column: span.End.Column}}
}
