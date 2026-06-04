# GoPDFKit Generation Examples

This cookbook shows the main ways to generate PDFs with GoPDFKit. The snippets
are intentionally compact; the complete runnable versions live under
[`examples/`](../examples).

Generated files in the runnable examples are written to
`assets/generated/pdf/examples`.

Snippet variables such as `sourcePDF`, `templatePDF`, `firstPDF`, `secondPDF`,
and `loadCertificateAndSigner` represent application-provided PDF bytes or
certificate loading code. The runnable examples build those inputs with public
fixtures.

## Basic Page

Use the root package when the default A4 portrait document is enough.

```go
package main

import "github.com/cssbruno/gopdfkit"

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 18)
	pdf.Cell(40, 10, "Hello from GoPDFKit")

	if err := pdf.OutputFileAndClose("hello-world.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/hello-world`

## Configured Document

Use `document.NewWithOptions` when page defaults should be explicit.

```go
package main

import "github.com/cssbruno/gopdfkit/document"

func main() {
	pdf := document.NewWithOptions(document.Options{
		OrientationStr: "P",
		UnitStr:        "mm",
		SizeStr:        "A4",
		Optimize:       true,
	})
	pdf.SetTitle("Operations Report", false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 20)
	pdf.Text(16, 24, "Operations Report")

	if err := pdf.OutputFileAndClose("report.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable examples: `go run ./examples/report`,
`go run ./examples/structured-report`

## Headers And Footers

Register callbacks before adding pages. `AliasNbPages` replaces the total page
placeholder at output time.

```go
package main

import (
	"fmt"

	"github.com/cssbruno/gopdfkit"
)

func main() {
	pdf := gopdfkit.New()
	pdf.AliasNbPages("{total}")

	pdf.SetHeaderFunc(func() {
		pdf.SetFont("Helvetica", "B", 12)
		pdf.CellFormat(0, 8, "Monthly Report", "B", 1, "C", false, 0, "")
		pdf.Ln(4)
	})

	pdf.SetFooterFunc(func() {
		pdf.SetY(-16)
		pdf.SetFont("Helvetica", "", 9)
		pdf.CellFormat(0, 8, fmt.Sprintf("Page %d / {total}", pdf.PageNo()), "T", 0, "C", false, 0, "")
	})

	for page := 1; page <= 3; page++ {
		pdf.AddPage()
		pdf.SetFont("Helvetica", "", 11)
		pdf.CellFormat(0, 7, fmt.Sprintf("Page %d content", page), "", 1, "L", false, 0, "")
	}

	if err := pdf.OutputFileAndClose("headers-footers.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/headers-footers`

## Tables And Manual Pagination

Manual table generation uses `CellFormat`, row widths, and a page-break check
before drawing each row.

```go
package main

import (
	"fmt"

	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	pdf := document.New("P", "mm", "A4", "")
	pdf.AddPage()
	drawHeader(pdf)

	for i := 1; i <= 130; i++ {
		if pdf.GetY()+7 > 270 {
			pdf.AddPage()
			drawHeader(pdf)
		}
		drawRow(pdf, i)
	}

	if err := pdf.OutputFileAndClose("table.pdf"); err != nil {
		panic(err)
	}
}

func drawHeader(pdf *document.Document) {
	pdf.SetFont("Helvetica", "B", 8)
	for i, label := range []string{"#", "Account", "Status", "Amount"} {
		pdf.CellFormat([]float64{18, 74, 38, 44}[i], 7, label, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(7)
}

func drawRow(pdf *document.Document, index int) {
	values := []string{
		fmt.Sprintf("%03d", index),
		fmt.Sprintf("Customer Account %03d", index),
		"Ready",
		fmt.Sprintf("$%0.2f", float64(index)*37.25),
	}
	for i, value := range values {
		pdf.CellFormat([]float64{18, 74, 38, 44}[i], 7, value, "1", 0, "L", false, 0, "")
	}
	pdf.Ln(7)
}
```

Runnable examples: `go run ./examples/table-report`,
`go run ./examples/pagination-table`

## Document Model Pagination

The structured document model can paginate paragraphs and repeat table headers.

```go
package main

import (
	"fmt"

	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	doc := document.NewGenericDocument("Document Pagination")
	doc.Footer = &document.FooterBlock{
		ShowPageNumber:  true,
		TotalPageAlias:  "{total}",
		ReservePageArea: true,
	}

	for i := 1; i <= 24; i++ {
		doc.Body = append(doc.Body, document.ParagraphBlock{
			Segments: []document.TextSegment{{Text: fmt.Sprintf("Paragraph %02d", i)}},
		})
	}
	doc.Body = append(doc.Body, document.TableBlock{
		Header: []document.TableRow{{Cells: []document.TableCell{{Blocks: []document.Block{
			document.ParagraphBlock{Segments: []document.TextSegment{{Text: "Description"}}},
		}}}}},
		Body: []document.TableRow{{Cells: []document.TableCell{{Blocks: []document.Block{
			document.ParagraphBlock{Segments: []document.TextSegment{{Text: "Generated row"}}},
		}}}}},
		Style: document.TableStyle{RepeatHeader: true, KeepRows: true},
	})

	pdf := document.New("P", "mm", "A4", "")
	pdf.WriteDocument(doc)
	if err := pdf.OutputFileAndClose("document-pagination.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/pagination-document`

## Invoice Or Transaction Document

Invoices are regular positioned text, fields, table rows, and totals.

```go
package main

import "github.com/cssbruno/gopdfkit"

func main() {
	pdf := gopdfkit.New()
	pdf.SetTitle("Invoice", false)
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 22)
	pdf.Text(16, 20, "Invoice INV-2026-0042")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(16, 34, "Bill To")
	pdf.SetXY(16, 40)
	pdf.MultiCell(76, 5, "Northwind Operations\n22 Market Street\nSeattle, WA 98101", "", "L", false)

	pdf.SetFont("Helvetica", "B", 9)
	for i, label := range []string{"Description", "Qty", "Rate", "Amount"} {
		pdf.CellFormat([]float64{88, 22, 32, 34}[i], 8, label, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(8)
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(88, 9, "PDF generation platform", "1", 0, "L", false, 0, "")
	pdf.CellFormat(22, 9, "1", "1", 0, "C", false, 0, "")
	pdf.CellFormat(32, 9, "$800.00", "1", 0, "R", false, 0, "")
	pdf.CellFormat(34, 9, "$800.00", "1", 1, "R", false, 0, "")

	if err := pdf.OutputFileAndClose("invoice.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/invoice`

## Styled Text And HTML Fragments

`HTMLNew` renders the supported HTML/CSS subset into PDF drawing operations.

```go
package main

import "github.com/cssbruno/gopdfkit"

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 11)

	html := pdf.HTMLNew()
	html.Write(6, `
		<style>
			.callout { background-color: #f2f7fc; border: 1px solid #b9c9dc; padding: 8px; }
		</style>
		<h1>Invoice Summary</h1>
		<p><strong>Status:</strong> paid</p>
		<p class="callout">HTML fragments support text styles, lists, tables, borders, colors, and spacing.</p>
	`)

	if err := pdf.OutputFileAndClose("html-fragment.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable examples: `go run ./examples/styled-paragraphs`,
`go run ./examples/html-fragment`, `go run ./examples/html-css-styles`

## HTML Tables

Build rows with Go, then render a controlled HTML table.

```go
package main

import (
	"fmt"
	"strings"

	"github.com/cssbruno/gopdfkit"
)

func main() {
	var rows strings.Builder
	for i := 1; i <= 20; i++ {
		_, _ = fmt.Fprintf(&rows, `<tr><td>INV-%04d</td><td>Ready</td><td class="amount">$%d.00</td></tr>`, i, 100+i)
	}

	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 10)
	pdf.HTMLNew().Write(5.5, `
		<style>
			table { width: 100%; border-collapse: collapse; }
			th, td { border: 1px solid #c6d0da; padding: 3px; }
			.amount { text-align: right; }
		</style>
		<table><thead><tr><th>Reference</th><th>Status</th><th>Amount</th></tr></thead><tbody>`+
		rows.String()+`</tbody></table>`)

	if err := pdf.OutputFileAndClose("html-table.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/html-tables`

## HTML Templates

Use `RenderHTMLTemplate` for escaped `{{key}}` values and explicit trusted
HTML/image placeholders.

```go
package main

import (
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	fragment, err := document.RenderHTMLTemplate(`
		<h1>{{title}}</h1>
		<p><strong>Customer:</strong> {{customer}}</p>
		{{logo}}
	`, document.HTMLTemplateValues{
		"title":    "HTML Template",
		"customer": "Northwind Trading",
		"logo": document.HTMLTemplateImage{
			Source:    "assets/static/image/gopdfkit.png",
			Alt:       "GoPDFKit logo",
			Width:     "45mm",
			ObjectFit: "contain",
		},
	})
	if err != nil {
		panic(err)
	}

	pdf := gopdfkit.New()
	pdf.AddPage()
	html := pdf.HTMLNew()
	html.AllowLocalImages = true
	html.Write(6, fragment)

	if err := pdf.OutputFileAndClose("html-template.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/html-template`

## Images

Place image files with `ImageOptions`. Width or height can be zero to preserve
aspect ratio.

```go
package main

import (
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.ImageOptions("assets/static/image/logo.png", 20, 30, 60, 0, false, document.ImageOptions{}, 0, "")
	pdf.ImageOptions("assets/static/image/logo.jpg", 20, 90, 60, 0, false, document.ImageOptions{ImageType: "jpg"}, 0, "")

	if err := pdf.OutputFileAndClose("images.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/add-images-to-pages`

## Image From Memory

Register generated image bytes with `RegisterImageOptionsReader`, then place
the registered name.

```go
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	img := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			img.Set(x, y, color.RGBA{R: uint8(x / 2), G: uint8(y / 2), B: 180, A: 255})
		}
	}

	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		panic(err)
	}

	pdf := gopdfkit.New()
	pdf.AddPage()
	options := document.ImageOptions{ImageType: "png"}
	pdf.RegisterImageOptionsReader("gradient", options, bytes.NewReader(pngData.Bytes()))
	pdf.ImageOptions("gradient", 20, 30, 100, 0, false, options, 0, "")

	if err := pdf.OutputFileAndClose("image-from-memory.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/image-from-memory`

## QR Code

`RegisterQRCodePNG` creates and registers a PNG image for a payload.

```go
package main

import (
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()

	name, err := pdf.RegisterQRCodePNG("https://example.test/verify/document-1", 256)
	if err != nil {
		panic(err)
	}
	pdf.ImageOptions(name, 20, 30, 35, 35, false, document.ImageOptions{ImageType: "png"}, 0, "")

	if err := pdf.OutputFileAndClose("qr-code.pdf"); err != nil {
		panic(err)
	}
}
```

Related example: `cd examples/external-qr-code && go run .`

## Drawing

Use drawing primitives for static shapes, diagrams, and visual treatments.

```go
package main

import (
	"math"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.RoundedRect(20, 30, 55, 28, 4, "1234", "DF")
	pdf.Circle(115, 44, 15, "D")
	pdf.Polygon(starPoints(60, 95, 20, 9), "DF")
	pdf.LinearGradient(100, 80, 70, 30, 30, 80, 170, 235, 245, 255, 0, 0, 1, 0)

	if err := pdf.OutputFileAndClose("drawing.pdf"); err != nil {
		panic(err)
	}
}

func starPoints(cx, cy, radius float64, count int) []document.Point {
	points := make([]document.Point, 0, count)
	for i := range count {
		angle := -math.Pi/2 + float64(i)*2*math.Pi/float64(count)
		r := radius
		if i%2 == 1 {
			r *= 0.48
		}
		points = append(points, document.Point{X: cx + math.Cos(angle)*r, Y: cy + math.Sin(angle)*r})
	}
	return points
}
```

Runnable example: `go run ./examples/drawing`

## Reusable Templates

Create a template once and reuse it on multiple pages.

```go
package main

import (
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	pdf := gopdfkit.New()
	header := pdf.CreateTemplate(func(tpl *document.Tpl) {
		tpl.SetFont("Helvetica", "B", 16)
		tpl.Text(20, 18, "Reusable template header")
		tpl.Line(20, 24, 190, 24)
	})

	for i := 0; i < 2; i++ {
		pdf.AddPage()
		pdf.UseTemplate(header)
	}

	if err := pdf.OutputFileAndClose("templates.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/templates`

## Import A Page

Import an existing page and draw it into a new PDF.

```go
package main

import (
	"bytes"

	"github.com/cssbruno/gopdfkit"
)

func main() {
	sourcePDF := []byte("%PDF ...")

	pdf := gopdfkit.New()
	pageID := pdf.ImportPageStream(bytes.NewReader(sourcePDF), 1, "MediaBox")
	if pdf.Err() {
		panic(pdf.Error())
	}

	pdf.AddPage()
	pdf.UseImportedPage(pageID, 25, 35, 150, 0)

	if err := pdf.OutputFileAndClose("import-page.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/import-page`

## Merge Pages

Import all pages from each source and write them into one output.

```go
package main

import "github.com/cssbruno/gopdfkit/document"

func main() {
	pdf := document.New("P", "pt", "A4", "")
	for _, source := range [][]byte{firstPDF, secondPDF} {
		for _, id := range pdf.ImportPagesFromSource(source, "MediaBox") {
			pdf.AddPage()
			pdf.UseImportedPage(id, 0, 0, 0, 0)
		}
	}

	if err := pdf.OutputFileAndClose("merged-pages.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/merge-pdf-pages`

## Split Or Reorder Pages

Import selected page numbers from a source PDF in the order you need.

```go
package main

import (
	"bytes"

	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	source := sourcePDF
	pdf := document.New("P", "pt", "A4", "")
	for _, pageNo := range []int{4, 2, 1, 3} {
		id := pdf.ImportPageStream(bytes.NewReader(source), pageNo, "MediaBox")
		pdf.AddPage()
		pdf.UseImportedPage(id, 0, 0, 0, 0)
	}

	if err := pdf.OutputFileAndClose("reordered-pages.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/split-reorder-pages`

## Rotate Pages

Read page sizes, add output pages with rotation, then place the imported pages.

```go
package main

import (
	"bytes"

	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	source := sourcePDF
	sizes, err := document.GetPageSizes(source)
	if err != nil {
		panic(err)
	}

	pdf := document.New("P", "pt", "A4", "")
	for pageNo, rotation := range []int{0, 90, 270} {
		pageNo++
		id := pdf.ImportPageStream(bytes.NewReader(source), pageNo, "MediaBox")
		pdf.AddPageFormatRotation("P", sizes[pageNo]["MediaBox"], rotation)
		pdf.UseImportedPage(id, 0, 0, 0, 0)
	}

	if err := pdf.OutputFileAndClose("rotated-pages.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/rotate-pages`

## Watermark Pages

Draw imported pages first, then overlay the watermark.

```go
package main

import "github.com/cssbruno/gopdfkit/document"

func main() {
	pdf := document.New("P", "pt", "A4", "")
	for _, id := range pdf.ImportPagesFromSource(sourcePDF, "MediaBox") {
		pdf.AddPage()
		pdf.UseImportedPage(id, 0, 0, 0, 0)
		pdf.AddTextWatermark("CONFIDENTIAL")
	}

	if err := pdf.OutputFileAndClose("watermarked.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/watermark-pdf`

## Four-Up Pages

Place several imported pages onto one output page.

```go
package main

import "github.com/cssbruno/gopdfkit/document"

func main() {
	pdf := document.New("P", "pt", "A4", "")
	slots := [][2]float64{{36, 42}, {326, 42}, {36, 440}, {326, 440}}

	for i, id := range pdf.ImportPagesFromSource(sourcePDF, "MediaBox") {
		if i%len(slots) == 0 {
			pdf.AddPage()
		}
		slot := slots[i%len(slots)]
		pdf.UseImportedPage(id, slot[0], slot[1], 0, 350)
	}

	if err := pdf.OutputFileAndClose("four-up-pages.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/four-up-pages`

## Template Overlay

Import a template page and draw new text over it.

```go
package main

import (
	"bytes"

	"github.com/cssbruno/gopdfkit"
)

func main() {
	pdf := gopdfkit.New()
	templateID := pdf.ImportPageStream(bytes.NewReader(templatePDF), 1, "MediaBox")
	pdf.AddPage()
	pdf.UseImportedPage(templateID, 0, 0, 0, 0)
	pdf.SetFont("Helvetica", "B", 12)
	pdf.Text(36, 96, "Generated overlay text")

	if err := pdf.OutputFileAndClose("template-overlay.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/template-overlay`

## Static Form

Static forms are generated page layouts, not interactive AcroForms.

```go
package main

import "github.com/cssbruno/gopdfkit"

func main() {
	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.Text(16, 24, "Intake Form")
	pdf.SetFont("Helvetica", "", 10)
	pdf.Text(16, 46, "Name")
	pdf.Line(16, 52, 110, 52)
	pdf.Text(16, 68, "Reviewer notes")
	pdf.Rect(16, 74, 178, 42, "D")

	if err := pdf.OutputFileAndClose("form.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/form-creation`

## Password Protection And Attachments

Password protection applies to newly generated output. Attachments can be added
as document-level files.

```go
package main

import (
	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/document"
)

func main() {
	pdf := gopdfkit.New()
	pdf.SetProtection(document.CnProtectPrint, "reader", "owner")
	pdf.SetAttachments([]document.Attachment{{
		Content:     []byte("attached notes"),
		Filename:    "notes.txt",
		Description: "Generated notes",
	}})

	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.MultiCell(0, 7, "This PDF is password-protected and has an attachment.", "", "L", false)

	if err := pdf.OutputFileAndClose("protected.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable examples: `go run ./examples/protect-pdf`,
`go run ./examples/protection-attachments`

## Compression

Enable `Optimize` at construction time for generated page and template streams.

```go
package main

import "github.com/cssbruno/gopdfkit/document"

func main() {
	pdf := document.NewWithOptions(document.Options{
		OrientationStr: "P",
		UnitStr:        "mm",
		SizeStr:        "A4",
		Optimize:       true,
	})
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(0, 8, "Compressed generated streams")

	if err := pdf.OutputFileAndClose("compressed.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/compress-optimize-pdf`

## UTF-8 Font

Register a TrueType font with `AddUTF8Font` before using Unicode text.

```go
package main

import "github.com/cssbruno/gopdfkit"

func main() {
	pdf := gopdfkit.New()
	pdf.AddUTF8Font("DejaVu", "", "assets/static/font/DejaVuSansCondensed.ttf")
	pdf.AddPage()
	pdf.SetFont("DejaVu", "", 12)
	pdf.MultiCell(0, 7, "Unicode text: Olá, Здравствуйте, مرحبا", "", "L", false)

	if err := pdf.OutputFileAndClose("utf8-font.pdf"); err != nil {
		panic(err)
	}
}
```

Runnable example: `go run ./examples/utf8-font`

## Signing With CMS

The signing API uses CMS terminology. `OutputSignedFile` signs the PDF while
writing it.

```go
package main

import (
	"crypto"
	"time"

	"github.com/cssbruno/gopdfkit"
	"github.com/cssbruno/gopdfkit/sign"
)

func main() {
	cert, signer := loadCertificateAndSigner()

	pdf := gopdfkit.New()
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)
	pdf.Cell(40, 10, "Signed PDF example")

	if err := pdf.OutputSignedFile("signed.pdf", sign.Options{
		Signer:          signer,
		Certificate:     cert,
		DigestAlgorithm: crypto.SHA256,
		Name:            "Example Signer",
		Reason:          "Generated by GoPDFKit",
		SigningTime:     time.Now().UTC(),
	}); err != nil {
		panic(err)
	}
}
```

Use `sign.CreateCMS`, `sign.DecodeCMS`, `sign.EmbedDetachedCMS`, and
`sign.VerifyDetachedCMS` when an external service returns a detached CMS
signature.

Runnable example: `go run ./examples/sign-pdf`

## Inspect Generated PDFs

Use `inspect` for lightweight generated-output checks.

```go
package main

import "github.com/cssbruno/gopdfkit/inspect"

func main() {
	pdfBytes := []byte("%PDF ...")

	count, err := inspect.PageCount(pdfBytes)
	if err != nil {
		panic(err)
	}
	text, err := inspect.Text(pdfBytes)
	if err != nil {
		panic(err)
	}
	_, _ = count, text
}
```

## Runnable Example Index

| Workflow | Command | Output |
| --- | --- | --- |
| Hello world | `go run ./examples/hello-world` | `hello-world.pdf` |
| Add images to pages | `go run ./examples/add-images-to-pages` | `images-on-pages.pdf` |
| Compression | `go run ./examples/compress-optimize-pdf` | `compressed-optimized.pdf`, `uncompressed-debug.pdf` |
| Drawing primitives | `go run ./examples/drawing` | `drawing.pdf` |
| Static form document | `go run ./examples/form-creation` | `form-creation.pdf` |
| Four-up pages | `go run ./examples/four-up-pages` | `four-up-pages.pdf` |
| Headers and footers | `go run ./examples/headers-footers` | `headers-footers.pdf` |
| HTML CSS styles | `go run ./examples/html-css-styles` | `html-css-styles.pdf` |
| HTML fragment | `go run ./examples/html-fragment` | `html-fragment.pdf` |
| HTML images and SVG | `go run ./examples/html-images` | `html-images.pdf` |
| HTML tables | `go run ./examples/html-tables` | `html-tables.pdf` |
| HTML template values | `go run ./examples/html-template` | `html-template.pdf` |
| Image from memory | `go run ./examples/image-from-memory` | `image-from-memory.pdf` |
| Import page | `go run ./examples/import-page` | `import-page.pdf` |
| Invoice | `go run ./examples/invoice` | `invoice.pdf` |
| Merge pages | `go run ./examples/merge-pdf-pages` | `merged-pages.pdf` |
| Document pagination | `go run ./examples/pagination-document` | `pagination-document.pdf` |
| Manual table pagination | `go run ./examples/pagination-table` | `pagination-table.pdf` |
| Password and attachments | `go run ./examples/protection-attachments` | `protection-attachments.pdf` |
| Password protection | `go run ./examples/protect-pdf` | `protected-password.pdf` |
| Report | `go run ./examples/report` | `gopdfkit-report.pdf` |
| Rendering gallery | `go run ./examples/rendering-gallery` | many generated PDFs |
| Rotate pages | `go run ./examples/rotate-pages` | `rotated-pages.pdf` |
| Signing | `go run ./examples/sign-pdf` | `signed.pdf` |
| Split and reorder pages | `go run ./examples/split-reorder-pages` | `split-page-2.pdf`, `reordered-pages.pdf` |
| Styled paragraphs | `go run ./examples/styled-paragraphs` | `styled-paragraphs.pdf` |
| Structured report | `go run ./examples/structured-report` | `structured-report.pdf` |
| Table report | `go run ./examples/table-report` | `gopdfkit-tables.pdf` |
| Template overlay | `go run ./examples/template-overlay` | `template-overlay.pdf` |
| Reusable templates | `go run ./examples/templates` | `templates.pdf` |
| Thumbnail | `go run ./examples/thumbnail` | `thumbnail.pdf` |
| UTF-8 font | `go run ./examples/utf8-font` | `utf8-font.pdf` |
| Watermark | `go run ./examples/watermark-pdf` | `watermarked.pdf` |
| External QR code module | `cd examples/external-qr-code && go run .` | `qr-code.pdf` |
