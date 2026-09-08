## 1. Interactive input contract

- [x] 1.1 Replace pane-switching `Tab` with pane-local `Tab` and `Shift-Tab`, and add spatial `Ctrl-h/j/k/l` focus movement for both dock layouts.
- [x] 1.2 Add mouse hit regions for pane focus, Files and History tabs, visible rows, wheel scrolling, and help scrolling.
- [x] 1.3 Add model and pseudoterminal tests that prove mouse capture, restoration, encoded mouse events, spatial focus, and pane-local tabs.

## 2. Commit selection and note generation

- [x] 2.1 Add canonical multi-commit marking to History with keyboard and mouse controls, visible markers, bounded selection, and repository state restoration.
- [x] 2.2 Refactor note generation into reusable generate, validate, and per-comparison write steps without changing provider actions.
- [x] 2.3 Add repeated commit selection and non-writing JSON draft output to `changes note generate` with generated command metadata and completions.
- [x] 2.4 Add the interactive draft review flow with commit grouping, include or exclude, editing, confirmation, cancellation, progress, and per-commit results.

## 3. Documentation and delivery

- [x] 3.1 Update generated help, schemas, README examples, and the interactive VHS demo with real mouse, pane, tab, and multi-commit generation flows.
- [x] 3.2 Run focused tests, race tests, provider validation, generation checks, PTY flows, complete diff inspection, and `nix flake check -L`.
- [x] 3.3 Commit, push, release for macOS ARM, Linux ARM, and Linux x86, verify archives, adopt through sysinit and sysinit.laurel, switch, and verify the installed TUI.
