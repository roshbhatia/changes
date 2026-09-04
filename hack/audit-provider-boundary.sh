#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

root=${1:-.}
manifests=("$root"/extras/*/provider.yaml)
provider_terms=$(
  for manifest in "${manifests[@]}"; do
    sed -nE \
      -e 's/^name:[[:space:]]*([^[:space:]]+).*/\1/p' \
      -e 's/^[[:space:]]*commands:[[:space:]]*\[([^]]+)\].*/\1/p' \
      "$manifest"
  done |
    tr ',' '\n' |
    sed -E 's/^[[:space:]"]+//; s/[[:space:]"]+$//' |
    awk 'NF && $0 != "git"' |
    sort -u |
    sed 's/[][\\.^$*+?(){}|]/\\&/g' |
    paste -sd '|' -
)

if [[ -n $provider_terms ]]; then
  found=0
  while IFS= read -r -d '' source; do
    if grep -nHiE "(^|[^[:alnum:]_])(${provider_terms})([^[:alnum:]_]|$)" "$source"; then
      found=1
    fi
  done < <(
    printf '%s\0' "$root/go.mod"
    find "$root/cmd" -type f -name '*.go' -print0
    find "$root/internal" -type f \( \
      -name '*.go' -o -name '*.txt' -o -name '*.tmpl' -o \
      -name '*.yaml' -o -name '*.yml' -o -name '*.json' \
      \) -print0
  )
  if [[ $found -ne 0 ]]; then
    echo "core source names an optional integration" >&2
    exit 1
  fi
fi

legacy_engine='(engine|Engine).{0,40}"(git|internal|command)"|case "(git|internal|command)"|enum=(git|internal|command)'
if grep -RniE --include='*.go' "$legacy_engine" \
  "$root/cmd" \
  "$root/internal/engine" \
  "$root/internal/appconfig"; then
  echo "core source exposes an implementation-specific diff engine" >&2
  exit 1
fi
