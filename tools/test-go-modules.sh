#!/bin/sh
set -eu

find . -name go.mod -not -path './go.mod' -not -path './tools/go.mod' -print |
sort |
while IFS= read -r go_mod; do
	module_dir=${go_mod%/go.mod}
	(
		cd "$module_dir"
		go mod tidy -diff
		go test ./...
		build_dir=$(mktemp -d)
		trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
		go build -o "$build_dir/" ./...
	)
done
