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
EOF

archives=("$source_dir"/dist/*.tar.gz)
if [[ ${#archives[@]} -ne 4 ]]; then
  printf 'expected 4 release archives, found %d\n' "${#archives[@]}" >&2
  exit 1
fi
for archive in "${archives[@]}"; do
  actual="$release_root/$(basename "$archive").contents"
  tar -tzf "$archive" | sed 's#^\./##' | LC_ALL=C sort >"$actual"
  diff -u "$expected" "$actual"
done
