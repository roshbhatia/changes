# Research source: interactive change workspace

> Audience: Changes maintainers and provider authors
> Date: 2026-09-07
> Status: implementation input

Scope includes:

- an interactive Git review workspace over the existing cohesive diff tree;
- file and commit navigation, unified and side-by-side views, note authoring, provenance, threads, refresh, and per-repository state;
- a versioned machine-readable CLI contract for Neovim and other clients;
- terminal progress while a non-difftool command prepares a view.

Scope does not include:

- automatic fetch, merge, or push of `refs/notes/changes`;
- staging, discarding, rebasing, committing, or resolving GitHub threads;
- a second note model owned by the interactive UI;
- terminal-screen scraping as an integration API.

## Direct answer

GitHub PR comments and post-hoc agent review are note providers. Harness-authored and manual annotations use a writable note provider. The interactive UI is not a provider. It composes one comparison, provider layers, Git history, and durable view state.

The review surface should read in this order:

```text
repository / selected Git view
├── logical change group
│   ├── directory / file
│   │   ├── symbol or hunk
│   │   │   ├── diff line
│   │   │   │   └── note thread
│   │   │   │       ├── author, source, session, working directory, time, URL
│   │   │   │       └── replies in source order
│   │   │   └── changed call edges
│   │   └── file-level notes
│   └── unmatched changes
└── commit history
    └── cached note authors for comparisons already opened in this workspace
```

This keeps context and notes beside the code they explain. A separate navigator changes focus; it does not split the explanation from the diff.

## Findings

### Terminal interaction

Bubble Tea provides a model-update-view loop, alternate-screen operation, terminal restoration around an external editor, and command-based asynchronous work. Its `ExecProcess` contract pauses the program, releases the terminal, runs the editor, and restores the terminal afterward. The official examples cover alternate-screen programs, editor execution, lists, mouse input, and composed views. [Bubble Tea examples](https://github.com/charmbracelet/bubbletea/blob/main/examples/README.md) [Bubble Tea process execution](https://github.com/charmbracelet/bubbletea/blob/main/exec.go)

The local Traces UI already proves the desired interaction language: Vim-shaped navigation, a `:` command line with prefix completion and history, a foldable tree, inspector tabs, and dock positions on each edge. Ask already proves a TTY-only progress animation that leaves non-interactive output clean. Changes should reuse those patterns, not import either application.

Lazygit confirms two useful conventions: files and commit-related views can share windows through tabs, and a file navigator can toggle between flat and tree forms. It also treats the diff renderer as replaceable while the application owns focus and navigation. [Lazygit codebase guide](https://github.com/jesseduffield/lazygit/blob/master/docs/dev/Codebase_Guide.md) [Lazygit keybindings](https://github.com/jesseduffield/lazygit/blob/master/docs/keybindings/Keybindings_en.md) [Lazygit custom pagers](https://github.com/jesseduffield/lazygit/blob/master/docs/Custom_Pagers.md)

Decision input:

- Use Bubble Tea v1 with Bubbles v1 because Ask and Traces already use and test that set in this workspace.
- Use one main diff viewport and one navigator. The navigator has Files and History tabs, toggles tree or list form, and docks left or bottom.
- Keep the cohesive diff tree as the main document. Selecting a file filters that document without changing its shape.
- Provide `:` commands for discoverability and keys for repeated actions.
- Release the terminal through `tea.ExecProcess` when the configured editor owns note input.

### Git views and history

Git defines distinct comparisons for working tree against index, index against a tree, and two trees. Those are separate views and should stay explicit. [Git diff](https://git-scm.com/docs/git-diff)

Git log supports machine formats and path-limited history. Changes should request NUL-delimited fields and disable decorations that a parser would otherwise need to interpret. [Git log](https://git-scm.com/docs/git-log)

Decision input:

- Name the primary views `working`, `staged`, and `commit`.
- `working` means index to working tree. `staged` means `HEAD` to index. `commit` means the selected commit's first parent to that commit, with the root commit compared to the empty tree.
- History selection changes the comparison. It does not mutate the repository.
- Cache comparison snapshots by resolved endpoints and patch fingerprint. A selected commit can reuse a prior snapshot without rerunning providers.

### Notes, provenance, and threads

GitHub distinguishes whole-PR comments, line review comments, and commit comments. Review comment payloads include the author, body, creation and update times, commit identities, reply identity, path, line, and URL. [GitHub comment types](https://docs.github.com/en/rest/guides/working-with-comments) [GitHub review comments](https://docs.github.com/en/rest/pulls/reviews)

The current GitHub provider already returns every comment in a review thread with `threadId`, `replyTo`, author, state, times, and URL. The current note contract also carries source and session. It does not carry the generating tool's working directory or a distinct run identity.

Decision input:

- Extend normalized notes with an optional provenance object: `kind`, `tool`, `sessionId`, `workingDirectory`, and `url`.
- Keep existing top-level `source`, `author`, `session`, and `url` fields for provider/v1 compatibility. Providers fill provenance when they know more.
- Group notes by `threadId`. Preserve reply order by creation time and source order as a stable tie-breaker.
- Render the complete thread at its current placement. Each reply retains its own author, time, source, and URL.
- Manual popup or editor input creates the same `NoteDraft` used by the CLI. The configured note writer remains the storage authority.

### Refresh and caching

GitHub recommends webhooks instead of polling. When polling is required, it recommends a fixed schedule, stable requests, conditional requests where supported, and explicit handling of repeated failures. [GitHub REST API best practices](https://docs.github.com/en/rest/using-the-rest-api/best-practices-for-using-the-rest-api)

The local GitHub provider uses GraphQL through `gh`, so the core cannot safely inject REST validators such as `ETag`. The current note model also treats provider failures as incomplete refreshes and retains prior notes in watch mode. That last-good behavior is the correct interactive fallback.

The XDG Base Directory specification separates persistent application state from replaceable cache data. It explicitly names view, layout, open files, and history as state that can survive restarts. [XDG Base Directory specification](https://specifications.freedesktop.org/basedir/)

Decision input:

- Store layout, navigator mode, active tab, selected view, selected file, and command history under `XDG_STATE_HOME/changes/workspaces`.
- Store replaceable comparison snapshots under `XDG_CACHE_HOME/changes/workspaces`.
- Load a matching cached snapshot immediately, mark it `refreshing`, and refresh in the background.
- Refresh live notes on `notes.refreshInterval`. Preserve last-good notes and mark them stale when a provider fails.
- `--refresh` bypasses the initial snapshot and provider result caches. It never changes a remote or a Git notes ref.
- Bound cache entries and reject symlinked cache or state paths before writing.

### External UI contract

Neovim exposes direct process execution with an argument vector through `vim.system`; it does not invoke a shell. Neovim extmarks attach highlights, signs, and virtual text to buffer positions and track edits. These primitives let a plugin render note markers while Changes remains the source of comparison and note data. [Neovim `vim.system`](https://neovim.io/doc/user/lua) [Neovim extmarks](https://neovim.io/doc/user/api/)

Decision input:

- Add `changes workspace` as the non-interactive contract.
- Emit a single `changes.workspace/v1` JSON snapshot by default.
- Add `--watch` JSON Lines events for clients that want refreshes without restarting the process.
- Include repository identity, comparison identity, freshness, logical groups, normalized files and hunks, notes and provenance, history, and the cohesive rendered document.
- Generate and ship `schema/workspace.schema.json`.
- Keep authoring on `changes note add`; external clients do not need a second mutation endpoint.
- Write diagnostics to standard error. Standard output contains only the requested machine stream.

## Gap matrix

| Need | Existing evidence | Gap | Implementation decision |
| --- | --- | --- | --- |
| Cohesive tree | Current renderer embeds notes under file or line rows | No interactive focus | Reuse the renderer in a viewport |
| Logical grouping | `changes.groups` provider and current grouped render | Navigator is path-only | Preserve group order in document; file navigator mirrors file order |
| File tree or list | Traces tree and Lazygit toggle | No Changes navigator | Add Files tab with both forms |
| Commit history | Git log machine format | No history model | Add read-only history records and Commit view |
| Inline or side-by-side | Current engine supports both | No live toggle | Rebuild the current snapshot from cached analysis |
| Note provenance | Source, author, session, URL exist | Tool and working directory absent | Add optional provenance |
| Full threads | GitHub provider returns replies | Renderer prints comments as siblings | Nest comments by thread and reply |
| Manual note | CLI and Neovim adapter exist | No Changes UI authoring | Add popup and editor modes over `note add` |
| Fast reopen | Provider result cache exists | No view or snapshot state | Separate XDG state and cache stores |
| Updated PR notes | Watch mode rereads notes | No interactive background refresh | Last-good refresh loop plus `--refresh` |
| External clients | Note JSON exists | No full workspace JSON | Add versioned snapshot and event stream |
| Progress | Ask and Traces animate TTY work | Changes blocks silently | Add stderr-only animation outside difftool |

## Risks and limits

- A cached working-tree snapshot can be stale when the process starts. The UI must label it until the background comparison confirms it.
- Refreshing every note provider is less efficient than provider-specific validators. Provider/v1 has no cache validator contract, so the first release keeps one bounded interval.
- First-parent commit diffs do not represent combined merge diffs. The UI must name this behavior.
- A terminal viewport cannot expose Neovim-native extmarks. The JSON snapshot carries exact line anchors so the Neovim client can own that presentation.
- Popup authoring adds a second text-entry presentation, but it still uses the existing note provider boundary and validation.

## Recommendations

1. Ship `changes interactive` and `changes workspace` in one release so the terminal UI and editor clients consume the same snapshot model.
2. Keep providers responsible for producing or storing note data. Keep navigation, cache state, and terminal rendering inside Changes.
3. Make cached data observable through `freshness`, `generatedAt`, `refreshedAt`, and provider failure fields.
4. Test inverse paths: redirected stdout, missing TTY, stale snapshots, provider failure, cache bypass, editor failure, and repository changes during refresh.
5. Record a permanent `examples/interactive-workspace` tape and GIF. The demo must show layout, tab, Git view, diff layout, note provenance, and forced refresh.

## Claim-source ledger

| Claim | Primary source | Applied check |
| --- | --- | --- |
| Bubble Tea can suspend for an editor and restore the terminal | Bubble Tea `exec.go` | Editor-mode test checks command completion and resumed UI state |
| Git comparisons have distinct working, staged, and tree forms | Git diff documentation | Fixture asserts each view produces different content |
| GitHub review data can identify authors, replies, commits, times, and URLs | GitHub review API documentation | GitHub provider fixture preserves the complete thread |
| Persistent UI layout is state, not cache | XDG Base Directory specification | State and snapshot paths resolve under different XDG roots |
| Neovim can consume a shell-free process contract and place annotations | Neovim Lua and API documentation | Headless contract decodes workspace JSON and installs extmarks |
| Polling must be bounded and failure-aware | GitHub REST API best practices | Refresh test keeps last-good notes and surfaces stale state |

## Verification status

The repository and local reference applications were inspected at clean `main` checkouts. The cited upstream pages were checked on 2026-09-07. Implementation and end-to-end validation remain pending.
