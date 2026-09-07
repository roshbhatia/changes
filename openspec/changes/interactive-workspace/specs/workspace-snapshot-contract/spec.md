## Purpose

Define a stable machine-readable Changes workspace for Neovim and other external clients.

## ADDED Requirements

### Requirement: Versioned snapshot

`changes workspace` MUST emit one `changes.workspace/v1` JSON object containing repository, comparison, freshness, history, groups, files, hunks, notes, failures, and the cohesive rendered document.

#### Scenario: External client requests a snapshot

- **WHEN** the command succeeds without `--watch`
- **THEN** standard output contains exactly one valid snapshot and a trailing newline

### Requirement: Watch event stream

`changes workspace --watch` MUST emit versioned JSON Lines events for cached startup, refreshing, fresh snapshot, and refresh error states.

#### Scenario: Provider refresh fails

- **WHEN** a prior snapshot exists and one note provider fails
- **THEN** the stream emits an error or stale snapshot event that retains last-good notes

### Requirement: Structured line identity

Every hunk line MUST expose its kind and old or new line identity. Notes MUST expose immutable anchors and current placements.

#### Scenario: Neovim installs a line marker

- **WHEN** a note has a current right-side line placement
- **THEN** the client can identify the repository path and 1-based target line without parsing rendered text

### Requirement: Clean stream boundary

Machine output MUST contain no ANSI progress, help, or diagnostics. Diagnostics MUST use standard error.

#### Scenario: Progress is enabled on a terminal

- **WHEN** `changes workspace` writes JSON to standard output
- **THEN** the JSON remains independently decodable

### Requirement: Generated schema

`changes generate` MUST publish `schema/workspace.schema.json`, and release archives MUST include it.

#### Scenario: Snapshot shape changes

- **WHEN** the Go contract changes without regenerated schema
- **THEN** `changes generate --check` fails
