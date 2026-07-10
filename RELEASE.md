# Release Process

This repository releases Go modules by pushing semver tags. The current release
line starts at `v0.1.0`.

## Versioning

- Use Go module semver tags: `vMAJOR.MINOR.PATCH`.
- Keep `VERSION` set to the next tag.
- Keep `CHANGELOG.md` updated with a matching `## vMAJOR.MINOR.PATCH` section.
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
- nested-module dependency and build checks,
- race detection and package coverage floors,
- `make lint` and `make gosec`,
- `make govulncheck`,
- `make build`.

`make quality` runs the release-independent code, coverage, lint, security,
and vulnerability gates. The analysis tools are pinned in the separate
`tools/` module so the library module keeps only runtime dependencies. The
Makefile runs them with the Go toolchain declared by `tools/go.mod`.

## GitHub Release

Pushing a tag such as `v0.1.0` runs `.github/workflows/release.yml`. The workflow
runs `make release-check`, extracts the matching changelog section, and creates
or updates the GitHub Release.
