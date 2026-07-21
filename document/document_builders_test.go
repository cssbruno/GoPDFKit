// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/layout"
)

func TestNewDocumentModel(t *testing.T) {
	doc := layout.NewDocumentModel("Custom Document", layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body text"}}})
	if doc.Title != "Custom Document" {
		t.Fatalf("title = %q, want Custom Document", doc.Title)
	}
	if len(doc.Body) != 2 {
		t.Fatalf("body blocks = %d, want title heading and body block", len(doc.Body))
	}
	heading, ok := doc.Body[0].(layout.HeadingBlock)
	if !ok {
		t.Fatalf("body[0] = %T, want layout.HeadingBlock", doc.Body[0])
	}
	if heading.Level != 1 || len(heading.Segments) != 1 || heading.Segments[0].Text != "Custom Document" {
		t.Fatalf("heading = %#v, want title heading", heading)
	}

	pdf := MustNew()
	pdf.SetCompression(false)
	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	for _, want := range []string{"Custom Document", "Body text"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("PDF output missing %q", want)
		}
	}
}

func TestNewDocumentModelWithoutTitle(t *testing.T) {
	doc := layout.NewDocumentModel("", layout.ParagraphBlock{Segments: []layout.TextSegment{{Text: "Body text"}}})
	if doc.Title != "" {
		t.Fatalf("title = %q, want empty", doc.Title)
	}
	if len(doc.Body) != 1 {
		t.Fatalf("body blocks = %d, want only supplied body block", len(doc.Body))
	}
}
