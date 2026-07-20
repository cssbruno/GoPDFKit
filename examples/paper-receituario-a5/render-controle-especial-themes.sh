#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "$0")" && pwd)
repo_root=$(cd "$script_dir/../.." && pwd)
source_file="$script_dir/controle-especial.paper"
assets_file="$script_dir/assets.json"
data_file="$script_dir/controle-especial.json"
output_dir="$repo_root/output/pdf/controle-especial-temas"
temporary_source=$(mktemp "${TMPDIR:-/tmp}/controle-especial-theme.XXXXXX.paper")
trap 'rm -f "$temporary_source"' EXIT

themes=(
  grafite
  azul-clinico
  verde-institucional
  vinho-classico
  areia-editorial
)

mkdir -p "$output_dir"

for theme in "${themes[@]}"; do
  awk -v selected="@$theme" '
    !changed && /^  theme: "@[^"]+"$/ {
      print "  theme: \"" selected "\""
      changed = 1
      next
    }
    { print }
    END { if (!changed) exit 2 }
  ' "$source_file" > "$temporary_source"

  (
    cd "$repo_root"
    go run ./cmd/paper render \
      --assets "$assets_file" \
      --data "$data_file" \
      -o "$output_dir/controle-especial-a5-$theme.pdf" \
      "$temporary_source"
  )
done

printf 'Rendered %d Paper themes in %s\n' "${#themes[@]}" "$output_dir"
