# Use another Git-compatible difftool

Install the two display tools, then create `~/.config/changes/config.yaml`:

```bash
nix profile install nixpkgs#git-delta nixpkgs#difftastic
```

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/roshbhatia/changes/main/schema/changes.schema.json
color: always
diff:
  engine: filter
  layout: unified
  filter: [delta, --paging=never]
  difftool: [difft, --color, always, --display, side-by-side, $LOCAL, $REMOTE]
```

Then use the same driver from Git or another tool:

```bash
changes difftool before.go after.go src/handler.go
git diff | changes render
```

`delta` reads a unified patch from standard input. `difft` accepts two file
paths. The commands use separate configuration because their argument
contracts differ.
