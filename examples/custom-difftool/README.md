# Use another Git-compatible difftool

Create `~/.config/changes/config.yaml`:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/roshbhatia/changes/main/schema/changes.schema.json
color: always
diff:
  engine: command
  layout: side-by-side
  command: [difft, --color, always, --display, side-by-side, $LOCAL, $REMOTE]
providers: []
```

Then use the same driver from Git or another tool:

```bash
changes difftool before.go after.go src/handler.go
git diff | changes render
```
