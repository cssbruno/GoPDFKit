#!/bin/sh

set -eu

out=${1:-artifacts/paper-engine-profiles}
cpu_seconds=${PAPER_ENGINE_PROFILE_CPU_SECONDS:-2}
alloc_iterations=${PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS:-20}

case "$cpu_seconds" in
	''|*[!0-9]*) echo "PAPER_ENGINE_PROFILE_CPU_SECONDS must be an integer from 1 to 30" >&2; exit 2 ;;
esac
case "$alloc_iterations" in
	''|*[!0-9]*) echo "PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS must be an integer from 1 to 100" >&2; exit 2 ;;
esac
if [ "$cpu_seconds" -lt 1 ] || [ "$cpu_seconds" -gt 30 ]; then
	echo "PAPER_ENGINE_PROFILE_CPU_SECONDS must be from 1 to 30" >&2
	exit 2
fi
if [ "$alloc_iterations" -lt 1 ] || [ "$alloc_iterations" -gt 100 ]; then
	echo "PAPER_ENGINE_PROFILE_ALLOC_ITERATIONS must be from 1 to 100" >&2
	exit 2
fi

mkdir -p "$out"
out=$(cd "$out" && pwd)
work="$out/.work"
rm -rf "$work"
mkdir -p "$work"
trap 'rm -rf "$work"' EXIT HUP INT TERM

cpu_count=$(getconf _NPROCESSORS_ONLN 2>/dev/null || true)
if [ -z "$cpu_count" ]; then
	cpu_count=unknown
fi
revision=$(git rev-parse HEAD 2>/dev/null || true)
if [ -z "$revision" ]; then
	revision=unknown
fi
if git diff --quiet --ignore-submodules HEAD 2>/dev/null && \
	git diff --cached --quiet --ignore-submodules HEAD 2>/dev/null; then
	worktree=clean
else
	worktree=dirty
fi

manifest="$out/report.txt"
{
	echo "# paperrune-paper-engine-profiles-v1"
	echo "# generator: tools/run-paper-engine-profiles.sh"
	echo "# go-version: $(go version)"
	echo "# goos: $(go env GOOS)"
	echo "# goarch: $(go env GOARCH)"
	echo "# cpu-count: $cpu_count"
	echo "# source-revision: $revision"
	echo "# worktree: $worktree"
	echo "# cpu-seconds: $cpu_seconds"
	echo "# allocation-iterations: $alloc_iterations"
} >"$manifest"

profile_workload() {
	name=$1
	benchmark=$2
	cpu="$out/$name.cpu.pprof"
	alloc="$out/$name.alloc.pprof"
	cpu_summary="$out/$name.cpu.txt"
	alloc_summary="$out/$name.alloc.txt"
	binary="$work/$name.test"

	{
		echo ""
		echo "## $name"
		echo "benchmark: $benchmark"
		echo "cpu-command: go test ./document -run '^$' -bench '^${benchmark}$' -benchtime=${cpu_seconds}s -count=1 -timeout=2m -cpuprofile=$name.cpu.pprof"
		echo "allocation-command: go test ./document -run '^$' -bench '^${benchmark}$' -benchtime=${alloc_iterations}x -count=1 -timeout=2m -memprofile=$name.alloc.pprof -memprofilerate=1"
	} >>"$manifest"

	go test ./document -run '^$' -bench "^${benchmark}$" \
		-benchtime="${cpu_seconds}s" -count=1 -timeout=2m \
		-o="$binary" -cpuprofile="$cpu"
	go tool pprof -top -nodecount=25 -sample_index=cpu "$binary" "$cpu" >"$cpu_summary"

	go test ./document -run '^$' -bench "^${benchmark}$" \
		-benchtime="${alloc_iterations}x" -count=1 -timeout=2m \
		-o="$binary" -memprofile="$alloc" -memprofilerate=1
	go tool pprof -top -nodecount=25 -sample_index=alloc_space "$binary" "$alloc" >"$alloc_summary"

	go tool pprof -raw "$binary" "$cpu" >/dev/null
	go tool pprof -raw "$binary" "$alloc" >/dev/null
	if grep -q 'Total samples = 0' "$cpu_summary"; then
		echo "$name CPU profile has no samples" >&2
		exit 1
	fi
	if grep -q 'Total: 0' "$alloc_summary"; then
		echo "$name allocation profile has no samples" >&2
		exit 1
	fi

	{
		echo "cpu-profile: $name.cpu.pprof"
		echo "cpu-summary: $name.cpu.txt"
		echo "allocation-profile: $name.alloc.pprof"
		echo "allocation-summary: $name.alloc.txt"
	} >>"$manifest"
}

profile_workload planner-typed BenchmarkPaperEnginePlannerTyped
profile_workload painter-typed BenchmarkPaperEnginePainterTyped
profile_workload end-to-end-typed BenchmarkPaperEngineEndToEndTyped

sh "$(dirname "$0")/check-paper-engine-profile-report.sh" "$out"
echo "paper engine profiles: $out"
