# GoPDFKit Examples

Each directory is a runnable example:

```sh
go run ./examples/hello-world
go run ./examples/add-images-to-pages
go run ./examples/compress-optimize-pdf
go run ./examples/drawing
go run ./examples/form-creation
go run ./examples/four-up-pages
go run ./examples/headers-footers
go run ./examples/html-css-styles
go run ./examples/html-fragment
go run ./examples/invoice
go run ./examples/image-from-memory
go run ./examples/import-page
go run ./examples/merge-pdf-pages
go run ./examples/pagination-document
go run ./examples/pagination-table
go run ./examples/protection-attachments
go run ./examples/protect-pdf
go run ./examples/report
go run ./examples/rendering-gallery
go run ./examples/rotate-pages
go run ./examples/split-reorder-pages
go run ./examples/styled-paragraphs
go run ./examples/structured-report
go run ./examples/sign-pdf
go run ./examples/table-report
go run ./examples/template-overlay
go run ./examples/templates
go run ./examples/thumbnail
go run ./examples/utf8-font
go run ./examples/watermark-pdf
```

The `external-qr-code` example is a separate module because it intentionally
uses a barcode dependency outside GoPDFKit:

```sh
cd examples/external-qr-code
go run .
```

Generated PDFs are written under `assets/generated/pdf/examples`. The focused
document workflow examples produce:

- `gopdfkit-report.pdf`
- `gopdfkit-tables.pdf`
- `invoice.pdf`
- `styled-paragraphs.pdf`
- `html-css-styles.pdf`
- `pagination-table.pdf`
- `pagination-document.pdf`
- `merged-pages.pdf`
- `split-page-2.pdf`
- `reordered-pages.pdf`
- `rotated-pages.pdf`
- `images-on-pages.pdf`
- `compressed-optimized.pdf`
- `uncompressed-debug.pdf`
- `watermarked.pdf`
- `four-up-pages.pdf`
- `template-overlay.pdf`
- `form-creation.pdf`
- `protected-password.pdf`

Workflow coverage:

- Create PDF reports: `go run ./examples/report`
- Table PDF reports: `go run ./examples/table-report`
- Invoice creation: `go run ./examples/invoice`
- Styled paragraphs: `go run ./examples/styled-paragraphs`
- HTML CSS styles with border radius and box shadows: `go run ./examples/html-css-styles`
- Manual table pagination: `go run ./examples/pagination-table`
- Document-model pagination and explicit page breaks: `go run ./examples/pagination-document`
- Merge PDF pages: `go run ./examples/merge-pdf-pages`
- Split PDF pages and change page order: `go run ./examples/split-reorder-pages`
- Rotate pages: `go run ./examples/rotate-pages`
- Add images to pages: `go run ./examples/add-images-to-pages`
- Compress/optimize generated PDF streams: `go run ./examples/compress-optimize-pdf`
- Watermark PDF files: `go run ./examples/watermark-pdf`
- Put 4 imported pages on 1 page: `go run ./examples/four-up-pages`
- Load a PDF page template and draw modifications over it: `go run ./examples/template-overlay`
- Create a static filled form PDF: `go run ./examples/form-creation`
- Protect generated PDFs with a user/owner password: `go run ./examples/protect-pdf`

Current gaps:

- Interactive AcroForm field creation is not implemented yet.
- Filling and flattening existing interactive AcroForms is not implemented yet.
- FDF import/merge is not implemented yet.
- Unlocking/decrypting existing password-protected PDFs is not implemented yet.
