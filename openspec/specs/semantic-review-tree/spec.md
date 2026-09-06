# semantic-review-tree Specification

## Purpose

Define the provider-neutral symbol and call context shown with a Git diff.

## Requirements

### Requirement: Symbol enrichment

Changes MUST request `changes.symbols` from every provider that advertises the
action and MUST associate returned symbols with repository-relative files.

#### Scenario: Multiple symbol providers match

- **WHEN** more than one provider returns symbols for one file
- **THEN** Changes merges their symbols into that file's semantic context

### Requirement: Call enrichment

Changes MUST request `changes.calls` from every provider that advertises the
action and MUST associate returned edges with repository-relative files.

#### Scenario: Call analysis is disabled

- **WHEN** the user supplies `--no-calls`
- **THEN** Changes does not invoke a call provider

### Requirement: Optional analysis failures

A symbol or call provider failure MUST NOT suppress the Git diff. Changes MUST
write the provider error to standard error and continue with available layers.

#### Scenario: One analyzer fails

- **WHEN** one semantic provider returns an error
- **THEN** Changes renders Git output and all successful semantic results

### Requirement: Workspace identity

Recursive output MUST prefix semantic paths with their repository identity so
that equal file names from different repositories remain distinct.

#### Scenario: Two repositories contain the same path

- **WHEN** both repositories change `main.go`
- **THEN** each semantic record appears under its own repository

### Requirement: Summary mode

Stat mode MUST show the change summary and MUST NOT invoke symbol or call
analysis.

#### Scenario: User requests stats

- **WHEN** the user runs `changes --stat`
- **THEN** Changes renders file statistics without semantic provider calls
