## Why

The one-shot tree places context and notes beside each change, but it cannot retain navigation, switch Git views, refresh live review threads, or support an editor UI without parsing terminal text.

## What Changes

- Add `changes interactive` with a cohesive diff viewport, file and history tabs, tree and list navigation, left and bottom docking, unified and side-by-side layouts, note authoring, and background refresh.
- Add `changes workspace` with a versioned JSON snapshot and JSON Lines watch stream for Neovim and other clients.
- Add optional note provenance for the producing tool, session, working directory, and source URL. Render complete note threads at their diff placement.
- Persist per-repository view state under XDG state storage and replaceable snapshots under XDG cache storage.
- Add TTY-only progress for non-difftool work, generated schemas, examples, recordings, and release checks.

## Behavior

- Interactive mode opens the current comparison as one logical tree. Notes and their context stay under the file, symbol, hunk, or line they explain.
- The navigator can show files as a tree or list, show commit history in a tab, and dock left or bottom. A user can switch the active Git view and diff layout without restarting.
- The UI restores its last layout for the repository. It may show a cached snapshot immediately, but it marks that snapshot until a background refresh confirms current data.
- GitHub or another live provider refreshes at the configured note interval. A failed refresh keeps last-good notes visible and marks them stale.
- `--refresh` bypasses the initial snapshot and provider result caches without fetching, pushing, or changing repository data.
- Popup and editor note input both call the existing note writer contract. Cancellation or failure creates no success state.
- `changes workspace` writes only versioned machine data to standard output. It includes repository and comparison identities, freshness, history, logical groups, files, hunks, notes, threads, provenance, and the cohesive rendered document.
- Non-difftool commands show progress only on a terminal error stream. Pipes, machine output, and `changes difftool` remain clean.

### Non-goals

- Stage, discard, commit, rebase, resolve a GitHub thread, or perform another Git mutation.
- Automatically synchronize `refs/notes/changes`.
- Replace provider-owned note generation or storage with UI-owned data.
- Make a cached snapshot indistinguishable from a confirmed fresh result.

## Capabilities

### New Capabilities

- `interactive-workspace`: Navigate and annotate cohesive Git comparisons in a persistent terminal workspace.
- `workspace-snapshot-contract`: Expose the same workspace as versioned JSON and JSON Lines events.
- `progress-feedback`: Show bounded TTY progress without contaminating data output or difftool use.

### Modified Capabilities

- `diff-notes`: Add optional provenance, complete thread presentation, and last-good interactive refresh behavior.
- `configuration-and-generated-interfaces`: Add interactive configuration and a generated workspace schema.

## Impact

- Adds Bubble Tea and Bubbles to the core package dependency set.
- Adds workspace, history, state, cache, progress, and interactive UI code.
- Extends provider note objects without changing required provider/v1 fields.
- Adds a permanent `examples/interactive-workspace` workflow with a VHS tape and GIF.
- Advances the minor release because it adds commands and a public machine contract.
