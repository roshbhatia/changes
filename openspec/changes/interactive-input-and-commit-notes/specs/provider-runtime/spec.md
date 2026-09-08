## ADDED Requirements

### Requirement: Note generator boundary

A provider advertising `changes.notes.generate` MUST receive one bounded committed comparison and return normalized draft notes with exact placements. Changes core MUST remain independent of the harness or model used by that provider.

#### Scenario: Generator targets another comparison

- **WHEN** a generator returns a note outside the requested files, patch, or commit identities
- **THEN** Changes rejects the batch before invoking a writer

#### Scenario: Generator is not installed

- **WHEN** no provider advertises `changes.notes.generate`
- **THEN** ordinary comparison and manual note workflows remain available

