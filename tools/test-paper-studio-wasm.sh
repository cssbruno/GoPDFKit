#!/bin/sh
set -eu

workspace_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary_dir=$(mktemp -d "${TMPDIR:-/tmp}/paper-studio-wasm.XXXXXX")
server_binary="$temporary_dir/paper-studio"
server_log="$temporary_dir/server.log"
server_pid=""

cleanup() {
	if [ -n "$server_pid" ]; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	rm -f "$server_binary" "$server_log"
	rmdir "$temporary_dir" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

cd "$workspace_dir"
go build -o "$server_binary" ./cmd/paper-studio
"$server_binary" -addr 127.0.0.1:17331 testdata/paper/studio-demo.paper >"$server_log" 2>&1 &
server_pid=$!

attempt=0
until curl -fsS http://127.0.0.1:17331/api/workspace >/dev/null 2>&1; do
	attempt=$((attempt + 1))
	if [ "$attempt" -ge 100 ]; then
		cat "$server_log"
		exit 1
	fi
	sleep 0.05
done

node tools/test-paper-studio-wasm.mjs http://127.0.0.1:17331
