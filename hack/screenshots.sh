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
  PATH="$screenshot_bin:$PATH" \
    CHANGES_DIFF_ENGINE=internal \
    CHANGES_DIFF_LAYOUT=unified \
    freeze --execute "changes -color always" \
    --output "$repo_dir/docs/changes.png" \
    --width 1100 \
    --padding 24 \
    --margin 16 \
    --window
)
