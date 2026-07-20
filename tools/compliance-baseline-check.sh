#!/usr/bin/env sh
set -eu

out_dir="${1:-${COMPLIANCE_OUT:-artifacts/compliance}}"
baseline_dir="${COMPLIANCE_BASELINE_DIR:-testdata/compliance}"
image="${VERAPDF_DOCKER_IMAGE:-verapdf/cli@sha256:d5ee329657cf9bc4b2400392dd54c7d0a0ce9980ff6fa2da5590eebeec007cdb}"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

normalize_xml() {
	sed -E \
		-e 's#<name>.*</name>#<name>PDF</name>#' \
		-e 's#<item size="[0-9]+">#<item size="SIZE">#' \
		-e 's#<duration start="[0-9]+" finish="[0-9]+">[^<]*</duration>#<duration>DURATION</duration>#'
}

normalize_json() {
	awk '/"processingTime"/ { exit } /"batchSummary"/ { exit } { print }' | sed -E \
		-e '/^[[:space:]]*"extensions"[[:space:]]*:[[:space:]]*"",?[[:space:]]*$/d' \
		-e 's#"version"[[:space:]]*:[[:space:]]*"[^"]*"#"version" : "VERSION"#g' \
		-e 's#"buildDate"[[:space:]]*:[[:space:]]*[0-9]+#"buildDate" : BUILDDATE#g' \
		-e 's#"size"[[:space:]]*:[[:space:]]*[0-9]+#"size" : SIZE#g' \
		-e 's#"passedRules"[[:space:]]*:[[:space:]]*[0-9]+#"passedRules" : PASSED_RULES#g' \
		-e 's#"passedChecks"[[:space:]]*:[[:space:]]*[0-9]+#"passedChecks" : PASSED_CHECKS#g' \
		-e 's#"start"[[:space:]]*:[[:space:]]*[0-9]+#"start" : TIME#g' \
		-e 's#"finish"[[:space:]]*:[[:space:]]*[0-9]+#"finish" : TIME#g' \
		-e 's#"difference"[[:space:]]*:[[:space:]]*[0-9]+#"difference" : DURATION#g' \
		-e 's#"duration"[[:space:]]*:[[:space:]]*"[^"]*"#"duration" : "DURATION"#g'
}

compare_normalized() {
	name="$1"
	kind="$2"
	actual="$3"
	expected="$baseline_dir/$name"
	if [ ! -f "$expected" ]; then
		echo "missing baseline: $expected" >&2
		return 1
	fi
	if [ ! -f "$actual" ]; then
		echo "missing report: $actual" >&2
		return 1
	fi
	case "$kind" in
		xml)
			normalize_xml < "$expected" > "$tmp_dir/expected-$name"
			normalize_xml < "$actual" > "$tmp_dir/actual-$name"
			;;
		json)
			normalize_json < "$expected" > "$tmp_dir/expected-$name"
			normalize_json < "$actual" > "$tmp_dir/actual-$name"
			;;
		*)
			echo "unknown report kind: $kind" >&2
			return 2
			;;
	esac
	diff -u "$tmp_dir/expected-$name" "$tmp_dir/actual-$name"
}

require_readable_file() {
	if [ ! -f "$1" ]; then
		echo "missing compliance artifact: $1" >&2
		echo "generate artifacts first, for example: make compliance-validate SRGB_ICC=/path/to/sRGB.icc" >&2
		return 1
	fi
	if [ ! -r "$1" ]; then
		echo "compliance artifact is not readable: $1" >&2
		return 1
	fi
}

verapdf_report() {
	profile="$1"
	pdf="$2"
	report="$3"
	require_readable_file "$pdf"
	input_dir="$tmp_dir/verapdf-input"
	mkdir -p "$input_dir"
	cp "$pdf" "$input_dir/input.pdf"
	docker run --rm \
		--network none \
		--read-only \
		--cap-drop ALL \
		--security-opt no-new-privileges \
		--tmpfs /tmp:rw,noexec,nosuid,size=64m \
		--mount "type=bind,source=$input_dir,target=/inputs,readonly" \
		"$image" \
		--format xml \
		--verbose \
		-f "$profile" \
		/inputs/input.pdf > "$report"
	rm -f "$input_dir/input.pdf"
}

compare_verapdf() {
	baseline="$1"
	profile="$2"
	pdf="$3"
	report="$tmp_dir/$baseline"
	verapdf_report "$profile" "$pdf" "$report"
	compare_normalized "$baseline" xml "$report"
}

compare_verapdf "verapdf-pdfa4.xml" 0 "$out_dir/pdfa4-metadata.pdf"
compare_verapdf "verapdf-pdfa4e.xml" 0 "$out_dir/pdfa4e-attachment-metadata.pdf"
compare_verapdf "verapdf-pdfa4f.xml" 0 "$out_dir/pdfa4f-attachment-metadata.pdf"
compare_verapdf "verapdf-pdfua2.xml" ua2 "$out_dir/pdfua2-arlington-metadata-foundation.pdf"
compare_verapdf "verapdf-signed-pdfa4f-pdfua2-arlington.xml" 0 "$out_dir/pdfa4f-pdfua2-arlington-signed.pdf"
compare_verapdf "verapdf-signed-pdfua2.xml" ua2 "$out_dir/pdfa4f-pdfua2-arlington-signed.pdf"

compare_normalized "arlington-pdfa4.json" json "$out_dir/pdfa4-metadata-arlington.json"
compare_normalized "arlington-pdfa4e.json" json "$out_dir/pdfa4e-attachment-metadata-arlington.json"
compare_normalized "arlington-pdfa4f.json" json "$out_dir/pdfa4f-attachment-metadata-arlington.json"
compare_normalized "arlington-pdf20.json" json "$out_dir/pdfua2-arlington-metadata-foundation-arlington.json"
compare_normalized "arlington-signed-pdfa4f-pdfua2.json" json "$out_dir/pdfa4f-pdfua2-arlington-signed-arlington.json"

echo "compliance baselines match $baseline_dir"
