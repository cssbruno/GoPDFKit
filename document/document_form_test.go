// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/layout"
)

func testFormDocument() FormDocument {
	return FormDocument{
		Title: "Form Title",
		Sections: []FormSection{
			{
				Title:        "Profile",
				KeepTogether: true,
				Questions: []FormQuestion{
					{Label: "Name", Required: true, Answer: FormAnswer{Text: "Alex Example"}},
					{Label: "Options", Answer: FormAnswer{Items: []string{"One", "Two"}}},
					{Label: "Scores", Answer: FormAnswer{Table: [][]string{{"Name", "Score"}, {"A", "10"}}}},
				},
			},
			{
				Title:       "Next page",
				BreakBefore: true,
				Questions: []FormQuestion{
					{Label: "Comment", Answer: FormAnswer{Text: "Continue"}},
				},
			},
		},
	}
}

func TestFormDocumentHTMLCanonicalOutput(t *testing.T) {
	html := FormDocumentHTML(testFormDocument())
	for _, want := range []string{
		"<h1>Form Title</h1>",
		`<section class="form-section" style="break-inside: avoid">`,
		`<dl class="form-qa">`,
		"<dt>Name *</dt>",
		`<ul class="form-answer-list">`,
		`<table class="form-answer-table"><tbody>`,
		`style="break-before: page"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("form HTML missing %q in %s", want, html)
		}
	}
}

func TestFormDocumentHTMLValidation(t *testing.T) {
	if messages := ValidateFormDocumentHTML(testFormDocument()); len(messages) != 0 {
		t.Fatalf("ValidateFormDocumentHTML messages = %#v, want none", messages)
	}
}

func TestFormDocumentBlocks(t *testing.T) {
	blocks := FormDocumentBlocks(testFormDocument())
	if len(blocks) != 3 {
		t.Fatalf("blocks = %d, want title plus two sections", len(blocks))
	}
	if got := blocks[0].DocumentBlockKind(); got != layout.BlockKindHeading {
		t.Fatalf("first block kind = %q, want heading", got)
	}
	section, ok := blocks[1].(layout.SectionBlock)
	if !ok {
		t.Fatalf("second block = %T, want layout.SectionBlock", blocks[1])
	}
	if !section.Box.KeepTogether {
		t.Fatal("first section should keep grouped questions together")
	}
}

func TestWriteDocumentRendersFormDocumentModel(t *testing.T) {
	pdf := MustNew()
	pdf.SetCompression(false)
	doc := FormDocumentModel(testFormDocument())

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := extractedDocumentText(t, out.Bytes())
	for _, want := range []string{"Form Title", "Name *", "Alex Example", "Options", "Score", "Comment"} {
		if !strings.Contains(content, want) {
			t.Fatalf("PDF output missing form content %q", want)
		}
	}
	if pdf.PageCount() < 2 {
		t.Fatalf("PageCount() = %d, want at least 2 from form page-break policy", pdf.PageCount())
	}
}
