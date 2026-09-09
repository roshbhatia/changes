#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"
output_dir=${CHANGES_MEDIA_OUTPUT_DIR:-"$repo_dir/docs"}
revision=6147beb23c88864180be2cccdec9a52dd1a3a6fc

source_fingerprint() {
  {
    printf '%s\n' go.mod go.sum flake.lock hack/changes.tape hack/screenshots.sh
    find cmd internal -type f -name '*.go' ! -name '*_test.go' -print | LC_ALL=C sort
  } | while IFS= read -r file; do
    sha256sum "$file"
  done | sha256sum | cut -d ' ' -f 1
}

media_is_valid() {
  local duration
  duration=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$output_dir/changes.gif") || return 1
  awk -v duration="$duration" 'BEGIN { exit !(duration >= 50 && duration <= 120) }' || return 1
  ffprobe -v error "$output_dir/changes.png" > /dev/null
}

fingerprint() {
  source_fingerprint
  (cd "$output_dir" && sha256sum changes.gif changes.png)
}

if [[ ${1:-} == --check ]]; then
  media_is_valid && [[ $(fingerprint) == "$(cat "$output_dir/.changes-media.sha256")" ]] || {
    echo 'Changes media is stale or invalid; run ./hack/screenshots.sh' >&2
    exit 1
  }
  exit 0
fi

media_root=$(mktemp -d)
trap 'rm -rf "${media_root:?}"' EXIT
mkdir -p "$output_dir" "$media_root/config" "$media_root/home" "$media_root/cache" "$media_root/data" "$media_root/state"
unset GIT_ALTERNATE_OBJECT_DIRECTORIES GIT_COMMON_DIR GIT_DIR GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_WORK_TREE
git clone --quiet --no-hardlinks "$repo_dir" "$media_root/changes"
git -C "$media_root/changes" checkout --quiet --detach "$revision"
full_path=${CHANGES_DEMO_FULL:-$(nix build .#full --no-link --print-out-paths)}
test -x "$full_path/bin/changes"
(
  cd "$media_root/changes"
  export HOME="$media_root/home"
  export XDG_CONFIG_HOME="$media_root/config"
  export XDG_CACHE_HOME="$media_root/cache"
  export XDG_DATA_HOME="$media_root/data"
  export XDG_STATE_HOME="$media_root/state"
  export XDG_DATA_DIRS="$full_path/share"
  export PATH="$full_path/bin:$PATH"
  unset CHANGES_CONFIG CHANGES_PROVIDERS_DIRECTORY
  vhs "$repo_dir/hack/changes.tape" --output "$output_dir/changes.gif"
  git diff --exit-code --quiet
)
ffmpeg -v error -y -i "$output_dir/changes.gif" -ss 25 -frames:v 1 "$output_dir/changes.png"
media_is_valid
fingerprint > "$output_dir/.changes-media.sha256"
