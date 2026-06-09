#!/usr/bin/env sh
set -eu

profile="${VERAPDF_PROFILE:-0}"
image="${VERAPDF_DOCKER_IMAGE:-verapdf/cli:v1.30.2}"
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
	-v /tmp:/tmp \
	-w /work \
	"$image" \
	--format text \
	--verbose \
	-f "$profile" \
	"$@"
