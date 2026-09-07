## Why

Local note storage does not provide a Git-native way to carry committed review context between clones. Neovim users also need a stable adapter contract that creates notes without shell quoting or editor-specific core code.

## What Changes

- Add a provider that reads and writes Changes note documents under `refs/notes/changes` for committed comparisons.
- Keep every fetch, merge, and push of the notes ref explicit and user-controlled.
- Add a Neovim Lua adapter that collects a cursor or visual range through `vim.ui.input` and invokes `changes note add --json` with an argument vector.
- Add provider packages, validation, automated tests, and permanent examples with VHS recordings.

## Behavior

- A committed note written through `git-notes` appears in the same committed comparison after a new process reads `refs/notes/changes`.
- A normal read or write never contacts a remote. Only documented Git commands synchronize the ref.
- `:ChangesNote` sends the current cursor or visual line range and popup text to `changes note add` without a shell.
- Cancelling the popup writes nothing. CLI, JSON, or process failures remain visible in Neovim.

### Non-goals

- Store working-tree or index notes in Git notes.
- Configure notes fetch, push, display, merge, or rewrite policy for the repository.
- Choose a Neovim user keybinding or implement a private popup UI.

## Capabilities

### New Capabilities

- `git-notes-store`: Store committed diff notes in an isolated Git notes ref without implicit remote operations.
- `neovim-note-adapter`: Define the Lua function, command, range, input, process, and result contract for Neovim annotation.

### Modified Capabilities

None.

## Impact

- Adds `extras/git-notes`, its provider manifest, and a Nix package.
- Adds a reusable Lua module and example under `examples/neovim-notes`.
- Extends provider aggregation, validation, release checks, README guidance, and demo freshness checks.
- Requires Git for storage and Neovim 0.10 or newer for `vim.system`.
