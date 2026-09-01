#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"

screenshot_bin=$(mktemp -d)
fixture=$(mktemp -d)
trap 'rm -rf "$screenshot_bin" "$fixture"' EXIT
go build -o "$screenshot_bin/changes" .

git -C "$fixture" init -q
git -C "$fixture" config user.email screenshot@example.com
git -C "$fixture" config user.name Screenshot
printf 'old\n' > "$fixture/note.txt"
git -C "$fixture" add note.txt
git -C "$fixture" commit -qm initial
printf 'new\nreviewed\n' > "$fixture/note.txt"

(
  cd "$fixture"
  PATH="$screenshot_bin:$PATH" \
    CHANGES_DIFF_ENGINE=delta \
    CHANGES_DIFF_LAYOUT=side-by-side \
    freeze --execute "changes -no-calls -no-symbols -color always" \
    --output "$repo_dir/docs/changes.png" \
    --width 1100 \
    --padding 24 \
    --margin 16 \
    --window
)
