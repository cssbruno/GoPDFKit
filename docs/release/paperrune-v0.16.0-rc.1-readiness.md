# PaperRune v0.16.0-rc.1 Readiness

Date: 2026-07-20

This record separates repeatable repository evidence from release decisions
that require elapsed time or named people. Historical security and performance
records keep their original commit identifiers because they describe those
older snapshots; they are not relabeled as PaperRune candidate evidence.

## Candidate identity

- Version file and changelog: `v0.16.0-rc.1`.
- Module identity: `github.com/cssbruno/paperrune`.
- Immutable commit identity: resolve `v0.16.0-rc.1^{commit}` after the candidate
  tag is created. The release workflow verifies and checks out that exact tag.
- Repository identity: the new independent GitHub repository and `origin`
  remote must exist before publication.

## Local automated evidence

The following passed from the PaperRune worktree on 2026-07-20:

- `make release-version VERSION=v0.16.0-rc.1`
- `make fmt-check`
- `go vet ./...`
- `go test ./...`
- `git diff --check`
- root and `tools/` module tidiness checks

The Brazilian laboratory paper was also exercised with ten deterministic edge
profiles, including empty, whitespace-only, multiline, long, unbroken,
Portuguese Unicode, punctuation, numeric-boundary, and 64-row list data. All 12
produced PDF pages passed structural parsing, exact page-count comparison, text
extraction, and the zero-layout-issue threshold. Poppler rasterized every final
PDF page, and the resulting PNGs and 13-page review PDF were visually inspected.

Local visual evidence is written to the ignored directory
`output/pdf/paperrune-edge-review-v016rc1/` and is reproducible with:

```sh
go run ./cmd/paper check --json \
  --assets examples/paper-lab-report/assets.json \
  --edge-cases 10 --edge-max-items 64 --seed 42 \
  --edge-output output/pdf/paperrune-edge-review-v016rc1 \
  --edge-visual examples/paper-lab-report/lab-report.paper
```

## CI gates added for the independent repository

- Full-history secret scanning with SHA-pinned Gitleaks Action v2.3.9.
- Poppler installation before tests so final-PDF visual tests run instead of
  being skipped.
- Existing formatting, module, coverage, static analysis, NilAway, gosec,
  vulnerability, race, Windows cross-build, compliance, and release gates.

These CI gates are configured but cannot have PaperRune run identifiers until
the independent GitHub repository exists and the branch is pushed.

## External gates that remain open

The following cannot be completed by a local code change and must not be
self-certified:

- [ ] Create and authenticate the independent PaperRune GitHub repository,
  configure `origin`, push `main`, and record the first green CI run.
- [ ] Complete the documented stabilization interval with real cohort evidence.
- [ ] Obtain named external security acceptance for the immutable candidate.
- [ ] Record rollback ownership and the final release decision.

The release tag and final release must not be published while any required
external gate above remains open.
