# Extras

These commands implement optional integrations outside the Changes core.

- `ast-grep/provider.yaml` advertises `changes.symbols` and runs
  `changes-provider-ast-grep`.
- `calldiff/provider.yaml` advertises `changes.calls` and runs
  `changes-provider-calldiff`.

Each provider reads one JSON request from standard input and writes one JSON
response. The manifest follows `provider/v1`, which Changes validates against
`schema/provider.cue` and `schema/provider.schema.json`. A provider can be
written in any language because the manifest only defines executable argv,
environment templates, requirements, and actions.

Install a manifest under `~/.config/changes/providers` for a user override, or
under `$XDG_DATA_DIRS/changes/providers/<name>/provider.yaml` with its
executable package. Changes does not contain an ast-grep or calldiff
integration.

Run `changes provider validate` to exercise every configured provider against
a synthetic working tree. This does not read or change the current repository.
