# paperd protocol evaluation

Evaluation date: 2026-07-18  
Candidate family: `codex/paper-engine-foundation`

The protocol, privacy, replay, concurrency, and recovery corpus passes on the
host platform:

```text
GOCACHE=/tmp/gopdfkit-go-cache go test ./internal/paperd -count=1
ok github.com/cssbruno/gopdfkit/internal/paperd 25.681s

GOCACHE=/tmp/gopdfkit-go-cache go test -race ./internal/paperd -count=1
ok github.com/cssbruno/gopdfkit/internal/paperd 415.964s
```

The Linux peer-credential boundary was executed against a real Linux kernel in
the locally installed `postgres:16-alpine` container. The test binary was
cross-built as a static Linux arm64 binary, but the test itself ran inside the
container; this is execution evidence, not merely a cross-compilation check.

```text
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c ./internal/paperd -o /tmp/gopdfkit-paperd-linux.test
docker run --rm -v /tmp/gopdfkit-paperd-linux.test:/paperd.test:ro \
  --entrypoint /paperd.test postgres:16-alpine \
  -test.run '^TestLinuxUnixProtocolUsesKernelPeerCredentials$' -test.v
=== RUN   TestLinuxUnixProtocolUsesKernelPeerCredentials
--- PASS: TestLinuxUnixProtocolUsesKernelPeerCredentials (0.01s)
PASS
```

The host package suite also executes the macOS `LOCAL_PEERCRED`/
`LOCAL_PEERPID` boundary. The shared corpus covers authenticated version
negotiation, downgrade rejection, framing and time limits, peer denial,
cross-workspace isolation, replay windows, panic containment, capability and
disclosure filtering, concurrent dispatch, cancellation, persistence restart,
and crash-boundary recovery.
