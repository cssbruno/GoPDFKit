# Local candidate security and hostile-input evaluation

Date: 2026-07-18  
Platform: darwin/arm64, Apple M2  
Go toolchain: go1.26.5

This record covers the local candidate containing this file. It supports local
engineering verification but does not replace the named external security
review, the pushed GitHub Actions fuzz campaign, or the signed/anchored release
acceptance record required by the Paper Engine checklist.

## Static and dependency analysis

- `GOCACHE=/tmp/gopdfkit-go-cache make gosec`: pass; 377 files, 127,744
  lines, 0 issues.
- `GOCACHE=/tmp/gopdfkit-go-cache make govulncheck`: pass; no known
  vulnerabilities reported by the current Go vulnerability database.
- Findings discovered during the first scan were resolved before this record:
  anonymous structural-key ordinals are explicitly bounded before conversion,
  caller-selected Studio paths identify both relevant gosec rules, and the
  component-preview response identifies its bounded XML-escaping serializer.

## Hostile-input fuzz targets

Each workflow target ran for `-fuzztime=10s` and passed:

- `document/FuzzDeserializeTemplate`
- `document/FuzzSVGParse`
- `importpdf/FuzzOpenBytes`
- `pdfcdr/FuzzSanitizePDF`
- `inspect/FuzzDecodedStreamsAndText`
- `sign/FuzzVerifyCMS`
- `sign/FuzzPDFSignatureScanner`
- `sign/FuzzAnalyzePDF`
- `internal/paperlang/FuzzIncrementalCSTMatchesCleanParse`
- `internal/paperpkg/FuzzMigrateLockfile`

No crash corpus was produced.

## Protocol and browser boundaries

- The authenticated Paper daemon protocol evaluation is recorded in
  [`paperd-protocol-evaluation.md`](paperd-protocol-evaluation.md).
- Paper Studio JavaScript tests pass all 38 tests, including transferred Worker
  payloads and propagated Worker failures.
- The real browser/WASM smoke test passes and renders page 1 at 1191 by 1684
  pixels from a 408,358-byte deterministic PNG.

## Remaining external acceptance

The following evidence cannot be self-certified by this local run:

1. a pushed GitHub Actions fuzz execution bound to the immutable candidate;
2. named security-reviewer acceptance;
3. an elapsed and accepted stabilization-window record; and
4. a signed or externally anchored audit root.

