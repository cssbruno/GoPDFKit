#!/usr/bin/env sh
set -eu

out_dir="${COMPLIANCE_OUT:-artifacts/compliance}"

if [ -z "${SRGB_ICC:-}" ]; then
	echo "SRGB_ICC is required, for example /usr/share/color/icc/colord/sRGB.icc" >&2
	exit 2
fi

rm -rf "$out_dir"
mkdir -p "$out_dir"

REQUIRE_COMPLIANCE_TOOLS="${REQUIRE_COMPLIANCE_TOOLS:-1}" \
COMPLIANCE_OUT="$out_dir" \
SRGB_ICC="$SRGB_ICC" \
VERAPDF="${VERAPDF:-tools/verapdf-docker.sh 0}" \
PDFUA_CHECKER="${PDFUA_CHECKER:-tools/verapdf-docker.sh ua2}" \
ARLINGTON_CHECKER="${ARLINGTON_CHECKER:-tools/arlington-validate.sh}" \
ARLINGTON_URL="${ARLINGTON_URL:-http://localhost:8080}" \
ARLINGTON_PROFILE="${ARLINGTON_PROFILE:-arlington2.0}" \
ARLINGTON_REPORT_DIR="${ARLINGTON_REPORT_DIR:-$out_dir}" \
sh tools/compliance-validate.sh

echo "regenerated compliance artifacts in $out_dir"
