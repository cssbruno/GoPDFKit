# PDF/HTML Rendering Implementation Checklist

Use this checklist to track what is missing before the project can rely on a
shared PDF-focused HTML/document renderer instead of many custom `gopdfkit`
drawing paths.

Status legend:

- missing: not implemented
- partial: implemented but needs hardening
- done: implemented

## 1. Renderer Contract

- [x] Create `doc/pdf-html-subset.md`.
- [x] List all supported HTML tags.
- [x] List all supported attributes.
- [x] List all supported CSS properties.
- [x] List all supported helper classes and `data-*` attributes.
- [x] Document unsupported HTML/CSS behavior.
- [x] Add examples for long-form documents, reports, letters, forms, and free-text
      documents.
- [x] Add a validation mode that reports unsupported tags/CSS before rendering.

## 2. Shared Document Model

- [x] Define a shared `Document` model.
- [x] Define `DocumentKind`.
- [x] Define shared metadata fields for parties, authors, dates, IDs, document
      title, and verification URL.
- [x] Define `HeaderBlock`.
- [x] Define `FooterBlock`.
- [x] Define `Body []Block`.
- [x] Define `SignatureBlock`.
- [x] Define `QRBlock`.
- [x] Define `AttachmentBlock` if PDF attachments remain in scope.
- [x] Make document-specific renderers build the model instead of drawing
      directly.

## 3. Shared Block Types

- [x] `ParagraphBlock`
- [x] `HeadingBlock`
- [x] `ListBlock`
- [x] `TableBlock`
- [x] `ImageBlock`
- [x] `SignatureRowBlock`
- [x] `MetadataGridBlock`
- [x] `QRVerificationBlock`
- [x] `NoteBoxBlock`
- [x] `SectionBlock`
- [x] `ClauseBlock`
- [x] `PageBreakBlock`

## 4. Block Measurement

- [x] Add a measure pass before rendering.
- [x] Each block reports width/height before drawing.
- [x] Blocks can declare whether they are splittable.
- [x] Blocks can declare minimum height needed before drawing.
- [x] Renderer can ask whether a block fits on the current page.
- [x] Renderer can move a block to the next page before drawing.
- [x] Measurement and rendering use the same fonts, widths, margins, and line
      heights.

## 5. Pagination

- [x] `break-before`
- [x] `page-break-before`
- [x] `break-after`
- [x] `page-break-after`
- [x] `break-inside: avoid`
- [x] `page-break-inside: avoid`
- [x] Keep heading with next block.
- [x] Keep image with caption.
- [x] Keep signature row together.
- [x] Keep QR + verification text together.
- [x] Keep metadata row together.
- [x] Avoid orphan table rows where possible.
- [x] Reserve footer space before laying out body blocks.
- [x] Support alternate footer height separately from normal footer height.

## 6. Tables

- [x] Parse `thead`.
- [x] Repeat `thead` after page breaks.
- [x] Parse `tbody`.
- [x] Parse `tfoot`.
- [x] Render `tfoot` if required.
- [x] Parse `caption`.
- [x] Support `colspan`.
- [x] Support `rowspan`.
- [x] Support per-cell `text-align`.
- [x] Support per-cell `vertical-align`.
- [x] Support per-cell padding.
- [x] Support per-cell background color.
- [x] Support per-cell border.
- [x] Support `border-collapse`.
- [x] Add a table width algorithm.
- [x] Add row height measurement.
- [x] Move unsplittable rows to next page.
- [x] Split large tables across pages.
- [x] Add table regression samples for long-form documents and forms.

## 7. Box Model

- [x] `margin`
- [x] `margin-top`
- [x] `margin-right`
- [x] `margin-bottom`
- [x] `margin-left`
- [x] `padding`
- [x] `padding-top`
- [x] `padding-right`
- [x] `padding-bottom`
- [x] `padding-left`
- [x] `border`
- [x] `border-width`
- [x] `border-color`
- [x] `border-style`
- [x] Per-side border widths.
- [x] Per-side border colors.
- [x] Per-side border styles.
- [x] `background-color`
- [x] Optional `border-radius` intentionally unsupported unless required by a
      design system.

## 8. Typography

- [x] `font-size`
- [x] `font-family`
- [x] `font-weight`
- [x] `font-style`
- [x] `text-decoration`
- [x] `line-height`
- [x] `text-align`
- [x] `vertical-align`
- [x] Strict font policy: unsupported font families fail clearly.
- [x] Strict font policy: unavailable bold/italic faces fail clearly.
- [x] Long-word wrapping policy.
- [x] Optional hyphenation intentionally unsupported.
- [x] Better nested list marker alignment.
- [x] Paragraph spacing rules.
- [x] Heading spacing rules.

## 9. Images

- [x] Shared image block.
- [x] Data URL image decoding.
- [x] Max data URL image size.
- [x] Local image policy.
- [x] Remote image policy.
- [x] `width`
- [x] `height`
- [x] `max-width`
- [x] `max-height`
- [x] Fit mode: contain.
- [x] Fit mode: cover if needed.
- [x] Align left/center/right.
- [x] Image captions.
- [x] Keep image with caption.
- [x] DPI behavior documented and tested.

## 10. Header/Footer/Page Chrome

- [x] Shared `PageChrome` abstraction.
- [x] Shared header renderer.
- [x] Shared footer renderer.
- [x] Per-document header options.
- [x] Per-document footer options.
- [x] Page number support.
- [x] Total page count alias support.
- [x] Footer reserved-height measurement.
- [x] External footer extraction into standard footer block.
- [x] Header/footer golden tests.

## 11. Signature Blocks

- [x] Shared signature block.
- [x] One-column signature layout.
- [x] Two-column signature layout.
- [x] Optional primary party signature.
- [x] Optional secondary party signature.
- [x] Optional metadata below signature.
- [x] Keep signature block together.
- [x] Digital-signature placeholder compatibility.
- [x] Signature visual regression tests.

## 12. QR/Verification Blocks

- [x] Shared QR block.
- [x] Shared QR + text side-by-side block.
- [x] Configurable QR size.
- [x] Configurable label text.
- [x] Optional verification URL text.
- [x] Keep QR block together.
- [x] QR generation failure behavior.
- [x] QR visual regression tests.

## 13. Form HTML Pipeline

- [x] Generate one canonical HTML representation for form documents.
- [x] Validate form HTML against the supported subset.
- [x] Standardize question/answer blocks.
- [x] Standardize grouped sections.
- [x] Standardize tables/lists for structured answers.
- [x] Add page-break policy for grouped questions.
- [x] Add form golden PDFs.

## 14. Renderer Safety

- [x] Max HTML input size.
- [x] Max DOM/block depth.
- [x] Max table rows.
- [x] Max table columns.
- [x] Max image bytes.
- [x] Max generated pages guard.
- [x] Unsupported tag policy.
- [x] Unsupported CSS policy.
- [x] Unsafe link policy.
- [x] Unsafe image source policy.
- [x] Deterministic error messages.
- [x] Strict invalid-content behavior: invalid content fails clearly.

## 15. Tests

- [x] Unit tests for parser.
- [x] Unit tests for CSS subset parsing.
- [x] Unit tests for block measurement.
- [x] Unit tests for table measurement.
- [x] Unit tests for pagination.
- [x] Unit tests for image sizing.
- [x] Unit tests for signature block layout.
- [x] Unit tests for QR block layout.
- [x] Golden PDF for structured report.
- [x] Golden PDF for tabular report.
- [x] Golden PDF for transactional document.
- [x] Golden PDF for attestation-style document.
- [x] Golden PDF for statement-style document.
- [x] Golden PDF for generic/free-text document.
- [x] Golden PDF for long-form document.
- [x] Golden PDF for form document.
- [x] Text extraction assertions for required content.
- [x] Page count assertions.
- [x] Regression tests for unsupported HTML/CSS.

## 16. Migration Tasks

- [x] Convert structured report renderer into a document builder.
- [x] Convert transactional renderer into a document builder.
- [x] Convert attestation-style renderer into a document builder.
- [x] Convert statement-style renderer into a document builder.
- [x] Convert generic/free-text renderer into a document builder.
- [x] Convert long-form document renderer into a document builder plus footer
      config.
- [x] Convert form HTML paths to supported subset generators.
- [x] Remove duplicated page-space checks from document-specific files.
- [x] Remove duplicated signature drawing from document-specific files.
- [x] Remove duplicated QR drawing from document-specific files.
- [x] Remove duplicated metadata drawing from document-specific files.

## Recommended Next Work Order

1. Freeze the supported HTML/CSS contract.
2. Extract shared block interfaces and measurement context.
3. Move signature, QR, metadata, note box, and paragraph rendering into blocks.
4. Add robust pagination and keep-together behavior.
5. Upgrade tables with `thead`, `colspan`, `rowspan`, and row measurement.
6. Convert one simple document type to the shared model.
7. Convert long-form documents and forms after table/pagination behavior is
   stable.

## Definition Of Done

- [x] New document types can be implemented by building blocks, not drawing
      coordinates directly.
- [x] Long-form documents render from sanitized supported HTML.
- [x] Forms render from generated supported HTML.
- [x] Tables split predictably across pages.
- [x] Signature and QR blocks are shared.
- [x] Headers and footers are shared.
- [x] Page breaks are mostly declarative.
- [x] Unsupported HTML/CSS is reported clearly.
- [x] Golden PDF tests cover all critical document kinds.
