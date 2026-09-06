# diff-notes Specification

## Purpose

Define source-neutral annotations that Changes reads, writes, and projects onto
a selected Git comparison.

## Requirements

### Requirement: Normalized notes

Every note MUST include provider-qualified identity, source identity, summary,
author, origin, authority, state, immutable anchor, and current placement.

#### Scenario: Provider returns an unqualified note ID

- **WHEN** a provider named `local` returns note ID `one`
- **THEN** Changes exposes the note as `local:one`

### Requirement: Safe anchors

An anchor path MUST be clean and repository-relative. Its side and line range
MUST describe a file, left-side line, right-side line, or valid range.

#### Scenario: Anchor escapes the repository

- **WHEN** a provider returns an absolute path or a path beginning with `../`
- **THEN** Changes rejects the note

### Requirement: Note reads

Changes MUST invoke every provider that advertises `changes.notes`, validate
each result, sort the combined notes, and keep note reads outside the cache.

#### Scenario: No note reader exists

- **WHEN** the user renders a comparison without a note provider
- **THEN** Changes renders normally without a note layer

### Requirement: Note creation

`changes note add` MUST use one selected provider that advertises
`changes.notes.create`. The returned note MUST exactly match the requested
anchor and placement.

#### Scenario: Multiple writable providers match

- **WHEN** more than one writable provider exists and none is selected
- **THEN** Changes requires the user to select one

#### Scenario: Provider changes the anchor

- **WHEN** a writable provider returns a different anchor or non-exact placement
- **THEN** Changes rejects the created note

### Requirement: Comparison identity

Notes MUST record whether they target commits, the index, or the working tree.
Changes MUST pass stable endpoint identities and a patch fingerprint to readers
and writers.

#### Scenario: Working-tree note is read later

- **WHEN** the patch fingerprint no longer matches its recorded comparison
- **THEN** the provider may return an outdated, contextual, file, or orphan placement

### Requirement: Watch refresh

Watch mode MUST poll note readers at `notes.refreshInterval`, independently of
the faster Git watch interval. It MUST NOT cache note reads.

#### Scenario: Only the Git poll is due

- **WHEN** the diff watch interval passes before the note refresh interval
- **THEN** Changes does not invoke note readers
