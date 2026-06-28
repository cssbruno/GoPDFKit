# Release Process

This repository releases Go modules by pushing semver tags. The current release
line starts at `v0.1.0`.

## Versioning

- Use Go module semver tags: `vMAJOR.MINOR.PATCH`.
- Keep `VERSION` set to the next tag.
- Keep `CHANGELOG.md` updated with a matching `## vMAJOR.MINOR.PATCH` section.
- Keep forward-looking release plans under `doc/` when a minor release has
  pre-implementation API design work. For the pre-`v1.0.0` production policy
  work, see `doc/release-plan-v0.9.0.md`, `doc/migration-v0.9.md`,
  `doc/production.md`, `doc/security.md`, and
  `doc/deterministic-output.md`. Track readiness and benchmark gates in
  `doc/v0.9-readiness-checklist.md` and `doc/benchmark-budgets-v0.9.md`.
- Until `v1.0.0`, the API is not considered stable and releases may include
  breaking changes.
- For the `v0.x` line, bump `MINOR` for new public functions, new public
  behavior, or breaking API changes. For example, release `v0.2.0` after
  `v0.1.0`.
- For the `v0.x` line, bump `PATCH` for bug fixes only. For example, release
  `v0.1.1` after `v0.1.0`.
- Do not use patch releases for breaking API changes or new public functions,
  even before `v1.0.0`.

## Local Release Commands

Run the release gate:

```sh
make release-check
```

Preview release notes:

```sh
make release-notes
```

Create an annotated tag after the working tree is clean:

```sh
make release-tag
```

Push the tag:

```sh
make release-push
```

`make release` is an alias for `make release-tag`.

## Gates

`make release-check` runs:

- version and changelog validation,
- `make check`,
- `make govulncheck`,
- `make build`.

`make quality` runs stricter analysis with `golangci-lint`, `nilaway`, and
`gosec`. These tools are pinned in the separate `tools/` module so the library
module keeps only runtime dependencies. The Makefile runs them with the Go
toolchain declared by `tools/go.mod`. It is expected to fail until the current
quality baseline is fixed.

## GitHub Release

Pushing a tag such as `v0.1.0` runs `.github/workflows/release.yml`. The workflow
runs `make release-check`, extracts the matching changelog section, and creates
or updates the GitHub Release.
