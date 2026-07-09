# Design

## Why a second function kind instead of wiring wasm alone

The agent (the only pipeline that builds apps) authors JSON and TypeScript; it cannot compile
WebAssembly, and pocketknife never compiles on-box by invariant. Wiring only the wasm path
would have made the runtime reachable while leaving every agent-built app exactly as
unintelligent as before. A prompt function is the smallest declarative unit the planner can
author that yields real LLM behaviour: a template, its parameters, and nothing else.

## The template contract is closed

One regex — `\{\{\s*([a-z][a-z0-9_]*)\s*\}\}`, defined once in `schema/prompt.go` and
mirrored in the agent's TypeScript seams — is the whole syntax. Params follow the manifest's
machineName rule; order of first appearance drives the generated client's params interface;
substitution is verbatim string insertion; there is no escaping, and any `{{` that does not
begin a well-formed placeholder is a validation error (`malformed_placeholder`). Closing the
syntax this hard keeps rendering trivially auditable and leaves no parser edge for a
malicious manifest to probe.

## Structure over convention: the oneOf and the exactly-model rule

Wasm-vs-prompt is enforced structurally (`function` becomes a `oneOf` of two closed variants,
mirroring the existing field-type union) rather than semantically, so a manifest with both
`entry` and `prompt`, or neither, never parses into the model at all. Likewise a prompt
function's capabilities are a dedicated `promptCapabilities` def — `required: ["model"]`,
`model: {const: true}`, `additionalProperties: false` — because a function with no code can
hold no other power, and requiring the declaration explicitly (rather than implying it) keeps
`consent.Union` a pure function of what the manifest says with zero changes to consent/.

## Where execution lives

`funcrun` is a new package between the HTTP surface and the two runtimes, so `api` needs no
sandbox/broker knowledge beyond error classification, and `sandbox` keeps not importing
`api` (the cycle that shaped data.go originally). The runner resolves a wasm entry via
`filepath.Join(app dir, entry)` — which is why the validator now rejects absolute or
`..`-escaping entries — and passes broker errors and sandbox sentinels through untouched for
the handler to map. The invoke route must live on `api`'s own mux (main.go mounts exactly one
handler at `/apps/`), where the literal `functions` segment outranks the `{entity}` wildcard;
reserving "functions" as an entity name closes the shadowing hole that wildcard left.

## Error mapping is class-only

The handler maps each failure to a code (`invalid_params`, `model_not_configured`,
`function_timeout`, `function_crashed`, `model_call_failed`, …) and a fixed message. Provider
error text, trap reasons and module paths never reach the response: the broker's failure
detail could quote provider internals, and a wasm trap string could leak host paths. The one
deliberate exception is a guest's own output on `function_failed` (422) — the guest wrote
those bytes for its caller.

## Provider selection at boot, not per-request

`brokerFromEnv` runs once in `runServe`: `ANTHROPIC_API_KEY` wins, then the generic
endpoint/token pair, then an unconfigured broker that answers every model call with
`ErrNotConfigured` (503 at the seam). The Anthropic caller is hand-rolled net/http rather
than an SDK dependency so the token's entire custody surface stays in one page of code, with
the same unexported-field discipline as the generic caller. `max_tokens` is fixed and
generous; `thinking` is adaptive; text blocks are joined and a `refusal` stop reason is an
error, never empty success.

## The empty-diff promotion fix

`Diff` is entity-only by design, so `migrate.Apply` treated "no entity ops" as "nothing to
do" and returned before promoting the manifest or re-registering — correct for data, wrong
for everything else a manifest declares. The fix promotes and re-registers when the version
moved while keeping `NoChange: true`, because the flag describes the data migration and
`build.Deploy` re-reads the registration after Apply either way. Without this, the feature's
primary flow (redeploying an existing app with its first prompt function) silently no-ops.

## Version-bump requirement, documented not enforced

A same-version redeploy is `KindInstall` and never re-reads the manifest, so adding a
function requires bumping `app.version`. That is an existing property of the deploy pipeline,
not a new rule; the planner skill states it explicitly so update-mode manifests get it right.
