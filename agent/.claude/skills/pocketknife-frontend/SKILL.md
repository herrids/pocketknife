---
name: pocketknife-frontend
description: How to author a Pocketknife app's frontend as a polished React app against its generated typed client. Use when building the frontend for an already-validated manifest on the provided Vite + React + Tailwind + shadcn/ui scaffold.
---

# Authoring a Pocketknife frontend

You are building a real app — something that should feel as polished as an app a
person would install on their phone, not a developer's CRUD scaffold. You are not
starting from a blank directory: a complete **Vite + React + TypeScript + Tailwind +
shadcn/ui** project is already in your current directory, and it already builds. Your
job is to turn it into *this specific app*.

Two files describe the app you're building:

- `manifest.json` — the validated app manifest. Read it first: the entities, their
  fields, types, constraints (`required`, `min`/`max`, enum `values`, `reference`
  targets), the per-entity enabled operations, and the app's `name`/`emoji`.
- `src/client.ts` — a generated, typed TypeScript client: a `Row`/`CreateInput`/
  `UpdateInput`/`ListParams` type set and a CRUD sub-client per entity, plus a root
  `<AppId>Client`. Read it to learn the exact types and the exact methods available.

Read both, and skim the scaffold (`src/components/ui/`, `src/index.css`,
`src/lib/`), before writing anything.

## The one hard rule: go through the client, never around it

Every read and write goes through the client in `src/client.ts`. **Never write a raw
`fetch`, never hand-build a `/apps/...` URL, and never invent a field or method the
client doesn't expose.** The client is the only contract with the backend; if
something feels missing, the manifest needs to change — that is not a reason to bypass
the client.

Construct one client instance and share it:

```ts
import { ReadingTrackerClient } from "@/client";
export const client = new ReadingTrackerClient(); // baseUrl defaults to same-origin
```

Use the per-entity sub-client: `client.book.list()`, `client.book.create(...)`,
`client.book.get(id)`, `client.book.update(id, ...)`, `client.book.delete(id)`. **Only
the operations the manifest enabled exist** — if `update` isn't on the sub-client, the
entity has no edit; don't try to work around it. `list()` returns a
`ListResult<T>` (`.data`, `.total`, `.limit`, `.offset`).

The client throws **`ApiError`** (with `.message`, `.code`, `.status`) on any non-2xx
response. Catch it at every call site a user can trigger and surface `.message` as a
toast — never let it fail silently or crash the view.

## What "polished" means here (the quality bar)

This is the part the old version of this skill got wrong. Aim high:

- **A real app shell.** A clean header (app `name` + `emoji`, a light/dark toggle) and
  navigation across entities — `Tabs`, or a sidebar when there are several entities.
  Mobile-first: it must feel right at phone width and scale up gracefully.
- **One considered view per `read`-enabled entity.** A list/table or a card grid of
  rows via `.list()`, with create / edit / delete wired to whatever ops that entity
  allows. Create/edit happen in a `Dialog` with a real form, not a bare row of inputs.
- **Every state designed, not just the happy path:**
  - *Loading* → `Skeleton` placeholders shaped like the content (never a bare
    "Loading…").
  - *Empty* → a genuine empty state: an icon, a line of copy, and the primary action
    (e.g. "Add your first book").
  - *Error* → a `sonner` toast with the `ApiError` message; inline field errors for
    validation.
  - *Success* → a confirming toast and an immediate, optimistic-feeling list update.
- **Motion, used with restraint.** `framer-motion` for list item enter/exit and
  dialog/layout transitions. Subtle and quick (≈150–250ms); never gratuitous.
- **Visual craft.** Consistent spacing rhythm, a clear type hierarchy, rounded cards
  with soft borders/shadows, `lucide-react` icons used purposefully, comfortable empty
  space. It should look intentional.

## Use the scaffold — compose, don't rebuild

The scaffold gives you the materials. Use them instead of reinventing them.

- **Components** in `src/components/ui/`: `button`, `card`, `input`, `textarea`,
  `label`, `select`, `dialog`, `dropdown-menu`, `tabs`, `table`, `badge`, `skeleton`,
  and `sonner` (the `<Toaster />`). Import as `@/components/ui/button`, etc. Need
  another shadcn/ui component? Add it in the same file style and token vocabulary —
  the underlying Radix primitives are installed.
- **Styling = semantic tokens only.** Style with Tailwind classes bound to the design
  tokens (`bg-background`, `text-foreground`, `bg-primary`, `text-muted-foreground`,
  `border`, `bg-card`, …). **Never hard-code hex colors.** To give the app its own
  accent, change `--primary` / `--ring` / `--accent` in `src/index.css` once — pick a
  hue that fits the app's purpose. Light + dark are both defined; keep both working.
- **Dark mode** is ready: `useTheme()` from `@/lib/use-theme` toggles and persists it.
  Wire it to a header button (a `Sun`/`Moon` icon).
- **`cn()`** from `@/lib/utils` merges conditional class lists.
- **Toaster**: keep one `<Toaster />` mounted at the app root (it's already in
  `App.tsx`); call `toast.success(...)` / `toast.error(...)` from `sonner` anywhere.

## Reflect the manifest faithfully in forms

- `required` fields: marked, and validated before submit.
- `enum` fields: a `Select` constrained to the declared `values` (humanize the labels).
- `integer` / `real` / `text` with `min`/`max`: enforce them on the input and validate.
- `boolean`: a checkbox or switch.
- `datetime`: a sensible date/time input; display values formatted for humans, not raw
  ISO strings.
- `reference` fields: a picker (a `Select`, searchable if the target list is large)
  populated from the target entity's `.list()`, showing a human label, submitting the
  id. Resolve references to readable labels when displaying rows, too.

## Structure & engineering

- Organize under `src/`: e.g. `src/lib/client.ts` (the shared instance),
  `src/hooks/` (a small data layer — `useList`, create/update/delete helpers over the
  client, owning loading/error state and optimistic updates), `src/components/<entity>/`
  (per-entity list + form), and a composed `src/App.tsx`.
- **It must pass `tsc` and `vite build`** — that gate runs after you finish, and a
  broken build is rejected. Use the client's generated types; **no `any`**, no unused
  throwaways that break strict mode. The generated input types already encode which
  fields are optional — let them guide your forms.
- **Accessibility**: the Radix-based components are accessible by default — keep
  `Label`s tied to inputs, give icon-only buttons an `aria-label`/`sr-only` text, keep
  dialogs labelled. Don't strip this away.
- Set the document `<title>` in `index.html` to the app's name.

## Boundaries — what not to touch

- Don't move, rename, or edit `src/client.ts` (it's generated — import from `@/client`).
- Don't edit build config (`vite.config.ts`, `tsconfig.json`, `package.json`,
  `tailwind.config.ts`) unless genuinely required; you should rarely need to. You add
  new dependencies by editing `package.json`, but prefer what's already installed — the
  scaffold already covers icons, motion, components, and styling.
- Write only inside the current directory (a hook enforces this).

When you're done, give a one-paragraph summary of the app you built.
