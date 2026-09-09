#!/usr/bin/env bash
set -euo pipefail
shopt -s nullglob

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
release_root=$(mktemp -d)
trap 'rm -rf "$release_root"' EXIT
source_dir="$release_root/source"
mkdir -p "$source_dir"

tar \
  --exclude=.git \
  --exclude=.direnv \
  --exclude=dist \
  -C "$repo_dir" -cf - . |
  tar -C "$source_dir" -xf -

git -C "$source_dir" init --quiet
git -C "$source_dir" config user.name "Changes release check"
git -C "$source_dir" config user.email changes@example.invalid
git -C "$source_dir" add .
git -C "$source_dir" commit --quiet -m snapshot

(
  cd "$source_dir"
  goreleaser release --snapshot --clean --skip=publish
)

expected="$release_root/expected"
cat >"$expected" <<'EOF'
LICENSE
README.md
changes
completions/changes.bash
completions/changes.fish
completions/changes.nu
completions/changes.zsh
schema/changes.schema.json
schema/narrow.cue
schema/provider.schema.json
schema/workspace.schema.json
EOF

archives=("$source_dir"/dist/changes_[0-9]*.tar.gz)
if [[ ${#archives[@]} -ne 3 ]]; then
  printf 'expected 3 release archives, found %d\n' "${#archives[@]}" >&2
  exit 1
fi
for archive in "${archives[@]}"; do
  actual="$release_root/$(basename "$archive").contents"
  tar -tzf "$archive" | sed 's#^\./##' | LC_ALL=C sort >"$actual"
  diff -u "$expected" "$actual"
done

manifests=("$source_dir"/extras/*/provider.yaml)
for manifest in "${manifests[@]}"; do
  provider_dir=$(dirname "$manifest")
  provider=$(basename "$provider_dir")
  archives=("$source_dir"/dist/changes_provider_"$provider"_*.tar.gz)
  if [[ ${#archives[@]} -ne 3 ]]; then
    printf 'expected 3 archives for %s, found %d\n' "$provider" "${#archives[@]}" >&2
    exit 1
  fi
  {
    printf '%s\n' LICENSE README.md "changes-provider-$provider" "share/changes/providers/$provider/provider.yaml"
    if [[ -d "$provider_dir/runtime" ]]; then
      printf '%s\n' runtime/package.json runtime/package-lock.json
    fi
  } | LC_ALL=C sort >"$expected"
  for archive in "${archives[@]}"; do
    actual="$release_root/$(basename "$archive").contents"
    tar -tzf "$archive" | sed 's#^\./##' | LC_ALL=C sort >"$actual"
    diff -u "$expected" "$actual"
  done
done

archives=("$source_dir"/dist/*.tar.gz)
expected_count=$((3 * (1 + ${#manifests[@]})))
if [[ ${#archives[@]} -ne $expected_count ]]; then
  printf 'expected %d total archives, found %d\n' "$expected_count" "${#archives[@]}" >&2
  exit 1
fi
