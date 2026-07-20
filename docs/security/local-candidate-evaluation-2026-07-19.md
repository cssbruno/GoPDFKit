# Local candidate security and hostile-input evaluation

Date: 2026-07-19  
Platform: darwin/arm64, Apple M2  
Go toolchain: go1.26.5  
Base commit: `2c199f99cbf3505aac0eff417e8b214f9bc0aba7` plus the current
uncommitted worktree

This record covers the local candidate containing this file. It supports local
engineering verification but does not replace the named external security
review, a pushed fuzz campaign bound to an immutable candidate, or the
signed/anchored release acceptance record required by the Paper Engine
checklist. The repository-wide analysis is in
[`security_full_analysis_report.md`](../../security_full_analysis_report.md).

## Static, dependency, and source analysis

- `make gosec`: pass; 382 files, 130,807 lines, 0 issues.
- `make govulncheck`: pass; no known vulnerabilities reported by the current Go
  vulnerability database.
- `make lint`: pass; 0 issues.
- `make nilaway`: pass.
- Best-effort credential and private-key search: no credential assignment or
  tracked private-key/certificate file found. Dedicated `gitleaks` and
  `trufflehog` binaries were unavailable.
- The initial `gosec` run found one G115 conversion warning. The complete
  `uint32` input domain is bounded to a maximum result of `43,980`; that bound is
  now documented and covered by a maximum-domain regression test.

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

## Protocol, browser, and concurrency boundaries

- `go test -race ./...`: pass across the complete repository (`paperd`, the
  longest package, completed in 197.554s).
- The authenticated Paper daemon boundary is described in
  [`paperd-protocol-evaluation.md`](paperd-protocol-evaluation.md).
- Paper Studio JavaScript tests pass all 41 tests.
- The browser/WASM smoke passes and renders page 1 at 1191 by 1684 pixels from a
  408,358-byte PNG. The rebuilt compressed WASM SHA-256 is
  `01f7ebb7fc8721b16c8600342547649b0eadef1d51960f0ebceaf4c69af62cfa`.

## Compatibility and performance evidence

- The complete `release-check` component set passes: release metadata, full Go
  tests and race tests, vet, formatting, module verification, coverage
  thresholds, lint, NilAway, `gosec`, current `govulncheck`, and build. The
  vulnerability component was rerun with network access after the aggregate
  target encountered the sandbox's DNS restriction.
- The non-containerized PDF-reader smoke passes.
- The calibrated Paper engine budget passes all 11 cohorts. The report SHA-256
  is `439076219b9f2b40d82178af97f4d7327f7b87940d2a3be17caf49e5551c9dbe`.
- Typed characterization contains 26 fixtures: 22 planned and one each
  accepted-malformed, canceled, rejected, and resource-limit. Artifact SHA-256:
  `767db791ead70ad5fa0d0600a1aab855c7350ff5a12bf5104ded5ee7823dd937`.
- HTML characterization contains 11 fixtures: 7 rendered and one each planned,
  recovered, rejected-by-policy, and unsupported. Artifact SHA-256:
  `2c9bf0b28e755da92eeb526255f8729f29eca31fb6e71516066a4fda42ca7be8`.

The first Docker-based veraPDF baseline attempt was not authorized because its
configured target mounted the full workspace and host `/tmp` into a third-party
container. The wrappers now copy only requested PDF inputs into a private
temporary directory, mount that directory read-only, disable networking, drop
Linux capabilities, prevent privilege escalation, make the container root
filesystem read-only, provide an isolated 64 MiB tmpfs, and default to the
release-pinned veraPDF image digest. The hardened target passes: retained
compliance baselines match `testdata/compliance`.

## Remaining external acceptance

The following evidence cannot be self-certified by this local run:

1. an immutable candidate commit and artifact set;
2. a GitHub Actions fuzz execution bound to that exact candidate (the latest
   visible Fuzz workflow run predates it);
3. named security-reviewer acceptance;
4. an elapsed and accepted stabilization-window record; and
5. a signed or externally anchored audit root.
