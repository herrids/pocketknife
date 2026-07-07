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
| `text`      | UTF-8 string           | `required`, `unique`, `default`, `min`/`max` (length)  | a product name, a recipe's ingredient list |
| `integer`   | 64-bit whole number    | `required`, `unique`, `default`, `min`/`max` (value)   | a quantity in stock, a table's seat count |
| `real`      | floating point number  | `required`, `unique`, `default`, `min`/`max` (value)   | a price, a package's weight |
| `boolean`   | true/false             | `required`, `default`                                | an "in stock" flag |
| `datetime`  | ISO-8601 UTC instant   | `required`, `default`                                | a due date, a reservation's start time |
| `enum`      | one of a fixed string set | `required`, `default`, `values` (required, non-empty) | a priority, a status |
| `reference` | points at another entity's row | `required`, `target` (required, an entity id), `onDelete` | a reservation's room, a comment's parent comment |

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
  `["create", "read", "update", "delete"]` — use it to restrict what's possible on that
  entity. For example, `["create", "read"]` makes an entity append-only (no edits, no
  deletes) — useful for an entity that's a record of things that happened, like a
  journal entry or an audit trail — while `["read"]` alone makes it read-only. Only use
  this when the user's domain actually calls for the restriction; most entities want all
  four.

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

These examples exist to show what the schema language can *express* — one-to-many,
many-to-many, self-reference, mixed operation subsets, enums, uniqueness — not to hand
you a menu of app shapes to pick from. A real user's app is its own domain: derive its
entities, fields, and relationships from what they actually described. Real domains vary
far more than these three examples — inventory systems, marketplaces, habit trackers,
recipe boxes, expense splitters, room bookings, and many others each have their own
nouns, relationships, and rules. Model *that* domain, don't reshape it to fit an example
below.

If a user's request is genuinely a single flat list with no relationships (e.g. "let me
jot down quick notes with a timestamp"), a one-entity manifest is the right and honest
answer — but reach that conclusion because the domain is that simple, not because it's
the easiest shape to imitate.

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

### 2. Many-to-many via a junction entity

A `reservation` sits between `guest` and `room`, referencing both — the standard way to
model a many-to-many relationship (a guest can have many reservations, a room can have
many reservations) plus its own fields (a time range, a status):

```json
{
  "app": { "id": "room_booking", "name": "Room Booking", "emoji": "🗓️", "version": 1 },
  "entities": [
    {
      "id": "ent_guest",
      "name": "guest",
      "fields": [
        { "id": "fld_name", "name": "name", "type": "text", "required": true }
      ]
    },
    {
      "id": "ent_room",
      "name": "room",
      "fields": [
        { "id": "fld_label", "name": "label", "type": "text", "required": true, "unique": true }
      ]
    },
    {
      "id": "ent_reservation",
      "name": "reservation",
      "fields": [
        { "id": "fld_guest", "name": "guest", "type": "reference", "target": "ent_guest", "onDelete": "cascade" },
        { "id": "fld_room",  "name": "room",  "type": "reference", "target": "ent_room",  "onDelete": "cascade" },
        { "id": "fld_starts_at", "name": "starts_at", "type": "datetime", "required": true },
        { "id": "fld_ends_at",   "name": "ends_at",   "type": "datetime", "required": true },
        {
          "id": "fld_status", "name": "status", "type": "enum",
          "values": ["pending", "confirmed", "cancelled"], "default": "pending"
        }
      ]
    }
  ]
}
```

### 3. Self-reference and a plain one-to-many

A `category` can nest under a parent category — a `reference` field whose `target` is
its own entity. `product` is an ordinary one-to-many off of `category`:

```json
{
  "app": { "id": "catalog", "name": "Catalog", "emoji": "🗂️", "version": 1 },
  "entities": [
    {
      "id": "ent_category",
      "name": "category",
      "fields": [
        { "id": "fld_name", "name": "name", "type": "text", "required": true, "unique": true },
        {
          "id": "fld_parent", "name": "parent", "type": "reference",
          "target": "ent_category", "onDelete": "set_null"
        }
      ]
    },
    {
      "id": "ent_product",
      "name": "product",
      "fields": [
        { "id": "fld_name", "name": "name", "type": "text", "required": true },
        { "id": "fld_price", "name": "price", "type": "real", "min": 0 },
        {
          "id": "fld_category", "name": "category", "type": "reference",
          "target": "ent_category", "onDelete": "set_null"
        }
      ]
    }
  ]
}
```

Not every domain needs relationships at all, and not every relationship is one of these
two shapes — a domain might need several one-to-many chains, a junction entity with its
own fields, or none of the above. Use these to recognize the pattern when the user's
domain calls for it, not to force their domain into one of them.
