#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"
example_names=()
while IFS= read -r example; do
  example_names+=("$(basename "$example")")
done < <(find examples -mindepth 1 -maxdepth 1 -type d | LC_ALL=C sort)

demo_fingerprint() {
  local example="$1"
  {
    printf '%s\n' \
      flake.lock \
      flake.nix \
      go.mod \
      go.sum \
      hack/recipe-fixture.py \
      hack/example-demos.sh
    case "$example" in
      agent-review-notes) printf '%s\n' hack/demo-codex.sh ;;
      github-pr-notes) printf '%s\n' hack/demo-gh.sh ;;
      neovim-notes) find integrations/neovim -type f -name '*.lua' -print | LC_ALL=C sort ;;
    esac
    find "examples/${example}" -maxdepth 1 -type f \
      ! -name '.demo.sha256' -print | LC_ALL=C sort
    find cmd internal extras integrations -type f \
      \( -name '*.go' -o -name 'package.nix' -o -name 'provider.yaml' -o -name 'package.json' -o -name 'package-lock.json' \) \
      ! -name '*_test.go' -print | LC_ALL=C sort
  } | while IFS= read -r path; do
    if [[ $path == flake.nix ]]; then
      versionless_hash=$(sed -E '/^[[:space:]]*version = "[^"]+";[[:space:]]*$/d' "$path" | sha256sum | cut -d ' ' -f 1)
      printf '%s  %s\n' "$versionless_hash" "$path"
    else
      sha256sum "$path"
    fi
  done | sha256sum | cut -d ' ' -f 1
}

demo_is_valid() {
  local example="$1"
  local format
  local output="$repo_dir/examples/$example/demo.gif"
  [[ -s $output ]] || return 1
  format=$(ffprobe -v error -show_entries format=format_name \
    -of default=noprint_wrappers=1:nokey=1 "$output") || return 1
  [[ $format == gif ]]
}

check_demos() {
  local example
  for example in "${example_names[@]}"; do
    check_demo "$example"
  done
}

check_demo() {
  local example="$1"
  local expected
  local recorded
  expected=$(demo_fingerprint "$example")
  recorded=$(cat "$repo_dir/examples/$example/.demo.sha256" 2> /dev/null || true)
  if [[ $recorded != "$expected" ]] || ! demo_is_valid "$example"; then
    echo "ERROR: ${example} demo is stale; run nix develop -c ./hack/example-demos.sh ${example}" >&2
    return 1
  fi
}

refresh_fingerprints() {
  local example
  for example in "${example_names[@]}"; do
    if ! demo_is_valid "$example"; then
      echo "ERROR: ${example} demo is missing or invalid; regenerate it before refreshing fingerprints" >&2
      return 1
    fi
    demo_fingerprint "$example" > "$repo_dir/examples/$example/.demo.sha256"
  done
}

if [[ ${1:-} == "--check" ]]; then
  check_demos
  exit 0
fi

if [[ ${1:-} == "--refresh-fingerprints" ]]; then
  refresh_fingerprints
  check_demos
  exit 0
fi

selected=${1:-all}
if [[ $selected != all ]]; then
  known=false
  for example in "${example_names[@]}"; do
    if [[ $example == "$selected" ]]; then
      known=true
      break
    fi
  done
  if [[ $known != true ]]; then
    echo "ERROR: unknown example: ${selected}" >&2
    exit 2
  fi
fi

full_path=${CHANGES_DEMO_FULL:-$(nix build "$repo_dir#full" --no-link --print-out-paths)}
git_notes_path=${CHANGES_DEMO_GIT_NOTES:-$(nix build "$repo_dir#provider-git-notes" --no-link --print-out-paths)}
neovim_plugin_path=$(nix build "$repo_dir#neovim-plugin" --no-link --print-out-paths)
demo_root=$(mktemp -d)
trap 'rm -rf "${demo_root:?}"' EXIT

init_repository() {
  python3 "$repo_dir/hack/recipe-fixture.py" "$1"
}

init_group_repository() {
  python3 "$repo_dir/hack/recipe-fixture.py" "$1" groups
}

setup_workspace() {
  local workspace="$1"
  init_repository "$workspace/api"
  init_repository "$workspace/client"
}

render_demo() {
  local example="$1"
  local environment_root="$demo_root/$example"
  local repository="$environment_root/repository"
  local working_directory="$repository"
  local output="$environment_root/demo.gif"
  local extra_path="$full_path/bin"
  local base_sha=""
  local difftool_local
  local difftool_remote
  local head_sha=""

  printf -v difftool_local '$%s' LOCAL
  printf -v difftool_remote '$%s' REMOTE

  mkdir -p \
    "$environment_root/cache" \
    "$environment_root/config" \
    "$environment_root/data" \
    "$environment_root/data-dirs" \
    "$environment_root/home" \
    "$environment_root/state"

  case "$example" in
    patch-reader)
      git clone --quiet --no-hardlinks "$repo_dir" "$repository"
      git -C "$repository" checkout --quiet --detach 6147beb23c88864180be2cccdec9a52dd1a3a6fc
      ;;
    logical-change-groups)
      init_group_repository "$repository"
      ;;
    provider-validation)
      working_directory="$repo_dir"
      ;;
    workspace-review)
      working_directory="$environment_root/workspace"
      setup_workspace "$working_directory"
      ;;
    *)
      init_repository "$repository"
      ;;
  esac

  case "$example" in
    patch-reader)
      mkdir -p "$environment_root/config/changes"
      printf '%s\n' 'interactive:' '  reader: [diffnav]' > "$environment_root/config/changes/config.yaml"
      ;;
    custom-difftool)
      mkdir -p "$environment_root/config/changes"
      printf '%s\n' \
        'color: always' \
        'diff:' \
        '  engine: filter' \
        '  layout: unified' \
        '  filter: [delta, --paging=never]' \
        "  difftool: [difft, --color, always, --display, side-by-side, $difftool_local, $difftool_remote]" \
        > "$environment_root/config/changes/config.yaml"
      mkdir -p "$repository/.demo"
      git -C "$repository" show HEAD:internal/auth/token.go > "$repository/.demo/before.go"
      cp "$repository/internal/auth/token.go" "$repository/.demo/after.go"
      printf '%s\n' '.demo/' >> "$repository/.git/info/exclude"
      ;;
    interactive-workspace)
      python3 "$repo_dir/hack/recipe-fixture.py" "$repository" history
      mkdir -p "$environment_root/config/changes"
      printf '%s\n' 'notes:' '  store: local-notes' > "$environment_root/config/changes/config.yaml"
      ;;
    github-pr-notes)
      git -C "$repository" add internal/auth/token.go
      git -C "$repository" commit -qm 'validate bearer token boundaries'
      git -C "$repository" remote add origin https://github.com/replay-fixtures/checkout-service.git
      base_sha=$(git -C "$repository" rev-parse HEAD^)
      head_sha=$(git -C "$repository" rev-parse HEAD)
      ;;
    git-notes)
      git -C "$repository" add internal/auth/token.go
      git -C "$repository" commit -qm 'validate bearer token boundaries'
      git init -q --bare "$environment_root/origin.git"
      git -C "$repository" remote add origin "$environment_root/origin.git"
      git -C "$repository" push -q -u origin HEAD
      git clone -q "$environment_root/origin.git" "$environment_root/reviewer"
      base_sha=$(git -C "$repository" rev-parse HEAD^)
      head_sha=$(git -C "$repository" rev-parse HEAD)
      extra_path="$git_notes_path/bin:$extra_path"
      ;;
    logical-change-groups)
      mkdir -p "$environment_root/config/changes/providers/demo-groups"
      cp "$repo_dir/examples/logical-change-groups/provider" "$environment_root/config/changes/providers/demo-groups/provider"
      cp "$repo_dir/examples/logical-change-groups/provider.yaml" "$environment_root/config/changes/providers/demo-groups/provider.yaml"
      ;;
    manual-notes)
      mkdir -p "$environment_root/config/changes"
      printf '%s\n' 'notes:' '  store: local-notes' > "$environment_root/config/changes/config.yaml"
      ;;
    neovim-notes)
      mkdir -p "$environment_root/config/changes"
      printf '%s\n' 'notes:' '  store: local-notes' > "$environment_root/config/changes/config.yaml"
      mkdir -p "$environment_root/config/nvim"
      printf '%s\n' 'vim.opt.runtimepath:prepend(vim.env.DEMO_NEOVIM_PLUGIN)' \
        'require("changes.notes").setup({ provider = "local-notes" })' \
        > "$environment_root/config/nvim/init.lua"
      ;;
  esac

  (
    cd "$working_directory"
    export CHANGES_DIFF_ENGINE=builtin
    export CHANGES_DIFF_LAYOUT=unified
    if [[ $example == agent-review-notes || $example == interactive-workspace ]]; then
      export CHANGES_CODEX_COMMAND="$repo_dir/hack/demo-codex.sh"
    else
      unset CHANGES_CODEX_COMMAND
    fi
    if [[ $example == github-pr-notes ]]; then
      export CHANGES_GH_COMMAND="$repo_dir/hack/demo-gh.sh"
    else
      unset CHANGES_GH_COMMAND
    fi
    export DEMO_BASE_SHA="$base_sha"
    export DEMO_HEAD_SHA="$head_sha"
    export HOME="$environment_root/home"
    export PATH="$extra_path:$PATH"
    export XDG_CACHE_HOME="$environment_root/cache"
    export XDG_CONFIG_HOME="$environment_root/config"
    export XDG_DATA_HOME="$environment_root/data"
    if [[ $example == git-notes ]]; then
      export XDG_DATA_DIRS="$git_notes_path/share:$environment_root/data-dirs"
    else
      export XDG_DATA_DIRS="$environment_root/data-dirs"
    fi
    export XDG_STATE_HOME="$environment_root/state"
    export DEMO_NEOVIM_PLUGIN="$neovim_plugin_path"
    unset CHANGES_CONFIG CHANGES_PROVIDERS_DIRECTORY
    if [[ $example == custom-difftool ]]; then
      unset CHANGES_DIFF_ENGINE CHANGES_DIFF_LAYOUT
    fi
    vhs "$repo_dir/examples/$example/demo.tape" --output "$output"
  )

  if [[ ! -s $output ]]; then
    echo "ERROR: ${example} demo produced no output" >&2
    return 1
  fi
  install -m 0644 "$output" "$repo_dir/examples/$example/demo.gif"
  demo_fingerprint "$example" > "$repo_dir/examples/$example/.demo.sha256"
}

for example in "${example_names[@]}"; do
  if [[ $selected == all || $selected == "$example" ]]; then
    echo "Recording ${example}" >&2
    render_demo "$example"
  fi
done

if [[ $selected == all ]]; then
  check_demos
else
  check_demo "$selected"
fi
