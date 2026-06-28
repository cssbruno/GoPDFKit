# Migration: v0.8 to v0.9

`v0.9.0` is the production-stability release before `v1.0.0`. The main
migration theme is moving from scattered feature-specific knobs to explicit
policy, bounded IO, error-aware low-level APIs, and clearer compatibility names.

For the full deprecation table, see [`migration-v0.9.md`](migration-v0.9.md).

## Preferred v0.9 APIs

| v0.8 API | Preferred v0.9 API | Why |
| --- | --- | --- |
| `SetProtection` | `SetLegacyProtection` | Names the MD5/RC4 compatibility behavior explicitly. |
| `RawWriteBuf` | `RawWriteBufError` | Reader failures become visible to callers. |
| `RawWriteArtifactBuf` | `RawWriteArtifactBufError` | Reader failures become visible to callers. |
| `OutputFileAndCloseWithOptions` | `OutputFileWithOptions` | Output options cover more than sync behavior. |
| `SetCompressionLevel` | `SetCompressionPolicy` | Compression level, enabled state, and workers are one policy. |
| `AttachmentFromFile` | `AttachmentFromFileWithOptions` | File-backed attachments can be eagerly validated and bounded. |
| `Document.Output` | `Document.OutputContext` | Request-scoped and job-scoped generation can be canceled. |
| `HTML.Write` | `HTML.WriteContext` | HTML generation can be canceled. |

## Server Migration Checklist

* Choose `ServerSafePolicy` or define a `ProductionPolicy`.
* Set explicit `Limits`, especially attachment, image, HTML, imported PDF, page,
  and referenced-object limits.
* Use explicit shared `ImageCache` and `FontCache` only when cross-document
  reuse is intended.
* Prefer `OutputWithOptions` or `OutputFileWithOptions` for output-time
  compression, limit, sync, and deterministic decisions.
* Replace raw writes with error-returning raw-write APIs.
* Replace legacy protection calls with `SetLegacyProtection` and document that
  it is advisory compatibility, not modern encryption.
* Use `AttachmentFromFileWithOptions` when attachments may come from mutable or
  user-controlled paths.
* Use deterministic output for golden PDFs, compliance fixtures, and CI.

## Compatibility Expectations

Deprecated v0.8 APIs should remain through `v0.9.x` where practical, usually as
wrappers over the new implementation. Any API removed for `v1.0.0` must be
listed in the `v1.0.0` changelog.
