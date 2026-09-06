# provider-runtime Specification

## Purpose

Define the implementation-neutral contract for optional Changes providers.

## Requirements

### Requirement: Provider independence

Changes core MUST know semantic actions, not provider implementations. A core
render MUST remain usable when no optional provider is installed.

#### Scenario: No provider is discovered

- **WHEN** the user runs Changes with only Git installed
- **THEN** Changes renders the comparison without semantic enrichment

### Requirement: Manifest discovery

Changes MUST discover `provider/v1` manifests through configured, XDG data,
executable-adjacent, and system data directories. The first valid manifest for
a name MUST win.

#### Scenario: Higher-precedence manifest is invalid

- **WHEN** an invalid manifest has the same name as a later valid manifest
- **THEN** Changes reports the invalid manifest and uses the valid manifest

### Requirement: Direct execution

Changes MUST render action arguments and environment values from the manifest,
then execute the resulting argument vector directly without an implicit shell.

#### Scenario: Action uses a relative executable

- **WHEN** a manifest action names an explicit relative executable
- **THEN** Changes resolves it from the manifest directory

### Requirement: Bounded provider execution

Changes MUST apply the action timeout and MUST reject provider output larger
than 16 MiB, invalid JSON, trailing JSON values, or invalid semantic content.

#### Scenario: Provider exceeds its timeout

- **WHEN** a provider does not finish before its effective timeout
- **THEN** Changes terminates the invocation and reports a timeout

### Requirement: Provider validation

`changes provider validate` MUST check the manifest, host requirements, every
advertised Changes action, and action-specific semantic output in an isolated
temporary repository.

#### Scenario: Provider output is structurally valid but semantically empty

- **WHEN** validation cannot find the expected fixture result
- **THEN** the action check fails

### Requirement: Analysis cache

Changes MUST cache symbol and call analysis by provider definition, executable
identity, relevant environment, action, and comparison input. It MUST bound
cached results by lifetime and entry count.

#### Scenario: Provider implementation changes

- **WHEN** an executable or provider-owned argument file changes
- **THEN** Changes does not reuse the prior cached result

#### Scenario: Caching is disabled

- **WHEN** cache lifetime or maximum entries is zero
- **THEN** Changes executes the provider and stores no result
