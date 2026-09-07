## Purpose

Store Changes annotations for committed comparisons in a dedicated Git notes ref while leaving all remote synchronization under explicit user control.

## ADDED Requirements

### Requirement: Dedicated notes ref

The provider MUST read and write only the exact `refs/notes/changes` and MUST attach each document to the compared head commit.

#### Scenario: First note for a commit

- **WHEN** a user creates a note for a committed comparison with no existing Changes note document
- **THEN** the provider creates `refs/notes/changes` and attaches the document to the resolved head commit

#### Scenario: Another Git notes namespace exists

- **WHEN** `refs/notes/commits` or another notes ref contains data
- **THEN** the provider leaves that ref unchanged

#### Scenario: The Changes notes ref is symbolic

- **WHEN** `refs/notes/changes` points to a branch or another notes ref
- **THEN** the provider rejects reads and writes without changing the symbolic target

#### Scenario: The Changes notes ref has a missing symbolic target

- **WHEN** `refs/notes/changes` points to a ref that does not exist
- **THEN** the provider still rejects reads and writes as symbolic

#### Scenario: A descendant notes ref exists

- **WHEN** `refs/notes/changes/archive` or another descendant exists without the exact Changes notes ref
- **THEN** the provider does not read the descendant as Changes data and does not change it

#### Scenario: The parent process exports Git repository overrides

- **WHEN** inherited Git variables point at another repository, worktree, index, namespace, or object database
- **THEN** the provider ignores those overrides and updates only the repository named by the request

### Requirement: Committed comparisons only

The provider MUST store notes only when the request identifies canonical full base and head commit IDs and an exact patch fingerprint.

#### Scenario: Working-tree note creation

- **WHEN** a caller asks the provider to create a working-tree or index note
- **THEN** the provider rejects the write and directs the caller to a committed comparison

#### Scenario: Working-tree note read

- **WHEN** a render asks the provider for working-tree or index notes
- **THEN** the provider returns an empty note set without changing the repository

#### Scenario: A commit ID uses noncanonical case

- **WHEN** a request names a commit with uppercase or otherwise noncanonical object ID text
- **THEN** the provider rejects it before reading or writing its note path

### Requirement: Mergeable note records

The stored note document MUST use bounded, canonical, line-delimited records so Git's `cat_sort_uniq` notes merge strategy can combine independent notes.

#### Scenario: Two clones add different notes

- **WHEN** their Changes notes refs are merged with `git notes merge -s cat_sort_uniq`
- **THEN** both canonical records remain available to the provider

#### Scenario: Separate comparisons reuse a stable key

- **WHEN** two comparisons with the same head but different base or fingerprint values use one stable key
- **THEN** each comparison retains its own deterministic note without a duplicate-key conflict

#### Scenario: Stored data is malformed or oversized

- **WHEN** a note document exceeds the provider limit or contains an invalid record
- **THEN** the provider rejects the document without replacing it

#### Scenario: A stored identifier is not canonical

- **WHEN** a keyed record ID differs from its comparison-derived digest or a random ID is not canonical 128-bit hexadecimal text
- **THEN** the provider rejects the document without returning or replacing it

#### Scenario: A stored comparison ID is not a canonical commit

- **WHEN** any stored base or head is symbolic, abbreviated, noncanonical, missing, or not a commit
- **THEN** the provider rejects the document before returning or republishing its records

#### Scenario: A stored record names another head commit

- **WHEN** a canonical record is attached under a head other than its anchor head
- **THEN** reads and writes reject the document without replacing it

#### Scenario: Another process updates the notes ref during creation

- **WHEN** the notes ref changes after the provider reads it but before the provider publishes its batch
- **THEN** the provider merges the current records and retries with compare-and-swap, or fails without replacing the external update

#### Scenario: The notes ref becomes symbolic during creation

- **WHEN** `refs/notes/changes` becomes symbolic before the provider publishes its batch
- **THEN** a non-dereferencing compare-and-swap fails without updating the symbolic target

#### Scenario: The notes ref contains many annotated commits

- **WHEN** the complete notes tree exceeds one head document's size limit but the selected head document remains within it
- **THEN** the provider reads and writes the selected head without applying the document limit to the complete ref

### Requirement: Atomic and idempotent creation

The provider MUST validate a complete batch before one ref update and MUST treat a stable note key as idempotent for the same payload.

#### Scenario: One draft in a batch is invalid

- **WHEN** any requested draft fails validation
- **THEN** the provider writes none of the drafts

#### Scenario: A generated batch is retried

- **WHEN** the same keys and payloads already exist for the comparison
- **THEN** the provider returns the existing notes without adding duplicate records

### Requirement: No implicit remote operation

The provider MUST NOT fetch, merge, pull, push, or configure any remote or notes rewrite rule.

#### Scenario: A local note is created

- **WHEN** the provider completes the write
- **THEN** only local objects and `refs/notes/changes` change

#### Scenario: A remote has a Changes notes ref

- **WHEN** the local provider reads notes
- **THEN** it does not contact the remote, lazy-fetch a promised object, or import that ref
