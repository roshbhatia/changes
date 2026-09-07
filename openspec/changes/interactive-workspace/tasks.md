## 1. Contract and comparison model

- **SHAPE** graph
- MERGE 1.4

- [x] 1.1 Add workspace snapshot, event, hunk-line, history, freshness, failure, and provenance types. `writes:` `internal/workspaceview/**`, `internal/provider/note.go` `deps:` none
- [x] 1.2 Add NUL-safe Git history and working, staged, first-parent commit comparison helpers. `writes:` `internal/source/git.go`, `internal/source/git_test.go` `deps:` none
- [x] 1.3 Generate and validate the workspace JSON schema and command metadata. `writes:` `cmd/changes/main.go`, `schema/workspace.schema.json`, `README.md`, `completions/**` `deps:` 1.1
- [x] 1.4 Extend reference providers and cohesive rendering with provenance and nested threads. `writes:` `extras/**`, `cmd/changes/main.go`, `internal/provider/**` `deps:` 1.1
- [x] 1.5 Adversarial review: skipped by explicit owner direction; no critic or mediator runs. `writes:` `openspec/changes/interactive-workspace/review.md` `deps:` none

## 2. Snapshot cache and state

- **SHAPE** graph
- MERGE 2.3

- [x] 2.1 Build snapshots through the existing renderer and expose `changes workspace`. `writes:` `cmd/changes/workspace.go`, `cmd/changes/main.go` `deps:` 1.1, 1.2, 1.4
- [x] 2.2 Add atomic, bounded, symlink-safe XDG snapshot cache and per-repository state stores. `writes:` `internal/workspaceview/**` `deps:` 1.1
- [x] 2.3 Add cached startup, `--refresh`, JSON Lines watch events, last-good notes, and focused regressions. `writes:` `cmd/changes/workspace.go`, `cmd/changes/workspace_test.go`, `internal/workspaceview/**` `deps:` 2.1, 2.2
- [x] 2.4 Adversarial review: skipped by explicit owner direction; deterministic cache boundary tests remain required. `writes:` `openspec/changes/interactive-workspace/review.md` `deps:` none

## 3. Interactive terminal

- **SHAPE** graph
- MERGE 3.4

- [x] 3.1 Add the Bubble Tea workspace frame, cohesive diff viewport, responsive left and bottom navigator docks, and focus handling. `writes:` `cmd/changes/interactive.go`, `cmd/changes/interactive_test.go`, `go.mod`, `go.sum` `deps:` 2.3
- [x] 3.2 Add Files and History tabs, tree and list forms, Git view switching, file filtering, and unified or side-by-side toggling. `writes:` `cmd/changes/interactive.go`, `cmd/changes/interactive_test.go` `deps:` 3.1
- [x] 3.3 Add `:` commands, completion, help, repository state restoration, and exact selection persistence. `writes:` `cmd/changes/interactive.go`, `cmd/changes/interactive_test.go` `deps:` 3.2
- [x] 3.4 Add popup and editor note authoring, refresh, failure display, and note indicators. `writes:` `cmd/changes/interactive.go`, `cmd/changes/interactive_test.go` `deps:` 3.3
- [x] 3.5 Adversarial review: skipped by explicit owner direction; interactive user-flow tests remain required. `writes:` `openspec/changes/interactive-workspace/review.md` `deps:` none

## 4. Progress and external client proof

- **SHAPE** graph
- MERGE 4.3

- [x] 4.1 Add TTY-only non-difftool progress with `--quiet` and clean shutdown. `writes:` `internal/progress/**`, `cmd/changes/main.go`, `cmd/changes/workspace.go` `deps:` 2.1
- [x] 4.2 Add a shell-free Neovim workspace reader that can place note extmarks from structured line identities. `writes:` `integrations/neovim/**` `deps:` 2.3
- [x] 4.3 Add headless and pseudoterminal tests for the external contract, terminal restoration, and clean output boundaries. `writes:` `integrations/neovim/tests/**`, `cmd/changes/**_test.go`, `flake.nix` `deps:` 3.4, 4.1, 4.2
- [x] 4.4 Adversarial review: skipped by explicit owner direction; external contract tests remain required. `writes:` `openspec/changes/interactive-workspace/review.md` `deps:` none

## 5. Documentation, media, and release

- **SHAPE** graph
- MERGE 5.4

- [x] 5.1 Add the interactive workflow README and VHS tape. `writes:` `examples/interactive-workspace/README.md`, `examples/interactive-workspace/demo.tape` `deps:` 4.3
- [x] 5.2 Record the GIF and integrate media freshness, release contents, and root documentation. `writes:` `examples/interactive-workspace/demo.gif`, `examples/interactive-workspace/.demo.sha256`, `hack/**`, `README.md`, `flake.nix` `deps:` 5.1
- [x] 5.3 Run focused tests, race tests, generation checks, demo checks, the complete diff inspection, calldiff, and `nix flake check -L`. `writes:` none `deps:` 5.2
- [x] 5.4 Commit, push, tag the next minor version, verify native release jobs, download every asset, check checksums and archive contents, and notify the active sysinit Codex session. `writes:` `flake.nix`, `openspec/**` `deps:` 5.3
- [x] 5.5 Adversarial review: skipped by explicit owner direction; release verification remains required. `writes:` `openspec/changes/interactive-workspace/review.md` `deps:` none
