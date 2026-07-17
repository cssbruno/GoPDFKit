#!/bin/sh

set -eu

dir=${1:-artifacts/paper-engine-profiles}
report="$dir/report.txt"

test -s "$report" || { echo "missing profile report: $report" >&2; exit 1; }

require_line() {
	grep -Fq "$1" "$report" || { echo "profile report is missing: $1" >&2; exit 1; }
}

require_line '# gopdfkit-paper-engine-profiles-v1'
require_line '# generator: tools/run-paper-engine-profiles.sh'
require_line '# go-version: go version '
require_line '# goos: '
require_line '# goarch: '
require_line '# cpu-count: '
require_line '# source-revision: '
require_line '# worktree: '
require_line '# cpu-seconds: '
require_line '# allocation-iterations: '

for name in planner-typed painter-typed end-to-end-typed; do
	require_line "## $name"
	require_line "cpu-profile: $name.cpu.pprof"
	require_line "cpu-summary: $name.cpu.txt"
	require_line "allocation-profile: $name.alloc.pprof"
	require_line "allocation-summary: $name.alloc.txt"

	for kind in cpu alloc; do
		profile="$dir/$name.$kind.pprof"
		test -s "$profile" || { echo "missing profile: $profile" >&2; exit 1; }
	done
	for kind in cpu alloc; do
		summary="$dir/$name.$kind.txt"
		test -s "$summary" || { echo "missing profile summary: $summary" >&2; exit 1; }
		grep -Fq 'Showing nodes accounting for' "$summary" || {
			echo "profile summary has no bounded top table: $summary" >&2
			exit 1
		}
	done
done

echo "paper engine profile report is complete: $report"
