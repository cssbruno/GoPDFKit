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

func TestCompileRowColumnAcceptsResponsiveImageChildren(t *testing.T) {
	source := "document:\n  page:\n    body:\n      row @media:\n        image @hero:\n          track-size: 40%\n          source: \"data:image/png;base64," + paperImagePNG + "\"\n          width: 100%\n          height: \"auto\"\n          alt: \"Evidence\"\n        paragraph @copy:\n          track-size: 60%\n          text: \"Caption\"\n"
	parsed := paperlang.Parse("row-image.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	row := compiled.Document.Body[0].(layout.RowColumnBlock)
	image, ok := row.Items[0].Block.(layout.ImageBlock)
	if !ok || image.WidthPercent != 100_000_000 || row.Items[0].Track.BasisPercent != 40_000_000 || len(image.Data) == 0 {
		t.Fatalf("row image = %#v", row.Items[0])
	}
	mappings := map[string]NodeMapping{}
	for _, mapping := range compiled.Mapping.Nodes {
		mappings[mapping.ID] = mapping
	}
	if mappings["@hero"].BodyIndex != 0 || mappings["@hero"].SegmentIndex != 0 || mappings["@hero"].ResourceDigest == "" {
		t.Fatalf("image mapping = %#v", mappings["@hero"])
	}
}

func TestCompileRowColumnAcceptsTableChildren(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      row @summary:\n        table @facts:\n          track-size: 70%\n          table-track:\n            width: 50%\n          table-track:\n            width: 50%\n          table-row:\n            cell:\n              text: \"Name\"\n            cell:\n              text: \"Value\"\n        paragraph @aside:\n          track-size: 30%\n          text: \"Aside\"\n"
	parsed := paperlang.Parse("row-table.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	row := compiled.Document.Body[0].(layout.RowColumnBlock)
	table, ok := row.Items[0].Block.(layout.TableBlock)
	if !ok || len(table.Columns) != 2 || table.Columns[0].WidthPercent != 50_000_000 ||
		len(table.Body) != 1 || len(table.Body[0].Cells) != 2 || row.Items[0].Track.BasisPercent != 70_000_000 {
		t.Fatalf("row table = %#v", row.Items[0])
	}
}

func TestCompileRowColumnExposesWrapAlignmentAndFlexConstraints(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      row:\n        cross-gap: 4pt\n        cross-size: 80pt\n        wrap: \"wrap-reverse\"\n        main-align: \"space-between\"\n        cross-align: \"center\"\n        align-content: \"stretch\"\n        reverse-main: true\n        paragraph:\n          track-size: 40pt\n          track-grow: 1.5\n          track-shrink: 0.5\n          cross-size: 50%\n          cross-min: 20%\n          cross-max: 80%\n          text: \"Flexible\"\n"
	parsed := paperlang.Parse("flex-row.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	row := compiled.Document.Body[0].(layout.RowColumnBlock)
	track, item := row.Items[0].Track, row.Items[0]
	if row.CrossGap != 4 || row.CrossSize != 80 || row.Wrap != "wrap-reverse" || row.MainAlign != "space-between" ||
		row.CrossAlign != "center" || row.AlignContent != "stretch" || !row.ReverseMain ||
		track.Kind != layout.RowColumnTrackFlex || track.BasisKind != layout.RowColumnFlexBasisFixed || track.Basis != 40 ||
		track.GrowFactor != 1_500_000 || track.ShrinkFactor != 500_000 || item.CrossMinPercent != 20_000_000 ||
		item.CrossMaxPercent != 80_000_000 {
		t.Fatalf("row/item = %#v / %#v", row, item)
	}
}

func TestCompileRowColumnAcceptsOneReadableNestedLevel(t *testing.T) {
	const source = "document:\n  page:\n    body:\n      row @outer:\n        column @details:\n          track-size: 70%\n          gap: 2pt\n          paragraph @first:\n            track-size: 14pt\n            text: \"First\"\n          paragraph @second:\n            track-size: 14pt\n            text: \"Second\"\n        paragraph @aside:\n          track-size: 30%\n          text: \"Aside\"\n"
	parsed := paperlang.Parse("nested-layout.paper", source)
	compiled := Compile(parsed.AST)
	if !parsed.OK() || !compiled.OK() {
		t.Fatalf("diagnostics = %+v / %+v", parsed.Diagnostics, compiled.Diagnostics)
	}
	outer := compiled.Document.Body[0].(layout.RowColumnBlock)
	nested, ok := outer.Items[0].Block.(layout.RowColumnBlock)
	if !ok || nested.Direction != layout.ColumnDirection || nested.Gap != 2 || len(nested.Items) != 2 ||
		outer.Items[0].Track.BasisPercent != 70_000_000 || nested.Items[0].Track.Size != 14 {
		t.Fatalf("nested layout = %#v", outer.Items[0])
	}
	mappings := map[string]NodeMapping{}
	for _, mapping := range compiled.Mapping.Nodes {
		mappings[mapping.ID] = mapping
	}
	if mappings["@details"].SegmentIndex != 0 || mappings["@first"].SegmentIndex != 0 || mappings["@first"].NestedBlockIndex != 0 ||
		mappings["@second"].NestedBlockIndex != 1 {
		t.Fatalf("nested mappings = %+v", mappings)
	}
}
