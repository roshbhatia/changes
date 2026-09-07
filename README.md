# changes

![Changes diff view](docs/changes.png)

![Changes animated diff review](docs/changes.gif)

`changes` reads Git changes as a repository tree. A grouping provider can put
related hunks into an ordered logical flow before the tree nests hunks under
their symbols, annotates changed calls, and places notes under anchored lines.

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
nix profile install github:roshbhatia/changes#provider-codex-review
nix profile install github:roshbhatia/changes#provider-local-notes
nix profile install github:roshbhatia/changes#provider-github-pr
nix profile install github:roshbhatia/changes#provider-git-notes
```

Each provider package exposes only its `changes-provider-*` adapter and
manifest. Its runtime tools stay private to the adapter, so these packages do
not replace profile commands such as `git` or `gh`.

Install core and the reference providers that are safe to compose as one
self-contained package:

```bash
nix profile install github:roshbhatia/changes#full
```

`full` excludes `git-notes` because both it and `local-notes` can write notes.
Install `provider-git-notes` separately and select it explicitly for committed
writes.

The flake also exports `neovim-plugin` for Nix-managed Neovim configurations:

```nix
programs.neovim.plugins = [ inputs.changes.packages.${pkgs.system}.neovim-plugin ];
```

Install a release and its shell completions with Homebrew:

```bash
brew install roshbhatia/tap/changes
```

`go install github.com/roshbhatia/changes/cmd/changes@latest` installs the core
only. Git must be on `PATH`.

## Use it

```bash
# Review the current repository as an embedded diff tree.
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
workflows. Note workflows cover
[`harness-authored notes`](examples/harness-notes/README.md),
[`post-hoc agent review`](examples/agent-review-notes/README.md),
[`GitHub PR review`](examples/github-pr-notes/README.md), and
[`manual notes`](examples/manual-notes/README.md). Provider authors can use
[`examples/provider-validation`](examples/provider-validation/README.md).
[`Logical change groups`](examples/logical-change-groups/README.md) order
related hunks by request flow instead of file name. See
[`Git notes storage`](examples/git-notes/README.md) for explicit ref sharing and
[`Neovim notes`](examples/neovim-notes/README.md) for popup annotations.

## Configure it

Changes loads `~/.config/changes/config.yaml`. Set `CHANGES_CONFIG` to use a
different path. Any setting can be overridden with a nested environment name,
such as `CHANGES_DIFF_LAYOUT=side-by-side`.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/roshbhatia/changes/main/schema/changes.schema.json
color: auto
diff:
  engine: builtin
  layout: unified
  difftool: [difft, --color, always, --display, side-by-side, $LOCAL, $REMOTE]
notes:
  editor: [nvim, $FILE]
  generatorTimeout: 5m
  refreshInterval: 30s
providers:
  cacheMaxEntries: 256
  cacheTtl: 1h
  group: my-group-provider
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
directly and never inserts a shell. The core knows the semantic actions
`changes.groups`, `changes.symbols`, `changes.calls`, `changes.notes`,
`changes.notes.create`, and `changes.notes.generate`.

A grouping provider returns an ordered tree plus file or line anchors. Changes
assigns each hunk to the first matching group and keeps unmatched hunks under
`other changes`. Select one with `providers.group` or `--group-provider`;
discovery priority chooses the default. Logical grouping requires the
`builtin` display engine because filter output has no stable hunk structure.

Group, symbol, and call results use the provider cache. Note reads, writes, and
generation are never cached. Watch mode polls readers at
`notes.refreshInterval`. It never runs a note generator. One `--budget` covers
all provider analysis for the rendered comparison.

`changes.notes.create` accepts either one `note` or one atomic `notes` batch.
`changes note generate` writes every generated note as one batch and uses the
generator note ID as its idempotency key. A generator must return the same ID
for the same semantic note on an equivalent request. A store retry must return
the existing note for the same key and payload, and must reject a changed
payload for that key.
Provider requests and responses are each limited to 16 MiB. Input patches are
limited to 64 MiB.

Provider results expire after `providers.cacheTtl`. Changes keeps at most
`providers.cacheMaxEntries` persistent results. Set either value to zero to
disable the provider cache. Relative executables and static script arguments
resolve from the directory that contains the provider manifest.

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
temporary repository, runs every advertised Changes action, and checks its
semantic output. A writable note provider writes only inside this fixture.

Generate the schema and command reference with `changes generate`. CI uses
`changes generate --check` to reject stale output.

## Command reference
<!-- BEGIN GENERATED:cli -->

### `changes`

Render Git changes with logical groups, symbols, calls, and notes

Refs follow git diff: none is the index against the working tree, one is that ref
against the working tree, and two compare the trees. A from of the form a..b is
split into two refs.

-r reads every repository under the workspace. Use -root to select its
boundary. Without -root, Changes uses the Git top level, then the working
directory. Each repository's files hang under its own name.

| Option | Description |
| --- | --- |
| `--budget` `<value>` | Analysis time budget |
| `--color` `<value>` | Color output |
| `--config` `<value>` | YAML configuration file |
| `--engine` `<value>` | Patch display engine |
| `--filter` `<value>` | Standard-input patch filter |
| `--group-provider` `<value>` | Logical change-group provider |
| `--interval` `<value>` | Watch interval |
| `--layout` `<value>` | Diff layout |
| `--no-calls` | Skip call analysis |
| `--no-groups` | Skip logical change grouping |
| `--no-notes` | Skip diff notes |
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

### `changes note`

Create and inspect diff notes

### `changes note add`

Create a note on the selected diff

| Option | Description |
| --- | --- |
| `--author` `<value>` | Note author |
| `--commit` `<value>` | First-parent commit comparison |
| `--config` `<value>` | YAML configuration file |
| `--expected-file-sha256` `<value>` | Require the selected file side to match this SHA-256 digest |
| `--file` `<value>` | Repository file to annotate |
| `--from` `<value>` | Left revision |
| `--json` | Print the created note as JSON |
| `--line` `<value>` | Last line of the note range |
| `--message` `<value>` | Summary and optional rationale |
| `--message-file` `<value>` | Read note text from a file or standard input |
| `--origin` `<value>` | Author kind |
| `--provider` `<value>` | Writable note provider |
| `--session` `<value>` | Harness session identifier |
| `--side` `<value>` | Diff side |
| `--staged` | Compare the index |
| `--start-line` `<value>` | First line of a multi-line range |
| `--to` `<value>` | Right revision |

### `changes note generate`

Generate notes with a provider and save them

| Option | Description |
| --- | --- |
| `--commit` `<value>` | First-parent commit comparison |
| `--config` `<value>` | YAML configuration file |
| `--from` `<value>` | Left revision |
| `--json` | Print generated notes as JSON |
| `--provider` `<value>` | Note generator provider |
| `--session` `<value>` | Harness session identifier |
| `--staged` | Compare the index |
| `--store` `<value>` | Writable note provider |
| `--to` `<value>` | Right revision |

### `changes note list`

List notes on the selected diff

| Option | Description |
| --- | --- |
| `--commit` `<value>` | First-parent commit comparison |
| `--config` `<value>` | YAML configuration file |
| `--from` `<value>` | Left revision |
| `--json` | Print JSON |
| `--provider` `<value>` | Note provider |
| `--staged` | Compare the index |
| `--to` `<value>` | Right revision |

### `changes provider`

Inspect and validate context providers

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
