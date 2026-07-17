// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperd

import (
	"strings"

	"github.com/cssbruno/gopdfkit/internal/paperedit"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

type PaperTableProperty string

const (
	PaperTableSplit        PaperTableProperty = "split"
	PaperTableRepeatHeader PaperTableProperty = "repeat-header"
	PaperTableTrackWidth   PaperTableProperty = "width"
	PaperTableTrackMin     PaperTableProperty = "min-width"
	PaperTableTrackMax     PaperTableProperty = "max-width"
	PaperTableCellHeader   PaperTableProperty = "header"
)

type PaperSetTablePropertyRequest struct {
	Guard    PaperMutationGuard `json:"guard"`
	Property PaperTableProperty `json:"property"`
	Split    string             `json:"split,omitempty"`
	Points   float64            `json:"points,omitempty"`
	Bool     bool               `json:"bool,omitempty"`
}

func (w *Workspace) PaperSetTableProperty(request PaperSetTablePropertyRequest) (PaperMutationResult, error) {
	opened, revision, err := w.mutationRevision(request.Guard)
	if err != nil {
		return PaperMutationResult{}, err
	}
	node, table := sourceNodeAndTable(revision.parsed.AST.Root, request.Guard.Target)
	if node == nil || table == nil || table.ID == "" {
		return PaperMutationResult{}, workspaceError("INVALID_TABLE_TARGET", "table mutation requires an exact table, table-track, or cell source node", paperedit.ErrInvalidOperation)
	}
	if table.ID != request.Guard.Target {
		if err := requireAdditionalTargetGuard(revision, request.Guard, table.ID); err != nil {
			return PaperMutationResult{}, err
		}
	}
	var value paperedit.Value
	switch request.Property {
	case PaperTableSplit:
		if node.Kind != paperlang.NodeTable || request.Points != 0 || request.Bool {
			return PaperMutationResult{}, workspaceError("INVALID_TABLE_VALUE", "split targets a table without unrelated values", paperedit.ErrInvalidOperation)
		}
		split := strings.ToLower(strings.TrimSpace(request.Split))
		if split != "rows" && split != "avoid" {
			return PaperMutationResult{}, workspaceError("INVALID_TABLE_VALUE", "split must be rows or avoid", paperedit.ErrInvalidOperation)
		}
		value = paperedit.StringValue(split)
	case PaperTableRepeatHeader:
		if node.Kind != paperlang.NodeTable || request.Split != "" || request.Points != 0 {
			return PaperMutationResult{}, workspaceError("INVALID_TABLE_VALUE", "repeat-header targets a table boolean", paperedit.ErrInvalidOperation)
		}
		value = paperedit.BoolValue(request.Bool)
	case PaperTableTrackWidth, PaperTableTrackMin, PaperTableTrackMax:
		if node.Kind != paperlang.NodeTableTrack || request.Split != "" || request.Bool || !finiteLayoutHandle(request.Points) || request.Points <= 0 || request.Points > 1_000_000 {
			return PaperMutationResult{}, workspaceError("INVALID_TABLE_VALUE", "table track requires a finite positive point value", paperedit.ErrInvalidOperation)
		}
		value = paperedit.UnitValue(request.Points, "pt")
	case PaperTableCellHeader:
		if node.Kind != paperlang.NodeTableCell || request.Split != "" || request.Points != 0 {
			return PaperMutationResult{}, workspaceError("INVALID_TABLE_VALUE", "header targets a cell boolean", paperedit.ErrInvalidOperation)
		}
		value = paperedit.BoolValue(request.Bool)
	default:
		return PaperMutationResult{}, workspaceError("INVALID_TABLE_PROPERTY", "table property is outside the closed mutation vocabulary", paperedit.ErrInvalidOperation)
	}
	targets := []string{request.Guard.Target}
	if table.ID != request.Guard.Target {
		targets = append(targets, table.ID)
	}
	return w.applyPaperMutation("set_table_property", request.Guard, opened, revision, targets, []paperedit.Operation{paperedit.SetProperty{Target: request.Guard.Target, Name: string(request.Property), Value: value}}, "INVALID_TABLE_PROPERTY_STATE")
}

func sourceNodeAndTable(root *paperlang.Node, target string) (*paperlang.Node, *paperlang.Node) {
	var found, table *paperlang.Node
	var walk func(*paperlang.Node, *paperlang.Node)
	walk = func(node, owner *paperlang.Node) {
		if node == nil || found != nil {
			return
		}
		next := owner
		if node.Kind == paperlang.NodeTable {
			next = node
		}
		if node.ID == target {
			found, table = node, next
			return
		}
		for _, member := range node.Members {
			walk(member.Node, next)
		}
	}
	walk(root, nil)
	return found, table
}
