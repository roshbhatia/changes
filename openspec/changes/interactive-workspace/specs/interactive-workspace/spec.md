## Purpose

Define a persistent terminal workspace for reading and annotating cohesive Git comparisons.

## ADDED Requirements

### Requirement: Interactive terminal boundary

`changes interactive` MUST require terminal input and output, use the alternate screen, and restore the terminal on exit or external editor completion.

#### Scenario: Output is redirected

- **WHEN** a user starts interactive mode without terminal input or output
- **THEN** Changes rejects the invocation and directs the user to `changes workspace`

### Requirement: Cohesive review pane

The main pane MUST render logical groups, paths, symbols, hunks, diff lines, notes, and note replies as one tree. It MUST NOT move notes into a separate context section.

#### Scenario: A line has a review thread

- **WHEN** the current comparison places several replies on one line
- **THEN** the complete thread appears directly below that diff line

### Requirement: Navigator layouts

The navigator MUST provide Files and History tabs. Files MUST toggle between tree and list forms. The navigator MUST dock left or bottom and preserve main-pane focus.

#### Scenario: User changes navigator layout

- **WHEN** the user selects tree on the bottom, list on the bottom, tree on the left, or list on the left
- **THEN** the frame adopts that combination without rebuilding provider data

### Requirement: Git view switching

Interactive mode MUST switch among working, staged, and selected first-parent commit comparisons without changing Git state.

#### Scenario: User selects a history entry

- **WHEN** the user activates one commit
- **THEN** the main pane shows its first-parent diff and the header names both endpoints

### Requirement: Diff layout switching

Interactive mode MUST toggle unified and side-by-side rendering from the current snapshot analysis.

#### Scenario: User toggles layout

- **WHEN** the active layout changes
- **THEN** file, history, cursor, and note state remain selected

### Requirement: Interactive note authoring

Interactive mode MUST support popup and editor input over the configured note writer. It MUST target the selected file or visible diff line and report completion.

#### Scenario: User cancels note input

- **WHEN** popup or editor input produces no note
- **THEN** Changes returns to the workspace without a success message

### Requirement: Per-repository session state

Changes MUST restore layout, navigation, active tab, selected view, selected path, and command history per repository from XDG state storage.

#### Scenario: User reopens a repository

- **WHEN** a prior valid workspace state exists
- **THEN** interactive mode restores it before the first frame

### Requirement: Command surface

Interactive mode MUST provide discoverable key help and a `:` command line for view, layout, dock, navigator, refresh, note, and quit actions.

#### Scenario: User abbreviates a command ambiguously

- **WHEN** a prefix matches more than one command
- **THEN** Changes reports the ambiguity and runs no action
