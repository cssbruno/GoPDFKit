// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"
	"errors"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
)

const canonicalTreeCompileSource = "document @report:\n" +
	"  page @sheet:\n" +
	"    body @content:\n" +
	"      paragraph @first:\n" +
	"        text: \"A\"\n" +
	"      paragraph @second:\n" +
	"        text: \"B\"\n"

func TestCompilePopulatesCanonicalPrivateTreeDeterministically(t *testing.T) {
	ast := paperlang.Parse("tree.paper", canonicalTreeCompileSource).AST
	first := Compile(ast)
	second := Compile(ast)
	if !first.OK() || !second.OK() {
		t.Fatalf("compile diagnostics = %+v / %+v", first.Diagnostics, second.Diagnostics)
	}
	p := first.Tree.Projection()
	if len(p.Nodes) != len(first.Mapping.Nodes) || len(p.Nodes) < 5 || len(p.Children) != len(p.Nodes)-1 {
		t.Fatalf("tree/mapping = %+v / %+v", p, first.Mapping.Nodes)
	}
	if p.Nodes[0].Key != "@report" || p.Nodes[0].ID == 0 || p.Nodes[0].ChildCount != uint32(len(p.Nodes)-1) {
		t.Fatalf("root = %+v", p.Nodes[0])
	}
	if len(p.Styles) != 1 {
		t.Fatalf("compiled paragraph styles were not interned: %+v", p.Styles)
	}
	h1, err := first.Tree.Hash()
	if err != nil {
		t.Fatal(err)
	}
	h2, err := second.Tree.Hash()
	if err != nil || h1 == "" || h1 != h2 {
		t.Fatalf("tree hashes = %q/%q, %v", h1, h2, err)
	}
}

func TestCompileMappingTreeHonorsCancellationAndWorkLimitsAtomically(t *testing.T) {
	compiled := Compile(paperlang.Parse("tree.paper", canonicalTreeCompileSource).AST)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if tree, err := LowerCompileMappingTreeContext(canceled, compiled.Mapping, layoutengine.CanonicalTreeLimits{}); !errors.Is(err, context.Canceled) || len(tree.Projection().Nodes) != 0 {
		t.Fatalf("canceled tree = %+v, %v", tree.Projection(), err)
	}
	limits := layoutengine.DefaultCanonicalTreeLimits()
	limits.MaxWork = 1
	if tree, err := LowerCompileMappingTreeContext(context.Background(), compiled.Mapping, limits); !errors.Is(err, layoutengine.ErrCanonicalTreeLimit) || len(tree.Projection().Nodes) != 0 {
		t.Fatalf("work-limited tree = %+v, %v", tree.Projection(), err)
	}
}
