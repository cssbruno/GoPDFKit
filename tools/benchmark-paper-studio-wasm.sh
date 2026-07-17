#!/bin/sh
set -eu

workspace_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/paper-studio-benchmark.XXXXXX")
server_binary="$temporary_dir/paper-studio"
source_file="$temporary_dir/studio-box-model.paper"
server_log="$temporary_dir/server.log"
server_pid=""
port=${PAPER_STUDIO_BENCH_PORT:-17332}
samples=${PAPER_STUDIO_BENCH_SAMPLES:-10}

cleanup() {
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -rf "$temporary_dir"
}
trap cleanup EXIT INT TERM

cd "$workspace_dir"
cp "${PAPER_STUDIO_BENCH_FILE:-testdata/paper/studio-box-model.paper}" "$source_file"
make paper-studio-wasm >/dev/null
go build -o "$server_binary" ./cmd/paper-studio
"$server_binary" -addr "127.0.0.1:$port" "$source_file" >"$server_log" 2>&1 &
server_pid=$!

attempt=0
until curl -fsS "http://127.0.0.1:$port/api/workspace" >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 200 ]; then
		cat "$server_log"
		exit 1
	fi
	sleep 0.05
done

if [ -n "${PAPER_STUDIO_LATENCY_REPORT:-}" ]; then
	mkdir -p "$(dirname "$PAPER_STUDIO_LATENCY_REPORT")"
	PAPER_STUDIO_BENCH_SOURCE="$source_file" \
		node tools/benchmark-paper-studio-wasm.mjs "http://127.0.0.1:$port" "$samples" | tee "$PAPER_STUDIO_LATENCY_REPORT"
else
	PAPER_STUDIO_BENCH_SOURCE="$source_file" \
		node tools/benchmark-paper-studio-wasm.mjs "http://127.0.0.1:$port" "$samples"
fi
