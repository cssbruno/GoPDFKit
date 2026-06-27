Below is a **v0.5.6 non-HTML performance checklist** with **60 concrete pprof targets**. I’m treating these as “profile-first” tasks: each item is plausible from the source, but should be confirmed with `-cpuprofile`, `-memprofile`, or benchmarks before merging.

Core validation command:

```sh
go test ./document -run '^$' \
  -bench 'BenchmarkGenerationText|BenchmarkGenerationLongText|BenchmarkGenerationUTF8Text|BenchmarkGenerationTemplates|BenchmarkGenerationImportedPDFPages|BenchmarkGenerationBaselineNoCompliance|BenchmarkGenerationBaselineNoComplianceCachedImage|BenchmarkGenerationBaselineNoComplianceSigned|BenchmarkGenerationProtection|BenchmarkGenerationAttachments' \
  -benchmem -count=5
```

Then profile specific areas:

```sh
go test ./document -run '^$' -bench BenchmarkGenerationUTF8Text -benchmem -cpuprofile utf8.cpu -memprofile utf8.mem
go test ./document -run '^$' -bench BenchmarkGenerationImportedPDFPages -benchmem -cpuprofile import.cpu -memprofile import.mem
go test ./document -run '^$' -bench BenchmarkGenerationBaselineNoComplianceSigned -benchmem -cpuprofile sign.cpu -memprofile sign.mem
go test ./document -run '^$' -bench BenchmarkGenerationBaselineNoComplianceCachedImage -benchmem -cpuprofile image.cpu -memprofile image.mem

go tool pprof -top ./document.test utf8.cpu
go tool pprof -top ./document.test import.cpu
go tool pprof -top ./document.test sign.cpu
go tool pprof -top ./document.test image.cpu
```

## Checklist

### Core text/cell rendering

* [x] **Move `strings.Fields(txtStr)` inside the UTF-8 justification branch in `CellFormat`.** It is currently computed before checking whether justification is needed, so normal UTF-8 cells pay for word splitting unnecessarily.

* [x] **Compute `GetStringWidth(txtStr)` once per `CellFormat`.** Alignment and link rectangle creation can ask for the same width more than once. Cache it locally for the cell.

* [x] **Replace non-UTF8 `strings.ReplaceAll` escaping with append-based escaping.** Non-UTF8 `CellFormat` currently escapes `\`, `(`, and `)` through repeated `strings.ReplaceAll`, creating intermediate strings.

* [x] **Add append-based UTF-8-to-PDF-string encoding.** `utf8toutf16` creates a byte slice and returns a string; callers then escape and append it. A direct `appendEscapedUTF16BE(dst, s)` helper can reduce allocations.

* [x] **Mark UTF-8 used runes during encoding.** `CellFormat` currently encodes text and separately loops over runes to populate `usedRunes`. Do both in one pass.

* [x] **Avoid repeated UTF-8 word encoding in justified text.** The justified branch splits text into words and calls `utf8toutf16`/escape per word. Cache encoded word fragments within the cell.

* [x] **Remove full `[]rune` allocation from `Document.write`.** UTF-8 `write` converts the whole string to `[]rune` and later converts rune slices back to strings. Use byte offsets from `range` and slice the original string.

* [x] **Avoid `strings.ReplaceAll(txtStr, "\r", "")` when there is no carriage return.** `Document.write` always calls `ReplaceAll`; guard it with `strings.Contains`.

* [x] **Add a bounded string-width cache.** `GetStringWidth` delegates to `GetStringSymbolWidth`, which loops over bytes/runes every time. Repeated labels and table values should hit a small per-render/per-document cache.

* [x] **Fast-path single-byte text width.** For non-UTF8 strings without NUL, `GetStringSymbolWidth` can use a tight indexed byte loop without range conversion overhead.

* [x] **Fast-path ASCII UTF-8 width.** If `isCurrentUTF8` but the string is ASCII, width can often use byte iteration plus `Cw` or a cached ASCII width path instead of rune decoding.

* [x] **Avoid repeated `strings.Contains(alignStr, ...)` in `CellFormat`.** Parse alignment once into booleans or a small enum before text layout.

* [x] **Avoid repeated `strings.ToUpper(borderStr)` for common border values.** `CellFormat` uppercases every border string; fast-path `""` and `"1"` first.

* [x] **Pre-size `CellFormat` output buffers more accurately.** It calls `ensurePDFBuffer` multiple times as features are discovered. Estimate once from text, border, fill, color, and tag state.

* [x] **Specialize `Cell` and `Cellf` hot paths.** `Cell` always calls full `CellFormat`. A simpler internal path for no border/fill/link/alignment can reduce branching in text-heavy PDFs.

### Document-model renderer

* [x] **Use prefix widths for document-model table colspans.** `renderTableRow` and `measureRenderedTableRow` call `sumFloat64(widths[col:...])`, repeating slice scans per cell. This is the non-HTML equivalent of the HTML table-span issue.

* [x] **Cache row measurement results during document-model table rendering.** Rows are measured, then rendered. Store measured row height and measured cell widths to avoid recomputation.

* [x] **Avoid copying header/body/footer rows in `renderTable`.** It builds a new `rows` slice by appending header, body, and footer before iteration. Iterate the three slices in order.

* [x] **Avoid repeated `NewMeasureContext` calls inside render methods.** `renderBlock`, `renderParagraph`, `renderHeading`, `renderList`, `renderMetadataGrid`, and QR rendering repeatedly build measurement contexts. Cache one context per renderer page/content width where possible.

* [x] **Cache `contentWidth()` per render pass.** Many renderer methods call `r.contentWidth()` repeatedly, which is cheap but hot; store in the renderer when margins/page size are stable.

* [x] **Avoid `textSegmentsPlainText` recomputation.** Paragraphs, headings, captions, QR text, and image captions flatten segments to plain text during render. Store/carry pre-flattened text in measured blocks when possible.

* [x] **List marker width optimization.** `listMarkerWidth` formats and measures every marker to find the max. For decimal lists, derive width from the last item; for repeated render, cache marker strings/widths.

* [x] **Avoid repeated style merges.** `mergedTextStyle(NewMeasureContext(...).DefaultStyle, block.Style)` appears repeatedly. Use a cached default style and merge once for measure/render where possible.

* [x] **Measure nested table-cell blocks once.** `measureRenderedTableCell` loops over `cell.Blocks` and calls `MeasureBlock`; rendering later loops again through the same blocks. Store cell block measurements.

* [x] **Avoid per-cell margin mutation when rendering tables.** `renderTableCell` mutates `lMargin` and `rMargin` for every cell. A lower-level cell rendering API that accepts width/box directly could reduce state churn.

* [x] **Avoid drawing empty table-cell `MultiCell`.** For cells with no blocks, `renderTableCell` calls `MultiCell` with an empty string. If only structure tagging is needed, emit less content.

* [x] **Optimize metadata grid string building.** `renderMetadataGrid` concatenates `Label + ": " + Value` per field. Use a builder or precomputed display string for large grids.

* [x] **Avoid repeated footer/header rendering measurements.** Page templates render header/footer around every page break. Cache static header/footer measurements and, where safe, static drawing templates.

### Images

* [x] **Replace `generateImageID` gob encoding with explicit hash writes.** It currently gob-encodes image bytes, mask, palette, metadata, and fields into SHA-1. Gob is flexible but expensive for hot image registration.

* [x] **Avoid hashing full inline image data repeatedly in document-model images.** `documentImageName` hashes format plus all image bytes every render. Let `ImageBlock` carry a stable name or cache names for repeated data.

* [x] **Add a file image cache keyed by path/stat/options.** `RegisterImageOptions` opens and parses a file per document unless callers manually use `ImageCache`. A built-in file cache helper would speed server workloads.

* [x] **Avoid repeated `strings.ToLower(options.ImageType)` in repeated registrations.** Normalize image type once in the caller or image cache.

* [x] **Avoid repeated file-extension scanning for image type.** `RegisterImageOptions` uses `LastIndex` on the file path when `ImageType` is empty. Cache inferred type with the image cache.

* [x] **Use append-based image placement output.** `imageOut` builds placement content with `sprintf`, converts to `[]byte`, then wraps tagged content. Replace with append-based numeric helpers.

* [x] **Use `drawImageXObject` or remove duplicate formatting path.** There is a `drawImageXObject` helper, but `imageOut` repeats a similar formatted string. Consolidate and optimize once.

* [x] **Avoid sorting image keys unless deterministic output is required.** `putimages` always builds `keyList`; sorting only happens with `catalogSort`, but key-list construction still costs for many images. If sorting is off, iterate the map directly.

* [x] **Avoid recursive `putimage` for soft masks where possible.** `putimage` creates a new `ImageInfo` and recursively calls itself for masks. A specialized grayscale mask writer could reduce validation and branching.

* [x] **Optimize transparency mask formatting.** Image `/Mask` output uses `fmtBuffer.printf` per transparency value. Use append-int helpers.

### PDF output and serialization

* [x] **Replace hot `outf`/`sprintf` calls with append-based output helpers.** Object refs, numeric arrays, image placement, page boxes, annotations, xref rows, and stream headers are hot serialization paths.

* [x] **Replace `fmtBuffer.printf` in page annotations and `/Kids`.** `putpages` builds annotation and kids arrays with `fmtBuffer.printf`. Use append-int/reference helpers.

* [x] **Avoid string conversion in `putImportedTemplates`.** It writes imported object bytes with `f.out(string(objsIDData[i]))`, converting whole object bodies to string. Use `outbytes` or `outbuf` with bytes directly.

* [x] **Optimize imported template object ID patching.** It uses `fmt.Sprintf("%40d", objID)` per replacement. Use a fixed `[40]byte` stack buffer and `strconv.AppendInt`.

* [x] **Avoid alias sort/replacer rebuild on every close.** `replaceAliases` builds aliases, sorts them, creates pairs/needles, and constructs a replacer each close. Compile alias replacement state when aliases are registered.

* [x] **Track pages containing aliases at write time.** `replaceAliases` scans every page buffer for each alias. Mark pages when aliases are written, then only process those pages.

* [x] **Avoid full page buffer string conversion during alias replacement.** When a page contains an alias, it converts the whole page buffer to string and replaces. Use byte replacement for simple aliases or in-place-safe rewriting if replacement length is equal.

* [x] **Parallelize page compression for large documents.** `putpages` compresses pages serially. For documents with many pages and compression enabled, compress pages concurrently, then emit in order.

* [x] **Reduce compressed stream copy.** `sliceCompressLevel` writes to a pooled buffer, then copies compressed bytes with `append([]byte(nil), buf.Bytes()...)`. For large streams, allow direct consumption or use an owned byte-slice pool.

* [x] **Avoid compression for tiny streams.** Compression setup can cost more than it saves for very small page/template/metadata streams. Add a threshold or benchmark-based heuristic.

* [x] **Pre-size final PDF buffer.** `buffer` grows through output. Estimate final size from page buffers, images, fonts, imported objects, and stream headers before `enddoc`. This targets `runtime.growslice`/`bytes.Buffer.grow`.

* [x] **Specialize xref row formatting.** Xref rows are fixed-width numeric records. Use a stack buffer instead of formatting strings.

### UTF-8 font subsetting

* [x] **Pre-size `offsets`, `glyfData`, `hmtxData`, and `locaData`.** `GenerateCutFont` grows these slices while iterating glyphs. Precompute capacity from `symbolCollectionKeys` and glyph bounds.

* [x] **Pass `glyfData` into composite glyph recursion.** `getSymbols` calls `utf.getTableData("glyf")` inside recursion. Fetch once and pass the slice.

* [x] **Patch font tables in place instead of using `splice`.** `splice` clones whole streams for fixed-width replacements. For owned table data, use `binary.BigEndian.PutUint16`.

* [x] **Avoid `make([]byte, padding)` per glyph.** Glyph padding appends newly allocated zero slices. Use a fixed zero array or branch append zeros.

* [x] **Avoid map-of-map `symbolData` for composite glyph metadata.** `utf.symbolData` is `map[int]map[string][]int`; if only component symbols are needed, use `map[int][]int` or a small struct.

* [x] **Avoid repeated `getMetrics` seeking.** `GenerateCutFont` calls `getMetrics` per glyph. Cache hmtx table data and read offsets directly.

* [x] **Avoid sorting or repeated key extraction where stable order already exists.** Font subsetting builds symbol collections and keys; ensure the symbol key order is maintained without repeated map-key extraction/sort.

* [x] **Cache generated subsets for identical used-rune sets.** In server workloads generating many PDFs with the same language/text template, the used-rune set can repeat. Cache subset by font ID plus sorted used-rune hash.

* [x] **Optimize `utf8ToUnicodeCMap`.** It uses `fmt.Sprintf` 256 times to build a constant CMap. Precompute it as a package-level constant or build with append-based hex formatting.

* [x] **Reduce `fileReader.Read` zero-fill allocation on error paths.** Error handling allocates `make([]byte, s)` for out-of-range reads. This is mostly malformed input, but fuzz/security tests may show allocation spikes.

### Imported PDFs

* [x] **Avoid unconditional copy in `importpdf.OpenBytes`.** It copies input bytes before parsing. Add an unsafe/trusted immutable variant or a reusable `Source` workflow.

* [x] **Avoid `io.ReadAll` for large PDF imports.** `OpenReader` reads the entire source into memory. A `ReaderAt` parser could avoid full copy and enable targeted reads.

* [x] **Cache parsed imported sources across documents.** Repeated imports of the same source currently need parsing unless users manage a source object. Make the reusable `Source` path more prominent or add document-level source cache.

* [x] **Avoid rewriting imported object refs every output when the mapping is unchanged.** `putImportedPages` rewrites indirect refs for every imported object and resources. Cache rewritten bodies per imported page mapping shape.

* [x] **Avoid sorting imported page IDs if insertion order is sufficient.** `putImportedPages` builds and sorts IDs. For deterministic output keep it; otherwise consider preserving insertion order.

* [x] **Avoid compressing imported page content again when already FlateDecode-compatible.** If imported content is already compressed and valid, preserve it instead of uncompressing/recompressing or copying through compression.

### Signing

* [x] **Avoid materializing `signedContent`.** `signPDFContext` builds `signedContent` by copying both byte ranges before CMS signing. Hash byte ranges directly or expose ranged digest input to CMS creation.

* [x] **Avoid full `output` copy when signing large PDFs.** Signing appends input plus increment into a new output buffer. Consider writing incrementally or using an output buffer supplied by caller.

* [x] **Avoid `strings.Repeat` for large signature placeholders.** `placeholderHex := strings.Repeat("0", signatureBytes*2)` allocates potentially large strings. Generate bytes directly into the increment.

* [x] **Avoid repeated string conversions of PDF dictionaries.** `buildIncrement` reads dictionaries as bytes, modifies them, then calls `string(rootDict)` and `string(pageDict)` to write objects. Keep bytes through output.

* [x] **Replace `fmt.Fprintf` in signing increment construction.** `buildIncrement` uses `fmt.Fprintf` for object and trailer generation. Use append-based object writers.

* [x] **Cache normalized signing options.** `normalizeSubFilter`, `normalizeDigest`, and date formatting are small but repeated in signing paths. Normalize once in `Options.validate` or a prepared signing context.

* [x] **Avoid repeated placeholder search.** `findContentsRange` and `bytes.Index` search the output for placeholders after building the increment. Track offsets while building the signature dictionary instead.

### Attachments, metadata, tags, compliance

* [x] **Audit tagged-PDF wrappers for per-draw allocations.** `CellFormat`, `imageOut`, raw writes, SVG, and table cells wrap content for tagging. Profile `wrapTaggedContent` and structure tree bookkeeping in PDF/UA benchmarks.

* [x] **Cache repeated structure role strings.** Table/list/paragraph rendering repeatedly uses `"TR"`, `"TD"`, `"TH"`, `"LBody"`, etc. If tagging allocates internally, intern or use constants.

* [x] **Reduce XMP/metadata string building with pre-sized buffers.** Compliance-heavy benchmarks likely spend time building metadata and XMP. Pre-size builders from metadata lengths.

* [x] **Avoid repeated page object lookup for links.** `putLinkAnnotation` resolves page object numbers and page heights per link. Cache page heights and object numbers in slices.

* [x] **Avoid sorting resources unless catalog sort is enabled.** Fonts/images/resources should not build sortable key lists when deterministic catalog sort is off. Images already build a key list unconditionally.

## Top 10 PR order

* [x] Structured document table prefix widths.
* [x] Move `strings.Fields` inside `CellFormat` UTF-8 justification branch.
* [x] Cache `GetStringWidth` locally in `CellFormat`.
* [x] Append-based non-UTF8 PDF text escaping.
* [x] Remove `[]rune` allocation from UTF-8 `Document.write`.
* [x] Replace `generateImageID` gob hashing.
* [x] Avoid `signedContent` copy in signing.
* [x] Pass `glyfData` through font composite glyph recursion.
* [x] Pre-size UTF-8 font subsetting buffers.
* [x] Replace hot `fmtBuffer.printf` output paths with append-based helpers.
