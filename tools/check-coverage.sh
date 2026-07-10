#!/bin/sh
set -eu

output=$(go test ./document ./importpdf ./inspect ./sign -cover)
printf '%s\n' "$output"

printf '%s\n' "$output" | awk '
BEGIN {
	minimum["github.com/cssbruno/gopdfkit/document"] = 80
	minimum["github.com/cssbruno/gopdfkit/importpdf"] = 60
	minimum["github.com/cssbruno/gopdfkit/inspect"] = 55
	minimum["github.com/cssbruno/gopdfkit/sign"] = 70
}
$1 == "ok" {
	pkg = $2
	for (i = 1; i <= NF; i++) {
		if ($i != "coverage:") {
			continue
		}
		coverage = $(i + 1)
		sub(/%$/, "", coverage)
		if (pkg in minimum) {
			seen[pkg] = 1
			if (coverage + 0 < minimum[pkg]) {
				printf "coverage for %s is %.1f%%; minimum is %.1f%%\n", pkg, coverage, minimum[pkg] > "/dev/stderr"
				failed = 1
			}
		}
	}
}
END {
	for (pkg in minimum) {
		if (!(pkg in seen)) {
			printf "coverage result missing for %s\n", pkg > "/dev/stderr"
			failed = 1
		}
	}
	exit failed
}'
