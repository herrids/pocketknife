# Agent flow

One job, two short-lived Claude Agent SDK sessions, one orchestrator that owns the
only irreversible step. Nothing the models do can submit an app on its own.

```
            ┌──────────────────────────────────────────────────────────────────┐
            │                          cli.ts (terminal)                       │
            │  npm run agent -- "<prompt>" [--paste file] [--app <app_id>]     │
            └───────────────────────────┬──────────────────────────────────────┘
                                         │ new Orchestrator()
                                         │ [--app] → orchestrator.loadExistingApp()
                                         │           → AppSourceFetcher.fetch(appId)
                                         │           → backend GET /export/{appId}[/source]
                                         ▼
            ┌──────────────────────────────────────────────────────────────────┐
            │                     orchestrator.ts (job state)                  │
            │  jobId, scratchDir = .scratch/<jobId>/                           │
            │  lastValid: last manifest+client that ever passed validation     │
            │  readyToBuild: flips true only if lastValid is set               │
            │  updateAppId: set in update mode; enforced at submit             │
            └───────┬───────────────────────────────────────────┬──────────────┘
                    │ startPlanning / refinePlan                │ build()
                    ▼                                           ▼
   ┌────────────────────────────────────┐      ┌──────────────────────────────────────┐
   │   PLANNER SESSION (planner.ts)     │      │   BUILDER SESSION (builder.ts)       │
   │   Claude Agent SDK query(), resumed│      │   Claude Agent SDK query(), one-shot │
   │   across turns via session_id      │      │   cwd pinned to scratchDir           │
   │                                    │      │                                       │
   │   tools: AskUserQuestion, Skill    │      │   tools: Read, Write, Edit, Glob,    │
   │   + mcp tools (below)              │      │          Skill                       │
   │   skill: pocketknife-manifest      │      │   skill: pocketknife-frontend        │
   │                                    │      │   hook: PreToolUse = scratch-guard.ts│
   │   update mode: prompt includes     │      │                                       │
   │   <current-manifest> block +       │      │   update mode: scratchDir seeded     │
   │   stable-id preservation rules     │      │   from fetched source tree instead   │
   │                                    │      │   of blank scaffold (fallback to     │
   │   loop:                            │      │   scaffold if no source stored)      │
   │    1. draft/repair manifest        │      │                                       │
   │    2. call validate_manifest ──┐   │      │   reads manifest.json + client.ts    │
   │    3. on errors: repair, retry │   │      │   (written by orchestrator.build())  │
   │    4. on success: summarize    │   │      │   authors frontend files against the │
   │       in plain language        │   │      │   typed client only (no raw fetch)   │
   │    5. watch for user intent    │   │      │   scratch-guard hook denies any      │
   │       ("build it", "ship it")  │   │      │   Write/Edit outside scratchDir,     │
   │       → call ready_to_build ───┼─┐ │      │   even via symlink or ../ escape     │
   └─────────────────────────────────┼─┼────────────────────────────────────────────┘
                                      │ │
                    ┌─────────────────┘ └───────────────────┐
                    ▼                                        ▼
   ┌──────────────────────────────────┐     ┌──────────────────────────────────────┐
   │ mcp__pocketknife__validate_      │     │ mcp__pocketknife__ready_to_build     │
   │ manifest  (validate-tool.ts)     │     │ (validate-tool.ts)                   │
   │                                  │     │                                       │
   │ → Validator.validate(manifest)   │     │ → onReady() just flips a flag in the │
   │   (seams/validator.ts)           │     │   orchestrator. The orchestrator is  │
   │                                  │     │   the actual gate: it only honors    │
   │ valid:  capture {manifest,       │     │   this if lastValid is already set   │
   │         client} as the new       │     │   for the exact manifest just        │
   │         lastValid                │     │   validated — a stale/mistaken call  │
   │ invalid: return error list,      │     │   can't force the transition.        │
   │          model repairs & retries │     └──────────────────────────────────────┘
   └──────────────────────────────────┘
```

## Step by step

1. **Entry point.** `npm run agent -- "<prompt>" [--paste file] [--app <app_id>]` runs
   `cli.ts`. It parses argv, optionally reads a pasted-code file, and constructs one
   `Orchestrator` — one process, one job, one `jobId` (`cli.ts:27-50`).

   When `--app <app_id>` is passed the CLI calls `orchestrator.loadExistingApp(appId)`
   before planning begins (`cli.ts:57-61`). This fetches the app's current manifest and
   editable source from the backend (via `AppSourceFetcher`), stores them on the
   orchestrator, and fails fast if the app is not found. The fetcher implementation is
   selected by `selectFetcher()` in `seams/select.ts` using the same `SUBMIT_MODE` env
   var as the submitter.

2. **Planning session starts.** `orchestrator.startPlanning()` calls
   `planner.start()`, which opens a Claude Agent SDK `query()` session
   (`planner.ts:72-101`). In update mode the initial prompt is prepended with a
   `<current-manifest>` block and stable-id preservation rules so the planner edits
   the existing manifest rather than authoring from scratch. The planner's tool set is
   deliberately narrow: `AskUserQuestion`, `Skill`, and two MCP tools served from an
   in-process MCP server (`tools/validate-tool.ts`). It has **no file tools** — it
   cannot read or write anything on disk.

3. **Draft → validate → repair loop.** Following the `pocketknife-manifest` skill,
   the planner drafts a manifest and calls `validate_manifest`. That tool calls
   into the injected `Validator` (`seams/validator.ts`). Today that's
   `StubValidator`: an ajv structural check against `schema/manifest.schema.json`,
   then the semantic checks ajv can't express (`seams/semantic.ts`) — stable-id
   uniqueness, reserved names, reference resolution. On success it also generates
   the TypeScript client surface (`seams/generate-client.ts`). Errors come back as
   a list the planner must fix and re-submit; a success caches
   `{manifest, client}` as `lastValid` on the orchestrator
   and returns the generated client text to the model so
   it can describe the app accurately.

4. **Conversational refinement.** Back in `cli.ts`, a `readline` loop reads user
   input and calls `orchestrator.refinePlan()` → `planner.refine()`, which resumes
   the *same* SDK session via its cached `session_id` (`planner.ts:84`). Each
   refinement re-runs step 3.

5. **Signaling readiness.** The planner is told to watch every user message for
   intent to proceed, in any phrasing. When it sees that *and* the current
   manifest already validated with no edits since, it calls `ready_to_build`. The
   tool handler just calls `onReady()` (`validate-tool.ts:65-81`); the
   orchestrator's callback only actually sets `readyToBuild = true` if `lastValid`
   is set — the model can report intent, but only a genuinely-validated manifest can
   make that intent stick.

6. **Build handoff.** The CLI polls `isReadyToBuild()`. Once true, it calls
   `orchestrator.build()`, which writes the cached `manifest.json` and `client.ts`
   into the scratch directory, then seeds the source tree:

   - **New-app mode**: copies the blank React/Vite/Tailwind scaffold from
     `templates/frontend/` (excluding `node_modules` and `dist`).
   - **Update mode with stored source**: extracts the fetched source archive into
     the scratch dir instead of the scaffold, so the builder edits real files.
   - **Update mode without source** (legacy app): falls back to the blank scaffold;
     the builder authors from scratch using the existing manifest as context.

   Then starts the **builder session** (`builder.ts`).

7. **Builder session.** A second, one-shot SDK `query()` with `cwd` pinned to the
   scratch directory, tools `Read/Write/Edit/Glob/Skill`,
   `permissionMode: "acceptEdits"`, and the `pocketknife-frontend` skill. It reads
   `manifest.json` + `client.ts` and authors a full frontend tree against the
   typed client only — the skill forbids raw `fetch` calls or hand-built URLs. A
   `PreToolUse` hook (`hooks/scratch-guard.ts`) is the actual enforcement: it
   resolves every `Write`/`Edit` target's real path (following `..` and symlinks,
   even for not-yet-existing files) and denies anything that resolves outside the
   scratch root — the prompt's instruction to "stay in the directory" is backed by
   a check that doesn't trust the model's judgment.

8. **Confirm.** Control returns to `cli.ts`, which asks `Submit this app? [y/N]`.
   This is plain code, not a tool either model can call.

9. **Submit — the one irreversible action.** On confirmation, `orchestrator.submit()`
   hands `{jobId, manifest, scratchDir}` to the injected `Submitter`
   (`seams/submitter.ts`). In update mode a submit-time guard first asserts that
   `manifest.app.id === updateAppId` — a reminted id would route to `firstInstall`
   on the backend instead of `redeploy`, orphaning the original app's data.

   `StubSubmitter` wipes and rewrites `out/<jobId>/` idempotently: `manifest.json`,
   a `frontend/` copy of the authored tree, and a `bundle.json` file manifest; it
   returns a stub `appId`.

   `HttpSubmitter` packs three parts as `multipart/form-data`:
   - `bundle`: gzip-tar of `dist/` (the built frontend)
   - `manifest`: the validated JSON manifest
   - `source`: gzip-tar of the scratch dir excluding `node_modules` and `dist`
     (the editable source, stored by the backend for future `--app` updates)

   The POST goes to `{GO_BASE_URL}/deploy`, idempotency-keyed on `jobId`. That
   endpoint installs a brand-new app id live or redeploys an already-known one
   through the existing build pipeline, then responds with the deployed `appId`.

   `submit()` lives in orchestrator code only — never behind a model-callable tool,
   never inside the planner or builder loop.

## The seam: stub by default, real Go backend on opt-in

`seams/select.ts` is the **only** place that reads `VALIDATE_MODE`, `SUBMIT_MODE`,
and `GO_BASE_URL`. Everything else depends on the `Validator`/`Submitter`/`AppSourceFetcher`
interfaces and has no idea which implementation is behind them.

- `VALIDATE_MODE=stub` (default) → `StubValidator`, local ajv + semantic checks.
- `VALIDATE_MODE=http` → `HttpValidator`, intended to `POST {GO_BASE_URL}/validate`
  against the real `validateapi.NewServer()` in the Go backend — **not implemented
  yet** (throws).
- `SUBMIT_MODE=stub` (default) → `StubSubmitter`, writes to `agent/out/`; `StubFetcher`
  which throws on `--app` (can't fetch without a backend).
- `SUBMIT_MODE=http` → `HttpSubmitter`: multipart POST to `{GO_BASE_URL}/deploy`
  with manifest + bundle + source. `HttpFetcher`: reads from `GET {GO_BASE_URL}/export/{appId}`
  and `GET {GO_BASE_URL}/export/{appId}/source`. A retried POST for a `jobId` that
  already succeeded short-circuits instead of deploying twice. The agent never
  authenticates these calls (see the Go repo's openspec design for the planned
  auth follow-up).

## Observability: optional Langfuse tracing

`tracing.ts` is the single place that decides whether `query()` calls get
instrumented, mirroring the seam pattern above: enabled only when
`LANGFUSE_PUBLIC_KEY` and `LANGFUSE_SECRET_KEY` are set (see `.env.example`),
otherwise `query` is the SDK's own export, untouched. When enabled, it wraps
the SDK's `query` with OpenInference's Claude Agent SDK instrumentation,
backed by a `LangfuseSpanProcessor`-registered `NodeTracerProvider`.
`planner.ts` and `builder.ts` import `query` from here instead of from the SDK
directly, so both sessions always get whichever variant this module decided
to hand out. Each `query()` turn becomes an OpenInference AGENT span with
child TOOL spans per tool call (`validate_manifest`, `ready_to_build`,
`Read`/`Write`/`Edit`/...). `orchestrator.ts` wraps `startPlanning`/
`refinePlan` and `build()` in `withJobTrace(jobId, name, fn)`, which tags
every span created during `fn` with the job's `jobId` as the Langfuse session
id — so the planner's turns and the builder's run (including any self-heal
fix session inside `build-frontend.ts`) for one job group together under one
session in the Langfuse UI. `cli.ts` calls `shutdownTracing()` on exit to
flush buffered spans before the process ends.

## Why two sessions, not one

| | Planner | Builder |
|---|---|---|
| Lifetime | resumed across every refinement turn | one shot, after build() |
| Tools | `AskUserQuestion`, `Skill`, 2 MCP tools | `Read/Write/Edit/Glob`, `Skill` |
| Can write files? | no | yes, but only inside scratchDir (hook-enforced) |
| Can submit? | no — not a tool it has | no — not a tool it has |
| Skill loaded | `pocketknife-manifest` | `pocketknife-frontend` |

Splitting the job this way means each session's blast radius matches its job: the
planner can shape state (the manifest) but touch nothing on disk; the builder can
touch disk but only inside a directory it can't escape. The only step that
reaches outside the process — `submit()` — is never a tool at all, just
orchestrator code gated by a real validation result and an explicit user
confirmation.
