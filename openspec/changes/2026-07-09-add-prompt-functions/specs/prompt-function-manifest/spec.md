# prompt-function-manifest Specification

## Purpose

Let a manifest declare LLM behaviour as data — a prompt template and its parameters — under a
contract closed enough that the planner agent can author it, the validator can fully check
it, and the runtime can render it with no parser ambiguity and no capability beyond the
model broker.

## Requirements

### Requirement: A function is exactly one of two kinds

The manifest schema SHALL accept a function as either a wasm function (an `entry` naming a
pre-built module) or a prompt function (a non-empty `prompt` template), and SHALL reject a
function declaring both, neither, or any undeclared key. Both kinds MAY carry an optional
`description`.

#### Scenario: A function with both entry and prompt is rejected structurally

- **WHEN** a manifest declares a function carrying both `entry` and `prompt`
- **THEN** structural validation rejects the manifest before it ever parses into the model

#### Scenario: A function with neither entry nor prompt is rejected structurally

- **WHEN** a manifest declares a function with `id`, `name` and `capabilities` only
- **THEN** structural validation rejects the manifest

### Requirement: A prompt function declares exactly the model capability

The manifest schema SHALL require a prompt function's capabilities to be exactly
`{"model": true}` — no data scopes, no network domains, no `model: false` — because a
function with no code can hold no other power, and the explicit declaration keeps the
derived consent union a pure function of the manifest.

#### Scenario: A prompt function with a data scope is rejected

- **WHEN** a prompt function declares `capabilities.data`
- **THEN** structural validation rejects the manifest

#### Scenario: A prompt function without model true is rejected

- **WHEN** a prompt function declares `{"model": false}` or empty capabilities
- **THEN** structural validation rejects the manifest

### Requirement: The template syntax is closed with no escaping

A prompt template's parameters SHALL be exactly its well-formed `{{param}}` placeholders
(machineName rule, optional interior whitespace), deduplicated in order of first appearance.
Validation SHALL reject any `{{` in the template that does not begin a well-formed
placeholder, and there SHALL be no escaping mechanism. A template with no placeholders is
valid (a static prompt); a repeated placeholder is valid.

#### Scenario: A malformed placeholder is a validation error

- **WHEN** a prompt template contains `hello {{Name}}` or an unclosed `hello {{name`
- **THEN** semantic validation reports `malformed_placeholder` at the function's prompt path

#### Scenario: Placeholder order and deduplication are deterministic

- **WHEN** a template reads `{{b}} {{a}} {{b}}`
- **THEN** the function's parameters are exactly ["b", "a"]

### Requirement: The entity name "functions" is reserved

Validation SHALL reject an entity whose name (or id) is "functions", because entity names
are URL segments under /apps/{app}/ and would shadow the function-invocation route.

#### Scenario: An entity named functions is rejected

- **WHEN** a manifest declares an entity named "functions"
- **THEN** semantic validation reports `reserved_name` at the entity's name path

### Requirement: A wasm entry stays inside the app directory

Validation SHALL reject a wasm function whose entry is an absolute path, is not a clean
relative path, or path-escapes the app directory, because the runtime resolves the entry
relative to the app's own directory.

#### Scenario: An escaping entry is rejected

- **WHEN** a wasm function declares an entry of `/etc/passwd` or `../outside.wasm`
- **THEN** semantic validation reports `bad_entry` at the function's entry path
