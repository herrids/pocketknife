---
name: pocketknife-manifest
description: The Pocketknife manifest contract — shape, closed type set, stable-ID discipline, and worked examples. Use whenever drafting, refining, or repairing a Pocketknife app manifest.
---

# Pocketknife manifest contract

A Pocketknife app is one declarative JSON document: a list of entities (tables), each
with a list of typed fields. There is no other way to define data — no code, no
migrations to write by hand. Your job is to turn a user's plain-language description of
an app into this document.

## Top-level shape

```json
{
  "app": { "id": "reading_tracker", "name": "Reading Tracker", "emoji": "📚", "version": 1 },
  "entities": [ /* one or more entity objects */ ],
  "frontend": { "dist": "frontend/dist", "entry": "index.html" },
  "functions": [ /* optional, sandboxed server-side functions — rarely needed for a new app */ ]
}
```

- `app.id`, `app.name`, `app.version` are required. `version` starts at `1`. `emoji` is
  optional, cosmetic.
- `entities` must have at least one entry.
- `frontend` and `functions` are populated later by the platform / the builder stage —
  as the planner, you normally emit a manifest **without** them. Don't invent a
  `frontend` or `functions` block unless the user explicitly asks for a sandboxed
  function.

## Stable IDs vs. names — the rule you must not break

Every entity and every field carries **two** identifiers:

- `id` — an immutable, internal stable ID. Convention: `ent_<name>` for entities,
  `fld_<name>` for fields (e.g. `ent_book`, `fld_title`). Once you've proposed an id for
  a thing across a conversation, **never reuse that id for a different thing**, and
  don't change a thing's id once the user has accepted it — only its `name` or other
  properties. IDs must match `^[a-z][a-z0-9_]*$` and be unique within their scope (entity
  ids unique across the app; field ids unique within their entity).
- `name` — the mutable, human/SQL/JSON-facing name. Must match `^[a-z][a-z0-9_]*$`, be
  unique among siblings, and must never be `id`, `created_at`, or `updated_at` (those
  three columns are added automatically by the platform on every entity).

You **propose** structure and ids; you do not own identity long-term — the platform is
what ultimately mints and preserves ids across the manifest's lifetime. Within a single
authoring session, treat the ids you've assigned as fixed once introduced, so renames
stay renames (no data loss) rather than becoming silent drops-and-adds.

## The closed type set

Exactly seven field types exist. There is no eighth. Prefer the most specific type
available; avoid inventing a generic "json" or "object" field — it does not exist in v1.

| type        | meaning                | constraint keys you may set                       | example |
|-------------|------------------------|-----------------------------------------------------|---------|
| `text`      | UTF-8 string           | `required`, `unique`, `default`, `min`/`max` (length)  | a title, a note |
| `integer`   | 64-bit whole number    | `required`, `unique`, `default`, `min`/`max` (value)   | a page count, a star rating |
| `real`      | floating point number  | `required`, `unique`, `default`, `min`/`max` (value)   | a price, a weight |
| `boolean`   | true/false             | `required`, `default`                                | a "done" flag |
| `datetime`  | ISO-8601 UTC instant   | `required`, `default`                                | a due date, a logged-at time |
| `enum`      | one of a fixed string set | `required`, `default`, `values` (required, non-empty) | a priority, a status |
| `reference` | points at another entity's row | `required`, `target` (required, an entity id), `onDelete` | a task's project |

Every field needs `id`, `name`, `type`. Setting a constraint key that isn't in that
field's list above will fail validation (e.g. `values` on a `text` field).

Other rules `validate_manifest` enforces, so don't second-guess them — just react to the
error if you get one:

- `required: true` means the column is `NOT NULL`. A field is optional by default.
- `unique: true` adds a uniqueness constraint (text/integer/real only).
- A `default` must itself satisfy the field's own `min`/`max`/`values`.
- An `enum` field's `default` (if any) must be one of its `values`.
- A `reference` field's `target` must be another entity's `id` **in this same manifest**.
- `onDelete` on a reference is `set_null` (default), `restrict`, or `cascade` — what
  happens to this field when the row it points at is deleted.
- `operations` on an entity (optional, default: all four) is a subset of
  `["create", "read", "update", "delete"]` — use it to make an entity append-only
  (`["create", "read"]`) or read-only, etc.

## Validation is mandatory — never assert validity yourself

You can't tell, just by looking, whether a manifest is valid — only the
`validate_manifest` tool can. **Always** call `validate_manifest` with your candidate
manifest before describing it to the user as ready, and before ever treating it as
final. If it returns errors, read them, fix the manifest, and call it again. Repeat
until it returns `valid: true`. Never tell the user "this is valid" without having just
gotten `valid: true` back from the tool. A manifest that has not passed validation can
never be built or submitted.

On success the tool returns a generated TypeScript client surface — that's the contract
the frontend-authoring stage will build against later; you don't need to do anything
with it yourself beyond knowing the manifest is now final.

## Worked examples

### 1. A tracker (one entity, full CRUD, every constraint kind)

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

### 2. An append-only log (create + read only — no edits, no deletes)

```json
{
  "app": { "id": "gratitude_log", "name": "Gratitude Log", "emoji": "🙏", "version": 1 },
  "entities": [
    {
      "id": "ent_entry",
      "name": "entry",
      "operations": ["create", "read"],
      "fields": [
        { "id": "fld_text",      "name": "text",      "type": "text",     "required": true },
        { "id": "fld_logged_at", "name": "logged_at", "type": "datetime" }
      ]
    }
  ]
}
```

### 3. Two entities with a reference, an enum, and a uniqueness constraint

```json
{
  "app": { "id": "tasks", "name": "Tasks", "emoji": "✅", "version": 1 },
  "entities": [
    {
      "id": "ent_project",
      "name": "project",
      "fields": [
        { "id": "fld_name", "name": "name", "type": "text", "required": true, "unique": true }
      ]
    },
    {
      "id": "ent_task",
      "name": "task",
      "fields": [
        { "id": "fld_title",    "name": "title",    "type": "text", "required": true },
        {
          "id": "fld_project", "name": "project", "type": "reference",
          "target": "ent_project", "onDelete": "set_null"
        },
        {
          "id": "fld_priority", "name": "priority", "type": "enum",
          "values": ["low", "medium", "high"], "default": "medium"
        }
      ]
    }
  ]
}
```

These three shapes — single tracker, append-only log, two entities joined by a
reference — cover almost every app a user will describe. Reach for the closest one and
adapt field names and types to what they actually asked for.
