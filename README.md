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
cmd/pocketknife   entrypoint (main)
schema/           manifest types + parser → schema model
validate/         JSON-Schema structural + semantic checks (the hard gate)
materialize/      schema → SQLite DDL
store/            per-app SQLite connections, parameterized queries
api/              generic CRUD + list handlers, query parser, error envelope
registry/         in-memory app registry, boot loader
apps/             example apps (manifests; data.db created at runtime)
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
./bin/pocketknife -addr :8080 -apps apps
```

On boot the server scans `apps/*/manifest.json`, validates each (skipping and
logging any that fail), materializes each app's `data.db`, registers the schema,
and serves. A restart re-derives the registry from disk and preserves all data.

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

## Deferred (out of scope for v1)

These are intentionally **not** built. Clean seams are left where they plug in
(notably `registry.Load`, where a `migrate(stored, new)` step would sit before an
app is served), but nothing here is implemented, stubbed, or depended upon:

- **Schema migrations / manifest versioning** beyond storing the version number.
  Reconciling a changed manifest against an existing `data.db` is the future
  migration engine's job.
- A generated typed client.
- Functions, a sandbox, or any capability system.
- Any LLM / generation.
- Authentication, users, sessions, multi-user, permissions.
- The frontend shell, tiles, or any UI.
- Real-time / subscriptions.
- Query features beyond the small v1 surface (no OR, nesting, joins, or extra
  operators/types).
