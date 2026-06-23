# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Pocketknife is a single, generic, schema-driven HTTP backend written in Go. One server turns a declarative **manifest** (`apps/<app_id>/manifest.json`) into a working API + SQLite database — no per-app code generation, no per-app process. `README.md` is the authoritative spec for the v1 runtime (manifest format, type set, HTTP API, query syntax, error envelope); read it for those details rather than re-deriving them. Note that the README's "Deferred / out of scope for v1" list is **stale** — migrations, a typed client, sandboxed functions, a model broker, and frontend serving are now all implemented (see Architecture).

## Commands

```sh
make test                 # full suite: go test ./...
make build                # build bin/pocketknife
make run                  # serve apps/ on :8080
make vet                  # go vet ./...
make fmt                  # go fmt ./...
make clean                # rm bin/, delete all apps/**/data.db

go test ./migrate/...                       # one package
go test ./migrate/ -run TestApply           # one test (regex on name)
go test ./... -run TestX -v                 # verbose single test across packages
```

Go must be on PATH. If absent (Homebrew is unavailable on this machine — see global guidance), install the official tarball into a user dir: `curl -fsSL https://go.dev/dl/go1.26.4.darwin-arm64.tar.gz | tar -C ~/.local -xz` then `export PATH="$HOME/.local/go/bin:$PATH"`.

### The binary's three modes (`cmd/pocketknife/main.go`)

```sh
./bin/pocketknife -addr :8080 -apps apps [-cors] [-platform-db platform.db]   # serve (default, no subcommand)
./bin/pocketknife migrate -app <id> -to <new.json> [-confirm] [-witnesses w.json]  # evolve schema, no data loss
./bin/pocketknife build   -app <id> [-to <new.json>] [-confirm] [-witnesses w.json] # rebuild/activate frontend, or full second deploy
```

## Architecture

The non-negotiable invariants (stable IDs as the spine, one SQLite file per app, manifests-on-disk as source of truth, validation as a hard gate, automatic platform columns, parameterized SQL, closed type set, determinism) are documented in `README.md` under "Contract / invariants" — **treat them as binding constraints on any change.**

### Request/boot path (the v1 core)

`schema/` (manifest types + parser → schema model) → `validate/` (JSON-Schema structural + semantic checks; the hard gate) → `materialize/` (schema → idempotent SQLite DDL) → `store/` (per-app connections, parameterized queries, stable-id-keyed columns) → `api/` (one generic CRUD/list handler set, query parser, error envelope) → `registry/` (in-memory app registry; `registry.Load` rebuilds it from disk on every boot). The manifest's canonical JSON Schema is `manifest.schema.json`, embedded into the binary via `schema_embed.go`.

### Migration engine (`migrate/`)

Evolves one app from its on-disk manifest to a new version without data loss. Pipeline: `Diff` (pure structural diff, matched **entirely by stable id** — same id + new name = rename moving no data) → `Classify` (labels each op `ClassSafe` or `ClassDestructive` purely from structure; never trusts a caller hint; ambiguous → destructive) → require explicit `-confirm` + `Witness`es for destructive ops → `snapshot` → `Execute` (one transaction via `store.RunMigration`). A **`Witness`** is the closed, declarative vocabulary (coerce / backfill / remap) a destructive op must supply — there is no arbitrary-code hook. On any execution failure: restore the snapshot, keep the prior registration.

### Build & activation (`build/`)

`build.Deploy` is the one entry point for both: `Kind=install` (build + activate a frontend for the current manifest) and `Kind=deploy` (a "second deploy" — data migration + frontend rebuild + activation as one operation with a single rollback contract). Ordering: snapshot data → migrate → build frontend → activate; any failure rolls back to the prior good manifest, snapshot, and activated build. Build-job state and the durable activation pointer live in a separate **platform database** (`platform.db`, distinct from per-app `data.db`s). `build.Reconcile` runs on every boot to fail interrupted jobs and reattach active builds. `build.NewStatusServer` serves read-only job/activation status at `/builds/`.

### Sandboxed functions (`sandbox/`, `broker/`, `consent/`)

`sandbox/` is the **real** security boundary (the manifest only *declares* capabilities). Function bodies run as adversarial WebAssembly under wazero with no filesystem, no env, no raw network — the only way out is a fixed, capability-checked host ABI (the `pocketknife` host module in `host.go`). Resource limits (memory pages, wall-clock timeout, input/output byte caps) are enforced per invocation. The three gated host calls (`data_call`, `network_fetch`, `model_call`) return sentinel codes; a `codeDenied` carries no payload so a function can't use responses as an oracle. `broker/` is the **only** path to a model provider — the provider token is read once from env, held unexported, and never reaches a function or the browser. `consent/` derives the union of capabilities an app's functions request (pure function of the manifest), for a future shell to render.

### Frontend serving (`assets/`, `client/`, `cors/`)

`assets.NewServer` serves each app's activated frontend bundle at `/ui/{app}/...` from one origin (resolved fresh from the registry per request, so activation cutover/rollback is visible to the next request with no restart; SPA fallback to the entry file). `client/Generate` renders a typed TypeScript client as a pure function of the schema model (byte-identical output for an unchanged manifest). `cors/` is dev-only middleware (`-cors`), off in production since API + UI share an origin.

## Workflow conventions

- This repo uses **OpenSpec** (`openspec/`, `schema: spec-driven`). Changes are proposed as specs under `openspec/changes/<date>-<name>/` (proposal, design, tasks, per-capability specs) and moved to `openspec/changes/archive/` when complete; long-lived capability specs live in `openspec/specs/`. The `openspec-*` / `opsx:*` skills drive this flow.
- Each Go package leads with a substantial doc comment stating its responsibility and security posture — read it before editing; match that altitude when adding code.
- `test_project_hub.sh` is an end-to-end shell exercise against a running server.
