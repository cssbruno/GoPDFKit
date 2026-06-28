# Production Usage

This document describes the `v0.9.0` production contract.

## Server-Safe Policy

Server applications should configure GoPDFKit from one policy object:

```go
pdf, err := document.NewDocument(
    document.WithProductionPolicy(document.ServerSafePolicy()),
)
```

A server-safe policy should:

* use bounded image, attachment, HTML, template, page, and imported PDF limits,
* avoid unbounded package-level cache growth,
* cap page and attachment compression workers,
* disable JavaScript, local HTML images, file-backed attachments, raw writes,
  and legacy protection unless explicitly allowed,
* allow deterministic output when PDFs are compared in tests or CI.

Initial `ServerSafePolicy` values:

| Setting | Value |
| --- | --- |
| Cache | `ResourceCacheDocument` |
| Compression level | `zlib.BestSpeed` |
| Page workers | `4` |
| Attachment workers | `2` |
| Attachment limit | `64 MiB` |
| Security gates | JavaScript, local HTML images, file-backed attachments, raw writes, and legacy RC4 protection disabled |

Precedence:

1. Per-call `OutputOptions`.
2. Later functional options.
3. Earlier `WithProductionPolicy` values.
4. Explicit `ImageCache` and `FontCache` over cache policy.
5. Per-document defaults.
6. Package defaults.

## Caches

The intended cache contract is:

* `ResourceCacheShared` uses package-level image and UTF-8 font caches.
* `ResourceCacheDocument` scopes file-backed caches to one document.
* `ResourceCacheDisabled` disables file-backed cache reuse.
* Explicit `ImageCache` or `FontCache` overrides the policy for that resource
  type.

`Document` remains a mutable builder and is not safe for concurrent mutation or
output. `ImageCache`, `FontCache`, imported `SourceCache`, and `CompiledHTML`
are safe for concurrent reuse.

## Output

Use output options for choices that happen during output:

```go
err := pdf.OutputFileWithOptions("report.pdf", document.OutputOptions{
    Compression:   document.CompressionPolicy{Enabled: true},
    Limits:        document.ServerSafeLimits(),
    Deterministic: true,
})
```

For request-scoped generation, prefer context-aware output:

```go
ctx, cancel := context.WithTimeout(parent, 10*time.Second)
defer cancel()

err := pdf.OutputContext(ctx, writer)
```

## File-Backed Attachments

Use `AttachmentFromFileWithOptions` for mutable or user-controlled paths:

```go
attachment, err := document.AttachmentFromFileWithOptions(path, document.AttachmentOptions{
    MaxBytes: 64 * 1024 * 1024,
    Eager:    true,
})
```

Output must still re-check limits because a file can change after eager
validation.

## Zero Values

Zero-value `OutputOptions` preserves the document's existing behavior and durable
file sync. Zero fields in `Limits` mean no override. A `SecurityPolicy` is only
enforced when explicitly installed; once installed, false booleans deny the
matching feature.
