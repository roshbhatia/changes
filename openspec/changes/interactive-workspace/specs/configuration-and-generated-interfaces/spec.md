## MODIFIED Requirements

### Requirement: Generated schemas and documentation

`changes generate` MUST render the JSON configuration schema, provider schema, workspace snapshot schema, README command reference, and published completion files. `--check` MUST fail when any generated artifact differs.

#### Scenario: Generated file is stale

- **WHEN** `changes generate --check` finds drift
- **THEN** it exits unsuccessfully without rewriting the file

## ADDED Requirements

### Requirement: Interactive configuration

Configuration MUST set defaults for navigator form, dock, note input, history limit, snapshot cache bounds, and progress. Command flags MUST override those defaults through the common command model.

#### Scenario: Environment overrides navigator form

- **WHEN** YAML selects tree and `CHANGES_INTERACTIVE_NAVIGATOR=list`
- **THEN** interactive mode starts with the list form unless repository state has a newer explicit selection
