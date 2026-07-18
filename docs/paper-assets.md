# Human-readable `.paper` assets

Binary image data does not need to live inside a `.paper` source file. Authors
can use a stable readable reference:

```paper
image @hero:
  source: "asset:hero-image"
  width: 240pt
  height: 135pt
  fit: "cover"
  focus-x: 0.5
  focus-y: 0.35
  alt: "Quarterly revenue by region"
```

The host supplies the bytes through an explicit content-addressed catalog:

```go
sum := sha256.Sum256(pngBytes)
assets, err := document.NewPaperAssetCatalog([]document.PaperAssetResource{{
    Name:      "hero-image",
    MediaType: "image/png",
    Digest:    hex.EncodeToString(sum[:]),
    Data:      pngBytes,
}})
if err != nil {
    return err
}

plan, result, err := document.PlanPaperWithAssets("report.paper", source, assets)
```

`PlanPaperScenarioWithAssets`, `WritePaperWithAssets`, and
`WritePaperScenarioWithAssets` use the same catalog.

The boundary is intentionally strict:

- names are portable lowercase identifiers;
- only decodable, bounded PNG/JPEG and signed, bounded TTF/OTF resources are
  admitted to the immutable planner catalog;
- encoded bytes and decoded pixel counts have independent hard limits;
- every digest is mandatory and must match the supplied bytes;
- catalogs are detached from caller-owned memory and resolve deterministically;
- missing references fail compilation;
- the compiler never searches paths, follows URLs, or consults environment or
  process-global resource state;
- the verified content digest participates in the immutable plan resource
  catalog and therefore in deterministic plan identity.

Inline `data:image/...;base64,...` remains accepted for small self-contained
fixtures and transport documents. `asset:name` is the normal readable project
authoring form.

## CLI and Paper Studio manifest

Every planning CLI command (`check`, `render`, `capture`, and `explain`) and
Paper Studio accepts assets only when the manifest is explicitly named. The
same flags work for ordinary, `--scenario`, `--data`, and generated edge-case
planning:

```json
{"assets":[
  {"name":"hero-image","media_type":"image/png","sha256":"<64 lowercase hex characters>","path":"images/hero.png","focus_x":0.5,"focus_y":0.4},
  {"name":"hero-image-v2","media_type":"image/png","sha256":"<64 lowercase hex characters>","path":"images/hero-v2.png","replaces":"hero-image"},
  {"name":"body-regular","media_type":"font/ttf","sha256":"<64 lowercase hex characters>","path":"fonts/body.ttf","family":"Readable Sans","weight":400,"style":"normal","license":"OFL-1.1","fallback":["body-fallback"]},
  {"name":"body-fallback","media_type":"font/ttf","sha256":"<64 lowercase hex characters>","path":"fonts/fallback.ttf","family":"Fallback Sans","license":"OFL-1.1"}
]}
```

```shell
go run ./cmd/paper-studio -assets project.assets.json -asset-root . report.paper

go run ./cmd/paper check -assets project.assets.json -asset-root . report.paper
go run ./cmd/paper render -assets project.assets.json -asset-root . -o report.pdf report.paper
go run ./cmd/paper capture -assets project.assets.json -asset-root . --scenario preview report.paper
go run ./cmd/paper render -assets project.assets.json -asset-root . --data report.json -o report.pdf report.paper
go run ./cmd/paper check -assets project.assets.json -asset-root . --edge-cases 12 --seed 42 report.paper
```

When `-asset-root` is omitted, the explicit manifest's directory is the root.
Paths are project-root-relative canonical slash paths. Absolute paths,
traversal, symlink components, non-regular files, digest/signature mismatches,
unknown manifest fields, and over-budget catalogs are rejected before a plan
is created. The CLI and Studio share this loader and never search beside a
source or stdin for an implicit manifest. Browser inventory responses contain
metadata and source usage only—never raw bytes or filesystem paths.

Font entries support metadata for `font/ttf`, `font/otf`, and `font/woff2`, with validated
file signatures, family, weight, style, and an SPDX-style license selected from
the enforced `OFL-1.1`, `Apache-2.0`, `Bitstream-Vera`, `MIT`, `CC0-1.0`, or
`Proprietary` policy.
An acyclic fallback list is validated with the same policy. `replaces` forms an
acyclic same-kind lifecycle edge. Image
entries may provide default crop focus; an authored image focus remains the
explicit usage-level override. Studio can apply a declared image replacement
to one exact source node through its source/plan-bound semantic journal. Font
metadata is inspectable but font bytes are not sent to the browser. TTF/OTF
entries are installed only in the detached planner, their metrics and bytes
are content-addressed, and the PDF painter uses the existing UTF-8 subsetter
before embedding the used glyphs. WOFF2 remains metadata-only until a bounded
WOFF2-to-TrueType adapter is admitted.

When Studio is started with `-assets`, its Resources panel also exposes a
small catalog-management workflow. Add requests name a project-relative file
and its metadata; the server derives the SHA-256 from the verified file,
revalidates the complete catalog, and atomically replaces the explicit
manifest. Remove requests require the exact source and plan revisions and
reject resources still referenced by the authored source. Existing lifecycle
relationships, file safety checks, byte budgets, and manifest-size limits are
validated before publication. The browser receives the refreshed metadata
inventory only, never the manifest path, asset root, or raw bytes.
