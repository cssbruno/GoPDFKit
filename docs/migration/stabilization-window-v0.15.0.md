# v0.15.0-rc.1 unified-layout stabilization-window record

Status: **pending**. This record starts the release-bound stabilization
window; it does not close the Stage 10 acceptance gate. The template in
`stabilization-window-record.md` remains the normative list of required
evidence and approvals.

## Release identity

- Release: `v0.15.0-rc.1`
- Candidate commit: `b290cdf9ef135ae4dd273113b02970b37224cd0a`
- Window start (UTC): `2026-07-18T11:05:56Z` (prerelease workflow start)
- Window end (UTC): pending — the stabilization window is active
- Owner: pending — release owner confirmation is required
- Decision: `pending`
- Release workflow: <https://github.com/cssbruno/PaperRune/actions/runs/29641974462>
- Published prerelease: <https://github.com/cssbruno/PaperRune/releases/tag/v0.15.0-rc.1>
- Published (UTC): `2026-07-18T11:08:09Z`

## Candidate corpus and routes

- Corpus manifest: local candidate artifacts under `artifacts/characterization/`;
  immutable release archive and owner acceptance are pending.
- Typed corpus revision/hash: inventory
  `46b27268608c3bc88a425dc7595834f3b154a86dddea068c4600f47f75304f6c`;
  artifact SHA-256
  `fc409eddc1a131e433f936e14f4681c5cddcd4206164b58202f845a248cdb32f`.
  The projection contains 26 fixtures: 19 planned, 1 accepted-malformed,
  1 canceled, 1 rejected, 1 resource-limit, and 3 unsupported.
- HTML corpus revision/hash: pending immutable corpus revision;
  artifact SHA-256
  `8e03e1bf94d9ac407308e44cc76249fe67d644e4700a5b9465efc71178b4a555`.
  The projection contains 11 fixtures: 7 rendered, 1 planned, 1 recovered,
  1 rejected-by-policy, and 1 unsupported.
- Typed route: `unified`
- HTML route: `unified`
- Compatibility, cursor/parity, extracted-text/link, semantic, and raster
  results: candidate artifacts and release-workflow validation passed; the
  accepted corpus, immutable archive, and named reviewer decision are pending.

## Approved budgets and observations

Thresholds must be approved before the stabilization observations are
accepted. No threshold is inferred from these initial candidate samples.

| Budget | Approved threshold | Observed result | Evidence |
| --- | --- | --- | --- |
| Typed legacy-renderer invocations | 0 | pending stabilization cohort | typed characterization artifact above |
| HTML legacy-renderer invocations | 0 | pending stabilization cohort | HTML characterization artifact above |
| Compatibility drift | pending owner approval | candidate validation passed | release workflow above |
| Performance | pending owner approval | passed calibrated local gate | `artifacts/paper-engine-benchmarks.txt`, SHA-256 `bc173fa667a856a275ce393108a14755ef9d35742b41f1a733f882425c7562a7` |
| Compliance | pending owner approval | release-workflow validation passed | release workflow above |
| Race/concurrency | pending owner approval | targeted local race tests passed | local test output; immutable evidence pending |

The performance baseline was recorded on Apple M2, darwin/arm64, Go
`go1.26.5`, using:

```text
GOCACHE=/tmp/paperrune-go-cache make bench-paper-engine-budget
```

The report used profile `docs/performance/calibrations/apple-m2-go1.26.json`,
source revision `8bad212eba38e1c2440895ecfb8cf19ffab0ba02`, and passed all 11
calibrated cohorts. The benchmark artifact is local and ignored; it still
needs to be archived or anchored for formal acceptance.

## Required external evidence

- Benchmark comparison: local calibrated report above; immutable archive pending.
- Race result: `go test -race ./internal/browseroracle ./internal/paperd` passed;
  immutable artifact pending.
- Security result: `make gosec` passed with 0 issues across 371 files;
  immutable artifact and reviewer approval pending.
- PDF/A result: release workflow external-compliance validation passed;
  validator/version artifact reference pending.
- PDF/UA result: release workflow external-compliance validation passed;
  validator/version artifact reference pending.
- Compliance result: release workflow external-compliance validation passed;
  immutable report reference pending.
- Generated-fixture reproduction: release workflow validation passed;
  clean-environment artifact reference pending.
- Audit root/signature: pending.

## Rollback decision

The release owner must record each criterion as `clear` or `triggered` after
the window is observed:

- Supported-corpus failure: pending
- Unapproved semantic or visual drift: pending
- Race failure: pending
- Calibrated budget breach: pending
- Unresolved blocker requiring legacy layout: pending

Until these are closed, the Stage 10 checklist must remain open.

## Closure approval

- Performance/compliance reviewer: pending name, date, and approval artifact
- Security reviewer: pending name, date, and approval artifact
- Release owner: pending name, date, and approval artifact
- Final decision artifact/hash: pending
