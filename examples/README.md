# PaperRune Examples

Each directory is a runnable example. Generated PDFs are written under
`assets/generated/pdf/examples`.

## Brazilian Laboratory Report References

The hemogram examples and the three-report laboratory suite synthesize public
Brazilian conventions instead of copying one laboratory's brand or a patient's
report:

- [Anvisa RDC 978/2025 guidance](https://www.gov.br/anvisa/pt-br/assuntos/noticias-anvisa/2025/anvisa-esclarece-duvidas-sobre-servicos-que-realizam-exames-de-analises-clinicas-eac)
  informed the identification, CNES, collection, method, reference interval,
  issue, and professional-signature fields.
- [Hilab's public hemogram guide](https://hilab.com.br/exames/hemograma/)
  informed the patient-friendly current-result, reference-range, and visual
  status treatment.
- [SES-GO's hemogram implementation guide](https://fhir-homolog.saude.go.gov.br/r4/exame/hemograma.html)
  informed the structured observation, unit, specimen, and historical-result
  model.
- [Brazilian Diabetes Society 2025 diagnostic guideline](https://diretriz.diabetes.org.br/diagnostico-de-diabetes-mellitus/)
  informed the demonstrative fasting-glucose and HbA1c interpretation note.
- [Fleury's public urinalysis catalog](https://www.fleury.com.br/medicos/exames/urina-exame-de-varios-materiais)
  informed the material, collection, physical-chemical, and sediment sections.
- [Anvisa's clinical microbiology manuals](https://www.gov.br/anvisa/pt-br/centraisdeconteudo/publicacoes/servicosdesaude/manuais/manuais-de-microbiologia-clinica)
  informed the culture, organism, and susceptibility-reporting structure.
- [BrCAST's current documents](https://brcast.org.br/documentos/documentos-3/)
  informed the labels for the S, I, and R antimicrobial susceptibility
  categories. No real breakpoint table is reproduced.

Every person, identifier, credential, address, result, verification code, and
laboratory name in these examples is fictional. The prominent warning is
intentional: generated files are product demos, not clinical documents.

## Brazilian Clinical Document References

The `medical-document-suite` and `paper-receituario-a5` examples use current
public Brazilian structures without producing usable prescriptions:

- [Anvisa's current physical receituary models](https://www.gov.br/anvisa/pt-br/assuntos/medicamentos/controlados/sncr/receituario-fisico/modelos-vigentes)
  informed the field organization for the special-control, type A, and type B
  demonstrations.
- [Anvisa's physical receituary transition guidance](https://www.gov.br/anvisa/pt-br/assuntos/medicamentos/controlados/sncr/modelos-de-receituarios)
  informed the 2026 version note and the distinction between official printing
  and a software demonstration.
- [Anvisa's SNCR overview](https://www.gov.br/anvisa/pt-br/assuntos/medicamentos/controlados/sncr)
  informed the explicit absence of valid sanitary numbering in the examples.

The controlled-prescription examples contain prominent invalidity watermarks,
zeroed credentials, no valid SNCR number, and no real controlled medication.
They demonstrate PDF layout only and must not be used for prescribing or
dispensing.

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
go run ./examples/lab-report-suite
go run ./examples/medical-document-suite
# See examples/paper-lab-report and examples/paper-receituario-a5 for .paper CLI examples.
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
| Modern Brazilian hemogram HTML template | `go run ./examples/lab-hemograma-html` | `lab-hemograma-html.pdf` |
| Modern Brazilian hemogram drawn report | `go run ./examples/lab-hemograma-report` | `lab-hemograma-report.pdf` |
| Brazilian laboratory report suite | `go run ./examples/lab-report-suite` | `lab-bioquimica-report.pdf`, `lab-urinalise-report.pdf`, `lab-microbiologia-report.pdf` |
| Brazilian A5 exam request and prescription suite | `go run ./examples/medical-document-suite` | `pedido-exames-a5.pdf`, `receituario-a5.pdf`, monochrome A/B print files, and colored-stock A/B previews |
| Data-driven Brazilian `.paper` lab report | See `examples/paper-lab-report/README.md` | `/tmp/lab-report.pdf` |
| Data-driven A5 `.paper` prescriptions and themes | See `examples/paper-receituario-a5/README.md` | `output/pdf/receituario-a5-paper.pdf`, `output/pdf/receita-controle-especial-a5-demo.pdf`, and five variants under `output/pdf/controle-especial-temas/` |
| Merge pages | `go run ./examples/merge-pdf-pages` | `merged-pages.pdf` |
| Document pagination | `go run ./examples/pagination-document` | `pagination-document.pdf` |
| Manual table pagination | `go run ./examples/pagination-table` | `pagination-table.pdf` |
| Packing slip report | `go run ./examples/packing-slip-report` | `packing-slip-report.pdf` |
| Project status report | `go run ./examples/project-status-report` | `project-status-report.pdf` |
| Password and attachments | `go run ./examples/protection-attachments` | `protection-attachments.pdf` |
| Password protection | `go run ./examples/protect-pdf` | `protected-password.pdf` |
| Report | `go run ./examples/report` | `paperrune-report.pdf` |
| Rendering gallery | `go run ./examples/rendering-gallery` | many generated PDFs |
| Rotate pages | `go run ./examples/rotate-pages` | `rotated-pages.pdf` |
| Service invoice with flex cards | `go run ./examples/service-invoice-flex` | `service-invoice-flex.pdf` |
| Signing | `go run ./examples/sign-pdf` | `signed.pdf` |
| Split and reorder pages | `go run ./examples/split-reorder-pages` | `split-page-2.pdf`, `reordered-pages.pdf` |
| Styled paragraphs | `go run ./examples/styled-paragraphs` | `styled-paragraphs.pdf` |
| Structured report | `go run ./examples/structured-report` | `structured-report.pdf` |
| Table report | `go run ./examples/table-report` | `paperrune-tables.pdf` |
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
