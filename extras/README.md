# Extras

These commands implement optional integrations outside the Changes core.

Each provider directory owns four parts: its manifest, adapter program,
runtime dependency package, and validation contract. The root flake discovers
directories that contain `package.nix`. It exports each one as
`provider-<name>` without adding it to the default package closure, and creates
an isolated check for every discovered package.

- `ast-grep/provider.yaml` advertises `changes.symbols` and runs
  `changes-provider-ast-grep`.
- `calldiff/provider.yaml` advertises `changes.calls` and runs
  `changes-provider-calldiff`.

Each adapter reads one JSON request from standard input and writes one JSON
response. The manifest follows `provider/v1`, which Changes validates against
`schema/provider.cue` and `schema/provider.schema.json`. A provider can be
written in any language because the manifest only defines executable argv,
environment templates, requirements, and actions.

Install a manifest under `~/.config/changes/providers` for a configuration
override, under `$XDG_DATA_HOME/changes/providers/<name>/provider.yaml` for a
user data installation, or under an `XDG_DATA_DIRS` entry with its executable
package. Changes core does not contain an ast-grep or calldiff integration.

Run `changes provider validate` to exercise every configured provider against
a synthetic working tree. This does not read or change the current repository.
