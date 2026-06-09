#!/usr/bin/env sh
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: tools/arlington-validate.sh file.pdf" >&2
	exit 2
fi

pdf="$1"
profile="${ARLINGTON_PROFILE:-arlington2.0}"
base_url="${ARLINGTON_URL:-http://localhost:8080}"
report_dir="${ARLINGTON_REPORT_DIR:-}"

if [ -n "$report_dir" ]; then
	mkdir -p "$report_dir"
	report="$report_dir/$(basename "$pdf" .pdf)-arlington.json"
else
	report="$(mktemp)"
	trap 'rm -f "$report"' EXIT
fi

curl -fsS \
	-H "Accept: application/json" \
	-F "file=@${pdf};type=application/pdf" \
	"$base_url/api/validate/$profile" > "$report"

if ! grep -q '"compliant"[[:space:]]*:[[:space:]]*true' "$report"; then
	cat "$report"
	echo "Arlington validation failed for $pdf using $profile" >&2
	exit 1
fi

echo "PASS $pdf $profile"
