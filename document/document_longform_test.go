// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document

import (
	"bytes"
	"strings"
	"testing"
)

func TestLongFormHTMLDocumentModel(t *testing.T) {
	doc, messages := LongFormHTMLDocumentModel("Long Form", `
		<h2>Clause One</h2>
		<p>Body text</p>
		<ol><li>First</li><li>Second</li></ol>
		<table><thead><tr><th>Name</th></tr></thead><tbody><tr><td>Alpha</td></tr></tbody></table>
		<footer>Footer text</footer>
	`)
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want none", messages)
	}
	if doc.Kind != DocumentKindLongForm {
		t.Fatalf("kind = %q, want long-form", doc.Kind)
	}
	if doc.PageTemplate.Footer == nil {
		t.Fatal("footer = nil, want extracted footer")
	}
	if len(doc.Body) < 5 {
		t.Fatalf("body blocks = %d, want title, heading, paragraph, list, table", len(doc.Body))
	}
}

func TestLongFormHTMLDocumentModelReportsUnsupportedHTML(t *testing.T) {
	_, messages := LongFormHTMLDocumentModel("Long Form", `<p>Body</p><video>clip</video>`)
	if len(messages) == 0 {
		t.Fatal("messages = none, want unsupported video diagnostic")
	}
}

func TestWriteDocumentRendersLongFormHTMLDocumentModel(t *testing.T) {
	doc, messages := LongFormHTMLDocumentModel("Long Form", `
		<h2>Clause One</h2>
		<p>Body text</p>
		<ul><li>First</li><li>Second</li></ul>
		<footer>Footer text</footer>
	`)
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want none", messages)
	}
	pdf := New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.WriteDocument(doc)
	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	content := out.String()
	for _, want := range []string{"Long Form", "Clause One", "Body text", "First", "Footer text"} {
		if !strings.Contains(content, want) {
			t.Fatalf("PDF output missing long-form content %q", want)
		}
	}
}
