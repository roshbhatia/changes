## Purpose

Define progress feedback for work that would otherwise leave a terminal silent.

## ADDED Requirements

### Requirement: TTY-only progress

Non-difftool work SHOULD show an animation on a terminal error stream and MUST keep standard output unchanged.

#### Scenario: Error output is redirected

- **WHEN** standard error is not a terminal
- **THEN** Changes writes no animation

### Requirement: Explicit suppression

Commands with progress MUST accept `--quiet` and MUST stop the animation before final output or an error.

#### Scenario: Quiet mode is selected

- **WHEN** the user supplies `--quiet`
- **THEN** Changes emits no progress frames

### Requirement: Difftool exclusion

`changes difftool` MUST NOT start the progress animation.

#### Scenario: Git invokes Changes as its difftool

- **WHEN** Changes compares local and remote files
- **THEN** its output contains only the configured diff result and diagnostics
