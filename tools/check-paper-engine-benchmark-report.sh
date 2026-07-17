#!/bin/sh
set -eu

input=${1:-artifacts/paper-engine-benchmarks.txt}
profile=${PAPER_ENGINE_CALIBRATION_PROFILE:-docs/performance/calibrations/apple-m2-go1.26.json}

if [ ! -f "$input" ]; then
	echo "Paper Engine benchmark report not found: $input" >&2
	exit 1
fi

exec go run ./cmd/paper-benchmark-check -profile "$profile" "$input"
