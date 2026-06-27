More `v0.6.0` pprof targets, focused on areas I did **not** cover deeply before: attachments, SVG, signing parser, templates, font loading, output/catalog, bookmarks, QR, and parser utilities. These are not “confirmed regressions”; they are concrete hot-path candidates to validate with CPU/memory profiles.

### New pprof checklist for `v0.6.0`

* [x] **Attachment content is copied in `SetAttachments`.** Large attachments are cloned with `append([]byte(nil), a.Content...)`; for repeated generation or huge files this will show as `runtime.memmove`. Add an unsafe/trusted immutable variant or delayed-copy mode.

* [x] **Attachment checksum hashes full content before compression.** `writeCompressedFileObject` computes MD5 over the whole attachment, then compresses the same bytes. For large attachments, profile `checksum`, `md5.Sum`, and `compressBytes`. Consider streaming checksum + compression in one pass where possible.

* [x] **Attachment compression is serial and per-object.** If many large attachments are embedded, compress them before final output using a bounded worker pool, similar to page compression. The current path embeds each attachment during `enddoc`.

* [x] **Attachment annotations dedupe by pointer, not by content.** `putAnnotationsAttachments` uses `map[*Attachment]bool`; two attachment objects with the same filename/content are embedded twice. Add optional content-hash dedupe.

* [x] **Global attachments and annotation attachments do not share a content cache.** `putAttachments` embeds global attachments first, then `putAnnotationsAttachments` embeds annotation attachments separately. A unified attachment object cache could avoid duplicate embedded streams.

* [x] **Attachment MIME/relationship normalization repeats string trimming.** `attachmentMIMEType` and `attachmentAFRelationship` repeatedly call `strings.TrimSpace` and MIME lookup. Normalize when setting/cloning attachments.

* [x] **Attachment name tree construction uses `fmt.Sprintf` per item plus `strings.Join`.** `getEmbeddedFiles` builds `names []string`, formats each entry, then joins into another string. Use a single `strings.Builder` or append-based output.

* [x] **`escapePDFName` scans reserved characters with `strings.ContainsRune` per byte.** Replace with a small `[256]bool` table for PDF-name escaping. It matters for many attachments, MIME subtypes, or resource names.

* [x] **Catalog always emits `/Names << /EmbeddedFiles ... >>`.** `putcatalog` calls `getEmbeddedFiles()` unconditionally inside `/Names`, even when there are no attachments. This is small, but avoidable output work and allocation.

* [x] **`fileIdentifier` hashes the full final buffer.** For PDF/A/Arlington, it computes SHA-256 over `f.buffer.Bytes()` near trailer time. On huge PDFs this becomes a full-buffer scan. Profile `fileIdentifier` in compliance-heavy output; consider incremental hashing while writing.

* [x] **Bookmark output uses many `outf` calls per outline.** `putbookmarks` emits each bookmark object through multiple formatted calls. Large outline trees should profile `putbookmarks`, `fmt`, and `textstring`. Use append-based object building.

* [x] **Bookmark destination page height/object lookups repeat per outline.** `putbookmarks` calls `pageObjectNumber` and `pageHeightPt` for each bookmark. Cache page object numbers/heights in local slices during bookmark output.

* [x] **Bookmark text may be UTF-16 encoded early and textstring-escaped later.** `Bookmark` converts text with `utf8toutf16` when current font is UTF-8, and output later calls `textstring`. Profile outline-heavy docs for `utf8toutf16`/escape.

* [x] **`estimateFinalBufferSize` scans pages, images, templates, imported objects, imported pages, attachments, XMP, and JavaScript every close.** This is useful, but for huge docs it can be visible. Profile whether the estimation scan costs more than saved buffer growth.

* [x] **`putresourcedict` still uses formatted output for font/extgstate/shading refs.** In resource-heavy PDFs, profile `putresourcedict`, `outf`, and `fmt`. Replace `/F%s %d 0 R`, `/GS%d`, `/Sh%d` with append helpers.

* [x] **JavaScript output encodes the script through `textstring` in one formatted call.** Large embedded JS will allocate/copy. Add append-based string escaping or stream the JavaScript object body.

* [x] **SVG path parsing uses `strings.Replacer`, `strings.ReplaceAll`, and `strings.Fields`.** `pathFields` and `svgNumberFields` allocate heavily for large `d` attributes. Replace with a scanner that tokenizes commands/numbers in one pass.

* [x] **SVG path parser copies every completed arg slice.** `pathParse` copies `args` into `rawArgs` for every segment, then `normalizePathSegments` converts again. Parse directly into normalized segments to remove the intermediate `svgPathRawSegment` layer.

* [x] **SVG XML unmarshal builds a full recursive tree.** `SVGParse` uses `xml.Unmarshal` into `svgNode`, then walks the tree. For large SVGs, a streaming decoder or hybrid parser could reduce peak memory.

* [x] **SVG style resolution allocates an attrs map per node.** `svgNodeStyle` creates `attrs := map[string]string{}` for every node, lowercases every attribute name, then creates another `declarations` map. Use a compact attr view or reusable scratch map.

* [x] **SVG CSS matching still scans all rules per node.** `svgNodeStyle` loops all style rules and selectors directly. It does not appear to use the newer HTML selector index/meta path. Reuse the compiled selector index for SVG style blocks.

* [x] **SVG `svgHTMLSegment` allocates attrs maps for every ancestor append.** Recursive collection passes `append(ancestors, svgHTMLSegment(node))`; each segment conversion builds a new attr map. Cache per-node HTML segment metadata during SVG collection.

* [x] **SVG recursion allocates ancestor slices repeatedly.** `svgCollectDepth` and `svgCollectClipSegments` use `append(ancestors, el)` in recursive calls. Deep SVGs will show slice growth/copying. Keep a mutable stack instead.

* [x] **SVG reference rendering may re-collect referenced subtrees repeatedly.** `<use>` resolves the referenced node and recursively calls `svgCollectDepth`; repeated use of the same symbol may redo style/path/text/image extraction. Cache resolved symbol elements by ref ID + transform/style key.

* [x] **SVG clip path resolution is not cached.** `svgResolveClipPath` collects clip segments every time a style references a clip path. Add a cache keyed by clip ID plus transform/rule context.

* [x] **SVG pattern rendering can emit thousands of repeated tile elements.** `svgWritePatternFill` loops tile rows/cols and renders each pattern element for every tile, up to `svgMaxPatternTiles`. Consider emitting a real PDF tiling pattern XObject or caching one tile as a form XObject.

* [x] **SVG pattern fill recomputes opacity inside the inner element loop.** Inside each tile, it calls `svgStyleOpacity(path.Style, false, true)` repeatedly. Compute once outside the nested loops.

* [x] **SVG path open/fill/stroke analysis rescans segments.** `svgApplyPathStyle` calls `svgPathHasStroke` and `svgPathHasFill`; those can call `svgPathOpen`, which scans segments from the end. Cache path openness per `SVGPath` or compute once locally.

* [x] **SVG bounds calculation approximates curves by control points and rescans segments for gradient/pattern fills.** `svgWritePatternFill` and `svgWriteGradientFill` call `svgPathBounds` per filled path. Store bounds on `SVGPath` during parse/normalization.

* [x] **SVG image rendering hashes image data every render.** `svgWriteImage` computes SHA-256 and formats a name before `RegisterImageOptionsReader`. Store the deterministic image name during SVG parse/compile.

* [x] **SVG image rendering re-registers images on every render.** Even with a deterministic name, `RegisterImageOptionsReader` still gets called. Check if document image map already contains the name before hashing/reader registration.

* [x] **SVG dash scaling allocates a new slice per styled path.** `svgScaledValues` allocates for every dashed path. Cache scaled dash arrays by original dash slice identity/values plus scale, or write a no-allocation `SetDashPatternScaled`.

* [x] **SVG state restore copies dash arrays for every `SVGWrite`.** `SVGWrite` snapshots `dashArray := append([]float64(nil), f.dashArray...)` even for SVGs with no stroke dash changes. Make dash snapshot lazy only when modified.

* [x] **SVG text anchor measures text width during render.** `svgWriteText` calls `GetStringWidth` for middle/end anchors. Cache text widths in `SVGText` if the same SVG is rendered repeatedly at the same font/scale.

* [x] **SVG arc conversion uses heavy trig per arc at parse time.** `svgArcSegments` calls `math.Sin`, `math.Cos`, `math.Acos`, `math.Tan`, `math.Hypot`, and `math.Sqrt`. For icon sets with many arcs, profile this and consider caching parsed SVGs more aggressively or preserving arc primitives until render.

* [x] **SVG gradient stops are sorted every gradient resolution.** `svgApplyGradientNode` sorts stops when applying a gradient node. If the same gradient is referenced many times, ensure the resolved-gradient cache covers all use paths.

* [x] **SVG style helpers repeat `TrimSpace`/`ToLower` chains.** `parseSVGVisibility`, `parseSVGFillRule`, `parseSVGLineCap`, `parseSVGLineJoin`, `parseSVGOpacity`, and `parseSVGDashArray` normalize strings repeatedly. Normalize declarations once when building the declaration map.

* [x] **QR code registration regenerates PNG for duplicate payloads.** `RegisterQRCodePNG` trims payload, calls `QRCodePNG`, which encodes, scales, draws into NRGBA, PNG-encodes, then registers. If the same QR payload is used repeatedly, check `f.images` by `QRCodeImageName(payload)` before generating.

* [x] **QR PNG generation uses an intermediate NRGBA draw.** `QRCodePNG` scales the barcode, creates `image.NewNRGBA`, draws into it, then PNG-encodes. If the scaled barcode already satisfies PNG encoding needs, avoid the extra draw/copy.

* [x] **Font loading copies UTF-8 font bytes.** `utf8FontDefinition` builds `fileReader{array: append([]byte(nil), utf8Bytes...)}` after `readFontResourceFile` already returned bytes. Avoid the second copy when ownership is clear.

* [x] **UTF-8 font add path does two filesystem stats for `.ttf`/`.otf`.** `AddUTF8Font` does `os.Stat` on the requested file and, for `.ttf`, may stat `.otf`. Cache font path resolution for repeated docs in server workloads.

* [x] **`makeSubsetRange` builds a map for dense ranges.** If used in font subsetting paths, a dense slice/bitset is likely faster and lower allocation than `map[int]int`.

* [x] **Font definition validation uses `strings.Fields` for encoding diffs.** `validFontDiff` splits the full diff string into fields, then validates each. For many custom fonts, replace with a scanner.

* [x] **Template ID hashes template bytes every call.** `DocumentTpl.ID()` computes SHA-1 over `t.Bytes()` each time. Many template operations call `child.ID()` repeatedly; store a cached ID on `DocumentTpl`.

* [x] **Template child image collection repeatedly calls child IDs.** `childrenImages` builds names with `child.ID()` inside loops over child images. Cache child ID once per child.

* [x] **Template top-level filtering repeatedly recomputes child template IDs.** `topLevelTemplates` builds `nestedIDs` from `childrenTemplates()` and then calls `child.ID()` again while filtering. Cache IDs in a local map or on the template.

* [x] **Template `ownImages` builds a child-image map just to filter keys.** For large nested template graphs, this allocates a full map. Consider storing image provenance or returning a set of inherited keys instead of full image map.

* [x] **Template serialization still uses gob.** `Serialize`, `GobEncode`, and `GobDecode` use gob and buffers. For performance-sensitive template caches, a compact custom binary format would reduce CPU and allocation.

* [x] **Template `createTemplate` copies page byte slices only by slice header.** It stores `tpl.pages[x].Bytes()` directly. This is fast, but if later immutability requires copying, add copy-on-write rather than unconditional deep copies. Profile template-heavy code before changing.

* [x] **Signing xref parsing splits the full xref tail into all lines.** `parseXrefTable` uses `bytes.Split(input[offset:], '\n')`, which allocates slice entries for the whole remainder of the file. Parse line-by-line until `trailer`.

* [x] **Signing xref parsing converts lines to strings.** It does `strings.TrimSpace(string(lines[i]))`, `strings.Fields(line)`, and `string(lines[i])`. Use byte parsing and `strconv.Atoi` replacement over byte spans.

* [x] **Signing `parseLeadingInt` converts byte slices to strings.** This is used in object parsing and references. Replace with a byte-based integer parser.

* [x] **Signing `findReference` compiles a regexp on every call.** It uses `regexp.MustCompile(fmt.Sprintf(...))` inside the function. Replace with a manual `/Key int int R` scanner or precompiled patterns for known keys.

* [x] **Signing trailer parsing uses regexp for simple numeric fields.** `/Root`, `/Size`, and `/Prev` are regex-driven. Manual dictionary scanning already exists nearby and would reduce regexp overhead.

* [x] **Signing `readObjectDict` copies dictionaries.** It returns `append([]byte(nil), object[start:start+dictEnd]...)`. If callers only read or immediately build a modified copy, return a slice and copy only on mutation.

* [x] **Signing `addDictEntry` and `addAnnotation` rebuild dictionaries.** They allocate a new byte slice and copy around the insertion. Track object body construction with a builder when adding multiple entries.

* [x] **PDF parser helpers use `strings.ContainsRune` in byte-class checks.** `isPDFTokenEnd` and `isPDFNameChar` call `strings.ContainsRune` for byte delimiters. Replace with a byte lookup table.

* [x] **`findPDFName` and related PDF name scans convert name to `[]byte` every call.** For repeated keys, precompute byte needles or use a small scanner object.

* [x] **`findFirstPage` recursively reads and copies dictionaries.** It calls `readObjectDict` per page-tree node. For deep or large page trees, cache dictionaries or return slices without copying.

* [x] **Layout model stores `TextStyle` by value in every segment/cell/block.** This is clean, but large generated documents may copy many style structs. Profile layout builders for `runtime.memmove`; consider style interning or pointer/shared styles.

* [x] **Layout table cells carry full `BoxStyle`/`TextStyle` by value.** Large tables may copy style structs per cell. Add table-level/default style inheritance to reduce per-cell copies in builders.

* [x] **Layout image blocks carry inline image bytes by value in structs.** Passing/copying `ImageBlock` values copies the slice header only, but repeated block copies can still create overhead; prefer pointers for heavy block graphs.

* [x] **Layout signature field name trims every call.** `PAdESFieldName` trims `PlaceholderReference` every time. Normalize when setting/constructing the signature block. Small, but trivial.

My next strongest candidates are: byte-scanner SVG path parsing, SVG node-style metadata/cache, signing xref byte parser, attachment content dedupe/cache, cached template IDs, and QR duplicate-payload short-circuit.
