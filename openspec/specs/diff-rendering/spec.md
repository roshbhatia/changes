# diff-rendering Specification

## Purpose

Define terminal rendering and Git-compatible external diff composition.

## Requirements

### Requirement: Dependency-free default

Changes MUST use Git's unified diff as its default display and MUST NOT require
an optional display command.

#### Scenario: No display configuration exists

- **WHEN** the user renders a repository comparison
- **THEN** Changes prints the Git diff with semantic context when available

### Requirement: Built-in layouts

The built-in engine MUST support unified and side-by-side layouts and MUST
respect the selected output width.

#### Scenario: Side-by-side is selected

- **WHEN** `diff.layout` or `CHANGES_DIFF_LAYOUT` is `side-by-side`
- **THEN** Changes renders paired old and new lines within the output width

### Requirement: Color policy

Changes MUST support `auto`, `always`, and `never`. `auto` MUST emit color only
when standard output is a terminal, and `never` MUST remove ANSI styling.

#### Scenario: Output is piped with automatic color

- **WHEN** color is `auto` and standard output is not a terminal
- **THEN** Changes emits plain text

### Requirement: Patch filters

The filter engine MUST pass one patch to a configured command on standard input.
It MUST NOT accept Git file placeholders.

#### Scenario: Filter contains a file placeholder

- **WHEN** a filter contains `$LOCAL`, `$REMOTE`, or `$MERGED`
- **THEN** Changes rejects the filter and directs the user to the difftool setting

### Requirement: Git difftool compatibility

`changes difftool` MUST compare `$LOCAL` and `$REMOTE`, MAY pass `$MERGED`, and
MUST append the two file paths when no file placeholder exists.

#### Scenario: External tool finds differences

- **WHEN** the configured difftool exits with status 1
- **THEN** Changes treats the comparison as successful

#### Scenario: External tool fails

- **WHEN** the configured difftool exits with any other non-zero status
- **THEN** Changes reports the command failure

### Requirement: Watch output

Watch mode MUST rerender only when its complete frame changes and MUST leave
rendered frames in ordinary terminal scrollback.

#### Scenario: Repository state is unchanged

- **WHEN** a watch interval passes without a changed frame
- **THEN** Changes emits no duplicate frame
