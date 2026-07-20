# PaperRune full security analysis

Assessment date: 2026-07-19  
Scope: repository source, tests, build and release workflows, Paper Studio,
`paperd`, package/archive resolution, PDF import/inspection/sanitization, and
signature verification  
Candidate basis: commit `2c199f99cbf3505aac0eff417e8b214f9bc0aba7`
plus the current uncommitted worktree  
Assessment type: local engineering review; not an independent penetration
test or release approval

## Executive summary

No open critical, high, medium, or low-severity security finding was identified
in the reviewed candidate. One static-analysis concern involving a bounded
`uint32` conversion was resolved during the review by documenting the complete
input-domain bound and adding a maximum-domain regression test.

The strongest controls are the closed, revision-bound mutation model; bounded
parsing and archive extraction; loopback-only Studio listener; authenticated
and replay-protected local daemon protocol; exact asset digests; and explicit
separation of mutation, export, signing, and disclosure capabilities.

This result is not sufficient to close the repository's release-governance
checklist. The candidate is not immutable, the current candidate has not run
the external GitHub Actions fuzz campaign, no independent security reviewer has
accepted it, and its audit root has not been signed or externally anchored.

| Open severity | Count |
| --- | ---: |
| Critical | 0 |
| High | 0 |
| Medium | 0 |
| Low | 0 |

## Architecture and trust boundaries

The repository has four security-relevant surfaces:

1. Public Go packages accept PDFs, templates, SVG, signatures, fonts, and
   structured document data from callers. The primary risks are malicious file
   structure, decompression or parser resource exhaustion, and ambiguous
   cryptographic trust.
2. Paper Studio exposes local HTTP APIs for preview, inspection, editing, and
   export. It is explicitly bound to loopback, applies browser security headers,
   bounds JSON input, serializes edits, and rejects stale source/plan revisions
   ([listener and timeouts](cmd/paper-studio/main.go#L130),
   [headers](cmd/paper-studio/main.go#L276),
   [bounded JSON](cmd/paper-studio/main.go#L777),
   [edit preconditions](cmd/paper-studio/studio_edit.go#L98)).
3. `paperd` crosses a local process boundary over Unix sockets. The socket is
   mode `0600`, verifies OS peer identity, authenticates protocol envelopes with
   HMAC, rejects replays, and filters methods by capability
   ([Unix boundary](internal/paperd/protocol_unix.go#L67),
   [transport checks](internal/paperd/protocol_transport.go#L197)).
4. `.paperdoc` and package inputs cross filesystem/archive boundaries. Resolvers
   use a rooted filesystem handle, canonicalize symlinks, require SHA-256
   identity, and enforce path, file, count, compression-ratio, and total-size
   limits ([resolver](internal/paperpkg/resolver.go#L51),
   [archive limits](internal/paperpkg/archive.go#L24),
   [lockfile limits](internal/paperpkg/lockfile.go#L30)).

Sensitive actions do not inherit authority merely from an edit-capable open.
Mutation grants bind one exact candidate/open, operation vocabulary, node scope,
protected scope, disclosure domain, partition, and expiry. Authorization checks
include transitive semantic effects
([grant](internal/paperd/authorization.go#L129),
[enforcement](internal/paperd/authorization.go#L238)).

## Threat scenarios and control assessment

| Scenario | Existing control | Assessment |
| --- | --- | --- |
| Crafted PDF causes unbounded read or parser work | Import reads through a context-aware limit and rejects sources above 128 MiB; sanitizer, inspector, and signature scanners have hostile-input fuzz targets | Locally tested; external sustained fuzzing remains pending |
| ZIP/package traversal or decompression bomb | Canonical relative paths, rooted resolution, digest verification, per-file and aggregate byte limits, file-count/depth/ratio limits | No bypass found in reviewed paths |
| Browser or remote host reaches Studio APIs | Explicit loopback address validation, server timeouts, restrictive CSP and same-origin headers | Appropriate for the documented local-only deployment model |
| Stale or ambiguous visual edit mutates the wrong node | Exact source and plan revisions, exactly-one target resolution, target fingerprint/instance preconditions, serialized mutation | Fail-closed design and tests present |
| Local process forges or replays daemon requests | Unix peer credential policy, HMAC envelope authentication, clock window, one-use request ID retention, capability filtering | Targeted race/protocol suite passes |
| Package or document signature is treated as trusted without validation | Detached CMS integrity and certificate-chain paths are distinct; package signatures use Ed25519 over canonical payloads | Callers must still provide and govern their trust store correctly |
| Compromised CI action gains broad repository authority | Actions and validator images are digest-pinned; normal jobs use read-only contents; release write permission is job-scoped | No unpinned action found in reviewed workflows |

## Findings

### Open findings

None identified in this local assessment.

### Resolved during assessment

#### R-001 — Bounded fixed-point conversion lacked machine-checkable rationale

- Severity before resolution: informational
- Location: `internal/papercompile/typed_tree.go`
- Trigger: `gosec` G115 flagged conversion of the result of arithmetic over a
  `uint32` percentage into the signed fixed-point type.
- Security impact: the full domain produces at most `43,980`, so the conversion
  cannot overflow `int64`; the implementation was safe but the bound was not
  explicit to the scanner or future maintainers.
- Resolution: documented the complete-domain bound, narrowly suppressed the
  false positive at the conversion, and added
  `TestTypedTreePercentCoversFullUint32Domain` using `^uint32(0)`.
- Verification: focused package tests pass and the repeated `gosec` scan reports
  zero issues.

## Verification evidence

The following checks passed on darwin/arm64 with Go 1.26.5:

- `make gosec`: 382 files, 130,807 lines, 0 issues.
- `make govulncheck`: no vulnerabilities found by the current Go vulnerability
  database at evaluation time.
- Ten hostile-input fuzz targets, each for 10 seconds: template and SVG decode,
  PDF import/sanitize/inspect, three signature scanners, incremental Paper CST,
  and package lockfile migration.
- `go test -race ./...`: pass across the complete repository.
- Paper Studio JavaScript suite: 41 tests pass.
- Browser/WASM smoke: page 1 rendered at 1191 by 1684 pixels to a deterministic
  408,358-byte PNG.
- `make lint` and `make nilaway`: pass; lint reports 0 issues.
- The complete `release-check` component set passes: release metadata, tests,
  vet, formatting, module verification, full race suite, coverage thresholds,
  lint, NilAway, `gosec`, current `govulncheck`, and build. The vulnerability
  query was rerun with network access after the aggregate target encountered
  the sandbox's DNS restriction.
- Benchmark budgets, characterization corpora, and non-containerized PDF-reader
  smoke also pass.
- The veraPDF compliance baseline matches after hardening the wrapper to use a
  digest-pinned, network-disabled, capability-free, read-only container with
  only copied PDF inputs mounted read-only.
- Best-effort repository secret search found no private-key/certificate files or
  credential assignment. Dedicated `gitleaks`/`trufflehog` binaries were not
  installed, so this does not replace a release secret-scanning service.

Fresh local evidence is recorded in
[`docs/security/local-candidate-evaluation-2026-07-19.md`](docs/security/local-candidate-evaluation-2026-07-19.md).

## Remediation roadmap

### Before release acceptance

1. Commit and publish one immutable candidate revision.
2. Run the external GitHub Actions fuzz workflow against that exact revision
   for the checklist-required duration and retain its artifacts.
3. Obtain named independent security-reviewer acceptance bound to the immutable
   revision and evidence hashes.
4. Sign or externally anchor the candidate audit root and retain the receipt.

### Next hardening cycle

1. Add a dedicated secret scanner to CI with an explicit baseline and fail-closed
   policy.
2. Keep the narrowed veraPDF wrappers covered by release tests. During this
   assessment they were changed from broad workspace and host-`/tmp` mounts to
   a digest-pinned, read-only, network-disabled container receiving only copied
   PDF inputs.
3. Continue sustained fuzzing and feed any external crash corpus into regression
   tests before accepting future candidates.

## Residual risk and limitations

- The reviewed state is a dirty worktree, not an immutable artifact.
- Ten-second local fuzz runs are smoke-level evidence and do not substitute for
  the external campaign.
- Dependency vulnerability results are point-in-time and depend on the current
  vulnerability database.
- Studio is assessed under its explicit loopback-only trust model; exposing it
  through a proxy or non-loopback tunnel would require authentication, CSRF and
  origin controls, TLS, and a separate deployment review.
- No independent reviewer, production telemetry, cloud configuration, or
  externally anchored audit receipt was available to this local assessment.
