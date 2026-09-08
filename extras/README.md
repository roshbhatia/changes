# Extras

These commands implement optional integrations outside the Changes core.

Providers may also advertise `changes.groups` and return an ordered
parent-child tree with file or line anchors. See the complete
[`logical-change-groups`](../examples/logical-change-groups/README.md) example.

Each provider directory owns four parts: its manifest, adapter program,
runtime dependency package, and validation contract. The root flake discovers
directories that contain `package.nix`. It exports each one as
`provider-<name>` without adding it to the default package closure, and creates
an isolated check for every discovered package. Providers join `full` unless
their package sets `includeInFull = false`. A package wraps its runtime tools
into the adapter path. It does not expose those tools as profile commands.

- `ast-grep/provider.yaml` advertises `changes.symbols` and runs
  `changes-provider-ast-grep`.
- `calldiff/provider.yaml` advertises `changes.calls` and runs
  `changes-provider-calldiff`.
- `codex-review/provider.yaml` advertises `changes.notes.generate`. It runs
  Codex only after an explicit `changes note generate` command. It uses an
  isolated read-only permission profile, disables model-command network, and
  ignores user configuration, execution rules, and repository instructions.
- `local-notes/provider.yaml` advertises `changes.notes` and
  `changes.notes.create`. It accepts one note or one atomic batch, and uses
  supplied note keys for idempotent retries. It stores local note records in
  the user's XDG state directory and never contacts a remote.
- `github-pr/provider.yaml` advertises `changes.notes`. It reads pull request
  review threads through the authenticated `gh` command and never changes the
  pull request.
- `git-notes/provider.yaml` advertises `changes.notes` and
  `changes.notes.create` for committed comparisons. It stores canonical records
  under `refs/notes/changes` and never fetches, merges, pushes, or changes Git
  configuration. Its package stays outside `full` to avoid two default writers.

Each adapter reads one JSON request from standard input and writes one JSON
response. The manifest follows `provider/v1`, which Changes validates against
the provider-spec contract plus `schema/narrow.cue`. A provider can be
written in any language because the manifest only defines executable argv,
environment templates, requirements, and actions.

Install a manifest under `~/.config/changes/providers` for a configuration
override, under `$XDG_DATA_HOME/changes/providers/<name>/provider.yaml` for a
user data installation, or under an `XDG_DATA_DIRS` entry with its executable
package. Changes core does not contain an ast-grep or calldiff integration.

Run `changes provider validate` to exercise every configured provider against
a synthetic working tree. This does not read or change the current repository.
The GitHub and Codex providers return synthetic validation notes without
network access.

Share the Git notes ref only when you choose to change the remote repository:

```bash
git fetch origin refs/notes/changes:refs/notes/changes-incoming
git notes --ref=refs/notes/changes merge \
  -s cat_sort_uniq refs/notes/changes-incoming
git push origin refs/notes/changes
```

A normal branch fetch or push does not include this ref. Resolve any conflicting
stable note keys before using the provider again. Changes does not configure
notes display or rewrite behavior.
