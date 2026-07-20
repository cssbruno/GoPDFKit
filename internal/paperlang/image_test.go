// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package paperlang

import "testing"

func TestParseImageNodePreservesReadablePropertiesLosslessly(t *testing.T) {
	const source = "document @report:\n  page @sheet:\n    body @body:\n      image @hero:\n        source: \"data:image/png;base64,iVBORw0KGgo=\"\n        fit: \"cover\"\n        focus-x: 0.25\n        alt: \"Evidence\"\n"
	result := ParseLossless("image.paper", source)
	if !result.OK() || result.CST == nil || !result.Semantic.OK() {
		t.Fatalf("ParseLossless() = %#v / %v", result.Semantic.Diagnostics, result.Err)
	}
	image := result.Semantic.AST.Root.Members[0].Node.Members[0].Node.Members[0].Node
	if image.Kind != NodeImage || image.ID != "@hero" || len(image.Members) != 4 || image.Span.File != "image.paper" {
		t.Fatalf("image = %#v", image)
	}
	printed, err := PrintLossless(result.CST)
	if err != nil || string(printed) != source {
		t.Fatalf("PrintLossless() = %q, %v", printed, err)
	}
}
