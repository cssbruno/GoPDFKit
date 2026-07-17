// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package papercompile

import (
	"fmt"
	"testing"

	"github.com/cssbruno/gopdfkit/internal/paperlang"
	"github.com/cssbruno/gopdfkit/layout"
)

func TestCompileNamedStyleAppliesRuleBeforeLocalOverrides(t *testing.T) {
	source := `document:
  style @body:
    font: "Helvetica"
    size: 12pt
    color: "#112233"
  page:
    body:
      paragraph:
        style: "@body"
        size: 18pt
        text: "Reusable"
`
	parsed := paperlang.Parse("local.paper", source)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics: %#v", parsed.Diagnostics)
	}
	compiled := Compile(parsed.AST)
	if !compiled.OK() {
		t.Fatalf("compile diagnostics: %#v", compiled.Diagnostics)
	}
	if len(compiled.Document.Body) != 1 {
		t.Fatalf("body count = %d, want 1", len(compiled.Document.Body))
	}
	paragraph, ok := compiled.Document.Body[0].(layout.ParagraphBlock)
	if !ok {
		t.Fatalf("body[0] = %T, want paragraph", compiled.Document.Body[0])
	}
	if paragraph.Style.FontFamily != "Helvetica" || paragraph.Style.FontSize != 18 || paragraph.Style.Color.R != 17 || paragraph.Style.Color.G != 34 || paragraph.Style.Color.B != 51 {
		t.Fatalf("compiled style = %#v, want named rule with local size override", paragraph.Style)
	}
}

func TestCompileResolverImportsDesignRules(t *testing.T) {
	mainSource := `document:
  import: "../styles/design.paper"
  page:
    body:
      paragraph:
        style: "@body"
        text: "Imported"
`
	designSource := `document:
  style @body:
    font: "Courier"
    size: 10pt
    line-height: 14pt
`
	parsed := paperlang.Parse("docs/main.paper", mainSource)
	if !parsed.OK() {
		t.Fatalf("parse diagnostics: %#v", parsed.Diagnostics)
	}
	compiled := CompileWithResolver(parsed.AST, func(importerFile, importPath string) (string, string, error) {
		if importerFile != "docs/main.paper" || importPath != "../styles/design.paper" {
			return "", "", fmt.Errorf("unexpected import %s from %s", importPath, importerFile)
		}
		return "styles/design.paper", designSource, nil
	})
	if !compiled.OK() {
		t.Fatalf("compile diagnostics: %#v", compiled.Diagnostics)
	}
	paragraph := compiled.Document.Body[0].(layout.ParagraphBlock)
	if paragraph.Style.FontFamily != "Courier" || paragraph.Style.FontSize != 10 || paragraph.Style.LineHeight != 14 {
		t.Fatalf("imported style = %#v", paragraph.Style)
	}
}

func TestCompileImportRequiresExplicitResolver(t *testing.T) {
	parsed := paperlang.Parse("main.paper", `document:
  import: "design.paper"
  page:
    body:
      paragraph:
        text: "No resolver"
`)
	compiled := Compile(parsed.AST)
	if compiled.OK() {
		t.Fatal("compile unexpectedly succeeded without an import resolver")
	}
	for _, diagnostic := range compiled.Diagnostics {
		if diagnostic.Code == "PAPER_IMPORT_RESOLVER" {
			return
		}
	}
	t.Fatalf("diagnostics = %#v, want PAPER_IMPORT_RESOLVER", compiled.Diagnostics)
}
