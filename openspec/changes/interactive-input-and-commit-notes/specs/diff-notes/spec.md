## ADDED Requirements

### Requirement: Generated note drafts

Changes MUST treat output from `changes.notes.generate` as uncommitted drafts. Interactive generation MUST expose every valid draft for review and MUST NOT call a writer before explicit confirmation.

#### Scenario: User cancels generated notes

- **WHEN** a generator returns drafts and the user cancels review
- **THEN** Changes writes no note and returns to the prior workspace state

#### Scenario: User excludes a draft

- **WHEN** the user removes one draft from the review selection and confirms the rest
- **THEN** Changes sends only the included drafts to the writer

### Requirement: Multi-commit note generation

Changes MUST generate notes for one or more selected commits as separate first-parent comparisons. It MUST validate every generated batch before it begins writing and MUST report write results per commit.

#### Scenario: No commits are marked

- **WHEN** generation starts from History with no marked commits
- **THEN** Changes uses the commit under the cursor

#### Scenario: Several commits are marked

- **WHEN** generation starts with several marked commits
- **THEN** Changes invokes the generator once per first-parent comparison and groups drafts by commit

#### Scenario: A later commit write fails

- **WHEN** one confirmed commit batch succeeds and another fails
- **THEN** Changes identifies both outcomes and does not report the whole selection as successful

### Requirement: Scriptable draft generation

The note generation command MUST accept repeated commit selection and MUST support machine-readable generation without storing the drafts.

#### Scenario: Automation requests drafts only

- **WHEN** a caller selects multiple commits and disables writing with JSON output
- **THEN** standard output contains the validated drafts and Changes does not invoke a note writer

