#!/bin/sh
set -eu

if [ "$#" -lt 2 ]; then
	echo "usage: $0 OUTPUT COMMAND [ARG ...]" >&2
	exit 2
fi

output=$1
shift

mkdir -p "$(dirname "$output")"
temporary="${output}.tmp.$$"
trap 'rm -f "$temporary"' EXIT HUP INT TERM

if "$@" >"$temporary" 2>&1; then
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
