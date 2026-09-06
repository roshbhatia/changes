# git-comparisons Specification

## Purpose

Define how Changes selects and reads Git comparisons without changing repository state.

## Requirements

### Requirement: Git comparison forms

Changes MUST support the working tree, the index, one revision against the
working tree, and two revisions. It MUST let Git resolve revisions and paths.

#### Scenario: No revision is given

- **WHEN** the user runs `changes` without a revision
- **THEN** Changes compares the index with the working tree

#### Scenario: Two revisions are given

- **WHEN** the user supplies two valid revisions
- **THEN** Changes compares those two trees

#### Scenario: Staged comparison is ambiguous

- **WHEN** the user combines `--staged` with two revisions
- **THEN** Changes rejects the request before it runs the comparison

### Requirement: Path selection

Changes MUST accept Git-style path arguments and MUST resolve relative paths
from the user's working directory before it runs Git at the repository root.

#### Scenario: Explicit path separator is used

- **WHEN** the user places paths after `--`
- **THEN** Changes treats every following argument as a path

#### Scenario: Argument is neither a revision nor a path

- **WHEN** an argument names neither a Git object nor an existing path
- **THEN** Changes reports the invalid argument instead of reporting no changes

### Requirement: Stable source data

Changes MUST produce repository-relative touched paths and a bounded unified
patch. Rename-aware note paths MUST include both the source and destination.

#### Scenario: Patch exceeds the source limit

- **WHEN** Git produces a patch larger than 64 MiB
- **THEN** Changes rejects the patch before analysis or rendering

#### Scenario: A file was renamed

- **WHEN** a comparison contains a rename
- **THEN** Changes exposes both names for note anchoring

### Requirement: Workspace comparisons

Changes MUST support one repository and recursive workspace discovery. A
missing revision in one recursive repository MUST NOT hide valid repositories.

#### Scenario: One recursive repository lacks the revision

- **WHEN** a workspace comparison succeeds in some repositories but not one
- **THEN** Changes names and skips that repository, then renders the others

#### Scenario: The only repository fails

- **WHEN** the comparison fails in a single-repository invocation
- **THEN** Changes returns the Git failure
