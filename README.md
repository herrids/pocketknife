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
assets), generates a **typed TypeScript client** from a manifest, and runs an
app's declared **sandboxed server-side functions** in a capability-checked
WebAssembly sandbox. Each is a separable concern with its own package; the core
above does not depend on any of them.

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
api/              generic CRUD + list handlers, query parser, error envelope
registry/         in-memory app registry, boot loader
migrate/          schema diff → classify → witness → snapshot → execute
build/            build & activation; second-deploy orchestration; platform DB
assets/           serves each app's activated frontend bundle (/ui/)
client/           typed TypeScript client generator (pure fn of the schema)
sandbox/          capability-checked WebAssembly sandbox for functions
broker/           the only path from a function to a model provider
consent/          derives an app's union capability surface from the manifest
cors/             optional dev-only cross-origin middleware
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
make run                  # serve apps/ on :8080
make build                # build bin/pocketknife
make vet / make fmt       # go vet / go fmt
```

The binary has three modes. With no subcommand it **serves**; `migrate` and
`build` are headless one-shot commands.

```sh
# serve (default): API + build-status + activated UIs over one origin
./bin/pocketknife -addr :8080 -apps apps [-platform-db platform.db] [-cors]

# migrate: evolve one app's schema to a new manifest version, no data loss
./bin/pocketknife migrate -app <id> -to <new_manifest.json> [-confirm] [-witnesses <file.json>]

# build: (re)build & activate the current frontend, or run a full second deploy
./bin/pocketknife build -app <id> [-to <new_manifest.json>] [-confirm] [-witnesses <file.json>]
```

On boot the server scans `apps/*/manifest.json`, validates each (skipping and
logging any that fail), materializes each app's `data.db`, registers the schema,
reconciles build state (failing any job interrupted by a restart and reattaching
the active build per app), and serves. A restart re-derives the registry from
disk and preserves all data. `-cors` enables permissive cross-origin headers for
running a separate frontend dev server; the production binary serves the API and
the built UI from one origin and never needs it.

## Manifest format

Canonical format is JSON, one immutable document per app version, at
`apps/<app_id>/manifest.json`. The written contract is
[`manifest.schema.json`](manifest.schema.json), used as the structural
validation layer.

```json
{
  "app": { "id": "reading_tracker", "name": "Reading Tracker", "emoji": "📚", "version": 1 },
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
  platform name (`id`, `created_at`, `updated_at`). `app.name` / `app.emoji` are
  free-form display values.
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
| `404`  | unknown app, entity, or row |
| `405`  | operation disabled for the entity |
| `409`  | unique constraint violation, or a reference constraint (e.g. `onDelete: restrict`) |
| `500`  | unexpected internal error |

Body validation reuses the manifest's field rules: `required`, `min`/`max`,
enum membership, and reference-target existence.

### Other routes

Alongside the per-app CRUD API, the serving binary mounts two more handlers
(same JSON error envelope):

| Method & path                  | Action |
|--------------------------------|--------|
| `GET /builds/{app}`            | every build job for an app, plus its durable activation pointer (read-only observability) |
| `GET /builds/job/{id}`         | one build job by id |
| `GET /ui/{app}/{path...}`      | the app's activated frontend bundle; unmatched paths fall back to the frontend's entry file (SPA routing) |

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

## Sandboxed functions (`sandbox/`, `broker/`, `consent/`)

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
declared capabilities for an app — a pure function of the manifest — so a future
shell can show the full capability surface before the app is allowed to run.

## Typed client (`client/`)

`client.Generate` renders a self-contained TypeScript module (entity types + a
typed client mirroring the CRUD/list surface, URL scheme, query syntax, JSON
shapes and error envelope) from a validated schema model. It is a pure function
of the `*schema.App`: an unchanged manifest produces byte-identical output, so
regenerating is a no-op diff.

## Deferred (still out of scope)

Intentionally **not** built:

- **On-box compilation.** Pocketknife references pre-built frontends and `.wasm`
  functions; it never bundles or compiles them itself.
- **LLM-authored manifests / changesets.** Migrations are derived structurally;
  there is a deliberate seam where a model-proposed, annotated changeset would be
  *verified* against the structural ground truth, but nothing authors one.
- **Authentication, users, sessions, multi-user, permissions.**
- **A frontend shell / tiles** (the host UI that would render `consent`'s
  capability surface and switch between apps).
- **Real-time / subscriptions.**
- **Query features beyond the v1 surface** — no OR, nesting, joins, or extra
  operators/types.
