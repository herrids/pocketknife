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
  "functions": [ /* optional — prompt functions give the app AI behaviour, see below */ ]
}
```

- `app.id`, `app.name`, `app.version` are required. `version` starts at `1`. `emoji` is
  optional, cosmetic.
- `entities` must have at least one entry.
- `frontend` is populated later by the platform / the builder stage — as the planner,
  you emit a manifest **without** it.
- `functions` is yours to author **when the user wants AI behaviour** (summarize,
  rewrite, classify, suggest, draft…). Emit a *prompt function* for each such feature —
  see "Prompt functions" below. Never emit a wasm function (one with an `entry` key);
  those require a pre-compiled module you cannot produce.

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

## Prompt functions — giving an app AI behaviour

A **prompt function** is a declarative LLM call: a prompt template the server renders
and sends to the platform's model broker. No code, no API key in the app — the frontend
calls it through the generated client and gets text back. Whenever the user asks for an
AI-flavoured feature ("summarize my notes", "suggest a reply", "categorize this"),
model it as one prompt function per feature.

```json
{
  "id": "fn_summarize",
  "name": "summarize",
  "prompt": "Summarize the following note in a {{tone}} tone. Reply with only the summary.\n\nNote:\n{{text}}",
  "description": "Summarizes a note.",
  "capabilities": { "model": true }
}
```

The contract, all enforced by `validate_manifest`:

- `id` (convention `fn_<name>`), `name` (machineName, unique among functions), `prompt`
  and `capabilities` are required. `description` is optional and becomes the doc comment
  on the generated client method — write one.
- `capabilities` must be **exactly** `{ "model": true }`. A prompt function can hold no
  other power — no `data` scopes, no `network` domains, no `entry`.
- `{{param}}` placeholders (machineName rule: `^[a-z][a-z0-9_]*$`) become the required
  string parameters of the generated client method, in order of first appearance. A
  repeated placeholder is fine; a template with no placeholders is fine (a static
  prompt). Any other `{{` in the prompt is a validation error — there is no escaping.
- The function's inputs are strings the **frontend** passes in (e.g. the note text the
  user selected). The function cannot read the database itself — design the frontend to
  fetch the rows it needs via the entity client and interpolate them into the call.
- Write the prompt like a good instruction: say what to produce, constrain the output
  shape ("Reply with only the summary."), and put user-supplied content after the
  instruction, clearly delimited.

**Adding a function to an existing app requires a version bump.** A redeploy that keeps
`app.version` unchanged never re-reads the manifest, so the new function would be
ignored. When you add or change functions in update mode, increment `app.version` like
any other schema change.

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

### 4. A tracker with an AI feature (prompt function)

```json
{
  "app": { "id": "journal", "name": "Journal", "emoji": "📓", "version": 1 },
  "entities": [
    {
      "id": "ent_entry",
      "name": "entry",
      "fields": [
        { "id": "fld_text",     "name": "text",     "type": "text", "required": true },
        { "id": "fld_mood",     "name": "mood",     "type": "enum", "values": ["low", "ok", "great"] }
      ]
    }
  ],
  "functions": [
    {
      "id": "fn_reflect",
      "name": "reflect",
      "prompt": "Read this journal entry and reply with one short, kind reflection question about it. Reply with only the question.\n\nEntry:\n{{text}}",
      "description": "Suggests a reflection question for a journal entry.",
      "capabilities": { "model": true }
    }
  ]
}
```

These four shapes — single tracker, append-only log, two entities joined by a
reference, and a tracker with an AI feature — cover almost every app a user will
describe. Reach for the closest one and adapt field names and types to what they
actually asked for.
