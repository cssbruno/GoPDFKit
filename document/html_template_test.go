// SPDX-License-Identifier: LicenseRef-PaperRune-Health-Sector-Restricted-1.0
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cssbruno/paperrune/document"
)

const tinyHTMLTemplatePNG = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
	"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func ExampleCompileHTMLTemplate() {
	template, err := document.CompileHTMLTemplate(`
		<h1>{{title}}</h1>
		<p>Customer: {{customer}}</p>
		<p><a href="{{link}}">{{linkText}}</a></p>
		<table border="1" cellpadding="2">
			<tr><td>Total</td><td>{{total}}</td></tr>
		</table>
	`)
	if err != nil {
		panic(err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)

	html := pdf.HTMLNew()
	html.WriteTemplate(6, template, document.HTMLTemplateValues{
		"title":    "Invoice A-100",
		"customer": "Northwind",
		"link":     "https://example.com/invoices/A-100",
		"linkText": "Open invoice",
		"total":    "$84.00",
	})

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		panic(err)
	}
	fmt.Println(out.Len() > 0)
	// Output: true
}

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
			Style:     "",
		},
	})
	if err != nil {
		t.Fatalf("RenderHTMLTemplate() error = %v", err)
	}

	pdf := document.MustNew()
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

func TestCompiledHTMLTemplateRendersChangingValues(t *testing.T) {
	tmpl, err := document.CompileHTMLTemplate(`
		<style>h1 { color:#123456; } td { border:1pt solid #ccd3da; padding:2pt; }</style>
		<h1>{{title}}</h1>
		<p>Customer: {{customer}}</p>
		<p><a href="{{link}}">{{linkText}}</a></p>
		<table>
			<tr><td>Invoice</td><td>{{invoice}}</td></tr>
			<tr><td>Total</td><td>{{total}}</td></tr>
		</table>
		<img src="{{logo}}" alt="{{alt}}" width="4mm" height="4mm">
	`)
	if err != nil {
		t.Fatalf("CompileHTMLTemplate() error = %v", err)
	}

	render := func(values document.HTMLTemplateValues) string {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 12)
		html := pdf.HTMLNew()
		if err := html.WriteTemplateContext(context.Background(), 6, tmpl, values); err != nil {
			t.Fatalf("WriteTemplateContext() error = %v", err)
		}
		var out bytes.Buffer
		if err := pdf.Output(&out); err != nil {
			t.Fatalf("Output() error = %v", err)
		}
		return out.String()
	}

	first := render(document.HTMLTemplateValues{
		"title":    "Invoice A",
		"customer": "Acme & Sons",
		"link":     "https://example.com/a",
		"linkText": "View invoice A",
		"invoice":  "A-100",
		"total":    "$42.00",
		"logo":     tinyHTMLTemplatePNG,
		"alt":      "Acme logo",
	})
	for _, want := range []string{"Invoice A", "Acme & Sons", "View invoice A", "A-100", "$42.00", "/Subtype /Image"} {
		if !strings.Contains(first, want) {
			t.Fatalf("first render missing %q", want)
		}
	}

	second := render(document.HTMLTemplateValues{
		"title":    "Invoice B",
		"customer": "Northwind",
		"link":     "https://example.com/b",
		"linkText": "View invoice B",
		"invoice":  "B-200",
		"total":    "$84.00",
		"logo":     tinyHTMLTemplatePNG,
		"alt":      "Northwind logo",
	})
	for _, want := range []string{"Invoice B", "Northwind", "View invoice B", "B-200", "$84.00", "/Subtype /Image"} {
		if !strings.Contains(second, want) {
			t.Fatalf("second render missing %q", want)
		}
	}
	for _, stale := range []string{"Invoice A", "Acme & Sons", "A-100", "$42.00"} {
		if strings.Contains(second, stale) {
			t.Fatalf("second render contains stale first-render value %q", stale)
		}
	}
}

func TestCompiledHTMLTemplateMissingValueErrors(t *testing.T) {
	tmpl, err := document.CompileHTMLTemplate(`<p>{{name}}</p>`)
	if err != nil {
		t.Fatalf("CompileHTMLTemplate() error = %v", err)
	}
	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	err = html.WriteTemplateContext(context.Background(), 6, tmpl, nil)
	if err == nil || !strings.Contains(err.Error(), "missing HTML template value: name") {
		t.Fatalf("WriteTemplateContext() error = %v, want missing value", err)
	}
}

func TestCompiledHTMLTemplateRejectsStructuralSlots(t *testing.T) {
	_, err := document.CompileHTMLTemplate(`<p class="{{class}}">Name</p>`)
	if err == nil || !strings.Contains(err.Error(), "cannot replace class attributes") {
		t.Fatalf("CompileHTMLTemplate() error = %v, want class rejection", err)
	}

	_, err = document.CompileHTMLTemplate(`<style>.{{name}}{color:red}</style><p>Text</p>`)
	if err == nil || !strings.Contains(err.Error(), "cannot be used inside <style>") {
		t.Fatalf("CompileHTMLTemplate() style error = %v, want style rejection", err)
	}
}

func TestCompiledHTMLTemplateRejectsRawHTMLValues(t *testing.T) {
	tmpl, err := document.CompileHTMLTemplate(`<p>{{body}}</p>`)
	if err != nil {
		t.Fatalf("CompileHTMLTemplate() error = %v", err)
	}
	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	html := pdf.HTMLNew()
	err = html.WriteTemplateContext(context.Background(), 6, tmpl, document.HTMLTemplateValues{
		"body": document.HTMLTemplateRaw(`<strong>trusted</strong>`),
	})
	if err == nil || !strings.Contains(err.Error(), "HTMLTemplateRaw is not supported") {
		t.Fatalf("WriteTemplateContext() error = %v, want raw rejection", err)
	}
}
