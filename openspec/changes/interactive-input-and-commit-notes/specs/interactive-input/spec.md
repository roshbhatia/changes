## Purpose

Define consistent keyboard and mouse behavior for panes, tabs, selections, and actions in the interactive Changes workspace.

## ADDED Requirements

### Requirement: Spatial pane navigation

Interactive mode MUST use `Ctrl-h/j/k/l` to move focus toward the pane in that direction. A direction without an adjacent pane MUST leave focus unchanged.

#### Scenario: Explorer is docked left

- **WHEN** focus is in Changes and the user presses `Ctrl-h`
- **THEN** focus moves to Explorer, and `Ctrl-l` moves it back

#### Scenario: Explorer is docked below

- **WHEN** focus is in Changes and the user presses `Ctrl-j`
- **THEN** focus moves to Explorer, and `Ctrl-k` moves it back

### Requirement: Pane-local tab navigation

`Tab` and `Shift-Tab` MUST move forward and backward through tabs inside the focused pane. They MUST NOT move focus between panes.

#### Scenario: Explorer has Files and History tabs

- **WHEN** Explorer is focused and the user presses `Tab`
- **THEN** the next Explorer tab becomes active while Explorer keeps focus

#### Scenario: Focused pane has one tab

- **WHEN** the user presses `Tab` in a pane with no alternate tab
- **THEN** focus and visible state remain unchanged

### Requirement: Mouse interaction

Interactive mode MUST enable terminal mouse reporting and restore it on exit. A click MUST focus its pane, select a visible row, or activate a visible tab as appropriate. A wheel event MUST scroll the pane under the pointer.

#### Scenario: User clicks History

- **WHEN** the user clicks the visible History tab
- **THEN** History becomes active, Explorer remains focused, and no comparison opens

#### Scenario: User wheels over Changes

- **WHEN** the pointer is over the Changes pane and the user scrolls the wheel
- **THEN** only the Changes viewport scrolls

### Requirement: Commit multi-selection

History MUST let the user toggle any number of commits without moving the cursor or changing Git state. The UI MUST distinguish selected commits from the cursor.

#### Scenario: User selects separate commits

- **WHEN** the user toggles two non-adjacent history entries
- **THEN** both full commit identities remain selected while the user continues navigating

