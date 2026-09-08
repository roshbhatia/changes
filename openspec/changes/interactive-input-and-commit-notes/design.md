## Context

The interactive workspace already enables Bubble Tea cell-motion events, but its handler covers only basic focus, row selection, and wheel movement. `Tab` currently changes pane focus. The existing `changes note generate` path already separates `changes.notes.generate` from `changes.notes.create`, validates exact placement, and writes one atomic batch for one comparison. The missing layer is interactive selection and draft review.

## Goals / Non-Goals

**Goals:**

- Match Traces and Orc pane, tab, and mouse behavior.
- Select one or several commits without conflating them into one Git range.
- Reuse the current provider actions while delaying writes until confirmation.
- Preserve a scriptable, non-writing path for generated drafts.

**Non-Goals:**

- Add a harness or model dependency to Changes core.
- Make writes across several commits transactional.
- Fetch, push, rewrite commits, or synchronize the Changes notes ref.
- Persist generated draft text across restarts.

## Decisions

- Decision: Pane focus follows screen geometry. With a left dock, only `Ctrl-h` and `Ctrl-l` cross the divider. With a bottom dock, only `Ctrl-j` and `Ctrl-k` cross it. `Tab` and `Shift-Tab` cycle tabs inside the focused pane.
- Alternative rejected: Treating two key pairs as aliases ignores the visible layout and conflicts with the shared TUI contract.

- Decision: Keep `tea.WithMouseCellMotion` and add explicit hit regions for tabs, rows, panes, and wheels. Tests MUST exercise encoded terminal mouse input, not only direct model messages.
- Alternative rejected: All-motion tracking adds hover traffic without a user-facing need.

- Decision: Store marked commits as canonical OIDs separate from the cursor. Space toggles the current History row. Generation uses marked commits, or the cursor when none are marked.
- Alternative rejected: A contiguous Git range changes note attachment semantics and cannot represent non-adjacent selection.

- Decision: Refactor the existing generator path into generate, review, and write steps. Each commit keeps its own first-parent snapshot and provider request. All batches are validated before review; confirmed writes remain atomic per commit.
- Alternative rejected: A new provider action duplicates `changes.notes.generate` and exposes UI lifecycle to providers.

- Decision: The review overlay groups rows by commit, file, and note. `j/k` moves, Space includes or excludes, `e` edits, Enter confirms included drafts, and Escape cancels without writes.
- Alternative rejected: Immediate generation and storage removes the user checkpoint and makes a mistaken multi-commit action expensive to undo.

- Decision: Non-interactive generation accepts repeated `--commit`. A no-write JSON mode returns validated drafts. Existing single-comparison write behavior remains available.
- Alternative rejected: Passing one combined range attaches all notes to one head and loses per-commit identity.

## Risks / Trade-offs

- Provider calls for many commits can be slow. Bound the selection, show progress, and keep each provider timeout.
- A repository can change during generation. Re-read and compare every selected snapshot before review and again before its write.
- Cross-commit writes can partly succeed. Validate first, report each result, and rely on stable keys for safe retry.
- Mouse coordinates vary with dock and terminal size. Derive hit regions from the same layout helpers used by rendering.

## Migration Plan

1. Land input behavior and direct model plus PTY coverage.
2. Refactor single-comparison generation without changing its provider contract.
3. Add History marking, draft review, repeated commit input, generated documentation, and demos.
4. Release, adopt through sysinit and Laurel, and prove the installed TUI.

Rollback can remove the new selection and review surfaces. Existing stored notes and provider manifests remain valid.
