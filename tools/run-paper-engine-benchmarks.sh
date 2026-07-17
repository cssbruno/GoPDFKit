#!/bin/sh
set -eu

output=${1:-artifacts/paper-engine-benchmarks.txt}
count=${PAPER_ENGINE_BENCH_COUNT:-10}
benchtime=${PAPER_ENGINE_BENCHTIME:-250ms}
benchmark='^BenchmarkPaperEngine(Planner|Painter|EndToEnd|WarmCompiled|Concurrent|Table)'

case "$count" in
	''|*[!0-9]*|0)
		echo "PAPER_ENGINE_BENCH_COUNT must be a positive integer, got: $count" >&2
		exit 2
		;;
esac

mkdir -p "$(dirname "$output")"
temporary="${output}.tmp.$$"
trap 'rm -f "$temporary"' EXIT HUP INT TERM

revision=$(git rev-parse HEAD 2>/dev/null || printf unknown)
if git diff --quiet --no-ext-diff HEAD 2>/dev/null &&
	[ -z "$(git ls-files --others --exclude-standard 2>/dev/null)" ]; then
	worktree=clean
else
	worktree=dirty
fi

{
	printf '# gopdfkit-paper-engine-benchmark-v3\n'
	printf '# command: go test ./document ./internal/layoutengine -run ^$ -bench %s -benchmem -benchtime=%s -count=%s\n' "$benchmark" "$benchtime" "$count"
	printf '# go-version: %s\n' "$(go version)"
	printf '# goos: %s\n' "$(go env GOOS)"
	printf '# goarch: %s\n' "$(go env GOARCH)"
	printf '# source-revision: %s\n' "$revision"
	printf '# worktree: %s\n' "$worktree"
} >"$temporary"

if go test ./document ./internal/layoutengine -run '^$' -bench "$benchmark" -benchmem -benchtime="$benchtime" -count="$count" >>"$temporary" 2>&1; then
	status=0
else
	status=$?
fi

cat "$temporary"
if [ "$status" -ne 0 ]; then
	exit "$status"
fi

mv "$temporary" "$output"
trap - EXIT HUP INT TERM
