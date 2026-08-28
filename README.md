# Pocketknife

A single, generic, schema-driven HTTP backend. One server turns a declarative
**manifest** into a working API and database — for any app — with **no per-app
code generation and no per-app process**.

A manifest does not compile to a program. It registers a schema with one generic
server that already knows how to serve any schema. "Creating a backend from a
manifest" means exactly three things: **validate** the manifest, **materialize**
that app's database tables, and **register** its schema in an in-memory registry.
One set of CRUD/query handlers then serves every app; a handler looks an app up
by id and serves it from its registered schema.

Beyond that core, the same binary also: **evolves** an app's schema to a new
manifest version without losing data (the migration engine), **builds and
activates** an app's pre-built frontend with a single rollback contract (build &
deploy), serves each app's activated UI from the same origin as its API (static
assets), and generates a **typed TypeScript client** from a manifest. It also
contains a capability-checked WebAssembly **sandbox** for declared
server-side functions — implemented and tested, but not yet invoked by any
HTTP or MCP entry point in this binary; see "Sandboxed functions" below for
what that means in practice today.

An app can also declare **tools**: named, manifest-defined operations that
run one or more of its own CRUD calls in sequence, exposed live over MCP at
`/mcp/{app_id}` — one MCP tool per declared tool, one call to the tools engine
(`tools/`) per invocation, atomic across every step. See "Optional: `tools`"
and "MCP tools" below.

The same binary also serves a **shell launcher** — a React SPA that is the host
UI for the whole system. From the shell a user can browse, open, and manage every
registered app. Pressing "+ New" starts a **conversational authoring session**
with a Claude-backed agent: the agent proposes a manifest and frontend,
the user reviews and refines it, then approves — after which the agent submits
it through `POST /deploy` and the app appears in the launcher grid within
seconds. The shell is compiled from `shell/` and served at `/` by the same
binary that serves all the app APIs. No separate process, no separate origin
in production.

## Contract / invariants

- **One generic server, no per-app code.** No codegen, no per-app processes.
- **Stable IDs are the spine.** Every app, entity and field has an immutable
  `id` separate from its `name`. The `id` never changes; the `name` may. Storage
  is keyed to a field's identity (its id), never its name.
- **One SQLite file per app** (`apps/<app_id>/data.db`). Never shared across apps
  — physical isolation.
- **Manifests on disk are the source of truth; the registry is a derived cache**
  rebuilt from disk on every boot. Deleting the registry loses nothing.
- **Validation is a hard gate.** A manifest that fails validation is never
  materialized and never served. Validation returns a structured list of errors
  (`path` + `code` + `message`).
- **Platform columns are automatic and never declared.** Every table gets
  `id` (TEXT PRIMARY KEY), `created_at`, `updated_at` (ISO-8601 UTC strings). A
  manifest declaring any of these reserved names is rejected.
- **All SQL values are parameterized.** Identifiers come only from validated,
  SQL-safe names. The only literals in generated DDL are enum `CHECK` values,
  which are schema constants (single-quote-escaped), never request data.
- **Closed type set** (below). Anything else is a validation error.
- **Determinism.** The same manifest always yields the same schema;
  materialization is idempotent (`CREATE TABLE IF NOT EXISTS`).

## Layout

```
cmd/pocketknife   entrypoint (serve / migrate / build modes)
schema/           manifest types + parser → schema model
validate/         JSON-Schema structural + semantic checks (the hard gate)
materialize/      schema → SQLite DDL
store/            per-app SQLite connections, parameterized queries
domain/           transport-neutral runtime operations (create/get/list/update/delete), shared field coercion
api/              thin net/http adapter over domain/: routing, query-string parsing, HTTP status mapping
registry/         in-memory app registry, boot loader
migrate/          schema diff → classify → witness → snapshot → execute
build/            build & activation; second-deploy orchestration; platform DB
assets/           serves each app's activated frontend bundle (/ui/)
client/           typed TypeScript client generator (pure fn of the schema)
sandbox/          capability-checked WebAssembly sandbox for functions
broker/           the only path from a function to a model provider
consent/          derives an app's union capability surface from the manifest
tools/            executes a manifest-declared tool's CRUD step sequence atomically
mcpserver/        MCP transport over tools/: one /mcp/{app} endpoint per app
platform/         session auth + registry API + agent bridge (all /platform/ routes)
shellserve/       serves the compiled shell SPA (shell/dist/) at /
deployapi/        POST /deploy ingest — receives approved bundles from the agent
cors/             optional dev-only cross-origin middleware
shell/            React SPA — the launcher shell (compiled to shell/dist/)
agent/            Node/TypeScript Claude agent — conversational app authoring
apps/             example apps (manifests + frontend; data.db created at runtime)
manifest.schema.json   canonical JSON Schema for the manifest format
schema_embed.go        embeds manifest.schema.json into the binary
```

## Running

Go is required. If it is not on your PATH (and Homebrew is unavailable), install
the official tarball into a user directory, e.g.:

```sh
curl -fsSL https://go.dev/dl/go1.26.4.darwin-arm64.tar.gz | tar -C ~/.local -xz
export PATH="$HOME/.local/go/bin:$PATH"
```

Then:

```sh
make test                 # run the full test suite
make build                # build bin/pocketknife + compile shell/ SPA
make shell-build          # compile shell/ SPA only → shell/dist/
make run                  # serve apps/ on 127.0.0.1:8080 (API-only, no shell)
make vet / make fmt       # go vet / go fmt
```

The binary has three modes. With no subcommand it **serves**; `migrate` and
`build` are headless one-shot commands.

```sh
# serve (default): API + shell launcher + activated UIs over one origin
./bin/pocketknife -apps apps [-addr 127.0.0.1:8080] [-platform-db platform.db] [-shell-dist shell/dist] [-agent-bin ""] [-cors]

# migrate: evolve one app's schema to a new manifest version, no data loss
./bin/pocketknife migrate -app <id> -to <new_manifest.json> [-confirm] [-witnesses <file.json>]

# build: (re)build & activate the current frontend, or run a full second deploy
./bin/pocketknife build -app <id> [-to <new_manifest.json>] [-confirm] [-witnesses <file.json>]
```

On boot the server scans `apps/*/manifest.json`, validates each (skipping and
logging any that fail), materializes each app's `data.db`, verifies the
resulting database actually matches the manifest (skipping, not silently
serving, an app whose `data.db` predates a manifest change that was never
migrated), registers the schema, reconciles build state (failing any job
interrupted by a restart and reattaching the active build per app), and
serves. A restart re-derives the registry from disk and preserves all data.

**Trust model.** `-addr` defaults to `127.0.0.1:8080` — the Workbench v0.1
runtime assumes a trusted local machine. Exposing the runtime to a network
(e.g. `-addr :8080` to bind every interface) is an explicit configuration
decision the operator has to make; stronger authentication for that case is
out of scope for v0.1 — see the Platform API section below for what is and
isn't authenticated today.

`-shell-dist` sets the compiled shell directory (default `shell/dist`). If the
directory is absent the shell routes return 503 — the API still works. `-agent-bin`
is the path to a pre-built agent binary; if empty the server uses `npx tsx
agent/src/cli.ts` when spawning a planning session. `-cors` enables permissive
cross-origin headers for running a separate frontend dev server; the production
binary serves the API and the built UI from one origin and never needs it.

### Two-process dev mode

In development it is convenient to run the Go API and the shell SPA dev server
side by side so you get Vite's hot-module replacement:

```sh
# Terminal 1: Go API with CORS enabled
POCKETKNIFE_ADMIN_EMAIL=you@example.com POCKETKNIFE_ADMIN_PASSWORD=mypassword ./bin/pocketknife -cors -apps apps

# Terminal 2: Shell SPA dev server (proxies /platform/, /apps/, /ui/ → :8080)
cd shell && npm install && npm run dev   # runs on http://localhost:3001
```

In production `make build` compiles both the Go binary and the shell SPA; the
binary serves everything from one origin with no CORS needed.

## Shell launcher

The shell is a React 18 + TypeScript + Vite SPA compiled to `shell/dist/` and
served at `/` by `shellserve.NewServer`. It has six screens:

| Screen | Route | Purpose |
|--------|-------|---------|
| Login | `/login` | Email + password entry; redirects to `/home` if already authed |
| Home | `/home` | App grid (emoji tiles, squircle style, per-app color), building progress banner |
| New App Sheet | (sheet on Home) | Describe the app you want; fires a planning session |
| Plan Review | `/plan/:sessionId` | SSE-backed chat with the agent; shows checklist, refinement input, approve CTA |
| App Inside View | `/app/:appId` | Iframe pointing at `/ui/{appId}/` with an inline "ask to change" bar |
| Building State | (banner on Home) | Live progress ring (queued → building → activating) with pocketPulse animation |

An app's tile emoji and accent color default to the `app.emoji` / `app.color`
declared in its manifest (see [Manifest format](#manifest-format)); both can be
overridden afterward via `PATCH /platform/registry/{appId}`. A bottom nav bar
(Home/Search/Recent/Menu) exists in `shell/src/components/BottomNav.tsx` but is
currently commented out of `Home.tsx` — none of its destinations are wired up
yet.

Design tokens: cream palette (`#F2ECE0` surface, `#E8E0D0` canvas), terracotta
accent (`#DD6440`), squircle border-radius (19 px tiles), Space Grotesk +
Space Mono (loaded from Google Fonts). Light/dark mode is a manual toggle
(`ThemeToggle`, top-right on Login and Home) backed by a `dark` class on
`<html>` and persisted to `localStorage`; it defaults to the OS preference on
first visit. Tailwind's `darkMode` is set to `"class"`, not `"media"`.

## Platform API (`/platform/`)

All `/platform/` routes require a valid session cookie (`pk_session`). The two
auth routes are exempt.

### Authentication

The server reads `POCKETKNIFE_ADMIN_EMAIL` and `POCKETKNIFE_ADMIN_PASSWORD` at
startup. `POCKETKNIFE_ADMIN_EMAIL` defaults to `admin@pocketknife.local` if
unset. If `POCKETKNIFE_ADMIN_PASSWORD` is absent, the server generates a random
password and prints it once to stdout alongside the admin email. The password
is bcrypt-hashed in memory and never written to disk; there is a single admin
identity, not a user table.

| Method & path | Action |
|---------------|--------|
| `POST /platform/auth/login` | Body `{"email":"...","password":"..."}`. On success: 200 + HttpOnly SameSite=Strict `pk_session` cookie (24h TTL, sliding renewal). On failure: 401 with a ≥ 200 ms floor to slow brute force. |
| `POST /platform/auth/logout` | Clears the cookie and invalidates the token. |

### App registry

| Method & path | Action |
|---------------|--------|
| `GET /platform/registry` | Returns a JSON array of all registered apps sorted by `grid_order`, each entry including `appId`, `emoji`, `color`, `displayName`, `gridOrder`, `buildState`, `manifestVersion`, `activeBuildId`. |
| `PATCH /platform/registry/{appId}` | Update `emoji`, `color`, and/or `displayName` for one app. |
| `POST /platform/registry/reorder` | Body `{"order":["appId1","appId2",...]}`. Sets `grid_order` in bulk. |

App display metadata (`emoji`, `color`, `display_name`, `grid_order`) is stored
in the platform database (`platform.db`, `app_meta` table) separate from per-app
`data.db` files. Every newly registered app gets a default row automatically.

### Agent bridge (planning sessions)

A *planning session* is a live connection to the agent subprocess. The shell
creates one, streams events over SSE, sends refinement messages, then approves
the plan — after which the agent builds and submits through `POST /deploy`.

| Method & path | Action |
|---------------|--------|
| `POST /platform/plan` | Body `{"prompt":"...","appId":"..."}`. Spawns an agent subprocess in `--bridge-mode`, returns `{"sessionId":"..."}`. |
| `GET /platform/plan/{sessionId}/events` | SSE stream of bridge events (replays buffered events from `Last-Event-ID`; max 4 concurrent subscribers). |
| `POST /platform/plan/{sessionId}/message` | Body `{"text":"..."}`. Sends a refinement to the running agent; returns 202. Returns 409 if the session is not in an active state. |
| `POST /platform/plan/{sessionId}/approve` | Signals the agent to proceed with the build; returns `{"ok":true,"appId":"..."}`. Returns 409 if the plan is not in `ready` state. |

Sessions time out after 30 minutes of inactivity. Up to 50 events are buffered
per session for SSE replay.

**Bridge event types** (agent → stdout as NDJSON):

| Type | Fields | Meaning |
|------|--------|---------|
| `turn` | `role`, `text` | A new planner assistant message |
| `plan` | `checklist:[{text,done}]` | Emitted when `validate_manifest` succeeds; summarizes planned features |
| `ready` | `manifestVersion`, `appId?` | Valid plan exists; waiting for `approve` |
| `error` | `reason` | Fatal agent error; session is dead |
| `done` | `appId?` | Clean exit after deploy submission |

See `docs/bridge-protocol.md` for the full NDJSON protocol and lifecycle diagram.

## Agent (`agent/`)

`agent/` is a separate Node/TypeScript process: a Claude-backed planner+builder
that converses with the user to design a manifest and frontend, then — on
explicit user approval, never on its own — submits the result through
`POST /deploy`. The agent is stateless with respect to the Go server; it reads
no files in `apps/` and never touches `platform.db`.

Two invocation modes:

- **Interactive CLI** (no flag): runs in a terminal, reads from readline, writes
  human-readable prose. Used for standalone authoring or development.
- **Bridge mode** (`--bridge-mode`): reads newline-delimited JSON from stdin
  (`{type:"message"|"approve",...}`), writes newline-delimited JSON events to
  stdout. This is the mode spawned by the Go server's planning session handler.

When `--app <id>` is passed alongside `--bridge-mode`, the agent treats the
session as a *change request* for an existing app and routes the approved result
through `build.Deploy` rather than `build.Bootstrap`.

## Manifest format

Canonical format is JSON, one immutable document per app version, at
`apps/<app_id>/manifest.json`. The written contract is
[`manifest.schema.json`](manifest.schema.json), used as the structural
validation layer.

```json
{
  "app": { "id": "reading_tracker", "name": "Reading Tracker", "emoji": "📚", "color": "#8E86CF", "version": 1 },
  "entities": [
    {
      "id": "ent_book",
      "name": "book",
      "operations": ["create", "read", "update", "delete"],
      "fields": [
        { "id": "fld_title",  "name": "title",  "type": "text",    "required": true, "max": 200 },
        { "id": "fld_author", "name": "author", "type": "text" },
        { "id": "fld_rating", "name": "rating", "type": "integer", "min": 1, "max": 5 },
        { "id": "fld_done",   "name": "done",   "type": "boolean", "default": false }
      ]
    }
  ]
}
```

Rules:

- `app.id`, `entity.id`, `field.id` are immutable stable IDs, unique within their
  scope, non-empty. Convention `ent_*` / `fld_*` (convention, not enforced).
- `name` (entities, fields) is the SQL identifier **and** JSON key. It must match
  `^[a-z][a-z0-9_]*$`, be unique among siblings, and must not be a reserved
  platform name (`id`, `created_at`, `updated_at`). `app.name` / `app.emoji` /
  `app.color` are free-form display values; `app.emoji` and `app.color` seed
  the shell launcher's tile (see [Shell launcher](#shell-launcher)) the first
  time the app is registered and can be edited afterward via the registry API.
- `operations` is optional per entity (default: all four). Subsetting it
  restricts the API surface; a disabled operation returns **405**.
- Unknown top-level keys, unknown field keys, constraint keys not allowed for a
  field's type, a default that violates the field's own constraints, an enum
  default not in `values`, and a reference whose `target` does not resolve are
  all rejected.

### Type set (v1, closed) and SQLite mapping

| type        | meaning                | allowed constraint keys                          | SQLite column | notes |
|-------------|------------------------|--------------------------------------------------|---------------|-------|
| `text`      | UTF-8 string           | `required`, `unique`, `default`, `min`/`max` (length) | `TEXT`    | length `CHECK`s |
| `integer`   | 64-bit int             | `required`, `unique`, `default`, `min`/`max` (value)  | `INTEGER` | range `CHECK`s |
| `real`      | float                  | `required`, `unique`, `default`, `min`/`max` (value)  | `REAL`    | range `CHECK`s |
| `boolean`   | true/false             | `required`, `default`                            | `INTEGER`     | stored 0/1; JSON `true`/`false` at the boundary |
| `datetime`  | ISO-8601 UTC instant   | `required`, `default`                            | `TEXT`        | one canonical encoding |
| `enum`      | one of a fixed set     | `required`, `default`, `values` (required, non-empty) | `TEXT`   | `CHECK (col IN (...))` |
| `reference` | points at another row  | `required`, `target` (required), `onDelete`      | `TEXT`        | `FOREIGN KEY (col) REFERENCES <target>(id) ON DELETE <action>` |

`onDelete` is `set_null` (default), `restrict`, or `cascade`. `required` →
`NOT NULL`; `unique` → a `UNIQUE` index. Every table additionally gets
`id TEXT PRIMARY KEY, created_at TEXT NOT NULL, updated_at TEXT NOT NULL`, and
`PRAGMA foreign_keys = ON` is enabled per connection.

Datetimes are stored and emitted in one canonical encoding: ISO-8601 UTC with
millisecond precision and a literal `Z` (e.g. `2026-06-21T15:21:58.940Z`).

### Optional: `frontend` and `functions`

Two optional top-level keys extend an app beyond a bare API. Both point at
**pre-built** artifacts — pocketknife never compiles a frontend or a function
on-box; the manifest only references output that already exists, relative to the
app directory.

```json
{
  "app": { "id": "tasks", "name": "Tasks", "emoji": "✅", "version": 1 },
  "entities": [ ... ],
  "frontend": { "dist": "frontend/dist", "entry": "index.html" },
  "functions": [
    {
      "id": "fn_summarize",
      "name": "summarize",
      "entry": "functions/summarize.wasm",
      "capabilities": {
        "data":    [ { "entity": "ent_task", "operations": ["read"] } ],
        "network": ["api.example.com"],
        "model":   true
      }
    }
  ]
}
```

- **`frontend`** names a built static bundle. `dist` (required) is the asset
  directory; `entry` (optional, default `index.html`) is the file served for the
  root and for any path that doesn't match a real asset (SPA fallback). It is
  served at `/ui/{app}/` once activated.
- **`functions`** declares sandboxed server-side functions. `entry` must name an
  already-built `.wasm` module. `capabilities` is the **closed** set of host
  power the sandbox grants — and the manifest only ever *declares* it; the
  sandbox is what enforces it:
  - `data` — per-entity operation grants (referenced by the entity's stable id),
    each restricted to a subset of that entity's enabled operations.
  - `network` — an **exact-match** hostname allow-list. No wildcards, no general
    fetch.
  - `model` — access to the model broker. The function never receives the
    underlying provider token.

### Optional: `tools`

A third optional top-level key, `tools`, declares named operations exposed to
MCP clients at `/mcp/{app_id}` (see "MCP tools" below). Unlike `functions`,
a tool is not code: it's an ordered, non-branching sequence of CRUD calls
into the app's *own* entities, so it needs no sandbox — it can't do anything
a direct call against `/apps/{app}/{entity}` couldn't already do; it only
names and sequences those calls, atomically, in one transaction.

```json
{
  "app": { "id": "tasks", "name": "Tasks", "version": 1 },
  "entities": [ ... ],
  "tools": [
    {
      "id": "tool_new_project_task",
      "name": "new_project_task",
      "description": "Create a project, then a task inside it",
      "params": [
        { "id": "p_project_name", "name": "project_name", "type": "text", "required": true },
        { "id": "p_title", "name": "title", "type": "text", "required": true }
      ],
      "steps": [
        { "id": "project", "op": "create", "entity": "ent_project", "set": { "name": "$params.project_name" } },
        { "id": "task", "op": "create", "entity": "ent_task", "set": { "title": "$params.title", "project": "$steps.project.id" } }
      ]
    }
  ]
}
```

- **`params`** declares the tool's call-time inputs using the exact same
  shape as an entity field (`type`, `required`, `default`, and that type's
  constraint keys) — so the same field-coercion rules validate them, and the
  MCP endpoint renders them as a real JSON Schema for the tool's arguments.
- **`steps`** is the ordered CRUD sequence, one of `create`/`read`/`update`/
  `delete` per step against one entity, restricted to operations that
  entity's own `operations` list already allows. `rowId` is required for
  read/update/delete and forbidden for create; `set` maps field name to value
  for create/update.
- Every `rowId`/`set` value is either a **literal** JSON value or a
  **reference**: a string of the form `"$params.<name>"` (this call's
  arguments) or `"$steps.<id>.<field>"` (an earlier step's result row in the
  same call — forward references and cycles are rejected at validation time,
  since a step can only ever reference something that ran before it).
- All steps in one call run inside **one database transaction**: a failure
  at any step rolls back every earlier step in that same call, so a tool
  either fully applies or has no effect at all.

## HTTP API

All routes are namespaced by app: `/apps/{app_id}/{entity_name}`.

| Method & path                         | Action | Success |
|---------------------------------------|--------|---------|
| `POST   /apps/{app}/{entity}`         | create (body: JSON object of field→value) | `201` created row |
| `GET    /apps/{app}/{entity}`         | list (query syntax below) | `200` `{data, total, limit, offset}` |
| `GET    /apps/{app}/{entity}/{id}`    | read one | `200` / `404` |
| `PATCH  /apps/{app}/{entity}/{id}`    | partial update (only supplied fields change; `updated_at` bumped) | `200` |
| `DELETE /apps/{app}/{entity}/{id}`    | delete | `204` |

### List query syntax (v1, AND-combined, no OR/nesting/joins)

- `filter=<field>:<op>:<value>` — repeatable, AND-ed. Ops: `eq`, `ne`, `gt`,
  `gte`, `lt`, `lte`, `like`.
- `sort=<field>` (ascending) or `sort=-<field>` (descending). Repeatable.
  `id`, `created_at`, `updated_at` are sortable/filterable too.
- `limit=<n>` (default 50, capped at 200), `offset=<n>` (default 0).
- `total` is the count of all rows matching the filters, ignoring limit/offset.

### Error envelope

```json
{ "error": { "code": "...", "message": "...", "details": [ ... ] } }
```

| Status | When |
|--------|------|
| `400`  | malformed request / body fails field validation (details list per-field issues) |
| `401`  | missing or expired session cookie (platform routes only) |
| `404`  | unknown app, entity, or row |
| `405`  | operation disabled for the entity |
| `409`  | unique constraint violation, or a reference constraint (e.g. `onDelete: restrict`) |
| `500`  | unexpected internal error |

Body validation reuses the manifest's field rules: `required`, `min`/`max`,
enum membership, and reference-target existence.

### Other routes

| Method & path | Action |
|---------------|--------|
| `GET /builds/{app}` | Every build job for an app, plus its durable activation pointer (read-only observability) |
| `GET /builds/job/{id}` | One build job by id |
| `GET /ui/{app}/{path...}` | The app's activated frontend bundle; unmatched paths fall back to the frontend's entry file (SPA routing) |
| `* /mcp/{app}` | MCP streamable-HTTP endpoint exposing the app's declared `tools` (see "MCP tools" below); `404` for an unknown app id |
| `POST /deploy` | Ingest an approved manifest + built frontend bundle (`multipart/form-data`: `jobId`, `manifest`, `bundle`); installs a new app or redeploys an existing one |
| `POST /platform/auth/login` | Password login → session cookie |
| `POST /platform/auth/logout` | Invalidate session |
| `GET /platform/registry` | List all apps with display metadata + build state (auth required) |
| `PATCH /platform/registry/{appId}` | Update app emoji/color/displayName (auth required) |
| `POST /platform/registry/reorder` | Reorder app grid (auth required) |
| `POST /platform/plan` | Start a planning session (auth required) |
| `GET /platform/plan/{id}/events` | SSE stream of planning events (auth required) |
| `POST /platform/plan/{id}/message` | Send refinement to agent (auth required) |
| `POST /platform/plan/{id}/approve` | Approve plan and trigger build (auth required) |
| `GET /` (and all unmatched paths) | Shell SPA (`shell/dist/index.html`) |

## curl examples

### reading_tracker (full CRUD, every constraint)

```sh
# create
curl -s -X POST localhost:8080/apps/reading_tracker/book \
  -d '{"title":"Dune","author":"Herbert","rating":5}'

# rating out of range -> 400 ; missing title -> 400
curl -s -X POST localhost:8080/apps/reading_tracker/book -d '{"title":"x","rating":6}'
curl -s -X POST localhost:8080/apps/reading_tracker/book -d '{"rating":3}'

# list: filter, newest-first, paginate
curl -s "localhost:8080/apps/reading_tracker/book?filter=done:eq:true"
curl -s "localhost:8080/apps/reading_tracker/book?sort=-created_at&limit=10&offset=0"

# read / update / delete (substitute the id from create)
curl -s localhost:8080/apps/reading_tracker/book/<id>
curl -s -X PATCH localhost:8080/apps/reading_tracker/book/<id> -d '{"done":true,"rating":4}'
curl -s -X DELETE localhost:8080/apps/reading_tracker/book/<id>
```

### gratitude_log (append-only: create + read only)

```sh
curl -s -X POST localhost:8080/apps/gratitude_log/entry -d '{"text":"sunshine"}'
curl -s localhost:8080/apps/gratitude_log/entry
curl -s localhost:8080/apps/gratitude_log/entry/<id>
# update and delete are disabled -> 405
curl -s -o /dev/null -w '%{http_code}\n' -X DELETE localhost:8080/apps/gratitude_log/entry/<id>
```

### tasks (references, enum, unique, onDelete)

```sh
# unique project name; a duplicate returns 409
curl -s -X POST localhost:8080/apps/tasks/project -d '{"name":"Home"}'
curl -s -X POST localhost:8080/apps/tasks/project -d '{"name":"Home"}'   # 409

# task references a project; default priority is "medium"
curl -s -X POST localhost:8080/apps/tasks/task -d '{"title":"Mow","project":"<project_id>"}'

# non-existent reference -> 400 ; priority outside enum -> 400
curl -s -X POST localhost:8080/apps/tasks/task -d '{"title":"x","project":"nope"}'
curl -s -X POST localhost:8080/apps/tasks/task -d '{"title":"x","priority":"urgent"}'

# deleting a referenced project sets the task's project to null (onDelete: set_null)
curl -s -X DELETE localhost:8080/apps/tasks/project/<project_id>
curl -s localhost:8080/apps/tasks/task/<task_id>   # "project": null
```

### Platform API (auth + registry)

```sh
# login
curl -s -c cookies.txt -X POST localhost:8080/platform/auth/login \
  -H 'Content-Type: application/json' -d '{"email":"<your-email>","password":"<your-password>"}'

# list apps (requires cookie)
curl -s -b cookies.txt localhost:8080/platform/registry | jq .

# update app emoji
curl -s -b cookies.txt -X PATCH localhost:8080/platform/registry/reading_tracker \
  -H 'Content-Type: application/json' -d '{"emoji":"🌊"}'

# reorder
curl -s -b cookies.txt -X POST localhost:8080/platform/registry/reorder \
  -H 'Content-Type: application/json' -d '{"order":["tasks","reading_tracker","gratitude_log"]}'

# logout
curl -s -b cookies.txt -X POST localhost:8080/platform/auth/logout
```

## Schema migrations (`migrate/`)

Evolves one app from its on-disk manifest to a new version **without losing
data**. `pocketknife migrate -app <id> -to <new.json>` runs the pipeline:

1. **validate** the new manifest (the same hard gate).
2. **diff** old vs. new — a pure structural diff matched **entirely by stable
   id**. A field whose id is unchanged but whose name differs is a *rename* and
   moves no data; a new id is an *add*, a missing id is a *drop*.
3. **classify** each operation as `safe` (information-preserving, auto-applied)
   or `destructive` (information-losing). Classification reads only the
   operation's structure — never a caller hint — and treats anything ambiguous
   as destructive.
4. destructive operations require an explicit **`-confirm`** *and* a
   **witness** for each one (no default, no silent coercion). The witness
   vocabulary is closed:
   - `coerce` — a type narrowing (e.g. `real`→`integer`): `truncate`, `round`,
     or `fail` the migration on any lossy value.
   - `backfill` — a nullable→not-null tightening: the value written into
     currently-null rows.
   - `remap` — an enum value removed: how to rewrite rows still holding it.
5. **snapshot** the database, then **execute** the whole changeset in one
   transaction. Renames touch no SQL (the physical column is the field's stable
   id); adds/drops use native `ADD`/`DROP COLUMN`; type/nullability/enum/
   reference changes use the SQLite table-rebuild pattern. On any failure the
   snapshot is restored and the prior registration kept.

Witnesses are supplied via `-witnesses <file.json>`, a JSON object keyed by the
stable **field id** the witness applies to.

## Build & activation (`build/`)

`pocketknife build` is the one entry point for landing a new state for an app.
Job and activation state live in a separate **platform database**
(`platform.db`, distinct from per-app `data.db`s); a job moves through
`queued → building → activating → ready`, or to `failed`.

- **`build -app <id>`** (no `-to`) — an *install*: (re)build and activate the
  frontend for the app's current manifest version. No data change.
- **`build -app <id> -to <new.json>`** — a *second deploy*: a data migration,
  frontend rebuild, and activation landed as **one operation with a single
  rollback contract**. Ordering is: snapshot data → migrate → build frontend →
  activate; any failure rolls back to the prior good manifest, snapshot, and
  activated build. The data side reuses `migrate` verbatim, so destructive
  changes still need `-confirm` and witnesses.

On boot the serving binary reconciles build state: any job interrupted by a
restart is moved to `failed`, and each app's active build is reattached (or the
app is served API-only if its activation pointer is stale).

## Deploy ingest (`deployapi/`)

`POST /deploy` is how an external authoring tool — today, the `agent/`
planner/builder — lands an approved app without filesystem or CLI access to
the server. It accepts one `multipart/form-data` request (`jobId` field,
`manifest` part, gzipped-tar `bundle` part of the built frontend's `dist/`
tree) and routes it one of two ways:

- **Unknown app id** → `build.Bootstrap`: stages a fresh `apps/<app_id>/`
  under a temp name, materializes its database, builds and activates the
  bundle, then renames into place and registers the app live — all inside one
  build job. A failure at any point removes the staging directory; nothing
  partial is ever registered or served.
- **Known app id** → the bundle is written into that app's directory and the
  request is routed through the existing `build.Deploy` (install or, for a new
  manifest version, a full second deploy), reusing its single rollback
  contract.

The endpoint is idempotent on `jobId` (a repeated request for an already-
deployed job returns the cached `{appId, version, url}` instead of deploying
again) and serializes concurrent requests for the same app id. A manifest
that omits a `frontend` block gets one injected by default
(`{"dist":"dist","entry":"index.html"}`) — the agent's manifests only ever
describe the data schema, never the bundle's on-disk location. The bundle's
asset URLs must already be relative (Vite's `base: "./"`) since this endpoint
never rewrites paths; see `agent/templates/frontend/vite.config.ts`. The
endpoint does **not** authenticate its caller — it trusts whatever can reach
it, which is a deliberate, separately-tracked gap, not an oversight.

On success the app is reachable at `/ui/<app_id>/` with no server restart,
exactly like any other activated build. The newly deployed app also receives a
default `app_meta` row — emoji and color seeded from the manifest's
`app.emoji` / `app.color` (falling back to `📦` / `#E0E0E0` if either is
omitted), grid order auto-assigned — so it immediately appears in the shell
launcher's home grid.

## Sandboxed functions (`sandbox/`, `broker/`, `consent/`)

**Status: implemented and tested in isolation; not yet wired into any live
entry point.** Everything below describes what the sandbox *enforces once a
function is invoked* — but nothing in this binary currently invokes one: no
HTTP route triggers `sandbox.Invoke`, `broker.New`/`NewHTTPCaller` is never
constructed from the environment, and no example app under `apps/` declares
a `functions` block. A future MCP transport (or a dedicated HTTP endpoint)
is the natural place to wire this in; until then, treat this section as a
description of a well-tested library, not of a running feature. See
`docs/phase-4-wiring.md` for the tracked wiring gaps.

A function's manifest entry only *declares* capabilities; `sandbox/` is the real
boundary that *enforces* them. Each function body is treated as adversarial: it
runs as a WebAssembly module under wazero with **no filesystem, no environment,
no raw network**, behind a fixed, capability-checked host ABI (the `pocketknife`
host module) that is the only way out. Per-invocation resource limits apply
(linear memory, a wall-clock timeout, input/output byte caps). The three gated
host calls (`data_call`, `network_fetch`, `model_call`) return sentinel codes; a
denial carries no payload, so a function can't use responses as an oracle for
capabilities it wasn't granted.

`broker/` is the **only** path from a function to a model provider: the provider
token is read once from the environment, held unexported, and never reaches a
function or the browser. `consent/` derives the union of every function's
declared capabilities for an app — a pure function of the manifest — so the
shell can show the full capability surface before the app is allowed to run.

## Typed client (`client/`)

`client.Generate` renders a self-contained TypeScript module (entity types + a
typed client mirroring the CRUD/list surface, URL scheme, query syntax, JSON
shapes and error envelope) from a validated schema model. It is a pure function
of the `*schema.App`: an unchanged manifest produces byte-identical output, so
regenerating is a no-op diff.

## MCP tools (`mcpserver/`, `tools/`)

Every app's declared `tools` (see "Optional: `tools`" above) are live over
MCP at `/mcp/{app_id}` — a [streamable-HTTP](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)
endpoint, one per app, served stateless (no session persistence needed since
a tool call is a single request/response, same as the rest of this API). An
unknown app id is a `404`, matching every other per-app route.

`tools.Execute` is the transport-neutral engine: given an app, a tool (by id
or name) and a JSON object of params, it validates the params against the
tool's declared parameter list (reusing `domain.CoerceFieldValue` exactly, so
a reference-typed param's target is existence-checked the same way a normal
write's would be), then runs every step in order against **one database
transaction** — `store.Tx`, a transaction-scoped view of the same
Insert/GetByID/Update/Delete/List/Exists surface `store.Store` exposes — so a
failure at any step rolls back everything earlier steps in that call did.
Each step calls `domain.CreateIn`/`GetIn`/`UpdateIn`/`DeleteIn`, the same
validation and coercion logic `domain.Create`/`Get`/`Update`/`Delete` run for
the HTTP API, just against that shared transaction instead of the store's
ambient connection.

`mcpserver.NewServer` is the thin transport adapter: for each app it builds
an `mcp.Server` (from the [official Go MCP SDK](https://github.com/modelcontextprotocol/go-sdk))
with one MCP tool per declared tool — the tool's `params` rendered as a JSON
Schema for the call arguments — and a handler that decodes the call's raw
arguments and runs `tools.Execute`. A tool-level failure (bad params, a
step's validation error, a row not found) comes back as
`CallToolResult.IsError`, not a protocol-level error, so an MCP client sees
it as something to explain or retry rather than a transport fault. The
server is resolved fresh from the registry per session, so a redeployed
app's tool set is visible immediately, exactly like `assets.NewServer`
resolving the active frontend bundle fresh on every request.

## Deferred (still out of scope)

Intentionally **not** built:

- **On-box compilation.** Pocketknife references pre-built frontends and `.wasm`
  functions; it never bundles or compiles them itself.
- **Multi-user auth, roles, permissions.** The session layer is single-user
  (one password, one role). Per-app row-level access control is not modeled.
- **Real-time / subscriptions.**
- **Query features beyond the v1 surface** — no OR, nesting, joins, or extra
  operators/types.
- **`POST /deploy` authentication.** The deploy ingest endpoint trusts any
  caller that can reach it; locking it to the agent bridge is a separately-
  tracked gap.
