# changes

![Changes diff view](docs/changes.png)

![Changes animated diff review](docs/changes.gif)

`changes` reads Git changes as a repository tree. It groups edits under
symbols and annotates them with changed call edges.

The core depends only on Git. Optional analysis and display tools run through
external command contracts. The `extras/` directory contains reference
providers for ast-grep and calldiff. Provider responses are cached by manifest,
action, and patch fingerprint under the user cache directory.

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
  engine: command
  layout: unified
  command: [delta, --paging=never]
providers:
  timeout: 20s
```

The command display engine reads a patch from standard input. During
`changes difftool`, `$LOCAL`, `$REMOTE`, and `$MERGED` arguments use Git's
difftool convention. If no file placeholders exist, Changes appends the local
and remote paths.

Changes loads user manifests from `~/.config/changes/providers`, then installed
manifests from `XDG_DATA_DIRS/changes/providers/<name>/provider.yaml`. Flat
manifest files remain valid in a user directory. A user manifest overrides an
installed manifest with the same name. Set `providers.directory` or
`CHANGES_PROVIDERS_DIRECTORY` to use one explicit user directory.

Each provider uses the shared `provider/v1` manifest. Actions add arguments and
environment values through Go templates. Changes executes the resulting argv
directly and never inserts a shell. The core only knows the semantic actions
`changes.symbols` and `changes.calls`.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/roshbhatia/changes/main/schema/provider.schema.json
version: provider/v1
name: ast-grep
description: Map changed lines to source symbols with ast-grep
command: [changes-provider-ast-grep]
actions:
  changes.symbols:
    description: Map changed lines to their enclosing source symbols
requires:
  commands: [ast-grep]
defaults:
  timeout: 20s
  priority: 100
```

Inspect and test discovered providers without rendering the current repository:

```bash
changes provider list
changes provider validate
changes provider validate ast-grep
```

Validation checks each manifest and host dependency. It then creates a temporary
Git repository, runs every advertised Changes action, and checks the returned
symbol or call data.

Generate the schema and command reference with `changes generate`. CI uses
`changes generate --check` to reject stale output.

## Command reference
<!-- BEGIN GENERATED:cli -->

### `changes`

Render Git changes with symbol and call analysis

| Option | Description |
| --- | --- |
| `--budget` `<value>` | Analysis time budget |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | Diff display engine |
| `--engine-command` `<value>` | Command display executable |
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

### `changes difftool`

Compare Git difftool LOCAL and REMOTE files

| Option | Description |
| --- | --- |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | Diff display engine |
| `--engine-command` `<value>` | Command display executable |
| `--layout` `<value>` | Diff layout |
| `--width` `<value>` | Render width |

### `changes render`

Render a patch from standard input

| Option | Description |
| --- | --- |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | Diff display engine |
| `--engine-command` `<value>` | Command display executable |
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
go run . generate --check
cue vet schema/provider.cue extras/ast-grep/provider.yaml -d '#Provider'
nix flake check
./hack/screenshots.sh
```
