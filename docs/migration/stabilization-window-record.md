# Unified layout stabilization-window record

This file is a template for the accepted release artifact required before
Stage 10 legacy-engine deletion can be closed. Copy it for a real release and
replace every placeholder. A local test run, benchmark, or characterization
file is not an accepted stabilization record by itself.

## Release identity

- Release: `<semver release or release candidate>`
- Candidate commit: `<full git commit>`
- Window start (UTC): `<timestamp>`
- Window end (UTC): `<timestamp>`
- Owner: `<named owner>`
- Decision: `pending | accepted | rejected`

## Accepted corpus and routes

- Corpus manifest: `<path or immutable URL>`
- Corpus revision/hash: `<hash>`
- Typed route: `unified`
- HTML route: `unified`
- Compatibility corpus result: `<artifact and result>`
- Cursor/parity result: `<artifact and result>`
- Extracted-text and link result: `<artifact and result>`
- Semantic and raster result: `<artifact and result>`

The corpus must identify the supported typed and HTML inputs, expected
outcomes, and the exact revision used for the window. Unsupported inputs must
remain explicit rejects; they must not be counted as successful fallback.

## Approved budgets and observations

Record the approved threshold before observing the release. Do not infer a
threshold from the first sample.

| Budget | Approved threshold | Observed result | Evidence |
| --- | --- | --- | --- |
| Typed legacy-renderer invocations | `0` | `<value>` | `<artifact>` |
| HTML legacy-renderer invocations | `0` | `<value>` | `<artifact>` |
| Compatibility drift | `<approved value>` | `<value>` | `<artifact>` |
| Performance | `<approved value>` | `<value>` | `<artifact>` |
| Compliance | `<approved value>` | `<value>` | `<artifact>` |
| Race/concurrency | `<approved value>` | `<value>` | `<artifact>` |

Legacy-renderer measurements must identify the cohort/corpus revision and show
zero invocations. Unsupported inputs are explicit failures and are reported by
privacy-safe reason category; they are never routed to another layout engine.
Named platform baselines must include the command, toolchain, hardware/OS
label, and immutable artifact hash.

## Required external evidence

- Benchmark comparison: `<named platform baseline and artifact>`
- Race result: `<command, environment, artifact>`
- Security result: `<approved scan/profile and artifact>`
- PDF/A result: `<validator, version, artifact>`
- PDF/UA result: `<validator, version, artifact>`
- Compliance result: `<profile, version, artifact>`
- Generated-fixture reproduction: `<clean-environment artifact>`
- Audit root/signature: `<signed or anchored artifact>`

## Rollback decision

The release owner must record each criterion as `clear` or `triggered`:

- Supported-corpus failure: `<clear | triggered>`
- Unapproved semantic or visual drift: `<clear | triggered>`
- Race failure: `<clear | triggered>`
- Calibrated budget breach: `<clear | triggered>`
- Unresolved blocker requiring legacy layout: `<clear | triggered>`

If any criterion is triggered, the decision is `rejected` or remains pending;
the Stage 10 checklist must not be marked complete.

## Closure approval

- Performance/compliance reviewer: `<name, date, approval artifact>`
- Security reviewer: `<name, date, approval artifact>`
- Release owner: `<name, date, approval artifact>`
- Final decision artifact/hash: `<hash>`

After the record is accepted, it can support these checklist claims:

1. both unified routes shipped through the window;
2. legacy automatic-renderer invocation counts are zero;
3. no blocker requires legacy layout;
4. compatibility, performance, and compliance budgets pass; and
5. rollback criteria are expired or formally closed.
