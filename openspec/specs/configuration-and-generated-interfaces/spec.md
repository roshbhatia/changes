# configuration-and-generated-interfaces Specification

## Purpose

Define stable configuration, help, schema, and completion surfaces.

## Requirements

### Requirement: Configuration precedence

Changes MUST start from dependency-free defaults, load YAML configuration, and
apply `CHANGES_*` environment overrides last.

#### Scenario: Environment overrides YAML

- **WHEN** YAML selects unified layout and `CHANGES_DIFF_LAYOUT=side-by-side`
- **THEN** Changes uses the side-by-side layout

### Requirement: Configurable file location

Changes MUST load `~/.config/changes/config.yaml` by default and MUST support an
explicit path through `--config` or `CHANGES_CONFIG`.

#### Scenario: Explicit config is selected

- **WHEN** the user supplies `--config custom.yaml`
- **THEN** Changes loads that file instead of the default path

### Requirement: One command model

Runtime help, README command documentation, and shell completions MUST derive
from one command metadata model.

#### Scenario: A command option is added

- **WHEN** the command model gains an option
- **THEN** generated help, documentation, and completions include it

### Requirement: Published completions

Changes MUST generate completions for Bash, Zsh, Fish, and Nushell. Dynamic
values MUST preserve paths as data and MUST omit values containing newlines.

#### Scenario: Repository path contains spaces

- **WHEN** completion queries Git for that path
- **THEN** the generated shell integration preserves the complete path

### Requirement: Generated schemas and documentation

`changes generate` MUST render the JSON configuration schema, provider schema,
README command reference, and published completion files. `--check` MUST fail
when any generated artifact differs.

#### Scenario: Generated file is stale

- **WHEN** `changes generate --check` finds drift
- **THEN** it exits unsuccessfully without rewriting the file

### Requirement: Provider inspection

`changes provider list` MUST show valid providers and diagnostics.
`changes provider validate [name]` MUST validate all providers or one named
provider and MUST support machine-readable JSON output.

#### Scenario: Named provider does not exist

- **WHEN** the user validates an unknown provider name
- **THEN** Changes reports that no provider matched
