# Release Process

This repository releases Go modules by pushing semver tags. The current release
line starts at `v0.1.0`.

## Versioning

- Use Go module semver tags: `vMAJOR.MINOR.PATCH`.
- Keep `VERSION` set to the next tag.
- Keep `CHANGELOG.md` updated with a matching `## vMAJOR.MINOR.PATCH` section.

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
