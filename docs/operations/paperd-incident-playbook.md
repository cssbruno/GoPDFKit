# `paperd` incident playbook

This runbook covers the isolated Paper workspace service and its local
authenticated transport. Never copy source, fixture values, raw capability
handles, approval nonces, signatures, or disclosure labels into monitoring or
incident tickets.

## Monitoring contract

Poll `Workspace.OperationalSnapshot` and retain only its aggregate counts. Alert
before a capacity reaches its configured limit and treat any name in
`saturated` as unavailable capacity. Track request outcomes by stable error code
at the authenticated caller boundary. Cancellation is a normal terminal result;
a sustained increase is operational evidence, not permission to retry a
sensitive operation automatically.

## Quota exhaustion

1. Stop admitting new mutation/render work for the affected workspace.
2. Capture the aggregate operational snapshot and stable error codes.
3. Prune expired handles. Do not raise hard caps during an incident.
4. If pressure remains, create a reviewed configuration change or move the
   project to a separately partitioned workspace.
5. Verify recovery with a bounded read and one non-sensitive dry-run plan.

## Replay, disclosure, or authorization denial spike

1. Fail closed and preserve the hash-only audit chain.
2. Revoke the affected open, authority, approval, and candidate capabilities.
3. Rotate transport authentication material outside `paperd`.
4. Check peer credentials, policy revision, disclosure partition, and clock
   window. Never replay an approval or reuse a nonce.
5. Resume only with fresh capabilities bound to the exact reviewed head.

## Audit-anchor failure

1. Treat a consumed approval as consumed even when the external signer fails.
2. Keep the failed execution audit outcome; do not delete or rewrite the chain.
3. Verify the current root, obtain a fresh signing authority and approval, and
   submit the newly bound statement to the configured external anchor.
4. Confirm the retained receipt hash, signature hash, key identity, and anchor
   URI through `SensitiveAuditAnchors`.

## Persistence corruption or interrupted commit

1. Stop writers and preserve the workspace directory read-only for analysis.
2. Recover only through the authenticated manifest-last generation loader.
3. Reject missing, truncated, noncanonical, symlinked, or authentication-failed
   state. Never hand-edit a generation into validity.
4. Restore the last authenticated generation, reissue fresh capabilities, and
   run bounded protocol/recovery tests before reopening writes.

## Cancellation or timeout storm

1. Confirm callers propagate context cancellation and socket deadlines.
2. Check aggregate retained counts for leaked opens, plans, or authorities.
3. Reduce admission concurrency at the Unix protocol listener.
4. Do not automatically retry export, publish, attachment, signing, production
   capture, candidate acceptance, or audit anchoring; each requires fresh exact
   authorization when its prior approval was consumed.

## Closure evidence

Record UTC start/end times, affected partition identifier held outside logs,
stable error-code counts, aggregate before/after snapshots, revoked capability
families, recovered persistence generation, audit-root/anchor receipt hashes,
tests run, reviewer, and the explicit decision to resume or remain closed.
