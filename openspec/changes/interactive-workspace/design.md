## Context

In scope: interactive review, a versioned external UI snapshot, note provenance and threads, bounded refresh, per-repository state, and TTY progress. Out of scope: Git mutations beyond explicit note creation, remote synchronization, and terminal scraping as an API.

The current call graph is `renderer.render` to `renderer.renderPatchesWithNotes`, then `noteContext`, `diffAnalysisContext`, and `renderTreeWithGroups`. The new snapshot builder MUST reuse that path so interactive, JSON, and one-shot views do not define different semantics. `calldiff reach -e renderer.render --to renderTreeWithGroups cmd/changes` confirms the edge. A reach query from `main` does not resolve the method edge, so the design binds to the method entry directly.

The keywords MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY in this document are interpreted as described in [RFC 2119](https://datatracker.ietf.org/doc/html/rfc2119).

## Goals / Non-Goals

Goals:

- Open a useful interactive frame from cached data, then prove or replace it in the background.
- Keep diff content, semantic context, note threads, and provenance in one readable tree.
- Make every interactive selection available to an external UI through stable machine data.
- Restore layout without treating cached comparison data as confirmed current data.

Non-Goals:

- Turn Changes into a full Git client.
- Add provider-specific UI code.
- Hide provider or refresh failures.
- Persist unsent popup text.

## Decisions

- Decision: `workspaceSnapshot` MUST be the shared data boundary. The terminal UI and `changes workspace` MUST consume the same snapshot, and the existing renderer MUST produce its `rendered` field.
- Alternative rejected: A terminal-only model would force Neovim to parse ANSI text or rebuild provider composition.

- Decision: A snapshot MUST use version `changes.workspace/v1`. JSON watch output MUST use versioned `snapshot`, `refreshing`, and `error` events. Standard output MUST contain only those values.
- Alternative rejected: An unversioned struct makes additive fields safe in Go but gives external clients no compatibility boundary.

- Decision: the navigator MUST have Files and History tabs. Files MUST toggle between tree and list. The navigator MUST dock left or bottom. The main pane MUST remain the cohesive diff tree.
- Alternative rejected: Simultaneous file and history panes spend too much space on navigation. Tabs satisfy the workflow on small terminals and preserve one focus model.

- Decision: Git views MUST be `working`, `staged`, and `commit`. Commit history MUST use a NUL-delimited Git format. Merge commits MUST use their first parent and MUST say so in snapshot metadata.
- Alternative rejected: Combined merge diffs require a different line identity model and cannot share current note placement rules.

- Decision: the UI MUST use Bubble Tea v1.3.10 and Bubbles v1.0.0, matching the tested Ask and Traces applications. It MUST use the alternate screen and `tea.ExecProcess` for editor note input.
- Alternative rejected: Bubble Tea v2 would introduce a separate terminal stack from both local reference applications during the first interactive release.

- Decision: note input MUST support `popup` and `editor`. Popup input MUST use a Bubble Tea textarea. Editor input MUST run `changes note add` as an argument vector after Bubble Tea releases the terminal. Both paths MUST use the configured note writer.
- Alternative rejected: Writing directly to a local file would bypass provider validation, Git notes selection, and idempotency rules.

- Decision: note provenance MUST be optional and additive. It MUST contain bounded `kind`, `tool`, `sessionId`, `workingDirectory`, and `url` fields. Existing source, author, session, and URL fields MUST remain valid.
- Alternative rejected: Replacing the existing fields would break strict provider/v1 decoders and stored note adapters.

- Decision: notes sharing a `threadId` MUST render as one thread. Replies MUST retain their own metadata and MUST sort by creation time with stable source order for equal or missing times.
- Alternative rejected: Flat comments repeat placement and hide reply relationships.

- Decision: workspace state MUST live below `XDG_STATE_HOME/changes/workspaces`. Snapshot cache MUST live below `XDG_CACHE_HOME/changes/workspaces`. Both stores MUST use atomic replacement, owner-only files, bounded documents, and symlink rejection.
- Alternative rejected: One directory would make a layout reset delete useful analysis or make cache cleanup delete user state.

- Decision: interactive startup MAY render the last snapshot before validation, but MUST mark it cached and refreshing. A fresh snapshot MUST replace it atomically. A provider failure MUST retain last-good notes, mark them stale, and expose the failure.
- Alternative rejected: Blocking on every provider loses the fast reopen. Treating cached data as fresh can show the wrong diff without warning.

- Decision: `--refresh` MUST skip the initial snapshot and existing provider result cache. It MUST NOT fetch a remote, push a ref, or mutate Git data.
- Alternative rejected: Refresh cannot mean remote synchronization because ordinary rendering is read-only and Git notes synchronization remains explicit.

- Decision: non-difftool progress MUST write to a TTY error stream, MUST stop before final output, and MUST be disabled by `--quiet`. It MUST not run for redirected error output.
- Alternative rejected: Standard-output progress breaks JSON, pipes, and pager composition.

## Risks / Trade-offs

- The first cached frame may be stale. A visible cached or refreshing state and immediate background refresh make the risk observable.
- Re-reading every note provider at one interval is less efficient than provider-specific validators. Provider/v1 does not expose validators, so the first release keeps the interval bounded.
- A shared snapshot contains both structured data and rendered text. This duplicates content, but it lets clients choose native rendering or exact Changes presentation.
- First-parent merge views omit other parents. The snapshot names the parent so the omission is inspectable.
- Bubble Tea adds core dependencies. Matching two existing local applications lowers integration risk.

## Validation

- Shared semantics: compare one-shot render and snapshot `rendered` output for the same fixture; inverse: a different comparison identity must not reuse that snapshot.
- Git views: create distinct working, staged, normal commit, and root commit fixtures; inverse: each command must leave status and refs unchanged.
- Layout state: restart the model with the same repository key and verify restoration; inverse: another repository must keep independent state.
- Cache: load a cached frame before a delayed refresh and verify its visible state; inverse: `--refresh` must not emit the cached frame.
- Notes: load a GitHub thread with replies and mixed authors; inverse: unrelated thread IDs must not merge.
- Provenance: round-trip every optional field through providers, JSON, cache, and rendering; inverse: legacy notes without provenance must remain valid.
- Authoring: test popup, editor, cancellation, writer failure, and exact argv; inverse: no failure path may report success.
- External contract: validate snapshot and event fixtures against the generated schema and run a headless Neovim consumer.
- Progress: capture TTY and pipe cases; inverse: difftool and redirected streams must contain no animation.

## Rollout & Gating

1. Land the snapshot model, schema, history reader, provenance, and persistence with focused tests.
2. Land the terminal model and authoring paths over that snapshot.
3. Add the external client example, tape, GIF, command docs, and full release gate.
4. Release as the next minor version after the complete flake check passes.

Rollback removes the new commands and UI packages. Existing provider data and Git notes remain intact. Cache and state documents are non-authoritative and may stay on disk.

## Adversarial Review

The owner directed Changes to stop after the current implementation run and not start another adversarial review. This change MUST record that omission and MUST NOT spawn review critics or a mediation round. Deterministic tests, complete diff inspection, call-graph inspection, native CI, and release asset verification remain required.

## Implementation history

- 2026-09-07 created from `report-source.md` and accepted owner direction.
