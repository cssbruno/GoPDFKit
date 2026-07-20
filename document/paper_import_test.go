// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"fmt"
	"testing"
)

func TestPlanPaperWithImportsUsesResolverForNamedStyle(t *testing.T) {
	source := `document:
  import: "design.paper"
  page:
    body:
      paragraph:
        style: "@body"
        text: "Imported design rule"
`
	plan, result, err := PlanPaperWithImports("main.paper", source, func(importerFile, importPath string) (string, string, error) {
		if importerFile != "main.paper" || importPath != "design.paper" {
			return "", "", fmt.Errorf("unexpected import %s from %s", importPath, importerFile)
		}
		return "design.paper", `document:
  style @body:
    font: "Courier"
    size: 11pt
`, nil
	})
	if err != nil || !result.OK() {
		t.Fatalf("PlanPaperWithImports() = %#v, %v", result, err)
	}
	if plan.PageCount() != 1 || plan.Hash() == "" {
		t.Fatalf("plan = pages %d hash %q", plan.PageCount(), plan.Hash())
	}
}
