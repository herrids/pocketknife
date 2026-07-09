# Add prompt functions: LLM-powered apps end to end

## Why

The sandboxed-function runtime (sandbox/, broker/, consent/) shipped complete but unreachable:
no HTTP route invoked a function, the server never constructed a broker or a sandbox, and no
real model provider existed behind the broker's generic caller. Worse, the one authoring
pipeline that actually builds apps — the planner/builder agent — can only produce a JSON
manifest and a React frontend, never a compiled `.wasm` module, so even a wired-up wasm path
would have left agent-built apps with no way to use a model. Apps that are built should be
able to be intelligent by utilizing an LLM; this change closes that gap end to end.

## What Changes

- **Prompt functions: a second, declarative function kind.** A manifest function is now
  either wasm (`entry`, unchanged) or a *prompt function* (`prompt`): an LLM prompt template
  with `{{param}}` placeholders, rendered server-side by verbatim substitution and sent
  through the model broker. No code runs; the closed template contract (machineName params,
  no escaping, malformed `{{` rejected) is the entire attack surface. A prompt function must
  declare exactly `{"model": true}` — structurally enforced — so `consent.Union` stays
  accurate with zero changes.
- **One invoke route serves both kinds.** `POST /apps/{app}/functions/{name}` executes a
  prompt function (JSON object of string params in, model text out) or a wasm function (raw
  body in, guest bytes out) behind the existing error envelope, with a closed error-code
  table (unknown function, invalid params, model not configured, timeout, crash, guest
  failure). "functions" becomes a reserved entity name so no manifest can shadow the route.
- **A real provider behind the broker.** `broker.NewAnthropicCaller` speaks the Anthropic
  Messages API; the server selects it from `ANTHROPIC_API_KEY` at boot (generic
  `POCKETKNIFE_MODEL_ENDPOINT` caller as fallback, unconfigured broker → 503 otherwise).
  Token hygiene is unchanged: the key is read once, held unexported, and has no path out.
- **The typed client exposes functions.** `client.Generate` (and the agent's hand-synced
  mirror) emit a `functions` sub-client: one method per function, a `<Name>Params` interface
  derived from the template's placeholders, prompt functions resolving to `Promise<string>`.
- **The agent authors intelligent apps.** The planner skill now teaches prompt functions
  (and forbids wasm ones, which the agent cannot produce); the builder skill teaches calling
  `client.functions.<name>()` with loading states and graceful model-not-configured handling;
  the agent's schema copy, manifest types, semantic mirror and client generator all learn the
  new shape.
- **A version bump that only adds a function now deploys correctly.** `migrate.Apply`'s
  empty-diff early return promoted nothing: a redeploy whose only change was a new function
  activated the frontend against the old schema and reverted on restart. The empty-diff path
  now promotes the manifest and re-registers when the version moved (still reporting
  NoChange, which describes the data).

## Capabilities

### New Capabilities
- `prompt-function-manifest`: the declarative prompt-function kind — the wasm/prompt oneOf,
  the `{{param}}` template contract, the exactly-model capability rule, and the reserved
  "functions" entity name.
- `function-invoke-api`: the HTTP seam that executes a declared function of either kind and
  maps every failure class onto the API's error envelope without leaking server-side detail.
- `anthropic-provider`: the Anthropic Messages API caller behind the broker, selected from
  the host environment at boot, with the same token-custody invariants as the generic caller.
- `function-client-generation`: the typed `functions` sub-client in the generated TypeScript
  client (and the agent's mirrored generator).

### Modified Capabilities
- `schema-migration`: an empty entity diff with a moved version now promotes the manifest and
  re-registers the schema instead of silently keeping the old version.

## Impact

- **New code:** `funcrun/` (prompt render + wasm invoke runner), `api/functions.go` (the
  invoke route), `broker/anthropic.go`, `schema/prompt.go` (the single placeholder-syntax
  definition), `cmd/pocketknife` broker/sandbox wiring.
- **Modified code:** `manifest.schema.json` (+ the agent's copy) splits `function` into a
  wasm/prompt oneOf and adds `description`; `schema.Function` gains `Prompt`/`Description`;
  `validate/semantic.go` adds reserved-entity-name, malformed-placeholder and wasm-entry-path
  checks; `api.NewServer` takes the runner; `client/generate.go` and the agent seams emit the
  functions sub-client; `migrate/apply.go` promotes on an empty diff with a version bump;
  both agent skills.
- **Data:** none. Prompt functions hold no state; nothing new is persisted beyond the
  manifest's own `functions` array.
- **Dependencies:** none added — the Anthropic caller is hand-rolled net/http, keeping the
  entire token-custody surface auditable in one small file.
- **Out of scope, left as clean seams:** wasm bundle upload through `POST /deploy` (prompt
  functions ship no files, so deployapi is untouched); the consent UI (still Phase 6);
  streaming model responses; per-function rate limits; authenticating the invoke route beyond
  what the session layer provides.
