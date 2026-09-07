## MODIFIED Requirements

### Requirement: Normalized notes

Every note MUST include provider-qualified identity, source identity, summary, author, origin, authority, state, immutable anchor, and current placement. A note MAY include provenance naming its kind, tool, session identity, working directory, and source URL.

#### Scenario: Provider returns an unqualified note ID

- **WHEN** a provider named `local` returns note ID `one`
- **THEN** Changes exposes the note as `local:one`

#### Scenario: Agent note includes run context

- **WHEN** a provider knows the producing tool, session, and working directory
- **THEN** Changes preserves and renders those values with the note

### Requirement: Note reads

Changes MUST invoke every provider that advertises `changes.notes`, validate each result, sort the combined notes, and keep note reads outside the provider result cache. Interactive mode MAY retain last-good notes inside a marked workspace snapshot, but it MUST refresh them independently before marking the snapshot fresh.

#### Scenario: No note reader exists

- **WHEN** the user renders a comparison without a note provider
- **THEN** Changes renders normally without a note layer

#### Scenario: Interactive refresh fails

- **WHEN** one reader fails after a prior successful snapshot
- **THEN** Changes retains that reader's last-good notes and marks them stale

### Requirement: Note threads

Notes that share a thread identity MUST render together at one current placement. Replies MUST preserve their own author, source, time, provenance, and URL.

#### Scenario: GitHub thread has three comments

- **WHEN** the provider returns one root and two replies with the same thread ID
- **THEN** Changes renders one thread with all three comments in source order
