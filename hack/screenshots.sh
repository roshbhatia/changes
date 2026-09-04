#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"
output_dir=${CHANGES_MEDIA_OUTPUT_DIR:-"$repo_dir/docs"}
mkdir -p "$output_dir"

media_fingerprint() {
  {
    printf '%s\n' flake.lock flake.nix go.mod go.sum hack/changes.tape hack/screenshots.sh
    find cmd internal extras -type f \
      \( -name '*.go' -o -name 'package.nix' -o -name 'provider.yaml' -o -name 'package.json' -o -name 'package-lock.json' \) \
      ! -name '*_test.go' -print | LC_ALL=C sort
  } | while IFS= read -r path; do
    sha256sum "$path"
  done | sha256sum | cut -d ' ' -f 1
}

media_is_valid() {
  local gif_format
  local png_codec
  [[ -s $output_dir/changes.png && -s $output_dir/changes.gif ]] || return 1
  png_codec=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name \
    -of default=noprint_wrappers=1:nokey=1 "$output_dir/changes.png") || return 1
  gif_format=$(ffprobe -v error -show_entries format=format_name \
    -of default=noprint_wrappers=1:nokey=1 "$output_dir/changes.gif") || return 1
  [[ $png_codec == png && $gif_format == gif ]]
}

if [[ ${1:-} == "--check" ]]; then
  expected=$(media_fingerprint)
  current=$(cat "$output_dir/.changes-media.sha256" 2> /dev/null || true)
  if [[ $current != "$expected" ]] || ! media_is_valid; then
    echo "Changes media is stale; run ./hack/screenshots.sh" >&2
    exit 1
  fi
  exit 0
fi

media_root=$(mktemp -d)
fixture="$media_root/fixture"
trap 'rm -rf "$media_root"' EXIT
mkdir -p \
  "$fixture" \
  "$media_root/cache" \
  "$media_root/config" \
  "$media_root/data" \
  "$media_root/data-dirs" \
  "$media_root/home"

full_path=$(nix build .#full --no-link --print-out-paths)

git -C "$fixture" init -q
git -C "$fixture" config user.email screenshot@example.com
git -C "$fixture" config user.name Screenshot
mkdir -p "$fixture/internal/auth"
printf '%s\n' \
  'package auth' \
  '' \
  'import (' \
  '    "errors"' \
  '    "strings"' \
  ')' \
  '' \
  'var ErrEmptyToken = errors.New("token is empty")' \
  '' \
  'func NormalizeToken(raw string) (string, error) {' \
  '    token := strings.TrimSpace(raw)' \
  '    if token == "" {' \
  '        return "", ErrEmptyToken' \
  '    }' \
  '    return token, nil' \
  '}' \
  '' \
  'func Authorize(raw string, allowed map[string]bool) error {' \
  '    token, err := NormalizeToken(raw)' \
  '    if err != nil {' \
  '        return err' \
  '    }' \
  '    if !allowed[token] {' \
  '        return errors.New("token rejected")' \
  '    }' \
  '    return nil' \
  '}' \
  > "$fixture/internal/auth/token.go"
git -C "$fixture" add internal/auth/token.go
git -C "$fixture" commit -qm initial
printf '%s\n' \
  'package auth' \
  '' \
  'import (' \
  '    "errors"' \
  '    "fmt"' \
  '    "strings"' \
  ')' \
  '' \
  'var (' \
  '    ErrEmptyToken = errors.New("token is empty")' \
  '    ErrTokenScheme = errors.New("token must use the Bearer scheme")' \
  '    ErrRejectedToken = errors.New("token rejected")' \
  ')' \
  '' \
  'func NormalizeToken(raw string) (string, error) {' \
  '    raw = strings.TrimSpace(raw)' \
  '    if raw == "" {' \
  '        return "", ErrEmptyToken' \
  '    }' \
  '    if !strings.HasPrefix(raw, "Bearer ") {' \
  '        return "", ErrTokenScheme' \
  '    }' \
  '    token := strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))' \
  '    if token == "" {' \
  '        return "", ErrEmptyToken' \
  '    }' \
  '    return token, nil' \
  '}' \
  '' \
  'func Authorize(raw string, allowed map[string]bool) error {' \
  '    token, err := NormalizeToken(raw)' \
  '    if err != nil {' \
  '        return err' \
  '    }' \
  '    if !allowed[token] {' \
  '        return fmt.Errorf("%w: %.4s…", ErrRejectedToken, token)' \
  '    }' \
  '    return nil' \
  '}' \
  > "$fixture/internal/auth/token.go"

(
  cd "$fixture"
  export HOME="$media_root/home"
  export XDG_CACHE_HOME="$media_root/cache"
  export XDG_CONFIG_HOME="$media_root/config"
  export XDG_DATA_HOME="$media_root/data"
  export XDG_DATA_DIRS="$media_root/data-dirs"
  unset CHANGES_CONFIG CHANGES_PROVIDERS_DIRECTORY
  PATH="$full_path/bin:$PATH" \
    CHANGES_DIFF_ENGINE=builtin \
    CHANGES_DIFF_LAYOUT=unified \
    freeze --execute "changes -color always" \
    --output "$output_dir/changes.png" \
    --width 1100 \
    --padding 24 \
    --margin 16 \
    --window

  PATH="$full_path/bin:$PATH" \
    CHANGES_DIFF_ENGINE=builtin \
    CHANGES_DIFF_LAYOUT=unified \
    vhs "$repo_dir/hack/changes.tape" --output "$output_dir/changes.gif"
)

if ! media_is_valid; then
  echo "Changes media generation produced an empty or invalid image" >&2
  exit 1
fi

media_fingerprint > "$output_dir/.changes-media.sha256"
