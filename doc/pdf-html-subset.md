# PDF HTML Subset

This document defines the HTML/CSS subset rendered by `HTMLNew()` into PDF
drawing operations. It is a PDF-focused rich-text renderer, not a browser
engine. Inputs should be normalized to this contract before rendering.

## HTML Render Support Table

Use this table as the quick checklist for HTML passed to `HTMLNew().Write()`.
Supported commands render into PDF drawing/text operations. Unsupported commands
are ignored, skipped, or reported by `HTML.ValidateHTML` / `HTML.DebugLog`
depending on where they appear.

| HTML command or feature | Works? | Render behavior |
| --- | --- | --- |
| `a href="..."` | Yes | Renders linked inline text. |
| `b`, `strong` | Yes | Renders bold text when the current font family supports bold. |
| `i`, `em` | Yes | Renders italic text when the current font family supports italic. |
| `u`, `ins` | Yes | Renders underlined text. |
| `s`, `strike`, `del` | Yes | Renders strikethrough text. |
| `sub`, `sup` | Yes | Renders smaller baseline-shifted text. |
| `code`, `kbd`, `samp` | Yes | Renders inline code-style text. |
| `br` | Yes | Inserts a line break. |
| `p`, `div`, `section`, `article`, `header`, `footer` | Yes | Renders block text with supported box styling. |
| `h1` through `h6` | Yes | Renders headings and keeps them with the next block when possible. |
| `pre` | Yes | Preserves whitespace according to supported whitespace handling. |
| `hr` | Yes | Renders a horizontal rule. |
| `center`, `left`, `right` | Yes | Applies simple block alignment. |
| `ul`, `ol`, `li` | Yes | Renders unordered and ordered lists. |
| `dl`, `dt`, `dd` | Yes | Renders definition lists. |
| `table`, `caption`, `thead`, `tbody`, `tfoot`, `tr`, `th`, `td` | Yes | Renders tables with the documented table behavior below. |
| `colspan`, `rowspan` | Yes | Applies cell spanning in supported tables. |
| `img` | Yes | Renders PNG, JPEG, and GIF data URLs; local images require `AllowLocalImages`. |
| `figure`, `figcaption` | Yes | Keeps image and caption together when they fit on one page. |
| Inline `svg` | Yes | Renders supported SVG content into PDF drawing operations. |
| `style` | Partial | CSS rules are collected; the tag body is not rendered as text. |
| `script`, `head` | Skipped | Content is skipped during body rendering. JavaScript is not executed. |
| Unknown custom tags | No | Unsupported tags are not part of the render contract and should be normalized before rendering. |
| Forms: `form`, `input`, `textarea`, `select`, `button` | No | Interactive form controls are not rendered as browser controls. Use text, tables, or the document model instead. |
| Embedded media: `video`, `audio`, `canvas`, `iframe`, `object`, `embed` | No | Embedded browser/runtime content is not rendered. |
| Browser layout tags used as layout engines | No | Tags may be skipped or flattened; flex/grid/browser layout behavior is not implemented. |

| Attribute or CSS command | Works? | Render behavior |
| --- | --- | --- |
| `id`, `class`, `style` | Yes | Used for supported CSS selection and inline declarations. |
| `href` on `a` | Yes | Creates a link target. |
| `src`, `alt`, `width`, `height`, `max-width`, `max-height`, `object-fit` on `img` | Yes | Controls supported image rendering and sizing. |
| `start`, `type` on lists | Yes | Controls supported list numbering/marker behavior. |
| `width`, `height`, `align`, `valign`, `bgcolor`, `border`, `bordercolor`, `colspan`, `rowspan` on table cells | Yes | Controls supported table/cell rendering. |
| Tag, class, ID, descendant, direct-child, and comma-separated selectors | Yes | Applies supported CSS declarations. |
| Text CSS: `color`, `font-family`, `font-size`, `font-style`, `font-weight`, `line-height`, `text-align`, `text-decoration`, `vertical-align`, `white-space` | Yes | Applies supported text styling. |
| Box CSS: `background`, `background-color`, borders, margins, and padding | Yes | Applies supported PDF box drawing. |
| Sizing CSS: `width`, `height`, `max-width`, `max-height` | Yes | Applies supported block, table, and image sizing. |
| Pagination CSS: `break-before`, `break-after`, `break-inside`, `page-break-before`, `page-break-after`, `page-break-inside` | Partial | Applies basic page-break behavior, not full paged-media layout. |
| Sibling selectors, attribute selectors, pseudo-classes, pseudo-elements | No | Unsupported selector forms are ignored/reported. |
| `@media`, `@page`, and other at-rules | No | Browser stylesheet at-rules are not implemented. |
| `display:flex`, `display:grid`, floats, positioning, `z-index`, `overflow` | No | Browser layout engines are not implemented. |
| CSS transforms, shadows, `border-radius` | No | Decorative browser effects are not implemented. |
| Remote CSS and remote images | No | Remote loading is rejected or unsupported for deterministic PDF generation. |

## Supported HTML Tags

Text and inline tags:

- `a`
- `b`, `strong`
- `i`, `em`
- `u`, `ins`
- `s`, `strike`, `del`
- `sub`, `sup`
- `code`, `kbd`, `samp`
- `br`

Block and document tags:

- `p`
- `div`
- `section`
- `article`
- `header`
- `footer`
- `h1`, `h2`, `h3`, `h4`, `h5`, `h6`
- `pre`
- `hr`
- `center`, `left`, `right`

Lists:

- `ul`
- `ol`
- `li`
- `dl`
- `dt`
- `dd`

Tables:

- `table`
- `caption`
- `thead`
- `tbody`
- `tfoot`
- `tr`
- `th`
- `td`

Media and skipped tags:

- `img`
- `figure`
- `figcaption`
- `svg`
- `style`
- `script`
- `head`

`style`, `script`, and `head` content is skipped during body rendering. CSS in
`style` tags is collected before rendering.

## Supported Attributes

Common attributes:

- `id`
- `class`
- `style`

Links:

- `href` on `a`

Images:

- `src`
- `alt`
- `width`
- `height`
- `max-width`
- `max-height`
- `object-fit`

Lists:

- `start` on `ol`
- `type` on `ol` and `ul`

Tables and cells:

- `width`
- `height`
- `align`
- `valign`
- `bgcolor`
- `border`
- `bordercolor`
- `colspan`
- `rowspan`

## Supported CSS Selectors

- Tag selectors: `p`
- Class selectors: `.note`
- ID selectors: `#patient`
- Tag-qualified class selectors: `td.total`
- Tag-qualified ID selectors: `table#items`
- Descendant selectors: `.section td`
- Direct-child selectors: `table > tr`
- Comma-separated selector lists

Unsupported selectors include sibling selectors, attribute selectors,
pseudo-classes, pseudo-elements, media queries, and other at-rules.

## Supported CSS Properties

Text:

- `color`
- `font-family`
- `font-size`
- `font-style`
- `font-weight`
- `line-height`
- `text-align`
- `text-decoration`
- `vertical-align`
- `white-space`

Lists:

- `list-style`
- `list-style-type`

Boxes:

- `background`
- `background-color`
- `border`
- `border-collapse`
- `border-width`
- `border-style`
- `border-color`
- `border-top`
- `border-top-width`
- `border-top-style`
- `border-top-color`
- `border-right`
- `border-right-width`
- `border-right-style`
- `border-right-color`
- `border-bottom`
- `border-bottom-width`
- `border-bottom-style`
- `border-bottom-color`
- `border-left`
- `border-left-width`
- `border-left-style`
- `border-left-color`
- `margin`
- `margin-top`
- `margin-right`
- `margin-bottom`
- `margin-left`
- `padding`
- `padding-top`
- `padding-right`
- `padding-bottom`
- `padding-left`

Sizing:

- `width`
- `height`
- `max-width`
- `max-height`

Pagination:

- `break-before`
- `break-after`
- `break-inside`
- `page-break-before`
- `page-break-after`
- `page-break-inside`

Headings keep with the following block, and figures keep images with captions,
when their measured height fits on a page.

## Current Table Behavior

The renderer supports `caption`, `thead`, `tbody`, and `tfoot`, repeated
`thead` rows after page breaks, `colspan`, `rowspan`, per-cell alignment,
per-cell vertical alignment, per-cell padding, per-cell background color, and
per-cell border style/color/width. `border-collapse: collapse` suppresses
duplicate internal cell borders for a single-stroke grid. Footer rows are
rendered after non-footer rows even if the source places `tfoot` before
`tbody`. Column widths combine explicit cell widths, width hints on spanning
cells, and longest-word minimums before distributing remaining table width.
When possible, table pagination avoids leaving a single body row at the bottom
of a page if that row and the following row fit together on the next page.

Still missing:

- browser-grade table auto-layout
- robust row keep-together behavior
- splitting oversized rows across pages

## Current Box Model Behavior

Block boxes support margin, padding, background color, whole-box borders, and
per-side border widths, colors, and styles. Per-side margins and paddings are
also supported. `border-radius`, shadows, floats, positioning, flexbox, and grid
are not supported.

## Current Image Behavior

The renderer supports PNG, JPEG, and GIF data URL images, `figure`/`figcaption`
captions, explicit `width` and `height`, `max-width`, `max-height`,
`object-fit: contain`, `object-fit: cover`, and left/center/right alignment.
Data URL images are size-limited by `HTML.MaxDataImageBytes`. HTML image
registration enables DPI reading, so PNG DPI metadata is honored when available.
When a figure's estimated image plus caption height fits on one page, the
renderer moves it to the next page instead of splitting the image from its
caption.

Local image paths are disabled by default and require `HTML.AllowLocalImages`.
Remote `http`/`https` images and `file:` URLs are rejected with deterministic
errors.

Long words that exceed their available text width are split as a fallback so
they do not force table cells or block boxes past their configured width.

## Project-Specific Contract

The base package does not define application-specific classes or `data-*`
attributes. Projects that generate legal terms, prescriptions, exam requests,
certificates, declarations, intake forms, signature rows, QR blocks, or metadata
grids should document their own accepted classes and `data-*` attributes here
or in a project-local extension of this file.

Recommended project classes:

- `.document-title`
- `.metadata-grid`
- `.note-box`
- `.signature-row`
- `.signature-column`
- `.qr-verification`
- `.legal-title`
- `.legal-clause`
- `.page-break`

Recommended project `data-*` attributes:

- `data-block`
- `data-keep-together`
- `data-page-break`
- `data-signature-role`
- `data-qr-url`

These names are not renderer features until the project maps them into
supported HTML/CSS or a shared block model.

## Unsupported Behavior

The renderer does not implement browser layout. In particular, it does not
support:

- flexbox or grid
- floats
- absolute or fixed positioning
- `z-index`
- `overflow`
- CSS transforms
- border radius
- shadows
- `@page`
- media queries
- widows and orphans
- JavaScript
- remote CSS loading
- browser-grade font shaping and fallback
- pixel-perfect HTML-to-PDF conversion

For pixel-faithful output, use a browser or a dedicated HTML-to-PDF engine and
import or post-process the result.

## Diagnostics

Use `HTML.ValidateHTML` to collect best-effort diagnostics for unsupported
HTML tags, unsupported CSS selectors, and unsupported CSS properties before
rendering. Set `HTML.DebugLog` to collect the same diagnostics during
rendering.

HTML rendering enforces configurable safety limits for input size, element
depth, table row count, data image bytes, and pages generated by one render
call.

`HTML.MaxHTMLBytes`, `HTML.MaxElementDepth`, `HTML.MaxTableRows`,
`HTML.MaxGeneratedPages`, and `HTML.MaxDataImageBytes` bound input
size, nested element depth, table size, pages spanned by a single render call,
and embedded image size. The renderer also caps parsed table columns.

## Project Examples

Legal terms should use headings, paragraphs, lists, tables, explicit
page-break controls, and a project-defined footer block. Avoid arbitrary nested
layout markup.

Prescriptions, exam requests, certificates, and declarations should generate a
shared metadata section, body paragraphs or tables, optional notes, signature
blocks, and optional QR verification blocks instead of drawing each document
directly.

Intake forms should normalize question groups into headings, paragraphs, lists,
and tables. Long free-text answers should use normal paragraphs or `pre` only
when whitespace must be preserved.
