// SPDX-License-Identifier: LicenseRef-GoPDFKit-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"context"
	"testing"
)

func TestPaperPlanExplainCarriesBindingAndTokenProvenance(t *testing.T) {
	source := "document @report:\n" +
		"  theme: \"@print\"\n" +
		"  theme @base:\n" +
		"    token @font:\n      type: \"string\"\n      value: \"Courier\"\n" +
		"    token @size:\n      type: \"length\"\n      value: 11pt\n" +
		"    token @leading:\n      type: \"length\"\n      value: 14pt\n" +
		"    token @ink:\n      type: \"color\"\n      value: \"#336699\"\n" +
		"  theme @print:\n" +
		"    parent: \"base\"\n" +
		"    token @print-ink:\n      type: \"color\"\n      reference: \"ink\"\n" +
		"  schema @invoice:\n" +
		"    field @total:\n      type: \"number\"\n" +
		"  page:\n" +
		"    width: 160pt\n    height: 80pt\n    margin: 8pt\n" +
		"    body:\n" +
		"      paragraph @message:\n" +
		"        bind: \"@invoice.total\"\n" +
		"        font-token: \"font\"\n" +
		"        size-token: \"size\"\n" +
		"        line-height-token: \"leading\"\n" +
		"        color-token: \"print-ink\"\n" +
		"        text: \"Visible\"\n"

	plan, planned, err := PlanPaper("provenance.paper", source)
	if err != nil || !planned.OK() {
		t.Fatalf("PlanPaper() = %#v, %v", planned, err)
	}
	provenance, err := plan.Provenance()
	if err != nil || len(provenance.Bindings) != 1 || len(provenance.StyleTokens) != 4 || len(provenance.ComputedStyles) != 1 {
		t.Fatalf("provenance = %#v, %v", provenance, err)
	}
	if style := provenance.ComputedStyles[0]; style.Node != "@message" || style.TextStyle == nil || style.TextStyle.FontFamily != "Courier" || style.TextStyle.FontSize != 11 || style.TextStyle.LineHeight != 14 {
		t.Fatalf("computed style provenance = %#v", style)
	}
	if binding := provenance.Bindings[0]; binding.Node != "@message" || binding.Path != "@invoice.total" || binding.Kind != "paragraph" || binding.Source.StartLine == 0 {
		t.Fatalf("binding provenance = %#v", binding)
	}
	var color PaperPlanStyleTokenProvenance
	for _, property := range provenance.StyleTokens {
		if property.Property == "color-token" {
			color = property
		}
	}
	if color.Node != "@message" || color.Theme != "print" || color.Token != "print-ink" || color.Value != "#336699" || len(color.TokenChain) != 2 || color.TokenChain[1].Theme != "base" {
		t.Fatalf("color provenance = %#v", color)
	}

	provenance.Bindings[0].Path = "@mutated"
	again, err := plan.Provenance()
	if err != nil || again.Bindings[0].Path != "@invoice.total" || again.StyleTokens[0].TokenChain[0].Scope != nil {
		t.Fatalf("provenance was not detached = %#v, %v", again, err)
	}

	explanation, err := plan.ExplainContext(context.Background(), []PaperPlanSelector{{Key: "@message", MaxResults: 16}}, 1, 1<<20, 1<<20)
	if err != nil || !bytes.Contains(explanation.JSON(), []byte(`"provenance":{"bindings"`)) || !bytes.Contains(explanation.JSON(), []byte(`"path":"@invoice.total"`)) || !bytes.Contains(explanation.JSON(), []byte(`"style_tokens"`)) || !bytes.Contains(explanation.JSON(), []byte(`"computed_styles"`)) {
		t.Fatalf("explanation provenance = %s / %v", explanation.JSON(), err)
	}
}
