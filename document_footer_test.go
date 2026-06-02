// Copyright (c) 2026 cssBruno

package gopdfkit

import (
	"bytes"
	"strings"
	"testing"
)

func TestExtractHTMLFooterBlock(t *testing.T) {
	body, footer := ExtractHTMLFooterBlock(`<section><p>Body</p></section><footer>Page footer</footer>`)
	if footer == nil {
		t.Fatal("footer = nil, want FooterBlock")
	}
	if strings.Contains(body, "footer") || !strings.Contains(body, "Body") {
		t.Fatalf("body HTML = %q, want body without footer", body)
	}
	if len(footer.Blocks) != 1 {
		t.Fatalf("footer blocks = %d, want 1", len(footer.Blocks))
	}
	block, ok := footer.Blocks[0].(ParagraphBlock)
	if !ok {
		t.Fatalf("footer block type = %T, want ParagraphBlock", footer.Blocks[0])
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

func TestWriteDocumentRendersExtractedHTMLFooterBlock(t *testing.T) {
	_, footer := ExtractHTMLFooterBlock(`<p>Body</p><footer>Extracted footer</footer>`)
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	doc := NewDocument(DocumentKindGeneric)
	doc.Body = []Block{ParagraphBlock{Segments: []TextSegment{{Text: "Document body"}}}}
	doc.Footer = footer

	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(out.String(), "Extracted footer") {
		t.Fatal("PDF output missing extracted footer")
	}
}
