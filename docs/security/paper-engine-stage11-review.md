# Paper Engine Stage 11 security review

Review date: 2026-07-18

Scope: `.paper` parsing and incremental edits, package/lockfile and Paper
Document decoding, resource resolution, immutable planning, `paperd` transport,
capability authorization, persistence, production capture, export, publishing,
attachments, and signing.

## Trust boundaries and conclusions

- Source, scenario data, manifests, archives, images, fonts, SVG, protocol
  frames, and persisted workspace generations are hostile inputs. Each enters
  through explicit byte, count, depth, dimension, work, or time bounds.
- Filesystem resources are rooted and content-addressed. Absolute paths,
  traversal, unsafe symlinks, undeclared archive members, digest substitution,
  and non-canonical manifests fail closed.
- Planning and expression evaluation have no ambient I/O authority. Plans bind
  exact source/scenario/policy/page-profile/resource identities and painters
  replay finalized commands after complete preflight.
- `paperd` dispatch requires an authenticated versioned envelope and an
  allowlisted method. Mutations require exact revisions and typed operations;
  protected transitive effects require explicit authority.
- Edit, accept, capture, export, publish, attachment, and signing grants are
  non-interchangeable, bounded, expiring, and replay protected. Denials retain
  bounded one-way evidence rather than protected values.
- Persistence uses authenticated generations, exact generation CAS, process
  locking, atomic replacement, corruption rejection, and crash-boundary
  recovery tests. Audit roots can be externally anchored without retaining raw
  signatures in ordinary protocol projections.
- Operational telemetry exposes aggregate counts and saturated capacities only.
  The incident playbook covers quota exhaustion, replay/disclosure denial,
  anchor failure, persistence corruption, and cancellation storms.

No unresolved code-level critical or high-severity finding was identified in
this scoped review. Release acceptance remains conditional on the automated
security, vulnerability, race, and external fuzz evidence for the exact pushed
candidate; a local run is not a substitute for that external evidence.

## Required verification

- `make gosec`
- `make govulncheck`
- `go test ./...`
- `go test -race ./...`
- GitHub Actions `Fuzz` workflow, including incremental `.paper` parsing and
  lockfile migration in addition to PDF, SVG, HTML, signature, and archive
  boundaries

External run identifiers and the exact candidate commit belong in the
stabilization acceptance record after the workflow completes.

## Residual risks

- New parser, archive, resource, protocol, and persistence entry points must add
  seeds and remain in the scheduled external fuzz workflow.
- Kernel credential behavior must continue to run on both Linux CI and macOS;
  cross-compilation alone is not execution evidence.
- Organization approvals and audit anchors prove exact identities and policy
  decisions, not the correctness of an external reviewer or signer.
- The active release candidate remains subject to its full stabilization window
  and named human acceptance. This review does not close that governance gate.
