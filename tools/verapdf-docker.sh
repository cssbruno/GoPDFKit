#!/usr/bin/env sh
set -eu

profile="${VERAPDF_PROFILE:-0}"
image="${VERAPDF_DOCKER_IMAGE:-verapdf/cli@sha256:d5ee329657cf9bc4b2400392dd54c7d0a0ce9980ff6fa2da5590eebeec007cdb}"
if [ "$#" -gt 1 ]; then
	profile="$1"
	shift
fi
if [ "$#" -lt 1 ]; then
	echo "usage: VERAPDF_PROFILE=0 tools/verapdf-docker.sh [profile] file.pdf" >&2
	exit 2
fi

input_dir="$(mktemp -d)"
trap 'rm -rf "$input_dir"' EXIT
index=0
for pdf do
	if [ ! -f "$pdf" ] || [ ! -r "$pdf" ]; then
		echo "veraPDF input is not a readable file: $pdf" >&2
		exit 1
	fi
	index=$((index + 1))
	cp "$pdf" "$input_dir/input-$index.pdf"
done

input_count="$index"
set --
index=0
while [ "$index" -lt "$input_count" ]; do
	index=$((index + 1))
	set -- "$@" "/inputs/input-$index.pdf"
done

docker run --rm \
	--network none \
	--read-only \
	--cap-drop ALL \
	--security-opt no-new-privileges \
	--tmpfs /tmp:rw,noexec,nosuid,size=64m \
	--mount "type=bind,source=$input_dir,target=/inputs,readonly" \
	"$image" \
	--format text \
	--verbose \
	-f "$profile" \
	"$@"
