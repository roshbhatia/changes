# Add manual notes from a terminal or editor

[![Manual notes demo](demo.gif)](demo.tape)

Run `note add` without message flags to open `notes.editor`, then `$VISUAL`,
then `$EDITOR`, then `vi`:

```bash
changes note add --file cmd/server.go --line 117
```

Configure an argv-safe editor command in `~/.config/changes/config.yaml`:

```yaml
notes:
  editor: [nvim, $FILE]
```

If the command omits `$FILE`, Changes appends the temporary note path. The
first non-comment line is the summary. Later text is the rationale. Empty or
comment-only text cancels without writing.

Use a headless command from Neovim, another terminal, or a script:

```bash
changes note add \
  --file internal/router.go \
  --line 63 \
  --side right \
  --message "This branch matches the legacy fallback"
```

For a deleted line, use `--side left`. For a file-level note, omit `--line`.
Use `changes note list --json` when an editor plugin needs structured output.

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh manual-notes
```
