// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"context"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/layoutengine"
	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompileRowColumnTracksAndSourceMappings(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      column @stack:\n        gap: 6pt\n        cross-align: \"center\"\n        heading @title:\n          track: \"fixed\"\n          track-size: 20pt\n          level: 2\n          text @title-copy: \"Title\"\n        paragraph @body-copy:\n          track: \"fraction\"\n          track-weight: 3\n          track-min: 12pt\n          cross-align: \"end\"\n          text: \"Body\"\n"
	parsed := paperlang.Parse("column.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	block, ok := compiled.Document.Body[0].(layout.RowColumnBlock)
	if !ok || block.Direction != layout.ColumnDirection || block.Gap != 6 || block.CrossAlign != "center" || len(block.Items) != 2 {
		t.Fatalf("compiled block = %#v", compiled.Document.Body[0])
	}
	if block.Items[0].Track.Kind != layout.RowColumnTrackFixed || block.Items[0].Track.Size != 20 ||
		block.Items[1].Track.Kind != layout.RowColumnTrackFraction || block.Items[1].Track.Weight != 3 || block.Items[1].Track.Min != 12 {
		t.Fatalf("compiled tracks = %#v", block.Items)
	}
	mappings := map[string]NodeMapping{}
	for _, mapping := range compiled.Mapping.Nodes {
		mappings[mapping.ID] = mapping
	}
	if mappings["@stack"].BodyIndex != 0 || mappings["@stack"].SegmentIndex != -1 ||
		mappings["@title"].SegmentIndex != 0 || mappings["@body-copy"].SegmentIndex != 1 ||
		mappings["@title-copy"].NestedBlockIndex != 0 {
		t.Fatalf("mappings = %+v", compiled.Mapping.Nodes)
	}
}

func TestCompileRowColumnDiagnosesInvalidTrackContract(t *testing.T) {
	parsed := paperlang.Parse("bad-track.paper", "document:\n  page:\n    body:\n      row:\n        paragraph:\n          track: \"fraction\"\n          text: \"missing weight\"\n")
	compiled := Compile(parsed.AST)
	if !parsed.OK() || compiled.OK() {
		t.Fatalf("parse/compile = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
}

func TestCompileRowColumnPreservesContainerRelativeAndAutoTrackSizes(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      row:\n        paragraph:\n          track-size: 50%\n          track-min: 20%\n          text: \"Half\"\n        paragraph:\n          track: \"flex\"\n          track-size: \"auto\"\n          track-max: 40%\n          text: \"Intrinsic\"\n"
	parsed := paperlang.Parse("responsive-row.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	row := compiled.Document.Body[0].(layout.RowColumnBlock)
	first, second := row.Items[0].Track, row.Items[1].Track
	if first.Kind != layout.RowColumnTrackFlex || first.BasisKind != layout.RowColumnFlexBasisPercent ||
		first.BasisPercent != 50_000_000 || first.MinPercent != 20_000_000 || first.Shrink != 1 {
		t.Fatalf("percentage track = %#v", first)
	}
	if second.Kind != layout.RowColumnTrackFlex || second.BasisKind != layout.RowColumnFlexBasisContent ||
		second.MaxPercent != 40_000_000 || second.Shrink != 1 {
		t.Fatalf("automatic track = %#v", second)
	}
	tree, err := LowerLayoutDocumentTreeContext(context.Background(), compiled.Document, layoutengine.CanonicalTreeLimits{})
	if err != nil {
		t.Fatal(err)
	}
	projection := tree.Projection()
	foundPercent := false
	for _, track := range projection.Tracks {
		foundPercent = foundPercent || track.Max.Kind == "percent" && track.Max.Value == 512
	}
	if !foundPercent {
		t.Fatalf("canonical tracks lost 50%% basis: %+v", projection.Tracks)
	}
}
