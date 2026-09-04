# changes

![Changes diff view](docs/changes.png)

![Changes animated diff review](docs/changes.gif)

`changes` reads Git changes as a repository tree. It groups edits under
symbols and annotates them with changed call edges.

The core depends only on Git. Optional analysis and display tools run through
external command contracts. The `extras/` directory owns every reference
provider and its runtime dependencies. Provider responses are cached by the
manifest, resolved provider and runtime executables, action, and patch
fingerprint under the user cache directory.

## Install it

Install the provider-free core, then add only the providers you use:

```bash
nix profile install github:roshbhatia/changes#changes
nix profile install github:roshbhatia/changes#provider-ast-grep
nix profile install github:roshbhatia/changes#provider-calldiff
```

Install core and every reference provider as one self-contained package:

```bash
nix profile install github:roshbhatia/changes#full
```

Install a release and its shell completions with Homebrew:

```bash
brew install --cask roshbhatia/tap/changes
```

`go install github.com/roshbhatia/changes/cmd/changes@latest` installs the core
only. Git must be on `PATH`.

## Use it

```bash
# Review the current repository with Git's inline diff.
changes

# Review all repositories in a workspace since two hours ago.
changes --recursive --since "2 hours ago"

# Use it anywhere Git accepts a difftool.
git -c diff.tool=changes \
  -c difftool.changes.cmd='changes difftool "$LOCAL" "$REMOTE" "$MERGED"' \
  -c difftool.prompt=false difftool
```

See [`examples/workspace-review`](examples/workspace-review/README.md) and
[`examples/custom-difftool`](examples/custom-difftool/README.md) for complete
workflows. Provider authors can use
[`examples/provider-validation`](examples/provider-validation/README.md).

## Configure it

Changes loads `~/.config/changes/config.yaml`. Set `CHANGES_CONFIG` to use a
different path. Any setting can be overridden with a nested environment name,
such as `CHANGES_DIFF_LAYOUT=side-by-side`.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/roshbhatia/changes/main/schema/changes.schema.json
color: auto
diff:
  engine: filter
  layout: unified
  filter: [delta, --paging=never]
  difftool: [difft, --color, always, --display, side-by-side, $LOCAL, $REMOTE]
providers:
  timeout: 20s
```

The filter display engine sends one patch to the configured command's standard
input. A filter must not contain Git file placeholders. During `changes
difftool`, the separate `diff.difftool` command expands `$LOCAL`, `$REMOTE`, and
`$MERGED`. If it has no local or remote placeholder, Changes appends both file
paths.

For configuration migration, replace the old `git` engine with `builtin`.
Replace `internal` with `builtin` and select its layout. Replace the old
`command` engine with `filter`, and rename `diff.command` to `diff.filter`.
Configure `diff.difftool` separately only when `changes difftool` should
launch another file comparison command.

Changes loads user manifests from `~/.config/changes/providers`, then
`$XDG_DATA_HOME/changes/providers`, executable-adjacent package data, and each
`XDG_DATA_DIRS/changes/providers` directory. Flat YAML manifest files remain
valid in a user directory. Packaged provider directories use a file named
`provider.yaml`, `provider.yml`, or `provider.json`. The first manifest with a
given name wins. Set
`providers.directory` or `CHANGES_PROVIDERS_DIRECTORY` to replace the first
configuration directory.

Each provider uses the shared `provider/v1` manifest. Actions add arguments and
environment values through Go templates. Changes executes the resulting argv
directly and never inserts a shell. The core only knows the semantic actions
`changes.symbols` and `changes.calls`.

See [`extras/README.md`](extras/README.md) for concrete manifests and package
definitions. Each provider directory owns its executable adapter, manifest,
runtime dependencies, and isolated validation.

Inspect and test discovered providers without rendering the current repository:

```bash
changes provider list
changes provider validate
changes provider validate provider-name
```

Validation checks each manifest and host dependency. It then creates a
temporary repository, runs every advertised Changes action, and checks the
returned symbol or call data.

Generate the schema and command reference with `changes generate`. CI uses
`changes generate --check` to reject stale output.

## Command reference
<!-- BEGIN GENERATED:cli -->

### `changes`

Render Git changes with symbol and call analysis

Refs follow git diff: none is HEAD against the working tree, one is that ref
against the working tree, and two compare the trees. A from of the form a..b is
split into two refs.

-r reads every repository under the workspace. The workspace is
$SYSINIT_WORKSPACE when the working directory sits inside it, then the Git top
level, then the working directory. Each repository's files hang under its own
name.

| Option | Description |
| --- | --- |
| `--budget` `<value>` | Analysis time budget |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | Patch display engine |
| `--filter` `<value>` | Standard-input patch filter |
| `--interval` `<value>` | Watch interval |
| `--layout` `<value>` | Diff layout |
| `--no-calls` | Skip call analysis |
| `--no-symbols` | Skip symbol analysis |
| `--recursive`, `-r` | Read all workspace repositories |
| `--root` `<value>` | Workspace scan root |
| `--since` `<value>` | Left revision or time |
| `--staged` | Compare the index |
| `--stat`, `-s` | Show change summary |
| `--watch`, `-w` | Watch for changes |
| `--width` `<value>` | Render width |
| `--version` | Print the Changes version |

### `changes completion`

Generate shell completions

### `changes difftool`

Compare Git difftool LOCAL and REMOTE files

| Option | Description |
| --- | --- |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | File comparison engine |
| `--difftool` `<value>` | Git-compatible difftool executable |
| `--layout` `<value>` | Diff layout |
| `--width` `<value>` | Render width |

### `changes render`

Render a patch from standard input

| Option | Description |
| --- | --- |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | Patch display engine |
| `--filter` `<value>` | Standard-input patch filter |
| `--layout` `<value>` | Diff layout |
| `--width` `<value>` | Render width |

### `changes generate`

Generate README command docs and JSON Schema

| Option | Description |
| --- | --- |
| `--check` | Fail when generated files are stale |

### `changes provider`

Inspect and validate analysis providers

### `changes provider list`

List configured analysis providers

| Option | Description |
| --- | --- |
| `--config` `<value>` | YAML configuration file |
| `--json` | Print JSON |

### `changes provider validate`

Validate provider commands and JSON behavior

| Option | Description |
| --- | --- |
| `--config` `<value>` | YAML configuration file |
| `--json` | Print JSON |

<!-- END GENERATED:cli -->

## Development

```bash
nix develop
go test -race ./...
go run ./cmd/changes generate --check
./hack/audit-provider-boundary.sh .
for manifest in extras/*/provider.yaml; do
  cue vet schema/provider.cue "$manifest" -d '#Provider'
done
nix flake check
./hack/screenshots.sh
./hack/screenshots.sh --check
```
