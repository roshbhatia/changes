## Context

In scope: a Git-native provider for committed notes and a reusable Neovim adapter over the existing CLI. Out of scope: automatic remote synchronization, working-tree storage in Git notes, a custom floating-window implementation, and editor code in the Changes core.

The provider protocol already separates note readers and writers from core rendering. `changes note add --message-file - --json` already provides the data boundary that an editor adapter needs.

The keywords MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY in this document are interpreted as described in [RFC 2119](https://datatracker.ietf.org/doc/html/rfc2119).

## Goals / Non-Goals

Goals:

- Store committed annotations in a ref that users can inspect and synchronize with Git.
- Preserve atomic batch writes and stable-key retries.
- Keep Neovim input, process execution, and result handling replaceable and testable.

Non-Goals:

- Store working-tree or index notes in synthetic Git objects.
- Fetch, merge, push, or configure note refs automatically.
- Choose a global Neovim keybinding or ship a custom popup renderer.

## Decisions

- Decision: The provider MUST own the exact `refs/notes/changes`, separate from its descendants and Git's default `refs/notes/commits`. It MUST use bounded exact lookup and reject live or dangling symbolic forms. Git supports custom notes refs and attaches notes without changing the annotated object. See the [Git notes documentation](https://git-scm.com/docs/git-notes).
- Alternative rejected: Reusing the default ref would mix machine-readable records with unrelated human notes.

- Decision: Provider Git commands MUST ignore inherited repository, worktree, index, namespace, replacement, and object-database redirection variables.
- Alternative rejected: `git -C` alone does not override variables such as `GIT_DIR`, which can redirect the note into another repository.

- Decision: One head commit MUST carry bounded, canonical newline-delimited JSON records for every Changes note anchored to that head. The bound applies per head, not to the complete notes ref. Records MUST include resolvable canonical commit IDs and the exact base, head, fingerprint, path, side, and line range. Keyed IDs MUST equal the digest derived from that complete comparison identity and key. Random IDs MUST use the canonical 128-bit hexadecimal form.
- Alternative rejected: One formatted JSON document is easier to read but does not compose with Git's line-based `cat_sort_uniq` merge strategy.

- Decision: Normal reads for working-tree and index comparisons MUST return no records. Writes for those targets MUST fail with a committed-comparison instruction.
- Alternative rejected: Synthetic annotated objects would add hidden object-database writes and would not follow ordinary commit reachability.

- Decision: A repository-scoped advisory lock MUST cover provider reads and writes. The provider MUST reject a symbolic `refs/notes/changes`. The final non-dereferencing ref update MUST use compare-and-swap so an external Git notes update cannot be overwritten. A batch MUST become one replacement note blob and one Git notes ref update.
- Alternative rejected: Sequential `git notes append` calls expose partial batches and can lose concurrent read-modify-write updates.

- Decision: The provider MUST never invoke a remote operation or allow Git to lazy-fetch a missing object. Documentation MUST show explicit fetch to a staging ref, `git notes merge -s cat_sort_uniq`, and push of `refs/notes/changes`.
- Alternative rejected: Automatic synchronization changes shared state during ordinary diff rendering and hides merge conflicts.

- Decision: The Neovim adapter MUST use `vim.ui.input` for presentation and `vim.system` with a string array for execution. Neovim documents `vim.ui.input` as the replaceable input interface and `vim.system` as direct process execution. See the [Lua API](https://neovim.io/doc/user/lua) and [Lua plugin guide](https://neovim.io/doc/user/lua-plugin/).
- Alternative rejected: Shell commands require quoting user text and file paths. A private floating window would bypass configured UI providers.

- Decision: The Neovim adapter MUST capture the buffer and location before opening an asynchronous prompt and MUST prove that an ordinary, bounded buffer matches its regular disk file immediately before execution. It MUST preserve empty, final-newline, missing-final-newline, and CRLF byte forms. It MUST send the captured digest to Changes, which MUST stabilize it with the selected comparison and match it against the selected working, index, or commit side. Changes MUST disable text conversion and reject working files with content-changing Git filters or encodings.
- Alternative rejected: Mapping live buffer line numbers onto a Git or disk comparison can attach a note to different content.

- Decision: The plugin MUST expose `:ChangesNote`, `require("changes.notes").prompt(opts)`, `require("changes.notes").add(opts, callback)`, and `<Plug>(changes-note)` mappings. It MUST NOT choose a user key.
- Alternative rejected: A setup-only keymap DSL adds editor policy and prevents users from composing the operation with ordinary Neovim mappings.

## Risks / Trade-offs

- Two clones can update the same head before synchronization. Canonical one-line records and `cat_sort_uniq` preserve distinct notes; duplicate keys with different payloads fail closed during the next read.
- Commit-only storage cannot preserve notes for an uncommitted harness session. The local provider remains the store for those notes.
- A custom `vim.ui.input` provider controls popup appearance. The adapter validates only the returned value.
- Git notes do not move across rewritten commits by default. This change does not configure rewrite behavior because that policy belongs to the repository owner.

## Rollout & Gating

1. Ship the new provider and Neovim package as optional Nix outputs.
2. Keep `git-notes` out of the `full` aggregate so `local-notes` remains its only writable provider.
3. Require `--provider git-notes` for committed writes when another writable provider is also installed.
4. Remove either package to roll back; existing `refs/notes/changes` data remains intact.

The gate requires provider validation, unit and concurrent-write tests, a headless Neovim contract test, both example recordings, and the full flake check.

## Adversarial Review

Run the repository adversarial-review loop after deterministic checks pass. Record surviving objections and revisions separately from owner approval.
