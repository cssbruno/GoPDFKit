#!/usr/bin/env sh
set -eu

profile="${VERAPDF_PROFILE:-0}"
if [ "$#" -gt 1 ]; then
	profile="$1"
	shift
fi
if [ "$#" -lt 1 ]; then
	echo "usage: VERAPDF_PROFILE=0 tools/verapdf-docker.sh [profile] file.pdf" >&2
	exit 2
fi

exec docker run --rm \
	-v "$PWD:/work" \
	-w /work \
	verapdf/cli \
	--format text \
	--verbose \
	-f "$profile" \
	"$@"
