## Purpose

Define a small Neovim adapter that collects an annotation near the cursor and invokes the stable Changes CLI without duplicating note storage logic.

## ADDED Requirements

### Requirement: Standard Neovim entry points

The adapter MUST expose `:ChangesNote`, a Lua function with an options table, and normal and visual `<Plug>` mappings. It MUST NOT install a user keybinding.

#### Scenario: Cursor annotation

- **WHEN** a user invokes `:ChangesNote` without a range
- **THEN** the adapter targets the current buffer path and cursor line on the right side

#### Scenario: Visual annotation

- **WHEN** a user invokes the visual `<Plug>` mapping over a line range
- **THEN** the adapter sends the ordered start and end lines as one note range

#### Scenario: The target buffer has unsaved changes

- **WHEN** a user invokes an annotation from a modified buffer
- **THEN** the adapter rejects the operation before starting Changes and asks the user to save the buffer

#### Scenario: The buffer view differs from its saved file

- **WHEN** an ordinary buffer reports no modification but its bounded content differs from the regular disk file
- **THEN** the adapter rejects the operation before starting Changes

#### Scenario: The target is not an ordinary UTF-8 file buffer

- **WHEN** the buffer uses a special type, byte-order mark, unsupported encoding, or non-regular disk path
- **THEN** the adapter rejects the operation before reading it as annotation content

### Requirement: UI-provider-compatible prompt

The popup MUST use `vim.ui.input` with line scope and MUST treat cancellation or empty input as no write.

#### Scenario: A UI plugin overrides input

- **WHEN** the user has configured a `vim.ui.input` implementation
- **THEN** that implementation owns the popup presentation

#### Scenario: The user switches buffers while input is open

- **WHEN** the input callback completes after the user moves to another buffer or repository
- **THEN** the adapter uses the buffer, path, directory, and line range captured when the prompt opened

#### Scenario: Another process changes the target file while input is open

- **WHEN** the final pre-launch validation observes that the file changed while input was open
- **THEN** the adapter rejects the operation before starting Changes

#### Scenario: The saved file differs from the selected comparison side

- **WHEN** the adapter targets a working, index, or commit side whose content does not match the captured file digest
- **THEN** Changes rejects the operation before asking the provider to create a note

#### Scenario: The comparison changes during capture

- **WHEN** endpoint IDs, patch content, or the selected-side digest differs between repeated tuple reads
- **THEN** Changes retries the complete capture or rejects it without creating a note

#### Scenario: Git would transform working content

- **WHEN** a working-side file uses a Git clean filter or working-tree encoding
- **THEN** Changes rejects the adapter digest because its line domain is not stable

#### Scenario: The target file exceeds the validation limit

- **WHEN** the target file is larger than 16 MiB
- **THEN** the adapter rejects the operation before reading its contents or starting Changes

#### Scenario: The target uses a valid newline form

- **WHEN** an ordinary UTF-8 target is empty, one newline, missing its final newline, or uses CRLF
- **THEN** the adapter hashes the same bytes that the buffer represents on disk

#### Scenario: The user cancels the prompt

- **WHEN** the input callback receives no value or an empty value
- **THEN** the adapter does not start `changes`

### Requirement: Shell-free CLI invocation

The adapter MUST call `vim.system` with an argument vector, send the note body through standard input, pass the captured file SHA-256 digest, and request JSON output.

#### Scenario: Note text contains shell syntax

- **WHEN** the submitted note includes spaces, quotes, command substitutions, or newlines
- **THEN** the text reaches `changes note add --message-file - --json` as data

#### Scenario: Buffer path contains spaces

- **WHEN** the current file path contains spaces
- **THEN** the complete path remains one argument

### Requirement: Observable completion

The adapter MUST decode a successful note response and MUST expose failures to both the callback and Neovim notification layer.

#### Scenario: Note creation succeeds

- **WHEN** `changes` exits zero with one valid note object
- **THEN** the callback receives that note and the user sees its identifier

#### Scenario: Note creation fails

- **WHEN** `changes` cannot start, exits non-zero, or returns invalid JSON
- **THEN** the callback receives an error and no success message appears
