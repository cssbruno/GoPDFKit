# v0.9 Migration Notes

`v0.9.0` is the last planned release line before the public API is prepared for
`v1.0.0`. The goal is to keep compatibility where practical while moving
production callers toward explicit policy, context-aware output, deterministic
generation, and typed errors.

## Deprecation Map

| Old API | New API | Notes |
| --- | --- | --- |
| `SetProtection` | `SetLegacyProtection` | Make the MD5/RC4 protection path explicit and steer new code toward modern signing/compliance workflows. |
| `SetProtectionError` | `SetLegacyProtection` | Preserve error-returning setup while naming the legacy behavior. |
| `RawWriteBuf` | `RawWriteBufError` | Prefer APIs that report reader failures directly. |
| `RawWriteArtifactBuf` | `RawWriteArtifactBufError` | Prefer APIs that report reader failures directly. |
| `OutputFileAndCloseWithOptions` | `OutputFileWithOptions` | Move from file-sync-only options to output-wide behavior. |
| `OutputFileAndClose` | `OutputFile` | Keep old name during `v0.x`; document the shorter output API for new code. |
| `SetCompressionLevel` | `SetCompressionPolicy` | Use one policy for enabled state, zlib level, workers, and tiny-stream thresholds. |
| `SetAttachmentCompressionWorkers` | `SetCompressionPolicy` | Keep as convenience wrapper, but production code should use one compression policy. |
| `WithPageCompressionWorkers` | `WithCompressionPolicy` or `WithProductionPolicy` | Keep the convenience option, but document the policy as the main production path. |
| `WithAttachmentCompressionWorkers` | `WithCompressionPolicy` or `WithProductionPolicy` | Keep the convenience option, but document the policy as the main production path. |
| `WithResourceCachePolicy` | `WithProductionPolicy` | Keep cache-specific options for simple use; use production policy for server profiles. |
| `AttachmentFromFile` | `AttachmentFromFileWithOptions` or loader-backed attachments | Prefer bounded file-backed attachments and explicit loader/security policy. |
| `PageRef.Content` | `PageRef.ContentWithError` | Preserve compatibility while making content decode errors visible. |
| `PageRef.ForEachObject` with borrowed bytes | `PageRef.ForEachObjectCopy` | Prefer safe copies for public iteration; use borrowed iteration only for advanced internal paths. |
| `HTML.Write` | `HTML.WriteContext` | Keep non-context API as a convenience wrapper. |
| `Document.Output` | `Document.OutputContext` | Keep non-context API as a convenience wrapper. |
| `Document.OutputSigned` | `Document.OutputSignedContext` | Keep non-context API as a convenience wrapper. |

## New Production Entry Points

Prefer policy helpers for new server code:

```go
pdf, err := document.NewDocument(
    document.WithProductionPolicy(document.ServerSafePolicy()),
)
```

Use output options when a choice belongs to the output operation rather than the
document builder:

```go
err := pdf.OutputFileWithOptions("report.pdf", document.OutputOptions{
    Compression:   document.CompressionPolicy{Enabled: true},
    Limits:        document.ServerSafeLimits(),
    Deterministic: true,
})
```

Use context-aware output for request-scoped or job-scoped generation:

```go
ctx, cancel := context.WithTimeout(parent, 10*time.Second)
defer cancel()

err := pdf.OutputContext(ctx, writer)
```

## Compatibility Policy

The old APIs should remain available through `v0.9.x` unless they are unsafe to
keep. Deprecated methods should call the new implementation where possible and
should appear in Go doc comments with direct migration guidance.

For `v1.0.0`, decide which deprecated names remain as compatibility wrappers and
which are removed. Any removal must be listed in the `v1.0.0` changelog.

## Template Serialization

Persisted templates now expose the serialized-format version:

```go
func TemplateSerializationVersion() string
```

In `v0.9.x`, this returns `GPKTPL1`, the serialized-template format marker. The
template fingerprint uses its own internal marker. Template serialization is
stable within the `v0.9.x` minor line unless a release note says otherwise.
Cross-major compatibility must be documented before `v1.0.0`.

## Error Handling

New code should prefer `errors.Is` over string matching. `v0.9.0` should add
sentinel errors for high-value caller branches, including invalid page size,
attachment limits, unsupported images, unsupported imported PDFs, and HTML
limits.

## Related Docs

* [`migration-v0.8-to-v0.9.md`](migration-v0.8-to-v0.9.md)
* [`production.md`](production.md)
* [`security.md`](security.md)
* [`deterministic-output.md`](deterministic-output.md)
