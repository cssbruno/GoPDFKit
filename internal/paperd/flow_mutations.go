// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

// PaperMoveNodeRequest relocates one authored flow node beneath an exact body,
// row, or column destination. Both the moving node and the destination are
// guarded independently so a visual drop cannot silently rebase after a
// concurrent edit.
type PaperMoveNodeRequest struct {
	Guard     PaperMutationGuard `json:"guard"`
	NewParent string             `json:"new_parent"`
}

func (w *Workspace) PaperMoveNode(request PaperMoveNodeRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	if !validAuthorityNodeID(request.NewParent) || request.NewParent == request.Guard.Target {
		return PaperMutationResult{}, workspaceError("INVALID_MOVE_PARENT", "move requires a distinct readable destination @id", paperedit.ErrInvalidOperation)
	}
	target := findNodeByID(revision.parsed.AST.Root, request.Guard.Target)
	parent := findNodeByID(revision.parsed.AST.Root, request.NewParent)
	if target == nil {
		return PaperMutationResult{}, workspaceError("INVALID_MOVE_TARGET", "move target was not found in the exact source revision", paperedit.ErrInvalidOperation)
	}
	if parent == nil || !paperFlowParentKind(parent.Kind) {
		return PaperMutationResult{}, workspaceError("INVALID_MOVE_PARENT", "move destination must be a body, row, or column", paperedit.ErrInvalidOperation)
	}
	if target.ID == revision.parsed.AST.Root.ID || !paperFlowChildKind(parent.Kind, target.Kind) {
		return PaperMutationResult{}, workspaceError("INVALID_MOVE_TARGET", "target kind is not valid at the requested flow destination", paperedit.ErrInvalidOperation)
	}
	if err := requireAdditionalTargetGuard(revision, request.Guard, request.NewParent); err != nil {
		return PaperMutationResult{}, err
	}
	return w.applyPaperMutation("move_node", request.Guard, opened, revision,
		[]string{request.Guard.Target, request.NewParent},
		[]paperedit.Operation{paperedit.MoveNode{Target: request.Guard.Target, NewParent: request.NewParent}},
		"INVALID_MOVE_RESULT")
}

func paperFlowParentKind(kind paperlang.NodeKind) bool {
	return kind == paperlang.NodeBody || kind == paperlang.NodeRow || kind == paperlang.NodeColumn
}

// Keep this vocabulary aligned with the parser's authored flow grammar. A
// move is deliberately narrower than arbitrary CST relocation: page shells,
// tables, canvas internals, and data nodes remain in their owning domains.
func paperFlowChildKind(parent, child paperlang.NodeKind) bool {
	switch parent {
	case paperlang.NodeBody:
		return child == paperlang.NodeHeading || child == paperlang.NodeParagraph || child == paperlang.NodeList ||
			child == paperlang.NodePageBreak || child == paperlang.NodeText || child == paperlang.NodeRow ||
			child == paperlang.NodeColumn || child == paperlang.NodeImage || child == paperlang.NodeTable ||
			child == paperlang.NodeCanvas || child == paperlang.NodeUse || child == paperlang.NodeRepeat || child == paperlang.NodeLoop
	case paperlang.NodeRow, paperlang.NodeColumn:
		return child == paperlang.NodeHeading || child == paperlang.NodeParagraph || child == paperlang.NodeUse
	default:
		return false
	}
}
