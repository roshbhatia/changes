# changes

`changes` renders Git changes as a repository tree. It groups hunks under
symbols and annotates them with changed call edges.

Install `git`, `ast-grep`, and `calldiff` to enable every layer.

## Development

```bash
nix develop
go test -race ./...
nix flake check
```
