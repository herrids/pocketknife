# function-invoke-api Specification

## Purpose

Give every app one HTTP seam — POST /apps/{app}/functions/{name} — that executes a declared
function of either kind and translates every failure class onto the API's error envelope,
without leaking provider detail, trap reasons or module paths, and without letting any
manifest shadow or weaken the route.

## Requirements

### Requirement: A prompt function round-trips parameters to model text

The system SHALL execute a prompt function by validating the request body (a JSON object of
string parameters) against the template's placeholders, rendering the template by verbatim
substitution, calling the model broker, and returning 200 with `{"output": <text>}`.

#### Scenario: A parameterised prompt function returns the model's text

- **WHEN** POST /apps/a/functions/summarize receives `{"tone": "cheerful", "text": "…"}`
  and the broker's provider answers
- **THEN** the response is 200 with the provider's text as the JSON-string `output`, and the
  provider received the template with both values substituted

### Requirement: Parameter problems are reported completely, before any model call

The system SHALL reject a request whose body is not a JSON object with 400 `invalid_body`,
and SHALL reject missing, unknown, or non-string parameters with 400 `invalid_params`
carrying one detail per problem — all problems at once — without invoking the broker.

#### Scenario: Missing and unknown parameters are reported together

- **WHEN** a two-parameter function receives one wrong-typed parameter, no second parameter,
  and one undeclared parameter
- **THEN** the response is 400 `invalid_params` with three details and the provider is never
  called

### Requirement: Every failure class maps to a closed error code

The system SHALL answer: an unknown app or function with 404; an unconfigured broker with
503 `model_not_configured`; a provider failure with 502 `model_call_failed`; a module that
cannot load with 500 `function_load_failed`; a timed-out invocation with 504
`function_timeout`; a trapped or resource-exhausted invocation with 500 `function_crashed`;
an oversized body with 413 `input_too_large`; and a server with no runner with 503
`functions_unavailable`. Responses SHALL NOT carry provider error text, trap detail or
server paths.

#### Scenario: A provider failure does not leak its detail

- **WHEN** the provider fails with an error message quoting internal detail
- **THEN** the response is 502 `model_call_failed` with a fixed message that contains none
  of the provider's text

### Requirement: A guest-reported failure is the caller's information

The system SHALL answer a wasm function whose guest run returned nonzero with 422
`function_failed`, carrying the guest's own output as a detail — the guest wrote those bytes
for its caller, unlike host-side failure detail.

#### Scenario: A guest failure returns the guest's output

- **WHEN** a wasm function's guest reports failure with explanatory output
- **THEN** the response is 422 with that output in the error details

### Requirement: The route coexists with entity CRUD

The invoke route SHALL NOT alter any entity route's behaviour: entity CRUD under
/apps/{app}/{entity} continues to work, and the literal "functions" segment wins over the
entity wildcard (backed by the reserved entity name).

#### Scenario: Entity create and function invoke are served side by side

- **WHEN** an app declares an entity and a prompt function
- **THEN** POST /apps/{app}/{entity} creates a row and POST /apps/{app}/functions/{name}
  invokes the function
