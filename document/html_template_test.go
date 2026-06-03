// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
)

const tinyHTMLTemplatePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
	"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestRenderHTMLTemplateEscapesPlainValues(t *testing.T) {
	got, err := document.RenderHTMLTemplate(`<p>{{name}}</p><p>{{count}}</p>`, document.HTMLTemplateValues{
		"name":  `<Northwind & Co>`,
		"count": 3,
	})
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}
	want := `<p>&lt;Northwind &amp; Co&gt;</p><p>3</p>`
	if got != want {
		t.Fatalf("RenderHTMLTemplate() = %q, want %q", got, want)
	}
}

func TestRenderHTMLTemplateSupportsRawAndDotKeys(t *testing.T) {
	got, err := document.RenderHTMLTemplate(`<section>{{.body}}</section>`, document.HTMLTemplateValues{
		"body": document.HTMLTemplateRaw(`<strong>Approved</strong>`),
	})
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}
	if got != `<section><strong>Approved</strong></section>` {
		t.Fatalf("RenderHTMLTemplate() = %q", got)
	}
}

func TestRenderHTMLTemplateImageValue(t *testing.T) {
	got, err := document.RenderHTMLTemplate(`<figure>{{logo}}</figure>`, document.HTMLTemplateValues{
		"logo": document.HTMLTemplateImage{
			Source:    `/tmp/logo.png`,
			Alt:       `A & B`,
			Width:     "40mm",
			Height:    "20mm",
			ObjectFit: "contain",
			Align:     "center",
			Style:     "margin: 4mm 0",
			Attributes: map[string]string{
				"data-id": "logo-1",
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}
	for _, want := range []string{
		`<img `,
		`src="/tmp/logo.png"`,
		`alt="A &amp; B"`,
		`width="40mm"`,
		`height="20mm"`,
		`align="center"`,
		`data-id="logo-1"`,
		`style="margin: 4mm 0; object-fit: contain"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("RenderHTMLTemplate() missing %q in %s", want, got)
		}
	}
}

func TestRenderHTMLTemplateMissingValueErrors(t *testing.T) {
	_, err := document.RenderHTMLTemplate(`<p>{{name}}</p>`, nil)
	if err == nil || !strings.Contains(err.Error(), "missing HTML template value: name") {
		t.Fatalf("RenderHTMLTemplate() error = %v, want missing value", err)
	}
}

func TestRenderHTMLTemplateImageRendersWithHTMLNew(t *testing.T) {
	fragment, err := document.RenderHTMLTemplate(`<p>Before</p>{{logo}}<p>After</p>`, document.HTMLTemplateValues{
		"logo": document.HTMLTemplateImage{
			Source:    tinyHTMLTemplatePNG,
			Alt:       "Logo",
			Width:     "12mm",
			Height:    "12mm",
			ObjectFit: "contain",
			Style:     "margin: 3mm 0 4mm 0",
		},
	})
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	pdf := document.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	html.Write(6, fragment)

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := out.String()
	if !strings.Contains(pdfText, "/Subtype /Image") {
		t.Fatal("generated PDF does not contain template image")
	}
	if !strings.Contains(pdfText, "Before") || !strings.Contains(pdfText, "After") {
		t.Fatal("generated PDF does not contain surrounding template text")
	}
}
