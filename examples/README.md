# GoPDFKit Examples

Each directory is a runnable example. Generated PDFs are written under
`assets/generated/pdf/examples`.

## Run All Core Examples

```sh
go run ./examples/hello-world
go run ./examples/add-images-to-pages
go run ./examples/compress-optimize-pdf
go run ./examples/day-to-day-report
go run ./examples/drawing
go run ./examples/form-creation
go run ./examples/four-up-pages
go run ./examples/headers-footers
go run ./examples/html-css-styles
go run ./examples/html-flex-edge-cases
go run ./examples/html-fragment
go run ./examples/html-images
go run ./examples/html-tables
go run ./examples/html-template
go run ./examples/image-from-memory
go run ./examples/import-page
go run ./examples/invoice
go run ./examples/lab-hemograma-html
go run ./examples/lab-hemograma-report
# See examples/paper-lab-report for the data-driven .paper CLI example.
go run ./examples/merge-pdf-pages
go run ./examples/pagination-document
go run ./examples/pagination-table
go run ./examples/packing-slip-report
go run ./examples/project-status-report
go run ./examples/protection-attachments
go run ./examples/protect-pdf
go run ./examples/report
go run ./examples/rendering-gallery
go run ./examples/rotate-pages
go run ./examples/service-invoice-flex
go run ./examples/sign-pdf
go run ./examples/split-reorder-pages
go run ./examples/styled-paragraphs
go run ./examples/structured-report
go run ./examples/table-report
go run ./examples/template-overlay
go run ./examples/templates
go run ./examples/thumbnail
go run ./examples/utf8-font
go run ./examples/watermark-pdf
```

## Output Index

| Workflow | Command | Output |
| --- | --- | --- |
| Hello world | `go run ./examples/hello-world` | `hello-world.pdf` |
| Add images to pages | `go run ./examples/add-images-to-pages` | `images-on-pages.pdf` |
| Compression | `go run ./examples/compress-optimize-pdf` | `compressed-optimized.pdf`, `uncompressed-debug.pdf` |
| Day-to-day HTML/CSS report | `go run ./examples/day-to-day-report` | `day-to-day-report.pdf` |
| Drawing primitives | `go run ./examples/drawing` | `drawing.pdf` |
| Static form document | `go run ./examples/form-creation` | `form-creation.pdf` |
| Four-up pages | `go run ./examples/four-up-pages` | `four-up-pages.pdf` |
| Headers and footers | `go run ./examples/headers-footers` | `headers-footers.pdf` |
| HTML CSS styles | `go run ./examples/html-css-styles` | `html-css-styles.pdf` |
| HTML flex edge cases | `go run ./examples/html-flex-edge-cases` | `html-flex-edge-cases.pdf` |
| HTML fragment | `go run ./examples/html-fragment` | `html-fragment.pdf` |
| HTML images and SVG | `go run ./examples/html-images` | `html-images.pdf` |
| HTML tables | `go run ./examples/html-tables` | `html-tables.pdf` |
| Compiled HTML template values | `go run ./examples/html-template` | `html-template.pdf` |
| Image from memory | `go run ./examples/image-from-memory` | `image-from-memory.pdf` |
| Import page | `go run ./examples/import-page` | `import-page.pdf` |
| Invoice | `go run ./examples/invoice` | `invoice.pdf` |
| Brazilian lab hemograma HTML template | `go run ./examples/lab-hemograma-html` | `lab-hemograma-html.pdf` |
| Brazilian lab hemograma report | `go run ./examples/lab-hemograma-report` | `lab-hemograma-report.pdf` |
| Data-driven Brazilian `.paper` lab report | See `examples/paper-lab-report/README.md` | `/tmp/lab-report.pdf` |
| Merge pages | `go run ./examples/merge-pdf-pages` | `merged-pages.pdf` |
| Document pagination | `go run ./examples/pagination-document` | `pagination-document.pdf` |
| Manual table pagination | `go run ./examples/pagination-table` | `pagination-table.pdf` |
| Packing slip report | `go run ./examples/packing-slip-report` | `packing-slip-report.pdf` |
| Project status report | `go run ./examples/project-status-report` | `project-status-report.pdf` |
| Password and attachments | `go run ./examples/protection-attachments` | `protection-attachments.pdf` |
| Password protection | `go run ./examples/protect-pdf` | `protected-password.pdf` |
| Report | `go run ./examples/report` | `gopdfkit-report.pdf` |
| Rendering gallery | `go run ./examples/rendering-gallery` | many generated PDFs |
| Rotate pages | `go run ./examples/rotate-pages` | `rotated-pages.pdf` |
| Service invoice with flex cards | `go run ./examples/service-invoice-flex` | `service-invoice-flex.pdf` |
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

## Feature Gaps

These workflows are intentionally not covered because they are not implemented
as general-purpose features:

- Interactive AcroForm field creation
- Filling or flattening existing interactive AcroForms
- FDF import or merge
- Unlocking or decrypting existing password-protected PDFs
