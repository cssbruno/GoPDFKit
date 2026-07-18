#!/bin/sh
set -eu

input=${1:-artifacts/generation-core-benchmarks.txt}

if [ ! -f "$input" ]; then
	echo "benchmark budget input not found: $input" >&2
	exit 1
fi

awk '
function require(name) {
	required[name] = 1
}

BEGIN {
	require("BenchmarkGenerationBaselineNoComplianceConcurrent40")
	require("BenchmarkGenerationBaselineNoComplianceNoImageConcurrent40")
	require("BenchmarkGenerationBaselineNoComplianceCachedImageConcurrent40")
	require("BenchmarkGenerationBaselineNoComplianceSignedConcurrent40")
	require("BenchmarkGenerationBaselineNoComplianceCachedImageSignedConcurrent40")
	require("BenchmarkGenerationTextConcurrent40")
	require("BenchmarkGenerationLongTextConcurrent40")
	require("BenchmarkGenerationUTF8TextConcurrent40")
	require("BenchmarkGenerationTextCompressionLevelConcurrent40")
	require("BenchmarkGenerationImagesConcurrent40")
	require("BenchmarkGenerationImagesCachedConcurrent40")
	require("BenchmarkGenerationSVGConcurrent40")
	require("BenchmarkGenerationTemplatesConcurrent40")
	require("BenchmarkGenerationImportedPDFPagesConcurrent40")
	require("BenchmarkGenerationProtectionConcurrent40")
	require("BenchmarkGenerationAttachmentsConcurrent40")
	require("BenchmarkGenerationHTMLLargeTableCompiled")
	require("BenchmarkGenerationHTMLWideTableCompiled")
}

function strip_arch(name) {
	sub(/-[0-9]+$/, "", name)
	return name
}

function fail(name, metric, got, limit) {
	printf("benchmark budget exceeded: %s %s=%s limit=%s\n", name, metric, got, limit) > "/dev/stderr"
	failures++
}

function check_max(name, metric, got, limit) {
	if (got != "" && got + 0 > limit) {
		fail(name, metric, got, limit)
	}
}

function check_generation_budget(name, ns, bytes, allocs) {
	# Budgets intentionally check only stable Go benchmark metrics: ns/op,
	# B/op, and allocs/op.
	root = name
	sub(/\/.*/, "", root)
	if (root in required) {
		# This deliberately broad ceiling catches order-of-magnitude throughput
		# regressions while allowing normal runner-to-runner variance.
		ns_limit = 3000000
		if (root == "BenchmarkGenerationHTMLLargeTableCompiled") {
			# The deleted renderer accepted an unbounded 600-row fixture. The
			# unified semantic planner bounded accepted cohort is 120 rows;
			# retain a calibrated ceiling for that real multi-page workload.
			ns_limit = 80000000
		} else if (root == "BenchmarkGenerationHTMLWideTableCompiled") {
			ns_limit = 40000000
		}
		check_max(name, "ns/op", ns, ns_limit)
	}
	if (name ~ /^BenchmarkGenerationLongText/) {
		check_max(name, "B/op", bytes, 153600)
		check_max(name, "allocs/op", allocs, 500)
		return
	}
	if (name ~ /^BenchmarkGenerationImagesCached/) {
		check_max(name, "B/op", bytes, 153600)
		return
	}
	if (name ~ /^BenchmarkGenerationImages/) {
		check_max(name, "B/op", bytes, 2097152)
		return
	}
	if (name ~ /^BenchmarkGenerationHTMLLargeTableCompiled/) {
		check_max(name, "ns/op", ns, 80000000)
		check_max(name, "B/op", bytes, 134217728)
		return
	}
	if (name ~ /^BenchmarkGenerationHTMLWideTableCompiled/) {
		# The unified track resolver accepts twelve compact columns in the
		# default body; the old 24-column fixture exceeded minimum tracks.
		check_max(name, "ns/op", ns, 40000000)
		check_max(name, "B/op", bytes, 67108864)
		return
	}
	if (name ~ /^BenchmarkGenerationImportedPDFPages/) {
		check_max(name, "B/op", bytes, 102400)
		check_max(name, "allocs/op", allocs, 500)
		return
	}
	if (name ~ /^BenchmarkGenerationAttachments/) {
		check_max(name, "B/op", bytes, 256000)
		check_max(name, "allocs/op", allocs, 400)
		return
	}
	if (name ~ /^BenchmarkGeneration.*Signed/) {
		check_max(name, "B/op", bytes, 1572864)
		check_max(name, "allocs/op", allocs, 2000)
		return
	}
	if (name ~ /^BenchmarkGeneration(TextConcurrent40|UTF8Text.*Concurrent40|TextCompressionLevelConcurrent40)(\/|$)/) {
		check_max(name, "B/op", bytes, 614400)
		check_max(name, "allocs/op", allocs, 2500)
		return
	}
}

/^Benchmark/ {
	seen++
	name = strip_arch($1)
	root = name
	sub(/\/.*/, "", root)
	if (root in required) {
		found[root] = 1
	}
	ns = bytes = allocs = ""
	for (i = 2; i < NF; i++) {
		if ($(i + 1) == "ns/op") {
			ns = $i
		}
		if ($(i + 1) == "B/op") {
			bytes = $i
		}
		if ($(i + 1) == "allocs/op") {
			allocs = $i
		}
	}
	check_generation_budget(name, ns, bytes, allocs)
}

END {
	if (seen == 0) {
		print "no benchmark rows found" > "/dev/stderr"
		exit 1
	}
	for (name in required) {
		if (!(name in found)) {
			fail(name, "benchmark row", "missing", "present")
		}
	}
	if (failures > 0) {
		exit 1
	}
	printf("benchmark budgets ok: %d rows checked\n", seen)
}
' "$input"
