# Agent flow

One job, two short-lived Claude Agent SDK sessions, one orchestrator that owns the
only irreversible step. Nothing the models do can submit an app on its own.

```
            ┌──────────────────────────────────────────────────────────────────┐
            │                          cli.ts (terminal)                       │
            │  npm run agent -- "<prompt>" [--paste file]                      │
            └───────────────────────────┬──────────────────────────────────────┘
                                         │ new Orchestrator()
                                         ▼
            ┌──────────────────────────────────────────────────────────────────┐
            │                     orchestrator.ts (job state)                  │
            │  jobId, scratchDir = .scratch/<jobId>/                           │
            │  lastValid: last manifest+client that ever passed validation     │
            │  readyToBuild: flips true only if lastValid is set               │
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
   │   loop:                            │      │                                       │
   │    1. draft/repair manifest        │      │   reads manifest.json + client.ts    │
   │    2. call validate_manifest ──┐   │      │   (written by orchestrator.build())  │
   │    3. on errors: repair, retry │   │      │   authors frontend files against the │
   │    4. on success: summarize    │   │      │   typed client only (no raw fetch)   │
   │       in plain language        │   │      │   scratch-guard hook denies any      │
   │    5. watch for user intent    │   │      │   Write/Edit outside scratchDir,     │
   │       ("build it", "ship it")  │   │      │   even via symlink or ../ escape     │
   │       → call ready_to_build ───┼─┐ │      └──────────────────────────────────────┘
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

1. **Entry point.** `npm run agent -- "<prompt>" [--paste file]` runs `cli.ts`. It
   parses argv, optionally reads a pasted-code file, and constructs one
   `Orchestrator` — one process, one job, one `jobId` (`cli.ts:27-45`).

2. **Planning session starts.** `orchestrator.startPlanning()` calls
   `planner.start()`, which opens a Claude Agent SDK `query()` session
   (`planner.ts:76-100`). The planner's tool set is deliberately narrow:
   `AskUserQuestion`, `Skill`, and two MCP tools served from an in-process MCP
   server (`tools/validate-tool.ts`). It has **no file tools** — it cannot read or
   write anything on disk.

3. **Draft → validate → repair loop.** Following the `pocketknife-manifest` skill,
   the planner drafts a manifest and calls `validate_manifest`. That tool calls
   into the injected `Validator` (`seams/validator.ts`). Today that's
   `StubValidator`: an ajv structural check against `schema/manifest.schema.json`,
   then the semantic checks ajv can't express (`seams/semantic.ts`) — stable-id
   uniqueness, reserved names, reference resolution. On success it also generates
   the TypeScript client surface (`seams/generate-client.ts`). Errors come back as
   a list the planner must fix and re-submit; a success caches
   `{manifest, client}` as `lastValid` on the orchestrator
   (`orchestrator.ts:42-44`) and returns the generated client text to the model so
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
   is set (`orchestrator.ts:45-50`) — the model can report intent, but only a
   genuinely-validated manifest can make that intent stick.

6. **Build handoff.** The CLI polls `isReadyToBuild()`. Once true, it calls
   `orchestrator.build()` (`orchestrator.ts:74-84`), which writes the cached
   `manifest.json` and `client.ts` into a fresh `.scratch/<jobId>/` directory,
   then starts the **builder session** (`builder.ts`).

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
   (`orchestrator.ts:87-96`) hands `{jobId, manifest, scratchDir}` to the injected
   `Submitter` (`seams/submitter.ts`). `StubSubmitter` wipes and rewrites
   `out/<jobId>/` idempotently: `manifest.json`, a `frontend/` copy of the
   authored tree, and a `bundle.json` file manifest; it returns a stub `appId`.
   `submit()` lives in orchestrator code only — never behind a model-callable
   tool, never inside the planner or builder loop.

## The seam: stub today, real Go backend later

`seams/select.ts` is the **only** place that reads `VALIDATE_MODE`, `SUBMIT_MODE`,
and `GO_BASE_URL`. Everything else depends on the `Validator`/`Submitter`
interfaces and has no idea which implementation is behind them.

- `VALIDATE_MODE=stub` (default) → `StubValidator`, local ajv + semantic checks.
- `VALIDATE_MODE=http` → `HttpValidator`, intended to `POST {GO_BASE_URL}/validate`
  against the real `validateapi.NewServer()` in the Go backend — **not implemented
  yet** (throws).
- `SUBMIT_MODE=stub` (default) → `StubSubmitter`, writes to `agent/out/`.
- `SUBMIT_MODE=http` → `HttpSubmitter`, intended for a multipart POST to the Go
  control plane, idempotency-keyed on `jobId` — **not implemented yet** (throws).

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
