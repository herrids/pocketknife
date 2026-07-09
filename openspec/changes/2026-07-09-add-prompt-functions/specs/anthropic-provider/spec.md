# anthropic-provider Specification

## Purpose

Put a real model provider behind the broker seam — the Anthropic Messages API — selected
from the host process's own environment at boot, under exactly the token-custody invariants
the broker already guarantees: the key is read once, held unexported, and has no code path
back out to a function, a response, or the browser.

## Requirements

### Requirement: The caller speaks the Anthropic Messages API

The system SHALL provide a broker Caller that POSTs a single-turn user message to the
Messages API with the configured key (`x-api-key`) and API version header, requests adaptive
thinking, and returns the response's text blocks joined as one string.

#### Scenario: A prompt round-trips through the Messages API shape

- **WHEN** the caller is invoked with a prompt
- **THEN** the outbound request carries the key header, the version header, the configured
  model and the prompt as one user message, and the returned string is the response's text
  content

### Requirement: Provider selection happens once, from the environment

The server SHALL select the provider at boot: `ANTHROPIC_API_KEY` selects the Anthropic
caller (model from `POCKETKNIFE_MODEL`, defaulting sensibly), else
`POCKETKNIFE_MODEL_ENDPOINT`/`POCKETKNIFE_MODEL_TOKEN` select the generic HTTP caller, else
the broker is unconfigured and every model call fails closed as not-configured.

#### Scenario: No credentials means fail-closed, not fail-open

- **WHEN** the server boots with none of the provider variables set
- **THEN** invoking a prompt function answers 503 `model_not_configured` and no outbound
  call is attempted

### Requirement: The key never surfaces

The caller SHALL hold the key in an unexported field with no accessor, JSON tag or String
method, and SHALL NOT include the key in any error it returns, including provider-rejection
errors that quote a status.

#### Scenario: A provider rejection does not echo the key

- **WHEN** the provider answers 401 with an error body
- **THEN** the caller's error names the status but contains no part of the key

### Requirement: A refusal is an error, not empty success

The caller SHALL treat a `refusal` stop reason (and a response with no text content) as an
error, so a declined request can never masquerade as a legitimate empty model answer.

#### Scenario: A refusal surfaces as a failed call

- **WHEN** the provider returns stop_reason "refusal"
- **THEN** the caller returns an error and the invoke route answers 502
