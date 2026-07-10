// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
)

func TestExtractHTMLFooterBlock(t *testing.T) {
	body, footer := ExtractHTMLFooterBlock(`<section><p>Body</p></section><footer>Page footer</footer>`)
	if footer == nil {
		t.Fatal("footer = nil, want layout.FooterBlock")
	}
	if strings.Contains(body, "footer") || !strings.Contains(body, "Body") {
		t.Fatalf("body HTML = %q, want body without footer", body)
	}
	if len(footer.Blocks) != 1 {
		t.Fatalf("footer blocks = %d, want 1", len(footer.Blocks))
	}
	block, ok := footer.Blocks[0].(layout.ParagraphBlock)
	if !ok {
		t.Fatalf("footer block type = %T, want layout.ParagraphBlock", footer.Blocks[0])
	}
	if got := textSegmentsPlainText(block.Segments); got != "Page footer" {
		t.Fatalf("footer text = %q, want Page footer", got)
	}
	if !footer.ReservePageArea {
		t.Fatal("footer should reserve page area")
	}
}

func TestExtractHTMLFooterBlockNoFooter(t *testing.T) {
	html := `<p>Body only</p>`
	body, footer := ExtractHTMLFooterBlock(html)
	if footer != nil {
		t.Fatalf("footer = %#v, want nil", footer)
	}
	if body != html {
		t.Fatalf("body = %q, want original HTML", body)
	}
}

func TestExtractHTMLFooterBlockWithFooterMarkers(t *testing.T) {
	tests := []struct {
		name string
		html string
	}{
		{
			name: "data attribute",
			html: `<section><p>Body</p><div data-pdf-footer>Data footer</div></section>`,
		},
		{
			name: "class marker",
			html: `<section><p>Body</p><div class="document-footer">Class footer</div></section>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, footer := ExtractHTMLFooterBlock(tt.html)
			if footer == nil {
				t.Fatal("footer = nil, want layout.FooterBlock")
			}
			if strings.Contains(body, "footer") {
				t.Fatalf("body HTML = %q, want body without footer marker", body)
			}
			if len(footer.Blocks) != 1 {
				t.Fatalf("footer blocks = %d, want 1", len(footer.Blocks))
			}
		})
	}
}

func TestWriteDocumentRendersExtractedHTMLFooterBlock(t *testing.T) {
	_, footer := ExtractHTMLFooterBlock(`<p>Body</p><footer>Extracted footer</footer>`)
	pdf := MustNew()
	pdf.SetCompression(false)
	doc := layout.NewLayoutDocument()
	doc.Body = []layout.Block{layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Document body"}}}}
	doc.PageTemplate.Footer = footer

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(out.String(), "Extracted footer") {
		t.Fatal("PDF output missing extracted footer")
	}
}
