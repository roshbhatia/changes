# Review changes interactively

[![Interactive workspace demo](demo.gif)](demo.tape)

Open one workspace for the cohesive diff, logical groups, note threads, files,
and commit history:

```bash
changes interactive
```

Use `Ctrl-h/j/k/l` to move to the pane in that screen direction. `Tab` and
`Shift-Tab` switch tabs inside the focused pane. They never move pane focus.
Click a pane, Files or History, or a visible row to focus or select it. The
mouse wheel scrolls the pane under the pointer.

Use `t` for tree or list, `d` for a left or bottom navigator, `s` for unified
or side-by-side diff, and `v` for working, staged, or first-parent commit
comparisons. Press `:` for commands such as `layout side-by-side`, `dock
bottom`, `view staged`, and `refresh`.

In History, press Space to mark any number of commits without moving the
cursor. Press `g` to generate notes for the marked commits, or for the commit
under the cursor when none are marked. Changes keeps each commit as its own
first-parent comparison. The review screen groups drafts by commit and file.
Use `j/k` to move, Space to include or exclude, `e` to edit, Enter to save the
included drafts, or Escape to cancel without writing.

Automation can request the same validated drafts without a writer:

```bash
changes note generate \
  --provider codex-review \
  --commit HEAD~1 \
  --commit HEAD \
  --draft \
  --json | jq '.comparisons[] | {commit, notes}'
```

Press `a` to use the configured annotation input. `n` always opens the popup.
`e` releases the terminal and opens `$EDITOR`. Both paths call `changes note
add`, so the selected writable provider remains the storage authority.

Changes restores this repository's layout and selection from `XDG_STATE_HOME`.
It can show a marked cached snapshot while a fresh render runs. Press `r`, use
`:refresh`, or pass `--refresh` to bypass cached workspace and provider data.
Refresh never fetches a remote or pushes `refs/notes/changes`.

External clients use the same data without parsing terminal output:

```bash
changes workspace --refresh >workspace.json
changes workspace --watch | jq --unbuffered '.type'
```

The Neovim plugin exposes `:ChangesWorkspace`, `:ChangesWorkspace!`, and
`:ChangesWorkspaceDecorate`. It invokes the command through `vim.system()` and
places extmarks from structured line identities.

The demo runs the real `changes` binary. It opens History, selects two commits,
reviews generated drafts, excludes one, and saves the remaining notes. The
pseudoterminal suite separately proves mouse capture, encoded click and wheel
handling, and terminal restoration.

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh interactive-workspace
```
