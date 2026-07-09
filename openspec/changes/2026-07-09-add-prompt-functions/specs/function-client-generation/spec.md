# function-client-generation Specification

## Purpose

Make declared functions part of the typed client contract the builder authors against — one
generated method per function, parameter types derived from the template itself — so a
frontend can use an app's LLM behaviour through the same "go through the client, never
around it" rule as CRUD, in both the production generator and the agent's hand-synced
mirror.

## Requirements

### Requirement: Declared functions generate a typed sub-client

Given a manifest with functions, the generated client SHALL contain a functions sub-client
with one method per function in manifest order, exposed as `readonly functions` on the root
client. An app with no functions SHALL generate no functions surface at all.

#### Scenario: The root client exposes functions

- **WHEN** a manifest declares any function
- **THEN** the generated root client carries `readonly functions` wired to the sub-client

#### Scenario: No functions, no sub-client

- **WHEN** a manifest declares no functions
- **THEN** the generated client contains no functions sub-client and no functions field

### Requirement: Prompt-function parameter types come from the template

For each prompt function with placeholders, generation SHALL emit a params interface with
one required string property per placeholder, in order of first appearance, and a method
taking that interface and resolving to the model's text (a string). A placeholder-free
prompt function's method SHALL take no arguments. A function's description SHALL become the
method's doc comment.

#### Scenario: Placeholders become the params interface

- **WHEN** a prompt function's template reads "Summarize in a {{tone}} tone: {{text}}"
- **THEN** the generated interface has exactly `tone: string` and `text: string`, and the
  method signature is `(params: <Name>Params) => Promise<string>`

### Requirement: Wasm functions stay untyped at the boundary

A wasm function's method SHALL take `unknown` input and resolve to `unknown`, mirroring the
runtime contract (raw bytes in, guest-defined JSON or text out).

#### Scenario: A wasm function generates an unknown-typed method

- **WHEN** a manifest declares a wasm function
- **THEN** its generated method is `(input: unknown) => Promise<unknown>`

### Requirement: Generation stays deterministic and mirrored

Generation SHALL remain a pure function of the schema model — an unchanged manifest yields
byte-identical output — and the agent's TypeScript generator SHALL emit the same surface
shapes so frontends authored against the stub client survive the swap to the production
client.

#### Scenario: Determinism holds with functions declared

- **WHEN** the same function-bearing manifest is generated twice
- **THEN** the outputs are byte-identical
