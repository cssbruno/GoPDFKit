// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

const paperImagePNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestCompileImageLowersResourceGeometryAndAccessibility(t *testing.T) {
	source := "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n        source: \"data:image/png;base64," + paperImagePNG + "\"\n        width: 40pt\n        height: 20pt\n        fit: \"cover\"\n        focus-x: 0.25\n        focus-y: 0.75\n        alt: \"Evidence chart\"\n        caption: \"Figure 1\"\n"
	parsed := paperlang.Parse("image.paper", source)
	result := Compile(parsed.AST)
	if !parsed.OK() || !result.OK() {
		t.Fatalf("diagnostics = %#v / %#v", parsed.Diagnostics, result.Diagnostics)
	}
	image := result.Document.Body[0].(layout.ImageBlock)
	if image.Format != "png" || len(image.Data) == 0 || image.Width != 40 || image.Height != 20 || image.Fit != layout.ImageFitCover ||
		!image.FocusSet || image.FocusX != 0.25 || image.FocusY != 0.75 || image.Alt != "Evidence chart" || image.Decorative ||
		len(image.Caption) != 1 || image.Caption[0].Text != "Figure 1" {
		t.Fatalf("image = %#v", image)
	}
	if len(result.Tree.Projection().Nodes) == 0 {
		t.Fatal("image was not retained in canonical tree")
	}
}

func TestCompileImagePreservesResponsiveWidthAndAutomaticHeight(t *testing.T) {
	source := "document:\n  page:\n    body:\n      image:\n        source: \"data:image/png;base64," + paperImagePNG + "\"\n        width: 50%\n        max-width: 100%\n        height: \"auto\"\n        alt: \"Responsive evidence\"\n"
	parsed := paperlang.Parse("responsive-image.paper", source)
	result := Compile(parsed.AST)
	if !parsed.OK() || !result.OK() {
		t.Fatalf("diagnostics = %#v / %#v", parsed.Diagnostics, result.Diagnostics)
	}
	image := result.Document.Body[0].(layout.ImageBlock)
	if image.Width != 0 || image.Height != 0 || image.WidthPercent != 50_000_000 || image.MaxWidthPercent != 100_000_000 {
		t.Fatalf("responsive image = %#v", image)
	}
}

func TestCompileImageRejectsPercentageHeightWithoutDefiniteFlowHeight(t *testing.T) {
	source := "document:\n  page:\n    body:\n      image:\n        source: \"data:image/png;base64," + paperImagePNG + "\"\n        width: 100%\n        height: 50%\n        alt: \"Evidence\"\n"
	result := Compile(paperlang.Parse("bad-height.paper", source).AST)
	if result.OK() || !hasCompileDiagnostic(result.Diagnostics, "PAPER_COMPILE_PERCENT_AXIS") {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
}

func TestCompileImageResolvesOnlyExplicitContentAddressedAssetReferences(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString(paperImagePNG)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	catalog, err := NewAssetCatalog([]AssetResource{{Name: "hero-image", MediaType: "image/png", Digest: hex.EncodeToString(digest[:]), Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	source := "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n        source: \"asset:hero-image\"\n        width: 40pt\n        height: 20pt\n        alt: \"Evidence chart\"\n"
	parsed := paperlang.Parse("asset.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse = %#v", parsed.Diagnostics)
	}
	if missing := Compile(parsed.AST); missing.OK() || len(missing.Document.Body) != 1 || len(missing.Document.Body[0].(layout.ImageBlock).Data) != 0 {
		t.Fatalf("ambient asset unexpectedly resolved: %#v", missing)
	}
	result := CompileWithAssets(parsed.AST, catalog)
	if !result.OK() {
		t.Fatalf("asset compile diagnostics = %#v", result.Diagnostics)
	}
	image := result.Document.Body[0].(layout.ImageBlock)
	if image.Format != "png" || string(image.Data) != string(data) || len(result.Mapping.Nodes) == 0 || result.Mapping.Nodes[len(result.Mapping.Nodes)-1].ResourceDigest != hex.EncodeToString(digest[:]) {
		t.Fatalf("resolved image/mapping = %#v / %#v", image, result.Mapping.Nodes)
	}
}

func TestCompileImageRejectsUnsafeOrAmbiguousResources(t *testing.T) {
	tests := []string{
		"source: \"assets/ambient.png\"\n        alt: \"x\"",
		"source: \"data:image/png;base64,bm90LXBuZw==\"\n        alt: \"x\"",
		"source: \"data:image/png;base64," + paperImagePNG + "\"",
		"source: \"data:image/png;base64," + paperImagePNG + "\"\n        decorative: true\n        alt: \"contradiction\"",
		"source: \"data:image/png;base64," + paperImagePNG + "\"\n        decorative: true\n        focus-x: 2",
		"source: \"data:image/png;base64," + strings.Repeat("A", 700000) + "\"\n        alt: \"too large\"",
	}
	for index, properties := range tests {
		source := "document:\n  page:\n    body:\n      image:\n        " + properties + "\n"
		parsed := paperlang.Parse("invalid-image.paper", source)
		if !parsed.OK() {
			t.Fatalf("case %d parse = %#v", index, parsed.Diagnostics)
		}
		if result := Compile(parsed.AST); result.OK() {
			t.Fatalf("case %d unexpectedly compiled", index)
		}
	}
}
