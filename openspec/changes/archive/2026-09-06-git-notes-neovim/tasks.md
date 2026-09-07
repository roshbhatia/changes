## 1. Git notes provider

- **SHAPE** graph
- MERGE 1.5

- [x] 1.1 Implement canonical committed-note storage under `refs/notes/changes`. `writes:` `extras/git-notes/main.go`, `extras/git-notes/provider.yaml` `deps:` none
- [x] 1.2 Add provider unit, repository, concurrency, corruption, and no-remote tests. `writes:` `extras/git-notes/main_test.go` `deps:` 1.1
- [x] 1.3 Package the provider as an optional Nix output outside `full`. `writes:` `extras/git-notes/package.nix`, `flake.nix` `deps:` 1.1
- [x] 1.4 Document and record explicit Git notes synchronization. `writes:` `extras/README.md`, `examples/git-notes/README.md`, `examples/git-notes/demo.tape`, `examples/git-notes/demo.gif`, `examples/git-notes/.demo.sha256` `deps:` 1.2, 1.3
- [x] 1.5 Adversarial review: audit ref isolation, atomicity, concurrency, corruption handling, and remote boundaries. `writes:` `openspec/changes/git-notes-neovim/review.md` `deps:` 1.4

## 2. Neovim adapter

- **SHAPE** graph
- MERGE 2.5

- [x] 2.1 Implement the Lua command, functions, and `<Plug>` mappings. `writes:` `integrations/neovim/lua/changes/notes.lua`, `integrations/neovim/plugin/changes.lua`, `integrations/neovim/doc/changes.txt` `deps:` none
- [x] 2.2 Add headless tests for input, ranges, argv, stdin, and failures. `writes:` `integrations/neovim/tests/contract.lua` `deps:` 2.1
- [x] 2.3 Expose the adapter as a separate Nix package and check. `writes:` `integrations/neovim/package.nix`, `flake.nix` `deps:` 2.1, 2.2
- [x] 2.4 Document and record the Neovim annotation flow. `writes:` `examples/neovim-notes/README.md`, `examples/neovim-notes/demo.tape`, `examples/neovim-notes/demo.gif`, `examples/neovim-notes/.demo.sha256` `deps:` 2.3
- [x] 2.5 Adversarial review: audit shell boundaries, range behavior, cancellation, errors, and editor policy. `writes:` `openspec/changes/git-notes-neovim/review.md` `deps:` 2.4

## 3. Integration and release gate

- **SHAPE** graph
- MERGE 3.3

- [x] 3.1 Integrate documentation and demo freshness checks. `writes:` `README.md`, `examples/harness-notes/README.md`, `hack/example-demos.sh`, `flake.nix` `deps:` 1.4, 2.4
- [x] 3.2 Run deterministic tests, generated checks, demos, and the full flake check. `writes:` none `deps:` 3.1
- [x] 3.3 Run adversarial review, inspect the complete diff, and resolve accepted findings. `writes:` `openspec/changes/git-notes-neovim/design.md`, `openspec/changes/git-notes-neovim/tasks.md` `deps:` 3.2
