#!/usr/bin/env sh
set -eu

out_dir="${COMPLIANCE_OUT:-artifacts/compliance}"
require_tools="${REQUIRE_COMPLIANCE_TOOLS:-0}"
mkdir -p "$out_dir"

if [ "${COMPLIANCE_GENERATE:-1}" != "0" ]; then
	if [ -n "${SRGB_ICC:-}" ]; then
		go run ./cmd/compliance-fixtures -out "$out_dir" -icc "$SRGB_ICC"
	else
		go run ./cmd/compliance-fixtures -out "$out_dir"
	fi
fi

local_pdf_files=""
for pdf in "$out_dir"/*.pdf; do
	if [ -f "$pdf" ]; then
		local_pdf_files="$local_pdf_files $pdf"
	fi
done
if [ -n "$local_pdf_files" ]; then
	# shellcheck disable=SC2086
	go run ./cmd/compliance-check $local_pdf_files
fi

missing=0
failed=0
ran=0

command_name() {
	printf '%s\n' "$1" | awk '{ print $1 }'
}

mark_missing() {
	echo "skip: $1 is not configured or not installed"
	missing=1
}

run_checker() {
	label="$1"
	command_line="$2"
	shift 2
	if [ -z "$command_line" ]; then
		mark_missing "$label"
		return
	fi
	bin="$(command_name "$command_line")"
	if ! command -v "$bin" >/dev/null 2>&1; then
		mark_missing "$label ($bin)"
		return
	fi
	for pdf in "$@"; do
		if [ ! -f "$pdf" ]; then
			continue
		fi
		ran=1
		echo "validate: $label $pdf"
		if ! sh -c "$command_line \"\$1\"" sh "$pdf"; then
			failed=1
		fi
	done
}

pdfa_files=""
for pdf in "$out_dir"/pdfa4*.pdf; do
	if [ -f "$pdf" ]; then
		pdfa_files="$pdfa_files $pdf"
	fi
done
if [ -n "$pdfa_files" ]; then
	# shellcheck disable=SC2086
	run_checker "PDF/A" "${VERAPDF:-verapdf}" $pdfa_files
else
	echo "skip: PDF/A fixtures were not generated; set SRGB_ICC to a real sRGB ICC profile"
fi

run_checker "PDF/UA" "${PDFUA_CHECKER:-}" "$out_dir/pdfua2-arlington-metadata-foundation.pdf"
# Arlington is a PDF model/grammar check, so run it over every generated
# compliance fixture rather than only the tagged PDF/UA fixture.
# shellcheck disable=SC2086
run_checker "Arlington" "${ARLINGTON_CHECKER:-}" $local_pdf_files

if [ "$failed" -ne 0 ]; then
	echo "compliance validation failed"
	exit 1
fi
if [ "$missing" -ne 0 ] && [ "$require_tools" = "1" ]; then
	echo "compliance validation tools are required but missing"
	exit 1
fi
if [ "$ran" -eq 0 ]; then
	echo "compliance fixture generation completed; no external validators ran"
fi
