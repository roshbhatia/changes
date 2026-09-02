# Extras

These commands implement optional integrations outside the Changes core.

- `changes-provider-ast-grep` advertises `symbols`.
- `changes-provider-calldiff` advertises `calls`.

Each provider reads one JSON request from standard input and writes one JSON
response. Configure commands and advertised capabilities in YAML. Changes can
therefore use a replacement without importing its SDK or package.
