# Review changes interactively

[![Interactive workspace demo](demo.gif)](demo.tape)

Open one workspace for the cohesive diff, logical groups, note threads, files,
and commit history:

```bash
changes interactive
```

Use `f` to switch Files and History. Use `t` for tree or list, `d` for a left
or bottom navigator, `s` for unified or side-by-side diff, and `v` for working,
staged, or first-parent commit comparisons. Press `:` for commands such as
`layout side-by-side`, `dock bottom`, `view staged`, and `refresh`.

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

Replay the [VHS tape](demo.tape) from the repository root:

```bash
nix develop -c ./hack/example-demos.sh interactive-workspace
```
