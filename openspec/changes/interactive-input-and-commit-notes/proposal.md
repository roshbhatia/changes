## Why

Changes currently overloads `Tab` as a pane switch and exposes incomplete mouse behavior, which conflicts with the shared Traces and Orc input model. Commit notes also require manual text entry one comparison at a time, so a user cannot select one or several commits and ask a provider to draft review notes before saving them.

## What Changes

- Make mouse input a first-class interactive control for focus, selection, scrolling, tabs, and actions.
- Reserve `Ctrl-h/j/k/l` for pane navigation and keep `Tab` and `Shift-Tab` inside the focused pane.
- Add multi-selection to the History tab without changing Git state.
- Add a provider-neutral note-generation action that accepts one or more committed comparisons and returns reviewable note drafts.
- Let the user review, edit, accept, or reject generated drafts before an existing writable note provider stores them.
- Keep generation and storage separate so Changes works without a generator and never embeds a harness implementation.

## Capabilities

### New Capabilities

- `interactive-input`: Define consistent keyboard, mouse, pane, tab, and multi-selection behavior for the interactive workspace.

### Modified Capabilities

- `diff-notes`: Add provider-generated draft notes, explicit user review, and explicit per-commit results across a selection.
- `provider-runtime`: Add discovery and validation for a note-generation action without teaching Changes about a provider implementation.

## Impact

- Changes interactive model, key metadata, help, mouse handling, state, and PTY tests.
- History selection and committed-comparison snapshot construction.
- Provider manifests, request and response schemas, validation fixtures, generated documentation, and completions.
- Reference extras for note generation and Git note storage; no implicit fetch, push, commit, or ref synchronization.
