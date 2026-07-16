#!/bin/sh
set -eu

{
	printf '%s\n' ./go.mod ./tools/go.mod
	find . -name go.mod -not -path './go.mod' -not -path './tools/go.mod' -print
} |
sort -u |
while IFS= read -r go_mod; do
	module_dir=${go_mod%/go.mod}
	(
		cd "$module_dir"
		go mod tidy -diff
		go mod verify
		packages=$(go list ./...)
		if [ -n "$packages" ]; then
			go test ./...
			build_dir=$(mktemp -d)
			trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
			go build -o "$build_dir/" ./...
		fi
	)
done
