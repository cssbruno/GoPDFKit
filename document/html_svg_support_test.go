// SPDX-License-Identifier: MIT
// Copyright (c) 2026 cssBruno

package document_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/cssbruno/gopdfkit/document"
)

func TestHTMLTokenizeAttributesAndEntities(t *testing.T) {
	segments := document.HTMLTokenize(`<a href="https://example.test?q=a b" title='A &amp; B'>A&nbsp;B</a><br/>`)
	if len(segments) != 5 {
		t.Fatalf("len(segments) = %d, want 5", len(segments))
	}
	if segments[0].Cat != 'O' || segments[0].Str != "a" {
		t.Fatalf("first segment = %#v, want open anchor", segments[0])
	}
	if got := segments[0].Attr["href"]; got != "https://example.test?q=a b" {
		t.Fatalf("href = %q", got)
	}
	if got := segments[0].Attr["title"]; got != "A & B" {
		t.Fatalf("title = %q", got)
	}
	if got := segments[1].Str; got != "A\u00a0B" {
		t.Fatalf("text = %q", got)
	}
	if segments[3].Cat != 'O' || segments[3].Str != "br" || segments[4].Cat != 'C' || segments[4].Str != "br" {
		t.Fatalf("self-closing br segments = %#v %#v", segments[3], segments[4])
	}
}

func TestHTMLTokenizeQuotedTagEndAndComments(t *testing.T) {
	segments := document.HTMLTokenize(`<p title="a > b">keep</p><!-- skip <b>comment</b> --><span data-x='c > d'>tail</span>`)
	if len(segments) != 6 {
		t.Fatalf("len(segments) = %d, want 6: %#v", len(segments), segments)
	}
	if got := segments[0].Attr["title"]; got != "a > b" {
		t.Fatalf("quoted title = %q", got)
	}
	if got := segments[3].Attr["data-x"]; got != "c > d" {
		t.Fatalf("quoted data-x = %q", got)
	}
	for _, segment := range segments {
		if strings.Contains(segment.Str, "comment") {
			t.Fatalf("comment leaked into tokens: %#v", segments)
		}
	}
}

func TestHTMLWriteStyledContent(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<h2>Title</h2><p style="text-align:center;color:#123456">`+
		`<strong>Bold</strong> <em>italic</em> <span style="font-size:14pt;text-decoration:underline">under</span>`+
		` <a href="https://example.test?a=1&amp;b=2">link</a></p><ol><li>first</li><li>second</li></ol>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("generated empty PDF")
	}
}

func TestHTMLWriteListMarkers(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<style>ol.css-alpha { list-style-type: lower-alpha; }</style>`+
		`<ol start="3"><li>third</li><li>fourth</li></ol>`+
		`<ol type="A" start="2"><li>upper alpha</li></ol>`+
		`<ol type="i" start="4"><li>roman</li></ol>`+
		`<ul type="square"><li>square item<ul><li>nested item</li></ul></li></ul>`+
		`<ol class="css-alpha" start="2"><li>css alpha</li></ol>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"3. ", "third", "4. ", "fourth", "B. ", "upper alpha", "iv. ", "roman", "* ", "square item", "- ", "nested item", "b. ", "css alpha"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain list marker/text %q", want)
		}
	}
}

func TestHTMLWriteDefinitionList(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<dl>`+
		`<dt>Term</dt><dd>Definition text</dd>`+
		`<dt><em>Other</em></dt><dd><span style="color:#123456">More detail</span></dd>`+
		`</dl>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Term", "Definition text", "Other", "More detail"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain definition-list text %q", want)
		}
	}
}

func TestHTMLWriteEmbeddedImage(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<p>Before image</p><a href="https://example.test/image">`+
		`<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ`+
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==" width="24" height="24"/>`+
		`</a><p>After image</p>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "/Subtype /Image") {
		t.Fatal("generated PDF does not contain embedded image")
	}
	if !strings.Contains(pdfText, "Before image") || !strings.Contains(pdfText, "After image") {
		t.Fatal("generated PDF does not contain surrounding text")
	}
}

func TestHTMLWriteFigureCaption(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<figure><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ`+
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==" width="12" height="12"/>`+
		`<figcaption>Figure caption text</figcaption></figure>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "/Subtype /Image") {
		t.Fatal("generated PDF does not contain embedded figure image")
	}
	if !strings.Contains(pdfText, "Figure caption text") {
		t.Fatal("generated PDF does not contain figure caption")
	}
}

func TestHTMLWriteImageAltFallback(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<p>Before</p><img alt="missing image text"/><p>After</p>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Before", "missing image text", "After"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain alt fallback text %q", want)
		}
	}
	if strings.Contains(pdfText, "/Subtype /Image") {
		t.Fatal("generated PDF unexpectedly contains an embedded image")
	}
}

func TestHTMLWriteInvalidDataImageErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "not-base64",
			src:  "data:image/png,not-base64",
			want: "must be base64",
		},
		{
			name: "unsupported-mime",
			src:  "data:image/svg+xml;base64,PHN2Zy8+",
			want: "unsupported HTML image type",
		},
		{
			name: "invalid-base64",
			src:  "data:image/png;base64,%",
			want: "invalid HTML image data URI",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdf := document.MustNew()
			pdf.SetCompression(false)
			pdf.AddPage()
			pdf.SetFont("Helvetica", "", 12)
			_, lineHeight := pdf.GetFontSize()
			html := pdf.HTMLNew()
			html.Write(lineHeight, `<img src="`+tt.src+`"/>`)

			var output bytes.Buffer
			err := pdf.Output(&output)
			if err == nil {
				t.Fatal("Output() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Output() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestHTMLWriteRejectsUnsafeImageSources(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "remote-http", src: "http://example.test/image.png", want: "remote HTML images are disabled"},
		{name: "remote-https", src: "https://example.test/image.png", want: "remote HTML images are disabled"},
		{name: "file-url", src: "file:///tmp/image.png", want: "file URL HTML images are disabled"},
		{name: "local-path", src: "/tmp/image.png", want: "local HTML images are disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pdf := document.MustNew()
			pdf.SetCompression(false)
			pdf.AddPage()
			pdf.SetFont("Helvetica", "", 12)
			_, lineHeight := pdf.GetFontSize()
			html := pdf.HTMLNew()
			html.Write(lineHeight, `<img src="`+tt.src+`"/>`)

			var output bytes.Buffer
			err := pdf.Output(&output)
			if err == nil {
				t.Fatal("Output() error = nil, want unsafe image source error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Output() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestHTMLWriteRejectsOversizedDataImage(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.MaxDataImageBytes = 4
	html.Write(lineHeight, `<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ`+
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="/>`)

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want oversized data image error")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Fatalf("Output() error = %q, want maximum size error", err)
	}
}

func TestHTMLWriteRejectsUnsafeLinkScheme(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<a href="javascript:alert(1)">bad link</a>`)

	var output bytes.Buffer
	err := pdf.Output(&output)
	if err == nil {
		t.Fatal("Output() error = nil, want unsafe link error")
	}
	if !strings.Contains(err.Error(), "unsupported HTML link scheme") {
		t.Fatalf("Output() error = %q, want unsupported link scheme error", err)
	}
}

func TestHTMLWriteTable(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1" cellpadding="4" width="100%">`+
		`<tr bgcolor="#eeeeee"><th width="30%">Name</th><th>Notes</th></tr>`+
		`<tr><td style="vertical-align:bottom; padding-left:8px">Alice</td><td style="text-align:right">first<br/>second</td></tr>`+
		`<tr><td colspan="2">Full row</td></tr>`+
		`</table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Name", "Notes", "Alice", "first", "second", "Full row"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain table text %q", want)
		}
	}
	if !strings.Contains(pdfText, " re ") {
		t.Fatal("generated PDF does not contain table cell rectangles")
	}
}

func TestHTMLWriteTableRowspan(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1" cellpadding="4" width="100%">`+
		`<tr><td rowspan="2">Category</td><td>First</td></tr>`+
		`<tr><td>Second</td></tr>`+
		`</table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Category", "First", "Second"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain rowspan table text %q", want)
		}
	}
	if strings.Count(pdfText, "Category") != 1 {
		t.Fatalf("rowspan cell rendered %d times, want once", strings.Count(pdfText, "Category"))
	}
}

func TestHTMLWriteTableCellBorderAndBackground(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table cellpadding="4" width="100%">`+
		`<tr><td style="border:1px solid #123456; background-color:#eeeeee">Boxed</td><td>Plain</td></tr>`+
		`</table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "Boxed") || !strings.Contains(pdfText, "Plain") {
		t.Fatal("generated PDF does not contain cell text")
	}
	if !strings.Contains(pdfText, " re ") {
		t.Fatal("generated PDF does not contain cell rectangle")
	}
}

func TestHTMLWriteTableRepeatsHeaderRowsAcrossPages(t *testing.T) {
	pdf := document.MustNew(
		document.WithUnit(document.UnitMillimeter),
		document.WithCustomPageSize(document.Size{Wd: 80, Ht: 70}),
	)
	pdf.SetCompression(false)
	pdf.SetAutoPageBreak(true, 5)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()

	var rows strings.Builder
	for i := 1; i <= 10; i++ {
		rows.WriteString(`<tr><td>Row `)
		rows.WriteString(strconv.Itoa(i))
		rows.WriteString(`</td><td>Value</td></tr>`)
	}

	html := pdf.HTMLNew()
	html.Write(lineHeight, `<table border="1" cellpadding="4" width="100%">`+
		`<thead><tr><th>Name</th><th>Notes</th></tr></thead>`+
		`<tbody>`+rows.String()+`</tbody>`+
		`</table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if count := strings.Count(pdfText, "Name"); count < 2 {
		t.Fatalf("table header rendered %d time(s), want repeated after page break", count)
	}
}

func TestHTMLWriteTextSemantics(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<p><code>code</code> <s>gone</s> H<sub>2</sub>O E=mc<sup>2</sup></p>`+
		`<pre> a  b`+"\n"+` c</pre>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"code", "gone", "H", "2", "O", "E=mc", " a  b", " c"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain semantic text %q", want)
		}
	}
}

func TestHTMLWriteInlineSVG(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<p>Before SVG</p><a href="https://example.test/svg">`+
		`<svg width="48" height="24" viewBox="0 0 48 24">`+
		`<rect x="1" y="1" width="46" height="22" fill="#00ff00" stroke="#000"/>`+
		`<text x="24" y="16" text-anchor="middle" font-size="10">Inline SVG</text>`+
		`</svg></a><p>After SVG</p>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Before SVG", "Inline SVG", "After SVG"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain inline SVG text %q", want)
		}
	}
	if !strings.Contains(pdfText, " m\n") || !strings.Contains(pdfText, " l\n") || !strings.Contains(pdfText, "\nh\nB") {
		t.Fatal("generated PDF does not contain inline SVG path output")
	}
}

func TestHTMLWriteCompiledRendersRepeatedFragment(t *testing.T) {
	compiled, err := document.CompileHTML(`<style>
		.note { font-weight: bold; }
		td { padding: 2px; border: 1px solid #333; }
	</style>` +
		`<h2>Compiled Fragment</h2>` +
		`<p class="note">Paragraph text</p>` +
		`<svg width="32" height="16" viewBox="0 0 32 16">` +
		`<rect x="1" y="1" width="30" height="14" fill="#eeeeee" stroke="#333"/>` +
		`<text x="16" y="11" text-anchor="middle" font-size="6">svg</text>` +
		`</svg>` +
		`<table><tr><td>Cell text</td></tr></table>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}
	stats := compiled.Stats()
	if stats.Nodes == 0 || stats.Tables != 1 || stats.InlineSVGs != 1 || stats.CSSRules == 0 || stats.CachedText == 0 {
		t.Fatalf("compiled stats = %#v, want populated node/table/svg/css/text counts", stats)
	}
	if dump := compiled.DebugDump(); !strings.Contains(dump, "table token=") || !strings.Contains(dump, "svg token=") {
		t.Fatalf("DebugDump() = %q, want table and svg nodes", dump)
	}

	for i := 0; i < 2; i++ {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 12)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)

		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			t.Fatalf("Output() render %d error = %v", i, err)
		}
		pdfText := output.String()
		for _, want := range []string{"Compiled Fragment", "Paragraph text", "svg", "Cell text"} {
			if !strings.Contains(pdfText, want) {
				t.Fatalf("render %d generated PDF does not contain %q", i, want)
			}
		}
	}
}

func TestHTMLWriteCompiledConcurrentReuse(t *testing.T) {
	compiled, err := document.CompileHTML(`<style>
		.note { color: #112233; font-weight: bold; }
		td { padding: 1px; border: 1px solid #333; }
	</style><h2>Concurrent Fragment</h2><p class="note">Shared compiled plan</p><table><tr><td>Concurrent cell</td></tr></table>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}

	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pdf := document.MustNew()
			pdf.SetCompression(false)
			pdf.AddPage()
			pdf.SetFont("Helvetica", "", 12)
			_, lineHeight := pdf.GetFontSize()
			html := pdf.HTMLNew()
			html.WriteCompiled(lineHeight, compiled)

			var output bytes.Buffer
			if err := pdf.Output(&output); err != nil {
				errs <- err
				return
			}
			pdfText := output.String()
			for _, want := range []string{"Concurrent Fragment", "Shared compiled plan", "Concurrent cell"} {
				if !strings.Contains(pdfText, want) {
					errs <- fmt.Errorf("generated PDF does not contain %q", want)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent WriteCompiled error = %v", err)
	}
}

func TestCompileHTMLHandlesDeeplyNestedFragment(t *testing.T) {
	var fragment strings.Builder
	for i := 0; i < 96; i++ {
		fragment.WriteString(`<section class="level">`)
	}
	fragment.WriteString(`<p>Deep text</p>`)
	for i := 0; i < 96; i++ {
		fragment.WriteString(`</section>`)
	}

	compiled, err := document.CompileHTML(fragment.String())
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}
	if stats := compiled.Stats(); stats.MaxDepth < 96 || stats.Recovery != 0 {
		t.Fatalf("Stats() = %#v, want deep tree without recovery", stats)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.WriteCompiled(lineHeight, compiled)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(output.String(), "Deep text") {
		t.Fatal("generated PDF does not contain deeply nested text")
	}
}

func TestCompileHTMLIgnoresDoctypeCommentsAndHeadContent(t *testing.T) {
	compiled, err := document.CompileHTML(`<!doctype html><!-- hidden comment --><html><head><title>Hidden title</title><style>.x{color:red}</style><script>hiddenScript()</script></head><body><p>Visible body</p></body></html>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.WriteCompiled(lineHeight, compiled)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "Visible body") {
		t.Fatal("generated PDF does not contain visible body text")
	}
	for _, blocked := range []string{"doctype", "hidden comment", "Hidden title", "hiddenScript", "color:red"} {
		if strings.Contains(pdfText, blocked) {
			t.Fatalf("generated PDF leaked ignored HTML content %q", blocked)
		}
	}
}

func TestCompileHTMLSkipsHiddenInlineSVG(t *testing.T) {
	compiled, err := document.CompileHTML(`<head><svg><path d="bad path"/></svg></head><p>Visible</p>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.WriteCompiled(lineHeight, compiled)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "Visible") {
		t.Fatal("generated PDF does not contain visible text")
	}
	if strings.Contains(pdfText, "bad path") {
		t.Fatal("generated PDF leaked hidden SVG text")
	}
}

func TestCompileHTMLReportsMalformedRecovery(t *testing.T) {
	for name, fragment := range map[string]string{
		"misnested":       `<div><p>Open <strong>strong</p><span>tail`,
		"unexpectedClose": `<p>Text</section><em>tail</em>`,
		"unclosedDeep":    `<section><article><p><span>tail`,
	} {
		t.Run(name, func(t *testing.T) {
			compiled, err := document.CompileHTML(fragment)
			if err != nil {
				t.Fatalf("CompileHTML() error = %v", err)
			}
			issues := compiled.RecoveryIssues()
			if len(issues) == 0 {
				t.Fatal("RecoveryIssues() is empty for malformed fragment")
			}
			stats := compiled.Stats()
			if stats.Recovery != len(issues) {
				t.Fatalf("Stats().Recovery = %d, want %d", stats.Recovery, len(issues))
			}
		})
	}
}

func TestCompileHTMLHandlesMalformedAttributes(t *testing.T) {
	compiled, err := document.CompileHTML(`<p class="note data-x='broken>tail</p>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}
	if stats := compiled.Stats(); stats.Tokens == 0 {
		t.Fatalf("Stats() = %#v, want tokenizer to preserve malformed input as recoverable content", stats)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.WriteCompiled(lineHeight, compiled)
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
}

func FuzzCompileHTMLRecovery(f *testing.F) {
	for _, seed := range []string{
		`<p>plain</p>`,
		`<div><p><strong>misnested</p></div>`,
		`<!doctype html><!-- comment --><head><title>x</title></head><body><p>visible</p></body>`,
		`<table><tr><td rowspan="2">x<td>y</table>`,
		`<img src="data:image/png;base64,%">`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, fragment string) {
		compiled, err := document.CompileHTML(fragment)
		if err != nil {
			return
		}
		stats := compiled.Stats()
		if stats.Tokens < 0 || stats.Nodes < 0 || stats.Recovery < 0 {
			t.Fatalf("negative compiled stats: %#v", stats)
		}
		if len(compiled.RecoveryIssues()) != stats.Recovery {
			t.Fatalf("RecoveryIssues len = %d, Stats().Recovery = %d", len(compiled.RecoveryIssues()), stats.Recovery)
		}
	})
}

func TestCompileHTMLDataURIImages(t *testing.T) {
	var jpegData bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0xff})
	if err := jpeg.Encode(&jpegData, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() error = %v", err)
	}
	pngDataURI := `data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ` +
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==`
	jpegDataURI := `data:image/jpeg;base64,` + base64.StdEncoding.EncodeToString(jpegData.Bytes())

	for _, tt := range []struct {
		name string
		src  string
	}{
		{name: "png", src: pngDataURI},
		{name: "jpeg", src: jpegDataURI},
	} {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := document.CompileHTML(`<p>Before</p><img src="` + tt.src + `" width="8" height="8"/><p>After</p>`)
			if err != nil {
				t.Fatalf("CompileHTML() error = %v", err)
			}
			if stats := compiled.Stats(); stats.Images != 1 {
				t.Fatalf("Stats().Images = %d, want 1", stats.Images)
			}
			pdf := document.MustNew()
			pdf.SetCompression(false)
			pdf.AddPage()
			pdf.SetFont("Helvetica", "", 12)
			_, lineHeight := pdf.GetFontSize()
			html := pdf.HTMLNew()
			html.WriteCompiled(lineHeight, compiled)

			var output bytes.Buffer
			if err := pdf.Output(&output); err != nil {
				t.Fatalf("Output() error = %v", err)
			}
			pdfText := output.String()
			if !strings.Contains(pdfText, "/Subtype /Image") || !strings.Contains(pdfText, "Before") || !strings.Contains(pdfText, "After") {
				t.Fatalf("generated PDF missing image or surrounding text for %s", tt.name)
			}
		})
	}
}

func TestCompileHTMLRejectsInvalidDataURIImages(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want string
	}{
		{name: "not-base64", src: "data:image/png,not-base64", want: "must be base64"},
		{name: "unsupported-mime", src: "data:image/svg+xml;base64,PHN2Zy8+", want: "unsupported HTML image type"},
		{name: "invalid-base64", src: "data:image/png;base64,%", want: "invalid HTML image data URI"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := document.CompileHTML(`<img src="` + tt.src + `"/>`)
			if err == nil {
				t.Fatal("CompileHTML() error = nil, want data URI error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("CompileHTML() error = %q, want containing %q", err, tt.want)
			}
		})
	}
}

func TestWriteCompiledEnforcesCustomDataImageLimit(t *testing.T) {
	compiled, err := document.CompileHTML(`<img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ` +
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="/>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}
	if stats := compiled.Stats(); stats.Images != 1 {
		t.Fatalf("compiled image count = %d, want 1", stats.Images)
	}
	pdf := document.MustNew()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.MaxDataImageBytes = 1
	html.WriteCompiled(lineHeight, compiled)
	if !pdf.Err() {
		t.Fatal("WriteCompiled with custom MaxDataImageBytes error = nil")
	}
	if !strings.Contains(pdf.Error().Error(), "exceeds maximum size") {
		t.Fatalf("WriteCompiled error = %v, want maximum size", pdf.Error())
	}
}

func TestWriteCompiledDataImageReuseDoesNotMutateCachedBytes(t *testing.T) {
	compiled, err := document.CompileHTML(`<p>Before</p><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ` +
		`AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==" width="8" height="8"/><p>After</p>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}

	for i := 0; i < 3; i++ {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 12)
		_, lineHeight := pdf.GetFontSize()
		html := pdf.HTMLNew()
		html.WriteCompiled(lineHeight, compiled)
		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			t.Fatalf("render %d Output() error = %v", i, err)
		}
		pdfText := output.String()
		if !strings.Contains(pdfText, "/Subtype /Image") || !strings.Contains(pdfText, "Before") || !strings.Contains(pdfText, "After") {
			t.Fatalf("render %d generated PDF missing cached data image or surrounding text", i)
		}
	}
}

func TestHTMLWriteStyleRules(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<style>
		#preish { white-space: pre; font-family: monospace; }
		p.note, td.note { text-decoration: line-through; font-weight: bold; }
	</style>`+
		`<p class="note">Styled note</p><p id="preish"> a  b</p>`+
		`<script>shouldNotRender()</script>`+
		`<table border="1"><tr><td class="note">Cell note</td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Styled note", " a  b", "Cell note"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain CSS-styled text %q", want)
		}
	}
	for _, blocked := range []string{"#preish", "shouldNotRender", "font-weight"} {
		if strings.Contains(pdfText, blocked) {
			t.Fatalf("generated PDF leaked style/script content %q", blocked)
		}
	}
}

func TestHTMLWriteBlockBoxStylesAndHorizontalRule(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<style>
		div.panel { background-color: #eeeeee; border: 1px solid #333; padding: 6px; margin: 3px; }
	</style>`+
		`<div class="panel">Boxed <strong>text</strong></div>`+
		`<hr style="height: 2px; color: #333; width: 50%">`+
		`<p>After rule</p>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Boxed text", "After rule"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain block text %q", want)
		}
	}
	if !strings.Contains(pdfText, " re ") {
		t.Fatal("generated PDF does not contain block rectangle")
	}
	if !strings.Contains(pdfText, " m ") || !strings.Contains(pdfText, " l S") {
		t.Fatal("generated PDF does not contain horizontal rule line")
	}
}

func TestHTMLWriteRoundedBoxShadowStyles(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<div style="background:#f8fbff;border:1px solid #6688aa;border-radius:8px;box-shadow:3px 4px 8px rgba(0,0,0,.25);padding:8px;margin:6px">Rounded shadow card</div>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "Rounded shadow card") {
		t.Fatal("generated PDF does not contain rounded shadow text")
	}
	if !strings.Contains(pdfText, " c ") {
		t.Fatal("generated PDF does not contain rounded corner curve operations")
	}
	if !strings.Contains(pdfText, "/GS") {
		t.Fatal("generated PDF does not contain alpha state for box shadow")
	}
}

func TestHTMLWriteLineHeightAndBoxEdges(t *testing.T) {
	render := func(fragment string) (float64, error) {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 12)
		_, lineHeight := pdf.GetFontSize()
		startY := pdf.GetY()
		html := pdf.HTMLNew()
		html.Write(lineHeight, fragment)
		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			return 0, err
		}
		return pdf.GetY() - startY, nil
	}

	compact, err := render(`<div style="border:1px solid #333; padding:0; margin:0; line-height:1">one<br/>two</div>`)
	if err != nil {
		t.Fatalf("compact Output() error = %v", err)
	}
	loose, err := render(`<div style="border:1px solid #333; padding-top:10px; padding-bottom:12px; margin-top:8px; margin-bottom:9px; line-height:2">one<br/>two</div>`)
	if err != nil {
		t.Fatalf("loose Output() error = %v", err)
	}
	if loose <= compact {
		t.Fatalf("loose block height = %.2f, want greater than compact height %.2f", loose, compact)
	}
}

func TestHTMLWritePageBreakControls(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<p>First</p>`+
		`<div style="break-before: page">Second</div>`+
		`<div style="page-break-after: always">Third</div>`+
		`<p>Fourth</p>`)

	if got := pdf.PageCount(); got != 3 {
		t.Fatalf("PageCount() = %d, want 3", got)
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	for _, want := range []string{"First", "Second", "Third", "Fourth"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("generated PDF does not contain %q", want)
		}
	}
}

func TestHTMLWriteSpanAndFlexBox(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	fragment := `<style>
		.row { display:flex; gap:4mm; justify-content:space-between; align-items:stretch; }
		.card { flex:1; background-color:#f4f8fb; border:1px solid #8ca9c7; padding:3mm; }
		.accent { color:#166944; font-weight:bold; }
	</style>
	<div class="row">
		<div class="card">First <span class="accent">flex</span> card</div>
		<div class="card">Second card</div>
		<div class="card">Third card</div>
	</div>`
	if messages := html.ValidateHTML(fragment); len(messages) != 0 {
		t.Fatalf("ValidateHTML messages = %#v, want none", messages)
	}
	html.Write(lineHeight, fragment)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"First", "flex", "card", "Second", "Third"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain flex text %q", want)
		}
	}
}

func TestHTMLWriteAdvancedFlexSizingAndStructuredItems(t *testing.T) {
	fragment := `<style>
		.board { display:flex; flex-wrap:wrap; row-gap:2mm; column-gap:3mm; justify-content:space-between; align-items:flex-start; align-content:center; height:72mm; border:1px solid #9fb0bf; padding:3mm; }
		.card { flex:1 1 48mm; min-width:42mm; max-width:66mm; background-color:#f5f8fb; border:1px solid #b6c6d5; padding:2mm; }
		.first { order:-1; }
		.third { order:3; }
		.center { align-self:center; min-height:28mm; }
		.fixed { flex:0 0 82mm; flex-shrink:0; }
		.stack { display:flex; flex-direction:column; gap:1mm; align-items:center; justify-content:center; }
		.pill { background-color:#e7f2eb; border:1px solid #9abea7; padding:1mm; max-width:35mm; }
		table.nested { width:100%; border-collapse:collapse; margin:1mm 0 0 0; }
		table.nested td { border:1px solid #c7d2dd; padding:1mm; font-size:8pt; }
		.muted { color:#657282; font-size:8pt; }
	</style>
	<div class="board">
		<div class="card third">Third visual card<br><span class="muted">source order first, flex order last</span></div>
		<div class="card first">First visual card
			<div class="stack"><span class="pill">Nested flex note A</span><span class="pill">Nested flex note B</span></div>
		</div>
		<div class="card center">Centered card with nested table
			<table class="nested"><tr><td>Code</td><td>Meaning</td></tr><tr><td>OK</td><td>Structured table inside flex</td></tr></table>
		</div>
		<div class="card fixed">Fixed shrink card<br><span class="muted">flex-shrink:0 with flex basis</span></div>
	</div>`

	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}
	if stats := compiled.Stats(); stats.Tables != 1 || stats.CSSRules == 0 {
		t.Fatalf("compiled stats = %#v, want nested table and CSS rules", stats)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) != 0 {
		t.Fatalf("ValidateHTML messages = %#v, want none", messages)
	}
	html.WriteCompiled(lineHeight, compiled)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{
		"First visual card",
		"note A",
		"note B",
		"Centered card with nested table",
		"Structured table",
		"inside flex",
		"Fixed shrink card",
		"Third visual card",
	} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain advanced flex text %q", want)
		}
	}
	firstIndex := strings.Index(pdfText, "First visual card")
	thirdIndex := strings.Index(pdfText, "Third visual card")
	if firstIndex < 0 || thirdIndex < 0 || firstIndex > thirdIndex {
		t.Fatalf("generated PDF order puts First visual card at %d and Third visual card at %d, want flex order applied", firstIndex, thirdIndex)
	}
}

func TestHTMLWriteFlexEdgeCases(t *testing.T) {
	fragment := `<style>
		p { color:#344151; line-height:1.3; }
		.empty { display:flex; gap:2mm; border:1px solid #d5dde6; padding:2mm; margin:1mm 0 3mm 0; }
		.reverse { display:flex; flex-direction:row-reverse; gap:2mm; align-items:center; margin:2mm 0; }
		.inline { display:inline-flex; flex-wrap:wrap; column-gap:2mm; row-gap:1mm; margin:2mm 0; }
		.page-flex { display:flex; flex-direction:column; justify-content:space-evenly; align-items:center; gap:2mm; height:48mm; page-break-before:always; border:1px solid #9fb0bf; padding:3mm; }
		.unit { flex:0 1 54mm; min-width:22mm; max-width:60mm; background-color:#f5f8fb; border:1px solid #b6c6d5; padding:2mm; }
		.fixed { flex:none; width:45mm; }
		.auto { flex:auto; min-width:30mm; max-width:64mm; }
		.align-end { align-self:flex-end; }
		.small { color:#657282; font-size:8pt; }
	</style>
	<p>Before edge cases</p>
	<div class="empty"></div>
	<div class="reverse">
		<div class="unit">Reverse A</div>
		<div class="unit fixed">Reverse B fixed</div>
		<div class="unit auto">Reverse C auto</div>
	</div>
	<div class="inline">
		Direct text flex item
		<span class="unit">Inline span item</span>
		<span class="unit">Wrapped span item with <span class="small">styled span child</span></span>
	</div>
	<div class="page-flex">
		<div class="unit fixed">Column fixed item</div>
		<div class="unit align-end">Column aligned end</div>
		<div class="unit auto">Column auto item</div>
	</div>
	<p>After edge cases</p>`

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) != 0 {
		t.Fatalf("ValidateHTML messages = %#v, want none", messages)
	}
	html.Write(lineHeight, fragment)
	if got := pdf.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want page-break-before flex block to create 2 pages", got)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{
		"Before edge cases",
		"Reverse A",
		"Reverse B fixed",
		"Reverse C auto",
		"Direct",
		"text",
		"Inline span item",
		"styled",
		"span child",
		"Column fixed item",
		"Column aligned end",
		"Column auto",
		"After edge cases",
	} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain flex edge text %q", want)
		}
	}
	reverseC := strings.Index(pdfText, "Reverse C auto")
	reverseA := strings.Index(pdfText, "Reverse A")
	if reverseC < 0 || reverseA < 0 || reverseC > reverseA {
		t.Fatalf("row-reverse text order C=%d A=%d, want Reverse C before Reverse A", reverseC, reverseA)
	}
}

func TestHTMLWriteRichFlexAndTableDesigns(t *testing.T) {
	fragment := `<style>
		h1 { color:#1f3652; font-size:18pt; margin:0 0 4mm 0; }
		h2 { color:#1f3652; font-size:13pt; margin:5mm 0 2mm 0; }
		p { color:#3f4b59; line-height:1.25; margin:0 0 2mm 0; }
		.summary { display:flex; gap:3mm; align-items:stretch; margin:2mm 0 4mm 0; }
		.card { flex:1; background-color:#f3f8fc; border:1px solid #9db8d2; border-radius:2mm; padding:3mm; }
		.kpi { color:#166944; font-size:16pt; font-weight:bold; }
		.muted { color:#687384; font-size:8pt; }
		.callout { background-color:#fff8e7; border-left:3px solid #d69222; border-top:1px solid #ead8ad; border-right:1px solid #ead8ad; border-bottom:1px solid #ead8ad; padding:3mm; margin:3mm 0; }
		table { width:100%; border-collapse:collapse; margin:2mm 0 4mm 0; }
		caption { color:#687384; font-size:8pt; margin-bottom:2mm; }
		th { background-color:#e8eef5; color:#1f3652; border:1px solid #aebcce; font-weight:bold; padding:2mm; text-align:left; }
		td { border:1px solid #c8d2df; color:#394654; padding:2mm; vertical-align:top; }
		td.money { text-align:right; }
		td.status { color:#166944; font-weight:bold; }
		tr.soft td { background-color:#f8fafc; }
		tfoot td { background-color:#f4f7fb; font-weight:bold; }
		table.compact td { padding:1mm; font-size:8pt; }
		table.nested { margin:1mm 0; }
		table.nested td { border:1px solid #d7e0ea; padding:1mm; font-size:7.5pt; }
		.approval td { height:12mm; background-color:#fbfcfd; }
	</style>
	<h1>Rich HTML PDF Test</h1>
	<div class="summary">
		<div class="card"><span class="kpi">98.7%</span><br><strong>Fulfillment SLA</strong><br><span class="muted">styled span inside flex</span></div>
		<div class="card"><span class="kpi">$18.4k</span><br><strong>Open invoices</strong><br><span class="muted">right-aligned table below</span></div>
		<div class="card"><span class="kpi">42 min</span><br><strong>Median response</strong><br><span class="muted">support queue</span></div>
	</div>
	<p class="callout"><strong>Design note: </strong>the fragment mixes flex cards, spans, captions, footers, colspan, rowspan, nested tables, and automatic PDF drawing styles.</p>
	<h2>Invoice Review</h2>
	<table>
		<caption>Styled billing table with right alignment and footer total</caption>
		<thead><tr><th width="18%">Invoice</th><th width="34%">Customer</th><th width="18%">Status</th><th width="15%">Due</th><th width="15%" class="money">Amount</th></tr></thead>
		<tbody>
			<tr><td>INV-4101</td><td>Acme Logistics</td><td class="status"><span>Ready</span></td><td>Jul 01</td><td class="money">$4,120.00</td></tr>
			<tr class="soft"><td>INV-4102</td><td>Northwind Trading</td><td>Review</td><td>Jul 03</td><td class="money">$2,950.00</td></tr>
			<tr><td>INV-4103</td><td>Contoso Retail</td><td class="status">Ready</td><td>Jul 05</td><td class="money">$1,340.00</td></tr>
		</tbody>
		<tfoot><tr><td colspan="4">Total ready for approval</td><td class="money">$8,410.00</td></tr></tfoot>
	</table>
	<h2>Operations Matrix</h2>
	<table class="compact">
		<thead><tr><th width="18%">Area</th><th width="26%">Owner</th><th width="28%">Today</th><th width="28%">Tomorrow</th></tr></thead>
		<tbody>
			<tr><td rowspan="2">Warehouse</td><td>Fulfillment</td><td>Pick-list release</td><td>Barcode audit</td></tr>
			<tr class="soft"><td>Inventory</td><td colspan="2">Cycle count exception review</td></tr>
			<tr><td>Support</td><td>Queue lead</td><td>Carrier follow-up</td><td>Account sync</td></tr>
		</tbody>
	</table>
	<div style="page-break-before:always"></div>
	<h1>Nested and Approval Tables</h1>
	<table>
		<thead><tr><th width="25%">Workstream</th><th width="45%">Details</th><th width="30%">Risk</th></tr></thead>
		<tbody>
			<tr>
				<td>Carrier mapping</td>
				<td>Partner code reconciliation
					<table class="nested">
						<tr><td>HTTP 202</td><td>Accepted import</td></tr>
						<tr><td>EXC-7</td><td>Manual address review</td></tr>
					</table>
				</td>
				<td><span class="status">Medium</span><br><span class="muted">Watch daily sync</span></td>
			</tr>
			<tr class="soft"><td>Template release</td><td>PDF packet QA and customer approval</td><td>Low</td></tr>
		</tbody>
	</table>
	<table class="approval">
		<tr><td width="50%"><strong>Prepared by</strong><br><br>PDF Services Team</td><td width="50%"><strong>Approved by</strong><br><br>Account Owner</td></tr>
	</table>`

	compiled, err := document.CompileHTML(fragment)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}
	if stats := compiled.Stats(); stats.Tables < 5 || stats.CSSRules == 0 || stats.CachedStyles == 0 {
		t.Fatalf("compiled stats = %#v, want rich table and CSS coverage", stats)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	if messages := html.ValidateHTML(fragment); len(messages) != 0 {
		t.Fatalf("ValidateHTML messages = %#v, want none", messages)
	}
	html.WriteCompiled(lineHeight, compiled)
	if got := pdf.PageCount(); got != 2 {
		t.Fatalf("PageCount() = %d, want 2", got)
	}

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{
		"Rich HTML PDF Test",
		"Fulfillment SLA",
		"Invoice Review",
		"Total ready for approval",
		"Operations Matrix",
		"Cycle count exception review",
		"Nested and Approval Tables",
		"Accepted import",
		"Manual address review",
		"PDF Services Team",
	} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not contain rich table text %q", want)
		}
	}
}

func TestHTMLWriteHeadStyleScriptAreNotRendered(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<head><title>hidden title</title><style>.x { color: red; }</style></head>`+
		`<p>Visible text</p><script>hiddenScript()</script>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "Visible text") {
		t.Fatal("generated PDF does not contain visible text")
	}
	for _, blocked := range []string{"hidden title", "hiddenScript", "color: red"} {
		if strings.Contains(pdfText, blocked) {
			t.Fatalf("generated PDF leaked hidden content %q", blocked)
		}
	}
}

func TestHTMLWriteStyleSelectorVariants(t *testing.T) {
	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.Write(lineHeight, `<style>
		notice { white-space: pre; font-family: monospace; }
		p#exact { white-space: pre; }
		span.keep { white-space: pre; }
		div .inside { white-space: pre; }
		div > span.direct { white-space: pre; }
		div .ignored { white-space: pre; }
		table td.note { white-space: pre; }
	</style>`+
		`<notice> a  tag</notice><p id="exact"> b  id</p><span class="keep other"> c  class</span>`+
		`<div><span class="inside"> e  descendant</span><span class="direct"> f  child</span>`+
		`<p><span class="direct">g  nested</span></p></div>`+
		`<span class="ignored">h  ignored</span>`+
		`<table><tr><td class="note"> i  cell</td></tr></table>`)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{" a  tag", " b  id", " c  class", " e  descendant", " f  child", " i  cell"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated PDF does not preserve selector-styled whitespace %q", want)
		}
	}
	for _, blocked := range []string{"g  nested", "h  ignored"} {
		if strings.Contains(pdfText, blocked) {
			t.Fatalf("selector unexpectedly preserved whitespace for %q", blocked)
		}
	}
	for _, want := range []string{"g nested", "h ignored"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("unmatched selector text was not rendered with collapsed whitespace %q", want)
		}
	}
}

func TestSVGParseStylesAndText(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="100" height="50">
		<g fill="#fff" font-size="12" text-anchor="middle">
			<rect x="1" y="2" width="30" height="10" fill="#123456"/>
			<path d="M1 1 L9 9" stroke="rgb(255,0,0)" fill="none" stroke-width="2"/>
			<text x="50" y="20">Label</text>
		</g>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 2 {
		t.Fatalf("len(Paths) = %d, want 2", len(svg.Paths))
	}
	if len(svg.Segments) != len(svg.Paths) {
		t.Fatalf("len(Segments) = %d, want %d", len(svg.Segments), len(svg.Paths))
	}
	rectFill := svg.Paths[0].Style.Fill
	if !rectFill.Set || rectFill.R != 0x12 || rectFill.G != 0x34 || rectFill.B != 0x56 {
		t.Fatalf("rect fill = %#v", rectFill)
	}
	pathStroke := svg.Paths[1].Style.Stroke
	if !pathStroke.Set || pathStroke.R != 255 || pathStroke.G != 0 || pathStroke.B != 0 {
		t.Fatalf("path stroke = %#v", pathStroke)
	}
	if !svg.Paths[1].Style.Fill.None || svg.Paths[1].Style.StrokeWidth != 2 {
		t.Fatalf("path style = %#v", svg.Paths[1].Style)
	}
	if len(svg.Texts) != 1 || svg.Texts[0].Text != "Label" {
		t.Fatalf("texts = %#v", svg.Texts)
	}
	if svg.Texts[0].Style.TextAnchor != "middle" || svg.Texts[0].Style.FontSize != 12 {
		t.Fatalf("text style = %#v", svg.Texts[0].Style)
	}
}

func TestSVGParseLineStyleOpacityAndTspan(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="100" height="50">
		<path d="M1 1 L20 20"
			stroke="#123456"
			fill="none"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="bevel"
			stroke-dasharray="2 3"
			stroke-dashoffset="1"
			opacity="50%"
			stroke-opacity=".75"/>
		<text x="10" y="30" fill="#000" fill-opacity=".25"><tspan>First</tspan><tspan>Second</tspan></text>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(svg.Paths))
	}
	style := svg.Paths[0].Style
	if style.StrokeLineCap != "round" || style.StrokeLineJoin != "bevel" {
		t.Fatalf("line style = cap %q join %q, want round/bevel", style.StrokeLineCap, style.StrokeLineJoin)
	}
	if !style.StrokeDashSet || len(style.StrokeDashArray) != 2 ||
		style.StrokeDashArray[0] != 2 || style.StrokeDashArray[1] != 3 || style.StrokeDashOffset != 1 {
		t.Fatalf("dash style = %#v", style)
	}
	if !style.OpacitySet || style.Opacity != 0.5 || !style.StrokeOpacitySet || style.StrokeOpacity != 0.75 {
		t.Fatalf("opacity style = %#v", style)
	}
	if len(svg.Texts) != 1 || svg.Texts[0].Text != "First Second" {
		t.Fatalf("texts = %#v, want combined tspan text", svg.Texts)
	}
	if !svg.Texts[0].Style.FillOpacitySet || svg.Texts[0].Style.FillOpacity != 0.25 {
		t.Fatalf("text opacity style = %#v", svg.Texts[0].Style)
	}
}

func TestSVGParseCurrentColor(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="30" height="20">
		<style>.accent { color: #abcdef; }</style>
		<g color="#123456">
			<path d="M1 1 H10 V10 Z" fill="currentColor"/>
			<path class="accent" d="M12 1 H20 V10 Z" stroke="currentColor" fill="none"/>
		</g>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 2 {
		t.Fatalf("len(Paths) = %d, want 2", len(svg.Paths))
	}
	firstFill := svg.Paths[0].Style.Fill
	if !firstFill.Set || firstFill.R != 0x12 || firstFill.G != 0x34 || firstFill.B != 0x56 {
		t.Fatalf("first fill = %#v, want inherited currentColor", firstFill)
	}
	secondStroke := svg.Paths[1].Style.Stroke
	if !secondStroke.Set || secondStroke.R != 0xab || secondStroke.G != 0xcd || secondStroke.B != 0xef {
		t.Fatalf("second stroke = %#v, want CSS currentColor", secondStroke)
	}
}

func TestSVGParseEmbeddedImage(t *testing.T) {
	const pngData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	svg, err := document.SVGParse([]byte(`<svg width="40" height="30">
		<image x="2" y="3" width="20" height="10" opacity=".5" href="data:image/png;base64,` + pngData + `"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Images) != 1 {
		t.Fatalf("len(Images) = %d, want 1", len(svg.Images))
	}
	image := svg.Images[0]
	if image.X != 2 || image.Y != 3 || image.Wd != 20 || image.Ht != 10 || image.ImageType != "png" || len(image.Data) == 0 {
		t.Fatalf("image = %#v, want embedded PNG at 2,3 20x10", image)
	}
	if !image.Style.OpacitySet || image.Style.Opacity != 0.5 {
		t.Fatalf("image style = %#v, want opacity .5", image.Style)
	}
}

func TestSVGParseElementOrder(t *testing.T) {
	const pngData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	svg, err := document.SVGParse([]byte(`<svg width="40" height="30">
		<text x="1" y="5" fill="#000">Before</text>
		<image x="2" y="6" width="10" height="10" href="data:image/png;base64,` + pngData + `"/>
		<text x="1" y="25" fill="#000">After</text>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Elements) != 3 {
		t.Fatalf("len(Elements) = %d, want 3", len(svg.Elements))
	}
	if svg.Elements[0].Kind != "text" || svg.Elements[1].Kind != "image" || svg.Elements[2].Kind != "text" {
		t.Fatalf("element order = %#v, want text/image/text", svg.Elements)
	}
	if svg.Elements[0].Text.Text != "Before" || svg.Elements[2].Text.Text != "After" {
		t.Fatalf("ordered text elements = %#v", svg.Elements)
	}
}

func TestSVGWriteElementOrder(t *testing.T) {
	const pngData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	svg, err := document.SVGParse([]byte(`<svg width="40" height="30">
		<text x="1" y="5" fill="#000">Before</text>
		<image x="2" y="6" width="10" height="10" href="data:image/png;base64,` + pngData + `"/>
		<text x="1" y="25" fill="#000">After</text>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SVGWrite(&svg, 1)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	before := strings.Index(pdfText, "Before")
	image := strings.Index(pdfText, " Do Q")
	after := strings.Index(pdfText, "After")
	if before < 0 || image < 0 || after < 0 {
		t.Fatalf("missing output markers: Before=%d image=%d After=%d", before, image, after)
	}
	if before >= image || image >= after {
		t.Fatalf("PDF paint order indexes: Before=%d image=%d After=%d, want text/image/text", before, image, after)
	}
}

func TestSVGWriteEmbeddedImage(t *testing.T) {
	const pngData = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJ" +
		"AAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	svg, err := document.SVGParse([]byte(`<svg width="40" height="30">
		<defs><clipPath id="clip"><rect x="0" y="0" width="30" height="30"/></clipPath></defs>
		<image x="2" y="3" width="20" height="10" clip-path="url(#clip)" href="data:image/png;base64,` + pngData + `"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SVGWrite(&svg, 1)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "/Subtype /Image") {
		t.Fatal("generated PDF does not contain SVG embedded image")
	}
	if !strings.Contains(pdfText, "W n") {
		t.Fatal("generated PDF does not clip SVG embedded image")
	}
}

func TestSVGWriteStyledContent(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="100" height="40">
		<rect x="1" y="1" width="20" height="10" fill="#00ff00" stroke="#000" stroke-width="1"/>
		<path d="M30 5 C35 1 45 1 50 5" stroke="blue" fill="none"/>
		<text x="10" y="30" fill="red" font-size="10">Hello SVG</text>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.SetXY(10, 10)
	pdf.SVGWrite(&svg, 0.5)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(output.String(), "Hello SVG") {
		t.Fatal("generated PDF does not contain SVG text")
	}
}

func TestSVGWriteDoesNotMutateParsedSVG(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="100" height="40">
		<g fill="#123456" font-size="10"><text x="10" y="20">Stable SVG</text></g>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	before := fmt.Sprintf("%#v", svg)

	for i := 0; i < 2; i++ {
		pdf := document.MustNew()
		pdf.SetCompression(false)
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 12)
		pdf.SVGWrite(&svg, 0.5)
		var output bytes.Buffer
		if err := pdf.Output(&output); err != nil {
			t.Fatalf("render %d Output() error = %v", i, err)
		}
		if !strings.Contains(output.String(), "Stable SVG") {
			t.Fatalf("render %d generated PDF does not contain SVG text", i)
		}
	}

	if after := fmt.Sprintf("%#v", svg); after != before {
		t.Fatal("SVGWrite mutated the parsed SVG value")
	}
}

func TestHTMLWriteCompiledInlineSVGTaggedRole(t *testing.T) {
	compiled, err := document.CompileHTML(`<a href="https://example.test/svg"><svg width="80" height="20" viewBox="0 0 80 20"><text x="2" y="14">Tagged SVG</text></svg></a>`)
	if err != nil {
		t.Fatalf("CompileHTML() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.SetComplianceMetadata(document.ComplianceMetadata{PDFUA2: true, Title: "Tagged SVG"})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	_, lineHeight := pdf.GetFontSize()
	html := pdf.HTMLNew()
	html.WriteCompiled(lineHeight, compiled)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	for _, want := range []string{"Tagged SVG", "/S /Link", "/Type /Annot /Subtype /Link", "/Type /OBJR /Obj"} {
		if !strings.Contains(pdfText, want) {
			t.Fatalf("generated tagged SVG PDF does not contain %q", want)
		}
	}
}

func TestSVGWriteEvenOddFillRule(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="20" height="20">
		<path fill="#000" fill-rule="evenodd" d="M0 0 H20 V20 H0 Z M5 5 H15 V15 H5 Z"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 1 || svg.Paths[0].Style.FillRule != "evenodd" {
		t.Fatalf("fill rule style = %#v", svg.Paths)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SVGWrite(&svg, 1)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(output.String(), "f*") {
		t.Fatal("generated PDF does not use even-odd fill operator")
	}
}

func TestSVGParseClipPathAndLinearGradient(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="40" height="40">
		<style>
			.paint { fill: url(#paint); clip-path: url(#clip); }
		</style>
		<defs>
			<clipPath id="clip" clip-rule="evenodd">
				<path d="M0 0 H30 V30 H0 Z M5 5 H25 V25 H5 Z"/>
			</clipPath>
			<linearGradient id="paint" x1="0%" y1="0%" x2="100%" y2="100%">
				<stop offset="0%" stop-color="#112233"/>
				<stop offset="50%" stop-color="#445566"/>
				<stop offset="100%" stop-color="#abcdef"/>
			</linearGradient>
		</defs>
		<rect class="paint" x="1" y="2" width="20" height="10"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(svg.Paths))
	}
	style := svg.Paths[0].Style
	if !style.FillGradient.Set || style.FillGradient.Kind != "linear" || len(style.FillGradient.Stops) != 3 {
		t.Fatalf("gradient style = %#v", style.FillGradient)
	}
	if style.FillGradient.Stops[0].Color.R != 0x11 || style.FillGradient.Stops[1].Offset != 0.5 ||
		style.FillGradient.Stops[2].Color.B != 0xef {
		t.Fatalf("gradient stops = %#v", style.FillGradient.Stops)
	}
	if len(style.ClipPath) == 0 || style.ClipRule != "evenodd" {
		t.Fatalf("clip style = rule %q segments %#v", style.ClipRule, style.ClipPath)
	}
}

func TestSVGParsePatternFill(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="40" height="40">
		<defs>
			<pattern id="dots" patternUnits="userSpaceOnUse" x="0" y="0" width="8" height="8">
				<circle cx="2" cy="2" r="2" fill="#ff0000"/>
			</pattern>
		</defs>
		<rect x="0" y="0" width="20" height="20" fill="url(#dots)"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want only the target rect path", len(svg.Paths))
	}
	pattern := svg.Paths[0].Style.FillPattern
	if !pattern.Set || pattern.Units != "userSpaceOnUse" || pattern.Wd != 8 || pattern.Ht != 8 {
		t.Fatalf("pattern = %#v, want userSpaceOnUse 8x8", pattern)
	}
	if len(pattern.Elements) != 1 || pattern.Elements[0].Kind != "path" {
		t.Fatalf("pattern elements = %#v, want one path element", pattern.Elements)
	}
	fill := pattern.Elements[0].Path.Style.Fill
	if !fill.Set || fill.R != 255 || fill.G != 0 || fill.B != 0 {
		t.Fatalf("pattern fill = %#v, want red", fill)
	}
}

func TestSVGParseFillOverridesInheritedPattern(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="40" height="40">
		<defs>
			<pattern id="dots" patternUnits="userSpaceOnUse" width="8" height="8">
				<circle cx="2" cy="2" r="2" fill="#ff0000"/>
			</pattern>
		</defs>
		<g fill="url(#dots)">
			<rect x="0" y="0" width="10" height="10"/>
			<rect x="12" y="0" width="10" height="10" fill="#00ff00"/>
		</g>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if len(svg.Paths) != 2 {
		t.Fatalf("len(Paths) = %d, want 2", len(svg.Paths))
	}
	if !svg.Paths[0].Style.FillPattern.Set {
		t.Fatalf("first path pattern = %#v, want inherited pattern", svg.Paths[0].Style.FillPattern)
	}
	if svg.Paths[1].Style.FillPattern.Set || svg.Paths[1].Style.FillGradient.Set || svg.Paths[1].Style.FillRef != "" {
		t.Fatalf("second path inherited paint ref = pattern %#v gradient %#v ref %q, want cleared",
			svg.Paths[1].Style.FillPattern, svg.Paths[1].Style.FillGradient, svg.Paths[1].Style.FillRef)
	}
	fill := svg.Paths[1].Style.Fill
	if !fill.Set || fill.R != 0 || fill.G != 255 || fill.B != 0 {
		t.Fatalf("second path fill = %#v, want green", fill)
	}
}

func TestSVGWritePatternFill(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="40" height="40">
		<defs>
			<pattern id="dots" patternUnits="userSpaceOnUse" width="8" height="8">
				<circle cx="2" cy="2" r="2" fill="#ff0000"/>
			</pattern>
		</defs>
		<rect x="0" y="0" width="20" height="20" fill="url(#dots)"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SVGWrite(&svg, 1)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if !strings.Contains(pdfText, "W n") {
		t.Fatal("generated PDF does not clip SVG pattern fill")
	}
	if strings.Count(pdfText, "1.000 0.000 0.000 rg") != 1 {
		t.Fatal("generated PDF should render pattern tile content once")
	}
	if strings.Count(pdfText, " Do Q") < 2 {
		t.Fatal("generated PDF does not stamp the cached pattern tile")
	}
}

func TestSVGWriteClipPathAndGradients(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="80" height="40">
		<defs>
			<clipPath id="clip"><circle cx="20" cy="20" r="18"/></clipPath>
			<linearGradient id="linear" x1="0" y1="0" x2="1" y2="1">
				<stop offset="0" stop-color="red"/>
				<stop offset=".5" stop-color="lime"/>
				<stop offset="1" stop-color="blue"/>
			</linearGradient>
			<radialGradient id="radial" cx="50%" cy="50%" r="50%">
				<stop offset="0" stop-color="white"/>
				<stop offset="1" stop-color="black"/>
			</radialGradient>
		</defs>
		<rect x="0" y="0" width="40" height="40" fill="url(#linear)" clip-path="url(#clip)" stroke="#000"/>
		<circle cx="60" cy="20" r="18" fill="url(#radial)"/>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}

	pdf := document.MustNew()
	pdf.SetCompression(false)
	pdf.AddPage()
	pdf.SVGWrite(&svg, 1)

	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	pdfText := output.String()
	if strings.Count(pdfText, "/Sh") < 2 {
		t.Fatalf("generated PDF contains %d shading uses, want at least 2", strings.Count(pdfText, "/Sh"))
	}
	if !strings.Contains(pdfText, "W n") {
		t.Fatal("generated PDF does not contain clipping operator")
	}
	if !strings.Contains(pdfText, "/FunctionType 3") || !strings.Contains(pdfText, "/Bounds [0.50000]") {
		t.Fatal("generated PDF does not contain multi-stop gradient stitching function")
	}
}

func TestSVGParseViewBoxAndTransforms(t *testing.T) {
	svg, err := document.SVGParse([]byte(`<svg width="100" height="50" viewBox="-10 -20 20 10">
		<g transform="translate(1,2)" stroke="black" fill="none">
			<path d="M-10 -20 H10 V-10"/>
			<text x="-5" y="-15" transform="translate(2,3)">Moved</text>
		</g>
	</svg>`))
	if err != nil {
		t.Fatalf("SVGParse() error = %v", err)
	}
	if svg.Wd != 100 || svg.Ht != 50 {
		t.Fatalf("extent = %.2f x %.2f, want 100 x 50", svg.Wd, svg.Ht)
	}
	if len(svg.Paths) != 1 {
		t.Fatalf("len(Paths) = %d, want 1", len(svg.Paths))
	}
	segs := svg.Paths[0].Segments
	if len(segs) != 3 {
		t.Fatalf("len(path segments) = %d, want 3", len(segs))
	}
	if segs[0].Cmd != 'M' || segs[0].Arg[0] != 5 || segs[0].Arg[1] != 10 {
		t.Fatalf("move segment = %#v, want M 5 10", segs[0])
	}
	if segs[1].Cmd != 'L' || segs[1].Arg[0] != 105 || segs[1].Arg[1] != 10 {
		t.Fatalf("horizontal segment = %#v, want L 105 10", segs[1])
	}
	if segs[2].Cmd != 'L' || segs[2].Arg[0] != 105 || segs[2].Arg[1] != 60 {
		t.Fatalf("vertical segment = %#v, want L 105 60", segs[2])
	}
	if len(svg.Texts) != 1 {
		t.Fatalf("len(Texts) = %d, want 1", len(svg.Texts))
	}
	if svg.Texts[0].Text != "Moved" || svg.Texts[0].X != 40 || svg.Texts[0].Y != 50 {
		t.Fatalf("text = %#v, want Moved at 40,50", svg.Texts[0])
	}
}
