## 1. Schema model and manifest contract

- [x] 1.1 `schema.Function` gains `Prompt` and `Description`; `IsPrompt()` and
      `PromptParams()` helpers; `schema/prompt.go` holds the single placeholder regex,
      `ScanPrompt` (params + malformed offsets) and `RenderPrompt`
- [x] 1.2 `schema.ReservedEntityNames` (["functions"]) next to the field-level ReservedNames
- [x] 1.3 `manifest.schema.json`: `function` becomes a oneOf of `function_wasm` /
      `function_prompt`; `promptCapabilities` requires exactly `model: true`; optional
      `description` on both kinds; byte-identical copy to `agent/schema/manifest.schema.json`
- [x] 1.4 `schema/parse.go` carries `prompt`/`description` through to the model
- [x] 1.5 Tests: ScanPrompt table (dedup, order, malformed offsets), render round-trip

## 2. Validation

- [x] 2.1 Reserved entity name/id check (`reserved_name`/`reserved_id` at /entities/{i})
- [x] 2.2 `malformed_placeholder` for every bad `{{` in a prompt template
- [x] 2.3 `bad_entry` for a wasm entry that is absolute, unclean, or escapes the app dir
- [x] 2.4 Tests: valid prompt/wasm functions; structural rejections (both kinds at once,
      neither, model:false, data/network on a prompt function); every new semantic code

## 3. Anthropic provider

- [x] 3.1 `broker.NewAnthropicCaller(apiKey, model)`: Messages API, x-api-key +
      anthropic-version headers, adaptive thinking, text-block join, refusal → error;
      unexported baseURL seam for tests; `DefaultAnthropicModel`
- [x] 3.2 Tests: request shape, model override, non-200 without echoing the key, refusal,
      json.Marshal never surfaces the key (mirroring the generic caller's suite)

## 4. funcrun runner

- [x] 4.1 `Runner{sandbox, broker}` with `Run(ctx, app, fn, input)`; prompt path validates
      params against the template (all issues at once), renders, calls the broker; wasm path
      compiles via the sandbox cache and invokes with the app's store/schema
- [x] 4.2 `BadInputError` (param issues), `ErrLoad` (module read/compile), sandbox sentinels
      passed through; guest `Failed` is an outcome, not an error
- [x] 4.3 Tests: render + param accumulation, nil broker → ErrNotConfigured, wasm echo
      round-trip and guest-failure via the shared sandbox guest fixture, missing module

## 5. Invoke API

- [x] 5.1 `POST /apps/{app}/functions/{name}` on the api mux; `api.NewServer(reg, runner)`;
      nil runner → 503 functions_unavailable
- [x] 5.2 Error table: app_not_found, function_not_found, invalid_body, invalid_params
      (with per-param details), input_too_large, model_not_configured, model_call_failed,
      function_load_failed, function_timeout, function_crashed, function_failed (422 with
      guest output)
- [x] 5.3 Success envelope `{"output": ...}`: model text as a string; wasm bytes embedded as
      JSON when valid, string otherwise
- [x] 5.4 Tests: full round-trip against a stub Caller, every error row, route precedence
      (entity CRUD unaffected; invoke not swallowed by entity create), provider detail never
      leaks into the response

## 6. Server wiring

- [x] 6.1 `brokerFromEnv` (ANTHROPIC_API_KEY → Anthropic; POCKETKNIFE_MODEL_ENDPOINT/_TOKEN →
      generic; else unconfigured) with one boot log line naming the mode
- [x] 6.2 One `sandbox.New` per process, closed on shutdown; runner threaded into
      `api.NewServer`; migrate/build subcommands untouched

## 7. Empty-diff promotion fix

- [x] 7.1 `migrate.Apply`: when the changeset is empty but the version moved, promote
      manifest.json and re-register before returning NoChange
- [x] 7.2 Tests at the migrate layer (function-only bump → new schema registered, manifest
      promoted, data untouched) and the build layer (second deploy adding only a prompt
      function ends ready with the function registered)

## 8. Client generation (Go + agent mirror)

- [x] 8.1 `client/generate.go`: `<Name>Params` interfaces from template placeholders,
      `<AppId>FunctionsClient` with one method per function (prompt → Promise<string>,
      wasm → unknown), `readonly functions` on the root client, description as JSDoc
- [x] 8.2 `agent/src/seams/generate-client.ts` mirrors 8.1 byte-shape for byte-shape
- [x] 8.3 Tests: substrings + determinism with functions; no functions → no sub-client

## 9. Agent seams and skills

- [x] 9.1 `manifest-types.ts`: WasmFunctionDecl | PromptFunctionDecl union, isPromptFunction,
      scanPrompt/promptParams mirror, RESERVED_ENTITY_NAMES
- [x] 9.2 `semantic.ts`: reserved entity names, malformed placeholders, entry-path mirror
- [x] 9.3 pocketknife-manifest SKILL.md: prompt-function contract, worked example, never emit
      wasm functions, version-bump-on-function-change rule
- [x] 9.4 pocketknife-frontend SKILL.md: client.functions usage, loading states, graceful
      model_not_configured handling
- [x] 9.5 Verified end-to-end: StubValidator accepts the prompt-function manifest and emits
      the functions client; rejects prompt+entry, model:false, malformed placeholder,
      reserved entity name

## 10. Gate

- [x] 10.1 `go build ./... && go vet ./... && gofmt -l . && go test ./...` green
- [x] 10.2 Agent `tsc --noEmit` and test suite green (one pre-existing flaky
      bridge.test.ts stdout-capture race noted, unrelated)
- [x] 10.3 Manual end-to-end: server booted with a stub provider; prompt function invoked
      over HTTP; error rows exercised
